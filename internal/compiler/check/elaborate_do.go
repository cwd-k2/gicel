package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Do elaboration files:
//   elaborate_do.go          — doElaborator, shared helpers, unified elaboration loop
//   elaborate_do_monadic.go  — checkDo dispatch, IxMonad helpers (extractMonadHead, mkIxBind, etc.)
//   grade.go                 — grade boundary check (checkGradeBoundary)

// --- doElaborator: unified do-block elaboration ---

// doMode selects which elaboration strategy a doElaborator uses.
type doMode int

const (
	doModeInfer   doMode = iota // fresh metas for pre/post; returns inferred type
	doModeChecked               // threads known pre/post from TyCBPV
	doModeMonadic               // IxMonad class dispatch via dictionary
	doModeGraded                // GIMonad class dispatch via dictionary (grade-aware)
)

// doElaborator parameterizes the differences between the three do elaboration paths.
// The statement dispatch loop (StmtBind/StmtPureBind/StmtExpr) is shared; mode-specific
// behavior is confined to bind construction, type extraction, and base-case handling.
type doElaborator struct {
	ch   *Checker
	mode doMode

	// checked mode: threading state.
	comp *types.TyCBPV

	// monadic/graded mode: class dispatch parameters.
	monadHead types.Type
	expected  types.Type

	// infer mode: post-state from the preceding statement.
	// Used to pre-resolve comp pre-states before pattern matching,
	// preventing pattern-bound variable metas from leaking into
	// the state position (the "put a after pattern bind" bug).
	lastPost types.Type
}

// errPair returns a mode-appropriate error pair.
func (d *doElaborator) errPair(s span.Span) (types.Type, ir.Core) {
	errCore := &ir.Error{S: s}
	switch d.mode {
	case doModeChecked:
		return d.comp, errCore
	case doModeMonadic, doModeGraded:
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
		if name, ok := syntax.PatVarName(st.Pat); ok {
			return d.elaborateBind(name, st.Comp, stmts[1:], st.S, s)
		}
		return d.elaboratePatternBind(st.Pat, st.Comp, stmts[1:], st.S, s)

	case *syntax.StmtPureBind:
		if _, ok := syntax.PatVarName(st.Pat); ok {
			var restTy types.Type
			c := ch.elaboratePureBind(st, func() ir.Core {
				var rc ir.Core
				restTy, rc = d.elaborate(stmts[1:], s)
				return rc
			})
			return restTy, c
		}
		return d.elaboratePatternPureBind(st.Pat, st.Expr, stmts[1:], st.S, s)

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
			if app, ok := d.expected.(*types.TyApp); ok {
				valCore := ch.check(pureVal, app.Arg)
				return d.expected, ch.mkIxPure(d.monadHead, valCore, s)
			}
		}
		return d.expected, ch.check(expr, d.expected)
	case doModeGraded:
		// Intercept `pure val` / `gipure val` at the end of a graded do block.
		if pureVal := extractPureArg(expr); pureVal != nil {
			if app, ok := d.expected.(*types.TyApp); ok {
				valCore := ch.check(pureVal, app.Arg)
				return d.expected, ch.mkGIPure(d.monadHead, valCore, s)
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
	case doModeGraded:
		return d.gradedBind(varName, comp, rest, stmtS, doS)
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
	case doModeGraded:
		return d.gradedExprStmt(expr, rest, stmtS, doS)
	}
	return d.errPair(stmtS)
}

// --- Infer mode ---

func (d *doElaborator) inferBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch
	compTy, compCore := ch.infer(comp)
	compTy = ch.unifier.Zonk(compTy)

	// Pre-resolve: if the preceding statement's post-state is known,
	// unify it with this comp's pre-state BEFORE elaborating rest.
	// This resolves result-type metas before pattern matching, preventing
	// pattern-bound variable metas from leaking into the state position.
	if d.lastPost != nil {
		if inferredComp, ok := compTy.(*types.TyCBPV); ok {
			ch.emitEq(d.lastPost, inferredComp.Pre, stmtS, nil)
			compTy = ch.unifier.Zonk(compTy)
		}
	}

	resultTy := ch.extractCompResult(compTy, stmtS)

	// Track post-state for the next statement.
	var savedPost types.Type
	if inferredComp, ok := compTy.(*types.TyCBPV); ok {
		savedPost = d.lastPost
		d.lastPost = inferredComp.Post
	}

	ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
	restTy, restCore := d.elaborate(rest, doS)
	ch.ctx.Pop()

	if savedPost != nil || d.lastPost != nil {
		d.lastPost = savedPost
	}

	// Thread post-state: unify comp's post with rest's pre so that
	// type information (e.g. state type from put) propagates to
	// variables bound by get in subsequent statements.
	d.unifyCompPostPre(compTy, restTy, stmtS)
	return restTy, &ir.Bind{Comp: compCore, Var: varName, Discard: varName == "_", Body: restCore, S: stmtS}
}

