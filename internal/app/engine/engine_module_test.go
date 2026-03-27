package engine

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// --- Module system integration tests ---

func TestRegisterModule(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Bool := { True: Bool; False: Bool; }
not :: Bool -> Bool
not := \b. case b { True => False; False => True }
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestImportModuleTypes(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Bool := { True: Bool; False: Bool; }
not :: Bool -> Bool
not := \b. case b { True => False; False => True }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Lib
main := not True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "False" {
		t.Errorf("expected False, got %s", result.Value)
	}
}

func TestImportModuleInstances(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("EqLib", `
form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import EqLib
main := eq True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestRegisterModuleFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/Lib.gicel"
	if err := os.WriteFile(path, []byte(`
form Color := { Red: Color; Blue: Color; }
`), 0644); err != nil {
		t.Fatal(err)
	}
	eng := NewEngine()
	if err := eng.RegisterModuleFile(path); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Lib
main := Red
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Red" {
		t.Errorf("expected Red, got %s", result.Value)
	}
}

func TestRegisterModuleFileMissing(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModuleFile("/nonexistent/path/Foo.gicel")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestCircularImportError(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	// Register module A that imports B.
	err := eng.RegisterModule("A", `
import B
form Unit := { Unit: Unit; }
`)
	// Should fail because B is not registered.
	if err == nil {
		// Now try registering B that imports A → circular.
		err = eng.RegisterModule("B", `
import A
form Void := MkVoid
`)
		if err == nil {
			t.Error("expected circular import error")
		}
	}
	// If A failed because B doesn't exist, that's also acceptable.
	if err != nil && !strings.Contains(err.Error(), "unknown module") && !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected import error, got: %s", err)
	}
}

func TestImportNameCollision(t *testing.T) {
	eng := NewEngine()
	// Module A defines Bool.
	err := eng.RegisterModule("A", `form Bool := { True: Bool; False: Bool; }`)
	if err != nil {
		t.Fatal(err)
	}
	// Module B also defines Bool.
	err = eng.RegisterModule("B", `form Bool := { Yes: Bool; No: Bool; }`)
	if err != nil {
		t.Fatal(err)
	}
	// Importing both should succeed at parse time but may produce
	// ambiguity at type check time. At minimum, the program should not panic.
	_, err = eng.NewRuntime(context.Background(), `
import A
import B
main := True
`)
	// This may or may not error depending on resolution order.
	// The important thing is no panic.
	_ = err
}

func TestPackModuleImport(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
v1 := 1 + 2
v2 := strlen "hello"
main := Just (v1 + v2)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %s", result.Value)
	}
	if hv := MustHost[int64](con.Args[0]); hv != 8 {
		t.Errorf("expected 8, got %d", hv)
	}
}

func TestModuleAccumulation(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("A", `
form Bool := { True: Bool; False: Bool; }
`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("B", `
import A
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import A
import B
main := MkPair True False
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Value.(*eval.ConVal); !ok {
		t.Fatalf("expected ConVal, got %T", result.Value)
	}
}

func TestInvalidModuleSource(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	err := eng.RegisterModule("Bad", `this is not valid syntax @@@@`)
	if err == nil {
		t.Fatal("expected error for invalid module source")
	}
}

// --- Module System Extension: Selective & Qualified Imports ---

func TestSelectiveImport(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Color := { Red: Color; Green: Color; Blue: Color; }
red :: Color
red := Red
green :: Color
green := Green
`)
	if err != nil {
		t.Fatal(err)
	}
	// Selective: only import Red and red
	rt, err := eng.NewRuntime(context.Background(), `
import Lib (red, Color(Red))
main := red
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Red" {
		t.Errorf("expected Red, got %s", result.Value)
	}
}

func TestSelectiveImportRejectsUnlisted(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Color := { Red: Color; Green: Color; Blue: Color; }
red :: Color
red := Red
green :: Color
green := Green
`)
	if err != nil {
		t.Fatal(err)
	}
	// Selective import doesn't include 'green'
	_, err = eng.NewRuntime(context.Background(), `
import Lib (red)
main := green
`)
	if err == nil {
		t.Fatal("expected error: green should not be in scope")
	}
}

func TestQualifiedImport(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Color := { Red: Color; Green: Color; Blue: Color; }
red :: Color
red := Red
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Lib as L
main := L.red
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Red" {
		t.Errorf("expected Red, got %s", result.Value)
	}
}

func TestQualifiedImportConstructor(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Color := { Red: Color; Green: Color; Blue: Color; }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Lib as L
main := L.Green
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Green" {
		t.Errorf("expected Green, got %s", result.Value)
	}
}

func TestQualifiedNotInUnqualifiedScope(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Color := { Red: Color; Green: Color; Blue: Color; }
red :: Color
red := Red
`)
	if err != nil {
		t.Fatal(err)
	}
	// Qualified import should NOT bring names into unqualified scope
	_, err = eng.NewRuntime(context.Background(), `
import Lib as L
main := red
`)
	if err == nil {
		t.Fatal("expected error: 'red' should not be in unqualified scope with qualified import")
	}
}

