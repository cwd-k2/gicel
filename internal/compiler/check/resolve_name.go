// Name resolution for type expressions — unqualified and qualified type names.
// Methods on *typeResolver; wired via resolve_bridge.go.
// Does NOT cover: variable lookup (bidir_lookup.go).
package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// resolveUnqualifiedTypeCon resolves an unqualified type constructor name.
// Handles zero-arity alias expansion, zero-arity type family expansion,
// and strict mode validation.
func (r *typeResolver) resolveUnqualifiedTypeCon(name string, s span.Span) types.Type {
	// Zero-arity alias: expand inline.
	if info, ok := r.lookupAlias(name); ok && len(info.Params) == 0 {
		return info.Body
	}
	// Zero-arity type family: immediate TyFamilyApp.
	if fam, ok := r.lookupFamily(name); ok && len(fam.Params) == 0 {
		return &types.TyFamilyApp{Name: name, Args: nil, Kind: fam.ResultKind, S: s}
	}
	// Strict mode: validate that the type constructor is known.
	if *r.strictTypeNames && !r.isKnownTypeName(name) {
		r.addCodedError(diagnostic.ErrUnboundCon, s, "unknown type: "+name)
		return &types.TyError{S: s}
	}
	return types.ConAt(name, s)
}

// resolveQualifiedTypeCon resolves a qualified type constructor name (Mod.Name).
// Looks up the qualifier in scope, then checks aliases, families, types, and
// promoted constructors in the module's exports.
func (r *typeResolver) resolveQualifiedTypeCon(qualifier, name string, s span.Span) types.Type {
	qs, ok := r.scope.LookupQualified(qualifier)
	if !ok {
		r.addCodedError(diagnostic.ErrImport, s, "unknown qualifier: "+qualifier)
		return &types.TyError{S: s}
	}
	// Aliases: zero-arity → expand inline; parameterized → inject into scope.
	if info, ok := qs.Exports.Aliases[name]; ok {
		if len(info.Params) == 0 {
			return info.Body
		}
		r.scope.InjectAlias(name, info)
		return types.ConAt(name, s)
	}
	// Type families: zero-arity → immediate; parameterized → inject into scope.
	if fam, ok := qs.Exports.TypeFamilies[name]; ok {
		if len(fam.Params) == 0 {
			return &types.TyFamilyApp{Name: name, Args: nil, Kind: fam.ResultKind, S: s}
		}
		r.scope.InjectFamily(name, fam.Clone())
		return types.ConAt(name, s)
	}
	// Module-defined types (data declarations, class types).
	if isModuleDefinedType(qs.Exports, name) {
		return types.ConAt(name, s)
	}
	// Promoted kinds and constructors.
	if _, ok := qs.Exports.PromotedKinds[name]; ok {
		return types.ConAt(name, s)
	}
	if _, ok := qs.Exports.PromotedCons[name]; ok {
		return types.ConAt(name, s)
	}
	r.addCodedError(diagnostic.ErrImport, s,
		"module "+qs.ModuleName+" does not export type: "+name)
	return &types.TyError{S: s}
}

// isModuleDefinedType checks if a type name was defined by the module itself
// (via data declarations or class declarations), as opposed to being inherited
// from built-in types or open imports.
func isModuleDefinedType(exports *ModuleExports, name string) bool {
	if exports.OwnedTypeNames[name] {
		return true
	}
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
func (r *typeResolver) isKnownTypeName(name string) bool {
	if builtinTypeNames[name] {
		return true
	}
	if _, ok := r.reg.LookupTypeKind(name); ok {
		return true
	}
	if _, ok := r.lookupAlias(name); ok {
		return true
	}
	if _, ok := r.lookupFamily(name); ok {
		return true
	}
	if _, ok := r.reg.LookupClass(name); ok {
		return true
	}
	if r.reg.HasPromotedKind(name) {
		return true
	}
	if r.reg.HasPromotedCon(name) {
		return true
	}
	return false
}
