package types

import (
	"fmt"
	"sort"

	"github.com/cwd-k2/gomputation/internal/span"
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
	MapChildren(f func(Type) Type) EvidenceEntries
	FiberKind() Kind
}

// TyEvidenceRow is a unified row type for both capability and constraint rows.
// It replaces the former TyRow and TyConstraintRow.
type TyEvidenceRow struct {
	Entries EvidenceEntries
	Tail    Type // nil = closed row, TyVar or TyMeta = open row
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
	}
	return ch
}

func (c *CapabilityEntries) MapChildren(f func(Type) Type) EvidenceEntries {
	fields := make([]RowField, len(c.Fields))
	for i, fld := range c.Fields {
		fields[i] = RowField{Label: fld.Label, Type: f(fld.Type), S: fld.S}
	}
	return &CapabilityEntries{Fields: fields}
}

func (c *CapabilityEntries) FiberKind() Kind { return KRow{} }

// --- Constraint fiber ---

// ConstraintEntries holds type class constraint entries.
type ConstraintEntries struct {
	Entries []ConstraintEntry
}

func (*ConstraintEntries) evidenceEntries() {}

func (c *ConstraintEntries) EntryCount() int { return len(c.Entries) }

func (c *ConstraintEntries) AllChildren() []Type {
	var ch []Type
	for _, e := range c.Entries {
		ch = append(ch, constraintEntryChildren(e)...)
	}
	return ch
}

func constraintEntryChildren(e ConstraintEntry) []Type {
	ch := make([]Type, 0, len(e.Args))
	ch = append(ch, e.Args...)
	if e.Quantified != nil {
		for _, c := range e.Quantified.Context {
			ch = append(ch, constraintEntryChildren(c)...)
		}
		ch = append(ch, constraintEntryChildren(e.Quantified.Head)...)
	}
	return ch
}

func (c *ConstraintEntries) MapChildren(f func(Type) Type) EvidenceEntries {
	entries := make([]ConstraintEntry, len(c.Entries))
	for i, e := range c.Entries {
		args := make([]Type, len(e.Args))
		for j, a := range e.Args {
			args[j] = f(a)
		}
		entries[i] = ConstraintEntry{
			ClassName:     e.ClassName,
			Args:          args,
			Quantified:    e.Quantified,
			ConstraintVar: e.ConstraintVar,
			S:             e.S,
		}
		if e.ConstraintVar != nil {
			entries[i].ConstraintVar = f(e.ConstraintVar)
		}
	}
	return &ConstraintEntries{Entries: entries}
}

func (c *ConstraintEntries) FiberKind() Kind { return KConstraint{} }

// --- Evidence row builders ---

// EmptyRow creates an empty closed capability evidence row.
func EmptyRow() *TyEvidenceRow {
	return &TyEvidenceRow{Entries: &CapabilityEntries{}}
}

// ClosedRow creates a closed capability evidence row from fields.
func ClosedRow(fields ...RowField) *TyEvidenceRow {
	return NormalizeRow(&TyEvidenceRow{Entries: &CapabilityEntries{Fields: fields}})
}

// OpenRow creates an open capability evidence row with a tail.
func OpenRow(fields []RowField, tail Type) *TyEvidenceRow {
	return NormalizeRow(&TyEvidenceRow{Entries: &CapabilityEntries{Fields: fields}, Tail: tail})
}

// EmptyConstraintRow creates an empty closed constraint evidence row.
func EmptyConstraintRow() *TyEvidenceRow {
	return &TyEvidenceRow{Entries: &ConstraintEntries{}}
}

// SingleConstraint creates a constraint evidence row with one entry.
func SingleConstraint(className string, args []Type) *TyEvidenceRow {
	return &TyEvidenceRow{
		Entries: &ConstraintEntries{
			Entries: []ConstraintEntry{{ClassName: className, Args: args}},
		},
	}
}

// NormalizeRow sorts capability fields in an evidence row.
func NormalizeRow(r *TyEvidenceRow) *TyEvidenceRow {
	cap, ok := r.Entries.(*CapabilityEntries)
	if !ok || len(cap.Fields) <= 1 {
		return r
	}
	sorted := make([]RowField, len(cap.Fields))
	copy(sorted, cap.Fields)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Label < sorted[j].Label
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Label == sorted[i-1].Label {
			panic(fmt.Sprintf("duplicate label in evidence row: %s", sorted[i].Label))
		}
	}
	return &TyEvidenceRow{
		Entries: &CapabilityEntries{Fields: sorted},
		Tail:    r.Tail,
		S:       r.S,
	}
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

// --- Capability row operations ---

// Labels returns the set of label names in a capability evidence row.
func Labels(r *TyEvidenceRow) map[string]struct{} {
	fields := r.CapFields()
	m := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		m[f.Label] = struct{}{}
	}
	return m
}

// HasLabel checks if a label exists in a capability evidence row.
func HasLabel(r *TyEvidenceRow, label string) bool {
	for _, f := range r.CapFields() {
		if f.Label == label {
			return true
		}
	}
	return false
}

// ExtendRow adds a field to a capability evidence row, maintaining sorted order.
func ExtendRow(r *TyEvidenceRow, f RowField) (*TyEvidenceRow, error) {
	fields := r.CapFields()
	for _, existing := range fields {
		if existing.Label == f.Label {
			return nil, fmt.Errorf("duplicate label: %s", f.Label)
		}
	}
	result := make([]RowField, 0, len(fields)+1)
	inserted := false
	for _, existing := range fields {
		if !inserted && f.Label < existing.Label {
			result = append(result, f)
			inserted = true
		}
		result = append(result, existing)
	}
	if !inserted {
		result = append(result, f)
	}
	return &TyEvidenceRow{
		Entries: &CapabilityEntries{Fields: result},
		Tail:    r.Tail,
		S:       r.S,
	}, nil
}

// RemoveLabel removes a field by label from a capability evidence row.
func RemoveLabel(r *TyEvidenceRow, label string) (RowField, *TyEvidenceRow, bool) {
	fields := r.CapFields()
	for i, f := range fields {
		if f.Label == label {
			remaining := make([]RowField, 0, len(fields)-1)
			remaining = append(remaining, fields[:i]...)
			remaining = append(remaining, fields[i+1:]...)
			return f, &TyEvidenceRow{
				Entries: &CapabilityEntries{Fields: remaining},
				Tail:    r.Tail,
				S:       r.S,
			}, true
		}
	}
	return RowField{}, r, false
}

// --- Constraint row operations ---

// NormalizeConstraints sorts constraint entries by canonical key.
func NormalizeConstraints(r *TyEvidenceRow) *TyEvidenceRow {
	entries := r.ConEntries()
	if len(entries) <= 1 {
		return r
	}
	sorted := make([]ConstraintEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool {
		return ConstraintKey(sorted[i]) < ConstraintKey(sorted[j])
	})
	return &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: sorted},
		Tail:    r.Tail,
		S:       r.S,
	}
}

// ExtendConstraint adds a constraint entry to a constraint evidence row.
func ExtendConstraint(r *TyEvidenceRow, e ConstraintEntry) *TyEvidenceRow {
	old := r.ConEntries()
	entries := make([]ConstraintEntry, len(old)+1)
	copy(entries, old)
	entries[len(old)] = e
	return &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: entries},
		Tail:    r.Tail,
		S:       r.S,
	}
}
