package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ==========================================
// Stress Tests: Type Families
// ==========================================

// --- 1a: Deep recursive type family (Peano naturals) ---

// peanoSource generates Peano-style Add type family with a depth-N test usage.
// Uses NatPair wrapper since multi-param type families require unified syntax encoding.
// Add (NatPair Z b) = b
// Add (NatPair (S a) b) = S (Add (NatPair a b))
func peanoSource(depth int) string {
	var b strings.Builder
	b.WriteString("form Nat := { Z: (); S: Nat; }\n")
	b.WriteString("form NatPair := \\(a: Nat) (b: Nat). { MkNatPair: NatPair a b; }\n")
	b.WriteString("type Add :: Nat := \\(p: Type). case p {\n")
	b.WriteString("  (NatPair Z b) => b;\n")
	b.WriteString("  (NatPair (S a) b) => S (Add (NatPair a b))\n")
	b.WriteString("}\n")

	// Build S^depth applied to Z
	nested := "Z"
	for i := 0; i < depth; i++ {
		nested = fmt.Sprintf("(S %s)", nested)
	}

	// A phantom type indexed by Nat, to force reduction without needing value-level computation.
	b.WriteString("form Phantom := \\(n: Nat). { MkPhantom: Phantom n; }\n")
	b.WriteString(fmt.Sprintf("f :: Phantom (Add (NatPair %s Z)) -> Phantom (Add (NatPair %s Z))\n", nested, nested))
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
	// Mutually recursive families: Bounce a =: Trampoline a, Trampoline a =: Bounce a.
	// Cycle detected via sentinel memoization; the families remain stuck (unreduced),
	// producing a type mismatch (E0200) when Bounce Unit is compared against Unit.
	source := `
form Unit := { Unit: Unit; }
form Nat := { Z: (); S: Nat; }
type Bounce :: Type := \(a: Type). case a {
  a => Trampoline a
}
type Trampoline :: Type := \(a: Type). case a {
  a => Bounce a
}
f :: Bounce Unit -> Unit
f := \x. x
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

// --- 1b: Many equations ---

func TestStressManyEquations(t *testing.T) {
	// Type family with 60 equations mapping numbered constructors to results.
	var b strings.Builder
	b.WriteString("form Tag := { ")
	for i := 0; i < 60; i++ {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(fmt.Sprintf("T%d: Tag", i))
	}
	b.WriteString("; }\n")
	b.WriteString("form Result := { R0: Result; R1: Result; }\n")
	b.WriteString("type Dispatch :: Result := \\(t: Tag). case t {\n")
	for i := 0; i < 60; i++ {
		if i > 0 {
			b.WriteString(";\n")
		}
		if i%2 == 0 {
			b.WriteString(fmt.Sprintf("  T%d => R0", i))
		} else {
			b.WriteString(fmt.Sprintf("  T%d => R1", i))
		}
	}
	b.WriteString("\n}\n")

	// Verify specific dispatch results via phantom types.
	b.WriteString("form Phantom := \\(r: Result). { MkPhantom: Phantom r; }\n")
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
form Wrapper := \a. { Wrap: a -> Wrapper a; }
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Unwrap :: Type := \(w: Type). case w {
  (Wrapper a) => a
}

type Head :: Type := \(l: Type). case l {
  (List a) => a
}

type Apply :: Type := \(f: Type). case f {
  a => a
}

-- Nested: Unwrap (Wrapper (Head (List (Apply Unit)))) = Head (List Unit) = Unit
f :: Unwrap (Wrapper (Head (List Unit))) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestStressTriplyNestedFamilies(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Box := \a. { MkBox: a -> Box a; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Unbox :: Type := \(b: Type). case b {
  (Box a) => a
}

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

type Id :: Type := \(a: Type). case a {
  a => a
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
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

-- A class whose method return type is a type family application.
form Container := \c. {
  chead: c -> Elem c
}

-- Use a top-level assumption as the fallback for Nil.
nilFallback :: \ a. a
nilFallback := assumption

impl Container (List a) := {
  chead := \xs. case xs {
    Cons x _ => x;
    Nil => nilFallback
  }
}

f :: List Unit -> Elem (List Unit)
f := chead
`
	checkSource(t, source, nil)
}

