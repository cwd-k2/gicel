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
// class dispatch via ixbind for other IxMonad instances.
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

	// Class dispatch: extract monad head and elaborate with dictionary.
	//
	// Two paths are available depending on the class instance:
	//  1. Monad m: desugar to mbind/mpure at the AST level and type-check normally.
	//  2. IxMonad m (direct): elaborate with ixbind/ixpure dictionary dispatch.
	//
	// Lift-based resolution (IxMonad (Lift m)) is excluded from do-elaboration.
	// Lift erases row parameters (Lift m r1 r2 a = m a), so when monadHead
	// carries partially-applied rows (e.g. State Int Int), wrapping it in Lift
	// loses the pre/post state threading and produces incorrect elaboration
	// whenever pre ≠ post.
	monadHead := ch.extractMonadHead(expected)
	if monadHead != nil {
		if ch.hasMonadInstance(monadHead, e.S) {
			desugared := ch.desugarDoToMonad(e.Stmts, e.S)
			return ch.check(desugared, expected)
		}
		if ch.hasDirectIxMonadInstance(monadHead, e.S) {
			head, _ := types.UnwindApp(monadHead)
			d := &doElaborator{ch: ch, mode: doModeMonadic, monadHead: head, expected: expected}
			_, result := d.elaborate(e.Stmts, e.S)
			return result
		}
		ch.addCodedError(diagnostic.ErrNoInstance, e.S,
			"do notation for "+types.Pretty(expected)+" requires a Monad or IxMonad instance")
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
	_, args := types.UnwindApp(ty)
	if len(args) > 0 {
		return args[len(args)-1]
	}
	// Generate fresh meta as fallback.
	result := ch.freshMeta(types.TypeOfTypes)
	headApp := &types.TyApp{Fun: monadHead, Arg: result}
	ch.emitEq(ty, headApp, s, solve.WithLazyContext(diagnostic.ErrBadComputation, func() string {
		return "expected " + types.Pretty(monadHead) + " type, got " + types.Pretty(ty)
	}))
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

// hasDirectIxMonadInstance checks whether the IxMonad class has a direct
// resolvable instance for the bare type constructor head of monadHead.
// Lift-based resolution is excluded; see checkDo comment for rationale.
func (ch *Checker) hasDirectIxMonadInstance(monadHead types.Type, s span.Span) bool {
	if _, ok := ch.reg.LookupClass("IxMonad"); !ok {
		return false
	}
	head, _ := types.UnwindApp(monadHead)
	_, ok := ch.tryResolveInstance("IxMonad", []types.Type{head}, s)
	return ok
}

// hasMonadInstance checks whether the Monad class has a resolvable instance
// for the given monadHead, without emitting errors.
func (ch *Checker) hasMonadInstance(monadHead types.Type, s span.Span) bool {
	if _, ok := ch.reg.LookupClass("Monad"); !ok {
		return false
	}
	_, ok := ch.tryResolveInstance("Monad", []types.Type{monadHead}, s)
	return ok
}

// extractIxMethod resolves an IxMonad dictionary for monadHead and extracts
// the method at methodIdx.
//
// Resolution order: direct IxMonad instance first, then Lift-wrapped fallback.
// The Lift fallback is needed for pure/ixpure elaboration outside do-blocks
// (bidir_cbpv.go), where Type→Type monads like Maybe resolve via
// IxMonad (Lift Maybe). Lift is safe there because Type→Type monads have
// no row parameters to lose.
//
// Inside do-blocks, checkDo's dispatch guards ensure doModeMonadic is only
// entered with a bare head that has a direct instance, so the Lift fallback
// is not reached from that path.
func (ch *Checker) extractIxMethod(monadHead types.Type, methodIdx int, s span.Span) ir.Core {
	classInfo, _ := ch.reg.LookupClass("IxMonad")
	if classInfo == nil {
		ch.addCodedError(diagnostic.ErrNoInstance, s, "IxMonad class not available (missing Prelude?)")
		return &ir.Error{S: s}
	}

	// Try direct IxMonad instance.
	if dict, ok := ch.tryResolveInstance("IxMonad", []types.Type{monadHead}, s); ok {
		fieldIdx := len(classInfo.Supers) + methodIdx
		return ch.extractDictField(classInfo, dict, fieldIdx, "ixm", s)
	}

	// Lift fallback: IxMonad (Lift monadHead).
	// Safe for Type→Type monads; unreachable for Row→Row→Type→Type types
	// because checkDo rejects those without a direct instance.
	liftedMonad := &types.TyApp{Fun: types.Con("Lift"), Arg: monadHead}
	dict := ch.resolveInstance("IxMonad", []types.Type{liftedMonad}, s)
	fieldIdx := len(classInfo.Supers) + methodIdx
	return ch.extractDictField(classInfo, dict, fieldIdx, "ixm", s)
}

// rewritePureToMpure rewrites `pure expr` or `ixpure expr` to `mpure expr`
// at the AST level. The `pure` builtin is linked to IxMonad; for Monad
// dispatch we need `mpure` instead. Non-pure expressions are returned as-is.
func rewritePureToMpure(expr syntax.Expr) syntax.Expr {
	if app, ok := expr.(*syntax.ExprApp); ok {
		if v, ok := app.Fun.(*syntax.ExprVar); ok {
			if v.Name == "pure" || v.Name == "ixpure" {
				return &syntax.ExprApp{
					Fun: &syntax.ExprVar{Name: "mpure", S: v.S},
					Arg: app.Arg,
					S:   app.S,
				}
			}
		}
	}
	return expr
}

// desugarDoToMonad desugars a do-block into explicit mbind/mpure calls.
// do { x <- a; b }  →  mbind a (\x. b)
// do { a; b }        →  mbind a (\_ . b)
// do { a }           →  a
// do { x <- a }      →  a   (bind with no rest is just the computation)
func (ch *Checker) desugarDoToMonad(stmts []syntax.Stmt, s span.Span) syntax.Expr {
	if len(stmts) == 0 {
		return &syntax.ExprError{S: s}
	}
	if len(stmts) == 1 {
		switch st := stmts[0].(type) {
		case *syntax.StmtExpr:
			return rewritePureToMpure(st.Expr)
		case *syntax.StmtBind:
			return st.Comp
		case *syntax.StmtPureBind:
			// let x = e as the last statement — just return e (the binding
			// has no continuation to use x in).
			return st.Expr
		}
		return &syntax.ExprError{S: s}
	}
	rest := ch.desugarDoToMonad(stmts[1:], s)
	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		// mbind comp (\x. rest)  — or for pattern: mbind comp (\$fresh. case $fresh { pat => rest })
		var lamBody syntax.Expr
		var lamParam syntax.Pattern
		if name, ok := syntax.PatVarName(st.Pat); ok {
			lamParam = &syntax.PatVar{Name: name, S: st.S}
			lamBody = rest
		} else {
			freshName := ch.freshName("$p")
			lamParam = &syntax.PatVar{Name: freshName, S: st.S}
			lamBody = &syntax.ExprCase{
				Scrutinee: &syntax.ExprVar{Name: freshName, S: st.S},
				Alts:      []syntax.AstAlt{{Pattern: st.Pat, Body: rest, S: st.S}},
				S:         st.S,
			}
		}
		return &syntax.ExprApp{
			Fun: &syntax.ExprApp{
				Fun: &syntax.ExprVar{Name: "mbind", S: st.S},
				Arg: rewritePureToMpure(st.Comp),
				S:   st.S,
			},
			Arg: &syntax.ExprLam{
				Params: []syntax.Pattern{lamParam},
				Body:   lamBody,
				S:      st.S,
			},
			S: st.S,
		}
	case *syntax.StmtExpr:
		// mbind comp (\_ . rest)
		return &syntax.ExprApp{
			Fun: &syntax.ExprApp{
				Fun: &syntax.ExprVar{Name: "mbind", S: st.S},
				Arg: rewritePureToMpure(st.Expr),
				S:   st.S,
			},
			Arg: &syntax.ExprLam{
				Params: []syntax.Pattern{&syntax.PatWild{S: st.S}},
				Body:   rest,
				S:      st.S,
			},
			S: st.S,
		}
	case *syntax.StmtPureBind:
		// (\x. rest) val  — or for pattern: (\$fresh. case $fresh { pat => rest }) val
		var lamBody syntax.Expr
		var lamParam syntax.Pattern
		if name, ok := syntax.PatVarName(st.Pat); ok {
			lamParam = &syntax.PatVar{Name: name, S: st.S}
			lamBody = rest
		} else {
			freshName := ch.freshName("$p")
			lamParam = &syntax.PatVar{Name: freshName, S: st.S}
			lamBody = &syntax.ExprCase{
				Scrutinee: &syntax.ExprVar{Name: freshName, S: st.S},
				Alts:      []syntax.AstAlt{{Pattern: st.Pat, Body: rest, S: st.S}},
				S:         st.S,
			}
		}
		return &syntax.ExprApp{
			Fun: &syntax.ExprLam{
				Params: []syntax.Pattern{lamParam},
				Body:   lamBody,
				S:      st.S,
			},
			Arg: st.Expr,
			S:   st.S,
		}
	}
	return &syntax.ExprError{S: s}
}

// mkIxPure generates Core for monadic pure using the IxMonad or Monad dictionary.
func (ch *Checker) mkIxPure(monadHead types.Type, val ir.Core, s span.Span) ir.Core {
	selector := ch.extractIxMethod(monadHead, ixMethodPure, s)
	return &ir.App{Fun: selector, Arg: val, S: s}
}

// mkIxBind generates Core for a monadic bind using the IxMonad or Monad dictionary.
func (ch *Checker) mkIxBind(monadHead types.Type, comp ir.Core, varName string, body ir.Core, s span.Span) ir.Core {
	selector := ch.extractIxMethod(monadHead, ixMethodBind, s)
	return &ir.App{
		Fun: &ir.App{Fun: selector, Arg: comp, S: s},
		Arg: &ir.Lam{Param: varName, Body: body, S: s},
		S:   s,
	}
}
