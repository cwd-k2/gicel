package vm

import (
	"context"
	"fmt"
	"runtime"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// apply dispatches a function application. If tail is true, the current frame
// is reused (TCO). Otherwise a new frame is pushed.
func (vm *VM) apply(fn eval.Value, arg eval.Value, frame *Frame, tail bool) error {
	switch f := fn.(type) {
	case *eval.VMClosure:
		proto := f.Proto.(*Proto)
		if tail {
			return vm.tailCallClosure(proto, f.Captured, arg, frame)
		}
		return vm.callClosure(proto, f.Captured, arg, frame)

	case *eval.Closure:
		// Tree-walker closure encountered in VM context (e.g., from builtin globals).
		// We cannot execute tree-walker closures in the VM directly.
		// Fall back to the tree-walker's evaluation for this application.
		return vm.applyTreeWalkerClosure(f, arg, frame)

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
	return nil
}

// tailCallClosure reuses the current frame for a tail call.
func (vm *VM) tailCallClosure(proto *Proto, captured []eval.Value, arg eval.Value, frame *Frame) error {
	bp := frame.bp
	vm.ensureLocals(bp + proto.NumLocals)
	// Clear old locals.
	for i := 0; i < proto.NumLocals; i++ {
		vm.locals[bp+i] = nil
	}
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
		// Tree-walker thunk — cannot execute directly in VM.
		return vm.forceTreeWalkerThunk(thv, frame)

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
func (vm *VM) forceEffectful(v eval.Value, capEnv eval.CapEnv, frame *Frame) (eval.Value, eval.CapEnv, error) {
	// Auto-force rec thunks.
	if thv, ok := v.(*eval.VMThunkVal); ok && thv.AutoForce {
		if err := vm.budget.Step(); err != nil {
			return nil, capEnv, err
		}
		proto := thv.Proto.(*Proto)
		// Execute the thunk body synchronously.
		result, err := vm.execSubProto(proto, thv.Captured, capEnv)
		if err != nil {
			return nil, capEnv, err
		}
		return result.Value, result.CapEnv, nil
	}
	if thv, ok := v.(*eval.ThunkVal); ok && thv.AutoForce {
		if err := vm.budget.Step(); err != nil {
			return nil, capEnv, err
		}
		if vm.fallbackEval == nil {
			return nil, capEnv, vm.runtimeError("tree-walker auto-force thunk in VM (no fallback)", frame)
		}
		result, err := vm.fallbackEval.Eval(thv.Locals, capEnv, thv.Comp)
		if err != nil {
			return nil, capEnv, err
		}
		return result.Value, result.CapEnv, nil
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
func (vm *VM) applyForPrim(fn eval.Value, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	// Save VM state.
	savedSP := vm.sp
	savedFP := vm.fp
	savedLocals := len(vm.locals)

	// Set up a temporary frame context.
	frame := vm.currentFrame()
	origCapEnv := frame.capEnv
	frame.capEnv = capEnv

	if err := vm.apply(fn, arg, frame, false); err != nil {
		frame.capEnv = origCapEnv
		return nil, capEnv, err
	}

	// If apply pushed a frame, execute until we return to this level.
	for vm.fp > savedFP {
		result, err := vm.executeUntilFrame(savedFP)
		if err != nil {
			frame.capEnv = origCapEnv
			return nil, capEnv, err
		}
		if result != nil {
			frame.capEnv = origCapEnv
			return result.Value, result.CapEnv, nil
		}
	}

	// Apply didn't push a frame (ConVal, PrimVal).
	result := vm.pop()
	resultCapEnv := frame.capEnv
	frame.capEnv = origCapEnv

	// Restore state.
	vm.sp = savedSP
	vm.locals = vm.locals[:savedLocals]

	return result, resultCapEnv, nil
}

// executeUntilFrame runs the dispatch loop until the frame stack is at or below targetFP.
func (vm *VM) executeUntilFrame(targetFP int) (*eval.EvalResult, error) {
	for vm.fp > targetFP {
		frame := vm.currentFrame()
		if frame.ip >= len(frame.proto.Code) {
			if vm.sp > 0 {
				result := vm.pop()
				capEnv := frame.capEnv
				if vm.fp <= targetFP+1 {
					return &eval.EvalResult{Value: result, CapEnv: capEnv}, nil
				}
				vm.budget.Leave()
				vm.locals = vm.locals[:frame.bp]
				vm.popFrame()
				vm.push(result)
				vm.currentFrame().capEnv = capEnv
				continue
			}
			return nil, &eval.RuntimeError{Message: "empty stack at return"}
		}

		// Run one step of the main loop.
		// This is a simplified re-entry — for correctness, we reuse execute()
		// but check frame depth after each instruction.
		result, err := vm.execute()
		if err != nil {
			return nil, err
		}
		return &result, nil
	}
	return nil, nil
}

// execSubProto executes a proto synchronously (for auto-force thunks in forceEffectful).
func (vm *VM) execSubProto(proto *Proto, captured []eval.Value, capEnv eval.CapEnv) (eval.EvalResult, error) {
	bp := len(vm.locals)
	vm.ensureLocals(bp + proto.NumLocals)
	copy(vm.locals[bp:], captured)
	vm.pushFrame(proto, bp, capEnv, proto.Source)
	return vm.execute()
}

// callPrim safely invokes a PrimImpl.
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
	return
}

// applyTreeWalkerClosure handles tree-walker Closures that appear in the VM
// (e.g., module bindings, builtins). Falls back to the tree-walker evaluator.
func (vm *VM) applyTreeWalkerClosure(clo *eval.Closure, arg eval.Value, frame *Frame) error {
	if vm.fallbackEval == nil {
		return vm.runtimeError(
			fmt.Sprintf("cannot execute tree-walker closure %q in VM (no fallback evaluator)", clo.Param),
			frame)
	}
	// Use the tree-walker to apply the closure.
	locals := eval.Push(clo.Locals, arg)
	result, err := vm.fallbackEval.Eval(locals, frame.capEnv, clo.Body)
	if err != nil {
		return err
	}
	frame.capEnv = result.CapEnv
	vm.push(result.Value)
	return nil
}

// forceTreeWalkerThunk handles tree-walker ThunkVals in the VM.
func (vm *VM) forceTreeWalkerThunk(thv *eval.ThunkVal, frame *Frame) error {
	if vm.fallbackEval == nil {
		return vm.runtimeError("cannot execute tree-walker thunk in VM (no fallback evaluator)", frame)
	}
	result, err := vm.fallbackEval.Eval(thv.Locals, frame.capEnv, thv.Comp)
	if err != nil {
		return err
	}
	frame.capEnv = result.CapEnv
	vm.push(result.Value)
	return nil
}

// callPrimWithContext invokes a PrimImpl with proper context handling.
func callPrimWithContext(ctx context.Context, impl eval.PrimImpl, capEnv eval.CapEnv, args []eval.Value, applier eval.Applier) (eval.Value, eval.CapEnv, error) {
	return impl(ctx, capEnv, args, applier)
}
