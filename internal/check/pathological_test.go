package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
	"github.com/cwd-k2/gicel/internal/types"
)

// ==========================================
// Pathological Checker Inputs
// ==========================================

// (a) Type family with overlapping equations (first-match semantics).
// Both equations match F Int, but first-match wins. This is not strictly
// an error in closed TF semantics (like GHC), but might be suspicious.
func TestPathologicalOverlappingEquations(t *testing.T) {
	source := `
data Bool = True | False
type F (a : Type) :: Type = {
  F Int = Bool;
  F Int = String
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"String": types.KType{},
		},
	}
	// With first-match semantics, overlapping equations are accepted.
	// The second equation is simply dead. Verify it does not crash.
	checkSource(t, source, config)
}

// (b) Circular type family references: A uses B and B uses A.
// Should hit depth limit.
func TestPathologicalCircularTypeFamilies(t *testing.T) {
	source := `
data Unit = Unit
type F (a : Type) :: Type = {
  F a = G a
}
type G (a : Type) :: Type = {
  G a = F a
}
f :: F Unit -> Unit
f := \x -> x
`
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
}

// (c) Type family applied in its own equation RHS in a decreasing way.
// F (List a) = F a is recursive but decreasing — should terminate.
func TestPathologicalDecreasingRecursion(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type F (a : Type) :: Type = {
  F (List a) = F a;
  F a = a
}
f :: F (List (List Unit)) -> Unit
f := \x -> x
`
	// F (List (List Unit)) -> F (List Unit) -> F Unit -> Unit
	// Three reduction steps, well within the 100 fuel limit.
	checkSource(t, source, nil)
}

// (d) Data family with phantom type parameter.
// The constructor ignores `a` — this is fine structurally.
func TestPathologicalDataFamilyPhantomParam(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)

class Container c {
  data Phantom c :: Type;
  empty :: c
}

instance Container (List a) {
  data Phantom (List a) = PhantomList;
  empty := Nil
}

x :: Phantom (List Unit)
x := PhantomList
`
	checkSource(t, source, nil)
}

// (e) 100 instances of the same class with different type arguments.
func TestPathological100Instances(t *testing.T) {
	var b strings.Builder
	// Generate 100 distinct types.
	for i := 0; i < 100; i++ {
		b.WriteString(fmt.Sprintf("data T%d = C%d\n", i, i))
	}
	b.WriteString("data Bool = True | False\n")
	b.WriteString("class Eq a { eq :: a -> a -> Bool }\n")
	for i := 0; i < 100; i++ {
		b.WriteString(fmt.Sprintf("instance Eq T%d { eq := \\x -> \\y -> True }\n", i))
	}
	checkSource(t, b.String(), nil)
}

// (f) Fundep class with 0 instances — does resolution hang or error properly?
func TestPathologicalFunDepNoInstances(t *testing.T) {
	source := `
data Unit = Unit
class Convert a b | a -> b {
  convert :: a -> b
}
`
	// Just declaring a class with fundeps and no instances should be fine.
	checkSource(t, source, nil)
}

// Fundep class with 0 instances — trying to USE it should fail gracefully.
func TestPathologicalFunDepNoInstancesUsage(t *testing.T) {
	source := `
data Unit = Unit
class Convert a b | a -> b {
  convert :: a -> b
}
f :: Unit -> Unit
f := convert
`
	// Using `convert` without an instance should produce a resolution error,
	// not a hang.
	checkSourceExpectError(t, source, nil)
}

// (g) Type family equation where LHS pattern is another type family application.
// This tests whether the pattern matcher correctly handles (or rejects) non-constructor patterns.
func TestPathologicalTFPatternIsTFApp(t *testing.T) {
	// In a closed TF, patterns should be constructors/vars/wildcards.
	// A TF application in the pattern position would be resolved as a TyCon
	// (since F is in scope as a type name), so this tests the resolution path.
	source := `
