// Named capability tests — label-parameterized state/fail/array/ref/mmap/mset.
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

// --- Named Array capabilities ---

func TestNamedArrayReadWrite(t *testing.T) {
	eng := NewEngine()
	for _, p := range []stdlib.Pack{stdlib.Prelude, stdlib.Array} {
		if err := eng.Use(p); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Array
main := do {
  arr <- newAt @#buf 3 0;
  writeAt @#buf 1 42 arr;
  readAt @#buf 1 arr
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	// Result should be Just 42
	cv, ok := result.Value.(*eval.ConVal)
	if !ok || cv.Con != "Just" {
		t.Fatalf("expected Just, got %v", result.Value)
	}
	assertHostInt(t, cv.Args[0], 42)
}

func TestNamedArrayTwoLabels(t *testing.T) {
	eng := NewEngine()
	for _, p := range []stdlib.Pack{stdlib.Prelude, stdlib.Array} {
		if err := eng.Use(p); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Array
main := do {
  a <- newAt @#x 2 10;
  b <- newAt @#y 2 20;
  va <- readAt @#x 0 a;
  vb <- readAt @#y 0 b;
  pure (maybe 0 id va + maybe 0 id vb)
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	assertHostInt(t, result.Value, 30)
}

// --- Named Ref capabilities ---

func TestNamedRefReadWriteModify(t *testing.T) {
	eng := NewEngine()
	for _, p := range []stdlib.Pack{stdlib.Prelude, stdlib.Ref} {
		if err := eng.Use(p); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Ref
main := do {
  r <- newAt @#cell 10;
  writeAt @#cell 20 r;
  modifyAt @#cell (\x. x + 5) r;
  readAt @#cell r
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	assertHostInt(t, result.Value, 25)
}

// --- Named MMap capabilities ---

func TestNamedMMapInsertLookup(t *testing.T) {
	eng := NewEngine()
	for _, p := range []stdlib.Pack{stdlib.Prelude, stdlib.EffectMap} {
		if err := eng.Use(p); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Map
main := do {
  m <- newAt @#db compare;
  insertAt @#db 1 "one" m;
  insertAt @#db 2 "two" m;
  v <- lookupAt @#db 1 m;
  pure v
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	cv, ok := result.Value.(*eval.ConVal)
	if !ok || cv.Con != "Just" {
		t.Fatalf("expected Just, got %v", result.Value)
	}
}

// --- Named MSet capabilities ---

func TestNamedMSetInsertMember(t *testing.T) {
	eng := NewEngine()
	for _, p := range []stdlib.Pack{stdlib.Prelude, stdlib.EffectSet} {
		if err := eng.Use(p); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Set
main := do {
  s <- newAt @#tags compare;
  insertAt @#tags 42 s;
  memberAt @#tags 42 s
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	cv, ok := result.Value.(*eval.ConVal)
	if !ok || cv.Con != "True" {
		t.Fatalf("expected True, got %v", result.Value)
	}
}