func TestDuplicateImportError(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Color := { Red: Color; Green: Color; Blue: Color; }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.NewRuntime(context.Background(), `
import Lib
import Lib
main := Red
`)
	if err == nil {
		t.Fatal("expected error for duplicate import")
	}
}

func TestAliasCollisionError(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("LibA", `
form X := MkX
`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("LibB", `
form Y := MkY
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.NewRuntime(context.Background(), `
import LibA as L
import LibB as L
main := L.MkX
`)
	if err == nil {
		t.Fatal("expected error for alias collision")
	}
}

func TestSelectiveImportAllSubs(t *testing.T) {
	eng := NewEngine()
	err := eng.RegisterModule("Lib", `
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Lib (Maybe(..))
main := Just 42
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Just" {
		t.Errorf("expected Just, got %s", result.Value)
	}
}

func TestCoreSelectiveImportRejected(t *testing.T) {
	eng := NewEngine()
	_, err := eng.NewRuntime(context.Background(), `
import Core (pure)
main := pure 42
`)
	if err == nil {
		t.Fatal("expected error: selective import of Core should be rejected")
	}
}

func TestCoreQualifiedImportRejected(t *testing.T) {
	eng := NewEngine()
	_, err := eng.NewRuntime(context.Background(), `
import Core as C
main := C.pure 42
`)
	if err == nil {
		t.Fatal("expected error: qualified import of Core should be rejected")
	}
}

// --- Exhaustive Module System Extension Tests ---

// Helper: register a standard set of test modules on an engine.
func setupTestModules(t *testing.T, eng *Engine) {
	t.Helper()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("Geometry", `
form Point := { MkPoint: Int -> Int -> Point; }
mkPoint :: Int -> Int -> Point
mkPoint := \x y. MkPoint x y
getX :: Point -> Int
getX := \p. case p { MkPoint x _ => x }
getY :: Point -> Int
getY := \p. case p { MkPoint _ y => y }
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("Color", `
import Prelude
form Color := { Red: Color; Green: Color; Blue: Color; }
name :: Color -> String
name := \c. case c { Red => "red"; Green => "green"; Blue => "blue" }
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("MathLib", `
import Prelude
double :: Int -> Int
double := \x. x + x
square :: Int -> Int
square := \x. x * x
`); err != nil {
		t.Fatal(err)
	}
}

func runExpectHost(t *testing.T, eng *Engine, source string, expected any) {
	t.Helper()
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %s", result.Value, result.Value)
	}
	if hv.Inner != expected {
		t.Errorf("expected %v, got %v", expected, hv.Inner)
	}
}

func runExpectCon(t *testing.T, eng *Engine, source string, expectedCon string) {
	t.Helper()
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T: %s", result.Value, result.Value)
	}
	if con.Con != expectedCon {
		t.Errorf("expected %s, got %s", expectedCon, con.Con)
	}
}

func runExpectError(t *testing.T, eng *Engine, source string, wantSubstr string) {
	t.Helper()
	_, err := eng.NewRuntime(context.Background(), source)
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}
	if wantSubstr != "" && !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSubstr)
	}
}

// === Open Import Tests ===

func TestOpenImportAllNames(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectHost(t, eng, `
import Geometry
main := getX (mkPoint 10 20)
`, int64(10))
}

func TestOpenImportAllConstructors(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectCon(t, eng, `
import Color
main := Blue
`, "Blue")
}

// === Selective Import Tests ===

func TestSelectiveImportValue(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectHost(t, eng, `
import Prelude
import MathLib (double)
main := double 21
`, int64(42))
}

func TestSelectiveImportValueRejectsOther(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import Prelude
import MathLib (double)
main := square 5
`, "unbound variable: square")
}

func TestSelectiveImportTypeBare(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	// Import type without constructors — can use in annotation but not construct
	runExpectError(t, eng, `
import Geometry (Point)
main := MkPoint 1 2
`, "unknown constructor: MkPoint")
}

func TestSelectiveImportTypeWithAllSubs(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectCon(t, eng, `
import Color (Color(..))
main := Green
`, "Green")
}

func TestSelectiveImportTypeWithSpecificSubs(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectCon(t, eng, `
import Color (Color(Red, Blue))
main := Red
`, "Red")
}

func TestSelectiveImportTypeRejectsUnlistedSub(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import Color (Color(Red))
main := Green
`, "unknown constructor: Green")
}

