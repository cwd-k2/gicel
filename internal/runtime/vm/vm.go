package vm

import (
	"context"
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Frame is a single activation record on the VM's call stack.
type Frame struct {
	proto    *Proto
	ip       int         // instruction pointer within proto.Code
	bp       int         // base pointer into vm.locals
	capEnv   eval.CapEnv // capability environment for this frame
	source   *span.Source
	leaveObs bool // true if LeaveInternal should be called on return
	barrier  bool // true for barrier frames (Applier callback boundary)
}

// VM is the bytecode virtual machine for GICEL.
type VM struct {
	// Operand stack.
	stack []eval.Value
	sp    int // stack pointer (index of next push)

	// Local variable storage (all frames share a flat array).
	locals []eval.Value

	// Call stack.
	frames []Frame
	fp     int // frame pointer (index of current frame in frames)

	// Global environment.
	globals     []eval.Value
	globalSlots map[string]int

	// Primitives.
	prims *eval.PrimRegistry

	// Resource budget.
	budget *budget.Budget
	ctx    context.Context

	// Observer (explain mode).
	obs *eval.ExplainObserver

	// Trace hook (low-level step events).
	trace eval.TraceHook

	// Source for error attribution.
	source *span.Source

	// Statistics.
	stats eval.EvalStats

	// Cached applier for primitive callbacks.
	cachedApplier eval.Applier

	// Scratch buffer for assembling primitive arguments in the saturated
	// non-effectful fast path. Avoids a heap allocation per call.
	// Invariant: only used in applyPrim (main dispatch), never in
	// applyForPrim (re-entrant from primitives), so no aliasing hazard.
	primScratch [8]eval.Value
}

// VMConfig holds configuration for creating a VM.
type VMConfig struct {
	Globals     []eval.Value
	GlobalSlots map[string]int
	Prims       *eval.PrimRegistry
	Budget      *budget.Budget
	Ctx         context.Context
	Observer    *eval.ExplainObserver
	Trace       eval.TraceHook
	Source      *span.Source
}

// NewVM creates a VM ready to execute a Proto.
func NewVM(cfg VMConfig) *VM {
	vm := &VM{
		stack:       make([]eval.Value, 0, 256),
		locals:      make([]eval.Value, 0, 256),
		frames:      make([]Frame, 0, 64),
		globals:     cfg.Globals,
		globalSlots: cfg.GlobalSlots,
		prims:       cfg.Prims,
		budget:      cfg.Budget,
		ctx:         cfg.Ctx,
		obs:         cfg.Observer,
		trace:       cfg.Trace,
		source:      cfg.Source,
	}
	vm.cachedApplier = eval.Applier{
		Apply:  vm.applyForPrim,
		ApplyN: vm.applyNForPrim,
	}
	return vm
}

// Run executes a Proto and returns the result. Can be called multiple times
// on the same VM (e.g., for evaluating module bindings sequentially).
func (vm *VM) Run(proto *Proto, capEnv eval.CapEnv) (eval.EvalResult, error) {
	// Save depth/observer state so we can restore on error.
	savedDepth := vm.budget.Depth()

	// Reset execution state for this run (globals/prims/budget are preserved).
	vm.sp = 0
	vm.stack = vm.stack[:0]
	vm.locals = vm.locals[:0]
	vm.frames = vm.frames[:0]
	vm.fp = 0
	vm.pushFrame(proto, 0, capEnv, proto.Source)
	result, err := vm.execute()
	if err != nil {
		// Restore budget depth to the pre-Run state so subsequent Runs
		// on the same VM do not accumulate unbalanced Enter() calls.
		for vm.budget.Depth() > savedDepth {
			vm.budget.Leave()
		}
		// Clear execution state to prevent stale references.
		vm.sp = 0
		vm.stack = vm.stack[:0]
		vm.locals = vm.locals[:0]
		vm.frames = vm.frames[:0]
		vm.fp = 0
	}
	return result, err
}

// Stats returns the accumulated evaluation statistics.
func (vm *VM) Stats() eval.EvalStats {
	vm.stats.Allocated = vm.budget.Allocated()
	if pd := vm.budget.PeakDepth(); pd > vm.stats.MaxDepth {
		vm.stats.MaxDepth = pd
	}
	return vm.stats
}

// --- frame management ---

func (vm *VM) pushFrame(proto *Proto, bp int, capEnv eval.CapEnv, source *span.Source) {
	vm.frames = append(vm.frames, Frame{
		proto:  proto,
		ip:     0,
		bp:     bp,
		capEnv: capEnv,
		source: source,
	})
	vm.fp = len(vm.frames) - 1
}

func (vm *VM) currentFrame() *Frame {
	return &vm.frames[vm.fp]
}

func (vm *VM) popFrame() {
	vm.fp--
	vm.frames = vm.frames[:vm.fp+1]
}

// --- stack operations ---

func (vm *VM) push(v eval.Value) {
	if vm.sp >= len(vm.stack) {
		vm.stack = append(vm.stack, v)
	} else {
		vm.stack[vm.sp] = v
	}
	vm.sp++
}

func (vm *VM) pop() eval.Value {
	vm.sp--
	v := vm.stack[vm.sp]
	vm.stack[vm.sp] = nil // GC
	return v
}

// --- local access ---

func (vm *VM) getLocal(frame *Frame, slot int) eval.Value {
	return vm.locals[frame.bp+slot]
}

func (vm *VM) setLocal(frame *Frame, slot int, v eval.Value) {
	idx := frame.bp + slot
	if idx >= len(vm.locals) {
		vm.ensureLocals(idx + 1)
	}
	vm.locals[idx] = v
}

// ensureLocals grows the locals array to at least the given size.
// New slots are zero-initialized (nil).
func (vm *VM) ensureLocals(n int) {
	cur := len(vm.locals)
	if cur >= n {
		return
	}
	if n <= cap(vm.locals) {
		vm.locals = vm.locals[:n]
		clear(vm.locals[cur:n])
	} else {
		grown := make([]eval.Value, n, max(n, cap(vm.locals)*2))
		copy(grown, vm.locals)
		vm.locals = grown
	}
}

// --- main dispatch loop ---

// execute drives the bytecode dispatch loop until one of two terminating
// conditions:
//
//  1. A frame's OpReturn pops back to a barrier frame caller (the case that
//     execBarrier / runCallee rely on); execute returns the popped value.
//  2. The bottom-most frame (vm.fp == 0) executes OpReturn or runs out of
//     bytecode; execute returns the value with the top-level capEnv.
//
// Pre-condition: vm.currentFrame() must be a non-barrier frame with at
// least one instruction to dispatch (or an empty proto whose return value
// is already on the operand stack). A barrier frame at the top is treated
// as a programming error — callers that need to push a barrier should also
// push a real call frame on top of it before calling execute.
func (vm *VM) execute() (eval.EvalResult, error) {
	for {
		frame := vm.currentFrame()
		if frame.barrier {
			// Should not execute a barrier frame; this indicates a bug.
			// Callers that need value-only setup should use runCallee,
			// which detects "no frame pushed" before invoking execute.
			return eval.EvalResult{}, &eval.RuntimeError{Message: "executed barrier frame"}
		}
		if frame.ip >= len(frame.proto.Code) {
			if vm.sp > 0 {
				result := vm.pop()
				if vm.fp == 0 {
					return eval.EvalResult{Value: result, CapEnv: frame.capEnv}, nil
				}
				callerCapEnv := frame.capEnv
				vm.budget.Leave()
				vm.locals = vm.locals[:frame.bp]
				vm.popFrame()
				vm.push(result)
				vm.currentFrame().capEnv = callerCapEnv
				continue
			}
			return eval.EvalResult{}, &eval.RuntimeError{Message: "empty stack at return"}
		}

		op := Opcode(frame.proto.Code[frame.ip])
		frame.ip++

		switch op {
		case OpStep:
			kindIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			vm.stats.Steps++
			if err := vm.budget.Step(); err != nil {
				return eval.EvalResult{}, err
			}
			if vm.trace != nil {
				kind := ""
				if int(kindIdx) < len(frame.proto.Strings) {
					kind = frame.proto.Strings[kindIdx]
				}
				if err := vm.trace(eval.TraceEvent{
					Depth:    vm.budget.Depth(),
					NodeKind: kind,
					CapEnv:   frame.capEnv,
				}); err != nil {
					return eval.EvalResult{}, err
				}
			}

		case OpLoadLocal:
			slot := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			v := vm.getLocal(frame, int(slot))
			// Dereference IndirectVal.
			if ind, ok := v.(*eval.IndirectVal); ok {
				if ind.Ref == nil {
					return eval.EvalResult{}, vm.runtimeError("uninitialized forward reference", frame)
				}
				v = *ind.Ref
			}
			vm.push(v)

		case OpLoadGlobal:
			slot := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			v := vm.globals[slot]
			if ind, ok := v.(*eval.IndirectVal); ok {
				if ind.Ref == nil {
					return eval.EvalResult{}, vm.runtimeError("uninitialized forward reference", frame)
				}
				v = *ind.Ref
			}
			vm.push(v)

		case OpStoreLocal:
			slot := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			vm.setLocal(frame, int(slot), vm.pop())

		case OpConst:
			idx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			v := frame.proto.Constants[idx]
			// Charge allocation for string literals (matches tree-walker).
			if hv, ok := v.(*eval.HostVal); ok {
				if s, ok := hv.Inner.(string); ok && len(s) > 0 {
					if err := vm.budget.Alloc(int64(len(s))); err != nil {
						return eval.EvalResult{}, err
					}
				}
			}
			vm.push(v)

		case OpConstUnit:
			vm.push(eval.UnitVal)

		case OpClosure:
			idx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			proto := frame.proto.Protos[idx]
			if err := vm.budget.Alloc(eval.CostClosure); err != nil {
				return eval.EvalResult{}, err
			}
			captured := vm.captureLocals(frame, proto.Captures)
			vm.push(&eval.VMClosure{
				Captured: captured,
				Proto:    proto,
				Source:   proto.Source,
			})

		case OpApply:
			arg := vm.pop()
			fn := vm.pop()
			if err := vm.apply(fn, arg, frame, false); err != nil {
				return eval.EvalResult{}, err
			}

		case OpTailApply:
			arg := vm.pop()
			fn := vm.pop()
			if err := vm.apply(fn, arg, frame, true); err != nil {
				return eval.EvalResult{}, err
			}

		case OpApplyN:
			n := int(frame.proto.Code[frame.ip])
			frame.ip++
			if err := vm.dispatchApplyN(n, frame, false); err != nil {
				return eval.EvalResult{}, err
			}

		case OpTailApplyN:
			n := int(frame.proto.Code[frame.ip])
			frame.ip++
			if err := vm.dispatchApplyN(n, frame, true); err != nil {
				return eval.EvalResult{}, err
			}

		case OpReturn:
			result := vm.pop()
			// Force auto-force thunks before returning (mirrors tree-walker's
			// deferred ForceEffectful on Bind body results).
			if thv, ok := result.(*eval.VMThunkVal); ok && thv.AutoForce {
				if err := vm.budget.Step(); err != nil {
					return eval.EvalResult{}, err
				}
				// Fire pending LeaveInternal before reusing frame.
				if frame.leaveObs {
					vm.obs.LeaveInternal()
					frame.leaveObs = false
				}
				proto := thv.Proto.(*Proto)
				// Reuse current frame for thunk execution (TCO).
				// INVARIANT: no frame push may occur between obtaining `frame`
				// (at loop top) and this `continue`. The pointer aliases
				// vm.frames[vm.fp]; a push would invalidate it.
				bp := frame.bp
				n := proto.NumLocals
				vm.ensureLocals(bp + n)
				numCap := len(thv.Captured)
				if numCap < n {
					clear(vm.locals[bp+numCap : bp+n])
				}
				copy(vm.locals[bp:], thv.Captured)
				frame.proto = proto
				frame.ip = 0
				continue
			}
			if frame.leaveObs {
				vm.obs.LeaveInternal()
			}
			if vm.fp == 0 {
				return eval.EvalResult{Value: result, CapEnv: frame.capEnv}, nil
			}
			callerCapEnv := frame.capEnv
			vm.budget.Leave()
			vm.locals = vm.locals[:frame.bp]
			vm.popFrame()
			caller := vm.currentFrame()
			if caller.barrier {
				// Barrier frame: stop execution, return result to Applier.
				return eval.EvalResult{Value: result, CapEnv: callerCapEnv}, nil
			}
			vm.push(result)
			caller.capEnv = callerCapEnv
			// Restore observer source context to caller's source.
			if caller.source != nil && vm.obs != nil {
				vm.obs.SetSource(caller.source)
			}

		case OpThunk:
			idx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			proto := frame.proto.Protos[idx]
			if err := vm.budget.Alloc(eval.CostThunk); err != nil {
				return eval.EvalResult{}, err
			}
			captured := vm.captureLocals(frame, proto.Captures)
			// Mark capEnv as shared to prevent thunk from seeing later mutations.
			frame.capEnv = frame.capEnv.MarkShared()
			vm.push(&eval.VMThunkVal{
				Captured: captured,
				Proto:    proto,
				Source:   proto.Source,
			})

		case OpForce:
			v := vm.pop()
			if err := vm.force(v, frame, false); err != nil {
				return eval.EvalResult{}, err
			}

		case OpForceTail:
			v := vm.pop()
			if err := vm.force(v, frame, true); err != nil {
				return eval.EvalResult{}, err
			}

		case OpForceEffectful:
			v := vm.pop()
			result, newCapEnv, err := vm.forceEffectful(v, frame.capEnv, frame, frame.proto.SpanAt(frame.ip))
			if err != nil {
				return eval.EvalResult{}, err
			}
			frame.capEnv = newCapEnv
			vm.push(result)

		case OpBind:
			bindIP := frame.ip - 1 // position of OpBind itself
			slot := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			v := vm.pop()
			result, newCapEnv, err := vm.forceEffectful(v, frame.capEnv, frame, frame.proto.SpanAt(bindIP))
			if err != nil {
				return eval.EvalResult{}, err
			}
			frame.capEnv = newCapEnv
			vm.setLocal(frame, int(slot), result)
			// Observer: emit bind event.
			if vm.obs.Active() {
				if name := vm.lookupBindName(frame.proto, int(slot)); name != "" {
					vm.obs.Emit(vm.budget.Depth(), eval.ExplainBind,
						eval.ExplainDetail{Var: name, Value: eval.PrettyValue(result), Monadic: true},
						frame.proto.SpanAt(frame.ip-3))
				}
			}

		case OpMerge:
			mergeIP := frame.ip - 1
			descIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			desc := frame.proto.MergeDescs[descIdx]
			rightVal := vm.pop()
			leftVal := vm.pop()
			origCapEnv := frame.capEnv.MarkShared()
			_ = frame.proto.SpanAt(mergeIP)
			// Split CapEnv by label sets when available. Type-level Merge row
			// family guarantees disjointness. When labels are nil (unresolved
			// metas at compile time), fall back to shared CapEnv.
			leftCap, rightCap := origCapEnv, origCapEnv
			if desc.LeftLabels != nil || desc.RightLabels != nil {
				leftCap = origCapEnv.Filter(desc.LeftLabels)
				rightCap = origCapEnv.Filter(desc.RightLabels)
			}
			val1, leftCE, err := vm.forceMergeChild(leftVal, leftCap)
			if err != nil {
				return eval.EvalResult{}, err
			}
			val2, rightCE, err := vm.forceMergeChild(rightVal, rightCap)
			if err != nil {
				return eval.EvalResult{}, err
			}
			// Merge resulting CapEnvs and push pair record.
			frame.capEnv = leftCE.MergeWith(rightCE)
			vm.push(eval.NewRecordFromMap(map[string]eval.Value{
				"_1": val1,
				"_2": val2,
			}))

		case OpCon:
			nameIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			arity := int(frame.proto.Code[frame.ip])
			frame.ip++
			if err := vm.budget.Alloc(eval.CostConBase + int64(eval.CostConArg*arity)); err != nil {
				return eval.EvalResult{}, err
			}
			name := frame.proto.Strings[nameIdx]
			args := make([]eval.Value, arity)
			for i := arity - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
			vm.push(&eval.ConVal{Con: name, Args: args})

		case OpRecord:
			descIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			desc := frame.proto.RecordDescs[descIdx]
			n := len(desc.Labels)
			if err := vm.budget.Alloc(eval.CostRecord + int64(eval.CostRecordField*n)); err != nil {
				return eval.EvalResult{}, err
			}
			fields := make([]eval.RecordField, n)
			for i := n - 1; i >= 0; i-- {
				fields[i] = eval.RecordField{Label: desc.Labels[i], Value: vm.pop()}
			}
			vm.push(eval.NewRecord(fields))

		case OpRecordProj:
			labelIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			label := frame.proto.Strings[labelIdx]
			rec := vm.pop()
			rv, ok := rec.(*eval.RecordVal)
			if !ok {
				return eval.EvalResult{}, vm.runtimeError(
					"record projection on non-record: "+rec.String(), frame)
			}
			val, ok := rv.Get(label)
			if !ok {
				return eval.EvalResult{}, vm.runtimeError(
					fmt.Sprintf("missing field %q in record", label), frame)
			}
			vm.push(val)

		case OpRecordUpdate:
			descIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			desc := frame.proto.RecordDescs[descIdx]
			n := len(desc.Labels)
			updates := make([]eval.RecordField, n)
			for i := n - 1; i >= 0; i-- {
				updates[i] = eval.RecordField{Label: desc.Labels[i], Value: vm.pop()}
			}
			rec := vm.pop()
			rv, ok := rec.(*eval.RecordVal)
			if !ok {
				return eval.EvalResult{}, vm.runtimeError(
					"record update on non-record: "+rec.String(), frame)
			}
			// Charge for the full result record (existing fields + updates).
			totalFields := rv.Len() + n
			if err := vm.budget.Alloc(eval.CostRecord + int64(eval.CostRecordField*totalFields)); err != nil {
				return eval.EvalResult{}, err
			}
			vm.push(rv.Update(updates))

		case OpMatchCon:
			descIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			failOffset := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			desc := frame.proto.MatchDescs[descIdx]
			scrut := vm.pop()
			cv, ok := scrut.(*eval.ConVal)
			if !ok || cv.Con != desc.ConName || len(cv.Args) != len(desc.ArgSlots) {
				frame.ip = int(failOffset)
				continue
			}
			for i, slot := range desc.ArgSlots {
				vm.setLocal(frame, slot, cv.Args[i])
			}
			// Observer: emit match event with scrutinee, pattern, and bindings.
			if vm.obs.Active() {
				detail := eval.ExplainDetail{
					Scrutinee: eval.PrettyValue(scrut),
					Pattern:   formatMatchConPattern(desc),
				}
				if len(desc.ArgNames) > 0 {
					detail.Bindings = make(map[string]string, len(desc.ArgNames))
					for i, name := range desc.ArgNames {
						if name != "" && !desc.ArgGenerated[i] && i < len(cv.Args) {
							detail.Bindings[name] = eval.PrettyValue(cv.Args[i])
						}
					}
				}
				vm.obs.Emit(vm.budget.Depth(), eval.ExplainMatch, detail,
					frame.proto.SpanAt(frame.ip-5))
			}

		case OpMatchLit:
			litIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			failOffset := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			expected := frame.proto.Constants[litIdx]
			scrut := vm.pop() // always pop
			if !litEqual(scrut, expected) {
				frame.ip = int(failOffset)
				continue
			}

		case OpMatchRecord:
			descIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			failOffset := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			desc := frame.proto.MatchDescs[descIdx]
			scrut := vm.pop() // always pop
			rv, ok := scrut.(*eval.RecordVal)
			if !ok {
				frame.ip = int(failOffset)
				continue
			}
			matched := true
			vals := make([]eval.Value, len(desc.Labels))
			for i, label := range desc.Labels {
				v, ok := rv.Get(label)
				if !ok {
					matched = false
					break
				}
				vals[i] = v
			}
			if !matched {
				frame.ip = int(failOffset)
				continue
			}
			for i, slot := range desc.FieldSlots {
				vm.setLocal(frame, slot, vals[i])
			}

		case OpMatchWild:
			vm.pop()

		case OpMatchFail:
			msg := "non-exhaustive pattern match"
			if vm.sp > 0 {
				v := vm.pop()
				msg += " on " + eval.PrettyValue(v)
			}
			return eval.EvalResult{}, vm.runtimeError(msg, frame)

		case OpRaise:
			v := vm.pop()
			msg := eval.PrettyValue(v)
			if hv, ok := v.(*eval.HostVal); ok {
				if s, ok := hv.Inner.(string); ok {
					msg = s
				}
			}
			return eval.EvalResult{}, vm.runtimeError(msg, frame)

		case OpJump:
			offset := DecodeI16(frame.proto.Code, frame.ip)
			frame.ip += 2
			frame.ip += int(offset)

		case OpPop:
			vm.pop()

		case OpFixClosure:
			idx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			proto := frame.proto.Protos[idx]
			if err := vm.budget.Alloc(eval.CostFix); err != nil {
				return eval.EvalResult{}, err
			}
			captured := vm.captureLocals(frame, proto.Captures)
			clo := &eval.VMClosure{
				Captured: make([]eval.Value, proto.NumLocals),
				Proto:    proto,
				Source:   proto.Source,
			}
			copy(clo.Captured, captured)
			// Tie the knot: self-reference at FixSelfSlot.
			clo.Captured[proto.FixSelfSlot] = clo
			vm.push(clo)

		case OpFixThunk:
			idx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			proto := frame.proto.Protos[idx]
			if err := vm.budget.Alloc(eval.CostFix); err != nil {
				return eval.EvalResult{}, err
			}
			captured := vm.captureLocals(frame, proto.Captures)
			thv := &eval.VMThunkVal{
				Captured:  make([]eval.Value, proto.NumLocals),
				Proto:     proto,
				Source:    proto.Source,
				AutoForce: true,
			}
			copy(thv.Captured, captured)
			thv.Captured[proto.FixSelfSlot] = thv
			vm.push(thv)

		case OpPrim:
			nameIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			arity := int(frame.proto.Code[frame.ip])
			frame.ip++
			// Resolve primitive: prefer compile-time resolved impl,
			// fall back to registry lookup for unresolved entries.
			var impl eval.PrimImpl
			if int(nameIdx) < len(frame.proto.ResolvedPrims) {
				impl = frame.proto.ResolvedPrims[nameIdx]
			}
			if impl == nil {
				name := frame.proto.Strings[nameIdx]
				var ok bool
				impl, ok = vm.prims.Lookup(name)
				if !ok {
					return eval.EvalResult{}, vm.runtimeError(
						"missing primitive: "+name, frame)
				}
			}
			// Args are contiguous on the stack. Pass a view directly
			// instead of allocating a fresh slice. Safe because OpPrim
			// is emitted only for non-effectful saturated primitives,
			// which do not call the Applier (no re-entrant stack mutation).
			base := vm.sp - arity
			val, newCap, err := vm.callTrustedPrim(impl, frame.capEnv, vm.stack[base:vm.sp])
			for i := base; i < vm.sp; i++ {
				vm.stack[i] = nil
			}
			vm.sp = base
			if err != nil {
				return eval.EvalResult{}, err
			}
			frame.capEnv = newCap
			vm.push(val)

		case OpPrimPartial:
			idx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			vm.push(frame.proto.Constants[idx])

		case OpEffectPrim:
			nameIdx := DecodeU16(frame.proto.Code, frame.ip)
			frame.ip += 2
			arity := int(frame.proto.Code[frame.ip])
			frame.ip++
			name := frame.proto.Strings[nameIdx]
			// Resolve impl up-front so the eventual force-effectful path
			// (and any subsequent applyN that touches this PrimVal) can
			// skip the per-call PrimRegistry lookup.
			var impl eval.PrimImpl
			if int(nameIdx) < len(frame.proto.ResolvedPrims) {
				impl = frame.proto.ResolvedPrims[nameIdx]
			}
			args := make([]eval.Value, arity)
			for i := arity - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
			vm.push(&eval.PrimVal{
				Name: name, Arity: arity,
				Effectful: true, Args: args,
				S: frame.proto.SpanAt(frame.ip),
				Impl: impl,
			})

		default:
			return eval.EvalResult{}, vm.runtimeError(
				fmt.Sprintf("unknown opcode: %d", op), frame)
		}
	}
}

// --- helpers ---

func (vm *VM) captureLocals(frame *Frame, indices []int) []eval.Value {
	if len(indices) == 0 {
		return nil
	}
	captured := make([]eval.Value, len(indices))
	for i, idx := range indices {
		captured[i] = vm.getLocal(frame, idx)
	}
	return captured
}

func (vm *VM) runtimeError(msg string, frame *Frame) *eval.RuntimeError {
	s := frame.proto.SpanAt(frame.ip)
	return &eval.RuntimeError{
		Message: msg,
		Span:    s,
		Source:  frame.source,
	}
}

// formatMatchConPattern formats a constructor pattern for observer display.
func formatMatchConPattern(desc MatchDesc) string {
	if len(desc.ArgNames) == 0 {
		return desc.ConName
	}
	return desc.ConName + " " + strings.Join(desc.ArgNames, " ")
}

// lookupBindName returns the variable name for an OpBind slot, or "".
func (vm *VM) lookupBindName(proto *Proto, slot int) string {
	for _, bi := range proto.BindNames {
		if bi.Slot == slot {
			return bi.Name
		}
	}
	return ""
}

// litEqual compares two literal values for equality (pattern matching).
func litEqual(a, b eval.Value) bool {
	ha, ok1 := a.(*eval.HostVal)
	hb, ok2 := b.(*eval.HostVal)
	if !ok1 || !ok2 {
		return false
	}
	return ha.Inner == hb.Inner
}
