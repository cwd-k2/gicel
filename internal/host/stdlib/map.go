package stdlib

import (
	"context"
	"errors"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
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
	e.RegisterPrim("_mapKeys", mapKeysImpl)
	e.RegisterPrim("_mapValues", mapValuesImpl)
	e.RegisterPrim("_mapMapValues", mapMapValuesImpl)
	e.RegisterPrim("_mapFilterWithKey", mapFilterWithKeyImpl)
	e.RegisterPrim("_mapInsertWith", mapInsertWithImpl)
	e.RegisterPrim("_mapIntersectionWith", mapIntersectionWithImpl)
	e.RegisterPrim("_mapFindMin", mapFindMinImpl)
	e.RegisterPrim("_mapFindMax", mapFindMaxImpl)
	e.RegisterPrim("_mapMapWithKey", mapMapWithKeyImpl)
	e.RegisterPrim("_mapFoldrWithKey", mapFoldrWithKeyImpl)
	return e.RegisterModule("Data.Map", mapSource)
}

var mapSource = mustReadSource("map")

// --- Primitives ---

func asMapVal(v eval.Value) (*mapVal, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, errExpected("map", "Map", v)
	}
	mv, ok := hv.Inner.(*mapVal)
	if !ok {
		return nil, errExpected("map", "Map", hv.Inner)
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
			return nil, ce, errExpected("mapFromList", "List", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("mapFromList", "list node", con.Con)
		}
		pair, ok := con.Args[0].(*eval.RecordVal)
		if !ok {
			return nil, ce, errExpected("mapFromList", "tuple", con.Args[0])
		}
		key, ok1 := pair.Get(types.TupleLabel(1))
		value, ok2 := pair.Get(types.TupleLabel(2))
		if !ok1 || !ok2 {
			return nil, ce, errors.New("mapFromList: tuple must have _1 and _2")
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
		insertVal, ce, err = apply.ApplyN(f, []eval.Value{existing, n.value}, ce)
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

// _mapKeys :: Map k v -> List k
func mapKeysImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costConsNode); err != nil {
		return nil, ce, err
	}
	return avlKeysToConsList(m.root), ce, nil
}

// _mapValues :: Map k v -> List v
func mapValuesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costConsNode); err != nil {
		return nil, ce, err
	}
	return avlValsToConsList(m.root), ce, nil
}

// _mapMapValues :: (v -> w) -> Map k v -> Map k w
func mapMapValuesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	m, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costAVLNode); err != nil {
		return nil, ce, err
	}
	newRoot, newCe, err := avlMapValues(m.root, f, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mapVal{root: newRoot, cmp: m.cmp, size: m.size}}, newCe, nil
}

// _mapFilterWithKey :: (k -> v -> Bool) -> Map k v -> Map k v
func mapFilterWithKeyImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	m, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costAVLNode); err != nil {
		return nil, ce, err
	}
	var newRoot *avlNode
	var newSize int
	newCe, err := avlFilterWithKey(m.root, pred, m.cmp, &newRoot, &newSize, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mapVal{root: newRoot, cmp: m.cmp, size: newSize}}, newCe, nil
}

// _mapInsertWith :: (k -> k -> Ordering) -> (v -> v -> v) -> k -> v -> Map k v -> Map k v
func mapInsertWithImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	// args[0] = compare (unused, map stores its own)
	f := args[1]
	key := args[2]
	value := args[3]
	m, err := asMapVal(args[4])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
		return nil, ce, err
	}
	newRoot, inserted, newCe, err := avlInsertWith(m.root, key, value, f, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	newSize := m.size
	if inserted {
		newSize++
	}
	return &eval.HostVal{Inner: &mapVal{root: newRoot, cmp: m.cmp, size: newSize}}, newCe, nil
}

// _mapIntersectionWith :: (v -> v -> w) -> Map k v -> Map k w -> Map k w
func mapIntersectionWithImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	m1, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	m2, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	result := &mapVal{root: nil, cmp: m1.cmp, size: 0}
	ce, err = avlIntersectWalk(ctx, m1.root, m2, f, result, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: result}, ce, nil
}

func avlIntersectWalk(ctx context.Context, n *avlNode, m2 *mapVal, f eval.Value, result *mapVal, ce eval.CapEnv, apply eval.Applier) (eval.CapEnv, error) {
	if n == nil {
		return ce, nil
	}
	var err error
	ce, err = avlIntersectWalk(ctx, n.left, m2, f, result, ce, apply)
	if err != nil {
		return ce, err
	}
	v2, found, newCe, err := avlLookup(m2.root, n.key, m2.cmp, ce, apply)
	if err != nil {
		return ce, err
	}
	ce = newCe
	if found {
		merged, ce2, err2 := apply.ApplyN(f, []eval.Value{n.value, v2}, ce)
		if err2 != nil {
			return ce, err2
		}
		ce = ce2
		if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
			return ce, err
		}
		var inserted bool
		result.root, inserted, ce, err = avlInsert(result.root, n.key, merged, result.cmp, ce, apply)
		if err != nil {
			return ce, err
		}
		if inserted {
			result.size++
		}
	}
	return avlIntersectWalk(ctx, n.right, m2, f, result, ce, apply)
}

// _mapFindMin :: Map k v -> Maybe (k, v)
func mapFindMinImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if m.root == nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	n := avlMinNode(m.root)
	pair := eval.NewRecordFromMap(map[string]eval.Value{types.TupleLabel(1): n.key, types.TupleLabel(2): n.value})
	return &eval.ConVal{Con: "Just", Args: []eval.Value{pair}}, ce, nil
}

// _mapFindMax :: Map k v -> Maybe (k, v)
func mapFindMaxImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if m.root == nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	n := avlMaxNode(m.root)
	pair := eval.NewRecordFromMap(map[string]eval.Value{types.TupleLabel(1): n.key, types.TupleLabel(2): n.value})
	return &eval.ConVal{Con: "Just", Args: []eval.Value{pair}}, ce, nil
}

// _mapMapWithKey :: (k -> v -> w) -> Map k v -> Map k w
func mapMapWithKeyImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	m, err := asMapVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costAVLNode); err != nil {
		return nil, ce, err
	}
	newRoot, newCe, err := avlMapWithKey(m.root, f, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &mapVal{root: newRoot, cmp: m.cmp, size: m.size}}, newCe, nil
}

// _mapFoldrWithKey :: (k -> v -> b -> b) -> b -> Map k v -> b
func mapFoldrWithKeyImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	acc := args[1]
	m, err := asMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	return avlFoldrWithKey(m.root, f, acc, ce, apply)
}
