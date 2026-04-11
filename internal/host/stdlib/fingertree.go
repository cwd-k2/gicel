package stdlib

// fingertree.go — 2-3 finger tree with size annotations.
// Provides O(1) amortized cons/snoc, O(log n) concatenation,
// O(log n) indexed access, O(n) fold/toList.

import "github.com/cwd-k2/gicel/internal/runtime/eval"

// --- Node types ---

// ftree is a finger tree. Three variants: ftEmpty, ftSingle, ftDeep.
type ftree interface{ ftreeSize() int }

type ftEmpty struct{}

func (ftEmpty) ftreeSize() int { return 0 }

type ftSingle struct{ elem eval.Value }

func (ftSingle) ftreeSize() int { return 1 }

type ftDeep struct {
	size  int
	left  []eval.Value // 1-4 elements (digit)
	mid   ftree        // finger tree of node3 values
	right []eval.Value // 1-4 elements (digit)
}

func (d *ftDeep) ftreeSize() int { return d.size }

// node23 is a 2-3 node used in the spine.
type node23 struct {
	size  int
	elems []eval.Value // 2 or 3 elements
}

func mkNode2(a, b eval.Value) *node23 {
	return &node23{size: 2, elems: []eval.Value{a, b}}
}

func mkNode3(a, b, c eval.Value) *node23 {
	return &node23{size: 3, elems: []eval.Value{a, b, c}}
}

func nodeVal(n *node23) eval.Value {
	return &eval.HostVal{Inner: n}
}

func asNode(v eval.Value) *node23 {
	return v.(*eval.HostVal).Inner.(*node23)
}

func digitSize(d []eval.Value) int {
	s := 0
	for _, v := range d {
		if n, ok := v.(*eval.HostVal); ok {
			if nd, ok := n.Inner.(*node23); ok {
				s += nd.size
				continue
			}
		}
		s++
	}
	return s
}

func deepSize(left []eval.Value, mid ftree, right []eval.Value) int {
	return digitSize(left) + mid.ftreeSize() + digitSize(right)
}

func mkDeep(left []eval.Value, mid ftree, right []eval.Value) *ftDeep {
	return &ftDeep{size: deepSize(left, mid, right), left: left, mid: mid, right: right}
}

// --- Cons (prepend) ---

func ftCons(a eval.Value, t ftree) ftree {
	switch t := t.(type) {
	case ftEmpty:
		return ftSingle{elem: a}
	case ftSingle:
		return mkDeep([]eval.Value{a}, ftEmpty{}, []eval.Value{t.elem})
	case *ftDeep:
		if len(t.left) < 4 {
			newLeft := make([]eval.Value, len(t.left)+1)
			newLeft[0] = a
			copy(newLeft[1:], t.left)
			return mkDeep(newLeft, t.mid, t.right)
		}
		// Left digit full: push a node3 into the spine.
		newLeft := []eval.Value{a, t.left[0]}
		pushed := mkNode3(t.left[1], t.left[2], t.left[3])
		return mkDeep(newLeft, ftCons(nodeVal(pushed), t.mid), t.right)
	}
	return t
}

// --- Snoc (append) ---

func ftSnoc(t ftree, a eval.Value) ftree {
	switch t := t.(type) {
	case ftEmpty:
		return ftSingle{elem: a}
	case ftSingle:
		return mkDeep([]eval.Value{t.elem}, ftEmpty{}, []eval.Value{a})
	case *ftDeep:
		if len(t.right) < 4 {
			newRight := make([]eval.Value, len(t.right)+1)
			copy(newRight, t.right)
			newRight[len(t.right)] = a
			return mkDeep(t.left, t.mid, newRight)
		}
		// Right digit full: push a node3 into the spine.
		pushed := mkNode3(t.right[0], t.right[1], t.right[2])
		newRight := []eval.Value{t.right[3], a}
		return mkDeep(t.left, ftSnoc(t.mid, nodeVal(pushed)), newRight)
	}
	return t
}

// --- Uncons ---

type ftViewL struct {
	head eval.Value
	tail ftree
}

func ftUncons(t ftree) *ftViewL {
	switch t := t.(type) {
	case ftEmpty:
		return nil
	case ftSingle:
		return &ftViewL{head: t.elem, tail: ftEmpty{}}
	case *ftDeep:
		h := t.left[0]
		if len(t.left) > 1 {
			return &ftViewL{head: h, tail: mkDeep(t.left[1:], t.mid, t.right)}
		}
		// Left digit exhausted: pull from spine.
		inner := ftUncons(t.mid)
		if inner == nil {
			return &ftViewL{head: h, tail: digitToTree(t.right)}
		}
		nd := asNode(inner.head)
		return &ftViewL{head: h, tail: mkDeep(nd.elems, inner.tail, t.right)}
	}
	return nil
}

