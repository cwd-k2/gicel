package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Registry holds semantic registries populated during declaration
// processing and read during type checking.
type Registry struct {
	typeKinds         map[string]types.Type // registered type constructor kinds (absorbed from config)
	conModules        map[string]string     // constructor name → source module name
	conTypes          map[string]types.Type
	conInfo           map[string]*DataTypeInfo
	dataTypeByName    map[string]*DataTypeInfo // type name → DataTypeInfo (reverse index)
	aliases           map[string]*AliasInfo
	classes           map[string]*ClassInfo
	dictToClass       map[string]string // dict type name → class name (reverse of ClassInfo.DictName)
	instances         []*InstanceInfo
	instancesByClass  map[string][]*InstanceInfo
	importedInstances map[*InstanceInfo]bool
	promotedKinds     map[string]types.Type      // DataKinds: data name → promoted data kind
	promotedCons      map[string]types.Type      // DataKinds: nullary con → promoted data kind
	kindVars          map[string]bool            // HKT: kind variables in scope
	families          map[string]*TypeFamilyInfo // type family declarations
}

// RegisterConstructor records a constructor's type, owning module, and
// parent data type info. This is the single point of registration for
// the conTypes / conModules / conInfo triple.
func (r *Registry) RegisterConstructor(name string, ty types.Type, module string, info *DataTypeInfo) {
	r.conTypes[name] = ty
	r.conModules[name] = module
	r.conInfo[name] = info
}

// RegisterInstance appends an instance to the global list and the
// per-class index.
func (r *Registry) RegisterInstance(inst *InstanceInfo) {
	r.instances = append(r.instances, inst)
	r.instancesByClass[inst.ClassName] = append(r.instancesByClass[inst.ClassName], inst)
}

// RegisterTypeKind records a type constructor's kind.
func (r *Registry) RegisterTypeKind(name string, kind types.Type) {
	r.typeKinds[name] = kind
}

// RegisterAlias records a type alias.
// Phase: 2 (alias processing). Qualified names use Scope.InjectAlias instead.
func (r *Registry) RegisterAlias(name string, info *AliasInfo) {
	r.aliases[name] = info
}

// RegisterClass records a class declaration and its dict-to-class reverse mapping.
func (r *Registry) RegisterClass(name string, info *ClassInfo) {
	r.classes[name] = info
	if info.DictName != "" {
		r.dictToClass[info.DictName] = name
	}
}

// RegisterFamily records a type family declaration or merges equations
// into an existing family. When multiple modules independently enrich
// the same associated type family (diamond import), equations from all
// sources are collected and deduplicated by structural pattern identity.
func (r *Registry) RegisterFamily(name string, info *TypeFamilyInfo) error {
	existing, ok := r.families[name]
	if !ok {
		r.families[name] = info
		return nil
	}
	// Reject collision between associated types from different classes.
	if existing.IsAssoc && info.IsAssoc && existing.ClassName != info.ClassName {
		return fmt.Errorf("associated type %s conflicts: defined in both %s and %s",
			name, existing.ClassName, info.ClassName)
	}
	// Merge: append equations from info not already present.
	// When multiple modules independently enrich the same associated type
	// family (diamond import), equations from all sources are collected
	// and deduplicated by structural pattern identity.
	seen := make(map[string]bool, len(existing.Equations))
	for _, eq := range existing.Equations {
		seen[equationPatternKey(eq)] = true
	}
	for _, eq := range info.Equations {
		if key := equationPatternKey(eq); !seen[key] {
			existing.Equations = append(existing.Equations, eq)
			seen[key] = true
		}
	}
	return nil
}

// equationPatternKey produces a canonical key from the LHS patterns of a
// type family equation. Two equations with structurally equal patterns
// are considered identical for deduplication purposes.
func equationPatternKey(eq tfEquation) string {
	var b strings.Builder
	for i, p := range eq.Patterns {
		if i > 0 {
			b.WriteByte(' ')
		}
		types.WriteTypeKey(&b, p)
	}
	return b.String()
}

// RegisterDataType records a data type's reverse lookup entry.
func (r *Registry) RegisterDataType(name string, info *DataTypeInfo) {
	r.dataTypeByName[name] = info
}

// RegisterPromotedKind records a DataKinds promoted data name.
func (r *Registry) RegisterPromotedKind(name string, kind types.Type) {
	r.promotedKinds[name] = kind
}

// RegisterPromotedCon records a DataKinds promoted nullary constructor.
func (r *Registry) RegisterPromotedCon(name string, kind types.Type) {
	r.promotedCons[name] = kind
}

// SetKindVar marks a name as a kind variable in scope.
func (r *Registry) SetKindVar(name string) {
	r.kindVars[name] = true
}

// UnsetKindVar removes a kind variable from scope.
func (r *Registry) UnsetKindVar(name string) {
	delete(r.kindVars, name)
}

// ImportInstance imports an instance with pointer-identity deduplication.
// Unlike RegisterInstance, this skips instances already seen via import.
func (r *Registry) ImportInstance(inst *InstanceInfo) {
	if r.importedInstances[inst] {
		return
	}
	r.instances = append(r.instances, inst)
	r.instancesByClass[inst.ClassName] = append(r.instancesByClass[inst.ClassName], inst)
	r.importedInstances[inst] = true
}

