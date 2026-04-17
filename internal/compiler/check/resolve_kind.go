package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func (r *typeResolver) resolveKindExpr(k syntax.TypeExpr) types.Type {
	if k == nil {
		return types.TypeOfTypes
	}
	switch ke := k.(type) {
	case *syntax.TyExprCon:
		switch ke.Name {
		case "Type":
			return types.TypeOfTypes
		case "Row":
			return types.TypeOfRows
		case "Constraint":
			return types.TypeOfConstraints
		case "Kind":
			return types.SortZero
		case "Level":
			return types.TypeOfLevels
		case "Label":
			return types.TypeOfLabels
		default:
			if r.reg.IsKindVar(ke.Name) {
				return &types.TyVar{Name: ke.Name}
			}
			if pk, ok := r.reg.LookupPromotedKind(ke.Name); ok {
				return pk
			}
			// Uppercase names are constructors — unrecognized ones are likely typos.
			r.addDiag(diagnostic.ErrKindMismatch, ke.S,
				diagFmt{Format: "unrecognized kind name %q", Args: []any{ke.Name}})
			return types.TypeOfTypes
		}
	case *syntax.TyExprVar:
		if r.reg.IsKindVar(ke.Name) {
			return &types.TyVar{Name: ke.Name}
		}
		if r.reg.IsLevelVar(ke.Name) {
			// Level variable in kind position: treat as Type at that level.
			return r.typeOps.ConLevel("Type", &types.LevelVar{Name: ke.Name}, false, span.Span{})
		}
		// Lowercase names in kind position get a fresh LevelMeta, making
		// the universe level inferrable. This replaces the former
		// TypeOfTypes default with evidence-based level inference.
		return r.typeOps.ConLevel("Type", r.unifier.FreshLevelMeta(), false, span.Span{})
	case *syntax.TyExprApp:
		// Handle Type l (Type applied to a level argument).
		if con, ok := ke.Fun.(*syntax.TyExprCon); ok && con.Name == "Type" {
			level := r.resolveLevelExpr(ke.Arg)
			return r.typeOps.ConLevel("Type", level, false, span.Span{})
		}
		// Kind-level application (e.g., F A in kind position).
		return r.typeOps.App(r.resolveKindExpr(ke.Fun), r.resolveKindExpr(ke.Arg), span.Span{})
	case *syntax.TyExprArrow:
		return r.typeOps.Arrow(r.resolveKindExpr(ke.From), r.resolveKindExpr(ke.To), span.Span{})
	case *syntax.TyExprParen:
		return r.resolveKindExpr(ke.Inner)
	case *syntax.TyExprError:
		return types.TypeOfTypes
	default:
		r.addDiag(diagnostic.ErrKindMismatch, k.Span(), diagFmt{Format: "unsupported kind expression: %T", Args: []any{k}})
		return types.TypeOfTypes
	}
}

// resolveLevelExpr resolves a syntax-level expression to a LevelExpr.
// Handles level variables, max/succ applications, and defaults to FreshLevelMeta.
func (r *typeResolver) resolveLevelExpr(e syntax.TypeExpr) types.LevelExpr {
	switch ee := e.(type) {
	case *syntax.TyExprVar:
		if r.reg.IsLevelVar(ee.Name) {
			return &types.LevelVar{Name: ee.Name}
		}
		return r.unifier.FreshLevelMeta()
	case *syntax.TyExprCon:
		return r.unifier.FreshLevelMeta()
	case *syntax.TyExprApp:
		// max l1 l2 → LevelMax{A, B}
		if app2, ok := ee.Fun.(*syntax.TyExprApp); ok {
			if con, ok := app2.Fun.(*syntax.TyExprCon); ok && con.Name == types.LevelFnMax {
				return &types.LevelMax{
					A: r.resolveLevelExpr(app2.Arg),
					B: r.resolveLevelExpr(ee.Arg),
				}
			}
		}
		// succ l → LevelSucc{E}
		if con, ok := ee.Fun.(*syntax.TyExprCon); ok && con.Name == types.LevelFnSucc {
			return &types.LevelSucc{E: r.resolveLevelExpr(ee.Arg)}
		}
		return r.unifier.FreshLevelMeta()
	case *syntax.TyExprParen:
		return r.resolveLevelExpr(ee.Inner)
	default:
		return r.unifier.FreshLevelMeta()
	}
}

