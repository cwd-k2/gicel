package engine

import (
	"fmt"
	"slices"
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
	HoverOperator                     // operator with fixity info
)

// OperatorFixity holds the fixity information for an operator.
type OperatorFixity struct {
	Assoc string // "infixl", "infixr", "infix"
	Prec  int
}

// String returns the fixity declaration form (e.g., "infixl 6").
func (f OperatorFixity) String() string {
	return fmt.Sprintf("%s %d", f.Assoc, f.Prec)
}

type hoverEntry struct {
	span   span.Span
	kind   HoverKind
	ty     types.Type      // nil for imports
	label  string          // "" for expressions
	doc    string          // doc comment (empty for expressions)
	module string          // non-empty for HoverOperator and imported variable references
	fixity *OperatorFixity // non-nil only for HoverOperator
}

// --- HoverIndexBuilder: write-side (Record → Rezonk → Finalize) ---

// HoverIndexBuilder accumulates hover entries during type checking.
// After all entries are recorded, call Finalize() to produce an
// immutable *HoverIndex for positional queries.
//
// Lifecycle: Record*/Attach* → RezonkAll → Finalize → (use *HoverIndex)
type HoverIndexBuilder struct {
	entries        []hoverEntry
	pendingDocs    map[span.Pos]string // span.Start → doc
	pendingModules map[span.Pos]string // span.Start → module
	pendingLabels  map[span.Pos]string // span.Start → var name
}

// NewHoverIndexBuilder creates an empty builder.
func NewHoverIndexBuilder() *HoverIndexBuilder {
	return &HoverIndexBuilder{}
}

// Record adds an expression type entry. Entries with zero-width spans
// or TyError types are silently discarded.
func (b *HoverIndexBuilder) Record(sp span.Span, ty types.Type) {
	if sp.Start >= sp.End {
		return
	}
	if _, ok := ty.(*types.TyError); ok {
		return
	}
	b.entries = append(b.entries, hoverEntry{span: sp, kind: HoverExpr, ty: ty})
}

// RecordDecl adds a declaration entry with a label, kind, and optional doc comment.
func (b *HoverIndexBuilder) RecordDecl(sp span.Span, kind HoverKind, label string, ty types.Type, doc string) {
	if sp.Start >= sp.End {
		return
	}
	b.entries = append(b.entries, hoverEntry{span: sp, kind: kind, ty: ty, label: label, doc: doc})
}

// RecordOperator adds an operator entry with its type, module, and fixity information.
func (b *HoverIndexBuilder) RecordOperator(sp span.Span, name, module string, ty types.Type, fixity *OperatorFixity) {
	if sp.Start >= sp.End {
		return
	}
	if _, ok := ty.(*types.TyError); ok {
		return
	}
	b.entries = append(b.entries, hoverEntry{span: sp, kind: HoverOperator, ty: ty, label: name, module: module, fixity: fixity})
}

// AttachDoc associates a doc comment with an expression span.
// Applied during Finalize to the matching HoverExpr entry.
func (b *HoverIndexBuilder) AttachDoc(sp span.Span, doc string) {
	if doc == "" || sp.Start >= sp.End {
		return
	}
	if b.pendingDocs == nil {
		b.pendingDocs = make(map[span.Pos]string)
	}
	b.pendingDocs[sp.Start] = doc
}

// AttachVarInfo associates documentation and module provenance with a
// variable reference span. Applied during Finalize to the matching
// HoverExpr entry.
func (b *HoverIndexBuilder) AttachVarInfo(sp span.Span, name, module, doc string) {
	if sp.Start >= sp.End {
		return
	}
	if doc != "" {
		if b.pendingDocs == nil {
			b.pendingDocs = make(map[span.Pos]string)
		}
		b.pendingDocs[sp.Start] = doc
	}
	if module != "" {
		if b.pendingModules == nil {
			b.pendingModules = make(map[span.Pos]string)
		}
		b.pendingModules[sp.Start] = module
	}
	if name != "" && module != "" {
		if b.pendingLabels == nil {
			b.pendingLabels = make(map[span.Pos]string)
		}
		b.pendingLabels[sp.Start] = name
	}
}

// RezonkAll applies a final zonk function to all recorded types,
// replacing unresolved metavariables with their solutions. Must be
// called after type checking completes and before Finalize.
func (b *HoverIndexBuilder) RezonkAll(zonk func(types.Type) types.Type) {
	if zonk == nil {
		return
	}
	for i := range b.entries {
		if b.entries[i].ty != nil {
			b.entries[i].ty = zonk(b.entries[i].ty)
		}
	}
}

// Finalize applies pending doc comments, sorts entries for positional queries,
// and returns an immutable *HoverIndex. The builder must not be used after this call.
func (b *HoverIndexBuilder) Finalize() *HoverIndex {
	// Apply pending doc comments and module provenance to matching expression entries.
	for i := range b.entries {
		e := &b.entries[i]
		if e.kind == HoverExpr {
			if e.doc == "" {
				if doc, ok := b.pendingDocs[e.span.Start]; ok {
					e.doc = doc
				}
			}
			if mod, ok := b.pendingModules[e.span.Start]; ok {
				if e.module == "" {
					e.module = mod
				}
			}
			if label, ok := b.pendingLabels[e.span.Start]; ok {
				if e.label == "" {
					e.label = label
				}
			}
		}
	}
	slices.SortStableFunc(b.entries, func(a, c hoverEntry) int {
		if a.span.Start != c.span.Start {
			return int(a.span.Start - c.span.Start)
		}
		return int((a.span.End - a.span.Start) - (c.span.End - c.span.Start))
	})
	idx := &HoverIndex{entries: b.entries}
	// Prevent further use of builder.
	b.entries = nil
	b.pendingDocs = nil
	b.pendingModules = nil
	b.pendingLabels = nil
	return idx
}

// --- HoverIndex: read-side (immutable, positional queries only) ---

// HoverIndex provides positional hover queries over a finalized set of entries.
// Created by HoverIndexBuilder.Finalize(). All methods are safe for concurrent use.
type HoverIndex struct {
	entries []hoverEntry
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
		if e.module != "" && e.label != "" {
			sig = e.module + "." + e.label + " :: " + types.PrettyDisplay(e.ty)
		} else {
			sig = types.PrettyDisplay(e.ty)
		}
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
	case HoverOperator:
		name := e.label
		if e.module != "" {
			name = e.module + "." + e.label
		}
		sig = "(" + name + ") :: " + types.PrettyDisplay(e.ty)
		if e.fixity != nil {
			sig += "\n" + e.fixity.String()
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
