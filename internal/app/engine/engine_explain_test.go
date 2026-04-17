package engine

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// ── Explain: line numbers (#4) ──

func TestExplainStepHasLineNumbers(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	var steps []eval.ExplainStep

	// Lines:
	// 1: import Effect.State
	// 2: main := do {
	// 3:   _ <- put 42;
	// 4:   get
	// 5: }
	rt, err := eng.NewRuntime(context.Background(), "import Effect.State\nmain := do {\n  _ <- put 42;\n  get\n}")
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Explain: func(s eval.ExplainStep) { steps = append(steps, s) },
	})
	if err != nil {
		t.Fatal(err)
	}

	// Filter to effect events (put, get) — these should have line numbers.
	var effects []eval.ExplainStep
	for _, s := range steps {
		if s.Kind == eval.ExplainEffect {
			effects = append(effects, s)
		}
	}
	if len(effects) < 2 {
		t.Fatalf("expected at least 2 effect events, got %d", len(effects))
	}

	for i, e := range effects {
		if e.Line == 0 {
			t.Errorf("effect[%d] op=%s: expected non-zero Line", i, e.Detail.Op)
		}
	}
}

func TestExplainLineNumbersForBindAndMatch(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)

	// Use an opaque binding so the optimizer cannot eliminate the bind.
	eng.DeclareBinding("getBool", testOps.Comp(
		EmptyRowType(), EmptyRowType(), testOps.Con("Bool"), nil,
	))

	var steps []eval.ExplainStep
	rt, err := eng.NewRuntime(context.Background(), "import Prelude\nmain := do {\n  x <- getBool;\n  case x {\n    True => pure x;\n    False => pure x\n  }\n}")
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Bindings: map[string]eval.Value{"getBool": &eval.ConVal{Con: "True"}},
		Explain:  func(s eval.ExplainStep) { steps = append(steps, s) },
	})
	if err != nil {
		t.Fatal(err)
	}

	var binds, matches []eval.ExplainStep
	for _, s := range steps {
		switch s.Kind {
		case eval.ExplainBind:
			binds = append(binds, s)
		case eval.ExplainMatch:
			matches = append(matches, s)
		}
	}

	if len(binds) == 0 {
		t.Fatal("expected at least 1 bind event")
	}
	if binds[0].Line == 0 {
		t.Errorf("bind var=%s: expected non-zero Line", binds[0].Detail.Var)
	}

	if len(matches) == 0 {
		t.Fatal("expected at least 1 match event")
	}
	if matches[0].Line == 0 {
		t.Errorf("match pattern=%s: expected non-zero Line", matches[0].Detail.Pattern)
	}
}

// ── Explain: JSON output (#6) ──

func TestExplainStepJSON(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	var steps []eval.ExplainStep
	rt, err := eng.NewRuntime(context.Background(), "import Effect.State\nmain := do {\n  _ <- put 42;\n  get\n}")
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Explain: func(s eval.ExplainStep) { steps = append(steps, s) },
	})
	if err != nil {
		t.Fatal(err)
	}

	// ExplainStep should be JSON-marshalable with structured fields.
	for _, s := range steps {
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("ExplainStep should marshal to JSON: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("ExplainStep JSON should unmarshal to map: %v", err)
		}
		if _, ok := m["kind"]; !ok {
			t.Error("ExplainStep JSON missing 'kind' field")
		}
	}
}

// ── Explain: function call boundaries (#5) and stdlib suppression (#7) ──

func TestExplainFunctionBoundaries(t *testing.T) {
	eng := NewEngine()
	eng.DisableInlining() // explain traces require function boundaries
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	var steps []eval.ExplainStep
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

step := \u. do {
  _ <- modify (\n. n + 1);
  get
}

main := do {
  _ <- put 0;
  _ <- step ();
  _ <- step ();
  get
}
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Explain: func(s eval.ExplainStep) { steps = append(steps, s) },
	})
	if err != nil {
		t.Fatal(err)
	}

	// Count "enter" label events for the user-defined "step" function.
	enterCount := 0
	for _, s := range steps {
		if s.Kind == eval.ExplainLabel && s.Detail.LabelKind == "enter" && s.Detail.Name == "step" {
			enterCount++
		}
	}
	if enterCount != 2 {
		t.Errorf("expected 2 'enter step' labels, got %d", enterCount)
	}
}

func TestExplainStdlibSuppression(t *testing.T) {
	eng := NewEngine()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.State)

	var steps []eval.ExplainStep
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

main := do {
  _ <- put 0;
  _ <- modify (\n. n + 1);
  get
}
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Explain: func(s eval.ExplainStep) { steps = append(steps, s) },
	})
	if err != nil {
		t.Fatal(err)
	}

	// When stdlib suppression is active, modify's internal get/s/put
	// should NOT appear. Only the user-level effects should be visible.
	for _, s := range steps {
		if s.Kind == eval.ExplainBind && s.Detail.Var == "s" && s.Detail.Value == "0" {
			t.Errorf("stdlib internal bind 's ← 0' should be suppressed")
		}
	}
}

func TestExplainKindNegativeJSON(t *testing.T) {
	step := eval.ExplainStep{Kind: eval.ExplainKind(-1)}
	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("should not error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("should unmarshal: %v", err)
	}
	if m["kind"] != "unknown" {
		t.Errorf("negative kind should serialize as 'unknown', got %v", m["kind"])
	}
}

func TestTraceHookViaRunWith(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	var events []eval.TraceEvent
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Trace: func(e eval.TraceEvent) error {
			events = append(events, e)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Error("expected at least one trace event")
	}
}

func TestSetCheckTraceHookPublicAPI(t *testing.T) {
	eng := NewEngine()
	var events []check.CheckTraceEvent
	eng.SetCheckTraceHook(func(e check.CheckTraceEvent) {
		events = append(events, e)
	})
	_, err := eng.Compile(context.Background(), `
form MyBool := { T: MyBool; F: MyBool; }
id := \x. x
main := id T
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Error("expected at least one check trace event")
	}
}

// ── Explain: merge + IO interaction (#E) ──

func TestExplainMergeWithIO(t *testing.T) {
	// Regression: capValEqual panicked on uncomparable CapEnv values
	// ([]string from IO output buffer) when explain trace computed CapEnv diffs.
	eng := NewEngine()
	eng.DisableInlining()
	_ = eng.Use(stdlib.Prelude)
	_ = eng.Use(stdlib.IO)
	_ = eng.Use(stdlib.State)

	var steps []eval.ExplainStep
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.IO
import Effect.State

main := do {
  _ <- put 1;
  _ <- log "hello";
  r <- merge (do { get }) (do { pure (2 :: Int) });
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": int64(0)}
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Explain: func(s eval.ExplainStep) { steps = append(steps, s) },
		Caps:    caps,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) == 0 {
		t.Error("expected trace steps")
	}
}
