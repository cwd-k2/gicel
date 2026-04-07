package types

import (
	"fmt"
	"sort"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// EvidenceEntries is the interface for fiber-specific entry collections
// in an evidence row. Two fibers exist: CapabilityEntries (for capability
// rows in Computation pre/post) and ConstraintEntries (for type class
// constraint rows in TyEvidence).
//
// New fibers can be added by implementing this interface.
type EvidenceEntries interface {
	evidenceEntries()
	EntryCount() int
	AllChildren() []Type
	// ForEachChild calls fn for each child type. Returns false early
	// if fn returns false. Unlike AllChildren, does not allocate a slice.
	ForEachChild(fn func(Type) bool)
	MapChildren(f func(Type) Type) (EvidenceEntries, bool) // mapped entries, changed
	FiberKind() Type
	Empty() EvidenceEntries                                   // empty entries of the same fiber
	ZonkEntries(zonk func(Type) Type) (EvidenceEntries, bool) // zonked entries, changed
}

// TyEvidenceRow is the unified row type for capability and constraint rows.
type TyEvidenceRow struct {
	Entries EvidenceEntries
	Tail    Type // nil = closed row, TyVar or TyMeta = open row
	Flags   uint8
	S       span.Span
}

func (*TyEvidenceRow) typeNode() {}

func (t *TyEvidenceRow) Span() span.Span { return t.S }

func (t *TyEvidenceRow) Children() []Type {
	ch := t.Entries.AllChildren()
	if t.Tail != nil {
		ch = append(ch, t.Tail)
	}
	return ch
}

// --- Capability fiber ---

// CapabilityEntries holds capability row fields (label → type pairs).
type CapabilityEntries struct {
	Fields []RowField
}

func (*CapabilityEntries) evidenceEntries() {}

func (c *CapabilityEntries) EntryCount() int { return len(c.Fields) }

func (c *CapabilityEntries) AllChildren() []Type {
	ch := make([]Type, 0, len(c.Fields))
	for _, f := range c.Fields {
		ch = append(ch, f.Type)
		ch = append(ch, f.Grades...)
	}
	return ch
}

func (c *CapabilityEntries) ForEachChild(fn func(Type) bool) {
	for _, f := range c.Fields {
		if !fn(f.Type) {
			return
		}
		for _, g := range f.Grades {
			if !fn(g) {
				return
			}
		}
	}
}

func (c *CapabilityEntries) MapChildren(f func(Type) Type) (EvidenceEntries, bool) {
	var fields []RowField // nil until first change
	for i, fld := range c.Fields {
		newTy := f(fld.Type)
		newGrades, gChanged := applyGrades(fld.Grades, f)
		if fields == nil && (newTy != fld.Type || gChanged) {
			fields = make([]RowField, len(c.Fields))
			copy(fields[:i], c.Fields[:i])
		}
		if fields != nil {
			fields[i] = RowField{Label: fld.Label, Type: newTy, Grades: newGrades, S: fld.S}
		}
	}
	if fields == nil {
		return c, false
	}
	return &CapabilityEntries{Fields: fields}, true
}

// applyGrades applies f to grade annotations, returning the original slice
// unchanged (and false) when no grade was modified.
func applyGrades(grades []Type, f func(Type) Type) ([]Type, bool) {
	if len(grades) == 0 {
		return grades, false
	}
	var out []Type // nil until first change
	for j, g := range grades {
		fg := f(g)
		if out == nil && fg != g {
			out = make([]Type, len(grades))
			copy(out[:j], grades[:j])
		}
		if out != nil {
			out[j] = fg
		}
	}
	if out == nil {
		return grades, false
	}
	return out, true
}

func (c *CapabilityEntries) FiberKind() Type { return TypeOfRows }

func (c *CapabilityEntries) Empty() EvidenceEntries { return &CapabilityEntries{} }

func (c *CapabilityEntries) ZonkEntries(zonk func(Type) Type) (EvidenceEntries, bool) {
	var fields []RowField // nil until first change detected
	for i, f := range c.Fields {
		zTy := zonk(f.Type)
		zGrades, gChanged := applyGrades(f.Grades, zonk)
		if fields == nil && (zTy != f.Type || gChanged) {
			fields = make([]RowField, len(c.Fields))
			copy(fields[:i], c.Fields[:i]) // backfill unchanged prefix
		}
		if fields != nil {
			fields[i] = RowField{Label: f.Label, Type: zTy, Grades: zGrades, S: f.S}
		}
	}
	if fields == nil {
		return c, false
	}
	return &CapabilityEntries{Fields: fields}, true
}

// --- Evidence row builders ---

// EvidenceRowFlags computes FlagMetaFree and FlagNoFamilyApp for a TyEvidenceRow
// by checking its entries and tail. O(n) in field count, each child O(1) via Flags.
func EvidenceRowFlags(entries EvidenceEntries, tail Type) uint8 {
	flags := FlagStable
	if tail != nil {
		flags &= nodeFlags(tail)
		if flags == 0 {
			return 0
		}
	}
	entries.ForEachChild(func(child Type) bool {
		flags &= nodeFlags(child)
		return flags != 0
	})
	return flags
}

// EmptyRow creates an empty closed capability evidence row.
func EmptyRow() *TyEvidenceRow {
	return &TyEvidenceRow{Entries: &CapabilityEntries{}, Flags: FlagMetaFree}
}

// ClosedRow creates a closed capability evidence row from fields.
// Panics on duplicate labels — callers must provide unique labels.
func ClosedRow(fields ...RowField) *TyEvidenceRow {
	entries := &CapabilityEntries{Fields: fields}
	r := mustNormalizeRow(&TyEvidenceRow{Entries: entries})
	r.Flags = EvidenceRowFlags(r.Entries, r.Tail)
	return r
}

// OpenRow creates an open capability evidence row with a tail.
// Panics on duplicate labels — callers must provide unique labels.
func OpenRow(fields []RowField, tail Type) *TyEvidenceRow {
	entries := &CapabilityEntries{Fields: fields}
	r := mustNormalizeRow(&TyEvidenceRow{Entries: entries, Tail: tail})
	r.Flags = EvidenceRowFlags(r.Entries, r.Tail)
	return r
}

// NormalizeRow sorts capability fields in an evidence row.
// Returns an error if duplicate labels are detected.
func NormalizeRow(r *TyEvidenceRow) (*TyEvidenceRow, error) {
	cap, ok := r.Entries.(*CapabilityEntries)
	if !ok || len(cap.Fields) <= 1 {
		return r, nil
	}
	sorted := make([]RowField, len(cap.Fields))
	copy(sorted, cap.Fields)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Label < sorted[j].Label
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Label == sorted[i-1].Label {
			return nil, fmt.Errorf("duplicate label in evidence row: %s", sorted[i].Label)
		}
	}
	entries := &CapabilityEntries{Fields: sorted}
	return &TyEvidenceRow{
		Entries: entries,
		Tail:    r.Tail,
		Flags:   EvidenceRowFlags(entries, r.Tail),
		S:       r.S,
	}, nil
}

// mustNormalizeRow calls NormalizeRow and panics on error.
// Used by convenience builders (ClosedRow, OpenRow) that construct
// rows from known-good data. Any panic here is caught by RunSandbox's
// top-level recover.
func mustNormalizeRow(r *TyEvidenceRow) *TyEvidenceRow {
	result, err := NormalizeRow(r)
	if err != nil {
		panic(err.Error())
	}
	return result
}

// IsCapabilityRow returns true if this evidence row uses the capability fiber.
func (r *TyEvidenceRow) IsCapabilityRow() bool {
	_, ok := r.Entries.(*CapabilityEntries)
	return ok
}

// IsConstraintRow returns true if this evidence row uses the constraint fiber.
func (r *TyEvidenceRow) IsConstraintRow() bool {
	_, ok := r.Entries.(*ConstraintEntries)
	return ok
}

// CapFields returns the capability fields. Panics if not a capability row.
func (r *TyEvidenceRow) CapFields() []RowField {
	return r.Entries.(*CapabilityEntries).Fields
}

// ConEntries returns the constraint entries. Panics if not a constraint row.
func (r *TyEvidenceRow) ConEntries() []ConstraintEntry {
	return r.Entries.(*ConstraintEntries).Entries
}
