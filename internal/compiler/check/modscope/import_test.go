// Import tests — module import processing, ambiguity resolution, ownership detection.
package modscope

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- test harness ---

type recorded struct {
	typeKinds    map[string]types.Type
	aliases      map[string]*env.AliasInfo
	classes      map[string]*env.ClassInfo
	families     map[string]*env.TypeFamilyInfo
	dataTypes    map[string]*env.DataTypeInfo
	promotedK    map[string]types.Type
	promotedC    map[string]types.Type
	conBindings  map[string]types.Type
	conModules   map[string]string
	conInfos     map[string]*env.DataTypeInfo
	instances    []*env.InstanceInfo
	vars         map[string]types.Type
	varModules   map[string]string
	errors       []string
	familyErrors []string
}

func newRecorded() *recorded {
	return &recorded{
		typeKinds:   make(map[string]types.Type),
		aliases:     make(map[string]*env.AliasInfo),
		classes:     make(map[string]*env.ClassInfo),
		families:    make(map[string]*env.TypeFamilyInfo),
		dataTypes:   make(map[string]*env.DataTypeInfo),
		promotedK:   make(map[string]types.Type),
		promotedC:   make(map[string]types.Type),
		conBindings: make(map[string]types.Type),
		conModules:  make(map[string]string),
		conInfos:    make(map[string]*env.DataTypeInfo),
		vars:        make(map[string]types.Type),
		varModules:  make(map[string]string),
	}
}

func (r *recorded) importEnv() ImportEnv {
	return ImportEnv{
		RegisterTypeKind:     func(n string, k types.Type) { r.typeKinds[n] = k },
		RegisterAlias:        func(n string, a *env.AliasInfo) { r.aliases[n] = a },
		RegisterClass:        func(n string, c *env.ClassInfo) { r.classes[n] = c },
		RegisterFamily:       func(n string, f *env.TypeFamilyInfo) error { r.families[n] = f; return nil },
		RegisterDataType:     func(n string, d *env.DataTypeInfo) { r.dataTypes[n] = d },
		RegisterPromotedKind: func(n string, k types.Type) { r.promotedK[n] = k },
		RegisterPromotedCon:  func(n string, k types.Type) { r.promotedC[n] = k },
		SetConBinding:        func(n string, t types.Type, m string) { r.conBindings[n] = t; r.conModules[n] = m },
		SetConInfo:           func(n string, d *env.DataTypeInfo) { r.conInfos[n] = d },
		ImportInstance:       func(i *env.InstanceInfo) { r.instances = append(r.instances, i) },
		AddError:             func(_ diagnostic.Code, _ span.Span, msg string) { r.errors = append(r.errors, msg) },
		PushVar:              func(n string, t types.Type, m string) { r.vars[n] = t; r.varModules[n] = m },
	}
}

// --- module builders ---

func emptyModule() *env.ModuleExports {
	return &env.ModuleExports{
		Types:           make(map[string]types.Type),
		ConTypes:        make(map[string]types.Type),
		ConstructorInfo: make(map[string]*env.DataTypeInfo),
		Aliases:         make(map[string]*env.AliasInfo),
		Classes:         make(map[string]*env.ClassInfo),
		Values:          make(map[string]types.Type),
		PromotedKinds:   make(map[string]types.Type),
		PromotedCons:    make(map[string]types.Type),
		TypeFamilies:    make(map[string]*env.TypeFamilyInfo),
	}
}

// withTypes adds type kinds to a module.
func withTypes(mod *env.ModuleExports, names ...string) *env.ModuleExports {
	for _, n := range names {
		mod.Types[n] = types.TypeOfTypes
	}
	return mod
}

// withValues adds value bindings to a module.
func withValues(mod *env.ModuleExports, names ...string) *env.ModuleExports {
	for _, n := range names {
		mod.Values[n] = types.Con(n + "Ty")
	}
	return mod
}