// checkTypeAppKind validates that a type application F A is kind-correct.
// Checks two properties:
//  1. F must have an arrow kind (k1 -> k2) to accept an argument.
//  2. The argument kind must match the parameter kind.
//
// Skips checking when kinds are indeterminate (metas, unresolved variables).
func (r *typeResolver) checkTypeAppKind(fun, arg types.Type, s span.Span) {
	if !r.hasDeterministicKind(arg) {
		return
	}
	funKind := r.kindOfType(fun)
	if funKind == nil {
		return
	}
	funKind = r.unifier.Zonk(funKind)
	// Skip if funKind is a meta — kind not yet determined.
	if _, isMeta := funKind.(*types.TyMeta); isMeta {
		return
	}
	ka, ok := funKind.(*types.TyArrow)
	if !ok {
		// F has a non-arrow kind — report error only when F's kind is reliably known.
		if r.hasDeterministicKind(fun) {
			r.addDiag(diagnostic.ErrKindMismatch, s,
				diagFmt{Format: "type %s has kind %s and cannot be applied to a type argument",
					Args: []any{r.typeOps.Pretty(fun), r.typeOps.PrettyTypeAsKind(funKind)}})
		}
		return
	}
	// When the parameter kind is Type (any level, including LevelMeta),
	// resolve via level unification. This provides implicit kind polymorphism
	// constrained by universe level: Type(L1) accepts any L1 kind (Type, Row,
	// promoted data kinds), and Type(?l) infers the level from usage.
	if tc, ok := ka.From.(*types.TyCon); ok && tc.Name == "Type" {
		argKind := r.kindOfType(arg)
		if argKind == nil {
			return
		}
		argKind = r.unifier.Zonk(argKind)
		if argCon, ok := argKind.(*types.TyCon); ok {
			_ = r.unifier.UnifyLevels(tc.Level, argCon.Level)
		}
		return
	}
	argKind := r.kindOfType(arg)
	if argKind == nil {
		return
	}
	argKind = r.unifier.Zonk(argKind)
	if m, isMeta := argKind.(*types.TyMeta); isMeta && r.typeOps.Equal(m.Kind, types.SortZero) {
		return
	}
	// Skip when the expected parameter kind is a type variable (kind-polymorphic).
	// The parametric kind will be checked during forall instantiation at check time.
	if _, isVar := ka.From.(*types.TyVar); isVar {
		return
	}
	r.emitEq(ka.From, argKind, s, solve.WithLazyContext(diagnostic.ErrKindMismatch, func() string {
		return "kind mismatch in type application: expected kind " + r.typeOps.PrettyTypeAsKind(ka.From) + ", got " + r.typeOps.PrettyTypeAsKind(argKind)
	}))
}

// hasDeterministicKind returns true if the type's kind is deterministic
// (i.e., derived from a registered type constructor, not a defaulted TyVar).
func (r *typeResolver) hasDeterministicKind(ty types.Type) bool {
	switch t := ty.(type) {
	case *types.TyCon:
		_, inReg := r.reg.LookupTypeKind(t.Name)
		inProm := r.reg.HasPromotedCon(t.Name)
		_, isAlias := r.lookupAlias(t.Name)
		_, isClass := r.reg.LookupClass(t.Name)
		_, isFamily := r.lookupFamily(t.Name)
		return inReg || inProm || isAlias || isClass || isFamily
	case *types.TyApp:
		// Recurse on the head to check if it's deterministic.
		head, _ := types.AppSpineHead(ty)
		if head != ty {
			return r.hasDeterministicKind(head)
		}
		return false
	case *types.TyMeta:
		return true
	case *types.TySkolem:
		return true
	case *types.TyVar:
		// Check forall-bound kind (populated during forall body resolution).
		if r.forallKinds != nil {
			if _, ok := r.forallKinds[t.Name]; ok {
				return true
			}
		}
		// Check checker context (populated during check phase).
		if k, ok := r.lookupTyVar(t.Name); ok && k != nil {
			return true
		}
		return false
	default:
		return false
	}
}

