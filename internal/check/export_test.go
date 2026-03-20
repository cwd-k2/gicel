// Export tests — private name filtering in ExportModule.
// Does NOT cover: import-level ambiguity (import_collision_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/types"
)

// setupExportChecker creates a minimal Checker suitable for ExportModule tests.
// It populates registries directly, bypassing the parser (since the parser
// enforces uppercase on type/constructor names, but private names use '_' prefix
// or compiler-generated '$').
func setupExportChecker() *Checker {
	ch := newTestChecker()
	ch.config.RegisteredTypes = map[string]types.Kind{
		"Int":      types.KType{},
		"Public":   types.KType{},
		"_Private": types.KType{},
	}
	publicInfo := &DataTypeInfo{
		Name:         "Public",
		Constructors: []ConstructorInfo{{Name: "MkPublic", Arity: 0}},
	}
	privateInfo := &DataTypeInfo{
		Name:         "_Private",
		Constructors: []ConstructorInfo{{Name: "MkP", Arity: 0}, {Name: "_Hidden", Arity: 0}},
	}
	ch.reg.conTypes = map[string]types.Type{
		"MkPublic": types.Con("Public"),
		"MkP":      types.Con("_Private"),
		"_Hidden":  types.Con("_Private"),
	}
	ch.reg.conInfo = map[string]*DataTypeInfo{
		"MkPublic": publicInfo,
		"MkP":      privateInfo,
		"_Hidden":  privateInfo,
	}
	ch.reg.aliases = map[string]*AliasInfo{
		"PubAlias":  {Body: types.Con("Int")},
		"_PriAlias": {Body: types.Con("Int")},
	}
	ch.reg.classes = map[string]*ClassInfo{
		"PubClass":  {Name: "PubClass"},
		"_PriClass": {Name: "_PriClass"},
	}
	ch.reg.instances = []*InstanceInfo{
		{ClassName: "PubClass", TypeArgs: []types.Type{types.Con("Int")}, DictBindName: "PubClass$Int"},
	}
	ch.reg.promotedKinds = map[string]types.Kind{
		"Public":   types.KType{}, // promoted from data Public
		"_Private": types.KType{}, // promoted from data _Private
	}
	ch.reg.promotedCons = map[string]types.Kind{
		"MkPublic": types.KType{}, // promoted from constructor MkPublic (parent: Public)
		"_Hidden":  types.KType{}, // promoted from constructor _Hidden (parent: _Private)
	}
	ch.reg.families = map[string]*TypeFamilyInfo{
		"PubFam":  {Name: "PubFam", ResultKind: types.KType{}},
		"_PriFam": {Name: "_PriFam", ResultKind: types.KType{}},
	}
	return ch
}

func makeExportProgram() *core.Program {
	return &core.Program{
		Bindings: []core.Binding{
			{Name: "pubVal", Type: types.Con("Int")},
			{Name: "_priVal", Type: types.Con("Int")},
		},
		DataDecls: []core.DataDecl{
			{Name: "Public", Cons: []core.ConDecl{{Name: "MkPublic"}}},
			{Name: "_Private", Cons: []core.ConDecl{{Name: "MkP"}, {Name: "_Hidden"}}},
		},
	}
}

// TestExport_PrivateTypeExcluded — a type starting with '_' should not
// appear in module exports.
func TestExport_PrivateTypeExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.Types["_Private"]; ok {
		t.Error("_Private type should not be exported")
	}
	if _, ok := exports.Types["Public"]; !ok {
		t.Error("Public type should be exported")
	}
	// Int is a builtin type, not defined by this module — should NOT be exported.
	if _, ok := exports.Types["Int"]; ok {
		t.Error("Int (builtin) should not be exported by a user module")
	}
}

// TestExport_PrivateTypeConstructorsExcluded — constructors of a private
// type should not appear in module exports.
func TestExport_PrivateTypeConstructorsExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.ConTypes["MkP"]; ok {
		t.Error("MkP constructor should not be exported (parent type _Private is private)")
	}
	if _, ok := exports.ConstructorInfo["MkP"]; ok {
		t.Error("MkP constructor info should not be exported")
	}
	if _, ok := exports.ConTypes["MkPublic"]; !ok {
		t.Error("MkPublic constructor should be exported")
	}
}

// TestExport_PrivateConstructorExcluded — a constructor starting with '_'
// on a public type should also be excluded.
func TestExport_PrivateConstructorExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.ConTypes["_Hidden"]; ok {
		t.Error("_Hidden constructor should not be exported")
	}
}