// withConstructors adds constructors for a type.
func withConstructors(mod *env.ModuleExports, typeName string, cons ...string) *env.ModuleExports {
	dti := &env.DataTypeInfo{Name: typeName}
	for _, c := range cons {
		mod.ConTypes[c] = types.Con(typeName)
		mod.ConstructorInfo[c] = dti
		dti.Constructors = append(dti.Constructors, env.ConstructorInfo{Name: c})
	}
	if mod.ConstructorsByType == nil {
		mod.ConstructorsByType = make(map[string][]string)
	}
	mod.ConstructorsByType[typeName] = cons
	return mod
}

// withClass adds a class with methods to a module.
func withClass(mod *env.ModuleExports, className string, methods ...string) *env.ModuleExports {
	cls := &env.ClassInfo{Name: className}
	for _, m := range methods {
		cls.Methods = append(cls.Methods, env.MethodInfo{Name: m, Type: types.Con(m + "Ty")})
		mod.Values[m] = types.Con(m + "Ty")
	}
	mod.Classes[className] = cls
	return mod
}

// withFamily adds a type family to a module.
func withFamily(mod *env.ModuleExports, name string) *env.ModuleExports {
	mod.TypeFamilies[name] = &env.TypeFamilyInfo{
		Name:       name,
		Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
	}
	return mod
}

// withAlias adds a type alias to a module.
func withAlias(mod *env.ModuleExports, name string) *env.ModuleExports {
	mod.Aliases[name] = &env.AliasInfo{Body: types.Con("Int")}
	return mod
}

// withInstance adds an instance to a module.
func withInstance(mod *env.ModuleExports, className string) *env.ModuleExports {
	mod.Instances = append(mod.Instances, &env.InstanceInfo{ClassName: className})
	return mod
}

// withPromoted adds promoted kinds and cons.
func withPromoted(mod *env.ModuleExports, names ...string) *env.ModuleExports {
	for _, n := range names {
		mod.PromotedKinds[n] = types.TypeOfTypes
		mod.PromotedCons[n] = types.TypeOfTypes
	}
	return mod
}

// withOwned marks names as owned by this module.
func withOwned(mod *env.ModuleExports, typeNames, names []string) *env.ModuleExports {
	if mod.OwnedTypeNames == nil {
		mod.OwnedTypeNames = make(map[string]bool)
	}
	if mod.OwnedNames == nil {
		mod.OwnedNames = make(map[string]bool)
	}
	for _, n := range typeNames {
		mod.OwnedTypeNames[n] = true
	}
	for _, n := range names {
		mod.OwnedNames[n] = true
	}
	return mod
}

// --- Import dispatch tests ---

func TestImport_OpenImport(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withValues(withTypes(emptyModule(), "Bool"), "not")

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Lib"}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.typeKinds["Bool"]; !ok {
		t.Error("expected Bool type to be registered")
	}
	if _, ok := rec.vars["not"]; !ok {
		t.Error("expected 'not' value to be registered")
	}
	if len(rec.errors) > 0 {
		t.Errorf("unexpected errors: %v", rec.errors)
	}
}

func TestImport_QualifiedImport(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withValues(emptyModule(), "foo")
	withInstance(mod, "Eq")

	scopes := imp.Import(
		[]syntax.DeclImport{{ModuleName: "Lib", Alias: "L"}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := scopes["L"]; !ok {
		t.Error("expected qualified scope L to be created")
	}
	// Values should NOT be in unqualified scope.
	if _, ok := rec.vars["foo"]; ok {
		t.Error("qualified import should not push values to unqualified scope")
	}
	// Instances must always be imported.
	if len(rec.instances) != 1 {
		t.Errorf("expected 1 instance imported, got %d", len(rec.instances))
	}
}

func TestImport_SelectiveImport(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withValues(withTypes(emptyModule(), "Color"), "red", "green")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "red"}, {Name: "Color"}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.vars["red"]; !ok {
		t.Error("expected 'red' to be imported")
	}
	if _, ok := rec.vars["green"]; ok {
		t.Error("'green' should not be imported (not in selective list)")
	}
	if _, ok := rec.typeKinds["Color"]; !ok {
		t.Error("expected Color type to be imported")
	}
}

func TestImport_DuplicateImport(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := emptyModule()

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "Lib"},
			{ModuleName: "Lib"},
		},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if len(rec.errors) == 0 || !strings.Contains(rec.errors[0], "duplicate import") {
		t.Errorf("expected duplicate import error, got: %v", rec.errors)
	}
}

