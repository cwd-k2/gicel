// Named capability tests — getAt, putAt, failWithAt with label-parameterized state/fail.
// Does NOT cover: unnamed state/fail (engine_integration_test.go), host API (engine_host_api_test.go).

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestGetAtNamedState(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := getAt @#myState
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	caps := map[string]any{"myState": &eval.HostVal{Inner: int64(99)}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	assertHostInt(t, result.Value, 99)
}

func TestPutAtThenGetAt(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do { putAt @#myState 42; getAt @#myState }
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	caps := map[string]any{"myState": &eval.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	assertHostInt(t, result.Value, 42)
}

func TestFailWithAtProducesError(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.Fail); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Fail
main := failWithAt @#myErr "oops"
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	caps := map[string]any{"myErr": eval.NewRecordFromMap(map[string]eval.Value{})}
	_, err = rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err == nil {
		t.Fatal("expected runtime error from failWithAt")
	}
	if !strings.Contains(err.Error(), "fail") {
		t.Errorf("expected error message containing 'fail', got: %s", err.Error())
	}
}

func TestMultipleNamedStates(t *testing.T) {
	// Two independent named states should not interfere.
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do {
  putAt @#x 10;
  putAt @#y 20;
  a <- getAt @#x;
  b <- getAt @#y;
  pure (add a b)
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	caps := map[string]any{
		"x": &eval.HostVal{Inner: int64(0)},
		"y": &eval.HostVal{Inner: int64(0)},
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	assertHostInt(t, result.Value, 30)
}

func TestNamedStateWithUnnamedState(t *testing.T) {
	// Named state and unnamed state should coexist.
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do {
  put 100;
  putAt @#named 200;
  a <- get;
  b <- getAt @#named;
  pure (add a b)
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	caps := map[string]any{
		"state": &eval.HostVal{Inner: int64(0)},
		"named": &eval.HostVal{Inner: int64(0)},
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{Caps: caps})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	assertHostInt(t, result.Value, 300)
}

func TestGetAtMissingCapability(t *testing.T) {
	// getAt on a capability that is not provided should produce a runtime error.
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(stdlib.State); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := getAt @#missing
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{Caps: map[string]any{}})
	if err == nil {
		t.Fatal("expected runtime error for missing capability")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected error mentioning 'missing', got: %s", err.Error())
	}
}
