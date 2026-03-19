package stress_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/cwd-k2/gicel"
)

// ---------------------------------------------------------------------------
// Stress test programs — each exercises a distinct stress vector.
// Programs are stored in testdata/stress/*.gicel files.
// ---------------------------------------------------------------------------

func loadStressProgram(t testing.TB, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "stress", name))
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
	setup func(*gicel.Engine)
	check func(*testing.T, gicel.Value)
	caps  map[string]any
	binds map[string]gicel.Value
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

var stressPrograms = []stressProgram{
	{
		name:  "adt_exhaustiveness",
		file:  "01_adt_exhaustiveness.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			// pairMatch (shapeToColor (chainAll Cyan)) (chainAll Cyan)
			// chainAll Cyan = dayToShape (colorToDay Cyan) = dayToShape Fri = Hexagon
			// shapeToColor Hexagon = Cyan
			// pairMatch Cyan Hexagon = False (Cyan → _ branch)
			assertCon(t, v, "False")
		},
	},
	{
		name:  "typeclass_hierarchy",
		file:  "02_typeclass_hierarchy.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			assertCon(t, v, "True") // isEqOrdering True True => EQ => True
		},
	},
	{
		name:  "hkt_functor",
		file:  "03_hkt_functor.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			assertCon(t, v, "False") // fmap not (Branch (Leaf True) ...) => Leaf False at left
		},
	},
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
				return &gicel.RecordVal{Fields: map[string]gicel.Value{}}, ce.Set("n", n+1), nil
			})
			e.RegisterPrim("addN", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("n")
				n, _ := v.(int)
				delta := gicel.MustHost[int](args[0])
				return &gicel.RecordVal{Fields: map[string]gicel.Value{}}, ce.Set("n", n+delta), nil
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
		name:  "multi_param_classes",
		file:  "05_multi_param_classes.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			// convertAndCoerce True = coerce (convert True)
			// convert True (Convert Bool (Maybe Bool)) = Just True
			// coerce (Just True) (Coercible (Maybe Bool) Bool) = True
			// coerced2 = (coerce :: () -> Bool) (coerce True) = True
			// main = case True { True -> coerced2; ... } = True
			assertCon(t, v, "True")
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
		name: "recursive_data",
		file: "07_recursive_data.gicel",
		setup: func(e *gicel.Engine) {
			e.EnableRecursion()
			e.SetStepLimit(100_000_000)
			e.SetDepthLimit(100_000)
			intPrimSetup(e)
		},
		check: func(t *testing.T, v gicel.Value) {
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) != 2 {
				t.Errorf("expected tuple, got %s", v)
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
				return &gicel.RecordVal{Fields: map[string]gicel.Value{}}, ce.Set("db", n), nil
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
				return &gicel.RecordVal{Fields: map[string]gicel.Value{}}, ce.Set("log", n+delta), nil
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
		name:  "conditional_instances",
		file:  "09_conditional_instances.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			// main = (eqTest1, (eqTest2, (eqTest3, eqTest4)))
			// eqTest1 = True (val1==val2), eqTest2 = False (val1!=val3)
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			assertCon(t, rv.Fields["_1"], "True") // val1 == val2
		},
	},
	{
		name: "full_grammar",
		file: "10_full_grammar.gicel",
		setup: func(e *gicel.Engine) {
			capEnvSetup(e)
			e.DeclareBinding("seed", gicel.ConType("Int"))
			e.RegisterPrim("readS", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				v, _ := ce.Get("s")
				n, _ := v.(int)
				return gicel.ToValue(n), ce, nil
			})
			e.RegisterPrim("writeS", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
				n := gicel.MustHost[int](args[0])
				return &gicel.RecordVal{Fields: map[string]gicel.Value{}}, ce.Set("s", n), nil
			})
		},
		caps:  map[string]any{"s": 0},
		binds: map[string]gicel.Value{"seed": gicel.ToValue(10)},
		check: func(t *testing.T, v gicel.Value) {
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) != 2 {
				t.Errorf("expected tuple, got %s", v)
			}
		},
	},
	{
		name:  "datakinds",
		file:  "11_datakinds.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			assertCon(t, v, "MkDB") // pipeline returns MkDB
		},
	},
	{
		name: "gadts",
		file: "12_gadts.gicel",
		setup: func(e *gicel.Engine) {
			e.EnableRecursion()
			e.SetStepLimit(100_000_000)
		},
		check: func(t *testing.T, v gicel.Value) {
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			// expr1 = And True (Not False) = And True True = True
			assertCon(t, rv.Fields["_1"], "True")
		},
	},
	{
		name: "modules",
		file: "13_modules.gicel",
		setup: func(e *gicel.Engine) {
			err := e.RegisterModule("Lib", `
data LibBool := LibTrue | LibFalse
libTrue := LibTrue
libNot :: LibBool -> LibBool
libNot := \b. case b { LibTrue -> LibFalse; LibFalse -> LibTrue }
`)
			if err != nil {
				panic(err)
			}
		},
		check: func(t *testing.T, v gicel.Value) {
			assertCon(t, v, "LibTrue") // double negation
		},
	},
	{
		name:  "stdlib",
		file:  "14_stdlib.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			// main = (True, (True, (False, (True, ((Just False), True)))))
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			assertCon(t, rv.Fields["_1"], "True") // eq True True
		},
	},
	{
		name:  "existentials",
		file:  "15_existentials.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			// selfEq packBool = True (eq True True)
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			assertCon(t, rv.Fields["_1"], "True")
		},
	},
	{
		name:  "higher_rank",
		file:  "16_higher_rank.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			// main = ((True, ()), (True, ...))
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			// first element is applyToBoth id = (True, ())
			inner, ok := rv.Fields["_1"].(*gicel.RecordVal)
			if !ok || len(inner.Fields) != 2 {
				t.Errorf("expected inner tuple, got %s", rv.Fields["_1"])
				return
			}
			assertCon(t, inner.Fields["_1"], "True")
			unitV, ok := inner.Fields["_2"].(*gicel.RecordVal)
			if !ok || len(unitV.Fields) != 0 {
				t.Errorf("expected () in inner _2, got %s", inner.Fields["_2"])
			}
		},
	},
	{
		name:  "stdlib_v05",
		file:  "17_stdlib_v05.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			// First element: sgOrdLT = append LT EQ = LT
			assertCon(t, rv.Fields["_1"], "LT")
		},
	},
	{
		name:  "literals_arithmetic",
		file:  "18_literals_arithmetic.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			// main = (42, (3, (7, (10, (True, (LT, 21))))))
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			// litInt = 42
			if hv := gicel.MustHost[int64](rv.Fields["_1"]); hv != 42 {
				t.Errorf("litInt: expected 42, got %d", hv)
			}
		},
	},
	{
		name:  "string_operations",
		file:  "19_string_operations.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			// main = ("hello world", (5, (True, ...)))
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			if hv := gicel.MustHost[string](rv.Fields["_1"]); hv != "hello world" {
				t.Errorf("strConcat: expected 'hello world', got '%s'", hv)
			}
		},
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
			"fail":  &gicel.RecordVal{Fields: map[string]gicel.Value{}},
		},
		check: func(t *testing.T, v gicel.Value) {
			// main = (v1, (v2, (v3, (v4, (v5, v6)))))
			// v1=100, v2=1, v3=42, v4=True, v5=20, v6=111
			rv, ok := v.(*gicel.RecordVal)
			if !ok || len(rv.Fields) < 2 {
				t.Errorf("expected tuple, got %s", v)
				return
			}
			if hv := gicel.MustHost[int64](rv.Fields["_1"]); hv != 100 {
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
	{
		name:  "records_tuples",
		file:  "22_records_tuples.gicel",
		setup: func(e *gicel.Engine) {},
		check: func(t *testing.T, v gicel.Value) {
			p := v
			// getX point = 3
			p = assertPairHead(t, p, "getX", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, int64(3))
			})
			// getY moved = 4 (y unchanged by update)
			p = assertPairHead(t, p, "getY moved", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, int64(4))
			})
			// label1 = "hello"
			p = assertPairHead(t, p, "label1", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, "hello")
			})
			// label2 = "world"
			p = assertPairHead(t, p, "label2", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, "world")
			})
			// innerA = True
			p = assertPairHead(t, p, "innerA", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// eqUnit = True
			p = assertPairHead(t, p, "eqUnit", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// eqPair = True
			p = assertPairHead(t, p, "eqPair", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// eqPairF = False
			p = assertPairHead(t, p, "eqPairF", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "False")
			})
			// cmpUnit = EQ
			p = assertPairHead(t, p, "cmpUnit", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "EQ")
			})
			// cmpPair = LT
			p = assertPairHead(t, p, "cmpPair", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "LT")
			})
			// swapped = (False, True)
			p = assertPairHead(t, p, "swapped", func(t *testing.T, v gicel.Value) {
				rv, ok := v.(*gicel.RecordVal)
				if !ok {
					t.Errorf("expected RecordVal, got %T", v)
					return
				}
				assertCon(t, rv.Fields["_1"], "False")
				assertCon(t, rv.Fields["_2"], "True")
			})
			// xyTuple = (3, 4)
			p = assertPairHead(t, p, "xyTuple", func(t *testing.T, v gicel.Value) {
				rv, ok := v.(*gicel.RecordVal)
				if !ok {
					t.Errorf("expected RecordVal, got %T", v)
					return
				}
				assertHostVal(t, rv.Fields["_1"], int64(3))
				assertHostVal(t, rv.Fields["_2"], int64(4))
			})
			// d1 = 1
			p = assertPairHead(t, p, "d1", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, int64(1))
			})
			// d2 = 2
			p = assertPairHead(t, p, "d2", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, int64(2))
			})
			// d3 = 3
			p = assertPairHead(t, p, "d3", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, int64(3))
			})
			// eqRecord = True
			p = assertPairHead(t, p, "eqRecord", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "True")
			})
			// ordRecord = LT
			p = assertPairHead(t, p, "ordRecord", func(t *testing.T, v gicel.Value) {
				assertCon(t, v, "LT")
			})
			// unwrapped = 1
			p = assertPairHead(t, p, "unwrapped", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, int64(1))
			})
			// sumBig = 55
			p = assertPairHead(t, p, "sumBig", func(t *testing.T, v gicel.Value) {
				assertHostVal(t, v, int64(55))
			})
			// firstPoint = 0
			assertHostVal(t, p, int64(0))
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

