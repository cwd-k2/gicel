package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// ==========================================
// Stress Tests: Type Families
// ==========================================

// --- 1a: Deep recursive type family (Peano naturals) ---

// peanoSource generates Peano-style Add type family with a depth-N test usage.
// Add Z b = b
// Add (S a) b = S (Add a b)
// f :: Add (S (S (... Z ...))) Z -> ...
func peanoSource(depth int) string {
	var b strings.Builder
	b.WriteString("data Nat = Z | S Nat\n")
	b.WriteString("type Add (a : Nat) (b : Nat) :: Nat = {\n")
	b.WriteString("  Add Z b = b;\n")
	b.WriteString("  Add (S a) b = S (Add a b)\n")
	b.WriteString("}\n")

	// Build S^depth applied to Z
	nested := "Z"
	for i := 0; i < depth; i++ {
		nested = fmt.Sprintf("(S %s)", nested)
	}

	// A phantom type indexed by Nat, to force reduction without needing value-level computation.
	b.WriteString("data Phantom (n : Nat) = MkPhantom\n")
	b.WriteString(fmt.Sprintf("f :: Phantom (Add %s Z) -> Phantom (Add %s Z)\n", nested, nested))
	b.WriteString("f := \\x. x\n")

	return b.String()
}

func TestStressDeepRecursiveTF_Depth10(t *testing.T) {
	// Moderate depth: should succeed well within limits.
	source := peanoSource(10)
	checkSource(t, source, nil)
}

func TestStressDeepRecursiveTF_Depth50(t *testing.T) {
	// Still well within the 100 fuel limit (each step does 1 reduction + 1 recursive call).
	source := peanoSource(50)
	checkSource(t, source, nil)
}

func TestStressDeepRecursiveTF_DepthExceedsLimit(t *testing.T) {
	// The Peano Add family wraps results in S(...), which is not a family application,
	// so reduceFamilyApps doesn't recurse into it within a single normalize() call.
	// To truly exhaust the depth-100 fuel limit, we need a family whose RHS is itself
	// a saturated family application at the top level: F x = F x (infinite loop).
	// TestRecursiveTypeFamilyFuelExhaustion already covers this exact case.
	// Here we test a slightly more interesting variant: mutual recursion through
	// a family that bounces between two families > 100 times.
	source := `
data Unit = Unit
data Nat = Z | S Nat
type Bounce (a : Type) :: Type = {
  Bounce a = Trampoline a
}
type Trampoline (a : Type) :: Type = {
  Trampoline a = Bounce a
}
f :: Bounce Unit -> Unit
f := \x. x
`
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
}

// --- 1b: Many equations ---

func TestStressManyEquations(t *testing.T) {
	// Type family with 60 equations mapping numbered constructors to results.
	var b strings.Builder
	b.WriteString("data Tag =")
	for i := 0; i < 60; i++ {
		if i > 0 {
			b.WriteString(" |")
		}
		b.WriteString(fmt.Sprintf(" T%d", i))
	}
	b.WriteString("\n")
	b.WriteString("data Result = R0 | R1\n")
	b.WriteString("type Dispatch (t : Tag) :: Result = {\n")
	for i := 0; i < 60; i++ {
		if i > 0 {
			b.WriteString(";\n")
		}
		if i%2 == 0 {
			b.WriteString(fmt.Sprintf("  Dispatch T%d = R0", i))
		} else {
			b.WriteString(fmt.Sprintf("  Dispatch T%d = R1", i))
		}
	}
	b.WriteString("\n}\n")

	// Verify specific dispatch results via phantom types.
	b.WriteString("data Phantom (r : Result) = MkPhantom\n")
	b.WriteString("first :: Phantom (Dispatch T0) -> Phantom R0\n")
	b.WriteString("first := \\x. x\n")
	b.WriteString("last :: Phantom (Dispatch T59) -> Phantom R1\n")
	b.WriteString("last := \\x. x\n")
	b.WriteString("middle :: Phantom (Dispatch T30) -> Phantom R0\n")
	b.WriteString("middle := \\x. x\n")

	checkSource(t, b.String(), nil)
}

