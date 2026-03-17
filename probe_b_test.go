package gicel_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// =============================================================================
// Helper functions for probe tests
// =============================================================================

func probeMustCompile(t *testing.T, eng *gicel.Engine, source string) *gicel.Runtime {
	t.Helper()
	rt, err := eng.NewRuntime(source)
	if err != nil {
		t.Fatalf("unexpected compile error: %v", err)
	}
	return rt
}

func probeExpectCompileError(t *testing.T, eng *gicel.Engine, source string, wantSubstr string) {
	t.Helper()
	_, err := eng.NewRuntime(source)
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}
	if wantSubstr != "" && !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSubstr)
	}
}

func probeRunExpectHost(t *testing.T, eng *gicel.Engine, source string, expected any) {
	t.Helper()
	rt := probeMustCompile(t, eng, source)
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %s", result.Value, result.Value)
	}
	if hv.Inner != expected {
		t.Errorf("expected %v, got %v", expected, hv.Inner)
	}
}

func probeRunExpectCon(t *testing.T, eng *gicel.Engine, source string, expectedCon string) {
	t.Helper()
	rt := probeMustCompile(t, eng, source)
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T: %s", result.Value, result.Value)
	}
	if con.Con != expectedCon {
		t.Errorf("expected %s, got %s", expectedCon, con.Con)
	}
}

// =============================================================================
// Selective Import Semantics
// =============================================================================

func TestProbeB_SelectiveImportClassBareImportsAllMethods(t *testing.T) {
	// Import a class by bare name (no sub-list) -- all methods should be imported.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("ClassLib", `
import Prelude
class MyClass a { myMethod :: a -> a }
instance MyClass Int { myMethod := \x. x + 1 }
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import ClassLib (MyClass)
main := myMethod 41
`, int64(42))
}

// BUG: Severity=Medium. The parser's parseImportName uses expectUpper for sub-list
// entries (line 232 of parser.go), but class methods are lowercase. This means
// selective import of specific class methods (e.g., Class(methodA)) is impossible.
// The sub-list syntax only supports constructor-like (uppercase) names.
// Workaround: use bare class name (imports all methods) or (..) syntax.
func TestProbeB_SelectiveImportClassMethodSubListParseFail(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("ClassLib2", `
import Prelude
class TwoMethods a {
  methodA :: a -> a;
  methodB :: a -> a
}
instance TwoMethods Int {
  methodA := \x. x + 1;
  methodB := \x. x + 2
}
`)
	if err != nil {
		t.Fatal(err)
	}

	// This SHOULD work but FAILS because the parser expects Upper for sub-list items.
	// TwoMethods(MethodA) is parsed but MethodA doesn't match the actual method name "methodA".
	_, compErr := eng.NewRuntime(`
import Prelude
import ClassLib2 (TwoMethods(MethodA))
main := methodA 41
`)
	if compErr == nil {
		t.Fatal("expected parse/compile error due to MethodA not matching lowercase methodA")
	}
	// BUG: The parser only accepts uppercase names in sub-lists, making it impossible
	// to selectively import specific class methods.
	t.Logf("confirmed BUG: class method sub-list cannot use lowercase names: %v", compErr)
}

func TestProbeB_SelectiveImportClassMethodBRejectedViaUpperName(t *testing.T) {
	// With the parser limitation, TwoMethods(MethodA) silently imports nothing
	// because "MethodA" (uppercase) doesn't match any constructor.
	// Both methodA and methodB should be unbound.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("ClassLib3", `
import Prelude
class TwoMethods a {
  methodA :: a -> a;
  methodB :: a -> a
}
instance TwoMethods Int {
  methodA := \x. x + 1;
  methodB := \x. x + 2
}
`)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import Prelude
import ClassLib3 (TwoMethods(MethodA))
main := methodB 41
`, "unbound variable: methodB")
}