func TestImport_UnknownModule(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Unknown"}},
		map[string]*env.ModuleExports{},
		nil,
	)

	if len(rec.errors) == 0 || !strings.Contains(rec.errors[0], "unknown module") {
		t.Errorf("expected unknown module error, got: %v", rec.errors)
	}
}

func TestImport_NilModules(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Lib"}},
		nil, nil,
	)

	if len(rec.errors) == 0 || !strings.Contains(rec.errors[0], "unknown module") {
		t.Errorf("expected unknown module error with nil modules, got: %v", rec.errors)
	}
}

func TestImport_NilModulesNoImports(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())

	scopes := imp.Import(nil, nil, nil)

	if len(rec.errors) != 0 {
		t.Errorf("expected no errors with no imports, got: %v", rec.errors)
	}
	if len(scopes) != 0 {
		t.Errorf("expected empty scopes, got %d", len(scopes))
	}
}

func TestImport_CoreSelectiveRejected(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Core", Names: []syntax.ImportName{{Name: "pure"}}}},
		map[string]*env.ModuleExports{"Core": emptyModule()},
		nil,
	)

	if len(rec.errors) == 0 || !strings.Contains(rec.errors[0], "Core module") {
		t.Errorf("expected Core rejection error, got: %v", rec.errors)
	}
}

func TestImport_CoreQualifiedRejected(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Core", Alias: "C"}},
		map[string]*env.ModuleExports{"Core": emptyModule()},
		nil,
	)

	if len(rec.errors) == 0 || !strings.Contains(rec.errors[0], "Core module") {
		t.Errorf("expected Core rejection error, got: %v", rec.errors)
	}
}

func TestImport_AliasCollision(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "A", Alias: "X"},
			{ModuleName: "B", Alias: "X"},
		},
		map[string]*env.ModuleExports{"A": emptyModule(), "B": emptyModule()},
		nil,
	)

	if len(rec.errors) == 0 || !strings.Contains(rec.errors[0], "alias X already used") {
		t.Errorf("expected alias collision error, got: %v", rec.errors)
	}
}

// --- Open import detail tests ---

func TestImportOpen_Constructors(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withConstructors(withTypes(emptyModule(), "Color"), "Color", "Red", "Blue")

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Lib"}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.conBindings["Red"]; !ok {
		t.Error("expected Red constructor")
	}
	if _, ok := rec.conBindings["Blue"]; !ok {
		t.Error("expected Blue constructor")
	}
	if _, ok := rec.conInfos["Red"]; !ok {
		t.Error("expected Red constructor info")
	}
}

func TestImportOpen_ClassAndMethods(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withClass(emptyModule(), "Eq", "eq", "neq")

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Lib"}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.classes["Eq"]; !ok {
		t.Error("expected Eq class")
	}
	if _, ok := rec.vars["eq"]; !ok {
		t.Error("expected eq method")
	}
	if _, ok := rec.vars["neq"]; !ok {
		t.Error("expected neq method")
	}
}

func TestImportOpen_AliasAndFamily(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withFamily(withAlias(emptyModule(), "Text"), "Map")

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Lib"}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.aliases["Text"]; !ok {
		t.Error("expected Text alias")
	}
	if _, ok := rec.families["Map"]; !ok {
		t.Error("expected Map family")
	}
}

