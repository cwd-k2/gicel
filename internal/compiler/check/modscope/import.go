package modscope

import (
	"fmt"
	"maps"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ImportEnv provides the checker capabilities needed for module import processing.
type ImportEnv struct {
	// Registry writes.
	RegisterTypeKind     func(string, types.Type)
	RegisterAlias        func(string, *env.AliasInfo)
	RegisterClass        func(string, *env.ClassInfo)
	RegisterFamily       func(string, *env.TypeFamilyInfo) error
	RegisterDataType     func(string, *env.DataTypeInfo)
	RegisterPromotedKind func(string, types.Type)
	RegisterPromotedCon  func(string, types.Type)
	SetConBinding        func(string, types.Type, string)
	SetConInfo           func(string, *env.DataTypeInfo)
	ImportInstance       func(*env.InstanceInfo)

	// Error reporting.
	AddError func(diagnostic.Code, span.Span, string)

	// Context injection.
	PushVar func(name string, ty types.Type, module string)
}

// QualifiedScope holds a module's exports for qualified name resolution.
type QualifiedScope struct {
	ModuleName string             // canonical module name
	Exports    *env.ModuleExports // the module's full exports
}

// Importer processes import declarations and produces qualified scope mappings.
type Importer struct {
	env ImportEnv

	// Internal state (populated during import).
	qualifiedScopes     map[string]*QualifiedScope
	importedNames       map[string]string          // name → source module (value namespace)
	importedTypeNames   map[string]string          // name → source module (type namespace)
	ownedNamesCache     map[string]map[string]bool // module → owned names
	ownedTypeNamesCache map[string]map[string]bool // module → owned type names

	// Inputs.
	modules map[string]*env.ModuleExports
	deps    map[string][]string
}

// NewImporter creates an Importer with the given environment callbacks.
func NewImporter(e ImportEnv) *Importer {
	return &Importer{
		env:               e,
		qualifiedScopes:   make(map[string]*QualifiedScope),
		importedNames:     make(map[string]string),
		importedTypeNames: make(map[string]string),
	}
}

// Import processes import declarations, registering exported bindings through
// the ImportEnv callbacks (types, values, instances, etc.) and returning the
// qualified scope map for qualified name resolution.
func (imp *Importer) Import(
	imports []syntax.DeclImport,
	modules map[string]*env.ModuleExports,
	deps map[string][]string,
) map[string]*QualifiedScope {
	imp.modules = modules
	imp.deps = deps

	if modules == nil {
		if len(imports) > 0 {
			imp.env.AddError(diagnostic.ErrImport, imports[0].S, "unknown module: "+imports[0].ModuleName)
		}
		return imp.qualifiedScopes
	}

	seen := make(map[string]bool)      // module names already imported
	aliases := make(map[string]string) // alias → module name (for collision detection)

	for _, decl := range imports {
		// Core is implicit and user-invisible. Selective/qualified Core is an error.
		if decl.ModuleName == env.CoreModuleName && (decl.Alias != "" || decl.Names != nil) {
			imp.env.AddError(diagnostic.ErrImport, decl.S,
				"Core module cannot be selectively or qualifiedly imported")
			continue
		}

		// Duplicate import detection.
		if seen[decl.ModuleName] {
			imp.env.AddError(diagnostic.ErrImport, decl.S,
				"duplicate import: "+decl.ModuleName)
			continue
		}
		seen[decl.ModuleName] = true

		mod, ok := modules[decl.ModuleName]
		if !ok {
			imp.env.AddError(diagnostic.ErrImport, decl.S,
				"unknown module: "+decl.ModuleName)
			continue
		}

		switch {
		case decl.Alias != "":
			// Qualified import: import M as N
			if prev, exists := aliases[decl.Alias]; exists {
				imp.env.AddError(diagnostic.ErrImport, decl.S,
					"alias "+decl.Alias+" already used for module "+prev)
				continue
			}
			aliases[decl.Alias] = decl.ModuleName
			imp.qualifiedScopes[decl.Alias] = &QualifiedScope{
				ModuleName: decl.ModuleName,
				Exports:    mod,
			}
			// Instances and type families always imported (coherence
			// requirement). Type families must be visible so that
			// associated types (e.g. Elem) can reduce when the
			// instance is used through the qualified scope.
			imp.importInstances(mod)
			imp.importFamilies(mod, decl.ModuleName, decl.S)

		case decl.Names != nil:
			// Selective import: import M (x, T(..), C(A,B))
			imp.importSelective(mod, decl)

		default:
			// Open import: import M — merge all exports.
			imp.importOpen(mod, decl.ModuleName, decl.S)
		}
	}
	return imp.qualifiedScopes
}

// checkAmbiguousName reports an error if name is already imported from a different module.
func (imp *Importer) checkAmbiguousName(name, moduleName string, s span.Span) bool {
	return imp.checkAmbiguous(imp.importedNames, imp.moduleOwnsName, "name", name, moduleName, s)
}

// checkAmbiguousTypeName reports an error if a type-level name is already imported
// from a different module.
func (imp *Importer) checkAmbiguousTypeName(name, moduleName string, s span.Span) bool {
	return imp.checkAmbiguous(imp.importedTypeNames, imp.moduleOwnsTypeName, "type name", name, moduleName, s)
}

// checkAmbiguous is the shared implementation for value- and type-namespace
// ambiguity detection. It returns true if the name is ambiguous and should be skipped.
// Private names (_-prefixed) and compiler-generated names ($-containing) are exempt.
// Ambiguity is suppressed when both sides re-export from a shared dependency,
// or when one side re-exports from the other (dependency chain).
func (imp *Importer) checkAmbiguous(
	namespace map[string]string,
	ownerFn func(moduleName, name string) bool,
	errLabel, name, moduleName string,
	s span.Span,
) bool {
	if env.IsPrivateName(name) {
		return false
	}
	prev, exists := namespace[name]
	if !exists {
		namespace[name] = moduleName
		return false
	}
	if prev == moduleName {
		return false
	}
	prevOwns := ownerFn(prev, name)
	curOwns := ownerFn(moduleName, name)
	if !prevOwns && !curOwns {
		namespace[name] = moduleName
		return false
	}
	if prevOwns && !curOwns && imp.moduleDependsOn(moduleName, prev) {
		namespace[name] = moduleName
		return false
	}
	if curOwns && !prevOwns && imp.moduleDependsOn(prev, moduleName) {
		namespace[name] = moduleName
		return false
	}
	imp.env.AddError(diagnostic.ErrImport, s,
		fmt.Sprintf("ambiguous %s %q: imported from both %s and %s (use qualified import to disambiguate)",
			errLabel, name, prev, moduleName))
	return true
}

// moduleOwnsTypeName checks if a module defines a type-level name in its own source.
func (imp *Importer) moduleOwnsTypeName(moduleName, name string) bool {
	owned := imp.moduleOwnedTypeNames(moduleName)
	return owned[name]
}

// moduleDependsOn reports whether module a transitively depends on module b.
func (imp *Importer) moduleDependsOn(a, b string) bool {
	visited := make(map[string]bool)
	var walk func(string) bool
	walk = func(mod string) bool {
		if visited[mod] {
			return false
		}
		visited[mod] = true
		for _, dep := range imp.deps[mod] {
			if dep == b || walk(dep) {
				return true
			}
		}
		return false
	}
	return walk(a)
}

// moduleOwnedTypeNames returns the set of type-level names that a module defines
// itself (not re-exported from dependencies). Lazily computed and cached.
func (imp *Importer) moduleOwnedTypeNames(moduleName string) map[string]bool {
	if imp.ownedTypeNamesCache == nil {
		imp.ownedTypeNamesCache = make(map[string]map[string]bool)
	}
	if cached, ok := imp.ownedTypeNamesCache[moduleName]; ok {
		return cached
	}
	mod, ok := imp.modules[moduleName]
	if !ok {
		return nil
	}
	owned := make(map[string]bool)
	// Precomputed owned type names from module exports.
	maps.Copy(owned, mod.OwnedTypeNames)
	// Aliases, Classes, TypeFamilies not in any direct dependency.
	depTypes := make(map[string]bool)
	for _, dep := range imp.deps[moduleName] {
		if depMod, ok := imp.modules[dep]; ok {
			for name := range depMod.Aliases {
				depTypes[name] = true
			}
			for name := range depMod.Classes {
				depTypes[name] = true
			}
			for name := range depMod.TypeFamilies {
				depTypes[name] = true
			}
		}
	}
	for name := range mod.Aliases {
		if !depTypes[name] {
			owned[name] = true
		}
	}
	for name := range mod.Classes {
		if !depTypes[name] {
			owned[name] = true
		}
	}
	for name := range mod.TypeFamilies {
		if !depTypes[name] {
			owned[name] = true
		}
	}
	imp.ownedTypeNamesCache[moduleName] = owned
	return owned
}

// moduleOwnsName checks if a module defines name in its own source (not re-exported).
// A module "owns" a name if it appears in the module's OwnedNames (type + constructor
// names), or in its Values but NOT in any of its direct dependency's Values.
// Results are cached per module for amortized O(1) lookups.
func (imp *Importer) moduleOwnsName(moduleName, name string) bool {
	owned := imp.moduleOwnedNames(moduleName)
	return owned[name]
}

// moduleOwnedNames returns the set of names that a module defines itself
// (not re-exported from dependencies). Lazily computed and cached.
func (imp *Importer) moduleOwnedNames(moduleName string) map[string]bool {
	if imp.ownedNamesCache == nil {
		imp.ownedNamesCache = make(map[string]map[string]bool)
	}
	if cached, ok := imp.ownedNamesCache[moduleName]; ok {
		return cached
	}
	mod, ok := imp.modules[moduleName]
	if !ok {
		return nil
	}
	owned := make(map[string]bool)
	// Precomputed owned type names and constructor names from module exports.
	maps.Copy(owned, mod.OwnedNames)
	// Values not in any direct dependency.
	depValues := make(map[string]bool)
	for _, dep := range imp.deps[moduleName] {
		if depMod, ok := imp.modules[dep]; ok {
			for name := range depMod.Values {
				depValues[name] = true
			}
		}
	}
	for name := range mod.Values {
		if !depValues[name] {
			owned[name] = true
		}
	}
	imp.ownedNamesCache[moduleName] = owned
	return owned
}

// importOpen merges all exports from a module into the scope (open import).
func (imp *Importer) importOpen(mod *env.ModuleExports, moduleName string, s span.Span) {
	for name, kind := range mod.Types {
		if !imp.checkAmbiguousTypeName(name, moduleName, s) {
			imp.env.RegisterTypeKind(name, kind)
		}
	}
	for name, ty := range mod.ConTypes {
		imp.importConstructor(name, ty, moduleName, s)
	}
	for name, info := range mod.ConstructorInfo {
		imp.env.SetConInfo(name, info)
		imp.env.RegisterDataType(info.Name, info)
	}
	for name, alias := range mod.Aliases {
		if !imp.checkAmbiguousTypeName(name, moduleName, s) {
			imp.env.RegisterAlias(name, alias)
		}
	}
	// Collect method names from ambiguous classes so they can be excluded
	// from the Values loop below. Methods of an ambiguous class must not
	// be imported — they would be orphaned (usable as values without a
	// registered class).
	var ambiguousClassMethods map[string]bool
	for name, cls := range mod.Classes {
		if !imp.checkAmbiguousTypeName(name, moduleName, s) {
			imp.env.RegisterClass(name, cls)
		} else {
			if ambiguousClassMethods == nil {
				ambiguousClassMethods = make(map[string]bool)
			}
			for _, m := range cls.Methods {
				ambiguousClassMethods[m.Name] = true
			}
		}
	}
	imp.importInstances(mod)
	for name, ty := range mod.Values {
		if ambiguousClassMethods[name] {
			continue
		}
		imp.importValue(name, ty, moduleName, s)
	}
	for name, kind := range mod.PromotedKinds {
		if !imp.checkAmbiguousTypeName(name, moduleName, s) {
			imp.env.RegisterPromotedKind(name, kind)
		}
	}
	for name, kind := range mod.PromotedCons {
		if !imp.checkAmbiguousTypeName(name, moduleName, s) {
			imp.env.RegisterPromotedCon(name, kind)
		}
	}
	imp.importFamilies(mod, moduleName, s)
}

// importInstances imports all instances from a module (for coherence).
func (imp *Importer) importInstances(mod *env.ModuleExports) {
	for _, inst := range mod.Instances {
		imp.env.ImportInstance(inst)
	}
}

// importFamilies imports type families from a module. Required for coherence:
// associated type families (e.g. Elem) must be visible so that instance method
// return types can reduce, even when the module is imported qualified.
func (imp *Importer) importFamilies(mod *env.ModuleExports, moduleName string, s span.Span) {
	for name, fam := range mod.TypeFamilies {
		if !imp.checkAmbiguousTypeName(name, moduleName, s) {
			if err := imp.env.RegisterFamily(name, fam.Clone()); err != nil {
					imp.env.AddError(diagnostic.ErrImport, s, "conflicting type family equations for "+name+": "+err.Error())
				}
		}
	}
}

// importValue pushes a value binding into scope with ambiguity checking.
func (imp *Importer) importValue(name string, ty types.Type, moduleName string, s span.Span) {
	if !imp.checkAmbiguousName(name, moduleName, s) {
		imp.env.PushVar(name, ty, moduleName)
	}
}

// importConstructor pushes a data constructor into scope with ambiguity
// checking and constructor-module bookkeeping.
func (imp *Importer) importConstructor(conName string, ty types.Type, moduleName string, s span.Span) {
	if !imp.checkAmbiguousName(conName, moduleName, s) {
		imp.env.SetConBinding(conName, ty, moduleName)
		imp.env.PushVar(conName, ty, moduleName)
	}
}

// importSelective imports only the names specified in the import list.
func (imp *Importer) importSelective(mod *env.ModuleExports, decl syntax.DeclImport) {
	// Instances always imported regardless of selective list (coherence).
	imp.importInstances(mod)

	for _, in := range decl.Names {
		if in.Error {
			continue // parser already reported; skip to avoid cascading errors
		}
		name := in.Name
		found := false

		// Value binding (lowercase name or operator)
		if ty, ok := mod.Values[name]; ok {
			imp.importValue(name, ty, decl.ModuleName, decl.S)
			found = true
		}

		// Type constructor
		if kind, ok := mod.Types[name]; ok {
			if !imp.checkAmbiguousTypeName(name, decl.ModuleName, decl.S) {
				imp.env.RegisterTypeKind(name, kind)
			}
			found = true

			// Import constructors if HasSub
			if in.HasSub {
				imp.importTypeSubs(mod, name, in, decl.ModuleName, decl.S)
			}
		}

		// Type alias
		if alias, ok := mod.Aliases[name]; ok {
			if !imp.checkAmbiguousTypeName(name, decl.ModuleName, decl.S) {
				imp.env.RegisterAlias(name, alias)
			}
			found = true
		}

		// Class
		if cls, ok := mod.Classes[name]; ok {
			ambiguous := imp.checkAmbiguousTypeName(name, decl.ModuleName, decl.S)
			if !ambiguous {
				imp.env.RegisterClass(name, cls)
			}
			found = true

			// Import class methods only if the class itself is not ambiguous.
			// An ambiguous class is not registered, so its methods must not
			// be imported — they would be orphaned values without a class.
			if !ambiguous {
				if in.HasSub {
					imp.importClassSubs(mod, cls, in, decl.ModuleName, decl.S)
				} else {
					// Bare class name: import all methods
					for _, m := range cls.Methods {
						if ty, ok := mod.Values[m.Name]; ok {
							imp.importValue(m.Name, ty, decl.ModuleName, decl.S)
						}
					}
				}
			}
		}

		// Type family
		if fam, ok := mod.TypeFamilies[name]; ok {
			if !imp.checkAmbiguousTypeName(name, decl.ModuleName, decl.S) {
				if err := imp.env.RegisterFamily(name, fam.Clone()); err != nil {
					imp.env.AddError(diagnostic.ErrImport, decl.S, "conflicting type family equations for "+name+": "+err.Error())
				}
			}
			found = true
		}

		// Promoted kinds
		if kind, ok := mod.PromotedKinds[name]; ok {
			if !imp.checkAmbiguousTypeName(name, decl.ModuleName, decl.S) {
				imp.env.RegisterPromotedKind(name, kind)
			}
			found = true
		}
		if kind, ok := mod.PromotedCons[name]; ok {
			if !imp.checkAmbiguousTypeName(name, decl.ModuleName, decl.S) {
				imp.env.RegisterPromotedCon(name, kind)
			}
			found = true
		}

		if !found {
			imp.env.AddError(diagnostic.ErrImport, decl.S,
				"module "+decl.ModuleName+" does not export: "+name)
		}
	}
}

// importTypeSubs imports constructors for a type based on the import name spec.
func (imp *Importer) importTypeSubs(mod *env.ModuleExports, typeName string, in syntax.ImportName, moduleName string, s span.Span) {
	cons := mod.ConstructorsByType[typeName]
	if len(cons) == 0 {
		return
	}
	// For selective sublists, build a set for O(1) lookup.
	var subSet map[string]bool
	if !in.AllSubs && len(in.SubList) > 0 {
		subSet = make(map[string]bool, len(in.SubList))
		for _, name := range in.SubList {
			subSet[name] = true
		}
	}
	for _, conName := range cons {
		if in.AllSubs || subSet[conName] {
			if info, ok := mod.ConstructorInfo[conName]; ok {
				imp.env.SetConInfo(conName, info)
			}
			if ty, ok := mod.ConTypes[conName]; ok {
				imp.importConstructor(conName, ty, moduleName, s)
			}
		}
	}
}

// importClassSubs imports class methods based on the import name spec.
func (imp *Importer) importClassSubs(mod *env.ModuleExports, cls *env.ClassInfo, in syntax.ImportName, moduleName string, s span.Span) {
	// For selective sublists, build a set for O(1) lookup.
	var subSet map[string]bool
	if !in.AllSubs && len(in.SubList) > 0 {
		subSet = make(map[string]bool, len(in.SubList))
		for _, name := range in.SubList {
			subSet[name] = true
		}
	}
	for _, m := range cls.Methods {
		if in.AllSubs || subSet[m.Name] {
			if ty, ok := mod.Values[m.Name]; ok {
				imp.importValue(m.Name, ty, moduleName, s)
			}
		}
	}
}
