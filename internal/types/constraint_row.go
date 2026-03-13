package types

import (
	"fmt"
	"sort"
	"strings"
)

// ConstraintKey returns a canonical string for a constraint entry.
// Used for sorting/display, not for matching (matching uses className + type unification).
func ConstraintKey(e ConstraintEntry) string {
	parts := []string{e.ClassName}
	for _, a := range e.Args {
		parts = append(parts, structuralKey(a))
	}
	return strings.Join(parts, " ")
}

// structuralKey builds a deterministic string key from the type structure,
// independent of display formatting.
func structuralKey(t Type) string {
	switch v := t.(type) {
	case *TyVar:
		return "Var(" + v.Name + ")"
	case *TyCon:
		return "Con(" + v.Name + ")"
	case *TyApp:
		return "App(" + structuralKey(v.Fun) + "," + structuralKey(v.Arg) + ")"
	case *TyArrow:
		return "Arrow(" + structuralKey(v.From) + "," + structuralKey(v.To) + ")"
	case *TyForall:
		return "Forall(" + v.Var + "," + structuralKey(v.Body) + ")"
	case *TyComp:
		return "Comp(" + structuralKey(v.Pre) + "," + structuralKey(v.Post) + "," + structuralKey(v.Result) + ")"
	case *TyThunk:
		return "Thunk(" + structuralKey(v.Pre) + "," + structuralKey(v.Post) + "," + structuralKey(v.Result) + ")"
	case *TyMeta:
		return fmt.Sprintf("Meta(%d)", v.ID)
	case *TySkolem:
		return fmt.Sprintf("Skolem(%d)", v.ID)
	case *TyRow:
		return structuralKey(v.ToEvidence())
	case *TyConstraintRow:
		return structuralKey(v.ToEvidence())
	case *TyEvidence:
		return "Ev(" + structuralKey(v.Constraints) + "," + structuralKey(v.Body) + ")"
	case *TyEvidenceRow:
		tail := "_"
		if v.Tail != nil {
			tail = structuralKey(v.Tail)
		}
		switch entries := v.Entries.(type) {
		case *CapabilityEntries:
			var parts []string
			for _, f := range entries.Fields {
				parts = append(parts, f.Label+":"+structuralKey(f.Type))
			}
			return "Row({" + strings.Join(parts, ",") + "}|" + tail + ")"
		case *ConstraintEntries:
			var parts []string
			for _, e := range entries.Entries {
				parts = append(parts, ConstraintKey(e))
			}
			return "CRow({" + strings.Join(parts, ",") + "}|" + tail + ")"
		default:
			return fmt.Sprintf("EvRow(%d|%s)", v.Entries.EntryCount(), tail)
		}
	case *TyError:
		return "Error"
	default:
		if t == nil {
			return "nil"
		}
		return "?"
	}
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
