// Stress typeclass tests — class hierarchy, HKT functors, multi-param classes, conditional instances.
// Does NOT cover: types, effect, stdlib, grammar.

package stress_test

import (
	"testing"

	"github.com/cwd-k2/gicel"
)

var typeclassPrograms = []stressProgram{
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
}

func TestStressTypeclass(t *testing.T) {
	runStressPrograms(t, typeclassPrograms)
}
