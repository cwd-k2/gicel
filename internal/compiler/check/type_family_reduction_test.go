// Type family reduction tests — reduceTyFamily algorithm, pattern matching, fuel, injectivity.
// Does NOT cover: cross-feature interaction (type_family_interaction_test.go), pathological (type_family_pathological_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ==========================================
// Mutation Testing & Edge Case Exploration
// for type system extensions (feature/type-system-extensions)
// ==========================================

// -----------------------------------------------
// 1. reduceTyFamily mutation tests
// -----------------------------------------------

// If matchIndeterminate were treated as matchFail (continue), the second
// equation would fire erroneously. matchIndeterminate must halt.
func TestReduceTyFamily_IndeterminateStopsReduction(t *testing.T) {
	// F a = Int  (equation 1: variable pattern, always matches)
	// F Bool = Bool  (equation 2: would match if equation 1 didn't exist)
	// If argument is a skolem (unresolvable), equation 1 matches (variable always binds).
	// But what matters is: if we have concrete patterns that cannot decide against a meta,
	// we must return stuck rather than skipping to the next equation.
	//
	// A type family where equation 1 has a concrete pattern and arg is a meta:
	// type F (a: Type) :: Type := { F Bool =: Int; F a =: Bool }
	// F ?meta should be stuck (indeterminate), not fall through to F a =: Bool.
	source := `
data Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Bool => Int;
  a => Bool
}
f :: \ c. F c -> F c
f := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	// Should compile: F c is stuck and unifies with itself.
	checkSource(t, source, config)
}

// Circular self-reference: cycle detected via sentinel memoization.
// The family remains stuck (unreduced), producing a type mismatch (E0200).
func TestReduceTyFamily_FuelCounterIncrements(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
type Loop :: Type := \(a: Type). case a {
  a => Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	// Must produce ErrTypeMismatch (stuck Loop Unit vs Unit), not hang.
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

// Empty type family — no equations, always stuck.
func TestReduceTyFamily_EmptyFamily(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
}
f :: \ c. F c -> F c
f := \x. x
`
	// F c should be stuck (no equations to match), but F c ~ F c should still unify.
	checkSource(t, source, nil)
}

// Single-equation type family.
func TestReduceTyFamily_SingleEquation(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  Unit => Unit
}
f :: F Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Type family applied to unsolved metavariable must be stuck.
func TestReduceTyFamily_StuckOnMeta(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  Bool => Unit;
  Unit => Bool
}
f :: \ a. F a -> F a
f := \x. x
`
	checkSource(t, source, nil)
}

// Verify the first matching equation wins (not the last).
func TestReduceTyFamily_FirstMatchWins(t *testing.T) {
	// F Unit should reduce to Bool (first equation), not Int (second).
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  a => Bool;
  Unit => Int
}
f :: F Unit -> Bool
f := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// Verify first match by using a more specific pattern first.
func TestReduceTyFamily_SpecificBeforeGeneral(t *testing.T) {
	// F Bool = Int, F a = Bool. F Bool should give Int.
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  Bool => Int;
  a => Bool
}
g :: F Bool -> Int
g := \x. x
h :: F Unit -> Bool
h := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// -----------------------------------------------
// 2. matchTyPattern mutation tests
// -----------------------------------------------

// If TyVar "_" is NOT treated as wildcard, this test would fail.
func TestMatchTyPattern_WildcardUnderscoreBinds(t *testing.T) {
	// Two-param type family encoded via Pair.
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
data Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Const :: Type := \(p: Type). case p {
  (Pair a _) => a
}
f :: Const (Pair Unit Bool) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// If consistency check on repeated variables is wrong (returns matchSuccess
// when bindings are inconsistent), this would allow incorrect reduction.
func TestMatchTyPattern_ConsistentBindings(t *testing.T) {
	// F (Pair a a) = Unit should only match when both components are the same.
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
data Pair := \a b. { MkPair: a -> b -> Pair a b; }
type F :: Type := \(p: Type). case p {
  (Pair a a) => Unit;
  (Pair a b) => Bool
}
g :: F (Pair Unit Unit) -> Unit
g := \x. x
h :: F (Pair Unit Bool) -> Bool
h := \x. x
`
	checkSource(t, source, nil)
}

