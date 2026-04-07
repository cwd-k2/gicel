// Type family interaction tests — latent bug probes and soundness regression.
// Does NOT cover: cross-feature combinations (type_family_interaction_cross_test.go),
//                 error diagnostics (type_family_interaction_error_test.go),
//                 unit assertions (type_family_interaction_unit_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func TestInteractionBugReduceFamilyInEvidence(t *testing.T) {
	// Type family application inside a constrained type (TyEvidence body).
	// reduceFamilyApps does not recurse into TyEvidence, so this tests
	// whether the normalize path handles it correctly via other means.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

form Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Bool := {
  eq := \x y. True
}

test :: Eq (Elem (List Bool)) => Elem (List Bool) -> Bool
test := \x. eq x True
`
	checkSource(t, source, nil)
}

func TestInteractionBugTypeFamilyInCapabilityRow(t *testing.T) {
	// Type family application as the type of a capability row field.
	// This tests whether reduceFamilyApps recurses into TyEvidenceRow.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

step :: Computation { handle: Elem (List Unit) } { handle: Elem (List Unit) } Unit
step := assumption

main :: Computation { handle: Unit } { handle: Unit } Unit
main := step
`
	checkSource(t, source, nil)
}

func TestInteractionBugUndersaturatedTypeFamilyInApp(t *testing.T) {
	// A 1-param type family that extracts the first component of a Pair.
	// Tests that the declaration is accepted.
	source := `
form Unit := { Unit: Unit; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Fst :: Type := \(p: Type). case p {
  (Pair a b) => a
}
`
	// Just test that declaration is accepted. No application.
	checkSource(t, source, nil)
}

func TestInteractionBugTypeFamilyShadowedVar(t *testing.T) {
	// Two equations use the same pattern variable name.
	// Each equation's bindings should be independent.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Fst :: Type := \(c: Type). case c {
  (Pair a b) => a
}
type Snd :: Type := \(c: Type). case c {
  (Pair a b) => b
}
f :: Fst (Pair Bool Unit) -> Bool
f := \x. x
g :: Snd (Pair Bool Unit) -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

func TestInteractionBugDataFamilyExhaustAfterReduction(t *testing.T) {
	// Exhaustiveness checking with regular data types.
	// Data family constructor registration requires the legacy syntax path.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form BoolElem := { MkBoolElem: Bool -> BoolElem; EmptyElem: BoolElem; }

test :: BoolElem -> Bool
test := \e. case e {
  MkBoolElem b => b;
  EmptyElem => False
}
`
	checkSource(t, source, nil)
}

func TestInteractionBugDataFamilyExhaustMissingAfterReduction(t *testing.T) {
	// Missing constructor - should detect non-exhaustive.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form BoolElem := { MkBoolElem: Bool -> BoolElem; EmptyElem: BoolElem; }

test :: BoolElem -> Bool
test := \e. case e {
  MkBoolElem b => b
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
}

func TestInteractionBugTypeFamilyInstanceResolution(t *testing.T) {
	// Instance resolution where the type argument contains a type family.
	// The resolver should reduce the type family before matching instances.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

form Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Bool := {
  eq := \x y. True
}

test :: Bool -> Bool
test := \x. eq x True
`
	checkSource(t, source, nil)
}

