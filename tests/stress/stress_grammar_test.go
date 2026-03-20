// Stress grammar tests — full grammar exercise, module system.
// Does NOT cover: types, typeclass, effect, stdlib.

package stress_test

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel"
)

var grammarPrograms = []stressProgram{
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
}

func TestStressGrammar(t *testing.T) {
	runStressPrograms(t, grammarPrograms)
}
