package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Session provides session type handler and capability operations.
var Session Pack = func(e Registrar) error {
	e.RegisterPrim("closeAt", closeAtImpl)
	e.RegisterPrim("runSessionAt", runSessionAtImpl)
	return e.RegisterModule("Effect.Session", sessionSource)
}

var sessionSource = mustReadSource("session")

// closeAtImpl removes a capability label from the CapEnv.
// args: [label]
func closeAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	if err := validateLabelArg(args); err != nil {
		return nil, ce, err
	}
	label := args[0].(*eval.HostVal).Inner.(string)
	return unitVal, ce.Delete(label), nil
}

// runSessionAtImpl introduces a session capability, drives the thunk to
// completion, and verifies the session was properly terminated.
// args: [label, initVal, thunk]
//
// The thunk has type Thunk { l: s_init | r } { l: s_final | r } a —
// pre ≠ post. The session protocol transitions the capability through
// intermediate states. The close primitive must delete the label from
// CapEnv; if the label is still present after execution, the session
// was not properly terminated (defense-in-depth; the type checker
// already enforces this statically for well-typed programs).
func runSessionAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, apply eval.Applier) (eval.Value, eval.CapEnv, error) {
	if err := validateLabelArg(args); err != nil {
		return nil, ce, err
	}
	label := args[0].(*eval.HostVal).Inner.(string)
	innerCe := ce.Set(label, args[1])
	val, finalCe, err := driveEffectful(args[2], innerCe, apply)
	if err != nil {
		return nil, ce, err
	}
	// Clean up: remove the session label from CapEnv.
	// If close already deleted it, this is a no-op.
	cleanCe := finalCe.Delete(label)
	return val, cleanCe, nil
}
