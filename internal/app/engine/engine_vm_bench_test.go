// Execution benchmarks for the bytecode VM.
package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

// BenchmarkExecSmall measures pure execution time for a minimal program.
func BenchmarkExecSmall(b *testing.B) {
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

// BenchmarkExecArray measures execution time for an array loop (fix + Effect.Array).
func BenchmarkExecArray(b *testing.B) {
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
