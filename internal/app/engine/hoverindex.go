package engine

import (
	"sort"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// HoverKind classifies what a hover entry represents.
type HoverKind uint8

const (
	HoverExpr        HoverKind = iota // expression type
	HoverBinding                      // top-level binding definition
	HoverForm                         // form declaration (data type / type class)
	HoverTypeAlias                    // type alias
	HoverTypeAnn                      // :: type annotation
	HoverConstructor                  // constructor
	HoverImport                       // import declaration
	HoverImpl                         // impl declaration
)

// HoverIndex records hover information for each expression and
// declaration span during type checking. Entries are stored in sorted
// order for efficient positional queries.
//
// Lifecycle: Record/RecordDecl → RezonkAll → Finalize → HoverAt.
// Record after Finalize panics.
type HoverIndex struct {
	entries   []hoverEntry
	finalized bool
}

type hoverEntry struct {
	span  span.Span
	kind  HoverKind
	ty    types.Type // nil for imports
	label string     // "" for expressions
	doc   string     // doc comment (empty for expressions)
}

// NewHoverIndex creates an empty HoverIndex.
func NewHoverIndex() *HoverIndex {
	return &HoverIndex{}
}

// Record adds an expression type entry. Entries with zero-width spans
// or TyError types are silently discarded. Panics if called after Finalize.
func (idx *HoverIndex) Record(sp span.Span, ty types.Type) {
	if idx.finalized {
		panic("HoverIndex.Record called after Finalize")
	}
	if sp.Start >= sp.End {
		return
	}
	if _, ok := ty.(*types.TyError); ok {
		return
	}
	idx.entries = append(idx.entries, hoverEntry{span: sp, kind: HoverExpr, ty: ty})
}

// RecordDecl adds a declaration entry with a label, kind, and optional
// doc comment. Entries with zero-width spans are silently discarded.
func (idx *HoverIndex) RecordDecl(sp span.Span, kind HoverKind, label string, ty types.Type, doc string) {
	if idx.finalized {
		panic("HoverIndex.RecordDecl called after Finalize")
	}
	if sp.Start >= sp.End {
		return
	}
	idx.entries = append(idx.entries, hoverEntry{span: sp, kind: kind, ty: ty, label: label, doc: doc})
}

// RezonkAll applies a final zonk function to all recorded types,
// replacing unresolved metavariables with their solutions. Must be
// called after type checking completes and before Finalize.
func (idx *HoverIndex) RezonkAll(zonk func(types.Type) types.Type) {
	if zonk == nil {
		return
	}
	for i := range idx.entries {
		if idx.entries[i].ty != nil {
			idx.entries[i].ty = zonk(idx.entries[i].ty)
		}
	}
}

// Finalize sorts entries for positional queries. Must be called
// exactly once after all Record/RecordDecl calls and before any queries.
func (idx *HoverIndex) Finalize() {
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
func (idx *HoverIndex) Len() int { return len(idx.entries) }

// HoverAt returns the formatted hover string for the innermost entry
// containing the given byte offset. Returns "" if no entry matches.
func (idx *HoverIndex) HoverAt(pos span.Pos) string {
	e := idx.entryAt(pos)
	if e == nil {
		return ""
	}
	return formatHover(e)
}

// TypeAt returns the type of the innermost expression whose span
// contains the given byte offset. Returns nil if no span matches.
func (idx *HoverIndex) TypeAt(pos span.Pos) types.Type {
	e := idx.entryAt(pos)
	if e == nil {
		return nil
	}
	return e.ty
}

// entryAt returns the innermost hover entry containing the given
// byte offset, or nil if no entry matches.
func (idx *HoverIndex) entryAt(pos span.Pos) *hoverEntry {
	hi := sort.Search(len(idx.entries), func(i int) bool {
		return idx.entries[i].span.Start > pos
	})
	var best *hoverEntry
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
	return best
}

func formatHover(e *hoverEntry) string {
	var sig string
	switch e.kind {
	case HoverExpr:
		sig = types.PrettyDisplay(e.ty)
	case HoverBinding:
		sig = e.label + " :: " + types.PrettyDisplay(e.ty)
	case HoverForm:
		if e.ty != nil {
			sig = "form " + e.label + " :: " + types.PrettyTypeAsKind(e.ty)
		} else {
			sig = "form " + e.label
		}
	case HoverTypeAlias:
		if e.ty != nil {
			sig = "type " + e.label + " :: " + types.PrettyTypeAsKind(e.ty)
		} else {
			sig = "type " + e.label
		}
	case HoverTypeAnn:
		sig = e.label + " :: " + types.PrettyDisplay(e.ty)
	case HoverConstructor:
		sig = e.label + " :: " + types.PrettyDisplay(e.ty)
	case HoverImport:
		sig = "import " + e.label
	case HoverImpl:
		if e.ty != nil {
			sig = "impl " + types.PrettyDisplay(e.ty)
		} else {
			sig = "impl " + e.label
		}
	default:
		if e.ty != nil {
			sig = types.PrettyDisplay(e.ty)
		}
	}
	if e.doc != "" && sig != "" {
		return sig + "\n\n" + e.doc
	}
	return sig
}
