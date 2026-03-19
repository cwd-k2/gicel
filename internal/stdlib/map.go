package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/types"
)

// Map provides an immutable ordered map backed by an AVL tree.
// Comparison uses the GICEL Ord dictionary (compare function) via Applier.
var Map Pack = func(e Registrar) error {
	e.RegisterPrim("_mapEmpty", mapEmptyImpl)
	e.RegisterPrim("_mapInsert", mapInsertImpl)
	e.RegisterPrim("_mapLookup", mapLookupImpl)
	e.RegisterPrim("_mapDelete", mapDeleteImpl)
	e.RegisterPrim("_mapSize", mapSizeImpl)
	e.RegisterPrim("_mapToList", mapToListImpl)
	e.RegisterPrim("_mapFromList", mapFromListImpl)
	e.RegisterPrim("_mapFoldlWithKey", mapFoldlWithKeyImpl)
	e.RegisterPrim("_mapMember", mapMemberImpl)
	e.RegisterPrim("_mapUnionWith", mapUnionWithImpl)
	return e.RegisterModule("Data.Map", mapSource)
}

var mapSource = mustReadSource("map")

// --- Primitives ---

func asMapVal(v eval.Value) (*mapVal, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, fmt.Errorf("map: expected Map, got %T", v)
	}
	mv, ok := hv.Inner.(*mapVal)
	if !ok {
		return nil, fmt.Errorf("map: expected Map, got %T", hv.Inner)
	}
	return mv, nil
}

// _mapEmpty :: (k -> k -> Ordering) -> Map k v
func mapEmptyImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	return &eval.HostVal{Inner: &mapVal{root: nil, cmp: cmp, size: 0}}, ce, nil
}

// _mapInsert :: (k -> k -> Ordering) -> k -> v -> Map k v -> Map k v
func mapInsertImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	// args[0] is the compare function passed from GICEL; we use m.cmp instead
	// because the map stores its comparator at creation time.
	key := args[1]
	value := args[2]
	m, err := asMapVal(args[3])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
		return nil, ce, err
	}
	newRoot, inserted, newCe, err := avlInsert(m.root, key, value, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	newSize := m.size
	if inserted {
		newSize++
	}
	return &eval.HostVal{Inner: &mapVal{root: newRoot, cmp: m.cmp, size: newSize}}, newCe, nil
}

// _mapLookup :: (k -> k -> Ordering) -> k -> Map k v -> Maybe v
func mapLookupImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	v, found, newCe, err := avlLookup(m.root, key, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	if found {
		return &eval.ConVal{Con: "Just", Args: []eval.Value{v}}, newCe, nil
	}
	return &eval.ConVal{Con: "Nothing"}, newCe, nil
}

// _mapDelete :: (k -> k -> Ordering) -> k -> Map k v -> Map k v
func mapDeleteImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
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

// _mapSize :: Map k v -> Int
func mapSizeImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(m.size)}, ce, nil
}

// _mapToList :: Map k v -> List (k, v)
func mapToListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*(costTupleNode+costConsNode)); err != nil {
		return nil, ce, err
	}
	return avlToConsList(m.root), ce, nil
}

// _mapFromList :: (k -> k -> Ordering) -> List (k, v) -> Map k v
func mapFromListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	list := args[1]
	m := &mapVal{root: nil, cmp: cmp, size: 0}
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, fmt.Errorf("mapFromList: expected List, got %T", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, fmt.Errorf("mapFromList: malformed list")
		}
		pair, ok := con.Args[0].(*eval.RecordVal)
		if !ok {
			return nil, ce, fmt.Errorf("mapFromList: expected tuple, got %T", con.Args[0])
		}
		key, ok1 := pair.Fields[types.TupleLabel(1)]
		value, ok2 := pair.Fields[types.TupleLabel(2)]
		if !ok1 || !ok2 {
			return nil, ce, fmt.Errorf("mapFromList: tuple must have _1 and _2")
		}
		if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
			return nil, ce, err
		}
		var inserted bool
		var err error
		m.root, inserted, ce, err = avlInsert(m.root, key, value, cmp, ce, apply)
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

// _mapFoldlWithKey :: (b -> k -> v -> b) -> b -> Map k v -> b
func mapFoldlWithKeyImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	acc := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	return avlFoldlWithKey(m.root, f, acc, ce, apply)
}

// _mapMember :: (k -> k -> Ordering) -> k -> Map k v -> Bool
func mapMemberImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	_, found, newCe, err := avlLookup(m.root, key, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return boolVal(found), newCe, nil
}

// _mapUnionWith :: (v -> v -> v) -> Map k v -> Map k v -> Map k v
func mapUnionWithImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	m1, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	m2, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	result := &mapVal{root: m1.root, cmp: m1.cmp, size: m1.size}
	ce, err = avlWalkMerge(ctx, m2.root, f, result, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

// avlWalkMerge traverses the source AVL tree in-order and inserts each entry
// into the result map, using f to combine values on key collision.
// No intermediate slice is allocated.
func avlWalkMerge(ctx context.Context, n *avlNode, f eval.Value, result *mapVal, ce eval.CapEnv, apply eval.Applier) (eval.CapEnv, error) {
	if n == nil {
		return ce, nil
	}
	var err error
	ce, err = avlWalkMerge(ctx, n.left, f, result, ce, apply)
	if err != nil {
		return ce, err
	}
	existing, found, newCe, err := avlLookup(result.root, n.key, result.cmp, ce, apply)
	if err != nil {
		return ce, err
	}
	ce = newCe
	insertVal := n.value
	if found {
		partial, newCe2, err := apply(f, existing, ce)
		if err != nil {
			return ce, err
		}
		insertVal, ce, err = apply(partial, n.value, newCe2)
		if err != nil {
			return ce, err
		}
	}
	if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
		return ce, err
	}
	var inserted bool
	result.root, inserted, ce, err = avlInsert(result.root, n.key, insertVal, result.cmp, ce, apply)
	if err != nil {
		return ce, err
	}
	if inserted {
		result.size++
	}
	return avlWalkMerge(ctx, n.right, f, result, ce, apply)
}