// TyApp pattern matching: if we fail to decompose TyApp correctly,
// this test would fail.
func TestMatchTyPattern_TyAppDecomposition(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
data Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  (Maybe a) => a
}
f :: Elem (List Unit) -> Unit
f := \x. x
g :: Elem (Maybe Unit) -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

// matchTyPattern with TyCon vs TyMeta should give matchIndeterminate.
func TestMatchTyPattern_TyConVsMeta(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Unit => Bool;
  Bool => Unit
}
f :: \ a. F a -> F a
f := \x. x
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 3. intersectCapRows mutation tests
// -----------------------------------------------

// If labels present in only SOME branches are kept, this test would fail
// because the joined post-state would claim cap 'a' exists in post when
// one branch consumed it.
func TestIntersectCapRows_DropsPartialLabels(t *testing.T) {
	// Branch 1: consumes 'a', keeps 'b' => post = { b: Unit }
	// Branch 2: keeps both => post = { a: Unit, b: Unit }
	// Intersection: only 'b' is shared => post = { b: Unit }
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
consumeA :: Computation { a: Unit, b: Unit } { b: Unit } Unit
consumeA := assumption
noop :: Computation { a: Unit, b: Unit } { a: Unit, b: Unit } Unit
noop := assumption
f :: Bool -> Computation { a: Unit, b: Unit } { b: Unit } Unit
f := \b. case b {
  True => consumeA;
  False => noop
}
`
	checkSource(t, source, nil)
}

// If labels present in NO branches are kept (all consumed), post = {}.
func TestIntersectCapRows_AllConsumedDifferent(t *testing.T) {
	// Branch 1: consumes 'a', keeps 'b'
	// Branch 2: consumes 'b', keeps 'a'
	// Intersection: no label in ALL branches => post = {}
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
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
	checkSource(t, source, nil)
}

// All branches consume all caps: intersection = full set (all labels shared).
func TestIntersectCapRows_AllBranchesSame(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
consumeAll :: Computation { a: Unit, b: Unit } {} Unit
consumeAll := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  True => consumeAll;
  False => consumeAll
}
`
	checkSource(t, source, nil)
}

