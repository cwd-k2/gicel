package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
)

// =============================================================================
// Stress Tests — Evidence Resolution under load
// =============================================================================

func TestStressDeepSuperclassChain(t *testing.T) {
	// Chain of 5 superclasses: C5 => C4 => C3 => C2 => C1.
	// Using C1's method with only C5 in scope requires traversing 4 levels.
	source := `data Bool = True | False
class C1 a { m1 :: a -> Bool }
class C1 a => C2 a { m2 :: a -> Bool }
class C2 a => C3 a { m3 :: a -> Bool }
class C3 a => C4 a { m4 :: a -> Bool }
class C4 a => C5 a { m5 :: a -> Bool }
instance C1 Bool { m1 := \x. True }
instance C2 Bool { m2 := \x. True }
instance C3 Bool { m3 := \x. True }
instance C4 Bool { m4 := \x. True }
instance C5 Bool { m5 := \x. True }
f :: \ a. C5 a => a -> Bool
f := \x. m1 x
main := f True`
	checkSource(t, source, nil)
}

func TestStressManyInstancesOneClass(t *testing.T) {
	// 10 types, each with an Eq instance. Resolve Eq for each.
	var sb strings.Builder
	sb.WriteString("data Bool = True | False\n")
	sb.WriteString("class Eq a { eq :: a -> a -> Bool }\n")
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("T%d", i)
		sb.WriteString(fmt.Sprintf("data %s = Mk%s\n", name, name))
		sb.WriteString(fmt.Sprintf("instance Eq %s { eq := \\x y. True }\n", name))
	}
	// Use eq on each type.
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("T%d", i)
		sb.WriteString(fmt.Sprintf("test%d := eq Mk%s Mk%s\n", i, name, name))
	}
	checkSource(t, sb.String(), nil)
}

func TestStressManyClasses(t *testing.T) {
	// 10 independent classes, each with one instance.
	var sb strings.Builder
	sb.WriteString("data Bool = True | False\n")
	for i := 0; i < 10; i++ {
		sb.WriteString(fmt.Sprintf("class C%d a { m%d :: a -> Bool }\n", i, i))
	}
	for i := 0; i < 10; i++ {
		sb.WriteString(fmt.Sprintf("instance C%d Bool { m%d := \\x. True }\n", i, i))
	}
	// Function requiring all 10 constraints (curried style).
	sb.WriteString("f :: \\ a. ")
	for i := 0; i < 10; i++ {
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
	source := `data Bool = True | False
data F a = MkF a
data G a = MkG a
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq (F a) { eq := \x y. True }
instance Eq a => Eq (G a) { eq := \x y. True }
main := eq (MkF (MkG True)) (MkF (MkG False))`
	checkSource(t, source, nil)
}

func TestStressMultiParamConstraints(t *testing.T) {
	// Curried constraints with different type variables.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Show a { show :: a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Show Bool { show := \x. True }
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
	source := `data Bool = True | False
data Unit = Unit
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq Unit { eq := \x y. True }
f :: \ a b. (Eq a, Eq b) => a -> b -> Bool
f := \x y. eq x x
main := f True Unit`
	checkSource(t, source, nil)
}

func TestEdgeConstraintSuperclass(t *testing.T) {
	// (Eq a, Ord a) where Ord a => Eq a — curried constraints with redundancy.
	// Both constraints should be available; superclass makes Eq doubly available.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
f :: \ a. (Eq a, Ord a) => a -> a -> Bool
f := \x y. compare x y
main := f True False`
	checkSource(t, source, nil)
}

func TestEdgeNestedConstraintAlias(t *testing.T) {
	// Constraint alias used in a curried constraints.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
type EqOrd a = (Eq a, Ord a) => a -> Bool
f :: \ a. EqOrd a
f := \x. eq x x
main := f True`
	checkSource(t, source, nil)
}

func TestEdgeConstraintInLet(t *testing.T) {
	// Curried constraints in a block-scoped binding.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Show a { show :: a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Show Bool { show := \x. True }
f :: \ a. (Eq a, Show a) => a -> Bool
f := \x. { r := eq x x; r }
main := f True`
	checkSource(t, source, nil)
}

func TestEdgeEmptyParensNotTuple(t *testing.T) {
	// () in type position is now valid (unit type = Record {}).
	// () => Bool -> Bool parses but should fail at check time
	// because Record {} is not a constraint.
	source := `data Bool = True | False
f :: () => Bool -> Bool
f := \x. x
main := f True`
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
		// Parse error is also acceptable — depends on parser's type resolution.
		return
	}
	// If it parses, it should fail at check time.
	_, checkErrs := Check(ast, src, nil)
	if !checkErrs.HasErrors() {
		t.Fatal("expected check error for () in constraint position, got none")
	}
}

func TestEdgeConstraintWithForall(t *testing.T) {
	// \ a b. (Eq a, Eq b) => Pair a b -> Bool
	source := `data Bool = True | False
data Pair a b = MkPair a b
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq b => Eq (Pair a b) { eq := \x y. True }
f :: \ a b. (Eq a, Eq b) => Pair a b -> Pair a b -> Bool
f := \x y. eq x y
main := f (MkPair True True) (MkPair False False)`
	checkSource(t, source, nil)
}

func TestEdgeMissingInstance(t *testing.T) {
	// (Eq a, Ord a) but no Ord instance — should error.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
f :: \ a. (Eq a, Ord a) => a -> Bool
f := \x. eq x x
main := f True`
	checkSourceExpectCode(t, source, nil, errs.ErrNoInstance)
}

func TestEdgeConstraintInstanceContext(t *testing.T) {
	// Instance with multiple context constraints (curried style).
	source := `data Bool = True | False
data Triple a b c = MkTriple a b c
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq b => Eq c => Eq (Triple a b c) { eq := \x y. True }
main := eq (MkTriple True True True) (MkTriple False False False)`
	checkSource(t, source, nil)
}

// =============================================================================
// Regression — curried constraints
// =============================================================================

func TestRegressionCurriedConstraints(t *testing.T) {
	// (Eq a, Ord a) => T must behave identically to Eq a => Ord a => T
	// in all aspects: check mode, subsCheck, instantiate.
	templateProd := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
%s`
	templateCurr := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
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
