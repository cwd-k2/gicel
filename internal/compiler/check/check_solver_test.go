// Solver loop tests — solveWanteds, processCtClass, processCtFunEq.
// Does NOT cover: deferred.go (old path).
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TestSolverSingleConstraint verifies that a single Num Int constraint
// is resolved by the solver with a valid Core expression.
func TestSolverSingleConstraint(t *testing.T) {
	ch := setupCheckerWithPrelude(t)

	ch.solver.Emit(&CtPlainClass{
		Placeholder: "$dict_1",
		ClassName:   "Num",
		Args:        []types.Type{&types.TyCon{Name: "Int"}},
	})

	resolutions, residuals := ch.solveWanteds(nil)
	if len(residuals) != 0 {
		t.Fatalf("expected 0 residuals, got %d", len(residuals))
	}
	expr, ok := resolutions["$dict_1"]
	if !ok {
		t.Fatal("expected resolution for $dict_1")
	}
	if expr == nil {
		t.Fatal("resolution should not be nil")
	}
}

// TestSolverDuplicateConstraints verifies that identical constraints
// share the same resolution via the inert set.
func TestSolverDuplicateConstraints(t *testing.T) {
	ch := setupCheckerWithPrelude(t)

	for i := range 5 {
		ch.solver.Emit(&CtPlainClass{
			Placeholder: placeholderName(i),
			ClassName:   "Num",
			Args:        []types.Type{&types.TyCon{Name: "Int"}},
		})
	}

	resolutions, residuals := ch.solveWanteds(nil)
	if len(residuals) != 0 {
		t.Fatalf("expected 0 residuals, got %d", len(residuals))
	}
	if len(resolutions) != 5 {
		t.Fatalf("expected 5 resolutions, got %d", len(resolutions))
	}
	// All resolutions should be equal (same dictionary expression).
	first := resolutions[placeholderName(0)]
	for i := 1; i < 5; i++ {
		if resolutions[placeholderName(i)] != first {
			t.Fatalf("expected identical resolution for constraint %d", i)
		}
	}
}

// TestSolverDeferrable verifies that constraints with unsolved metas
// are returned as residuals when a shouldDefer predicate is provided.
func TestSolverDeferrable(t *testing.T) {
	ch := setupCheckerWithPrelude(t)

	meta := ch.freshMeta(types.TypeOfTypes)
	ch.solver.Emit(&CtPlainClass{
		Placeholder: "$dict_1",
		ClassName:   "Num",
		Args:        []types.Type{meta},
	})

	resolutions, residuals := ch.solveWanteds(func(className string, zonkedArgs []types.Type) bool {
		return sliceHasMeta(zonkedArgs)
	})
	if len(resolutions) != 0 {
		t.Fatalf("expected 0 resolutions (deferred), got %d", len(resolutions))
	}
	if len(residuals) != 1 {
		t.Fatalf("expected 1 residual, got %d", len(residuals))
	}
	if residuals[0].ClassName != "Num" {
		t.Fatalf("expected residual class Num, got %s", residuals[0].ClassName)
	}
}

// TestSolverMixedConstraints verifies that resolved and deferred constraints
// are handled correctly in the same worklist.
func TestSolverMixedConstraints(t *testing.T) {
	ch := setupCheckerWithPrelude(t)

	meta := ch.freshMeta(types.TypeOfTypes)
	// Resolved: Eq Int
	ch.solver.Emit(&CtPlainClass{
		Placeholder: "$dict_1",
		ClassName:   "Eq",
		Args:        []types.Type{&types.TyCon{Name: "Int"}},
	})
	// Deferred: Num ?meta
	ch.solver.Emit(&CtPlainClass{
		Placeholder: "$dict_2",
		ClassName:   "Num",
		Args:        []types.Type{meta},
	})

	resolutions, residuals := ch.solveWanteds(func(className string, zonkedArgs []types.Type) bool {
		return sliceHasMeta(zonkedArgs)
	})
	if len(resolutions) != 1 {
		t.Fatalf("expected 1 resolution, got %d", len(resolutions))
	}
	if _, ok := resolutions["$dict_1"]; !ok {
		t.Fatal("expected Eq Int to be resolved")
	}
	if len(residuals) != 1 {
		t.Fatalf("expected 1 residual, got %d", len(residuals))
	}
}

// TestSolverEmptyWorklist verifies that an empty worklist produces no results.
func TestSolverEmptyWorklist(t *testing.T) {
	ch := setupCheckerWithPrelude(t)

	resolutions, residuals := ch.solveWanteds(nil)
	if len(resolutions) != 0 || len(residuals) != 0 {
		t.Fatal("expected empty results for empty worklist")
	}
}

func placeholderName(i int) string {
	return "$dict_" + string(rune('1'+i))
}
