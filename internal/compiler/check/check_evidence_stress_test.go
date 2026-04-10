package check

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/desugar"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// =============================================================================
// Stress Tests — Evidence Resolution under load
// =============================================================================

func TestStressDeepSuperclassChain(t *testing.T) {
	// Chain of 5 superclasses: C5 => C4 => C3 => C2 => C1.
	// Using C1's method with only C5 in scope requires traversing 4 levels.
	source := `form Bool := { True: Bool; False: Bool; }
form C1 := \a. { m1: a -> Bool }
form C2 := \a. C1 a => { m2: a -> Bool }
form C3 := \a. C2 a => { m3: a -> Bool }
form C4 := \a. C3 a => { m4: a -> Bool }
form C5 := \a. C4 a => { m5: a -> Bool }
impl C1 Bool := { m1 := \x. True }
impl C2 Bool := { m2 := \x. True }
impl C3 Bool := { m3 := \x. True }
impl C4 Bool := { m4 := \x. True }
impl C5 Bool := { m5 := \x. True }
f :: \ a. C5 a => a -> Bool
f := \x. m1 x
main := f True`
	checkSource(t, source, nil)
}

func TestStressManyInstancesOneClass(t *testing.T) {
	// 10 types, each with an Eq instance. Resolve Eq for each.
	var sb strings.Builder
	sb.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	sb.WriteString("form Eq := \\a. { eq: a -> a -> Bool }\n")
	for i := range 10 {
		name := fmt.Sprintf("T%d", i)
		sb.WriteString(fmt.Sprintf("form %s := { Mk%s: %s; }\n", name, name, name))
		sb.WriteString(fmt.Sprintf("impl Eq %s := { eq := \\x y. True }\n", name))
	}
	// Use eq on each type.
	for i := range 10 {
		name := fmt.Sprintf("T%d", i)
		sb.WriteString(fmt.Sprintf("test%d := eq Mk%s Mk%s\n", i, name, name))
	}
	checkSource(t, sb.String(), nil)
}

func TestStressManyClasses(t *testing.T) {
	// 10 independent classes, each with one instance.
	var sb strings.Builder
	sb.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	for i := range 10 {
		sb.WriteString(fmt.Sprintf("form C%d := \\a. { m%d: a -> Bool }\n", i, i))
	}
	for i := range 10 {
		sb.WriteString(fmt.Sprintf("impl C%d Bool := { m%d := \\x. True }\n", i, i))
	}
	// Function requiring all 10 constraints (curried style).
	sb.WriteString("f :: \\ a. ")
	for i := range 10 {
		sb.WriteString(fmt.Sprintf("C%d a => ", i))
	}
	sb.WriteString("a -> Bool\n")
	sb.WriteString("f := \\x. m0 x\n")
	sb.WriteString("main := f True\n")
	checkSource(t, sb.String(), nil)
}

