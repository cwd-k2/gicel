package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Fail provides the fail effect capability.
var Fail Pack = func(e Registrar) error {
	e.RegisterPrim("failWith", failImpl)
	e.RegisterPrim("failWithAt", withLabel(failImpl))
	e.RegisterPrim("_try", tryImpl)
	e.RegisterPrim("tryAt", tryAtImpl)
	return e.RegisterModule("Effect.Fail", failSource)
}

var failSource = mustReadSource("fail")

// tryImpl forces a suspended computation and catches fail effects.
// Success → Ok(value) with updated CapEnv. Failure → Err(detail) with original CapEnv (rollback).
func tryImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	val, newCe, err := apply.Apply(args[0], unitVal, ce)
	if err != nil {
		if re, ok := err.(*eval.RuntimeError); ok && re.Detail != nil {
			return &eval.ConVal{Con: "Err", Args: []eval.Value{re.Detail}}, ce, nil
		}
		// Non-fail errors (budget, depth, etc.) propagate unchanged.
		return nil, ce, err
	}
	return &eval.ConVal{Con: "Ok", Args: []eval.Value{val}}, newCe, nil
}

// tryAtImpl handles the named fail effect: same as try but with a label
// argument from label erasure. args: [label, suspendedComp]
// Currently label-agnostic (catches all fail effects regardless of label).
// The suspended computation must be driven to completion by applying unitVal
// and then driving the resulting effectful value (see driveEffectful).
func tryAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	// args[0] = label, args[1] = suspended computation
	val, newCe, err := driveEffectful(args[1], ce, apply)
	if err != nil {
		if re, ok := err.(*eval.RuntimeError); ok && re.Detail != nil {
			return &eval.ConVal{Con: "Err", Args: []eval.Value{re.Detail}}, ce, nil
		}
		return nil, ce, err
	}
	return &eval.ConVal{Con: "Ok", Args: []eval.Value{val}}, newCe, nil
}

func failImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	msg := "fail"
	var detail eval.Value
	if len(args) > 0 && args[0] != nil {
		detail = args[0]
		msg = "fail: " + eval.PrettyValue(detail)
	}
	return nil, ce, &eval.RuntimeError{Message: msg, Detail: detail}
}
