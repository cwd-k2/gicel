package types

// FreeVars returns the set of free type/row variables in a type.
func FreeVars(t Type) map[string]struct{} {
	fv := make(map[string]struct{})
	freeVarsRec(t, nil, fv, 0)
	return fv
}

func freeVarsRec(t Type, bound map[string]bool, fv map[string]struct{}, depth int) {
	if depth > maxTraversalDepth {
		depthExceeded()
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
		// Push/pop the bound variable to avoid map copy.
		if bound == nil {
			bound = make(map[string]bool, 4)
		}
		prev := bound[ty.Var]
		bound[ty.Var] = true
		freeVarsRec(ty.Body, bound, fv, depth+1)
		if prev {
			bound[ty.Var] = prev
		} else {
			delete(bound, ty.Var)
		}
	case *TyCBPV:
		freeVarsRec(ty.Pre, bound, fv, depth+1)
		freeVarsRec(ty.Post, bound, fv, depth+1)
		freeVarsRec(ty.Result, bound, fv, depth+1)
		if ty.Grade != nil {
			freeVarsRec(ty.Grade, bound, fv, depth+1)
		}
	case *TyEvidenceRow:
		switch entries := ty.Entries.(type) {
		case *CapabilityEntries:
			for _, f := range entries.Fields {
				if f.IsLabelVar && (bound == nil || !bound[f.Label]) {
					fv[f.Label] = struct{}{}
				}
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
		depthExceeded()
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
			return occursIn(name, ty.Kind, bound, depth+1)
		}
		if occursIn(name, ty.Kind, bound, depth+1) {
			return true
		}
		if bound == nil {
			bound = make(map[string]bool, 4)
		}
		prev := bound[ty.Var]
		bound[ty.Var] = true
		result := occursIn(name, ty.Body, bound, depth+1)
		if prev {
			bound[ty.Var] = prev
		} else {
			delete(bound, ty.Var)
		}
		return result
	case *TyCBPV:
		return occursIn(name, ty.Pre, bound, depth+1) ||
			occursIn(name, ty.Post, bound, depth+1) ||
			occursIn(name, ty.Result, bound, depth+1) ||
			(ty.Grade != nil && occursIn(name, ty.Grade, bound, depth+1))
	case *TyEvidenceRow:
		switch entries := ty.Entries.(type) {
		case *CapabilityEntries:
			for _, f := range entries.Fields {
				// Check field label: when IsLabelVar is set, the label
				// originates from a label-kinded forall variable (e.g.
				// { l: () | r } where l is bound by \(l: Label)).
				// Without this check, OccursIn misses the occurrence
				// and Subst bails out early.
				if f.IsLabelVar && f.Label == name && (bound == nil || !bound[name]) {
					return true
				}
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
		depthExceeded()
	}
	switch e := e.(type) {
	case *ClassEntry:
		for _, a := range e.Args {
			if occursIn(name, a, bound, depth+1) {
				return true
			}
		}
		return false
	case *EqualityEntry:
		return occursIn(name, e.Lhs, bound, depth+1) || occursIn(name, e.Rhs, bound, depth+1)
	case *VarEntry:
		return occursIn(name, e.Var, bound, depth+1)
	case *QuantifiedConstraint:
		if bound == nil {
			bound = make(map[string]bool, len(e.Vars))
		}
		prevs := make([]bool, len(e.Vars))
		for i, v := range e.Vars {
			prevs[i] = bound[v.Name]
			bound[v.Name] = true
		}
		found := false
		for _, c := range e.Context {
			if occursInConstraintEntry(name, c, bound, depth+1) {
				found = true
				break
			}
		}
		if !found && e.Head != nil {
			for _, a := range e.Head.Args {
				if occursIn(name, a, bound, depth+1) {
					found = true
					break
				}
			}
		}
		for i, v := range e.Vars {
			if prevs[i] {
				bound[v.Name] = true
			} else {
				delete(bound, v.Name)
			}
		}
		return found
	}
	return false
}

// freeVarsConstraintEntry collects free vars from a constraint entry,
// respecting bound variables in quantified constraints.
func freeVarsConstraintEntry(e ConstraintEntry, bound map[string]bool, fv map[string]struct{}, depth int) {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	switch e := e.(type) {
	case *ClassEntry:
		for _, a := range e.Args {
			freeVarsRec(a, bound, fv, depth+1)
		}
	case *EqualityEntry:
		freeVarsRec(e.Lhs, bound, fv, depth+1)
		freeVarsRec(e.Rhs, bound, fv, depth+1)
	case *VarEntry:
		freeVarsRec(e.Var, bound, fv, depth+1)
	case *QuantifiedConstraint:
		if bound == nil {
			bound = make(map[string]bool, len(e.Vars))
		}
		// Push quantified variables.
		prevs := make([]bool, len(e.Vars))
		for i, v := range e.Vars {
			prevs[i] = bound[v.Name]
			bound[v.Name] = true
		}
		for _, c := range e.Context {
			freeVarsConstraintEntry(c, bound, fv, depth+1)
		}
		if e.Head != nil {
			for _, a := range e.Head.Args {
				freeVarsRec(a, bound, fv, depth+1)
			}
		}
		// Pop quantified variables.
		for i, v := range e.Vars {
			if prevs[i] {
				bound[v.Name] = true
			} else {
				delete(bound, v.Name)
			}
		}
	}
}

// ContainsSkolemOrFamily returns true if the type contains any TySkolem
// or TyFamilyApp node. Used to determine whether an equality constraint
// in evidence position should be treated as given (deferred) or wanted.
func ContainsSkolemOrFamily(t Type) bool {
	return AnyType(t, func(t Type) bool {
		switch t.(type) {
		case *TySkolem, *TyFamilyApp:
			return true
		}
		return false
	})
}
