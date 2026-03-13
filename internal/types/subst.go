package types

import (
	"fmt"
	"sync/atomic"
)

var freshCounter int64

func freshName(base string) string {
	n := atomic.AddInt64(&freshCounter, 1)
	return fmt.Sprintf("%s$%d", base, n)
}

// Subst applies a substitution [varName := replacement] throughout a type.
func Subst(t Type, varName string, replacement Type) Type {
	switch ty := t.(type) {
	case *TyVar:
		if ty.Name == varName {
			return replacement
		}
		return ty

	case *TyCon:
		return ty

	case *TyApp:
		newFun := Subst(ty.Fun, varName, replacement)
		newArg := Subst(ty.Arg, varName, replacement)
		if newFun == ty.Fun && newArg == ty.Arg {
			return ty
		}
		return &TyApp{Fun: newFun, Arg: newArg, S: ty.S}

	case *TyArrow:
		newFrom := Subst(ty.From, varName, replacement)
		newTo := Subst(ty.To, varName, replacement)
		if newFrom == ty.From && newTo == ty.To {
			return ty
		}
		return &TyArrow{From: newFrom, To: newTo, S: ty.S}

	case *TyForall:
		if ty.Var == varName {
			return ty // shadowed
		}
		// Capture avoidance.
		if OccursIn(ty.Var, replacement) {
			fresh := freshName(ty.Var)
			body := Subst(ty.Body, ty.Var, &TyVar{Name: fresh})
			body = Subst(body, varName, replacement)
			return &TyForall{Var: fresh, Kind: ty.Kind, Body: body, S: ty.S}
		}
		newBody := Subst(ty.Body, varName, replacement)
		if newBody == ty.Body {
			return ty
		}
		return &TyForall{Var: ty.Var, Kind: ty.Kind, Body: newBody, S: ty.S}

	case *TyComp:
		newPre := Subst(ty.Pre, varName, replacement)
		newPost := Subst(ty.Post, varName, replacement)
		newResult := Subst(ty.Result, varName, replacement)
		if newPre == ty.Pre && newPost == ty.Post && newResult == ty.Result {
			return ty
		}
		return &TyComp{Pre: newPre, Post: newPost, Result: newResult, S: ty.S}

	case *TyThunk:
		newPre := Subst(ty.Pre, varName, replacement)
		newPost := Subst(ty.Post, varName, replacement)
		newResult := Subst(ty.Result, varName, replacement)
		if newPre == ty.Pre && newPost == ty.Post && newResult == ty.Result {
			return ty
		}
		return &TyThunk{Pre: newPre, Post: newPost, Result: newResult, S: ty.S}

	case *TyRow:
		changed := false
		fields := make([]RowField, len(ty.Fields))
		for i, f := range ty.Fields {
			newT := Subst(f.Type, varName, replacement)
			if newT != f.Type {
				changed = true
			}
			fields[i] = RowField{Label: f.Label, Type: newT, S: f.S}
		}
		var newTail Type
		if ty.Tail != nil {
			newTail = Subst(ty.Tail, varName, replacement)
			if newTail != ty.Tail {
				changed = true
			}
		}
		if !changed {
			return ty
		}
		return &TyRow{Fields: fields, Tail: newTail, S: ty.S}

	case *TyConstraintRow:
		changed := false
		entries := make([]ConstraintEntry, len(ty.Entries))
		for i, e := range ty.Entries {
			entries[i] = substConstraintEntry(e, varName, replacement, &changed)
		}
		var newTail Type
		if ty.Tail != nil {
			newTail = Subst(ty.Tail, varName, replacement)
			if newTail != ty.Tail {
				changed = true
			}
		}
		if !changed {
			return ty
		}
		return &TyConstraintRow{Entries: entries, Tail: newTail, S: ty.S}

	case *TyEvidence:
		newConstraints := Subst(ty.Constraints, varName, replacement)
		newBody := Subst(ty.Body, varName, replacement)
		if newConstraints == ty.Constraints && newBody == ty.Body {
			return ty
		}
		cr, ok := newConstraints.(*TyConstraintRow)
		if !ok {
			// Subst produced a non-constraint-row; preserve original to avoid nil.
			return &TyEvidence{Constraints: ty.Constraints, Body: newBody, S: ty.S}
		}
		return &TyEvidence{Constraints: cr, Body: newBody, S: ty.S}

	case *TySkolem:
		return ty

	case *TyEvidenceRow:
		switch entries := ty.Entries.(type) {
		case *CapabilityEntries:
			changed := false
			fields := make([]RowField, len(entries.Fields))
			for i, f := range entries.Fields {
				newT := Subst(f.Type, varName, replacement)
				if newT != f.Type {
					changed = true
				}
				fields[i] = RowField{Label: f.Label, Type: newT, S: f.S}
			}
			var newTail Type
			if ty.Tail != nil {
				newTail = Subst(ty.Tail, varName, replacement)
				if newTail != ty.Tail {
					changed = true
				}
			}
			if !changed {
				return ty
			}
			return &TyEvidenceRow{Entries: &CapabilityEntries{Fields: fields}, Tail: newTail, S: ty.S}
		case *ConstraintEntries:
			changed := false
			ces := make([]ConstraintEntry, len(entries.Entries))
			for i, e := range entries.Entries {
				ces[i] = substConstraintEntry(e, varName, replacement, &changed)
			}
			var newTail Type
			if ty.Tail != nil {
				newTail = Subst(ty.Tail, varName, replacement)
				if newTail != ty.Tail {
					changed = true
				}
			}
			if !changed {
				return ty
			}
			return &TyEvidenceRow{Entries: &ConstraintEntries{Entries: ces}, Tail: newTail, S: ty.S}
		default:
			return ty
		}

	case *TyMeta:
		return ty

	case *TyError:
		return ty

	default:
		return ty
	}
}

