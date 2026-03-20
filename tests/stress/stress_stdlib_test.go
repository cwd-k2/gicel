// Stress stdlib tests — Prelude, stdlib v05, literals/arithmetic, string operations.
// Does NOT cover: types, typeclass, effect, grammar.

package stress_test

import (
	"testing"

	"github.com/cwd-k2/gicel"
)

var stdlibPrograms = []stressProgram{
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
}

func TestStressStdlib(t *testing.T) {
	runStressPrograms(t, stdlibPrograms)
}
