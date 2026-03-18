//go:build probe

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
	"github.com/cwd-k2/gicel/internal/types"
)

// =============================================================================
// Module probe tests — qualified patterns, ambiguity detection, qualified
// variable/constructor lookup, unknown qualifier errors, selective imports,
// private name access, and module-scoped resolution.
// =============================================================================

// probeModuleExports creates a minimal ModuleExports for a module that
// defines a data type with the given constructors.
func probeModuleExports(typeName string, cons []string) *ModuleExports {
	info := &DataTypeInfo{
		Name:         typeName,
		Constructors: make([]ConInfo, len(cons)),
	}
	for i, c := range cons {
		info.Constructors[i] = ConInfo{Name: c, Arity: 0}
	}
	conTypes := make(map[string]types.Type, len(cons))
	conInfo := make(map[string]*DataTypeInfo, len(cons))
	for _, c := range cons {
		conTypes[c] = types.Con(typeName)
		conInfo[c] = info
	}
	conDecls := make([]core.ConDecl, len(cons))
	for i, c := range cons {
		conDecls[i] = core.ConDecl{Name: c}
	}
	return &ModuleExports{
		Types:    map[string]types.Kind{typeName: types.KType{}},
		ConTypes: conTypes,
		ConInfo:  conInfo,
		Aliases:  map[string]*AliasInfo{},
		Classes:  map[string]*ClassInfo{},
		Values:   map[string]types.Type{},
		DataDecls: []core.DataDecl{
			{
				Name: typeName,
				Cons: conDecls,
			},
		},
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
}

// makeModuleConfig creates a CheckConfig with one pre-compiled module.
func makeModuleConfig(t *testing.T, moduleName, moduleSource string) *CheckConfig {
	t.Helper()
	src := span.NewSource(moduleName, moduleSource)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatalf("lex errors in module %s: %s", moduleName, lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatalf("parse errors in module %s: %s", moduleName, es.Format())
	}
	modConfig := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	_, exports, checkErrs := CheckModule(ast, src, modConfig)
	if checkErrs.HasErrors() {
		t.Fatalf("check errors in module %s: %s", moduleName, checkErrs.Format())
	}
	return &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{moduleName: exports},
	}
}

// =====================================================================
// From probe_a: Qualified patterns
// =====================================================================

// TestProbeA_QualPattern_WrongQualifier — using a qualifier that doesn't
// correspond to any imported module should produce an error.
func TestProbeA_QualPattern_WrongQualifier(t *testing.T) {
	modExports := probeModuleExports("Color", []string{"Red", "Blue"})
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"MyMod": modExports},
	}
	source := `
import MyMod as M
data Bool := True | False

-- Use a wrong qualifier "X" (not imported).
f := \x. case x { X.Red -> True; M.Blue -> False }
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "unknown qualifier") && !strings.Contains(errMsg, "X") {
		t.Errorf("expected error about unknown qualifier X, got: %s", errMsg)
	}
}

// TestProbeA_QualPattern_NonExportedConstructor — a qualified pattern referencing
// a constructor not exported by the module.
func TestProbeA_QualPattern_NonExportedConstructor(t *testing.T) {
	// Module only exports Red, not Blue.
	modExports := probeModuleExports("Color", []string{"Red"})
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"MyMod": modExports},
	}
	source := `
import MyMod as M
data Bool := True | False

-- Blue is NOT exported by MyMod.
f := \x. case x { M.Red -> True; M.Blue -> False }
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "Blue") {
		t.Errorf("expected error mentioning Blue, got: %s", errMsg)
	}
}

// TestProbeA_QualPattern_ValidQualPattern — a qualified pattern that should succeed.
func TestProbeA_QualPattern_ValidQualPattern(t *testing.T) {
	modExports := probeModuleExports("Color", []string{"Red", "Blue"})
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"MyMod": modExports},
	}
	// Use the Color type without qualified type reference (M.Color is only
	// valid for types with isModuleDefinedType, which requires DataDecls).
	// Since Color is a qualified-imported type, we reference it via M.Color.
	source := `
import MyMod as M
data Bool := True | False

f :: M.Color -> Bool
f := \x. case x { M.Red -> True; M.Blue -> False }

main := f M.Red
`
	checkSource(t, source, config)
}

