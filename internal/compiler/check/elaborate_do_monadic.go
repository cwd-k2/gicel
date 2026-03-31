package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// checkDo type-checks a do block against an expected type.
// Uses direct Core.Bind for Computation types (fast path) and
// class dispatch via GIMonad for other indexed monad instances.
func (ch *Checker) checkDo(e *syntax.ExprDo, expected types.Type) ir.Core {
	if len(e.Stmts) == 0 {
		ch.addCodedError(diagnostic.ErrEmptyDo, e.S, "empty do block")
		return &ir.Error{S: e.S}
	}

	expected = ch.unifier.Zonk(expected)

	// Fast path: Computation types use Core.Bind with expected pre/post threading.
	if comp, ok := expected.(*types.TyCBPV); ok && comp.Tag == types.TagComp {
		d := &doElaborator{ch: ch, mode: doModeChecked, comp: comp}
		_, result := d.elaborate(e.Stmts, e.S)
		ch.checkGradeBoundary(comp, e.S)
		return result
	}
	switch expected.(type) {
	case *types.TyMeta, *types.TyError:
		d := &doElaborator{ch: ch, mode: doModeInfer}
		inferredTy, coreExpr := d.elaborate(e.Stmts, e.S)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	// Class dispatch: extract monad head and elaborate with GIMonad dictionary.
	monadHead := ch.extractMonadHead(expected)
	if monadHead != nil {
		if ch.hasDirectGIMonadInstance(monadHead, e.S) {
			head, _ := types.AppSpineHead(monadHead)
			d := &doElaborator{ch: ch, mode: doModeGraded, monadHead: head, expected: expected}
			_, result := d.elaborate(e.Stmts, e.S)
			return result
		}
		ch.addCodedError(diagnostic.ErrNoInstance, e.S,
			"do notation for "+types.Pretty(expected)+" requires a GIMonad instance")
		return &ir.Error{S: e.S}
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
	if app, ok := ty.(*types.TyApp); ok {
		return app.Arg
	}
	// Generate fresh meta as fallback.
	result := ch.freshMeta(types.TypeOfTypes)
	headApp := &types.TyApp{Fun: monadHead, Arg: result}
	ch.emitEq(ty, headApp, s, solve.WithLazyContext(diagnostic.ErrBadComputation, func() string {
		return "expected " + types.Pretty(monadHead) + " type, got " + types.Pretty(ty)
	}))
	return result
}

// extractPureArg checks if an expression is `pure val` or `gipure val` and returns val.
func extractPureArg(expr syntax.Expr) syntax.Expr {
	app, ok := expr.(*syntax.ExprApp)
	if !ok {
		return nil
	}
	v, ok := app.Fun.(*syntax.ExprVar)
	if !ok {
		return nil
	}
	if v.Name == "pure" || v.Name == "gipure" {
		return app.Arg
	}
	return nil
}

// --- GIMonad (graded indexed monad) dispatch ---

// GIMonad method indices (offset from the start of the methods block, after supers).
// GIMonad has 1 super (GradeAlgebra g).
const (
	giMethodPure = 0 // gipure
	giMethodBind = 1 // gibind
)

// hasDirectGIMonadInstance checks whether the GIMonad class has a direct
// resolvable instance for the bare type constructor head of monadHead.
// Also tries Lift-wrapped resolution for Type→Type monads (e.g. Maybe, List).
func (ch *Checker) hasDirectGIMonadInstance(monadHead types.Type, s span.Span) bool {
	if _, ok := ch.reg.LookupClass("GIMonad"); !ok {
		return false
	}
	head, _ := types.AppSpineHead(monadHead)
	gradeMeta := ch.freshMeta(types.TypeOfTypes)
	if _, ok := ch.tryResolveInstance("GIMonad", []types.Type{gradeMeta, head}, s); ok {
		return true
	}
	// Lift fallback: GIMonad g (Lift head).
	// Safe for Type→Type monads; do-block elaboration for Lift-wrapped types
	// produces correct results because these monads have no row parameters.
	liftedHead := &types.TyApp{Fun: types.Con("Lift"), Arg: head}
	gradeMeta2 := ch.freshMeta(types.TypeOfTypes)
	_, ok := ch.tryResolveInstance("GIMonad", []types.Type{gradeMeta2, liftedHead}, s)
	return ok
}

// extractGIMethod resolves a GIMonad dictionary for monadHead and extracts
// the method at methodIdx.
// Tries direct resolution first, then Lift-wrapped fallback for Type→Type monads.
func (ch *Checker) extractGIMethod(monadHead types.Type, methodIdx int, s span.Span) ir.Core {
	classInfo, _ := ch.reg.LookupClass("GIMonad")
	if classInfo == nil {
		ch.addCodedError(diagnostic.ErrNoInstance, s, "GIMonad class not available")
		return &ir.Error{S: s}
	}

	// Try direct GIMonad instance.
	gradeMeta := ch.freshMeta(types.TypeOfTypes)
	if dict, ok := ch.tryResolveInstance("GIMonad", []types.Type{gradeMeta, monadHead}, s); ok {
		fieldIdx := len(classInfo.Supers) + methodIdx
		return ch.solver.ExtractDictField(classInfo, dict, fieldIdx, "gim", s)
	}

	// Lift fallback: GIMonad g (Lift monadHead).
	// Safe for Type→Type monads (Maybe, List, etc.).
	liftedMonad := &types.TyApp{Fun: types.Con("Lift"), Arg: monadHead}
	gradeMeta2 := ch.freshMeta(types.TypeOfTypes)
	if dict, ok := ch.tryResolveInstance("GIMonad", []types.Type{gradeMeta2, liftedMonad}, s); ok {
		fieldIdx := len(classInfo.Supers) + methodIdx
		return ch.solver.ExtractDictField(classInfo, dict, fieldIdx, "gim", s)
	}

	ch.addCodedError(diagnostic.ErrNoInstance, s, "no GIMonad instance for "+types.Pretty(monadHead))
	return &ir.Error{S: s}
}

// mkGIPure generates Core for graded monadic pure using the GIMonad dictionary.
func (ch *Checker) mkGIPure(monadHead types.Type, val ir.Core, s span.Span) ir.Core {
	selector := ch.extractGIMethod(monadHead, giMethodPure, s)
	return &ir.App{Fun: selector, Arg: val, S: s}
}

// mkGIBind generates Core for a graded monadic bind using the GIMonad dictionary.
func (ch *Checker) mkGIBind(monadHead types.Type, comp ir.Core, varName string, body ir.Core, s span.Span) ir.Core {
	selector := ch.extractGIMethod(monadHead, giMethodBind, s)
	return &ir.App{
		Fun: &ir.App{Fun: selector, Arg: comp, S: s},
		Arg: &ir.Lam{Param: varName, Body: body, S: s},
		S:   s,
	}
}

// gradedBind handles x <- comp; rest in GIMonad mode.
func (d *doElaborator) gradedBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch

	var compCore ir.Core
	var resultTy types.Type

	if pureVal := extractPureArg(comp); pureVal != nil {
		rty, vc := ch.infer(pureVal)
		compCore = ch.mkGIPure(d.monadHead, vc, stmtS)
		resultTy = rty
	} else {
		compTy, cc := ch.infer(comp)
		compCore = cc
		resultTy = ch.extractMonadResult(compTy, d.monadHead, stmtS)
	}

	ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
	_, restCore := d.elaborate(rest, doS)
	ch.ctx.Pop()
	return d.expected, ch.mkGIBind(d.monadHead, compCore, varName, restCore, stmtS)
}

// gradedExprStmt handles comp; rest (expression statement) in GIMonad mode.
func (d *doElaborator) gradedExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch

	var compCore ir.Core
	if pureVal := extractPureArg(expr); pureVal != nil {
		_, vc := ch.infer(pureVal)
		compCore = ch.mkGIPure(d.monadHead, vc, stmtS)
	} else {
		_, cc := ch.infer(expr)
		compCore = cc
	}

	_, restCore := d.elaborate(rest, doS)
	return d.expected, ch.mkGIBind(d.monadHead, compCore, "_", restCore, stmtS)
}
