package types

import (
	"fmt"
	"slices"
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
func (o *TypeOps) NormalizeConstraints(r *TyEvidenceRow) *TyEvidenceRow {
	entries := r.ConEntries()
	if len(entries) <= 1 {
		return r
	}
	sorted := make([]ConstraintEntry, len(entries))
	copy(sorted, entries)
	slices.SortFunc(sorted, func(a, b ConstraintEntry) int {
		ka, kb := o.ConstraintKey(a), o.ConstraintKey(b)
		if ka < kb {
			return -1
		}
		if ka > kb {
			return 1
		}
		return 0
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
func (o *TypeOps) ExtendConstraint(r *TyEvidenceRow, e ConstraintEntry) *TyEvidenceRow {
	old := r.ConEntries()
	entries := make([]ConstraintEntry, len(old)+1)
	key := o.ConstraintKey(e)
	inserted := false
	j := 0
	for _, existing := range old {
		if !inserted && key < o.ConstraintKey(existing) {
			entries[j] = e
			j++
			inserted = true
		}
		entries[j] = existing
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

// FlattenCapRow recursively flattens a nested capability evidence row.
// For example, { fail: String | { state: Int | #r } } is flattened to
// { fail: String, state: Int | #r }. Returns the flattened row.
// Non-capability rows (constraint rows) are returned unchanged.
func FlattenCapRow(r *TyEvidenceRow) *TyEvidenceRow {
	cap, ok := r.Entries.(*CapabilityEntries)
	if !ok {
		return r
	}
	// Check if tail is a nested TyEvidenceRow with CapabilityEntries.
	inner, ok := r.Tail.(*TyEvidenceRow)
	if !ok {
		return r
	}
	innerCap, ok := inner.Entries.(*CapabilityEntries)
	if !ok {
		return r
	}
	// Flatten: merge fields from outer and inner, keep inner's tail.
	merged := make([]RowField, 0, len(cap.Fields)+len(innerCap.Fields))
	merged = append(merged, cap.Fields...)
	merged = append(merged, innerCap.Fields...)
	entries := &CapabilityEntries{Fields: merged}
	flat := &TyEvidenceRow{
		Entries: entries,
		Tail:    inner.Tail,
		Flags:   EvidenceRowFlags(entries, inner.Tail),
		S:       r.S,
	}
	// Recurse in case of deeper nesting.
	return FlattenCapRow(flat)
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

// classHeadArgsEqual checks if two class-headed constraint entries have the
// same class name and structurally equal head args. Non-class variants and
// mismatched classes return false.
func classHeadArgsEqual(ops *TypeOps, a, b ConstraintEntry) bool {
	clsA := HeadClassName(a)
	if clsA == "" || clsA != HeadClassName(b) {
		return false
	}
	argsA := HeadClassArgs(a)
	argsB := HeadClassArgs(b)
	if len(argsA) != len(argsB) {
		return false
	}
	for i := range argsA {
		if !ops.Equal(argsA[i], argsB[i]) {
			return false
		}
	}
	return true
}

// ClassifyConstraints partitions constraint entries into shared (matched by class
// head name), onlyA, and onlyB. Greedy matching: first by structural equality on
// head args, then by position within the same class name.
//
// Non-class variants (equality, var) are classified by canonical key so each such
// entry must find an exact-key match in b; otherwise it falls into onlyA/onlyB.
func (o *TypeOps) ClassifyConstraints(a, b []ConstraintEntry) (
	shared []constraintMatch,
	onlyA, onlyB []ConstraintEntry,
) {
	// Bucket b by head class name for class-headed entries; non-class entries
	// live in a separate by-key map so structural match still works.
	bByClass := make(map[string][]int)
	bByKey := make(map[string][]int)
	for i, e := range b {
		if cls := HeadClassName(e); cls != "" {
			bByClass[cls] = append(bByClass[cls], i)
		} else {
			bByKey[o.ConstraintKey(e)] = append(bByKey[o.ConstraintKey(e)], i)
		}
	}
	bUsed := make([]bool, len(b))

	for _, ea := range a {
		matched := false
		if cls := HeadClassName(ea); cls != "" {
			candidates := bByClass[cls]
			for _, bi := range candidates {
				if bUsed[bi] {
					continue
				}
				if classHeadArgsEqual(o, ea, b[bi]) {
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
		} else {
			key := o.ConstraintKey(ea)
			for _, bi := range bByKey[key] {
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