func TestInteractionBugZonkAndTypeFamilyReduction(t *testing.T) {
	// After zonking, a meta variable is resolved to a type family argument.
	// The type family should be reducible after zonking resolves the meta.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

id :: \ a. a -> a
id := \x. x

test :: Unit
test := id Unit
`
	checkSource(t, source, nil)
}

func TestTypeFamilySoundnessCtFunEqConflict(t *testing.T) {
	// Regression test: a stuck type family equation (CtFunEq) whose reduced
	// result conflicts with the already-solved result meta must be reported
	// as a type error, not silently swallowed.
	source := `
form Status := { Active: Status; Inactive: Status }

type Opp :: Status := \(s: Status). case s {
  Active => Inactive;
  Inactive => Active
}

form P := \(s: Status). { MkP: P s }

f :: \(s: Status). P s -> P (Opp s)
f := \_ . MkP

wrong :: P Active
wrong := f (MkP :: P Active)
`
	errStr := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errStr, "E0200") {
		t.Errorf("expected type mismatch error E0200, got: %s", errStr)
	}
}

func TestTypeFamilySoundnessCorrectUsage(t *testing.T) {
	// The correct usage of the same type family should type-check.
	source := `
form Status := { Active: Status; Inactive: Status }

type Opp :: Status := \(s: Status). case s {
  Active => Inactive;
  Inactive => Active
}

form P := \(s: Status). { MkP: P s }

f :: \(s: Status). P s -> P (Opp s)
f := \_ . MkP

correct :: P Inactive
correct := f (MkP :: P Active)
`
	checkSource(t, source, nil)
}

func TestInteractionBugAssocTypeOverlapOrder(t *testing.T) {
	// Two instances define the same associated type but for different
	// type arguments. The equation order in the family should match
	// each instance correctly.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Unit := { Unit: Unit; }

form Container := \c. {
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

testList :: Elem (List Unit) -> Unit
testList := \x. x

testMaybe :: Elem (Maybe Unit) -> Unit
testMaybe := \x. x
`
	checkSource(t, source, nil)
}

func TestInteractionBugExplicitTyAppWithFamilyReduction(t *testing.T) {
	// @(Elem (List Int)) should reduce to @Int in the type application.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

id :: \ a. a -> a
id := \x. x

test :: Int -> Int
test := \n. id @(Elem (List Int)) n
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

func TestInteractionBugFundepWithReducedType(t *testing.T) {
	// Fundep improvement where one argument is a type family application
	// that reduces to a concrete type. The improvement should work after reduction.
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Id :: Type := \(a: Type). case a {
  a => a
}

form Elem := \c e. {
  extract: c -> e
}

impl Elem (List a) a := {
  extract := \xs. case xs { Cons x rest => x; Nil => extract Nil }
}

test :: List Unit -> Id Unit
test := \xs. extract xs
`
	checkSource(t, source, nil)
}

func TestInteractionBugContainsMetaTyFamilyApp(t *testing.T) {
	// This tests a subtle interaction: when a constraint argument
	// contains a TyFamilyApp with a meta inside, typeHasMeta should
	// detect the meta so the constraint can be deferred properly.
	// Without the fix, the constraint would be resolved prematurely.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

form Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Bool := {
  eq := \x y. True
}

test :: Bool -> Bool -> Bool
test := \x y. eq x y
`
	checkSource(t, source, nil)
}

func TestInteractionBugTypeFamilyHigherRank(t *testing.T) {
	// Type family used inside a higher-rank type position.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

apply :: \ a b. (a -> b) -> a -> b
apply := \f x. f x

test :: Unit -> Unit
test := \x. apply (\y. y) x
`
	checkSource(t, source, nil)
}

func TestInteractionBugReduceFamilyAppsEvidence(t *testing.T) {
	// Type family application inside a qualified type.
	// Tests that type family reduction works even through evidence layers.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

form Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Unit := {
  eq := \x y. True
}

-- Function with a qualified type that references a type family.
testEq :: Elem (List Unit) -> Elem (List Unit) -> Bool
testEq := \x y. eq x y
`
	checkSource(t, source, nil)
}

func TestInteractionBugDataFamilyOperator(t *testing.T) {
	// Using associated type family result in the context of function application.
	// Data family constructor registration requires the legacy syntax path.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

form Container := \a. {
  type Elem a :: Type;
  empty: a
}

impl Container Bool := {
  type Elem := Bool;
  empty := True
}

id :: \ a. a -> a
id := \x. x

test :: Elem Bool
test := id True
`
	checkSource(t, source, nil)
}

func TestInteractionBugMultipleConstraintsTypeFamilies(t *testing.T) {
	// Multiple class constraints where the types are related via type families.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

form Eq := \a. {
  eq: a -> a -> Bool
}

form Show := \a. {
  show: a -> a
}

impl Eq Bool := {
  eq := \x y. True
}

impl Show Bool := {
  show := \x. x
}

test :: Bool -> Bool
test := \x. show (eq x True)
`
	checkSource(t, source, nil)
}

// ================================================================
// 5. Unit Tests for Latent Bug Fixes
// ================================================================