// --- Unsnoc ---

type ftViewR struct {
	init ftree
	last eval.Value
}

func ftUnsnoc(t ftree) *ftViewR {
	switch t := t.(type) {
	case ftEmpty:
		return nil
	case ftSingle:
		return &ftViewR{init: ftEmpty{}, last: t.elem}
	case *ftDeep:
		l := t.right[len(t.right)-1]
		if len(t.right) > 1 {
			return &ftViewR{init: mkDeep(t.left, t.mid, t.right[:len(t.right)-1]), last: l}
		}
		// Right digit exhausted: pull from spine.
		inner := ftUnsnoc(t.mid)
		if inner == nil {
			return &ftViewR{init: digitToTree(t.left), last: l}
		}
		nd := asNode(inner.last)
		return &ftViewR{init: mkDeep(t.left, inner.init, nd.elems), last: l}
	}
	return nil
}

func digitToTree(d []eval.Value) ftree {
	switch len(d) {
	case 0:
		return ftEmpty{}
	case 1:
		return ftSingle{elem: d[0]}
	default:
		mid := len(d) / 2
		return mkDeep(d[:mid], ftEmpty{}, d[mid:])
	}
}

// --- Append (concatenation) ---

func ftAppend(left, right ftree) ftree {
	return ftAppend3(left, nil, right)
}

func ftAppend3(left ftree, mid []eval.Value, right ftree) ftree {
	switch l := left.(type) {
	case ftEmpty:
		t := right
		for i := len(mid) - 1; i >= 0; i-- {
			t = ftCons(mid[i], t)
		}
		return t
	case ftSingle:
		t := right
		for i := len(mid) - 1; i >= 0; i-- {
			t = ftCons(mid[i], t)
		}
		return ftCons(l.elem, t)
	case *ftDeep:
		switch r := right.(type) {
		case ftEmpty:
			t := ftree(l)
			for _, v := range mid {
				t = ftSnoc(t, v)
			}
			return t
		case ftSingle:
			t := ftree(l)
			for _, v := range mid {
				t = ftSnoc(t, v)
			}
			return ftSnoc(t, r.elem)
		case *ftDeep:
			combined := make([]eval.Value, 0, len(l.right)+len(mid)+len(r.left))
			combined = append(combined, l.right...)
			combined = append(combined, mid...)
			combined = append(combined, r.left...)
			nodes := nodify(combined)
			return mkDeep(l.left, ftAppend3(l.mid, nodes, r.mid), r.right)
		}
	}
	return left
}

// nodify groups a list of elements into node2/node3 values.
func nodify(xs []eval.Value) []eval.Value {
	switch len(xs) {
	case 2:
		return []eval.Value{nodeVal(mkNode2(xs[0], xs[1]))}
	case 3:
		return []eval.Value{nodeVal(mkNode3(xs[0], xs[1], xs[2]))}
	case 4:
		return []eval.Value{nodeVal(mkNode2(xs[0], xs[1])), nodeVal(mkNode2(xs[2], xs[3]))}
	default:
		if len(xs) < 2 {
			return nil
		}
		// len >= 5: take first 3 as node3, recurse on rest.
		rest := nodify(xs[3:])
		return append([]eval.Value{nodeVal(mkNode3(xs[0], xs[1], xs[2]))}, rest...)
	}
}

// --- Index ---

func ftIndex(t ftree, i int) eval.Value {
	switch t := t.(type) {
	case ftEmpty:
		return nil
	case ftSingle:
		if i == 0 {
			return t.elem
		}
		return nil
	case *ftDeep:
		ls := digitSize(t.left)
		if i < ls {
			return digitIndex(t.left, i)
		}
		i -= ls
		ms := t.mid.ftreeSize()
		if i < ms {
			return ftIndexNode(t.mid, i)
		}
		i -= ms
		return digitIndex(t.right, i)
	}
	return nil
}

func digitIndex(d []eval.Value, i int) eval.Value {
	for _, v := range d {
		s := elemSize(v)
		if i < s {
			if n, ok := extractNode(v); ok {
				return nodeIndex(n, i)
			}
			return v
		}
		i -= s
	}
	return nil
}

func ftIndexNode(t ftree, i int) eval.Value {
	switch t := t.(type) {
	case ftEmpty:
		return nil
	case ftSingle:
		nd := asNode(t.elem)
		return nodeIndex(nd, i)
	case *ftDeep:
		ls := digitSize(t.left)
		if i < ls {
			return digitIndex(t.left, i)
		}
		i -= ls
		ms := t.mid.ftreeSize()
		if i < ms {
			return ftIndexNode(t.mid, i)
		}
		i -= ms
		return digitIndex(t.right, i)
	}
	return nil
}