func TestSelectiveImportOperator(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	// Prelude exports (+) operator
	runExpectHost(t, eng, `
import Prelude ((+))
main := 1 + 2
`, int64(3))
}

func TestSelectiveImportMultipleNames(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectHost(t, eng, `
import Prelude
import MathLib (double, square)
main := double 3 + square 4
`, int64(22))
}

func TestSelectiveImportNonExistentName(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import MathLib (nonexistent)
main := 42
`, "does not export: nonexistent")
}

// === Qualified Import Tests ===

func TestQualifiedImportValue(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectHost(t, eng, `
import Prelude
import MathLib as M
main := M.double 21
`, int64(42))
}

func TestQualifiedImportConstructor2(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectHost(t, eng, `
import Geometry as G
main := G.getX (G.mkPoint 7 8)
`, int64(7))
}

func TestQualifiedImportConPattern(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	// Use qualified constructor in expressions
	runExpectCon(t, eng, `
import Color as C
main := C.Red
`, "Red")
}

func TestQualifiedDoesNotPolluteUnqualified(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import MathLib as M
main := double 5
`, "unbound variable: double")
}

func TestQualifiedUnknownQualifier(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import Prelude
main := X.something
`, "unknown qualifier: X")
}

func TestQualifiedNonExportedValue(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import Prelude
import MathLib as M
main := M.cube 3
`, "does not export value: cube")
}

func TestQualifiedNonExportedConstructor(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import Geometry as G
main := G.FakeConstructor
`, "does not export constructor: FakeConstructor")
}

// === Qualified Type in Type Annotations ===

func TestQualifiedTypeInAnnotation(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	// Qualified type in annotation; use qualified functions (no qualified patterns yet)
	runExpectHost(t, eng, `
import Geometry as G
extractX :: G.Point -> Int
extractX := \p. G.getX p
main := extractX (G.mkPoint 99 0)
`, int64(99))
}

// === Mixing Import Forms ===

func TestMixedImportForms(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectHost(t, eng, `
import Prelude
import Geometry
import Color (Color(..), name)
import MathLib as M
point := mkPoint 5 12
colorName := name Red
doubled := M.double (getX point)
main := doubled
`, int64(10))
}

// === Error Cases ===

func TestDuplicateImportSameForm(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import Geometry
import Geometry
main := mkPoint 1 2
`, "duplicate import: Geometry")
}

func TestDuplicateImportDifferentForms(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	// Open then qualified: still a duplicate
	runExpectError(t, eng, `
import Geometry
import Geometry as G
main := mkPoint 1 2
`, "duplicate import: Geometry")
}

func TestAliasCollisionDifferentModules(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	runExpectError(t, eng, `
import Geometry as M
import MathLib as M
main := M.double 5
`, "alias M already used for module Geometry")
}

func TestUnknownModuleImport(t *testing.T) {
	eng := NewEngine()
	runExpectError(t, eng, `
import NonExistent
main := 42
`, "unknown module: NonExistent")
}

// === Instance Coherence ===

