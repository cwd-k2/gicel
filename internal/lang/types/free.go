package types

// FreeVars returns the set of free type/row variables in a type.
func FreeVars(t Type) map[string]struct{} {
	fv := make(map[string]struct{})
	freeVarsRec(t, nil, fv, 0)
	return fv
}

func freeVarsRec(t Type, bound map[string]bool, fv map[string]struct{}, depth int) {
	if depth > maxTraversalDepth {
		return
	}
	switch ty := t.(type) {
	case *TyVar:
		if bound == nil || !bound[ty.Name] {
			fv[ty.Name] = struct{}{}
		}
	case *TyCon:
		// no free vars
	case *TyApp:
		freeVarsRec(ty.Fun, bound, fv, depth+1)
		freeVarsRec(ty.Arg, bound, fv, depth+1)
	case *TyArrow:
		freeVarsRec(ty.From, bound, fv, depth+1)
		freeVarsRec(ty.To, bound, fv, depth+1)
	case *TyForall:
		newBound := make(map[string]bool, len(bound)+1)
		for k, v := range bound {
			newBound[k] = v
		}
		newBound[ty.Var] = true
		freeVarsRec(ty.Body, newBound, fv, depth+1)
	case *TyCBPV:
		freeVarsRec(ty.Pre, bound, fv, depth+1)
		freeVarsRec(ty.Post, bound, fv, depth+1)
		freeVarsRec(ty.Result, bound, fv, depth+1)
	case *TyEvidenceRow:
		switch entries := ty.Entries.(type) {
		case *CapabilityEntries:
			for _, f := range entries.Fields {
				freeVarsRec(f.Type, bound, fv, depth+1)
				for _, g := range f.Grades {
					freeVarsRec(g, bound, fv, depth+1)
				}
			}
		case *ConstraintEntries:
			for _, e := range entries.Entries {
				freeVarsConstraintEntry(e, bound, fv, depth+1)
			}
		}
		if ty.Tail != nil {
			freeVarsRec(ty.Tail, bound, fv, depth+1)
		}
	case *TyEvidence:
		freeVarsRec(ty.Constraints, bound, fv, depth+1)
		freeVarsRec(ty.Body, bound, fv, depth+1)
	case *TyFamilyApp:
		for _, a := range ty.Args {
			freeVarsRec(a, bound, fv, depth+1)
		}
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
func freeVarsConstraintEntry(e ConstraintEntry, bound map[string]bool, fv map[string]struct{}, depth int) {
	if depth > maxTraversalDepth {
		return
	}
	for _, a := range e.Args {
		freeVarsRec(a, bound, fv, depth+1)
	}
	if e.ConstraintVar != nil {
		freeVarsRec(e.ConstraintVar, bound, fv, depth+1)
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
			freeVarsConstraintEntry(c, newBound, fv, depth+1)
		}
		freeVarsConstraintEntry(e.Quantified.Head, newBound, fv, depth+1)
	}
}