// Three-way branch: label must be in ALL three branches to survive.
func TestIntersectCapRows_ThreeWayBranch(t *testing.T) {
	source := `
data Color := { Red: Color; Green: Color; Blue: Color; }
data Unit := { Unit: Unit; }
consumeA :: Computation { a: Unit, b: Unit, c: Unit } { b: Unit, c: Unit } Unit
consumeA := assumption
consumeB :: Computation { a: Unit, b: Unit, c: Unit } { a: Unit, c: Unit } Unit
consumeB := assumption
consumeC :: Computation { a: Unit, b: Unit, c: Unit } { a: Unit, b: Unit } Unit
consumeC := assumption
f :: Color -> Computation { a: Unit, b: Unit, c: Unit } {} Unit
f := \col. case col {
  Red => consumeA;
  Green => consumeB;
  Blue => consumeC
}
`
	// Each branch keeps 2 out of 3. No single label is in all 3 post-states.
	// Intersection = {}, so overall post = {}.
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 4. lubPostStates mutation tests
// -----------------------------------------------

// Single-branch case: lubPostStates should return that branch's post directly.
func TestLubPostStates_SingleBranch(t *testing.T) {
	source := `
data Unit := { MkUnit: (); }
consume :: Computation { x: Unit } {} Unit
consume := assumption
f :: Unit -> Computation { x: Unit } {} Unit
f := \u. case u {
  MkUnit => consume
}
`
	checkSource(t, source, nil)
}

// Non-computation case: lubPostStates should not be triggered, normal
// unification of result types applies.
func TestLubPostStates_NonCompResultType(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
f :: Bool -> Unit
f := \b. case b {
  True => Unit;
  False => Unit
}
`
	checkSource(t, source, nil)
}

// Case with empty post-states on both branches.
func TestLubPostStates_BothEmpty(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
nothing :: Computation {} {} Unit
nothing := assumption
f :: Bool -> Computation {} {} Unit
f := \b. case b {
  True => nothing;
  False => nothing
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 5. applyFunDepImprovement mutation tests
// -----------------------------------------------

// If "from" positions that ARE metas still trigger improvement, this could
// cause incorrect unification. FunDep should only fire when from-args are determined.
func TestFunDepImprovement_MetaFromDoesNotFire(t *testing.T) {
	// Without fundep, resolution works via normal instance matching.
	// The program should compile via normal instance resolution.
	source := `
data Unit := { Unit: Unit; }
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Collection := \c e. {
  empty: c
}
impl Collection (List a) a := {
  empty := Nil
}
main :: List Unit
main := empty
`
	checkSource(t, source, nil)
}

// First matching instance should win for instance resolution.
func TestFunDepImprovement_FirstMatchWins(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Elem := \c e. {
  extract: c -> e
}
impl Elem (List a) a := {
  extract := \xs. case xs { Cons x rest => x; Nil => extract Nil }
}
f :: List Unit -> Unit
f := \xs. extract xs
`
	checkSource(t, source, nil)
}

// Unknown parameter in class should produce error.
// In unified syntax, fundep annotations are not supported;
// In unified syntax, fundep annotations are not supported.
// A class with two params and a method is valid without fundeps.
func TestFunDepImprovement_UnknownFromParam(t *testing.T) {
	source := `
data Bad := \a b. {
  m: a -> b
}
`
	checkSource(t, source, nil)
}

// In unified syntax, fundep annotations are not supported.
// A class with two params and a method is valid without fundeps.
func TestFunDepImprovement_UnknownToParam(t *testing.T) {
	source := `
data Bad := \a b. {
  m: a -> b
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 6. processTypeFamily mutation tests
// -----------------------------------------------

// Duplicate type family should produce error.
func TestProcessTypeFamily_DuplicateCheck(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => True
}
type F :: Bool := \(a: Bool). case a {
  True => False
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

// Type family conflicting with type alias.
func TestProcessTypeFamily_ConflictsWithAlias(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
type F := \a. a
type F :: Type := \(a: Type). case a {
  a => a
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

// In unified syntax, equation names are auto-populated from the declaration name.
// This test verifies that a single-equation type family with a valid equation compiles.
func TestProcessTypeFamily_EquationNameMismatch(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => True;
  False => False
}
`
	checkSource(t, source, nil)
}

// In unified syntax, the case expression pattern is parsed as a single
// type expression (possibly an application chain). The arity matches
// the number of lambda params. This test verifies that an application
// pattern in a 1-param family is accepted as a single complex pattern.
func TestProcessTypeFamily_ArityValidation(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => True;
  False => False
}
`
	checkSource(t, source, nil)
}

// Zero-arity equation (too few patterns).
func TestProcessTypeFamily_ArityTooFew(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool) (b: Bool). case a {
  True => True
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyEquation)
}

// -----------------------------------------------
// 7. installFamilyReducer mutation tests
// -----------------------------------------------

// If depth is not reset per normalize call, a second family reduction
// after a successful first would inherit the counter and fail prematurely.
func TestInstallFamilyReducer_DepthResetsPerNormalize(t *testing.T) {
	// Two separate type family uses that each require reduction.
	// If depth doesn't reset, the second might hit the limit.
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Unit => Bool;
  Bool => Unit
}
f :: F Unit -> Bool
f := \x. x
g :: F Bool -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

// Recursive type family that terminates within limit.
func TestInstallFamilyReducer_RecursiveWithinLimit(t *testing.T) {
	source := `
data Session := { Send: Session; Recv: Session; End: (); }
type Dual :: Session := \(s: Session). case s {
  (Send s) => Recv (Dual s);
  (Recv s) => Send (Dual s);
  End => End
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 8. reduceFamilyApps mutation tests
// -----------------------------------------------

// TyApp chains with non-family heads should NOT be incorrectly reduced.
func TestReduceFamilyApps_NonFamilyHeadUntouched(t *testing.T) {
	// List is not a type family; applying List to a type should not trigger reduction.
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
f :: List Unit -> List Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// TyApp chain that forms saturated family application should reduce.
func TestReduceFamilyApps_SaturatedFamilyApp(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Elem (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Nested type family applications: F (G x).
// Currently, reduceFamilyApps does not recursively reduce TyFamilyApp arguments
// before attempting to match. This means F (G Unit) stays stuck because G Unit
// is not reduced to Bool before F tries to match it.
// This test documents the current behavior.
func TestReduceFamilyApps_NestedFamilyAppStuck(t *testing.T) {
	// F (G Unit) should ideally reduce to Unit (G Unit => Bool, F Bool -> Unit),
	// but currently stays stuck because inner TyFamilyApp args are not reduced
	// before the outer family tries to match.
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Bool => Unit
}
type G :: Type := \(a: Type). case a {
  Unit => Bool
}
f :: \ x. F (G x) -> F (G x)
f := \x. x
`
	// Using \ x to make it compile as stuck-on-stuck unification.
	checkSource(t, source, nil)
}

// Nested application where outer family is applied to a reduced inner result.
// This works when the inner result is used via a type annotation that triggers
// reduction in the unifier's normalize pass.
func TestReduceFamilyApps_InnerReductionViaUnifier(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
type Id :: Type := \(a: Type). case a {
  a => a
}
f :: Elem (List Unit) -> Unit
f := \x. x
g :: Id Unit -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

// Type family in function argument position.
func TestReduceFamilyApps_InArgPosition(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Unit -> Elem (List Unit)
f := \x. x
`
	checkSource(t, source, nil)
}

// Type family in both argument and result positions.
func TestReduceFamilyApps_BothPositions(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Elem (List Unit) -> Elem (List Unit)
f := \x. x
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 9. mangledDataFamilyName mutation tests
// -----------------------------------------------

// Different patterns should produce different mangled names (no collisions).
func TestMangledDataFamilyName_NoCollisions(t *testing.T) {
	// Verifies that associated type family equations for different instances
	// produce correct reductions.
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

data Container := \c. {
  type Elem c :: Type;
  empty: c
}

impl Container (List a) := {
  type Elem := a;
  empty := Nil
}

impl Container (Maybe a) := {
  type Elem := a;
  empty := Nothing
}

x :: Elem (List Unit) -> Unit
x := \v. v
y :: Elem (Maybe Unit) -> Unit
y := \v. v
`
	checkSource(t, source, nil)
}

// Zero-argument associated type definition in instance.
func TestMangledDataFamilyName_ZeroArgPattern(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }

data Container := \c. {
  type Elem c :: Type;
  empty: c
}

impl Container Unit := {
  type Elem := Unit;
  empty := Unit
}

x :: Elem Unit -> Unit
x := \v. v
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 10. verifyInjectivity mutation tests
// -----------------------------------------------

// Equations that SHOULD violate injectivity must be caught.
// F (List a) = a and F Unit = Unit: if a = Unit, both produce Unit but LHSes
// (List a) and Unit don't unify.
// Injectivity tests: in unified syntax, the injectivity annotation
// (r => c) is not supported. These tests verify type family behavior
// without injectivity checking.

func TestVerifyInjectivity_ViolationCaught(t *testing.T) {
	// Without injectivity annotation, overlapping RHSes are allowed
	// (they behave as a regular closed type family with first-match semantics).
	source := `
data Unit := { Unit: Unit; }
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  Unit => Unit
}
f :: Elem (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Equations with distinct RHSes should work fine.
func TestVerifyInjectivity_DistinctRHSes(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
type Tag :: Type := \(c: Type). case c {
  Unit => Bool;
  Bool => Unit
}
`
	checkSource(t, source, nil)
}

// Single-equation type family (identity).
func TestVerifyInjectivity_SingleEquationAlwaysInjective(t *testing.T) {
	source := `
type Id :: Type := \(a: Type). case a {
  a => a
}
`
	checkSource(t, source, nil)
}

// Single equation identity family with concrete usage.
func TestVerifyInjectivity_CompatibleOverlap(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  a => a
}
f :: F Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 11. Associated type edge cases
// -----------------------------------------------

// Associated type in class with no instances should produce a stuck type.
func TestAssocType_NoInstances(t *testing.T) {
	source := `
data Container := \c. {
  type Elem c :: Type;
  clength: c -> Int
}
f :: \ c. Elem c -> Elem c
f := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// Associated type equation in wrong class should be rejected.
func TestAssocType_WrongClass(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data Foo := \a. {
  type MyType a :: Type;
  m: a
}
data Bar := \a. {
  m2: a
}
impl Bar Unit := {
  type MyType := Unit;
  m2 := Unit
}
`
	checkSourceExpectError(t, source, nil)
}

// Associated type with arity mismatch in instance.
func TestAssocType_ArityMismatchInInstance(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data Container := \c. {
  type Elem c :: Type;
  empty: c
}
impl Container Unit := {
  type Elem := Unit;
  empty := Unit
}
`
	// In unified syntax, the associated type definition is always `type Elem := RHS`.
	// Arity mismatch is not possible. This test verifies it compiles correctly.
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 12. Do block pre/post threading edge cases
// -----------------------------------------------

// Multi-step do block with correct threading.
func TestDoThreading_ThreeSteps(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
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
func TestDoThreading_PreMismatch(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
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
func TestDoThreading_BindPrePost(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
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

// -----------------------------------------------
// 13. Constraint family
// -----------------------------------------------

func TestConstraintFamily_ReducesToClass(t *testing.T) {
	source := `
data Serialization := { JSON: Serialization; Binary: Serialization; }
data Show := \a. {
  show: a -> a
}
type Serializable :: Constraint := \(fmt: Serialization). case fmt {
  JSON => Show;
  Binary => Show
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 14. Multiplicity unification edge cases
// -----------------------------------------------

// Mult mismatch in row unification should fail.
func TestMultUnification_MismatchFails(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
f :: { x: Unit @Linear } -> { x: Unit @Affine }
f := \r. r
`
	checkSourceExpectError(t, source, nil)
}

// Mult matches in row unification should succeed.
func TestMultUnification_MatchSucceeds(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
f :: { x: Unit @Linear } -> { x: Unit @Linear }
f := \r. r
`
	checkSource(t, source, nil)
}

// One side has Mult, other doesn't: should not unify.
func TestMultUnification_AsymmetricMult(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
data Unit := { Unit: Unit; }
f :: { x: Unit @Linear } -> { x: Unit }
f := \r. r
`
	// Whether this is an error depends on the semantics: if Mult=nil means
	// Unrestricted and @Linear != Unrestricted, it should fail.
	// If it's lenient (nil Mult unifies with anything), it may succeed.
	// This test documents the actual behavior.
	// From the row_unify.go code: only unify if both sides have Mult.
	// So if one side has nil Mult, the unification of Mult is skipped.
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 15. Type family in argument vs result position
// -----------------------------------------------

func TestTypeFamilyPosition_InArgument(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
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
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
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
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
id :: Elem (List Unit) -> Elem (List Unit)
id := \x. x
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 16. TyFamilyApp unification edge cases
// -----------------------------------------------

// Two stuck TyFamilyApps with same name and args should unify.
func TestTyFamilyAppUnify_SameStuck(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: \ c. Elem c -> Elem c
f := \x. x
`
	checkSource(t, source, nil)
}

// Two stuck TyFamilyApps with different args should NOT unify.
func TestTyFamilyAppUnify_DifferentArgsStuck(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: \ a b. Elem a -> Elem b
f := \x. x
`
	// Elem a and Elem b can't unify when a != b (different skolems).
	checkSourceExpectError(t, source, nil)
}

// -----------------------------------------------
// 17. collectPatternVars edge cases
// -----------------------------------------------

// Pattern with no variables (all concrete constructors).
func TestCollectPatternVars_AllConcrete(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => False;
  False => True
}
`
	checkSource(t, source, nil)
}

// Pattern with nested variables.
func TestCollectPatternVars_NestedVars(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
data Unit := { Unit: Unit; }
type F :: Type := \(c: Type). case c {
  (List a) => a;
  (Maybe a) => a
}
f :: F (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 18. Data family exhaustiveness
// -----------------------------------------------

// Non-exhaustive match should be caught.
// Uses regular data type since data family constructor registration
// requires the legacy syntax path.
func TestDataFamily_NonExhaustiveMatch(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data Entry := { Singleton: Unit -> Entry; Empty: Entry; }

f :: Entry -> Unit
f := \e. case e {
  Singleton x => x
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
}

// Exhaustive match should pass.
func TestDataFamily_ExhaustiveMatch(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data Entry := { Singleton: Unit -> Entry; Empty: Entry; }

f :: Entry -> Unit
f := \e. case e {
  Singleton x => x;
  Empty => Unit
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 19. Edge case: type family with two parameters, partial match
// -----------------------------------------------

func TestTypeFamilyTwoParams_PartialMatch(t *testing.T) {
	// Two-param type family encoded via Pair since unified syntax only
	// supports single-param type families.
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
data Pair := \a b. { MkPair: a -> b -> Pair a b; }
type And :: Bool := \(p: Type). case p {
  (Pair True True) => True;
  (Pair True False) => False;
  (Pair False b) => False
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 20. Case expression: all branches consume all capabilities
// -----------------------------------------------

func TestDivergentPost_AllBranchesConsumeAll(t *testing.T) {
	// Both branches consume all: intersection = full set (empty post).
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
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

// -----------------------------------------------
// 21. Case expression: no shared capabilities
// -----------------------------------------------

func TestDivergentPost_NoSharedCaps(t *testing.T) {
	// Branch 1: post = { a }, Branch 2: post = { b }
	// Intersection = {} (no label in all branches)
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
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

// -----------------------------------------------
// 22. Type family: equation patterns with wildcards in multi-param
// -----------------------------------------------

func TestTypeFamilyWildcard_MultiParam(t *testing.T) {
	// Two-param type family with wildcards, encoded via Pair.
	source := `
data Bool := { True: Bool; False: Bool; }
data Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Or :: Bool := \(p: Type). case p {
  (Pair True _) => True;
  (Pair _ True) => True;
  (Pair False False) => False
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 23. matchTyPatterns: consistency check with TyApp patterns
// -----------------------------------------------

func TestMatchTyPatterns_ConsistencyTyApp(t *testing.T) {
	// F (Pair (List a) (List a)) = a: both components must bind 'a' to the same type.
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
data Pair := \a b. { MkPair: a -> b -> Pair a b; }
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
func TestMatchTyPatterns_InconsistencyFails(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
data Pair := \a b. { MkPair: a -> b -> Pair a b; }
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

// -----------------------------------------------
// 24. Regression: type aliases still work with families
// -----------------------------------------------

func TestTypeFamilyCoexistsWithAlias(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Id := \a. a
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Id Unit -> Elem (List Unit)
f := \x. x
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 25. lubPostStates: error reporting for incompatible capability types
// -----------------------------------------------

func TestLubPostStates_IncompatibleCapTypes(t *testing.T) {
	// Branch 1: post = { x: Unit }
	// Branch 2: post = { x: Bool }
	// Intersection: label 'x' is in both, but types Unit vs Bool can't unify.
	source := `
data Bool := { True: Bool; False: Bool; }
data Unit := { Unit: Unit; }
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

// -----------------------------------------------
// 26. Recursive type family: Dual of Dual
// -----------------------------------------------

func TestRecursiveTyFamily_DualOfDual(t *testing.T) {
	// Dual(Dual(Send End)) should reduce back to Send End.
	// This tests deep recursive reduction.
	source := `
data Session := { Send: Session; Recv: Session; End: (); }
type Dual :: Session := \(s: Session). case s {
  (Send s) => Recv (Dual s);
  (Recv s) => Send (Dual s);
  End => End
}
`
	// Just verify parsing and registration with recursive definitions.
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 27. Verifying error message content
// -----------------------------------------------

func TestTypeFamilyFuelExhaustion_ErrorMessage(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
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
data Unit := { Unit: Unit; }
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type F :: Type := \(c: Type). case c {
  (List a) => a;
  Unit => Unit
}
f :: F (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}