// TestProbeA_QualPattern_NestedQualPattern — qualified constructor with
// arguments in a pattern.
func TestProbeA_QualPattern_NestedQualPattern(t *testing.T) {
	// Module exports Maybe with Just and Nothing.
	info := &DataTypeInfo{
		Name:         "Maybe",
		Constructors: []ConInfo{{Name: "Just", Arity: 1}, {Name: "Nothing", Arity: 0}},
	}
	modExports := &ModuleExports{
		Types: map[string]types.Kind{
			"Maybe": &types.KArrow{From: types.KType{}, To: types.KType{}},
		},
		ConTypes: map[string]types.Type{
			"Just": &types.TyForall{
				Var: "a", Kind: types.KType{},
				Body: types.MkArrow(&types.TyVar{Name: "a"}, &types.TyApp{
					Fun: types.Con("Maybe"),
					Arg: &types.TyVar{Name: "a"},
				}),
			},
			"Nothing": &types.TyForall{
				Var: "a", Kind: types.KType{},
				Body: &types.TyApp{
					Fun: types.Con("Maybe"),
					Arg: &types.TyVar{Name: "a"},
				},
			},
		},
		ConInfo: map[string]*DataTypeInfo{
			"Just":    info,
			"Nothing": info,
		},
		Aliases: map[string]*AliasInfo{},
		Classes: map[string]*ClassInfo{},
		Values:  map[string]types.Type{},
		DataDecls: []core.DataDecl{
			{
				Name:     "Maybe",
				TyParams: []core.TyParam{{Name: "a", Kind: types.KType{}}},
				Cons: []core.ConDecl{
					{Name: "Just", Fields: []types.Type{&types.TyVar{Name: "a"}}},
					{Name: "Nothing"},
				},
			},
		},
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"MaybeLib": modExports},
	}
	source := `
import MaybeLib as ML
data Bool := True | False

f :: ML.Maybe Bool -> Bool
f := \x. case x { ML.Just b -> b; ML.Nothing -> False }

main := f (ML.Just True)
`
	checkSource(t, source, config)
}

// =====================================================================
// From probe_a: Ambiguity detection
// =====================================================================

