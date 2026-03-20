// Shared test helpers for internal/engine/ tests.

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// runWithPacks compiles source with given packs and evaluates "main".
func runWithPacks(t *testing.T, source string, packs ...registry.Pack) eval.Value {
	t.Helper()
	eng := NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.Value
}

// runPure compiles a source fragment (with Prelude) and evaluates "main".
func runPure(t *testing.T, source string) eval.Value {
	t.Helper()
	eng := NewEngine()
	if err := stdlib.Prelude(eng); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "import Prelude") {
		source = "import Prelude\n" + source
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return result.Value
}

// assertHostString checks that v is a HostVal containing the expected string.
func assertHostString(t *testing.T, v eval.Value, expected string) {
	t.Helper()
	hv, ok := v.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal(%q), got %T: %v", expected, v, v)
	}
	s, ok := hv.Inner.(string)
	if !ok {
		t.Fatalf("expected string, got %T: %v", hv.Inner, hv.Inner)
	}
	if s != expected {
		t.Fatalf("expected %q, got %q", expected, s)
	}
}

// assertHostBool checks that v is a ConVal representing the expected boolean.
func assertHostBool(t *testing.T, v eval.Value, expected bool) {
	t.Helper()
	con, ok := v.(*eval.ConVal)
	if !ok {
		t.Fatalf("expected ConVal(True/False), got %T: %v", v, v)
	}
	if expected && con.Con != "True" {
		t.Fatalf("expected True, got %s", con.Con)
	}
	if !expected && con.Con != "False" {
		t.Fatalf("expected False, got %s", con.Con)
	}
}

// assertHostInt checks that v is a HostVal containing the expected int64.
func assertHostInt(t *testing.T, v eval.Value, expected int64) {
	t.Helper()
	hv, ok := v.(*eval.HostVal)
	if !ok {
		t.Fatalf("expected HostVal(%d), got %T: %v", expected, v, v)
	}
	n, ok := hv.Inner.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T: %v", hv.Inner, hv.Inner)
	}
	if n != expected {
		t.Fatalf("expected %d, got %d", expected, n)
	}
}

// assertConName checks that v is a ConVal with the expected constructor name.
func assertConName(t *testing.T, v eval.Value, name string) {
	t.Helper()
	con, ok := v.(*eval.ConVal)
	if !ok {
		t.Errorf("expected ConVal(%s), got %T: %s", name, v, v)
		return
	}
	if name != "" && con.Con != name {
		t.Errorf("expected %s, got %s", name, con.Con)
	}
}

// assertList checks that a Value is a List with the given int64 elements.
func assertList(t *testing.T, v eval.Value, expected []int64) {
	t.Helper()
	for i, want := range expected {
		con, ok := v.(*eval.ConVal)
		if !ok || con.Con != "Cons" {
			t.Fatalf("element %d: expected Cons, got %v", i, v)
		}
		if len(con.Args) != 2 {
			t.Fatalf("element %d: Cons has %d args, expected 2", i, len(con.Args))
		}
		hv, ok := con.Args[0].(*eval.HostVal)
		if !ok {
			t.Fatalf("element %d: expected HostVal, got %T", i, con.Args[0])
		}
		if hv.Inner != want {
			t.Fatalf("element %d: expected %d, got %v", i, want, hv.Inner)
		}
		v = con.Args[1]
	}
	con, ok := v.(*eval.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil at end, got %v", v)
	}
}
