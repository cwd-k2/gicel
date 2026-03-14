package gicel_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cwd-k2/gicel"
	"github.com/cwd-k2/gicel/internal/check"
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
)

// addSeedCorpus adds all .gicel files from testdata/stress/ as seed inputs.
func addSeedCorpus(f *testing.F) {
	seeds, _ := filepath.Glob("testdata/stress/*.gicel")
	for _, path := range seeds {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f.Add(data)
	}
}

// --- Stage 1: Lexer ---
// Detects panics and infinite loops in tokenization.
func FuzzLexer(f *testing.F) {
	addSeedCorpus(f)
	f.Add([]byte("f :: Int -> Int; f x := x"))
	f.Add([]byte("(.) :: (b -> c) -> (a -> b) -> a -> c"))
	f.Add([]byte("data T = { Con :: forall a. a -> T }"))
	f.Add([]byte("{ x = 1, y = 2 }!#x"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, src []byte) {
		source := span.NewSource("fuzz", string(src))
		l := parse.NewLexer(source)
		l.Tokenize() // panics are the signal
	})
}

// --- Stage 2: Parser ---
// Detects panics in parsing; lex errors are expected and skipped.
func FuzzParser(f *testing.F) {
	addSeedCorpus(f)
	f.Add([]byte("f :: Int -> Int; f x := x"))
	f.Add([]byte("main := \\x -> x"))
	f.Add([]byte("class Eq a { eq :: a -> a -> Bool }"))
	f.Add([]byte("instance Eq Int { eq := \\a -> \\b -> True }"))
	f.Add([]byte("import Foo"))

	f.Fuzz(func(t *testing.T, src []byte) {
		source := span.NewSource("fuzz", string(src))
		l := parse.NewLexer(source)
		tokens, lexErrs := l.Tokenize()
		if lexErrs.HasErrors() {
			return // expected: invalid tokens
		}
		es := &errs.Errors{Source: source}
		p := parse.NewParser(tokens, es)
		p.ParseProgram() // panics are the signal
	})
}

// --- Stage 3: Type Checker ---
// Detects panics in type checking via public API (includes Prelude).
func FuzzCheck(f *testing.F) {
	addSeedCorpus(f)
	f.Add([]byte("id :: forall a. a -> a; id := \\x -> x; main := id True"))
	f.Add([]byte("data Maybe a = Nothing | Just a; main := Just True"))
	f.Add([]byte("f :: Int -> Int; f := \\x -> x; main := f 42"))

	f.Fuzz(func(t *testing.T, src []byte) {
		eng := gicel.NewEngine()
		eng.Check(string(src)) // panics are the signal; compile/type errors are expected
	})
}

// --- Stage 3b: Type Checker (bare, no Prelude) ---
// Tests checker robustness against arbitrary input without Prelude environment.
// Catches nil-guard omissions (e.g. missing IxMonad class).
func FuzzCheckBare(f *testing.F) {
	f.Add([]byte("id := \\x -> x; main := id True"))
	f.Add([]byte("data T = A | B; main := A"))
	f.Add([]byte("do { x <- pure True; pure x }"))
	f.Add([]byte("class Foo a { bar :: a -> a }"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, src []byte) {
		source := span.NewSource("fuzz", string(src))
		l := parse.NewLexer(source)
		tokens, lexErrs := l.Tokenize()
		if lexErrs.HasErrors() {
			return
		}
		es := &errs.Errors{Source: source}
		p := parse.NewParser(tokens, es)
		ast := p.ParseProgram()
		if es.HasErrors() {
			return
		}
		check.Check(ast, source, nil) // panics are the signal
	})
}

