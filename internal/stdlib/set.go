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

var unitVal = &eval.RecordVal{Fields: map[string]eval.Value{}}

// _setEmpty :: (k -> k -> Ordering) -> Set k
func setEmptyImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	return &eval.HostVal{Inner: &mapVal{root: nil, cmp: cmp, size: 0}}, ce, nil
}

// _setInsert :: (k -> k -> Ordering) -> k -> Set k -> Set k
func setInsertImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	newRoot, newCe, err := avlInsert(m.root, key, unitVal, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mapVal{root: newRoot, cmp: m.cmp, size: avlSize(newRoot)}}, newCe, nil
}

// _setMember :: (k -> k -> Ordering) -> k -> Set k -> Bool
func setMemberImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	_, found, newCe, err := avlLookup(m.root, key, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	if found {
		return &eval.ConVal{Con: "True"}, newCe, nil
	}
	return &eval.ConVal{Con: "False"}, newCe, nil
}

// _setDelete :: (k -> k -> Ordering) -> k -> Set k -> Set k
func setDeleteImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
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
func setSizeImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(m.size)}, ce, nil
}

// _setToList :: Set k -> List k
func setToListImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	var keys []eval.Value
	collectKeys(m.root, &keys)
	return buildList(keys), ce, nil
}

func collectKeys(n *avlNode, acc *[]eval.Value) {
	if n == nil {
		return
	}
	collectKeys(n.left, acc)
	*acc = append(*acc, n.key)
	collectKeys(n.right, acc)
}

// _setFromList :: (k -> k -> Ordering) -> List k -> Set k
func setFromListImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
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
		var err error
		m.root, ce, err = avlInsert(m.root, con.Args[0], unitVal, cmp, ce, apply)
		if err != nil {
			return nil, ce, err
		}
		list = con.Args[1]
	}
	m.size = avlSize(m.root)
	return &eval.HostVal{Inner: m}, ce, nil
}
