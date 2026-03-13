package types

// FreeVars returns the set of free type/row variables in a type.
func FreeVars(t Type) map[string]struct{} {
	fv := make(map[string]struct{})
	freeVarsRec(t, nil, fv)
	return fv
}

func freeVarsRec(t Type, bound map[string]bool, fv map[string]struct{}) {
	switch ty := t.(type) {
	case *TyVar:
		if bound == nil || !bound[ty.Name] {
			fv[ty.Name] = struct{}{}
		}
	case *TyCon:
		// no free vars
	case *TyApp:
		freeVarsRec(ty.Fun, bound, fv)
		freeVarsRec(ty.Arg, bound, fv)
	case *TyArrow:
		freeVarsRec(ty.From, bound, fv)
		freeVarsRec(ty.To, bound, fv)
	case *TyForall:
		newBound := make(map[string]bool, len(bound)+1)
		for k, v := range bound {
			newBound[k] = v
		}
		newBound[ty.Var] = true
		freeVarsRec(ty.Body, newBound, fv)
	case *TyComp:
		freeVarsRec(ty.Pre, bound, fv)
		freeVarsRec(ty.Post, bound, fv)
		freeVarsRec(ty.Result, bound, fv)
	case *TyThunk:
		freeVarsRec(ty.Pre, bound, fv)
		freeVarsRec(ty.Post, bound, fv)
		freeVarsRec(ty.Result, bound, fv)
	case *TyRow:
		for _, f := range ty.Fields {
			freeVarsRec(f.Type, bound, fv)
		}
		if ty.Tail != nil {
			freeVarsRec(ty.Tail, bound, fv)
		}
	case *TyConstraintRow:
		for _, e := range ty.Entries {
			freeVarsConstraintEntry(e, bound, fv)
		}
		if ty.Tail != nil {
			freeVarsRec(ty.Tail, bound, fv)
		}
	case *TyEvidence:
		freeVarsRec(ty.Constraints, bound, fv)
		freeVarsRec(ty.Body, bound, fv)
	case *TySkolem, *TyMeta, *TyError:
		// no free vars
	}
}

// OccursIn checks if a variable name appears free in a type.
func OccursIn(name string, t Type) bool {
	fv := FreeVars(t)
	_, ok := fv[name]
	return ok
}

// freeVarsConstraintEntry collects free vars from a constraint entry,
// respecting bound variables in quantified constraints.
func freeVarsConstraintEntry(e ConstraintEntry, bound map[string]bool, fv map[string]struct{}) {
	for _, a := range e.Args {
		freeVarsRec(a, bound, fv)
	}
	if e.Quantified != nil {
		// Extend bound set with quantified variables.
		newBound := make(map[string]bool, len(bound)+len(e.Quantified.Vars))
		for k, v := range bound {
			newBound[k] = v
		}
		for _, v := range e.Quantified.Vars {
			newBound[v.Name] = true
		}
		for _, c := range e.Quantified.Context {
			freeVarsConstraintEntry(c, newBound, fv)
		}
		freeVarsConstraintEntry(e.Quantified.Head, newBound, fv)
	}
}
