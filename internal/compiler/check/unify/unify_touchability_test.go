// Touchability tests — SolverLevel guard in solveMeta.
// Does NOT cover: unify_constraint_test.go, unify_evidence_test.go.
package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TestSolverLevelDisabledAllowsSolve verifies that SolverLevel = -1
// (disabled) allows solving any meta regardless of its level.
func TestSolverLevelDisabledAllowsSolve(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	// SolverLevel is -1 by default.
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes, Level: 0}
	if err := u.Unify(m, testOps.Con("Int")); err != nil {
		t.Fatalf("SolverLevel=-1 should allow solving level-0 meta: %v", err)
	}
	soln := u.Zonk(m)
	if !testOps.Equal(soln, testOps.Con("Int")) {
		t.Errorf("expected Int, got %s", testOps.Pretty(soln))
	}
}

// TestSolverLevelBlocksOuterMeta verifies that a meta at level 0
// cannot be solved when SolverLevel = 1 (inner implication scope).
func TestSolverLevelBlocksOuterMeta(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	u.SolverLevel = 1
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes, Level: 0}
	err := u.Unify(m, testOps.Con("Int"))
	if err == nil {
		t.Fatal("expected UnifyUntouchable error, got nil")
	}
	ue, ok := err.(UnifyError)
	if !ok {
		t.Fatalf("expected UnifyError, got %T", err)
	}
	if ue.Kind() != UnifyUntouchable {
		t.Errorf("expected UnifyUntouchable, got %v", ue.Kind())
	}
}

// TestSolverLevelAllowsSameLevelMeta verifies that a meta at level 1
// can be solved when SolverLevel = 1.
func TestSolverLevelAllowsSameLevelMeta(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	u.SolverLevel = 1
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes, Level: 1}
	if err := u.Unify(m, testOps.Con("Bool")); err != nil {
		t.Fatalf("same-level meta should be solvable: %v", err)
	}
	soln := u.Zonk(m)
	if !testOps.Equal(soln, testOps.Con("Bool")) {
		t.Errorf("expected Bool, got %s", testOps.Pretty(soln))
	}
}

// TestSolverLevelZeroAllowsLevelZeroMeta verifies that SolverLevel = 0
// allows solving a level-0 meta (touchable at top level).
func TestSolverLevelZeroAllowsLevelZeroMeta(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	u.SolverLevel = 0
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes, Level: 0}
	if err := u.Unify(m, testOps.Con("String")); err != nil {
		t.Fatalf("level-0 meta at SolverLevel=0 should be solvable: %v", err)
	}
	soln := u.Zonk(m)
	if !testOps.Equal(soln, testOps.Con("String")) {
		t.Errorf("expected String, got %s", testOps.Pretty(soln))
	}
}

// TestSolverLevelBlocksRHS verifies untouchable check applies symmetrically
// when the meta is on the RHS of unification.
func TestSolverLevelBlocksRHS(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	u.SolverLevel = 2
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes, Level: 0}
	err := u.Unify(testOps.Con("Int"), m)
	if err == nil {
		t.Fatal("expected UnifyUntouchable error for RHS meta")
	}
	ue, ok := err.(UnifyError)
	if !ok {
		t.Fatalf("expected UnifyError, got %T", err)
	}
	if ue.Kind() != UnifyUntouchable {
		t.Errorf("expected UnifyUntouchable, got %v", ue.Kind())
	}
}

// TestSolverLevelAllowsHigherLevelMeta verifies that a meta at level 2
// can be solved when SolverLevel = 1 (meta is "inner" relative to solver).
func TestSolverLevelAllowsHigherLevelMeta(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	u.SolverLevel = 1
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes, Level: 2}
	if err := u.Unify(m, testOps.Con("Rune")); err != nil {
		t.Fatalf("higher-level meta should be solvable: %v", err)
	}
}
