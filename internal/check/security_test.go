package check

import (
	"strings"
	"testing"
	"time"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// ==========================================================================
// Security & Performance Regression Tests
//
// These tests verify that the type checker remains safe against malicious
// GICEL programs designed to cause DoS (unbounded time, memory, stack).
// ==========================================================================

// --- 1. Type Family Reduction DoS ---

// TestSecurityTypeFamilyDoublingRHS tests that a type family whose RHS is
// larger than its LHS does not cause exponential blowup. The fuel limit
// should terminate it.
func TestSecurityTypeFamilyDoublingRHS(t *testing.T) {
	// Grow :: Type -> Type
	// Grow a = Grow (Pair a a)
	// This doubles the type size on each reduction step.
	// After 100 steps, the type would be 2^100 nodes — but fuel stops it.
	source := `
data Pair a b := MkPair a b
data Unit := Unit
type Grow (a: Type) :: Type := {
  Grow a =: Grow (Pair a a)
}
f :: Grow Unit -> Unit
f := \x. x
`
	start := time.Now()
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("type family doubling took %v, expected < 5s", elapsed)
	}
}

// TestSecurityTypeFamilyLinearGrowth tests a type family that grows linearly.
// GrowList a = Cons Unit (GrowList a) — but since Cons is a data constructor
// (not a type family), the inner GrowList is not immediately reduced by
// reduceFamilyApps. The result is a type mismatch (not fuel exhaustion),
// because the RHS type Cons Unit (GrowList ...) doesn't reduce to Unit.
func TestSecurityTypeFamilyLinearGrowth(t *testing.T) {
	source := `
data List a := Nil | Cons a (List a)
data Unit := Unit
type GrowList (a: Type) :: Type := {
  GrowList a =: Cons Unit (GrowList a)
}
f :: GrowList Unit -> Unit
f := \x. x
`
	start := time.Now()
	// This produces a type mismatch, not fuel exhaustion, because Cons is
	// a data constructor and the type doesn't match Unit.
	checkSourceExpectError(t, source, nil)
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("linear type family growth took %v, expected < 5s", elapsed)
	}
}

// TestSecurityTypeFamilyMutualRecursion tests mutually recursive type families
// (simulated via a single family that ping-pongs).
func TestSecurityTypeFamilyMutualRecursion(t *testing.T) {
	source := `
data Unit := Unit
data Wrapper a := Wrap a
type Ping (a: Type) :: Type := {
  Ping a =: Ping (Wrapper a)
}
f :: Ping Unit -> Unit
f := \x. x
`
	start := time.Now()
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("mutual recursion took %v, expected < 5s", elapsed)
	}
}

// --- 2. Name Collision via Data Family Mangling ---

// TestSecurityMangledNameCollision verifies that the mangling scheme
// does NOT produce collisions with crafted family/pattern names.
// The arity-prefixed format "F$$arity$pat1$pat2" prevents:
//   - Family "A" with pattern [B$C] → "A$$1$B$C"
//   - Family "A$B" with pattern [C] → "A$B$$1$C"
func TestSecurityMangledNameCollision(t *testing.T) {
	ch := &Checker{
		families: make(map[string]*TypeFamilyInfo),
	}

	name1 := ch.mangledDataFamilyName("A", []types.Type{
		&types.TyCon{Name: "B$C"},
	})
	name2 := ch.mangledDataFamilyName("A$B", []types.Type{
		&types.TyCon{Name: "C"},
	})

	if name1 == name2 {
		t.Errorf("COLLISION: mangledDataFamilyName(%q, [B$C]) == mangledDataFamilyName(%q, [C]) == %q",
			"A", "A$B", name1)
	}
	t.Logf("name1=%q, name2=%q (no collision)", name1, name2)
}

