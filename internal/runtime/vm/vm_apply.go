package vm

import (
	"context"
	"errors"
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// runCallee drives a single sub-call to completion, isolated from the
// surrounding execute() loop. Used by host primitives (Applier callbacks),
// auto-force thunks, and applyN over-application — anywhere code inside the
// VM needs "run this one callee, give me its result" semantics.
//
// Mechanism:
//  1. Save VM state (fp, locals length, sp, budget depth)
//  2. Push a barrier frame on top of the current call stack
//  3. Invoke `setup`, which dispatches the call by either:
//     - pushing a real call frame (closure / thunk paths), or
//     - pushing a value directly (saturated prim, partial PAP, ConVal)
//  4. Resolve the result based on what setup did:
//     - frame-push: run vm.execute(); OpReturn-to-barrier returns the result
//     - value-push: pop the value and return it (no execute needed)
//  5. Restore the saved state regardless of error
//
// Setup callbacks that violate the post-condition (push neither frame nor
// value) are caught as a runtime error rather than silently misbehaving —
// this surfaces internal invariant violations rather than masking them.
func (vm *VM) runCallee(capEnv eval.CapEnv, setup func(barrier *Frame) error) (eval.EvalResult, error) {
	savedLocals := len(vm.locals)
	savedFP := vm.fp
	savedSP := vm.sp
	savedDepth := vm.budget.Depth()
	vm.frames = append(vm.frames, Frame{barrier: true, capEnv: capEnv, bp: savedLocals})
	barrierIdx := len(vm.frames) - 1
	vm.fp = barrierIdx
	barrierFrame := &vm.frames[barrierIdx]
	if err := setup(barrierFrame); err != nil {
		vm.locals = vm.locals[:savedLocals]
		vm.fp = savedFP
		vm.frames = vm.frames[:savedFP+1]
		for vm.budget.Depth() > savedDepth {
			vm.budget.Leave()
		}
		return eval.EvalResult{}, err
	}
	var result eval.EvalResult
	var err error
	if vm.fp > barrierIdx {
		// setup pushed a real frame on top of the barrier.
		// Drive it to completion; OpReturn pops back to the barrier
		// and execute() returns the result.
		result, err = vm.execute()
	} else if vm.sp > savedSP {
		// setup pushed a value directly (no frame). Take it.
		val := vm.pop()
		// Use the barrier's capEnv as authoritative — value-only setups
		// (apply on a saturated prim, etc.) update barrierFrame.capEnv.
		result = eval.EvalResult{Value: val, CapEnv: vm.frames[barrierIdx].capEnv}
	} else {
		err = &eval.RuntimeError{Message: "runCallee: setup pushed neither frame nor value"}
	}
	vm.locals = vm.locals[:savedLocals]
	vm.fp = savedFP
	vm.frames = vm.frames[:savedFP+1]
	if err != nil {
		for vm.budget.Depth() > savedDepth {
			vm.budget.Leave()
		}
	}
	return result, err
}

// apply dispatches a function application. If tail is true, the current frame
// is reused (TCO). Otherwise a new frame is pushed.
func (vm *VM) apply(fn eval.Value, arg eval.Value, frame *Frame, tail bool) error {
	switch f := fn.(type) {
	case *eval.VMClosure:
		proto := f.Proto.(*Proto)
		arity := len(proto.Params)
		if arity > 1 {
			// Multi-param closure: accumulate arg in a PAPVal.
			if err := vm.budget.Alloc(eval.CostPAP + eval.CostPAPArg); err != nil {
				return err
			}
			vm.push(&eval.PAPVal{Fun: f, Args: []eval.Value{arg}, Arity: arity})
			return nil
		}
		leaveObs := vm.emitEnterObs(f.Name, arg, frame)
		if tail {
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

	case *eval.PAPVal:
		newArgs := make([]eval.Value, len(f.Args)+1)
		copy(newArgs, f.Args)
		newArgs[len(f.Args)] = arg
		if len(newArgs) == f.Arity {
			// Saturated: enter the closure body directly.
			proto := f.Fun.Proto.(*Proto)
			return vm.enterClosureMulti(proto, f.Fun.Captured, newArgs, frame, tail, f.Fun.Name)
		}
		// Still partial.
		if err := vm.budget.Alloc(eval.CostPAPArg); err != nil {
			return err
		}
		vm.push(&eval.PAPVal{Fun: f.Fun, Args: newArgs, Arity: f.Arity})
		return nil

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

	case *eval.VMThunkVal:
		// Thunks applied to any argument: force the thunk (ignore arg).
		// This enables Go primitives (e.g. forceTail) to force lazy
		// co-data fields via the Applier callback transparently.
		return vm.force(f, frame, tail)

	default:
		msg := "application of non-function"
		if fn != nil {
			msg = "application of non-function: " + fn.String()
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
	if len(proto.Params) > 0 {
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
	oldNumLocals := frame.proto.NumLocals
	newNumLocals := proto.NumLocals
	clearN := max(newNumLocals, oldNumLocals)
	vm.ensureLocals(bp + clearN)

	// Determine how many leading slots will be overwritten by captures + param.
	numCaptures := len(captured)
	writtenUpTo := numCaptures
	if len(proto.Params) > 0 {
		paramSlot := numCaptures
		if proto.FixSelfSlot >= 0 {
			paramSlot = proto.FixSelfSlot + 1
		}
		writtenUpTo = paramSlot + 1
	}
	// Clear only slots not overwritten by captures/param, to avoid
	// holding stale references that prevent GC.
	if writtenUpTo < clearN {
		clear(vm.locals[bp+writtenUpTo : bp+clearN])
	}

	// Shrink locals to the new frame's requirement.
	vm.locals = vm.locals[:bp+newNumLocals]
	// Copy captures.
	copy(vm.locals[bp:], captured)
	// Store parameter.
	if len(proto.Params) > 0 {
		paramSlot := len(proto.Captures)
		if proto.FixSelfSlot >= 0 {
			paramSlot = proto.FixSelfSlot + 1
		}
		vm.locals[bp+paramSlot] = arg
	}
	frame.proto = proto
	frame.ip = 0
	frame.source = proto.Source
	if proto.Source != nil && vm.obs != nil {
		vm.obs.SetSource(proto.Source)
	}
	return nil
}

// callClosureMulti pushes a new frame for a multi-param closure call.
// Local layout: [captures...] [self if fix] [param0 ... paramN-1] [body locals...]
func (vm *VM) callClosureMulti(proto *Proto, captured []eval.Value, args []eval.Value, frame *Frame) error {
	if err := vm.budget.Enter(); err != nil {
		return err
	}
	bp := len(vm.locals)
	vm.ensureLocals(bp + proto.NumLocals)
	copy(vm.locals[bp:], captured)
	paramBase := len(proto.Captures)
	if proto.FixSelfSlot >= 0 {
		paramBase = proto.FixSelfSlot + 1
	}
	copy(vm.locals[bp+paramBase:], args)
	vm.pushFrame(proto, bp, frame.capEnv, proto.Source)
	if proto.Source != nil && vm.obs != nil {
		vm.obs.SetSource(proto.Source)
	}
	return nil
}

// tailCallClosureMulti reuses the current frame for a multi-param tail call.
func (vm *VM) tailCallClosureMulti(proto *Proto, captured []eval.Value, args []eval.Value, frame *Frame) error {
	bp := frame.bp
	oldN := frame.proto.NumLocals
	newN := proto.NumLocals
	clearN := max(oldN, newN)
	vm.ensureLocals(bp + clearN)

	numCaptures := len(captured)
	paramBase := numCaptures
	if proto.FixSelfSlot >= 0 {
		paramBase = proto.FixSelfSlot + 1
	}
	writtenUpTo := paramBase + len(args)
	if writtenUpTo < clearN {
		clear(vm.locals[bp+writtenUpTo : bp+clearN])
	}

	vm.locals = vm.locals[:bp+newN]
	copy(vm.locals[bp:], captured)
	copy(vm.locals[bp+paramBase:], args)

	frame.proto = proto
	frame.ip = 0
	frame.source = proto.Source
	if proto.Source != nil && vm.obs != nil {
		vm.obs.SetSource(proto.Source)
	}
	return nil
}

// force evaluates a VMThunkVal.
func (vm *VM) force(v eval.Value, frame *Frame, tail bool) error {
	switch thv := v.(type) {
	case *eval.VMThunkVal:
		proto := thv.Proto.(*Proto)
		if tail {
			return vm.tailCallThunk(proto, thv.Captured, frame)
		}
		return vm.callThunk(proto, thv.Captured, frame)

	default:
		return vm.runtimeError("force on non-thunk: "+v.String(), frame)
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
	if proto.Source != nil && vm.obs != nil {
		vm.obs.SetSource(proto.Source)
	}
	return nil
}

// tailCallThunk reuses the current frame for a tail thunk force.
func (vm *VM) tailCallThunk(proto *Proto, captured []eval.Value, frame *Frame) error {
	bp := frame.bp
	n := proto.NumLocals
	vm.ensureLocals(bp + n)
	// Clear only slots not overwritten by captures.
	numCaptures := len(captured)
	if numCaptures < n {
		clear(vm.locals[bp+numCaptures : bp+n])
	}
	copy(vm.locals[bp:], captured)
	frame.proto = proto
	frame.ip = 0
	frame.source = proto.Source
	if proto.Source != nil && vm.obs != nil {
		vm.obs.SetSource(proto.Source)
	}
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
		result, err := vm.runCallee(capEnv, func(b *Frame) error {
			return vm.callThunk(proto, thv.Captured, b)
		})
		if err != nil {
			return nil, capEnv, err
		}
		return result.Value, result.CapEnv, nil
	}
	// Saturated effectful PrimVal.
	if pv, ok := v.(*eval.PrimVal); ok && pv.Effectful && len(pv.Args) >= pv.Arity {
		impl, err := vm.resolvePrimImplFrame(pv, frame)
		if err != nil {
			return nil, capEnv, err
		}
		capForImpl := capEnv.MarkShared()
		val, newCap, callErr := vm.callPrim(impl, capForImpl, pv.Args)
		if callErr != nil {
			return nil, capEnv, callErr
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

// forceMergeChild executes a thunk (merge child proto) with a specific CapEnv
// using the barrier-frame approach. Returns the result value and final CapEnv.
func (vm *VM) forceMergeChild(v eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	thv, ok := v.(*eval.VMThunkVal)
	if !ok {
		// Not a thunk — already a value. Return as-is with the given CapEnv.
		return v, capEnv, nil
	}
	proto := thv.Proto.(*Proto)
	if err := vm.budget.Step(); err != nil {
		return nil, capEnv, err
	}
	result, err := vm.runCallee(capEnv, func(b *Frame) error {
		return vm.callThunk(proto, thv.Captured, b)
	})
	if err != nil {
		return nil, capEnv, err
	}
	return result.Value, result.CapEnv, nil
}

// resolvePrimImplFrame returns the cached PrimImpl for a PrimVal, falling
// back to a registry lookup. Used by frame-driven dispatch (forceEffectful,
// applyPrim, applyN PrimVal cases) where the caller has a current Frame for
// span attribution. The returned error, when non-nil, is already a structured
// *eval.RuntimeError carrying source location.
func (vm *VM) resolvePrimImplFrame(pv *eval.PrimVal, frame *Frame) (eval.PrimImpl, error) {
	if pv.Impl != nil {
		return pv.Impl, nil
	}
	impl, ok := vm.prims.Lookup(pv.Name)
	if !ok {
		return nil, vm.runtimeError("missing primitive: "+pv.Name, frame)
	}
	return impl, nil
}

// resolvePrimImplBare returns the cached PrimImpl for a PrimVal, falling
// back to a registry lookup. Used by barrier-driven dispatch (applyForPrim,
// applyNForPrim) where no Frame is available. The returned error, when
// non-nil, is a plain "missing primitive" error without source attribution
// — the host callback boundary is the right place to add context if needed.
//
// Two helpers (Frame vs Bare) instead of one with optional Frame: the return
// shapes are categorically different (RuntimeError with span vs plain error),
// and forcing callers to pick the right one preserves the layering between
// VM-internal and host-callback dispatch.
func (vm *VM) resolvePrimImplBare(pv *eval.PrimVal) (eval.PrimImpl, error) {
	if pv.Impl != nil {
		return pv.Impl, nil
	}
	impl, ok := vm.prims.Lookup(pv.Name)
	if !ok {
		return nil, errors.New("missing primitive: " + pv.Name)
	}
	return impl, nil
}

// asPartialPrim returns a PrimVal stub representing a partially-applied
// primitive. The args slice MUST outlive the call (it becomes the stub's
// Args field) — callers building it from scratch and from primScratch must
// distinguish: scratch is heap-managed and aliasable, so partial stubs
// MUST use a heap-allocated args slice.
func asPartialPrim(pv *eval.PrimVal, args []eval.Value) *eval.PrimVal {
	return &eval.PrimVal{
		Name:      pv.Name,
		Arity:     pv.Arity,
		Effectful: pv.Effectful,
		Args:      args,
		S:         pv.S,
		Impl:      pv.Impl,
	}
}

// asDeferredEffectful returns a PrimVal stub representing a saturated
// (or over-saturated) effectful primitive whose execution is deferred until
// `forceEffectful` runs it. Same args-lifetime contract as asPartialPrim.
func asDeferredEffectful(pv *eval.PrimVal, args []eval.Value) *eval.PrimVal {
	return &eval.PrimVal{
		Name:      pv.Name,
		Arity:     pv.Arity,
		Effectful: true,
		Args:      args,
		S:         pv.S,
		Impl:      pv.Impl,
	}
}

// applyPrim handles application to a PrimVal.
func (vm *VM) applyPrim(pv *eval.PrimVal, arg eval.Value, frame *Frame) error {
	newLen := len(pv.Args) + 1

	// Fast path: saturated non-effectful with small arity.
	// Uses the VM's scratch buffer to avoid a heap allocation for the
	// transient argument slice. Safe because non-effectful primitives
	// return before any re-entrant VM execution can alias the buffer.
	if newLen >= pv.Arity && !pv.Effectful && newLen <= len(vm.primScratch) {
		impl, err := vm.resolvePrimImplFrame(pv, frame)
		if err != nil {
			return err
		}
		args := vm.primScratch[:newLen]
		copy(args, pv.Args)
		args[len(pv.Args)] = arg
		val, newCap, callErr := vm.callPrim(impl, frame.capEnv, args)
		clear(vm.primScratch[:newLen])
		if callErr != nil {
			return callErr
		}
		frame.capEnv = newCap
		vm.push(val)
		return nil
	}

	args := make([]eval.Value, newLen)
	copy(args, pv.Args)
	args[len(pv.Args)] = arg

	if newLen < pv.Arity {
		// Partially applied.
		vm.push(asPartialPrim(pv, args))
		return nil
	}
	if pv.Effectful {
		// Saturated effectful — defer (keep as PrimVal).
		vm.push(asDeferredEffectful(pv, args))
		return nil
	}
	// Saturated non-effectful, arity > scratch size.
	impl, err := vm.resolvePrimImplFrame(pv, frame)
	if err != nil {
		return err
	}
	val, newCap, callErr := vm.callPrim(impl, frame.capEnv, args)
	if callErr != nil {
		return callErr
	}
	frame.capEnv = newCap
	vm.push(val)
	return nil
}

// dispatchApplyN runs an OpApplyN / OpTailApplyN instruction using a
// stack view: the function and its n arguments are taken directly from
// the operand stack without allocating an args slice. After applyN
// finishes, the [fn, arg0..argn-1] residue is cleared and, if applyN
// produced a value (PrimVal/PAPVal/ConVal/etc.), it is placed at the
// fn's old slot.
//
// For frame-pushing dispatches (saturated VMClosure / PAPVal entry,
// thunk force), applyN returns (nil, nil) and the new frame is already
// on the call stack. The cleanup still happens, leaving the operand
// stack at sp = fnIdx so the closure body's eventual OpReturn can push
// its result onto a clean baseline.
func (vm *VM) dispatchApplyN(n int, frame *Frame, tail bool) error {
	fnIdx := vm.sp - n - 1
	fn := vm.stack[fnIdx]
	args := vm.stack[fnIdx+1 : vm.sp]
	val, err := vm.applyN(fn, args, frame, tail)
	if err != nil {
		return err
	}
	// Clear the [fn, args] section. applyN has either copied the args
	// into a heap-retaining value (PAPVal/PrimVal/ConVal), copied them
	// into the new closure's locals via callClosureMulti, or consumed
	// them by passing to a trusted prim. The stack slots are now dead.
	clear(vm.stack[fnIdx:vm.sp])
	vm.sp = fnIdx
	if val != nil {
		vm.push(val)
	}
	return nil
}

// emitEnterObs emits the observer "enter" label for a closure call (or
// records an internal-frame entry, depending on whether the named binding
// is user-visible). Returns true if the caller should set leaveObs on its
// frame so the matching LeaveInternal fires when the closure returns.
//
// Centralized so that apply, applySingle, and the multi-arg paths in
// applyN share one obs-event protocol. The host-callback paths
// (applyForPrim / applyNForPrim) currently inline a different variant
// because they have no caller frame to anchor the source span on; that
// asymmetry is documented in the Tier B / C cleanup notes.
func (vm *VM) emitEnterObs(name string, sampleArg eval.Value, frame *Frame) bool {
	if vm.obs == nil || name == "" {
		return false
	}
	if vm.obs.IsInternal(name) {
		vm.obs.EnterInternal()
		return true
	}
	if vm.obs.Active() {
		vm.obs.Emit(vm.budget.Depth(), eval.ExplainLabel,
			eval.ExplainDetail{Name: name, LabelKind: "enter", Value: eval.PrettyValue(sampleArg)},
			frame.proto.SpanAt(frame.ip))
	}
	return false
}

// applyN dispatches a multi-argument application.
//
// Returns one of three outcomes:
//   - (val, nil): a value was produced synchronously; the OpApplyN
//     handler must push it onto the operand stack after cleanup.
//   - (nil, nil): a new call frame was pushed (closure / thunk entry);
//     the operand stack must still be cleaned up of [fn, args] residue,
//     but no value to push — the closure's eventual OpReturn will push
//     its result onto the cleaned stack.
//   - (nil, err): error.
//
// Pre-condition: the args slice may be aliased to the operand stack
// (the caller passes a stack view to avoid heap allocation). applyN
// MUST copy args before storing them into any heap-retaining value
// (PAPVal, PrimVal, ConVal). Saturated dispatches that consume args
// immediately (callClosureMulti, callPrim) read args before
// the operand stack is cleaned up, so a stack view is safe there.
func (vm *VM) applyN(fn eval.Value, args []eval.Value, frame *Frame, tail bool) (eval.Value, error) {
	switch f := fn.(type) {
	case *eval.VMClosure:
		proto := f.Proto.(*Proto)
		arity := len(proto.Params)
		switch {
		case len(args) == arity:
			if arity == 1 {
				return nil, vm.applySingle(f, args[0], frame, tail)
			}
			return nil, vm.enterClosureMulti(proto, f.Captured, args, frame, tail, f.Name)
		case len(args) < arity:
			if err := vm.budget.Alloc(eval.CostPAP + int64(eval.CostPAPArg*len(args))); err != nil {
				return nil, err
			}
			// Copy args — the slice is aliased to the operand stack
			// and PAPVal retains the underlying storage long-term.
			storedArgs := make([]eval.Value, len(args))
			copy(storedArgs, args)
			return &eval.PAPVal{Fun: f, Args: storedArgs, Arity: arity}, nil
		default:
			// Over-application: enter with first arity args via a barrier
			// so the closure body runs in isolation, then apply remaining
			// args to its result. Each subsequent apply also goes through
			// runCallee because it may itself enter a closure.
			//
			// Pin the extras into a heap slice now: the operand stack
			// (which `args` views) gets cleaned up by runCallee's barrier
			// machinery during sub-execution.
			extras := make([]eval.Value, len(args)-arity)
			copy(extras, args[arity:])
			callerCapEnv := frame.capEnv
			result, err := vm.runCallee(callerCapEnv, func(b *Frame) error {
				return vm.callClosureMulti(proto, f.Captured, args[:arity], b)
			})
			if err != nil {
				return nil, err
			}
			cur := result.Value
			capEnv := result.CapEnv
			for _, extra := range extras {
				next, err := vm.runCallee(capEnv, func(b *Frame) error {
					return vm.apply(cur, extra, b, false)
				})
				if err != nil {
					return nil, err
				}
				cur = next.Value
				capEnv = next.CapEnv
			}
			// Re-acquire current frame: runCallee may have reallocated
			// the frames slice, invalidating the original `frame` ptr.
			vm.currentFrame().capEnv = capEnv
			return cur, nil
		}

	case *eval.PAPVal:
		combined := combineArgs(f.Args, args)
		remaining := f.Arity - len(combined)
		switch {
		case remaining == 0:
			proto := f.Fun.Proto.(*Proto)
			return nil, vm.enterClosureMulti(proto, f.Fun.Captured, combined, frame, tail, f.Fun.Name)
		case remaining > 0:
			return &eval.PAPVal{Fun: f.Fun, Args: combined, Arity: f.Arity}, nil
		default:
			// Over-saturated PAP: enter with the saturating prefix via a
			// barrier, then apply remaining args to the result.
			proto := f.Fun.Proto.(*Proto)
			callerCapEnv := frame.capEnv
			result, err := vm.runCallee(callerCapEnv, func(b *Frame) error {
				return vm.callClosureMulti(proto, f.Fun.Captured, combined[:f.Arity], b)
			})
			if err != nil {
				return nil, err
			}
			cur := result.Value
			capEnv := result.CapEnv
			for _, extra := range combined[f.Arity:] {
				next, err := vm.runCallee(capEnv, func(b *Frame) error {
					return vm.apply(cur, extra, b, false)
				})
				if err != nil {
					return nil, err
				}
				cur = next.Value
				capEnv = next.CapEnv
			}
			vm.currentFrame().capEnv = capEnv
			return cur, nil
		}

	case *eval.PrimVal:
		newLen := len(f.Args) + len(args)
		// Fast path: saturated non-effectful prim that fits in scratch.
		// Builds the combined arg view in primScratch with no heap alloc.
		// Safe because non-effectful primitives never call back into the VM.
		if newLen == f.Arity && !f.Effectful && newLen <= len(vm.primScratch) {
			impl, err := vm.resolvePrimImplFrame(f, frame)
			if err != nil {
				return nil, err
			}
			scratch := vm.primScratch[:newLen]
			copy(scratch, f.Args)
			copy(scratch[len(f.Args):], args)
			val, newCap, callErr := vm.callPrim(impl, frame.capEnv, scratch)
			clear(vm.primScratch[:newLen])
			if callErr != nil {
				return nil, callErr
			}
			frame.capEnv = newCap
			return val, nil
		}
		// All other PrimVal cases need a single combined slice that
		// outlives this call (it lands inside a deferred PrimVal or
		// gets passed to a non-trusted prim impl). Allocate once.
		combined := combineArgs(f.Args, args)
		switch {
		case newLen < f.Arity:
			return asPartialPrim(f, combined), nil
		case newLen == f.Arity && f.Effectful:
			// Saturated effectful — defer execution until forced.
			return asDeferredEffectful(f, combined), nil
		case newLen == f.Arity:
			// Saturated non-effectful, but arity exceeds scratch.
			impl, err := vm.resolvePrimImplFrame(f, frame)
			if err != nil {
				return nil, err
			}
			val, newCap, callErr := vm.callPrim(impl, frame.capEnv, combined)
			if callErr != nil {
				return nil, callErr
			}
			frame.capEnv = newCap
			return val, nil
		}
		// Over-application: saturate the prim, then thread the result
		// through the remaining args. Effectful prims defer; non-effectful
		// ones execute immediately.
		var head eval.Value
		callerCapEnv := frame.capEnv
		if f.Effectful {
			head = asDeferredEffectful(f, combined[:f.Arity])
		} else {
			impl, err := vm.resolvePrimImplFrame(f, frame)
			if err != nil {
				return nil, err
			}
			val, newCap, callErr := vm.callPrim(impl, callerCapEnv, combined[:f.Arity])
			if callErr != nil {
				return nil, callErr
			}
			head = val
			callerCapEnv = newCap
		}
		cur := head
		capEnv := callerCapEnv
		for _, extra := range combined[f.Arity:] {
			next, err := vm.runCallee(capEnv, func(b *Frame) error {
				return vm.apply(cur, extra, b, false)
			})
			if err != nil {
				return nil, err
			}
			cur = next.Value
			capEnv = next.CapEnv
		}
		vm.currentFrame().capEnv = capEnv
		return cur, nil

	case *eval.ConVal:
		// Constructors are uncurried at the IR level (Con.Args is the
		// full argument list); seeing one in apply position means a
		// curried Con value is being applied to more arguments. Append
		// in one shot to avoid the per-arg allocation that the sequential
		// apply chain would produce.
		if err := vm.budget.Alloc(eval.CostConBase + int64(eval.CostConArg*(len(f.Args)+len(args)))); err != nil {
			return nil, err
		}
		return &eval.ConVal{Con: f.Con, Args: combineArgs(f.Args, args)}, nil

	case *eval.VMThunkVal:
		// Force the thunk in isolation, then apply all original args to
		// its result. The args slice may be aliased to the operand stack
		// — pin into a heap copy because runCallee will manipulate the
		// stack while forcing.
		stored := make([]eval.Value, len(args))
		copy(stored, args)
		result, err := vm.runCallee(frame.capEnv, func(b *Frame) error {
			return vm.force(f, b, false)
		})
		if err != nil {
			return nil, err
		}
		// Recurse with the forced value as the new function.
		// frame may be stale after runCallee — fetch fresh.
		return vm.applyN(result.Value, stored, vm.currentFrame(), tail)

	default:
		msg := "application of non-function"
		if fn != nil {
			msg = "application of non-function: " + fn.String()
		}
		return nil, vm.runtimeError(msg, frame)
	}
}

// applySingle is the single-arg path extracted from apply for VMClosure.
func (vm *VM) applySingle(f *eval.VMClosure, arg eval.Value, frame *Frame, tail bool) error {
	proto := f.Proto.(*Proto)
	leaveObs := vm.emitEnterObs(f.Name, arg, frame)
	if tail {
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
}

// enterClosureMulti is the frame-driven multi-argument closure entry path,
// the multi-arg counterpart of applySingle. Used by apply (saturated PAPVal
// case) and applyN (VMClosure exact case, PAPVal saturated case) — anywhere
// VM-internal dispatch needs to enter a multi-param closure body with TCO
// support and observer attribution.
//
// Pre-condition: len(args) > 0. The first arg is used as the observer
// "sample" for the enter event; the call passes the full args slice through
// to callClosureMulti / tailCallClosureMulti.
//
// Note: this helper is frame-driven (TCO + observer hooks). Host-callback
// dispatch (applyForPrim / applyNForPrim) cannot use it — those paths run
// outside the bytecode loop, have no caller frame to attribute spans to,
// and dispatch through runCallee + callClosureMulti instead. The two layers
// are categorically different abstractions; do not try to unify.
func (vm *VM) enterClosureMulti(proto *Proto, captured []eval.Value, args []eval.Value, frame *Frame, tail bool, name string) error {
	leaveObs := vm.emitEnterObs(name, args[0], frame)
	if tail {
		if frame.leaveObs {
			vm.obs.LeaveInternal()
		}
		frame.leaveObs = leaveObs
		return vm.tailCallClosureMulti(proto, captured, args, frame)
	}
	err := vm.callClosureMulti(proto, captured, args, frame)
	if err == nil && leaveObs {
		vm.currentFrame().leaveObs = true
	}
	return err
}

// combineArgs builds a freshly heap-allocated slice that concatenates an
// existing args prefix (typically from a PAPVal/ConVal/PrimVal that has
// accumulated arguments over time) with the new args slice from a current
// application. The result outlives the call (it lands inside a new aggregate
// value or feeds a saturating closure entry), so the operand-stack-aliased
// input `more` is copied rather than referenced.
//
// Used by both frame-driven (applyN PAPVal/ConVal/PrimVal cases) and
// barrier-driven (applyForPrim, applyNForPrim PAPVal/PrimVal cases) dispatch
// — the args-merging step is structural and shared across layers.
func combineArgs(prefix []eval.Value, more []eval.Value) []eval.Value {
	combined := make([]eval.Value, len(prefix)+len(more))
	copy(combined, prefix)
	copy(combined[len(prefix):], more)
	return combined
}

// applyForPrim is the Applier callback exposed to primitives.
// Uses barrier-frame approach: pushes a barrier frame + closure frame onto
// the existing frame stack, then re-enters execute(). When OpReturn hits
// the barrier, it stops and returns the result. No slice allocation needed.
func (vm *VM) applyForPrim(fn eval.Value, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	switch f := fn.(type) {
	case *eval.VMClosure:
		proto := f.Proto.(*Proto)
		if len(proto.Params) > 1 {
			return &eval.PAPVal{Fun: f, Args: []eval.Value{arg}, Arity: len(proto.Params)}, capEnv, nil
		}
		result, err := vm.runCallee(capEnv, func(b *Frame) error {
			return vm.callClosure(proto, f.Captured, arg, b)
		})
		if err != nil {
			return nil, capEnv, err
		}
		return result.Value, result.CapEnv, nil

	case *eval.PAPVal:
		newArgs := make([]eval.Value, len(f.Args)+1)
		copy(newArgs, f.Args)
		newArgs[len(f.Args)] = arg
		if len(newArgs) == f.Arity {
			proto := f.Fun.Proto.(*Proto)
			result, err := vm.runCallee(capEnv, func(b *Frame) error {
				return vm.callClosureMulti(proto, f.Fun.Captured, newArgs, b)
			})
			if err != nil {
				return nil, capEnv, err
			}
			return result.Value, result.CapEnv, nil
		}
		return &eval.PAPVal{Fun: f.Fun, Args: newArgs, Arity: f.Arity}, capEnv, nil

	case *eval.ConVal:
		args := make([]eval.Value, len(f.Args)+1)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		return &eval.ConVal{Con: f.Con, Args: args}, capEnv, nil

	case *eval.PrimVal:
		newLen := len(f.Args) + 1
		// Fast path: saturated non-effectful prim that fits in scratch.
		// Mirrors the applyN/applyPrim fast path; safe in the host
		// callback context because non-effectful prims do not re-enter
		// the VM operand stack.
		if newLen == f.Arity && !f.Effectful && newLen <= len(vm.primScratch) {
			impl, err := vm.resolvePrimImplBare(f)
			if err != nil {
				return nil, capEnv, err
			}
			scratch := vm.primScratch[:newLen]
			copy(scratch, f.Args)
			scratch[len(f.Args)] = arg
			val, newCap, callErr := vm.callPrim(impl, capEnv, scratch)
			clear(vm.primScratch[:newLen])
			if callErr != nil {
				return nil, capEnv, callErr
			}
			return val, newCap, nil
		}
		args := make([]eval.Value, newLen)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		if newLen < f.Arity {
			return asPartialPrim(f, args), capEnv, nil
		}
		if f.Effectful {
			return asDeferredEffectful(f, args), capEnv, nil
		}
		impl, err := vm.resolvePrimImplBare(f)
		if err != nil {
			return nil, capEnv, err
		}
		val, newCap, callErr := vm.callPrim(impl, capEnv, args)
		if callErr != nil {
			return nil, capEnv, callErr
		}
		return val, newCap, nil

	case *eval.VMThunkVal:
		// Force thunk (ignore argument) — enables Go primitives to
		// transparently handle lazy co-data fields.
		proto := f.Proto.(*Proto)
		result, err := vm.runCallee(capEnv, func(b *Frame) error {
			return vm.callThunk(proto, f.Captured, b)
		})
		if err != nil {
			return nil, capEnv, err
		}
		return result.Value, result.CapEnv, nil

	default:
		return nil, capEnv, fmt.Errorf("application of non-function in Applier: %s", fn)
	}
}

// applyNForPrim is the multi-argument Applier callback exposed to primitives.
// Eliminates intermediate PAPVal/PrimVal allocations for curried calls by
// dispatching all arguments in a single operation, mirroring the VM's internal
// applyN but using barrier-frame approach for host re-entry.
func (vm *VM) applyNForPrim(fn eval.Value, args []eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	if len(args) == 0 {
		return fn, capEnv, nil
	}
	if len(args) == 1 {
		return vm.applyForPrim(fn, args[0], capEnv)
	}

	switch f := fn.(type) {
	case *eval.VMClosure:
		proto := f.Proto.(*Proto)
		arity := len(proto.Params)
		switch {
		case len(args) == arity:
			result, err := vm.runCallee(capEnv, func(b *Frame) error {
				return vm.callClosureMulti(proto, f.Captured, args, b)
			})
			if err != nil {
				return nil, capEnv, err
			}
			return result.Value, result.CapEnv, nil
		case len(args) < arity:
			cpy := make([]eval.Value, len(args))
			copy(cpy, args)
			return &eval.PAPVal{Fun: f, Args: cpy, Arity: arity}, capEnv, nil
		default:
			result, err := vm.runCallee(capEnv, func(b *Frame) error {
				return vm.callClosureMulti(proto, f.Captured, args[:arity], b)
			})
			if err != nil {
				return nil, capEnv, err
			}
			return vm.applyNForPrim(result.Value, args[arity:], result.CapEnv)
		}

	case *eval.PAPVal:
		combined := combineArgs(f.Args, args)
		switch {
		case len(combined) == f.Arity:
			proto := f.Fun.Proto.(*Proto)
			result, err := vm.runCallee(capEnv, func(b *Frame) error {
				return vm.callClosureMulti(proto, f.Fun.Captured, combined, b)
			})
			if err != nil {
				return nil, capEnv, err
			}
			return result.Value, result.CapEnv, nil
		case len(combined) < f.Arity:
			return &eval.PAPVal{Fun: f.Fun, Args: combined, Arity: f.Arity}, capEnv, nil
		default:
			proto := f.Fun.Proto.(*Proto)
			result, err := vm.runCallee(capEnv, func(b *Frame) error {
				return vm.callClosureMulti(proto, f.Fun.Captured, combined[:f.Arity], b)
			})
			if err != nil {
				return nil, capEnv, err
			}
			return vm.applyNForPrim(result.Value, combined[f.Arity:], result.CapEnv)
		}

	case *eval.ConVal:
		newArgs := make([]eval.Value, len(f.Args)+len(args))
		copy(newArgs, f.Args)
		copy(newArgs[len(f.Args):], args)
		return &eval.ConVal{Con: f.Con, Args: newArgs}, capEnv, nil

	case *eval.PrimVal:
		newLen := len(f.Args) + len(args)
		// Fast path: saturated non-effectful prim that fits in scratch.
		// This is the host-callback hot path: stdlib helpers like
		// avlInsert -> compareKeys -> apply.ApplyN(_cmpInt, [a,b], ce)
		// land here, and the cached f.Impl + scratch buffer eliminate
		// the per-call hash lookup and args slice allocation.
		if newLen == f.Arity && !f.Effectful && newLen <= len(vm.primScratch) {
			impl, err := vm.resolvePrimImplBare(f)
			if err != nil {
				return nil, capEnv, err
			}
			scratch := vm.primScratch[:newLen]
			copy(scratch, f.Args)
			copy(scratch[len(f.Args):], args)
			val, newCap, callErr := vm.callPrim(impl, capEnv, scratch)
			clear(vm.primScratch[:newLen])
			if callErr != nil {
				return nil, capEnv, callErr
			}
			return val, newCap, nil
		}
		combined := combineArgs(f.Args, args)
		if newLen < f.Arity {
			return asPartialPrim(f, combined), capEnv, nil
		}
		if f.Effectful {
			if newLen == f.Arity {
				return asDeferredEffectful(f, combined), capEnv, nil
			}
			// Over-saturated effectful: defer first, then apply rest.
			return vm.applyNForPrim(asDeferredEffectful(f, combined[:f.Arity]), combined[f.Arity:], capEnv)
		}
		impl, err := vm.resolvePrimImplBare(f)
		if err != nil {
			return nil, capEnv, err
		}
		if newLen == f.Arity {
			val, newCap, callErr := vm.callPrim(impl, capEnv, combined)
			if callErr != nil {
				return nil, capEnv, callErr
			}
			return val, newCap, nil
		}
		// Over-saturated: invoke with arity args, then apply rest.
		val, newCap, err := vm.callPrim(impl, capEnv, combined[:f.Arity])
		if err != nil {
			return nil, capEnv, err
		}
		return vm.applyNForPrim(val, combined[f.Arity:], newCap)

	case *eval.VMThunkVal:
		proto := f.Proto.(*Proto)
		result, err := vm.runCallee(capEnv, func(b *Frame) error {
			return vm.callThunk(proto, f.Captured, b)
		})
		if err != nil {
			return nil, capEnv, err
		}
		return vm.applyNForPrim(result.Value, args, result.CapEnv)

	default:
		return nil, capEnv, fmt.Errorf("application of non-function in ApplyN: %s", fn)
	}
}

// callPrim invokes a host-registered PrimImpl. It is the single dispatch
// path for every prim invocation in the VM (OpPrim direct dispatch, applyN
// saturated, applyForPrim/applyNForPrim host re-entry, applyPrim slow path,
// forceEffectful). One way for one thing.
//
// Trust boundary: prim impls are host-registered code under our control,
// not user-supplied. A panic from a prim impl is a programming bug in the
// host code, not a recoverable runtime condition. We deliberately do NOT
// wrap the call in defer/recover — letting the panic propagate surfaces
// the bug at its source rather than masking it as a generic
// "primitive panicked: internal error" downstream. Stdlib regression tests
// must catch any such panic before reaching production.
//
// Errors from the impl pass through wrapPrimError, which preserves
// structured errors (RuntimeError, ctx cancellation, budget limits) and
// wraps plain errors as RuntimeError so the runtime error path is uniform.
func (vm *VM) callPrim(impl eval.PrimImpl, capEnv eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
	val, newCap, err := impl(vm.ctx, capEnv, args, vm.cachedApplier)
	if err != nil {
		return nil, capEnv, wrapPrimError(err)
	}
	return val, newCap, nil
}

// wrapPrimError wraps plain errors from stdlib primitives into RuntimeError.
// Errors that already carry structured information (RuntimeError, context
// cancellation, budget limits) pass through unchanged.
func wrapPrimError(err error) error {
	switch {
	case errors.As(err, new(*eval.RuntimeError)):
		return err
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case budget.IsLimitError(err):
		return err
	default:
		return &eval.RuntimeError{Message: err.Error()}
	}
}