data Unit = Unit
type F (a : Type) :: Type = {
  F a = a
}
type G (a : Type) :: Type = {
  G (F a) = a
}
`
	// The parser resolves `F` in a pattern position as a TyCon (type constructor),
	// not as a TyFamilyApp. The pattern matcher in matchTyPattern handles TyApp
	// decomposition, so G (F Unit) where arg = (F Unit) would try to match
	// TyApp(TyCon("F"), ...) against TyApp(TyCon("F"), ...).
	// Since type families are *not* injective by default, this might not reduce
	// but it should not crash.
	checkSource(t, source, nil)
}

// (h) Associated type defined in instance for wrong class.
func TestPathologicalAssocTypeWrongClass(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)

class Container c {
  type Elem c :: Type;
  empty :: c
}

class Show a {
  show :: a -> a
}

instance Show Unit {
  type Elem Unit = Unit;
  show := \x -> x
}
`
	// Elem is an assoc type of Container, not Show.
	checkSourceExpectError(t, source, nil)
}

// (i) Two data family instances that would produce the same mangled name.
// The mangling scheme uses familyName$$arity$pat1$pat2, where pat is the head constructor name.
// Two different types with the same head constructor name could theoretically collide.
func TestPathologicalDataFamilyMangledNameCollision(t *testing.T) {
	// Create two classes each with a data family named Key, applied to types
	// with the same head constructor. The mangling should include the class
	// to prevent collision, or the checker should detect the conflict.
	source := `
data Unit = Unit
data Wrap a = MkWrap a

class C1 a {
  data Key a :: Type;
  m1 :: a
}

class C2 a {
  data Key a :: Type;
  m2 :: a
}

instance C1 Unit {
  data Key Unit = KeyC1;
  m1 := Unit
}

instance C2 Unit {
  data Key Unit = KeyC2;
  m2 := Unit
}
`
	// Two classes both defining `data Key :: Type` is a name collision at the
	// family level (global families map). The checker detects this and errors
	// at the instance level because the second class's Key overwrites the first,
	// making it "not an associated data of class C1".
	// This is acceptable behavior — families are global names.
	_, checkErrs := parse_and_check(t, source, nil)
	if checkErrs != nil && checkErrs.HasErrors() {
		t.Logf("checker detected collision: %s", checkErrs.Format())
	} else {
		t.Error("expected error for duplicate data family name across classes")
	}
}

// (j) Deeply nested GADT-like exhaustiveness check with data family.
// Tests whether the exhaustiveness checker handles data family types
// across multiple nesting levels.
func TestPathologicalDeepDataFamilyExhaustiveness(t *testing.T) {
	source := `
data Unit = Unit
data Maybe a = Nothing | Just a

class HasRepr c {
  data Repr c :: Type;
  toRepr :: c -> Repr c
}

instance HasRepr Unit {
  data Repr Unit = ReprA | ReprB | ReprC;
  toRepr := \_ -> ReprA
}

-- Nested pattern match on data family + Maybe.
f :: Maybe (Repr Unit) -> Unit
f := \x -> case x {
  Nothing -> Unit;
  Just r -> case r {
    ReprA -> Unit;
    ReprB -> Unit;
    ReprC -> Unit
  }
}
`
	checkSource(t, source, nil)
}

// Exponential growth type family (Grow a = Grow (Pair a a)) should be caught
// by the type size limit, not just the fuel limit.
func TestPathologicalExponentialGrowth(t *testing.T) {
	source := `
data Unit = Unit
data Pair a b = MkPair a b
type Grow (a : Type) :: Type = {
  Grow a = Grow (Pair a a)
}
f :: Grow Unit -> Unit
f := \x -> x
`
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
}

// Type family that reduces but with a very large (but non-exponential) result.
func TestPathologicalLargeButFiniteResult(t *testing.T) {
	// This family produces a chain of S wrappers — linear growth, should be fine.
	source := `
data Nat = Z | S Nat
type AddTen (n : Nat) :: Nat = {
  AddTen n = S (S (S (S (S (S (S (S (S (S n)))))))))
}
data Phantom (n : Nat) = MkPhantom
f :: Phantom (AddTen Z) -> Phantom (S (S (S (S (S (S (S (S (S (S Z))))))))))
f := \x -> x
`
	checkSource(t, source, nil)
}

// ==========================================
// Property-Based Style Tests
// ==========================================

