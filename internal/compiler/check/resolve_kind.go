package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func (ch *Checker) resolveKindExpr(k syntax.KindExpr) types.Kind {
	if k == nil {
		return types.KType{}
	}
	switch ke := k.(type) {
	case *syntax.KindExprType:
		return types.KType{}
	case *syntax.KindExprRow:
		return types.KRow{}
	case *syntax.KindExprConstraint:
		return types.KConstraint{}
	case *syntax.KindExprArrow:
		return &types.KArrow{From: ch.resolveKindExpr(ke.From), To: ch.resolveKindExpr(ke.To)}
	case *syntax.KindExprName:
		if ch.reg.kindVars[ke.Name] {
			return types.KVar{Name: ke.Name}
		}
		if pk, ok := ch.reg.promotedKinds[ke.Name]; ok {
			return pk
		}
		return types.KType{}
	case *syntax.KindExprSort:
		return types.KSort{}
	default:
		ch.addCodedError(diagnostic.ErrKindMismatch, k.Span(), fmt.Sprintf("unsupported kind expression: %T", k))
		return types.KType{}
	}
}

// checkTypeAppKind validates that a type application F A is kind-correct.
// Only checks when:
//   - F has an explicitly annotated parameter kind (not the default KType)
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
	funKind = ch.unifier.ZonkKind(funKind)
	ka, ok := funKind.(*types.KArrow)
	if !ok {
		return
	}
	// Skip if the parameter kind is the default KType (unannotated parameter).
	if _, isType := ka.From.(types.KType); isType {
		return
	}
	argKind := ch.kindOfType(arg)
	if argKind == nil {
		return
	}
	argKind = ch.unifier.ZonkKind(argKind)
	if _, isMeta := argKind.(*types.KMeta); isMeta {
		return
	}
	if err := ch.unifier.UnifyKinds(ka.From, argKind); err != nil {
		ch.addCodedError(diagnostic.ErrKindMismatch, s,
			fmt.Sprintf("kind mismatch in type application: expected kind %s, got %s", ka.From, argKind))
	}
}

// hasDeterministicKind returns true if the type's kind is deterministic
// (i.e., derived from a registered type constructor, not a defaulted TyVar).
func (ch *Checker) hasDeterministicKind(ty types.Type) bool {
	switch t := ty.(type) {
	case *types.TyCon:
		_, inReg := ch.reg.typeKinds[t.Name]
		_, inProm := ch.reg.promotedCons[t.Name]
		_, isAlias := ch.reg.aliases[t.Name]
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

// isModuleDefinedType checks if a type name was defined by the module itself
// (via data declarations or class declarations), as opposed to being inherited
// from built-in types or open imports.
func isModuleDefinedType(exports *ModuleExports, name string) bool {
	for _, dd := range exports.DataDecls {
		if dd.Name == name {
			return true
		}
	}
	// Classes are already checked separately, but class-defined types
	// (dict types) might appear in Types.
	if _, ok := exports.Classes[name]; ok {
		return true
	}
	return false
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
	if _, ok := ch.reg.typeKinds[name]; ok {
		return true
	}
	if _, ok := ch.reg.aliases[name]; ok {
		return true
	}
	if _, ok := ch.reg.families[name]; ok {
		return true
	}
	if _, ok := ch.reg.classes[name]; ok {
		return true
	}
	if _, ok := ch.reg.promotedKinds[name]; ok {
		return true
	}
	if _, ok := ch.reg.promotedCons[name]; ok {
		return true
	}
	return false
}

// aliasParamKind returns the kind of the i-th parameter of a type alias.
func (ch *Checker) aliasParamKind(aliasName string, i int) types.Kind {
	info, ok := ch.reg.aliases[aliasName]
	if !ok || i >= len(info.ParamKinds) {
		return types.KType{}
	}
	return info.ParamKinds[i]
}

// kindOfType returns the kind of a resolved type, or nil if unknown.
func (ch *Checker) kindOfType(ty types.Type) types.Kind {
	switch t := ty.(type) {
	case *types.TyCon:
		if k, ok := ch.reg.typeKinds[t.Name]; ok {
			return k
		}
		// Type aliases: compute kind from parameter kinds.
		if info, ok := ch.reg.aliases[t.Name]; ok {
			var kind types.Kind = types.KType{}
			for i := len(info.Params) - 1; i >= 0; i-- {
				paramKind := ch.aliasParamKind(t.Name, i)
				kind = &types.KArrow{From: paramKind, To: kind}
			}
			return kind
		}
		if k, ok := ch.reg.promotedCons[t.Name]; ok {
			return k
		}
		return types.KType{}
	case *types.TyApp:
		funKind := ch.kindOfType(t.Fun)
		if ka, ok := funKind.(*types.KArrow); ok {
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
		return types.KType{}
	default:
		return types.KType{}
	}
}
