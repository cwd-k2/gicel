// Instantiation and spine elaboration — matchArrow, instantiate, inferApply.
// Does NOT cover: name resolution (bidir_lookup.go), diagnostics (bidir_suggest.go).
package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func (ch *Checker) matchArrow(ty types.Type, s span.Span) (types.Type, types.Type) {
	ty = ch.unifier.Zonk(ty)
	// Peel foralls: a higher-rank return type (e.g., from mkId :: () -> \a. a -> a)
	// must be instantiated before arrow decomposition.
	ty = ch.typeOps.PeelForalls(ty, func(f *types.TyForall) (types.Type, types.LevelExpr) {
		if isLevelKind(f.Kind) {
			return ch.freshMeta(types.SortZero), ch.unifier.FreshLevelMeta()
		}
		return ch.freshMeta(f.Kind), nil
	})
	// Reduce type family applications before arrow decomposition.
	// check() already reduces the expected type, but matchArrow is also called
	// from infer paths where the type may not have been pre-reduced.
	ty = ch.reduceFamilyInType(ty)
	if arr, ok := ty.(*types.TyArrow); ok {
		return arr.From, arr.To
	}
	// A Computation type in function position is a strong signal of a missing
	// semicolon in a do-block: `x <- get; put (x + 1) get` parses the last
	// two statements as a single application `(put (x + 1)) get`, and the
	// return type of `put (x + 1)` is Computation — not a function.
	if cbpv, ok := ty.(*types.TyCBPV); ok && cbpv.Tag == types.TagComp {
		msg := "expected function type, got " + ch.typeOps.Pretty(ty)
		hints := []diagnostic.Hint{{Message: "did you forget a ';' between statements in a do-block?"}}
		ch.addDiagHints(diagnostic.ErrBadApplication, s, diagMsg(msg), hints)
		return ch.freshMeta(types.TypeOfTypes), ch.freshMeta(types.TypeOfTypes)
	}
	// Generate fresh metas and decompose eagerly.
	// Eager unification is required here: callers use argTy/retTy immediately
	// for downstream checking (e.g., check(arg, argTy)), so the metas must
	// be solved before control returns. The headIsMeta check in processCtEq
	// would correctly handle error detection, but deferral would leave the
	// decomposition metas unsolved when callers need them.
	argTy := ch.freshMeta(types.TypeOfTypes)
	retTy := ch.freshMeta(types.TypeOfTypes)
	if err := ch.unifier.Unify(ty, ch.typeOps.Arrow(argTy, retTy, span.Span{})); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadApplication, err, s, "expected function type, got "+ch.typeOps.Pretty(ty))
	}
	return argTy, retTy
}

// inferApply decomposes the function type into (argTy -> retTy), then
// checks the argument against the parameter type, and wraps in ir.App.
// For lazy co-data constructors, arguments are automatically wrapped in ir.Thunk.
func (ch *Checker) inferApply(funTy types.Type, funCore ir.Core, arg syntax.Expr, s span.Span) (types.Type, ir.Core) {
	argTy, retTy := ch.matchArrow(funTy, s)
	argCore := ch.check(arg, argTy)
	argCore = ch.wrapAutoThunk(funCore, argCore, arg.Span())
	return retTy, &ir.App{Fun: funCore, Arg: argCore, S: s}
}

// wrapAutoThunk wraps argCore in ir.Thunk if funCore is a lazy constructor application.
// Lazy co-data constructors suspend their arguments at construction time;
// the corresponding auto-force happens at pattern match (see autoForceLazy).
func (ch *Checker) wrapAutoThunk(funCore ir.Core, argCore ir.Core, s span.Span) ir.Core {
	if ch.isLazyConApp(funCore) {
		return &ir.Thunk{Comp: argCore, S: s}
	}
	return argCore
}

// isLazyConApp returns true if the Core node is a constructor application
// originating from a lazy form declaration.
func (ch *Checker) isLazyConApp(core ir.Core) bool {
	switch c := core.(type) {
	case *ir.Con:
		if info, ok := ch.reg.LookupConInfo(c.Name); ok {
			return info.IsLazy
		}
	case *ir.App:
		return ch.isLazyConApp(c.Fun)
	case *ir.TyApp:
		return ch.isLazyConApp(c.Expr)
	}
	return false
}

