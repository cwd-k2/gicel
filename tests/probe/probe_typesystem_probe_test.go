//go:build probe

// Typesystem probe tests — type classes, type families, GADTs, higher-rank, cross-feature interactions.
// Does NOT cover: probe_typeclass_probe_test.go, probe_session_probe_test.go, probe_pattern_probe_test.go.
package probe_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

func assertConName(t *testing.T, v gicel.Value, name string) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal %s, got %T: %v", name, v, v)
	}
	if con.Con != name {
		t.Fatalf("expected %s, got %s", name, con.Con)
	}
}

// =====================================================================
// 1. Type class resolution edge cases
// =====================================================================

// TestProbeE_TC_OverlappingInstancesRejectOrChoose — when two instances
// could match, the checker should either reject at instance declaration
// time or deterministically choose one. No panic.
func TestProbeE_TC_OverlappingInstancesRejectOrChoose(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
form Bool := True | False
form C := \a. { method: a -> Bool }
impl C Bool := { method := \x. x }
impl C Bool := { method := \x. True }
main := method True
`)
	// An error is acceptable (overlapping instances). A result is also acceptable
	// if one instance is chosen. A panic is not.
	_ = err
}

// TestProbeE_TC_EmptyClassInstanceCompile — a class with no methods and
// an instance with no methods should compile and run.
func TestProbeE_TC_EmptyClassInstanceCompile(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
form Bool := True | False
form Phantom := \a. { _marker: () }
impl Phantom Bool := { _marker := () }
main := True
`)
	if err != nil {
		t.Fatalf("empty class/instance should compile: %v", err)
	}
}

// TestProbeE_TC_SuperclassTransitiveResolution — using a method from a
// grandparent class through a 3-level hierarchy.
func TestProbeE_TC_SuperclassTransitiveResolution(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
form Bool := { True: Bool; False: Bool; }
form A := \a. { ma: a -> Bool }
form B := \a. A a => { mb: a -> Bool }
form C := \a. B a => { mc: a -> Bool }
impl A Bool := { ma := \x. True }
impl B Bool := { mb := \x. True }
impl C Bool := { mc := \x. True }

-- Accessing ma through a C constraint (C -> B -> A)
f :: \a. C a => a -> Bool
f := \x. ma x

main := f True
`)
	if err != nil {
		t.Fatalf("superclass transitive resolution should work: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	assertConName(t, result.Value, "True")
}

// TestProbeE_TC_InstanceResolutionDepthLimit — an instance that requires
// itself (infinite resolution loop) should hit the depth limit.
func TestProbeE_TC_InstanceResolutionDepthLimit(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
form Bool := True | False
form C := \a. { method: a -> Bool }
-- An instance that requires itself as a context: C a => C a
-- This creates an infinite resolution loop
impl C a => C a := { method := \x. method x }
main := method True
`)
	// Should error (resolution depth or ambiguity), not hang
	if err == nil {
		t.Log("NOTICE: self-referential instance compiled without error")
	}
}

// TestProbeE_TC_MultiParamClassResolution — multi-parameter type class
// with ambiguity.
func TestProbeE_TC_MultiParamClassResolution(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
form Bool := True | False
form Convert := \a b. { convert: a -> b }
impl Convert Bool Bool := { convert := \x. x }

-- The 'b' type is ambiguous from the call site alone
f :: Bool -> Bool
f := \x. convert x

main := f True
`)
	// With the type annotation f :: Bool -> Bool, both a and b are determined.
	if err != nil {
		t.Logf("NOTICE: multi-param class error: %v", err)
	}
}

// =====================================================================
// 2. Type family reduction edge cases
// =====================================================================

// TestProbeE_TF_RecursiveFamilyHitsFuelLimit — recursive type family
// should not cause infinite loop.
func TestProbeE_TF_RecursiveFamilyHitsFuelLimit(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
form Nat := Z | S Nat

type Loop :: Type := \(n: Type). case n {
  n => Loop (S n)
}

main := (Z :: Loop Z)
`)
	if err == nil {
		t.Fatal("expected error for infinitely recursive type family")
	}
	if !strings.Contains(err.Error(), "depth limit") &&
		!strings.Contains(err.Error(), "reduction") &&
		!strings.Contains(err.Error(), "mismatch") {
		t.Logf("NOTICE: recursive TF error: %v", err)
	}
}

