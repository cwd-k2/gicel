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
		return ok && at.Name == bt.Name && LevelEqual(at.Level, bt.Level)

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
		if !equalAlpha(at.Kind, bt.Kind, bindings) {
			return false
		}
		newBindings := append(bindings, alphaBinding{at.Var, bt.Var})
		return equalAlpha(at.Body, bt.Body, newBindings)

	case *TyCBPV:
		bt, ok := b.(*TyCBPV)
		if !ok || at.Tag != bt.Tag {
			return false
		}
		if !equalAlpha(at.Pre, bt.Pre, bindings) || !equalAlpha(at.Post, bt.Post, bindings) || !equalAlpha(at.Result, bt.Result, bindings) {
			return false
		}
		if at.Grade == nil && bt.Grade == nil {
			return true
		}
		if at.Grade == nil || bt.Grade == nil {
			return false
		}
		return equalAlpha(at.Grade, bt.Grade, bindings)

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
			// Cap rows are maintained in sorted order by construction,
			// so normalization is not needed for equality comparison.
			aFields := aEntries.Fields
			bFields := bEntries.Fields
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
				// Compare grade annotations.
				if len(aFields[i].Grades) != len(bFields[i].Grades) {
					return false
				}
				for j := range aFields[i].Grades {
					if !equalAlpha(aFields[i].Grades[j], bFields[i].Grades[j], bindings) {
						return false
					}
				}
			}
			if (at.Tail == nil) != (bt.Tail == nil) {
				return false
			}
			if at.Tail != nil {
				return equalAlpha(at.Tail, bt.Tail, bindings)
			}
			return true
		case *ConstraintEntries:
			bEntries, ok := bt.Entries.(*ConstraintEntries)
			if !ok {
				return false
			}
			// Defensive normalization: constraint rows may be constructed
			// directly without ExtendConstraint in tests or external code.
			an := NormalizeConstraints(&TyEvidenceRow{Entries: aEntries, Tail: at.Tail, Flags: EvidenceRowFlags(aEntries, at.Tail)})
			bn := NormalizeConstraints(&TyEvidenceRow{Entries: bEntries, Tail: bt.Tail, Flags: EvidenceRowFlags(bEntries, bt.Tail)})
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
			if (at.Tail == nil) != (bt.Tail == nil) {
				return false
			}
			if at.Tail != nil {
				return equalAlpha(at.Tail, bt.Tail, bindings)
			}
			return true
		default:
			// Generic fallback for future fiber types: compare via AllChildren.
			bChildren := bt.Entries.AllChildren()
			aChildren := at.Entries.AllChildren()
			if len(aChildren) != len(bChildren) {
				return false
			}
			for i := range aChildren {
				if !equalAlpha(aChildren[i], bChildren[i], bindings) {
					return false
				}
			}
			if (at.Tail == nil) != (bt.Tail == nil) {
				return false
			}
			if at.Tail != nil {
				return equalAlpha(at.Tail, bt.Tail, bindings)
			}
			return true
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

	case *TyFamilyApp:
		bt, ok := b.(*TyFamilyApp)
		if !ok || at.Name != bt.Name || len(at.Args) != len(bt.Args) {
			return false
		}
		for i := range at.Args {
			if !equalAlpha(at.Args[i], bt.Args[i], bindings) {
				return false
			}
		}
		return true

	case *TySkolem:
		bt, ok := b.(*TySkolem)
		return ok && at.ID == bt.ID

	case *TyMeta:
		bt, ok := b.(*TyMeta)
		return ok && at.ID == bt.ID

	case *TyError:
		// TyError is structurally equal only to another TyError.
		// This differs from Unify, where TyError absorbs any type (error recovery).
		// The distinction is intentional: Equal answers "are these the same type?"
		// (no — TyError is not Int), while Unify answers "can these coexist without
		// reporting a new error?" (yes — cascading errors are suppressed).
		// Making Equal treat TyError as universal would suppress grade violations,
		// corrupt type family pattern matching, and break row label deduplication.
		_, ok := b.(*TyError)
		return ok

	default:
		return false
	}
}

func equalConstraintEntry(a, b ConstraintEntry, bindings []alphaBinding) bool {
	switch av := a.(type) {
	case *ClassEntry:
		bv, ok := b.(*ClassEntry)
		if !ok {
			return false
		}
		return equalClassEntry(av, bv, bindings)
	case *EqualityEntry:
		bv, ok := b.(*EqualityEntry)
		if !ok {
			return false
		}
		return equalAlpha(av.Lhs, bv.Lhs, bindings) && equalAlpha(av.Rhs, bv.Rhs, bindings)
	case *VarEntry:
		bv, ok := b.(*VarEntry)
		if !ok {
			return false
		}
		return equalAlpha(av.Var, bv.Var, bindings)
	case *QuantifiedConstraint:
		bv, ok := b.(*QuantifiedConstraint)
		if !ok {
			return false
		}
		return equalQuantifiedConstraint(av, bv, bindings)
	}
	return false
}

func equalClassEntry(a, b *ClassEntry, bindings []alphaBinding) bool {
	if a.ClassName != b.ClassName || len(a.Args) != len(b.Args) {
		return false
	}
	for j := range a.Args {
		if !equalAlpha(a.Args[j], b.Args[j], bindings) {
			return false
		}
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
		if !equalAlpha(av.Kind, b.Vars[i].Kind, bindings) {
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
	if (a.Head == nil) != (b.Head == nil) {
		return false
	}
	if a.Head == nil {
		return true
	}
	return equalClassEntry(a.Head, b.Head, newBindings)
}
