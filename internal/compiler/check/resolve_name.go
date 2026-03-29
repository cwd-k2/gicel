// Name resolution for type expressions — unqualified and qualified type names.
// Does NOT cover: variable lookup (bidir_lookup.go), raw alias/family
// primitives (checker_lookup.go).
package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// resolveUnqualifiedTypeCon resolves an unqualified type constructor name.
// Handles zero-arity alias expansion, zero-arity type family expansion,
// and strict mode validation.
func (ch *Checker) resolveUnqualifiedTypeCon(name string, s span.Span) types.Type {
	// Zero-arity alias: expand inline.
	if info, ok := ch.lookupAlias(name); ok && len(info.Params) == 0 {
		return info.Body
	}
	// Zero-arity type family: immediate TyFamilyApp.
	if fam, ok := ch.lookupFamily(name); ok && len(fam.Params) == 0 {
		return &types.TyFamilyApp{Name: name, Args: nil, Kind: fam.ResultKind, S: s}
	}
	// Strict mode: validate that the type constructor is known.
	if ch.strictTypeNames && !ch.isKnownTypeName(name) {
		ch.addCodedError(diagnostic.ErrUnboundCon, s, "unknown type: "+name)
		return &types.TyError{S: s}
	}
	return types.ConAt(name, s)
}

// resolveQualifiedTypeCon resolves a qualified type constructor name (Mod.Name).
// Looks up the qualifier in scope, then checks aliases, families, types, and
// promoted constructors in the module's exports.
func (ch *Checker) resolveQualifiedTypeCon(qualifier, name string, s span.Span) types.Type {
	qs, ok := ch.scope.LookupQualified(qualifier)
	if !ok {
		ch.addCodedError(diagnostic.ErrImport, s, "unknown qualifier: "+qualifier)
		return &types.TyError{S: s}
	}
	// Aliases: zero-arity → expand inline; parameterized → inject into scope.
	if info, ok := qs.Exports.Aliases[name]; ok {
		if len(info.Params) == 0 {
			return info.Body
		}
		ch.scope.InjectAlias(name, info)
		return types.ConAt(name, s)
	}
	// Type families: zero-arity → immediate; parameterized → inject into scope.
	if fam, ok := qs.Exports.TypeFamilies[name]; ok {
		if len(fam.Params) == 0 {
			return &types.TyFamilyApp{Name: name, Args: nil, Kind: fam.ResultKind, S: s}
		}
		ch.scope.InjectFamily(name, fam.Clone())
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
	ch.addCodedError(diagnostic.ErrImport, s,
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
