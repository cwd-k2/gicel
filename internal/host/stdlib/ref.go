package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Ref provides mutable reference cells gated by the { ref: () } effect.
var Ref Pack = func(e Registrar) error {
	e.RegisterPrim("_refNew", refNewImpl)
	e.RegisterPrim("_refRead", refReadImpl)
	e.RegisterPrim("_refWrite", refWriteImpl)
	// Named capability variants.
	e.RegisterPrim("_refNewNamed", withLabel(refNewImpl))
	e.RegisterPrim("_refReadNamed", withLabel(refReadImpl))
	e.RegisterPrim("_refWriteNamed", withLabel(refWriteImpl))
	e.RegisterPrim("_refModifyNamed", refModifyNamedImpl)
	return e.RegisterModule("Effect.Ref", refSource)
}

var refSource = mustReadSource("ref")

// refCell is the Go-level representation of a mutable reference.
type refCell struct {
	value eval.Value
}

func (*refCell) String() string { return "Ref(...)" }

func asRefCell(v eval.Value) (*refCell, error) {
	hv, ok := v.(*eval.HostVal)
	if !ok {
		return nil, errExpected("stdlib/ref", "HostVal", v)
	}
	r, ok := hv.Inner.(*refCell)
	if !ok {
		return nil, errExpected("stdlib/ref", "*refCell", hv.Inner)
	}
	return r, nil
}

// _refNew :: a -> Computation { ref: () | r } { ref: () | r } (Ref a)
func refNewImpl(ctx context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	if err := budget.ChargeAlloc(ctx, costSlotSize); err != nil {
		return nil, ce, err
	}
	return &eval.HostVal{Inner: &refCell{value: args[0]}}, ce, nil
}

// _refRead :: Ref a -> Computation { ref: () | r } { ref: () | r } a
func refReadImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRefCell(args[0])
	if err != nil {
		return nil, ce, err
	}
	return r.value, ce, nil
}

// _refWrite :: a -> Ref a -> Computation { ref: () | r } { ref: () | r } ()
func refWriteImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	r, err := asRefCell(args[1])
	if err != nil {
		return nil, ce, err
	}
	r.value = args[0] // in-place mutation
	return unitVal, ce, nil
}

// _refModifyNamed :: label -> (a -> a) -> Ref a -> ()
// Named variant of modify as a prim (cannot forward label through gicel-defined modify).
func refModifyNamedImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	// args[0] = label (ignored), args[1] = f, args[2] = ref
	r, err := asRefCell(args[2])
	if err != nil {
		return nil, ce, err
	}
	newVal, newCe, err := apply.Apply(args[1], r.value, ce)
	if err != nil {
		return nil, ce, err
	}
	r.value = newVal
	return unitVal, newCe, nil
}
