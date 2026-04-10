package vm

import (
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

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
			val, newCap, callErr := vm.callPrim(impl, frame.capEnv, scratch, f.S)
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
			val, newCap, callErr := vm.callPrim(impl, frame.capEnv, combined, f.S)
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
			val, newCap, callErr := vm.callPrim(impl, callerCapEnv, combined[:f.Arity], f.S)
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
		combined := combineArgs(f.Args, args)
		dc := f.DictArgCount
		if dc == len(f.Args) {
			dc = eval.CountLeadingDictArgs(combined)
		}
		return &eval.ConVal{Con: f.Con, Args: combined, DictArgCount: dc}, nil

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
