//go:build probe

// Capability environment and effect interaction tests.
package probe_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe C: Capability environment edge cases
// ===================================================================

func TestProbeC_CapEnv_EmptyCapsPureProgram(t *testing.T) {
	// No caps, no effects — should work fine.
	result, err := probeSandbox("main := 42", nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_CapEnv_StateWithoutCap(t *testing.T) {
	// Using state without providing the "state" capability should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do { get }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when using state without providing state cap")
	}
}

func TestProbeC_CapEnv_FailWithoutCap(t *testing.T) {
	// Using fail without providing the "fail" capability should fail.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectFail)
	rt, err := eng.NewRuntime(context.Background(), `
import Effect.Fail
main := do { fail }
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when using fail without providing fail cap")
	}
}

func TestProbeC_CapEnv_StatePutGet(t *testing.T) {
	// Basic put/get to verify cap env threading.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do { put 99; get }
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 99)
}

func TestProbeC_CapEnv_CapEnvIsolation(t *testing.T) {
	// Multiple executions of the same runtime should not share cap state.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.Use(gicel.EffectState)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State
main := do { n <- get; put (n + 1); get }
`)
	if err != nil {
		t.Fatal(err)
	}

	caps1 := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result1, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps1})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result1.Value, 1)

	// Second run with fresh caps — should start from 0 again.
	caps2 := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result2, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps2})
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result2.Value, 1)
}

// ===================================================================
// Probe D: Effect interaction — state + fail
// ===================================================================

func TestProbeD_Effect_StateBeforeFail(t *testing.T) {
	// put then fail: error should propagate.
	caps := map[string]any{
		"state": &gicel.HostVal{Inner: int64(0)},
		"fail":  gicel.NewRecordFromMap(map[string]gicel.Value{}),
	}
	_, err := pdRunWithCaps(t, `
import Prelude
import Effect.Fail
import Effect.State
main := do { put 42; fail }
`, caps, gicel.Prelude, gicel.EffectFail, gicel.EffectState)
	if err == nil {
		t.Fatal("expected error from fail after put")
	}
}

func TestProbeD_Effect_GetWithoutPut(t *testing.T) {
	// get without initial state capability should error.
	_, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := get
`, nil, gicel.Prelude, gicel.EffectState)
	if err == nil {
		t.Fatal("expected error from get without state capability")
	}
	if !strings.Contains(err.Error(), "no state capability") {
		t.Fatalf("expected 'no state capability' error, got: %v", err)
	}
}

func TestProbeD_Effect_PutThenGet(t *testing.T) {
	caps := map[string]any{
		"state": &gicel.HostVal{Inner: int64(0)},
	}
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := do { put 100; get }
`, caps, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 100)
}

func TestProbeD_Effect_NestedModify(t *testing.T) {
	caps := map[string]any{
		"state": &gicel.HostVal{Inner: int64(0)},
	}
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := do {
  put 1;
  modify (+ 10);
  modify (* 3);
  get
}
`, caps, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 33) // (1 + 10) * 3 = 33
}

// ===================================================================
// Probe E: State effect handlers — runState/evalState/execState
// ===================================================================

func TestProbeE_RunState_Basic(t *testing.T) {
	// runState introduces state, runs computation, returns (finalState, result).
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := runState 0 (thunk do {
  modify (+ 10);
  get
})
`, nil, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// (10, 10) — final state and result are both 10
	rec, ok := v.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal, got %T", v)
	}
	s, _ := rec.Get("_1")
	a, _ := rec.Get("_2")
	pdAssertInt(t, s, 10)
	pdAssertInt(t, a, 10)
}

func TestProbeE_EvalState_Basic(t *testing.T) {
	// evalState returns only the result, discarding final state.
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := evalState 0 (thunk do {
  put 42;
  get
})
`, nil, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 42)
}

func TestProbeE_ExecState_Basic(t *testing.T) {
	// execState returns only the final state, discarding result.
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := execState 0 (thunk do {
  modify (+ 1);
  modify (+ 2);
  pure "ignored"
})
`, nil, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 3)
}

func TestProbeE_RunState_NoCapsNeeded(t *testing.T) {
	// runState does NOT require initial caps — it introduces the capability.
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := evalState 100 (thunk do {
  x <- get;
  put (x * 2);
  get
})
`, nil, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 200)
}

func TestProbeE_RunState_CapEnvIsolation(t *testing.T) {
	// State capability is eliminated after runState — outer CapEnv unchanged.
	v, err := pdRunWithCaps(t, `
import Prelude
import Effect.State
main := do {
  a <- evalState 10 (thunk do { modify (+ 5); get });
  b <- evalState 20 (thunk do { modify (+ 5); get });
  pure (a + b)
}
`, nil, gicel.Prelude, gicel.EffectState)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pdAssertInt(t, v, 40) // 15 + 25
}

func TestProbeD_Effect_FromMaybeNothing(t *testing.T) {
	caps := map[string]any{
		"fail": gicel.NewRecordFromMap(map[string]gicel.Value{}),
	}
	_, err := pdRunWithCaps(t, `
import Prelude
import Effect.Fail
main := fromMaybe Nothing
`, caps, gicel.Prelude, gicel.EffectFail)
	if err == nil {
		t.Fatal("expected error from fromMaybe Nothing")
	}
}
