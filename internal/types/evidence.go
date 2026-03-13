package types

import "github.com/cwd-k2/gomputation/internal/span"

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

// EvEmptyRow creates an empty closed capability evidence row.
func EvEmptyRow() *TyEvidenceRow {
	return &TyEvidenceRow{Entries: &CapabilityEntries{}}
}

// EvClosedRow creates a closed capability evidence row from fields.
func EvClosedRow(fields ...RowField) *TyEvidenceRow {
	return EvNormalize(&TyEvidenceRow{Entries: &CapabilityEntries{Fields: fields}})
}

// EvOpenRow creates an open capability evidence row with a tail.
func EvOpenRow(fields []RowField, tail Type) *TyEvidenceRow {
	return EvNormalize(&TyEvidenceRow{Entries: &CapabilityEntries{Fields: fields}, Tail: tail})
}

// EvEmptyConstraintRow creates an empty closed constraint evidence row.
func EvEmptyConstraintRow() *TyEvidenceRow {
	return &TyEvidenceRow{Entries: &ConstraintEntries{}}
}

// EvSingleConstraint creates a constraint evidence row with one entry.
func EvSingleConstraint(className string, args []Type) *TyEvidenceRow {
	return &TyEvidenceRow{
		Entries: &ConstraintEntries{
			Entries: []ConstraintEntry{{ClassName: className, Args: args}},
		},
	}
}

// EvMkEvidence creates a TyEvidence from constraint entries and a body type.
func EvMkEvidence(entries []ConstraintEntry, body Type) *TyEvidence {
	return &TyEvidence{
		Constraints: &TyConstraintRow{Entries: entries},
		Body:        body,
	}
}

// EvNormalize sorts capability fields in an evidence row.
func EvNormalize(r *TyEvidenceRow) *TyEvidenceRow {
	if cap, ok := r.Entries.(*CapabilityEntries); ok {
		normalized := Normalize(&TyRow{Fields: cap.Fields, Tail: r.Tail})
		return &TyEvidenceRow{
			Entries: &CapabilityEntries{Fields: normalized.Fields},
			Tail:    normalized.Tail,
			S:       r.S,
		}
	}
	return r
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
