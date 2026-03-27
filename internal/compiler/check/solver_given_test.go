// Solver given equality tests — GADT given + type family reduction, contradiction, class resolution.
// Does NOT cover: checker_gadt_test.go (GADT registration, refinement, exhaustiveness).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Given enables type family reduction ---

func TestSolverGivenEnablesTFReduction(t *testing.T) {
	// GADT Tag with IntTag :: Tag Int, BoolTag :: Tag Bool.
	// Type family F dispatches on the type parameter.
	// In a case branch matching IntTag, the given equality a ~ Int
	// makes F a reducible to F Int = String via Zonk + FamilyReducer.
	source := `
form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

type F :: Type := \(a: Type). case a {
  Int => Int;
  Bool => Bool
}

useTag :: \a. Tag a -> F a -> F a
useTag := \tag x. case tag {
  IntTag => x;
  BoolTag => x
}

main := useTag IntTag 42
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{
			"Int":  types.TypeOfTypes,
			"Bool": types.TypeOfTypes,
		},
	}
	prog := checkSource(t, source, config)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

func TestSolverGivenEnablesTFMultiBranch(t *testing.T) {
	// Both branches of the GADT case use the type family result.
	// IntTag branch: given a ~ Int, F a = Int.
	// BoolTag branch: given a ~ Bool, F a = Bool.
	source := `
form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

type F :: Type := \(a: Type). case a {
  Int => Int;
  Bool => Bool
}

identity :: \a. Tag a -> F a -> F a
identity := \tag x. case tag {
  IntTag => x;
  BoolTag => x
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{
			"Int":  types.TypeOfTypes,
			"Bool": types.TypeOfTypes,
		},
	}
	checkSource(t, source, config)
}

// --- Given equality contradiction ---

func TestSolverGivenContradiction(t *testing.T) {
	// Tag Int scrutinee: BoolTag branch has contradictory given (a ~ Bool
	// but a ~ Int from scrutinee type). The branch is inaccessible.
	// Exhaustiveness should only require IntTag.
	source := `
form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

f :: Tag Int -> Int
f := \tag. case tag {
  IntTag => 42
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{
			"Int":  types.TypeOfTypes,
			"Bool": types.TypeOfTypes,
		},
	}
	checkSource(t, source, config)
}

func TestSolverGivenContradictionNoPanic(t *testing.T) {
	// Multiple contradictory branches: ensure no panic during solving.
	source := `
form Unit := { Unit: Unit; }
form Tag := \a. { TagInt: Tag Int; TagBool: Tag Bool; TagUnit: Tag Unit }

f :: Tag Int -> Int
f := \tag. case tag {
  TagInt => 0
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{
			"Int":  types.TypeOfTypes,
			"Bool": types.TypeOfTypes,
		},
	}
	checkSourceNoPanic(t, source, config)
}

// --- Given equality enables GADT branch typing ---

func TestSolverGivenEqEnablesGADTBranch(t *testing.T) {
	// GADT branch with given a ~ Int makes x: a usable where return type is a.
	// This tests given equality installation and GADT type refinement, not class
	// resolution. For given-enabled class resolution, see the engine-level test
	// TestGivenEnablesClassResolution which has Prelude (Num) available.
	source := `
form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

addOne :: \a. Tag a -> a -> a
addOne := \tag x. case tag {
  IntTag => x;
  BoolTag => x
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{
			"Int":  types.TypeOfTypes,
			"Bool": types.TypeOfTypes,
		},
	}
	checkSource(t, source, config)
}

// --- Given + type family: wrong branch type ---

func TestSolverGivenTFTypeMismatchDetected(t *testing.T) {
	// In IntTag branch, F a = Int (via given a ~ Int).
	// Returning a Unit where F a = Int is expected should be rejected.
	source := `
form Unit := { Unit: Unit; }
form Tag := \a. { IntTag: Tag Int; BoolTag: Tag Bool }

type F :: Type := \(a: Type). case a {
  Int => Int;
  Bool => Bool
}

bad :: \a. Tag a -> F a
bad := \tag. case tag {
  IntTag => Unit;
  BoolTag => Unit
}
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{
			"Int":  types.TypeOfTypes,
			"Bool": types.TypeOfTypes,
		},
	}
	errMsg := checkSourceExpectError(t, source, config)
	// Should contain a type mismatch (Unit =/= Int in IntTag branch or Unit =/= Bool in BoolTag).
	if !strings.Contains(errMsg, "mismatch") && !strings.Contains(errMsg, "expected") {
		t.Errorf("expected type mismatch error, got: %s", errMsg)
	}
}
