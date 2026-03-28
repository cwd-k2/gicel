package stdlib

import (
	"context"
	"errors"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// EffectMap provides mutable ordered maps gated by the { mmap: () } effect.
// Backed by the same AVL tree as Data.Map; the difference is in-place root mutation.
var EffectMap Pack = func(e Registrar) error {
	e.RegisterPrim("_mmapNew", mmapNewImpl)
	e.RegisterPrim("_mmapInsert", mmapInsertImpl)
	e.RegisterPrim("_mmapLookup", mmapLookupImpl)
	e.RegisterPrim("_mmapDelete", mmapDeleteImpl)
	e.RegisterPrim("_mmapSize", mmapSizeImpl)
	e.RegisterPrim("_mmapMember", mmapMemberImpl)
	e.RegisterPrim("_mmapToList", mmapToListImpl)
	e.RegisterPrim("_mmapFromList", mmapFromListImpl)
	e.RegisterPrim("_mmapFoldlWithKey", mmapFoldlWithKeyImpl)
	e.RegisterPrim("_mmapKeys", mmapKeysImpl)
	e.RegisterPrim("_mmapValues", mmapValuesImpl)
	e.RegisterPrim("_mmapAdjust", mmapAdjustImpl)
	return e.RegisterModule("Effect.Map", mmapSource)
}

var mmapSource = mustReadSource("mmap")

// mutMapVal is the Go-level representation of a mutable GICEL MMap.
// Unlike mapVal (persistent), operations mutate root/size in place.
type mutMapVal struct {
	root *avlNode
	cmp  eval.Value // compare :: k -> k -> Ordering
	size int
}

func (*mutMapVal) String() string { return "MMap(...)" }

func asMutMapVal(v eval.Value) (*mutMapVal, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, errExpected("stdlib/mmap", "HostVal", v)
	}
	m, ok := hv.Inner.(*mutMapVal)
	if !ok {
		return nil, errExpected("stdlib/mmap", "*mutMapVal", hv.Inner)
	}
	return m, nil
}

// _mmapNew :: (k -> k -> Ordering) -> MMap k v
func mmapNewImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	return &eval.HostVal{Inner: &mutMapVal{root: nil, cmp: cmp, size: 0}}, ce, nil
}

// _mmapInsert :: (k -> k -> Ordering) -> k -> v -> MMap k v -> ()
func mmapInsertImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	value := args[2]
	m, err := asMutMapVal(args[3])
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
	m.root = newRoot
	if inserted {
		m.size++
	}
	return unitVal, newCe, nil
}

// _mmapLookup :: (k -> k -> Ordering) -> k -> MMap k v -> Maybe v
func mmapLookupImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMutMapVal(args[2])
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

// _mmapDelete :: (k -> k -> Ordering) -> k -> MMap k v -> ()
func mmapDeleteImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMutMapVal(args[2])
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
	m.root = newRoot
	if deleted {
		m.size--
	}
	return unitVal, newCe, nil
}

// _mmapSize :: MMap k v -> Int
func mmapSizeImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMutMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(m.size)}, ce, nil
}

// _mmapMember :: (k -> k -> Ordering) -> k -> MMap k v -> Bool
func mmapMemberImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	m, err := asMutMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	_, found, newCe, err := avlLookup(m.root, key, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	return boolVal(found), newCe, nil
}

// _mmapToList :: MMap k v -> List (k, v)
func mmapToListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMutMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*(costTupleNode+costConsNode)); err != nil {
		return nil, ce, err
	}
	return avlToConsList(m.root), ce, nil
}

// _mmapFromList :: (k -> k -> Ordering) -> List (k, v) -> MMap k v
func mmapFromListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	list := args[1]
	m := &mutMapVal{root: nil, cmp: cmp, size: 0}
	for {
		con, ok := list.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("mmapFromList", "List", list)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errors.New("mmapFromList: malformed list")
		}
		pair, ok := con.Args[0].(*eval.RecordVal)
		if !ok {
			return nil, ce, errExpected("mmapFromList", "tuple", con.Args[0])
		}
		key, ok1 := pair.Get(ir.TupleLabel(1))
		value, ok2 := pair.Get(ir.TupleLabel(2))
		if !ok1 || !ok2 {
			return nil, ce, errors.New("mmapFromList: tuple must have _1 and _2")
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

// _mmapFoldlWithKey :: (b -> k -> v -> b) -> b -> MMap k v -> b
func mmapFoldlWithKeyImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	acc := args[1]
	m, err := asMutMapVal(args[2])
	if err != nil {
		return nil, ce, err
	}
	return avlFoldlWithKey(m.root, f, acc, ce, apply)
}

// _mmapKeys :: MMap k v -> List k
func mmapKeysImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMutMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costConsNode); err != nil {
		return nil, ce, err
	}
	return avlKeysToConsList(m.root), ce, nil
}

// _mmapValues :: MMap k v -> List v
func mmapValuesImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	m, err := asMutMapVal(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(m.size)*costConsNode); err != nil {
		return nil, ce, err
	}
	return avlValsToConsList(m.root), ce, nil
}

// _mmapAdjust :: (k -> k -> Ordering) -> k -> (v -> v) -> MMap k v -> ()
func mmapAdjustImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	key := args[1]
	f := args[2]
	m, err := asMutMapVal(args[3])
	if err != nil {
		return nil, ce, err
	}
	v, found, newCe, err := avlLookup(m.root, key, m.cmp, ce, apply)
	if err != nil {
		return nil, ce, err
	}
	if !found {
		return unitVal, newCe, nil
	}
	newVal, newCe, err := apply(f, v, newCe)
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, costAVLNode); err != nil {
		return nil, ce, err
	}
	m.root, _, newCe, err = avlInsert(m.root, key, newVal, m.cmp, newCe, apply)
	if err != nil {
		return nil, ce, err
	}
	return unitVal, newCe, nil
}
