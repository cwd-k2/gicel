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

	case *TyQual:
		changed := false
		args := make([]Type, len(ty.Args))
		for i, a := range ty.Args {
			newA := Subst(a, varName, replacement)
			if newA != a {
				changed = true
			}
			args[i] = newA
		}
		newBody := Subst(ty.Body, varName, replacement)
		if newBody != ty.Body {
			changed = true
		}
		if !changed {
			return ty
		}
		return &TyQual{ClassName: ty.ClassName, Args: args, Body: newBody, S: ty.S}

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
			args := make([]Type, len(e.Args))
			for j, a := range e.Args {
				newA := Subst(a, varName, replacement)
				if newA != a {
					changed = true
				}
				args[j] = newA
			}
			entries[i] = ConstraintEntry{ClassName: e.ClassName, Args: args, S: e.S}
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
		cr, _ := newConstraints.(*TyConstraintRow)
		return &TyEvidence{Constraints: cr, Body: newBody, S: ty.S}

	case *TySkolem:
		return ty

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
