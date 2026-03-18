package stdlib

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/eval"
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