// --- Stage 4: Full Pipeline ---
// Detects panics in evaluation; compile errors are expected and skipped.
func FuzzEval(f *testing.F) {
	addSeedCorpus(f)
	f.Add([]byte("main := True"))
	f.Add([]byte("id := \\x -> x; main := id True"))
	f.Add([]byte("data Pair a b = MkPair a b; main := MkPair True False"))

	f.Fuzz(func(t *testing.T, src []byte) {
		_, err := gicel.RunSandbox(string(src), &gicel.SandboxConfig{
			MaxSteps: 10_000,
			MaxDepth: 50,
			MaxAlloc: 1024 * 1024, // 1 MiB
			Timeout:  0,           // use default (5s)
		})
		_ = err // all errors are expected; only panics indicate bugs
	})
}

// --- Stage 3.5: FV Annotation ---
// Ensures AnnotateFreeVars does not panic and produces correct annotations.
func FuzzAnnotateFreeVars(f *testing.F) {
	addSeedCorpus(f)
	f.Add([]byte("f := \\x -> \\y -> x; main := f True False"))
	f.Add([]byte("compose := \\f -> \\g -> \\x -> f (g x); main := compose (\\x -> x) (\\y -> y) True"))

	f.Fuzz(func(t *testing.T, src []byte) {
		// Parse and type-check without Prelude (lightweight; Prelude uses same code paths).
		source := span.NewSource("fuzz", string(src))
		l := parse.NewLexer(source)
		tokens, lexErrs := l.Tokenize()
		if lexErrs.HasErrors() {
			return
		}
		es := &errs.Errors{Source: source}
		p := parse.NewParser(tokens, es)
		ast := p.ParseProgram()
		if es.HasErrors() {
			return
		}
		// Use recover to handle known checker panics on edge-case input.
		var prog *core.Program
		func() {
			defer func() { recover() }()
			var checkErrs *errs.Errors
			prog, checkErrs = check.Check(ast, source, nil)
			if checkErrs.HasErrors() {
				prog = nil
			}
		}()
		if prog == nil {
			return
		}
		core.AnnotateFreeVarsProgram(prog)
		for _, b := range prog.Bindings {
			verifyFVAnnotation(t, b.Expr)
		}
	})
}

// verifyFVAnnotation checks that Lam FV annotations match actual free vars.
func verifyFVAnnotation(t *testing.T, c core.Core) {
	t.Helper()
	core.Walk(c, func(n core.Core) bool {
		lam, ok := n.(*core.Lam)
		if !ok || lam.FV == nil {
			return true
		}
		actual := core.FreeVars(lam.Body)
		delete(actual, lam.Param)
		for _, name := range lam.FV {
			if _, ok := actual[name]; !ok {
				t.Errorf("FV annotation contains %q but it is not free in body", name)
			}
		}
		if len(lam.FV) != len(actual) {
			t.Errorf("FV annotation has %d entries but body has %d free vars", len(lam.FV), len(actual))
		}
		return true
	})
}

// --- Eval-level: Core IR limit enforcement ---
// Fuzzes the evaluator with constructed Core terms to test allocation limits.
func FuzzEvalLimits(f *testing.F) {
	f.Add(100, 10) // fields, depth

	f.Fuzz(func(t *testing.T, nFields, nDepth int) {
		if nFields < 0 || nFields > 500 || nDepth < 0 || nDepth > 100 {
			return
		}

		// Build a record with nFields fields.
		fields := make([]core.RecordField, nFields)
		for i := range fields {
			fields[i] = core.RecordField{Label: fmt.Sprintf("f%d", i), Value: &core.Lit{Value: int64(i)}}
		}
		term := core.Core(&core.RecordLit{Fields: fields})

		// Nest in nDepth lambdas applied to Unit.
		for range nDepth {
			term = &core.App{
				Fun: &core.Lam{Param: "_", Body: term},
				Arg: &core.Con{Name: "Unit"},
			}
		}

		limit := eval.NewLimit(100_000, 200)
		limit.SetAllocLimit(64 * 1024) // 64 KiB
		ev := eval.NewEvaluator(context.Background(), eval.NewPrimRegistry(), limit, nil)
		ev.Eval(eval.EmptyEnv(), eval.EmptyCapEnv(), term) // panics are the signal
	})
}
