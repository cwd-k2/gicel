package types

// MapType applies f to every direct child of t, reconstructing the node
// with the transformed children. Identity-preserving: if all children are
// pointer-equal after transformation, the original node is returned.
//
// f is called on each direct child — for deep transformation, f should
// call MapType recursively. Leaf nodes (TyVar, TyCon, TyMeta, TySkolem,
// TyError) have no children and are returned as-is.
//
// Note: Zonk intentionally does NOT use MapType because it requires
// path compression on TyMeta (mutating the solution map during traversal)
// and identity-preserving return via pointer equality checks. MapType is
// the correct default for all other structural traversals.
func MapType(t Type, f func(Type) Type) Type {
	switch ty := t.(type) {
	case *TyApp:
		fun := f(ty.Fun)
		arg := f(ty.Arg)
		if fun == ty.Fun && arg == ty.Arg {
			return t
		}
		return &TyApp{Fun: fun, Arg: arg, S: ty.S}
	case *TyArrow:
		from := f(ty.From)
		to := f(ty.To)
		if from == ty.From && to == ty.To {
			return t
		}
		return &TyArrow{From: from, To: to, S: ty.S}
	case *TyForall:
		body := f(ty.Body)
		if body == ty.Body {
			return t
		}
		return &TyForall{Var: ty.Var, Kind: ty.Kind, Body: body, S: ty.S}
	case *TyCBPV:
		pre := f(ty.Pre)
		post := f(ty.Post)
		result := f(ty.Result)
		if pre == ty.Pre && post == ty.Post && result == ty.Result {
			return t
		}
		return &TyCBPV{Tag: ty.Tag, Pre: pre, Post: post, Result: result, S: ty.S}
	case *TyEvidence:
		constraints := f(ty.Constraints)
		body := f(ty.Body)
		if constraints == ty.Constraints && body == ty.Body {
			return t
		}
		cr, ok := constraints.(*TyEvidenceRow)
		if !ok {
			cr = ty.Constraints
		}
		return &TyEvidence{Constraints: cr, Body: body, S: ty.S}
	case *TyEvidenceRow:
		changed := false
		newEntries := ty.Entries.MapChildren(func(child Type) Type {
			r := f(child)
			if r != child {
				changed = true
			}
			return r
		})
		var tail Type
		if ty.Tail != nil {
			tail = f(ty.Tail)
			if tail != ty.Tail {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return &TyEvidenceRow{Entries: newEntries, Tail: tail, S: ty.S}
	case *TyFamilyApp:
		changed := false
		args := make([]Type, len(ty.Args))
		for i, a := range ty.Args {
			args[i] = f(a)
			if args[i] != a {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return &TyFamilyApp{Name: ty.Name, Args: args, Kind: ty.Kind, S: ty.S}
	default:
		// TyVar, TyCon, TyMeta, TySkolem, TyError — leaves
		return t
	}
}

// AnyType returns true if pred holds for t or any descendant.
// Short-circuits on the first true result.
func AnyType(t Type, pred func(Type) bool) bool {
	return anyTypeDepth(t, pred, 0)
}

func anyTypeDepth(t Type, pred func(Type) bool, depth int) bool {
	if depth > maxTraversalDepth {
		return false
	}
	if pred(t) {
		return true
	}
	for _, ch := range t.Children() {
		if anyTypeDepth(ch, pred, depth+1) {
			return true
		}
	}
	return false
}

// CollectTypes traverses a type tree and collects values extracted by f.
// For each node where f returns (value, true), the value is appended to
// the result. The traversal is depth-first, pre-order.
func CollectTypes[T any](t Type, f func(Type) (T, bool)) []T {
	var result []T
	collectTypesRec(t, f, &result, 0)
	return result
}

func collectTypesRec[T any](t Type, f func(Type) (T, bool), result *[]T, depth int) {
	if depth > maxTraversalDepth {
		return
	}
	if v, ok := f(t); ok {
		*result = append(*result, v)
	}
	for _, ch := range t.Children() {
		collectTypesRec(ch, f, result, depth+1)
	}
}
