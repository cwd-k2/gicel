// Type family pathological tests — edge cases, property tests, circular families, exponential growth.
// Does NOT cover: reduction algorithm (type_family_reduction_test.go), interaction (type_family_interaction_test.go).

package check

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ==========================================
// Pathological Checker Inputs
// ==========================================

// (a) Type family with overlapping equations (first-match semantics).
// Both equations match F Int, but first-match wins. This is not strictly
// an error in closed TF semantics (like GHC), but might be suspicious.
func TestPathologicalOverlappingEquations(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Int => Bool;
  Int => String
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{
			"Int":    types.TypeOfTypes,
			"String": types.TypeOfTypes,
		},
	}
	// With first-match semantics, overlapping equations are accepted.
	// The second equation is simply dead. Verify it does not crash.
	checkSource(t, source, config)
}

// (b) Circular type family references: A uses B and B uses A.
// Cycle detected via sentinel memoization; families remain stuck (unreduced),
// producing a type mismatch (E0200) when F Unit is compared against Unit.
func TestPathologicalCircularTypeFamilies(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  a => G a
}
type G :: Type := \(a: Type). case a {
  a => F a
}
f :: F Unit -> Unit
f := \x. x
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

// (c) Type family applied in its own equation RHS in a decreasing way.
// F (List a) = F a is recursive but decreasing — should terminate.
func TestPathologicalDecreasingRecursion(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type F :: Type := \(a: Type). case a {
  (List a) => F a;
  a => a
}
f :: F (List (List Unit)) -> Unit
f := \x. x
`
	// F (List (List Unit)) -> F (List Unit) -> F Unit -> Unit
	// Three reduction steps, well within the 100 fuel limit.
	checkSource(t, source, nil)
}

// (d) Data family with phantom type parameter.
// The constructor ignores `a` — this is fine structurally.
func TestPathologicalDataFamilyPhantomParam(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

form Container := \c. {
  type Phantom c :: Type;
  empty: c
}

impl Container (List a) := {
  type Phantom := Unit;
  empty := Nil
}

x :: Phantom (List Unit)
x := Unit
`
	checkSource(t, source, nil)
}

// (e) 100 instances of the same class with different type arguments.
func TestPathological100Instances(t *testing.T) {
	var b strings.Builder
	// Generate 100 distinct types.
	for i := 0; i < 100; i++ {
		b.WriteString(fmt.Sprintf("form T%d := { C%d: T%d; }\n", i, i, i))
	}
	b.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	b.WriteString("form Eq := \\a. { eq: a -> a -> Bool }\n")
	for i := 0; i < 100; i++ {
		b.WriteString(fmt.Sprintf("impl Eq T%d := { eq := \\x y. True }\n", i))
	}
	checkSource(t, b.String(), nil)
}

// (f) Type family equation where LHS pattern is another type family application.
// This tests whether the pattern matcher correctly handles (or rejects) non-constructor patterns.
func TestPathologicalTFPatternIsTFApp(t *testing.T) {
	// In a closed TF, patterns should be constructors/vars/wildcards.
	// A TF application in the pattern position would be resolved as a TyCon
	// (since F is in scope as a type name), so this tests the resolution path.
	source := `
form Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  a => a
}
type G :: Type := \(a: Type). case a {
  (F a) => a
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
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

form Container := \c. {
  type Elem c :: Type;
  empty: c
}

form Show := \a. {
  show: a -> a
}

impl Show Unit := {
  type Elem := Unit;
  show := \x. x
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
form Unit := { Unit: Unit; }
form Wrap := \a. { MkWrap: a -> Wrap a; }

form C1 := \a. {
  form Key a :: Type;
  m1: a
}

form C2 := \a. {
  form Key a :: Type;
  m2: a
}

impl C1 Unit := {
  type Key := Unit;
  m1 := Unit
}

impl C2 Unit := {
  type Key := Unit;
  m2 := Unit
}
`
	// Two classes both defining `form Key :: Type` is a name collision at the
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
	// Deeply nested exhaustiveness check with associated type and regular data types.
	source := `
form Unit := { Unit: Unit; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Repr := { ReprA: Repr; ReprB: Repr; ReprC: Repr; }

form HasRepr := \c. {
  type ReprOf c :: Type;
  toRepr: c -> ReprOf c
}

impl HasRepr Unit := {
  type ReprOf := Repr;
  toRepr := \_. ReprA
}

-- Nested pattern match on associated type result + Maybe.
f :: Maybe Repr -> Unit
f := \x. case x {
  Nothing => Unit;
  Just r => case r {
    ReprA => Unit;
    ReprB => Unit;
    ReprC => Unit
  }
}
`
	checkSource(t, source, nil)
}

