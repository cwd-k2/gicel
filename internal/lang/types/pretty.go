package types

import (
	"slices"
	"strconv"
	"strings"
)

// Pretty renders a type as human-readable text.
func (o *TypeOps) Pretty(t Type) string {
	switch ty := t.(type) {
	case *TyVar:
		return ty.Name
	case *TyCon:
		if ty.IsLabel {
			return "#" + ty.Name
		}
		return ty.Name
	case *TyApp:
		if s, ok := prettyTuple(ty, o.Pretty); ok {
			return s
		}
		return o.Pretty(ty.Fun) + " " + prettyAtom(o, ty.Arg)
	case *TyArrow:
		from := o.Pretty(ty.From)
		if _, ok := ty.From.(*TyArrow); ok {
			from = "(" + from + ")"
		}
		return from + " -> " + o.Pretty(ty.To)
	case *TyForall:
		vars, body := collectForalls(ty)
		return `\` + strings.Join(vars, " ") + ". " + o.Pretty(body)
	case *TyCBPV:
		name := TyConComputation
		if ty.Tag == TagThunk {
			name = TyConThunk
		}
		if ty.IsGraded() {
			return name + " " + prettyAtom(o, ty.Grade) + " " + prettyAtom(o, ty.Pre) + " " + prettyAtom(o, ty.Post) + " " + prettyAtom(o, ty.Result)
		}
		return name + " " + prettyAtom(o, ty.Pre) + " " + prettyAtom(o, ty.Post) + " " + prettyAtom(o, ty.Result)
	case *TyEvidenceRow:
		return prettyEvidenceRow(o, ty)
	case *TyEvidence:
		return o.Pretty(ty.Constraints) + " => " + o.Pretty(ty.Body)
	case *TyFamilyApp:
		parts := []string{ty.Name}
		for _, a := range ty.Args {
			parts = append(parts, prettyAtom(o, a))
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
func (o *TypeOps) PrettyAtom(t Type) string {
	return prettyAtom(o, t)
}

func prettyAtom(o *TypeOps, t Type) string {
	switch ty := t.(type) {
	case *TyVar, *TyCon, *TyEvidenceRow, *TySkolem, *TyMeta, *TyError:
		return o.Pretty(t)
	case *TyApp:
		if s, ok := prettyTuple(ty, o.Pretty); ok {
			return s
		}
		return "(" + o.Pretty(t) + ")"
	default:
		return "(" + o.Pretty(t) + ")"
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

// flattenTupleRow collects capability fields from a possibly nested row.
// Walks nested row tails: { _1: a | { _2: b | r } } → [_1: a, _2: b], tail=r.
// Returns (fields, tail, ok). ok is false for non-capability rows.
func flattenTupleRow(row *TyEvidenceRow) (fields []RowField, tail Type, ok bool) {
	for {
		caps, isCap := row.Entries.(*CapabilityEntries)
		if !isCap {
			return nil, nil, false
		}
		fields = append(fields, caps.Fields...)
		if row.IsClosed() {
			return fields, nil, true
		}
		next, isRow := row.Tail.(*TyEvidenceRow)
		if !isRow {
			return fields, row.Tail, true
		}
		row = next
	}
}

// prettyTuple checks if the type is a Record row with tuple-shaped labels
// and renders it with tuple sugar:
//
//	Record {}                                   → ()
//	Record { _1: T1, _2: T2 }                  → (T1, T2)
//	Record { _1: a | { _2: b | r } }           → (a, b | r)
//
// Fields must be _1, _2, ..., _N in order after flattening nested rows.
func prettyTuple(app *TyApp, render func(Type) string) (string, bool) {
	con, ok := app.Fun.(*TyCon)
	if !ok || con.Name != TyConRecord {
		return "", false
	}
	row, ok := app.Arg.(*TyEvidenceRow)
	if !ok {
		return "", false
	}
	fields, tail, ok := flattenTupleRow(row)
	if !ok {
		return "", false
	}
	// 0 fields, no tail = unit ()
	if len(fields) == 0 && tail == nil {
		return "()", true
	}
	// Sort fields by _N index so { _2: Int, _1: Bool } → (Bool, Int).
	slices.SortFunc(fields, func(a, b RowField) int {
		na, _ := strconv.Atoi(a.Label[1:])
		nb, _ := strconv.Atoi(b.Label[1:])
		return na - nb
	})
	// Check all fields are _1, _2, ..., _N consecutively.
	for i, f := range fields {
		if f.Label != "_"+strconv.Itoa(i+1) {
			return "", false
		}
	}
	// At least 2 fields, or 1+ field with a tail.
	if len(fields) < 2 && tail == nil {
		return "", false
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		parts[i] = render(f.Type)
	}
	inner := strings.Join(parts, ", ")
	if tail != nil {
		inner += " | " + render(tail)
	}
	return "(" + inner + ")", true
}

func prettyCapFields(o *TypeOps, fields []RowField, tail Type) string {
	if len(fields) == 0 && tail == nil {
		return "{}"
	}
	parts := make([]string, len(fields))
	for i, f := range fields {
		if f.IsGraded() {
			gs := make([]string, len(f.Grades))
			for j, g := range f.Grades {
				gs[j] = o.Pretty(g)
			}
			parts[i] = f.Label + ": " + o.Pretty(f.Type) + " @ " + strings.Join(gs, " @ ")
		} else {
			parts[i] = f.Label + ": " + o.Pretty(f.Type)
		}
	}
	inner := strings.Join(parts, ", ")
	if tail != nil {
		if len(parts) > 0 {
			inner += " | " + o.Pretty(tail)
		} else {
			inner = "| " + o.Pretty(tail)
		}
	}
	return "{ " + inner + " }"
}

func prettyConstraintEntries(o *TypeOps, entries []ConstraintEntry, tail Type) string {
	if len(entries) == 0 && tail == nil {
		return "{}"
	}
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = prettyConstraintEntry(o, e)
	}
	inner := strings.Join(parts, ", ")
	if tail != nil {
		if len(parts) > 0 {
			inner += " | " + o.Pretty(tail)
		} else {
			inner = "| " + o.Pretty(tail)
		}
	}
	return "{ " + inner + " }"
}

func prettyEvidenceRow(o *TypeOps, r *TyEvidenceRow) string {
	switch r.Entries.(type) {
	case *CapabilityEntries:
		fields, tail, _ := flattenTupleRow(r)
		return prettyCapFields(o, fields, tail)
	case *ConstraintEntries:
		return prettyConstraintEntries(o, r.Entries.(*ConstraintEntries).Entries, r.Tail)
	default:
		// Generic fallback for future fiber types.
		children := r.Entries.AllChildren()
		parts := make([]string, len(children))
		for i, c := range children {
			parts[i] = o.Pretty(c)
		}
		result := "{ " + strings.Join(parts, ", ") + " }"
		if r.IsOpen() {
			result = "{ " + strings.Join(parts, ", ") + " | " + o.Pretty(r.Tail) + " }"
		}
		return result
	}
}

func prettyConstraintEntry(o *TypeOps, e ConstraintEntry) string {
	switch e := e.(type) {
	case *ClassEntry:
		return prettyClassEntry(o, e)
	case *EqualityEntry:
		return o.Pretty(e.Lhs) + " ~ " + o.Pretty(e.Rhs)
	case *VarEntry:
		return o.Pretty(e.Var)
	case *QuantifiedConstraint:
		return prettyQuantifiedConstraint(o, e)
	}
	return "<?>"
}

func prettyClassEntry(o *TypeOps, e *ClassEntry) string {
	items := make([]string, 0, 1+len(e.Args))
	items = append(items, e.ClassName)
	for _, a := range e.Args {
		items = append(items, prettyAtom(o, a))
	}
	return strings.Join(items, " ")
}

func prettyQuantifiedConstraint(o *TypeOps, qc *QuantifiedConstraint) string {
	var vars []string
	for _, v := range qc.Vars {
		if o.Equal(v.Kind, TypeOfTypes) {
			vars = append(vars, v.Name)
		} else {
			vars = append(vars, "("+v.Name+": "+o.PrettyTypeAsKind(v.Kind)+")")
		}
	}
	var result strings.Builder
	result.WriteString(`\` + strings.Join(vars, " ") + ". ")
	for _, c := range qc.Context {
		result.WriteString(prettyConstraintEntry(o, c) + " => ")
	}
	if qc.Head != nil {
		result.WriteString(prettyClassEntry(o, qc.Head))
	}
	return result.String()
}

// PrettyDisplay renders a type for IDE display (hover, completion).
// Unlike Pretty, it shows TySkolem as plain type variables without the # prefix.
func (o *TypeOps) PrettyDisplay(t Type) string {
	switch ty := t.(type) {
	case *TySkolem:
		return ty.Name
	case *TyArrow:
		from := o.PrettyDisplay(ty.From)
		if _, ok := ty.From.(*TyArrow); ok {
			from = "(" + from + ")"
		}
		return from + " -> " + o.PrettyDisplay(ty.To)
	case *TyApp:
		if s, ok := prettyTuple(ty, o.PrettyDisplay); ok {
			return s
		}
		return o.PrettyDisplay(ty.Fun) + " " + prettyDisplayAtom(o, ty.Arg)
	case *TyForall:
		vars, body := collectForalls(ty)
		return `\` + strings.Join(vars, " ") + ". " + o.PrettyDisplay(body)
	case *TyEvidence:
		return o.PrettyDisplay(ty.Constraints) + " => " + o.PrettyDisplay(ty.Body)
	default:
		return o.Pretty(t)
	}
}

func prettyDisplayAtom(o *TypeOps, t Type) string {
	switch ty := t.(type) {
	case *TyVar, *TyCon, *TyEvidenceRow, *TySkolem, *TyMeta, *TyError:
		return o.PrettyDisplay(t)
	case *TyApp:
		if s, ok := prettyTuple(ty, o.PrettyDisplay); ok {
			return s
		}
		return "(" + o.PrettyDisplay(t) + ")"
	default:
		return "(" + o.PrettyDisplay(t) + ")"
	}
}

// PrettyTypeAsKind renders a type that represents a kind (level >= 1).
// Used for error messages and diagnostics during/after Type/Kind unification.
func (o *TypeOps) PrettyTypeAsKind(t Type) string {
	switch ty := t.(type) {
	case *TyCon:
		return ty.Name
	case *TyArrow:
		from := o.PrettyTypeAsKind(ty.From)
		if _, ok := ty.From.(*TyArrow); ok {
			from = "(" + from + ")"
		}
		return from + " -> " + o.PrettyTypeAsKind(ty.To)
	case *TyVar:
		return ty.Name
	case *TyMeta:
		return "_"
	default:
		return o.Pretty(t)
	}
}