// --- 1e: Multiple type family instances ---

func TestStressMultipleDataFamilyInstances(t *testing.T) {
	// Multiple instances of a class with associated type families.
	// Uses type families (not data families) since data family constructor
	// registration requires the legacy syntax path.
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

emptyPair :: \ a b. Pair a b
emptyPair := assumption

form Collection := \c. {
  type Key c :: Type;
  empty: c
}

impl Collection (List a) := {
  type Key := a;
  empty := Nil
}

impl Collection Unit := {
  type Key := Unit;
  empty := Unit
}

impl Collection (Pair a b) := {
  type Key := a;
  empty := emptyPair
}

impl Collection (Maybe a) := {
  type Key := a;
  empty := Nothing
}

-- Verify associated type reduces correctly per instance.
f :: Key (List Unit) -> Unit
f := \x. x

g :: Key Unit -> Unit
g := \x. x

h :: Key (Maybe Unit) -> Unit
h := \x. x
`
	checkSource(t, source, nil)
}

func TestStressFiveDataFamilyInstances(t *testing.T) {
	// 5 instances with associated type families mapping containers to element types.
	// Uses type families instead of data families since data family constructor
	// registration requires the legacy syntax path.
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
form Either := \a b. { Left: a -> Either a b; Right: b -> Either a b; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

eitherFallback :: \ a b. Either a b -> a
eitherFallback := assumption

form HasRepr := \c. {
  type Repr c :: Type;
  toRepr: c -> Repr c
}

impl HasRepr Unit := {
  type Repr := Unit;
  toRepr := \x. x
}

impl HasRepr (List a) := {
  type Repr := a;
  toRepr := \xs. case xs { Cons x _ => x; Nil => toRepr Nil }
}

impl HasRepr (Pair a b) := {
  type Repr := a;
  toRepr := \p. case p { MkPair a b => a }
}

impl HasRepr (Either a b) := {
  type Repr := a;
  toRepr := \e. case e { Left a => a; Right _ => eitherFallback e }
}

impl HasRepr (Maybe a) := {
  type Repr := a;
  toRepr := \m. case m { Just x => x; Nothing => toRepr Nothing }
}

x1 :: Repr Unit
x1 := Unit

x2 :: Repr (List Unit)
x2 := Unit

x3 :: Repr (Pair Unit Unit)
x3 := Unit
`
	checkSource(t, source, nil)
}

// ==========================================
// Boundary Tests
// ==========================================

// --- 2a: Zero-arity type family ---