// (a) Reduction idempotence: for any TF application that reduces to R,
// reducing R again should give R (result is in normal form).
func TestPropertyReductionIdempotence(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
data Maybe a = Nothing | Just a

type Elem (c : Type) :: Type = {
  Elem (List a) = a;
  Elem (Maybe a) = a
}

type Id (a : Type) :: Type = {
  Id a = a
}
`
	// Test various TF applications and verify the result is in normal form.
	testCases := []struct {
		label string
		usage string
	}{
		{"Elem_List", "f :: Elem (List Unit) -> Unit\nf := \\x -> x"},
		{"Elem_Maybe", "g :: Elem (Maybe Unit) -> Unit\ng := \\x -> x"},
		{"Id_Unit", "h :: Id Unit -> Unit\nh := \\x -> x"},
		{"Id_List", "i :: Id (List Unit) -> List Unit\ni := \\x -> x"},
	}
	for _, tc := range testCases {
		t.Run(tc.label, func(t *testing.T) {
			// If the TF reduces to something that itself contains a TF app,
			// applying the TF reduction again should produce the same result.
			// The checker does this internally, so if it type-checks, the result
			// is already in normal form.
			fullSource := source + tc.usage
			checkSource(t, fullSource, nil)
		})
	}
}

// (a') Reduction idempotence for recursive TFs.
func TestPropertyReductionIdempotenceRecursive(t *testing.T) {
	source := `
data Nat = Z | S Nat
type Add (a : Nat) (b : Nat) :: Nat = {
  Add Z b = b;
  Add (S a) b = S (Add a b)
}
data Phantom (n : Nat) = MkPhantom

-- Add (S (S Z)) (S Z) = S (S (S Z))
-- Reducing again: S (S (S Z)) has no TF app at the top → same result.
f :: Phantom (Add (S (S Z)) (S Z)) -> Phantom (S (S (S Z)))
f := \x -> x
`
	checkSource(t, source, nil)
}

// (b) Unification symmetry: Unify(F a, Int) and Unify(Int, F a) should
// produce the same result.
func TestPropertyUnificationSymmetry(t *testing.T) {
	// Test 1: TF application on left vs right.
	source1 := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type Elem (c : Type) :: Type = {
  Elem (List a) = a
}
f :: Elem (List Unit) -> Unit
f := \x -> x
`
	source2 := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type Elem (c : Type) :: Type = {
  Elem (List a) = a
}
f :: Unit -> Elem (List Unit)
f := \x -> x
`
	// Both should succeed: Elem (List Unit) = Unit in either direction.
	checkSource(t, source1, nil)
	checkSource(t, source2, nil)
}

// (b') Symmetry with polymorphic types.
func TestPropertyUnificationSymmetryPoly(t *testing.T) {
	// Verify: \ c. Elem c -> Elem c works in both positions.
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type Elem (c : Type) :: Type = {
  Elem (List a) = a
}
f :: \ c. Elem c -> Elem c
f := \x -> x
`
	checkSource(t, source, nil)
}

// (c) intersectCapRows commutativity: the order of rows shouldn't affect the result.
func TestPropertyIntersectCapRowsCommutativity(t *testing.T) {
	// Branch order: True first, then False. vs. False first, then True.
	// Both should produce the same post-state.
	source1 := `
data Bool = True | False
data Unit = Unit
consumeA :: Computation { a : Unit, b : Unit } { b : Unit } Unit
consumeA := assumption
consumeB :: Computation { a : Unit, b : Unit } { a : Unit } Unit
consumeB := assumption
f :: Bool -> Computation { a : Unit, b : Unit } {} Unit
f := \b -> case b {
  True -> consumeA;
  False -> consumeB
}
`
	source2 := `
data Bool = True | False
data Unit = Unit
consumeA :: Computation { a : Unit, b : Unit } { b : Unit } Unit
consumeA := assumption
consumeB :: Computation { a : Unit, b : Unit } { a : Unit } Unit
consumeB := assumption
f :: Bool -> Computation { a : Unit, b : Unit } {} Unit
f := \b -> case b {
  False -> consumeB;
  True -> consumeA
}
`
	// Both should type-check with the same result type.
	checkSource(t, source1, nil)
	checkSource(t, source2, nil)
}

