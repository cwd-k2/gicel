package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/infra/budget"
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
	pair := &eval.ConVal{Con: "Tuple2", Args: []eval.Value{view.head, seqWrap(view.tail)}}
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
	pair := &eval.ConVal{Con: "Tuple2", Args: []eval.Value{seqWrap(view.init), view.last}}
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
	if err := budget.ChargeAlloc(ctx, costFTNode*4); err != nil {
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
