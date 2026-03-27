// Stress effect tests — do chains, thunk/force, row polymorphism, capabilities, IxMonad, list/maybe scaling.
// Does NOT cover: types, typeclass, stdlib, grammar.

package stress_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/cwd-k2/gicel"
)

var effectPrograms = []stressProgram{
	{
		name: "deep_do_chain",
		file: "04_deep_do_chain.gicel",
		setup: func(e *gicel.Engine) {
			capEnvSetup(e)
			e.RegisterPrim("getN", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("n")
				n, _ := v.(int)
				return gicel.ToValue(n), ce, nil
			})
			e.RegisterPrim("incN", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("n")
				n, _ := v.(int)
				return gicel.NewRecordFromMap(map[string]gicel.Value{}), ce.Set("n", n+1), nil
			})
			e.RegisterPrim("addN", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("n")
				n, _ := v.(int)
				delta := gicel.MustHost[int](args[0])
				return gicel.NewRecordFromMap(map[string]gicel.Value{}), ce.Set("n", n+delta), nil
			})
		},
		caps: map[string]any{"n": 0},
		check: func(t *testing.T, v gicel.Value) {
			// After 20 steps of addN(n)+incN, counter grows rapidly
			n := gicel.MustHost[int](v)
			if n <= 0 {
				t.Errorf("expected positive counter, got %d", n)
			}
		},
	},
	{
		name: "thunk_force",
		file: "06_thunk_force.gicel",
		setup: func(e *gicel.Engine) {
			eng := e
			eng.RegisterType("Int", gicel.KindType())
			counter := 0
			eng.RegisterPrim("mkVal", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				counter++
				return gicel.ToValue(counter), ce, nil
			})
			eng.RegisterPrim("addInts", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				a := gicel.MustHost[int](args[0])
				b := gicel.MustHost[int](args[1])
				return gicel.ToValue(a + b), ce, nil
			})
		},
		check: func(t *testing.T, v gicel.Value) {
			n := gicel.MustHost[int](v)
			if n <= 0 {
				t.Errorf("expected positive result from thunk chain, got %d", n)
			}
		},
	},
	{
		name: "row_polymorphism",
		file: "08_row_polymorphism.gicel",
		setup: func(e *gicel.Engine) {
			capEnvSetup(e)
			e.RegisterPrim("readDB", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("db")
				n, _ := v.(int)
				return gicel.ToValue(n), ce, nil
			})
			e.RegisterPrim("writeDB", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				n := gicel.MustHost[int](args[0])
				return gicel.NewRecordFromMap(map[string]gicel.Value{}), ce.Set("db", n), nil
			})
			e.RegisterPrim("getLog", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("log")
				n, _ := v.(int)
				return gicel.ToValue(n), ce, nil
			})
			e.RegisterPrim("appendLog", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("log")
				n, _ := v.(int)
				delta := gicel.MustHost[int](args[0])
				return gicel.NewRecordFromMap(map[string]gicel.Value{}), ce.Set("log", n+delta), nil
			})
			e.RegisterPrim("readConfig", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("cfg")
				n, _ := v.(int)
				return gicel.ToValue(n), ce, nil
			})
		},
		caps: map[string]any{"db": 0, "log": 0, "cfg": 1},
	},
	{
		name: "effect_capabilities",
		file: "20_effect_capabilities.gicel",
		setup: func(e *gicel.Engine) {
			if err := e.Use(gicel.EffectFail); err != nil {
				panic(err)
			}
			if err := e.Use(gicel.EffectState); err != nil {
				panic(err)
			}
		},
		caps: map[string]any{
			"state": &gicel.HostVal{Inner: int64(0)},
			"fail":  gicel.NewRecordFromMap(map[string]gicel.Value{}),
		},
		check: func(t *testing.T, v gicel.Value) {
			// main = (v1, (v2, (v3, (v4, (v5, v6)))))
			// v1=100, v2=1, v3=42, v4=True, v5=20, v6=111
			rv, ok := v.(*gicel.RecordVal)
			if !ok || rv.Len() < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			if hv := gicel.MustHost[int64](rv.MustGet("_1")); hv != 100 {
				t.Errorf("v1 (putAndGet): expected 100, got %d", hv)
			}
		},
	},
	{
		name: "ixmonad_monadic",
		file: "21_ixmonad_monadic.gicel",
		setup: func(e *gicel.Engine) {
			e.EnableRecursion()
		},
		check: func(t *testing.T, v gicel.Value) {
			// Deeply nested tuple. Walk the spine and check key values.
			p := v
			// Element 0: maybeChain = Just True
			p = assertPairHead(t, p, "maybeChain", func(t *testing.T, v gicel.Value) {
				assertConArg(t, v, "Just", "True")
			})
			// Element 1: nothingFirst = Nothing
			p = assertPairHead(t, p, "nothingFirst", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "Nothing")
			})
			// Element 2: nothingMiddle = Nothing
			p = assertPairHead(t, p, "nothingMiddle", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "Nothing")
			})
			// Element 3: nothingLast = Nothing
			p = assertPairHead(t, p, "nothingLast", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "Nothing")
			})
			// Element 4: nestedMaybeDo = Just (Just True)
			p = assertPairHead(t, p, "nestedMaybeDo", func(t *testing.T, v gicel.Value) {
				assertConArg(t, v, "Just", "")
			})
			// Element 5: maybePure = Just True
			p = assertPairHead(t, p, "maybePure", func(t *testing.T, v gicel.Value) {
				assertConArg(t, v, "Just", "True")
			})
			// Element 6: maybeDoCase = Just LT
			p = assertPairHead(t, p, "maybeDoCase", func(t *testing.T, v gicel.Value) {
				assertConArg(t, v, "Just", "LT")
			})
			// Element 7: maybeDoNestedCase = Just True
			p = assertPairHead(t, p, "maybeDoNestedCase", func(t *testing.T, v gicel.Value) {
				assertConArg(t, v, "Just", "True")
			})
			// Element 8: listFlatMap = [True, True, False, False]
			p = assertPairHead(t, p, "listFlatMap", func(t *testing.T, v gicel.Value) {
				items, ok := gicel.FromList(v)
				if !ok {
					t.Errorf("expected list, got %v", v)
					return
				}
				if len(items) != 4 {
					t.Errorf("listFlatMap: expected 4 elements, got %d", len(items))
				}
			})
			// Element 9: listFilter = [True, True]
			p = assertPairHead(t, p, "listFilter", func(t *testing.T, v gicel.Value) {
				items, ok := gicel.FromList(v)
				if !ok {
					t.Errorf("expected list, got %v", v)
					return
				}
				if len(items) != 2 {
					t.Errorf("listFilter: expected 2 elements, got %d", len(items))
				}
			})
			// Element 10: listCartesian = 4 pairs
			p = assertPairHead(t, p, "listCartesian", func(t *testing.T, v gicel.Value) {
				items, ok := gicel.FromList(v)
				if !ok {
					t.Errorf("expected list, got %v", v)
					return
				}
				if len(items) != 4 {
					t.Errorf("listCartesian: expected 4 elements, got %d", len(items))
				}
			})
			// Element 11: listEmpty = Nil
			p = assertPairHead(t, p, "listEmpty", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "Nil")
			})
			// Element 12: listSingleton = Cons True Nil
			p = assertPairHead(t, p, "listSingleton", func(t *testing.T, v gicel.Value) {
				items, ok := gicel.FromList(v)
				if !ok || len(items) != 1 {
					t.Errorf("listSingleton: expected 1 element, got %v", v)
				}
			})
			// Element 13: listEqTest = True
			p = assertPairHead(t, p, "listEqTest", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// Element 14: listEqTestFalse = False
			p = assertPairHead(t, p, "listEqTestFalse", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "False")
			})
			// Element 15: listEqNilNil = True
			p = assertPairHead(t, p, "listEqNilNil", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// Element 16: listEqNilCons = False
			p = assertPairHead(t, p, "listEqNilCons", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "False")
			})
			// Element 17: listAppendTest = [True, False]
			p = assertPairHead(t, p, "listAppendTest", func(t *testing.T, v gicel.Value) {
				items, ok := gicel.FromList(v)
				if !ok || len(items) != 2 {
					t.Errorf("listAppendTest: expected 2 elements, got %v", v)
				}
			})
			// Element 18: listMonoidTest = [True]
			p = assertPairHead(t, p, "listMonoidTest", func(t *testing.T, v gicel.Value) {
				items, ok := gicel.FromList(v)
				if !ok || len(items) != 1 {
					t.Errorf("listMonoidTest: expected 1 element, got %v", v)
				}
			})
			// Element 19: listFmapTest = [False, True]
			p = assertPairHead(t, p, "listFmapTest", func(t *testing.T, v gicel.Value) {
				items, ok := gicel.FromList(v)
				if !ok || len(items) != 2 {
					t.Errorf("listFmapTest: expected 2 elements, got %v", v)
				}
			})
			// Element 20: listFoldrTest = True
			p = assertPairHead(t, p, "listFoldrTest", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// Element 21: eqMaybeChain = True
			p = assertPairHead(t, p, "eqMaybeChain", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// Element 22: ordMaybeTest = LT
			p = assertPairHead(t, p, "ordMaybeTest", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "LT")
			})
			// Element 23: eqPairMaybe = True
			p = assertPairHead(t, p, "eqPairMaybe", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// Skip remaining elements — just verify evaluation completes.
			_ = p
		},
	},
}

