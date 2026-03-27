package stdlib

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Ref provides mutable reference cells gated by the { ref: () } effect.
var Ref Pack = func(e Registrar) error {
	e.RegisterPrim("_refNew", refNewImpl)
	e.RegisterPrim("_refRead", refReadImpl)
	e.RegisterPrim("_refWrite", refWriteImpl)
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
		return nil, fmt.Errorf("stdlib/ref: expected HostVal, got %T", v)
	}
	r, ok := hv.Inner.(*refCell)
	if !ok {
		return nil, fmt.Errorf("stdlib/ref: expected *refCell, got %T", hv.Inner)
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
