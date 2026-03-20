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
		d := &doElaborator{ch: ch, mode: doModeChecked, comp: comp, steps: &steps}
		_, result := d.elaborate(e.Stmts, e.S)
		ch.checkMultiplicity(comp, steps, e.S)
		return result
	}
	switch expected.(type) {
	case *types.TyMeta, *types.TyError:
		d := &doElaborator{ch: ch, mode: doModeInfer}
		inferredTy, coreExpr := d.elaborate(e.Stmts, e.S)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	// Class dispatch: extract monad head and elaborate with dictionary.
	monadHead := ch.extractMonadHead(expected)
	if monadHead != nil {
		d := &doElaborator{ch: ch, mode: doModeMonadic, monadHead: monadHead, expected: expected}
		_, result := d.elaborate(e.Stmts, e.S)
		return result
	}

	// Fallback: try Computation inference.
	d := &doElaborator{ch: ch, mode: doModeInfer}
	inferredTy, coreExpr := d.elaborate(e.Stmts, e.S)
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
		ch.addCodedError(errs.ErrNoInstance, s, "IxMonad class not available (missing Prelude?)")
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
