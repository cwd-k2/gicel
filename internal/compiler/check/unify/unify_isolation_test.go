// Probe-isolation tests — BeginProbeIsolated / EndProbeIsolated.
// Verifies that the trial scope leaves no observable trace on the
// unifier after EndProbeIsolated, even when the trial committed
// solutions, mutated state, or had side-effect callbacks installed.
//
// Does NOT cover: trail-only snapshot/restore (level_unify_test.go),
// trail walks (unify_trail_test.go).
package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TestProbeIsolated_RollbackSoln verifies that solutions written during
// the probe are rolled back on End.
func TestProbeIsolated_RollbackSoln(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}

	tok := u.BeginProbeIsolated()
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Fatalf("trial unify failed: %v", err)
	}
	if u.Solve(1) == nil {
		t.Errorf("expected meta 1 to be solved inside probe scope")
	}
	u.EndProbeIsolated(tok)

	if u.Solve(1) != nil {
		t.Errorf("expected meta 1 to be unsolved after EndProbeIsolated, got %v", u.Solve(1))
	}
}

// TestProbeIsolated_SuspendsSolverLevel verifies that touchability is
// disabled inside the scope and restored on exit.
func TestProbeIsolated_SuspendsSolverLevel(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	u.SolverLevel = 5
	tok := u.BeginProbeIsolated()
	if u.SolverLevel != -1 {
		t.Errorf("expected SolverLevel == -1 inside probe, got %d", u.SolverLevel)
	}
	u.EndProbeIsolated(tok)
	if u.SolverLevel != 5 {
		t.Errorf("expected SolverLevel == 5 restored, got %d", u.SolverLevel)
	}
}

// TestProbeIsolated_NilsCallbacks verifies that OnSolve, FamilyReducer,
// and AliasExpander are nilled inside the scope and restored on exit.
// The "OnSolve nilled" guarantee is the critical safety property: it
// prevents the trial from reaching into the solver's worklist via
// Reactivate, since worklist mutations are not rolled back by Snapshot.
func TestProbeIsolated_NilsCallbacks(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	called := 0
	u.OnSolve = func(int) { called++ }
	famCalled := 0
	u.FamilyReducer = func(t types.Type) types.Type { famCalled++; return t }
	aliasCalled := 0
	u.AliasExpander = func(t types.Type) types.Type { aliasCalled++; return t }

	tok := u.BeginProbeIsolated()
	if u.OnSolve != nil {
		t.Errorf("expected OnSolve == nil inside probe")
	}
	if u.FamilyReducer != nil {
		t.Errorf("expected FamilyReducer == nil inside probe")
	}
	if u.AliasExpander != nil {
		t.Errorf("expected AliasExpander == nil inside probe")
	}

	// Solve a meta inside the probe — this would normally fire OnSolve,
	// which is nilled, so `called` must remain 0.
	m := &types.TyMeta{ID: 7, Kind: types.TypeOfTypes}
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Errorf("expected OnSolve to NOT fire inside probe, called %d times", called)
	}

	u.EndProbeIsolated(tok)

	if u.OnSolve == nil {
		t.Errorf("expected OnSolve restored after EndProbeIsolated")
	}
	if u.FamilyReducer == nil {
		t.Errorf("expected FamilyReducer restored")
	}
	if u.AliasExpander == nil {
		t.Errorf("expected AliasExpander restored")
	}
}

// TestProbeIsolated_FlexSkolemsRoundtrip verifies that an inner mutation
// of FlexSkolems is rolled back on exit. canUnifyWith depends on this:
// it sets FlexSkolems = true inside the scope to allow GADT skolems to
// unify with arbitrary types, and the surrounding code must NOT see
// the flag set after the scope exits.
func TestProbeIsolated_FlexSkolemsRoundtrip(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if u.FlexSkolems {
		t.Fatalf("expected fresh Unifier to have FlexSkolems == false")
	}
	tok := u.BeginProbeIsolated()
	u.FlexSkolems = true
	u.EndProbeIsolated(tok)
	if u.FlexSkolems {
		t.Errorf("expected FlexSkolems restored to false after End, still true")
	}
}

// TestProbeIsolated_NoLeakAfterFailedUnify verifies that even when the
// inner unification FAILS, no partial state remains after End. The
// snapshot must capture the state before any partial soln writes.
func TestProbeIsolated_NoLeakAfterFailedUnify(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}

	tok := u.BeginProbeIsolated()
	// Solve m to Int.
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Fatal(err)
	}
	// Now try to unify Int with Bool — this fails. The earlier solve
	// of m to Int is still in soln; the failure does not auto-rollback.
	_ = u.Unify(types.Con("Int"), types.Con("Bool"))
	u.EndProbeIsolated(tok)

	if u.Solve(1) != nil {
		t.Errorf("expected meta 1 to be unsolved after End despite mid-scope failure")
	}
}

// TestProbeIsolated_NestedScopes verifies that BeginProbeIsolated can
// be nested. Inner scope's End must restore the state captured by the
// inner Begin (NOT the outer Begin), so that the outer scope still sees
// its own modifications until its own End.
func TestProbeIsolated_NestedScopes(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})

	outerTok := u.BeginProbeIsolated()
	m1 := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	if err := u.Unify(m1, types.Con("Int")); err != nil {
		t.Fatal(err)
	}

	innerTok := u.BeginProbeIsolated()
	m2 := &types.TyMeta{ID: 2, Kind: types.TypeOfTypes}
	if err := u.Unify(m2, types.Con("Bool")); err != nil {
		t.Fatal(err)
	}
	u.EndProbeIsolated(innerTok)

	// After inner End: m2 rolled back, m1 still solved (we are inside
	// the outer probe, which has not yet ended).
	if u.Solve(1) == nil {
		t.Errorf("expected m1 still solved after inner End, got nil")
	}
	if u.Solve(2) != nil {
		t.Errorf("expected m2 rolled back by inner End, got %v", u.Solve(2))
	}

	u.EndProbeIsolated(outerTok)

	// After outer End: both rolled back.
	if u.Solve(1) != nil {
		t.Errorf("expected m1 rolled back by outer End, got %v", u.Solve(1))
	}
	if u.Solve(2) != nil {
		t.Errorf("expected m2 still rolled back, got %v", u.Solve(2))
	}
}