func TestStressEffect(t *testing.T) {
	runStressPrograms(t, effectPrograms)
}

// ---------------------------------------------------------------------------
// List stress tests (Group 1E)
// ---------------------------------------------------------------------------

func TestStressListFoldrLarge(t *testing.T) {
	// Deep fold: foldr over a large list.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), ce, nil
	})
	eng.EnableRecursion()
	eng.SetStepLimit(10_000_000)
	eng.SetDepthLimit(100_000)
	eng.DeclareBinding("xs", gicel.AppType(gicel.ConType("List"), gicel.ConType("Int")))

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
add :: Int -> Int -> Int
add := assumption
main := foldr add 0 xs
`)
	if err != nil {
		t.Fatal(err)
	}

	// Build a large list: [1, 2, ..., 1000]
	const n = 1000
	items := make([]any, n)
	for i := range items {
		items[i] = int64(i + 1)
	}
	start := time.Now()
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"xs": gicel.ToList(items),
	}})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("foldr over %d elements: %v, steps=%d", n, elapsed, result.Stats.Steps)

	// Sum 1..1000 = 500500
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != int64(500500) {
		t.Fatalf("expected 500500, got %v", result.Value)
	}
}

func TestStressListFmapLarge(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.RegisterPrim("add", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), ce, nil
	})
	eng.EnableRecursion()
	eng.SetStepLimit(10_000_000)
	eng.SetDepthLimit(100_000)
	eng.DeclareBinding("xs", gicel.AppType(gicel.ConType("List"), gicel.ConType("Int")))

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
add :: Int -> Int -> Int
add := assumption
main := fmap (\x. add x 1) xs
`)
	if err != nil {
		t.Fatal(err)
	}

	const n = 500
	items := make([]any, n)
	for i := range items {
		items[i] = int64(i)
	}
	start := time.Now()
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"xs": gicel.ToList(items),
	}})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fmap over %d elements: %v, steps=%d", n, elapsed, result.Stats.Steps)

	got, ok := gicel.FromList(result.Value)
	if !ok || len(got) != n {
		t.Fatalf("expected %d elements, got %d", n, len(got))
	}
	// Verify first and last
	if hv := got[0].(*gicel.HostVal); hv.Inner != int64(1) {
		t.Fatalf("first element: expected 1, got %v", hv.Inner)
	}
	if hv := got[n-1].(*gicel.HostVal); hv.Inner != int64(n) {
		t.Fatalf("last element: expected %d, got %v", n, hv.Inner)
	}
}

