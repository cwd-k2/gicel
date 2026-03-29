package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Array provides mutable fixed-size arrays with O(1) read/write,
// gated by the { array: () } effect.
var Array Pack = func(e Registrar) error {
	e.RegisterPrim("_arrayNew", arrayNewImpl)
	e.RegisterPrim("_arrayRead", arrayReadImpl)
	e.RegisterPrim("_arrayWrite", arrayWriteImpl)
	e.RegisterPrim("_arraySize", arraySizeImpl)
	e.RegisterPrim("_arrayResize", arrayResizeImpl)
	e.RegisterPrim("_arrayFromSlice", arrayFromSliceImpl)
	e.RegisterPrim("_arrayToSlice", arrayToSliceImpl)
	// Named capability variants: label at args[0], stripped by withLabel.
	e.RegisterPrim("_arrayNewNamed", withLabel(arrayNewImpl))
	e.RegisterPrim("_arrayReadNamed", withLabel(arrayReadImpl))
	e.RegisterPrim("_arrayWriteNamed", withLabel(arrayWriteImpl))
	e.RegisterPrim("_arrayResizeNamed", withLabel(arrayResizeImpl))
	e.RegisterPrim("_arrayFromSliceNamed", withLabel(arrayFromSliceImpl))
	e.RegisterPrim("_arrayToSliceNamed", withLabel(arrayToSliceImpl))
	return e.RegisterModule("Effect.Array", arraySource)
}

var arraySource = mustReadSource("array")

// mutArray is the Go-level representation of a mutable GICEL Array.
// Distinguished from Slice ([]eval.Value) to prevent aliasing.
type mutArray struct {
	data []eval.Value
}

func asMutArray(v eval.Value) (*mutArray, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, errExpected("stdlib/array", "HostVal", v)
	}
	a, ok := hv.Inner.(*mutArray)
	if !ok {
		return nil, errExpected("stdlib/array", "*mutArray", hv.Inner)
	}
	return a, nil
}

func arrayVal(a *mutArray) eval.Value {
	return &eval.HostVal{Inner: a}
}

func arrayNewImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "array")
	if err != nil {
		return nil, ce, err
	}
	if n < 0 {
		return nil, ce, fmt.Errorf("array.new: negative size %d", n)
	}
	if err := budget.ChargeAlloc(ctx, n*costSlotSize); err != nil {
		return nil, ce, err
	}
	data := make([]eval.Value, n)
	for i := range data {
		data[i] = args[1]
	}
	return arrayVal(&mutArray{data: data}), ce, nil
}

func arrayReadImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	idx, err := asInt64(args[0], "array")
	if err != nil {
		return nil, ce, err
	}
	a, err := asMutArray(args[1])
	if err != nil {
		return nil, ce, err
	}
	if idx < 0 || idx >= int64(len(a.data)) {
		return &eval.ConVal{Con: "Nothing"}, ce, nil
	}
	return &eval.ConVal{Con: "Just", Args: []eval.Value{a.data[idx]}}, ce, nil
}

func arrayWriteImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	idx, err := asInt64(args[0], "array")
	if err != nil {
		return nil, ce, err
	}
	a, err := asMutArray(args[2])
	if err != nil {
		return nil, ce, err
	}
	if idx < 0 || idx >= int64(len(a.data)) {
		return unitVal, ce, nil
	}
	a.data[idx] = args[1] // in-place mutation
	return unitVal, ce, nil
}

func arraySizeImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asMutArray(args[0])
	if err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: int64(len(a.data))}, ce, nil
}

func arrayResizeImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	n, err := asInt64(args[0], "array")
	if err != nil {
		return nil, ce, err
	}
	a, err := asMutArray(args[2])
	if err != nil {
		return nil, ce, err
	}
	if n < 0 {
		return nil, ce, fmt.Errorf("array.resize: negative size %d", n)
	}
	if err := budget.ChargeAlloc(ctx, n*costSlotSize); err != nil {
		return nil, ce, err
	}
	data := make([]eval.Value, n)
	copied := copy(data, a.data)
	for i := copied; i < int(n); i++ {
		data[i] = args[1] // fill with default
	}
	return arrayVal(&mutArray{data: data}), ce, nil
}

func arrayFromSliceImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	s, err := asSlice(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(len(s))*costSlotSize); err != nil {
		return nil, ce, err
	}
	data := make([]eval.Value, len(s))
	copy(data, s)
	return arrayVal(&mutArray{data: data}), ce, nil
}

func arrayToSliceImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	a, err := asMutArray(args[0])
	if err != nil {
		return nil, ce, err
	}
	if err := budget.ChargeAlloc(ctx, int64(len(a.data))*costSlotSize); err != nil {
		return nil, ce, err
	}
	data := make([]eval.Value, len(a.data))
	copy(data, a.data)
	return sliceVal(data), ce, nil
}
