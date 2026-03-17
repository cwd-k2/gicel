package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
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
	return e.RegisterModule("Std.Map", mapSource)
}

var mapSource = mustReadSource("map")

// --- AVL tree ---

type avlNode struct {
	key    eval.Value
	value  eval.Value
	left   *avlNode
	right  *avlNode
	height int
}

type mapVal struct {
	root *avlNode
	cmp  eval.Value // compare :: k -> k -> Ordering
	size int
}

func (*mapVal) String() string { return "Map(...)" }

func avlHeight(n *avlNode) int {
	if n == nil {
		return 0
	}
	return n.height
}

func avlBalance(n *avlNode) int {
	if n == nil {
		return 0
	}
	return avlHeight(n.left) - avlHeight(n.right)
}

func avlNewNode(key, value eval.Value) *avlNode {
	return &avlNode{key: key, value: value, height: 1}
}

func avlUpdateHeight(n *avlNode) {
	lh, rh := avlHeight(n.left), avlHeight(n.right)
	if lh > rh {
		n.height = lh + 1
	} else {
		n.height = rh + 1
	}
}

func avlRotateRight(y *avlNode) *avlNode {
	x := y.left
	// Copy both nodes to preserve persistence — x may be shared.
	newY := &avlNode{key: y.key, value: y.value, left: x.right, right: y.right, height: y.height}
	newX := &avlNode{key: x.key, value: x.value, left: x.left, right: newY, height: x.height}
	avlUpdateHeight(newY)
	avlUpdateHeight(newX)
	return newX
}

func avlRotateLeft(x *avlNode) *avlNode {
	y := x.right
	// Copy both nodes to preserve persistence — y may be shared.
	newX := &avlNode{key: x.key, value: x.value, left: x.left, right: y.left, height: x.height}
	newY := &avlNode{key: y.key, value: y.value, left: newX, right: y.right, height: y.height}
	avlUpdateHeight(newX)
	avlUpdateHeight(newY)
	return newY
}

func avlRebalance(n *avlNode) *avlNode {
	avlUpdateHeight(n)
	bal := avlBalance(n)
	if bal > 1 {
		if avlBalance(n.left) < 0 {
			// Left-right case: copy n with rotated left child.
			n = &avlNode{key: n.key, value: n.value,
				left: avlRotateLeft(n.left), right: n.right, height: n.height}
		}
		return avlRotateRight(n)
	}
	if bal < -1 {
		if avlBalance(n.right) > 0 {
			// Right-left case: copy n with rotated right child.
			n = &avlNode{key: n.key, value: n.value,
				left: n.left, right: avlRotateRight(n.right), height: n.height}
		}
		return avlRotateLeft(n)
	}
	return n
}

// compareKeys applies the compare function via Applier.
// Returns: -1 (LT), 0 (EQ), 1 (GT).
func compareKeys(cmp eval.Value, a, b eval.Value, ce eval.CapEnv, apply eval.Applier) (int, eval.CapEnv, error) {
	partial, newCe, err := apply(cmp, a, ce)
	if err != nil {
		return 0, ce, err
	}
	result, newCe, err := apply(partial, b, newCe)
	if err != nil {
		return 0, ce, err
	}
	con, ok := result.(*eval.ConVal)
	if !ok {
		return 0, newCe, fmt.Errorf("map: compare must return Ordering, got %T", result)
	}
	switch con.Con {
	case "LT":
		return -1, newCe, nil
	case "EQ":
		return 0, newCe, nil
	case "GT":
		return 1, newCe, nil
	default:
		return 0, newCe, fmt.Errorf("map: unknown Ordering constructor: %s", con.Con)
	}
}

// avlInsert returns the new root and whether a new key was added (not an overwrite).
func avlInsert(n *avlNode, key, value, cmp eval.Value, ce eval.CapEnv, apply eval.Applier) (*avlNode, bool, eval.CapEnv, error) {
	if n == nil {
		return avlNewNode(key, value), true, ce, nil
	}
	ord, newCe, err := compareKeys(cmp, key, n.key, ce, apply)
	if err != nil {
		return n, false, ce, err
	}
	// Persistent: copy node.
	node := &avlNode{key: n.key, value: n.value, left: n.left, right: n.right, height: n.height}
	var inserted bool
	switch ord {
	case -1:
		node.left, inserted, newCe, err = avlInsert(n.left, key, value, cmp, newCe, apply)
	case 1:
		node.right, inserted, newCe, err = avlInsert(n.right, key, value, cmp, newCe, apply)
	default:
		node.value = value
		return node, false, newCe, nil
	}
	if err != nil {
		return n, false, ce, err
	}
	return avlRebalance(node), inserted, newCe, nil
}

