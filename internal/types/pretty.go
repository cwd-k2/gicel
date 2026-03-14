package types

import (
	"fmt"
	"strings"
)

// Pretty renders a type as human-readable text.
func Pretty(t Type) string {
	switch ty := t.(type) {
	case *TyVar:
		return ty.Name
	case *TyCon:
		return ty.Name
	case *TyApp:
		return fmt.Sprintf("%s %s", Pretty(ty.Fun), prettyAtom(ty.Arg))
	case *TyArrow:
		from := Pretty(ty.From)
		if _, ok := ty.From.(*TyArrow); ok {
			from = "(" + from + ")"
		}
		return fmt.Sprintf("%s -> %s", from, Pretty(ty.To))
	case *TyForall:
		vars, body := collectForalls(ty)
		return fmt.Sprintf("forall %s. %s", strings.Join(vars, " "), Pretty(body))
	case *TyComp:
		return fmt.Sprintf("Computation %s %s %s",
			prettyAtom(ty.Pre), prettyAtom(ty.Post), prettyAtom(ty.Result))
	case *TyThunk:
		return fmt.Sprintf("Thunk %s %s %s",
			prettyAtom(ty.Pre), prettyAtom(ty.Post), prettyAtom(ty.Result))
	case *TyEvidenceRow:
		return prettyEvidenceRow(ty)
	case *TyEvidence:
		return Pretty(ty.Constraints) + " => " + Pretty(ty.Body)
	case *TySkolem:
		return fmt.Sprintf("#%s", ty.Name)
	case *TyMeta:
		return fmt.Sprintf("?%d", ty.ID)
	case *TyError:
		return "<error>"
	default:
		return "?"
	}
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
		parts[i] = fmt.Sprintf("%s : %s", f.Label, Pretty(f.Type))
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
		return "{?}"
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
		if _, ok := v.Kind.(KType); ok {
			vars = append(vars, v.Name)
		} else {
			vars = append(vars, fmt.Sprintf("(%s : %s)", v.Name, v.Kind))
		}
	}
	result := "forall " + strings.Join(vars, " ") + ". "
	for _, c := range qc.Context {
		result += prettyConstraintEntry(c) + " => "
	}
	result += prettyConstraintEntry(qc.Head)
	return result
}

// PrettyKind renders a kind.
func PrettyKind(k Kind) string {
	return k.String()
}