func TestImportOpen_Promoted(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withPromoted(emptyModule(), "True", "False")

	imp.Import(
		[]syntax.DeclImport{{ModuleName: "Lib"}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.promotedK["True"]; !ok {
		t.Error("expected True promoted kind")
	}
	if _, ok := rec.promotedC["False"]; !ok {
		t.Error("expected False promoted con")
	}
}

// --- Selective import detail tests ---

func TestImportSelective_TypeWithAllSubs(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withConstructors(withTypes(emptyModule(), "Maybe"), "Maybe", "Nothing", "Just")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "Maybe", HasSub: true, AllSubs: true}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.conBindings["Just"]; !ok {
		t.Error("expected Just constructor via (..) import")
	}
	if _, ok := rec.conBindings["Nothing"]; !ok {
		t.Error("expected Nothing constructor via (..) import")
	}
}

func TestImportSelective_TypeWithSpecificSubs(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withConstructors(withTypes(emptyModule(), "Color"), "Color", "Red", "Green", "Blue")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "Color", HasSub: true, SubList: []string{"Red"}}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.conBindings["Red"]; !ok {
		t.Error("expected Red constructor")
	}
	if _, ok := rec.conBindings["Green"]; ok {
		t.Error("Green should not be imported (not in sub-list)")
	}
}

func TestImportSelective_ClassWithAllSubs(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withClass(emptyModule(), "Show", "show", "showList")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "Show", HasSub: true, AllSubs: true}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.classes["Show"]; !ok {
		t.Error("expected Show class")
	}
	if _, ok := rec.vars["show"]; !ok {
		t.Error("expected show method")
	}
	if _, ok := rec.vars["showList"]; !ok {
		t.Error("expected showList method")
	}
}

func TestImportSelective_ClassWithSpecificSubs(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withClass(emptyModule(), "Show", "show", "showList")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "Show", HasSub: true, SubList: []string{"show"}}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.vars["show"]; !ok {
		t.Error("expected show method")
	}
	if _, ok := rec.vars["showList"]; ok {
		t.Error("showList should not be imported")
	}
}

func TestImportSelective_ClassBareImportsAllMethods(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withClass(emptyModule(), "Eq", "eq", "neq")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "Eq"}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.classes["Eq"]; !ok {
		t.Error("expected Eq class")
	}
	// Bare class name imports all methods.
	if _, ok := rec.vars["eq"]; !ok {
		t.Error("expected eq method (bare class import)")
	}
	if _, ok := rec.vars["neq"]; !ok {
		t.Error("expected neq method (bare class import)")
	}
}

func TestImportSelective_TypeSubsNoConstructors(t *testing.T) {
	// Type with (..) but no constructors registered → importTypeSubs returns early.
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withTypes(emptyModule(), "Void")
	// Void has no constructors in ConstructorsByType.

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "Void", HasSub: true, AllSubs: true}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.typeKinds["Void"]; !ok {
		t.Error("expected Void type to be imported")
	}
	if len(rec.conBindings) != 0 {
		t.Error("expected no constructors")
	}
	if len(rec.errors) != 0 {
		t.Errorf("expected no errors, got: %v", rec.errors)
	}
}

func TestImportSelective_NonExportedName(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withValues(emptyModule(), "foo")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "bar"}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if len(rec.errors) == 0 || !strings.Contains(rec.errors[0], "does not export: bar") {
		t.Errorf("expected 'does not export' error, got: %v", rec.errors)
	}
}

func TestImportSelective_ErrorNameSkipped(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withValues(emptyModule(), "foo")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "bad", Error: true}, {Name: "foo"}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if len(rec.errors) != 0 {
		t.Errorf("expected no errors (error names skipped), got: %v", rec.errors)
	}
	if _, ok := rec.vars["foo"]; !ok {
		t.Error("expected foo to still be imported")
	}
}

func TestImportSelective_AliasAndFamily(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withFamily(withAlias(emptyModule(), "Text"), "Map")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "Text"}, {Name: "Map"}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.aliases["Text"]; !ok {
		t.Error("expected Text alias")
	}
	if _, ok := rec.families["Map"]; !ok {
		t.Error("expected Map family")
	}
}

