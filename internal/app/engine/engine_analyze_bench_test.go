// Analyze benchmarks — LSP hot path (lex → parse → check + hover/completion/symbols/defs).
// Does NOT cover: compile + optimize (engine_bench_test.go), evaluator microbenchmarks (eval/).

package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setupAnalyzeEngine(packs ...registry.Pack) *Engine {
	eng := NewEngine()
	eng.EnableHoverIndex()
	for _, p := range packs {
		p(eng)
	}
	return eng
}

// ---------------------------------------------------------------------------
// Baseline: Analyze vs NewRuntime comparison
// ---------------------------------------------------------------------------

// BenchmarkAnalyzeSmall measures the LSP analyze path for a minimal program.
// Compare with BenchmarkEngineCompileSmall (NewRuntime) to isolate the
// optimize-vs-hover/completion cost difference.
func BenchmarkAnalyzeSmall(b *testing.B) {
	for b.Loop() {
		eng := setupAnalyzeEngine(stdlib.Prelude)
		eng.Analyze(context.Background(), smallSource)
	}
}

// BenchmarkAnalyzeLarge mirrors BenchmarkEngineCompileLarge: 100 identity decls.
func BenchmarkAnalyzeLarge(b *testing.B) {
	source := largeSource(100)
	b.ResetTimer()
	for b.Loop() {
		eng := setupAnalyzeEngine(stdlib.Prelude)
		eng.Analyze(context.Background(), source)
	}
}

// BenchmarkAnalyzeAllPacks mirrors BenchmarkEngineNewRuntimeAllPacks: full stdlib.
func BenchmarkAnalyzeAllPacks(b *testing.B) {
	source := "import Prelude\nimport Effect.State\nimport Effect.Fail\nimport Effect.IO\nimport Data.Slice\nimport Effect.Array as Arr\nimport Data.Map as Map\nimport Data.Set as Set\nimport Effect.Map as MMap\nimport Effect.Set as MSet\nmain := True\n"
	for b.Loop() {
		eng := setupAnalyzeEngine(
			stdlib.Prelude, stdlib.State, stdlib.Fail, stdlib.IO,
			stdlib.Slice, stdlib.Array, stdlib.Map, stdlib.Set,
			stdlib.EffectMap, stdlib.EffectSet,
		)
		eng.Analyze(context.Background(), source)
	}
}

// BenchmarkAnalyzeDoBlock30 mirrors BenchmarkEngineCompileDoBlock30.
func BenchmarkAnalyzeDoBlock30(b *testing.B) {
	source := doBlockSource(30)
	b.ResetTimer()
	for b.Loop() {
		eng := setupAnalyzeEngine(stdlib.Prelude, stdlib.State)
		eng.Analyze(context.Background(), source)
	}
}

// ---------------------------------------------------------------------------
// Hover index cost isolation
// ---------------------------------------------------------------------------

// BenchmarkAnalyzeSmallNoHover measures Analyze without HoverIndex construction.
// The delta against BenchmarkAnalyzeSmall is the hover recording overhead.
func BenchmarkAnalyzeSmallNoHover(b *testing.B) {
	for b.Loop() {
		eng := NewEngine()
		stdlib.Prelude(eng)
		eng.Analyze(context.Background(), smallSource)
	}
}

// ---------------------------------------------------------------------------
// Realistic sources: type-checker intensive patterns
// ---------------------------------------------------------------------------