// TestProbeA_Ambiguity_TwoOpenImportsSameName — two modules both export
// a value named "foo". Open-importing both should trigger an ambiguity error.
func TestProbeA_Ambiguity_TwoOpenImportsSameName(t *testing.T) {
	modA := &ModuleExports{
		Types:    map[string]types.Kind{},
		ConTypes: map[string]types.Type{},
		ConInfo:  map[string]*DataTypeInfo{},
		Aliases:  map[string]*AliasInfo{},
		Classes:  map[string]*ClassInfo{},
		Values:   map[string]types.Type{"foo": types.Con("Int")},
		DataDecls: []core.DataDecl{
			{Name: "FakeA", Cons: []core.ConDecl{}},
		},
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	modB := &ModuleExports{
		Types:    map[string]types.Kind{},
		ConTypes: map[string]types.Type{},
		ConInfo:  map[string]*DataTypeInfo{},
		Aliases:  map[string]*AliasInfo{},
		Classes:  map[string]*ClassInfo{},
		Values:   map[string]types.Type{"foo": types.Con("String")},
		DataDecls: []core.DataDecl{
			{Name: "FakeB", Cons: []core.ConDecl{}},
		},
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"String": types.KType{},
		},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
	}
	source := `
import ModA
import ModB

main := foo
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "ambiguous") {
		t.Errorf("expected ambiguity error, got: %s", errMsg)
	}
}

// TestProbeA_Ambiguity_TwoOpenImportsSameConstructor — two modules both
// export a constructor named "MkVal". Should trigger ambiguity.
// BUG: medium — Constructor ambiguity is not detected when two modules
// export constructors with the same name via open import. The
// checkAmbiguousName function is called for ConTypes during importOpen,
// but the moduleOwnsName check looks for constructors in DataDecls.
// If DataDecls are populated (which they should be for real modules),
// the ambiguity IS detected. However, this test reveals that constructor
// names pushed to the context via CtxVar are silently overwritten when
// the second import pushes the same name — the last import wins.
// The ambiguity detection relies on moduleOwnsName, which correctly
// identifies ownership when DataDecls are present. With DataDecls, this
// test passes. The root issue is that incomplete ModuleExports (missing
// DataDecls) bypass the ownership check.
func TestProbeA_Ambiguity_TwoOpenImportsSameConstructor(t *testing.T) {
	makeModWithCon := func(typeName string) *ModuleExports {
		info := &DataTypeInfo{
			Name: typeName,
			Constructors: []ConInfo{
				{Name: "MkVal", Arity: 0},
			},
		}
		return &ModuleExports{
			Types:    map[string]types.Kind{typeName: types.KType{}},
			ConTypes: map[string]types.Type{"MkVal": types.Con(typeName)},
			ConInfo:  map[string]*DataTypeInfo{"MkVal": info},
			Aliases:  map[string]*AliasInfo{},
			Classes:  map[string]*ClassInfo{},
			Values:   map[string]types.Type{},
			DataDecls: []core.DataDecl{
				{Name: typeName, Cons: []core.ConDecl{{Name: "MkVal"}}},
			},
			PromotedKinds: map[string]types.Kind{},
			PromotedCons:  map[string]types.Kind{},
			TypeFamilies:  map[string]*TypeFamilyInfo{},
			Instances:     nil,
		}
	}
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{
			"ModA": makeModWithCon("TypeA"),
			"ModB": makeModWithCon("TypeB"),
		},
	}
	source := `
import ModA
import ModB

main := MkVal
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "ambiguous") {
		t.Errorf("expected ambiguity error for constructor MkVal, got: %s", errMsg)
	}
}

// TestProbeA_Ambiguity_QualifiedDisambiguates — qualified import should
// not trigger ambiguity even if both modules export the same name.
func TestProbeA_Ambiguity_QualifiedDisambiguates(t *testing.T) {
	mkMod := func() *ModuleExports {
		return &ModuleExports{
			Types:         map[string]types.Kind{},
			ConTypes:      map[string]types.Type{},
			ConInfo:       map[string]*DataTypeInfo{},
			Aliases:       map[string]*AliasInfo{},
			Classes:       map[string]*ClassInfo{},
			Values:        map[string]types.Type{"foo": types.Con("Int")},
			DataDecls:     nil,
			PromotedKinds: map[string]types.Kind{},
			PromotedCons:  map[string]types.Kind{},
			TypeFamilies:  map[string]*TypeFamilyInfo{},
			Instances:     nil,
		}
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{
			"ModA": mkMod(),
			"ModB": mkMod(),
		},
	}
	// Both imported qualified — no ambiguity; use A.foo.
	source := `
import ModA as A
import ModB as B

main := A.foo
`
	checkSource(t, source, config)
}

// =====================================================================
// From probe_d: Module-scoped resolution
// =====================================================================

// TestProbeD_Module_QualifiedVarLookup — qualified variable lookup
// from an imported module.
func TestProbeD_Module_QualifiedVarLookup(t *testing.T) {
	modExports := &ModuleExports{
		Types:         map[string]types.Kind{"Int": types.KType{}},
		ConTypes:      map[string]types.Type{},
		ConInfo:       map[string]*DataTypeInfo{},
		Aliases:       map[string]*AliasInfo{},
		Classes:       map[string]*ClassInfo{},
		Values:        map[string]types.Type{"add": types.MkArrow(types.Con("Int"), types.MkArrow(types.Con("Int"), types.Con("Int")))},
		DataDecls:     nil,
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"MathLib": modExports},
	}
	source := `
import MathLib as M

main := M.add 1 2
`
	checkSource(t, source, config)
}

