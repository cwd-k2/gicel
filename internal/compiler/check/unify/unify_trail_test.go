// Trail introspection tests — TrailLen and VisitSolnWritesSince.
// Does NOT cover: snapshot/restore (level_unify_test.go covers level trail).
package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TestTrailLenInitial verifies a fresh unifier reports zero trail length.
func TestTrailLenInitial(t *testing.T) {
	u := NewUnifier()
	if got := u.TrailLen(); got != 0 {
		t.Errorf("expected TrailLen() == 0 for fresh Unifier, got %d", got)
	}
}

// TestTrailLenAdvancesOnSolve verifies that solving a meta advances the
// trail position.
func TestTrailLenAdvancesOnSolve(t *testing.T) {
	u := NewUnifier()
	pos0 := u.TrailLen()
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Fatal(err)
	}
	pos1 := u.TrailLen()
	if pos1 <= pos0 {
		t.Errorf("expected TrailLen() to advance after solve, got %d → %d", pos0, pos1)
	}
}

// TestVisitSolnWritesSince_Empty verifies the visitor is a no-op when
// no writes happened since the saved position.
func TestVisitSolnWritesSince_Empty(t *testing.T) {
	u := NewUnifier()
	pos := u.TrailLen()
	called := false
	u.VisitSolnWritesSince(pos, func(metaID int) { called = true })
	if called {
		t.Errorf("expected no callbacks for empty range, got at least one")
	}
}

// TestVisitSolnWritesSince_SingleWrite verifies a single write is reported.
func TestVisitSolnWritesSince_SingleWrite(t *testing.T) {
	u := NewUnifier()
	pos := u.TrailLen()
	m := &types.TyMeta{ID: 42, Kind: types.TypeOfTypes}
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Fatal(err)
	}
	var seen []int
	u.VisitSolnWritesSince(pos, func(metaID int) { seen = append(seen, metaID) })
	if len(seen) != 1 || seen[0] != 42 {
		t.Errorf("expected single visit of meta 42, got %v", seen)
	}
}

// TestVisitSolnWritesSince_ReportsDuplicates verifies the documented
// non-deduping contract: multiple writes to the same meta are reported
// multiple times. Callers whose check is idempotent over the current
// value can ignore duplicates without paying for a dedup-map allocation.
func TestVisitSolnWritesSince_ReportsDuplicates(t *testing.T) {
	u := NewUnifier()
	pos := u.TrailLen()
	// Manually drive the trail by writing the same meta id twice.
	u.trailSolnWrite(7)
	u.soln[7] = types.Con("Int")
	u.trailSolnWrite(7) // overwrite — second trail entry for same id
	u.soln[7] = types.Con("Bool")
	count := 0
	u.VisitSolnWritesSince(pos, func(metaID int) { count++ })
	if count != 2 {
		t.Errorf("expected 2 visits (no dedup), got %d", count)
	}
}

// TestVisitSolnWritesSince_TrailOrder verifies that visits arrive in
// trail (insertion) order. This is the documented contract callers can
// rely on when they need to inspect writes in causal order.
func TestVisitSolnWritesSince_TrailOrder(t *testing.T) {
	u := NewUnifier()
	pos := u.TrailLen()
	writes := []int{1, 2, 3, 1, 2, 3}
	for _, id := range writes {
		u.trailSolnWrite(id)
		u.soln[id] = types.Con("Int")
	}
	var visits []int
	u.VisitSolnWritesSince(pos, func(metaID int) { visits = append(visits, metaID) })
	if len(visits) != len(writes) {
		t.Fatalf("expected %d visits, got %d (visits: %v)", len(writes), len(visits), visits)
	}
	for i, id := range writes {
		if visits[i] != id {
			t.Errorf("visit[%d]: expected meta %d, got %d", i, id, visits[i])
		}
	}
}

// TestVisitSolnWritesSince_IgnoresOtherTrailTags verifies that non-soln
// trail entries (label / skolem / level writes) are skipped.
func TestVisitSolnWritesSince_IgnoresOtherTrailTags(t *testing.T) {
	u := NewUnifier()
	pos := u.TrailLen()
	// Write a label entry — should be invisible to soln visitor.
	u.RegisterLabelContext(99, map[string]struct{}{"a": {}})
	called := false
	u.VisitSolnWritesSince(pos, func(metaID int) { called = true })
	if called {
		t.Errorf("VisitSolnWritesSince saw a non-soln trail entry")
	}
}

// TestVisitSolnWritesSince_PartialRange verifies that the saved position
// correctly excludes earlier writes.
func TestVisitSolnWritesSince_PartialRange(t *testing.T) {
	u := NewUnifier()
	// First batch: should be excluded by the saved pos.
	for _, id := range []int{10, 11} {
		u.trailSolnWrite(id)
		u.soln[id] = types.Con("Int")
	}
	pos := u.TrailLen()
	// Second batch: should be visible.
	for _, id := range []int{20, 21} {
		u.trailSolnWrite(id)
		u.soln[id] = types.Con("Bool")
	}
	var visits []int
	u.VisitSolnWritesSince(pos, func(metaID int) { visits = append(visits, metaID) })
	if len(visits) != 2 || visits[0] != 20 || visits[1] != 21 {
		t.Errorf("expected to see metas [20, 21] in trail order, got %v", visits)
	}
}
