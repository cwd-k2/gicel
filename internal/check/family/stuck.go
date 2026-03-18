package family

import (
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// stuckEntry records a type family application that could not reduce
// because one or more arguments contain unsolved metavariables.
type stuckEntry struct {
	familyName string
	args       []types.Type
	span       span.Span
	resultMeta *types.TyMeta // placeholder substituted for the stuck app
	blockingOn []int         // meta IDs that blocked reduction
}

// StuckIndex manages stuck type family applications and their
// re-activation when blocking metavariables are solved.
type StuckIndex struct {
	entries map[int][]*stuckEntry
	rework  []*stuckEntry
}

func (idx *StuckIndex) register(entry *stuckEntry) {
	if idx.entries == nil {
		idx.entries = make(map[int][]*stuckEntry)
	}
	for _, metaID := range entry.blockingOn {
		idx.entries[metaID] = append(idx.entries[metaID], entry)
	}
}

// Reactivate marks stuck entries blocking on the given meta ID for rework.
func (idx *StuckIndex) Reactivate(metaID int) {
	if entries, ok := idx.entries[metaID]; ok {
		idx.rework = append(idx.rework, entries...)
		delete(idx.entries, metaID)
	}
}

func (idx *StuckIndex) drainRework() []*stuckEntry {
	if len(idx.rework) == 0 {
		return nil
	}
	entries := idx.rework
	idx.rework = nil
	return entries
}

func (idx *StuckIndex) hasRework() bool {
	return len(idx.rework) > 0
}

// StuckSnapshot captures the stuck index state for rollback.
type StuckSnapshot struct {
	entries   map[int][]*stuckEntry
	reworkLen int
}

// Snapshot saves the current stuck index state.
func (idx *StuckIndex) Snapshot() StuckSnapshot {
	entriesCopy := make(map[int][]*stuckEntry, len(idx.entries))
	for k, v := range idx.entries {
		entriesCopy[k] = append([]*stuckEntry(nil), v...)
	}
	return StuckSnapshot{
		entries:   entriesCopy,
		reworkLen: len(idx.rework),
	}
}

// Restore rolls back the stuck index to a previously saved snapshot.
func (idx *StuckIndex) Restore(snap StuckSnapshot) {
	for k := range idx.entries {
		if _, existed := snap.entries[k]; !existed {
			delete(idx.entries, k)
		}
	}
	for k, v := range snap.entries {
		idx.entries[k] = v
	}
	if snap.reworkLen < len(idx.rework) {
		idx.rework = idx.rework[:snap.reworkLen]
	}
}
