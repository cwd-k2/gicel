// Engine VM bench tests — pre-compiled execution benchmarks (RunWith only).
// Does NOT cover: correctness tests (see adjacent test files).
//
// These compile once outside the bench loop and time only RunWith.
// This isolates steady-state runtime cost from per-iteration compile
// overhead, which the BenchmarkEngineEndToEnd* / BenchmarkEndToEnd*
// variants conflate. When investigating runtime performance, prefer
// these; when investigating cold-start (CLI invocation, NewRuntime)
// cost, use the EndToEnd variants.
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
      else do { write i (i * i) arr; loop (i + 1) }
  ) 0;
  read 99 arr
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

// BenchmarkExecMapInsert50 measures the steady-state runtime cost of 50
// sequential Data.Map.insert calls. Compare against BenchmarkEndToEndMapInsert50
// (cold) to see the per-call compile share.
func BenchmarkExecMapInsert50(b *testing.B) {
	source := mapInsertSource(50)
	eng := NewEngine()
	stdlib.Prelude(eng)
	stdlib.Map(eng)
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

// BenchmarkExecMutableMapInsert50 measures steady-state Effect.Map insert.
func BenchmarkExecMutableMapInsert50(b *testing.B) {
	source := mutableMapInsertSource(50)
	eng := NewEngine()
	stdlib.Prelude(eng)
	stdlib.EffectMap(eng)
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

// BenchmarkExecSetAlgebra measures steady-state Data.Set union/intersection.
// Mirrors BenchmarkEndToEndSetAlgebra but pre-compiles the source.
func BenchmarkExecSetAlgebra(b *testing.B) {
	source := `import Prelude
import Data.Set
main := {
  s1 := (fromList [1,2,3,4,5,6,7,8,9,10] :: Set Int);
  s2 := (fromList [5,6,7,8,9,10,11,12,13,14] :: Set Int);
  u := union s1 s2;
  i := intersection s1 s2;
  d := difference s1 s2;
  (size u, size i, size d)
}
`
	eng := NewEngine()
	stdlib.Prelude(eng)
	stdlib.Set(eng)
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

// BenchmarkExecArithmeticLoop measures a tight arithmetic loop without
// effects. Stresses OpPrim direct dispatch and OpFix tail-call.
func BenchmarkExecArithmeticLoop(b *testing.B) {
	eng := NewEngine()
	stdlib.Prelude(eng)
	eng.EnableRecursion()
	eng.SetStepLimit(10_000_000)
	source := `import Prelude
sumTo := fix (\loop n acc.
  if n <= 0
    then acc
    else loop (n - 1) (acc + n))
main := sumTo 1000 0
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

// BenchmarkExecListFoldrSum exercises applyNForPrim with a 2-arg VMClosure
// (the user lambda for the fold step). This is the host-callback dispatch
// path most embedders hit through HoFs (foldr / foldl / map / filter), and
// is distinct from the Map/Set compareKeys path covered by ExecMapInsert50:
// compareKeys lands on the PrimVal scratch fast path, while a user closure
// lands on the VMClosure callClosureMulti path. Both share callPrim's
// defer/recover overhead, so widening coverage matters for B2.
func BenchmarkExecListFoldrSum(b *testing.B) {
	eng := NewEngine()
	stdlib.Prelude(eng)
	source := `import Prelude
xs := replicate 100 (1 :: Int)
main := foldr (\x acc. x + acc) 0 xs
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

// BenchmarkExecListMap exercises applyForPrim with a 1-arg VMClosure (the
// per-element map function). Complements ExecListFoldrSum by isolating the
// single-arg dispatch path.
func BenchmarkExecListMap(b *testing.B) {
	eng := NewEngine()
	stdlib.Prelude(eng)
	source := `import Prelude
xs := replicate 100 (1 :: Int)
main := foldr (\x acc. x + acc) 0 (map (\n. n + 1) xs)
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

// BenchmarkExecListMapFilter exercises R19 (map×filter fusion):
// map f (filter p xs) → foldr with fused kernel, eliminating both
// the intermediate filtered list and the intermediate mapped list.
func BenchmarkExecListMapFilter(b *testing.B) {
	eng := NewEngine()
	stdlib.Prelude(eng)
	source := `import Prelude
xs := replicate 100 (1 :: Int)
main := map (\n. n + 10) (filter (\n. n > 0) xs)
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

// BenchmarkExecListFoldrFilter exercises R17 (foldr×filter fusion):
// foldr k z (filter p xs) → single-pass foldr with predicate guard,
// eliminating the intermediate filtered list.
func BenchmarkExecListFoldrFilter(b *testing.B) {
	eng := NewEngine()
	stdlib.Prelude(eng)
	source := `import Prelude
xs := replicate 100 (1 :: Int)
main := foldr (\x acc. x + acc) 0 (filter (\n. n > 0) xs)
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

// BenchmarkExecListChain exercises multi-rule fusion composition:
// foldr k z (map f (filter p xs)) should fire R19 (map×filter → foldr)
// then the outer foldr composes with the inner. Tests that the
// optimizer's fixed-point iteration composes multiple fusion rules.
func BenchmarkExecListChain(b *testing.B) {
	eng := NewEngine()
	stdlib.Prelude(eng)
	source := `import Prelude
xs := replicate 100 (1 :: Int)
main := foldr (\x acc. x + acc) 0 (map (\n. n * 2) (filter (\n. n > 0) xs))
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
