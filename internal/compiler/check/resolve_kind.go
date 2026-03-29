package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func (ch *Checker) resolveKindExpr(k syntax.TypeExpr) types.Type {
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
		case "Label":
			return types.TypeOfLabels
		default:
			if ch.reg.IsKindVar(ke.Name) {
				return &types.TyVar{Name: ke.Name}
			}
			if pk, ok := ch.reg.LookupPromotedKind(ke.Name); ok {
				return pk
			}
			// Uppercase names are constructors — unrecognized ones are likely typos.
			ch.addCodedError(diagnostic.ErrKindMismatch, ke.S,
				fmt.Sprintf("unrecognized kind name %q", ke.Name))
			return types.TypeOfTypes
		}
	case *syntax.TyExprVar:
		if ch.reg.IsKindVar(ke.Name) {
			return &types.TyVar{Name: ke.Name}
		}
		// Lowercase names in kind position are treated as defaulting to Type.
		// This supports kind-polymorphic annotations where the variable is
		// not explicitly bound as a kind variable.
		return types.TypeOfTypes
	case *syntax.TyExprArrow:
		return &types.TyArrow{From: ch.resolveKindExpr(ke.From), To: ch.resolveKindExpr(ke.To)}
	case *syntax.TyExprParen:
		return ch.resolveKindExpr(ke.Inner)
	case *syntax.TyExprError:
		return types.TypeOfTypes
	default:
		ch.addCodedError(diagnostic.ErrKindMismatch, k.Span(), fmt.Sprintf("unsupported kind expression: %T", k))
		return types.TypeOfTypes
	}
}

// checkTypeAppKind validates that a type application F A is kind-correct.
// Checks two properties:
//  1. F must have an arrow kind (k1 -> k2) to accept an argument.
//  2. The argument kind must match the parameter kind.
//
// Skips checking when kinds are indeterminate (metas, unresolved variables).
func (ch *Checker) checkTypeAppKind(fun, arg types.Type, s span.Span) {
	if !ch.hasDeterministicKind(arg) {
		return
	}
	funKind := ch.kindOfType(fun)
	if funKind == nil {
		return
	}
	funKind = ch.unifier.Zonk(funKind)
	// Skip if funKind is a meta — kind not yet determined.
	if _, isMeta := funKind.(*types.TyMeta); isMeta {
		return
	}
	ka, ok := funKind.(*types.TyArrow)
	if !ok {
		// F has a non-arrow kind — report error only when F's kind is reliably known
		// (registered type or alias). Type classes and type families are not tracked
		// in LookupTypeKind, so their kindOfType defaults to TypeOfTypes; we must
		// not reject those applications.
		if ch.hasDeterministicKind(fun) {
			ch.addCodedError(diagnostic.ErrKindMismatch, s,
				fmt.Sprintf("type %s has kind %s and cannot be applied to a type argument",
					types.Pretty(fun), types.PrettyTypeAsKind(funKind)))
		}
		return
	}
	// Skip argument kind check when the parameter kind is the default TypeOfTypes.
	// This means the parameter kind was not explicitly annotated, so we cannot
	// reliably compare it against the argument kind (P3 design debt).
	if types.Equal(ka.From, types.TypeOfTypes) {
		return
	}
	argKind := ch.kindOfType(arg)
	if argKind == nil {
		return
	}
	argKind = ch.unifier.Zonk(argKind)
	if m, isMeta := argKind.(*types.TyMeta); isMeta && types.Equal(m.Kind, types.SortZero) {
		return
	}
	ch.emitEq(ka.From, argKind, s, solve.WithLazyContext(diagnostic.ErrKindMismatch, func() string {
		return "kind mismatch in type application: expected kind " + types.PrettyTypeAsKind(ka.From) + ", got " + types.PrettyTypeAsKind(argKind)
	}))
}

// hasDeterministicKind returns true if the type's kind is deterministic
// (i.e., derived from a registered type constructor, not a defaulted TyVar).
func (ch *Checker) hasDeterministicKind(ty types.Type) bool {
	switch t := ty.(type) {
	case *types.TyCon:
		_, inReg := ch.reg.LookupTypeKind(t.Name)
		inProm := ch.reg.HasPromotedCon(t.Name)
		_, isAlias := ch.lookupAlias(t.Name)
		return inReg || inProm || isAlias
	case *types.TyApp:
		// Recurse on the head to check if it's deterministic.
		head, _ := types.UnwindApp(ty)
		if head != ty {
			return ch.hasDeterministicKind(head)
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
func (ch *Checker) aliasParamKind(aliasName string, i int) types.Type {
	info, ok := ch.lookupAlias(aliasName)
	if !ok || i >= len(info.ParamKinds) {
		return types.TypeOfTypes
	}
	return info.ParamKinds[i]
}

// kindOfType returns the kind of a resolved type, or nil if unknown.
func (ch *Checker) kindOfType(ty types.Type) types.Type {
	switch t := ty.(type) {
	case *types.TyCon:
		if k, ok := ch.reg.LookupTypeKind(t.Name); ok {
			return k
		}
		// Type aliases: compute kind from parameter kinds.
		if info, ok := ch.lookupAlias(t.Name); ok {
			var kind types.Type = types.TypeOfTypes
			for i := len(info.Params) - 1; i >= 0; i-- {
				paramKind := ch.aliasParamKind(t.Name, i)
				kind = &types.TyArrow{From: paramKind, To: kind}
			}
			return kind
		}
		if k, ok := ch.reg.LookupPromotedCon(t.Name); ok {
			return k
		}
		if t.IsLabel {
			return types.TypeOfLabels
		}
		return types.TypeOfTypes
	case *types.TyApp:
		funKind := ch.kindOfType(t.Fun)
		if ka, ok := funKind.(*types.TyArrow); ok {
			return ka.To
		}
		return nil
	case *types.TyMeta:
		return t.Kind
	case *types.TySkolem:
		return t.Kind
	case *types.TyVar:
		if k, ok := ch.ctx.LookupTyVar(t.Name); ok {
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
func (ch *Checker) promotedFieldKind(fieldType types.Type) types.Type {
	switch t := fieldType.(type) {
	case *types.TyCon:
		if pk, ok := ch.reg.LookupPromotedKind(t.Name); ok {
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
	k := ch.kindOfType(fieldType)
	if k == nil {
		return types.TypeOfTypes
	}
	return k
}
