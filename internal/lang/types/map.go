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
func (o *TypeOps) MapType(t Type, f func(Type) Type) Type {
	switch ty := t.(type) {
	case *TyApp:
		fun := f(ty.Fun)
		arg := f(ty.Arg)
		if fun == ty.Fun && arg == ty.Arg {
			return t
		}
		if ty.IsGrade {
			return o.AppGrade(fun, arg, ty.S)
		}
		return o.App(fun, arg, ty.S)
	case *TyArrow:
		from := f(ty.From)
		to := f(ty.To)
		if from == ty.From && to == ty.To {
			return t
		}
		return o.Arrow(from, to, ty.S)
	case *TyForall:
		kind := f(ty.Kind)
		body := f(ty.Body)
		if kind == ty.Kind && body == ty.Body {
			return t
		}
		return o.Forall(ty.Var, kind, body, ty.S)
	case *TyCBPV:
		pre := f(ty.Pre)
		post := f(ty.Post)
		result := f(ty.Result)
		grade := ty.Grade
		if grade != nil {
			grade = f(grade)
		}
		if pre == ty.Pre && post == ty.Post && result == ty.Result && grade == ty.Grade {
			return t
		}
		if ty.Tag == TagThunk {
			return o.ThunkGraded(pre, post, result, grade, ty.S)
		}
		return o.Comp(pre, post, result, grade, ty.S)
	case *TyEvidence:
		constraints := f(ty.Constraints)
		body := f(ty.Body)
		if constraints == ty.Constraints && body == ty.Body {
			return t
		}
		cr, ok := constraints.(*TyEvidenceRow)
		if !ok {
			panic("MapType: TyEvidence.Constraints transformed to non-*TyEvidenceRow")
		}
		return &TyEvidence{Constraints: cr, Body: body, Flags: MetaFreeFlags(cr, body), S: ty.S}
	case *TyEvidenceRow:
		newEntries, changed := ty.Entries.MapChildren(func(child Type) Type {
			return f(child)
		})
		var tail Type
		if ty.IsOpen() {
			tail = f(ty.Tail)
			if tail != ty.Tail {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return &TyEvidenceRow{Entries: newEntries, Tail: tail, Flags: EvidenceRowFlags(newEntries, tail), S: ty.S}
	case *TyFamilyApp:
		kind := f(ty.Kind)
		var args []Type
		for i, a := range ty.Args {
			nA := f(a)
			if args == nil && nA != a {
				args = make([]Type, len(ty.Args))
				copy(args[:i], ty.Args[:i])
			}
			if args != nil {
				args[i] = nA
			}
		}
		if args == nil && kind == ty.Kind {
			return t
		}
		if args == nil {
			args = ty.Args
		}
		return o.FamilyApp(ty.Name, args, kind, ty.S)
	case *TyVar, *TyCon, *TyMeta, *TySkolem, *TyError:
		return t
	default:
		panic(unhandledTypeMsg("MapType", t))
	}
}

// AnyType returns true if pred holds for t or any descendant.
// Short-circuits on the first true result.
func AnyType(t Type, pred func(Type) bool) bool {
	return anyTypeDepth(t, pred, 0)
}

func anyTypeDepth(t Type, pred func(Type) bool, depth int) bool {
	if depth > maxTraversalDepth {
		depthExceeded()
	}
	if pred(t) {
		return true
	}
	found := false
	ForEachChild(t, func(ch Type) bool {
		if anyTypeDepth(ch, pred, depth+1) {
			found = true
			return false
		}
		return true
	})
	return found
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
		depthExceeded()
	}
	if v, ok := f(t); ok {
		*result = append(*result, v)
	}
	ForEachChild(t, func(ch Type) bool {
		collectTypesRec(ch, f, result, depth+1)
		return true
	})
}
