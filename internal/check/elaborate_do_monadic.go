package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// checkDo type-checks a do block against an expected type.
// Uses direct Core.Bind for Computation types (fast path) and
// class dispatch via ixbind for other IxMonad instances.
func (ch *Checker) checkDo(e *syntax.ExprDo, expected types.Type) core.Core {
	if len(e.Stmts) == 0 {
		ch.addCodedError(errs.ErrEmptyDo, e.S, "empty do block")
		return &core.Var{Name: "<error>", S: e.S}
	}

	expected = ch.unifier.Zonk(expected)

	// Fast path: Computation types use Core.Bind with expected pre/post threading.
	if comp, ok := expected.(*types.TyCBPV); ok && comp.Tag == types.TagComp {
		var steps []multStep
		result := ch.elaborateStmtsChecked(e.Stmts, comp, e.S, &steps)
		ch.checkMultiplicity(comp, steps, e.S)
		return result
	}
	switch expected.(type) {
	case *types.TyMeta, *types.TyError:
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
		if ch.rejectDoEnding(stmts[0]) {
			return &core.Var{Name: "<error>", S: stmts[0].Span()}
		}
		st := stmts[0].(*syntax.StmtExpr)
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
	}

	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		// x <- comp; rest  →  ixbind comp (\x. rest)
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
		// comp; rest  →  ixbind comp (\_. rest)
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
		// x := e; rest  →  (\x. rest) e
		return ch.elaboratePureBind(st, func() core.Core {
			return ch.elaborateDoMonadic(stmts[1:], monadHead, expected, s)
		})
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

// IxMonad method indices (offset from the start of the methods block, after supers).
const (
	ixMethodPure = 0 // ixpure
	ixMethodBind = 1 // ixbind
)

// extractIxMethod resolves the IxMonad (Lift monadHead) dictionary and
// extracts the method at the given index via pattern matching.
func (ch *Checker) extractIxMethod(monadHead types.Type, methodIdx int, s span.Span) core.Core {
	classInfo := ch.reg.classes["IxMonad"]
	if classInfo == nil {
		ch.errors.Add(&errs.Error{Code: errs.ErrNoInstance, Span: s, Message: "IxMonad class not available (missing Prelude?)"})
		return &core.Var{Name: "<error>", S: s}
	}
	liftedMonad := &types.TyApp{Fun: &types.TyCon{Name: "Lift"}, Arg: monadHead}
	dict := ch.resolveInstance("IxMonad", []types.Type{liftedMonad}, s)
	fieldIdx := len(classInfo.Supers) + methodIdx
	return ch.extractDictField(classInfo, dict, fieldIdx, "ixm", s)
}

// mkIxPure generates Core for monadic pure using the IxMonad dictionary.
func (ch *Checker) mkIxPure(monadHead types.Type, val core.Core, s span.Span) core.Core {
	selector := ch.extractIxMethod(monadHead, ixMethodPure, s)
	return &core.App{Fun: selector, Arg: val, S: s}
}

// mkIxBind generates Core for a monadic bind using the IxMonad dictionary.
func (ch *Checker) mkIxBind(monadHead types.Type, comp core.Core, varName string, body core.Core, s span.Span) core.Core {
	selector := ch.extractIxMethod(monadHead, ixMethodBind, s)
	return &core.App{
		Fun: &core.App{Fun: selector, Arg: comp, S: s},
		Arg: &core.Lam{Param: varName, Body: body, S: s},
		S:   s,
	}
}