// TestProbeE_TF_ReducibleFamily — a well-formed type family should reduce
// and be usable in type annotations.
func TestProbeE_TF_ReducibleFamily(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
form Bool := { True: Bool; False: Bool; }
form Nat := Z | S Nat

type IsZero :: Type := \(n: Type). case n {
  Z => Bool;
  S n => Nat
}

val :: IsZero Z
val := True

main := val
`)
	if err != nil {
		t.Fatalf("reducible type family should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	assertConName(t, result.Value, "True")
}

// TestProbeE_TF_FamilyWithDataKinds — type family that dispatches on
// promoted nullary constructors (DataKinds).
func TestProbeE_TF_FamilyWithDataKinds(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Color := Red | Green | Blue

type ColorToInt :: Type := \(c: Color). case c {
  Red => Int;
  Green => Int;
  Blue => Int
}

val :: ColorToInt Red
val := 42

main := val
`)
	if err != nil {
		t.Fatalf("type family with DataKinds should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected 42, got %s", result.Value)
	}
}

// =====================================================================
// 3. GADT edge cases
// =====================================================================

// TestProbeE_GADT_ExistentialEscapeError — existential type leaking out
// of a case branch should be rejected.
func TestProbeE_GADT_ExistentialEscapeError(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
form Bool := True | False
form Exists := { MkExists: \a. a -> Exists; }

-- x has type 'a' (existential), but return type is 'a' which escapes
bad :: Exists -> Bool
bad := \e. case e { MkExists x => x }
`)
	if err == nil {
		t.Fatal("expected error for existential type escape")
	}
}

// TestProbeE_GADT_ValidExistential — existential type used within its scope
// should work.
func TestProbeE_GADT_ValidExistential(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Exists := { MkExists: \a. Show a => a -> Exists; }

showExists :: Exists -> String
showExists := \e. case e { MkExists x => show x }

main := showExists (MkExists True)
`)
	if err != nil {
		t.Fatalf("valid existential should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %s", result.Value, result.Value)
	}
	if hv.Inner != "True" {
		t.Errorf("expected \"True\", got %v", hv.Inner)
	}
}

// TestProbeE_GADT_NestedExistentialWithClass — nested existentials with
// class constraints.
func TestProbeE_GADT_NestedExistentialWithClass(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form SomeEq := { MkSomeEq: \a. Eq a => a -> a -> SomeEq; }

test :: SomeEq -> Bool
test := \s. case s { MkSomeEq x y => eq x y }

main := test (MkSomeEq True True)
`)
	if err != nil {
		t.Fatalf("nested existential with class should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	assertConName(t, result.Value, "True")
}

// =====================================================================
// 4. Module system edge cases
// =====================================================================

// TestProbeE_Module_AmbiguousNameFromTwoOpenImports — importing the same
// name from two open imports should be detected.
func TestProbeE_Module_AmbiguousNameFromTwoOpenImports(t *testing.T) {
	eng := gicel.NewEngine()
	eng.RegisterModule("A", `form Bool := True | False`)
	eng.RegisterModule("B", `form Bool := Yes | No`)
	_, err := eng.Compile(context.Background(), `
import A
import B
main := True
`)
	// Should either error (ambiguous) or resolve (first import wins).
	// The important thing is no panic.
	_ = err
}

// TestProbeE_Module_QualifiedAccessDisambiguates — qualified imports should
// disambiguate names that would otherwise collide.
func TestProbeE_Module_QualifiedAccessDisambiguates(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.RegisterModule("ModA", `
import Prelude
val :: Int
val := 10
`)
	eng.RegisterModule("ModB", `
import Prelude
val :: Int
val := 20
`)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import ModA as A
import ModB as B
main := A.val + B.val
`)
	if err != nil {
		t.Fatalf("qualified disambiguation should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %s", result.Value, result.Value)
	}
	if hv.Inner != int64(30) {
		t.Errorf("expected 30, got %v", hv.Inner)
	}
}

