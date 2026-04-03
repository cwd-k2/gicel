package stdlib

import (
	"context"
	"errors"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Primitive names for slice operations used in registration and fusion rules.
const (
	primSliceMap      = "_sliceMap"
	primSliceFoldr    = "_sliceFoldr"
	primSliceToList   = "_sliceToList"
	primSliceFromList = "_sliceFromList"
)

// Optimizer-generated variable names for fusion rewrites.
const (
	optVarX   = "$opt_x"
	optVarAcc = "$opt_acc"
)

// Slice provides immutable indexed snapshots: O(1) length/index,
// fromList/toList conversion, and Functor/Foldable instances.
var Slice Pack = func(e Registrar) error {
	e.RegisterPrim("_sliceEmpty", sliceEmptyImpl)
	e.RegisterPrim("_sliceSingleton", sliceSingletonImpl)
	e.RegisterPrim("_sliceLength", sliceLengthImpl)
	e.RegisterPrim("_sliceIndex", sliceIndexImpl)
	e.RegisterPrim(primSliceFromList, sliceFromListImpl)
	e.RegisterPrim(primSliceToList, sliceToListImpl)
	e.RegisterPrim(primSliceFoldr, sliceFoldrImpl)
	e.RegisterPrim(primSliceMap, sliceMapImpl)
	e.RegisterPrim("_sliceFoldl", sliceFoldlImpl)
	// Fusion rules: domain-specific rewrites for this pack's primitives.
	e.RegisterRewriteRule(sliceMapMapFusion)
	e.RegisterRewriteRule(sliceFoldrMapFusion)
	e.RegisterRewriteRule(slicePackedRoundtrip)
	return e.RegisterModule("Data.Slice", sliceSource)
}

// --- Fusion rules (applied by optimizer) ---

// R10: _sliceMap f (_sliceMap g xs) → _sliceMap (\$x -> f (g $x)) xs
func sliceMapMapFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primSliceMap || len(po.Args) != 2 {
		return c
	}
	inner, ok := po.Args[1].(*ir.PrimOp)
	if !ok || inner.Name != primSliceMap || len(inner.Args) != 2 {
		return c
	}
	f, g, xs := po.Args[0], inner.Args[0], inner.Args[1]
	x := optVarX
	if freeIn(x, f) || freeIn(x, g) {
		return c
	}
	composed := &ir.Lam{Param: x, Body: &ir.App{
		Fun: f, Arg: &ir.App{Fun: g, Arg: &ir.Var{Name: x}},
	}}
	return &ir.PrimOp{Name: primSliceMap, Arity: 2, Args: []ir.Core{composed, xs}, S: po.S}
}

// R11: _sliceFoldr k z (_sliceMap f xs) → _sliceFoldr (\$x $acc -> k (f $x) $acc) z xs
func sliceFoldrMapFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primSliceFoldr || len(po.Args) != 3 {
		return c
	}
	inner, ok := po.Args[2].(*ir.PrimOp)
	if !ok || inner.Name != primSliceMap || len(inner.Args) != 2 {
		return c
	}
	k, z, f, xs := po.Args[0], po.Args[1], inner.Args[0], inner.Args[1]
	x, acc := optVarX, optVarAcc
	if freeIn(x, k) || freeIn(x, f) || freeIn(acc, k) || freeIn(acc, f) {
		return c
	}
	fused := &ir.Lam{Param: x, Body: &ir.Lam{Param: acc, Body: &ir.App{
		Fun: &ir.App{Fun: k, Arg: &ir.App{Fun: f, Arg: &ir.Var{Name: x}}},
		Arg: &ir.Var{Name: acc},
	}}}
	return &ir.PrimOp{Name: primSliceFoldr, Arity: 3, Args: []ir.Core{fused, z, xs}, S: po.S}
}

// R12: _sliceToList (_sliceFromList xs) → xs
func slicePackedRoundtrip(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primSliceToList || len(po.Args) != 1 {
		return c
	}
	inner, ok := po.Args[0].(*ir.PrimOp)
	if !ok || inner.Name != primSliceFromList || len(inner.Args) != 1 {
		return c
	}
	return inner.Args[0]
}

var sliceSource = mustReadSource("slice")

func asSlice(v eval.Value) ([]eval.Value, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, errExpected("stdlib/slice", "HostVal", v)
	}
	s, ok := hv.Inner.([]eval.Value)
	if !ok {
		return nil, errExpected("stdlib/slice", "[]Value", hv.Inner)
	}
	return s, nil
}

func sliceVal(items []eval.Value) eval.Value {
	return &eval.HostVal{Inner: items}
}

func sliceEmptyImpl(_ context.Context, ce eval.CapEnv, _ []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return sliceVal([]eval.Value{}), ce, nil
}

func sliceSingletonImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return sliceVal([]eval.Value{args[0]}), ce, nil
}

func sliceLengthImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(len(s))}, ce, nil
}

func sliceIndexImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	idx, err := asInt64(args[0], "slice")
	if err != nil {
		return nil, ce, err
	}
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	if idx < 0 || idx >= int64(len(s)) {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{s[idx]}}, ce, nil
}

func sliceFromListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	items, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, errors.New("sliceFromList: expected List")
	}
	if err := budget.ChargeAlloc(ctx, int64(len(items))*2*costSlotSize); err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, len(items))
	copy(result, items)
	return sliceVal(result), ce, nil
}

func sliceToListImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(len(s))*costConsNode); err != nil {
		return nil, ce, err
	}
	return buildList(s), ce, nil
}

func sliceFoldrImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	z := args[1]
	s, err := asSlice(args[2])
	if err != nil {
		return nil, ce, err
	}
	acc := z
	for i := len(s) - 1; i >= 0; i-- {
		acc, ce, err = apply.ApplyN(f, []eval.Value{s[i], acc}, ce)
		if err != nil {
			return nil, ce, err
		}
	}
	return acc, ce, nil
}

func sliceMapImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(len(s))*costSlotSize); err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, len(s))
	for i, item := range s {
		mapped, newCe, err := apply.Apply(f, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		result[i] = mapped
	}
	return sliceVal(result), ce, nil
}

func sliceFoldlImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	acc := args[1]
	s, err := asSlice(args[2])
	if err != nil {
		return nil, ce, err
	}
	for _, item := range s {
		acc, ce, err = apply.ApplyN(f, []eval.Value{acc, item}, ce)
		if err != nil {
			return nil, ce, err
		}
	}
	return acc, ce, nil
}
