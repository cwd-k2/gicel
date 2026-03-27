// Evaluator stress tests — recursive let, nested pattern matching, records, tuples, CBPV triad.
// Does NOT cover: stress_parser_test.go, stress_checker_test.go, stress_correctness_test.go, stress_crosscutting_test.go, stress_specialized_test.go, stress_limits_test.go.
package stress_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// E1. Evaluator: LetRec (recursive let) with fix
// ---------------------------------------------------------------------------

func TestLetRecFix(t *testing.T) {
	// Both inferred and annotated polymorphic recursion work correctly.
	eng := gicel.NewEngine()
	eng.EnableRecursion()
	eng.Use(gicel.Prelude)

	// Inferred version (no annotation):
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

myLength := fix (\self xs. case xs { Nil => 0; Cons _ rest => 1 + self rest })

main := myLength (Cons True (Cons False (Cons True Nil)))
`)
	if err != nil {
		t.Fatal("inferred version should succeed:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}

	// Annotated version with implicit forall:
	eng2 := gicel.NewEngine()
	eng2.EnableRecursion()
	eng2.Use(gicel.Prelude)
	rt2, err := eng2.NewRuntime(context.Background(), `
import Prelude

myLength :: List a -> Int
myLength := fix (\self xs. case xs { Nil => 0; Cons _ rest => 1 + self rest })

main := myLength (Cons True (Cons False (Cons True Nil)))
`)
	if err != nil {
		t.Fatal("annotated version should work:", err)
	}
	result2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv2 := gicel.MustHost[int64](result2.Value)
	if hv2 != 3 {
		t.Errorf("expected 3, got %d", hv2)
	}
}

// ---------------------------------------------------------------------------
// E2. Evaluator: Pattern matching with nested constructors
// ---------------------------------------------------------------------------

func TestNestedConstructorPatternMatch(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
form Pair := \a b. { MkPair: a -> b -> Pair a b; }

fst :: \a b. Pair a b -> a
fst := \p. case p { MkPair x _ => x }

snd :: \a b. Pair a b -> b
snd := \p. case p { MkPair _ y => y }

main := fst (MkPair True False)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

func TestDoubleNestedPatternMatch(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
isJustTrue :: Maybe Bool -> Bool
isJustTrue := \m. case m {
  Just b => case b { True => True; False => False };
  Nothing => False
}
main := isJustTrue (Just True)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// E3. Evaluator: Record operations end-to-end
// ---------------------------------------------------------------------------

func TestRecordProjectionEndToEnd(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
r := { x: 1, y: 2 }
main := r.#x + r.#y
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 3 {
		t.Errorf("expected 3, got %d", hv)
	}
}

func TestRecordUpdateEndToEnd(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
r := { x: 1, y: 2 }
r2 := { r | x: 100 }
main := r2.#x + r2.#y
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 102 {
		t.Errorf("expected 102, got %d", hv)
	}
}

func TestRecordPatternMatchEndToEnd(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
swap := \r. case r { { x: a, y: b } => { x: b, y: a } }
r := swap { x: 1, y: 2 }
main := r.#x
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 2 {
		t.Errorf("expected 2 (swapped), got %d", hv)
	}
}

// ---------------------------------------------------------------------------
// E4. Evaluator: Tuple operations
// ---------------------------------------------------------------------------

func TestTupleConstructAndProject(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
t := (1, 2, 3)
main := t.#_1 + t.#_2 + t.#_3
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	hv := gicel.MustHost[int64](result.Value)
	if hv != 6 {
		t.Errorf("expected 6, got %d", hv)
	}
}

func TestTuplePatternMatch(t *testing.T) {
	// Tuple patterns in lambda position are desugared to case expressions
	// via isStructuredPattern. Both case and direct lambda forms work.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	// Case destructure form:
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
myFst := \p. case p { (a, b) => a }
main := myFst (True, False)
`)
	if err != nil {
		t.Fatal("case destructure should succeed:", err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")

	// Direct lambda pattern form:
	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	rt2, err := eng2.NewRuntime(context.Background(), `
import Prelude
myFst := \(a, b). a
main := myFst (True, False)
`)
	if err != nil {
		t.Fatal("tuple pattern in lambda should work:", err)
	}
	result2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result2.Value, "True")
}

// ---------------------------------------------------------------------------
// E5. Evaluator: CBPV triad (pure/thunk/force) correctness
// ---------------------------------------------------------------------------

func TestCBPVTriadInDo(t *testing.T) {
	// thunk delays evaluation; force resumes it. pure lifts values.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := do {
  t := thunk (pure True);
  x <- force t;
  pure x
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

func TestForceNonThunkError(t *testing.T) {
	// Forcing a non-thunk value should produce a type error at compile time.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := force True
`)
	if err == nil {
		t.Fatal("expected compile error: cannot force a non-thunk")
	}
}
