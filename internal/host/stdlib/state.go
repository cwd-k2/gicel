package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// capState is the canonical capability key for the anonymous state effect.
const capState = "state"

// State provides get/put state capabilities and runState effect handler.
var State Pack = func(e Registrar) error {
	e.RegisterPrim("get", getImpl)
	e.RegisterPrim("put", putImpl)
	e.RegisterPrim("getAt", getAtImpl)
	e.RegisterPrim("putAt", putAtImpl)
	e.RegisterPrim("modifyAt", modifyAtImpl)
	e.RegisterPrim("_runState", runStateImpl)
	return e.RegisterModule("Effect.State", stateSource)
}

var stateSource = mustReadSource("state")

func getImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	v, ok := ce.Get(capState)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "get: no state capability"}
	}
	val, ok2 := v.(eval.Value)
	if !ok2 {
		return nil, ce, &eval.RuntimeError{Message: "get: state capability is not a Value"}
	}
	return val, ce, nil
}

func putImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	newCe := ce.Set(capState, args[0])
	return unitVal, newCe, nil
}

// Named capability variants: label is the first argument (erased from type-level label literal).

func getAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	label, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "getAt: label argument is not a string"}
	}
	name, ok := label.Inner.(string)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "getAt: label argument is not a string"}
	}
	v, ok := ce.Get(name)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "getAt: no capability " + name}
	}
	val, ok := v.(eval.Value)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "getAt: capability " + name + " is not a Value"}
	}
	return val, ce, nil
}

func putAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	label, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "putAt: label argument is not a string"}
	}
	name, ok := label.Inner.(string)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "putAt: label argument is not a string"}
	}
	newCe := ce.Set(name, args[1])
	return unitVal, newCe, nil
}

// runStateImpl handles the anonymous state effect: introduce capability, run,
// extract final state, eliminate capability. Follows the tryImpl pattern.
// args: [initialState, suspendedComputation]
func runStateImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	return doRunState(capState, args[0], args[1], ce, apply)
}

// doRunState is the shared handler logic for runState.
// It introduces a state capability, forces the suspended computation,
// extracts the final state, and eliminates the capability from the CapEnv.
// On error, the original CapEnv is returned (rollback).
func doRunState(label string, initVal eval.Value, thunk eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	innerCe := ce.Set(label, initVal)
	val, finalCe, err := apply(thunk, unitVal, innerCe)
	if err != nil {
		return nil, ce, err
	}
	raw, ok := finalCe.Get(label)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "runState: state capability lost during execution"}
	}
	stateVal, ok := raw.(eval.Value)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "runState: state capability is not a Value"}
	}
	cleanCe := finalCe.Delete(label)
	tuple := eval.NewRecordFromMap(map[string]eval.Value{
		ir.TupleLabel(1): stateVal,
		ir.TupleLabel(2): val,
	})
	return tuple, cleanCe, nil
}

// modifyAt :: label -> (s -> s) -> Effect { l: s | r } ()
func modifyAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	label, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "modifyAt: label argument is not a string"}
	}
	name, ok := label.Inner.(string)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "modifyAt: label argument is not a string"}
	}
	v, ok := ce.Get(name)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "modifyAt: no capability " + name}
	}
	val, ok := v.(eval.Value)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "modifyAt: capability " + name + " is not a Value"}
	}
	newVal, newCe, err := apply(args[1], val, ce)
	if err != nil {
		return nil, ce, err
	}
	return unitVal, newCe.Set(name, newVal), nil
}