func TestStressListFromSliceRoundTrip(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}
	eng.DeclareBinding("xs", gicel.AppType(gicel.ConType("List"), gicel.ConType("Int")))

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := toSlice xs
`)
	if err != nil {
		t.Fatal(err)
	}

	const n = 5000
	items := make([]any, n)
	for i := range items {
		items[i] = int64(i)
	}
	start := time.Now()
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"xs": gicel.ToList(items),
	}})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("toSlice round-trip %d elements: %v, steps=%d", n, elapsed, result.Stats.Steps)

	// toSlice returns a HostVal([]any)
	hv, ok := result.Value.(*gicel.HostVal)
	if !ok {
		t.Fatalf("expected HostVal, got %T", result.Value)
	}
	slice, ok := hv.Inner.([]any)
	if !ok || len(slice) != n {
		t.Fatalf("expected slice of %d, got %v", n, hv.Inner)
	}
}

// ---------------------------------------------------------------------------
// IxMonad stress tests (Group 4C)
// ---------------------------------------------------------------------------

func TestStressDeepDoChainComputation(t *testing.T) {
	// Deep Computation do chain (Core.Bind path) — 100 binds.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.DeclareBinding("x", gicel.ConType("Int"))

	// Generate: main := do { v0 <- pure x; v1 <- pure v0; ... ; pure vN }
	source := ""
	const depth = 100
	source += "main := do {\n"
	source += "  v0 <- pure x;\n"
	for i := 1; i < depth; i++ {
		source += fmt.Sprintf("  v%d <- pure v%d;\n", i, i-1)
	}
	source += fmt.Sprintf("  pure v%d\n}\n", depth-1)

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"x": gicel.ToValue(42),
	}})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("deep Computation do chain (%d binds): %v, steps=%d", depth, elapsed, result.Stats.Steps)

	hv, ok := result.Value.(*gicel.HostVal)
	if !ok || hv.Inner != 42 {
		t.Fatalf("expected 42, got %v", result.Value)
	}
}

func TestStressDeepDoChainMaybe(t *testing.T) {
	// Deep Maybe do chain (class dispatch path) — 50 binds.
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)

	const depth = 50
	source := "import Prelude\nmain :: Maybe Int\nmain := do {\n"
	source += "  v0 <- Just 1;\n"
	for i := 1; i < depth; i++ {
		source += fmt.Sprintf("  v%d <- Just v%d;\n", i, i-1)
	}
	source += fmt.Sprintf("  pure v%d\n}\n", depth-1)

	rt, err := eng.NewRuntime(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	result, err := rt.RunWith(context.Background(), nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("deep Maybe do chain (%d binds): %v, steps=%d", depth, elapsed, result.Stats.Steps)

	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just 1, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok || hv.Inner != int64(1) {
		t.Fatalf("expected Just 1, got Just %v", con.Args[0])
	}
}

// ---------------------------------------------------------------------------
// Programmatic stress: Maybe monad chain scaling
// ---------------------------------------------------------------------------

func TestStressMaybeDoChainScaling(t *testing.T) {
	for _, depth := range []int{10, 25, 50, 100} {
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			eng := gicel.NewEngine()
			eng.Use(gicel.Prelude)
			source := "import Prelude\nmain :: Maybe Bool\nmain := do {\n"
			source += "  v0 <- Just True;\n"
			for i := 1; i < depth; i++ {
				source += fmt.Sprintf("  v%d <- Just v%d;\n", i, i-1)
			}
			source += fmt.Sprintf("  pure v%d\n}\n", depth-1)

			start := time.Now()
			rt, err := eng.NewRuntime(context.Background(), source)
			compileTime := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("compiled in %v", compileTime)

			start = time.Now()
			result, err := rt.RunWith(context.Background(), nil)
			evalTime := time.Since(start)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("depth=%d: eval %v, steps=%d", depth, evalTime, result.Stats.Steps)

			con, ok := result.Value.(*gicel.ConVal)
			if !ok || con.Con != "Just" {
				t.Fatalf("expected Just True, got %v", result.Value)
			}
			assertCon(t, con.Args[0], "True")
		})
	}
}

// ---------------------------------------------------------------------------
// Programmatic stress: List monad cartesian product scaling
// ---------------------------------------------------------------------------

func TestStressListCartesianProduct(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(100_000_000)
	eng.SetDepthLimit(100_000)

	// do { x <- [T,F,T]; y <- [T,F]; Cons (x, y) Nil } :: List (Bool, Bool)
	// = 3x2 = 6 elements
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: List (Bool, Bool)
main := do {
  x <- Cons True (Cons False (Cons True Nil));
  y <- Cons True (Cons False Nil);
  Cons (x, y) Nil
}
`)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	result, err := rt.RunWith(context.Background(), nil)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("list cartesian 3x2: %v, steps=%d", elapsed, result.Stats.Steps)

	items, ok := gicel.FromList(result.Value)
	if !ok || len(items) != 6 {
		t.Fatalf("expected 6 pairs, got %d", len(items))
	}
}

