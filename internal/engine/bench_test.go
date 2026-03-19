// Engine benchmarks — compile, runtime assembly, end-to-end.
// Does NOT cover: evaluator microbenchmarks (eval/), checker benchmarks (check/).

package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/stdlib"
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
	for i := 0; i < n; i++ {
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
	source := `data MyBool := MyTrue | MyFalse
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
	modTemplate := `data M%dBool := M%dTrue | M%dFalse
_m%dNot :: M%dBool -> M%dBool
_m%dNot := \x. case x { M%dTrue -> M%dFalse; M%dFalse -> M%dTrue }
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
