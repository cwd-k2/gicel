package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/eval"
)

// State provides get/put state capabilities.
var State Pack = func(e Registrar) error {
	e.RegisterPrim("get", getImpl)
	e.RegisterPrim("put", putImpl)
	return e.RegisterModule("Std.State", stateSource)
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
	return &eval.RecordVal{Fields: map[string]eval.Value{}}, newCe, nil
}
