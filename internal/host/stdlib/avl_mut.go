package stdlib

import "github.com/cwd-k2/gicel/internal/runtime/eval"

// In-place AVL operations for mutable collections (MMap, MSet).
// Unlike avl.go (persistent, path-copying), these functions mutate nodes
// directly. Only new leaf nodes are allocated; existing nodes on the
// insertion/deletion path are reused. This is safe because mutable
// collections are gated by effect capabilities that guarantee exclusive
// access — no external code holds references to internal AVL nodes.
//
// The persistent functions in avl.go are unchanged and continue to serve
// Data.Map and Data.Set.

func avlRotateRightMut(y *avlNode) *avlNode {
	x := y.left
	y.left = x.right
	x.right = y
	avlUpdateHeight(y)
	avlUpdateHeight(x)
	return x
}

func avlRotateLeftMut(x *avlNode) *avlNode {
	y := x.right
	x.right = y.left
	y.left = x
	avlUpdateHeight(x)
	avlUpdateHeight(y)
	return y
}

func avlRebalanceMut(n *avlNode) *avlNode {
	avlUpdateHeight(n)
	bal := avlBalance(n)
	if bal > 1 {
		if avlBalance(n.left) < 0 {
			n.left = avlRotateLeftMut(n.left)
		}
		return avlRotateRightMut(n)
	}
	if bal < -1 {
		if avlBalance(n.right) > 0 {
			n.right = avlRotateRightMut(n.right)
		}
		return avlRotateLeftMut(n)
	}
	return n
}

// avlInsertMut inserts a key-value pair, mutating nodes in place.
// Only allocates a new avlNode for genuinely new keys (leaf insertion).
func avlInsertMut(n *avlNode, key, value, cmp eval.Value, ce eval.CapEnv, apply eval.Applier) (*avlNode, bool, eval.CapEnv, error) {
	if n == nil {
		return avlNewNode(key, value), true, ce, nil
	}
	ord, newCe, err := compareKeys(cmp, key, n.key, ce, apply)
	if err != nil {
		return n, false, ce, err
	}
	var inserted bool
	switch ord {
	case -1:
		n.left, inserted, newCe, err = avlInsertMut(n.left, key, value, cmp, newCe, apply)
	case 1:
		n.right, inserted, newCe, err = avlInsertMut(n.right, key, value, cmp, newCe, apply)
	default:
		n.value = value
		return n, false, newCe, nil
	}
	if err != nil {
		return n, false, ce, err
	}
	return avlRebalanceMut(n), inserted, newCe, nil
}

// avlDeleteMut removes a key, mutating nodes in place.
func avlDeleteMut(n *avlNode, key, cmp eval.Value, ce eval.CapEnv, apply eval.Applier) (*avlNode, bool, eval.CapEnv, error) {
	if n == nil {
		return nil, false, ce, nil
	}
	ord, newCe, err := compareKeys(cmp, key, n.key, ce, apply)
	if err != nil {
		return n, false, ce, err
	}
	var deleted bool
	switch ord {
	case -1:
		n.left, deleted, newCe, err = avlDeleteMut(n.left, key, cmp, newCe, apply)
	case 1:
		n.right, deleted, newCe, err = avlDeleteMut(n.right, key, cmp, newCe, apply)
	default:
		deleted = true
		if n.left == nil {
			return n.right, true, newCe, nil
		}
		if n.right == nil {
			return n.left, true, newCe, nil
		}
		successor := avlMinNode(n.right)
		n.key = successor.key
		n.value = successor.value
		n.right, _, newCe, err = avlDeleteMut(n.right, successor.key, cmp, newCe, apply)
	}
	if err != nil {
		return n, false, ce, err
	}
	if !deleted {
		return n, false, newCe, nil
	}
	return avlRebalanceMut(n), true, newCe, nil
}
