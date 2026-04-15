package vm

import (
	"fmt"

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

// forceEffectful handles auto-force thunks and saturated effectful PrimVals.
// callSpan is the source span of the call site (for observer events).
func (vm *VM) forceEffectful(v eval.Value, capEnv eval.CapEnv, frame *Frame, callSpan span.Span) (eval.Value, eval.CapEnv, error) {
	// Auto-force rec thunks.
	if thv, ok := v.(*eval.VMThunkVal); ok && thv.AutoForce {
		if err := vm.budget.Step(); err != nil {
			return nil, capEnv, err
		}
		proto := protoOf(thv.Proto)
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
		val, newCap, callErr := vm.callPrim(impl, capForImpl, pv.Args, pv.S)
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

// forceEffectfulForPrim is the Applier callback that drives a deferred
// effectful value to completion. Unlike forceEffectful (which runs inside
// the execute loop with a frame for span attribution), this operates in the
// host-callback context where no frame is available.
//
// Handles the same two cases as forceEffectful:
//   - Auto-force thunks (rec thunks): force via runCallee
//   - Saturated effectful PrimVals: execute the prim directly
//
// Values that are neither are returned as-is.
func (vm *VM) forceEffectfulForPrim(v eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	if thv, ok := v.(*eval.VMThunkVal); ok && thv.AutoForce {
		if err := vm.budget.Step(); err != nil {
			return nil, capEnv, err
		}
		proto := protoOf(thv.Proto)
		result, err := vm.runCallee(capEnv, func(b *Frame) error {
			return vm.callThunk(proto, thv.Captured, b)
		})
		if err != nil {
			return nil, capEnv, err
		}
		return result.Value, result.CapEnv, nil
	}
	if pv, ok := v.(*eval.PrimVal); ok && pv.Effectful && len(pv.Args) >= pv.Arity {
		impl, err := vm.resolvePrimImplBare(pv)
		if err != nil {
			return nil, capEnv, err
		}
		val, newCap, callErr := vm.callPrim(impl, capEnv.MarkShared(), pv.Args, pv.S)
		if callErr != nil {
			return nil, capEnv, callErr
		}
		return val, newCap, nil
	}
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
	proto := protoOf(thv.Proto)
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

// applyForPrim is the Applier callback exposed to primitives.
// Uses barrier-frame approach: pushes a barrier frame + closure frame onto
// the existing frame stack, then re-enters execute(). When OpReturn hits
// the barrier, it stops and returns the result. No slice allocation needed.
func (vm *VM) applyForPrim(fn eval.Value, arg eval.Value, capEnv eval.CapEnv) (eval.Value, eval.CapEnv, error) {
	switch f := fn.(type) {
	case *eval.VMClosure:
		proto := protoOf(f.Proto)
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
			proto := protoOf(f.Fun.Proto)
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
		dc := f.DictArgCount
		if dc == len(f.Args) {
			dc = eval.CountLeadingDictArgs(args)
		}
		return &eval.ConVal{Con: f.Con, Args: args, DictArgCount: dc}, capEnv, nil

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
			val, newCap, callErr := vm.callPrim(impl, capEnv, scratch, f.S)
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
		val, newCap, callErr := vm.callPrim(impl, capEnv, args, f.S)
		if callErr != nil {
			return nil, capEnv, callErr
		}
		return val, newCap, nil

	case *eval.VMThunkVal:
		// Force thunk (ignore argument) — enables Go primitives to
		// transparently handle lazy co-data fields.
		proto := protoOf(f.Proto)
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
		proto := protoOf(f.Proto)
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
			proto := protoOf(f.Fun.Proto)
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
			proto := protoOf(f.Fun.Proto)
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
		dc := f.DictArgCount
		if dc == len(f.Args) {
			dc = eval.CountLeadingDictArgs(newArgs)
		}
		return &eval.ConVal{Con: f.Con, Args: newArgs, DictArgCount: dc}, capEnv, nil

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
			val, newCap, callErr := vm.callPrim(impl, capEnv, scratch, f.S)
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
			val, newCap, callErr := vm.callPrim(impl, capEnv, combined, f.S)
			if callErr != nil {
				return nil, capEnv, callErr
			}
			return val, newCap, nil
		}
		// Over-saturated: invoke with arity args, then apply rest.
		val, newCap, err := vm.callPrim(impl, capEnv, combined[:f.Arity], f.S)
		if err != nil {
			return nil, capEnv, err
		}
		return vm.applyNForPrim(val, combined[f.Arity:], newCap)

	case *eval.VMThunkVal:
		proto := protoOf(f.Proto)
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