// TestProbeE_Module_SelectiveImportOfNonexistentName — should produce a
// clear error message.
func TestProbeE_Module_SelectiveImportOfNonexistentName(t *testing.T) {
	eng := gicel.NewEngine()
	eng.RegisterModule("Lib", `form Color := Red | Blue`)
	_, err := eng.Compile(context.Background(), `
import Lib (nonexistent)
main := 42
`)
	if err == nil {
		t.Fatal("expected error for importing nonexistent name")
	}
	if !strings.Contains(err.Error(), "does not export") {
		t.Logf("NOTICE: nonexistent selective import error: %v", err)
	}
}

// TestProbeE_Module_CircularDependencyDetection — circular module dependencies
// should be detected.
func TestProbeE_Module_CircularDependencyDetection(t *testing.T) {
	eng := gicel.NewEngine()
	// Module A imports B, but B doesn't exist yet
	err := eng.RegisterModule("A", `
import B
form Unit := Unit
`)
	if err == nil {
		// If A compiled (unlikely since B doesn't exist), try to register B importing A
		err = eng.RegisterModule("B", `
import A
form Void := MkVoid
`)
		if err == nil {
			t.Fatal("expected circular dependency error")
		}
	}
	// A should have failed because B doesn't exist yet
	if err != nil && !strings.Contains(err.Error(), "unknown module") &&
		!strings.Contains(err.Error(), "circular") {
		t.Logf("NOTICE: circular dependency error: %v", err)
	}
}

// TestProbeE_Module_InstanceCoherenceAcrossModules — instances from imported
// modules should be visible for constraint resolution.
func TestProbeE_Module_InstanceCoherenceAcrossModules(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.RegisterModule("ClassDef", `
form Bool := { True: Bool; False: Bool; }
form MyEq := \a. { myEq: a -> a -> Bool }
`); err != nil {
		t.Fatal(err)
	}
	if err := eng.RegisterModule("InstanceDef", `
import ClassDef
impl MyEq Bool := { myEq := \x y. True }
`); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import ClassDef
import InstanceDef
main := myEq True False
`)
	if err != nil {
		t.Fatalf("cross-module instance should be visible: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	assertConName(t, result.Value, "True")
}

// =====================================================================
// 5. Higher-rank types and polymorphism
// =====================================================================

// TestProbeE_HigherRank_RankNAnnotation — a rank-2 type annotation should
// be accepted.
func TestProbeE_HigherRank_RankNAnnotation(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
form Bool := { True: Bool; False: Bool; }

-- Rank-2: f takes a polymorphic function
apply :: (\a. a -> a) -> Bool -> Bool
apply := \f x. f x

main := apply (\x. x) True
`)
	if err != nil {
		t.Fatalf("rank-2 type should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	assertConName(t, result.Value, "True")
}

// TestProbeE_HigherRank_Subsumption — passing a polymorphic function where
// a monomorphic one is expected (subsumption).
func TestProbeE_HigherRank_Subsumption(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
form Bool := { True: Bool; False: Bool; }

f :: (Bool -> Bool) -> Bool
f := \g. g True

id :: \a. a -> a
id := \x. x

-- Pass the polymorphic id where Bool -> Bool is expected
main := f id
`)
	if err != nil {
		t.Fatalf("subsumption should work: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	assertConName(t, result.Value, "True")
}

// =====================================================================
// 6. Cross-feature interaction
// =====================================================================

// TestProbeE_Cross_TypeFamilyWithClass — type class with associated type
// family usage.
func TestProbeE_Cross_TypeFamilyWithClass(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude
form List a := Nil | Cons a (List a)

class Container (c :: Type) {
  type Elem c :: Type
}

instance Container (List a) {
  type Elem (List a) = a
}
`)
	if err != nil {
		t.Logf("NOTICE: associated type family error (may be expected): %v", err)
	}
}

