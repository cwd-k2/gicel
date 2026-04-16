// Runtime warn callback tests — per-call warning sink, no cross-engine leak.
// Does NOT cover: cache hit/miss (runtimecache_test.go).

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// TestRunWith_WarnFiresOnUndeclaredBinding verifies the basic case:
// providing a binding that was not declared via DeclareBinding fires
// the per-call Warn sink with a message naming the typo'd binding.
func TestRunWith_WarnFiresOnUndeclaredBinding(t *testing.T) {
	eng := NewEngine()
	eng.DeclareBinding("declared", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `main := 1`)
	if err != nil {
		t.Fatal(err)
	}
	var warnings []string
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Bindings: map[string]eval.Value{
			"declared": &eval.HostVal{Inner: int64(1)},
			"typo":     &eval.HostVal{Inner: int64(2)},
		},
		Warn: func(msg string) { warnings = append(warnings, msg) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], `"typo"`) {
		t.Errorf("warning should name the undeclared binding, got %q", warnings[0])
	}
}

// TestRunWith_WarnIsolatedAcrossRuntimeCacheHit is the regression test
// for the stage2 review Finding 1: a *Runtime returned from the
// process-global cache must not carry a callback from an earlier
// embedder's NewRuntime call. Each RunWith reads its own Warn from
// RunOptions, so the warning surfaces in the caller's sink and only
// there.
func TestRunWith_WarnIsolatedAcrossRuntimeCacheHit(t *testing.T) {
	ResetRuntimeCache()
	defer ResetRuntimeCache()

	source := `main := 1`

	var sink1 []string
	eng1 := NewEngine()
	eng1.DeclareBinding("host_x", ConType("Int"))
	rt1, err := eng1.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	var sink2 []string
	eng2 := NewEngine()
	eng2.DeclareBinding("host_x", ConType("Int"))
	rt2, err := eng2.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 != rt2 {
		t.Fatalf("expected runtime cache hit (identical config + source); rt1=%p rt2=%p", rt1, rt2)
	}

	// eng1 runs first with sink1.
	if _, err := rt1.RunWith(context.Background(), &RunOptions{
		Bindings: map[string]eval.Value{
			"host_x": &eval.HostVal{Inner: int64(7)},
			"typo_a": &eval.HostVal{Inner: int64(1)},
		},
		Warn: func(msg string) { sink1 = append(sink1, msg) },
	}); err != nil {
		t.Fatal(err)
	}
	// eng2 then runs with sink2 — must not bleed into sink1.
	if _, err := rt2.RunWith(context.Background(), &RunOptions{
		Bindings: map[string]eval.Value{
			"host_x": &eval.HostVal{Inner: int64(8)},
			"typo_b": &eval.HostVal{Inner: int64(2)},
		},
		Warn: func(msg string) { sink2 = append(sink2, msg) },
	}); err != nil {
		t.Fatal(err)
	}

	if len(sink1) != 1 || !strings.Contains(sink1[0], "typo_a") {
		t.Errorf("sink1 should contain only typo_a warning, got %v", sink1)
	}
	if len(sink2) != 1 || !strings.Contains(sink2[0], "typo_b") {
		t.Errorf("sink2 should contain only typo_b warning, got %v", sink2)
	}
}

// TestRunWith_WarnNilFallsBackToStderr verifies that a nil Warn does
// not panic — the buildGlobalArray path falls back to os.Stderr.
// We cannot easily capture stderr here, so this is a smoke test that
// the call completes normally with a typo'd binding and Warn=nil.
func TestRunWith_WarnNilFallsBackToStderr(t *testing.T) {
	eng := NewEngine()
	eng.DeclareBinding("declared", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `main := 1`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), &RunOptions{
		Bindings: map[string]eval.Value{
			"declared": &eval.HostVal{Inner: int64(1)},
			"typo":     &eval.HostVal{Inner: int64(2)},
		},
		// Warn intentionally omitted — must not panic.
	})
	if err != nil {
		t.Fatal(err)
	}
}
