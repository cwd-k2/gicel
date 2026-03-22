// Solver implication tests — processCtImplication, constraintMentionsLocal.
// Does NOT cover: solver_test.go (plain CtClass/CtFunEq).
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TestImplicationEmptyWanteds verifies that an implication with no inner
// constraints is a no-op.
func TestImplicationEmptyWanteds(t *testing.T) {
	ch := setupCheckerWithPrelude(t)

	ch.solver.Emit(&CtImplication{
		Wanteds: nil,
	})

	resolutions, residuals := ch.solveWanteds(nil)
	if len(resolutions) != 0 {
		t.Errorf("expected 0 resolutions, got %d", len(resolutions))
	}
	if len(residuals) != 0 {
		t.Errorf("expected 0 residuals, got %d", len(residuals))
	}
}

// TestImplicationResolvableWanteds verifies that inner wanteds are
// resolved and their resolutions propagated to the outer scope.
func TestImplicationResolvableWanteds(t *testing.T) {
	ch := setupCheckerWithPrelude(t)

	ch.solver.Emit(&CtImplication{
		Wanteds: []Ct{
			&CtClass{
				Placeholder: "$dict_inner_1",
				ClassName:   "Num",
				Args:        []types.Type{&types.TyCon{Name: "Int"}},
			},
		},
	})

	resolutions, residuals := ch.solveWanteds(nil)
	if len(residuals) != 0 {
		t.Fatalf("expected 0 residuals, got %d", len(residuals))
	}
	if _, ok := resolutions["$dict_inner_1"]; !ok {
		t.Fatal("expected inner resolution to propagate to outer resolutions")
	}
}

// TestImplicationStuckLocalSkolem verifies that a residual constraint
// mentioning a local skolem produces an error (cannot float).
func TestImplicationStuckLocalSkolem(t *testing.T) {
	ch := setupCheckerWithPrelude(t)
	sk := &types.TySkolem{ID: 999, Name: "a", Kind: types.KType{}}

	ch.solver.Emit(&CtImplication{
		Skolems: []*types.TySkolem{sk},
		Wanteds: []Ct{
			&CtClass{
				Placeholder: "$dict_stuck",
				ClassName:   "Num",
				Args:        []types.Type{sk},
			},
		},
	})

	ch.solveWanteds(nil)
	// Instance resolution fails for Num skolem (no matching instance).
	// The error is emitted by resolveInstance, not by partitioning.
	if ch.errors.Len() == 0 {
		t.Fatal("expected error for unresolvable constraint with local skolem")
	}
}

// TestImplicationFloatableResidual verifies that a residual constraint
// mentioning only outer-level metas is floated back to the outer worklist.
func TestImplicationFloatableResidual(t *testing.T) {
	ch := setupCheckerWithPrelude(t)

	// Outer meta at level 0 — should be floatable.
	outerMeta := &types.TyMeta{ID: ch.fresh(), Kind: types.KType{}, Level: 0}

	// Emit an outer constraint first to verify it's preserved.
	ch.solver.Emit(&CtClass{
		Placeholder: "$dict_outer",
		ClassName:   "Eq",
		Args:        []types.Type{&types.TyCon{Name: "Int"}},
	})

	ch.solver.Emit(&CtImplication{
		Wanteds: []Ct{
			&CtClass{
				Placeholder: "$dict_float",
				ClassName:   "Num",
				Args:        []types.Type{outerMeta},
			},
		},
	})

	// Use shouldDefer to capture the floated constraint as a residual.
	resolutions, residuals := ch.solveWanteds(func(className string, zonkedArgs []types.Type) bool {
		return sliceHasMeta(zonkedArgs)
	})

	// Outer Eq Int should be resolved.
	if _, ok := resolutions["$dict_outer"]; !ok {
		t.Error("expected outer Eq Int to be resolved")
	}
	// Floated Num ?meta should appear as residual.
	found := false
	for _, r := range residuals {
		if r.Placeholder == "$dict_float" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected floated constraint to appear as residual")
	}
}