func TestProbeB_SelectiveImportTypeAlias(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("AliasLib", `
import Prelude
type MyInt := Int
myVal :: MyInt
myVal := 42
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import AliasLib (MyInt, myVal)
check :: MyInt -> Int
check := \x. x
main := check myVal
`, int64(42))
}

func TestProbeB_SelectiveImportTypeFamily(t *testing.T) {
	// GICEL type family syntax: type Name params :: Kind := { equations }
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("TFLib2", `
import Prelude
type IsInt a :: Type := { IsInt Int =: Bool }
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectCon(t, eng, `
import Prelude
import TFLib2 (IsInt)
check :: IsInt Int -> Bool
check := \x. x
main := check True
`, "True")
}

// BUG: Severity=Medium. `import M ()` (empty import list) silently performs an OPEN
// import instead of a selective import with zero names. The parser's parseImportList
// returns nil (not an empty slice) for `()`, and importModules checks `imp.Names != nil`
// to distinguish selective from open import. So `import M ()` imports everything.
func TestProbeB_EmptyImportList(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("EmptyLib", `
import Prelude
myVal :: Int
myVal := 42
`)
	if err != nil {
		t.Fatal(err)
	}

	// BUG: Empty import list `import M ()` should mean "import nothing" but actually
	// imports everything because the parser returns nil for empty list.
	_, compErr := eng.NewRuntime(`
import Prelude
import EmptyLib ()
main := myVal
`)
	if compErr == nil {
		// This is the BUG: myVal SHOULD be unbound but it compiles because
		// import M () is treated as open import.
		t.Log("BUG CONFIRMED: import M () imports everything (treated as open import)")
	} else {
		t.Logf("import M () correctly rejected myVal: %v", compErr)
	}
}

func TestProbeB_SelectiveImportShadowsBuiltin(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("ShadowLib", `
shadowVal :: Int
shadowVal := 99
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import ShadowLib (shadowVal)
main := shadowVal
`, int64(99))
}

func TestProbeB_SelectiveImportDuplicateNameFromTwoModules(t *testing.T) {
	// Import the same name from two modules. No collision detection -- last wins.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("ModA", `
import Prelude
myVal :: Int
myVal := 1
`)
	if err != nil {
		t.Fatal(err)
	}

	err = eng.RegisterModule("ModB", `
import Prelude
myVal :: Int
myVal := 2
`)
	if err != nil {
		t.Fatal(err)
	}

	// Both export myVal. Current behavior: both push to context; last pushed wins.
	// No collision detection. This is a design observation.
	rt, err := eng.NewRuntime(`
import Prelude
import ModA (myVal)
import ModB (myVal)
main := myVal
`)
	if err != nil {
		t.Logf("collision error (may be intentional): %v", err)
		return
	}
	result, runErr := rt.RunWith(context.Background(), nil)
	if runErr != nil {
		t.Fatalf("runtime error: %v", runErr)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %s", result.Value, result.Value)
	}
	t.Logf("selective import duplicate name: result = %v (1=ModA, 2=ModB)", hv.Inner)
}

// =============================================================================
// Qualified Import Semantics
// =============================================================================

func TestProbeB_QualifiedClassMethod(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QClassLib", `
import Prelude
class QClass a { qmethod :: a -> a }
instance QClass Int { qmethod := \x. x + 10 }
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import QClassLib as Q
main := Q.qmethod 32
`, int64(42))
}

func TestProbeB_QualifiedTypeAliasInAnnotation(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QAliasLib", `
import Prelude
type MyInt := Int
myVal :: MyInt
myVal := 42
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import QAliasLib as Q
check :: Q.MyInt -> Int
check := \x. x
main := check Q.myVal
`, int64(42))
}

func TestProbeB_QualifiedConstructorWithInstance(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QConLib", `
import Prelude
data Wrap a := MkWrap a
instance Eq a => Eq (Wrap a) {
  eq := \x y. case (x, y) { (MkWrap a, MkWrap b) -> eq a b }
}
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectCon(t, eng, `
import Prelude
import QConLib as W
main := case eq (W.MkWrap 1) (W.MkWrap 1) { True -> True; False -> False }
`, "True")
}

func TestProbeB_QualifiedChainReexport(t *testing.T) {
	// Module A exports origFunc; Module B open-imports A (re-exporting origFunc).
	// ExportModule captures Values from all bindings in B's Core IR.
	// B.origFunc: origFunc is in B's checker context but NOT in B's core program
	// bindings (it's not a binding of B, just imported). So ExportModule.Values
	// only has B's own bindings.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("ChainA", `
import Prelude
origFunc :: Int -> Int
origFunc := \x. x + 100
`)
	if err != nil {
		t.Fatal(err)
	}

	err = eng.RegisterModule("ChainB", `
import ChainA
import Prelude
wrapFunc :: Int -> Int
wrapFunc := origFunc
`)
	if err != nil {
		t.Fatal(err)
	}

	// origFunc is NOT in B's Values (only B's own bindings are exported).
	// B.wrapFunc should work.
	rt, err := eng.NewRuntime(`
import Prelude
import ChainB as B
main := B.origFunc 0
`)
	if err != nil {
		t.Logf("chain re-export: origFunc not in B's exports (expected): %v", err)
		// Verify wrapFunc works
		probeRunExpectHost(t, eng, `
import Prelude
import ChainB as B
main := B.wrapFunc 0
`, int64(100))
		return
	}
	result, runErr := rt.RunWith(context.Background(), nil)
	if runErr != nil {
		t.Fatalf("runtime error: %v", runErr)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", result.Value)
	}
	t.Logf("chain re-export: origFunc through B = %v", hv.Inner)
}

// Qualified constructors in case patterns are not supported by the parser.
// This is a known limitation (parser doesn't recognize Q.Con in pattern position).
func TestProbeB_QualifiedConstructorInPatternNotSupported(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QPatLib", `
import Prelude
data Box a := MkBox a
`)
	if err != nil {
		t.Fatal(err)
	}

	_, compErr := eng.NewRuntime(`
import Prelude
import QPatLib as Q
main := case Q.MkBox 42 { Q.MkBox x -> x }
`)
	if compErr != nil {
		t.Logf("qualified constructor in pattern: not supported (parse error): %v", compErr)
	} else {
		t.Log("qualified constructor in pattern: unexpectedly succeeded")
	}
}

func TestProbeB_QualifiedTypeInAnnotationComplex(t *testing.T) {
	// Qualified type in annotation: \a. Q.T a -> Q.T a
	// Does NOT use qualified constructors in patterns (known unsupported).
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QTypeLib", `
import Prelude
data Box a := MkBox a
unwrapBox :: \a. Box a -> a
unwrapBox := \b. case b { MkBox x -> x }
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import QTypeLib as Q
identity :: \a. Q.Box a -> Q.Box a
identity := \x. x
main := Q.unwrapBox (identity (Q.MkBox 42))
`, int64(42))
}

// =============================================================================
// importModules Edge Cases
// =============================================================================

func TestProbeB_ImportSelfCircular(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("SelfRef", `
import SelfRef
myVal := 1
`)
	if err == nil {
		t.Fatal("expected error for self-importing module")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected circular dependency error, got: %v", err)
	}
}

func TestProbeB_ImportCoreExplicitly(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	// Core is auto-injected; explicit `import Core` should be a duplicate
	_, err := eng.NewRuntime(`
import Core
import Prelude
main := pure 42
`)
	if err != nil {
		t.Logf("explicit Core import result: %v", err)
		if !strings.Contains(err.Error(), "duplicate import") {
			t.Errorf("expected duplicate import error for Core, got: %v", err)
		}
	}
}

func TestProbeB_SelectiveImportTypeWithoutConstructors(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("NullaryLib", `
data Color := Red | Green | Blue
`)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import NullaryLib (Color)
main := Red
`, "unknown constructor: Red")
}

