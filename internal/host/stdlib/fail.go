package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Fail provides the fail effect capability.
var Fail Pack = func(e Registrar) error {
	e.RegisterPrim("failWith", failImpl)
	e.RegisterPrim("failWithAt", failAtImpl)
	e.RegisterPrim("_try", tryImpl)
	e.RegisterPrim("tryAt", tryAtImpl)
	return e.RegisterModule("Effect.Fail", failSource)
}

var failSource = mustReadSource("fail")

// tryImpl forces a suspended computation and catches anonymous fail effects.
// Only catches RuntimeErrors with Detail != nil and FailLabel == "" (anonymous).
// Named fail effects (FailLabel != "") propagate to outer handlers.
// Uses driveEffectful to force deferred effectful tails (e.g. function calls
// in do-block tail position), matching tryAtImpl's behavior.
func tryImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	val, newCe, err := driveEffectful(args[0], ce, apply)
	if err != nil {
		if re, ok := err.(*eval.RuntimeError); ok && re.Detail != nil && re.FailLabel == "" {
			return &eval.ConVal{Con: "Err", Args: []eval.Value{re.Detail}}, ce, nil
		}
		return nil, ce, err
	}
	return &eval.ConVal{Con: "Ok", Args: []eval.Value{val}}, newCe, nil
}

// tryAtImpl handles a named fail effect. Only catches RuntimeErrors whose
// FailLabel matches the handler's label. Non-matching labeled errors and
// anonymous errors propagate unchanged.
func tryAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	if err := validateLabelArg(args); err != nil {
		return nil, ce, err
	}
	label := args[0].(*eval.HostVal).Inner.(string)
	val, newCe, err := driveEffectful(args[1], ce, apply)
	if err != nil {
		if re, ok := err.(*eval.RuntimeError); ok && re.Detail != nil && re.FailLabel == label {
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

// failAtImpl is the named variant. args: [label, detail].
// Tags the RuntimeError with FailLabel so only the matching tryAt handler
// catches it; other handlers and anonymous try let it propagate.
func failAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	if err := validateLabelArg(args); err != nil {
		return nil, ce, err
	}
	label := args[0].(*eval.HostVal).Inner.(string)
	msg := "fail"
	var detail eval.Value
	if len(args) > 1 && args[1] != nil {
		detail = args[1]
		msg = "fail: " + eval.PrettyValue(detail)
	}
	return nil, ce, &eval.RuntimeError{Message: msg, Detail: detail, FailLabel: label}
}
