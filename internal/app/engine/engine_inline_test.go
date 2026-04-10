// Inline optimization tests — verifies that selective inlining works correctly.
// Does NOT cover: explain trace interaction (engine_explain_test.go).

package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestInlineSmallHelper(t *testing.T) {
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

id := \x. x
main := id 42
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := res.Value.(*eval.HostVal); !ok || v.Inner != int64(42) {
		t.Fatalf("expected 42, got %v", eval.PrettyValue(res.Value))
	}
}

func TestInlineDisabledPreservesSemantics(t *testing.T) {
	eng := NewEngine()
	eng.DisableInlining()
	_ = eng.Use(stdlib.Prelude)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

id := \x. x
main := id 42
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := res.Value.(*eval.HostVal); !ok || v.Inner != int64(42) {
		t.Fatalf("expected 42, got %v", eval.PrettyValue(res.Value))
	}
}

// TestInlineCaseOfCaseCapture is a regression test for a variable-capture
// bug in caseOfCase: when an inlined function's pattern bindings share
// names with the caller's bindings, the outer body's free-variable
// references were incorrectly captured by the inner pattern after
// case-of-case transformation.
func TestInlineCaseOfCaseCapture(t *testing.T) {
	eng := NewEngine()
	eng.EnableRecursion()
	_ = eng.Use(stdlib.Prelude)

	// Two-level recursive descent parser where both levels use the same
	// pattern variable names (left, rest). Without capture avoidance,
	// the inlined inner pattern shadows the outer 'left'.
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

parseNum :: List Rune -> Maybe (Int, List Rune)
parseNum := \cs. go 0 0 cs

go :: Int -> Int -> List Rune -> Maybe (Int, List Rune)
go := \acc d input. case input {
  Cons c rest =>
    if isDigit c
      then case digitToInt c {
        Just v => go (acc * 10 + v) (d + 1) rest;
        Nothing => Nothing
      }
      else if d > 0 then Just (acc, input) else Nothing;
  Nil => if d > 0 then Just (acc, []) else Nothing
}

mul :: List Rune -> Maybe (Int, List Rune)
mul := \cs. case parseNum cs {
  Just (left, rest) => mulTail left rest;
  Nothing => Nothing
}

mulTail :: Int -> List Rune -> Maybe (Int, List Rune)
mulTail := \left cs. case cs {
  Cons c rest =>
    if c == '*'
      then case parseNum rest {
        Just (right, rest2) => mulTail (left * right) rest2;
        Nothing => Nothing
      }
      else Just (left, cs);
  Nil => Just (left, [])
}

add :: List Rune -> Maybe (Int, List Rune)
add := \cs. case mul cs {
  Just (left, rest) => addTail left rest;
  Nothing => Nothing
}

addTail :: Int -> List Rune -> Maybe (Int, List Rune)
addTail := \left cs. case cs {
  Cons c rest =>
    if c == '+'
      then case mul rest {
        Just (right, rest2) => addTail (left + right) rest2;
        Nothing => Nothing
      }
      else Just (left, cs);
  Nil => Just (left, [])
}

calc :: String -> Maybe Int
calc := \s. case add (toRunes s) {
  Just (v, _) => Just v;
  Nothing => Nothing
}

main := (calc "3+4*2", calc "1+2+3+4")
`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := eval.PrettyValue(res.Value)
	want := "(Just 11, Just 10)"
	if got != want {
		t.Errorf("case-of-case capture: got %s, want %s", got, want)
	}
}

func TestInlineDoesNotBreakExplain(t *testing.T) {
	eng := NewEngine()
	eng.DisableInlining()
	_ = eng.Use(stdlib.Prelude)

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude

double := \x. x + x
main := double 21
`)
	if err != nil {
		t.Fatal(err)
	}
	var enterCount int
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Explain: func(step eval.ExplainStep) {
			if step.Kind == eval.ExplainLabel && step.Detail.LabelKind == "enter" && step.Detail.Name == "double" {
				enterCount++
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if enterCount != 1 {
		t.Errorf("expected 1 'enter double' label, got %d", enterCount)
	}
}