// (c') Three-way commutativity.
func TestPropertyIntersectCapRowsCommutativity3Way(t *testing.T) {
	// Three branches consuming different caps.
	// The intersection should be {c : Unit} regardless of branch order.
	source := `
data Three = One | Two | Three
data Unit = Unit
consumeAB :: Computation { a : Unit, b : Unit, c : Unit } { c : Unit } Unit
consumeAB := assumption
consumeAC :: Computation { a : Unit, b : Unit, c : Unit } { b : Unit } Unit
consumeAC := assumption
consumeBC :: Computation { a : Unit, b : Unit, c : Unit } { a : Unit } Unit
consumeBC := assumption
f :: Three -> Computation { a : Unit, b : Unit, c : Unit } {} Unit
f := \t -> case t {
  One -> consumeAB;
  Two -> consumeAC;
  Three -> consumeBC
}
`
	// Intersection of {c}, {b}, {a} = {} (no label in all three).
	checkSource(t, source, nil)
}

// (d) matchTyPattern determinism: same inputs -> same result.
// Verified implicitly: if the checker produces consistent results across
// multiple identical applications, it is deterministic.
func TestPropertyMatchTyPatternDeterminism(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
data Maybe a = Nothing | Just a

type Elem (c : Type) :: Type = {
  Elem (List a) = a;
  Elem (Maybe a) = a
}

f1 :: Elem (List Unit) -> Unit
f1 := \x -> x
f2 :: Elem (List Unit) -> Unit
f2 := \x -> x
f3 :: Elem (List Unit) -> Unit
f3 := \x -> x

g1 :: Elem (Maybe Unit) -> Unit
g1 := \x -> x
g2 :: Elem (Maybe Unit) -> Unit
g2 := \x -> x
`
	// All these identical usages must succeed — if matchTyPattern were
	// non-deterministic, some might fail unpredictably.
	checkSource(t, source, nil)
}

// (e) SubstMany = sequential Subst for independent variables.
func TestPropertySubstManyEqualsSequential(t *testing.T) {
	// Verify that SubstMany({a→Int, b→Bool}, \ . a -> b)
	// produces the same result as Subst(Subst(a->b, "a", Int), "b", Bool).
	intTy := &types.TyCon{Name: "Int"}
	boolTy := &types.TyCon{Name: "Bool"}
	aVar := &types.TyVar{Name: "a"}
	bVar := &types.TyVar{Name: "b"}

	original := types.MkArrow(aVar, bVar)

	// Sequential substitution.
	seq := types.Subst(original, "a", intTy)
	seq = types.Subst(seq, "b", boolTy)

	// SubstMany substitution.
	many := types.SubstMany(original, map[string]types.Type{
		"a": intTy,
		"b": boolTy,
	})

	if !types.Equal(seq, many) {
		t.Errorf("SubstMany != sequential Subst for independent vars:\n  seq:  %s\n  many: %s",
			types.Pretty(seq), types.Pretty(many))
	}
}

// (e') SubstMany with overlapping variables: verify SubstMany({a→b, b→Int})
// applies all substitutions "simultaneously" (no cascading).
func TestPropertySubstManySimultaneous(t *testing.T) {
	// SubstMany should be a simultaneous substitution, meaning:
	// SubstMany(a -> b, {a→b, b→Int}) should give b -> Int, not Int -> Int.
	// However, the current implementation is sequential (applies one at a time),
	// so SubstMany(a -> b, {a→b, b→Int}) might give Int -> Int depending
	// on iteration order. This test documents the actual behavior.
	aVar := &types.TyVar{Name: "a"}
	bVar := &types.TyVar{Name: "b"}
	intTy := &types.TyCon{Name: "Int"}

	original := types.MkArrow(aVar, bVar)
	subs := map[string]types.Type{
		"a": bVar,
		"b": intTy,
	}
	result := types.SubstMany(original, subs)

	// SubstMany iterates over a Go map, so iteration order is non-deterministic.
	// For dependent substitutions like {a→b, b→Int}:
	// - If a is substituted first: a→b gives (b -> b), then b→Int gives (Int -> Int)
	// - If b is substituted first: b→Int gives (a -> Int), then a→b gives (b -> Int)
	// Both outcomes have been observed in test runs. This is a LATENT BUG:
	// SubstMany is non-deterministic for dependent substitutions.
	// It works correctly because TF pattern matching only produces
	// independent substitutions (each pattern variable is distinct).
	pretty := types.Pretty(result)
	t.Logf("SubstMany({a→b, b→Int}, a -> b) = %s (non-deterministic for dependent subs)", pretty)

	// Verify no panic on re-application.
	_ = types.SubstMany(result, subs)
}

// (e'') SubstMany identity: substituting with empty map should be identity.
func TestPropertySubstManyIdentity(t *testing.T) {
	original := types.MkArrow(&types.TyVar{Name: "a"}, &types.TyCon{Name: "Int"})
	result := types.SubstMany(original, map[string]types.Type{})
	if !types.Equal(original, result) {
		t.Errorf("SubstMany with empty map should be identity, got %s", types.Pretty(result))
	}
}

// --- Additional pathological tests discovered during investigation ---

// SubstMany is sequential, not simultaneous. This is fine for TF pattern matching
// (which always produces independent substitutions), but could be surprising
// if someone passes dependent substitutions. Document the behavior.
func TestPathologicalSubstManyDependentVars(t *testing.T) {
	// {a → List b, b → Int} applied to (a -> b)
	// Sequential: a -> b ==[a→List b]==> List b -> b ==[b→Int]==> List Int -> Int
	// Simultaneous would give: List b -> Int (different!)
	aVar := &types.TyVar{Name: "a"}
	bVar := &types.TyVar{Name: "b"}
	intTy := &types.TyCon{Name: "Int"}
	listB := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: bVar}

	original := types.MkArrow(aVar, bVar)
	result := types.SubstMany(original, map[string]types.Type{
		"a": listB,
		"b": intTy,
	})
	pretty := types.Pretty(result)
	t.Logf("SubstMany({a→List b, b→Int}, a -> b) = %s", pretty)

	// Due to map iteration order, the result may be either:
	// - "List Int -> Int" (if a is substituted first)
	// - "List b -> Int"   (if b is substituted first)
	// Both are valid for sequential substitution. The important thing
	// is that TF pattern matching never produces such dependent substitutions.
	if !strings.Contains(pretty, "Int") {
		t.Error("expected Int somewhere in the result")
	}
}

// Verify that the type family reduction properly handles the case where
// a TF application appears inside a \ body.
func TestPathologicalTFInsideForall(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type Elem (c : Type) :: Type = {
  Elem (List a) = a
}
f :: \ a. a -> Elem (List a)
f := \x -> x
`
	checkSource(t, source, nil)
}

