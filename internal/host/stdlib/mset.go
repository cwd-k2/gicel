package stdlib

import (
	"context"
	"errors"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// EffectSet provides mutable ordered sets gated by the { mset: () } effect.
// Backed by mutMapVal (MMap k ()) internally — mirrors Data.Set backing on mapVal.
var EffectSet Pack = func(e Registrar) error {
	e.RegisterPrim("_msetNew", msetNewImpl)
	e.RegisterPrim("_msetInsert", msetInsertImpl)
	e.RegisterPrim("_msetMember", msetMemberImpl)
	e.RegisterPrim("_msetDelete", msetDeleteImpl)
	e.RegisterPrim("_msetSize", msetSizeImpl)
	e.RegisterPrim("_msetToList", msetToListImpl)
	e.RegisterPrim("_msetFromList", msetFromListImpl)
	e.RegisterPrim("_msetFold", msetFoldImpl)
	e.RegisterPrim("_msetUnion", msetUnionImpl)
	e.RegisterPrim("_msetIntersection", msetIntersectionImpl)
	e.RegisterPrim("_msetDifference", msetDifferenceImpl)
	return e.RegisterModule("Effect.Set", msetSource)
}

var msetSource = mustReadSource("mset")

func asMutSetVal(v eval.Value) (*mutMapVal, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, errExpected("stdlib/mset", "HostVal", v)
	}
	m, ok := hv.Inner.(*mutMapVal)
	if !ok {
		return nil, errExpected("stdlib/mset", "*mutMapVal", hv.Inner)
	}
	return m, nil
}

// _msetNew :: (k -> k -> Ordering) -> MSet k
func msetNewImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	return &eval.HostVal{Inner: &mutMapVal{root: nil, cmp: cmp, size: 0}}, ce, nil
}

// _msetInsert :: (k -> k -> Ordering) -> k -> MSet k -> ()
func msetInsertImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMutSetVal(args[2])
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
	m.root = newRoot
	if inserted {
		m.size++
	}
	return unitVal, newCe, nil
}

// _msetMember :: (k -> k -> Ordering) -> k -> MSet k -> Bool
func msetMemberImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMutSetVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	_, found, newCe, err := avlLookup(m.root, key, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return boolVal(found), newCe, nil
}

// _msetDelete :: (k -> k -> Ordering) -> k -> MSet k -> ()
func msetDeleteImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMutSetVal(args[2])
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
	m.root = newRoot
	if deleted {
		m.size--
	}
	return unitVal, newCe, nil
}

// _msetSize :: MSet k -> Int
func msetSizeImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMutSetVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(m.size)}, ce, nil
}

// _msetToList :: MSet k -> List k
func msetToListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMutSetVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costConsNode); err != nil {
		return nil, ce, err
	}
	return avlKeysToConsList(m.root), ce, nil
}

// _msetFromList :: (k -> k -> Ordering) -> List k -> MSet k
func msetFromListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	list := args[1]
	m := &mutMapVal{root: nil, cmp: cmp, size: 0}
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("msetFromList", "List", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errors.New("msetFromList: malformed list")
		}
		if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
			return nil, ce, err
		}
		var inserted bool
		var err error
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

// _msetFold :: (b -> k -> b) -> b -> MSet k -> b
func msetFoldImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	acc := args[1]
	m, err := asMutSetVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	// Wrap f to ignore the unit value: (\acc k _ -> f acc k)
	wrapper := func(node *avlNode, accum eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
		partial, newCe, err := apply(f, accum, capEnv)
		if err != nil {
			return nil, capEnv, err
		}
		return apply(partial, node.key, newCe)
	}
	return avlFoldl(m.root, wrapper, acc, ce)
}

// _msetUnion :: (k -> k -> Ordering) -> MSet k -> MSet k -> MSet k
// O(n₁+n₂) merge-join: flatten both trees, merge sorted slices, rebuild.
func msetUnionImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	a, err := asMutSetVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	b, err := asMutSetVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	entries, ce, err := avlMergeUnion(a.root, b.root, cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mutMapVal{root: sortedToAVL(entries), cmp: cmp, size: len(entries)}}, ce, nil
}

// _msetIntersection :: (k -> k -> Ordering) -> MSet k -> MSet k -> MSet k
// O(n₁+n₂) merge-join.
func msetIntersectionImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	a, err := asMutSetVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	b, err := asMutSetVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	entries, ce, err := avlMergeIntersect(a.root, b.root, cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mutMapVal{root: sortedToAVL(entries), cmp: cmp, size: len(entries)}}, ce, nil
}

// _msetDifference :: (k -> k -> Ordering) -> MSet k -> MSet k -> MSet k
// O(n₁+n₂) merge-join.
func msetDifferenceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	a, err := asMutSetVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	b, err := asMutSetVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	entries, ce, err := avlMergeDifference(a.root, b.root, cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mutMapVal{root: sortedToAVL(entries), cmp: cmp, size: len(entries)}}, ce, nil
}

// avlFoldl performs an in-order traversal with a Go-level fold function.
func avlFoldl(n *avlNode, f func(*avlNode, eval.Value, eval.CapEnv) (eval.Value, eval.CapEnv, error), acc eval.Value, ce eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	if n == nil {
		return acc, ce, nil
	}
	var err error
	acc, ce, err = avlFoldl(n.left, f, acc, ce)
	if err != nil {
		return nil, ce, err
	}
	acc, ce, err = f(n, acc, ce)
	if err != nil {
		return nil, ce, err
	}
	return avlFoldl(n.right, f, acc, ce)
}