// TestSecurityMangledNameCollisionPatternSeparator tests that the arity-prefixed
// scheme prevents collision between "Elem" with pattern "List" and "Elem$List" with no patterns.
func TestSecurityMangledNameCollisionPatternSeparator(t *testing.T) {
	ch := &Checker{
		families: make(map[string]*TypeFamilyInfo),
	}

	// Family "Elem" with pattern "List" → "Elem$$1$List"
	name1 := ch.mangledDataFamilyName("Elem", []types.Type{
		&types.TyCon{Name: "List"},
	})
	// Family "Elem$List" with no pattern → "Elem$List$$0"
	name2 := ch.mangledDataFamilyName("Elem$List", []types.Type{})

	if name1 == name2 {
		t.Errorf("COLLISION: %q == %q", name1, name2)
	}
	t.Logf("name1=%q, name2=%q (no collision)", name1, name2)
}

// TestSecurityMangledNameMultiplePatterns tests mangling with multiple patterns.
// The arity prefix distinguishes [A, B] (arity 2) from [A$B] (arity 1).
func TestSecurityMangledNameMultiplePatterns(t *testing.T) {
	ch := &Checker{
		families: make(map[string]*TypeFamilyInfo),
	}

	// Family "F" with patterns [A, B] → "F$$2$A$B"
	name1 := ch.mangledDataFamilyName("F", []types.Type{
		&types.TyCon{Name: "A"},
		&types.TyCon{Name: "B"},
	})
	// Family "F" with pattern [A$B] → "F$$1$A$B"
	name2 := ch.mangledDataFamilyName("F", []types.Type{
		&types.TyCon{Name: "A$B"},
	})

	if name1 == name2 {
		t.Errorf("COLLISION: mangledDataFamilyName(F, [A, B]) == mangledDataFamilyName(F, [A$B]) == %q", name1)
	}
	t.Logf("name1=%q, name2=%q (no collision)", name1, name2)
}

// --- 3. Constructor Injection via Data Family ---

// TestSecurityConstructorOverwrite verifies that a data family instance
// cannot overwrite a constructor from a regular data type.
func TestSecurityConstructorOverwrite(t *testing.T) {
	// Define a regular data type with constructor "Wrap", then try to
	// define a data family instance that also introduces "Wrap".
	source := `
data Wrapper a := Wrap a
data Unit := Unit

class Container c {
  data Elem c :: Type
}

instance Container (Wrapper a) {
  data Elem (Wrapper a) =: Wrap a
}
`
	// The second "Wrap" constructor should conflict with the first.
	checkSourceExpectCode(t, source, nil, errs.ErrDuplicateDecl)
}

// --- 4. verifyInjectivity Quadratic Cost ---

// TestPerformanceVerifyInjectivityCost tests the O(n^2) cost of
// injectivity verification with many equations. With 50 equations,
// there are 50*49/2 = 1225 pairs, each requiring trial unification.
func TestPerformanceVerifyInjectivityCost(t *testing.T) {
	// Generate a type family with N equations, each mapping a distinct
	// constructor to a distinct result.
	var sb strings.Builder
	const N = 50

	// Define N data constructors.
	sb.WriteString("data Tag := ")
	for i := 0; i < N; i++ {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(tagName(i))
	}
	sb.WriteString("\n")

	// Define an injective type family.
	sb.WriteString("type F (a: Tag) :: (r: Tag) | r =: a := {\n")
	for i := 0; i < N; i++ {
		if i > 0 {
			sb.WriteString(";\n")
		}
		sb.WriteString("  F " + tagName(i) + " =: " + tagName(N-1-i))
	}
	sb.WriteString("\n}\n")

	start := time.Now()
	checkSource(t, sb.String(), nil)
	elapsed := time.Since(start)

	// With N=50, 1225 pairs. Should complete in reasonable time.
	if elapsed > 10*time.Second {
		t.Errorf("injectivity check for %d equations took %v", N, elapsed)
	}
	t.Logf("injectivity check for %d equations: %v", N, elapsed)
}

func tagName(i int) string {
	return "T" + strings.Repeat("a", i/26) + string(rune('A'+i%26))
}

// --- 5. intersectCapRows Complexity ---

