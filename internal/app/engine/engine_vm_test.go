// VM backend integration tests.
// Does NOT cover: vm unit tests (internal/runtime/vm/).
package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

type Pack = registry.Pack

func TestVMBackendLiteral(t *testing.T) {
	eng := NewEngine()
	eng.SetBackend(BackendVM)
	rt, err := eng.NewRuntime(context.Background(), `main := 42`)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 42)
}

func TestVMBackendRecState(t *testing.T) {
	eng := NewEngine()
	eng.SetBackend(BackendVM)
	eng.EnableRecursion()
	eng.Use(stdlibPrelude)
	eng.Use(stdlibState)
	eng.SetStepLimit(1000000)
	eng.SetNestingLimit(100000)
	src := `import Prelude
import Effect.State
main := do {
  put 3;
  rec (\self. do {
    n <- get;
    case n == 0 {
      True  => pure n;
      False => do { put (n - 1); self }
    }
  })
}`
	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("IR: %s", rt.Program().Pretty())
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 0)
}

var stdlibPrelude = func() Pack {
	return stdlib.Prelude
}()
var stdlibState = func() Pack {
	return stdlib.State
}()

func TestVMBackendFixSimple(t *testing.T) {
	eng := NewEngine()
	eng.SetBackend(BackendVM)
	eng.EnableRecursion()
	src := `main := fix (\self n. n) 42`
	rt, err := eng.NewRuntime(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	// Dump entry IR.
	t.Logf("Entry IR: %s", rt.Program().Pretty())
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertVMInt(t, res, 42)
}

func assertVMInt(t *testing.T, res *RunResult, expected int64) {
	t.Helper()
	hv, ok := res.Value.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T: %s", res.Value, res.Value)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T", hv.Inner)
	}
	if n != expected {
		t.Fatalf("expected %d, got %d", expected, n)
	}
}
