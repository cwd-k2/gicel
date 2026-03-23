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
	MapChildren(f func(Type) Type) EvidenceEntries
	FiberKind() Kind
	Empty() EvidenceEntries                                   // empty entries of the same fiber
	ZonkEntries(zonk func(Type) Type) (EvidenceEntries, bool) // zonked entries, changed
}

// TyEvidenceRow is the unified row type for capability and constraint rows.
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
		ch = append(ch, f.Grades...)
	}
	return ch
}

func (c *CapabilityEntries) MapChildren(f func(Type) Type) EvidenceEntries {
	fields := make([]RowField, len(c.Fields))
	for i, fld := range c.Fields {
		var grades []Type
		if len(fld.Grades) > 0 {
			grades = make([]Type, len(fld.Grades))
			for j, g := range fld.Grades {
				grades[j] = f(g)
			}
		}
		fields[i] = RowField{Label: fld.Label, Type: f(fld.Type), Grades: grades, S: fld.S}
	}
	return &CapabilityEntries{Fields: fields}
}

func (c *CapabilityEntries) FiberKind() Kind { return KRow{} }

func (c *CapabilityEntries) Empty() EvidenceEntries { return &CapabilityEntries{} }

func (c *CapabilityEntries) ZonkEntries(zonk func(Type) Type) (EvidenceEntries, bool) {
	changed := false
	fields := make([]RowField, len(c.Fields))
	for i, f := range c.Fields {
		zTy := zonk(f.Type)
		if zTy != f.Type {
			changed = true
		}
		var zGrades []Type
		if len(f.Grades) > 0 {
			zGrades = make([]Type, len(f.Grades))
			for j, g := range f.Grades {
				zGrades[j] = zonk(g)
				if zGrades[j] != g {
					changed = true
				}
			}
		}
		fields[i] = RowField{Label: f.Label, Type: zTy, Grades: zGrades, S: f.S}
	}
	return &CapabilityEntries{Fields: fields}, changed
}

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
			ClassName: e.ClassName,
			Args:      args,
			S:         e.S,
		}
		if e.ConstraintVar != nil {
			entries[i].ConstraintVar = f(e.ConstraintVar)
		}
		if e.Quantified != nil {
			entries[i].Quantified = mapQuantifiedConstraint(e.Quantified, f)
		}
	}
	return &ConstraintEntries{Entries: entries}
}

// mapQuantifiedConstraint applies f to all type children inside a QuantifiedConstraint.
func mapQuantifiedConstraint(qc *QuantifiedConstraint, f func(Type) Type) *QuantifiedConstraint {
	ctx := make([]ConstraintEntry, len(qc.Context))
	for i, ce := range qc.Context {
		ctx[i] = mapConstraintEntry(ce, f)
	}
	head := mapConstraintEntry(qc.Head, f)
	return &QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head}
}

// mapConstraintEntry applies f to all type children in a ConstraintEntry.
func mapConstraintEntry(e ConstraintEntry, f func(Type) Type) ConstraintEntry {
	args := make([]Type, len(e.Args))
	for j, a := range e.Args {
		args[j] = f(a)
	}
	result := ConstraintEntry{ClassName: e.ClassName, Args: args, S: e.S}
	if e.ConstraintVar != nil {
		result.ConstraintVar = f(e.ConstraintVar)
	}
	if e.Quantified != nil {
		result.Quantified = mapQuantifiedConstraint(e.Quantified, f)
	}
	return result
}

func (c *ConstraintEntries) FiberKind() Kind { return KConstraint{} }

func (c *ConstraintEntries) Empty() EvidenceEntries { return &ConstraintEntries{} }

func (c *ConstraintEntries) ZonkEntries(zonk func(Type) Type) (EvidenceEntries, bool) {
	changed := false
	ces := make([]ConstraintEntry, len(c.Entries))
	for i, e := range c.Entries {
		ces[i] = zonkConstraintEntryWith(e, zonk, &changed)
	}
	return &ConstraintEntries{Entries: ces}, changed
}

// zonkConstraintEntryWith zonks a single constraint entry using the provided
// zonk function, including any quantified sub-structure and constraint
// variable decomposition.
func zonkConstraintEntryWith(e ConstraintEntry, zonk func(Type) Type, changed *bool) ConstraintEntry {
	args := make([]Type, len(e.Args))
	for j, a := range e.Args {
		args[j] = zonk(a)
		if args[j] != a {
			*changed = true
		}
	}
	result := ConstraintEntry{ClassName: e.ClassName, Args: args, S: e.S}
	if e.ConstraintVar != nil {
		newCV := zonk(e.ConstraintVar)
		if newCV != e.ConstraintVar {
			*changed = true
		}
		result.ConstraintVar = newCV
		// If zonked ConstraintVar is now concrete, decompose into ClassName + Args.
		if result.ClassName == "" {
			head, tArgs := UnwindApp(newCV)
			if con, ok := head.(*TyCon); ok {
				result.ClassName = con.Name
				result.Args = tArgs
			}
		}
	}
	if e.Quantified != nil {
		result.Quantified = zonkQuantifiedConstraintWith(e.Quantified, zonk, changed)
	}
	return result
}

func zonkQuantifiedConstraintWith(qc *QuantifiedConstraint, zonk func(Type) Type, changed *bool) *QuantifiedConstraint {
	ctx := make([]ConstraintEntry, len(qc.Context))
	for i, c := range qc.Context {
		ctx[i] = zonkConstraintEntryWith(c, zonk, changed)
	}
	head := zonkConstraintEntryWith(qc.Head, zonk, changed)
	return &QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head}
}

// --- Evidence row builders ---

// EmptyRow creates an empty closed capability evidence row.
func EmptyRow() *TyEvidenceRow {
	return &TyEvidenceRow{Entries: &CapabilityEntries{}}
}

// ClosedRow creates a closed capability evidence row from fields.
// Panics on duplicate labels — callers must provide unique labels.
func ClosedRow(fields ...RowField) *TyEvidenceRow {
	return mustNormalizeRow(&TyEvidenceRow{Entries: &CapabilityEntries{Fields: fields}})
}

// OpenRow creates an open capability evidence row with a tail.
// Panics on duplicate labels — callers must provide unique labels.
func OpenRow(fields []RowField, tail Type) *TyEvidenceRow {
	return mustNormalizeRow(&TyEvidenceRow{Entries: &CapabilityEntries{Fields: fields}, Tail: tail})
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
	return &TyEvidenceRow{
		Entries: &CapabilityEntries{Fields: sorted},
		Tail:    r.Tail,
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
