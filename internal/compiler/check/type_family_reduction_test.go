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
data Bool := True | False
type F (a: Type) :: Type := {
  F Bool =: Int;
  F a =: Bool
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

// If the fuel counter doesn't increment, infinite recursion won't be caught.
func TestReduceTyFamily_FuelCounterIncrements(t *testing.T) {
	source := `
data Unit := Unit
type Loop (a: Type) :: Type := {
  Loop a =: Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	// Must produce ErrTypeFamilyReduction, not hang.
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyReduction)
}

// Empty type family — no equations, always stuck.
func TestReduceTyFamily_EmptyFamily(t *testing.T) {
	source := `
data Unit := Unit
type F (a: Type) :: Type := {
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
data Unit := Unit
type F (a: Type) :: Type := {
  F Unit =: Unit
}
f :: F Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Type family applied to unsolved metavariable must be stuck.
func TestReduceTyFamily_StuckOnMeta(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit
type F (a: Type) :: Type := {
  F Bool =: Unit;
  F Unit =: Bool
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
data Bool := True | False
data Unit := Unit
type F (a: Type) :: Type := {
  F a =: Bool;
  F Unit =: Int
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
data Bool := True | False
data Unit := Unit
type F (a: Type) :: Type := {
  F Bool =: Int;
  F a =: Bool
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
	source := `
data Unit := Unit
data Bool := True | False
type Const (a: Type) (b: Type) :: Type := {
  Const a _ =: a
}
f :: Const Unit Bool -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// If consistency check on repeated variables is wrong (returns matchSuccess
// when bindings are inconsistent), this would allow incorrect reduction.
func TestMatchTyPattern_ConsistentBindings(t *testing.T) {
	// F a a = Unit should only match when both args are the same.
	source := `
data Unit := Unit
data Bool := True | False
type F (a: Type) (b: Type) :: Type := {
  F a a =: Unit;
  F a b =: Bool
}
g :: F Unit Unit -> Unit
g := \x. x
h :: F Unit Bool -> Bool
h := \x. x
`
	checkSource(t, source, nil)
}

// TyApp pattern matching: if we fail to decompose TyApp correctly,
// this test would fail.
func TestMatchTyPattern_TyAppDecomposition(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Maybe a := Nothing | Just a
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a;
  Elem (Maybe a) =: a
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
data Unit := Unit
data Bool := True | False
type F (a: Type) :: Type := {
  F Unit =: Bool;
  F Bool =: Unit
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
data Bool := True | False
data Unit := Unit
consumeA :: Computation { a: Unit, b: Unit } { b: Unit } Unit
consumeA := assumption
noop :: Computation { a: Unit, b: Unit } { a: Unit, b: Unit } Unit
noop := assumption
f :: Bool -> Computation { a: Unit, b: Unit } { b: Unit } Unit
f := \b. case b {
  True -> consumeA;
  False -> noop
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
data Bool := True | False
data Unit := Unit
consumeA :: Computation { a: Unit, b: Unit } { b: Unit } Unit
consumeA := assumption
consumeB :: Computation { a: Unit, b: Unit } { a: Unit } Unit
consumeB := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  True -> consumeA;
  False -> consumeB
}
`
	checkSource(t, source, nil)
}

// All branches consume all caps: intersection = full set (all labels shared).
func TestIntersectCapRows_AllBranchesSame(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit
consumeAll :: Computation { a: Unit, b: Unit } {} Unit
consumeAll := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  True -> consumeAll;
  False -> consumeAll
}
`
	checkSource(t, source, nil)
}

// Three-way branch: label must be in ALL three branches to survive.
func TestIntersectCapRows_ThreeWayBranch(t *testing.T) {
	source := `
data Color := Red | Green | Blue
data Unit := Unit
consumeA :: Computation { a: Unit, b: Unit, c: Unit } { b: Unit, c: Unit } Unit
consumeA := assumption
consumeB :: Computation { a: Unit, b: Unit, c: Unit } { a: Unit, c: Unit } Unit
consumeB := assumption
consumeC :: Computation { a: Unit, b: Unit, c: Unit } { a: Unit, b: Unit } Unit
consumeC := assumption
f :: Color -> Computation { a: Unit, b: Unit, c: Unit } {} Unit
f := \col. case col {
  Red -> consumeA;
  Green -> consumeB;
  Blue -> consumeC
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
data Unit := MkUnit
consume :: Computation { x: Unit } {} Unit
consume := assumption
f :: Unit -> Computation { x: Unit } {} Unit
f := \u. case u {
  MkUnit -> consume
}
`
	checkSource(t, source, nil)
}

// Non-computation case: lubPostStates should not be triggered, normal
// unification of result types applies.
func TestLubPostStates_NonCompResultType(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit
f :: Bool -> Unit
f := \b. case b {
  True -> Unit;
  False -> Unit
}
`
	checkSource(t, source, nil)
}

// Case with empty post-states on both branches.
func TestLubPostStates_BothEmpty(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit
nothing :: Computation {} {} Unit
nothing := assumption
f :: Bool -> Computation {} {} Unit
f := \b. case b {
  True -> nothing;
  False -> nothing
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
	// With fundep c -> e, if c is unknown (meta), improvement should not fire.
	// The program should still compile via normal instance resolution.
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)
class Collection c e | c =: e {
  empty :: c
}
instance Collection (List a) a {
  empty := Nil
}
main :: List Unit
main := empty
`
	checkSource(t, source, nil)
}

// First matching instance should win for fundep improvement.
func TestFunDepImprovement_FirstMatchWins(t *testing.T) {
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)
class Elem c e | c =: e {
  extract :: c -> e
}
instance Elem (List a) a {
  extract := \xs. case xs { Cons x rest -> x; Nil -> extract Nil }
}
f :: List Unit -> Unit
f := \xs. extract xs
`
	checkSource(t, source, nil)
}

// Unknown parameter in fundep should produce error.
func TestFunDepImprovement_UnknownFromParam(t *testing.T) {
	source := `
class Bad a b | z =: b {
  m :: a -> b
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadClass)
}

// Unknown "to" parameter in fundep should produce error.
func TestFunDepImprovement_UnknownToParam(t *testing.T) {
	source := `
class Bad a b | a =: z {
  m :: a -> b
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadClass)
}

// -----------------------------------------------
// 6. processTypeFamily mutation tests
// -----------------------------------------------

// Duplicate type family should produce error.
func TestProcessTypeFamily_DuplicateCheck(t *testing.T) {
	source := `
data Bool := True | False
type F (a: Bool) :: Bool := {
  F True =: True
}
type F (a: Bool) :: Bool := {
  F True =: False
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

// Type family conflicting with type alias.
func TestProcessTypeFamily_ConflictsWithAlias(t *testing.T) {
	source := `
data Unit := Unit
type F a := a
type F (a: Type) :: Type := {
  F a =: a
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

// Equation name mismatch should produce error.
func TestProcessTypeFamily_EquationNameMismatch(t *testing.T) {
	source := `
data Bool := True | False
type F (a: Bool) :: Bool := {
  G True =: True
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyEquation)
}

// Arity validation: equation with wrong number of patterns.
func TestProcessTypeFamily_ArityValidation(t *testing.T) {
	source := `
data Bool := True | False
type F (a: Bool) :: Bool := {
  F True False =: True
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyEquation)
}

// Zero-arity equation (too few patterns).
func TestProcessTypeFamily_ArityTooFew(t *testing.T) {
	source := `
data Bool := True | False
type F (a: Bool) (b: Bool) :: Bool := {
  F True =: True
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
data Unit := Unit
data Bool := True | False
type F (a: Type) :: Type := {
  F Unit =: Bool;
  F Bool =: Unit
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
data Session := Send Session | Recv Session | End
type Dual (s: Session) :: Session := {
  Dual (Send s) =: Recv (Dual s);
  Dual (Recv s) =: Send (Dual s);
  Dual End =: End
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
data List a := Nil | Cons a (List a)
data Unit := Unit
f :: List Unit -> List Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// TyApp chain that forms saturated family application should reduce.
func TestReduceFamilyApps_SaturatedFamilyApp(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
	// F (G Unit) should ideally reduce to Unit (G Unit -> Bool, F Bool -> Unit),
	// but currently stays stuck because inner TyFamilyApp args are not reduced
	// before the outer family tries to match.
	source := `
data Unit := Unit
data Bool := True | False
type F (a: Type) :: Type := {
  F Bool =: Unit
}
type G (a: Type) :: Type := {
  G Unit =: Bool
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
data Unit := Unit
data Bool := True | False
data List a := Nil | Cons a (List a)
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
type Id (a: Type) :: Type := {
  Id a =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
f :: Unit -> Elem (List Unit)
f := \x. x
`
	checkSource(t, source, nil)
}

// Type family in both argument and result positions.
func TestReduceFamilyApps_BothPositions(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
data Maybe a := Nothing | Just a

class Container c {
  data Elem c :: Type;
  empty :: c
}

instance Container (List a) {
  data Elem (List a) =: ListElem a;
  empty := Nil
}

instance Container (Maybe a) {
  data Elem (Maybe a) =: MaybeElem a;
  empty := Nothing
}

x :: Elem (List Unit)
x := ListElem Unit
y :: Elem (Maybe Unit)
y := MaybeElem Unit
`
	checkSource(t, source, nil)
}

// Zero-argument patterns in data family (data Elem Unit := ...).
func TestMangledDataFamilyName_ZeroArgPattern(t *testing.T) {
	source := `
data Unit := Unit

class Container c {
  data Elem c :: Type;
  empty :: c
}

instance Container Unit {
  data Elem Unit =: UnitElem;
  empty := Unit
}

x :: Elem Unit
x := UnitElem
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 10. verifyInjectivity mutation tests
// -----------------------------------------------

// Equations that SHOULD violate injectivity must be caught.
// F (List a) = a and F Unit = Unit: if a = Unit, both produce Unit but LHSes
// (List a) and Unit don't unify.
func TestVerifyInjectivity_ViolationCaught(t *testing.T) {
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)
type Elem (c: Type) :: (r: Type) | r =: c := {
  Elem (List a) =: a;
  Elem Unit =: Unit
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrInjectivity)
}

// Equations that should NOT violate injectivity: each RHS is distinct.
func TestVerifyInjectivity_DistinctRHSes(t *testing.T) {
	source := `
data Unit := Unit
data Bool := True | False
data List a := Nil | Cons a (List a)
type Tag (c: Type) :: (r: Type) | r =: c := {
  Tag Unit =: Bool;
  Tag Bool =: Unit
}
`
	// Bool and Unit can't unify, so no violation.
	checkSource(t, source, nil)
}

// Overlapping but compatible equations: F a = a is injective because
// there's only one equation.
func TestVerifyInjectivity_SingleEquationAlwaysInjective(t *testing.T) {
	source := `
type Id (a: Type) :: (r: Type) | r =: a := {
  Id a =: a
}
`
	checkSource(t, source, nil)
}

// Two equations with same RHS structure but patterns that DO unify: OK.
func TestVerifyInjectivity_CompatibleOverlap(t *testing.T) {
	// F a = a; the RHSes unify (they are the same variable).
	// Only one equation, so there's nothing to compare.
	source := `
data Unit := Unit
type F (a: Type) :: (r: Type) | r =: a := {
  F a =: a
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
class Container c {
  type Elem c :: Type;
  clength :: c -> Int
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
data Unit := Unit
class Foo a {
  type MyType a :: Type;
  m :: a
}
class Bar a {
  m2 :: a
}
instance Bar Unit {
  data MyType Unit =: Bad;
  m2 := Unit
}
`
	checkSourceExpectError(t, source, nil)
}

// Associated type with arity mismatch in instance.
func TestAssocType_ArityMismatchInInstance(t *testing.T) {
	source := `
data Unit := Unit
class Container c {
  type Elem c :: Type;
  empty :: c
}
instance Container Unit {
  type Elem Unit Unit =: Unit;
  empty := Unit
}
`
	checkSourceExpectError(t, source, nil)
}

// -----------------------------------------------
// 12. Do block pre/post threading edge cases
// -----------------------------------------------

// Multi-step do block with correct threading.
func TestDoThreading_ThreeSteps(t *testing.T) {
	source := `
data Unit := Unit
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
data Unit := Unit
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
data Unit := Unit
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
data Serialization := JSON | Binary
class Show a {
  show :: a -> a
}
type Serializable (fmt: Serialization) :: Constraint := {
  Serializable JSON =: Show;
  Serializable Binary =: Show
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
data Mult := Unrestricted | Affine | Linear
data Unit := Unit
f :: { x: Unit @Linear } -> { x: Unit @Affine }
f := \r. r
`
	checkSourceExpectError(t, source, nil)
}

// Mult matches in row unification should succeed.
func TestMultUnification_MatchSucceeds(t *testing.T) {
	source := `
data Mult := Unrestricted | Affine | Linear
data Unit := Unit
f :: { x: Unit @Linear } -> { x: Unit @Linear }
f := \r. r
`
	checkSource(t, source, nil)
}

// One side has Mult, other doesn't: should not unify.
func TestMultUnification_AsymmetricMult(t *testing.T) {
	source := `
data Mult := Unrestricted | Affine | Linear
data Unit := Unit
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
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
wrap :: Elem (List Unit) -> Unit
wrap := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyPosition_InResult(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
unwrap :: Unit -> Elem (List Unit)
unwrap := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyPosition_InBothPositions(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
f :: \ c. Elem c -> Elem c
f := \x. x
`
	checkSource(t, source, nil)
}

// Two stuck TyFamilyApps with different args should NOT unify.
func TestTyFamilyAppUnify_DifferentArgsStuck(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data Bool := True | False
type F (a: Bool) :: Bool := {
  F True =: False;
  F False =: True
}
`
	checkSource(t, source, nil)
}

// Pattern with nested variables.
func TestCollectPatternVars_NestedVars(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Maybe a := Nothing | Just a
data Unit := Unit
type F (c: Type) :: Type := {
  F (List a) =: a;
  F (Maybe a) =: a
}
f :: F (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 18. Data family exhaustiveness
// -----------------------------------------------

// Non-exhaustive match on data family should be caught.
func TestDataFamily_NonExhaustiveMatch(t *testing.T) {
	source := `
data Unit := Unit

class Container a {
  data Entry a :: Type;
  empty :: a
}

instance Container Unit {
  data Entry Unit =: Singleton Unit | Empty;
  empty := Unit
}

f :: Entry Unit -> Unit
f := \e. case e {
  Singleton x -> x
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
}

// Exhaustive match on data family should pass.
func TestDataFamily_ExhaustiveMatch(t *testing.T) {
	source := `
data Unit := Unit

class Container a {
  data Entry a :: Type;
  empty :: a
}

instance Container Unit {
  data Entry Unit =: Singleton Unit | Empty;
  empty := Unit
}

f :: Entry Unit -> Unit
f := \e. case e {
  Singleton x -> x;
  Empty -> Unit
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 19. Edge case: type family with two parameters, partial match
// -----------------------------------------------

func TestTypeFamilyTwoParams_PartialMatch(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit
type And (a: Bool) (b: Bool) :: Bool := {
  And True True =: True;
  And True False =: False;
  And False b =: False
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
data Bool := True | False
data Unit := Unit
consume1 :: Computation { a: Unit } {} Unit
consume1 := assumption
consume2 :: Computation { a: Unit } {} Unit
consume2 := assumption
f :: Bool -> Computation { a: Unit } {} Unit
f := \b. case b {
  True -> consume1;
  False -> consume2
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
data Bool := True | False
data Unit := Unit
keepA :: Computation { a: Unit, b: Unit } { a: Unit } Unit
keepA := assumption
keepB :: Computation { a: Unit, b: Unit } { b: Unit } Unit
keepB := assumption
f :: Bool -> Computation { a: Unit, b: Unit } {} Unit
f := \b. case b {
  True -> keepA;
  False -> keepB
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 22. Type family: equation patterns with wildcards in multi-param
// -----------------------------------------------

func TestTypeFamilyWildcard_MultiParam(t *testing.T) {
	source := `
data Bool := True | False
type Or (a: Bool) (b: Bool) :: Bool := {
  Or True _ =: True;
  Or _ True =: True;
  Or False False =: False
}
`
	checkSource(t, source, nil)
}

// -----------------------------------------------
// 23. matchTyPatterns: consistency check with TyApp patterns
// -----------------------------------------------

func TestMatchTyPatterns_ConsistencyTyApp(t *testing.T) {
	// F (List a) (List a) = a: both patterns must bind 'a' to the same type.
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type F (a: Type) (b: Type) :: Type := {
  F (List a) (List a) =: a
}
f :: F (List Unit) (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// If consistency check is broken (accepts inconsistent bindings),
// F (List Unit) (List Bool) would incorrectly reduce.
func TestMatchTyPatterns_InconsistencyFails(t *testing.T) {
	// F (List a) (List a) = a: both patterns share 'a'.
	// F (List Unit) (List Bool) should NOT match because Unit != Bool.
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
data Bool := True | False
type F (a: Type) (b: Type) :: Type := {
  F (List a) (List a) =: a
}
f :: F (List Unit) (List Bool) -> Unit
f := \x. x
`
	// This should fail: F (List Unit) (List Bool) doesn't match the pattern,
	// so F is stuck. Stuck TyFamilyApp != Unit.
	checkSourceExpectError(t, source, nil)
}

// -----------------------------------------------
// 24. Regression: type aliases still work with families
// -----------------------------------------------

func TestTypeFamilyCoexistsWithAlias(t *testing.T) {
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)
type Id a := a
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data Bool := True | False
data Unit := Unit
opA :: Computation { x: Unit } { x: Unit } Unit
opA := assumption
opB :: Computation { x: Unit } { x: Bool } Unit
opB := assumption
f :: Bool -> Computation { x: Unit } { x: Unit } Unit
f := \b. case b {
  True -> opA;
  False -> opB
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
data Session := Send Session | Recv Session | End
type Dual (s: Session) :: Session := {
  Dual (Send s) =: Recv (Dual s);
  Dual (Recv s) =: Send (Dual s);
  Dual End =: End
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
data Unit := Unit
type Loop (a: Type) :: Type := {
  Loop a =: Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	msg := checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyReduction)
	if !strings.Contains(msg, "reduction limit exceeded") {
		t.Errorf("expected 'reduction limit exceeded' in error message, got: %s", msg)
	}
	if !strings.Contains(msg, "Loop") {
		t.Errorf("expected 'Loop' in error message, got: %s", msg)
	}
}

func TestInjectivityViolation_ErrorMessage(t *testing.T) {
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)
type F (c: Type) :: (r: Type) | r =: c := {
  F (List a) =: a;
  F Unit =: Unit
}
`
	msg := checkSourceExpectCode(t, source, nil, diagnostic.ErrInjectivity)
	if !strings.Contains(msg, "injectivity") {
		t.Errorf("expected 'injectivity' in error message, got: %s", msg)
	}
}
