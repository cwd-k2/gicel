// Review 2026-04-12 regression tests — QL kind check, label collision namespace, alloc budget.
// Does NOT cover: type family re-activation (false positive), quantified constraint (false positive).

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// --- A-3: QL kind check ---

func TestQLKindCheckRejectsRowMetaWithTypeSolution(t *testing.T) {
	// A Row-kinded meta should not be solved with a Type-kinded value.
	// Before the fix, qlKindClash only caught a few hard-coded cases.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.Compile(context.Background(), `
import Prelude

-- Force a situation where Quick Look tries to unify a Row meta with Int.
-- rowId expects a Row argument; passing Int should be rejected.
rowId :: \(r: Row). { | r } -> { | r }
rowId := \x. x

main := rowId 42
`)
	if err == nil {
		t.Fatal("expected type error: Row-kinded meta should not accept Int")
	}
}

// --- D-2: Label erasure namespace ---

func TestLabelErasureNamespaceAnonymousVsNamed(t *testing.T) {
	// Anonymous state (capState = "state") and named @#myS must not collide.
	// Named labels erase to "#<name>" prefix, so @#myS becomes "#myS".
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.State)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  put 100;
  putAt @#myS 200;
  a <- get;
  b <- getAt @#myS;
  pure a
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	caps := map[string]any{
		"state": &eval.HostVal{Inner: int64(0)},
		"#myS":  &eval.HostVal{Inner: int64(0)},
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	// Anonymous get should return 100 (not corrupted by named state).
	assertHostInt(t, result.Value, 100)
}

// --- stdlib alloc budget ---

func TestAllocBudgetFromSlice(t *testing.T) {
	// fromSlice (list materialization) should respect allocation budget.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.SetAllocLimit(128) // very tight limit
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
-- Create a list with enough elements to exhaust the budget.
main := replicate 1000 1
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{})
	if err == nil {
		t.Fatal("expected alloc budget error")
	}
	if !strings.Contains(err.Error(), "alloc") {
		t.Logf("got error: %s", err)
	}
}

func TestAllocBudgetSeqOperations(t *testing.T) {
	// Sequence fromList + reverse should charge allocation budget.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.Use(stdlib.Sequence)
	eng.SetAllocLimit(256) // tight limit
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Data.Sequence

bigList :: List Int
bigList := replicate 1000 1

seqFromList :: Seq Int
seqFromList := fromList bigList

main := seqFromList
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{})
	if err == nil {
		t.Fatal("expected alloc budget error for Seq.fromList")
	}
}