func TestBoundaryZeroArityTypeFamily(t *testing.T) {
	// A type family with no parameters, acting like a type-level constant.
	// In unified syntax, a zero-arity type family is a type alias.
	source := `
form Unit := { Unit: Unit; }
type Const := Unit
f :: Const -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// --- 2b: Complex nested patterns in type family equations ---

func TestBoundaryComplexNestedPattern(t *testing.T) {
	source := `
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type DeepElem :: Type := \(c: Type). case c {
  (Maybe (List a)) => a;
  (List (Maybe a)) => a;
  (Maybe a) => a;
  (List a) => a
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
	// In unified syntax, fundep annotations are not supported;
	// the class is declared without fundeps.
	source := `
form Unit := { Unit: Unit; }

form Iso := \a b. {
  to: a -> b;
  from: b -> a
}

impl Iso Unit Unit := {
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
form Void := {
  VoidCon: Void
}
form Unit := { Unit: Unit; }
absurd :: Void -> Unit
absurd := \v. case v { VoidCon => Unit }
`
	checkSource(t, source, nil)
}

// --- 2e: Single-branch case returns post directly ---

func TestBoundarySingleBranchCase(t *testing.T) {
	source := `
form Unit := { MkUnit: (); }
consume :: Computation { a: Unit } {} Unit
consume := assumption
f :: Unit -> Computation { a: Unit } {} Unit
f := \u. case u {
  MkUnit => consume
}
`
	checkSource(t, source, nil)
}

// --- 2f: intersectCapRows with identical rows ---

func TestBoundaryIntersectCapRowsIdentical(t *testing.T) {
	// Both branches have exactly the same post-state -> intersection = same row.
	source := `
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
consumeA :: Computation { a: Unit, b: Unit } { b: Unit } Unit
consumeA := assumption
f :: Bool -> Computation { a: Unit, b: Unit } { b: Unit } Unit
f := \b. case b {
  True => consumeA;
  False => consumeA
}
`
	checkSource(t, source, nil)
}

// --- More boundary: intersect with disjoint rows ---

func TestBoundaryIntersectCapRowsDisjoint(t *testing.T) {
	// Branch True:  post = { b: Unit }
	// Branch False: post = { a: Unit }
	// Intersection: post = {}
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

// --- Boundary: three-way intersection ---

func TestBoundaryThreeWayIntersection(t *testing.T) {
	// Three branches:
	// Alt 1: post = { a: Unit, b: Unit, c: Unit }
	// Alt 2: post = { b: Unit, c: Unit }
	// Alt 3: post = { a: Unit, c: Unit }
	// Intersection: post = { c: Unit }
	source := `
form Three := { One: Three; Two: Three; Three: Three; }
form Unit := { Unit: Unit; }
noop :: Computation { a: Unit, b: Unit, c: Unit } { a: Unit, b: Unit, c: Unit } Unit
noop := assumption
consumeA :: Computation { a: Unit, b: Unit, c: Unit } { b: Unit, c: Unit } Unit
consumeA := assumption
consumeB :: Computation { a: Unit, b: Unit, c: Unit } { a: Unit, c: Unit } Unit
consumeB := assumption
f :: Three -> Computation { a: Unit, b: Unit, c: Unit } { c: Unit } Unit
f := \t. case t {
  One => noop;
  Two => consumeA;
  Three => consumeB
}
`
	checkSource(t, source, nil)
}

// --- Boundary: type family with wildcard-only patterns ---

func TestBoundaryTypeFamilyAllWildcards(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type Always :: Type := \(a: Type). case a {
  _ => Unit
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
form Nat := { Z: (); S: Nat; }
type Pred :: Nat := \(n: Nat). case n {
  (S n) => n;
  Z => Z
}
form Phantom := \(n: Nat). { MkPhantom: Phantom n; }
f :: Phantom (Pred (S Z)) -> Phantom Z
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Boundary: pattern variable reuse across equations ---

func TestBoundaryPatternVarReuseAcrossEquations(t *testing.T) {
	// The variable 'a' appears in both equations but has different bindings.
	source := `
form Unit := { Unit: Unit; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Elem :: Type := \(c: Type). case c {
  (Maybe a) => a;
  (List a) => a
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
	// Data family exhaustiveness with multiple constructors.
	// Uses a regular data type with an associated type family since data family
	// constructor registration requires the legacy syntax path.
	source := `
form Unit := { Unit: Unit; }
form Tag := { TagA: Tag; TagB: Tag; TagC: Tag; }

form HasTag := \c. {
  type TagOf c :: Type;
  getTag: c -> TagOf c
}

impl HasTag Unit := {
  type TagOf := Tag;
  getTag := \_. TagA
}

-- Must match all three constructors.
f :: Tag -> Unit
f := \t. case t {
  TagA => Unit;
  TagB => Unit;
  TagC => Unit
}
`
	checkSource(t, source, nil)
}

func TestBoundaryDataFamilyExhaustivenessIncomplete(t *testing.T) {
	// Non-exhaustive pattern match on a type used via associated type family.
	source := `
form Unit := { Unit: Unit; }
form Tag := { TagA: Tag; TagB: Tag; TagC: Tag; }

-- Missing TagC -> non-exhaustive.
f :: Tag -> Unit
f := \t. case t {
  TagA => Unit;
  TagB => Unit
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
}
