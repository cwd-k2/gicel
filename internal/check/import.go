package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// ImportModules injects exported declarations from imported modules into the scope.
func (sc *Scope) ImportModules(imports []syntax.DeclImport) {
	if sc.config.ImportedModules == nil {
		if len(imports) > 0 {
			sc.session.addCodedError(errs.ErrImport, imports[0].S, fmt.Sprintf("unknown module: %s", imports[0].ModuleName))
		}
		return
	}

	seen := make(map[string]bool)      // module names already imported
	aliases := make(map[string]string) // alias → module name (for collision detection)

	for _, imp := range imports {
		// Core is implicit and user-invisible. Selective/qualified Core is an error.
		if imp.ModuleName == "Core" && (imp.Alias != "" || imp.Names != nil) {
			sc.session.addCodedError(errs.ErrImport, imp.S,
				"Core module cannot be selectively or qualifiedly imported")
			continue
		}

		// Duplicate import detection.
		if seen[imp.ModuleName] {
			sc.session.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("duplicate import: %s", imp.ModuleName))
			continue
		}
		seen[imp.ModuleName] = true

		mod, ok := sc.config.ImportedModules[imp.ModuleName]
		if !ok {
			sc.session.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("unknown module: %s", imp.ModuleName))
			continue
		}

		switch {
		case imp.Alias != "":
			// Qualified import: import M as N
			if prev, exists := aliases[imp.Alias]; exists {
				sc.session.addCodedError(errs.ErrImport, imp.S,
					fmt.Sprintf("alias %s already used for module %s", imp.Alias, prev))
				continue
			}
			aliases[imp.Alias] = imp.ModuleName
			sc.qualifiedScopes[imp.Alias] = &qualifiedScope{
				moduleName: imp.ModuleName,
				exports:    mod,
			}
			// Instances always imported (coherence requirement).
			sc.importInstances(mod)

		case imp.Names != nil:
			// Selective import: import M (x, T(..), C(A,B))
			sc.importSelective(mod, imp)

		default:
			// Open import: import M — merge all exports.
			sc.importOpen(mod, imp.ModuleName, imp.S)
		}
	}
}

// checkAmbiguousName reports an error if name is already imported from a different module.
// Returns true if the name is ambiguous and should be skipped.
// Names from Core are exempt (Core is implicit and its names are re-exported by dependent modules).
// Compiler-generated names ($-prefixed) are also exempt (internal to elaboration).
func (sc *Scope) checkAmbiguousName(name, moduleName string, s span.Span) bool {
	if isPrivateName(name) {
		return false // compiler-generated or private, skip
	}
	prev, exists := sc.importedNames[name]
	if !exists {
		sc.importedNames[name] = moduleName
		return false
	}
	if prev == moduleName {
		return false // same module, no conflict
	}
	// Both modules provide this name: check ownership and provenance.
	// Ambiguity is suppressed when both sides re-export from a shared
	// dependency, or when one side re-exports from the other (dependency chain).
	prevOwns := sc.moduleOwnsName(prev, name)
	curOwns := sc.moduleOwnsName(moduleName, name)
	if !prevOwns && !curOwns {
		// Both are re-exports — assumed from a shared dependency.
		sc.importedNames[name] = moduleName
		return false
	}
	if prevOwns && !curOwns && sc.moduleDependsOn(moduleName, prev) {
		// cur re-exports from prev via dependency chain.
		sc.importedNames[name] = moduleName
		return false
	}
	if curOwns && !prevOwns && sc.moduleDependsOn(prev, moduleName) {
		// prev re-exports from cur via dependency chain.
		sc.importedNames[name] = moduleName
		return false
	}
	sc.session.addCodedError(errs.ErrImport, s,
		fmt.Sprintf("ambiguous name %q: imported from both %s and %s (use qualified import to disambiguate)", name, prev, moduleName))
	return true
}