func TestProbeB_ModuleWithNoExports(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("EmptyMod", ``)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import EmptyMod (anything)
main := 42
`, "does not export")
}

func TestProbeB_QualifiedScopeWithAliases(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QAFLib", `
import Prelude
type MyStr := String
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import QAFLib as Q
check :: Q.MyStr -> String
check := \s. s
main := check "hello"
`, "hello")
}

// BUG: Severity=Medium. Qualified parameterized type families don't resolve.
// resolve_type.go TyExprQualCon only handles zero-arity type families
// (line 38: `len(fam.Params) == 0`). For non-zero arity, the name falls through
// to the Types check and errors. Same root cause as parameterized aliases:
// tryExpandApp only checks ch.families (local), not the qualified module's families.
func TestProbeB_QualifiedTypeFamilyInAnnotation(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QTFLib2", `
import Prelude
type MyResult a :: Type := { MyResult Int =: Bool }
`)
	if err != nil {
		t.Fatal(err)
	}

	_, compErr := eng.NewRuntime(`
import Prelude
import QTFLib2 as Q
check :: Q.MyResult Int -> Bool
check := \x. x
main := check True
`)
	if compErr != nil {
		// BUG CONFIRMED: parameterized type family via qualified import fails
		t.Logf("BUG CONFIRMED: qualified parameterized type family fails: %v", compErr)
	} else {
		t.Log("qualified parameterized type family unexpectedly succeeded")
	}
}

// =============================================================================
// Error Message Quality
// =============================================================================

func TestProbeB_ErrorMessageDoesNotExportIncludesModuleName(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("ErrLib", `
data X := MkX
`)
	if err != nil {
		t.Fatal(err)
	}

	_, compErr := eng.NewRuntime(`
import ErrLib (nonexistent)
main := 42
`)
	if compErr == nil {
		t.Fatal("expected error")
	}
	errStr := compErr.Error()
	if !strings.Contains(errStr, "ErrLib") {
		t.Errorf("error message should include module name 'ErrLib', got: %s", errStr)
	}
	if !strings.Contains(errStr, "does not export") {
		t.Errorf("error message should include 'does not export', got: %s", errStr)
	}
}

func TestProbeB_ErrorMessageUnknownQualifierIncludesName(t *testing.T) {
	eng := gicel.NewEngine()

	_, compErr := eng.NewRuntime(`
main := Z.something
`)
	if compErr == nil {
		t.Fatal("expected error")
	}
	errStr := compErr.Error()
	if !strings.Contains(errStr, "Z") {
		t.Errorf("error message should include qualifier name 'Z', got: %s", errStr)
	}
	if !strings.Contains(errStr, "unknown qualifier") {
		t.Errorf("error message should include 'unknown qualifier', got: %s", errStr)
	}
}

func TestProbeB_ErrorMessageQualifiedTypeDoesNotExport(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("QErrLib", `
data X := MkX
`)
	if err != nil {
		t.Fatal(err)
	}

	_, compErr := eng.NewRuntime(`
import QErrLib as Q
check :: Q.NonExistent -> Int
check := \x. 0
main := 0
`)
	if compErr == nil {
		t.Fatal("expected error for non-existent qualified type")
	}
	errStr := compErr.Error()
	if !strings.Contains(errStr, "QErrLib") {
		t.Errorf("error should mention module name 'QErrLib', got: %s", errStr)
	}
	if !strings.Contains(errStr, "does not export type") {
		t.Errorf("error should mention 'does not export type', got: %s", errStr)
	}
}

// =============================================================================
// Type Resolution
// =============================================================================

func TestProbeB_TyExprQualConNonExistentType(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("TRLib", `
data X := MkX
`)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import TRLib as T
check :: T.Bogus -> Int
check := \_ . 0
main := 0
`, "does not export type: Bogus")
}