// SetConBinding records a constructor's type and owning module without
// updating conInfo. Used during import when conInfo is set separately.
func (r *Registry) SetConBinding(name string, ty types.Type, module string) {
	r.conTypes[name] = ty
	r.conModules[name] = module
}

// SetConInfo records the DataTypeInfo for a constructor name.
func (r *Registry) SetConInfo(name string, info *DataTypeInfo) {
	r.conInfo[name] = info
}

// ---- Read accessors ----

// LookupConType returns the full type scheme for a constructor.
func (r *Registry) LookupConType(name string) (types.Type, bool) {
	ty, ok := r.conTypes[name]
	return ty, ok
}

// LookupConInfo returns the owning DataTypeInfo for a constructor.
func (r *Registry) LookupConInfo(name string) (*DataTypeInfo, bool) {
	info, ok := r.conInfo[name]
	return info, ok
}

// LookupConModule returns the source module for a constructor.
func (r *Registry) LookupConModule(name string) (string, bool) {
	mod, ok := r.conModules[name]
	return mod, ok
}

// LookupAlias returns the AliasInfo for a type alias.
func (r *Registry) LookupAlias(name string) (*AliasInfo, bool) {
	info, ok := r.aliases[name]
	return info, ok
}

// LookupClass returns the ClassInfo for a type class.
func (r *Registry) LookupClass(name string) (*ClassInfo, bool) {
	info, ok := r.classes[name]
	return info, ok
}

// LookupFamily returns the TypeFamilyInfo for a type family.
func (r *Registry) LookupFamily(name string) (*TypeFamilyInfo, bool) {
	info, ok := r.families[name]
	return info, ok
}

// LookupTypeKind returns the kind of a registered type constructor.
func (r *Registry) LookupTypeKind(name string) (types.Type, bool) {
	k, ok := r.typeKinds[name]
	return k, ok
}

// LookupDataType returns the DataTypeInfo for a data type name.
func (r *Registry) LookupDataType(name string) (*DataTypeInfo, bool) {
	info, ok := r.dataTypeByName[name]
	return info, ok
}

// ClassFromDict returns the class name for a given dict type name.
func (r *Registry) ClassFromDict(dictName string) (string, bool) {
	name, ok := r.dictToClass[dictName]
	return name, ok
}

// InstancesForClass returns all instances registered for a class.
func (r *Registry) InstancesForClass(className string) []*InstanceInfo {
	return r.instancesByClass[className]
}

// IsKindVar reports whether a name is currently a kind variable in scope.
func (r *Registry) IsKindVar(name string) bool {
	return r.kindVars[name]
}

// HasPromotedKind reports whether a DataKinds promoted kind exists.
func (r *Registry) HasPromotedKind(name string) bool {
	_, ok := r.promotedKinds[name]
	return ok
}

// LookupPromotedKind returns the kind for a DataKinds promoted data name.
func (r *Registry) LookupPromotedKind(name string) (types.Type, bool) {
	k, ok := r.promotedKinds[name]
	return k, ok
}

// HasPromotedCon reports whether a DataKinds promoted constructor exists.
func (r *Registry) HasPromotedCon(name string) bool {
	_, ok := r.promotedCons[name]
	return ok
}

// LookupPromotedCon returns the kind for a DataKinds promoted constructor.
func (r *Registry) LookupPromotedCon(name string) (types.Type, bool) {
	k, ok := r.promotedCons[name]
	return k, ok
}

// IsImportedInstance reports whether an instance was imported (for dedup).
func (r *Registry) IsImportedInstance(inst *InstanceInfo) bool {
	return r.importedInstances[inst]
}

// ---- Iteration accessors (read-only views) ----
//
// These return the underlying maps directly. Callers must not mutate them.
// Used by ExportModule to filter and project registry contents.

// AllDataTypes returns the type name -> DataTypeInfo map.
// The caller must not modify the returned map.
func (r *Registry) AllDataTypes() map[string]*DataTypeInfo {
	return r.dataTypeByName
}

// AllConInfo returns the constructor -> DataTypeInfo map.
// The caller must not modify the returned map.
func (r *Registry) AllConInfo() map[string]*DataTypeInfo {
	return r.conInfo
}

// AllConTypes returns the constructor -> full type scheme map.
// The caller must not modify the returned map.
func (r *Registry) AllConTypes() map[string]types.Type {
	return r.conTypes
}

// AllAliases returns the alias name -> AliasInfo map.
// The caller must not modify the returned map.
func (r *Registry) AllAliases() map[string]*AliasInfo {
	return r.aliases
}

// AllClasses returns the class name -> ClassInfo map.
// The caller must not modify the returned map.
func (r *Registry) AllClasses() map[string]*ClassInfo {
	return r.classes
}

// AllInstances returns the ordered slice of all registered instances.
// The caller must not modify the returned slice.
func (r *Registry) AllInstances() []*InstanceInfo {
	return r.instances
}

// AllPromotedKinds returns the promoted data name -> kind map.
// The caller must not modify the returned map.
func (r *Registry) AllPromotedKinds() map[string]types.Type {
	return r.promotedKinds
}

// AllPromotedCons returns the promoted constructor -> kind map.
// The caller must not modify the returned map.
func (r *Registry) AllPromotedCons() map[string]types.Type {
	return r.promotedCons
}

// AllFamilies returns the type family name -> TypeFamilyInfo map.
// The caller must not modify the returned map.
func (r *Registry) AllFamilies() map[string]*TypeFamilyInfo {
	return r.families
}
