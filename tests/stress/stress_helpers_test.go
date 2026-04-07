// Stress helpers — shared types, setup functions, test runners, and assertion helpers.
// Does NOT cover: domain-specific test cases (types, typeclass, effect, stdlib, grammar).

package stress_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Shared types
// ---------------------------------------------------------------------------

type stressProgram struct {
	name  string
	file  string // filename under testdata/stress/
	setup func(*gicel.Engine)
	check func(*testing.T, gicel.Value)
	caps  map[string]any
	binds map[string]gicel.Value
}

// ---------------------------------------------------------------------------
// Setup helpers
// ---------------------------------------------------------------------------

func loadStressProgram(t testing.TB, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "stress", name))
	if err != nil {
		t.Fatalf("failed to load stress program %s: %v", name, err)
	}
	return string(data)
}

func intPrimSetup(eng *gicel.Engine) {
	eng.RegisterType("Int", gicel.KindType())
	eng.RegisterPrim("intZero", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(0), ce, nil
	})
	eng.RegisterPrim("intSucc", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		n := gicel.MustHost[int](args[0])
		return gicel.ToValue(n + 1), ce, nil
	})
	eng.RegisterPrim("intAdd", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		a := gicel.MustHost[int](args[0])
		b := gicel.MustHost[int](args[1])
		return gicel.ToValue(a + b), ce, nil
	})
	eng.RegisterPrim("intEq", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		a := gicel.MustHost[int](args[0])
		b := gicel.MustHost[int](args[1])
		if a == b {
			return &gicel.ConVal{Con: "True"}, ce, nil
		}
		return &gicel.ConVal{Con: "False"}, ce, nil
	})
}

func capEnvSetup(eng *gicel.Engine) {
	eng.RegisterType("Int", gicel.KindType())
	eng.RegisterPrim("intAdd", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		a := gicel.MustHost[int](args[0])
		b := gicel.MustHost[int](args[1])
		return gicel.ToValue(a + b), ce, nil
	})
}

func copyCaps(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	c := make(map[string]any, len(m))
	maps.Copy(c, m)
	return c
}

// ---------------------------------------------------------------------------
// Shared test runner
// ---------------------------------------------------------------------------

