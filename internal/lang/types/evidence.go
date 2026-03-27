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
	MapChildren(f func(Type) Type) (EvidenceEntries, bool) // mapped entries, changed
	FiberKind() Type
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

func (c *ConstraintEntries) MapChildren(f func(Type) Type) (EvidenceEntries, bool) {
	var entries []ConstraintEntry // nil until first change
	for i, e := range c.Entries {
		me, entryChanged := mapConstraintEntryChanged(e, f)
		if entries == nil && entryChanged {
			entries = make([]ConstraintEntry, len(c.Entries))
			copy(entries[:i], c.Entries[:i])
		}
		if entries != nil {
			entries[i] = me
		}
	}
	if entries == nil {
		return c, false
	}
	return &ConstraintEntries{Entries: entries}, true
}

// mapConstraintEntryChanged applies f to all type children in a ConstraintEntry,
// returning the original unchanged when no child was modified.
func mapConstraintEntryChanged(e ConstraintEntry, f func(Type) Type) (ConstraintEntry, bool) {
	changed := false

	var args []Type
	for j, a := range e.Args {
		fa := f(a)
		if args == nil && fa != a {
			args = make([]Type, len(e.Args))
			copy(args[:j], e.Args[:j])
			changed = true
		}
		if args != nil {
			args[j] = fa
		}
	}
	if args == nil {
		args = e.Args
	}

	newCV := e.ConstraintVar
	if e.ConstraintVar != nil {
		newCV = f(e.ConstraintVar)
		if newCV != e.ConstraintVar {
			changed = true
		}
	}

	newQ := e.Quantified
	if e.Quantified != nil {
		var qChanged bool
		newQ, qChanged = mapQuantifiedConstraintChanged(e.Quantified, f)
		if qChanged {
			changed = true
		}
	}

	if !changed {
		return e, false
	}
	return ConstraintEntry{ClassName: e.ClassName, Args: args, ConstraintVar: newCV, Quantified: newQ, S: e.S}, true
}

// mapQuantifiedConstraintChanged applies f to all type children inside a
// QuantifiedConstraint, returning the original unchanged when no child was modified.
func mapQuantifiedConstraintChanged(qc *QuantifiedConstraint, f func(Type) Type) (*QuantifiedConstraint, bool) {
	changed := false
	var ctx []ConstraintEntry
	for i, ce := range qc.Context {
		mc, cChanged := mapConstraintEntryChanged(ce, f)
		if ctx == nil && cChanged {
			ctx = make([]ConstraintEntry, len(qc.Context))
			copy(ctx[:i], qc.Context[:i])
			changed = true
		}
		if ctx != nil {
			ctx[i] = mc
		}
	}
	if ctx == nil {
		ctx = qc.Context
	}
	head, headChanged := mapConstraintEntryChanged(qc.Head, f)
	if headChanged {
		changed = true
	}
	if !changed {
		return qc, false
	}
	return &QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head}, true
}

func (c *ConstraintEntries) FiberKind() Type { return TypeOfConstraints }

func (c *ConstraintEntries) Empty() EvidenceEntries { return &ConstraintEntries{} }

func (c *ConstraintEntries) ZonkEntries(zonk func(Type) Type) (EvidenceEntries, bool) {
	var ces []ConstraintEntry // nil until first change detected
	for i, e := range c.Entries {
		ze, entryChanged := zonkConstraintEntry(e, zonk)
		if ces == nil && entryChanged {
			ces = make([]ConstraintEntry, len(c.Entries))
			copy(ces[:i], c.Entries[:i]) // backfill unchanged prefix
		}
		if ces != nil {
			ces[i] = ze
		}
	}
	if ces == nil {
		return c, false
	}
	return &ConstraintEntries{Entries: ces}, true
}

// zonkConstraintEntry zonks a single constraint entry, returning the
// original unchanged when no child type was modified. Handles quantified
// sub-structure and constraint variable decomposition.
func zonkConstraintEntry(e ConstraintEntry, zonk func(Type) Type) (ConstraintEntry, bool) {
	changed := false

	// Zonk Args with lazy alloc.
	var args []Type
	for j, a := range e.Args {
		za := zonk(a)
		if args == nil && za != a {
			args = make([]Type, len(e.Args))
			copy(args[:j], e.Args[:j])
			changed = true
		}
		if args != nil {
			args[j] = za
		}
	}
	if args == nil {
		args = e.Args
	}

	// Zonk ConstraintVar.
	newCV := e.ConstraintVar
	if e.ConstraintVar != nil {
		newCV = zonk(e.ConstraintVar)
		if newCV != e.ConstraintVar {
			changed = true
		}
	}

	// Zonk Quantified sub-structure.
	newQ := e.Quantified
	if e.Quantified != nil {
		var qChanged bool
		newQ, qChanged = zonkQuantifiedConstraint(e.Quantified, zonk)
		if qChanged {
			changed = true
		}
	}

	if !changed {
		return e, false
	}

	result := ConstraintEntry{ClassName: e.ClassName, Args: args, S: e.S}
	if e.ConstraintVar != nil {
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
		result.Quantified = newQ
	}
	return result, true
}

// zonkQuantifiedConstraint zonks a QuantifiedConstraint, returning the
// original unchanged when no child was modified.
func zonkQuantifiedConstraint(qc *QuantifiedConstraint, zonk func(Type) Type) (*QuantifiedConstraint, bool) {
	changed := false
	var ctx []ConstraintEntry
	for i, c := range qc.Context {
		zc, cChanged := zonkConstraintEntry(c, zonk)
		if ctx == nil && cChanged {
			ctx = make([]ConstraintEntry, len(qc.Context))
			copy(ctx[:i], qc.Context[:i])
			changed = true
		}
		if ctx != nil {
			ctx[i] = zc
		}
	}
	if ctx == nil {
		ctx = qc.Context
	}

	head, headChanged := zonkConstraintEntry(qc.Head, zonk)
	if headChanged {
		changed = true
	}

	if !changed {
		return qc, false
	}
	return &QuantifiedConstraint{Vars: qc.Vars, Context: ctx, Head: head}, true
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
