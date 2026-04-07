// Type family reduction tests — reduceTyFamily, matchTyPattern, intersectCapRows,
// and lubPostStates mutation probes.
// Does NOT cover: declaration processing (type_family_reduction_decl_test.go),
//                 integration scenarios (type_family_reduction_integration_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

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
form Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Bool => Int;
  a => Bool
}
f :: \ c. F c -> F c
f := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	// Should compile: F c is stuck and unifies with itself.
	checkSource(t, source, config)
}

// Circular self-reference: cycle detected via sentinel memoization.
// The family remains stuck (unreduced), producing a type mismatch (E0200).

// Circular self-reference: cycle detected via sentinel memoization.
// The family remains stuck (unreduced), producing a type mismatch (E0200).
func TestReduceTyFamily_FuelCounterIncrements(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
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

// Empty type family — no equations, always stuck.
func TestReduceTyFamily_EmptyFamily(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
}
f :: \ c. F c -> F c
f := \x. x
`
	// F c should be stuck (no equations to match), but F c ~ F c should still unify.
	checkSource(t, source, nil)
}

// Single-equation type family.

// Single-equation type family.
func TestReduceTyFamily_SingleEquation(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  Unit => Unit
}
f :: F Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Type family applied to unsolved metavariable must be stuck.

// Type family applied to unsolved metavariable must be stuck.
func TestReduceTyFamily_StuckOnMeta(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
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

// Verify the first matching equation wins (not the last).
func TestReduceTyFamily_FirstMatchWins(t *testing.T) {
	// F Unit should reduce to Bool (first equation), not Int (second).
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  a => Bool;
  Unit => Int
}
f :: F Unit -> Bool
f := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

// Verify first match by using a more specific pattern first.

// Verify first match by using a more specific pattern first.
func TestReduceTyFamily_SpecificBeforeGeneral(t *testing.T) {
	// F Bool = Int, F a = Bool. F Bool should give Int.
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
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
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

// If TyVar "_" is NOT treated as wildcard, this test would fail.
func TestMatchTyPattern_WildcardUnderscoreBinds(t *testing.T) {
	// Two-param type family encoded via Pair.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
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

// If consistency check on repeated variables is wrong (returns matchSuccess
// when bindings are inconsistent), this would allow incorrect reduction.
func TestMatchTyPattern_ConsistentBindings(t *testing.T) {
	// F (Pair a a) = Unit should only match when both components are the same.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
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

// TyApp pattern matching: if we fail to decompose TyApp correctly,
// this test would fail.
func TestMatchTyPattern_TyAppDecomposition(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Unit := { Unit: Unit; }
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

// matchTyPattern with TyCon vs TyMeta should give matchIndeterminate.
func TestMatchTyPattern_TyConVsMeta(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Unit => Bool;
  Bool => Unit
}
f :: \ a. F a -> F a
f := \x. x
`
	checkSource(t, source, nil)
}

// If labels present in only SOME branches are kept, this test would fail
// because the joined post-state would claim cap 'a' exists in post when
// one branch consumed it.
func TestIntersectCapRows_DropsPartialLabels(t *testing.T) {
	// Branch 1: consumes 'a', keeps 'b' => post = { b: Unit }
	// Branch 2: keeps both => post = { a: Unit, b: Unit }
	// Intersection: only 'b' is shared => post = { b: Unit }
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
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

// If labels present in NO branches are kept (all consumed), post = {}.
func TestIntersectCapRows_AllConsumedDifferent(t *testing.T) {
	// Branch 1: consumes 'a', keeps 'b'
	// Branch 2: consumes 'b', keeps 'a'
	// Intersection: no label in ALL branches => post = {}
	source := `
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
	checkSource(t, source, nil)
}

// All branches consume all caps: intersection = full set (all labels shared).

// All branches consume all caps: intersection = full set (all labels shared).
func TestIntersectCapRows_AllBranchesSame(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
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

// Three-way branch: label must be in ALL three branches to survive.
func TestIntersectCapRows_ThreeWayBranch(t *testing.T) {
	source := `
form Color := { Red: Color; Green: Color; Blue: Color; }
form Unit := { Unit: Unit; }
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

// Single-branch case: lubPostStates should return that branch's post directly.
func TestLubPostStates_SingleBranch(t *testing.T) {
	source := `
form Unit := { MkUnit: (); }
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

// Non-computation case: lubPostStates should not be triggered, normal
// unification of result types applies.
func TestLubPostStates_NonCompResultType(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
f :: Bool -> Unit
f := \b. case b {
  True => Unit;
  False => Unit
}
`
	checkSource(t, source, nil)
}

// Case with empty post-states on both branches.

// Case with empty post-states on both branches.
func TestLubPostStates_BothEmpty(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
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
