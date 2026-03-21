package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Do elaboration files:
//   elaborate_do.go          — doElaborator, shared helpers, unified elaboration loop
//   elaborate_do_monadic.go  — checkDo dispatch, IxMonad helpers (extractMonadHead, mkIxBind, etc.)
//   elaborate_do_mult.go     — multiplicity enforcement (checkMultiplicity)

// --- doElaborator: unified do-block elaboration ---

// doMode selects which elaboration strategy a doElaborator uses.
type doMode int

const (
	doModeInfer   doMode = iota // fresh metas for pre/post; returns inferred type
	doModeChecked               // threads known pre/post from TyCBPV; records multSteps
	doModeMonadic               // IxMonad class dispatch via dictionary
)

// doElaborator parameterizes the differences between the three do elaboration paths.
// The statement dispatch loop (StmtBind/StmtPureBind/StmtExpr) is shared; mode-specific
// behavior is confined to bind construction, type extraction, and base-case handling.
type doElaborator struct {
	ch   *Checker
	mode doMode

	// checked mode: threading state and multiplicity recording.
	comp  *types.TyCBPV
	steps *[]multStep

	// monadic mode: IxMonad dispatch parameters.
	monadHead types.Type
	expected  types.Type
}

// errPair returns a mode-appropriate error pair.
func (d *doElaborator) errPair(s span.Span) (types.Type, ir.Core) {
	errCore := &ir.Var{Name: "<error>", S: s}
	switch d.mode {
	case doModeChecked:
		return d.comp, errCore
	case doModeMonadic:
		return d.expected, errCore
	default:
		return &types.TyError{S: s}, errCore
	}
}

// elaborate processes a do-block statement sequence.
// All three modes share the same structural loop; differences are in
// base-case handling and bind construction.
func (d *doElaborator) elaborate(stmts []syntax.Stmt, s span.Span) (types.Type, ir.Core) {
	ch := d.ch

	// Base case: single statement must be an expression.
	if len(stmts) == 1 {
		if ch.rejectDoEnding(stmts[0]) {
			return d.errPair(stmts[0].Span())
		}
		st := stmts[0].(*syntax.StmtExpr)
		return d.elaborateBase(st.Expr, st.S)
	}

	// Recursive case: dispatch on first statement.
	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		return d.elaborateBind(st.Var, st.Comp, stmts[1:], st.S, s)

	case *syntax.StmtPureBind:
		var restTy types.Type
		c := ch.elaboratePureBind(st, func() ir.Core {
			var rc ir.Core
			restTy, rc = d.elaborate(stmts[1:], s)
			return rc
		})
		return restTy, c

	case *syntax.StmtExpr:
		return d.elaborateExprStmt(st.Expr, stmts[1:], st.S, s)

	default:
		ch.addCodedError(diagnostic.ErrBadComputation, s, "unexpected statement in do block")
		return d.errPair(s)
	}
}

// elaborateBase handles the last expression in a do block.
func (d *doElaborator) elaborateBase(expr syntax.Expr, s span.Span) (types.Type, ir.Core) {
	ch := d.ch
	switch d.mode {
	case doModeInfer:
		return ch.infer(expr)
	case doModeChecked:
		return d.comp, ch.check(expr, d.comp)
	case doModeMonadic:
		// Intercept `pure val` / `ixpure val` at the end of a monadic do block.
		if pureVal := extractPureArg(expr); pureVal != nil {
			_, args := types.UnwindApp(d.expected)
			if len(args) > 0 {
				resultTy := args[len(args)-1]
				valCore := ch.check(pureVal, resultTy)
				return d.expected, ch.mkIxPure(d.monadHead, valCore, s)
			}
		}
		return d.expected, ch.check(expr, d.expected)
	}
	return d.errPair(s)
}

// elaborateBind handles x <- comp; rest.
func (d *doElaborator) elaborateBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	switch d.mode {
	case doModeInfer:
		return d.inferBind(varName, comp, rest, stmtS, doS)
	case doModeChecked:
		return d.checkedBind(varName, comp, rest, stmtS, doS, "do bind")
	case doModeMonadic:
		return d.monadicBind(varName, comp, rest, stmtS, doS)
	}
	return d.errPair(stmtS)
}