func TestImportSelective_PromotedKindsAndCons(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withPromoted(emptyModule(), "True")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "True"}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if _, ok := rec.promotedK["True"]; !ok {
		t.Error("expected True promoted kind")
	}
	if _, ok := rec.promotedC["True"]; !ok {
		t.Error("expected True promoted con")
	}
}

func TestImportSelective_InstancesAlwaysImported(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withInstance(withValues(emptyModule(), "foo"), "Eq")

	imp.Import(
		[]syntax.DeclImport{{
			ModuleName: "Lib",
			Names:      []syntax.ImportName{{Name: "foo"}},
		}},
		map[string]*env.ModuleExports{"Lib": mod},
		nil,
	)

	if len(rec.instances) != 1 {
		t.Errorf("instances must always be imported, got %d", len(rec.instances))
	}
}

// --- Ambiguity resolution tests ---

func TestAmbiguity_SameModuleNoConflict(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	mod := withValues(emptyModule(), "foo")

	// Import same module twice via open (first import sets name, second is dup).
	// We test the internal logic directly: import once, then check same-module exemption.
	imp.Import(
		[]syntax.DeclImport{{ModuleName: "A"}},
		map[string]*env.ModuleExports{"A": mod},
		nil,
	)

	if len(rec.errors) != 0 {
		t.Errorf("expected no errors, got: %v", rec.errors)
	}
}

func TestAmbiguity_DifferentModulesConflict(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withOwned(withValues(emptyModule(), "foo"), nil, []string{"foo"})
	modB := withOwned(withValues(emptyModule(), "foo"), nil, []string{"foo"})

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "A"},
			{ModuleName: "B"},
		},
		map[string]*env.ModuleExports{"A": modA, "B": modB},
		nil,
	)

	hasAmbig := false
	for _, e := range rec.errors {
		if strings.Contains(e, "ambiguous") {
			hasAmbig = true
			break
		}
	}
	if !hasAmbig {
		t.Errorf("expected ambiguity error, got: %v", rec.errors)
	}
}

func TestAmbiguity_ReExportNoConflict(t *testing.T) {
	// Both A and B re-export "foo" from a shared dep C.
	// Neither owns it → no conflict.
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modC := withOwned(withValues(emptyModule(), "foo"), nil, []string{"foo"})
	modA := withValues(emptyModule(), "foo") // re-exports foo, doesn't own it
	modB := withValues(emptyModule(), "foo")

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "A"},
			{ModuleName: "B"},
		},
		map[string]*env.ModuleExports{"A": modA, "B": modB, "C": modC},
		map[string][]string{"A": {"C"}, "B": {"C"}},
	)

	for _, e := range rec.errors {
		if strings.Contains(e, "ambiguous") {
			t.Errorf("should not conflict when both re-export: %v", rec.errors)
			break
		}
	}
}

func TestAmbiguity_PrivateNameExempt(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withValues(emptyModule(), "_private")
	modB := withValues(emptyModule(), "_private")

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "A"},
			{ModuleName: "B"},
		},
		map[string]*env.ModuleExports{"A": modA, "B": modB},
		nil,
	)

	for _, e := range rec.errors {
		if strings.Contains(e, "ambiguous") {
			t.Errorf("private names should be exempt from ambiguity: %v", rec.errors)
		}
	}
}

func TestAmbiguity_OwnerDependsOnPrevious(t *testing.T) {
	// A owns "foo", B re-exports from A (B depends on A).
	// B's "foo" should not conflict.
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withOwned(withValues(emptyModule(), "foo"), nil, []string{"foo"})
	modB := withValues(emptyModule(), "foo") // re-export

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "A"},
			{ModuleName: "B"},
		},
		map[string]*env.ModuleExports{"A": modA, "B": modB},
		map[string][]string{"B": {"A"}},
	)

	for _, e := range rec.errors {
		if strings.Contains(e, "ambiguous") {
			t.Errorf("re-export from dependency should not conflict: %v", rec.errors)
		}
	}
}

