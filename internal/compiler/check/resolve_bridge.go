// resolve_bridge.go — Type/kind/name resolution boundary.
//
// typeResolver holds the subset of Checker capabilities needed for
// surface-syntax → internal-type resolution. Methods on *typeResolver
// cannot access bidirectional checking, constraint solving, or
// declaration processing — only registry lookups, scope, unifier,
// and error reporting.
//
// This follows the same struct-injection pattern as family.ReduceEnv:
// capabilities are injected at construction time, preventing accidental
// coupling to unrelated Checker responsibilities.
package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// typeResolver performs surface-syntax to internal-type resolution.
// It holds only the capabilities needed for type, kind, and name
// resolution — no access to infer/check, solver, or declaration state.
type typeResolver struct {
	// Registry lookups (read-only after declaration phases).
	reg *Registry

	// Module scoping for qualified name resolution.
	scope *Scope

	// Unifier for kind checking (Zonk, Unify).
	unifier *unify.Unifier

	// Scope-aware alias and family lookups (registry + injected).
	lookupAlias  func(string) (*AliasInfo, bool)
	lookupFamily func(string) (*TypeFamilyInfo, bool)

	// Context lookup for type variable kinds.
	lookupTyVar func(string) (types.Type, bool)

	// Constraint emission callback.
	emitEq func(types.Type, types.Type, span.Span, *solve.CtOrigin)

	// Error reporting callback.
	addDiag func(diagnostic.Code, span.Span, diagnostic.DetailFormatter)

	// Strict type name validation (enabled after declaration processing).
	strictTypeNames *bool

	// labelVars tracks forall-bound label-kinded variables currently in scope.
	// Used by row field resolution to mark IsLabelVar on RowField.
	labelVars map[string]bool

	// forallKinds tracks forall-bound type variable kinds during body resolution.
	// Populated by resolveTypeExpr(TyExprForall), consumed by hasDeterministicKind
	// and kindOfType to enable kind checking inside quantified type annotations.
	forallKinds map[string]types.Type
}

// typeResolver returns the cached typeResolver, constructing it on first use.
// Caching is safe because all fields are references or pointers to Checker
// state, so mutations are visible through the cached object.
func (ch *Checker) typeResolver() *typeResolver {
	if ch.cachedTypeResolver == nil {
		ch.cachedTypeResolver = &typeResolver{
			reg:             ch.reg,
			scope:           ch.scope,
			unifier:         ch.unifier,
			lookupAlias:     ch.lookupAlias,
			lookupFamily:    ch.lookupFamily,
			lookupTyVar:     ch.ctx.LookupTyVar,
			emitEq:          ch.emitEq,
			addDiag:         ch.addDiag,
			strictTypeNames: &ch.CheckState.strictTypeNames,
		}
	}
	return ch.cachedTypeResolver
}

// --- Scope-aware lookups (injected into typeResolver) ---

// lookupAlias searches for a type alias by name: first in the Registry
// (populated during declaration phases), then in Scope's injected aliases
// (populated from qualified references).
func (ch *Checker) lookupAlias(name string) (*AliasInfo, bool) {
	if info, ok := ch.reg.LookupAlias(name); ok {
		return info, true
	}
	if ch.scope != nil && ch.scope.injectedAliases != nil {
		if info, ok := ch.scope.injectedAliases[name]; ok {
			return info, true
		}
	}
	return nil, false
}

// lookupFamily searches for a type family by name: first in the Registry,
// then in Scope's injected families.
func (ch *Checker) lookupFamily(name string) (*TypeFamilyInfo, bool) {
	if info, ok := ch.reg.LookupFamily(name); ok {
		return info, true
	}
	if ch.scope != nil && ch.scope.injectedFamilies != nil {
		if info, ok := ch.scope.injectedFamilies[name]; ok {
			return info, true
		}
	}
	return nil, false
}

// --- Thin wrappers — preserve existing Checker API ---

func (ch *Checker) resolveTypeExpr(texpr syntax.TypeExpr) types.Type {
	return ch.typeResolver().resolveTypeExpr(texpr)
}

func (ch *Checker) resolveKindExpr(k syntax.TypeExpr) types.Type {
	return ch.typeResolver().resolveKindExpr(k)
}

func (ch *Checker) kindOfType(ty types.Type) types.Type {
	return ch.typeResolver().kindOfType(ty)
}

func (ch *Checker) hasDeterministicKind(ty types.Type) bool {
	return ch.typeResolver().hasDeterministicKind(ty)
}

func (ch *Checker) promotedFieldKind(fieldType types.Type) types.Type {
	return ch.typeResolver().promotedFieldKind(fieldType)
}
