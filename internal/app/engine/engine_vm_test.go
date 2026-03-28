// VM backend integration tests.
// Does NOT cover: vm unit tests (internal/runtime/vm/).
package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

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
