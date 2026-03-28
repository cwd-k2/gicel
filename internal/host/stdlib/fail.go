package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Fail provides the fail effect capability.
var Fail Pack = func(e Registrar) error {
	e.RegisterPrim("failWith", failImpl)
	e.RegisterPrim("failWithAt", failWithAtImpl)
	return e.RegisterModule("Effect.Fail", failSource)
}

var failSource = mustReadSource("fail")

func failImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	msg := "fail"
	var detail eval.Value
	if len(args) > 0 && args[0] != nil {
		detail = args[0]
		msg = "fail: " + eval.PrettyValue(detail)
	}
	return nil, ce, &eval.RuntimeError{Message: msg, Detail: detail}
}

// failWithAtImpl is the named-capability variant: label is args[0], error value is args[1].
func failWithAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	msg := "fail"
	var detail eval.Value
	if len(args) > 1 && args[1] != nil {
		detail = args[1]
		msg = "fail: " + eval.PrettyValue(detail)
	}
	return nil, ce, &eval.RuntimeError{Message: msg, Detail: detail}
}
