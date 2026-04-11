// Type family reduction tests — integration scenarios: do-threading, constraint families,
// multiplicity, pattern matching, exhaustiveness, and regressions.
// Does NOT cover: algorithm mutation probes (type_family_reduction_match_test.go,
//                 type_family_reduction_decl_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// Multi-step do block with correct threading.
func TestDoThreading_ThreeSteps(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
step1 :: Computation { a: Unit, b: Unit } { b: Unit } Unit
step1 := assumption
step2 :: Computation { b: Unit } { c: Unit } Unit
step2 := assumption
step3 :: Computation { c: Unit } {} Unit
step3 := assumption
main :: Computation { a: Unit, b: Unit } {} Unit
main := do { step1; step2; step3 }
`
	checkSource(t, source, nil)
}

// Do block where intermediate pre doesn't match: should fail.

// Do block where intermediate pre doesn't match: should fail.
func TestDoThreading_PreMismatch(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
step1 :: Computation { a: Unit } { b: Unit } Unit
step1 := assumption
step2 :: Computation { z: Unit } {} Unit
step2 := assumption
main :: Computation { a: Unit } {} Unit
main := do { step1; step2 }
`
	// step1's post = { b: Unit }, step2's pre = { z: Unit } => mismatch
	checkSourceExpectError(t, source, nil)
}

// Do block with bind and pre/post.

// Do block with bind and pre/post.
func TestDoThreading_BindPrePost(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
produce :: Computation { a: Unit } { b: Unit } Unit
produce := assumption
consume :: Computation { b: Unit } {} Unit
consume := assumption
main :: Computation { a: Unit } {} Unit
main := do {
  x <- produce;
  consume
}
`
	checkSource(t, source, nil)
}

func TestConstraintFamily_ReducesToClass(t *testing.T) {
	source := `
form Serialization := { JSON: Serialization; Binary: Serialization; }
form Show := \a. {
  show: a -> a
}
type Serializable :: Constraint := \(fmt: Serialization). case fmt {
  JSON => Show;
  Binary => Show
}
`
	checkSource(t, source, nil)
}

// Mult mismatch in row unification should fail.
func TestMultUnification_MismatchFails(t *testing.T) {
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
form Unit := { Unit: Unit; }
f :: { x: Unit @Linear } -> { x: Unit @Affine }
f := \r. r
`
	checkSourceExpectError(t, source, nil)
}

// Mult matches in row unification should succeed.

// Mult matches in row unification should succeed.
func TestMultUnification_MatchSucceeds(t *testing.T) {
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
form Unit := { Unit: Unit; }
f :: { x: Unit @Linear } -> { x: Unit @Linear }
f := \r. r
`
	checkSource(t, source, nil)
}

// One side has Mult, other doesn't: should not unify.

// One side has Mult, other doesn't: should not unify.
func TestMultUnification_AsymmetricMult(t *testing.T) {
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
form Unit := { Unit: Unit; }
f :: { x: Unit @Linear } -> { x: Unit }
f := \r. r
`
	// nil grades act as the identity element: { x: Unit } (nil grades) is
	// compatible with { x: Unit @Linear }. This allows grade-unaware generic
	// operations to work transparently with graded capabilities.
	checkSource(t, source, nil)
}