func avlLookup(n *avlNode, key, cmp eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, bool, eval.CapEnv, error) {
	if n == nil {
		return nil, false, ce, nil
	}
	ord, newCe, err := compareKeys(cmp, key, n.key, ce, apply)
	if err != nil {
		return nil, false, ce, err
	}
	switch ord {
	case -1:
		return avlLookup(n.left, key, cmp, newCe, apply)
	case 1:
		return avlLookup(n.right, key, cmp, newCe, apply)
	default:
		return n.value, true, newCe, nil
	}
}

func avlMinNode(n *avlNode) *avlNode {
	for n.left != nil {
		n = n.left
	}
	return n
}

func avlDelete(n *avlNode, key, cmp eval.Value, ce eval.CapEnv, apply eval.Applier) (*avlNode, bool, eval.CapEnv, error) {
	if n == nil {
		return nil, false, ce, nil
	}
	ord, newCe, err := compareKeys(cmp, key, n.key, ce, apply)
	if err != nil {
		return n, false, ce, err
	}
	node := &avlNode{key: n.key, value: n.value, left: n.left, right: n.right, height: n.height}
	var deleted bool
	switch ord {
	case -1:
		node.left, deleted, newCe, err = avlDelete(n.left, key, cmp, newCe, apply)
	case 1:
		node.right, deleted, newCe, err = avlDelete(n.right, key, cmp, newCe, apply)
	default:
		deleted = true
		if node.left == nil {
			return node.right, true, newCe, nil
		}
		if node.right == nil {
			return node.left, true, newCe, nil
		}
		successor := avlMinNode(node.right)
		node.key = successor.key
		node.value = successor.value
		node.right, _, newCe, err = avlDelete(node.right, successor.key, cmp, newCe, apply)
	}
	if err != nil {
		return n, false, ce, err
	}
	if !deleted {
		return n, false, newCe, nil
	}
	return avlRebalance(node), true, newCe, nil
}

// avlToConsList builds an in-order ConVal list from the AVL tree without
// an intermediate slice. Traverses right-to-left, consing each (key, value)
// pair onto the accumulator.
func avlToConsList(n *avlNode) eval.Value {
	var acc eval.Value = &eval.ConVal{Con: "Nil"}
	avlConsRight(n, &acc)
	return acc
}

func avlConsRight(n *avlNode, acc *eval.Value) {
	if n == nil {
		return
	}
	avlConsRight(n.right, acc)
	pair := &eval.RecordVal{Fields: map[string]eval.Value{"_1": n.key, "_2": n.value}}
	*acc = &eval.ConVal{Con: "Cons", Args: []eval.Value{pair, *acc}}
	avlConsRight(n.left, acc)
}

func avlFoldlWithKey(n *avlNode, f, acc eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	if n == nil {
		return acc, ce, nil
	}
	var err error
	acc, ce, err = avlFoldlWithKey(n.left, f, acc, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	// f acc key value
	partial1, ce, err := apply(f, acc, ce)
	if err != nil {
		return nil, ce, err
	}
	partial2, ce, err := apply(partial1, n.key, ce)
	if err != nil {
		return nil, ce, err
	}
	acc, ce, err = apply(partial2, n.value, ce)
	if err != nil {
		return nil, ce, err
	}
	return avlFoldlWithKey(n.right, f, acc, ce, apply)
}

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
	if err := eval.ChargeAlloc(ctx, costAVLNode); err != nil {
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
	if err := eval.ChargeAlloc(ctx, int64(m.size)*(costTupleNode+costConsNode)); err != nil {
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
		key, ok1 := pair.Fields["_1"]
		value, ok2 := pair.Fields["_2"]
		if !ok1 || !ok2 {
			return nil, ce, fmt.Errorf("mapFromList: tuple must have _1 and _2")
		}
		if err := eval.ChargeAlloc(ctx, costAVLNode); err != nil {
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
	if err := eval.ChargeAlloc(ctx, costAVLNode); err != nil {
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
