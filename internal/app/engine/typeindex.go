package engine

import (
	"sort"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TypeIndex records the type assigned to each expression span during
// type checking. Entries are stored in sorted order for efficient
// positional queries.
//
// Lifecycle: Record → Finalize → TypeAt. Record after Finalize panics.
type TypeIndex struct {
	entries   []typeEntry
	finalized bool
}

type typeEntry struct {
	span span.Span
	ty   types.Type
}

// NewTypeIndex creates an empty TypeIndex.
func NewTypeIndex() *TypeIndex {
	return &TypeIndex{}
}

// Record adds a span→type mapping. Entries with zero-width spans or
// TyError types are silently discarded. Panics if called after Finalize.
func (idx *TypeIndex) Record(sp span.Span, ty types.Type) {
	if idx.finalized {
		panic("TypeIndex.Record called after Finalize")
	}
	if sp.Start >= sp.End {
		return
	}
	if _, ok := ty.(*types.TyError); ok {
		return
	}
	idx.entries = append(idx.entries, typeEntry{span: sp, ty: ty})
}

// Finalize sorts entries for positional queries. Must be called
// exactly once after all Record calls and before any TypeAt calls.
func (idx *TypeIndex) Finalize() {
	idx.finalized = true
	sort.Slice(idx.entries, func(i, j int) bool {
		a, b := idx.entries[i].span, idx.entries[j].span
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return (a.End - a.Start) < (b.End - b.Start)
	})
}

// Len returns the number of recorded entries.
func (idx *TypeIndex) Len() int { return len(idx.entries) }

// TypeAt returns the type of the innermost expression whose span
// contains the given byte offset. Returns nil if no span matches.
func (idx *TypeIndex) TypeAt(pos span.Pos) types.Type {
	hi := sort.Search(len(idx.entries), func(i int) bool {
		return idx.entries[i].span.Start > pos
	})
	var best *typeEntry
	for i := hi - 1; i >= 0; i-- {
		e := &idx.entries[i]
		if e.span.End <= pos {
			continue
		}
		if best == nil || (e.span.End-e.span.Start) < (best.span.End-best.span.Start) {
			best = e
		}
		if best != nil && (pos-e.span.Start) > (best.span.End-best.span.Start) {
			break
		}
	}
	if best == nil {
		return nil
	}
	return best.ty
}
