package stdlib

import (
	"context"
	"errors"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// fromSliceImpl converts a HostVal([]any) to a ConVal chain (Cons/Nil).
// If the input is already a ConVal chain, it passes through unchanged.
func fromSliceImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	v := args[0]
	// If already a ConVal (Cons/Nil), pass through.
	if con, ok := v.(*eval.ConVal); ok {
		if con.Con == "Cons" || con.Con == "Nil" {
			return v, ce, nil
		}
	}
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, ce, errExpected("fromSlice", "HostVal or List", v)
	}
	slice, ok := hv.Inner.([]any)
	if !ok {
		return nil, ce, errExpected("fromSlice", "[]any", hv.Inner)
	}
	return sliceToList(slice), ce, nil
}

// toSliceImpl converts a ConVal chain (Cons/Nil) to a HostVal([]any).
// If the input is already a HostVal([]any), it passes through unchanged.
func toSliceImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	v := args[0]
	if hv, ok := v.(*eval.HostVal); ok {
		if _, ok := hv.Inner.([]any); ok {
			return v, ce, nil
		}
	}
	items, ok := listToSlice(v)
	if !ok {
		return nil, ce, errExpected("toSlice", "List (Cons/Nil)", v)
	}
	if err := budget.ChargeAlloc(ctx, int64(len(items))*costSlotSize); err != nil {
		return nil, ce, err
	}
	anys := make([]any, len(items))
	for i, item := range items {
		anys[i] = item
	}
	return &eval.HostVal{Inner: anys}, ce, nil
}

// lengthImpl counts the elements of a ConVal chain.
func lengthImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n := int64(0)
	v := args[0]
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("length", "List", v)
		}
		if con.Con == "Nil" {
			return &eval.HostVal{Inner: n}, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("length", "list node", con.Con)
		}
		n++
		if n&1023 == 0 {
			if err := budget.CheckContext(ctx); err != nil {
				return nil, ce, err
			}
		}
		v = con.Args[1]
	}
}

// concatImpl concatenates two ConVal chain lists.
func concatImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	xs, ok := listToSlice(args[0])
	if !ok {
		return nil, ce, errors.New("concat: first argument is not a List")
	}
	if err := budget.ChargeAlloc(ctx, int64(len(xs))*costConsNode); err != nil {
		return nil, ce, err
	}
	ys := args[1]
	// Build result from the end: start with ys, prepend xs elements.
	result := ys
	for i := len(xs) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{xs[i], result}}
	}
	return result, ce, nil
}

// sliceToList converts a Go []any to a ConVal chain.
func sliceToList(items []any) eval.Value {
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(eval.Value)
		if !ok {
			item = &eval.HostVal{Inner: items[i]}
		}
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{item, result}}
	}
	return result
}

// listToSlice converts a ConVal chain to a Go []eval.Value.
func listToSlice(v eval.Value) ([]eval.Value, bool) {
	var result []eval.Value
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, false
		}
		switch con.Con {
		case "Nil":
			return result, true
		case "Cons":
			if len(con.Args) != 2 {
				return nil, false
			}
			result = append(result, con.Args[0])
			v = con.Args[1]
		default:
			return nil, false
		}
	}
}

// --- New List primitives ---

func asInt64List(v eval.Value) (int64, error) { return asInt64(v, "list") }

func takeImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	if n <= 0 {
		return &eval.ConVal{Con: "Nil"}, ce, nil
	}
	v := args[1]
	var prefix []eval.Value
	for i := int64(0); i < n; i++ {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("take", "List", v)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("take", "list node", con.Con)
		}
		prefix = append(prefix, con.Args[0])
		v = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, int64(len(prefix))*costConsNode); err != nil {
		return nil, ce, err
	}
	return buildList(prefix), ce, nil
}

func dropImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	if n <= 0 {
		return args[1], ce, nil
	}
	v := args[1]
	for i := int64(0); i < n; i++ {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("drop", "List", v)
		}
		if con.Con == "Nil" {
			return v, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("drop", "list node", con.Con)
		}
		v = con.Args[1]
	}
	return v, ce, nil
}

func indexImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	idx, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	v := args[1]
	var i int64
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, errors.New("index: expected List")
		}
		if con.Con == "Nil" {
			return &eval.ConVal{Con: "Nothing"}, ce, nil
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("index", "list node", con.Con)
		}
		if i == idx {
			return &eval.ConVal{Con: "Just", Args: []eval.Value{con.Args[0]}}, ce, nil
		}
		i++
		v = con.Args[1]
	}
}

func replicateImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	if n <= 0 {
		return &eval.ConVal{Con: "Nil"}, ce, nil
	}
	if err := budget.ChargeAlloc(ctx, n*costConsNode); err != nil {
		return nil, ce, err
	}
	elem := args[1]
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := int64(0); i < n; i++ {
		if i&1023 == 0 {
			if err := budget.CheckContext(ctx); err != nil {
				return nil, ce, err
			}
		}
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{elem, result}}
	}
	return result, ce, nil
}

func reverseImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	// Pass 1: count elements and validate structure.
	v := args[0]
	var n int64
	for {
		con, ok := v.(*eval.ConVal)
		if !ok {
			return nil, ce, errExpected("reverse", "List", v)
		}
		if con.Con == "Nil" {
			break
		}
		if con.Con != "Cons" || len(con.Args) != 2 {
			return nil, ce, errMalformed("reverse", "list node", con.Con)
		}
		n++
		v = con.Args[1]
	}
	if err := budget.ChargeAlloc(ctx, n*costConsNode); err != nil {
		return nil, ce, err
	}
	// Pass 2: build reversed list via foldl-cons.
	v = args[0]
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := int64(0); i < n; i++ {
		con := v.(*eval.ConVal)
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{con.Args[0], result}}
		v = con.Args[1]
	}
	return result, ce, nil
}

// rangeImpl generates an inclusive integer range [from..to].
func rangeImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	from, err := asInt64List(args[0])
	if err != nil {
		return nil, ce, err
	}
	to, err := asInt64List(args[1])
	if err != nil {
		return nil, ce, err
	}
	if from > to {
		return &eval.ConVal{Con: "Nil"}, ce, nil
	}
	n := to - from + 1
	if err := budget.ChargeAlloc(ctx, n*costConsNode); err != nil {
		return nil, ce, err
	}
	items := make([]eval.Value, n)
	for i := int64(0); i < n; i++ {
		if i&1023 == 0 {
			if err := budget.CheckContext(ctx); err != nil {
				return nil, ce, err
			}
		}
		items[i] = &eval.HostVal{Inner: from + i}
	}
	return buildList(items), ce, nil
}

// Primitive names for list operations used in registration and fusion rules.
const (
	primListFromSlice = "_listFromSlice"
	primListToSlice   = "_listToSlice"
	primListMap       = "_listMap"
	primListFoldr     = "_listFoldr"
)

// R15: _listMap f (_listMap g xs) → _listMap (\$x -> f (g $x)) xs
func listMapMapFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primListMap || len(po.Args) != 2 {
		return c
	}
	inner, ok := po.Args[1].(*ir.PrimOp)
	if !ok || inner.Name != primListMap || len(inner.Args) != 2 {
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
	return &ir.PrimOp{Name: primListMap, Arity: 2, Args: []ir.Core{composed, xs}, S: po.S}
}

// R16: _listFoldr k z (_listMap f xs) → _listFoldr (\$x $acc -> k (f $x) $acc) z xs
func listFoldrMapFusion(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || po.Name != primListFoldr || len(po.Args) != 3 {
		return c
	}
	inner, ok := po.Args[2].(*ir.PrimOp)
	if !ok || inner.Name != primListMap || len(inner.Args) != 2 {
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
	return &ir.PrimOp{Name: primListFoldr, Arity: 3, Args: []ir.Core{fused, z, xs}, S: po.S}
}

// R14: _listFromSlice (_listToSlice xs) → xs  and  _listToSlice (_listFromSlice xs) → xs
func listPackedRoundtrip(c ir.Core) ir.Core {
	po, ok := c.(*ir.PrimOp)
	if !ok || len(po.Args) != 1 {
		return c
	}
	inner, ok := po.Args[0].(*ir.PrimOp)
	if !ok || len(inner.Args) != 1 {
		return c
	}
	if po.Name == primListFromSlice && inner.Name == primListToSlice {
		return inner.Args[0]
	}
	if po.Name == primListToSlice && inner.Name == primListFromSlice {
		return inner.Args[0]
	}
	return c
}

// buildList creates a ConVal Cons/Nil chain from a slice.
func buildList(items []eval.Value) eval.Value {
	var result eval.Value = &eval.ConVal{Con: "Nil"}
	for i := len(items) - 1; i >= 0; i-- {
		result = &eval.ConVal{Con: "Cons", Args: []eval.Value{items[i], result}}
	}
	return result
}