// ---------------------------------------------------------------------------
// Programmatic stress: List monad flatMap scaling
// ---------------------------------------------------------------------------

func TestStressListFlatMapScaling(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(100_000_000)
	eng.SetDepthLimit(100_000)

	// Each element duplicated: [True, False, True] >>= \x. [x, x]
	// = 6 elements
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: List Bool
main := do {
  x <- Cons True (Cons False (Cons True Nil));
  Cons x (Cons x Nil)
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	items, ok := gicel.FromList(result.Value)
	if !ok || len(items) != 6 {
		t.Fatalf("expected 6, got %d", len(items))
	}
	// [True, True, False, False, True, True]
	expected := []string{"True", "True", "False", "False", "True", "True"}
	for i, v := range items {
		con, ok := v.(*gicel.ConVal)
		if !ok || con.Con != expected[i] {
			t.Fatalf("element %d: expected %s, got %v", i, expected[i], v)
		}
	}
}

// ---------------------------------------------------------------------------
// Programmatic stress: Nothing short-circuit at various depths
// ---------------------------------------------------------------------------

func TestStressMaybeNothingShortCircuit(t *testing.T) {
	for _, nothingAt := range []int{1, 5, 10, 20} {
		t.Run(fmt.Sprintf("nothing_at_%d", nothingAt), func(t *testing.T) {
			eng := gicel.NewEngine()
			eng.Use(gicel.Prelude)
			const depth = 25
			source := "import Prelude\nmain :: Maybe Bool\nmain := do {\n"
			for i := 0; i < depth; i++ {
				if i == nothingAt {
					source += fmt.Sprintf("  v%d <- Nothing;\n", i)
				} else if i == 0 {
					source += "  v0 <- Just True;\n"
				} else {
					source += fmt.Sprintf("  v%d <- Just v%d;\n", i, i-1)
				}
			}
			source += fmt.Sprintf("  pure v%d\n}\n", depth-1)

			rt, err := eng.NewRuntime(context.Background(), source)
			if err != nil {
				t.Fatal(err)
			}
			result, err := rt.RunWith(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			con, ok := result.Value.(*gicel.ConVal)
			if !ok || con.Con != "Nothing" {
				t.Fatalf("expected Nothing, got %v", result.Value)
			}
		})
	}
}