const analyzePatternMatchSource = `import Prelude

form Expr := {
  Lit:  Int -> Expr;
  Add:  Expr -> Expr -> Expr;
  Mul:  Expr -> Expr -> Expr;
  Neg:  Expr -> Expr;
  Cond: Bool -> Expr -> Expr -> Expr
}

eval :: Expr -> Int
eval := \e. case e {
  Lit n       => n;
  Add a b     => eval a + eval b;
  Mul a b     => eval a * eval b;
  Neg a       => 0 - eval a;
  Cond c t f  =>
    if c
    then eval t
    else eval f
}

impl Show Expr := {
  show := \e. case e {
    Lit n   => show n;
    Add a b => "(" <> show a <> " + " <> show b <> ")";
    Mul a b => "(" <> show a <> " * " <> show b <> ")";
    Neg a   => "(-" <> show a <> ")";
    Cond c t f => "if " <> show c <> " then " <> show t <> " else " <> show f
  }
}

expr1 := Add (Lit 1) (Mul (Lit 2) (Lit 3))
expr2 := Cond True (Neg (Lit 5)) (Lit 10)
main  := (eval expr1, eval expr2, show expr1)
`

// BenchmarkAnalyzePatternMatch exercises form declarations, deep pattern matching,
// and type class instances — a realistic checker-heavy source.
func BenchmarkAnalyzePatternMatch(b *testing.B) {
	for b.Loop() {
		eng := setupAnalyzeEngine(stdlib.Prelude)
		eng.Analyze(context.Background(), analyzePatternMatchSource)
	}
}

func analyzeTypeClassSource(instances int) string {
	var b strings.Builder
	b.WriteString("import Prelude\n\n")
	b.WriteString("class MyClass a := {\n  myMethod :: a -> String\n}\n\n")
	for i := range instances {
		fmt.Fprintf(&b, "form T%d := MkT%d\n", i, i)
		fmt.Fprintf(&b, "impl Show T%d := { show := \\_.\"T%d\" }\n", i, i)
		fmt.Fprintf(&b, "impl MyClass T%d := { myMethod := show }\n\n", i)
	}
	b.WriteString("main := myMethod MkT0\n")
	return b.String()
}

// BenchmarkAnalyzeTypeClass10 measures the cost of 10 form+impl declarations,
// exercising instance resolution at scale.
func BenchmarkAnalyzeTypeClass10(b *testing.B) {
	source := analyzeTypeClassSource(10)
	b.ResetTimer()
	for b.Loop() {
		eng := setupAnalyzeEngine(stdlib.Prelude)
		eng.Analyze(context.Background(), source)
	}
}

// BenchmarkAnalyzeTypeClass30 pushes instance resolution to 30 declarations.
func BenchmarkAnalyzeTypeClass30(b *testing.B) {
	source := analyzeTypeClassSource(30)
	b.ResetTimer()
	for b.Loop() {
		eng := setupAnalyzeEngine(stdlib.Prelude)
		eng.Analyze(context.Background(), source)
	}
}

// ---------------------------------------------------------------------------
// Repeated analyze: LSP steady-state (single Engine, multiple Analyze calls)
// ---------------------------------------------------------------------------

// BenchmarkAnalyzeRepeated measures repeated Analyze calls on a shared Engine,
// which is the actual LSP usage pattern (one Engine per session, analyze on
// every debounced keystroke). Module registrations are amortized.
func BenchmarkAnalyzeRepeated(b *testing.B) {
	eng := setupAnalyzeEngine(stdlib.Prelude, stdlib.State)
	source := doBlockSource(10)
	// Prime once so we measure steady-state, not first-compile.
	eng.Analyze(context.Background(), source)
	b.ResetTimer()
	for b.Loop() {
		eng.Analyze(context.Background(), source)
	}
}

// BenchmarkAnalyzeRepeatedVarying rotates through distinct sources on a
// shared Engine, preventing any future per-source analysis cache from hitting.
func BenchmarkAnalyzeRepeatedVarying(b *testing.B) {
	eng := setupAnalyzeEngine(stdlib.Prelude)
	const variants = 16
	sources := make([]string, variants)
	for i := range variants {
		sources[i] = fmt.Sprintf("import Prelude\nf%d := %d\nmain := f%d + 1\n", i, i, i)
	}
	for _, src := range sources {
		eng.Analyze(context.Background(), src)
	}
	b.ResetTimer()
	i := 0
	for b.Loop() {
		eng.Analyze(context.Background(), sources[i%variants])
		i++
	}
}
