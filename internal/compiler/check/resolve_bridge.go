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
	addCodedError func(diagnostic.Code, span.Span, string)

	// Strict type name validation (enabled after declaration processing).
	strictTypeNames *bool
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
			lookupAlias:     ch.scopedLookupAlias,
			lookupFamily:    ch.scopedLookupFamily,
			lookupTyVar:     ch.ctx.LookupTyVar,
			emitEq:          ch.emitEq,
			addCodedError:   ch.addCodedError,
			strictTypeNames: &ch.CheckState.strictTypeNames,
		}
	}
	return ch.cachedTypeResolver
}

// --- Scope-aware lookups (injected into typeResolver) ---

// scopedLookupAlias searches for a type alias by name: first in the Registry
// (populated during declaration phases), then in Scope's injected aliases
// (populated from qualified references).
func (ch *Checker) scopedLookupAlias(name string) (*AliasInfo, bool) {
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

// scopedLookupFamily searches for a type family by name: first in the Registry,
// then in Scope's injected families.
func (ch *Checker) scopedLookupFamily(name string) (*TypeFamilyInfo, bool) {
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

// --- Scope-aware lookup wrappers (used by alias.go, type_family.go, tests) ---

func (ch *Checker) lookupAlias(name string) (*AliasInfo, bool) {
	return ch.scopedLookupAlias(name)
}

func (ch *Checker) lookupFamily(name string) (*TypeFamilyInfo, bool) {
	return ch.scopedLookupFamily(name)
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

func (ch *Checker) checkTypeAppKind(fun, arg types.Type, s span.Span) {
	ch.typeResolver().checkTypeAppKind(fun, arg, s)
}

func (ch *Checker) aliasParamKind(aliasName string, i int) types.Type {
	return ch.typeResolver().aliasParamKind(aliasName, i)
}

func (ch *Checker) promotedFieldKind(fieldType types.Type) types.Type {
	return ch.typeResolver().promotedFieldKind(fieldType)
}

func (ch *Checker) resolveUnqualifiedTypeCon(name string, s span.Span) types.Type {
	return ch.typeResolver().resolveUnqualifiedTypeCon(name, s)
}

func (ch *Checker) resolveQualifiedTypeCon(qualifier, name string, s span.Span) types.Type {
	return ch.typeResolver().resolveQualifiedTypeCon(qualifier, name, s)
}

func (ch *Checker) isKnownTypeName(name string) bool {
	return ch.typeResolver().isKnownTypeName(name)
}

func (ch *Checker) tryExpandApp(fun, arg types.Type, s span.Span) types.Type {
	return ch.typeResolver().tryExpandApp(fun, arg, s)
}

func (ch *Checker) decomposeQuantifiedConstraint(ty types.Type) *types.QuantifiedConstraint {
	return ch.typeResolver().decomposeQuantifiedConstraint(ty)
}
