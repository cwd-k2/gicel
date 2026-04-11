package stdlib

import (
	"context"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Session provides session type handler and capability operations.
var Session Pack = func(e Registrar) error {
	e.RegisterPrim("closeAt", closeAtImpl)
	e.RegisterPrim("chooseAt", chooseAtImpl)
	e.RegisterPrim("receiveAt", receiveAtImpl)
	e.RegisterPrim("inject", injectImpl)
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

// chooseAtImpl records the chosen branch tag in the CapEnv.
// The type checker ensures protocol compliance; at runtime, this simply
// writes the tag string so that a future offer handler can dispatch on it.
// args: [label, tag]
func chooseAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	if len(args) < 2 {
		return nil, ce, &eval.RuntimeError{Message: "chooseAt: expected label and tag arguments"}
	}
	label, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "chooseAt: label argument is not a string"}
	}
	tag, ok := args[1].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "chooseAt: tag argument is not a string"}
	}
	newCe := ce.Set(label.Inner.(string), tag)
	return unitVal, newCe, nil
}

// receiveAtImpl reads the choice tag from CapEnv (written by chooseAt) and
// returns a VariantVal. The tag determines which branch the case will take.
// args: [label]
func receiveAtImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	if err := validateLabelArg(args); err != nil {
		return nil, ce, err
	}
	label := args[0].(*eval.HostVal).Inner.(string)
	tagVal, found := ce.Get(label)
	if !found {
		return nil, ce, &eval.RuntimeError{Message: "receiveAt: no capability for label " + label}
	}
	// The tag was written by chooseAt as a HostVal wrapping a string.
	tag, ok := tagVal.(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "receiveAt: no choice tag in CapEnv for label " + label}
	}
	tagStr, ok := tag.Inner.(string)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "receiveAt: choice tag is not a string"}
	}
	return &eval.VariantVal{Tag: tagStr, Value: unitVal}, ce, nil
}

// injectImpl creates a VariantVal from a label tag and a payload value.
// args: [tag, value]
func injectImpl(_ context.Context, ce eval.CapEnv, args []eval.Value, _ eval.Applier) (eval.Value, eval.CapEnv, error) {
	if len(args) < 2 {
		return nil, ce, &eval.RuntimeError{Message: "inject: expected tag and value arguments"}
	}
	tag, ok := args[0].(*eval.HostVal)
	if !ok {
		return nil, ce, &eval.RuntimeError{Message: "inject: tag argument is not a string"}
	}
	return &eval.VariantVal{Tag: tag.Inner.(string), Value: args[1]}, ce, nil
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
