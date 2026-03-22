// Import collision tests — type-level ambiguity detection.
// Does NOT cover: value-level ambiguity (import_module_probe_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/family"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// mkTypeModule creates a ModuleExports that defines a data type.
func mkTypeModule(typeName string, cons []string) *ModuleExports {
	info := &DataTypeInfo{
		Name:         typeName,
		Constructors: make([]ConstructorInfo, len(cons)),
	}
	for i, c := range cons {
		info.Constructors[i] = ConstructorInfo{Name: c, Arity: 0}
	}
	conTypes := make(map[string]types.Type, len(cons))
	conInfo := make(map[string]*DataTypeInfo, len(cons))
	for _, c := range cons {
		conTypes[c] = types.Con(typeName)
		conInfo[c] = info
	}
	ownedNames := map[string]bool{typeName: true}
	for _, c := range cons {
		ownedNames[c] = true
	}
	return &ModuleExports{
		Types:           map[string]types.Kind{typeName: types.KType{}},
		ConTypes:        conTypes,
		ConstructorInfo: conInfo,
		Aliases:         map[string]*AliasInfo{},
		Classes:         map[string]*ClassInfo{},
		Values:          map[string]types.Type{},
		PromotedKinds:   map[string]types.Kind{},
		PromotedCons:    map[string]types.Kind{},
		TypeFamilies:    map[string]*TypeFamilyInfo{},
		ModuleOwnership: ModuleOwnership{
			OwnedTypeNames:     map[string]bool{typeName: true},
			OwnedNames:         ownedNames,
			ConstructorsByType: map[string][]string{typeName: cons},
		},
	}
}

// mkClassModule creates a ModuleExports that defines a type class.
func mkClassModule(className string, methodName string) *ModuleExports {
	cls := &ClassInfo{
		Name:         className,
		TyParams:     []string{"a"},
		TyParamKinds: []types.Kind{types.KType{}},
		Methods:      []MethodInfo{{Name: methodName}},
	}
	return &ModuleExports{
		Types:           map[string]types.Kind{},
		ConTypes:        map[string]types.Type{},
		ConstructorInfo: map[string]*DataTypeInfo{},
		Aliases:         map[string]*AliasInfo{},
		Classes:         map[string]*ClassInfo{className: cls},
		Values:          map[string]types.Type{methodName: types.Con("Int")},
		PromotedKinds:   map[string]types.Kind{},
		PromotedCons:    map[string]types.Kind{},
		TypeFamilies:    map[string]*TypeFamilyInfo{},
	}
}

// mkFamilyModule creates a ModuleExports that defines a type family.
func mkFamilyModule(famName string) *ModuleExports {
	return &ModuleExports{
		Types:           map[string]types.Kind{},
		ConTypes:        map[string]types.Type{},
		ConstructorInfo: map[string]*DataTypeInfo{},
		Aliases:         map[string]*AliasInfo{},
		Classes:         map[string]*ClassInfo{},
		Values:          map[string]types.Type{},
		PromotedKinds:   map[string]types.Kind{},
		PromotedCons:    map[string]types.Kind{},
		TypeFamilies: map[string]*TypeFamilyInfo{
			famName: {
				Name:       famName,
				Params:     []family.TFParam{{Name: "a", Kind: types.KType{}}},
				ResultKind: types.KType{},
			},
		},
	}
}

// TestCollision_TwoOpenImportsSameType — two modules both export a type
// named "Color". Open-importing both should trigger an ambiguity error.
func TestCollision_TwoOpenImportsSameType(t *testing.T) {
	modA := mkTypeModule("Color", []string{"RedA"})
	modB := mkTypeModule("Color", []string{"RedB"})
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
		ModuleDeps:      map[string][]string{"ModA": nil, "ModB": nil},
	}
	source := `
import ModA
import ModB

main := 42
`
	errMsg := checkSourceExpectCode(t, source, config, diagnostic.ErrImport)
	if !strings.Contains(errMsg, "ambiguous type name") || !strings.Contains(errMsg, "Color") {
		t.Errorf("expected type ambiguity error for Color, got: %s", errMsg)
	}
}

