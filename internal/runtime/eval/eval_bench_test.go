// Evaluator benchmarks — globals lookup, curried application, closure capture, match.
// Does NOT cover: full pipeline (engine/engine_bench_test.go).

package eval

import (
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// ---------------------------------------------------------------------------
// Globals: map lookup (O(1))
// ---------------------------------------------------------------------------

// BenchmarkGlobalsDeepLookup measures global lookup cost with many bindings.
func BenchmarkGlobalsDeepLookup(b *testing.B) {
	globals := make(map[string]Value, 200)
	for i := range 200 {
		globals[fmt.Sprintf("v%d", i)] = &HostVal{Inner: i}
	}
	_ = globals["v100"]
	b.ResetTimer()
	for b.Loop() {
		_ = globals["v100"]
	}
}

// BenchmarkGlobalsShallowLookup measures global lookup cost with few bindings.
func BenchmarkGlobalsShallowLookup(b *testing.B) {
	globals := make(map[string]Value, 10)
	for i := range 10 {
		globals[fmt.Sprintf("v%d", i)] = &HostVal{Inner: i}
	}
	b.ResetTimer()
	for b.Loop() {
		_ = globals["v5"]
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
// Globals map build (eval throughput proxy)
// ---------------------------------------------------------------------------

// BenchmarkGlobalsMapBuild measures the cost of building globals via map insert.
func BenchmarkGlobalsMapBuild(b *testing.B) {
	for b.Loop() {
		globals := make(map[string]Value, 100)
		for i := range 100 {
			globals[fmt.Sprintf("captured%d", i)] = &HostVal{Inner: i}
		}
		_ = globals["captured50"]
	}
}

// BenchmarkGlobalsMapBuild100 measures the cost of building globals via map insert
// with the legacy b.N loop style.
func BenchmarkGlobalsMapBuild100(b *testing.B) {
	for i := 0; i < b.N; i++ {
		globals := make(map[string]Value, 100)
		for j := 0; j < 100; j++ {
			globals[fmt.Sprintf("v%d", j)] = &HostVal{Inner: j}
		}
		// Force a lookup to prevent dead-code elimination.
		_ = globals["v50"]
	}
}

// ---------------------------------------------------------------------------
// Capture (closure creation hot path)
// ---------------------------------------------------------------------------

// BenchmarkCapture measures the cost of Capture, which extracts specific
// locals by de Bruijn index. Called on every closure/thunk creation.
func BenchmarkCapture(b *testing.B) {
	var locals []Value
	for i := range 100 {
		locals = Push(locals, &HostVal{Inner: i})
	}
	indices := []int{10, 30, 50, 70, 90}
	b.ResetTimer()
	for b.Loop() {
		Capture(locals, indices, 1)
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