func TestSelectiveImportAlwaysImportsInstances(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	err := eng.RegisterModule("EqLib", `
form MyBool := { MyTrue: MyBool; MyFalse: MyBool; }
form MyEq := \a. { myEq: a -> a -> MyBool }
impl MyEq MyBool := { myEq := \_ _. MyTrue }
`)
	if err != nil {
		t.Fatal(err)
	}
	// Selective import of class + type, instances should be implicitly imported
	rt, err := eng.NewRuntime(context.Background(), `
import EqLib (MyEq(..), MyBool(..))
main := myEq MyTrue MyFalse
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "MyTrue" {
		t.Errorf("expected MyTrue (instance should be imported), got %s", result.Value)
	}
}

func TestQualifiedImportAlwaysImportsInstances(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	err := eng.RegisterModule("EqLib", `
form MyBool := { MyTrue: MyBool; MyFalse: MyBool; }
form MyEq := \a. { myEq: a -> a -> MyBool }
impl MyEq MyBool := { myEq := \_ _. MyTrue }
`)
	if err != nil {
		t.Fatal(err)
	}
	// Qualified import: instances are still available for constraint resolution
	// (even though names need qualification)
	rt, err := eng.NewRuntime(context.Background(), `
import EqLib as E
main := E.myEq E.MyTrue E.MyFalse
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "MyTrue" {
		t.Errorf("expected MyTrue (instance should be imported), got %s", result.Value)
	}
}

// === Adjacency Disambiguation ===

func TestDotCompositionNotQualified(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	// N . f with spaces should be function composition, not qualified access
	runExpectHost(t, eng, `
import Prelude
f :: Int -> Int
f := \x. x + 1
g :: Int -> Int
g := \x. x * 2
main := (f . g) 3
`, int64(7))
}

// === Multi-Module End-to-End ===

func TestMultiModuleEndToEnd(t *testing.T) {
	eng := NewEngine()
	setupTestModules(t, eng)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Geometry
import Color (Color(..), name)
import MathLib as M
point := mkPoint 3 4
colorName := name Red
doubled := M.double (getX point)
main := (getX point, colorName, doubled)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Should be a record (tuple): { _1: 3, _2: "red", _3: 6 }
	rec, ok := result.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %s", result.Value, result.Value)
	}
	f1, ok := rec.Get("_1")
	if !ok {
		t.Fatal("missing field _1")
	}
	if hv, ok := f1.(*eval.HostVal); !ok || hv.Inner != int64(3) {
		t.Errorf("_1: expected 3, got %s", f1)
	}
	f2, ok := rec.Get("_2")
	if !ok {
		t.Fatal("missing field _2")
	}
	if hv, ok := f2.(*eval.HostVal); !ok || hv.Inner != "red" {
		t.Errorf("_2: expected \"red\", got %s", f2)
	}
	f3, ok := rec.Get("_3")
	if !ok {
		t.Fatal("missing field _3")
	}
	if hv, ok := f3.(*eval.HostVal); !ok || hv.Inner != int64(6) {
		t.Errorf("_3: expected 6, got %s", f3)
	}
}

// === Same-Name Disambiguation via Qualified Import ===

func TestQualifiedDisambiguatesSameNames(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	// Two modules export "value", qualified import distinguishes them
	err := eng.RegisterModule("ModA", `
import Prelude
value :: Int
value := 10
`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("ModB", `
import Prelude
value :: Int
value := 20
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import ModA as A
import ModB as B
main := A.value + B.value
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv, ok := result.Value.(*eval.HostVal)
	if !ok || hv.Inner != int64(30) {
		t.Errorf("expected 30, got %s", result.Value)
	}
}

// === Qualified Constraint Tests ===

func TestQualifiedConstraintSingle(t *testing.T) {
	// P.Num a => ... should resolve the class via qualified scope.
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	runExpectHost(t, eng, `
import Prelude as P
f :: P.Num a => a -> a
f := \x. P.add x x
main := f 21
`, int64(42))
}

func TestQualifiedConstraintTuple(t *testing.T) {
	// (P.Eq a, P.Num a) => ... should resolve multiple qualified constraints.
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	runExpectCon(t, eng, `
import Prelude as P
f :: (P.Eq a, P.Num a) => a -> P.Bool
f := \x. P.eq x (P.add x x)
main := f 0
`, "True")
}

func TestQualifiedConstraintUserClass(t *testing.T) {
	// Qualified constraint on a user-defined class in another module.
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	err := eng.RegisterModule("MyLib", `
import Prelude
form MyClass := \a. { myMethod: a -> Int }
impl MyClass Int := { myMethod := \x. x + 1 }
`)
	if err != nil {
		t.Fatal(err)
	}
	runExpectHost(t, eng, `
import Prelude
import MyLib as M
f :: M.MyClass a => a -> Int
f := \x. M.myMethod x
main := f 41
`, int64(42))
}

// TestModuleExplainSourceName verifies that explain trace events
// from module code carry the correct SourceName when ExplainAll is enabled.
// Module closures are marked internal (suppressed by default), so ExplainAll
// is required to observe events from inside module function bodies.
func TestModuleExplainSourceName(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	err := eng.RegisterModule("Logic", `
import Prelude
form Answer := { Yes: Answer; No: Answer; }
decide :: Answer -> Int
decide := \a. case a { Yes => 1; No => 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Logic
main := decide Yes
`)
	if err != nil {
		t.Fatal(err)
	}

	var steps []eval.ExplainStep
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Explain:      func(s eval.ExplainStep) { steps = append(steps, s) },
		ExplainDepth: ExplainAll,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The case match inside decide's body should carry SourceName "Logic".
	var foundLogicMatch bool
	for _, s := range steps {
		if s.SourceName == "Logic" && s.Kind == eval.ExplainMatch {
			foundLogicMatch = true
			break
		}
	}
	if !foundLogicMatch {
		names := make(map[string]int)
		for _, s := range steps {
			if s.SourceName != "" {
				names[s.SourceName]++
			}
		}
		t.Errorf("no ExplainMatch with SourceName=\"Logic\"; source names seen: %v", names)
	}
}