func TestAmbiguity_OwnerDependsOnCurrent(t *testing.T) {
	// B owns "foo", A re-exports from B (A depends on B).
	// Import A first, then B → previous (A) depends on current (B).
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withValues(emptyModule(), "foo") // re-export
	modB := withOwned(withValues(emptyModule(), "foo"), nil, []string{"foo"})

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "A"},
			{ModuleName: "B"},
		},
		map[string]*env.ModuleExports{"A": modA, "B": modB},
		map[string][]string{"A": {"B"}},
	)

	for _, e := range rec.errors {
		if strings.Contains(e, "ambiguous") {
			t.Errorf("re-export from dependency should not conflict: %v", rec.errors)
		}
	}
}

func TestAmbiguity_TypeNamespace(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withOwned(withTypes(emptyModule(), "T"), []string{"T"}, nil)
	modB := withOwned(withTypes(emptyModule(), "T"), []string{"T"}, nil)

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "A"},
			{ModuleName: "B"},
		},
		map[string]*env.ModuleExports{"A": modA, "B": modB},
		nil,
	)

	hasAmbig := false
	for _, e := range rec.errors {
		if strings.Contains(e, "ambiguous type name") {
			hasAmbig = true
			break
		}
	}
	if !hasAmbig {
		t.Errorf("expected type name ambiguity error, got: %v", rec.errors)
	}
}

func TestAmbiguity_AmbiguousClassBlocksMethods(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withOwned(withClass(emptyModule(), "Eq", "eq"), []string{"Eq"}, nil)
	modB := withOwned(withClass(emptyModule(), "Eq", "eq"), []string{"Eq"}, nil)

	imp.Import(
		[]syntax.DeclImport{
			{ModuleName: "A"},
			{ModuleName: "B"},
		},
		map[string]*env.ModuleExports{"A": modA, "B": modB},
		nil,
	)

	// eq should not be registered because Eq is ambiguous in open import.
	// The first module's eq is registered, but the second's Eq is ambiguous,
	// and its methods are blocked. The first eq remains.
	// The key test: no panic, and ambiguity is reported for the class.
	hasClassAmbig := false
	for _, e := range rec.errors {
		if strings.Contains(e, "ambiguous") && strings.Contains(e, "Eq") {
			hasClassAmbig = true
		}
	}
	if !hasClassAmbig {
		t.Errorf("expected class ambiguity for Eq, got: %v", rec.errors)
	}
}

// --- moduleDependsOn tests ---

func TestModuleDependsOn_Direct(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	imp.modules = map[string]*env.ModuleExports{}
	imp.deps = map[string][]string{"A": {"B"}}

	if !imp.moduleDependsOn("A", "B") {
		t.Error("A directly depends on B")
	}
}

func TestModuleDependsOn_Transitive(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	imp.modules = map[string]*env.ModuleExports{}
	imp.deps = map[string][]string{"A": {"B"}, "B": {"C"}}

	if !imp.moduleDependsOn("A", "C") {
		t.Error("A transitively depends on C via B")
	}
}

func TestModuleDependsOn_NoDep(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	imp.modules = map[string]*env.ModuleExports{}
	imp.deps = map[string][]string{"A": {"B"}}

	if imp.moduleDependsOn("B", "A") {
		t.Error("B does not depend on A")
	}
	_ = rec
}

func TestModuleDependsOn_Cycle(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	imp.modules = map[string]*env.ModuleExports{}
	imp.deps = map[string][]string{"A": {"B"}, "B": {"A"}}

	// Should not infinite loop; returns true since A → B → A finds B.
	if !imp.moduleDependsOn("A", "B") {
		t.Error("A depends on B (even with cycle)")
	}
	_ = rec
}

func TestModuleDependsOn_CycleNotFound(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	imp.modules = map[string]*env.ModuleExports{}
	imp.deps = map[string][]string{"A": {"B"}, "B": {"A"}}

	// A depends on B and B depends on A, but neither depends on C.
	// The visited guard should stop the cycle and return false.
	if imp.moduleDependsOn("A", "C") {
		t.Error("A does not depend on C (cycle between A and B only)")
	}
	_ = rec
}

