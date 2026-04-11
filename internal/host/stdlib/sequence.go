package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

const costFTNode = 64 // finger tree node (node23 or deep)

// Sequence provides a double-ended sequence backed by a 2-3 finger tree.
// O(1) amortized cons/snoc, O(log(min(m,n))) append, O(log n) index.
var Sequence Pack = func(e Registrar) error {
	e.RegisterPrim("_seqEmpty", seqEmptyImpl)
	e.RegisterPrim("_seqCons", seqConsImpl)
	e.RegisterPrim("_seqSnoc", seqSnocImpl)
	e.RegisterPrim("_seqUncons", seqUnconsImpl)
	e.RegisterPrim("_seqUnsnoc", seqUnsnocImpl)
	e.RegisterPrim("_seqAppend", seqAppendImpl)
	e.RegisterPrim("_seqIndex", seqIndexImpl)
	e.RegisterPrim("_seqLength", seqLengthImpl)
	e.RegisterPrim("_seqToList", seqToListImpl)
	e.RegisterPrim("_seqFromList", seqFromListImpl)
	e.RegisterPrim("_seqFoldl", seqFoldlImpl)
	e.RegisterPrim("_seqFoldr", seqFoldrImpl)
	e.RegisterPrim("_seqMap", seqMapImpl)
	e.RegisterPrim("_seqFilter", seqFilterImpl)
	e.RegisterPrim("_seqReverse", seqReverseImpl)
	return e.RegisterModule("Data.Sequence", seqSource)
}

var seqSource = mustReadSource("sequence")

// seqVal wraps a finger tree for the GICEL runtime.
type seqVal struct {
	tree ftree
}

func asSeqVal(v eval.Value) (*seqVal, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, errExpected("sequence", "Seq", v)
	}
	sv, ok := hv.Inner.(*seqVal)
	if !ok {
		return nil, errExpected("sequence", "Seq", hv.Inner)
	}
	return sv, nil
}

func seqWrap(t ftree) eval.Value {
	return &eval.HostVal{Inner: &seqVal{tree: t}}
}

func seqEmptyImpl(_ context.Context, ce eval.CapEnv, _ []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return seqWrap(ftEmpty{}), ce, nil
}

func seqConsImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	elem := args[0]
	sv, err := asSeqVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, costFTNode); err != nil {
		return nil, ce, err
	}
	return seqWrap(ftCons(elem, sv.tree)), ce, nil
}

func seqSnocImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sv, err := asSeqVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	elem := args[1]
	if err := budget.ChargeAlloc(ctx, costFTNode); err != nil {
		return nil, ce, err
	}
	return seqWrap(ftSnoc(sv.tree, elem)), ce, nil
}

func seqUnconsImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sv, err := asSeqVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	view := ftUncons(sv.tree)
	if view == nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	pair := eval.NewRecordFromMap(map[string]eval.Value{
		types.TupleLabel(1): view.head,
		types.TupleLabel(2): seqWrap(view.tail),
	})
	return &eval.ConVal{Con: "Just", Args: []eval.Value{pair}}, ce, nil
}

func seqUnsnocImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sv, err := asSeqVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	view := ftUnsnoc(sv.tree)
	if view == nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	pair := eval.NewRecordFromMap(map[string]eval.Value{
		types.TupleLabel(1): seqWrap(view.init),
		types.TupleLabel(2): view.last,
	})
	return &eval.ConVal{Con: "Just", Args: []eval.Value{pair}}, ce, nil
}

func seqAppendImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asSeqVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	b, err := asSeqVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	// Append creates O(log(min(m,n))) intermediate spine nodes.
	// Charge proportional to the smaller tree's depth.
	minSize := a.tree.ftreeSize()
	if bs := b.tree.ftreeSize(); bs < minSize {
		minSize = bs
	}
	nodes := int64(4) // base cost for digit recombination
	for s := minSize; s > 1; s /= 3 {
		nodes++
	}
	if err := budget.ChargeAlloc(ctx, costFTNode*nodes); err != nil {
		return nil, ce, err
	}
	return seqWrap(ftAppend(a.tree, b.tree)), ce, nil
}

func seqIndexImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	i, err := asInt64(args[0], "sequence")
	if err != nil {
		return nil, ce, err
	}
	sv, err := asSeqVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	if i < 0 || int(i) >= sv.tree.ftreeSize() {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	elem := ftIndex(sv.tree, int(i))
	if elem == nil {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{elem}}, ce, nil
}

func seqLengthImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sv, err := asSeqVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	return eval.IntVal(int64(sv.tree.ftreeSize())), ce, nil
}

func seqToListImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sv, err := asSeqVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	return ftToList(sv.tree), ce, nil
}

func seqFromListImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return seqWrap(ftFromList(args[0])), ce, nil
}

func seqFoldlImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	acc := args[1]
	sv, err := asSeqVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	return ftFoldl(sv.tree, f, acc, ce, apply)
}

func seqFoldrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	z := args[1]
	sv, err := asSeqVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	// Convert to list and fold right — O(n) but correct.
	items := ftToSlice(sv.tree)
	acc := z
	var err2 error
	for i := len(items) - 1; i >= 0; i-- {
		acc, ce, err2 = apply.ApplyN(f, []eval.Value{items[i], acc}, ce)
		if err2 != nil {
			return nil, ce, err2
		}
	}
	return acc, ce, nil
}

func seqMapImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	sv, err := asSeqVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	items := ftToSlice(sv.tree)
	if err := budget.ChargeAlloc(ctx, int64(len(items))*costFTNode); err != nil {
		return nil, ce, err
	}
	result := ftree(ftEmpty{})
	for _, item := range items {
		mapped, newCe, err := apply.Apply(f, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		result = ftSnoc(result, mapped)
	}
	return seqWrap(result), ce, nil
}

func seqFilterImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	sv, err := asSeqVal(args[1])
	if err != nil {
		return nil, ce, err
	}
	items := ftToSlice(sv.tree)
	result := ftree(ftEmpty{})
	for _, item := range items {
		val, newCe, err := apply.Apply(pred, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		if con, ok := val.(*eval.ConVal); ok && con.Con == "True" {
			result = ftSnoc(result, item)
		}
	}
	if err := budget.ChargeAlloc(ctx, int64(result.ftreeSize())*costFTNode); err != nil {
		return nil, ce, err
	}
	return seqWrap(result), ce, nil
}

func seqReverseImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	sv, err := asSeqVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	items := ftToSlice(sv.tree)
	result := ftree(ftEmpty{})
	for i := len(items) - 1; i >= 0; i-- {
		result = ftSnoc(result, items[i])
	}
	return seqWrap(result), ce, nil
}

// ftToSlice extracts all elements from a finger tree into a Go slice.
func ftToSlice(t ftree) []eval.Value {
	var items []eval.Value
	ftCollect(t, &items)
	return items
}

func ftCollect(t ftree, acc *[]eval.Value) {
	switch t := t.(type) {
	case ftEmpty:
	case ftSingle:
		*acc = append(*acc, t.elem)
	case *ftDeep:
		for _, v := range t.left {
			*acc = append(*acc, v)
		}
		ftCollectSpine(t.mid, acc)
		for _, v := range t.right {
			*acc = append(*acc, v)
		}
	}
}

func ftCollectSpine(t ftree, acc *[]eval.Value) {
	switch t := t.(type) {
	case ftEmpty:
	case ftSingle:
		nd := asNode(t.elem)
		for _, e := range nd.elems {
			*acc = append(*acc, e)
		}
	case *ftDeep:
		for _, v := range t.left {
			nd := asNode(v)
			for _, e := range nd.elems {
				*acc = append(*acc, e)
			}
		}
		ftCollectSpine(t.mid, acc)
		for _, v := range t.right {
			nd := asNode(v)
			for _, e := range nd.elems {
				*acc = append(*acc, e)
			}
		}
	}
}
