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
	case *TyRow:
		return prettyRow(ty)
	case *TyConstraintRow:
		return prettyConstraintRow(ty)
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
	case *TyVar, *TyCon, *TyRow, *TySkolem, *TyMeta, *TyError:
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

func prettyRow(r *TyRow) string {
	if len(r.Fields) == 0 && r.Tail == nil {
		return "{}"
	}
	parts := make([]string, len(r.Fields))
	for i, f := range r.Fields {
		parts[i] = fmt.Sprintf("%s : %s", f.Label, Pretty(f.Type))
	}
	inner := strings.Join(parts, ", ")
	if r.Tail != nil {
		if len(parts) > 0 {
			inner += " | " + Pretty(r.Tail)
		} else {
			inner = "| " + Pretty(r.Tail)
		}
	}
	return "{ " + inner + " }"
}

func prettyConstraintRow(r *TyConstraintRow) string {
	if len(r.Entries) == 0 && r.Tail == nil {
		return "{}"
	}
	parts := make([]string, len(r.Entries))
	for i, e := range r.Entries {
		items := []string{e.ClassName}
		for _, a := range e.Args {
			items = append(items, prettyAtom(a))
		}
		parts[i] = strings.Join(items, " ")
	}
	inner := strings.Join(parts, ", ")
	if r.Tail != nil {
		if len(parts) > 0 {
			inner += " | " + Pretty(r.Tail)
		} else {
			inner = "| " + Pretty(r.Tail)
		}
	}
	return "{ " + inner + " }"
}

// PrettyKind renders a kind.
func PrettyKind(k Kind) string {
	return k.String()
}
