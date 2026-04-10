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
//   elaborate_do.go          — doStrategy interface, shared loop, doInfer, doChecked
//   elaborate_do_monadic.go  — checkDo dispatch, doGraded, GIMonad helpers

// --- doStrategy: polymorphic do-block elaboration ---

// doStrategy defines the four operations that differ between infer, checked,
// and graded do-block elaboration. The shared statement dispatch loop
// (doElaborate) calls these methods; each concrete type carries only the
// state relevant to its mode.
type doStrategy interface {
	errPair(s span.Span) (types.Type, ir.Core)
	elaborateBase(expr syntax.Expr, s span.Span) (types.Type, ir.Core)
	elaborateBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core)
	elaborateExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core)
}

// doElaborate processes a do-block statement sequence.
// Shared across all modes; dispatches to strat for mode-specific behavior.
func doElaborate(ch *Checker, strat doStrategy, stmts []syntax.Stmt, s span.Span) (types.Type, ir.Core) {
	// Base case: single statement must be an expression.
	if len(stmts) == 1 {
		if ch.rejectDoEnding(stmts[0]) {
			return strat.errPair(stmts[0].Span())
		}
		st := stmts[0].(*syntax.StmtExpr)
		return strat.elaborateBase(st.Expr, st.S)
	}

	// Recursive case: dispatch on first statement.
	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		if name, ok := syntax.PatVarName(st.Pat); ok {
			return strat.elaborateBind(name, st.Comp, stmts[1:], st.S, s)
		}
		return doPatternBind(ch, strat, st.Pat, st.Comp, stmts[1:], st.S, s)

	case *syntax.StmtPureBind:
		if _, ok := syntax.PatVarName(st.Pat); ok {
			var restTy types.Type
			c := ch.elaboratePureBind(st, func() ir.Core {
				var rc ir.Core
				restTy, rc = doElaborate(ch, strat, stmts[1:], s)
				return rc
			})
			return restTy, c
		}
		return doPatternPureBind(ch, strat, st.Pat, st.Expr, stmts[1:], st.S, s)

	case *syntax.StmtExpr:
		return strat.elaborateExprStmt(st.Expr, stmts[1:], st.S, s)

	default:
		ch.addDiag(diagnostic.ErrBadComputation, s, diagMsg("unexpected statement in do block"))
		return strat.errPair(s)
	}
}

// doPatternBind handles pat <- comp; rest for irrefutable patterns.
// Desugars to: $fresh <- comp; case $fresh { pat => rest }
func doPatternBind(ch *Checker, strat doStrategy, pat syntax.Pattern, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	freshName := ch.freshName("$p")
	freshPat := &syntax.PatVar{Name: freshName, S: pat.Span()}
	freshBind := &syntax.StmtBind{Pat: freshPat, Comp: comp, S: stmtS}
	caseStmt := &syntax.StmtExpr{
		Expr: &syntax.ExprCase{
			Scrutinee: &syntax.ExprVar{Name: freshName, S: stmtS},
			Alts:      []syntax.AstAlt{{Pattern: pat, Body: stmtsToDoExpr(rest, doS), S: stmtS}},
			S:         stmtS,
		},
		S: stmtS,
	}
	return doElaborate(ch, strat, []syntax.Stmt{freshBind, caseStmt}, doS)
}

// doPatternPureBind handles pat := expr; rest for irrefutable patterns.
// Desugars to: case expr { pat => rest }
func doPatternPureBind(ch *Checker, strat doStrategy, pat syntax.Pattern, expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	caseExpr := &syntax.ExprCase{
		Scrutinee: expr,
		Alts:      []syntax.AstAlt{{Pattern: pat, Body: stmtsToDoExpr(rest, doS), S: stmtS}},
		S:         stmtS,
	}
	stmts := []syntax.Stmt{&syntax.StmtExpr{Expr: caseExpr, S: stmtS}}
	return doElaborate(ch, strat, stmts, doS)
}

func stmtsToDoExpr(stmts []syntax.Stmt, s span.Span) syntax.Expr {
	return &syntax.ExprDo{Stmts: stmts, S: s}
}

// --- doInfer: fresh metas for pre/post, returns inferred type ---

type doInfer struct {
	ch *Checker
	// Post-state from the preceding statement. Used to pre-resolve
	// comp pre-states before pattern matching, preventing pattern-bound
	// variable metas from leaking into the state position.
	lastPost types.Type
}

func (d *doInfer) errPair(s span.Span) (types.Type, ir.Core) {
	return &types.TyError{S: s}, &ir.Error{S: s}
}

