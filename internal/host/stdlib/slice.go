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
	e.RegisterPrim("_sliceFilter", sliceFilterImpl)
	e.RegisterPrim("_sliceReverse", sliceReverseImpl)
	e.RegisterPrim("_sliceZipWith", sliceZipWithImpl)
	e.RegisterPrim("_sliceConcat", sliceConcatImpl)
	e.RegisterPrim("_sliceReplicate", sliceReplicateImpl)
	e.RegisterPrim("_sliceGenerate", sliceGenerateImpl)
	e.RegisterPrim("_sliceRange", sliceRangeImpl)
	e.RegisterPrim("_sliceSlice", sliceSliceImpl)
	e.RegisterPrim("_sliceSortBy", sliceSortByImpl)
	e.RegisterPrim("_sliceAny", sliceAnyImpl)
	e.RegisterPrim("_sliceAll", sliceAllImpl)
	e.RegisterPrim("_sliceFind", sliceFindImpl)
	e.RegisterPrim("_sliceFindIndex", sliceFindIndexImpl)
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

// --- Slice enhancement (2026-04-11) ---

func sliceFilterImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	var result []eval.Value
	for _, item := range s {
		val, newCe, err := apply.Apply(pred, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		if con, ok := val.(*eval.ConVal); ok && con.Con == "True" {
			result = append(result, item)
		}
	}
	if err := budget.ChargeAlloc(ctx, int64(len(result))*costSlotSize); err != nil {
		return nil, ce, err
	}
	return sliceVal(result), ce, nil
}

func sliceReverseImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	n := len(s)
	if err := budget.ChargeAlloc(ctx, int64(n)*costSlotSize); err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, n)
	for i, v := range s {
		result[n-1-i] = v
	}
	return sliceVal(result), ce, nil
}

func sliceZipWithImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	f := args[0]
	sa, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	sb, err := asSlice(args[2])
	if err != nil {
		return nil, ce, err
	}
	n := len(sa)
	if len(sb) < n {
		n = len(sb)
	}
	if err := budget.ChargeAlloc(ctx, int64(n)*costSlotSize); err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, n)
	for i := range n {
		val, newCe, err := apply.ApplyN(f, []eval.Value{sa[i], sb[i]}, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		result[i] = val
	}
	return sliceVal(result), ce, nil
}

func sliceConcatImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	outer, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	total := 0
	for _, item := range outer {
		inner, err := asSlice(item)
		if err != nil {
			return nil, ce, err
		}
		total += len(inner)
	}
	if err := budget.ChargeAlloc(ctx, int64(total)*costSlotSize); err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, 0, total)
	for _, item := range outer {
		inner, _ := asSlice(item)
		result = append(result, inner...)
	}
	return sliceVal(result), ce, nil
}

func sliceReplicateImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "slice")
	if err != nil {
		return nil, ce, err
	}
	if n <= 0 {
		return sliceVal(nil), ce, nil
	}
	if err := budget.ChargeAlloc(ctx, n*costSlotSize); err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, n)
	for i := range result {
		result[i] = args[1]
	}
	return sliceVal(result), ce, nil
}

func sliceGenerateImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "slice")
	if err != nil {
		return nil, ce, err
	}
	f := args[1]
	if n <= 0 {
		return sliceVal(nil), ce, nil
	}
	if err := budget.ChargeAlloc(ctx, n*costSlotSize); err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, n)
	for i := range result {
		val, newCe, err := apply.Apply(f, eval.IntVal(int64(i)), ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		result[i] = val
	}
	return sliceVal(result), ce, nil
}

func sliceRangeImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	lo, err := asInt64(args[0], "slice")
	if err != nil {
		return nil, ce, err
	}
	hi, err := asInt64(args[1], "slice")
	if err != nil {
		return nil, ce, err
	}
	if lo > hi {
		return sliceVal(nil), ce, nil
	}
	n := hi - lo
	if err := budget.ChargeAlloc(ctx, n*costSlotSize); err != nil {
		return nil, ce, err
	}
	result := make([]eval.Value, n)
	for i := range result {
		result[i] = eval.IntVal(lo + int64(i))
	}
	return sliceVal(result), ce, nil
}

func sliceSliceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	start, err := asInt64(args[0], "slice")
	if err != nil {
		return nil, ce, err
	}
	end, err := asInt64(args[1], "slice")
	if err != nil {
		return nil, ce, err
	}
	s, err := asSlice(args[2])
	if err != nil {
		return nil, ce, err
	}
	n := int64(len(s))
	if start < 0 {
		start = 0
	}
	if end > n {
		end = n
	}
	if start >= end {
		return sliceVal(nil), ce, nil
	}
	return sliceVal(s[start:end]), ce, nil
}

func sliceSortByImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	cmp := args[0]
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	if len(s) <= 1 {
		return sliceVal(s), ce, nil
	}
	if err := budget.ChargeAlloc(ctx, int64(len(s))*costSlotSize); err != nil {
		return nil, ce, err
	}
	// Copy to avoid mutating original.
	sorted := make([]eval.Value, len(s))
	copy(sorted, s)
	// Insertion sort (stable, simple, good for small slices).
	var sortErr error
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 {
			val, newCe, err := apply.ApplyN(cmp, []eval.Value{sorted[j], key}, ce)
			if err != nil {
				sortErr = err
				break
			}
			ce = newCe
			// Ordering: LT=-1, EQ=0, GT=1
			ord := int64(1) // default: GT (don't move)
			if con, ok := val.(*eval.ConVal); ok {
				switch con.Con {
				case "LT":
					ord = -1
				case "EQ":
					ord = 0
				case "GT":
					ord = 1
				}
			}
			if ord <= 0 {
				break
			}
			sorted[j+1] = sorted[j]
			j--
		}
		if sortErr != nil {
			return nil, ce, sortErr
		}
		sorted[j+1] = key
	}
	return sliceVal(sorted), ce, nil
}

func sliceAnyImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	for _, item := range s {
		val, newCe, err := apply.Apply(pred, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		if con, ok := val.(*eval.ConVal); ok && con.Con == "True" {
			return &eval.ConVal{Con: "True"}, ce, nil
		}
	}
	return &eval.ConVal{Con: "False"}, ce, nil
}

func sliceAllImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	for _, item := range s {
		val, newCe, err := apply.Apply(pred, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		if con, ok := val.(*eval.ConVal); ok && con.Con == "False" {
			return &eval.ConVal{Con: "False"}, ce, nil
		}
	}
	return &eval.ConVal{Con: "True"}, ce, nil
}

func sliceFindImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	for _, item := range s {
		val, newCe, err := apply.Apply(pred, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		if con, ok := val.(*eval.ConVal); ok && con.Con == "True" {
			return &eval.ConVal{Con: "Just", Args: []eval.Value{item}}, ce, nil
		}
	}
	return &eval.ConVal{Con: "Nothing"}, ce, nil
}

func sliceFindIndexImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	pred := args[0]
	s, err := asSlice(args[1])
	if err != nil {
		return nil, ce, err
	}
	for i, item := range s {
		val, newCe, err := apply.Apply(pred, item, ce)
		if err != nil {
			return nil, ce, err
		}
		ce = newCe
		if con, ok := val.(*eval.ConVal); ok && con.Con == "True" {
			return &eval.ConVal{Con: "Just", Args: []eval.Value{eval.IntVal(int64(i))}}, ce, nil
		}
	}
	return &eval.ConVal{Con: "Nothing"}, ce, nil
}