func (d *doElaborator) inferExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch
	compTy, compCore := ch.infer(expr)
	compTy = ch.unifier.Zonk(compTy)

	// Track post-state for the next statement's pre-resolve.
	var savedPost types.Type
	if inferredComp, ok := compTy.(*types.TyCBPV); ok {
		savedPost = d.lastPost
		d.lastPost = inferredComp.Post
	}

	restTy, restCore := d.elaborate(rest, doS)

	if savedPost != nil || d.lastPost != nil {
		d.lastPost = savedPost
	}

	// Thread post-state: see inferBind comment.
	d.unifyCompPostPre(compTy, restTy, stmtS)
	return restTy, &ir.Bind{Comp: compCore, Var: "_", Discard: true, Body: restCore, S: stmtS}
}

// unifyCompPostPre unifies the post-state of compTy with the pre-state of
// restTy. This propagates type information through capability rows in
// infer-mode do-blocks (e.g. put sets state to Int, get's result becomes Int).
func (d *doElaborator) unifyCompPostPre(compTy, restTy types.Type, s span.Span) {
	ch := d.ch
	compTy = ch.unifier.Zonk(compTy)
	restTy = ch.unifier.Zonk(restTy)
	compComp, ok1 := compTy.(*types.TyCBPV)
	restComp, ok2 := restTy.(*types.TyCBPV)
	if ok1 && ok2 {
		ch.emitEq(compComp.Post, restComp.Pre, s, nil)
	}
}

// --- Pattern bind (all modes) ---

// elaboratePatternBind handles pat <- comp; rest for irrefutable patterns.
// Desugars to: $fresh <- comp; case $fresh { pat => rest }
func (d *doElaborator) elaboratePatternBind(pat syntax.Pattern, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	freshName := d.ch.freshName("$p")
	freshPat := &syntax.PatVar{Name: freshName, S: pat.Span()}
	// Rewrite as: $fresh <- comp; case $fresh { pat => rest... }
	freshBind := &syntax.StmtBind{Pat: freshPat, Comp: comp, S: stmtS}
	caseStmt := &syntax.StmtExpr{
		Expr: &syntax.ExprCase{
			Scrutinee: &syntax.ExprVar{Name: freshName, S: stmtS},
			Alts:      []syntax.AstAlt{{Pattern: pat, Body: stmtsToDoExpr(rest, doS), S: stmtS}},
			S:         stmtS,
		},
		S: stmtS,
	}
	return d.elaborate([]syntax.Stmt{freshBind, caseStmt}, doS)
}

// elaboratePatternPureBind handles pat := expr; rest for irrefutable patterns.
// Desugars to: case expr { pat => rest }
func (d *doElaborator) elaboratePatternPureBind(pat syntax.Pattern, expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	caseExpr := &syntax.ExprCase{
		Scrutinee: expr,
		Alts:      []syntax.AstAlt{{Pattern: pat, Body: stmtsToDoExpr(rest, doS), S: stmtS}},
		S:         stmtS,
	}
	stmts := []syntax.Stmt{&syntax.StmtExpr{Expr: caseExpr, S: stmtS}}
	return d.elaborate(stmts, doS)
}

// stmtsToDoExpr wraps remaining do-block statements as a do expression.
func stmtsToDoExpr(stmts []syntax.Stmt, s span.Span) syntax.Expr {
	return &syntax.ExprDo{Stmts: stmts, S: s}
}

// --- Checked mode ---

