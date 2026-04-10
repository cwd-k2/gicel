package types

import (
	"strconv"
	"strings"
)

// Pretty renders a type as human-readable text.
func Pretty(t Type) string {
	switch ty := t.(type) {
	case *TyVar:
		return ty.Name
	case *TyCon:
		if ty.IsLabel {
			return "#" + ty.Name
		}
		return ty.Name
	case *TyApp:
		if s, ok := prettyTuple(ty, Pretty); ok {
			return s
		}
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
		if ty.Grade != nil {
			return name + " " + prettyAtom(ty.Grade) + " " + prettyAtom(ty.Pre) + " " + prettyAtom(ty.Post) + " " + prettyAtom(ty.Result)
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
	switch ty := t.(type) {
	case *TyVar, *TyCon, *TyEvidenceRow, *TySkolem, *TyMeta, *TyError:
		return Pretty(t)
	case *TyApp:
		if s, ok := prettyTuple(ty, Pretty); ok {
			return s
		}
		return "(" + Pretty(t) + ")"
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

// prettyTuple checks if the type is a closed Record row with tuple-shaped labels
// and renders it with tuple sugar:
//
//	Record {}                         → ()
//	Record { _1: T1, _2: T2, ... }   → (T1, T2, ...)
//
// Open rows (tail != nil) are not sugared — they represent row-polymorphic
// record access, not tuple operations.
func prettyTuple(app *TyApp, render func(Type) string) (string, bool) {
	con, ok := app.Fun.(*TyCon)
	if !ok || con.Name != TyConRecord {
		return "", false
	}
	row, ok := app.Arg.(*TyEvidenceRow)
	if !ok || row.Tail != nil {
		return "", false
	}
	caps, ok := row.Entries.(*CapabilityEntries)
	if !ok {
		return "", false
	}
	// 0 fields = unit ()
	if len(caps.Fields) == 0 {
		return "()", true
	}
	// All fields must be _1, _2, ... in order, with at least 2.
	if len(caps.Fields) < 2 {
		return "", false
	}
	for i, f := range caps.Fields {
		if f.Label != "_"+strconv.Itoa(i+1) {
			return "", false
		}
	}
	parts := make([]string, len(caps.Fields))
	for i, f := range caps.Fields {
		parts[i] = render(f.Type)
	}
	return "(" + strings.Join(parts, ", ") + ")", true
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
	switch e := e.(type) {
	case *ClassEntry:
		return prettyClassEntry(e)
	case *EqualityEntry:
		return Pretty(e.Lhs) + " ~ " + Pretty(e.Rhs)
	case *VarEntry:
		return Pretty(e.Var)
	case *QuantifiedConstraint:
		return prettyQuantifiedConstraint(e)
	}
	return "<?>"
}

func prettyClassEntry(e *ClassEntry) string {
	items := make([]string, 0, 1+len(e.Args))
	items = append(items, e.ClassName)
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
	var result strings.Builder
	result.WriteString(`\` + strings.Join(vars, " ") + ". ")
	for _, c := range qc.Context {
		result.WriteString(prettyConstraintEntry(c) + " => ")
	}
	if qc.Head != nil {
		result.WriteString(prettyClassEntry(qc.Head))
	}
	return result.String()
}

// PrettyDisplay renders a type for IDE display (hover, completion).
// Unlike Pretty, it shows TySkolem as plain type variables without the # prefix.
func PrettyDisplay(t Type) string {
	switch ty := t.(type) {
	case *TySkolem:
		return ty.Name
	case *TyArrow:
		from := PrettyDisplay(ty.From)
		if _, ok := ty.From.(*TyArrow); ok {
			from = "(" + from + ")"
		}
		return from + " -> " + PrettyDisplay(ty.To)
	case *TyApp:
		if s, ok := prettyTuple(ty, PrettyDisplay); ok {
			return s
		}
		return PrettyDisplay(ty.Fun) + " " + prettyDisplayAtom(ty.Arg)
	case *TyForall:
		vars, body := collectForalls(ty)
		return `\` + strings.Join(vars, " ") + ". " + PrettyDisplay(body)
	case *TyEvidence:
		return PrettyDisplay(ty.Constraints) + " => " + PrettyDisplay(ty.Body)
	default:
		return Pretty(t)
	}
}

func prettyDisplayAtom(t Type) string {
	switch ty := t.(type) {
	case *TyVar, *TyCon, *TyEvidenceRow, *TySkolem, *TyMeta, *TyError:
		return PrettyDisplay(t)
	case *TyApp:
		if s, ok := prettyTuple(ty, PrettyDisplay); ok {
			return s
		}
		return "(" + PrettyDisplay(t) + ")"
	default:
		return "(" + PrettyDisplay(t) + ")"
	}
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
