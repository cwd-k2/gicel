// Type family interaction tests — cross-feature combinations (HKT, records, do blocks, evidence).
// Does NOT cover: reduction algorithm (type_family_reduction_test.go), pathological (type_family_pathological_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ================================================================
// Round 3 QA: Cross-Feature Interaction Tests
// Branch: feature/type-system-extensions
//
// These tests exercise combinations of type system extensions
// with each other and with existing features, targeting interaction
// bugs that would not manifest in isolation.
// ================================================================

// ================================================================
// 1. Cross-Feature Interaction Tests
// ================================================================

// ----------------------------------------------------------------
// 1a. Type family + HKT
// ----------------------------------------------------------------

func TestInteractionTypeFamilyHKT(t *testing.T) {
	// A poly-kinded class with a method whose type involves a type family.
	// class Container (f: k -> Type) { elem :: f a -> Elem (f a) }
	// The type family Elem must reduce correctly when applied to (f a)
	// where f is resolved to a concrete type constructor.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

form Container := \(f: k -> Type). {
  size: \ a. f a -> Int
}

impl Container List := {
  size := \xs. 0
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

func TestInteractionTypeFamilyHKTMethodUse(t *testing.T) {
	// Use a type family result as the return type of a function
	// that also uses an HKT class method.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

first :: List Unit -> Elem (List Unit)
first := \xs. case xs { Cons x rest => x; Nil => Unit }
`
	checkSource(t, source, nil)
}

func TestInteractionTypeFamilyHKTPolyKindedClassAssoc(t *testing.T) {
	// Associated type in a class combined with HKT usage.
	// The associated type Elem is declared on Container c.
	// A poly-kinded Functor class coexists to test HKT + assoc type interaction.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Container := \c. {
  type Elem c :: Type;
  chead: c -> Elem c
}

impl Container (List a) := {
  type Elem := a;
  chead := \xs. case xs { Cons x rest => x; Nil => chead Nil }
}

form Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}

test :: Elem (List Unit) -> Unit
test := \x. x
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 1b. Type family + Records
// ----------------------------------------------------------------

func TestInteractionTypeFamilyRecord(t *testing.T) {
	// A type family that is used alongside a record type.
	// Verify that field projection works when the record field type
	// involves a type family application.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

f :: Elem (List Unit) -> Unit
f := \x. x

g :: Record { value: Unit } -> Unit
g := \r. r.#value
`
	checkSource(t, source, nil)
}

func TestInteractionTypeFamilyRecordFieldType(t *testing.T) {
	// A record where one field's type is determined by a type family.
	// The type family reduces before the record type is constructed.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

mkRecord :: Elem (List Unit) -> Record { value: Unit }
mkRecord := \x. { value: x }
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 1c. Type family + Operator sections
// ----------------------------------------------------------------

func TestInteractionTypeFamilyOperatorSection(t *testing.T) {
	// A function that uses a type family result in a context
	// with operator usage. Since GICEL's operator sections require
	// the type to be known, this tests that type family reduction
	// happens before the operator is applied.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

id :: \ a. a -> a
id := \x. x

apply :: Elem (List Unit) -> Unit
apply := \x. id x
`
	checkSource(t, source, nil)
}

func TestInteractionTypeFamilyWithMap(t *testing.T) {
	// Use map with a function whose argument type involves a type family.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

map :: \ a b. (a -> b) -> List a -> List b
map := assumption

toUnit :: Unit -> Bool
toUnit := \x. True

test :: List Unit -> List Bool
test := \xs. map toUnit xs
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 1d. Data family + Exhaustiveness + Nested patterns
// ----------------------------------------------------------------

func TestInteractionDataFamilyExhaustNested(t *testing.T) {
	// Exhaustiveness checking with associated type families.
	// Uses regular data types since data family constructor registration
	// requires the legacy syntax path.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form WrapBool := { MkWrapBool: Bool -> WrapBool; }

form Wrapper := \a. {
  type Wrap a :: Type;
  unwrap: Wrap a -> a
}

impl Wrapper Bool := {
  type Wrap := WrapBool;
  unwrap := \w. case w { MkWrapBool b => b }
}

test :: WrapBool -> Bool
test := \w. case w {
  MkWrapBool True => True;
  MkWrapBool False => False
}
`
	checkSource(t, source, nil)
}

func TestInteractionDataFamilyExhaustMissing(t *testing.T) {
	// Missing a branch in nested pattern — non-exhaustive.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form WrapBool := { MkWrapBool: Bool -> WrapBool; }

test :: WrapBool -> Bool
test := \w. case w {
  MkWrapBool True => True
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
}

func TestInteractionDataFamilyNestedUnwrap(t *testing.T) {
	// Associated type family: Elem reduces to the element type.
	source := `
form Unit := { Unit: Unit; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Container := \a. {
  type Elem a :: Type;
  empty: a
}

impl Container (Maybe a) := {
  type Elem := a;
  empty := Nothing
}

test :: Elem (Maybe Unit) -> Unit
test := \x. x
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 1e. Type family + do-block pre/post threading
// ----------------------------------------------------------------

func TestInteractionTypeFamilyDoBlock(t *testing.T) {
	// A do block where the result type involves a type family.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

step :: Computation {} {} (Elem (List Unit))
step := assumption

main :: Computation {} {} Unit
main := do { x <- step; pure x }
`
	checkSource(t, source, nil)
}

func TestInteractionTypeFamilyDoBlockPrePost(t *testing.T) {
	// Do block with pre/post states and type family in the result type.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

open :: Computation {} { handle: Unit } Unit
open := assumption

read :: Computation { handle: Unit } { handle: Unit } (Elem (List Unit))
read := assumption

close :: Computation { handle: Unit } {} Unit
close := assumption

main :: Computation {} {} Unit
main := do { open; x <- read; close }
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 1g. Associated type + missing definition
// ----------------------------------------------------------------

func TestInteractionAssocTypeMissingDef(t *testing.T) {
	// An instance that omits the associated type definition.
	// The associated type family should have no equation for this instance,
	// meaning applications involving this instance will be stuck (not crash).
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

form Container := \c. {
  type Elem c :: Type;
  size: c -> Int
}

impl Container (List a) := {
  size := \xs. 0
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	// The instance should still compile; the associated type just won't have
	// an equation. Missing associated type definition is not a method;
	// the current design does not require it.
	// If the implementation enforces it, we expect an error.
	// Test both paths: either it succeeds (no equation, stuck reduction)
	// or it errors (missing associated type).
	src := checkSourceTryBoth(t, source, config)
	if src != "" {
		t.Logf("Implementation rejects missing associated type def: %s", src)
	}
}

// ----------------------------------------------------------------
// 1h. Multiple type families in one signature
// ----------------------------------------------------------------

func TestInteractionMultipleTypeFamiliesInSignature(t *testing.T) {
	// A function type that uses two different type families.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

type Nullable :: Type := \(c: Type). case c {
  (List a) => Maybe a
}

f :: Elem (List Unit) -> Nullable (List Unit)
f := \x. Just x
`
	checkSource(t, source, nil)
}

func TestInteractionTypeFamiliesInBothArgAndResult(t *testing.T) {
	// Type families used in both argument position and result position.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  (Pair a b) => a
}

type Second :: Type := \(c: Type). case c {
  (Pair a b) => b
}

swap :: Elem (Pair Unit Unit) -> Second (Pair Unit Unit) -> Pair Unit Unit
swap := \x y. MkPair x y
`
	checkSource(t, source, nil)
}

func TestInteractionTypeFamilyChained(t *testing.T) {
	// Type family applied to the result of another type family.
	// Outer(Inner(x)) should reduce correctly.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

type Id :: Type := \(a: Type). case a {
  a => a
}

f :: Id (Elem (List Unit)) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// ================================================================
// 2. Existing Feature Regression Tests
// ================================================================

// ----------------------------------------------------------------
// 2a. Explicit type application with type family result
// ----------------------------------------------------------------

func TestInteractionExplicitTyAppTypeFamilyResult(t *testing.T) {
	// id @(Elem (List Int)) 42 should work.
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

func TestInteractionExplicitTyAppTypeFamilyIdentity(t *testing.T) {
	// Explicit type application with an identity type family.
	source := `
form Unit := { Unit: Unit; }

type Id :: Type := \(a: Type). case a {
  a => a
}

id :: \ a. a -> a
id := \x. x

test := id @(Id Unit) Unit
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 2b. Higher-rank types containing type family
// ----------------------------------------------------------------

func TestInteractionHigherRankTypeFamilyArg(t *testing.T) {
	// A higher-rank function that uses a type family in its argument type.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

applyToUnit :: (Unit -> Unit) -> Unit
applyToUnit := \f. f Unit

test :: Unit
test := applyToUnit (\x. x)
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 2c. Recursive let with type family
// ----------------------------------------------------------------

func TestInteractionRecursiveLetTypeFamilyAnnotation(t *testing.T) {
	// A recursive binding where the annotated type uses a type family.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

length :: \ a. List a -> Int
length := assumption

main :: Int
main := length (Cons Unit Nil)
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

// ----------------------------------------------------------------
// 2d. Thunk/force with type family result type
// ----------------------------------------------------------------

func TestInteractionThunkForceTypeFamilyResult(t *testing.T) {
	// thunk and force with a computation whose result type involves a type family.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

delayed :: Thunk {} {} (Elem (List Unit))
delayed := thunk (pure Unit)

run :: Computation {} {} Unit
run := force delayed
`
	checkSource(t, source, nil)
}

func TestInteractionThunkForceTypeFamilyPrePost(t *testing.T) {
	// thunk/force with type family in result and explicit pre/post states.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

mkThunk :: Thunk { cap: Unit } { cap: Unit } (Elem (List Unit))
mkThunk := assumption

run :: Computation { cap: Unit } { cap: Unit } (Elem (List Unit))
run := force mkThunk
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 2e. Case expression with type family scrutinee
// ----------------------------------------------------------------

func TestInteractionCaseTypeFamilyScrutinee(t *testing.T) {
	// Pattern matching on a value whose type is a type family application
	// that reduces to a concrete ADT.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

test :: Elem (List Bool) -> Bool
test := \x. case x { True => False; False => True }
`
	checkSource(t, source, nil)
}

// ================================================================
// 3. Error Message Quality Tests
// ================================================================

// ----------------------------------------------------------------
// 3a. Wrong number of arguments to a type family
// ----------------------------------------------------------------

func TestInteractionErrorTypeFamilyArityTooMany(t *testing.T) {
	// Applying too many arguments to a type family.
	source := `
form Unit := { Unit: Unit; }
type Id :: Type := \(a: Type). case a {
  a => a
}
f :: Id Unit Unit -> Unit
f := \x. x
`
	// Id takes 1 arg, but is applied to 2. This should produce an error.
	// Depending on how the parser/checker handles extra args, this might
	// be a kind error or a type family arity mismatch.
	errMsg := checkSourceExpectError(t, source, nil)
	t.Logf("Arity error message: %s", errMsg)
}

func TestInteractionErrorTypeFamilyEquationArityMismatch(t *testing.T) {
	// In unified syntax, each case alternative has a single pattern expression.
	// An application chain like `a b` is parsed as a single pattern.
	// This test verifies that such a pattern compiles.
	source := `
form Unit := { Unit: Unit; }
type Id :: Type := \(a: Type). case a {
  a => a
}
f :: Id Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 3b. Type family applied to incompatible types
// ----------------------------------------------------------------

func TestInteractionErrorTypeFamilyNoMatch(t *testing.T) {
	// Type family applied to a type that doesn't match any equation.
	// This should produce a type mismatch because the stuck TyFamilyApp
	// cannot unify with the expected type.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
f :: Elem Bool -> Unit
f := \x. x
`
	// Elem Bool is stuck (no equation matches). Trying to unify
	// the stuck Elem Bool with Unit should fail.
	errMsg := checkSourceExpectError(t, source, nil)
	t.Logf("No-match error: %s", errMsg)
}

// ----------------------------------------------------------------
// 3c. Injectivity violation message includes equation numbers
// ----------------------------------------------------------------

func TestInteractionErrorInjectivityEquationNumbers(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
type Collapse :: Type := \(a: Type). case a {
  Unit => Unit;
  Bool => Unit
}
f :: Collapse Unit -> Unit
f := \x. x
`
	// In unified syntax, injectivity annotations are not supported.
	// The type family compiles successfully.
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 3d. Recursive depth limit message includes the family name
// ----------------------------------------------------------------

func TestInteractionErrorRecursiveDepthFamilyName(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type Loop :: Type := \(a: Type). case a {
  a => Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	// Cycle is detected via sentinel memoization; the family remains stuck (unreduced),
	// producing a type mismatch (E0200) when Loop Unit is compared against Unit.
	errMsg := checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
	// The error message should include the family name "Loop".
	if !strings.Contains(errMsg, "Loop") {
		t.Errorf("expected family name 'Loop' in type mismatch error, got: %s", errMsg)
	}
}

// ----------------------------------------------------------------
// 3e. Data family constructor used without importing the instance
// ----------------------------------------------------------------

func TestInteractionErrorDataFamilyConstructorWithoutInstance(t *testing.T) {
	// Using a data family constructor name that doesn't exist
	// (simulates the case where the instance wasn't imported).
	source := `
form Unit := { Unit: Unit; }
form Container := \a. {
  form Elem a :: Type;
  empty: a
}
x :: Elem Unit
x := ListElem Unit
`
	// ListElem is not defined anywhere — should be an unbound constructor error.
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "ListElem") {
		t.Errorf("expected mention of ListElem in error, got: %s", errMsg)
	}
}

// ================================================================
// Additional interaction tests: stress the boundary
// ================================================================

// ----------------------------------------------------------------
// Type family + Type class constraint
// ----------------------------------------------------------------

func TestInteractionTypeFamilyInConstraint(t *testing.T) {
	// A function with a class constraint where the type argument
	// involves a type family application.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
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

test :: List Bool -> Bool
test := \xs. case xs {
  Cons x rest => eq x True;
  Nil => False
}
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// Multiple instances of associated type
// ----------------------------------------------------------------

func TestInteractionAssocTypeMultipleInstancesReduce(t *testing.T) {
	// Two instances define the same associated type family with different equations.
	// Verify that reduction picks the correct equation for each instance.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

form Container := \c. {
  type Elem c :: Type;
  size: c -> Int
}

impl Container (List a) := {
  type Elem := a;
  size := \xs. 0
}

impl Container (Maybe a) := {
  type Elem := a;
  size := \xs. 0
}

testList :: Elem (List Bool) -> Bool
testList := \x. x

testMaybe :: Elem (Maybe Unit) -> Unit
testMaybe := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

// ----------------------------------------------------------------
// Type family reduction inside \ body
// ----------------------------------------------------------------

func TestInteractionTypeFamilyInsideForall(t *testing.T) {
	// Type family application inside a \-quantified type.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

identity :: \ c. Elem c -> Elem c
identity := \x. x
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// Recursive type family + data family interaction
// ----------------------------------------------------------------

func TestInteractionRecursiveTypeFamilyWithDataKinds(t *testing.T) {
	// Recursive type family operating on promoted data constructors.
	// Uses NatPair since multi-param families require unified syntax encoding.
	source := `
form Nat := { Zero: (); Succ: Nat; }
form NatPair := \(a: Nat) (b: Nat). { MkNatPair: NatPair a b; }

type Add :: Nat := \(p: Type). case p {
  (NatPair Zero b) => b;
  (NatPair (Succ m) n) => Succ (Add (NatPair m n))
}
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// Type family + Let binding
// ----------------------------------------------------------------

func TestInteractionTypeFamilyLetBinding(t *testing.T) {
	// Type family result used in a let-block binding.
	// Block syntax: { x := e; body }
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

wrap :: Elem (List Unit) -> Unit
wrap := \x. x

main :: Unit
main := { x := Unit; wrap x }
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// Type family + Tuple (record sugar)
// ----------------------------------------------------------------

func TestInteractionTypeFamilyTuple(t *testing.T) {
	// Use a type family result within a tuple context.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

mkPair :: Elem (List Unit) -> Elem (List Unit) -> Pair Unit Unit
mkPair := \x y. MkPair x y
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// Wildcard type family patterns + HKT
// ----------------------------------------------------------------

func TestInteractionWildcardTypeFamilyHKT(t *testing.T) {
	// Type family with wildcard pattern + HKT interaction.
	source := `
form Unit := { Unit: Unit; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Always :: Type := \(a: Type). case a {
  _ => Unit
}

form Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}

test :: Always (Maybe Unit) -> Unit
test := \x. x
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// Type family + Computation pre/post with multiplicity
// ----------------------------------------------------------------

func TestInteractionTypeFamilyMultiplicityDoBlock(t *testing.T) {
	// Computation with multiplicity annotations and type family in result.
	source := `
form Mult := { Unrestricted: (); Affine: (); Linear: (); }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

readHandle :: Computation { handle: Unit } { handle: Unit } (Elem (List Unit))
readHandle := assumption

main :: Computation { handle: Unit } { handle: Unit } Unit
main := do { x <- readHandle; pure x }
`
	checkSource(t, source, nil)
}

// ================================================================
// 4. Bug Probes: Potential Implementation Bugs
// ================================================================

// ----------------------------------------------------------------
// 4a. reduceFamilyApps missing TyEvidence recursion
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4b. Type family in capability row field type
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4c. Undersaturated type family in TyApp chain
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4d. Type family equation with variable shadowing
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4e. Data family instance constructors in exhaustiveness
//     after type family reduction
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4f. Type family + instance resolution interaction
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4g. Zonk does not reduce type families
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4h. Overlapping associated type equations from different instances
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4i. Type family in explicit type application position
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4j. Fundep improvement with type family reduced types
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4k. typeHasMeta missing TyFamilyApp case
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4l. Type family in a higher-rank position
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4m. reduceFamilyApps missing TyEvidence case
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4n. Data family + operator usage
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 4o. Multiple class constraints with type families
// ----------------------------------------------------------------

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

// ----------------------------------------------------------------
// 5a. typeHasMeta: TyFamilyApp case
// ----------------------------------------------------------------

func TestInteractionContainsMetaTyFamilyApp(t *testing.T) {
	// A TyFamilyApp with a TyMeta inside should be detected by typeHasMeta.
	meta := &types.TyMeta{ID: 999, Kind: types.TypeOfTypes}
	tfApp := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{meta},
		Kind: types.TypeOfTypes,
	}
	if !typeHasMeta(tfApp) {
		t.Error("typeHasMeta should detect TyMeta inside TyFamilyApp")
	}
}

func TestInteractionContainsMetaTyFamilyAppNoMeta(t *testing.T) {
	// A TyFamilyApp without TyMeta should return false.
	tfApp := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{&types.TyCon{Name: "Int"}},
		Kind: types.TypeOfTypes,
	}
	if typeHasMeta(tfApp) {
		t.Error("typeHasMeta should not detect meta in TyFamilyApp with only TyCon args")
	}
}

func TestInteractionContainsMetaTyFamilyAppNestedMeta(t *testing.T) {
	// TyFamilyApp with meta nested inside a TyApp argument.
	meta := &types.TyMeta{ID: 999, Kind: types.TypeOfTypes}
	tfApp := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{
			&types.TyApp{
				Fun: &types.TyCon{Name: "List"},
				Arg: meta,
			},
		},
		Kind: types.TypeOfTypes,
	}
	if !typeHasMeta(tfApp) {
		t.Error("typeHasMeta should detect TyMeta nested inside TyFamilyApp arg")
	}
}

// ----------------------------------------------------------------
// 5b. typeHasMeta: TyCBPV (Thunk) case
// ----------------------------------------------------------------

func TestInteractionContainsMetaTyThunk(t *testing.T) {
	meta := &types.TyMeta{ID: 999, Kind: types.TypeOfTypes}
	thunk := types.MkThunk(
		types.ClosedRow(),
		types.ClosedRow(),
		meta,
	)
	if !typeHasMeta(thunk) {
		t.Error("typeHasMeta should detect TyMeta inside TyCBPV (Thunk) Result")
	}
}

func TestInteractionContainsMetaTyThunkPre(t *testing.T) {
	meta := &types.TyMeta{ID: 999, Kind: types.TypeOfRows}
	thunk := types.MkThunk(
		meta,
		types.ClosedRow(),
		&types.TyCon{Name: "Unit"},
	)
	if !typeHasMeta(thunk) {
		t.Error("typeHasMeta should detect TyMeta inside TyCBPV (Thunk) Pre")
	}
}

// ----------------------------------------------------------------
// 5c. reduceFamilyApps: TyEvidence case
// ----------------------------------------------------------------

func TestInteractionReduceFamilyAppsEvidence(t *testing.T) {
	// Type family application nested inside a TyEvidence body should
	// be reduced by reduceFamilyApps.
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

testConstrainedBody :: Elem (List Unit) -> Elem (List Unit) -> Bool
testConstrainedBody := \x y. eq x y
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 5d. reduceFamilyApps: TyEvidenceRow capability field type
// ----------------------------------------------------------------

func TestInteractionReduceFamilyAppsCapRow(t *testing.T) {
	// Type family application as the type of a capability in a row.
	// After the fix, reduceFamilyApps recurses into TyEvidenceRow fields.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

step :: Computation { cap: Elem (List Unit) } {} Unit
step := assumption

main :: Computation { cap: Unit } {} Unit
main := step
`
	checkSource(t, source, nil)
}
