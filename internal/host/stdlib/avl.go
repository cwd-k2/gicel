package stdlib

import (
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Persistent AVL tree used by both Map and Set.
// Comparison uses the GICEL Ord dictionary (compare function) via Applier.
// Cost constant costAVLNode is defined in stdlib.go.

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
	newY := &avlNode{key: y.key, value: y.value, left: x.right, right: y.right, height: y.height}
	newX := &avlNode{key: x.key, value: x.value, left: x.left, right: newY, height: x.height}
	avlUpdateHeight(newY)
	avlUpdateHeight(newX)
	return newX
}

func avlRotateLeft(x *avlNode) *avlNode {
	y := x.right
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
			n = &avlNode{key: n.key, value: n.value,
				left: avlRotateLeft(n.left), right: n.right, height: n.height}
		}
		return avlRotateRight(n)
	}
	if bal < -1 {
		if avlBalance(n.right) > 0 {
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
		return 0, newCe, errExpected("map: compare", "Ordering", result)
	}
	switch con.Con {
	case "LT":
		return -1, newCe, nil
	case "EQ":
		return 0, newCe, nil
	case "GT":
		return 1, newCe, nil
	default:
		return 0, newCe, errMalformed("map", "Ordering constructor", con.Con)
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

// avlToConsList builds an in-order ConVal list from the AVL tree.
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
	pair := eval.NewRecordFromMap(map[string]eval.Value{ir.TupleLabel(1): n.key, ir.TupleLabel(2): n.value})
	*acc = &eval.ConVal{Con: "Cons", Args: []eval.Value{pair, *acc}}
	avlConsRight(n.left, acc)
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

// avlValsToConsList builds an in-order ConVal list of values from the AVL tree.
func avlValsToConsList(n *avlNode) eval.Value {
	var acc eval.Value = &eval.ConVal{Con: "Nil"}
	avlValsConsRight(n, &acc)
	return acc
}

func avlValsConsRight(n *avlNode, acc *eval.Value) {
	if n == nil {
		return
	}
	avlValsConsRight(n.right, acc)
	*acc = &eval.ConVal{Con: "Cons", Args: []eval.Value{n.value, *acc}}
	avlValsConsRight(n.left, acc)
}

// avlMapValues applies f to every value in the tree via Applier, returning
// a new tree with the same structure and keys but transformed values.
func avlMapValues(n *avlNode, f eval.Value, ce eval.CapEnv, apply eval.Applier) (*avlNode, eval.CapEnv, error) {
	if n == nil {
		return nil, ce, nil
	}
	var err error
	var left, right *avlNode
	left, ce, err = avlMapValues(n.left, f, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	var newVal eval.Value
	newVal, ce, err = apply(f, n.value, ce)
	if err != nil {
		return nil, ce, err
	}
	right, ce, err = avlMapValues(n.right, f, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &avlNode{key: n.key, value: newVal, left: left, right: right, height: n.height}, ce, nil
}

// avlFilterWithKey walks the tree in-order and applies a predicate (k -> v -> Bool)
// to each entry via Applier. Entries where the predicate returns True are collected;
// others are discarded. Builds a fresh tree from kept entries.
func avlFilterWithKey(n *avlNode, pred eval.Value, cmp eval.Value, root **avlNode, size *int, ce eval.CapEnv, apply eval.Applier) (eval.CapEnv, error) {
	if n == nil {
		return ce, nil
	}
	var err error
	ce, err = avlFilterWithKey(n.left, pred, cmp, root, size, ce, apply)
	if err != nil {
		return ce, err
	}
	// Apply predicate: pred k v
	var partial eval.Value
	partial, ce, err = apply(pred, n.key, ce)
	if err != nil {
		return ce, err
	}
	var result eval.Value
	result, ce, err = apply(partial, n.value, ce)
	if err != nil {
		return ce, err
	}
	if con, ok := result.(*eval.ConVal); ok && con.Con == eval.BoolTrue {
		var inserted bool
		*root, inserted, ce, err = avlInsert(*root, n.key, n.value, cmp, ce, apply)
		if err != nil {
			return ce, err
		}
		if inserted {
			*size++
		}
	}
	return avlFilterWithKey(n.right, pred, cmp, root, size, ce, apply)
}

func avlMaxNode(n *avlNode) *avlNode {
	for n.right != nil {
		n = n.right
	}
	return n
}

// avlInsertWith inserts with a merge function: if key exists, value = f(existing, new).
func avlInsertWith(n *avlNode, key, value eval.Value, f, cmp eval.Value, ce eval.CapEnv, apply eval.Applier) (*avlNode, bool, eval.CapEnv, error) {
	if n == nil {
		return avlNewNode(key, value), true, ce, nil
	}
	ord, newCe, err := compareKeys(cmp, key, n.key, ce, apply)
	if err != nil {
		return n, false, ce, err
	}
	node := &avlNode{key: n.key, value: n.value, left: n.left, right: n.right, height: n.height}
	var inserted bool
	switch ord {
	case -1:
		node.left, inserted, newCe, err = avlInsertWith(n.left, key, value, f, cmp, newCe, apply)
	case 1:
		node.right, inserted, newCe, err = avlInsertWith(n.right, key, value, f, cmp, newCe, apply)
	default:
		// Key exists: apply f existing new
		partial, ce2, err2 := apply(f, n.value, newCe)
		if err2 != nil {
			return n, false, ce, err2
		}
		merged, ce3, err3 := apply(partial, value, ce2)
		if err3 != nil {
			return n, false, ce, err3
		}
		node.value = merged
		return node, false, ce3, nil
	}
	if err != nil {
		return n, false, ce, err
	}
	return avlRebalance(node), inserted, newCe, nil
}

// avlMapWithKey applies f(key, value) to every entry.
func avlMapWithKey(n *avlNode, f eval.Value, ce eval.CapEnv, apply eval.Applier) (*avlNode, eval.CapEnv, error) {
	if n == nil {
		return nil, ce, nil
	}
	var err error
	var left, right *avlNode
	left, ce, err = avlMapWithKey(n.left, f, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	partial, ce, err := apply(f, n.key, ce)
	if err != nil {
		return nil, ce, err
	}
	var newVal eval.Value
	newVal, ce, err = apply(partial, n.value, ce)
	if err != nil {
		return nil, ce, err
	}
	right, ce, err = avlMapWithKey(n.right, f, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return &avlNode{key: n.key, value: newVal, left: left, right: right, height: n.height}, ce, nil
}

// avlFoldrWithKey walks the tree in reverse-order (right to left).
func avlFoldrWithKey(n *avlNode, f, acc eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	if n == nil {
		return acc, ce, nil
	}
	var err error
	acc, ce, err = avlFoldrWithKey(n.right, f, acc, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	partial1, ce, err := apply(f, n.key, ce)
	if err != nil {
		return nil, ce, err
	}
	partial2, ce, err := apply(partial1, n.value, ce)
	if err != nil {
		return nil, ce, err
	}
	acc, ce, err = apply(partial2, acc, ce)
	if err != nil {
		return nil, ce, err
	}
	return avlFoldrWithKey(n.left, f, acc, ce, apply)
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
