package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/lang/types"
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
	e.RegisterPrim("runStateAt", runStateAtImpl)
	e.RegisterPrim("evalStateAt", evalStateAtImpl)
	e.RegisterPrim("execStateAt", execStateAtImpl)
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
// extracts the final state, and restores (or eliminates) the capability
// from the CapEnv. Save/restore ensures nested anonymous state handlers
// each operate on their own slot without clobbering the outer handler's
// state — they all share the same label ("state").
// On error, the original CapEnv is returned (rollback).
func doRunState(label string, initVal eval.Value, thunk eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	// Save any existing capability under this label so nested handlers
	// can coexist with an outer handler using the same label.
	savedState, hadSaved := ce.Get(label)

	innerCe := ce.Set(label, initVal)
	val, finalCe, err := driveEffectful(thunk, innerCe, apply)
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

	// Restore the outer capability if one existed; otherwise remove.
	var cleanCe eval.CapEnv
	if hadSaved {
		cleanCe = finalCe.Set(label, savedState)
	} else {
		cleanCe = finalCe.Delete(label)
	}

	tuple := eval.NewRecordFromMap(map[string]eval.Value{
		types.TupleLabel(1): stateVal,
		types.TupleLabel(2): val,
	})
	return tuple, cleanCe, nil
}

// runStateAtImpl handles the named state effect: same as runState but with
// a label argument from label erasure. args: [label, initialState, suspendedComp]
func runStateAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	label, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "runStateAt: label argument is not a string"}
	}
	name, ok := label.Inner.(string)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "runStateAt: label argument is not a string"}
	}
	return doRunStateDrive(name, args[1], args[2], ce, apply)
}

func evalStateAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	label, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "evalStateAt: label argument is not a string"}
	}
	name, ok := label.Inner.(string)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "evalStateAt: label inner is not a string"}
	}
	tuple, newCe, err := doRunStateDrive(name, args[1], args[2], ce, apply)
	if err != nil {
		return nil, ce, err
	}
	rv := tuple.(*eval.RecordVal)
	val, _ := rv.Get(types.TupleLabel(2))
	return val, newCe, nil
}

func execStateAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	label, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "execStateAt: label argument is not a string"}
	}
	name, ok := label.Inner.(string)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "execStateAt: label inner is not a string"}
	}
	tuple, newCe, err := doRunState(name, args[1], args[2], ce, apply)
	if err != nil {
		return nil, ce, err
	}
	rv := tuple.(*eval.RecordVal)
	state, _ := rv.Get(types.TupleLabel(1))
	return state, newCe, nil
}

// doRunStateDrive is like doRunState but drives the thunk via driveEffectful
// instead of relying on apply.Apply to fully execute the computation. This
// handles the case where the thunk body evaluates to a deferred effectful
// PrimVal (common with named capability primitives after label erasure).
// Same save/restore semantics as doRunState for nested handler safety.
func doRunStateDrive(label string, initVal eval.Value, thunk eval.Value, ce eval.CapEnv, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	savedState, hadSaved := ce.Get(label)

	innerCe := ce.Set(label, initVal)
	val, finalCe, err := driveEffectful(thunk, innerCe, apply)
	if err != nil {
		return nil, ce, err
	}
	raw, ok := finalCe.Get(label)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "runStateAt: state capability lost during execution"}
	}
	stateVal, ok := raw.(eval.Value)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "runStateAt: state capability is not a Value"}
	}

	var cleanCe eval.CapEnv
	if hadSaved {
		cleanCe = finalCe.Set(label, savedState)
	} else {
		cleanCe = finalCe.Delete(label)
	}

	tuple := eval.NewRecordFromMap(map[string]eval.Value{
		types.TupleLabel(1): stateVal,
		types.TupleLabel(2): val,
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
	newVal, newCe, err := apply.Apply(args[1], val, ce)
	if err != nil {
		return nil, ce, err
	}
	return unitVal, newCe.Set(name, newVal), nil
}
