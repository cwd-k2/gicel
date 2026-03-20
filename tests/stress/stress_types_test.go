// Stress types tests — ADTs, recursive data, datakinds, GADTs, existentials, higher-rank, records/tuples.
// Does NOT cover: typeclass, effect, stdlib, grammar.

package stress_test

import (
	"testing"

	"github.com/cwd-k2/gicel"
)

var typesPrograms = []stressProgram{
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

func TestStressTypes(t *testing.T) {
	runStressPrograms(t, typesPrograms)
}

func BenchmarkStressTypesCompile(b *testing.B) {
	benchStressCompile(b, typesPrograms)
}

func BenchmarkStressTypesEval(b *testing.B) {
	benchStressEval(b, typesPrograms)
}
