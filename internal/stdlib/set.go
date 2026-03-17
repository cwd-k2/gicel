package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
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
	return e.RegisterModule("Std.Set", setSource)
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
	if err := eval.ChargeAlloc(ctx, costAVLNode); err != nil {
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
	if err := eval.ChargeAlloc(ctx, costAVLNode); err != nil {
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
	if err := eval.ChargeAlloc(ctx, int64(m.size)*costConsNode); err != nil {
		return nil, ce, err
	}
	return avlKeysToConsList(m.root), ce, nil
}

// avlKeysToConsList builds an in-order ConVal list of keys from the AVL tree.
func avlKeysToConsList(n *avlNode) eval.Value {
	var acc eval.Value = &eval.ConVal{Con: "Nil"}
	avlKeysConsRight(n, &acc)
	return acc
}

func avlKeysConsRight(n *avlNode, acc *eval.Value) {
	if n == nil {
		return
	}
	avlKeysConsRight(n.right, acc)
	*acc = &eval.ConVal{Con: "Cons", Args: []eval.Value{n.key, *acc}}
	avlKeysConsRight(n.left, acc)
}

// _setFromList :: (k -> k -> Ordering) -> List k -> Set k
func setFromListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	list := args[1]
	m := &mapVal{root: nil, cmp: cmp, size: 0}
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("setFromList: expected List, got %T", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("setFromList: malformed list")
		}
		if err := eval.ChargeAlloc(ctx, costAVLNode); err != nil {
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