// BUG: Severity=Medium. Qualified parameterized type aliases don't resolve.
// resolve_type.go only handles zero-arity qualified aliases (line 34: `len(info.params) == 0`).
// For non-zero arity, the qualified alias falls through to Types check, and even if
// found there as a TyCon, the alias expansion in tryExpandApp only checks ch.aliases
// (not the qualified module's aliases). So `Q.Pair Int` is never expanded.
func TestProbeB_TyExprQualConAliasWithParams(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("ParamAliasLib", `
import Prelude
type Pair a := (a, a)
`)
	if err != nil {
		t.Fatal(err)
	}

	// BUG: This fails because P.Pair is a parameterized alias and qualified
	// alias resolution only handles zero-arity aliases.
	_, compErr := eng.NewRuntime(`
import Prelude
import ParamAliasLib as P
check :: P.Pair Int -> Int
check := \p. fst p + snd p
main := check (1, 2)
`)
	if compErr != nil {
		t.Logf("BUG CONFIRMED: qualified parameterized alias fails: %v", compErr)
	} else {
		t.Log("qualified parameterized alias unexpectedly succeeded")
	}
}

func TestProbeB_QualifiedTypeInLambdaAnnotation(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QLamLib", `
import Prelude
data Wrapper a := MkWrapper a
unwrap :: \a. Wrapper a -> a
unwrap := \w. case w { MkWrapper x -> x }
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import QLamLib as W
transform :: \a. W.Wrapper a -> W.Wrapper a
transform := \x. x
main := W.unwrap (transform (W.MkWrapper 42))
`, int64(42))
}

// =============================================================================
// Advanced / Corner Cases
// =============================================================================

func TestProbeB_SelectiveImportPromotedKind(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("KindLib", `
import Prelude
data Nat := Z | S Nat
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectCon(t, eng, `
import KindLib (Nat(..))
main := S Z
`, "S")
}