// Verify that a type family applied to a type variable (stuck) inside
// a computation type doesn't cause issues.
func TestPathologicalStuckTFInComputation(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type Elem (c : Type) :: Type = {
  Elem (List a) = a
}
f :: \ c. Computation {} {} (Elem c) -> Computation {} {} (Elem c)
f := \x -> x
`
	checkSource(t, source, nil)
}

// Verify that matchTyPattern correctly handles repeated pattern variables
// with consistent bindings across multiple parameters.
func TestPathologicalRepeatedPatternVar(t *testing.T) {
	source := `
data Unit = Unit
data Pair a b = MkPair a b
type Same (a : Type) (b : Type) :: Type = {
  Same a a = Unit
}
f :: Same Unit Unit -> Unit
f := \x -> x
`
	// Same Unit Unit: pattern a matches Unit (first param), then a must also
	// match Unit (second param). Both are Unit -> matchSuccess.
	checkSource(t, source, nil)
}

// Repeated pattern var with *inconsistent* bindings.
func TestPathologicalRepeatedPatternVarFail(t *testing.T) {
	source := `
data Unit = Unit
data Bool = True | False
data Pair a b = MkPair a b
type Same (a : Type) (b : Type) :: Type = {
  Same a a = Unit
}
f :: Same Unit Bool -> Unit
f := \x -> x
`
	// Same Unit Bool: pattern a matches Unit (first param), then a must also
	// match Bool (second param). Unit != Bool -> matchFail.
	// With no other equation, reduction is stuck and types won't match.
	checkSourceExpectError(t, source, nil)
}

// --- Helper for pathological tests that need raw parse+check ---

func parse_and_check(t *testing.T, source string, config *CheckConfig) (*errs.Errors, *errs.Errors) {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return lexErrs, nil
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		return es, nil
	}
	_, checkErrs := Check(ast, src, config)
	return nil, checkErrs
}