// Exponential growth type family (Grow a =: Grow (Pair a a)) should be caught
// by the type size limit, not just the fuel limit.
func TestPathologicalExponentialGrowth(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Grow :: Type := \(a: Type). case a {
  a => Grow (Pair a a)
}
f :: Grow Unit -> Unit
f := \x. x
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyReduction)
}

// Type family that reduces but with a very large (but non-exponential) result.
func TestPathologicalLargeButFiniteResult(t *testing.T) {
	// This family produces a chain of S wrappers — linear growth, should be fine.
	source := `
form Nat := { Z: (); S: Nat; }
type AddTen :: Nat := \(n: Nat). case n {
  n => S (S (S (S (S (S (S (S (S (S n)))))))))
}
form Phantom := \(n: Nat). { MkPhantom: Phantom n; }
f :: Phantom (AddTen Z) -> Phantom (S (S (S (S (S (S (S (S (S (S Z))))))))))
f := \x. x
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
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  (Maybe a) => a
}

type Id :: Type := \(a: Type). case a {
  a => a
}
`
	// Test various TF applications and verify the result is in normal form.
	testCases := []struct {
		label string
		usage string
	}{
		{"Elem_List", "f :: Elem (List Unit) -> Unit\nf := \\x. x"},
		{"Elem_Maybe", "g :: Elem (Maybe Unit) -> Unit\ng := \\x. x"},
		{"Id_Unit", "h :: Id Unit -> Unit\nh := \\x. x"},
		{"Id_List", "i :: Id (List Unit) -> List Unit\ni := \\x. x"},
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
form Nat := { Z: (); S: Nat; }
form NatPair := \(a: Nat) (b: Nat). { MkNatPair: NatPair a b; }
type Add :: Nat := \(p: Type). case p {
  (NatPair Z b) => b;
  (NatPair (S a) b) => S (Add (NatPair a b))
}
form Phantom := \(n: Nat). { MkPhantom: Phantom n; }

-- Add (NatPair (S (S Z)) (S Z)) = S (S (S Z))
-- Reducing again: S (S (S Z)) has no TF app at the top -> same result.
f :: Phantom (Add (NatPair (S (S Z)) (S Z))) -> Phantom (S (S (S Z)))
f := \x. x
`
	checkSource(t, source, nil)
}

// (b) Unification symmetry: Unify(F a, Int) and Unify(Int, F a) should
// produce the same result.
func TestPropertyUnificationSymmetry(t *testing.T) {
	// Test 1: TF application on left vs right.
	source1 := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Elem (List Unit) -> Unit
f := \x. x
`
	source2 := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Unit -> Elem (List Unit)
f := \x. x
`
	// Both should succeed: Elem (List Unit) = Unit in either direction.
	checkSource(t, source1, nil)
	checkSource(t, source2, nil)
}

// (b') Symmetry with polymorphic types.
func TestPropertyUnificationSymmetryPoly(t *testing.T) {
	// Verify: \ c. Elem c -> Elem c works in both positions.
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: \ c. Elem c -> Elem c
f := \x. x
`
	checkSource(t, source, nil)
}

// (c) intersectCapRows commutativity: the order of rows shouldn't affect the result.
func TestPropertyIntersectCapRowsCommutativity(t *testing.T) {
	// Branch order: True first, then False. vs. False first, then True.
	// Both should produce the same post-state.
	source1 := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
consumeA :: Computation { a: Unit, b: Unit } { b: Unit } Unit
consumeA := assumption
consumeB :: Computation { a: Unit, b: Unit } { a: Unit } Unit
consumeB := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  True => consumeA;
  False => consumeB
}
`
	source2 := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
consumeA :: Computation { a: Unit, b: Unit } { b: Unit } Unit
consumeA := assumption
consumeB :: Computation { a: Unit, b: Unit } { a: Unit } Unit
consumeB := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  False => consumeB;
  True => consumeA
}
`
	// Both should type-check with the same result type.
	checkSource(t, source1, nil)
	checkSource(t, source2, nil)
}