// TestProbeE_Cross_GADTWithTypeFamily — using type families inside GADT
// constructor types.
func TestProbeE_Cross_GADTWithTypeFamily(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
form Bool := True | False
form Nat := Z | S Nat

type IsZero (n: Type) :: Type := {
  IsZero Z =: Bool;
  IsZero (S n) =: Nat
}

form Proof n := { ProofZ :: Proof Z; ProofS :: \m. Proof m -> Proof (S m) }

main := ProofZ
`)
	if err != nil {
		t.Logf("NOTICE: GADT with type family error: %v", err)
	}
}

// TestProbeE_Cross_LetGenWithMultipleConstraints — let generalization should
// handle multiple constraints correctly.
func TestProbeE_Cross_LetGenWithMultipleConstraints(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
-- eqAndShow should generalize to: \ a. (Eq a, Show a) => a -> a -> String
eqAndShow := \x y. case eq x y { True => show x; False => show y }
main := eqAndShow 1 2
`)
	if err != nil {
		t.Fatalf("let-gen with multiple constraints should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %s", result.Value, result.Value)
	}
	if hv.Inner != "2" {
		t.Errorf("expected \"2\", got %v", hv.Inner)
	}
}

// =====================================================================
// 7. Crash resistance with malformed programs
// =====================================================================

// TestProbeE_Crash_ApplyNonFunctionType — applying a non-function type
// should produce a clean error.
func TestProbeE_Crash_ApplyNonFunctionType(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
form Bool := True | False
main := True True
`)
	if err == nil {
		t.Fatal("expected error applying non-function type")
	}
}

// TestProbeE_Crash_UndefinedVariable — using an undefined variable should
// produce a clean error.
func TestProbeE_Crash_UndefinedVariable(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
main := undefinedVar
`)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "unbound") {
		t.Logf("NOTICE: undefined variable error: %v", err)
	}
}

// TestProbeE_Crash_DoBlockEndingWithBind — a do block that doesn't end
// with an expression should fail cleanly.
func TestProbeE_Crash_DoBlockEndingWithBind(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), `
main := do { x <- pure (); }
`)
	// Might succeed (trailing semicolons) or error. No panic.
	_ = err
}

// TestProbeE_Crash_VeryLongIdentifier — the checker should handle very
// long identifiers without issues.
func TestProbeE_Crash_VeryLongIdentifier(t *testing.T) {
	eng := gicel.NewEngine()
	long := strings.Repeat("a", 10000)
	source := long + " := 42\nmain := " + long
	eng.Use(gicel.Prelude)
	// Should either compile or produce a clean error
	_, err := eng.Compile(context.Background(), "import Prelude\n"+source)
	_ = err
}

// TestProbeE_Crash_ManyTopLevelBindings — the checker should handle many
// bindings without stack overflow.
func TestProbeE_Crash_ManyTopLevelBindings(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	var b strings.Builder
	b.WriteString("import Prelude\n")
	for i := 0; i < 200; i++ {
		b.WriteString("val")
		b.WriteString(strings.Repeat("_", 1))
		// Use a simple scheme to avoid collisions
		b.WriteString(strings.Replace(strings.Replace(
			strings.Replace(strings.Replace(
				strings.Replace(string(rune('a'+i%26)), "", "", 0),
				"", "", 0), "", "", 0), "", "", 0), "", "", 0))
		b.WriteString("_")
		for d := i; d > 0; d /= 10 {
			b.WriteByte(byte('0' + d%10))
		}
		if i == 0 {
			b.WriteByte('0')
		}
		b.WriteString(" := ")
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	b.WriteString("main := val_a_0\n")
	_, err := eng.Compile(context.Background(), b.String())
	_ = err // Just checking no panic
}

// TestProbeE_Crash_EmptySource — empty source should not panic.
func TestProbeE_Crash_EmptySource(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), "")
	// Should produce "no main binding" or similar error, not panic
	_ = err
}

// TestProbeE_Crash_OnlyComments — source with only comments should not panic.
func TestProbeE_Crash_OnlyComments(t *testing.T) {
	eng := gicel.NewEngine()
	_, err := eng.Compile(context.Background(), "-- just a comment\n-- another comment\n")
	_ = err
}