// inferHead infers the type of an expression without instantiating outer foralls.
// Used by ExprTyApp to preserve the forall for explicit type application (@).
func (ch *Checker) inferHead(expr syntax.Expr) (types.Type, ir.Core) {
	switch e := expr.(type) {
	case *syntax.ExprVar:
		ty, coreExpr, _ := ch.lookupVar(e)
		return ty, coreExpr
	case *syntax.ExprCon:
		ty, coreExpr, _ := ch.lookupCon(e)
		return ty, coreExpr
	case *syntax.ExprQualVar:
		ty, coreExpr, _ := ch.lookupQualVar(e)
		return ty, coreExpr
	case *syntax.ExprQualCon:
		ty, coreExpr, _ := ch.lookupQualCon(e)
		return ty, coreExpr
	case *syntax.ExprTyApp:
		innerTy, innerCore := ch.inferHead(e.Expr)
		ty := ch.resolveTypeExpr(e.TyArg)
		innerTy = ch.unifier.Zonk(innerTy)
		f, ok := innerTy.(*types.TyForall)
		if !ok {
			ch.addDiag(diagnostic.ErrBadTypeApp, e.S, diagMsg("type application to non-polymorphic type"))
			return &types.TyError{S: e.S}, innerCore
		}
		// Kind check: verify the type argument has the kind expected by the
		// forall binder. Without this, an explicit @-application can bypass
		// the kind discipline that resolve_type.go enforces for type
		// expressions (e.g. passing a Row-kinded type to a Type-kinded binder).
		if f.Kind != nil {
			if argKind := ch.kindOfType(ty); argKind != nil {
				if err := ch.unifier.Unify(f.Kind, argKind); err != nil {
					ch.addDiag(diagnostic.ErrKindMismatch, e.S,
						diagFmt{Format: "type argument has kind %s, expected %s",
							Args: []any{types.PrettyTypeAsKind(argKind), types.PrettyTypeAsKind(f.Kind)}})
					return &types.TyError{S: e.S}, innerCore
				}
			}
		}
		resultTy := ch.typeOps.Subst(f.Body, f.Var, ty)
		return resultTy, &ir.TyApp{Expr: innerCore, TyArg: ty, S: e.S}
	default:
		// Non-variable/constructor/TyApp expressions cannot be targets of explicit
		// type application. Falling through to infer (which instantiates) is correct:
		// the caller's instantiate call becomes a no-op since foralls are already gone.
		return ch.infer(expr)
	}
}

func (ch *Checker) instantiate(ty types.Type, expr ir.Core) (types.Type, ir.Core) {
	for {
		ty = ch.unifier.Zonk(ty)
		if _, ok := ty.(*types.TyForall); !ok {
			if ev, ok := ty.(*types.TyEvidence); ok {
				for _, entry := range ev.Constraints.ConEntries() {
					if eq, ok := entry.(*types.EqualityEntry); ok {
						ch.emitEq(eq.Lhs, eq.Rhs, eq.S, nil)
						continue
					}
					placeholder := ch.freshName(prefixDictDefer)
					ch.emitClassConstraint(placeholder, entry, expr.Span())
					expr = &ir.App{Fun: expr, Arg: &ir.Var{Name: placeholder, S: expr.Span()}, S: expr.Span()}
				}
				ty = ev.Body
				continue
			}
			return ty, expr
		}
		ty = ch.typeOps.PeelForalls(ty, func(f *types.TyForall) (types.Type, types.LevelExpr) {
			if isLevelKind(f.Kind) {
				// Levels are erased — no TyApp node emitted.
				return ch.freshMeta(types.SortZero), ch.unifier.FreshLevelMeta()
			}
			meta := ch.freshMeta(f.Kind)
			if ch.config.Trace != nil {
				ch.trace(TraceInstantiate, span.Span{}, "instantiate: %s → %s[%s := ?%d]",
					ch.typeOps.Pretty(f), f.Var, ch.typeOps.Pretty(meta), meta.ID)
			}
			expr = &ir.TyApp{Expr: expr, TyArg: meta, S: expr.Span()}
			return meta, nil
		})
	}
}

func (ch *Checker) patternName(p syntax.Pattern) string {
	switch pat := p.(type) {
	case *syntax.PatVar:
		return pat.Name
	case *syntax.PatWild:
		return "_"
	case *syntax.PatParen:
		return ch.patternName(pat.Inner)
	default:
		return "_"
	}
}

// inferList handles list literal [e1, e2, ...] by desugaring to Cons/Nil chain.
func (ch *Checker) inferList(e *syntax.ExprList) (types.Type, ir.Core) {
	elemTy := ch.freshMeta(types.TypeOfTypes)
	listTy := &types.TyApp{Fun: ch.typeOps.Con("List", span.Span{}), Arg: elemTy, Flags: types.MetaFreeFlags(ch.typeOps.Con("List", span.Span{}), elemTy)}

	// Build from the end: Nil, then Cons e_n (Cons e_{n-1} ...)
	nilMod, nilOk := ch.reg.LookupConModule("Nil")
	consMod, consOk := ch.reg.LookupConModule("Cons")
	if !nilOk || !consOk {
		ch.addDiag(diagnostic.ErrUnboundCon, e.S, diagMsg("list literals require Prelude; add 'import Prelude'"))
		return ch.errorPair(e.S)
	}
	var result ir.Core = &ir.Con{Name: "Nil", Module: nilMod, S: e.S}

	for i := len(e.Elems) - 1; i >= 0; i-- {
		elemCore := ch.check(e.Elems[i], elemTy)
		result = &ir.App{
			Fun: &ir.App{
				Fun: &ir.Con{Name: "Cons", Module: consMod, S: e.S},
				Arg: elemCore,
				S:   e.S,
			},
			Arg: result,
			S:   e.S,
		}
	}

	return ch.unifier.Zonk(listTy), result
}