// elaborateExprStmt handles comp; rest (expression statement, no binding variable).
func (d *doElaborator) elaborateExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	switch d.mode {
	case doModeInfer:
		return d.inferExprStmt(expr, rest, stmtS, doS)
	case doModeChecked:
		return d.checkedBind("_", expr, rest, stmtS, doS, "do statement")
	case doModeMonadic:
		return d.monadicExprStmt(expr, rest, stmtS, doS)
	}
	return d.errPair(stmtS)
}

// --- Infer mode ---

func (d *doElaborator) inferBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch
	compTy, compCore := ch.infer(comp)
	resultTy := ch.extractCompResult(compTy, stmtS)
	ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
	restTy, restCore := d.elaborate(rest, doS)
	ch.ctx.Pop()
	return restTy, &ir.Bind{Comp: compCore, Var: varName, Body: restCore, S: stmtS}
}

func (d *doElaborator) inferExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch
	_, compCore := ch.infer(expr)
	restTy, restCore := d.elaborate(rest, doS)
	return restTy, &ir.Bind{Comp: compCore, Var: "_", Body: restCore, S: stmtS}
}

// --- Checked mode ---

func (d *doElaborator) checkedBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span, errLabel string) (types.Type, ir.Core) {
	ch := d.ch
	isBind := varName != "_"

	compTy, compCore := ch.infer(comp)
	compTy = ch.unifier.Zonk(compTy)

	if inferredComp, ok := compTy.(*types.TyCBPV); ok {
		// Record step for multiplicity analysis.
		*d.steps = append(*d.steps, multStep{pre: inferredComp.Pre, post: inferredComp.Post, s: stmtS})
		// Unify inferred pre with expected pre.
		if err := ch.unifier.Unify(inferredComp.Pre, d.comp.Pre); err != nil {
			ch.addUnifyError(err, stmtS, fmt.Sprintf(
				"%s: pre-state mismatch: expected %s, got %s",
				errLabel, types.Pretty(d.comp.Pre), types.Pretty(inferredComp.Pre)))
		}

		if isBind {
			ch.ctx.Push(&CtxVar{Name: varName, Type: inferredComp.Result})
		}

		// Rest: Computation mid post result — mid from inferred post, post/result from expected.
		restComp := &types.TyCBPV{Tag: types.TagComp, Pre: inferredComp.Post, Post: d.comp.Post, Result: d.comp.Result, S: d.comp.S}
		savedComp := d.comp
		d.comp = restComp
		_, restCore := d.elaborate(rest, doS)
		d.comp = savedComp

		if isBind {
			ch.ctx.Pop()
		}
		return d.comp, &ir.Bind{Comp: compCore, Var: varName, Body: restCore, S: stmtS}
	}

	// Fallback: infer didn't give TyCBPV, extract result and continue with infer mode.
	if isBind {
		resultTy := ch.extractCompResult(compTy, stmtS)
		ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
	}
	fallback := &doElaborator{ch: ch, mode: doModeInfer}
	restTy, restCore := fallback.elaborate(rest, doS)
	if isBind {
		ch.ctx.Pop()
	}
	// Advisory unification: pre/post threading is unavailable in infer-fallback.
	// Failure here is expected when the do-block mixes monadic and pure branches;
	// the downstream subsumption check will report the actual type mismatch.
	_ = ch.unifier.Unify(restTy, d.comp) //nolint:errcheck // advisory
	return d.comp, &ir.Bind{Comp: compCore, Var: varName, Body: restCore, S: stmtS}
}

// --- Monadic mode ---

func (d *doElaborator) monadicBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch

	var compCore ir.Core
	var resultTy types.Type

	// Intercept `x <- pure val` / `x <- ixpure val`.
	if pureVal := extractPureArg(comp); pureVal != nil {
		rty, vc := ch.infer(pureVal)
		compCore = ch.mkIxPure(d.monadHead, vc, stmtS)
		resultTy = rty
	} else {
		compTy, cc := ch.infer(comp)
		compCore = cc
		resultTy = ch.extractMonadResult(compTy, d.monadHead, stmtS)
	}

	ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
	_, restCore := d.elaborate(rest, doS)
	ch.ctx.Pop()
	return d.expected, ch.mkIxBind(d.monadHead, compCore, varName, restCore, stmtS)
}