// TestExport_PrivateClassExcluded — a class starting with '_' should
// not be exported.
func TestExport_PrivateClassExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.Classes["_PriClass"]; ok {
		t.Error("_PriClass should not be exported")
	}
	if _, ok := exports.Classes["PubClass"]; !ok {
		t.Error("PubClass should be exported")
	}
}

// TestExport_InstancesNotFiltered — instances should never be filtered
// even if they involve private types (coherence requirement).
func TestExport_InstancesNotFiltered(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if len(exports.Instances) == 0 {
		t.Fatal("instances should not be filtered (coherence)")
	}
	if exports.Instances[0].ClassName != "PubClass" {
		t.Errorf("expected PubClass instance, got %s", exports.Instances[0].ClassName)
	}
}

// TestExport_PrivateDataDeclExcluded — DataDecls for private types should
// not be exported.
func TestExport_PrivateDataDeclExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	for _, dd := range exports.DataDecls {
		if dd.Name == "_Private" {
			t.Error("_Private DataDecl should not be exported")
		}
	}
	found := false
	for _, dd := range exports.DataDecls {
		if dd.Name == "Public" {
			found = true
		}
	}
	if !found {
		t.Error("Public DataDecl should be exported")
	}
}

// TestExport_PrivateValueExcluded — value bindings starting with '_'
// should not be exported.
func TestExport_PrivateValueExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.Values["_priVal"]; ok {
		t.Error("_priVal should not be exported")
	}
	if _, ok := exports.Values["pubVal"]; !ok {
		t.Error("pubVal should be exported")
	}
}

// TestExport_PrivateAliasExcluded — aliases starting with '_' should not be exported.
func TestExport_PrivateAliasExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.Aliases["_PriAlias"]; ok {
		t.Error("_PriAlias should not be exported")
	}
	if _, ok := exports.Aliases["PubAlias"]; !ok {
		t.Error("PubAlias should be exported")
	}
}

// TestExport_PrivatePromotionsExcluded — promoted kinds/cons starting with '_'
// should not be exported.
func TestExport_PrivatePromotionsExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.PromotedKinds["_Private"]; ok {
		t.Error("_Private promoted kind should not be exported")
	}
	if _, ok := exports.PromotedKinds["Public"]; !ok {
		t.Error("Public promoted kind should be exported")
	}
	if _, ok := exports.PromotedCons["_Hidden"]; ok {
		t.Error("_Hidden promoted cons should not be exported")
	}
	if _, ok := exports.PromotedCons["MkPublic"]; !ok {
		t.Error("MkPublic promoted cons should be exported")
	}
}

// TestExport_PrivateFamilyExcluded — type families starting with '_'
// should not be exported.
func TestExport_PrivateFamilyExcluded(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.TypeFamilies["_PriFam"]; ok {
		t.Error("_PriFam should not be exported")
	}
	if _, ok := exports.TypeFamilies["PubFam"]; !ok {
		t.Error("PubFam should be exported")
	}
}

// TestExport_ConstructorsByTypeConsistent — ConstructorsByType should not
// contain private type entries and should exclude private constructors.
func TestExport_ConstructorsByTypeConsistent(t *testing.T) {
	ch := setupExportChecker()
	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.ConstructorsByType["_Private"]; ok {
		t.Error("_Private should not appear in ConstructorsByType")
	}
	pubCons := exports.ConstructorsByType["Public"]
	if len(pubCons) != 1 || pubCons[0] != "MkPublic" {
		t.Errorf("expected [MkPublic] in ConstructorsByType[Public], got %v", pubCons)
	}
}

// TestExport_CompilerGeneratedFiltered — names with '$' (compiler-generated)
// should be filtered as private.
func TestExport_CompilerGeneratedFiltered(t *testing.T) {
	ch := setupExportChecker()
	ch.reg.aliases["$internal"] = &AliasInfo{Body: types.Con("Int")}
	ch.reg.classes["Eq$Dict"] = &ClassInfo{Name: "Eq$Dict"}

	exports := ch.ExportModule(makeExportProgram())

	if _, ok := exports.Aliases["$internal"]; ok {
		t.Error("$internal alias should not be exported")
	}
	if _, ok := exports.Classes["Eq$Dict"]; ok {
		t.Error("Eq$Dict class should not be exported")
	}
}
