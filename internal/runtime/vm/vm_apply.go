package vm

import (
	"fmt"
	"runtime"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// apply dispatches a function application. If tail is true, the current frame
// is reused (TCO). Otherwise a new frame is pushed.
func (vm *VM) apply(fn eval.Value, arg eval.Value, frame *Frame, tail bool) error {
	switch f := fn.(type) {
	case *eval.VMClosure:
		proto := f.Proto.(*Proto)
		var leaveObs bool
		if vm.obs != nil && f.Name != "" {
			if vm.obs.IsInternal(f.Name) {
				vm.obs.EnterInternal()
				leaveObs = true
			} else if vm.obs.Active() {
				vm.obs.Emit(vm.budget.Depth(), eval.ExplainLabel,
					eval.ExplainDetail{Name: f.Name, LabelKind: "enter", Value: eval.PrettyValue(arg)},
					frame.proto.SpanAt(frame.ip))
			}
		}
		if tail {
			// Before reusing the frame for a tail call, fire any pending
			// LeaveInternal from the current frame to keep suppress balanced.
			if frame.leaveObs {
				vm.obs.LeaveInternal()
			}
			frame.leaveObs = leaveObs
			return vm.tailCallClosure(proto, f.Captured, arg, frame)
		}
		err := vm.callClosure(proto, f.Captured, arg, frame)
		if err == nil && leaveObs {
			vm.currentFrame().leaveObs = true
		}
		return err

	case *eval.Closure:
		return vm.runtimeError(
			fmt.Sprintf("tree-walker Closure in VM context (invariant violation: tree-walker type in VM-only execution): %s", f.Param), frame)

	case *eval.ConVal:
		if err := vm.budget.Alloc(eval.CostConBase + int64(eval.CostConArg*(len(f.Args)+1))); err != nil {
			return err
		}
		args := make([]eval.Value, len(f.Args)+1)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		vm.push(&eval.ConVal{Con: f.Con, Args: args})
		return nil

	case *eval.PrimVal:
		return vm.applyPrim(f, arg, frame)

	default:
		msg := "application of non-function"
		if fn != nil {
			msg = fmt.Sprintf("application of non-function: %s", fn)
		}
		return vm.runtimeError(msg, frame)
	}
}

// callClosure pushes a new frame for a VMClosure call.
// Local layout: [captures...] [self (if fix)] [param] [body locals...]
func (vm *VM) callClosure(proto *Proto, captured []eval.Value, arg eval.Value, frame *Frame) error {
	if err := vm.budget.Enter(); err != nil {
		return err
	}
	bp := len(vm.locals)
	vm.ensureLocals(bp + proto.NumLocals)
	// Copy captures into slots 0..numCaptures-1.
	copy(vm.locals[bp:], captured)
	// Store parameter after captures (and after self-ref slot if fix).
	if proto.ParamName != "" {
		paramSlot := len(proto.Captures)
		if proto.FixSelfSlot >= 0 {
			paramSlot = proto.FixSelfSlot + 1
		}
		vm.locals[bp+paramSlot] = arg
	}
	vm.pushFrame(proto, bp, frame.capEnv, proto.Source)
	if proto.Source != nil && vm.obs != nil {
		vm.obs.SetSource(proto.Source)
	}
	return nil
}

// tailCallClosure reuses the current frame for a tail call.
func (vm *VM) tailCallClosure(proto *Proto, captured []eval.Value, arg eval.Value, frame *Frame) error {
	bp := frame.bp
	// Clear all old locals (may be more than new proto needs) to avoid
	// holding stale references that prevent GC.
	oldNumLocals := frame.proto.NumLocals
	newNumLocals := proto.NumLocals
	clearN := oldNumLocals
	if newNumLocals > clearN {
		clearN = newNumLocals
	}
	vm.ensureLocals(bp + clearN)
	for i := range clearN {
		vm.locals[bp+i] = nil
	}
	// Shrink locals to the new frame's requirement.
	vm.locals = vm.locals[:bp+newNumLocals]
	// Copy captures.
	copy(vm.locals[bp:], captured)
	// Store parameter.
	if proto.ParamName != "" {
		paramSlot := len(proto.Captures)
		if proto.FixSelfSlot >= 0 {
			paramSlot = proto.FixSelfSlot + 1
		}
		vm.locals[bp+paramSlot] = arg
	}
	frame.proto = proto
	frame.ip = 0
	frame.source = proto.Source
	return nil
}

// force evaluates a ThunkVal.
func (vm *VM) force(v eval.Value, frame *Frame, tail bool) error {
	switch thv := v.(type) {
	case *eval.VMThunkVal:
		proto := thv.Proto.(*Proto)
		if tail {
			return vm.tailCallThunk(proto, thv.Captured, frame)
		}
		return vm.callThunk(proto, thv.Captured, frame)

	case *eval.ThunkVal:
		return vm.runtimeError("tree-walker ThunkVal in VM context (invariant violation: tree-walker type in VM-only execution)", frame)

	default:
		return vm.runtimeError(fmt.Sprintf("force on non-thunk: %s", v), frame)
	}
}

// callThunk pushes a new frame for a thunk force.
func (vm *VM) callThunk(proto *Proto, captured []eval.Value, frame *Frame) error {
	if err := vm.budget.Enter(); err != nil {
		return err
	}
	bp := len(vm.locals)
	vm.ensureLocals(bp + proto.NumLocals)
	copy(vm.locals[bp:], captured)
	vm.pushFrame(proto, bp, frame.capEnv, proto.Source)
	return nil
}

// tailCallThunk reuses the current frame for a tail thunk force.
func (vm *VM) tailCallThunk(proto *Proto, captured []eval.Value, frame *Frame) error {
	bp := frame.bp
	vm.ensureLocals(bp + proto.NumLocals)
	for i := 0; i < proto.NumLocals; i++ {
		vm.locals[bp+i] = nil
	}
	copy(vm.locals[bp:], captured)
	frame.proto = proto
	frame.ip = 0
	frame.source = proto.Source
	return nil
}

// forceEffectful handles auto-force thunks and saturated effectful PrimVals.
// callSpan is the source span of the call site (for observer events).
func (vm *VM) forceEffectful(v eval.Value, capEnv eval.CapEnv, frame *Frame, callSpan span.Span) (eval.Value, eval.CapEnv, error) {
	// Auto-force rec thunks.
	if thv, ok := v.(*eval.VMThunkVal); ok && thv.AutoForce {
		if err := vm.budget.Step(); err != nil {
			return nil, capEnv, err
		}
		proto := thv.Proto.(*Proto)
		// Execute via barrier frame. Save state for robust cleanup.
		savedLocals := len(vm.locals)
		savedFP := vm.fp
		vm.frames = append(vm.frames, Frame{barrier: true, capEnv: capEnv, bp: savedLocals})
		vm.fp = len(vm.frames) - 1
		barrierFrame := &vm.frames[vm.fp]
		if err := vm.callThunk(proto, thv.Captured, barrierFrame); err != nil {
			vm.locals = vm.locals[:savedLocals]
			vm.fp = savedFP
			vm.frames = vm.frames[:savedFP+1]
			return nil, capEnv, err
		}
		result, err := vm.execute()
		vm.locals = vm.locals[:savedLocals]
		vm.fp = savedFP
		vm.frames = vm.frames[:savedFP+1]
		if err != nil {
			return nil, capEnv, err
		}
		return result.Value, result.CapEnv, nil
	}
	if _, ok := v.(*eval.ThunkVal); ok {
		return nil, capEnv, vm.runtimeError("tree-walker ThunkVal in VM forceEffectful (invariant violation: tree-walker type in VM-only execution)", frame)
	}
	// Saturated effectful PrimVal.
	if pv, ok := v.(*eval.PrimVal); ok && pv.Effectful && len(pv.Args) >= pv.Arity {
		impl, ok := vm.prims.Lookup(pv.Name)
		if !ok {
			return nil, capEnv, vm.runtimeError(fmt.Sprintf("missing primitive: %s", pv.Name), frame)
		}
		capForImpl := capEnv.MarkShared()
		val, newCap, err := vm.callPrim(impl, capForImpl, pv.Args)
		if err != nil {
			return nil, capEnv, err
		}
		// Observer: emit effect event with CapEnv diff.
		// Use the call site span (from OpBind) for user-visible location.
		if vm.obs.Active() {
			detail := eval.EffectDetail(pv.Name, pv.Args, val, capForImpl, newCap)
			vm.obs.Emit(vm.budget.Depth(), eval.ExplainEffect, detail, callSpan)
		}
		return val, newCap, nil
	}
	// Nothing to force.
	return v, capEnv, nil
}

// applyPrim handles application to a PrimVal.
func (vm *VM) applyPrim(pv *eval.PrimVal, arg eval.Value, frame *Frame) error {
	args := make([]eval.Value, len(pv.Args)+1)
	copy(args, pv.Args)
	args[len(pv.Args)] = arg

	if len(args) < pv.Arity {
		// Partially applied.
		vm.push(&eval.PrimVal{
			Name: pv.Name, Arity: pv.Arity,
			Effectful: pv.Effectful, Args: args, S: pv.S,
		})
		return nil
	}
	if pv.Effectful {
		// Saturated effectful — defer (keep as PrimVal).
		vm.push(&eval.PrimVal{
			Name: pv.Name, Arity: pv.Arity,
			Effectful: true, Args: args, S: pv.S,
		})
		return nil
	}
	// Saturated non-effectful — call immediately.
	impl, ok := vm.prims.Lookup(pv.Name)
	if !ok {
		return vm.runtimeError(fmt.Sprintf("missing primitive: %s", pv.Name), frame)
	}
	val, newCap, err := vm.callPrim(impl, frame.capEnv, args)
	if err != nil {
		return err
	}
	frame.capEnv = newCap
	vm.push(val)
	return nil
}

// applyForPrim is the Applier callback exposed to primitives.
// Uses barrier-frame approach: pushes a barrier frame + closure frame onto
// the existing frame stack, then re-enters execute(). When OpReturn hits
// the barrier, it stops and returns the result. No slice allocation needed.
func (vm *VM) applyForPrim(fn eval.Value, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	switch f := fn.(type) {
	case *eval.VMClosure:
		proto := f.Proto.(*Proto)
		savedLocals := len(vm.locals)
		savedFP := vm.fp
		vm.frames = append(vm.frames, Frame{barrier: true, capEnv: capEnv, bp: savedLocals})
		vm.fp = len(vm.frames) - 1
		barrierFrame := &vm.frames[vm.fp]
		if err := vm.callClosure(proto, f.Captured, arg, barrierFrame); err != nil {
			vm.locals = vm.locals[:savedLocals]
			vm.fp = savedFP
			vm.frames = vm.frames[:savedFP+1]
			return nil, capEnv, err
		}
		result, err := vm.execute()
		vm.locals = vm.locals[:savedLocals]
		vm.fp = savedFP
		vm.frames = vm.frames[:savedFP+1]
		if err != nil {
			return nil, capEnv, err
		}
		return result.Value, result.CapEnv, nil

	case *eval.ConVal:
		args := make([]eval.Value, len(f.Args)+1)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		return &eval.ConVal{Con: f.Con, Args: args}, capEnv, nil

	case *eval.PrimVal:
		args := make([]eval.Value, len(f.Args)+1)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		if len(args) < f.Arity {
			return &eval.PrimVal{Name: f.Name, Arity: f.Arity, Effectful: f.Effectful, Args: args, S: f.S}, capEnv, nil
		}
		if f.Effectful {
			return &eval.PrimVal{Name: f.Name, Arity: f.Arity, Effectful: true, Args: args, S: f.S}, capEnv, nil
		}
		impl, ok := vm.prims.Lookup(f.Name)
		if !ok {
			return nil, capEnv, fmt.Errorf("missing primitive: %s", f.Name)
		}
		val, newCap, err := vm.callPrim(impl, capEnv, args)
		if err != nil {
			return nil, capEnv, err
		}
		return val, newCap, nil

	default:
		return nil, capEnv, fmt.Errorf("application of non-function in Applier: %s", fn)
	}
}

// callPrim safely invokes a PrimImpl, wrapping plain errors with source/span.
func (vm *VM) callPrim(impl eval.PrimImpl, capEnv eval.CapEnv, args []eval.Value) (val eval.Value, newCap eval.CapEnv, err error) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			val, newCap, err = nil, capEnv, fmt.Errorf("primitive panicked: %v\n%s", r, buf[:n])
		}
	}()
	val, newCap, err = impl(vm.ctx, capEnv, args, vm.cachedApplier)
	if err == nil && val == nil {
		err = fmt.Errorf("primitive returned nil value")
	}
	if err != nil {
		err = wrapPrimError(err)
	}
	return
}

// wrapPrimError wraps plain string errors into RuntimeError.
// Typed errors (RuntimeError, budget limits, context errors) pass through.
func wrapPrimError(err error) error {
	if _, ok := err.(*eval.RuntimeError); ok {
		return err
	}
	typeName := fmt.Sprintf("%T", err)
	if typeName == "*errors.errorString" || typeName == "*fmt.wrapError" {
		return &eval.RuntimeError{Message: err.Error()}
	}
	return err
}
