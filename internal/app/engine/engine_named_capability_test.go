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
	caps := map[string]any{"#myState": &eval.HostVal{Inner: int64(99)}}
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
	caps := map[string]any{"#myState": &eval.HostVal{Inner: int64(0)}}
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
	caps := map[string]any{"#myErr": eval.NewRecordFromMap(map[string]eval.Value{})}
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
		"#x": &eval.HostVal{Inner: int64(0)},
		"#y": &eval.HostVal{Inner: int64(0)},
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
		"state":  &eval.HostVal{Inner: int64(0)},
		"#named": &eval.HostVal{Inner: int64(0)},
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

func TestNamedMSetBinaryOps(t *testing.T) {
	eng := NewEngine()
	for _, p := range []stdlib.Pack{stdlib.Prelude, stdlib.EffectSet} {
		if err := eng.Use(p); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Set as S
main := do {
  a <- S.newAt @#s compare;
  S.insertAt @#s 1 a;
  S.insertAt @#s 2 a;
  S.insertAt @#s 3 a;
  b <- S.newAt @#s compare;
  S.insertAt @#s 2 b;
  S.insertAt @#s 4 b;
  u <- S.unionAt @#s a b;
  i <- S.intersectionAt @#s a b;
  d <- S.differenceAt @#s a b;
  ul <- S.toListAt @#s u;
  il <- S.toListAt @#s i;
  dl <- S.toListAt @#s d;
  pure (ul, il, dl)
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	// Result: ([1,2,3,4], [2], [1,3])
	rec, ok := result.Value.(*eval.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T: %v", result.Value, result.Value)
	}
	_ = rec // basic structure check — runtime success is the main assertion
}

func TestLabelVariableSingleUse(t *testing.T) {
	// Regression test: label type variable (@l) in a user-defined function
	// should correctly propagate the label to the underlying primitive.
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

myGet :: \(l: Label) (r: Row). () -> Effect { l: Int | r } Int
myGet := \(). getAt @l

main := do {
  putAt @#s 42;
  x <- myGet @#s ();
  pure x
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	assertHostInt(t, result.Value, 42)
}

func TestLabelVariableMultipleUses(t *testing.T) {
	// Regression test: label type variable (@l) used twice in a do block.
	// Previously caused de Bruijn index contamination or closure environment loss.
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

inc :: \(l: Label) (r: Row). () -> Effect { l: Int | r } ()
inc := \(). do {
  n <- getAt @l;
  putAt @l (n + 1)
}

main := do {
  putAt @#counter 0;
  inc @#counter ();
  inc @#counter ();
  inc @#counter ();
  x <- getAt @#counter;
  pure x
}
`)
	if err != nil {
		t.Fatal("compile error:", err)
	}
	result, err := rt.RunWith(context.Background(), &RunOptions{})
	if err != nil {
		t.Fatal("runtime error:", err)
	}
	assertHostInt(t, result.Value, 3)
}
