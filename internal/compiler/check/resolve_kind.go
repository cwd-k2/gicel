package check

import (
	"fmt"

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
			r.addCodedError(diagnostic.ErrKindMismatch, ke.S,
				fmt.Sprintf("unrecognized kind name %q", ke.Name))
			return types.TypeOfTypes
		}
	case *syntax.TyExprVar:
		if r.reg.IsKindVar(ke.Name) {
			return &types.TyVar{Name: ke.Name}
		}
		if r.reg.IsLevelVar(ke.Name) {
			// Level variable in kind position: treat as Type at that level.
			return &types.TyCon{Name: "Type", Level: &types.LevelVar{Name: ke.Name}}
		}
		// Lowercase names in kind position get a fresh LevelMeta, making
		// the universe level inferrable. This replaces the former
		// TypeOfTypes default with evidence-based level inference.
		return &types.TyCon{Name: "Type", Level: r.unifier.FreshLevelMeta()}
	case *syntax.TyExprApp:
		// Handle Type l (Type applied to a level argument).
		if con, ok := ke.Fun.(*syntax.TyExprCon); ok && con.Name == "Type" {
			level := r.resolveLevelExpr(ke.Arg)
			return &types.TyCon{Name: "Type", Level: level}
		}
		// Kind-level application (e.g., F A in kind position).
		return &types.TyApp{Fun: r.resolveKindExpr(ke.Fun), Arg: r.resolveKindExpr(ke.Arg)}
	case *syntax.TyExprArrow:
		return &types.TyArrow{From: r.resolveKindExpr(ke.From), To: r.resolveKindExpr(ke.To)}
	case *syntax.TyExprParen:
		return r.resolveKindExpr(ke.Inner)
	case *syntax.TyExprError:
		return types.TypeOfTypes
	default:
		r.addCodedError(diagnostic.ErrKindMismatch, k.Span(), fmt.Sprintf("unsupported kind expression: %T", k))
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
		// Future: numeric level literals or named levels.
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
			r.addCodedError(diagnostic.ErrKindMismatch, s,
				fmt.Sprintf("type %s has kind %s and cannot be applied to a type argument",
					types.Pretty(fun), types.PrettyTypeAsKind(funKind)))
		}
		return
	}
	// When the parameter kind is Type with a LevelMeta (from unannotated
	// params), resolve via level unification rather than skipping entirely.
	// This provides implicit kind polymorphism constrained by universe level:
	// unannotated parameters accept any kind at a compatible level.
	if tc, ok := ka.From.(*types.TyCon); ok && tc.Name == "Type" {
		if _, isLevelMeta := tc.Level.(*types.LevelMeta); isLevelMeta {
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
		// Concrete Type(L1): also skip for backward compatibility
		// with explicitly annotated `:: Type` parameters.
		if types.LevelEqual(tc.Level, types.L1) {
			return
		}
	}
	argKind := r.kindOfType(arg)
	if argKind == nil {
		return
	}
	argKind = r.unifier.Zonk(argKind)
	if m, isMeta := argKind.(*types.TyMeta); isMeta && types.Equal(m.Kind, types.SortZero) {
		return
	}
	r.emitEq(ka.From, argKind, s, solve.WithLazyContext(diagnostic.ErrKindMismatch, func() string {
		return "kind mismatch in type application: expected kind " + types.PrettyTypeAsKind(ka.From) + ", got " + types.PrettyTypeAsKind(argKind)
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

// kindOfType returns the kind of a resolved type, or nil if unknown.
func (r *typeResolver) kindOfType(ty types.Type) types.Type {
	switch t := ty.(type) {
	case *types.TyCon:
		if k, ok := r.reg.LookupTypeKind(t.Name); ok {
			return k
		}
		// Type aliases: compute kind from parameter kinds.
		if info, ok := r.lookupAlias(t.Name); ok {
			var kind types.Type = types.TypeOfTypes
			for i := len(info.Params) - 1; i >= 0; i-- {
				paramKind := r.aliasParamKind(t.Name, i)
				kind = &types.TyArrow{From: paramKind, To: kind}
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
				kind = &types.TyArrow{From: cls.TyParamKinds[i], To: kind}
			}
			return kind
		}
		// Type families: kind = paramKind₁ → ... → paramKindₙ → resultKind.
		if fam, ok := r.lookupFamily(t.Name); ok {
			var kind types.Type = fam.ResultKind
			for i := len(fam.Params) - 1; i >= 0; i-- {
				kind = &types.TyArrow{From: fam.Params[i].Kind, To: kind}
			}
			return kind
		}
		if t.IsLabel {
			return types.TypeOfLabels
		}
		return types.TypeOfTypes
	case *types.TyApp:
		funKind := r.kindOfType(t.Fun)
		if ka, ok := funKind.(*types.TyArrow); ok {
			return ka.To
		}
		return nil
	case *types.TyMeta:
		return t.Kind
	case *types.TySkolem:
		return t.Kind
	case *types.TyVar:
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