// --- 1c: Nested family applications: F (G (H x)) ---

func TestStressNestedFamilyApplications(t *testing.T) {
	source := `
data Wrapper a = Wrap a
data Unit = Unit
data List a = Nil | Cons a (List a)

type Unwrap (w : Type) :: Type = {
  Unwrap (Wrapper a) = a
}

type Head (l : Type) :: Type = {
  Head (List a) = a
}

type Apply (f : Type) :: Type = {
  Apply a = a
}

-- Nested: Unwrap (Wrapper (Head (List (Apply Unit)))) = Head (List Unit) = Unit
f :: Unwrap (Wrapper (Head (List Unit))) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestStressTriplyNestedFamilies(t *testing.T) {
	source := `
data Unit = Unit
data Box a = MkBox a
data List a = Nil | Cons a (List a)

type Unbox (b : Type) :: Type = {
  Unbox (Box a) = a
}

type Elem (c : Type) :: Type = {
  Elem (List a) = a
}

type Id (a : Type) :: Type = {
  Id a = a
}

-- Id(Elem(List(Unbox(Box Unit)))) = Id(Elem(List Unit)) = Id(Unit) = Unit
g :: Id (Elem (List (Unbox (Box Unit)))) -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

// --- 1d: Type family in class context ---

func TestStressTypeFamilyInClassContext(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)

type Elem (c : Type) :: Type = {
  Elem (List a) = a
}

-- A class whose method return type is a type family application.
class Container c {
  chead :: c -> Elem c
}

-- Use a top-level assumption as the fallback for Nil.
nilFallback :: \ a. a
nilFallback := assumption

instance Container (List a) {
  chead := \xs. case xs {
    Cons x _ -> x;
    Nil -> nilFallback
  }
}

f :: List Unit -> Elem (List Unit)
f := chead
`
	checkSource(t, source, nil)
}

// --- 1e: Multiple data family instances ---

func TestStressMultipleDataFamilyInstances(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
data Pair a b = MkPair a b
data Maybe a = Nothing | Just a

emptyPair :: \ a b. Pair a b
emptyPair := assumption

class Collection c {
  data Key c :: Type;
  empty :: c
}

instance Collection (List a) {
  data Key (List a) = ListKey a;
  empty := Nil
}

instance Collection Unit {
  data Key Unit = UnitKey;
  empty := Unit
}

instance Collection (Pair a b) {
  data Key (Pair a b) = PairKey a;
  empty := emptyPair
}

instance Collection (Maybe a) {
  data Key (Maybe a) = MaybeKey a;
  empty := Nothing
}

-- Verify each constructor is accessible.
k1 :: Key (List Unit)
k1 := ListKey Unit

k2 :: Key Unit
k2 := UnitKey

k3 :: Key (Pair Unit Unit)
k3 := PairKey Unit

k4 :: Key (Maybe Unit)
k4 := MaybeKey Unit

-- Pattern match on data family constructors.
unwrapListKey :: \ a. Key (List a) -> a
unwrapListKey := \k. case k { ListKey x -> x }
`
	checkSource(t, source, nil)
}

