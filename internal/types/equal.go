package types

// Equal checks structural equality of two types.
// For rows: compares normalized forms (label order irrelevant).
// For forall: alpha-equivalence (bound variable names irrelevant).
func Equal(a, b Type) bool {
	return equalAlpha(a, b, nil)
}

type alphaBinding struct {
	left, right string
}

func equalAlpha(a, b Type, bindings []alphaBinding) bool {
	switch at := a.(type) {
	case *TyVar:
		bt, ok := b.(*TyVar)
		if !ok {
			return false
		}
		// Check alpha bindings.
		for i := len(bindings) - 1; i >= 0; i-- {
			if bindings[i].left == at.Name {
				return bindings[i].right == bt.Name
			}
			if bindings[i].right == bt.Name {
				return false
			}
		}
		return at.Name == bt.Name

	case *TyCon:
		bt, ok := b.(*TyCon)
		return ok && at.Name == bt.Name

	case *TyApp:
		bt, ok := b.(*TyApp)
		if !ok {
			return false
		}
		return equalAlpha(at.Fun, bt.Fun, bindings) && equalAlpha(at.Arg, bt.Arg, bindings)

	case *TyArrow:
		bt, ok := b.(*TyArrow)
		if !ok {
			return false
		}
		return equalAlpha(at.From, bt.From, bindings) && equalAlpha(at.To, bt.To, bindings)

	case *TyForall:
		bt, ok := b.(*TyForall)
		if !ok {
			return false
		}
		if !at.Kind.Equal(bt.Kind) {
			return false
		}
		newBindings := append(bindings, alphaBinding{at.Var, bt.Var})
		return equalAlpha(at.Body, bt.Body, newBindings)

	case *TyComp:
		bt, ok := b.(*TyComp)
		if !ok {
			return false
		}
		return equalAlpha(at.Pre, bt.Pre, bindings) &&
			equalAlpha(at.Post, bt.Post, bindings) &&
			equalAlpha(at.Result, bt.Result, bindings)

	case *TyThunk:
		bt, ok := b.(*TyThunk)
		if !ok {
			return false
		}
		return equalAlpha(at.Pre, bt.Pre, bindings) &&
			equalAlpha(at.Post, bt.Post, bindings) &&
			equalAlpha(at.Result, bt.Result, bindings)

	case *TyRow:
		bt, ok := b.(*TyRow)
		if !ok {
			return false
		}
		return equalAlpha(at.ToEvidence(), bt.ToEvidence(), bindings)

	case *TyConstraintRow:
		bt, ok := b.(*TyConstraintRow)
		if !ok {
			return false
		}
		return equalAlpha(at.ToEvidence(), bt.ToEvidence(), bindings)

	case *TyEvidenceRow:
		bt, ok := b.(*TyEvidenceRow)
		if !ok {
			return false
		}
		switch aEntries := at.Entries.(type) {
		case *CapabilityEntries:
			bEntries, ok := bt.Entries.(*CapabilityEntries)
			if !ok {
				return false
			}
			an := EvNormalize(&TyEvidenceRow{Entries: aEntries, Tail: at.Tail})
			bn := EvNormalize(&TyEvidenceRow{Entries: bEntries, Tail: bt.Tail})
			aFields := an.CapFields()
			bFields := bn.CapFields()
			if len(aFields) != len(bFields) {
				return false
			}
			for i := range aFields {
				if aFields[i].Label != bFields[i].Label {
					return false
				}
				if !equalAlpha(aFields[i].Type, bFields[i].Type, bindings) {
					return false
				}
			}
			if (an.Tail == nil) != (bn.Tail == nil) {
				return false
			}
			if an.Tail != nil {
				return equalAlpha(an.Tail, bn.Tail, bindings)
			}
			return true
		case *ConstraintEntries:
			bEntries, ok := bt.Entries.(*ConstraintEntries)
			if !ok {
				return false
			}
			an := EvNormalizeConstraintEntries(&TyEvidenceRow{Entries: aEntries, Tail: at.Tail})
			bn := EvNormalizeConstraintEntries(&TyEvidenceRow{Entries: bEntries, Tail: bt.Tail})
			aCons := an.ConEntries()
			bCons := bn.ConEntries()
			if len(aCons) != len(bCons) {
				return false
			}
			for i := range aCons {
				if !equalConstraintEntry(aCons[i], bCons[i], bindings) {
					return false
				}
			}
			if (an.Tail == nil) != (bn.Tail == nil) {
				return false
			}
			if an.Tail != nil {
				return equalAlpha(an.Tail, bn.Tail, bindings)
			}
			return true
		default:
			return false
		}

	case *TyEvidence:
		bt, ok := b.(*TyEvidence)
		if !ok {
			return false
		}
		if !equalAlpha(at.Constraints, bt.Constraints, bindings) {
			return false
		}
		return equalAlpha(at.Body, bt.Body, bindings)

	case *TySkolem:
		bt, ok := b.(*TySkolem)
		return ok && at.ID == bt.ID

	case *TyMeta:
		bt, ok := b.(*TyMeta)
		return ok && at.ID == bt.ID

	case *TyError:
		_, ok := b.(*TyError)
		return ok

	default:
		return false
	}
}

func equalConstraintEntry(a, b ConstraintEntry, bindings []alphaBinding) bool {
	if a.ClassName != b.ClassName {
		return false
	}
	if len(a.Args) != len(b.Args) {
		return false
	}
	for j := range a.Args {
		if !equalAlpha(a.Args[j], b.Args[j], bindings) {
			return false
		}
	}
	// ConstraintVar: both must match.
	if (a.ConstraintVar == nil) != (b.ConstraintVar == nil) {
		return false
	}
	if a.ConstraintVar != nil {
		if !equalAlpha(a.ConstraintVar, b.ConstraintVar, bindings) {
			return false
		}
	}
	// Both must be quantified or both simple.
	if (a.Quantified == nil) != (b.Quantified == nil) {
		return false
	}
	if a.Quantified != nil {
		return equalQuantifiedConstraint(a.Quantified, b.Quantified, bindings)
	}
	return true
}

func equalQuantifiedConstraint(a, b *QuantifiedConstraint, bindings []alphaBinding) bool {
	if len(a.Vars) != len(b.Vars) {
		return false
	}
	// Extend bindings with alpha-equivalence for bound variables.
	newBindings := make([]alphaBinding, len(bindings), len(bindings)+len(a.Vars))
	copy(newBindings, bindings)
	for i, av := range a.Vars {
		if !av.Kind.Equal(b.Vars[i].Kind) {
			return false
		}
		newBindings = append(newBindings, alphaBinding{av.Name, b.Vars[i].Name})
	}
	if len(a.Context) != len(b.Context) {
		return false
	}
	for i := range a.Context {
		if !equalConstraintEntry(a.Context[i], b.Context[i], newBindings) {
			return false
		}
	}
	return equalConstraintEntry(a.Head, b.Head, newBindings)
}
