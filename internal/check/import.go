package check

import (
	"fmt"
	"slices"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
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
	// If both modules define this name themselves (not re-exported), it's a true ambiguity.
	// If one or both re-export it from a shared dependency, it's the same name — no conflict.
	prevOwns := ch.moduleOwnsName(prev, name)
	curOwns := ch.moduleOwnsName(moduleName, name)
	if !prevOwns || !curOwns {
		// At least one side is a re-export — no true conflict.
		ch.scope.importedNames[name] = moduleName
		return false
	}
	ch.addCodedError(errs.ErrImport, s,
		fmt.Sprintf("ambiguous name %q: imported from both %s and %s (use qualified import to disambiguate)", name, prev, moduleName))
	return true
}

// moduleOwnsName checks if a module defines name in its own source (not re-exported).
// A module "owns" a name if it appears in the module's DataDecls (constructors),
// or in its Values but NOT in any of its direct dependency's Values.
func (ch *Checker) moduleOwnsName(moduleName, name string) bool {
	mod, ok := ch.config.ImportedModules[moduleName]
	if !ok {
		return false
	}
	// Check DataDecl constructors.
	for _, dd := range mod.DataDecls {
		if dd.Name == name {
			return true
		}
		for _, con := range dd.Cons {
			if con.Name == name {
				return true
			}
		}
	}
	// Check if name is in Values but not in any direct dep's Values.
	if _, inValues := mod.Values[name]; inValues {
		for _, dep := range ch.config.ModuleDeps[moduleName] {
			if depMod, ok := ch.config.ImportedModules[dep]; ok {
				if _, inDep := depMod.Values[name]; inDep {
					return false // inherited from dependency
				}
			}
		}
		return true // defined in this module
	}
	return false
}

// importOpen merges all exports from a module into the checker state (open import).
func (ch *Checker) importOpen(mod *ModuleExports, moduleName string, s span.Span) {
	for name, kind := range mod.Types {
		ch.config.RegisteredTypes[name] = kind
	}
	for name, ty := range mod.ConTypes {
		if ch.checkAmbiguousName(name, moduleName, s) {
			continue
		}
		ch.reg.conTypes[name] = ty
		ch.ctx.Push(&CtxVar{Name: name, Type: ty, Module: moduleName})
		ch.reg.conModules[name] = moduleName
	}
	for name, info := range mod.ConstructorInfo {
		ch.reg.conInfo[name] = info
		ch.reg.dataTypeByName[info.Name] = info
	}
	for name, alias := range mod.Aliases {
		ch.reg.aliases[name] = alias
	}
	for name, cls := range mod.Classes {
		ch.reg.classes[name] = cls
	}
	ch.importInstances(mod)
	for name, ty := range mod.Values {
		if ch.checkAmbiguousName(name, moduleName, s) {
			continue
		}
		ch.ctx.Push(&CtxVar{Name: name, Type: ty, Module: moduleName})
	}
	for name, kind := range mod.PromotedKinds {
		ch.reg.promotedKinds[name] = kind
	}
	for name, kind := range mod.PromotedCons {
		ch.reg.promotedCons[name] = kind
	}
	for name, fam := range mod.TypeFamilies {
		ch.reg.families[name] = fam.Clone()
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

// importSelective imports only the names specified in the import list.
func (ch *Checker) importSelective(mod *ModuleExports, imp syntax.DeclImport) {
	// Instances always imported regardless of selective list (coherence).
	ch.importInstances(mod)

	for _, in := range imp.Names {
		name := in.Name
		found := false

		// Value binding (lowercase name or operator)
		if ty, ok := mod.Values[name]; ok {
			if !ch.checkAmbiguousName(name, imp.ModuleName, imp.S) {
				ch.ctx.Push(&CtxVar{Name: name, Type: ty, Module: imp.ModuleName})
			}
			found = true
		}

		// Type constructor
		if kind, ok := mod.Types[name]; ok {
			ch.config.RegisteredTypes[name] = kind
			found = true

			// Import constructors if HasSub
			if in.HasSub {
				ch.importTypeSubs(mod, name, in, imp.ModuleName, imp.S)
			}
		}

		// Type alias
		if alias, ok := mod.Aliases[name]; ok {
			ch.reg.aliases[name] = alias
			found = true
		}

		// Class
		if cls, ok := mod.Classes[name]; ok {
			ch.reg.classes[name] = cls
			found = true

			// Import class methods
			if in.HasSub {
				ch.importClassSubs(mod, cls, in, imp.ModuleName, imp.S)
			} else {
				// Bare class name: import all methods
				for _, m := range cls.Methods {
					if ty, ok := mod.Values[m.Name]; ok {
						if !ch.checkAmbiguousName(m.Name, imp.ModuleName, imp.S) {
							ch.ctx.Push(&CtxVar{Name: m.Name, Type: ty, Module: imp.ModuleName})
						}
					}
				}
			}
		}

		// Type family
		if fam, ok := mod.TypeFamilies[name]; ok {
			ch.reg.families[name] = fam.Clone()
			found = true
		}

		// Promoted kinds
		if kind, ok := mod.PromotedKinds[name]; ok {
			ch.reg.promotedKinds[name] = kind
			found = true
		}
		if kind, ok := mod.PromotedCons[name]; ok {
			ch.reg.promotedCons[name] = kind
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
	for conName, info := range mod.ConstructorInfo {
		if info.Name != typeName {
			continue
		}
		if in.AllSubs || slices.Contains(in.SubList, conName) {
			ch.reg.conInfo[conName] = info
			if ty, ok := mod.ConTypes[conName]; ok {
				if !ch.checkAmbiguousName(conName, moduleName, s) {
					ch.reg.conTypes[conName] = ty
					ch.ctx.Push(&CtxVar{Name: conName, Type: ty, Module: moduleName})
					ch.reg.conModules[conName] = moduleName
				}
			}
		}
	}
}

// importClassSubs imports class methods based on the import name spec.
func (ch *Checker) importClassSubs(mod *ModuleExports, cls *ClassInfo, in syntax.ImportName, moduleName string, s span.Span) {
	for _, m := range cls.Methods {
		if in.AllSubs || slices.Contains(in.SubList, m.Name) {
			if ty, ok := mod.Values[m.Name]; ok {
				if !ch.checkAmbiguousName(m.Name, moduleName, s) {
					ch.ctx.Push(&CtxVar{Name: m.Name, Type: ty, Module: moduleName})
				}
			}
		}
	}
}
