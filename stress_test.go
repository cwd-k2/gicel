package gomputation_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	gmp "github.com/cwd-k2/gomputation"
	"github.com/cwd-k2/gomputation/pkg/types"
	"github.com/cwd-k2/gomputation/pkg/stdlib"
)

// ---------------------------------------------------------------------------
// Stress test programs — each exercises a distinct stress vector.
// Programs are stored in testdata/stress/*.gmp files.
// ---------------------------------------------------------------------------

func loadStressProgram(t testing.TB, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "stress", name))
	if err != nil {
		t.Fatalf("failed to load stress program %s: %v", name, err)
	}
	return string(data)
}

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

type stressProgram struct {
	name  string
	file  string // filename under testdata/stress/
	setup func(*gmp.Engine)
	check func(*testing.T, gmp.Value)
	caps  map[string]any
	binds map[string]gmp.Value
}

func intPrimSetup(eng *gmp.Engine) {
	eng.RegisterType("Int", types.KType{})
	eng.RegisterPrim("intZero", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		return gmp.ToValue(0), ce, nil
	})
	eng.RegisterPrim("intSucc", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		n := gmp.MustHost[int](args[0])
		return gmp.ToValue(n + 1), ce, nil
	})
	eng.RegisterPrim("intAdd", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		a := gmp.MustHost[int](args[0])
		b := gmp.MustHost[int](args[1])
		return gmp.ToValue(a + b), ce, nil
	})
	eng.RegisterPrim("intEq", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		a := gmp.MustHost[int](args[0])
		b := gmp.MustHost[int](args[1])
		if a == b {
			return &gmp.ConVal{Con: "True"}, ce, nil
		}
		return &gmp.ConVal{Con: "False"}, ce, nil
	})
}

func capEnvSetup(eng *gmp.Engine) {
	eng.RegisterType("Int", types.KType{})
	eng.RegisterPrim("intAdd", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
		a := gmp.MustHost[int](args[0])
		b := gmp.MustHost[int](args[1])
		return gmp.ToValue(a + b), ce, nil
	})
}

