package check

import (
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ExportModule captures the current checker state as a ModuleExports.
// Only names defined by this module are exported; inherited types from
// imported modules are excluded (no transitive re-export).
// Names starting with '_' are private and excluded from exports.
// Instances are never filtered (coherence requirement).
func (ch *Checker) ExportModule(prog *ir.Program) *ModuleExports {
	values := make(map[string]types.Type)
	for _, b := range prog.Bindings {
		if !isPrivateName(b.Name) {
			values[b.Name] = b.Type
		}
	}

	// Owned data type names — the primary ownership signal.
	ownedDataNames := make(map[string]bool)
	for _, dd := range prog.DataDecls {
		if !isPrivateName(dd.Name) {
			ownedDataNames[dd.Name] = true
		}
	}

	// Types: only kind entries for data types defined by this module.
	ownedTypes := make(map[string]types.Kind, len(ownedDataNames))
	for name := range ownedDataNames {
		if kind, ok := ch.reg.typeKinds[name]; ok {
			ownedTypes[name] = kind
		}
	}

	// Constructors: only from owned data types.
	filteredConInfo := make(map[string]*DataTypeInfo)
	for name, info := range ch.reg.conInfo {
		if ownedDataNames[info.Name] && !isPrivateName(name) {
			filteredConInfo[name] = info
		}
	}
	filteredConTypes := make(map[string]types.Type)
	for name := range filteredConInfo {
		if ty, ok := ch.reg.conTypes[name]; ok {
			filteredConTypes[name] = ty
		}
	}
	consByType := make(map[string][]string, len(filteredConInfo))
	for conName, info := range filteredConInfo {
		consByType[info.Name] = append(consByType[info.Name], conName)
	}

	// Collect names imported from other modules to distinguish locally-defined
	// aliases, classes, and type families from inherited ones.
	impAliases := make(map[string]bool)
	impClasses := make(map[string]bool)
	impFamilyEqCount := make(map[string]int) // name → max equation count from imports
	for _, mod := range ch.config.ImportedModules {
		for n := range mod.Aliases {
			impAliases[n] = true
		}
		for n := range mod.Classes {
			impClasses[n] = true
		}
		for n, fam := range mod.TypeFamilies {
			if cnt := len(fam.Equations); cnt > impFamilyEqCount[n] {
				impFamilyEqCount[n] = cnt
			}
		}
	}

	// Promoted kinds/cons: only from owned data types.
	ownedPromKinds := make(map[string]types.Kind)
	for name, kind := range ch.reg.promotedKinds {
		if ownedDataNames[name] {
			ownedPromKinds[name] = kind
		}
	}
	ownedPromCons := make(map[string]types.Kind)
	for name, kind := range ch.reg.promotedCons {
		if info, ok := ch.reg.conInfo[name]; ok && ownedDataNames[info.Name] && !isPrivateName(name) {
			ownedPromCons[name] = kind
		}
	}

	return &ModuleExports{
		Types:              ownedTypes,
		ConTypes:           filteredConTypes,
		ConstructorInfo:    filteredConInfo,
		ConstructorsByType: consByType,
		Aliases:            filterOwnedMap(ch.reg.aliases, impAliases),
		Classes:            filterOwnedMap(ch.reg.classes, impClasses),
		Instances:          ch.reg.instances,
		Values:             values,
		PromotedKinds:      ownedPromKinds,
		PromotedCons:       ownedPromCons,
		// TypeFamilies: export locally defined families and imported families
		// that were enriched with new equations by this module (e.g. associated
		// type instances). Purely inherited families are excluded.
		TypeFamilies: filterOwnedOrEnrichedFamilies(ch.reg.families, impFamilyEqCount),
		DataDecls:    filterPrivateDataDecls(prog.DataDecls),
	}
}

// filterOwnedMap returns a new map excluding private names and names
// present in the imported set (i.e. inherited from other modules).
func filterOwnedMap[V any](m map[string]V, imported map[string]bool) map[string]V {
	result := make(map[string]V, len(m))
	for k, v := range m {
		if !isPrivateName(k) && !imported[k] {
			result[k] = v
		}
	}
	return result
}

// filterOwnedOrEnrichedFamilies returns families that are either locally defined
// (not present in any import) or locally enriched (have more equations than the
// imported version, indicating this module added associated type instances).
// Purely inherited families are excluded. Private names are always excluded.
func filterOwnedOrEnrichedFamilies(families map[string]*TypeFamilyInfo, impEqCount map[string]int) map[string]*TypeFamilyInfo {
	result := make(map[string]*TypeFamilyInfo, len(families))
	for name, fam := range families {
		if isPrivateName(name) {
			continue
		}
		importedCount, imported := impEqCount[name]
		if !imported {
			// Locally defined: always export.
			result[name] = fam.Clone()
		} else if len(fam.Equations) > importedCount {
			// Imported but locally enriched: export with new equations.
			result[name] = fam.Clone()
		}
		// Otherwise: purely inherited, skip.
	}
	return result
}

// filterPrivateDataDecls returns a copy of decls with private data declarations removed.
func filterPrivateDataDecls(decls []ir.DataDecl) []ir.DataDecl {
	result := make([]ir.DataDecl, 0, len(decls))
	for _, d := range decls {
		if !isPrivateName(d.Name) {
			result = append(result, d)
		}
	}
	return result
}

// isPrivateName reports whether a name is module-private.
// Private: '_' prefix (user convention) or compiler-generated identifier containing '$'.
// Operator names (e.g., <$>, $, +>) are never private even if they contain '$'.
func isPrivateName(name string) bool {
	if len(name) == 0 {
		return false
	}
	if name[0] == '_' {
		return true
	}
	// Compiler-generated names contain '$' in identifier context.
	// Operators (all non-alphanumeric) are exempt.
	if isOperatorName(name) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] == '$' {
			return true
		}
	}
	return false
}

// isOperatorName returns true if the name is an operator (all symbol characters).
func isOperatorName(name string) bool {
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return false
		}
	}
	return true
}
