// Engine benchmarks — compile, runtime assembly, end-to-end.
// Does NOT cover: evaluator microbenchmarks (eval/), checker benchmarks (check/).

package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

// ---------------------------------------------------------------------------
// Compile benchmarks (parse + check + optimize)
// ---------------------------------------------------------------------------

const smallSource = `import Prelude
id :: \ a. a -> a
id := \x. x
main := id True
`

func largeSource(n int) string {
	var b strings.Builder
	b.WriteString("import Prelude\n")
	for i := range n {
		fmt.Fprintf(&b, "f%d :: \\ a. a -> a\nf%d := \\x. x\n", i, i)
	}
	b.WriteString("main := f0 True\n")
	return b.String()
}

func BenchmarkEngineCompileSmall(b *testing.B) {
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		_, err := eng.NewRuntime(context.Background(), smallSource)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineCompileLarge(b *testing.B) {
	source := largeSource(100)
	b.ResetTimer()
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		_, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// NewRuntime benchmarks (compile only, varying module count)
// ---------------------------------------------------------------------------

func BenchmarkEngineNewRuntimeNoModules(b *testing.B) {
	source := `form MyBool := { MyTrue: MyBool; MyFalse: MyBool; }
main := MyTrue
`
	for b.Loop() {
		eng := NewEngine()
		_, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineNewRuntimePrelude(b *testing.B) {
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		_, err := eng.NewRuntime(context.Background(), smallSource)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineNewRuntimeWithModules(b *testing.B) {
	modTemplate := `form M%dBool := M%dTrue | M%dFalse
_m%dNot :: M%dBool -> M%dBool
_m%dNot := \x. case x { M%dTrue => M%dFalse; M%dFalse => M%dTrue }
`
	mainSource := "import Prelude\nimport M1\nimport M2\nimport M3\nmain := True\n"
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		for i := 1; i <= 3; i++ {
			src := fmt.Sprintf(modTemplate, i, i, i, i, i, i, i, i, i, i, i)
			eng.RegisterModule(fmt.Sprintf("M%d", i), src)
		}
		_, err := eng.NewRuntime(context.Background(), mainSource)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// End-to-end (compile + eval)
// ---------------------------------------------------------------------------

func BenchmarkEngineEndToEndSmall(b *testing.B) {
	for b.Loop() {
		eng := NewEngine()
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

func BenchmarkEngineEndToEndSmallCold(b *testing.B) {
	for b.Loop() {
		ResetModuleCache()
		eng := NewEngine()
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

// ---------------------------------------------------------------------------
// Scale: large declaration count (500+ decls)
// ---------------------------------------------------------------------------

func BenchmarkEngineCompileLarge500(b *testing.B) {
	source := largeSource(500)
	b.ResetTimer()
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		_, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// All packs loaded (compile cost of full stdlib)
// ---------------------------------------------------------------------------

func BenchmarkEngineNewRuntimeAllPacks(b *testing.B) {
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		stdlib.State(eng)
		stdlib.Fail(eng)
		stdlib.IO(eng)
		stdlib.Slice(eng)
		stdlib.Array(eng)
		stdlib.Map(eng)
		stdlib.Set(eng)
		stdlib.EffectMap(eng)
		stdlib.EffectSet(eng)
		_, err := eng.NewRuntime(context.Background(),
			"import Prelude\nimport Effect.State\nimport Effect.Fail\nimport Effect.IO\nimport Data.Slice\nimport Effect.Array as Arr\nimport Data.Map as Map\nimport Data.Set as Set\nimport Effect.Map as MMap\nimport Effect.Set as MSet\nmain := True\n")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// Do-block compile cost (state-threading bind chains)
// ---------------------------------------------------------------------------

func doBlockSource(binds int) string {
	var b strings.Builder
	b.WriteString("import Prelude\nimport Effect.State\n")
	b.WriteString("compute := thunk do {\n  put 0;\n")
	for i := range binds {
		fmt.Fprintf(&b, "  n%d <- get; put (n%d + 1);\n", i, i)
	}
	b.WriteString("  get\n}\nmain := compute\n")
	return b.String()
}

func BenchmarkEngineCompileDoBlock10(b *testing.B) {
	source := doBlockSource(10)
	b.ResetTimer()
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		stdlib.State(eng)
		_, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEngineCompileDoBlock30(b *testing.B) {
	source := doBlockSource(30)
	b.ResetTimer()
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		stdlib.State(eng)
		_, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// Effect.Array end-to-end (mutable array operations)
// ---------------------------------------------------------------------------

func BenchmarkEngineEndToEndArray(b *testing.B) {
	source := `import Prelude
import Effect.Array
compute := thunk do {
  arr <- new 100 0;
  write 0 42 arr;
  write 50 99 arr;
  v <- read 50 arr;
  pure v
}
main := compute
`
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		stdlib.Array(eng)
		rt, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
		_, err = rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// Data.Map end-to-end (immutable map operations)
// ---------------------------------------------------------------------------

func BenchmarkEngineEndToEndMap(b *testing.B) {
	source := `import Prelude
import Data.Map as Map
m := Map.insert 3 "c" $ Map.insert 2 "b" $ Map.insert 1 "a" $ (Map.empty :: Map Int String)
main := (Map.size m, Map.lookup 2 m)
`
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		stdlib.Map(eng)
		rt, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
		_, err = rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// Stdlib collection benchmarks — map insert scale, set algebra, mutable map
// ---------------------------------------------------------------------------

func mapInsertSource(n int) string {
	var b strings.Builder
	b.WriteString("import Prelude\nimport Data.Map as Map\n")
	b.WriteString("main := {\n  m := (Map.empty :: Map Int Int);\n")
	for i := range n {
		fmt.Fprintf(&b, "  m := Map.insert %d %d m;\n", i, i*10)
	}
	fmt.Fprintf(&b, "  Map.lookup %d m\n}\n", n/2)
	return b.String()
}

func BenchmarkEndToEndMapInsert50(b *testing.B) {
	source := mapInsertSource(50)
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		stdlib.Map(eng)
		rt, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
		_, err = rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func mutableMapInsertSource(n int) string {
	var b strings.Builder
	b.WriteString("import Prelude\nimport Effect.Map\n")
	b.WriteString("main := thunk do {\n  m <- new;\n")
	for i := range n {
		fmt.Fprintf(&b, "  insert %d %d m;\n", i, i*10)
	}
	fmt.Fprintf(&b, "  lookup %d m\n}\n", n/2)
	return b.String()
}

func BenchmarkEndToEndMutableMapInsert50(b *testing.B) {
	source := mutableMapInsertSource(50)
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		stdlib.EffectMap(eng)
		rt, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
		_, err = rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEndToEndSetAlgebra(b *testing.B) {
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
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		stdlib.Set(eng)
		rt, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			b.Fatal(err)
		}
		_, err = rt.RunWith(context.Background(), nil)
		if err != nil {
			b.Fatal(err)
		}
	}
}
