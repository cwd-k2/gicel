package engine

import (
	"sort"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TypeIndex records the type assigned to each expression span during
// type checking. Entries are stored in sorted order for efficient
// positional queries.
type TypeIndex struct {
	entries []typeEntry
}

type typeEntry struct {
	span span.Span
	ty   types.Type
}

// NewTypeIndex creates an empty TypeIndex.
func NewTypeIndex() *TypeIndex {
	return &TypeIndex{}
}

// Record adds a span→type mapping. Entries with zero spans or
// TyError types are silently discarded.
func (idx *TypeIndex) Record(sp span.Span, ty types.Type) {
	if sp == (span.Span{}) {
		return
	}
	if _, ok := ty.(*types.TyError); ok {
		return
	}
	idx.entries = append(idx.entries, typeEntry{span: sp, ty: ty})
}

// Finalize sorts entries for positional queries. Must be called
// once after all Record calls and before any TypeAt calls.
func (idx *TypeIndex) Finalize() {
	sort.Slice(idx.entries, func(i, j int) bool {
		a, b := idx.entries[i].span, idx.entries[j].span
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		// Same start: narrower span first (more specific).
		return (a.End - a.Start) < (b.End - b.Start)
	})
}

// Len returns the number of recorded entries.
func (idx *TypeIndex) Len() int { return len(idx.entries) }

// TypeAt returns the type of the innermost expression whose span
// contains the given byte offset. Returns nil if no span matches.
func (idx *TypeIndex) TypeAt(pos span.Pos) types.Type {
	// Binary search for the first entry with Start > pos.
	hi := sort.Search(len(idx.entries), func(i int) bool {
		return idx.entries[i].span.Start > pos
	})
	// Scan backwards for the tightest enclosing span.
	var best *typeEntry
	for i := hi - 1; i >= 0; i-- {
		e := &idx.entries[i]
		if e.span.End <= pos {
			continue
		}
		// e.span contains pos.
		if best == nil || (e.span.End-e.span.Start) < (best.span.End-best.span.Start) {
			best = e
		}
		// Once the gap between pos and Start exceeds the best span
		// width, no tighter match is possible.
		if best != nil && (pos-e.span.Start) > (best.span.End-best.span.Start) {
			break
		}
	}
	if best == nil {
		return nil
	}
	return best.ty
}
