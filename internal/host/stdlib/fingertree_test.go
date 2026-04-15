// Finger tree tests — ftreeSize, ftFromList, ftCons, ftSnoc.
// Does NOT cover: stdlib_collection_test.go, stdlib_test.go.
package stdlib

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// mkList builds a GICEL cons-list of Int values from a Go slice.
func mkList(xs []int) eval.Value {
	var v eval.Value = &eval.ConVal{Con: "Nil"}
	for i := len(xs) - 1; i >= 0; i-- {
		v = &eval.ConVal{Con: "Cons", Args: []eval.Value{eval.IntVal(int64(xs[i])), v}}
	}
	return v
}

func TestFtFromListSize(t *testing.T) {
	for n := 0; n <= 50; n++ {
		xs := make([]int, n)
		for i := range xs {
			xs[i] = i + 1
		}
		tree := ftFromList(mkList(xs))
		got := tree.ftreeSize()
		if got != n {
			t.Errorf("ftFromList(%d elements).ftreeSize() = %d, want %d", n, got, n)
		}
	}
}

func TestFtSnocSize(t *testing.T) {
	for n := 0; n <= 50; n++ {
		tree := ftree(ftEmpty{})
		for i := 0; i < n; i++ {
			tree = ftSnoc(tree, eval.IntVal(int64(i+1)))
		}
		got := tree.ftreeSize()
		if got != n {
			t.Errorf("ftSnoc x%d: ftreeSize() = %d, want %d", n, got, n)
		}
	}
}

func TestFtConsSize(t *testing.T) {
	for n := 0; n <= 50; n++ {
		tree := ftree(ftEmpty{})
		for i := 0; i < n; i++ {
			tree = ftCons(eval.IntVal(int64(i+1)), tree)
		}
		got := tree.ftreeSize()
		if got != n {
			t.Errorf("ftCons x%d: ftreeSize() = %d, want %d", n, got, n)
		}
	}
}

func TestFtSingleNodeSize(t *testing.T) {
	// A ftSingle holding a node3 must report the node's size, not 1.
	n := mkNode3(eval.IntVal(1), eval.IntVal(2), eval.IntVal(3))
	s := ftSingle{elem: nodeVal(n)}
	if s.ftreeSize() != 3 {
		t.Errorf("ftSingle{node3}.ftreeSize() = %d, want 3", s.ftreeSize())
	}

	n2 := mkNode2(eval.IntVal(1), eval.IntVal(2))
	s2 := ftSingle{elem: nodeVal(n2)}
	if s2.ftreeSize() != 2 {
		t.Errorf("ftSingle{node2}.ftreeSize() = %d, want 2", s2.ftreeSize())
	}
}

func TestFtRoundTrip(t *testing.T) {
	// Verify that ftToSlice preserves all elements after ftFromList.
	for _, n := range []int{0, 1, 5, 6, 7, 8, 9, 20, 21, 30, 50} {
		xs := make([]int, n)
		for i := range xs {
			xs[i] = i + 1
		}
		tree := ftFromList(mkList(xs))
		items := ftToSlice(tree)
		if len(items) != n {
			t.Errorf("ftToSlice(ftFromList(%d)): got %d elements, want %d", n, len(items), n)
			continue
		}
		for i, v := range items {
			iv := v.(*eval.HostVal).Inner.(int64)
			if int(iv) != xs[i] {
				t.Errorf("ftToSlice(ftFromList(%d))[%d] = %d, want %d", n, i, iv, xs[i])
				break
			}
		}
	}
}

func TestFtToListRoundTrip(t *testing.T) {
	// Verify that ftToList (GICEL-facing) preserves all elements.
	for _, n := range []int{0, 1, 5, 6, 7, 8, 9, 20, 21, 30, 50} {
		xs := make([]int, n)
		for i := range xs {
			xs[i] = i + 1
		}
		tree := ftFromList(mkList(xs))
		list := ftToList(tree)
		items, ok := listToSlice(list)
		if !ok {
			t.Errorf("ftToList(ftFromList(%d)): invalid cons-list", n)
			continue
		}
		if len(items) != n {
			t.Errorf("ftToList(ftFromList(%d)): got %d elements, want %d", n, len(items), n)
			continue
		}
		for i, v := range items {
			iv, ok := v.(*eval.HostVal)
			if !ok {
				t.Errorf("ftToList(ftFromList(%d))[%d]: not a HostVal (got %T)", n, i, v)
				break
			}
			val, ok := iv.Inner.(int64)
			if !ok {
				t.Errorf("ftToList(ftFromList(%d))[%d]: not an int64 (got %T: %v)", n, i, iv.Inner, iv.Inner)
				break
			}
			if int(val) != xs[i] {
				t.Errorf("ftToList(ftFromList(%d))[%d] = %d, want %d", n, i, val, xs[i])
				break
			}
		}
	}
}
