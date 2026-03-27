package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// State provides get/put state capabilities.
var State Pack = func(e Registrar) error {
	e.RegisterPrim("get", getImpl)
	e.RegisterPrim("put", putImpl)
	e.RegisterPrim("getAt", getAtImpl)
	e.RegisterPrim("putAt", putAtImpl)
	return e.RegisterModule("Effect.State", stateSource)
}

var stateSource = mustReadSource("state")

func getImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	v, ok := ce.Get("state")
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
	newCe := ce.Set("state", args[0])
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
