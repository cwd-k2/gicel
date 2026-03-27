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
	default:
		ch.addCodedError(diagnostic.ErrKindMismatch, k.Span(), fmt.Sprintf("unsupported kind expression: %T", k))
		return types.TypeOfTypes
	}
}

// checkTypeAppKind validates that a type application F A is kind-correct.
// Only checks when:
//   - F has an explicitly annotated parameter kind (not the default Type)
//   - A is a concrete type constructor (TyCon or TyApp) with a deterministic kind
//
// This avoids false positives from type variables whose kind isn't yet in context.
func (ch *Checker) checkTypeAppKind(fun, arg types.Type, s span.Span) {
	// Only check when arg has a deterministic kind (concrete TyCon, not TyVar).
	if !ch.hasDeterministicKind(arg) {
		return
	}
	funKind := ch.kindOfType(fun)
	if funKind == nil {
		return
	}
	funKind = ch.unifier.Zonk(funKind)
	ka, ok := funKind.(*types.TyArrow)
	if !ok {
		return
	}
	// Skip if the parameter kind is the default TypeOfTypes (unannotated parameter).
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
	ch.emitEq(ka.From, argKind, s, &solve.CtOrigin{
		Code:    diagnostic.ErrKindMismatch,
		Context: fmt.Sprintf("kind mismatch in type application: expected kind %s, got %s", types.PrettyTypeAsKind(ka.From), types.PrettyTypeAsKind(argKind)),
	})
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

// builtinTypeNames are type constructor names that are intrinsic to the checker
// (used in TyCBPV expansion) but not registered in RegisteredTypes.
var builtinTypeNames = map[string]bool{
	types.TyConComputation: true,
	types.TyConThunk:       true,
}

// isKnownTypeName returns true if name refers to a known type: registered type,
// parameterized alias, parameterized type family, class, promoted kind/constructor,
// or checker-intrinsic type (Computation, Thunk).
func (ch *Checker) isKnownTypeName(name string) bool {
	if builtinTypeNames[name] {
		return true
	}
	if _, ok := ch.reg.LookupTypeKind(name); ok {
		return true
	}
	if _, ok := ch.lookupAlias(name); ok {
		return true
	}
	if _, ok := ch.lookupFamily(name); ok {
		return true
	}
	if _, ok := ch.reg.LookupClass(name); ok {
		return true
	}
	if ch.reg.HasPromotedKind(name) {
		return true
	}
	if ch.reg.HasPromotedCon(name) {
		return true
	}
	return false
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