func (d *doElaborator) checkedBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span, errLabel string) (types.Type, ir.Core) {
	ch := d.ch
	isBind := varName != "_"

	compTy, compCore := ch.infer(comp)
	compTy = ch.unifier.Zonk(compTy)

	if inferredComp, ok := compTy.(*types.TyCBPV); ok {
		// Emit equality constraint: inferred pre vs expected pre.
		ch.emitEq(inferredComp.Pre, d.comp.Pre, stmtS, solve.WithLazyContext(0, func() string {
			return errLabel + ": pre-state mismatch: expected " + types.Pretty(d.comp.Pre) + ", got " + types.Pretty(inferredComp.Pre)
		}))

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
		return d.comp, &ir.Bind{Comp: compCore, Var: varName, Discard: !isBind, Body: restCore, S: stmtS}
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
	ch.emitEq(restTy, d.comp, stmtS, nil)
	return d.comp, &ir.Bind{Comp: compCore, Var: varName, Discard: !isBind, Body: restCore, S: stmtS}
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
// Caller must ensure st.Pat is a simple PatVar or PatWild.
func (ch *Checker) elaboratePureBind(st *syntax.StmtPureBind, rest func() ir.Core) ir.Core {
	name, _ := syntax.PatVarName(st.Pat)
	bindTy, bindCore := ch.infer(st.Expr)
	ch.ctx.Push(&CtxVar{Name: name, Type: bindTy})
	restCore := rest()
	ch.ctx.Pop()
	return &ir.App{
		Fun: &ir.Lam{Param: name, Body: restCore, S: st.S},
		Arg: bindCore,
		S:   st.S,
	}
}

func (ch *Checker) inferBlock(e *syntax.ExprBlock) (types.Type, ir.Core) {
	// Desugar: { x := e1; body } → App(Lam(x, body), e1)
	// Pattern binds: { (a,b) := e1; body } → case e1 { (a,b) => body }
	// Forward pass: infer each binding, add to context.
	type bindInfo struct {
		pat  syntax.Pattern
		ty   types.Type
		core ir.Core
		pr   *patternResult // non-nil for pattern binds
		s    span.Span
	}
	binds := make([]bindInfo, len(e.Binds))
	for i, bind := range e.Binds {
		bindTy, bindCore := ch.infer(bind.Expr)
		if name, ok := syntax.PatVarName(bind.Pat); ok {
			binds[i] = bindInfo{pat: bind.Pat, ty: bindTy, core: bindCore, s: bind.S}
			ch.ctx.Push(&CtxVar{Name: name, Type: bindTy})
		} else {
			pr := ch.checkPattern(bind.Pat, bindTy)
			binds[i] = bindInfo{pat: bind.Pat, ty: bindTy, core: bindCore, pr: &pr, s: bind.S}
			for bname, bty := range pr.Bindings {
				ch.ctx.Push(&CtxVar{Name: bname, Type: bty})
			}
		}
	}

	// Infer body with all bindings in scope.
	if e.Body == nil {
		ch.addCodedError(diagnostic.ErrEmptyDo, e.S, "block must end with an expression")
		for _, b := range binds {
			if b.pr != nil {
				for range b.pr.Bindings {
					ch.ctx.Pop()
				}
			} else {
				ch.ctx.Pop()
			}
		}
		return &types.TyError{S: e.S}, &ir.Lit{Value: nil, S: e.S}
	}
	resultTy, result := ch.infer(e.Body)

	// Pop all bindings.
	for _, b := range binds {
		if b.pr != nil {
			for range b.pr.Bindings {
				ch.ctx.Pop()
			}
		} else {
			ch.ctx.Pop()
		}
	}

	// Backward pass: build Core IR desugaring.
	for i := len(binds) - 1; i >= 0; i-- {
		b := binds[i]
		if b.pr != nil {
			// Pattern bind: case expr { pat => body }
			result = &ir.Case{
				Scrutinee: b.core,
				Alts:      []ir.Alt{{Pattern: b.pr.Pattern, Body: result, S: b.s}},
				S:         b.s,
			}
		} else {
			name, _ := syntax.PatVarName(b.pat)
			lam := &ir.Lam{Param: name, Body: result, S: b.s}
			result = &ir.App{Fun: lam, Arg: b.core, S: b.s}
		}
	}

	return resultTy, result
}

func (ch *Checker) extractCompResult(ty types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	if comp, ok := ty.(*types.TyCBPV); ok {
		return comp.Result
	}
	// Try to unify with a fresh Computation.
	pre := ch.freshMeta(types.TypeOfRows)
	post := ch.freshMeta(types.TypeOfRows)
	result := ch.freshMeta(types.TypeOfTypes)
	expected := types.MkComp(pre, post, result)
	if err := ch.unifier.Unify(ty, expected); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadComputation, err, s, "expected computation type, got "+types.Pretty(ty))
		return &types.TyError{S: s}
	}
	return result
}