// --- moduleOwnedNames tests ---

func TestModuleOwnedNames_OwnedVsReExported(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modC := withValues(emptyModule(), "shared")
	modA := withOwned(withValues(emptyModule(), "own", "shared"), nil, []string{"own"})

	imp.modules = map[string]*env.ModuleExports{"A": modA, "C": modC}
	imp.deps = map[string][]string{"A": {"C"}}

	owned := imp.moduleOwnedNames("A")
	if !owned["own"] {
		t.Error("expected 'own' to be owned by A")
	}
	// "shared" is in both A.Values and C.Values → not owned by A.
	if owned["shared"] {
		t.Error("'shared' should not be owned by A (from dep C)")
	}
}

func TestModuleOwnedNames_Caching(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withOwned(withValues(emptyModule(), "foo"), nil, []string{"foo"})
	imp.modules = map[string]*env.ModuleExports{"A": modA}
	imp.deps = map[string][]string{}

	owned1 := imp.moduleOwnedNames("A")
	owned2 := imp.moduleOwnedNames("A")
	// Should return same cached map.
	if len(owned1) != len(owned2) {
		t.Error("cached result should be identical")
	}
	_ = rec
}

func TestModuleOwnedNames_UnknownModule(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	imp.modules = map[string]*env.ModuleExports{}
	imp.deps = map[string][]string{}

	owned := imp.moduleOwnedNames("Unknown")
	if owned != nil {
		t.Error("expected nil for unknown module")
	}
	_ = rec
}

func TestModuleOwnedTypeNames_OwnedVsReExported(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modC := withAlias(withClass(withFamily(emptyModule(), "FamC"), "ClsC"), "AliC")
	modA := withOwned(
		withAlias(withClass(withFamily(emptyModule(), "FamC"), "ClsC"), "AliC"),
		[]string{"OwnType"}, nil,
	)
	withFamily(modA, "FamA")

	imp.modules = map[string]*env.ModuleExports{"A": modA, "C": modC}
	imp.deps = map[string][]string{"A": {"C"}}

	owned := imp.moduleOwnedTypeNames("A")
	if !owned["OwnType"] {
		t.Error("expected 'OwnType' to be owned")
	}
	if !owned["FamA"] {
		t.Error("expected 'FamA' to be owned (not in dep)")
	}
	if owned["FamC"] {
		t.Error("FamC should not be owned (from dep C)")
	}
	if owned["ClsC"] {
		t.Error("ClsC should not be owned (from dep C)")
	}
	if owned["AliC"] {
		t.Error("AliC should not be owned (from dep C)")
	}
	_ = rec
}

func TestModuleOwnedTypeNames_Caching(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withOwned(withAlias(emptyModule(), "MyAlias"), []string{"MyAlias"}, nil)
	imp.modules = map[string]*env.ModuleExports{"A": modA}
	imp.deps = map[string][]string{}

	owned1 := imp.moduleOwnedTypeNames("A")
	owned2 := imp.moduleOwnedTypeNames("A")
	if len(owned1) != len(owned2) {
		t.Error("cached result should be identical")
	}
	_ = rec
}

func TestModuleOwnedTypeNames_OwnedAlias(t *testing.T) {
	// Alias in module A that is NOT in any dep → owned.
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	modA := withAlias(emptyModule(), "MyAlias")
	imp.modules = map[string]*env.ModuleExports{"A": modA}
	imp.deps = map[string][]string{}

	owned := imp.moduleOwnedTypeNames("A")
	if !owned["MyAlias"] {
		t.Error("expected MyAlias to be owned (no deps)")
	}
	_ = rec
}

func TestModuleOwnedTypeNames_UnknownModule(t *testing.T) {
	rec := newRecorded()
	imp := NewImporter(rec.importEnv())
	imp.modules = map[string]*env.ModuleExports{}
	imp.deps = map[string][]string{}

	owned := imp.moduleOwnedTypeNames("Unknown")
	if owned != nil {
		t.Error("expected nil for unknown module")
	}
	_ = rec
}