func (d *doElaborator) monadicExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch

	var compCore ir.Core
	// Intercept `pure val` / `ixpure val`.
	if pureVal := extractPureArg(expr); pureVal != nil {
		_, vc := ch.infer(pureVal)
		compCore = ch.mkIxPure(d.monadHead, vc, stmtS)
	} else {
		_, cc := ch.infer(expr)
		compCore = cc
	}

	_, restCore := d.elaborate(rest, doS)
	return d.expected, ch.mkIxBind(d.monadHead, compCore, "_", restCore, stmtS)
}

// --- Entry points ---

func (ch *Checker) inferDo(e *syntax.ExprDo) (types.Type, ir.Core) {
	if len(e.Stmts) == 0 {
		ch.addCodedError(diagnostic.ErrEmptyDo, e.S, "empty do block")
		return ch.errorPair(e.S)
	}
	d := &doElaborator{ch: ch, mode: doModeInfer}
	return d.elaborate(e.Stmts, e.S)
}

// --- Shared helpers ---

// rejectDoEnding reports ErrBadDoEnding if the last statement is a binding.
// Returns true if the statement was rejected.
func (ch *Checker) rejectDoEnding(st syntax.Stmt) bool {
	switch st.(type) {
	case *syntax.StmtBind, *syntax.StmtPureBind:
		ch.addCodedError(diagnostic.ErrBadDoEnding, st.Span(), "do block must end with an expression")
		return true
	}
	return false
}

// elaboratePureBind desugars x := e into App(Lam(x, rest), e).
// The binding is in scope for the duration of the rest callback.
func (ch *Checker) elaboratePureBind(st *syntax.StmtPureBind, rest func() ir.Core) ir.Core {
	bindTy, bindCore := ch.infer(st.Expr)
	ch.ctx.Push(&CtxVar{Name: st.Var, Type: bindTy})
	restCore := rest()
	ch.ctx.Pop()
	return &ir.App{
		Fun: &ir.Lam{Param: st.Var, Body: restCore, S: st.S},
		Arg: bindCore,
		S:   st.S,
	}
}

func (ch *Checker) inferBlock(e *syntax.ExprBlock) (types.Type, ir.Core) {
	// Desugar: { x := e1; body } → App(Lam(x, body), e1)
	// Forward pass: infer each binding, add to context.
	type bindInfo struct {
		name string
		ty   types.Type
		core ir.Core
		s    span.Span
	}
	binds := make([]bindInfo, len(e.Binds))
	for i, bind := range e.Binds {
		bindTy, bindCore := ch.infer(bind.Expr)
		binds[i] = bindInfo{name: bind.Var, ty: bindTy, core: bindCore, s: bind.S}
		ch.ctx.Push(&CtxVar{Name: bind.Var, Type: bindTy})
	}

	// Infer body with all bindings in scope.
	resultTy, result := ch.infer(e.Body)

	// Pop all bindings.
	for range e.Binds {
		ch.ctx.Pop()
	}

	// Backward pass: build Core IR desugaring.
	for i := len(binds) - 1; i >= 0; i-- {
		b := binds[i]
		lam := &ir.Lam{Param: b.name, Body: result, S: b.s}
		result = &ir.App{Fun: lam, Arg: b.core, S: b.s}
	}

	return resultTy, result
}

func (ch *Checker) extractCompResult(ty types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	if comp, ok := ty.(*types.TyCBPV); ok {
		return comp.Result
	}
	// Try to unify with a fresh Computation.
	pre := ch.freshMeta(types.KRow{})
	post := ch.freshMeta(types.KRow{})
	result := ch.freshMeta(types.KType{})
	expected := types.MkComp(pre, post, result)
	if err := ch.unifier.Unify(ty, expected); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadComputation, err, s, fmt.Sprintf("expected computation type, got %s", types.Pretty(ty)))
		return &types.TyError{S: s}
	}
	return result
}
