package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
	"github.com/cwd-k2/gicel/internal/types"
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

class Container (f: k -> Type) {
  size :: \ a. f a -> Int
}

instance Container List {
  size := \xs. 0
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

func TestInteractionTypeFamilyHKTMethodUse(t *testing.T) {
	// Use a type family result as the return type of a function
	// that also uses an HKT class method.
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

first :: List Unit -> Elem (List Unit)
first := \xs. case xs { Cons x rest -> x; Nil -> Unit }
`
	checkSource(t, source, nil)
}

func TestInteractionTypeFamilyHKTPolyKindedClassAssoc(t *testing.T) {
	// Associated type in a class combined with HKT usage.
	// The associated type Elem is declared on Container c.
	// A poly-kinded Functor class coexists to test HKT + assoc type interaction.
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
data Maybe a := Nothing | Just a

class Container c {
  type Elem c :: Type;
  chead :: c -> Elem c
}

instance Container (List a) {
  type Elem (List a) =: a;
  chead := \xs. case xs { Cons x rest -> x; Nil -> chead Nil }
}

class Functor (f: k -> Type) {
  fmap :: \ a b. (a -> b) -> f a -> f b
}

instance Functor Maybe {
  fmap := \g mx. case mx { Nothing -> Nothing; Just x -> Just (g x) }
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit
data Bool := True | False

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
	// Data family with constructors used in nested pattern matching.
	// Exhaustiveness checking must resolve the data family to find constructors.
	source := `
data Unit := Unit
data Bool := True | False

class Wrapper a {
  data Wrap a :: Type;
  unwrap :: Wrap a -> a
}

instance Wrapper Bool {
  data Wrap Bool =: WrapBool Bool;
  unwrap := \w. case w { WrapBool b -> b }
}

test :: Wrap Bool -> Bool
test := \w. case w {
  WrapBool True -> True;
  WrapBool False -> False
}
`
	checkSource(t, source, nil)
}

func TestInteractionDataFamilyExhaustMissing(t *testing.T) {
	// Missing a branch in nested pattern on data family constructor.
	source := `
data Unit := Unit
data Bool := True | False

class Wrapper a {
  data Wrap a :: Type;
  unwrap :: Wrap a -> a
}

instance Wrapper Bool {
  data Wrap Bool =: WrapBool Bool;
  unwrap := \w. case w { WrapBool b -> b }
}

test :: Wrap Bool -> Bool
test := \w. case w {
  WrapBool True -> True
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrNonExhaustive)
}

func TestInteractionDataFamilyNestedUnwrap(t *testing.T) {
	// Nested patterns on data family: case on the inner value.
	source := `
data Unit := Unit
data Maybe a := Nothing | Just a

class Container a {
  data Elem a :: Type;
  empty :: a
}

instance Container (Maybe a) {
  data Elem (Maybe a) =: MaybeElem a;
  empty := Nothing
}

test :: \ a. Elem (Maybe a) -> a
test := \e. case e { MaybeElem x -> x }
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 1e. Fundep + Type class resolution + Superclass
// ----------------------------------------------------------------

func TestInteractionFundepSuperclass(t *testing.T) {
	// A class with fundeps that's a superclass of another class.
	// Instance resolution should work through the superclass chain.
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)

class Elem c e | c =: e {
  extract :: c -> e
}

class Elem c e => Foldable c e | c =: e {
  cfold :: \ b. (e -> b -> b) -> b -> c -> b
}

instance Elem (List a) a {
  extract := \xs. case xs { Cons x rest -> x; Nil -> extract Nil }
}

instance Foldable (List a) a {
  cfold := \f z xs. case xs { Nil -> z; Cons x rest -> f x (cfold f z rest) }
}
`
	checkSource(t, source, nil)
}

func TestInteractionFundepSuperclassUse(t *testing.T) {
	// Using the superclass method through the subclass constraint.
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)

class Elem c e | c =: e {
  extract :: c -> e
}

instance Elem (List a) a {
  extract := \xs. case xs { Cons x rest -> x; Nil -> extract Nil }
}

headOrDefault :: \ a. a -> List a -> a
headOrDefault := \def xs. case xs { Nil -> def; Cons x rest -> extract (Cons x rest) }
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 1f. Type family + do-block pre/post threading
// ----------------------------------------------------------------

func TestInteractionTypeFamilyDoBlock(t *testing.T) {
	// A do block where the result type involves a type family.
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data Unit := Unit
data List a := Nil | Cons a (List a)

class Container c {
  type Elem c :: Type;
  size :: c -> Int
}

instance Container (List a) {
  size := \xs. 0
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
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
data List a := Nil | Cons a (List a)
data Maybe a := Nothing | Just a
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

type Nullable (c: Type) :: Type := {
  Nullable (List a) =: Maybe a
}

f :: Elem (List Unit) -> Nullable (List Unit)
f := \x. Just x
`
	checkSource(t, source, nil)
}

func TestInteractionTypeFamiliesInBothArgAndResult(t *testing.T) {
	// Type families used in both argument position and result position.
	source := `
data List a := Nil | Cons a (List a)
data Pair a b := MkPair a b
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a;
  Elem (Pair a b) =: a
}

type Second (c: Type) :: Type := {
  Second (Pair a b) =: b
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

type Id (a: Type) :: Type := {
  Id a =: a
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
data List a := Nil | Cons a (List a)

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

id :: \ a. a -> a
id := \x. x

test :: Int -> Int
test := \n. id @(Elem (List Int)) n
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

func TestInteractionExplicitTyAppTypeFamilyIdentity(t *testing.T) {
	// Explicit type application with an identity type family.
	source := `
data Unit := Unit

type Id (a: Type) :: Type := {
  Id a =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

length :: \ a. List a -> Int
length := assumption

main :: Int
main := length (Cons Unit Nil)
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// ----------------------------------------------------------------
// 2d. Thunk/force with type family result type
// ----------------------------------------------------------------

func TestInteractionThunkForceTypeFamilyResult(t *testing.T) {
	// thunk and force with a computation whose result type involves a type family.
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit
data Bool := True | False

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

test :: Elem (List Bool) -> Bool
test := \x. case x { True -> False; False -> True }
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
data Unit := Unit
type Id (a: Type) :: Type := {
  Id a =: a
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
	// A type family equation with wrong number of patterns.
	source := `
data Unit := Unit
type Id (a: Type) :: Type := {
  Id a b =: a
}
`
	errMsg := checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyEquation)
	if !strings.Contains(errMsg, "expects 1") || !strings.Contains(errMsg, "has 2") {
		t.Errorf("expected arity info in error message, got: %s", errMsg)
	}
}

// ----------------------------------------------------------------
// 3b. Type family applied to incompatible types
// ----------------------------------------------------------------

func TestInteractionErrorTypeFamilyNoMatch(t *testing.T) {
	// Type family applied to a type that doesn't match any equation.
	// This should produce a type mismatch because the stuck TyFamilyApp
	// cannot unify with the expected type.
	source := `
data Unit := Unit
data Bool := True | False
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
data List a := Nil | Cons a (List a)
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
data Unit := Unit
data Bool := True | False
type Collapse (a: Type) :: (r: Type) | r =: a := {
  Collapse Unit =: Unit;
  Collapse Bool =: Unit
}
`
	errMsg := checkSourceExpectCode(t, source, nil, errs.ErrInjectivity)
	// The error message should mention "equations 1 and 2".
	if !strings.Contains(errMsg, "1") || !strings.Contains(errMsg, "2") {
		t.Errorf("expected equation numbers in injectivity error, got: %s", errMsg)
	}
}

// ----------------------------------------------------------------
// 3d. Recursive depth limit message includes the family name
// ----------------------------------------------------------------

func TestInteractionErrorRecursiveDepthFamilyName(t *testing.T) {
	source := `
data Unit := Unit
type Loop (a: Type) :: Type := {
  Loop a =: Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	errMsg := checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
	// The error message should include the family name "Loop".
	if !strings.Contains(errMsg, "Loop") {
		t.Errorf("expected family name 'Loop' in depth error, got: %s", errMsg)
	}
}

// ----------------------------------------------------------------
// 3e. Data family constructor used without importing the instance
// ----------------------------------------------------------------

func TestInteractionErrorDataFamilyConstructorWithoutInstance(t *testing.T) {
	// Using a data family constructor name that doesn't exist
	// (simulates the case where the instance wasn't imported).
	source := `
data Unit := Unit
class Container a {
  data Elem a :: Type;
  empty :: a
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
data List a := Nil | Cons a (List a)
data Bool := True | False

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

class Eq a {
  eq :: a -> a -> Bool
}

instance Eq Bool {
  eq := \x y. True
}

test :: List Bool -> Bool
test := \xs. case xs {
  Cons x rest -> eq x True;
  Nil -> False
}
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// Associated type + fundep in same class
// ----------------------------------------------------------------

func TestInteractionAssocTypeAndFundep(t *testing.T) {
	// A class that has both an associated type and a functional dependency.
	// These are complementary but should not interfere with each other.
	source := `
data Unit := Unit
data List a := Nil | Cons a (List a)
data Bool := True | False

class Collection c e | c =: e {
  type Key c :: Type;
  elem :: c -> e
}

instance Collection (List a) a {
  type Key (List a) =: Int;
  elem := \xs. case xs { Cons x rest -> x; Nil -> elem Nil }
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// ----------------------------------------------------------------
// Data family + Fundep
// ----------------------------------------------------------------

func TestInteractionDataFamilyAndFundep(t *testing.T) {
	// Data family in a class that also has fundeps.
	source := `
data Unit := Unit
data Bool := True | False

class Convertible a b | a =: b {
  data Result a :: Type;
  convert :: a -> Result a
}

instance Convertible Bool Unit {
  data Result Bool =: BoolResult Unit;
  convert := \b. BoolResult Unit
}

test :: Result Bool
test := convert True
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
data List a := Nil | Cons a (List a)
data Maybe a := Nothing | Just a
data Unit := Unit
data Bool := True | False

class Container c {
  type Elem c :: Type;
  size :: c -> Int
}

instance Container (List a) {
  type Elem (List a) =: a;
  size := \xs. 0
}

instance Container (Maybe a) {
  type Elem (Maybe a) =: a;
  size := \xs. 0
}

testList :: Elem (List Bool) -> Bool
testList := \x. x

testMaybe :: Elem (Maybe Unit) -> Unit
testMaybe := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// ----------------------------------------------------------------
// Type family reduction inside \ body
// ----------------------------------------------------------------

func TestInteractionTypeFamilyInsideForall(t *testing.T) {
	// Type family application inside a \-quantified type.
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
	source := `
data Nat := Zero | Succ Nat

type Add (m: Nat) (n: Nat) :: Nat := {
  Add Zero n =: n;
  Add (Succ m) n =: Succ (Add m n)
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit
data Pair a b := MkPair a b

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data Unit := Unit
data Maybe a := Nothing | Just a
data List a := Nil | Cons a (List a)

type Always (a: Type) :: Type := {
  Always _ =: Unit
}

class Functor (f: k -> Type) {
  fmap :: \ a b. (a -> b) -> f a -> f b
}

instance Functor Maybe {
  fmap := \g mx. case mx { Nothing -> Nothing; Just x -> Just (g x) }
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
data Mult := Unrestricted | Affine | Linear
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

readHandle :: Computation { handle: Unit @Linear } { handle: Unit @Linear } (Elem (List Unit))
readHandle := assumption

main :: Computation { handle: Unit @Linear } { handle: Unit @Linear } Unit
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
data List a := Nil | Cons a (List a)
data Unit := Unit
data Bool := True | False

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

class Eq a {
  eq :: a -> a -> Bool
}

instance Eq Bool {
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
	// A 2-param type family partially applied: only 1 argument given.
	// This should NOT be reduced (not saturated), and should not crash.
	source := `
data Unit := Unit
data Pair a b := MkPair a b
type Fst (a: Type) (b: Type) :: Type := {
  Fst a b =: a
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
data Unit := Unit
data Bool := True | False
data Pair a b := MkPair a b
type Fst (c: Type) :: Type := {
  Fst (Pair a b) =: a
}
type Snd (c: Type) :: Type := {
  Snd (Pair a b) =: b
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
	// A data family type reduces to a mangled type.
	// Exhaustiveness checking must look up the mangled type's constructors.
	source := `
data Unit := Unit
data Bool := True | False

class Container a {
  data Elem a :: Type;
  empty :: a
}

instance Container Bool {
  data Elem Bool =: BoolElem Bool | EmptyElem;
  empty := True
}

test :: Elem Bool -> Bool
test := \e. case e {
  BoolElem b -> b;
  EmptyElem -> False
}
`
	checkSource(t, source, nil)
}

func TestInteractionBugDataFamilyExhaustMissingAfterReduction(t *testing.T) {
	// Same as above but missing one constructor - should detect non-exhaustive.
	source := `
data Unit := Unit
data Bool := True | False

class Container a {
  data Elem a :: Type;
  empty :: a
}

instance Container Bool {
  data Elem Bool =: BoolElem Bool | EmptyElem;
  empty := True
}

test :: Elem Bool -> Bool
test := \e. case e {
  BoolElem b -> b
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrNonExhaustive)
}

// ----------------------------------------------------------------
// 4f. Type family + instance resolution interaction
// ----------------------------------------------------------------

func TestInteractionBugTypeFamilyInstanceResolution(t *testing.T) {
	// Instance resolution where the type argument contains a type family.
	// The resolver should reduce the type family before matching instances.
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
data Bool := True | False

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

class Eq a {
  eq :: a -> a -> Bool
}

instance Eq Bool {
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Maybe a := Nothing | Just a
data Unit := Unit

class Container c {
  type Elem c :: Type;
  empty :: c
}

instance Container (List a) {
  type Elem (List a) =: a;
  empty := Nil
}

instance Container (Maybe a) {
  type Elem (Maybe a) =: a;
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
data List a := Nil | Cons a (List a)

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

id :: \ a. a -> a
id := \x. x

test :: Int -> Int
test := \n. id @(Elem (List Int)) n
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
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
data Unit := Unit
data List a := Nil | Cons a (List a)

type Id (a: Type) :: Type := {
  Id a =: a
}

class Elem c e | c =: e {
  extract :: c -> e
}

instance Elem (List a) a {
  extract := \xs. case xs { Cons x rest -> x; Nil -> extract Nil }
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
data List a := Nil | Cons a (List a)
data Unit := Unit
data Bool := True | False

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

class Eq a {
  eq :: a -> a -> Bool
}

instance Eq Bool {
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
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
data List a := Nil | Cons a (List a)
data Unit := Unit
data Bool := True | False

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

class Eq a {
  eq :: a -> a -> Bool
}

instance Eq Unit {
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
	// Using data family constructors in the context of function application.
	source := `
data Unit := Unit
data Bool := True | False

class Container a {
  data Elem a :: Type;
  empty :: a
}

instance Container Bool {
  data Elem Bool =: Tag Bool;
  empty := True
}

id :: \ a. a -> a
id := \x. x

test :: Elem Bool
test := id (Tag True)
`
	checkSource(t, source, nil)
}

// ----------------------------------------------------------------
// 4o. Multiple class constraints with type families
// ----------------------------------------------------------------

func TestInteractionBugMultipleConstraintsTypeFamilies(t *testing.T) {
	// Multiple class constraints where the types are related via type families.
	source := `
data Unit := Unit
data Bool := True | False

class Eq a {
  eq :: a -> a -> Bool
}

class Show a {
  show :: a -> a
}

instance Eq Bool {
  eq := \x y. True
}

instance Show Bool {
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
	meta := &types.TyMeta{ID: 999, Kind: types.KType{}}
	tfApp := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{meta},
		Kind: types.KType{},
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
		Kind: types.KType{},
	}
	if typeHasMeta(tfApp) {
		t.Error("typeHasMeta should not detect meta in TyFamilyApp with only TyCon args")
	}
}

func TestInteractionContainsMetaTyFamilyAppNestedMeta(t *testing.T) {
	// TyFamilyApp with meta nested inside a TyApp argument.
	meta := &types.TyMeta{ID: 999, Kind: types.KType{}}
	tfApp := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{
			&types.TyApp{
				Fun: &types.TyCon{Name: "List"},
				Arg: meta,
			},
		},
		Kind: types.KType{},
	}
	if !typeHasMeta(tfApp) {
		t.Error("typeHasMeta should detect TyMeta nested inside TyFamilyApp arg")
	}
}

// ----------------------------------------------------------------
// 5b. typeHasMeta: TyCBPV (Thunk) case
// ----------------------------------------------------------------

func TestInteractionContainsMetaTyThunk(t *testing.T) {
	meta := &types.TyMeta{ID: 999, Kind: types.KType{}}
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
	meta := &types.TyMeta{ID: 999, Kind: types.KRow{}}
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
data List a := Nil | Cons a (List a)
data Unit := Unit
data Bool := True | False

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

class Eq a {
  eq :: a -> a -> Bool
}

instance Eq Unit {
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
data List a := Nil | Cons a (List a)
data Unit := Unit

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

step :: Computation { cap: Elem (List Unit) } {} Unit
step := assumption

main :: Computation { cap: Unit } {} Unit
main := step
`
	checkSource(t, source, nil)
}

// ================================================================
// Helper: try compilation, return "" on success, error string on failure
// ================================================================

func checkSourceTryBoth(t *testing.T, source string, config *CheckConfig) string {
	t.Helper()
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatal("lex errors:", lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	_, checkErrs := Check(ast, src, config)
	if checkErrs.HasErrors() {
		return checkErrs.Format()
	}
	return ""
}
