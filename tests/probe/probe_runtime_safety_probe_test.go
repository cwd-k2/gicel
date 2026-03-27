//go:build probe

// Runtime safety probe tests — defensive copy, failWith message preservation.
// Does NOT cover: runtime_test.go (entry points, errors, concurrency, regressions).

package probe_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

func TestProbeE_RunWithCapsNotMutated(t *testing.T) {
	eng := gicel.NewEngine()
	if err := gicel.Prelude(eng); err != nil {
		t.Fatal(err)
	}
	if err := gicel.EffectState(eng); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do { put 999; get }
`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	caps := map[string]any{"state": int64(0)}
	original := caps["state"]
	_, err = rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	// The original caps map must not be mutated by the computation.
	if caps["state"] != original {
		t.Errorf("caps mutated: expected %v, got %v", original, caps["state"])
	}
}

func TestProbeE_FailWithMessagePreserved(t *testing.T) {
	eng := gicel.NewEngine()
	if err := gicel.Prelude(eng); err != nil {
		t.Fatal(err)
	}
	if err := gicel.EffectFail(eng); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.Fail
main := do { failWith "custom error message"; pure 0 }
`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, runErr := rt.RunWith(context.Background(), nil)
	if runErr == nil {
		t.Fatal("expected runtime error from failWith")
	}
	var rtErr *gicel.RuntimeError
	if !errors.As(runErr, &rtErr) {
		t.Fatalf("expected RuntimeError, got %T: %v", runErr, runErr)
	}
	if !strings.Contains(rtErr.Message, "custom error message") {
		t.Errorf("expected error to contain 'custom error message', got: %s", rtErr.Message)
	}
}
