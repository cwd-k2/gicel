//go:build probe

// Shared test helpers for probe tests.
package probe_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Probe C helpers
// ---------------------------------------------------------------------------

// probeRun compiles and executes source with given packs, returning value or error.
func probeRun(t *testing.T, source string, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

// probeSandbox is a convenience wrapper around RunSandbox.
func probeSandbox(source string, cfg *gicel.SandboxConfig) (*gicel.RunResult, error) {
	return gicel.RunSandbox(source, cfg)
}

// probeAssertConVal checks the value is a ConVal with the given constructor name.
func probeAssertConVal(t *testing.T, v gicel.Value, name string) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal(%s), got %T: %v", name, v, v)
	}
	if con.Con != name {
		t.Fatalf("expected ConVal(%s), got ConVal(%s)", name, con.Con)
	}
}

// probeAssertHostInt checks the value is a HostVal wrapping the given int64.
func probeAssertHostInt(t *testing.T, v gicel.Value, expected int64) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
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

// probeAssertHostString checks the value is a HostVal wrapping the given string.
func probeAssertHostString(t *testing.T, v gicel.Value, expected string) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
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

// ---------------------------------------------------------------------------
// Probe D helpers
// ---------------------------------------------------------------------------

// pdRun compiles and executes source with given packs, returning value or error.
func pdRun(t *testing.T, source string, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

// pdRunWithCaps compiles and executes with packs and capabilities.
func pdRunWithCaps(t *testing.T, source string, caps map[string]any, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	eng := gicel.NewEngine()
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

// pdRunWithLimits compiles and executes with custom engine limits.
func pdRunWithLimits(t *testing.T, source string, steps, depth int, alloc int64, packs ...gicel.Pack) (gicel.Value, error) {
	t.Helper()
	eng := gicel.NewEngine()
	eng.SetStepLimit(steps)
	eng.SetDepthLimit(depth)
	eng.SetAllocLimit(alloc)
	for _, p := range packs {
		if err := p(eng); err != nil {
			t.Fatal(err)
		}
	}
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		return nil, err
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	return result.Value, nil
}

func pdAssertInt(t *testing.T, v gicel.Value, expected int64) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
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

func pdAssertString(t *testing.T, v gicel.Value, expected string) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
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

func pdAssertCon(t *testing.T, v gicel.Value, name string) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok {
		t.Fatalf("expected ConVal(%s), got %T: %v", name, v, v)
	}
	if con.Con != name {
		t.Fatalf("expected ConVal(%s), got ConVal(%s)", name, con.Con)
	}
}