func TestStressContextualInstanceChain(t *testing.T) {
	// Nested contextual instances: Eq a => Eq (F a), Eq a => Eq (G a).
	// Resolve Eq (F (G Bool)).
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form G := \a. { MkG: a -> G a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq (F a) := { eq := \x y. True }
impl Eq a => Eq (G a) := { eq := \x y. True }
main := eq (MkF (MkG True)) (MkF (MkG False))`
	checkSource(t, source, nil)
}

func TestStressMultiParamConstraints(t *testing.T) {
	// Curried constraints with different type variables.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Show := \a. { show: a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. True }
f :: \ a b. (Eq a, Show b) => a -> b -> Bool
f := \x y. eq x x
main := f True False`
	checkSource(t, source, nil)
}

// =============================================================================
// Edge Cases — Evidence Resolution
// =============================================================================

func TestEdgeSameClassDifferentArgs(t *testing.T) {
	// Two Eq constraints with different type args.
	source := `form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq Unit := { eq := \x y. True }
f :: \ a b. (Eq a, Eq b) => a -> b -> Bool
f := \x y. eq x x
main := f True Unit`
	checkSource(t, source, nil)
}

func TestEdgeConstraintSuperclass(t *testing.T) {
	// (Eq a, Ord a) where Ord a => Eq a — curried constraints with redundancy.
	// Both constraints should be available; superclass makes Eq doubly available.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Ord := \a. Eq a => { compare: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Ord Bool := { compare := \x y. True }
f :: \ a. (Eq a, Ord a) => a -> a -> Bool
f := \x y. compare x y
main := f True False`
	checkSource(t, source, nil)
}

func TestEdgeNestedConstraintAlias(t *testing.T) {
	// Constraint alias used in a curried constraints.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Ord := \a. Eq a => { compare: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Ord Bool := { compare := \x y. True }
type EqOrd := \a. (Eq a, Ord a) => a -> Bool
f :: \ a. EqOrd a
f := \x. eq x x
main := f True`
	checkSource(t, source, nil)
}

func TestEdgeConstraintInLet(t *testing.T) {
	// Curried constraints in a block-scoped binding.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Show := \a. { show: a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. True }
f :: \ a. (Eq a, Show a) => a -> Bool
f := \x. { r := eq x x; r }
main := f True`
	checkSource(t, source, nil)
}

func TestEdgeEmptyParensNotTuple(t *testing.T) {
	// () in type position is now valid (unit type = Record {}).
	// () => Bool -> Bool parses but should fail at check time
	// because Record {} is not a constraint.
	source := `form Bool := { True: Bool; False: Bool; }
f :: () -> Bool -> Bool
f := \x. x
main := f True`
	src := span.NewSource("test", source)
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() || es.HasErrors() {
		// Lex/parse error is also acceptable — depends on parser's type resolution.
		return
	}
	// If it parses, it should fail at check time.
	desugar.Program(ast)
	_, checkErrs := Check(ast, src, nil)
	if !checkErrs.HasErrors() {
		t.Fatal("expected check error for () in constraint position, got none")
	}
}

func TestEdgeConstraintWithForall(t *testing.T) {
	// \ a b. (Eq a, Eq b) => Pair a b -> Bool
	source := `form Bool := { True: Bool; False: Bool; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq b => Eq (Pair a b) := { eq := \x y. True }
f :: \ a b. (Eq a, Eq b) => Pair a b -> Pair a b -> Bool
f := \x y. eq x y
main := f (MkPair True True) (MkPair False False)`
	checkSource(t, source, nil)
}

func TestEdgeMissingInstance(t *testing.T) {
	// (Eq a, Ord a) but no Ord instance — should error.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Ord := \a. Eq a => { compare: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
f :: \ a. (Eq a, Ord a) => a -> Bool
f := \x. eq x x
main := f True`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNoInstance)
}

func TestEdgeConstraintInstanceContext(t *testing.T) {
	// Instance with multiple context constraints (curried style).
	source := `form Bool := { True: Bool; False: Bool; }
form Triple := \a b c. { MkTriple: a -> b -> c -> Triple a b c; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq b => Eq c => Eq (Triple a b c) := { eq := \x y. True }
main := eq (MkTriple True True True) (MkTriple False False False)`
	checkSource(t, source, nil)
}

// =============================================================================
// Regression — curried constraints
// =============================================================================

func TestRegressionCurriedConstraints(t *testing.T) {
	// (Eq a, Ord a) => T must behave identically to Eq a => Ord a => T
	// in all aspects: check mode, subsCheck, instantiate.
	templateProd := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Ord := \a. Eq a => { compare: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Ord Bool := { compare := \x y. True }
%s`
	templateCurr := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Ord := \a. Eq a => { compare: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Ord Bool := { compare := \x y. True }
%s`

	cases := []string{
		// Check mode: annotated binding.
		"f :: \\ a. %s a -> a -> Bool\nf := \\x y. eq x y\nmain := f True False",
		// Infer mode: unannotated binding calling eq and compare.
		"f :: \\ a. %s a -> Bool\nf := \\x. compare x x\nmain := f True",
	}

	constraints := []struct {
		product string
		curried string
	}{
		{"(Eq a, Ord a) =>", "Eq a => Ord a =>"},
	}

	for _, tc := range cases {
		for _, c := range constraints {
			prodSrc := fmt.Sprintf(templateProd, fmt.Sprintf(tc, c.product))
			currSrc := fmt.Sprintf(templateCurr, fmt.Sprintf(tc, c.curried))
			checkSource(t, prodSrc, nil)
			checkSource(t, currSrc, nil)
		}
	}
}
