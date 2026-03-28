// VM backend benchmarks — compare with tree-walker benchmarks in engine_bench_test.go.
package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

func BenchmarkVMEndToEndSmall(b *testing.B) {
	for b.Loop() {
		eng := NewEngine()
		eng.SetBackend(BackendVM)
		stdlib.Prelude(eng)
		rt, err := eng.NewRuntime(context.Background(), smallSource)
		if err != nil {
			b.Fatal(err)
		}
		_, err = rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVMExecSmall measures pure execution time (no compile).
func BenchmarkVMExecSmall(b *testing.B) {
	eng := NewEngine()
	eng.SetBackend(BackendVM)
	stdlib.Prelude(eng)
	rt, err := eng.NewRuntime(context.Background(), smallSource)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, err = rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTreeExecSmall measures tree-walker pure execution time (no compile).
func BenchmarkTreeExecSmall(b *testing.B) {
	eng := NewEngine()
	stdlib.Prelude(eng)
	rt, err := eng.NewRuntime(context.Background(), smallSource)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, err = rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVMEndToEndArray(b *testing.B) {
	eng := NewEngine()
	eng.SetBackend(BackendVM)
	stdlib.Prelude(eng)
	stdlib.Array(eng)
	eng.EnableRecursion()
	eng.SetStepLimit(10_000_000)
	source := `import Prelude
import Effect.Array
main := do {
  arr <- new 100 0;
  fix (\loop i.
    if i >= 100
      then pure ()
      else do { writeAt i (i * i) arr; loop (i + 1) }
  ) 0;
  readAt 99 arr
}
`
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, err := rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTreeExecArray measures tree-walker pure execution time for array ops.
func BenchmarkTreeExecArray(b *testing.B) {
	eng := NewEngine()
	stdlib.Prelude(eng)
	stdlib.Array(eng)
	eng.EnableRecursion()
	eng.SetStepLimit(10_000_000)
	source := `import Prelude
import Effect.Array
main := do {
  arr <- new 100 0;
  fix (\loop i.
    if i >= 100
      then pure ()
      else do { writeAt i (i * i) arr; loop (i + 1) }
  ) 0;
  readAt 99 arr
}
`
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, err := rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVMExecArray measures VM pure execution time for array ops.
func BenchmarkVMExecArray(b *testing.B) {
	eng := NewEngine()
	eng.SetBackend(BackendVM)
	stdlib.Prelude(eng)
	stdlib.Array(eng)
	eng.EnableRecursion()
	eng.SetStepLimit(10_000_000)
	source := `import Prelude
import Effect.Array
main := do {
  arr <- new 100 0;
  fix (\loop i.
    if i >= 100
      then pure ()
      else do { writeAt i (i * i) arr; loop (i + 1) }
  ) 0;
  readAt 99 arr
}
`
	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, err := rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
