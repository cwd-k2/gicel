package engine

import (
	"fmt"
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

// HoverIndex records hover information for each expression and
// declaration span during type checking. Entries are stored in sorted
// order for efficient positional queries.
//
// Lifecycle: Record/RecordDecl → RezonkAll → Finalize → HoverAt.
// Record after Finalize panics.
type HoverIndex struct {
	entries        []hoverEntry
	pendingDocs    map[span.Pos]string // span.Start → doc (set by AttachDoc/AttachVarInfo, applied at Finalize)
	pendingModules map[span.Pos]string // span.Start → module (set by AttachVarInfo, applied at Finalize)
	pendingLabels  map[span.Pos]string // span.Start → var name (set by AttachVarInfo, applied at Finalize)
	finalized      bool
}

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

// NewHoverIndex creates an empty HoverIndex.
func NewHoverIndex() *HoverIndex {
	return &HoverIndex{}
}

// guard checks common preconditions for recording: not finalized, non-zero span.
// Returns false if the entry should be skipped.
func (idx *HoverIndex) guard(sp span.Span) bool {
	if idx.finalized {
		panic("HoverIndex: Record called after Finalize")
	}
	return sp.Start < sp.End
}

// Record adds an expression type entry. Entries with zero-width spans
// or TyError types are silently discarded.
func (idx *HoverIndex) Record(sp span.Span, ty types.Type) {
	if !idx.guard(sp) {
		return
	}
	if _, ok := ty.(*types.TyError); ok {
		return
	}
	idx.entries = append(idx.entries, hoverEntry{span: sp, kind: HoverExpr, ty: ty})
}

// RecordDecl adds a declaration entry with a label, kind, and optional doc comment.
func (idx *HoverIndex) RecordDecl(sp span.Span, kind HoverKind, label string, ty types.Type, doc string) {
	if !idx.guard(sp) {
		return
	}
	idx.entries = append(idx.entries, hoverEntry{span: sp, kind: kind, ty: ty, label: label, doc: doc})
}

// RecordOperator adds an operator entry with its type, module, and fixity information.
func (idx *HoverIndex) RecordOperator(sp span.Span, name, module string, ty types.Type, fixity *OperatorFixity) {
	if !idx.guard(sp) {
		return
	}
	if _, ok := ty.(*types.TyError); ok {
		return
	}
	idx.entries = append(idx.entries, hoverEntry{span: sp, kind: HoverOperator, ty: ty, label: name, module: module, fixity: fixity})
}

// AttachDoc associates a doc comment with an expression span.
// Applied during Finalize to the matching HoverExpr entry.
func (idx *HoverIndex) AttachDoc(sp span.Span, doc string) {
	if doc == "" || sp.Start >= sp.End {
		return
	}
	if idx.pendingDocs == nil {
		idx.pendingDocs = make(map[span.Pos]string)
	}
	idx.pendingDocs[sp.Start] = doc
}

// AttachVarInfo associates documentation and module provenance with a
// variable reference span. Applied during Finalize to the matching
// HoverExpr entry.
func (idx *HoverIndex) AttachVarInfo(sp span.Span, name, module, doc string) {
	if sp.Start >= sp.End {
		return
	}
	if doc != "" {
		if idx.pendingDocs == nil {
			idx.pendingDocs = make(map[span.Pos]string)
		}
		idx.pendingDocs[sp.Start] = doc
	}
	if module != "" {
		if idx.pendingModules == nil {
			idx.pendingModules = make(map[span.Pos]string)
		}
		idx.pendingModules[sp.Start] = module
	}
	if name != "" && module != "" {
		if idx.pendingLabels == nil {
			idx.pendingLabels = make(map[span.Pos]string)
		}
		idx.pendingLabels[sp.Start] = name
	}
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

// Finalize applies pending doc comments and sorts entries for positional queries.
// Must be called exactly once after all Record/RecordDecl calls and before any queries.
func (idx *HoverIndex) Finalize() {
	// Apply pending doc comments and module provenance to matching expression entries.
	for i := range idx.entries {
		e := &idx.entries[i]
		if e.kind == HoverExpr {
			if e.doc == "" {
				if doc, ok := idx.pendingDocs[e.span.Start]; ok {
					e.doc = doc
				}
			}
			if mod, ok := idx.pendingModules[e.span.Start]; ok {
				if e.module == "" {
					e.module = mod
				}
			}
			if label, ok := idx.pendingLabels[e.span.Start]; ok {
				if e.label == "" {
					e.label = label
				}
			}
		}
	}
	idx.pendingDocs = nil
	idx.pendingModules = nil
	idx.pendingLabels = nil
	idx.finalized = true
	sort.SliceStable(idx.entries, func(i, j int) bool {
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