func nodeIndex(n *node23, i int) eval.Value {
	for _, e := range n.elems {
		s := elemSize(e)
		if i < s {
			if nd, ok := extractNode(e); ok {
				return nodeIndex(nd, i)
			}
			return e
		}
		i -= s
	}
	return nil
}

func elemSize(v eval.Value) int {
	if hv, ok := v.(*eval.HostVal); ok {
		if nd, ok := hv.Inner.(*node23); ok {
			return nd.size
		}
	}
	return 1
}

func extractNode(v eval.Value) (*node23, bool) {
	if hv, ok := v.(*eval.HostVal); ok {
		if nd, ok := hv.Inner.(*node23); ok {
			return nd, true
		}
	}
	return nil, false
}

// --- ToList ---

func ftToList(t ftree) eval.Value {
	var acc eval.Value = &eval.ConVal{Con: "Nil"}
	ftToListR(t, &acc)
	return acc
}

func ftToListR(t ftree, acc *eval.Value) {
	switch t := t.(type) {
	case ftEmpty:
	case ftSingle:
		*acc = &eval.ConVal{Con: "Cons", Args: []eval.Value{t.elem, *acc}}
	case *ftDeep:
		digitToListR(t.right, acc)
		ftToListSpine(t.mid, acc)
		digitToListR(t.left, acc)
	}
}

func ftToListSpine(t ftree, acc *eval.Value) {
	switch t := t.(type) {
	case ftEmpty:
	case ftSingle:
		nd := asNode(t.elem)
		nodeToListR(nd, acc)
	case *ftDeep:
		digitToListSpineR(t.right, acc)
		ftToListSpine(t.mid, acc)
		digitToListSpineR(t.left, acc)
	}
}

func digitToListR(d []eval.Value, acc *eval.Value) {
	for i := len(d) - 1; i >= 0; i-- {
		*acc = &eval.ConVal{Con: "Cons", Args: []eval.Value{d[i], *acc}}
	}
}

func digitToListSpineR(d []eval.Value, acc *eval.Value) {
	for i := len(d) - 1; i >= 0; i-- {
		nd := asNode(d[i])
		nodeToListR(nd, acc)
	}
}

func nodeToListR(n *node23, acc *eval.Value) {
	for i := len(n.elems) - 1; i >= 0; i-- {
		*acc = &eval.ConVal{Con: "Cons", Args: []eval.Value{n.elems[i], *acc}}
	}
}

// --- FromList ---

func ftFromList(v eval.Value) ftree {
	t := ftree(ftEmpty{})
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return t
		}
		if con.Con == "Nil" {
			return t
		}
		if con.Con == "Cons" && len(con.Args) == 2 {
			t = ftSnoc(t, con.Args[0])
			v = con.Args[1]
			continue
		}
		return t
	}
}

// --- Foldl ---

func ftFoldl(t ftree, f, acc eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	switch t := t.(type) {
	case ftEmpty:
		return acc, ce, nil
	case ftSingle:
		return apply.ApplyN(f, []eval.Value{acc, t.elem}, ce)
	case *ftDeep:
		var err error
		for _, v := range t.left {
			acc, ce, err = apply.ApplyN(f, []eval.Value{acc, v}, ce)
			if err != nil {
				return nil, ce, err
			}
		}
		acc, ce, err = ftFoldlSpine(t.mid, f, acc, ce, apply)
		if err != nil {
			return nil, ce, err
		}
		for _, v := range t.right {
			acc, ce, err = apply.ApplyN(f, []eval.Value{acc, v}, ce)
			if err != nil {
				return nil, ce, err
			}
		}
		return acc, ce, nil
	}
	return acc, ce, nil
}

func ftFoldlSpine(t ftree, f, acc eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	switch t := t.(type) {
	case ftEmpty:
		return acc, ce, nil
	case ftSingle:
		nd := asNode(t.elem)
		return nodeFoldl(nd, f, acc, ce, apply)
	case *ftDeep:
		var err error
		for _, v := range t.left {
			nd := asNode(v)
			acc, ce, err = nodeFoldl(nd, f, acc, ce, apply)
			if err != nil {
				return nil, ce, err
			}
		}
		acc, ce, err = ftFoldlSpine(t.mid, f, acc, ce, apply)
		if err != nil {
			return nil, ce, err
		}
		for _, v := range t.right {
			nd := asNode(v)
			acc, ce, err = nodeFoldl(nd, f, acc, ce, apply)
			if err != nil {
				return nil, ce, err
			}
		}
		return acc, ce, nil
	}
	return acc, ce, nil
}

func nodeFoldl(n *node23, f, acc eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	var err error
	for _, e := range n.elems {
		acc, ce, err = apply.ApplyN(f, []eval.Value{acc, e}, ce)
		if err != nil {
			return nil, ce, err
		}
	}
	return acc, ce, nil
}