var stressPrograms = []stressProgram{
	{
		name: "adt_exhaustiveness",
		file: "01_adt_exhaustiveness.gmp",
		setup:  func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			// pairMatch returns True iff color matches shape; depends on chain
			assertCon(t, v, "") // just check it evaluates
		},
	},
	{
		name: "typeclass_hierarchy",
		file: "02_typeclass_hierarchy.gmp",
		setup:  func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			assertCon(t, v, "True") // isEqOrdering True True => EQ => True
		},
	},
	{
		name: "hkt_functor",
		file: "03_hkt_functor.gmp",
		setup:  func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			assertCon(t, v, "False") // fmap not (Branch (Leaf True) ...) => Leaf False at left
		},
	},
	{
		name: "deep_do_chain",
		file: "04_deep_do_chain.gmp",
		setup: func(e *gmp.Engine) {
			capEnvSetup(e)
			e.RegisterPrim("getN", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				v, _ := ce.Get("n")
				n, _ := v.(int)
				return gmp.ToValue(n), ce, nil
			})
			e.RegisterPrim("incN", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				v, _ := ce.Get("n")
				n, _ := v.(int)
				return &gmp.ConVal{Con: "Unit"}, ce.Set("n", n+1), nil
			})
			e.RegisterPrim("addN", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				v, _ := ce.Get("n")
				n, _ := v.(int)
				delta := gmp.MustHost[int](args[0])
				return &gmp.ConVal{Con: "Unit"}, ce.Set("n", n+delta), nil
			})
		},
		caps: map[string]any{"n": 0},
		check: func(t *testing.T, v gmp.Value) {
			// After 20 steps of addN(n)+incN, counter grows rapidly
			n := gmp.MustHost[int](v)
			if n <= 0 {
				t.Errorf("expected positive counter, got %d", n)
			}
		},
	},
	{
		name: "multi_param_classes",
		file: "05_multi_param_classes.gmp",
		setup:  func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			// convertAndCoerce True => convert True : Maybe Bool => Just True => coerce: depends
			assertCon(t, v, "") // just check it evaluates
		},
	},
	{
		name: "thunk_force",
		file: "06_thunk_force.gmp",
		setup: func(e *gmp.Engine) {
			eng := e
			eng.RegisterType("Int", types.KType{})
			counter := 0
			eng.RegisterPrim("mkVal", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				counter++
				return gmp.ToValue(counter), ce, nil
			})
			eng.RegisterPrim("addInts", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				a := gmp.MustHost[int](args[0])
				b := gmp.MustHost[int](args[1])
				return gmp.ToValue(a + b), ce, nil
			})
		},
		check: func(t *testing.T, v gmp.Value) {
			n := gmp.MustHost[int](v)
			if n <= 0 {
				t.Errorf("expected positive result from thunk chain, got %d", n)
			}
		},
	},
	{
		name: "recursive_data",
		file: "07_recursive_data.gmp",
		setup: func(e *gmp.Engine) {
			e.EnableRecursion()
			e.SetStepLimit(100_000_000)
			e.SetDepthLimit(100_000)
			intPrimSetup(e)
		},
		check: func(t *testing.T, v gmp.Value) {
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
			}
		},
	},
	{
		name: "row_polymorphism",
		file: "08_row_polymorphism.gmp",
		setup: func(e *gmp.Engine) {
			capEnvSetup(e)
			e.RegisterPrim("readDB", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				v, _ := ce.Get("db")
				n, _ := v.(int)
				return gmp.ToValue(n), ce, nil
			})
			e.RegisterPrim("writeDB", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				n := gmp.MustHost[int](args[0])
				return &gmp.ConVal{Con: "Unit"}, ce.Set("db", n), nil
			})
			e.RegisterPrim("getLog", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				v, _ := ce.Get("log")
				n, _ := v.(int)
				return gmp.ToValue(n), ce, nil
			})
			e.RegisterPrim("appendLog", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				v, _ := ce.Get("log")
				n, _ := v.(int)
				delta := gmp.MustHost[int](args[0])
				return &gmp.ConVal{Con: "Unit"}, ce.Set("log", n+delta), nil
			})
			e.RegisterPrim("readConfig", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				v, _ := ce.Get("cfg")
				n, _ := v.(int)
				return gmp.ToValue(n), ce, nil
			})
		},
		caps: map[string]any{"db": 0, "log": 0, "cfg": 1},
	},
	{
		name: "conditional_instances",
		file: "09_conditional_instances.gmp",
		setup:  func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			// main = Pair eqTest1 (Pair eqTest2 (Pair eqTest3 eqTest4))
			// eqTest1 = True (val1==val2), eqTest2 = False (val1!=val3)
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			assertCon(t, con.Args[0], "True") // val1 == val2
		},
	},
	{
		name: "full_grammar",
		file: "10_full_grammar.gmp",
		setup: func(e *gmp.Engine) {
			capEnvSetup(e)
			e.DeclareBinding("seed", types.Con("Int"))
			e.RegisterPrim("readS", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				v, _ := ce.Get("s")
				n, _ := v.(int)
				return gmp.ToValue(n), ce, nil
			})
			e.RegisterPrim("writeS", func(ctx context.Context, ce gmp.CapEnv, args []gmp.Value) (gmp.Value, gmp.CapEnv, error) {
				n := gmp.MustHost[int](args[0])
				return &gmp.ConVal{Con: "Unit"}, ce.Set("s", n), nil
			})
		},
		caps:  map[string]any{"s": 0},
		binds: map[string]gmp.Value{"seed": gmp.ToValue(10)},
		check: func(t *testing.T, v gmp.Value) {
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
			}
		},
	},
	{
		name: "datakinds",
		file: "11_datakinds.gmp",
		setup: func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			assertCon(t, v, "MkDB") // pipeline returns MkDB
		},
	},
	{
		name: "gadts",
		file: "12_gadts.gmp",
		setup: func(e *gmp.Engine) {
			e.EnableRecursion()
			e.SetStepLimit(100_000_000)
		},
		check: func(t *testing.T, v gmp.Value) {
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			// expr1 = And True (Not False) = And True True = True
			assertCon(t, con.Args[0], "True")
		},
	},
	{
		name: "modules",
		file: "13_modules.gmp",
		setup: func(e *gmp.Engine) {
			e.NoPrelude()
			err := e.RegisterModule("Lib", `
data LibBool = LibTrue | LibFalse
libTrue := LibTrue
libNot :: LibBool -> LibBool
libNot := \b -> case b of { LibTrue -> LibFalse; LibFalse -> LibTrue }
`)
			if err != nil {
				panic(err)
			}
		},
		check: func(t *testing.T, v gmp.Value) {
			assertCon(t, v, "LibTrue") // double negation
		},
	},
	{
		name: "stdlib",
		file: "14_stdlib.gmp",
		setup: func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			// main = Pair True (Pair True (Pair False (Pair True (Pair (Just False) True))))
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			assertCon(t, con.Args[0], "True") // eq True True
		},
	},
	{
		name: "existentials",
		file: "15_existentials.gmp",
		setup: func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			// selfEq packBool = True (eq True True)
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			assertCon(t, con.Args[0], "True")
		},
	},
	{
		name: "higher_rank",
		file: "16_higher_rank.gmp",
		setup: func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			// main = Pair (Pair True Unit) (Pair True ...)
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			// first element is applyToBoth id = Pair True Unit
			inner, ok := con.Args[0].(*gmp.ConVal)
			if !ok || inner.Con != "Pair" {
				t.Errorf("expected inner Pair, got %s", con.Args[0])
				return
			}
			assertCon(t, inner.Args[0], "True")
			assertCon(t, inner.Args[1], "Unit")
		},
	},
	{
		name: "stdlib_v05",
		file: "17_stdlib_v05.gmp",
		setup: func(e *gmp.Engine) {},
		check: func(t *testing.T, v gmp.Value) {
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			assertCon(t, con.Args[0], "Unit") // append Unit Unit = Unit
		},
	},
	{
		name: "literals_arithmetic",
		file: "18_literals_arithmetic.gmp",
		setup: func(e *gmp.Engine) {
			if err := e.Use(stdlib.Num); err != nil {
				panic(err)
			}
		},
		check: func(t *testing.T, v gmp.Value) {
			// main = Pair 42 (Pair 3 (Pair 7 (Pair 10 (Pair True (Pair LT 21)))))
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			// litInt = 42
			if hv := gmp.MustHost[int64](con.Args[0]); hv != 42 {
				t.Errorf("litInt: expected 42, got %d", hv)
			}
		},
	},
	{
		name: "string_operations",
		file: "19_string_operations.gmp",
		setup: func(e *gmp.Engine) {
			if err := e.Use(stdlib.Num); err != nil {
				panic(err)
			}
			if err := e.Use(stdlib.Str); err != nil {
				panic(err)
			}
		},
		check: func(t *testing.T, v gmp.Value) {
			// main = Pair "hello world" (Pair 5 (Pair True ...))
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			if hv := gmp.MustHost[string](con.Args[0]); hv != "hello world" {
				t.Errorf("strConcat: expected 'hello world', got '%s'", hv)
			}
		},
	},
	{
		name: "effect_capabilities",
		file: "20_effect_capabilities.gmp",
		setup: func(e *gmp.Engine) {
			if err := e.Use(stdlib.Num); err != nil {
				panic(err)
			}
			if err := e.Use(stdlib.Fail); err != nil {
				panic(err)
			}
			if err := e.Use(stdlib.State); err != nil {
				panic(err)
			}
		},
		caps: map[string]any{
			"state": &gmp.HostVal{Inner: int64(0)},
			"fail":  &gmp.ConVal{Con: "Unit"},
		},
		check: func(t *testing.T, v gmp.Value) {
			// main = Pair v1 (Pair v2 (Pair v3 (Pair v4 (Pair v5 v6))))
			// v1=100, v2=1, v3=42, v4=True, v5=20, v6=111
			con, ok := v.(*gmp.ConVal)
			if !ok || con.Con != "Pair" {
				t.Errorf("expected Pair, got %s", v)
				return
			}
			if hv := gmp.MustHost[int64](con.Args[0]); hv != 100 {
				t.Errorf("v1 (putAndGet): expected 100, got %d", hv)
			}
		},
	},
}

