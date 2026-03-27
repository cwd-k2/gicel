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
		freeVarsRec(ty.Kind, bound, fv, depth+1)
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
// Uses early-exit traversal to avoid building the full free variable set.
func OccursIn(name string, t Type) bool {
	return occursIn(name, t, nil, 0)
}

func occursIn(name string, t Type, bound map[string]bool, depth int) bool {
	if depth > maxTraversalDepth {
		return false
	}
	switch ty := t.(type) {
	case *TyVar:
		return ty.Name == name && (bound == nil || !bound[ty.Name])
	case *TyCon:
		return false
	case *TyApp:
		return occursIn(name, ty.Fun, bound, depth+1) || occursIn(name, ty.Arg, bound, depth+1)
	case *TyArrow:
		return occursIn(name, ty.From, bound, depth+1) || occursIn(name, ty.To, bound, depth+1)
	case *TyForall:
		if ty.Var == name {
			// Shadowed in body, but Kind is outside the binding scope.
			return occursIn(name, ty.Kind, bound, depth+1)
		}
		if occursIn(name, ty.Kind, bound, depth+1) {
			return true
		}
		if bound == nil {
			return occursIn(name, ty.Body, map[string]bool{ty.Var: true}, depth+1)
		}
		newBound := make(map[string]bool, len(bound)+1)
		for k, v := range bound {
			newBound[k] = v
		}
		newBound[ty.Var] = true
		return occursIn(name, ty.Body, newBound, depth+1)
	case *TyCBPV:
		return occursIn(name, ty.Pre, bound, depth+1) ||
			occursIn(name, ty.Post, bound, depth+1) ||
			occursIn(name, ty.Result, bound, depth+1)
	case *TyEvidenceRow:
		switch entries := ty.Entries.(type) {
		case *CapabilityEntries:
			for _, f := range entries.Fields {
				if occursIn(name, f.Type, bound, depth+1) {
					return true
				}
				for _, g := range f.Grades {
					if occursIn(name, g, bound, depth+1) {
						return true
					}
				}
			}
		case *ConstraintEntries:
			for _, e := range entries.Entries {
				if occursInConstraintEntry(name, e, bound, depth+1) {
					return true
				}
			}
		}
		if ty.Tail != nil {
			return occursIn(name, ty.Tail, bound, depth+1)
		}
		return false
	case *TyEvidence:
		return occursIn(name, ty.Constraints, bound, depth+1) ||
			occursIn(name, ty.Body, bound, depth+1)
	case *TyFamilyApp:
		for _, a := range ty.Args {
			if occursIn(name, a, bound, depth+1) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func occursInConstraintEntry(name string, e ConstraintEntry, bound map[string]bool, depth int) bool {
	if depth > maxTraversalDepth {
		return false
	}
	for _, a := range e.Args {
		if occursIn(name, a, bound, depth+1) {
			return true
		}
	}
	if e.ConstraintVar != nil && occursIn(name, e.ConstraintVar, bound, depth+1) {
		return true
	}
	if e.Quantified != nil {
		newBound := make(map[string]bool, len(bound)+len(e.Quantified.Vars))
		for k, v := range bound {
			newBound[k] = v
		}
		for _, v := range e.Quantified.Vars {
			newBound[v.Name] = true
		}
		for _, c := range e.Quantified.Context {
			if occursInConstraintEntry(name, c, newBound, depth+1) {
				return true
			}
		}
		if occursInConstraintEntry(name, e.Quantified.Head, newBound, depth+1) {
			return true
		}
	}
	return false
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