// TestProbeE_Crash_DataDeclWithManyConstructors — data type with many
// constructors should work.
func TestProbeE_Crash_DataDeclWithManyConstructors(t *testing.T) {
	eng := gicel.NewEngine()
	var b strings.Builder
	b.WriteString("form Big :=")
	for i := 0; i < 50; i++ {
		if i > 0 {
			b.WriteString(" |")
		}
		b.WriteString(" C")
		for d := i; d > 0; d /= 10 {
			b.WriteByte(byte('0' + d%10))
		}
		if i == 0 {
			b.WriteByte('0')
		}
	}
	b.WriteString("\nmain := C0\n")
	_, err := eng.Compile(context.Background(), b.String())
	if err != nil {
		t.Logf("NOTICE: many constructors error: %v", err)
	}
}

// =====================================================================
// 8. Computation type edge cases
// =====================================================================

// TestProbeE_Comp_PureUnit — pure () should type-check and run.
func TestProbeE_Comp_PureUnit(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `main := pure ()`)
	if err != nil {
		t.Fatalf("pure () should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || rv.Len() != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

// TestProbeE_Comp_BindChain — a chain of binds in a do block.
func TestProbeE_Comp_BindChain(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do {
  x <- pure 1;
  y <- pure 2;
  z <- pure 3;
  pure (x + y + z)
}
`)
	if err != nil {
		t.Fatalf("bind chain should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != int64(6) {
		t.Errorf("expected 6, got %s", result.Value)
	}
}

// =====================================================================
// 9. Record/tuple edge cases
// =====================================================================

// TestProbeE_Record_EmptyRecordType — unit type () should work in all
// positions.
func TestProbeE_Record_EmptyRecordType(t *testing.T) {
	eng := gicel.NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `
f :: () -> ()
f := \x. x
main := f ()
`)
	if err != nil {
		t.Fatalf("empty record/unit should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || rv.Len() != 0 {
		t.Errorf("expected (), got %s", result.Value)
	}
}

// TestProbeE_Record_NestedRecords — records containing other records.
func TestProbeE_Record_NestedRecords(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := { outer: { inner: 42 } }
`)
	if err != nil {
		t.Fatalf("nested records should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %s", result.Value, result.Value)
	}
	inner, ok := rv.Get("outer")
	if !ok {
		t.Fatal("missing field 'outer'")
	}
	innerRv, ok := inner.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected inner RecordVal, got %T: %s", inner, inner)
	}
	val, ok := innerRv.Get("inner")
	if !ok {
		t.Fatal("missing field 'inner'")
	}
	hv, ok := val.(*gicel.HostVal)
	if !ok || hv.Inner != int64(42) {
		t.Errorf("expected 42, got %s", val)
	}
}

// TestProbeE_Record_TupleWithManyElements — large tuples should work.
func TestProbeE_Record_TupleWithManyElements(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := (1, 2, 3, 4, 5, 6, 7, 8)
`)
	if err != nil {
		t.Fatalf("8-tuple should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %s", result.Value, result.Value)
	}
	if rv.Len() != 8 {
		t.Errorf("expected 8 fields, got %d", rv.Len())
	}
}

// =====================================================================
// 10. Compile-only checks (type inference correctness)
// =====================================================================

// TestProbeE_Infer_LetPolymorphism — let-bound values should be polymorphic.
func TestProbeE_Infer_LetPolymorphism(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
id := \x. x
-- id should be usable at both Bool and Int
main := (id True, id 42)
`)
	if err != nil {
		t.Fatalf("let polymorphism should work: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || rv.Len() != 2 {
		t.Fatalf("expected 2-tuple, got %s", result.Value)
	}
	assertConName(t, rv.MustGet("_1"), "True")
}

// TestProbeE_Infer_ConstrainedLetGen — inferred constraints should be
// generalized and usable at different types.
func TestProbeE_Infer_ConstrainedLetGen(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
same := \x y. eq x y
-- same should work at both Int and Bool
main := (same 1 1, same True False)
`)
	if err != nil {
		t.Fatalf("constrained let-gen should compile: %v", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("runtime error: %v", err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || rv.Len() != 2 {
		t.Fatalf("expected 2-tuple, got %s", result.Value)
	}
	assertConName(t, rv.MustGet("_1"), "True")
	assertConName(t, rv.MustGet("_2"), "False")
}
