// Runtime cache tests — cache hit/miss, fingerprint sensitivity, concurrent
// safety. Verifies that *Runtime is correctly content-addressed by source +
// runtime fingerprint.
// Does NOT cover: modcache_test.go (compiledModule-level cache).
package engine

import (
	"context"
	"sync"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func resetCaches() {
	ResetModuleCache()
	ResetRuntimeCache()
}

// TestRuntimeCache_HitOnSameSource verifies that two NewRuntime calls with
// identical source and identical Engine state return the *same* *Runtime
// pointer. Pointer identity is the contract — Layer A is a pure object cache.
func TestRuntimeCache_HitOnSameSource(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := 1 + 2
`
	eng := NewEngine()
	stdlib.Prelude(eng)

	rt1, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 != rt2 {
		t.Errorf("expected cache hit (same *Runtime pointer), got distinct values")
	}
}

// TestRuntimeCache_HitAcrossEngines verifies that two distinct Engines with
// identical configuration produce a cache hit. This is the realistic
// embedder pattern: multiple short-lived Engines compiling the same source.
func TestRuntimeCache_HitAcrossEngines(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := True
`
	eng1 := NewEngine()
	stdlib.Prelude(eng1)
	rt1, err := eng1.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	eng2 := NewEngine()
	stdlib.Prelude(eng2)
	rt2, err := eng2.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 != rt2 {
		t.Errorf("expected cache hit across identical Engines, got distinct *Runtime pointers")
	}
}

func TestRuntimeCache_MissOnDifferentSource(t *testing.T) {
	resetCaches()
	defer resetCaches()

	eng := NewEngine()
	stdlib.Prelude(eng)

	rt1, err := eng.NewRuntime(context.Background(), "import Prelude\nmain := 1\n")
	if err != nil {
		t.Fatal(err)
	}
	rt2, err := eng.NewRuntime(context.Background(), "import Prelude\nmain := 2\n")
	if err != nil {
		t.Fatal(err)
	}
	if rt1 == rt2 {
		t.Errorf("expected cache miss for different sources, got same *Runtime")
	}
}

func TestRuntimeCache_MissOnDifferentRegisteredType(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := True
`
	eng1 := NewEngine()
	stdlib.Prelude(eng1)
	rt1, err := eng1.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	eng2 := NewEngine()
	stdlib.Prelude(eng2)
	eng2.RegisterType("Extra", nil)
	rt2, err := eng2.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 == rt2 {
		t.Errorf("expected cache miss for different registered types, got same *Runtime")
	}
}

func TestRuntimeCache_MissOnDifferentLimits(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := 1
`
	eng1 := NewEngine()
	stdlib.Prelude(eng1)
	eng1.SetStepLimit(100)
	rt1, err := eng1.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	eng2 := NewEngine()
	stdlib.Prelude(eng2)
	eng2.SetStepLimit(200) // different
	rt2, err := eng2.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 == rt2 {
		t.Errorf("expected cache miss for different stepLimit, got same *Runtime")
	}
}

func TestRuntimeCache_MissOnDifferentEntryPoint(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := 1
go := 2
`
	eng1 := NewEngine()
	stdlib.Prelude(eng1)
	rt1, err := eng1.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	eng2 := NewEngine()
	stdlib.Prelude(eng2)
	eng2.SetEntryPoint("go")
	rt2, err := eng2.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 == rt2 {
		t.Errorf("expected cache miss for different entryPoint, got same *Runtime")
	}
}

func TestRuntimeCache_MissOnDifferentPrim(t *testing.T) {
	resetCaches()
	defer resetCaches()

	// Two distinct function literals (defined at different source
	// positions) compile to distinct code bodies, so reflect.Pointer()
	// returns distinct entry points → distinct fingerprints → cache
	// miss. (Closure-capture identity L1 is the documented exception
	// to this; this case avoids it because the literals are textually
	// distinct functions.)
	source := `import Prelude
main := 1
`
	primA := func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
		return nil, capEnv, nil
	}
	primB := func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
		return nil, capEnv, nil
	}

	makeEngine := func(prim eval.PrimImpl) *Engine {
		eng := NewEngine()
		stdlib.Prelude(eng)
		eng.RegisterPrim("custom_unused", prim)
		return eng
	}

	eng1 := makeEngine(primA)
	rt1, err := eng1.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	eng2 := makeEngine(primB)
	rt2, err := eng2.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 == rt2 {
		t.Errorf("expected cache miss for different prim impls, got same *Runtime")
	}
}

// TestRuntimeCache_HitProducesCorrectResult verifies that a cache hit
// returns a *Runtime that, when executed, produces the same result as a
// freshly compiled Runtime.
func TestRuntimeCache_HitProducesCorrectResult(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := 1 + 2 + 3
`
	eng1 := NewEngine()
	stdlib.Prelude(eng1)
	rt1, err := eng1.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	res1, err := rt1.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Second engine: should hit the cache.
	eng2 := NewEngine()
	stdlib.Prelude(eng2)
	rt2, err := eng2.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 != rt2 {
		t.Fatalf("expected cache hit, got distinct Runtimes (test setup bug)")
	}
	res2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	v1 := MustHost[int64](res1.Value)
	v2 := MustHost[int64](res2.Value)
	if v1 != 6 || v2 != 6 {
		t.Errorf("expected 6 from both, got %d and %d", v1, v2)
	}
}

// TestRuntimeCache_HostBindingsRespected verifies that even when two
// callers share a cached *Runtime, they can pass different host binding
// values via RunOptions.Bindings without cross-contamination.
func TestRuntimeCache_HostBindingsRespected(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := host_x
`
	makeEngine := func() *Engine {
		eng := NewEngine()
		stdlib.Prelude(eng)
		eng.DeclareBinding("host_x", ConType("Int"))
		return eng
	}

	eng1 := makeEngine()
	rt1, err := eng1.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	res1, err := rt1.RunWith(context.Background(), &RunOptions{
		Bindings: map[string]eval.Value{"host_x": &eval.HostVal{Inner: int64(42)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	eng2 := makeEngine()
	rt2, err := eng2.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if rt1 != rt2 {
		t.Fatalf("expected cache hit (test setup bug); got distinct *Runtime")
	}
	res2, err := rt2.RunWith(context.Background(), &RunOptions{
		Bindings: map[string]eval.Value{"host_x": &eval.HostVal{Inner: int64(99)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	v1 := MustHost[int64](res1.Value)
	v2 := MustHost[int64](res2.Value)
	if v1 != 42 {
		t.Errorf("expected first run to return 42, got %d", v1)
	}
	if v2 != 99 {
		t.Errorf("expected second run to return 99, got %d", v2)
	}
}

// TestRuntimeCache_Concurrent verifies that many goroutines hitting the
// same cache entry simultaneously is safe. The first to call RunWith
// populates cachedGlobals via sync.Once; the rest see the populated
// template. Documented goroutine-safe behavior of *Runtime.
func TestRuntimeCache_Concurrent(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := 1 + 2
`
	const N = 16

	var wg sync.WaitGroup
	results := make(chan int64, N)
	errs := make(chan error, N)

	for range N {
		wg.Go(func() {
			eng := NewEngine()
			stdlib.Prelude(eng)
			rt, err := eng.NewRuntime(context.Background(), source)
			if err != nil {
				errs <- err
				return
			}
			res, err := rt.RunWith(context.Background(), nil)
			if err != nil {
				errs <- err
				return
			}
			results <- MustHost[int64](res.Value)
		})
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Errorf("concurrent error: %v", err)
	}
	count := 0
	for v := range results {
		count++
		if v != 3 {
			t.Errorf("expected 3 from all goroutines, got %d", v)
		}
	}
	if count != N {
		t.Errorf("expected %d results, got %d", N, count)
	}
}

func TestRuntimeCache_ResetClearsEntries(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := 1
`
	eng := NewEngine()
	stdlib.Prelude(eng)
	if _, err := eng.NewRuntime(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	if RuntimeCacheLen() == 0 {
		t.Errorf("expected cache to have at least one entry, got 0")
	}
	ResetRuntimeCache()
	if RuntimeCacheLen() != 0 {
		t.Errorf("expected cache to be empty after reset, got %d", RuntimeCacheLen())
	}
}

// TestFingerprintInvalidation_MutationAfterFirstCompile verifies the
// invalidation hooks on every mutator. Pattern: compile once (populates
// fingerprint cache), mutate Engine state, compile again, assert the
// cache key changed (cache miss on same source). If any mutator forgets
// to invalidate, this test fails — a stale cached fingerprint would
// return the first *Runtime for the second compile despite different
// Engine state.
//
// Covers: DeclareBinding, DeclareAssumption, RegisterType, RegisterPrim,
// EnableRecursion, DenyAssumptions, EnableVerifyIR, RegisterModule,
// SetStepLimit, SetDepthLimit, SetNestingLimit, SetAllocLimit,
// SetMaxTFSteps, SetMaxSolverSteps, SetMaxResolveDepth, SetEntryPoint,
// DisableInlining.
func TestFingerprintInvalidation_MutationAfterFirstCompile(t *testing.T) {
	source := `import Prelude
main := 1
`

	cases := []struct {
		name   string
		mutate func(e *Engine)
	}{
		{"DeclareBinding", func(e *Engine) { e.DeclareBinding("x", ConType("Int")) }},
		{"DeclareAssumption", func(e *Engine) { e.DeclareAssumption("op", ConType("Int")) }},
		{"RegisterType", func(e *Engine) { e.RegisterType("Extra", nil) }},
		{"RegisterPrim", func(e *Engine) {
			e.RegisterPrim("p_extra", func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
				return nil, capEnv, nil
			})
		}},
		{"EnableRecursion", func(e *Engine) { e.EnableRecursion() }},
		{"DenyAssumptions", func(e *Engine) { e.DenyAssumptions() }},
		{"EnableVerifyIR", func(e *Engine) { e.EnableVerifyIR() }},
		{"SetStepLimit", func(e *Engine) { e.SetStepLimit(e.limits.stepLimit + 1) }},
		{"SetDepthLimit", func(e *Engine) { e.SetDepthLimit(e.limits.depthLimit + 1) }},
		{"SetNestingLimit", func(e *Engine) { e.SetNestingLimit(e.limits.nestingLimit + 1) }},
		{"SetAllocLimit", func(e *Engine) { e.SetAllocLimit(123_456) }},
		{"SetMaxTFSteps", func(e *Engine) { e.SetMaxTFSteps(e.limits.maxTFSteps + 1) }},
		{"SetMaxSolverSteps", func(e *Engine) { e.SetMaxSolverSteps(e.limits.maxSolverSteps + 1) }},
		{"SetMaxResolveDepth", func(e *Engine) { e.SetMaxResolveDepth(e.limits.maxResolveDepth + 1) }},
		{"SetEntryPoint", func(e *Engine) { e.SetEntryPoint("other") }},
		{"DisableInlining", func(e *Engine) { e.DisableInlining() }},
		{"RegisterModule", func(e *Engine) {
			_ = e.RegisterModule("Extra", "main := 7")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetCaches()
			defer resetCaches()

			eng := NewEngine()
			stdlib.Prelude(eng)
			pc := eng.pipeline(context.Background())
			k1 := pc.computeRuntimeCacheKey(source)

			tc.mutate(eng)
			pc2 := eng.pipeline(context.Background())
			k2 := pc2.computeRuntimeCacheKey(source)

			if k1 == k2 {
				t.Errorf("%s: fingerprint unchanged after mutation — stale cached fingerprint", tc.name)
			}
		})
	}
}

// TestFingerprintInvalidation_RegisterModuleRecRestoresState verifies
// that RegisterModuleRec's direct mutation of gatedBuiltins (set then
// restore) does not leave a stale cached fingerprint. The "rec/fix
// enabled" state is transient to the RegisterModule call; afterwards
// gatedBuiltins is restored and the fingerprint must reflect the
// restored state.
func TestFingerprintInvalidation_RegisterModuleRecRestoresState(t *testing.T) {
	resetCaches()
	defer resetCaches()

	eng := NewEngine()
	stdlib.Prelude(eng)

	// Baseline fingerprint before any rec module.
	source := `import Prelude
main := 1
`
	pc := eng.pipeline(context.Background())
	kBefore := pc.computeRuntimeCacheKey(source)

	// Register a rec module. Inside this call, gatedBuiltins gains
	// rec/fix and then loses them again. The store.recursion flag
	// does get set, so kAfter is expected to differ from kBefore on
	// the "rec:1" vs "rec:0" component — but the gatedBuiltins part
	// must match the restored state.
	if err := eng.RegisterModuleRec("RecMod", "fixpoint := 1"); err != nil {
		t.Fatal(err)
	}
	pc2 := eng.pipeline(context.Background())
	kAfter := pc2.computeRuntimeCacheKey(source)

	// They should differ (store.recursion flipped, new module registered).
	if kBefore == kAfter {
		t.Errorf("expected fingerprint to change after RegisterModuleRec")
	}

	// Second compile with identical state should hit the cache.
	pc3 := eng.pipeline(context.Background())
	kAfter2 := pc3.computeRuntimeCacheKey(source)
	if kAfter != kAfter2 {
		t.Errorf("expected fingerprint stability after RegisterModuleRec settled")
	}
}

// TestFingerprintInvalidation_Memoization verifies that repeated
// computeRuntimeCacheKey calls on an unchanged Engine produce the same
// cache key — the positive path of the memoization. This doesn't prove
// alloc savings (that's the benchmark's job) but it proves we are not
// producing a different digest across calls when state is unchanged.
func TestFingerprintInvalidation_Memoization(t *testing.T) {
	resetCaches()
	defer resetCaches()

	source := `import Prelude
main := True
`
	eng := NewEngine()
	stdlib.Prelude(eng)
	pc := eng.pipeline(context.Background())

	k1 := pc.computeRuntimeCacheKey(source)
	k2 := pc.computeRuntimeCacheKey(source)
	k3 := pc.computeRuntimeCacheKey(source)

	if k1 != k2 || k2 != k3 {
		t.Errorf("repeated computeRuntimeCacheKey on unchanged Engine produced different keys")
	}

	// Also verify the Engine-level cached fingerprint is stable.
	fp1 := eng.runtimeFingerprint()
	fp2 := eng.runtimeFingerprint()
	if fp1 != fp2 {
		t.Errorf("repeated runtimeFingerprint calls returned different digests")
	}
	if !eng.runtimeFpValid {
		t.Errorf("runtimeFpValid should be true after runtimeFingerprint call")
	}
}
