// Level unification tests — LevelMeta solving, occurs check, zonking.
// Does NOT cover: cumulativity (kind_unify_cumulativity_test.go).
package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Literal level unification
// =============================================================================

func TestUnifyLevelsLiteral(t *testing.T) {
	u := NewUnifier()
	if err := u.unifyLevels(types.L0, types.L0); err != nil {
		t.Errorf("L0 = L0 should succeed: %v", err)
	}
	if err := u.unifyLevels(types.L1, types.L1); err != nil {
		t.Errorf("L1 = L1 should succeed: %v", err)
	}
	if err := u.unifyLevels(types.L0, types.L1); err == nil {
		t.Error("L0 = L1 should fail")
	}
}

func TestUnifyLevelsNilIsL0(t *testing.T) {
	u := NewUnifier()
	if err := u.unifyLevels(nil, types.L0); err != nil {
		t.Errorf("nil = L0 should succeed: %v", err)
	}
	if err := u.unifyLevels(types.L0, nil); err != nil {
		t.Errorf("L0 = nil should succeed: %v", err)
	}
}

// =============================================================================
// Level metavariable solving
// =============================================================================

func TestUnifyLevelsMeta(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	if err := u.unifyLevels(m, types.L1); err != nil {
		t.Errorf("?l1 = L1 should succeed: %v", err)
	}
	solved := u.zonkLevel(m)
	if !types.LevelEqual(solved, types.L1) {
		t.Errorf("expected L1, got %s", solved.LevelString())
	}
}

func TestUnifyLevelsMetaReflexive(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	if err := u.unifyLevels(m, m); err != nil {
		t.Errorf("?l1 = ?l1 should succeed: %v", err)
	}
}

func TestUnifyLevelsMetaChain(t *testing.T) {
	u := NewUnifier()
	m1 := &types.LevelMeta{ID: 1}
	m2 := &types.LevelMeta{ID: 2}
	if err := u.unifyLevels(m1, m2); err != nil {
		t.Fatalf("?l1 = ?l2 should succeed: %v", err)
	}
	if err := u.unifyLevels(m2, types.L2); err != nil {
		t.Fatalf("?l2 = L2 should succeed: %v", err)
	}
	solved := u.zonkLevel(m1)
	if !types.LevelEqual(solved, types.L2) {
		t.Errorf("expected L2, got %s", solved.LevelString())
	}
}

func TestUnifyLevelsMetaRHS(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	if err := u.unifyLevels(types.L0, m); err != nil {
		t.Errorf("L0 = ?l1 should succeed: %v", err)
	}
	solved := u.zonkLevel(m)
	if !types.LevelEqual(solved, types.L0) {
		t.Errorf("expected L0, got %s", solved.LevelString())
	}
}

// =============================================================================
// Occurs check
// =============================================================================

func TestUnifyLevelsOccursCheck(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	cycle := &types.LevelSucc{E: m}
	err := u.unifyLevels(m, cycle)
	if err == nil {
		t.Error("should detect occurs check")
	}
	ue, ok := err.(*UnifyError)
	if !ok || ue.Kind != UnifyOccursCheck {
		t.Errorf("expected UnifyOccursCheck, got %v", err)
	}
}

func TestUnifyLevelsOccursCheckMax(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	cycle := &types.LevelMax{A: types.L0, B: m}
	err := u.unifyLevels(m, cycle)
	if err == nil {
		t.Error("should detect occurs check in max")
	}
}

// =============================================================================
// Structural level unification (LevelVar, LevelMax, LevelSucc)
// =============================================================================

func TestUnifyLevelsVar(t *testing.T) {
	u := NewUnifier()
	v1 := &types.LevelVar{Name: "l"}
	v2 := &types.LevelVar{Name: "l"}
	if err := u.unifyLevels(v1, v2); err != nil {
		t.Errorf("l = l should succeed: %v", err)
	}
	v3 := &types.LevelVar{Name: "k"}
	if err := u.unifyLevels(v1, v3); err == nil {
		t.Error("l = k should fail")
	}
}

func TestUnifyLevelsMax(t *testing.T) {
	u := NewUnifier()
	a := &types.LevelMax{A: types.L0, B: types.L1}
	b := &types.LevelMax{A: types.L0, B: types.L1}
	if err := u.unifyLevels(a, b); err != nil {
		t.Errorf("max(0,1) = max(0,1) should succeed: %v", err)
	}
}

func TestUnifyLevelsMaxMismatch(t *testing.T) {
	u := NewUnifier()
	a := &types.LevelMax{A: types.L0, B: types.L1}
	b := &types.LevelMax{A: types.L1, B: types.L0}
	if err := u.unifyLevels(a, b); err == nil {
		t.Error("max(0,1) = max(1,0) should fail (structural)")
	}
}

func TestUnifyLevelsSucc(t *testing.T) {
	u := NewUnifier()
	a := &types.LevelSucc{E: types.L0}
	b := &types.LevelSucc{E: types.L0}
	if err := u.unifyLevels(a, b); err != nil {
		t.Errorf("succ(0) = succ(0) should succeed: %v", err)
	}
}

func TestUnifyLevelsMaxWithMeta(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	a := &types.LevelMax{A: m, B: types.L1}
	b := &types.LevelMax{A: types.L0, B: types.L1}
	if err := u.unifyLevels(a, b); err != nil {
		t.Errorf("max(?l1, 1) = max(0, 1) should succeed: %v", err)
	}
	solved := u.zonkLevel(m)
	if !types.LevelEqual(solved, types.L0) {
		t.Errorf("expected L0, got %s", solved.LevelString())
	}
}

// =============================================================================
// TyCon with LevelMeta — integration with Unify
// =============================================================================