// aliasParamKind returns the kind of the i-th parameter of a type alias.
func (r *typeResolver) aliasParamKind(aliasName string, i int) types.Type {
	info, ok := r.lookupAlias(aliasName)
	if !ok || i >= len(info.ParamKinds) {
		return types.TypeOfTypes
	}
	return info.ParamKinds[i]
}

// extractTypeLevel returns the universe level from a Type-kinded TyCon.
// Returns (level, true) when k is TyCon "Type"; (nil, false) otherwise.
func extractTypeLevel(k types.Type) (types.LevelExpr, bool) {
	tc, ok := k.(*types.TyCon)
	if !ok || tc.Name != "Type" {
		return nil, false
	}
	if tc.Level == nil {
		return types.L0, true
	}
	return tc.Level, true
}

// joinLevel computes LevelMax(a, b), eliding the node when both sides are equal.
func joinLevel(a, b types.LevelExpr) types.LevelExpr {
	if types.LevelEqual(a, b) {
		return a
	}
	return &types.LevelMax{A: a, B: b}
}

// typeAtMaxLevel returns Type(max(l_a, l_b)) when both kinds are Type-kinded.
// Falls back to TypeOfTypes when either kind is nil or non-Type-kinded.
func typeAtMaxLevel(kindA, kindB types.Type) types.Type {
	if kindA == nil || kindB == nil {
		return types.TypeOfTypes
	}
	la, okA := extractTypeLevel(kindA)
	lb, okB := extractTypeLevel(kindB)
	if okA && okB {
		return &types.TyCon{Name: "Type", Level: joinLevel(la, lb)}
	}
	return types.TypeOfTypes
}

// typeAtMaxLevelOps is the TypeOps-aware variant of typeAtMaxLevel.
func typeAtMaxLevelOps(ops *types.TypeOps, kindA, kindB types.Type) types.Type {
	if kindA == nil || kindB == nil {
		return types.TypeOfTypes
	}
	la, okA := extractTypeLevel(kindA)
	lb, okB := extractTypeLevel(kindB)
	if okA && okB {
		return ops.ConLevel("Type", joinLevel(la, lb), false, span.Span{})
	}
	return types.TypeOfTypes
}

