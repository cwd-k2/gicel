package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// importModules injects exported declarations from imported modules into the checker state.
func (ch *Checker) importModules(imports []syntax.DeclImport) {
	if ch.config.ImportedModules == nil {
		if len(imports) > 0 {
			ch.addCodedError(errs.ErrImport, imports[0].S, fmt.Sprintf("unknown module: %s", imports[0].ModuleName))
		}
		return
	}

	seen := make(map[string]bool)      // module names already imported
	aliases := make(map[string]string) // alias → module name (for collision detection)

	for _, imp := range imports {
		// Core is implicit and user-invisible. Selective/qualified Core is an error.
		if imp.ModuleName == "Core" && (imp.Alias != "" || imp.Names != nil) {
			ch.addCodedError(errs.ErrImport, imp.S,
				"Core module cannot be selectively or qualifiedly imported")
			continue
		}

		// Duplicate import detection.
		if seen[imp.ModuleName] {
			ch.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("duplicate import: %s", imp.ModuleName))
			continue
		}
		seen[imp.ModuleName] = true

		mod, ok := ch.config.ImportedModules[imp.ModuleName]
		if !ok {
			ch.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("unknown module: %s", imp.ModuleName))
			continue
		}

		switch {
		case imp.Alias != "":
			// Qualified import: import M as N
			if prev, exists := aliases[imp.Alias]; exists {
				ch.addCodedError(errs.ErrImport, imp.S,
					fmt.Sprintf("alias %s already used for module %s", imp.Alias, prev))
				continue
			}
			aliases[imp.Alias] = imp.ModuleName
			ch.scope.qualifiedScopes[imp.Alias] = &qualifiedScope{
				moduleName: imp.ModuleName,
				exports:    mod,
			}
			// Instances always imported (coherence requirement).
			ch.importInstances(mod)

		case imp.Names != nil:
			// Selective import: import M (x, T(..), C(A,B))
			ch.importSelective(mod, imp)

		default:
			// Open import: import M — merge all exports.
			ch.importOpen(mod, imp.ModuleName, imp.S)
		}
	}
}

// checkAmbiguousName reports an error if name is already imported from a different module.
// Returns true if the name is ambiguous and should be skipped.
// Names from Core are exempt (Core is implicit and its names are re-exported by dependent modules).
// Compiler-generated names ($-prefixed) are also exempt (internal to elaboration).
func (ch *Checker) checkAmbiguousName(name, moduleName string, s span.Span) bool {
	if isPrivateName(name) {
		return false // compiler-generated or private, skip
	}
	prev, exists := ch.scope.importedNames[name]
	if !exists {
		ch.scope.importedNames[name] = moduleName
		return false
	}
	if prev == moduleName {
		return false // same module, no conflict
	}
	// Both modules provide this name: check ownership and provenance.
	// Ambiguity is suppressed when both sides re-export from a shared
	// dependency, or when one side re-exports from the other (dependency chain).
	prevOwns := ch.moduleOwnsName(prev, name)
	curOwns := ch.moduleOwnsName(moduleName, name)
	if !prevOwns && !curOwns {
		// Both are re-exports — assumed from a shared dependency.
		ch.scope.importedNames[name] = moduleName
		return false
	}
	if prevOwns && !curOwns && ch.moduleDependsOn(moduleName, prev) {
		// cur re-exports from prev via dependency chain.
		ch.scope.importedNames[name] = moduleName
		return false
	}
	if curOwns && !prevOwns && ch.moduleDependsOn(prev, moduleName) {
		// prev re-exports from cur via dependency chain.
		ch.scope.importedNames[name] = moduleName
		return false
	}
	ch.addCodedError(errs.ErrImport, s,
		fmt.Sprintf("ambiguous name %q: imported from both %s and %s (use qualified import to disambiguate)", name, prev, moduleName))
	return true
}

// checkAmbiguousTypeName reports an error if a type-level name is already imported
// from a different module. Returns true if the name is ambiguous and should be skipped.
// Logic mirrors checkAmbiguousName but operates on the type namespace.
func (ch *Checker) checkAmbiguousTypeName(name, moduleName string, s span.Span) bool {
	if isPrivateName(name) {
		return false
	}
	prev, exists := ch.scope.importedTypeNames[name]
	if !exists {
		ch.scope.importedTypeNames[name] = moduleName
		return false
	}
	if prev == moduleName {
		return false
	}
	prevOwns := ch.moduleOwnsTypeName(prev, name)
	curOwns := ch.moduleOwnsTypeName(moduleName, name)
	if !prevOwns && !curOwns {
		ch.scope.importedTypeNames[name] = moduleName
		return false
	}
	if prevOwns && !curOwns && ch.moduleDependsOn(moduleName, prev) {
		ch.scope.importedTypeNames[name] = moduleName
		return false
	}
	if curOwns && !prevOwns && ch.moduleDependsOn(prev, moduleName) {
		ch.scope.importedTypeNames[name] = moduleName
		return false
	}
	ch.addCodedError(errs.ErrImport, s,
		fmt.Sprintf("ambiguous type name %q: imported from both %s and %s (use qualified import to disambiguate)", name, prev, moduleName))
	return true
}