// TestCollision_TwoOpenImportsSameClass — two modules both export a class
// named "MyClass". Open-importing both should trigger an ambiguity error.
func TestCollision_TwoOpenImportsSameClass(t *testing.T) {
	modA := mkClassModule("MyClass", "methodA")
	modA.OwnedTypeNames = map[string]bool{"DummyA": true} // ensure ownership
	modA.OwnedNames = map[string]bool{"DummyA": true}
	modB := mkClassModule("MyClass", "methodB")
	modB.OwnedTypeNames = map[string]bool{"DummyB": true}
	modB.OwnedNames = map[string]bool{"DummyB": true}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
		ModuleDeps:      map[string][]string{"ModA": nil, "ModB": nil},
	}
	source := `
import ModA
import ModB

main := 42
`
	errMsg := checkSourceExpectCode(t, source, config, diagnostic.ErrImport)
	if !strings.Contains(errMsg, "ambiguous type name") || !strings.Contains(errMsg, "MyClass") {
		t.Errorf("expected type ambiguity error for MyClass, got: %s", errMsg)
	}
}

// TestCollision_TwoOpenImportsSameFamily — two modules both export a type
// family named "F". Open-importing both should trigger an ambiguity error.
func TestCollision_TwoOpenImportsSameFamily(t *testing.T) {
	modA := mkFamilyModule("F")
	modA.OwnedTypeNames = map[string]bool{"DummyA": true}
	modA.OwnedNames = map[string]bool{"DummyA": true}
	modB := mkFamilyModule("F")
	modB.OwnedTypeNames = map[string]bool{"DummyB": true}
	modB.OwnedNames = map[string]bool{"DummyB": true}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
		ModuleDeps:      map[string][]string{"ModA": nil, "ModB": nil},
	}
	source := `
import ModA
import ModB

main := 42
`
	errMsg := checkSourceExpectCode(t, source, config, diagnostic.ErrImport)
	if !strings.Contains(errMsg, "ambiguous type name") || !strings.Contains(errMsg, "\"F\"") {
		t.Errorf("expected type ambiguity error for F, got: %s", errMsg)
	}
}

// TestCollision_AmbiguousClassBlocksMethods_Open — when two modules export
// the same class name via open import, the class methods must also be blocked.
// Previously, methods bypassed the class-level ambiguity gate and remained
// in scope as orphaned values.
func TestCollision_AmbiguousClassBlocksMethods_Open(t *testing.T) {
	modA := mkClassModule("Show", "showA")
	modA.OwnedTypeNames = map[string]bool{"DummyA": true}
	modA.OwnedNames = map[string]bool{"DummyA": true}
	modB := mkClassModule("Show", "showB")
	modB.OwnedTypeNames = map[string]bool{"DummyB": true}
	modB.OwnedNames = map[string]bool{"DummyB": true}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
		ModuleDeps:      map[string][]string{"ModA": nil, "ModB": nil},
	}
	// showB is a method of the ambiguous class Show from ModB.
	// It should not be importable as a standalone value.
	source := `
import ModA
import ModB

main := showB
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "showB") {
		t.Errorf("expected error about showB (orphaned method of ambiguous class), got: %s", errMsg)
	}
}

// TestCollision_AmbiguousClassBlocksMethods_Selective — when a class name
// collides via selective import, methods of the second class must be blocked.
func TestCollision_AmbiguousClassBlocksMethods_Selective(t *testing.T) {
	modA := mkClassModule("Show", "showA")
	modA.OwnedTypeNames = map[string]bool{"DummyA": true}
	modA.OwnedNames = map[string]bool{"DummyA": true}
	modB := mkClassModule("Show", "showB")
	modB.OwnedTypeNames = map[string]bool{"DummyB": true}
	modB.OwnedNames = map[string]bool{"DummyB": true}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
		ModuleDeps:      map[string][]string{"ModA": nil, "ModB": nil},
	}
	source := `
import ModA (Show)
import ModB (Show)

