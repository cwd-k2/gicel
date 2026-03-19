package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

func (ch *Checker) inferBlock(e *syntax.ExprBlock) (types.Type, core.Core) {
	// Desugar: { x := e1; body } → App(Lam(x, body), e1)
	// Forward pass: infer each binding, add to context.
	type bindInfo struct {
		name string
		ty   types.Type
		core core.Core
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
		lam := &core.Lam{Param: b.name, Body: result, S: b.s}
		result = &core.App{Fun: lam, Arg: b.core, S: b.s}
	}

	return resultTy, result
}

func (ch *Checker) inferDo(e *syntax.ExprDo) (types.Type, core.Core) {
	if len(e.Stmts) == 0 {
		ch.addCodedError(errs.ErrEmptyDo, e.S, "empty do block")
		return ch.errorPair(e.S)
	}
	return ch.elaborateStmts(e.Stmts, e.S)
}

func (ch *Checker) elaborateStmts(stmts []syntax.Stmt, s span.Span) (types.Type, core.Core) {
	if len(stmts) == 1 {
		// Last statement: must be an expression.
		switch st := stmts[0].(type) {
		case *syntax.StmtExpr:
			return ch.infer(st.Expr)
		case *syntax.StmtBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return ch.errorPair(st.S)
		case *syntax.StmtPureBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return ch.errorPair(st.S)
		}
	}

	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		// x <- c; rest  →  Bind(c, x, rest)
		compTy, compCore := ch.infer(st.Comp)
		resultTy := ch.extractCompResult(compTy, st.S)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: resultTy})
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		ch.ctx.Pop()
		return restTy, &core.Bind{Comp: compCore, Var: st.Var, Body: restCore, S: st.S}

	case *syntax.StmtPureBind:
		// x := e; rest  →  App(Lam(x, rest), e)
		bindTy, bindCore := ch.infer(st.Expr)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: bindTy})
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		ch.ctx.Pop()
		return restTy, &core.App{
			Fun: &core.Lam{Param: st.Var, Body: restCore, S: st.S},
			Arg: bindCore,
			S:   st.S,
		}

	case *syntax.StmtExpr:
		// c; rest  →  Bind(c, "_", rest)
		_, compCore := ch.infer(st.Expr)
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		return restTy, &core.Bind{Comp: compCore, Var: "_", Body: restCore, S: st.S}

	default:
		ch.addCodedError(errs.ErrBadComputation, s, "unexpected statement in do block")
		return ch.errorPair(s)
	}
}

// elaborateStmtsChecked elaborates a do block against a known Computation type.
// This threads the pre/post state through the bind chain, unlike elaborateStmts
// which infers fresh metas for pre/post.
// steps accumulates pre/post pairs for multiplicity analysis.
func (ch *Checker) elaborateStmtsChecked(stmts []syntax.Stmt, comp *types.TyCBPV, s span.Span, steps *[]multStep) core.Core {
	if len(stmts) == 1 {
		switch st := stmts[0].(type) {
		case *syntax.StmtExpr:
			return ch.check(st.Expr, comp)
		case *syntax.StmtBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return &core.Var{Name: "<error>", S: st.S}
		case *syntax.StmtPureBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return &core.Var{Name: "<error>", S: st.S}
		}
	}

	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		// x <- c; rest
		// c: Computation pre mid a — infer, but pre must match comp.Pre
		compTy, compCore := ch.infer(st.Comp)
		compTy = ch.unifier.Zonk(compTy)
		if inferredComp, ok := compTy.(*types.TyCBPV); ok {
			// Record step for multiplicity analysis.
			*steps = append(*steps, multStep{pre: inferredComp.Pre, post: inferredComp.Post, s: st.S})
			// Unify inferred pre with expected pre.
			if err := ch.unifier.Unify(inferredComp.Pre, comp.Pre); err != nil {
				ch.addUnifyError(err, st.S, fmt.Sprintf(
					"do bind: pre-state mismatch: expected %s, got %s",
					types.Pretty(comp.Pre), types.Pretty(inferredComp.Pre)))
			}
			resultTy := inferredComp.Result
			ch.ctx.Push(&CtxVar{Name: st.Var, Type: resultTy})
			// Rest: Computation mid post result — mid from inferred post, post/result from expected.
			restComp := &types.TyCBPV{Tag: types.TagComp, Pre: inferredComp.Post, Post: comp.Post, Result: comp.Result, S: comp.S}
			restCore := ch.elaborateStmtsChecked(stmts[1:], restComp, s, steps)
			ch.ctx.Pop()
			return &core.Bind{Comp: compCore, Var: st.Var, Body: restCore, S: st.S}
		}
		// Fallback: infer didn't give TyCBPV, extract result and continue.
		resultTy := ch.extractCompResult(compTy, st.S)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: resultTy})
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		ch.ctx.Pop()
		// Best-effort: infer didn't produce TyCBPV, so pre/post threading
		// is unavailable. Unifying the inferred rest type with the expected
		// computation type is advisory — failure means the do block types
		// are already inconsistent and errors will surface elsewhere.
		_ = ch.unifier.Unify(restTy, comp)
		return &core.Bind{Comp: compCore, Var: st.Var, Body: restCore, S: st.S}

	case *syntax.StmtPureBind:
		// x := e; rest
		bindTy, bindCore := ch.infer(st.Expr)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: bindTy})
		restCore := ch.elaborateStmtsChecked(stmts[1:], comp, s, steps)
		ch.ctx.Pop()
		return &core.App{
			Fun: &core.Lam{Param: st.Var, Body: restCore, S: st.S},
			Arg: bindCore,
			S:   st.S,
		}

	case *syntax.StmtExpr:
		// c; rest
		compTy, compCore := ch.infer(st.Expr)
		compTy = ch.unifier.Zonk(compTy)
		if inferredComp, ok := compTy.(*types.TyCBPV); ok {
			// Record step for multiplicity analysis.
			*steps = append(*steps, multStep{pre: inferredComp.Pre, post: inferredComp.Post, s: st.S})
			if err := ch.unifier.Unify(inferredComp.Pre, comp.Pre); err != nil {
				ch.addUnifyError(err, st.S, fmt.Sprintf(
					"do statement: pre-state mismatch: expected %s, got %s",
					types.Pretty(comp.Pre), types.Pretty(inferredComp.Pre)))
			}
			restComp := &types.TyCBPV{Tag: types.TagComp, Pre: inferredComp.Post, Post: comp.Post, Result: comp.Result, S: comp.S}
			restCore := ch.elaborateStmtsChecked(stmts[1:], restComp, s, steps)
			return &core.Bind{Comp: compCore, Var: "_", Body: restCore, S: st.S}
		}
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		// Best-effort: infer didn't produce TyCBPV, so pre/post threading
		// is unavailable. Unifying the inferred rest type with the expected
		// computation type is advisory — failure means the do block types
		// are already inconsistent and errors will surface elsewhere.
		_ = ch.unifier.Unify(restTy, comp)
		return &core.Bind{Comp: compCore, Var: "_", Body: restCore, S: st.S}
	}

	ch.addCodedError(errs.ErrBadComputation, s, "unexpected statement in do block")
	return &core.Var{Name: "<error>", S: s}
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
		ch.addSemanticUnifyError(errs.ErrBadComputation, err, s, fmt.Sprintf("expected computation type, got %s", types.Pretty(ty)))
		return &types.TyError{S: s}
	}
	return result
}