func (d *doInfer) elaborateBase(expr syntax.Expr, s span.Span) (types.Type, ir.Core) {
	return d.ch.infer(expr)
}

func (d *doInfer) elaborateBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch
	compTy, compCore := ch.infer(comp)
	compTy = ch.unifier.Zonk(compTy)
	compTy, compCore = ch.autoForceIfThunk(compTy, compCore, comp.Span())

	// Pre-resolve: unify preceding post-state with this comp's pre-state
	// BEFORE elaborating rest, preventing pattern-bound metas from leaking.
	if d.lastPost != nil {
		if inferredComp, ok := compTy.(*types.TyCBPV); ok {
			ch.emitEq(d.lastPost, inferredComp.Pre, stmtS, nil)
			compTy = ch.unifier.Zonk(compTy)
		}
	}

	resultTy := ch.extractCompResult(compTy, stmtS)

	// Error recovery: skip post-state threading on decomposition failure.
	if _, isErr := resultTy.(*types.TyError); isErr {
		ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
		restTy, restCore := doElaborate(ch, d, rest, doS)
		ch.ctx.Pop()
		return restTy, &ir.Bind{Comp: compCore, Var: varName, Discard: varName == "_", Body: restCore, S: stmtS}
	}

	restore := d.pushPost(compTy)
	ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
	restTy, restCore := doElaborate(ch, d, rest, doS)
	ch.ctx.Pop()
	restore()

	d.unifyCompPostPre(compTy, restTy, stmtS)
	return d.withFirstPre(compTy, restTy), &ir.Bind{Comp: compCore, Var: varName, Discard: varName == "_", Body: restCore, S: stmtS}
}

func (d *doInfer) elaborateExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch
	compTy, compCore := ch.infer(expr)
	compTy = ch.unifier.Zonk(compTy)
	compTy, compCore = ch.autoForceIfThunk(compTy, compCore, expr.Span())

	restore := d.pushPost(compTy)
	restTy, restCore := doElaborate(ch, d, rest, doS)
	restore()

	d.unifyCompPostPre(compTy, restTy, stmtS)
	return d.withFirstPre(compTy, restTy), &ir.Bind{Comp: compCore, Var: "_", Discard: true, Body: restCore, S: stmtS}
}

// pushPost saves lastPost and sets it to compTy's Post if compTy is a
// Computation. Returns a restore function. Eliminates the fragile
// manual save/restore pattern across elaboration calls.
func (d *doInfer) pushPost(compTy types.Type) func() {
	inferredComp, ok := compTy.(*types.TyCBPV)
	if !ok {
		return func() {}
	}
	saved := d.lastPost
	d.lastPost = inferredComp.Post
	return func() { d.lastPost = saved }
}

// withFirstPre returns restTy with its Pre replaced by compTy's Pre.
func (d *doInfer) withFirstPre(compTy, restTy types.Type) types.Type {
	comp, ok1 := compTy.(*types.TyCBPV)
	rest, ok2 := restTy.(*types.TyCBPV)
	if !ok1 || !ok2 || comp.Tag != types.TagComp || rest.Tag != types.TagComp {
		return restTy
	}
	return &types.TyCBPV{
		Tag:    types.TagComp,
		Pre:    comp.Pre,
		Post:   rest.Post,
		Result: rest.Result,
		Grade:  rest.Grade,
		Flags:  types.MetaFreeFlags(comp.Pre, rest.Post, rest.Result),
		S:      rest.S,
	}
}

func (d *doInfer) unifyCompPostPre(compTy, restTy types.Type, s span.Span) {
	ch := d.ch
	compTy = ch.unifier.Zonk(compTy)
	restTy = ch.unifier.Zonk(restTy)
	compComp, ok1 := compTy.(*types.TyCBPV)
	restComp, ok2 := restTy.(*types.TyCBPV)
	if ok1 && ok2 {
		ch.emitEq(compComp.Post, restComp.Pre, s, nil)
	}
}

// --- doChecked: threads known pre/post from TyCBPV ---

type doChecked struct {
	ch   *Checker
	comp *types.TyCBPV
}

func (d *doChecked) errPair(s span.Span) (types.Type, ir.Core) {
	return d.comp, &ir.Error{S: s}
}

func (d *doChecked) elaborateBase(expr syntax.Expr, s span.Span) (types.Type, ir.Core) {
	return d.comp, d.ch.check(expr, d.comp)
}