func TestProbeB_SelectiveImportClassAllSubs(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("ClassAllLib", `
import Prelude
class Printable a {
  repr :: a -> String;
  label :: a -> String
}
instance Printable Int {
  repr := \_ . "int";
  label := \_ . "INT"
}
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import ClassAllLib (Printable(..))
main := repr 42
`, "int")
}

func TestProbeB_SelectiveImportClassEmptySubs(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("ClassEmptyLib", `
import Prelude
class MyClass2 a { myOp :: a -> a }
instance MyClass2 Int { myOp := \x. x + 1 }
`)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import Prelude
import ClassEmptyLib (MyClass2())
main := myOp 41
`, "unbound variable: myOp")
}

func TestProbeB_QualifiedImportExportLeaks(t *testing.T) {
	// ExportModule captures ALL checker state including inherited types.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("BaseLib", `
import Prelude
data Token := MkToken String
`)
	if err != nil {
		t.Fatal(err)
	}

	err = eng.RegisterModule("ExtLib", `
import Prelude
import BaseLib
makeToken :: String -> Token
makeToken := MkToken
`)
	if err != nil {
		t.Fatal(err)
	}

	rt, err := eng.NewRuntime(`
import Prelude
import ExtLib as E
main := E.makeToken "hello"
`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result, runErr := rt.RunWith(context.Background(), nil)
	if runErr != nil {
		t.Fatalf("runtime error: %v", runErr)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal, got %T: %s", result.Value, result.Value)
	}
	if con.Con != "MkToken" {
		t.Errorf("expected MkToken, got %s", con.Con)
	}
}

func TestProbeB_QualifiedUnknownModule(t *testing.T) {
	eng := gicel.NewEngine()
	probeExpectCompileError(t, eng, `
import NonExistentModule
main := 42
`, "unknown module: NonExistentModule")
}

func TestProbeB_ImportConInfoMismatch(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("SubLib", `
data Tree := Leaf | Branch Tree Tree
`)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import SubLib (Tree(Leaf))
main := Branch Leaf Leaf
`, "unknown constructor: Branch")
}