// TestProbeD_Module_QualifiedConLookup — qualified constructor lookup.
func TestProbeD_Module_QualifiedConLookup(t *testing.T) {
	modExports := probeModuleExports("Color", []string{"Red", "Blue"})
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"Colors": modExports},
	}
	source := `
import Colors as C
data Bool := True | False

f :: C.Color -> Bool
f := \x. case x { C.Red -> True; C.Blue -> False }

main := f C.Red
`
	checkSource(t, source, config)
}

// TestProbeD_Module_UnknownQualifierError — referencing a qualifier not
// in scope should produce a clear error.
func TestProbeD_Module_UnknownQualifierError(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	source := `
main := X.foo
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "unknown qualifier") && !strings.Contains(errMsg, "X") {
		t.Errorf("expected error about unknown qualifier X, got: %s", errMsg)
	}
}

// TestProbeD_Module_QualifiedShadowLocal — a local binding should NOT
// shadow a qualified import (qualified names are always explicit).
func TestProbeD_Module_QualifiedShadowLocal(t *testing.T) {
	modExports := &ModuleExports{
		Types:         map[string]types.Kind{"Int": types.KType{}},
		ConTypes:      map[string]types.Type{},
		ConInfo:       map[string]*DataTypeInfo{},
		Aliases:       map[string]*AliasInfo{},
		Classes:       map[string]*ClassInfo{},
		Values:        map[string]types.Type{"val": types.Con("Int")},
		DataDecls:     nil,
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}, "Bool": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"Lib": modExports},
	}
	source := `
import Lib as L

-- Local "val" of different type
data Bool := True | False
val :: Bool
val := True

-- L.val should still refer to the module's Int-typed val, not local Bool-typed val
main := L.val
`
	checkSource(t, source, config)
}

// =====================================================================
// From probe_e: Module import edge cases
// =====================================================================

// TestProbeE_Module_SelectiveImportNonExistent — importing a name that
// doesn't exist in the module should produce a clear error.
func TestProbeE_Module_SelectiveImportNonExistent(t *testing.T) {
	// This requires setting up a module first
	source := `
data Bool := True | False
main := True
`
	// Register a module, then try to selectively import a nonexistent name
	config := makeModuleConfig(t, "Lib", `
data Color := Red | Blue
`)
	errSource := `
import Lib (nonexistent)
main := 42
`
	errMsg := checkSourceExpectError(t, errSource, config)
	if !strings.Contains(errMsg, "does not export") && !strings.Contains(errMsg, "nonexistent") {
		t.Logf("NOTICE: selective import error: %s", errMsg)
	}
	_ = source // suppress unused
}

// TestProbeE_Module_QualifiedAccessToPrivateName — qualified access to a
// private name (underscore prefix) should fail.
func TestProbeE_Module_QualifiedAccessToPrivateName(t *testing.T) {
	config := makeModuleConfig(t, "Lib", `
_private := 42
data Bool := True | False
public := True
`)
	errSource := `
import Lib as L
main := L._private
`
	// Should fail because _private is not exported
	errMsg := checkSourceExpectError(t, errSource, config)
	if !strings.Contains(errMsg, "does not export") && !strings.Contains(errMsg, "_private") {
		t.Logf("NOTICE: private access error: %s", errMsg)
	}
}

// TestProbeE_Module_UnknownModule — importing a module that doesn't exist
// should produce a clear error.
func TestProbeE_Module_UnknownModule(t *testing.T) {
	errMsg := checkSourceExpectError(t, `
import NonExistent
main := 42
`, nil)
	if !strings.Contains(errMsg, "unknown module") {
		t.Logf("NOTICE: unknown module error: %s", errMsg)
	}
}
