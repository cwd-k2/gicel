package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/internal/types"
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

// checkDo type-checks a do block against an expected type.
// Uses direct Core.Bind for Computation types (fast path) and
// class dispatch via ixbind for other IxMonad instances.
func (ch *Checker) checkDo(e *syntax.ExprDo, expected types.Type) core.Core {
	if len(e.Stmts) == 0 {
		ch.addCodedError(errs.ErrEmptyDo, e.S, "empty do block")
		return &core.Var{Name: "<error>", S: e.S}
	}

	expected = ch.unifier.Zonk(expected)

	// Fast path: Computation types, metas (unknown), and errors use Core.Bind.
	switch expected.(type) {
	case *types.TyComp, *types.TyMeta, *types.TyError:
		inferredTy, coreExpr := ch.elaborateStmts(e.Stmts, e.S)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	// Class dispatch: extract monad head and elaborate with dictionary.
	monadHead := ch.extractMonadHead(expected)
	if monadHead != nil {
		return ch.elaborateDoMonadic(e.Stmts, monadHead, expected, e.S)
	}

	// Fallback: try Computation inference.
	inferredTy, coreExpr := ch.elaborateStmts(e.Stmts, e.S)
	return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
}

// extractMonadHead extracts the type constructor head from a monadic type.
// e.g. Maybe Int → Maybe, List Bool → List
func (ch *Checker) extractMonadHead(ty types.Type) types.Type {
	head, args := types.UnwindApp(ty)
	if _, ok := head.(*types.TyCon); ok && len(args) > 0 {
		// Reconstruct head with all but last arg (the result type).
		var result types.Type = head
		for _, a := range args[:len(args)-1] {
			result = &types.TyApp{Fun: result, Arg: a}
		}
		return result
	}
	return nil
}

// elaborateDoMonadic elaborates a do block using IxMonad class dispatch.
// The monad head is used to resolve the IxMonad (Lift m) instance.
func (ch *Checker) elaborateDoMonadic(stmts []syntax.Stmt, monadHead types.Type, expected types.Type, s span.Span) core.Core {
	if len(stmts) == 1 {
		switch st := stmts[0].(type) {
		case *syntax.StmtExpr:
			// Intercept `pure val` / `ixpure val` at the end of a monadic do block.
			if pureVal := extractPureArg(st.Expr); pureVal != nil {
				_, args := types.UnwindApp(expected)
				if len(args) > 0 {
					resultTy := args[len(args)-1]
					valCore := ch.check(pureVal, resultTy)
					return ch.mkIxPure(monadHead, valCore, s)
				}
			}
			return ch.check(st.Expr, expected)
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
		// x <- comp; rest  →  ixbind comp (\x -> rest)
		// Intercept `x <- pure val` / `x <- ixpure val`.
		var compCore core.Core
		var resultTy types.Type
		if pureVal := extractPureArg(st.Comp); pureVal != nil {
			rty, vc := ch.infer(pureVal)
			compCore = ch.mkIxPure(monadHead, vc, st.S)
			resultTy = rty
		} else {
			compTy, cc := ch.infer(st.Comp)
			compCore = cc
			resultTy = ch.extractMonadResult(compTy, monadHead, st.S)
		}
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: resultTy})
		restCore := ch.elaborateDoMonadic(stmts[1:], monadHead, expected, s)
		ch.ctx.Pop()
		return ch.mkIxBind(monadHead, compCore, st.Var, restCore, st.S)

	case *syntax.StmtExpr:
		// comp; rest  →  ixbind comp (\_ -> rest)
		var compCore core.Core
		if pureVal := extractPureArg(st.Expr); pureVal != nil {
			_, vc := ch.infer(pureVal)
			compCore = ch.mkIxPure(monadHead, vc, st.S)
		} else {
			_, cc := ch.infer(st.Expr)
			compCore = cc
		}
		restCore := ch.elaborateDoMonadic(stmts[1:], monadHead, expected, s)
		return ch.mkIxBind(monadHead, compCore, "_", restCore, st.S)

	case *syntax.StmtPureBind:
		// x := e; rest  →  (\x -> rest) e
		bindTy, bindCore := ch.infer(st.Expr)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: bindTy})
		restCore := ch.elaborateDoMonadic(stmts[1:], monadHead, expected, s)
		ch.ctx.Pop()
		return &core.App{
			Fun: &core.Lam{Param: st.Var, Body: restCore, S: st.S},
			Arg: bindCore,
			S:   st.S,
		}
	}

	return &core.Var{Name: "<error>", S: s}
}