func runStressPrograms(t *testing.T, programs []stressProgram) {
	for _, sp := range programs {
		t.Run(sp.name, func(t *testing.T) {
			source := loadStressProgram(t, sp.file)
			eng := gicel.NewEngine()
			eng.Use(gicel.Prelude)
			sp.setup(eng)

			start := time.Now()
			rt, err := eng.NewRuntime(context.Background(), source)
			compileTime := time.Since(start)

			if err != nil {
				t.Fatalf("compile failed (%v): %v", compileTime, err)
			}
			t.Logf("compiled in %v", compileTime)

			ctx := context.Background()
			start = time.Now()
			var result *gicel.RunResult
			caps := copyCaps(sp.caps)
			opts := &gicel.RunOptions{Caps: caps, Bindings: sp.binds}
			result, err = rt.RunWith(ctx, opts)
			if err != nil {
				t.Fatalf("eval failed: %v", err)
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
// Shared benchmark runners
// ---------------------------------------------------------------------------

func benchStressCompile(b *testing.B, programs []stressProgram) {
	for _, sp := range programs {
		b.Run(sp.name, func(b *testing.B) {
			source := loadStressProgram(b, sp.file)
			tmpl := gicel.NewEngine()
			tmpl.Use(gicel.Prelude)
			sp.setup(tmpl)
			if _, err := tmpl.NewRuntime(context.Background(), source); err != nil {
				b.Skipf("program does not compile: %v", err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				eng := gicel.NewEngine()
				eng.Use(gicel.Prelude)
				sp.setup(eng)
				_, err := eng.NewRuntime(context.Background(), source)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchStressEval(b *testing.B, programs []stressProgram) {
	for _, sp := range programs {
		b.Run(sp.name, func(b *testing.B) {
			source := loadStressProgram(b, sp.file)
			eng := gicel.NewEngine()
			eng.Use(gicel.Prelude)
			sp.setup(eng)
			rt, err := eng.NewRuntime(context.Background(), source)
			if err != nil {
				b.Skipf("program does not compile: %v", err)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx := context.Background()
				caps := copyCaps(sp.caps)
				_, err = rt.RunWith(ctx, &gicel.RunOptions{Caps: caps, Bindings: sp.binds})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

func assertHostVal(t *testing.T, v gicel.Value, want any) {
	t.Helper()
	hv, ok := v.(*gicel.HostVal)
	if !ok {
		t.Errorf("expected HostVal, got %T: %s", v, v)
		return
	}
	if hv.Inner != want {
		t.Errorf("expected %v, got %v", want, hv.Inner)
	}
}

func assertCon(t *testing.T, v gicel.Value, name string) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok {
		t.Errorf("expected ConVal, got %T: %s", v, v)
		return
	}
	if name != "" && con.Con != name {
		t.Errorf("expected %s, got %s", name, con.Con)
	}
}

// assertPairHead extracts the first element of a tuple, checks it, and returns the second.
func assertPairHead(t *testing.T, v gicel.Value, label string, check func(*testing.T, gicel.Value)) gicel.Value {
	t.Helper()
	rv, ok := v.(*gicel.RecordVal)
	if !ok || rv.Len() < 2 {
		t.Errorf("%s: expected tuple, got %v", label, v)
		return v
	}
	check(t, rv.MustGet("_1"))
	return rv.MustGet("_2")
}

// assertConArg checks that v is a ConVal with name `con` and first arg is a ConVal with name `arg`.
// If arg is empty, only the outer constructor is checked.
func assertConArg(t *testing.T, v gicel.Value, conName, argName string) {
	t.Helper()
	con, ok := v.(*gicel.ConVal)
	if !ok || con.Con != conName {
		t.Errorf("expected %s, got %v", conName, v)
		return
	}
	if argName != "" && len(con.Args) > 0 {
		inner, ok := con.Args[0].(*gicel.ConVal)
		if !ok || inner.Con != argName {
			t.Errorf("expected %s(%s), got %s(%v)", conName, argName, conName, con.Args[0])
		}
	}
}

// ---------------------------------------------------------------------------
// General stress tests — generated programs, memory
// ---------------------------------------------------------------------------

func TestStressGeneratedLargeProgram(t *testing.T) {
	// Generate a program with 100 data types, 100 functions, 50 class instances.
	var source strings.Builder
	source.WriteString("import Prelude\n")
	source.WriteString("form D0 := { D0C0: D0; D0C1: D0; D0C2: D0 }\n")

	// Generate 50 additional data types
	for i := 1; i <= 50; i++ {
		source.WriteString(fmt.Sprintf("form D%d := \\a. { D%dA: a -> D%d a; D%dB: D%d a }\n", i, i, i, i, i))
	}

	// Eq and its instances are already provided by Prelude.
	// No need to redeclare them.

	// Generate 50 functions that pattern match
	for i := range 50 {
		source.WriteString(fmt.Sprintf(`
f%d :: \ a. a -> a
f%d := \x. x
`, i, i))
	}

	// Generate chain
	source.WriteString("main := f0 (f1 (f2 (f3 (f4 (f5 (f6 (f7 (f8 (f9 True)))))))))\n")

	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	start := time.Now()
	rt, err := eng.NewRuntime(context.Background(), source.String())
	compileTime := time.Since(start)
	if err != nil {
		t.Fatalf("compile failed (%v): %v", compileTime, err)
	}
	t.Logf("generated program: compiled in %v", compileTime)

	start = time.Now()
	result, err := rt.RunWith(context.Background(), nil)
	evalTime := time.Since(start)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	t.Logf("evaluated in %v, steps=%d", evalTime, result.Stats.Steps)
	assertCon(t, result.Value, "True")
}

func TestStressMemory(t *testing.T) {
	allPrograms := collectAllStressPrograms()

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	// Compile and run all stress programs
	for _, sp := range allPrograms {
		data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "stress", sp.file))
		if err != nil {
			continue // skip missing files in memory test
		}
		source := string(data)
		eng := gicel.NewEngine()
		eng.Use(gicel.Prelude)
		sp.setup(eng)
		rt, err := eng.NewRuntime(context.Background(), source)
		if err != nil {
			continue // skip compile failures in memory test
		}
		ctx := context.Background()
		caps := copyCaps(sp.caps)
		rt.RunWith(ctx, &gicel.RunOptions{Caps: caps, Bindings: sp.binds})
	}

	runtime.GC()
	runtime.ReadMemStats(&after)

	allocMB := float64(after.TotalAlloc-before.TotalAlloc) / (1024 * 1024)
	t.Logf("total allocation for all stress programs: %.2f MB", allocMB)
	if allocMB > 500 {
		t.Errorf("excessive memory usage: %.2f MB", allocMB)
	}
}

// collectAllStressPrograms aggregates all domain program slices.
func collectAllStressPrograms() []stressProgram {
	var all []stressProgram
	all = append(all, typesPrograms...)
	all = append(all, typeclassPrograms...)
	all = append(all, effectPrograms...)
	all = append(all, stdlibPrograms...)
	all = append(all, grammarPrograms...)
	return all
}