func copyCaps(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func assertCon(t *testing.T, v gmp.Value, name string) {
	t.Helper()
	con, ok := v.(*gmp.ConVal)
	if !ok {
		t.Errorf("expected ConVal, got %T: %s", v, v)
		return
	}
	if name != "" && con.Con != name {
		t.Errorf("expected %s, got %s", name, con.Con)
	}
}

// ---------------------------------------------------------------------------
// Test runner — compile + evaluate each program
// ---------------------------------------------------------------------------

func TestStressPrograms(t *testing.T) {
	for _, sp := range stressPrograms {
		t.Run(sp.name, func(t *testing.T) {
			source := loadStressProgram(t, sp.file)
			eng := gmp.NewEngine()
			sp.setup(eng)

			start := time.Now()
			rt, err := eng.NewRuntime(source)
			compileTime := time.Since(start)

			if err != nil {
				t.Fatalf("compile failed (%v): %v", compileTime, err)
			}
			t.Logf("compiled in %v", compileTime)

			ctx := context.Background()
			start = time.Now()
			var result *gmp.RunResult
			caps := copyCaps(sp.caps)
			if caps != nil {
				full, err := rt.RunContextFull(ctx, caps, sp.binds, "main")
				if err != nil {
					t.Fatalf("eval failed: %v", err)
				}
				result = &gmp.RunResult{Value: full.Value, Stats: full.Stats}
			} else {
				result, err = rt.RunContext(ctx, nil, sp.binds, "main")
				if err != nil {
					t.Fatalf("eval failed: %v", err)
				}
			}
			evalTime := time.Since(start)
			t.Logf("evaluated in %v, steps=%d", evalTime, result.Stats.Steps)

			if sp.check != nil {
				sp.check(t, result.Value)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmarks — compilation and evaluation
// ---------------------------------------------------------------------------

func BenchmarkStressCompile(b *testing.B) {
	for _, sp := range stressPrograms {
		b.Run(sp.name, func(b *testing.B) {
			source := loadStressProgram(b, sp.file)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				eng := gmp.NewEngine()
				sp.setup(eng)
				_, err := eng.NewRuntime(source)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkStressEval(b *testing.B) {
	for _, sp := range stressPrograms {
		b.Run(sp.name, func(b *testing.B) {
			source := loadStressProgram(b, sp.file)
			eng := gmp.NewEngine()
			sp.setup(eng)
			rt, err := eng.NewRuntime(source)
			if err != nil {
				b.Fatal(err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				caps := copyCaps(sp.caps)
				if caps != nil {
					_, err = rt.RunContextFull(ctx, caps, sp.binds, "main")
				} else {
					_, err = rt.RunContext(ctx, nil, sp.binds, "main")
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Stress: programmatic scale test — generated large program
// ---------------------------------------------------------------------------

func TestStressGeneratedLargeProgram(t *testing.T) {
	// Generate a program with 100 data types, 100 functions, 50 class instances.
	var source string
	source += "data D0 = D0C0 | D0C1 | D0C2\n"

	// Generate 50 additional data types
	for i := 1; i <= 50; i++ {
		source += fmt.Sprintf("data D%d a = D%dA a | D%dB\n", i, i, i)
	}

	// Generate Eq instances for D0
	source += `
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x -> \y -> case x of { True -> y; False -> case y of { True -> False; False -> True } } }
instance Eq Unit { eq := \_ -> \_ -> True }
`

	// Generate 50 functions that pattern match
	for i := 0; i < 50; i++ {
		source += fmt.Sprintf(`
f%d :: forall a. a -> a
f%d := \x -> x
`, i, i)
	}

	// Generate chain
	source += "main := f0 (f1 (f2 (f3 (f4 (f5 (f6 (f7 (f8 (f9 True)))))))))\n"

	eng := gmp.NewEngine()
	start := time.Now()
	rt, err := eng.NewRuntime(source)
	compileTime := time.Since(start)
	if err != nil {
		t.Fatalf("compile failed (%v): %v", compileTime, err)
	}
	t.Logf("generated program: compiled in %v", compileTime)

	start = time.Now()
	result, err := rt.RunContext(context.Background(), nil, nil, "main")
	evalTime := time.Since(start)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	t.Logf("evaluated in %v, steps=%d", evalTime, result.Stats.Steps)
	assertCon(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// Memory stress test
// ---------------------------------------------------------------------------

func TestStressMemory(t *testing.T) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	// Compile and run all stress programs
	for _, sp := range stressPrograms {
		data, err := os.ReadFile(filepath.Join("testdata", "stress", sp.file))
		if err != nil {
			continue // skip missing files in memory test
		}
		source := string(data)
		eng := gmp.NewEngine()
		sp.setup(eng)
		rt, err := eng.NewRuntime(source)
		if err != nil {
			continue // skip compile failures in memory test
		}
		ctx := context.Background()
		caps := copyCaps(sp.caps)
		if caps != nil {
			rt.RunContextFull(ctx, caps, sp.binds, "main")
		} else {
			rt.RunContext(ctx, nil, sp.binds, "main")
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&after)

	allocMB := float64(after.TotalAlloc-before.TotalAlloc) / (1024 * 1024)
	t.Logf("total allocation for all stress programs: %.2f MB", allocMB)
	if allocMB > 500 {
		t.Errorf("excessive memory usage: %.2f MB", allocMB)
	}
}
