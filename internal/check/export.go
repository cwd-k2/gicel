package check

import (
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/types"
)

// ExportModule captures the current checker state as a ModuleExports.
// Names starting with '_' are private and excluded from exports.
// Instances are never filtered (coherence requirement).
func (ch *Checker) ExportModule(prog *core.Program) *ModuleExports {
	values := make(map[string]types.Type)
	for _, b := range prog.Bindings {
		if !isPrivateName(b.Name) {
			values[b.Name] = b.Type
		}
	}

	// Filter constructors: exclude private constructors and constructors of private types.
	filteredConInfo := filterPrivateConstructors(ch.reg.conInfo, ch.reg.conInfo)
	filteredConTypes := filterPrivateConstructors(ch.reg.conTypes, ch.reg.conInfo)

	// Build precomputed constructor-by-type index from filtered data.
	consByType := make(map[string][]string, len(filteredConInfo))
	for conName, info := range filteredConInfo {
		consByType[info.Name] = append(consByType[info.Name], conName)
	}

	return &ModuleExports{
		Types:              filterPrivateMap(ch.config.RegisteredTypes),
		ConTypes:           filteredConTypes,
		ConstructorInfo:    filteredConInfo,
		ConstructorsByType: consByType,
		Aliases:            filterPrivateMap(ch.reg.aliases),
		Classes:            filterPrivateMap(ch.reg.classes),
		Instances:          ch.reg.instances,
		Values:             values,
		PromotedKinds:      filterPrivateMap(ch.reg.promotedKinds),
		PromotedCons:       filterPrivateMap(ch.reg.promotedCons),
		TypeFamilies:       filterPrivateMap(cloneFamilies(ch.reg.families)),
		DataDecls:          filterPrivateDataDecls(prog.DataDecls),
	}
}

// filterPrivateMap returns a new map with private-named keys removed.
func filterPrivateMap[V any](m map[string]V) map[string]V {
	result := make(map[string]V, len(m))
	for k, v := range m {
		if !isPrivateName(k) {
			result[k] = v
		}
	}
	return result
}

// filterPrivateConstructors returns a new map with constructors removed
// that are private or belong to a private type.
func filterPrivateConstructors[V any](m map[string]V, conInfo map[string]*DataTypeInfo) map[string]V {
	result := make(map[string]V, len(m))
	for name, v := range m {
		if isPrivateName(name) {
			continue
		}
		if info, ok := conInfo[name]; ok && isPrivateName(info.Name) {
			continue
		}
		result[name] = v
	}
	return result
}

// filterPrivateDataDecls returns a copy of decls with private data declarations removed.
func filterPrivateDataDecls(decls []core.DataDecl) []core.DataDecl {
	result := make([]core.DataDecl, 0, len(decls))
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