// checkAmbiguousTypeName reports an error if a type-level name is already imported
// from a different module. Returns true if the name is ambiguous and should be skipped.
// Logic mirrors checkAmbiguousName but operates on the type namespace.
func (sc *Scope) checkAmbiguousTypeName(name, moduleName string, s span.Span) bool {
	if isPrivateName(name) {
		return false
	}
	prev, exists := sc.importedTypeNames[name]
	if !exists {
		sc.importedTypeNames[name] = moduleName
		return false
	}
	if prev == moduleName {
		return false
	}
	prevOwns := sc.moduleOwnsTypeName(prev, name)
	curOwns := sc.moduleOwnsTypeName(moduleName, name)
	if !prevOwns && !curOwns {
		sc.importedTypeNames[name] = moduleName
		return false
	}
	if prevOwns && !curOwns && sc.moduleDependsOn(moduleName, prev) {
		sc.importedTypeNames[name] = moduleName
		return false
	}
	if curOwns && !prevOwns && sc.moduleDependsOn(prev, moduleName) {
		sc.importedTypeNames[name] = moduleName
		return false
	}
	sc.session.addCodedError(errs.ErrImport, s,
		fmt.Sprintf("ambiguous type name %q: imported from both %s and %s (use qualified import to disambiguate)", name, prev, moduleName))
	return true
}

// moduleOwnsTypeName checks if a module defines a type-level name in its own source.
func (sc *Scope) moduleOwnsTypeName(moduleName, name string) bool {
	owned := sc.moduleOwnedTypeNames(moduleName)
	return owned[name]
}