// (c') Three-way commutativity.
func TestPropertyIntersectCapRowsCommutativity3Way(t *testing.T) {
	// Three branches consuming different caps.
	// The intersection should be {c: Unit} regardless of branch order.
	source := `
form Three := { One: Three; Two: Three; Three: Three; }
form Unit := { Unit: Unit; }
consumeAB :: Computation { a: Unit, b: Unit, c: Unit } { c: Unit } Unit
consumeAB := assumption
consumeAC :: Computation { a: Unit, b: Unit, c: Unit } { b: Unit } Unit
consumeAC := assumption
consumeBC :: Computation { a: Unit, b: Unit, c: Unit } { a: Unit } Unit
consumeBC := assumption
f :: Three -> Computation { a: Unit, b: Unit, c: Unit } {} Unit
f := \t. case t {
  One => consumeAB;
  Two => consumeAC;
  Three => consumeBC
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
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  (Maybe a) => a
}

f1 :: Elem (List Unit) -> Unit
f1 := \x. x
f2 :: Elem (List Unit) -> Unit
f2 := \x. x
f3 :: Elem (List Unit) -> Unit
f3 := \x. x

g1 :: Elem (Maybe Unit) -> Unit
g1 := \x. x
g2 :: Elem (Maybe Unit) -> Unit
g2 := \x. x
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
// applies all substitutions simultaneously (no cascading).
func TestPropertySubstManySimultaneous(t *testing.T) {
	// SubstMany is a true simultaneous substitution: the TyVar case returns
	// the replacement as-is without recursing into it. For {a→b, b→Int}
	// applied to (a -> b):
	//   - a is looked up → b (returned without further substitution)
	//   - b is looked up → Int
	//   - Result: b -> Int (not Int -> Int)
	// This is deterministic and correct for all substitution maps.
	aVar := &types.TyVar{Name: "a"}
	bVar := &types.TyVar{Name: "b"}
	intTy := &types.TyCon{Name: "Int"}

	original := types.MkArrow(aVar, bVar)
	subs := map[string]types.Type{
		"a": bVar,
		"b": intTy,
	}
	result := types.SubstMany(original, subs)

	pretty := types.Pretty(result)
	if pretty != "b -> Int" {
		t.Errorf("expected b -> Int, got %s", pretty)
	}

	// Verify no panic on re-application.
	_ = types.SubstMany(result, subs)
}

// (e”) SubstMany identity: substituting with empty map should be identity.
func TestPropertySubstManyIdentity(t *testing.T) {
	original := types.MkArrow(&types.TyVar{Name: "a"}, &types.TyCon{Name: "Int"})
	result := types.SubstMany(original, map[string]types.Type{})
	if !types.Equal(original, result) {
		t.Errorf("SubstMany with empty map should be identity, got %s", types.Pretty(result))
	}
}

// --- Additional pathological tests discovered during investigation ---

// SubstMany is simultaneous: replacements are returned as-is without
// recursing into them, so dependent variables within replacements are
// not expanded. This matches the formal definition of simultaneous
// substitution [a↦List b, b↦Int](a -> b) = List b -> Int.
func TestPathologicalSubstManyDependentVars(t *testing.T) {
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

	// Simultaneous: a → List b (as-is), b → Int. Result: List b -> Int.
	if pretty != "List b -> Int" {
		t.Errorf("expected List b -> Int, got %s", pretty)
	}
}

// Verify that the type family reduction properly handles the case where
// a TF application appears inside a \ body.
func TestPathologicalTFInsideForall(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: \ a. a -> Elem (List a)
f := \x. x
`
	checkSource(t, source, nil)
}

// Verify that a type family applied to a type variable (stuck) inside
// a computation type doesn't cause issues.
func TestPathologicalStuckTFInComputation(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: \ c. Computation {} {} (Elem c) -> Computation {} {} (Elem c)
f := \x. x
`
	checkSource(t, source, nil)
}

// Verify that matchTyPattern correctly handles repeated pattern variables
// with consistent bindings across multiple parameters.
func TestPathologicalRepeatedPatternVar(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Same :: Type := \(p: Type). case p {
  (Pair a a) => Unit
}
f :: Same (Pair Unit Unit) -> Unit
f := \x. x
`
	// Same (Pair Unit Unit): pattern (Pair a a) matches with a = Unit.
	// Both positions bind a to Unit => matchSuccess.
	checkSource(t, source, nil)
}

// Repeated pattern var with *inconsistent* bindings.
func TestPathologicalRepeatedPatternVarFail(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Same :: Type := \(p: Type). case p {
  (Pair a a) => Unit
}
f :: Same (Pair Unit Bool) -> Unit
f := \x. x
`
	// Same (Pair Unit Bool): pattern (Pair a a) tries to bind a to Unit and Bool.
	// Unit != Bool -> matchFail.
	// With no other equation, reduction is stuck and types won't match.
	checkSourceExpectError(t, source, nil)
}

// --- Helper for pathological tests that need raw parse+check ---

func parse_and_check(t *testing.T, source string, config *CheckConfig) (*diagnostic.Errors, *diagnostic.Errors) {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return lexErrs, nil
	}
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		return es, nil
	}
	_, checkErrs := Check(ast, src, config)
	return nil, checkErrs
}