// ---------------------------------------------------------------------------
// Test runner — compile + evaluate each program
// ---------------------------------------------------------------------------

func TestStressPrograms(t *testing.T) {
	for _, sp := range stressPrograms {
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
// Benchmarks — compilation and evaluation
// ---------------------------------------------------------------------------

func BenchmarkStressCompile(b *testing.B) {
	for _, sp := range stressPrograms {
		b.Run(sp.name, func(b *testing.B) {
			source := loadStressProgram(b, sp.file)
			// Build a template engine to verify compilability before benchmarking.
			// Each iteration creates a fresh engine to measure full compilation cost.
			tmpl := gicel.NewEngine()
			tmpl.Use(gicel.Prelude)
			sp.setup(tmpl)
			// Verify the program compiles before benchmarking.
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

func BenchmarkStressEval(b *testing.B) {
	for _, sp := range stressPrograms {
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
// Stress: programmatic scale test — generated large program
// ---------------------------------------------------------------------------

func TestStressGeneratedLargeProgram(t *testing.T) {
	// Generate a program with 100 data types, 100 functions, 50 class instances.
	var source string
	source += "import Prelude\n"
	source += "data D0 := D0C0 | D0C1 | D0C2\n"

	// Generate 50 additional data types
	for i := 1; i <= 50; i++ {
		source += fmt.Sprintf("data D%d a := D%dA a | D%dB\n", i, i, i)
	}

	// Eq and its instances are already provided by Prelude.
	// No need to redeclare them.

	// Generate 50 functions that pattern match
	for i := 0; i < 50; i++ {
		source += fmt.Sprintf(`
f%d :: \ a. a -> a
f%d := \x. x
`, i, i)
	}

	// Generate chain
	source += "main := f0 (f1 (f2 (f3 (f4 (f5 (f6 (f7 (f8 (f9 True)))))))))\n"

	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	start := time.Now()
	rt, err := eng.NewRuntime(context.Background(), source)
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

// ---------------------------------------------------------------------------
// Memory stress test
// ---------------------------------------------------------------------------

func TestStressMemory(t *testing.T) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	// Compile and run all stress programs
	for _, sp := range stressPrograms {
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
// Stress test helpers for 21_ixmonad_monadic
// ---------------------------------------------------------------------------

// assertPairHead extracts the first element of a tuple, checks it, and returns the second.
func assertPairHead(t *testing.T, v gicel.Value, label string, check func(*testing.T, gicel.Value)) gicel.Value {
	t.Helper()
	rv, ok := v.(*gicel.RecordVal)
	if !ok || len(rv.Fields) < 2 {
		t.Errorf("%s: expected tuple, got %v", label, v)
		return v
	}
	check(t, rv.Fields["_1"])
	return rv.Fields["_2"]
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
	// = 3×2 = 6 elements
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
	t.Logf("list cartesian 3×2: %v, steps=%d", elapsed, result.Stats.Steps)

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

// ---------------------------------------------------------------------------
// Stress: Computation do with effects + IxMonad dispatch coexistence
// ---------------------------------------------------------------------------

func TestStressComputationEffectsWithMonadicValues(t *testing.T) {
	eng := gicel.NewEngine()
	if err := eng.Use(gicel.Prelude); err != nil {
		t.Fatal(err)
	}
	if err := eng.Use(gicel.EffectState); err != nil {
		t.Fatal(err)
	}

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Effect.State

-- Computation do block that manipulates Maybe values internally
main :: Computation { state: Int | r } { state: Int | r } (Maybe Int)
main := do {
  x <- get;
  put (x + 1);
  y <- get;
  put (y + 1);
  z <- get;
  pure (Just z)
}
`)
	if err != nil {
		t.Fatal(err)
	}
	caps := map[string]any{"state": &gicel.HostVal{Inner: int64(0)}}
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Caps: caps})
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just, got %v", result.Value)
	}
	hv, ok := con.Args[0].(*gicel.HostVal)
	if !ok || hv.Inner != int64(2) {
		t.Fatalf("expected Just 2, got Just %v", con.Args[0])
	}
}

// ---------------------------------------------------------------------------
// Stress: Type class instance resolution scaling
// ---------------------------------------------------------------------------

func TestStressInstanceResolutionDeep(t *testing.T) {
	// Deeply nested conditional instance: Eq (Maybe (Maybe (Maybe ... Bool)))
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	const depth = 10

	// Build: Eq (Maybe (Maybe (... (Maybe Bool) ...)))
	inner := "Bool"
	for i := 0; i < depth; i++ {
		inner = "(Maybe " + inner + ")"
	}

	// Build value: Just (Just (... (Just True) ...))
	valInner := "True"
	for i := 0; i < depth; i++ {
		valInner = "(Just " + valInner + ")"
	}

	source := fmt.Sprintf(`import Prelude
main := eq %s %s
`, valInner, valInner)

	start := time.Now()
	rt, err := eng.NewRuntime(context.Background(), source)
	compileTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("instance resolution depth=%d: compiled in %v", depth, compileTime)

	start = time.Now()
	result, err := rt.RunWith(context.Background(), nil)
	evalTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("instance resolution depth=%d: eval %v, steps=%d", depth, evalTime, result.Stats.Steps)

	assertCon(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// Stress: All prelude classes combined
// ---------------------------------------------------------------------------

func TestStressAllClassesCombined(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
-- Eq
r1 := eq True True
r2 := eq (Just LT) (Just LT)
r3 := eq (True, ()) (True, ())
r4 := eq (Cons True (Cons False Nil)) (Cons True (Cons False Nil))

-- Ord
r5 := compare False True
r6 := compare (Just False) (Just True)
r7 := compare (True, False) (True, True)

-- Semigroup
r8 := append LT GT
r9 := append (Cons True Nil) (Cons False Nil)

-- Monoid
r10 := (empty :: Ordering)
r11 := (empty :: List Bool)

-- Functor
r12 := fmap (\x. case x { True -> False; False -> True }) (Just True)
r13 := fmap (\x. Just x) (Cons True (Cons False Nil))

-- Foldable
r14 := foldr (\x acc. acc) False (Just True)
r15 := foldr (\x acc. Cons x acc) Nil (Cons True (Cons False Nil))

-- Applicative
r16 := (wrap True :: Maybe Bool)
r17 := ap (Just (\x. case x { True -> False; False -> True })) (Just True)

main := (r1, (r2, (r3, (r4, (r5, (r6, (r7, (r8,
  (r9, (r10, (r11, (r12, (r13, (r14, (r15,
  (r16, r17))))))))))))))))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// r1 = True
	p := result.Value
	p = assertPairHead(t, p, "eq Bool", func(t *testing.T, v gicel.Value) { assertCon(t, v, "True") })
	// r2 = True (eq (Just LT) (Just LT))
	p = assertPairHead(t, p, "eq Maybe Ordering", func(t *testing.T, v gicel.Value) { assertCon(t, v, "True") })
	// r3 = True (eq (True, ()) (True, ()))
	p = assertPairHead(t, p, "eq tuple", func(t *testing.T, v gicel.Value) { assertCon(t, v, "True") })
	// r4 = True (eq List 2-element)
	p = assertPairHead(t, p, "eq List 2", func(t *testing.T, v gicel.Value) { assertCon(t, v, "True") })
	// r5 = LT (compare False True)
	p = assertPairHead(t, p, "compare Bool", func(t *testing.T, v gicel.Value) { assertCon(t, v, "LT") })
	// r6 = LT (compare Just False, Just True)
	p = assertPairHead(t, p, "compare Maybe", func(t *testing.T, v gicel.Value) { assertCon(t, v, "LT") })
	// r7 = LT (compare (True, False) (True, True) → append EQ LT = LT)
	p = assertPairHead(t, p, "compare tuple", func(t *testing.T, v gicel.Value) { assertCon(t, v, "LT") })
	// r8 = LT (append LT GT = LT, non-EQ preserves)
	p = assertPairHead(t, p, "semigroup Ordering", func(t *testing.T, v gicel.Value) { assertCon(t, v, "LT") })
	_ = p
}

// ---------------------------------------------------------------------------
// Stress: Deeply nested monadic operations mixing do and value-level
// ---------------------------------------------------------------------------

func TestStressMixedMonadicValueLevel(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
-- fmap over a Maybe-do result
not :: Bool -> Bool
not := \b. case b { True -> False; False -> True }

inner :: Maybe Bool
inner := do { x <- Just True; pure (not x) }

outer := fmap not inner

main := (inner, outer)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok || len(rv.Fields) != 2 {
		t.Fatalf("expected tuple, got %v", result.Value)
	}
	// inner = Just False (not True = False)
	assertConArg(t, rv.Fields["_1"], "Just", "False")
	// outer = fmap not (Just False) = Just True
	assertConArg(t, rv.Fields["_2"], "Just", "True")
}

// ---------------------------------------------------------------------------
// Stress: Computation vs Maybe performance comparison
// ---------------------------------------------------------------------------

func TestStressComputationVsMaybePerformance(t *testing.T) {
	// Same depth chain, compare step counts.
	const depth = 30

	// Computation path
	compSource := "import Prelude\nmain := do {\n  v0 <- pure True;\n"
	for i := 1; i < depth; i++ {
		compSource += fmt.Sprintf("  v%d <- pure v%d;\n", i, i-1)
	}
	compSource += fmt.Sprintf("  pure v%d\n}\n", depth-1)

	eng1 := gicel.NewEngine()
	eng1.Use(gicel.Prelude)
	rt1, err := eng1.NewRuntime(context.Background(), compSource)
	if err != nil {
		t.Fatal(err)
	}
	r1, err := rt1.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Maybe path
	maybeSource := "import Prelude\nmain :: Maybe Bool\nmain := do {\n  v0 <- Just True;\n"
	for i := 1; i < depth; i++ {
		maybeSource += fmt.Sprintf("  v%d <- Just v%d;\n", i, i-1)
	}
	maybeSource += fmt.Sprintf("  pure v%d\n}\n", depth-1)

	eng2 := gicel.NewEngine()
	eng2.Use(gicel.Prelude)
	rt2, err := eng2.NewRuntime(context.Background(), maybeSource)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("depth=%d Computation steps=%d, Maybe steps=%d, ratio=%.2f",
		depth, r1.Stats.Steps, r2.Stats.Steps, float64(r2.Stats.Steps)/float64(r1.Stats.Steps))

	// Both should produce correct results
	assertCon(t, r1.Value, "True")
	con, ok := r2.Value.(*gicel.ConVal)
	if !ok || con.Con != "Just" {
		t.Fatalf("expected Just True, got %v", r2.Value)
	}
	assertCon(t, con.Args[0], "True")
}

// ---------------------------------------------------------------------------
// Stress: Large generated List do block
// ---------------------------------------------------------------------------

func TestStressListDoLargeCartesian(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(100_000_000)
	eng.SetDepthLimit(100_000)
	eng.RegisterPrim("add", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), ce, nil
	})
	eng.DeclareBinding("xs", gicel.AppType(gicel.ConType("List"), gicel.ConType("Int")))
	eng.DeclareBinding("ys", gicel.AppType(gicel.ConType("List"), gicel.ConType("Int")))

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
add :: Int -> Int -> Int
add := assumption
main :: List Int
main := do {
  x <- xs;
  y <- ys;
  pure (add x y)
}
`)
	if err != nil {
		t.Fatal(err)
	}

	// xs = [1..10], ys = [1..5] → 50 elements
	xs := make([]any, 10)
	for i := range xs {
		xs[i] = int64(i + 1)
	}
	ys := make([]any, 5)
	for i := range ys {
		ys[i] = int64(i + 1)
	}

	start := time.Now()
	result, err := rt.RunWith(context.Background(), &gicel.RunOptions{Bindings: map[string]gicel.Value{
		"xs": gicel.ToList(xs),
		"ys": gicel.ToList(ys),
	}})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}

	items, ok := gicel.FromList(result.Value)
	if !ok || len(items) != 50 {
		t.Fatalf("expected 50 elements, got %d", len(items))
	}
	t.Logf("list cartesian 10×5: %v, steps=%d", elapsed, result.Stats.Steps)

	// First element: add 1 1 = 2
	hv, ok := items[0].(*gicel.HostVal)
	if !ok || hv.Inner != int64(2) {
		t.Fatalf("first element: expected 2, got %v", items[0])
	}
}

// ---------------------------------------------------------------------------
// Stress: Traversable + Applicative on lists
// ---------------------------------------------------------------------------

func TestStressTraverseMaybeOverList(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()

	// traverse Just [True, False, True] → Just [True, False, True]
	// (via Traversable List + Applicative Maybe)
	// Note: Traversable List is NOT in prelude yet, so we test
	// Traversable Maybe instead.
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
not :: Bool -> Bool
not := \b. case b { True -> False; False -> True }

-- traverse over Maybe (Traversable Maybe is in prelude)
test1 := traverse (\x. Just (not x)) (Just True)
test2 := traverse (\x. Just (not x)) (Nothing :: Maybe Bool)

-- traverse that short-circuits to Nothing
test3 := traverse (\_. Nothing :: Maybe Bool) (Just True)

main := (test1, (test2, test3))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	p := result.Value
	// test1 = Just (Just False)
	p = assertPairHead(t, p, "traverse Just over Just", func(t *testing.T, v gicel.Value) {
		outer, ok := v.(*gicel.ConVal)
		if !ok || outer.Con != "Just" {
			t.Errorf("expected Just, got %v", v)
			return
		}
		inner, ok := outer.Args[0].(*gicel.ConVal)
		if !ok || inner.Con != "Just" {
			t.Errorf("expected Just (Just False), got %v", v)
			return
		}
		assertCon(t, inner.Args[0], "False")
	})
	// test2 = Just Nothing
	p = assertPairHead(t, p, "traverse Just over Nothing", func(t *testing.T, v gicel.Value) {
		outer, ok := v.(*gicel.ConVal)
		if !ok || outer.Con != "Just" {
			t.Errorf("expected Just, got %v", v)
			return
		}
		assertCon(t, outer.Args[0], "Nothing")
	})
	// test3 = Nothing (traverse over Just True with a function that returns Nothing)
	con, ok := p.(*gicel.ConVal)
	if !ok || con.Con != "Nothing" {
		t.Fatalf("expected Nothing, got %v", p)
	}
}

// ---------------------------------------------------------------------------
// Stress: Higher-rank + IxMonad interaction
// ---------------------------------------------------------------------------

func TestStressHigherRankWithMonad(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
-- Higher-rank function applied across types, combined with monadic operations
applyToBoth :: (\ a. a -> a) -> (Bool, ())
applyToBoth := \f. (f True, f ())

id :: \ a. a -> a
id := \x. x

-- Use result in a Computation do block
main := do {
  r <- pure (applyToBoth id);
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (tuple), got %v", result.Value)
	}
	assertCon(t, rv.Fields["_1"], "True")
	unitVal, ok := rv.Fields["_2"].(*gicel.RecordVal)
	if !ok || len(unitVal.Fields) != 0 {
		t.Fatalf("expected (), got %v", rv.Fields["_2"])
	}
}

// ---------------------------------------------------------------------------
// Stress: Existentials + monadic patterns
// ---------------------------------------------------------------------------

func TestStressExistentialWithMonadic(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data SomeEq := { MkSomeEq :: \ a. Eq a => a -> SomeEq }

testSelf :: SomeEq -> Bool
testSelf := \s. case s { MkSomeEq x -> eq x x }

-- Pack a Maybe value into SomeEq and test self-equality
packed := MkSomeEq (Just True)

-- Use in Computation
main := do {
  r <- pure (testSelf packed);
  pure r
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	assertCon(t, result.Value, "True")
}

// ---------------------------------------------------------------------------
// Stress: Multiple monadic types in one program
// ---------------------------------------------------------------------------

func TestStressMultiMonadProgram(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	eng.RegisterPrim("add", func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
		return gicel.ToValue(gicel.MustHost[int64](args[0]) + gicel.MustHost[int64](args[1])), ce, nil
	})

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
add :: Int -> Int -> Int
add := assumption

-- Maybe monad
maybeResult :: Maybe Int
maybeResult := do { x <- Just 10; y <- Just 20; pure (add x y) }

-- List monad
listResult :: List Int
listResult := do { x <- Cons 1 (Cons 2 Nil); y <- Cons 10 Nil; pure (add x y) }

-- Computation monad
compResult := do {
  m <- pure maybeResult;
  l <- pure listResult;
  pure (m, l)
}

main := compResult
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (tuple), got %v", result.Value)
	}
	// maybeResult = Just 30
	maybeVal, ok := rv.Fields["_1"].(*gicel.ConVal)
	if !ok || maybeVal.Con != "Just" {
		t.Fatalf("expected Just 30, got %v", rv.Fields["_1"])
	}
	hv, ok := maybeVal.Args[0].(*gicel.HostVal)
	if !ok || hv.Inner != int64(30) {
		t.Fatalf("expected Just 30, got Just %v", maybeVal.Args[0])
	}
	// listResult = [11, 12]
	items, ok := gicel.FromList(rv.Fields["_2"])
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 elements, got %v", rv.Fields["_2"])
	}
}

// ---------------------------------------------------------------------------
// Stress: GADTs + IxMonad coexistence
// ---------------------------------------------------------------------------

func TestStressGADTWithMonadic(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
data Expr a := { LitBool :: Bool -> Expr Bool; Not :: Expr Bool -> Expr Bool }

eval :: Expr Bool -> Bool
eval := fix (\self e. case e {
  LitBool b -> b;
  Not inner -> case self inner { True -> False; False -> True }
})

-- Use GADT eval result in a Maybe do block
maybeEval :: Maybe Bool
maybeEval := do {
  x <- Just (eval (Not (LitBool True)));
  pure x
}

-- Use GADT eval result in Computation
compEval := do {
  x <- pure (eval (Not (Not (LitBool False))));
  pure x
}

main := (maybeEval, compEval)
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	rv, ok := result.Value.(*gicel.RecordVal)
	if !ok {
		t.Fatalf("expected RecordVal (tuple), got %v", result.Value)
	}
	// maybeEval = Just False (Not True = False)
	assertConArg(t, rv.Fields["_1"], "Just", "False")
	// compEval = False (Not (Not False) = False)
	assertCon(t, rv.Fields["_2"], "False")
}

// ---------------------------------------------------------------------------
// Stress: Concurrent monadic evaluation
// ---------------------------------------------------------------------------

func TestStressConcurrentMonadic(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Maybe Bool
main := do {
  x <- Just True;
  y <- Just False;
  pure x
}
`)
	if err != nil {
		t.Fatal(err)
	}

	errs := make(chan error, 20)
	for range 20 {
		go func() {
			result, err := rt.RunWith(context.Background(), nil)
			if err != nil {
				errs <- err
				return
			}
			con, ok := result.Value.(*gicel.ConVal)
			if !ok || con.Con != "Just" {
				errs <- fmt.Errorf("expected Just, got %v", result.Value)
				return
			}
			inner, ok := con.Args[0].(*gicel.ConVal)
			if !ok || inner.Con != "True" {
				errs <- fmt.Errorf("expected True, got %v", con.Args[0])
				return
			}
			errs <- nil
		}()
	}
	for range 20 {
		if err := <-errs; err != nil {
			t.Errorf("concurrent monadic eval failed: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Stress: Edge case — empty List do block with pure
// ---------------------------------------------------------------------------

func TestStressListDoEmpty(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: List Bool
main := do {
  x <- Nil;
  pure x
}
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*gicel.ConVal)
	if !ok || con.Con != "Nil" {
		t.Fatalf("expected Nil, got %v", result.Value)
	}
}

// ---------------------------------------------------------------------------
// Stress: Monoid + Foldable interaction on lists
// ---------------------------------------------------------------------------

func TestStressMonoidFoldableList(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	eng.EnableRecursion()

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
-- foldr over List of Orderings using Semigroup's append
-- foldr append EQ [LT, EQ, GT] → LT (first non-EQ wins)
main := foldr (\x acc. append x acc) (empty :: Ordering) (Cons LT (Cons EQ (Cons GT Nil)))
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// foldr append EQ [LT, EQ, GT]
	// = append LT (append EQ (append GT EQ))
	// = append LT (append EQ GT)
	// = append LT GT
	// = LT
	assertCon(t, result.Value, "LT")
}
