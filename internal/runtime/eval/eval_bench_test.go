// Evaluator benchmarks — env operations, curried application, closure capture, match.
// Does NOT cover: env extend (eval_test.go), full pipeline (engine/engine_bench_test.go).

package eval

import (
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// ---------------------------------------------------------------------------
// Env: global lookup (flat map, O(1))
// ---------------------------------------------------------------------------

// BenchmarkEnvDeepLookup measures global lookup cost with many bindings.
func BenchmarkEnvDeepLookup(b *testing.B) {
	env := EmptyEnv()
	for i := range 200 {
		env = env.Extend(fmt.Sprintf("v%d", i), &HostVal{Inner: i})
	}
	env.LookupGlobal("v100")
	b.ResetTimer()
	for b.Loop() {
		env.LookupGlobal("v100")
	}
}

// BenchmarkEnvShallowLookup measures global lookup cost with few bindings.
func BenchmarkEnvShallowLookup(b *testing.B) {
	env := EmptyEnv()
	for i := range 10 {
		env = env.Extend(fmt.Sprintf("v%d", i), &HostVal{Inner: i})
	}
	b.ResetTimer()
	for b.Loop() {
		env.LookupGlobal("v5")
	}
}

// ---------------------------------------------------------------------------
// ConVal curried application
// ---------------------------------------------------------------------------

// BenchmarkConValCurriedApply measures the allocation cost of building a
// constructor value via repeated curried application.
func BenchmarkConValCurriedApply(b *testing.B) {
	for b.Loop() {
		var v Value = &ConVal{Con: "MkTuple", Args: nil}
		for i := range 10 {
			args := make([]Value, len(v.(*ConVal).Args)+1)
			copy(args, v.(*ConVal).Args)
			args[len(args)-1] = &HostVal{Inner: i}
			v = &ConVal{Con: "MkTuple", Args: args}
		}
	}
}

// ---------------------------------------------------------------------------
// Closure globals build (eval throughput proxy)
// ---------------------------------------------------------------------------

// BenchmarkClosureEnvBuild measures the cost of building globals via Extend.
func BenchmarkClosureEnvBuild(b *testing.B) {
	for b.Loop() {
		env := EmptyEnv()
		for i := range 100 {
			env = env.Extend(fmt.Sprintf("captured%d", i), &HostVal{Inner: i})
		}
		env.LookupGlobal("captured50")
	}
}

// ---------------------------------------------------------------------------
// Env.Capture (closure creation hot path)
// ---------------------------------------------------------------------------

// BenchmarkEnvCapture measures the cost of Capture, which extracts specific
// locals by de Bruijn index. Called on every closure/thunk creation.
func BenchmarkEnvCapture(b *testing.B) {
	env := EmptyEnv()
	for i := range 100 {
		env = env.Push(&HostVal{Inner: i})
	}
	indices := []int{10, 30, 50, 70, 90}
	b.ResetTimer()
	for b.Loop() {
		env.Capture(indices, 1)
	}
}

// ---------------------------------------------------------------------------
// Match allocation
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