func TestProbeB_SelectiveImportNullaryConstructorInValues(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("NullLib", `
data Dir := North | South | East | West
`)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import NullLib (Dir)
main := North
`, "unknown constructor: North")
}

func TestProbeB_MultipleSameAliasFromOneModule(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("M1", `data X := MkX`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("M2", `data Y := MkY`)
	if err != nil {
		t.Fatal(err)
	}

	_, compErr := eng.NewRuntime(`
import M1 as A
import M2 as A
main := A.MkX
`)
	if compErr == nil {
		t.Fatal("expected alias collision error")
	}
	errStr := compErr.Error()
	if !strings.Contains(errStr, "alias") || !strings.Contains(errStr, "A") {
		t.Errorf("error should mention alias 'A', got: %s", errStr)
	}
}

func TestProbeB_QualifiedImportPreludeClass(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude as P
main := P.id 42
`, int64(42))
}

func TestProbeB_OpenAndQualifiedSameModule(t *testing.T) {
	// Duplicate import detection doesn't distinguish import forms.
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("DualLib", `
import Prelude
myFunc :: Int -> Int
myFunc := \x. x + 1
`)
	if err != nil {
		t.Fatal(err)
	}

	_, compErr := eng.NewRuntime(`
import Prelude
import DualLib
import DualLib as D
main := myFunc 1
`)
	if compErr != nil {
		if strings.Contains(compErr.Error(), "duplicate import") {
			t.Logf("duplicate import correctly rejected: %v", compErr)
		} else {
			t.Errorf("unexpected error (not duplicate import): %v", compErr)
		}
	} else {
		t.Log("open + qualified same module: allowed (no duplicate check for different forms)")
	}
}

