package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
	"github.com/cwd-k2/gicel/internal/runtime/vm"
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
	f.Add([]byte("form T := { Con :: \\ a. a -> T }"))
	f.Add([]byte("{ x: 1, y: 2 }.#x"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, src []byte) {
		source := span.NewSource("fuzz", string(src))
		s := parse.NewScanner(source)
		for {
			tok := s.Next()
			if tok.Kind == syntax.TokEOF {
				break
			}
		} // panics are the signal
	})
}

// --- Stage 2: Parser ---
// Detects panics in parsing; lex errors are expected and skipped.
func FuzzParser(f *testing.F) {
	addSeedCorpus(f)
	f.Add([]byte("f :: Int -> Int; f x := x"))
	f.Add([]byte("main := \\x. x"))
	f.Add([]byte("class Eq a { eq :: a -> a -> Bool }"))
	f.Add([]byte("instance Eq Int { eq := \\a b. True }"))
	f.Add([]byte("import Foo"))

	f.Fuzz(func(t *testing.T, src []byte) {
		source := span.NewSource("fuzz", string(src))
		es := &diagnostic.Errors{Source: source}
		p := parse.NewParser(context.Background(), source, es)
		p.ParseProgram() // panics are the signal
	})
}

// --- Stage 3: Type Checker ---
// Detects panics in type checking via public API (includes Prelude).
func FuzzCheck(f *testing.F) {
	addSeedCorpus(f)
	f.Add([]byte("id :: \\ a. a -> a; id := \\x. x; main := id True"))
	f.Add([]byte("form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }; main := Just True"))
	f.Add([]byte("f :: Int -> Int; f := \\x. x; main := f 42"))

	f.Fuzz(func(t *testing.T, src []byte) {
		eng := NewEngine()
		eng.Compile(context.Background(), string(src)) // panics are the signal; compile/type errors are expected
	})
}

// --- Stage 3b: Type Checker (bare, no Prelude) ---
// Tests checker robustness against arbitrary input without Prelude environment.
// Catches nil-guard omissions (e.g. missing GIMonad class).
func FuzzCheckBare(f *testing.F) {
	f.Add([]byte("id := \\x. x; main := id True"))
	f.Add([]byte("form T := { A: T; B: T; }; main := A"))
	f.Add([]byte("do { x <- pure True; pure x }"))
	f.Add([]byte("class Foo a { bar :: a -> a }"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, src []byte) {
		source := span.NewSource("fuzz", string(src))
		es := &diagnostic.Errors{Source: source}
		p := parse.NewParser(context.Background(), source, es)
		ast := p.ParseProgram()
		if p.LexErrors().HasErrors() || es.HasErrors() {
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
	f.Add([]byte("id := \\x. x; main := id True"))
	f.Add([]byte("form Pair := \a b. { MkPair: a -> b -> Pair a b; }; main := MkPair True False"))

	f.Fuzz(func(t *testing.T, src []byte) {
		_, err := RunSandbox(string(src), &SandboxConfig{
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
	f.Add([]byte("f := \\x y. x; main := f True False"))
	f.Add([]byte("compose := \\f g x. f (g x); main := compose (\\x. x) (\\y. y) True"))

	f.Fuzz(func(t *testing.T, src []byte) {
		// Parse and type-check without Prelude (lightweight; Prelude uses same code paths).
		source := span.NewSource("fuzz", string(src))
		es := &diagnostic.Errors{Source: source}
		p := parse.NewParser(context.Background(), source, es)
		ast := p.ParseProgram()
		if p.LexErrors().HasErrors() || es.HasErrors() {
			return
		}
		// Use recover to handle known checker panics on edge-case input.
		var prog *ir.Program
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Logf("checker panic on fuzz input (expected for edge cases): %v", r)
				}
			}()
			var checkErrs *diagnostic.Errors
			prog, checkErrs = check.Check(ast, source, nil)
			if checkErrs.HasErrors() {
				prog = nil
			}
		}()
		if prog == nil {
			return
		}
		annots := ir.AnnotateFreeVarsProgram(prog)
		for _, b := range prog.Bindings {
			verifyFVAnnotation(t, b.Expr, annots)
		}
	})
}

// verifyFVAnnotation checks that Lam FV entries in the side table match
// the recomputed free variables of the body.
func verifyFVAnnotation(t *testing.T, c ir.Core, annots *ir.FVAnnotations) {
	t.Helper()
	ir.Walk(c, func(n ir.Core) bool {
		lam, ok := n.(*ir.Lam)
		if !ok {
			return true
		}
		info, ok := annots.Lams[lam]
		if !ok || info.Overflow {
			return true
		}
		actual, overflow := ir.FreeVars(lam.Body)
		if overflow {
			return true // can't verify — skip
		}
		delete(actual, ir.LocalKey(lam.Param))
		for _, name := range info.Vars {
			if _, ok := actual[ir.LocalKey(name)]; !ok {
				t.Errorf("FV annotation contains %q but it is not free in body", name)
			}
		}
		if len(info.Vars) != len(actual) {
			t.Errorf("FV annotation has %d entries but body has %d free vars", len(info.Vars), len(actual))
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
		fields := make([]ir.Field, nFields)
		for i := range fields {
			fields[i] = ir.Field{Label: fmt.Sprintf("f%d", i), Value: &ir.Lit{Value: int64(i)}}
		}
		term := ir.Core(&ir.RecordLit{Fields: fields})

		// Nest in nDepth lambdas applied to Unit.
		for range nDepth {
			term = &ir.App{
				Fun: &ir.Lam{Param: "_", Body: term},
				Arg: &ir.Con{Name: "Unit"},
			}
		}

		b := budget.New(context.Background(), 100_000, 200)
		b.SetAllocLimit(64 * 1024) // 64 KiB
		annots := ir.AnnotateFreeVars(term)
		ir.AssignIndices(term, annots)
		compiler := vm.NewCompiler(map[ir.VarKey]int{}, nil)
		compiler.SetFVAnnots(annots)
		proto := compiler.CompileExpr(term)
		machine := vm.NewVM(vm.VMConfig{
			Globals:     make([]eval.Value, 0),
			GlobalSlots: map[ir.VarKey]int{},
			Prims:       eval.NewPrimRegistry(),
			Budget:      b,
			Ctx:         context.Background(),
		})
		machine.Run(proto, eval.EmptyCapEnv()) // panics are the signal
	})
}