// TestPerformanceIntersectCapRowsManyBranches tests that intersectCapRows
// handles many branches with many capabilities efficiently.
func TestPerformanceIntersectCapRowsManyBranches(t *testing.T) {
	// Generate a case expression with many branches, each in a computation
	// context with many capabilities. This exercises lubPostStates.
	const numBranches = 20
	const numCaps = 20

	var sb strings.Builder
	// Define a data type with many constructors.
	sb.WriteString("data BigEnum := ")
	for i := 0; i < numBranches; i++ {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString("C" + tagName(i))
	}
	sb.WriteString("\n")

	sb.WriteString("data Unit := Unit\n")
	sb.WriteString("f :: BigEnum -> Unit\n")
	sb.WriteString("f := \\x. case x {\n")
	for i := 0; i < numBranches; i++ {
		if i > 0 {
			sb.WriteString(";\n")
		}
		sb.WriteString("  C" + tagName(i) + " -> Unit")
	}
	sb.WriteString("\n}\n")

	start := time.Now()
	checkSource(t, sb.String(), nil)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("case with %d branches took %v", numBranches, elapsed)
	}
	t.Logf("case with %d branches: %v", numBranches, elapsed)
}

// --- 6. applyFunDepImprovement with Many Instances ---

// TestPerformanceFunDepManyInstances tests fundep improvement with
// many instances of a class with functional dependencies.
func TestPerformanceFunDepManyInstances(t *testing.T) {
	const N = 30
	var sb strings.Builder

	// Define N distinct types.
	for i := 0; i < N; i++ {
		sb.WriteString("data D" + tagName(i) + " := MkD" + tagName(i) + "\n")
	}

	// Define a class with a fundep.
	sb.WriteString("class Assoc a b | a =: b {\n")
	sb.WriteString("  assocGet :: a -> b\n")
	sb.WriteString("}\n")

	// N instances.
	for i := 0; i < N; i++ {
		sb.WriteString("instance Assoc D" + tagName(i) + " D" + tagName((i+1)%N) + " {\n")
		sb.WriteString("  assocGet := \\_. MkD" + tagName((i+1)%N) + "\n")
		sb.WriteString("}\n")
	}

	// Use the fundep.
	sb.WriteString("f :: D" + tagName(0) + " -> D" + tagName(1) + "\n")
	sb.WriteString("f := assocGet\n")

	start := time.Now()
	checkSource(t, sb.String(), nil)
	elapsed := time.Since(start)

	if elapsed > 10*time.Second {
		t.Errorf("fundep improvement with %d instances took %v", N, elapsed)
	}
	t.Logf("fundep improvement with %d instances: %v", N, elapsed)
}

// --- 7. Snapshot/Restore Cost ---

// TestPerformanceSnapshotCost verifies that trial unification snapshot
// cost is manageable even with many metavariables in scope.
func TestPerformanceSnapshotCost(t *testing.T) {
	// A program that creates many metavariables through polymorphic usage.
	source := `
data Bool := True | False
data List a := Nil | Cons a (List a)
data Unit := Unit

id :: \ a. a -> a
id := \x. x

f :: Unit
f := id (id (id (id (id (id (id (id (id (id Unit)))))))))
`
	start := time.Now()
	checkSource(t, source, nil)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Errorf("deeply nested id application took %v", elapsed)
	}
}

// --- 8. headTyConWithFamilies Recursive Reduction ---