// extractMonadResult extracts the result type from a monadic type given the monad head.
// e.g. Maybe Int with head Maybe → Int
func (ch *Checker) extractMonadResult(ty types.Type, monadHead types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	_, args := types.UnwindApp(ty)
	if len(args) > 0 {
		return args[len(args)-1]
	}
	// Generate fresh meta as fallback.
	result := ch.freshMeta(types.KType{})
	headApp := &types.TyApp{Fun: monadHead, Arg: result}
	if err := ch.unifier.Unify(ty, headApp); err != nil {
		ch.addSemanticUnifyError(errs.ErrBadComputation, err, s, fmt.Sprintf("expected %s type, got %s",
			types.Pretty(monadHead), types.Pretty(ty)))
		return &types.TyError{S: s}
	}
	return result
}

// extractPureArg checks if an expression is `pure val` or `ixpure val` and returns val.
func extractPureArg(expr syntax.Expr) syntax.Expr {
	app, ok := expr.(*syntax.ExprApp)
	if !ok {
		return nil
	}
	v, ok := app.Fun.(*syntax.ExprVar)
	if !ok {
		return nil
	}
	if v.Name == "pure" || v.Name == "ixpure" {
		return app.Arg
	}
	return nil
}

// extractIxMethod resolves the IxMonad (Lift monadHead) dictionary and
// extracts the method at the given index via pattern matching.
func (ch *Checker) extractIxMethod(monadHead types.Type, methodIdx int, s span.Span) core.Core {
	liftedMonad := &types.TyApp{Fun: &types.TyCon{Name: "Lift"}, Arg: monadHead}
	dict := ch.resolveInstance("IxMonad", []types.Type{liftedMonad}, s)

	classInfo := ch.classes["IxMonad"]
	allFields := len(classInfo.Supers) + len(classInfo.Methods)
	var patArgs []core.Pattern
	var methodExpr core.Core
	freshBase := ch.fresh()
	for j := 0; j < allFields; j++ {
		argName := fmt.Sprintf("$ixm_%d_%d", j, freshBase)
		patArgs = append(patArgs, &core.PVar{Name: argName, S: s})
		if j == len(classInfo.Supers)+methodIdx {
			methodExpr = &core.Var{Name: argName, S: s}
		}
	}
	return &core.Case{
		Scrutinee: dict,
		Alts: []core.Alt{{
			Pattern: &core.PCon{Con: classInfo.DictConName, Args: patArgs, S: s},
			Body:    methodExpr,
			S:       s,
		}},
		S: s,
	}
}

// mkIxPure generates Core for monadic pure using the IxMonad dictionary.
func (ch *Checker) mkIxPure(monadHead types.Type, val core.Core, s span.Span) core.Core {
	selector := ch.extractIxMethod(monadHead, 0, s) // ixpure is method 0
	return &core.App{Fun: selector, Arg: val, S: s}
}

// mkIxBind generates Core for a monadic bind using the IxMonad dictionary.
func (ch *Checker) mkIxBind(monadHead types.Type, comp core.Core, varName string, body core.Core, s span.Span) core.Core {
	selector := ch.extractIxMethod(monadHead, 1, s) // ixbind is method 1
	return &core.App{
		Fun: &core.App{Fun: selector, Arg: comp, S: s},
		Arg: &core.Lam{Param: varName, Body: body, S: s},
		S:   s,
	}
}

func (ch *Checker) extractCompResult(ty types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	if comp, ok := ty.(*types.TyComp); ok {
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
