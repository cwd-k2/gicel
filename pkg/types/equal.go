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
		an := Normalize(at)
		bn := Normalize(bt)
		if len(an.Fields) != len(bn.Fields) {
			return false
		}
		for i := range an.Fields {
			if an.Fields[i].Label != bn.Fields[i].Label {
				return false
			}
			if !equalAlpha(an.Fields[i].Type, bn.Fields[i].Type, bindings) {
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