// moduleOwnsTypeName checks if a module defines a type-level name in its own source.
func (ch *Checker) moduleOwnsTypeName(moduleName, name string) bool {
	owned := ch.moduleOwnedTypeNames(moduleName)
	return owned[name]
}

// moduleDependsOn reports whether module a transitively depends on module b.
func (ch *Checker) moduleDependsOn(a, b string) bool {
	visited := make(map[string]bool)
	var walk func(string) bool
	walk = func(mod string) bool {
		if visited[mod] {
			return false
		}
		visited[mod] = true
		for _, dep := range ch.config.ModuleDeps[mod] {
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
func (ch *Checker) moduleOwnedTypeNames(moduleName string) map[string]bool {
	if ch.scope.ownedTypeNamesCache == nil {
		ch.scope.ownedTypeNamesCache = make(map[string]map[string]bool)
	}
	if cached, ok := ch.scope.ownedTypeNamesCache[moduleName]; ok {
		return cached
	}
	mod, ok := ch.config.ImportedModules[moduleName]
	if !ok {
		return nil
	}
	owned := make(map[string]bool)
	// DataDecl type names are always module-owned.
	for _, dd := range mod.DataDecls {
		owned[dd.Name] = true
	}
	// Aliases, Classes, TypeFamilies not in any direct dependency.
	depTypes := make(map[string]bool)
	for _, dep := range ch.config.ModuleDeps[moduleName] {
		if depMod, ok := ch.config.ImportedModules[dep]; ok {
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
	ch.scope.ownedTypeNamesCache[moduleName] = owned
	return owned
}

// moduleOwnsName checks if a module defines name in its own source (not re-exported).
// A module "owns" a name if it appears in the module's DataDecls (constructors),
// or in its Values but NOT in any of its direct dependency's Values.
// Results are cached per module for amortized O(1) lookups.
func (ch *Checker) moduleOwnsName(moduleName, name string) bool {
	owned := ch.moduleOwnedNames(moduleName)
	return owned[name]
}

// moduleOwnedNames returns the set of names that a module defines itself
// (not re-exported from dependencies). Lazily computed and cached.
func (ch *Checker) moduleOwnedNames(moduleName string) map[string]bool {
	if ch.scope.ownedNamesCache == nil {
		ch.scope.ownedNamesCache = make(map[string]map[string]bool)
	}
	if cached, ok := ch.scope.ownedNamesCache[moduleName]; ok {
		return cached
	}
	mod, ok := ch.config.ImportedModules[moduleName]
	if !ok {
		return nil
	}
	owned := make(map[string]bool)
	// DataDecl type names and constructor names.
	for _, dd := range mod.DataDecls {
		owned[dd.Name] = true
		for _, con := range dd.Cons {
			owned[con.Name] = true
		}
	}
	// Values not in any direct dependency.
	depValues := make(map[string]bool)
	for _, dep := range ch.config.ModuleDeps[moduleName] {
		if depMod, ok := ch.config.ImportedModules[dep]; ok {
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
	ch.scope.ownedNamesCache[moduleName] = owned
	return owned
}

// importOpen merges all exports from a module into the checker state (open import).
func (ch *Checker) importOpen(mod *ModuleExports, moduleName string, s span.Span) {
	for name, kind := range mod.Types {
		if !ch.checkAmbiguousTypeName(name, moduleName, s) {
			ch.reg.typeKinds[name] = kind
		}
	}
	for name, ty := range mod.ConTypes {
		ch.importConstructor(name, ty, moduleName, s)
	}
	for name, info := range mod.ConstructorInfo {
		ch.reg.conInfo[name] = info
		ch.reg.dataTypeByName[info.Name] = info
	}
	for name, alias := range mod.Aliases {
		if !ch.checkAmbiguousTypeName(name, moduleName, s) {
			ch.reg.aliases[name] = alias
		}
	}
	for name, cls := range mod.Classes {
		if !ch.checkAmbiguousTypeName(name, moduleName, s) {
			ch.reg.classes[name] = cls
		}
	}
	ch.importInstances(mod)
	for name, ty := range mod.Values {
		ch.importValue(name, ty, moduleName, s)
	}
	for name, kind := range mod.PromotedKinds {
		if !ch.checkAmbiguousTypeName(name, moduleName, s) {
			ch.reg.promotedKinds[name] = kind
		}
	}
	for name, kind := range mod.PromotedCons {
		if !ch.checkAmbiguousTypeName(name, moduleName, s) {
			ch.reg.promotedCons[name] = kind
		}
	}
	for name, fam := range mod.TypeFamilies {
		if !ch.checkAmbiguousTypeName(name, moduleName, s) {
			ch.reg.families[name] = fam.Clone()
		}
	}
}

// importInstances imports all instances from a module (for coherence).
func (ch *Checker) importInstances(mod *ModuleExports) {
	for _, inst := range mod.Instances {
		if ch.reg.importedInstances[inst] {
			continue
		}
		ch.reg.instances = append(ch.reg.instances, inst)
		ch.reg.instancesByClass[inst.ClassName] = append(ch.reg.instancesByClass[inst.ClassName], inst)
		ch.reg.importedInstances[inst] = true
	}
}

// importValue pushes a value binding into scope with ambiguity checking.
func (ch *Checker) importValue(name string, ty types.Type, moduleName string, s span.Span) {
	if !ch.checkAmbiguousName(name, moduleName, s) {
		ch.ctx.Push(&CtxVar{Name: name, Type: ty, Module: moduleName})
	}
}

// importConstructor pushes a data constructor into scope with ambiguity
// checking and constructor-module bookkeeping.
func (ch *Checker) importConstructor(conName string, ty types.Type, moduleName string, s span.Span) {
	if !ch.checkAmbiguousName(conName, moduleName, s) {
		ch.reg.conTypes[conName] = ty
		ch.ctx.Push(&CtxVar{Name: conName, Type: ty, Module: moduleName})
		ch.reg.conModules[conName] = moduleName
	}
}

// importSelective imports only the names specified in the import list.
func (ch *Checker) importSelective(mod *ModuleExports, imp syntax.DeclImport) {
	// Instances always imported regardless of selective list (coherence).
	ch.importInstances(mod)

	for _, in := range imp.Names {
		name := in.Name
		found := false

		// Value binding (lowercase name or operator)
		if ty, ok := mod.Values[name]; ok {
			ch.importValue(name, ty, imp.ModuleName, imp.S)
			found = true
		}

		// Type constructor
		if kind, ok := mod.Types[name]; ok {
			if !ch.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				ch.reg.typeKinds[name] = kind
			}
			found = true

			// Import constructors if HasSub
			if in.HasSub {
				ch.importTypeSubs(mod, name, in, imp.ModuleName, imp.S)
			}
		}

		// Type alias
		if alias, ok := mod.Aliases[name]; ok {
			if !ch.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				ch.reg.aliases[name] = alias
			}
			found = true
		}

		// Class
		if cls, ok := mod.Classes[name]; ok {
			if !ch.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				ch.reg.classes[name] = cls
			}
			found = true

			// Import class methods
			if in.HasSub {
				ch.importClassSubs(mod, cls, in, imp.ModuleName, imp.S)
			} else {
				// Bare class name: import all methods
				for _, m := range cls.Methods {
					if ty, ok := mod.Values[m.Name]; ok {
						ch.importValue(m.Name, ty, imp.ModuleName, imp.S)
					}
				}
			}
		}

		// Type family
		if fam, ok := mod.TypeFamilies[name]; ok {
			if !ch.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				ch.reg.families[name] = fam.Clone()
			}
			found = true
		}

		// Promoted kinds
		if kind, ok := mod.PromotedKinds[name]; ok {
			if !ch.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				ch.reg.promotedKinds[name] = kind
			}
			found = true
		}
		if kind, ok := mod.PromotedCons[name]; ok {
			if !ch.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				ch.reg.promotedCons[name] = kind
			}
			found = true
		}

		if !found {
			ch.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("module %s does not export: %s", imp.ModuleName, name))
		}
	}
}

// importTypeSubs imports constructors for a type based on the import name spec.
func (ch *Checker) importTypeSubs(mod *ModuleExports, typeName string, in syntax.ImportName, moduleName string, s span.Span) {
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
				ch.reg.conInfo[conName] = info
			}
			if ty, ok := mod.ConTypes[conName]; ok {
				ch.importConstructor(conName, ty, moduleName, s)
			}
		}
	}
}

// importClassSubs imports class methods based on the import name spec.
func (ch *Checker) importClassSubs(mod *ModuleExports, cls *ClassInfo, in syntax.ImportName, moduleName string, s span.Span) {
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
				ch.importValue(m.Name, ty, moduleName, s)
			}
		}
	}
}
