package check

import "github.com/cwd-k2/gicel/internal/lang/types"

// Registry holds semantic registries populated during declaration
// processing and read during type checking.
type Registry struct {
	typeKinds         map[string]types.Kind // registered type constructor kinds (absorbed from config)
	conModules        map[string]string     // constructor name → source module name
	conTypes          map[string]types.Type
	conInfo           map[string]*DataTypeInfo
	dataTypeByName    map[string]*DataTypeInfo // type name → DataTypeInfo (reverse index)
	aliases           map[string]*AliasInfo
	classes           map[string]*ClassInfo
	instances         []*InstanceInfo
	instancesByClass  map[string][]*InstanceInfo
	importedInstances map[*InstanceInfo]bool
	promotedKinds     map[string]types.Kind      // DataKinds: data name → KData
	promotedCons      map[string]types.Kind      // DataKinds: nullary con → KData
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
func (r *Registry) RegisterTypeKind(name string, kind types.Kind) {
	r.typeKinds[name] = kind
}

// RegisterAlias records a type alias.
func (r *Registry) RegisterAlias(name string, info *AliasInfo) {
	r.aliases[name] = info
}

// RegisterClass records a class declaration.
func (r *Registry) RegisterClass(name string, info *ClassInfo) {
	r.classes[name] = info
}

// RegisterFamily records a type family declaration.
func (r *Registry) RegisterFamily(name string, info *TypeFamilyInfo) {
	r.families[name] = info
}

// RegisterDataType records a data type's reverse lookup entry.
func (r *Registry) RegisterDataType(name string, info *DataTypeInfo) {
	r.dataTypeByName[name] = info
}

// RegisterPromotedKind records a DataKinds promoted data name.
func (r *Registry) RegisterPromotedKind(name string, kind types.Kind) {
	r.promotedKinds[name] = kind
}

// RegisterPromotedCon records a DataKinds promoted nullary constructor.
func (r *Registry) RegisterPromotedCon(name string, kind types.Kind) {
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