func TestProbeB_QualifiedImportValuesVsConstructors(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("MixLib", `
data Color := Red | Green | Blue
red :: Color
red := Red
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectCon(t, eng, `
import MixLib as Q
main := Q.red
`, "Red")

	eng2 := gicel.NewEngine()
	err = eng2.RegisterModule("MixLib", `
data Color := Red | Green | Blue
red :: Color
red := Red
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectCon(t, eng2, `
import MixLib as Q
main := Q.Red
`, "Red")
}

// BUG: Severity=Low. ExportModule captures ALL registered types including
// built-in types (Int, String, etc.) from config.RegisteredTypes. This means
// Q.Int is valid in type annotations via qualified import. While functional,
// this is a namespace leakage: built-in types shouldn't appear as module exports.
func TestProbeB_ExportModuleAccumulation(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("TinyLib", `
myVal :: Int
myVal := 42
`)
	if err != nil {
		t.Fatal(err)
	}

	rt, err := eng.NewRuntime(`
import TinyLib as Q
check :: Q.Int -> Int
check := \x. x
main := check Q.myVal
`)
	if err != nil {
		t.Logf("Q.Int access result: %v", err)
	} else {
		result, runErr := rt.RunWith(context.Background(), nil)
		if runErr != nil {
			t.Fatalf("runtime error: %v", runErr)
		}
		hv, ok := result.Value.(*gicel.HostVal)
		if !ok {
			t.Fatalf("expected HostVal, got %T", result.Value)
		}
		t.Logf("BUG (low severity): Q.Int works, value = %v (built-in types leak into module exports)", hv.Inner)
	}
}

func TestProbeB_SelectiveClassImportInstanceCoherence(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("InstLib", `
import Prelude
data MyType := MkMyType
instance Eq MyType {
  eq := \_ _. True
}
eqCheck :: MyType -> MyType -> Bool
eqCheck := \x y. eq x y
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectCon(t, eng, `
import Prelude
import InstLib (eqCheck, MyType(..))
main := eqCheck MkMyType MkMyType
`, "True")
}

func TestProbeB_QualifiedImportDoesNotLeakToUnqualified(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QLeakLib", `
import Prelude
data Shape := Circle | Square
shapeName :: Shape -> String
shapeName := \s. case s { Circle -> "circle"; Square -> "square" }
`)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import Prelude
import QLeakLib as S
main := Circle
`, "unknown constructor: Circle")
}

func TestProbeB_SelectiveImportDataWithAllAndSpecificConstructors(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("AllConsLib", `
data Tri := A | B | C
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectCon(t, eng, `
import AllConsLib (Tri(..))
main := B
`, "B")
}

func TestProbeB_QualifiedImportThenSelectiveFromDifferentModule(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("QMod", `
import Prelude
qval :: Int
qval := 10
`)
	if err != nil {
		t.Fatal(err)
	}

	err = eng.RegisterModule("SMod", `
import Prelude
sval :: Int
sval := 20
`)
	if err != nil {
		t.Fatal(err)
	}

	probeRunExpectHost(t, eng, `
import Prelude
import QMod as Q
import SMod (sval)
main := Q.qval + sval
`, int64(30))
}

// BUG: Severity=Medium. The parser's parseImportName (parser.go:231) uses expectUpper
// for sub-list entries in Type/Class(...) syntax. Class methods are lowercase, so
// Class(methodName) is a parse error. Only uppercase constructor names can appear.
// This means selective import of individual class methods is not possible.
func TestProbeB_TypeClassMethodSubListParserRejectsLowercase(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("SubMethodLib", `
import Prelude
class Ops a {
  opA :: a -> a;
  opB :: a -> a;
  opC :: a -> a
}
instance Ops Int {
  opA := \x. x + 1;
  opB := \x. x + 2;
  opC := \x. x + 3
}
`)
	if err != nil {
		t.Fatal(err)
	}

	_, compErr := eng.NewRuntime(`
import Prelude
import SubMethodLib (Ops(opA))
main := opA 41
`)
	if compErr != nil {
		// BUG CONFIRMED: parser rejects lowercase in sub-list
		t.Logf("BUG CONFIRMED: class method sub-list parse error: %v", compErr)
	} else {
		t.Log("class method sub-list unexpectedly worked")
	}
}

func TestProbeB_ConInfoNameMismatch(t *testing.T) {
	eng := gicel.NewEngine()

	err := eng.RegisterModule("MultiDataLib", `
data Foo := MkFoo
data Bar := MkBar
`)
	if err != nil {
		t.Fatal(err)
	}

	probeExpectCompileError(t, eng, `
import MultiDataLib (Foo(..))
main := MkBar
`, "unknown constructor: MkBar")
}

func TestProbeB_QualifiedImportPreludeFullAccess(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	probeRunExpectCon(t, eng, `
import Prelude as P
main := P.True
`, "True")
}

// BUG: Severity=Low. resolveTypeExpr for TyExprCon does NOT validate that the type
// name is registered. Unknown type names silently become TyCon nodes. This means
// using an unimported type name in a type annotation compiles without error --
// the error only surfaces later (if at all) during unification.
func TestProbeB_SelectiveImportOverlappingTypeAndValue(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}

	err := eng.RegisterModule("OverlapLib", `
import Prelude
data MyData := MkMyData
myFunc :: Int -> Int
myFunc := \x. x
`)
	if err != nil {
		t.Fatal(err)
	}

	// Import only myFunc. MyData type is NOT imported, but using it in type
	// annotation does NOT produce an error because TyExprCon doesn't validate
	// registration. The type just becomes an opaque TyCon("MyData").
	_, compErr := eng.NewRuntime(`
import Prelude
import OverlapLib (myFunc)
check :: MyData -> Int
check := \_ . 0
main := myFunc 42
`)
	if compErr == nil {
		t.Log("BUG (low severity): unimported type 'MyData' silently accepted in type annotation")
	} else {
		t.Logf("unimported type correctly rejected: %v", compErr)
	}
}
