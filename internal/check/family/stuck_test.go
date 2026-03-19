// Stuck family tests — re-activation and rework queue behavior.
// Does NOT cover: reduce.go, verify.go.
package family

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/types"
)

func TestStuckFamily_ReactivationOnMetaSolve(t *testing.T) {
	var idx StuckIndex
	resultMeta := &types.TyMeta{ID: 2, Kind: types.KType{}}

	entry := &stuckEntry{
		familyName: "Elem",
		args:       []types.Type{&types.TyMeta{ID: 1, Kind: types.KType{}}},
		resultMeta: resultMeta,
		blockingOn: []int{1},
	}
	idx.register(entry)

	if idx.hasRework() {
		t.Fatal("rework queue should be empty before reactivation")
	}

	idx.Reactivate(1)

	if !idx.hasRework() {
		t.Fatal("rework queue should be non-empty after reactivation")
	}

	entries := idx.drainRework()
	if len(entries) != 1 {
		t.Fatalf("expected 1 rework entry, got %d", len(entries))
	}
	if entries[0].familyName != "Elem" {
		t.Errorf("expected family name Elem, got %s", entries[0].familyName)
	}
	if entries[0].resultMeta.ID != 2 {
		t.Errorf("expected result meta ID 2, got %d", entries[0].resultMeta.ID)
	}

	if idx.hasRework() {
		t.Fatal("rework queue should be empty after drain")
	}
}

func TestStuckFamily_ReactivateUnrelatedMeta(t *testing.T) {
	var idx StuckIndex

	entry := &stuckEntry{
		familyName: "F",
		args:       []types.Type{&types.TyMeta{ID: 1, Kind: types.KType{}}},
		resultMeta: &types.TyMeta{ID: 2, Kind: types.KType{}},
		blockingOn: []int{1},
	}
	idx.register(entry)

	// Reactivating an unrelated meta should not produce rework.
	idx.Reactivate(99)

	if idx.hasRework() {
		t.Fatal("reactivating unrelated meta should not produce rework")
	}
}

func TestStuckFamily_MultipleBlockingMetas(t *testing.T) {
	var idx StuckIndex

	entry := &stuckEntry{
		familyName: "G",
		args:       []types.Type{&types.TyMeta{ID: 1, Kind: types.KType{}}, &types.TyMeta{ID: 3, Kind: types.KType{}}},
		resultMeta: &types.TyMeta{ID: 4, Kind: types.KType{}},
		blockingOn: []int{1, 3},
	}
	idx.register(entry)

	// Solving either blocking meta should trigger rework.
	idx.Reactivate(1)
	if !idx.hasRework() {
		t.Fatal("expected rework after solving first blocking meta")
	}
	entries := idx.drainRework()
	if len(entries) != 1 || entries[0].familyName != "G" {
		t.Fatalf("unexpected rework entries: %v", entries)
	}

	// The entry was also registered under meta 3; reactivating it
	// should produce rework again.
	idx.Reactivate(3)
	if !idx.hasRework() {
		t.Fatal("expected rework after solving second blocking meta")
	}
	idx.drainRework()
}

func TestStuckFamily_SnapshotRestore(t *testing.T) {
	var idx StuckIndex

	entry := &stuckEntry{
		familyName: "H",
		args:       []types.Type{&types.TyMeta{ID: 5, Kind: types.KType{}}},
		resultMeta: &types.TyMeta{ID: 6, Kind: types.KType{}},
		blockingOn: []int{5},
	}

	snap := idx.Snapshot()
	idx.register(entry)
	idx.Reactivate(5)

	if !idx.hasRework() {
		t.Fatal("expected rework before restore")
	}

	idx.Restore(snap)

	if idx.hasRework() {
		t.Fatal("rework queue should be empty after restore to pre-registration snapshot")
	}
}
