package types

import (
	"strings"
)

// Pretty renders a type as human-readable text.
func Pretty(t Type) string {
	switch ty := t.(type) {
	case *TyVar:
		return ty.Name
	case *TyCon:
		// Label literals (L1, non-builtin) display with # prefix.
		if IsKindLevel(ty.Level) && !IsBuiltinKindCon(ty) {
			return "#" + ty.Name
		}
		return ty.Name
	case *TyApp:
		return Pretty(ty.Fun) + " " + prettyAtom(ty.Arg)
	case *TyArrow:
		from := Pretty(ty.From)
		if _, ok := ty.From.(*TyArrow); ok {
			from = "(" + from + ")"
		}
		return from + " -> " + Pretty(ty.To)
	case *TyForall:
		vars, body := collectForalls(ty)
		return `\` + strings.Join(vars, " ") + ". " + Pretty(body)
	case *TyCBPV:
		name := TyConComputation
		if ty.Tag == TagThunk {
			name = TyConThunk
		}
		return name + " " + prettyAtom(ty.Pre) + " " + prettyAtom(ty.Post) + " " + prettyAtom(ty.Result)
	case *TyEvidenceRow:
		return prettyEvidenceRow(ty)
	case *TyEvidence:
		return Pretty(ty.Constraints) + " => " + Pretty(ty.Body)
	case *TyFamilyApp:
		parts := []string{ty.Name}
		for _, a := range ty.Args {
			parts = append(parts, prettyAtom(a))
		}
		return strings.Join(parts, " ")
	case *TySkolem:
		return "#" + ty.Name
	case *TyMeta:
		return "_"
	case *TyError:
		return "<error>"
	default:
		return "?"
	}
}

// PrettyAtom renders a type as a single syntactic atom.
// Compound types (arrows, applications, foralls, etc.) are wrapped in parentheses.
func PrettyAtom(t Type) string {
	return prettyAtom(t)
}

func prettyAtom(t Type) string {
	switch t.(type) {
	case *TyVar, *TyCon, *TyEvidenceRow, *TySkolem, *TyMeta, *TyError:
		return Pretty(t)
	default:
		return "(" + Pretty(t) + ")"
	}
}

func collectForalls(t *TyForall) ([]string, Type) {
	vars := []string{t.Var}
	body := t.Body
	for {
		if f, ok := body.(*TyForall); ok {
			vars = append(vars, f.Var)
			body = f.Body
		} else {
			break
		}
	}
	return vars, body
}

func prettyCapFields(fields []RowField, tail Type) string {
	if len(fields) == 0 && tail == nil {
		return "{}"
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		if len(f.Grades) > 0 {
			gs := make([]string, len(f.Grades))
			for j, g := range f.Grades {
				gs[j] = Pretty(g)
			}
			parts[i] = f.Label + ": " + Pretty(f.Type) + " @ " + strings.Join(gs, " @ ")
		} else {
			parts[i] = f.Label + ": " + Pretty(f.Type)
		}
	}
	inner := strings.Join(parts, ", ")
	if tail != nil {
		if len(parts) > 0 {
			inner += " | " + Pretty(tail)
		} else {
			inner = "| " + Pretty(tail)
		}
	}
	return "{ " + inner + " }"
}

func prettyConstraintEntries(entries []ConstraintEntry, tail Type) string {
	if len(entries) == 0 && tail == nil {
		return "{}"
	}
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = prettyConstraintEntry(e)
	}
	inner := strings.Join(parts, ", ")
	if tail != nil {
		if len(parts) > 0 {
			inner += " | " + Pretty(tail)
		} else {
			inner = "| " + Pretty(tail)
		}
	}
	return "{ " + inner + " }"
}

func prettyEvidenceRow(r *TyEvidenceRow) string {
	switch entries := r.Entries.(type) {
	case *CapabilityEntries:
		return prettyCapFields(entries.Fields, r.Tail)
	case *ConstraintEntries:
		return prettyConstraintEntries(entries.Entries, r.Tail)
	default:
		// Generic fallback for future fiber types.
		children := r.Entries.AllChildren()
		parts := make([]string, len(children))
		for i, c := range children {
			parts[i] = Pretty(c)
		}
		result := "{ " + strings.Join(parts, ", ") + " }"
		if r.Tail != nil {
			result = "{ " + strings.Join(parts, ", ") + " | " + Pretty(r.Tail) + " }"
		}
		return result
	}
}

func prettyConstraintEntry(e ConstraintEntry) string {
	if e.Quantified != nil {
		return prettyQuantifiedConstraint(e.Quantified)
	}
	if e.ConstraintVar != nil && e.ClassName == "" {
		return Pretty(e.ConstraintVar)
	}
	items := []string{e.ClassName}
	for _, a := range e.Args {
		items = append(items, prettyAtom(a))
	}
	return strings.Join(items, " ")
}

func prettyQuantifiedConstraint(qc *QuantifiedConstraint) string {
	var vars []string
	for _, v := range qc.Vars {
		if Equal(v.Kind, TypeOfTypes) {
			vars = append(vars, v.Name)
		} else {
			vars = append(vars, "("+v.Name+": "+PrettyTypeAsKind(v.Kind)+")")
		}
	}
	result := `\` + strings.Join(vars, " ") + ". "
	for _, c := range qc.Context {
		result += prettyConstraintEntry(c) + " => "
	}
	result += prettyConstraintEntry(qc.Head)
	return result
}

// PrettyTypeAsKind renders a type that represents a kind (level >= 1).
// Used for error messages and diagnostics during/after Type/Kind unification.
func PrettyTypeAsKind(t Type) string {
	switch ty := t.(type) {
	case *TyCon:
		return ty.Name
	case *TyArrow:
		from := PrettyTypeAsKind(ty.From)
		if _, ok := ty.From.(*TyArrow); ok {
			from = "(" + from + ")"
		}
		return from + " -> " + PrettyTypeAsKind(ty.To)
	case *TyVar:
		return ty.Name
	case *TyMeta:
		return "_"
	default:
		return Pretty(t)
	}
}
