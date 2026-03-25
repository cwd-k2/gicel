// Evaluator benchmarks — env lookup, curried application, closure chains.
// Does NOT cover: env extend (eval_test.go), full pipeline (engine/engine_bench_test.go).

package eval

import (
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/ir"
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

// ---------------------------------------------------------------------------
// Env.TrimTo (hot path in pprof: 16% of eval alloc)
// ---------------------------------------------------------------------------

// BenchmarkEnvTrimTo measures the cost of TrimTo, which creates a new env
// keeping only named bindings. This is called on every closure application.
func BenchmarkEnvCapture(b *testing.B) {
	env := EmptyEnv()
	for i := 0; i < 100; i++ {
		env = env.Push(&HostVal{Inner: i})
	}
	indices := []int{10, 30, 50, 70, 90}
	b.ResetTimer()
	for b.Loop() {
		env.Capture(indices)
	}
}

// ---------------------------------------------------------------------------
// Match allocation (hot path in pprof: 13% of eval alloc)
// ---------------------------------------------------------------------------

// BenchmarkMatchNestedCon measures pattern matching on a nested constructor.
func BenchmarkMatchNestedCon(b *testing.B) {
	// Simulate: Cons x (Cons y (Cons z Nil)) → bindings {x, y, z}
	inner3 := &ConVal{Con: "Nil"}
	inner2 := &ConVal{Con: "Cons", Args: []Value{&HostVal{Inner: 3}, inner3}}
	inner1 := &ConVal{Con: "Cons", Args: []Value{&HostVal{Inner: 2}, inner2}}
	val := &ConVal{Con: "Cons", Args: []Value{&HostVal{Inner: 1}, inner1}}

	// Pattern: Cons x (Cons y (Cons z Nil))
	patNil := &ir.PCon{Con: "Nil"}
	patZ := &ir.PCon{Con: "Cons", Args: []ir.Pattern{&ir.PVar{Name: "z"}, patNil}}
	patY := &ir.PCon{Con: "Cons", Args: []ir.Pattern{&ir.PVar{Name: "y"}, patZ}}
	pat := &ir.PCon{Con: "Cons", Args: []ir.Pattern{&ir.PVar{Name: "x"}, patY}}

	b.ResetTimer()
	for b.Loop() {
		Match(val, pat)
	}
}