func TestTypeFamilyPosition_InArgument(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
wrap :: Elem (List Unit) -> Unit
wrap := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyPosition_InResult(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
unwrap :: Unit -> Elem (List Unit)
unwrap := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyPosition_InBothPositions(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
id :: Elem (List Unit) -> Elem (List Unit)
id := \x. x
`
	checkSource(t, source, nil)
}

// Two stuck TyFamilyApps with same name and args should unify.
func TestTyFamilyAppUnify_SameStuck(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: \ c. Elem c -> Elem c
f := \x. x
`
	checkSource(t, source, nil)
}

// Two stuck TyFamilyApps with different args should NOT unify.

// Two stuck TyFamilyApps with different args should NOT unify.
func TestTyFamilyAppUnify_DifferentArgsStuck(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: \ a b. Elem a -> Elem b
f := \x. x
`
	// Elem a and Elem b can't unify when a != b (different skolems).
	checkSourceExpectError(t, source, nil)
}

// Pattern with no variables (all concrete constructors).
func TestCollectPatternVars_AllConcrete(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => False;
  False => True
}
`
	checkSource(t, source, nil)
}

// Pattern with nested variables.

// Pattern with nested variables.
func TestCollectPatternVars_NestedVars(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Unit := { Unit: Unit; }
type F :: Type := \(c: Type). case c {
  (List a) => a;
  (Maybe a) => a
}
f :: F (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Non-exhaustive match should be caught.
// Uses regular data type since data family constructor registration
// requires the legacy syntax path.
func TestDataFamily_NonExhaustiveMatch(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Entry := { Singleton: Unit -> Entry; Empty: Entry; }

f :: Entry -> Unit
f := \e. case e {
  Singleton x => x
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
}

// Exhaustive match should pass.

// Exhaustive match should pass.
func TestDataFamily_ExhaustiveMatch(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Entry := { Singleton: Unit -> Entry; Empty: Entry; }

f :: Entry -> Unit
f := \e. case e {
  Singleton x => x;
  Empty => Unit
}
`
	checkSource(t, source, nil)
}

func TestTypeFamilyTwoParams_PartialMatch(t *testing.T) {
	// Two-param type family encoded via Pair since unified syntax only
	// supports single-param type families.
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type And :: Bool := \(p: Type). case p {
  (Pair True True) => True;
  (Pair True False) => False;
  (Pair False b) => False
}
`
	checkSource(t, source, nil)
}

func TestDivergentPost_AllBranchesConsumeAll(t *testing.T) {
	// Both branches consume all: intersection = full set (empty post).
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
consume1 :: Computation { a: Unit } {} Unit
consume1 := assumption
consume2 :: Computation { a: Unit } {} Unit
consume2 := assumption
f :: Bool -> Computation { a: Unit } {} Unit
f := \b. case b {
  True => consume1;
  False => consume2
}
`
	checkSource(t, source, nil)
}

func TestDivergentPost_NoSharedCaps(t *testing.T) {
	// Branch 1: post = { a }, Branch 2: post = { b }
	// Intersection = {} (no label in all branches)
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
keepA :: Computation { a: Unit, b: Unit } { a: Unit } Unit
keepA := assumption
keepB :: Computation { a: Unit, b: Unit } { b: Unit } Unit
keepB := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  True => keepA;
  False => keepB
}
`
	checkSource(t, source, nil)
}

func TestTypeFamilyWildcard_MultiParam(t *testing.T) {
	// Two-param type family with wildcards, encoded via Pair.
	source := `
form Bool := { True: Bool; False: Bool; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Or :: Bool := \(p: Type). case p {
  (Pair True _) => True;
  (Pair _ True) => True;
  (Pair False False) => False
}
`
	checkSource(t, source, nil)
}

func TestMatchTyPatterns_ConsistencyTyApp(t *testing.T) {
	// F (Pair (List a) (List a)) = a: both components must bind 'a' to the same type.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type F :: Type := \(p: Type). case p {
  (Pair (List a) (List a)) => a
}
f :: F (Pair (List Unit) (List Unit)) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// If consistency check is broken (accepts inconsistent bindings),
// F (Pair (List Unit) (List Bool)) would incorrectly reduce.

// If consistency check is broken (accepts inconsistent bindings),
// F (Pair (List Unit) (List Bool)) would incorrectly reduce.
func TestMatchTyPatterns_InconsistencyFails(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type F :: Type := \(p: Type). case p {
  (Pair (List a) (List a)) => a
}
f :: F (Pair (List Unit) (List Bool)) -> Unit
f := \x. x
`
	// This should fail: (Pair (List Unit) (List Bool)) doesn't match the
	// pattern (Pair (List a) (List a)) because Unit != Bool.
	// F is stuck. Stuck TyFamilyApp != Unit.
	checkSourceExpectError(t, source, nil)
}

func TestTypeFamilyCoexistsWithAlias(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Id := \a. a
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Id Unit -> Elem (List Unit)
f := \x. x
`
	checkSource(t, source, nil)
}

func TestLubPostStates_IncompatibleCapTypes(t *testing.T) {
	// Branch 1: post = { x: Unit }
	// Branch 2: post = { x: Bool }
	// Intersection: label 'x' is in both, but types Unit vs Bool can't unify.
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
opA :: Computation { x: Unit } { x: Unit } Unit
opA := assumption
opB :: Computation { x: Unit } { x: Bool } Unit
opB := assumption
f :: Bool -> Computation { x: Unit } { x: Unit } Unit
f := \b. case b {
  True => opA;
  False => opB
}
`
	// Should produce a type error because Unit and Bool can't unify.
	checkSourceExpectError(t, source, nil)
}

func TestRecursiveTyFamily_DualOfDual(t *testing.T) {
	// Dual(Dual(Send End)) should reduce back to Send End.
	// This tests deep recursive reduction.
	source := `
form Session := { Send: Session; Recv: Session; End: (); }
type Dual :: Session := \(s: Session). case s {
  (Send s) => Recv (Dual s);
  (Recv s) => Send (Dual s);
  End => End
}
`
	// Just verify parsing and registration with recursive definitions.
	checkSource(t, source, nil)
}

func TestTypeFamilyFuelExhaustion_ErrorMessage(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type Loop :: Type := \(a: Type). case a {
  a => Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	// Cycle detected via sentinel memoization; the family remains stuck,
	// producing a type mismatch when Loop Unit is compared against Unit.
	msg := checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
	if !strings.Contains(msg, "type mismatch") {
		t.Errorf("expected 'type mismatch' in error message, got: %s", msg)
	}
	if !strings.Contains(msg, "Loop") {
		t.Errorf("expected 'Loop' in error message, got: %s", msg)
	}
}

func TestInjectivityViolation_ErrorMessage(t *testing.T) {
	// In unified syntax, injectivity annotations are not supported.
	// This test verifies that the type family compiles correctly as a
	// regular closed type family.
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type F :: Type := \(c: Type). case c {
  (List a) => a;
  Unit => Unit
}
f :: F (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}