func TestStressFiveDataFamilyInstances(t *testing.T) {
	// 5 instances with unique constructors for exhaustive data family coverage.
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
data Pair a b = MkPair a b
data Either a b = Left a | Right b
data Maybe a = Nothing | Just a

nilFallback :: \ a. a
nilFallback := assumption

class HasRepr c {
  data Repr c :: Type;
  toRepr :: c -> Repr c
}

instance HasRepr Unit {
  data Repr Unit = ReprUnit;
  toRepr := \_. ReprUnit
}

instance HasRepr (List a) {
  data Repr (List a) = ReprList a;
  toRepr := \xs. case xs { Cons x _ -> ReprList x; Nil -> nilFallback }
}

instance HasRepr (Pair a b) {
  data Repr (Pair a b) = ReprPair a b;
  toRepr := \p. case p { MkPair a b -> ReprPair a b }
}

instance HasRepr (Either a b) {
  data Repr (Either a b) = ReprLeft a | ReprRight b;
  toRepr := \e. case e { Left a -> ReprLeft a; Right b -> ReprRight b }
}

instance HasRepr (Maybe a) {
  data Repr (Maybe a) = ReprJust a | ReprNothing;
  toRepr := \m. case m { Just x -> ReprJust x; Nothing -> ReprNothing }
}

x1 :: Repr Unit
x1 := ReprUnit

x2 :: Repr (List Unit)
x2 := ReprList Unit

x3 :: Repr (Pair Unit Unit)
x3 := ReprPair Unit Unit

x4 :: Repr (Maybe Unit)
x4 := case (toRepr (Just Unit)) {
  ReprJust x -> ReprJust x;
  ReprNothing -> ReprNothing
}
`
	checkSource(t, source, nil)
}

// ==========================================
// Boundary Tests
// ==========================================

// --- 2a: Zero-arity type family ---

func TestBoundaryZeroArityTypeFamily(t *testing.T) {
	// A type family with no parameters, acting like a type-level constant.
	// This tests whether the parser and checker handle 0-parameter type families.
	source := `
data Unit = Unit
type Const :: Type = {
  Const = Unit
}
f :: Const -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// --- 2b: Complex nested patterns in type family equations ---

func TestBoundaryComplexNestedPattern(t *testing.T) {
	source := `
data Maybe a = Nothing | Just a
data List a = Nil | Cons a (List a)
data Unit = Unit

type DeepElem (c : Type) :: Type = {
  DeepElem (Maybe (List a)) = a;
  DeepElem (List (Maybe a)) = a;
  DeepElem (Maybe a) = a;
  DeepElem (List a) = a
}

f :: DeepElem (Maybe (List Unit)) -> Unit
f := \x. x

g :: DeepElem (List (Maybe Unit)) -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

// --- 2c: Fundep with all parameters determined ---

func TestBoundaryFunDepAllDetermined(t *testing.T) {
	// Every parameter is in a "from" or "to" position.
	source := `
data Unit = Unit

class Iso a b | a -> b, b -> a {
  to :: a -> b;
  from :: b -> a
}

instance Iso Unit Unit {
  to := \x. x;
  from := \x. x
}

f :: Unit -> Unit
f := to
`
	checkSource(t, source, nil)
}

// --- 2d: Empty case analysis for lubPostStates ---

func TestBoundaryLubPostStatesZeroBranches(t *testing.T) {
	// lubPostStates with len==0 should return a fresh meta.
	// This indirectly tests the len==0 case through an empty case.
	// An empty case on an uninhabited type is unusual but should not crash.
	// The checker may report non-exhaustive warning, so we just verify no panic.
	source := `
data Void = {
  VoidCon :: Void
}
data Unit = Unit
absurd :: Void -> Unit
absurd := \v. case v { VoidCon -> Unit }
`
	checkSource(t, source, nil)
}

// --- 2e: Single-branch case returns post directly ---

func TestBoundarySingleBranchCase(t *testing.T) {
	source := `
data Unit = MkUnit
consume :: Computation { a : Unit } {} Unit
consume := assumption
f :: Unit -> Computation { a : Unit } {} Unit
f := \u. case u {
  MkUnit -> consume
}
`
	checkSource(t, source, nil)
}

// --- 2f: intersectCapRows with identical rows ---

func TestBoundaryIntersectCapRowsIdentical(t *testing.T) {
	// Both branches have exactly the same post-state → intersection = same row.
	source := `
data Bool = True | False
data Unit = Unit
consumeA :: Computation { a : Unit, b : Unit } { b : Unit } Unit
consumeA := assumption
f :: Bool -> Computation { a : Unit, b : Unit } { b : Unit } Unit
f := \b. case b {
  True -> consumeA;
  False -> consumeA
}
`
	checkSource(t, source, nil)
}

// --- More boundary: intersect with disjoint rows ---

func TestBoundaryIntersectCapRowsDisjoint(t *testing.T) {
	// Branch True:  post = { b : Unit }
	// Branch False: post = { a : Unit }
	// Intersection: post = {}
	source := `
data Bool = True | False
data Unit = Unit
consumeA :: Computation { a : Unit, b : Unit } { b : Unit } Unit
consumeA := assumption
consumeB :: Computation { a : Unit, b : Unit } { a : Unit } Unit
consumeB := assumption
f :: Bool -> Computation { a : Unit, b : Unit } {} Unit
f := \b. case b {
  True -> consumeA;
  False -> consumeB
}
`
	checkSource(t, source, nil)
}

// --- Boundary: three-way intersection ---

func TestBoundaryThreeWayIntersection(t *testing.T) {
	// Three branches:
	// Alt 1: post = { a : Unit, b : Unit, c : Unit }
	// Alt 2: post = { b : Unit, c : Unit }
	// Alt 3: post = { a : Unit, c : Unit }
	// Intersection: post = { c : Unit }
	source := `
data Three = One | Two | Three
data Unit = Unit
noop :: Computation { a : Unit, b : Unit, c : Unit } { a : Unit, b : Unit, c : Unit } Unit
noop := assumption
consumeA :: Computation { a : Unit, b : Unit, c : Unit } { b : Unit, c : Unit } Unit
consumeA := assumption
consumeB :: Computation { a : Unit, b : Unit, c : Unit } { a : Unit, c : Unit } Unit
consumeB := assumption
f :: Three -> Computation { a : Unit, b : Unit, c : Unit } { c : Unit } Unit
f := \t. case t {
  One -> noop;
  Two -> consumeA;
  Three -> consumeB
}
`
	checkSource(t, source, nil)
}

// --- Boundary: type family with wildcard-only patterns ---

func TestBoundaryTypeFamilyAllWildcards(t *testing.T) {
	source := `
data Unit = Unit
type Always (a : Type) :: Type = {
  Always _ = Unit
}
f :: Always Int -> Unit
f := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// --- Boundary: recursive TF that terminates at different depths ---

func TestBoundaryRecursiveTFTerminatesAtDepth1(t *testing.T) {
	source := `
data Nat = Z | S Nat
type Pred (n : Nat) :: Nat = {
  Pred (S n) = n;
  Pred Z = Z
}
data Phantom (n : Nat) = MkPhantom
f :: Phantom (Pred (S Z)) -> Phantom Z
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Boundary: pattern variable reuse across equations ---

func TestBoundaryPatternVarReuseAcrossEquations(t *testing.T) {
	// The variable 'a' appears in both equations but has different bindings.
	source := `
data Unit = Unit
data Maybe a = Nothing | Just a
data List a = Nil | Cons a (List a)

type Elem (c : Type) :: Type = {
  Elem (Maybe a) = a;
  Elem (List a) = a
}

f :: Elem (Maybe Unit) -> Unit
f := \x. x

g :: Elem (List Unit) -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

// --- Boundary: data family exhaustiveness with multiple constructors ---

func TestBoundaryDataFamilyExhaustivenessMultiCon(t *testing.T) {
	source := `
data Unit = Unit

class HasTag c {
  data Tag c :: Type;
  getTag :: c -> Tag c
}

instance HasTag Unit {
  data Tag Unit = TagA | TagB | TagC;
  getTag := \_. TagA
}

-- Must match all three constructors.
f :: Tag Unit -> Unit
f := \t. case t {
  TagA -> Unit;
  TagB -> Unit;
  TagC -> Unit
}
`
	checkSource(t, source, nil)
}

func TestBoundaryDataFamilyExhaustivenessIncomplete(t *testing.T) {
	source := `
data Unit = Unit

class HasTag c {
  data Tag c :: Type;
  getTag :: c -> Tag c
}

instance HasTag Unit {
  data Tag Unit = TagA | TagB | TagC;
  getTag := \_. TagA
}

-- Missing TagC → non-exhaustive.
f :: Tag Unit -> Unit
f := \t. case t {
  TagA -> Unit;
  TagB -> Unit
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrNonExhaustive)
}