// kindOfType returns the kind of a resolved type, or nil if unknown.
func (r *typeResolver) kindOfType(ty types.Type) types.Type {
	switch t := ty.(type) {
	case *types.TyCon:
		// Label literals have kind Label, regardless of universe level.
		if t.IsLabel {
			return types.TypeOfLabels
		}
		// Universe stratification: L1 TyCons (Type, Row, Constraint,
		// promoted data kinds) have kind Kind (SortZero). L2 TyCons
		// (Kind itself) sit at the top — return nil (no further kind).
		if types.LevelEqual(t.Level, types.L1) {
			return types.SortZero
		}
		if types.LevelEqual(t.Level, types.L2) {
			return nil
		}
		if k, ok := r.reg.LookupTypeKind(t.Name); ok {
			return k
		}
		// Type aliases: compute kind from parameter kinds.
		if info, ok := r.lookupAlias(t.Name); ok {
			var kind types.Type = types.TypeOfTypes
			for i := len(info.Params) - 1; i >= 0; i-- {
				paramKind := r.aliasParamKind(t.Name, i)
				kind = r.typeOps.Arrow(paramKind, kind, span.Span{})
			}
			return kind
		}
		if k, ok := r.reg.LookupPromotedCon(t.Name); ok {
			return k
		}
		// Type classes: kind = paramKind₁ → ... → paramKindₙ → Constraint.
		if cls, ok := r.reg.LookupClass(t.Name); ok {
			var kind types.Type = types.TypeOfConstraints
			for i := len(cls.TyParamKinds) - 1; i >= 0; i-- {
				kind = r.typeOps.Arrow(cls.TyParamKinds[i], kind, span.Span{})
			}
			return kind
		}
		// Type families: kind = paramKind₁ → ... → paramKindₙ → resultKind.
		if fam, ok := r.lookupFamily(t.Name); ok {
			var kind types.Type = fam.ResultKind
			for i := len(fam.Params) - 1; i >= 0; i-- {
				kind = r.typeOps.Arrow(fam.Params[i].Kind, kind, span.Span{})
			}
			return kind
		}
		if t.IsLabel {
			return types.TypeOfLabels
		}
		return types.TypeOfTypes
	case *types.TyArrow:
		// Universe-polymorphic function arrow:
		// (A : Type l₁) -> (B : Type l₂) has kind Type(max(l₁, l₂)).
		return typeAtMaxLevelOps(r.typeOps, r.kindOfType(t.From), r.kindOfType(t.To))
	case *types.TyApp:
		funKind := r.kindOfType(t.Fun)
		if ka, ok := funKind.(*types.TyArrow); ok {
			return ka.To
		}
		return nil
	case *types.TyMeta:
		// Zonk the Kind to resolve stale TyVar references left by PeelForalls.
		// Defense-in-depth: PeelForalls applies accumulated substitutions to
		// Kind before visiting, but Zonk ensures correctness if a meta's Kind
		// was set before outer binders were solved.
		return r.unifier.Zonk(t.Kind)
	case *types.TySkolem:
		return t.Kind
	case *types.TyEvidenceRow:
		return t.Entries.FiberKind()
	case *types.TyEvidence:
		return types.TypeOfTypes
	case *types.TyCBPV:
		return types.TypeOfTypes
	case *types.TyForall:
		return types.TypeOfTypes
	case *types.TyFamilyApp:
		if fam, ok := r.lookupFamily(t.Name); ok {
			return fam.ResultKind
		}
		return nil
	case *types.TyError:
		return nil
	case *types.TyVar:
		// Forall-bound kind (available during type resolution phase).
		if r.forallKinds != nil {
			if k, ok := r.forallKinds[t.Name]; ok {
				return k
			}
		}
		// Checker context (available during check phase).
		if k, ok := r.lookupTyVar(t.Name); ok {
			return k
		}
		return types.TypeOfTypes
	default:
		return types.TypeOfTypes
	}
}

// promotedFieldKind computes the kind to use for a constructor field when
// promoting the constructor to the type level (DataKinds). If the field type
// is a data type that has itself been promoted, the promoted kind is used
// instead of the value-level kind. For example, a field of type Nat (kind
// Type at value level) becomes kind Nat (the promoted data kind) at the
// type level, so that S :: Nat -> Nat rather than S :: Type -> Nat.
func (r *typeResolver) promotedFieldKind(fieldType types.Type) types.Type {
	switch t := fieldType.(type) {
	case *types.TyCon:
		if pk, ok := r.reg.LookupPromotedKind(t.Name); ok {
			return pk
		}
	case *types.TyApp:
		// For applied types like (Maybe Nat), the overall kind is determined
		// by the head constructor's kind applied to arguments — which is
		// the value-level kind (Type), not a promoted kind.
	case *types.TyVar:
		// Type variables in promoted context represent kind variables.
		// Their kind is Type (the default for value-level type vars).
	}
	k := r.kindOfType(fieldType)
	if k == nil {
		return types.TypeOfTypes
	}
	return k
}
