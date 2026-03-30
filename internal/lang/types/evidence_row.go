package types

import (
	"fmt"
	"sort"
)

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
	e := &CapabilityEntries{Fields: result}
	return &TyEvidenceRow{
		Entries: e,
		Tail:    r.Tail,
		Flags:   EvidenceRowFlags(e, r.Tail),
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
			e := &CapabilityEntries{Fields: remaining}
			return f, &TyEvidenceRow{
				Entries: e,
				Tail:    r.Tail,
				Flags:   EvidenceRowFlags(e, r.Tail),
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
	ce := &ConstraintEntries{Entries: sorted}
	return &TyEvidenceRow{
		Entries: ce,
		Tail:    r.Tail,
		Flags:   EvidenceRowFlags(ce, r.Tail),
		S:       r.S,
	}
}

// ExtendConstraint adds a constraint entry to a constraint evidence row,
// maintaining sorted order by canonical key.
func ExtendConstraint(r *TyEvidenceRow, e ConstraintEntry) *TyEvidenceRow {
	old := r.ConEntries()
	entries := make([]ConstraintEntry, len(old)+1)
	key := ConstraintKey(e)
	inserted := false
	j := 0
	for _, o := range old {
		if !inserted && key < ConstraintKey(o) {
			entries[j] = e
			j++
			inserted = true
		}
		entries[j] = o
		j++
	}
	if !inserted {
		entries[j] = e
	}
	ce := &ConstraintEntries{Entries: entries}
	return &TyEvidenceRow{
		Entries: ce,
		Tail:    r.Tail,
		Flags:   EvidenceRowFlags(ce, r.Tail),
		S:       r.S,
	}
}

// --- Row classification ---

// ClassifyRowFields partitions two sets of row fields into shared labels,
// labels only in a, and labels only in b.
func ClassifyRowFields(a, b []RowField) (shared, onlyA, onlyB []string) {
	aMap := make(map[string]bool)
	bMap := make(map[string]bool)
	for _, f := range a {
		aMap[f.Label] = true
	}
	for _, f := range b {
		bMap[f.Label] = true
	}
	for _, f := range a {
		if bMap[f.Label] {
			shared = append(shared, f.Label)
		} else {
			onlyA = append(onlyA, f.Label)
		}
	}
	for _, f := range b {
		if !aMap[f.Label] {
			onlyB = append(onlyB, f.Label)
		}
	}
	return
}

// RowFieldByLabel returns a pointer to the row field with the given label, or nil.
func RowFieldByLabel(fields []RowField, label string) *RowField {
	for i := range fields {
		if fields[i].Label == label {
			return &fields[i]
		}
	}
	return nil
}

// RowFieldType returns the type of a row field with the given label, or nil.
func RowFieldType(fields []RowField, label string) Type {
	for _, f := range fields {
		if f.Label == label {
			return f.Type
		}
	}
	return nil
}

// RowFieldGrades returns the grade annotations of a row field with the given label, or nil.
func RowFieldGrades(fields []RowField, label string) []Type {
	for _, f := range fields {
		if f.Label == label {
			return f.Grades
		}
	}
	return nil
}

// CollectCapFields returns the subset of fields whose labels appear in the labels list.
func CollectCapFields(fields []RowField, labels []string) []RowField {
	set := make(map[string]bool, len(labels))
	for _, l := range labels {
		set[l] = true
	}
	var result []RowField
	for _, f := range fields {
		if set[f.Label] {
			result = append(result, f)
		}
	}
	return result
}

// constraintMatch pairs two constraint entries that were matched during classification.
type constraintMatch struct {
	A, B ConstraintEntry
}

// constraintArgsEqual checks if two constraint entries have the same className
// and structurally equal args.
func constraintArgsEqual(a, b ConstraintEntry) bool {
	if a.ClassName != b.ClassName || len(a.Args) != len(b.Args) {
		return false
	}
	for i := range a.Args {
		if !Equal(a.Args[i], b.Args[i]) {
			return false
		}
	}
	return true
}

// ClassifyConstraints partitions constraint entries into shared (matched by className),
// onlyA, and onlyB. For entries with the same className, it attempts greedy matching:
// first by structural equality on args, then by position.
func ClassifyConstraints(a, b []ConstraintEntry) (
	shared []constraintMatch,
	onlyA, onlyB []ConstraintEntry,
) {
	bByClass := make(map[string][]int)
	for i, e := range b {
		bByClass[e.ClassName] = append(bByClass[e.ClassName], i)
	}
	bUsed := make([]bool, len(b))

	for _, ea := range a {
		matched := false
		candidates := bByClass[ea.ClassName]
		for _, bi := range candidates {
			if bUsed[bi] {
				continue
			}
			if constraintArgsEqual(ea, b[bi]) {
				shared = append(shared, constraintMatch{A: ea, B: b[bi]})
				bUsed[bi] = true
				matched = true
				break
			}
		}
		if !matched {
			for _, bi := range candidates {
				if bUsed[bi] {
					continue
				}
				shared = append(shared, constraintMatch{A: ea, B: b[bi]})
				bUsed[bi] = true
				matched = true
				break
			}
		}
		if !matched {
			onlyA = append(onlyA, ea)
		}
	}
	for i, e := range b {
		if !bUsed[i] {
			onlyB = append(onlyB, e)
		}
	}
	return
}

// DecomposeConstraintType decomposes a concrete constraint type
// (e.g., TyApp(TyCon("Eq"), TyCon("Bool"))) into its class name and type arguments.
func DecomposeConstraintType(ty Type) (className string, args []Type, ok bool) {
	head, tArgs := UnwindApp(ty)
	if con, isCon := head.(*TyCon); isCon {
		return con.Name, tArgs, true
	}
	return "", nil, false
}