// SubstMany applies multiple substitutions simultaneously.
func SubstMany(t Type, subs map[string]Type) Type {
	result := t
	for name, repl := range subs {
		result = Subst(result, name, repl)
	}
	return result
}

// substConstraintEntry substitutes within a single ConstraintEntry,
// handling the Quantified field with proper variable shadowing.
func substConstraintEntry(e ConstraintEntry, varName string, replacement Type, changed *bool) ConstraintEntry {
	args := make([]Type, len(e.Args))
	for j, a := range e.Args {
		newA := Subst(a, varName, replacement)
		if newA != a {
			*changed = true
		}
		args[j] = newA
	}
	result := ConstraintEntry{ClassName: e.ClassName, Args: args, S: e.S}
	if e.ConstraintVar != nil {
		newCV := Subst(e.ConstraintVar, varName, replacement)
		if newCV != e.ConstraintVar {
			*changed = true
		}
		result.ConstraintVar = newCV
	}
	if e.Quantified != nil {
		// Check if varName is shadowed by any quantified variable.
		for _, v := range e.Quantified.Vars {
			if v.Name == varName {
				result.Quantified = e.Quantified // shadowed, no substitution inside
				return result
			}
		}
		newQC := substQuantifiedConstraint(e.Quantified, varName, replacement, changed)
		result.Quantified = newQC
	}
	return result
}

func substQuantifiedConstraint(qc *QuantifiedConstraint, varName string, replacement Type, changed *bool) *QuantifiedConstraint {
	// Capture avoidance: check if any bound var appears free in replacement.
	vars := make([]ForallBinder, len(qc.Vars))
	copy(vars, qc.Vars)
	for i, v := range vars {
		if OccursIn(v.Name, replacement) {
			fresh := freshName(v.Name)
			// Rename this bound var in context and head.
			vars[i] = ForallBinder{Name: fresh, Kind: v.Kind}
			*changed = true
		}
	}
	ctx := make([]ConstraintEntry, len(qc.Context))
	for i, c := range qc.Context {
		ctx[i] = substConstraintEntry(c, varName, replacement, changed)
	}
	head := substConstraintEntry(qc.Head, varName, replacement, changed)
	// Also apply renames from capture avoidance.
	for i, orig := range qc.Vars {
		if vars[i].Name != orig.Name {
			for j := range ctx {
				ctx[j] = renameInConstraintEntry(ctx[j], orig.Name, vars[i].Name)
			}
			head = renameInConstraintEntry(head, orig.Name, vars[i].Name)
		}
	}
	return &QuantifiedConstraint{Vars: vars, Context: ctx, Head: head}
}

func renameInConstraintEntry(e ConstraintEntry, oldName, newName string) ConstraintEntry {
	changed := false
	args := make([]Type, len(e.Args))
	for j, a := range e.Args {
		args[j] = Subst(a, oldName, &TyVar{Name: newName})
		if args[j] != a {
			changed = true
		}
	}
	if !changed {
		return e
	}
	return ConstraintEntry{ClassName: e.ClassName, Args: args, Quantified: e.Quantified, S: e.S}
}