main := showB
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "showB") {
		t.Errorf("expected error about showB (orphaned method of ambiguous class), got: %s", errMsg)
	}
}

// TestCollision_QualifiedDisambiguatesType — qualified import should
// not trigger ambiguity even if both modules export the same type name.
func TestCollision_QualifiedDisambiguatesType(t *testing.T) {
	modA := mkTypeModule("Color", []string{"Red"})
	modB := mkTypeModule("Color", []string{"Blue"})
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
		ModuleDeps:      map[string][]string{"ModA": nil, "ModB": nil},
	}
	source := `
import ModA as A
import ModB as B
data Bool := { True: (); False: (); }

f :: A.Color -> Bool
f := \x. case x { A.Red => True }

main := f A.Red
`
	checkSource(t, source, config)
}

// TestCollision_DiamondReexportType — A and B both re-export type Color
// from shared dependency C. This is NOT a conflict (diamond import).
func TestCollision_DiamondReexportType(t *testing.T) {
	modC := mkTypeModule("Color", []string{"Red", "Blue"})
	// ModA re-exports from C (Color is NOT in ModA's OwnedTypeNames).
	modA := &ModuleExports{
		Types:           map[string]types.Kind{"Color": types.KType{}},
		ConTypes:        map[string]types.Type{"Red": types.Con("Color")},
		ConstructorInfo: modC.ConstructorInfo,
		Aliases:         map[string]*AliasInfo{},
		Classes:         map[string]*ClassInfo{},
		Values:          map[string]types.Type{"helperA": types.Con("Int")},
		PromotedKinds:   map[string]types.Kind{},
		PromotedCons:    map[string]types.Kind{},
		TypeFamilies:    map[string]*TypeFamilyInfo{},
		ModuleOwnership: ModuleOwnership{
			OwnedTypeNames:     map[string]bool{"ModAOwn": true},
			OwnedNames:         map[string]bool{"ModAOwn": true},
			ConstructorsByType: map[string][]string{"Color": {"Red", "Blue"}},
		},
	}
	modB := &ModuleExports{
		Types:           map[string]types.Kind{"Color": types.KType{}},
		ConTypes:        map[string]types.Type{"Blue": types.Con("Color")},
		ConstructorInfo: modC.ConstructorInfo,
		Aliases:         map[string]*AliasInfo{},
		Classes:         map[string]*ClassInfo{},
		Values:          map[string]types.Type{"helperB": types.Con("Int")},
		PromotedKinds:   map[string]types.Kind{},
		PromotedCons:    map[string]types.Kind{},
		TypeFamilies:    map[string]*TypeFamilyInfo{},
		ModuleOwnership: ModuleOwnership{
			OwnedTypeNames:     map[string]bool{"ModBOwn": true},
			OwnedNames:         map[string]bool{"ModBOwn": true},
			ConstructorsByType: map[string][]string{"Color": {"Red", "Blue"}},
		},
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{
			"ModC": modC, "ModA": modA, "ModB": modB,
		},
		ModuleDeps: map[string][]string{
			"ModC": nil,
			"ModA": {"ModC"},
			"ModB": {"ModC"},
		},
	}
	source := `
import ModA
import ModB

main := 42
`
	// Should NOT error — Color is re-exported from C by both A and B.
	checkSource(t, source, config)
}

// TestCollision_SelectiveImportTypeCollision — selective import of the
// same type name from two modules should trigger an ambiguity error.
func TestCollision_SelectiveImportTypeCollision(t *testing.T) {
	modA := mkTypeModule("Color", []string{"Red"})
	modB := mkTypeModule("Color", []string{"Blue"})
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
		ModuleDeps:      map[string][]string{"ModA": nil, "ModB": nil},
	}
	source := `
import ModA (Color(..))
import ModB (Color(..))

main := 42
`
	errMsg := checkSourceExpectCode(t, source, config, diagnostic.ErrImport)
	if !strings.Contains(errMsg, "ambiguous type name") || !strings.Contains(errMsg, "Color") {
		t.Errorf("expected type ambiguity error for Color, got: %s", errMsg)
	}
}
