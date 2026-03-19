// Evaluator benchmarks — env lookup, curried application, closure chains.
// Does NOT cover: env extend (eval_test.go), full pipeline (engine/engine_bench_test.go).

package eval

import (
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// Env: deep lookup (Finding 1 regression check)
// ---------------------------------------------------------------------------

// BenchmarkEnvDeepLookup measures lookup cost in a deep parent chain.
// After Finding 1 fix, lookups on chains > flatThreshold should amortize to O(1).
func BenchmarkEnvDeepLookup(b *testing.B) {
	env := EmptyEnv()
	for i := 0; i < 200; i++ {
		env = env.Extend(fmt.Sprintf("v%d", i), &HostVal{Inner: i})
	}
	// First lookup triggers flattening.
	env.Lookup("v100")
	b.ResetTimer()
	for b.Loop() {
		env.Lookup("v100")
	}
}

// BenchmarkEnvShallowLookup measures lookup cost in a short chain (no flattening).
func BenchmarkEnvShallowLookup(b *testing.B) {
	env := EmptyEnv()
	for i := 0; i < 10; i++ {
		env = env.Extend(fmt.Sprintf("v%d", i), &HostVal{Inner: i})
	}
	b.ResetTimer()
	for b.Loop() {
		env.Lookup("v5")
	}
}

// ---------------------------------------------------------------------------
// ConVal curried application (Finding 6)
// ---------------------------------------------------------------------------

// BenchmarkConValCurriedApply measures the allocation cost of building a
// constructor value via repeated curried application.
func BenchmarkConValCurriedApply(b *testing.B) {
	for b.Loop() {
		var v Value = &ConVal{Con: "MkTuple", Args: nil}
		for i := 0; i < 10; i++ {
			args := make([]Value, len(v.(*ConVal).Args)+1)
			copy(args, v.(*ConVal).Args)
			args[len(args)-1] = &HostVal{Inner: i}
			v = &ConVal{Con: "MkTuple", Args: args}
		}
	}
}

// ---------------------------------------------------------------------------
// Closure chain (eval throughput proxy)
// ---------------------------------------------------------------------------

// BenchmarkClosureEnvBuild measures the cost of building a closure's
// captured environment chain via repeated Extend.
func BenchmarkClosureEnvBuild(b *testing.B) {
	for b.Loop() {
		env := EmptyEnv()
		for i := 0; i < 100; i++ {
			env = env.Extend(fmt.Sprintf("captured%d", i), &HostVal{Inner: i})
		}
		// Simulate closure capture + lookup.
		env.Lookup("captured50")
	}
}