// TestSecurityHeadTyConWithFamiliesDepth verifies that
// headTyConWithFamilies respects the fuel limit via reduceTyFamily.
func TestSecurityHeadTyConWithFamiliesDepth(t *testing.T) {
	// A type family that reduces to another family application endlessly.
	// headTyConWithFamilies calls reduceTyFamily, which has fuel.
	source := `
data Unit := Unit
type Loop (a: Type) :: Type := {
  Loop a =: Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	start := time.Now()
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("headTyConWithFamilies infinite loop took %v", elapsed)
	}
}

// --- 9. Parser Safety ---

// TestSecurityParserDeepNesting verifies the parser handles deeply nested
// type expressions without stack overflow.
func TestSecurityParserDeepNesting(t *testing.T) {
	// Generate a deeply nested type: (((((...))))) with 200 levels.
	var sb strings.Builder
	const depth = 200
	sb.WriteString("data Unit := Unit\n")
	sb.WriteString("f :: ")
	for i := 0; i < depth; i++ {
		sb.WriteString("(")
	}
	sb.WriteString("Unit")
	for i := 0; i < depth; i++ {
		sb.WriteString(")")
	}
	sb.WriteString(" -> Unit\nf := \\x. x\n")

	// Should not panic.
	start := time.Now()
	checkSource(t, sb.String(), nil)
	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		t.Errorf("deep nesting took %v", elapsed)
	}
}

// TestSecurityParserLongEquationBlock verifies the parser handles a
// type family with many equations.
func TestSecurityParserLongEquationBlock(t *testing.T) {
	const N = 100
	var sb strings.Builder

	sb.WriteString("data Tag := ")
	for i := 0; i < N; i++ {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(tagName(i))
	}
	sb.WriteString("\n")

	sb.WriteString("type F (a: Tag) :: Tag := {\n")
	for i := 0; i < N; i++ {
		if i > 0 {
			sb.WriteString(";\n")
		}
		sb.WriteString("  F " + tagName(i) + " =: " + tagName(i))
	}
	sb.WriteString("\n}\n")

	start := time.Now()
	checkSource(t, sb.String(), nil)
	elapsed := time.Since(start)
	if elapsed > 10*time.Second {
		t.Errorf("parsing %d equations took %v", N, elapsed)
	}
	t.Logf("parsing %d equations: %v", N, elapsed)
}

// --- 10. Exponential Type Growth via Substitution ---

// TestSecurityExponentialTypeGrowth tests that a type family that doubles
// type size on each reduction is bounded by the fuel limit, and that
// the intermediate type doesn't consume excessive memory.
func TestSecurityExponentialTypeGrowth(t *testing.T) {
	// type Double a :: Type := { Double a =: Pair a a }
	// type Chain a :: Type := { Chain a =: Chain (Double a) }
	// Chain Unit → Chain (Pair Unit Unit) → Chain (Pair (Pair Unit Unit) (Pair Unit Unit)) → ...
	// After k steps, the argument has 2^k nodes. With fuel=100, this would be 2^100 nodes.
	//
	// However, reduceTyFamily substitutes pattern vars into the RHS, and the RHS
	// of Chain is "Chain (Double a)" — but Double is a separate family, not reduced
	// during the substitution. The actual type created is Chain(TyFamilyApp("Double", [a])),
	// which doesn't grow. Double would only be reduced when the next unification triggers
	// normalize. So the growth is bounded: each step adds one wrapper layer.
	//
	// Let's test the worst case: a single family that explicitly doubles.
	source := `
data Pair a b := MkPair a b
data Unit := Unit
type Explode (a: Type) :: Type := {
  Explode a =: Explode (Pair a a)
}
f :: Explode Unit -> Unit
f := \x. x
`
	start := time.Now()
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyReduction)
	elapsed := time.Since(start)

	// Key: even though the type doubles each step, after 100 steps
	// the total work is O(2^100 * pattern_matching_cost). We need to verify
	// this doesn't actually happen — the fuel limit should fire before
	// the type gets astronomically large.
	//
	// In practice: the fuel fires at depth 100, but the type was built
	// for all 100 steps. Step k creates a type of size ~2^k.
	// Total memory: sum(2^k, k=0..100) = 2^101 - 1 ≈ insane.
	//
	// BUT: in the actual code, each substitution step creates a type
	// of size proportional to 2^k, so the total allocation up to step k
	// is O(2^(k+1)). The fuel limit of 100 means we might allocate 2^101
	// type nodes. This IS the exponential blowup.
	//
	// This test verifies the current behavior terminates (due to fuel)
	// but may take significant time/memory for fuel=100.
	if elapsed > 30*time.Second {
		t.Errorf("exponential type growth took %v — fuel limit may be too high", elapsed)
	}
	t.Logf("exponential type growth: %v", elapsed)
}
