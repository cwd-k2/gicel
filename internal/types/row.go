package types

import (
	"fmt"
	"sort"
)

// Normalize sorts fields by label and returns a new TyRow.
// Panics if duplicate labels are found.
func Normalize(r *TyRow) *TyRow {
	if len(r.Fields) <= 1 {
		return r
	}
	sorted := make([]RowField, len(r.Fields))
	copy(sorted, r.Fields)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Label < sorted[j].Label
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Label == sorted[i-1].Label {
			panic(fmt.Sprintf("duplicate label in row: %s", sorted[i].Label))
		}
	}
	return &TyRow{Fields: sorted, Tail: r.Tail, S: r.S}
}

// Labels returns the set of label names in a row (excluding tail).
func Labels(r *TyRow) map[string]struct{} {
	m := make(map[string]struct{}, len(r.Fields))
	for _, f := range r.Fields {
		m[f.Label] = struct{}{}
	}
	return m
}

// HasLabel checks if a label exists in a row's fields.
func HasLabel(r *TyRow, label string) bool {
	for _, f := range r.Fields {
		if f.Label == label {
			return true
		}
	}
	return false
}

// ExtendRow adds a field to a row, maintaining sorted order.
// Returns error if label already exists.
func ExtendRow(r *TyRow, f RowField) (*TyRow, error) {
	if HasLabel(r, f.Label) {
		return nil, fmt.Errorf("duplicate label: %s", f.Label)
	}
	fields := make([]RowField, 0, len(r.Fields)+1)
	inserted := false
	for _, existing := range r.Fields {
		if !inserted && f.Label < existing.Label {
			fields = append(fields, f)
			inserted = true
		}
		fields = append(fields, existing)
	}
	if !inserted {
		fields = append(fields, f)
	}
	return &TyRow{Fields: fields, Tail: r.Tail, S: r.S}, nil
}

// RemoveLabel removes a field by label. Returns the field, the remaining row, and whether found.
func RemoveLabel(r *TyRow, label string) (RowField, *TyRow, bool) {
	for i, f := range r.Fields {
		if f.Label == label {
			remaining := make([]RowField, 0, len(r.Fields)-1)
			remaining = append(remaining, r.Fields[:i]...)
			remaining = append(remaining, r.Fields[i+1:]...)
			return f, &TyRow{Fields: remaining, Tail: r.Tail, S: r.S}, true
		}
	}
	return RowField{}, r, false
}