// moduleDependsOn reports whether module a transitively depends on module b.
func (sc *Scope) moduleDependsOn(a, b string) bool {
	visited := make(map[string]bool)
	var walk func(string) bool
	walk = func(mod string) bool {
		if visited[mod] {
			return false
		}
		visited[mod] = true
		for _, dep := range sc.config.ModuleDeps[mod] {
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
func (sc *Scope) moduleOwnedTypeNames(moduleName string) map[string]bool {
	if sc.ownedTypeNamesCache == nil {
		sc.ownedTypeNamesCache = make(map[string]map[string]bool)
	}
	if cached, ok := sc.ownedTypeNamesCache[moduleName]; ok {
		return cached
	}
	mod, ok := sc.config.ImportedModules[moduleName]
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
	for _, dep := range sc.config.ModuleDeps[moduleName] {
		if depMod, ok := sc.config.ImportedModules[dep]; ok {
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
	sc.ownedTypeNamesCache[moduleName] = owned
	return owned
}

// moduleOwnsName checks if a module defines name in its own source (not re-exported).
// A module "owns" a name if it appears in the module's DataDecls (constructors),
// or in its Values but NOT in any of its direct dependency's Values.
// Results are cached per module for amortized O(1) lookups.
func (sc *Scope) moduleOwnsName(moduleName, name string) bool {
	owned := sc.moduleOwnedNames(moduleName)
	return owned[name]
}

// moduleOwnedNames returns the set of names that a module defines itself
// (not re-exported from dependencies). Lazily computed and cached.
func (sc *Scope) moduleOwnedNames(moduleName string) map[string]bool {
	if sc.ownedNamesCache == nil {
		sc.ownedNamesCache = make(map[string]map[string]bool)
	}
	if cached, ok := sc.ownedNamesCache[moduleName]; ok {
		return cached
	}
	mod, ok := sc.config.ImportedModules[moduleName]
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
	for _, dep := range sc.config.ModuleDeps[moduleName] {
		if depMod, ok := sc.config.ImportedModules[dep]; ok {
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
	sc.ownedNamesCache[moduleName] = owned
	return owned
}

// importOpen merges all exports from a module into the scope (open import).
func (sc *Scope) importOpen(mod *ModuleExports, moduleName string, s span.Span) {
	for name, kind := range mod.Types {
		if !sc.checkAmbiguousTypeName(name, moduleName, s) {
			sc.reg.typeKinds[name] = kind
		}
	}
	for name, ty := range mod.ConTypes {
		sc.importConstructor(name, ty, moduleName, s)
	}
	for name, info := range mod.ConstructorInfo {
		sc.reg.conInfo[name] = info
		sc.reg.dataTypeByName[info.Name] = info
	}
	for name, alias := range mod.Aliases {
		if !sc.checkAmbiguousTypeName(name, moduleName, s) {
			sc.reg.aliases[name] = alias
		}
	}
	for name, cls := range mod.Classes {
		if !sc.checkAmbiguousTypeName(name, moduleName, s) {
			sc.reg.classes[name] = cls
		}
	}
	sc.importInstances(mod)
	for name, ty := range mod.Values {
		sc.importValue(name, ty, moduleName, s)
	}
	for name, kind := range mod.PromotedKinds {
		if !sc.checkAmbiguousTypeName(name, moduleName, s) {
			sc.reg.promotedKinds[name] = kind
		}
	}
	for name, kind := range mod.PromotedCons {
		if !sc.checkAmbiguousTypeName(name, moduleName, s) {
			sc.reg.promotedCons[name] = kind
		}
	}
	for name, fam := range mod.TypeFamilies {
		if !sc.checkAmbiguousTypeName(name, moduleName, s) {
			sc.reg.families[name] = fam.Clone()
		}
	}
}

// importInstances imports all instances from a module (for coherence).
func (sc *Scope) importInstances(mod *ModuleExports) {
	for _, inst := range mod.Instances {
		if sc.reg.importedInstances[inst] {
			continue
		}
		sc.reg.instances = append(sc.reg.instances, inst)
		sc.reg.instancesByClass[inst.ClassName] = append(sc.reg.instancesByClass[inst.ClassName], inst)
		sc.reg.importedInstances[inst] = true
	}
}

// importValue pushes a value binding into scope with ambiguity checking.
func (sc *Scope) importValue(name string, ty types.Type, moduleName string, s span.Span) {
	if !sc.checkAmbiguousName(name, moduleName, s) {
		sc.session.ctx.Push(&CtxVar{Name: name, Type: ty, Module: moduleName})
	}
}

// importConstructor pushes a data constructor into scope with ambiguity
// checking and constructor-module bookkeeping.
func (sc *Scope) importConstructor(conName string, ty types.Type, moduleName string, s span.Span) {
	if !sc.checkAmbiguousName(conName, moduleName, s) {
		sc.reg.conTypes[conName] = ty
		sc.session.ctx.Push(&CtxVar{Name: conName, Type: ty, Module: moduleName})
		sc.reg.conModules[conName] = moduleName
	}
}

// importSelective imports only the names specified in the import list.
func (sc *Scope) importSelective(mod *ModuleExports, imp syntax.DeclImport) {
	// Instances always imported regardless of selective list (coherence).
	sc.importInstances(mod)

	for _, in := range imp.Names {
		name := in.Name
		found := false

		// Value binding (lowercase name or operator)
		if ty, ok := mod.Values[name]; ok {
			sc.importValue(name, ty, imp.ModuleName, imp.S)
			found = true
		}

		// Type constructor
		if kind, ok := mod.Types[name]; ok {
			if !sc.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				sc.reg.typeKinds[name] = kind
			}
			found = true

			// Import constructors if HasSub
			if in.HasSub {
				sc.importTypeSubs(mod, name, in, imp.ModuleName, imp.S)
			}
		}

		// Type alias
		if alias, ok := mod.Aliases[name]; ok {
			if !sc.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				sc.reg.aliases[name] = alias
			}
			found = true
		}

		// Class
		if cls, ok := mod.Classes[name]; ok {
			if !sc.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				sc.reg.classes[name] = cls
			}
			found = true

			// Import class methods
			if in.HasSub {
				sc.importClassSubs(mod, cls, in, imp.ModuleName, imp.S)
			} else {
				// Bare class name: import all methods
				for _, m := range cls.Methods {
					if ty, ok := mod.Values[m.Name]; ok {
						sc.importValue(m.Name, ty, imp.ModuleName, imp.S)
					}
				}
			}
		}

		// Type family
		if fam, ok := mod.TypeFamilies[name]; ok {
			if !sc.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				sc.reg.families[name] = fam.Clone()
			}
			found = true
		}

		// Promoted kinds
		if kind, ok := mod.PromotedKinds[name]; ok {
			if !sc.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				sc.reg.promotedKinds[name] = kind
			}
			found = true
		}
		if kind, ok := mod.PromotedCons[name]; ok {
			if !sc.checkAmbiguousTypeName(name, imp.ModuleName, imp.S) {
				sc.reg.promotedCons[name] = kind
			}
			found = true
		}

		if !found {
			sc.session.addCodedError(errs.ErrImport, imp.S,
				fmt.Sprintf("module %s does not export: %s", imp.ModuleName, name))
		}
	}
}

// importTypeSubs imports constructors for a type based on the import name spec.
func (sc *Scope) importTypeSubs(mod *ModuleExports, typeName string, in syntax.ImportName, moduleName string, s span.Span) {
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
				sc.reg.conInfo[conName] = info
			}
			if ty, ok := mod.ConTypes[conName]; ok {
				sc.importConstructor(conName, ty, moduleName, s)
			}
		}
	}
}

// importClassSubs imports class methods based on the import name spec.
func (sc *Scope) importClassSubs(mod *ModuleExports, cls *ClassInfo, in syntax.ImportName, moduleName string, s span.Span) {
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
				sc.importValue(m.Name, ty, moduleName, s)
			}
		}
	}
}