func TestUnifyTyConWithLevelMeta(t *testing.T) {
	u := NewUnifier()
	lm := &types.LevelMeta{ID: 1}
	a := &types.TyCon{Name: "Type", Level: lm}
	b := types.TypeOfTypes // TyCon{Name: "Type", Level: L1}
	if err := u.Unify(a, b); err != nil {
		t.Errorf("TyCon with LevelMeta should unify: %v", err)
	}
	solved := u.zonkLevel(lm)
	if !types.LevelEqual(solved, types.L1) {
		t.Errorf("expected L1, got %s", solved.LevelString())
	}
}

func TestUnifyTyConLevelMetaBothSides(t *testing.T) {
	u := NewUnifier()
	lm1 := &types.LevelMeta{ID: 1}
	lm2 := &types.LevelMeta{ID: 2}
	a := &types.TyCon{Name: "Type", Level: lm1}
	b := &types.TyCon{Name: "Type", Level: lm2}
	if err := u.Unify(a, b); err != nil {
		t.Errorf("TyCon with two LevelMetas should unify: %v", err)
	}
	// Solve one; the other should follow.
	if err := u.unifyLevels(lm2, types.L1); err != nil {
		t.Fatalf("?l2 = L1 should succeed: %v", err)
	}
	solved := u.zonkLevel(lm1)
	if !types.LevelEqual(solved, types.L1) {
		t.Errorf("expected L1 via chain, got %s", solved.LevelString())
	}
}

func TestUnifyTyConNameMismatchIgnoresLevel(t *testing.T) {
	u := NewUnifier()
	lm := &types.LevelMeta{ID: 1}
	a := &types.TyCon{Name: "Type", Level: lm}
	b := &types.TyCon{Name: "Row", Level: types.L1}
	if err := u.Unify(a, b); err == nil {
		t.Error("Type vs Row should fail even with LevelMeta")
	}
}

// =============================================================================
// Zonk TyCon level
// =============================================================================

func TestZonkTyConLevel(t *testing.T) {
	u := NewUnifier()
	lm := &types.LevelMeta{ID: 1}
	if err := u.unifyLevels(lm, types.L1); err != nil {
		t.Fatalf("solve ?l1 = L1: %v", err)
	}
	tc := &types.TyCon{Name: "Type", Level: lm}
	zonked := u.Zonk(tc)
	tc2, ok := zonked.(*types.TyCon)
	if !ok {
		t.Fatalf("expected TyCon, got %T", zonked)
	}
	if !types.LevelEqual(tc2.Level, types.L1) {
		t.Errorf("expected level L1, got %s", tc2.Level.LevelString())
	}
}

func TestZonkTyConLevelUnsolved(t *testing.T) {
	u := NewUnifier()
	lm := &types.LevelMeta{ID: 1}
	tc := &types.TyCon{Name: "Type", Level: lm}
	zonked := u.Zonk(tc)
	// Unsolved meta: TyCon should be returned as-is.
	if zonked != tc {
		t.Error("unsolved level meta should return TyCon unchanged")
	}
}

func TestZonkTyConNilLevel(t *testing.T) {
	u := NewUnifier()
	tc := &types.TyCon{Name: "Int"}
	zonked := u.Zonk(tc)
	if zonked != tc {
		t.Error("nil level TyCon should return unchanged")
	}
}

// =============================================================================
// ZonkLevelDefault
// =============================================================================

func TestZonkLevelDefaultUnsolved(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	result := u.ZonkLevelDefault(m)
	if !types.LevelEqual(result, types.L0) {
		t.Errorf("unsolved level meta should default to L0, got %s", result.LevelString())
	}
}

func TestZonkLevelDefaultSolved(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	u.levelSoln[1] = types.L2
	result := u.ZonkLevelDefault(m)
	if !types.LevelEqual(result, types.L2) {
		t.Errorf("solved level meta should return L2, got %s", result.LevelString())
	}
}

func TestZonkLevelDefaultNil(t *testing.T) {
	u := NewUnifier()
	result := u.ZonkLevelDefault(nil)
	if !types.LevelEqual(result, types.L0) {
		t.Errorf("nil should default to L0, got %s", result.LevelString())
	}
}

// =============================================================================
// Trail / Snapshot / Restore
// =============================================================================

func TestLevelSolnRestore(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	snap := u.Snapshot()
	if err := u.unifyLevels(m, types.L1); err != nil {
		t.Fatalf("solve ?l1 = L1: %v", err)
	}
	// Verify solved.
	if !types.LevelEqual(u.zonkLevel(m), types.L1) {
		t.Fatal("expected L1 after solve")
	}
	// Restore.
	u.Restore(snap)
	// Should be unsolved again.
	result := u.zonkLevel(m)
	if _, ok := result.(*types.LevelMeta); !ok {
		t.Errorf("expected unsolved LevelMeta after restore, got %s", result.LevelString())
	}
}

func TestLevelSolnRestoreOverwrite(t *testing.T) {
	u := NewUnifier()
	m := &types.LevelMeta{ID: 1}
	// Solve to L0 first.
	if err := u.unifyLevels(m, types.L0); err != nil {
		t.Fatalf("solve ?l1 = L0: %v", err)
	}
	snap := u.Snapshot()
	// Overwrite.
	u.trailLevelWrite(m.ID)
	u.levelSoln[m.ID] = types.L2
	if !types.LevelEqual(u.zonkLevel(m), types.L2) {
		t.Fatal("expected L2 after overwrite")
	}
	// Restore should bring back L0.
	u.Restore(snap)
	if !types.LevelEqual(u.zonkLevel(m), types.L0) {
		t.Errorf("expected L0 after restore, got %s", u.zonkLevel(m).LevelString())
	}
}
