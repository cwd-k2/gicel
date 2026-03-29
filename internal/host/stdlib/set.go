package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Set provides an immutable ordered set backed by Map k ().
var Set Pack = func(e Registrar) error {
	e.RegisterPrim("_setEmpty", setEmptyImpl)
	e.RegisterPrim("_setInsert", setInsertImpl)
	e.RegisterPrim("_setMember", setMemberImpl)
	e.RegisterPrim("_setDelete", setDeleteImpl)
	e.RegisterPrim("_setSize", setSizeImpl)
	e.RegisterPrim("_setToList", setToListImpl)
	e.RegisterPrim("_setFromList", setFromListImpl)
	e.RegisterPrim("_setUnion", setUnionImpl)
	e.RegisterPrim("_setIntersection", setIntersectionImpl)
	e.RegisterPrim("_setDifference", setDifferenceImpl)
	e.RegisterPrim("_setIsSubsetOf", setIsSubsetOfImpl)
	e.RegisterPrim("_setFindMin", setFindMinImpl)
	e.RegisterPrim("_setFindMax", setFindMaxImpl)
	return e.RegisterModule("Data.Set", setSource)
}

var setSource = mustReadSource("set")

// _setEmpty :: (k -> k -> Ordering) -> Set k
func setEmptyImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	return &eval.HostVal{Inner: &mapVal{root: nil, cmp: cmp, size: 0}}, ce, nil
}

// _setInsert :: (k -> k -> Ordering) -> k -> Set k -> Set k
func setInsertImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
		return nil, ce, err
	}
	newRoot, inserted, newCe, err := avlInsert(m.root, key, unitVal, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	newSize := m.size
	if inserted {
		newSize++
	}
	return &eval.HostVal{Inner: &mapVal{root: newRoot, cmp: m.cmp, size: newSize}}, newCe, nil
}

// _setMember :: (k -> k -> Ordering) -> k -> Set k -> Bool
// Set is backed by Map k (), so member is identical to mapMember.
var setMemberImpl = mapMemberImpl

// _setDelete :: (k -> k -> Ordering) -> k -> Set k -> Set k
func setDeleteImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
		return nil, ce, err
	}
	newRoot, deleted, newCe, err := avlDelete(m.root, key, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	newSize := m.size
	if deleted {
		newSize--
	}
	return &eval.HostVal{Inner: &mapVal{root: newRoot, cmp: m.cmp, size: newSize}}, newCe, nil
}

// _setSize :: Set k -> Int
// Set is backed by Map k (), so size is identical to mapSize.
var setSizeImpl = mapSizeImpl

// _setToList :: Set k -> List k
func setToListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costConsNode); err != nil {
		return nil, ce, err
	}
	return avlKeysToConsList(m.root), ce, nil
}

// _setFromList :: (k -> k -> Ordering) -> List k -> Set k
func setFromListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	list := args[1]
	m := &mapVal{root: nil, cmp: cmp, size: 0}
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("setFromList", "List", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("setFromList", "list node", con.Con)
		}
		if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
			return nil, ce, err
		}
		var err error
		var inserted bool
		m.root, inserted, ce, err = avlInsert(m.root, con.Args[0], unitVal, cmp, ce, apply)
		if err != nil {
			return nil, ce, err
		}
		if inserted {
			m.size++
		}
		list = con.Args[1]
	}
	return &eval.HostVal{Inner: m}, ce, nil
}

// _setUnion :: (k -> k -> Ordering) -> Set k -> Set k -> Set k
// O(n₁+n₂) merge-join.
func setUnionImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	s1, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	s2, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	entries, ce, err := avlMergeUnion(s1.root, s2.root, s1.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mapVal{root: sortedToAVL(entries), cmp: s1.cmp, size: len(entries)}}, ce, nil
}

// _setIntersection :: (k -> k -> Ordering) -> Set k -> Set k -> Set k
// O(n₁+n₂) merge-join.
func setIntersectionImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	s1, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	s2, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	entries, ce, err := avlMergeIntersect(s1.root, s2.root, s1.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mapVal{root: sortedToAVL(entries), cmp: s1.cmp, size: len(entries)}}, ce, nil
}

// _setDifference :: (k -> k -> Ordering) -> Set k -> Set k -> Set k
// O(n₁+n₂) merge-join.
func setDifferenceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	s1, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	s2, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	entries, ce, err := avlMergeDifference(s1.root, s2.root, s1.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mapVal{root: sortedToAVL(entries), cmp: s1.cmp, size: len(entries)}}, ce, nil
}

// _setIsSubsetOf :: (k -> k -> Ordering) -> Set k -> Set k -> Bool
func setIsSubsetOfImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	s1, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	s2, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	if s1.size > s2.size {
		return boolVal(false), ce, nil
	}
	result, newCe, err := avlAllMember(s1.root, s2, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return boolVal(result), newCe, nil
}

// avlAllMember checks that every key in n is present in target.
func avlAllMember(n *avlNode, target *mapVal, ce eval.CapEnv, apply eval.Applier) (bool, eval.CapEnv, error) {
	if n == nil {
		return true, ce, nil
	}
	ok, newCe, err := avlAllMember(n.left, target, ce, apply)
	if err != nil || !ok {
		return false, ce, err
	}
	_, found, newCe, err := avlLookup(target.root, n.key, target.cmp, newCe, apply)
	if err != nil {
		return false, ce, err
	}
	if !found {
		return false, newCe, nil
	}
	return avlAllMember(n.right, target, newCe, apply)
}

// _setFindMin :: Set k -> Maybe k
func setFindMinImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if m.root == nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	n := avlMinNode(m.root)
	return &eval.ConVal{Con: "Just", Args: []eval.Value{n.key}}, ce, nil
}

// _setFindMax :: Set k -> Maybe k
func setFindMaxImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if m.root == nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	n := avlMaxNode(m.root)
	return &eval.ConVal{Con: "Just", Args: []eval.Value{n.key}}, ce, nil
}
