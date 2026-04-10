package vm

import (
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

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
		dc := f.DictArgCount
		if dc == len(f.Args) {
			// New arg may also be a dict.
			dc = eval.CountLeadingDictArgs(args)
		}
		vm.push(&eval.ConVal{Con: f.Con, Args: args, DictArgCount: dc})
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
