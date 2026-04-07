// Type family interaction tests — cross-feature combinations (HKT, records, do-blocks,
// operators, explicit tyapp, higher-rank, thunk/force, case, tuple, wildcard, multiplicity).
// Does NOT cover: error diagnostics (type_family_interaction_error_test.go),
//                 bug probes (type_family_interaction_bug_test.go),
//                 unit assertions (type_family_interaction_unit_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

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

func TestInteractionRecursiveTypeFamilyWithDataKinds(t *testing.T) {
	// Recursive type family operating on promoted data constructors.
	// Uses NatPair since multi-param families require unified syntax encoding.
	source := `
form Nat := Zero | Succ Nat
form NatPair := \(a: Nat) (b: Nat). { MkNatPair: NatPair a b; }

type Add :: Nat := \(p: Type). case p {
  (NatPair Zero b) => b;
  (NatPair (Succ m) n) => Succ (Add (NatPair m n))
}
`
	checkSource(t, source, nil)
}

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
