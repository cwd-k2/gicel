package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/eval"
)

// Fail provides the fail effect capability.
var Fail Pack = func(e Registrar) error {
	e.RegisterPrim("failWith", failImpl)
	return e.RegisterModule("Effect.Fail", failSource)
}

var failSource = mustReadSource("fail")

func failImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	return nil, ce, &eval.RuntimeError{Message: "fail"}
}
