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
