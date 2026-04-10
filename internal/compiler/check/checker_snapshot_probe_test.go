//go:build probe

// Snapshot/Restore probe tests — solution rollback, label context rollback, kind solution rollback.
// Does NOT cover: check_solver_test.go, check_solver_implication_test.go, check_solver_given_test.go.
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Snapshot/Restore probe tests — solution rollback, label context rollback,
// kind solution rollback, nested snapshot/restore, and prior solution
// preservation.
// =============================================================================

// =====================================================================
// From probe_d: Snapshot/Restore correctness
// =====================================================================

// TestProbeD_SnapshotRestore_SolutionRollback — unify a meta, snapshot,
// unify another meta, restore, verify second meta is unsolved.
func TestProbeD_SnapshotRestore_SolutionRollback(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	m2 := &types.TyMeta{ID: 2, Kind: types.TypeOfTypes}

	// Solve m1 = Int
	if err := u.Unify(m1, types.Con("Int")); err != nil {
		t.Fatal(err)
	}

	snap := u.Snapshot()

	// Solve m2 = Bool (after snapshot)
	if err := u.Unify(m2, types.Con("Bool")); err != nil {
		t.Fatal(err)
	}
	// Verify m2 is solved
	if _, ok := u.Zonk(m2).(*types.TyCon); !ok {
		t.Fatal("m2 should be solved before restore")
	}

	u.Restore(snap)

	// m1 should still be solved (it was before snapshot)
	z1 := u.Zonk(m1)
	if con, ok := z1.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("m1 should still be Int after restore, got %s", types.Pretty(z1))
	}

	// m2 should be unsolved (it was solved after snapshot)
	z2 := u.Zonk(m2)
	if _, ok := z2.(*types.TyMeta); !ok {
		t.Errorf("m2 should be unsolved after restore, got %s", types.Pretty(z2))
	}
}

// TestProbeD_SnapshotRestore_LabelContextRollback — label context registered
// after snapshot should be rolled back.
func TestProbeD_SnapshotRestore_LabelContextRollback(t *testing.T) {
	u := unify.NewUnifier()

	snap := u.Snapshot()

	// Register labels after snapshot
	u.RegisterLabelContext(42, map[string]struct{}{"x": {}, "y": {}})
	if len(u.Labels()[42]) != 2 {
		t.Fatal("expected 2 labels before restore")
	}

	u.Restore(snap)

	// Labels for meta 42 should be gone
	if _, exists := u.Labels()[42]; exists {
		t.Error("label context for meta 42 should be removed after restore")
	}
}

// TestProbeD_SnapshotRestore_KindSolutionRollback — kind meta solutions
// after snapshot should be rolled back.
func TestProbeD_SnapshotRestore_KindSolutionRollback(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.TyMeta{ID: 10, Kind: types.SortZero}

	snap := u.Snapshot()

	if err := u.Unify(km, types.TypeOfTypes); err != nil {
		t.Fatal(err)
	}
	// Verify solved
	zonked := u.Zonk(km)
	if !types.Equal(zonked, types.TypeOfTypes) {
		t.Fatal("kind meta should be solved before restore")
	}

	u.Restore(snap)

	// Kind meta should be unsolved
	zonked = u.Zonk(km)
	if m, ok := zonked.(*types.TyMeta); !ok || m.ID != 10 {
		t.Errorf("kind meta should be unsolved after restore, got %s", types.PrettyTypeAsKind(zonked))
	}
}

// TestProbeD_SnapshotRestore_MultipleSnapshotsNested — nested snapshot/restore
// should work correctly (restore inner, then outer).
func TestProbeD_SnapshotRestore_MultipleSnapshotsNested(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	m2 := &types.TyMeta{ID: 2, Kind: types.TypeOfTypes}
	m3 := &types.TyMeta{ID: 3, Kind: types.TypeOfTypes}

	if err := u.Unify(m1, types.Con("A")); err != nil {
		t.Fatal(err)
	}
	snap1 := u.Snapshot()

	if err := u.Unify(m2, types.Con("B")); err != nil {
		t.Fatal(err)
	}
	snap2 := u.Snapshot()

	if err := u.Unify(m3, types.Con("C")); err != nil {
		t.Fatal(err)
	}

	// Restore inner: m3 unsolved, m1 and m2 solved
	u.Restore(snap2)
	if _, ok := u.Zonk(m3).(*types.TyMeta); !ok {
		t.Error("m3 should be unsolved after restoring snap2")
	}
	if con, ok := u.Zonk(m2).(*types.TyCon); !ok || con.Name != "B" {
		t.Error("m2 should still be B after restoring snap2")
	}

	// Restore outer: m2 and m3 unsolved, m1 solved
	u.Restore(snap1)
	if _, ok := u.Zonk(m2).(*types.TyMeta); !ok {
		t.Error("m2 should be unsolved after restoring snap1")
	}
	if con, ok := u.Zonk(m1).(*types.TyCon); !ok || con.Name != "A" {
		t.Error("m1 should still be A after restoring snap1")
	}
}

// =====================================================================
// From probe_e: Snapshot/Restore correctness
// =====================================================================

// TestProbeE_Snapshot_RestoreUndoesSolution — solutions added after snapshot
// should be undone on restore.
func TestProbeE_Snapshot_RestoreUndoesSolution(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	snap := u.Snapshot()
	// Solve m = Int
	u.Unify(m, types.Con("Int"))
	if soln := u.Solve(1); soln == nil {
		t.Fatal("expected m solved after unify")
	}
	// Restore
	u.Restore(snap)
	if soln := u.Solve(1); soln != nil {
		t.Fatal("expected m unsolved after restore")
	}
}

// TestProbeE_Snapshot_PreservesPriorSolutions — solutions before snapshot
// should survive restore.
func TestProbeE_Snapshot_PreservesPriorSolutions(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	m2 := &types.TyMeta{ID: 2, Kind: types.TypeOfTypes}
	u.Unify(m1, types.Con("Int"))
	snap := u.Snapshot()
	u.Unify(m2, types.Con("Bool"))
	u.Restore(snap)
	// m1 should still be solved
	if soln := u.Solve(1); soln == nil {
		t.Fatal("m1 should remain solved after restore")
	}
	// m2 should be unsolved
	if soln := u.Solve(2); soln != nil {
		t.Fatal("m2 should be unsolved after restore")
	}
}

// TestProbeE_Snapshot_KindSolutionRestore — kind solutions should also be
// rolled back on restore.
func TestProbeE_Snapshot_KindSolutionRestore(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.TyMeta{ID: 1, Kind: types.SortZero}
	snap := u.Snapshot()
	u.Unify(km, types.TypeOfTypes)
	if types.Equal(u.Zonk(km), km) {
		t.Fatal("expected km solved")
	}
	u.Restore(snap)
	if !types.Equal(u.Zonk(km), km) {
		t.Fatal("expected km unsolved after restore")
	}
}