func (d *doChecked) elaborateBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch
	compTy, compCore := ch.infer(comp)
	compTy = ch.unifier.Zonk(compTy)

	if inferredComp, ok := compTy.(*types.TyCBPV); ok {
		ch.emitEq(inferredComp.Pre, d.comp.Pre, stmtS, solve.WithLazyContext(0, func() string {
			return "do bind: pre-state mismatch: expected " + types.Pretty(d.comp.Pre) + ", got " + types.Pretty(inferredComp.Pre)
		}))
		ch.ctx.Push(&CtxVar{Name: varName, Type: inferredComp.Result})
		restComp := &types.TyCBPV{Tag: types.TagComp, Pre: inferredComp.Post, Post: d.comp.Post, Result: d.comp.Result, S: d.comp.S}
		savedComp := d.comp
		d.comp = restComp
		_, restCore := doElaborate(ch, d, rest, doS)
		d.comp = savedComp
		ch.ctx.Pop()
		return d.comp, &ir.Bind{Comp: compCore, Var: varName, Body: restCore, S: stmtS}
	}

	// Fallback: infer didn't give TyCBPV, continue with infer mode.
	resultTy := ch.extractCompResult(compTy, stmtS)
	ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
	fallback := &doInfer{ch: ch}
	restTy, restCore := doElaborate(ch, fallback, rest, doS)
	ch.ctx.Pop()
	ch.emitEq(restTy, d.comp, stmtS, nil)
	return d.comp, &ir.Bind{Comp: compCore, Var: varName, Body: restCore, S: stmtS}
}

func (d *doChecked) elaborateExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch
	compTy, compCore := ch.infer(expr)
	compTy = ch.unifier.Zonk(compTy)

	if inferredComp, ok := compTy.(*types.TyCBPV); ok {
		ch.emitEq(inferredComp.Pre, d.comp.Pre, stmtS, solve.WithLazyContext(0, func() string {
			return "do statement: pre-state mismatch: expected " + types.Pretty(d.comp.Pre) + ", got " + types.Pretty(inferredComp.Pre)
		}))
		restComp := &types.TyCBPV{Tag: types.TagComp, Pre: inferredComp.Post, Post: d.comp.Post, Result: d.comp.Result, S: d.comp.S}
		savedComp := d.comp
		d.comp = restComp
		_, restCore := doElaborate(ch, d, rest, doS)
		d.comp = savedComp
		return d.comp, &ir.Bind{Comp: compCore, Var: "_", Discard: true, Body: restCore, S: stmtS}
	}

	// Fallback: infer didn't give TyCBPV, continue with infer mode.
	fallback := &doInfer{ch: ch}
	restTy, restCore := doElaborate(ch, fallback, rest, doS)
	ch.emitEq(restTy, d.comp, stmtS, nil)
	return d.comp, &ir.Bind{Comp: compCore, Var: "_", Discard: true, Body: restCore, S: stmtS}
}

// --- Entry points ---

func (ch *Checker) inferDo(e *syntax.ExprDo) (types.Type, ir.Core) {
	if len(e.Stmts) == 0 {
		ch.addDiag(diagnostic.ErrEmptyDo, e.S, diagMsg("empty do block"))
		return ch.errorPair(e.S)
	}
	d := &doInfer{ch: ch}
	return doElaborate(ch, d, e.Stmts, e.S)
}

// --- Shared helpers ---

// rejectDoEnding reports ErrBadDoEnding if the last statement is a binding.
// Returns true if the statement was rejected.
func (ch *Checker) rejectDoEnding(st syntax.Stmt) bool {
	switch st.(type) {
	case *syntax.StmtBind, *syntax.StmtPureBind:
		ch.addDiag(diagnostic.ErrBadDoEnding, st.Span(), diagMsg("do block must end with an expression"))
		return true
	}
	return false
}

func (ch *Checker) extractCompResult(ty types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	if comp, ok := ty.(*types.TyCBPV); ok {
		return comp.Result
	}
	// Try to unify with a fresh Computation (graded).
	grade := ch.freshMeta(types.TypeOfTypes)
	pre := ch.freshMeta(types.TypeOfRows)
	post := ch.freshMeta(types.TypeOfRows)
	result := ch.freshMeta(types.TypeOfTypes)
	expected := types.MkCompGraded(pre, post, result, grade)
	if err := ch.unifier.Unify(ty, expected); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadComputation, err, s, "expected computation type, got "+types.Pretty(ty))
		return &types.TyError{S: s}
	}
	return result
}
