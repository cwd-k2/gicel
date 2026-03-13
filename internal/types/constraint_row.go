package types

import (
	"sort"
	"strings"
)

// ConstraintKey returns a canonical string for a constraint entry.
// Used for sorting/display, not for matching (matching uses className + type unification).
func ConstraintKey(e ConstraintEntry) string {
	parts := []string{e.ClassName}
	for _, a := range e.Args {
		parts = append(parts, Pretty(a))
	}
	return strings.Join(parts, " ")
}

// NormalizeConstraints sorts entries by canonical key and returns a new TyConstraintRow.
// For display ordering only; matching uses className + type unification.
func NormalizeConstraints(r *TyConstraintRow) *TyConstraintRow {
	if len(r.Entries) <= 1 {
		return r
	}
	sorted := make([]ConstraintEntry, len(r.Entries))
	copy(sorted, r.Entries)
	sort.Slice(sorted, func(i, j int) bool {
		return ConstraintKey(sorted[i]) < ConstraintKey(sorted[j])
	})
	return &TyConstraintRow{Entries: sorted, Tail: r.Tail, S: r.S}
}

// ExtendConstraint adds an entry to a constraint row.
func ExtendConstraint(r *TyConstraintRow, e ConstraintEntry) *TyConstraintRow {
	entries := make([]ConstraintEntry, len(r.Entries)+1)
	copy(entries, r.Entries)
	entries[len(r.Entries)] = e
	return &TyConstraintRow{Entries: entries, Tail: r.Tail, S: r.S}
}
