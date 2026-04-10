package vm

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// --- main dispatch loop ---

// execute drives the bytecode dispatch loop until one of two terminating
// conditions:
//
//  1. A frame's OpReturn pops back to a barrier frame caller (the case that
//     runCallee relies on); execute returns the popped value.
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
			if err := vm.budget.Alloc(eval.CostClosure + int64(len(proto.Captures))*eval.CostPAPArg); err != nil {
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

		case OpRecurseSelf:
			// Saturated self-call inside a fix body: target proto is
			// frame.proto, captures + self slot live at the current
			// frame's bp..bp+FixSelfSlot+1. Skip type dispatch / arity
			// check / observer (fix closures have no name) and enter
			// the new frame directly.
			arity := int(frame.proto.Code[frame.ip])
			frame.ip++
			proto := frame.proto
			if err := vm.budget.Enter(); err != nil {
				return eval.EvalResult{}, err
			}
			numCaptured := proto.FixSelfSlot + 1
			oldBp := frame.bp
			newBp := len(vm.locals)
			argsBase := vm.sp - arity
			vm.ensureLocals(newBp + proto.NumLocals)
			// Both regions now live in the same backing slice; copy the
			// captures + self slot from the caller's frame verbatim.
			copy(vm.locals[newBp:newBp+numCaptured], vm.locals[oldBp:oldBp+numCaptured])
			// Copy args from the operand stack into the new frame's
			// param slots (paramBase == numCaptured for a fix frame).
			copy(vm.locals[newBp+numCaptured:newBp+numCaptured+arity], vm.stack[argsBase:vm.sp])
			clear(vm.stack[argsBase:vm.sp])
			vm.sp = argsBase
			vm.pushFrame(proto, newBp, frame.capEnv, proto.Source)
			// frame pointer is now stale; the execute loop re-fetches
			// via currentFrame on the next iteration.

		case OpTailRecurseSelf:
			// Tail variant: reuse the current frame. Captures + self
			// slot are already in place (same proto), so only body
			// locals need clearing and the param slots need rewriting.
			arity := int(frame.proto.Code[frame.ip])
			frame.ip++
			proto := frame.proto
			paramBase := proto.FixSelfSlot + 1
			writtenUpTo := paramBase + arity
			numLocals := proto.NumLocals
			bp := frame.bp
			// Clear body locals carried over from the previous iteration
			// so GC can reclaim values no longer reachable.
			if writtenUpTo < numLocals {
				clear(vm.locals[bp+writtenUpTo : bp+numLocals])
			}
			argsBase := vm.sp - arity
			copy(vm.locals[bp+paramBase:bp+writtenUpTo], vm.stack[argsBase:vm.sp])
			clear(vm.stack[argsBase:vm.sp])
			vm.sp = argsBase
			frame.ip = 0

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
			if err := vm.budget.Alloc(eval.CostThunk + int64(len(proto.Captures))*eval.CostPAPArg); err != nil {
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
			vm.push(&eval.ConVal{Con: name, Args: args, DictArgCount: eval.CountLeadingDictArgs(args)})

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
			// Charge for the temporary updates slice upfront.
			if err := vm.budget.Alloc(int64(n) * eval.CostRecordField); err != nil {
				return eval.EvalResult{}, err
			}
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
			if err := vm.budget.Alloc(int64(len(desc.Labels)) * eval.CostPAPArg); err != nil {
				return eval.EvalResult{}, err
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
			// Charge header + capture slice + full NumLocals environment.
			fixCost := eval.CostFix + int64(len(proto.Captures))*eval.CostPAPArg + int64(proto.NumLocals)*eval.CostPAPArg
			if err := vm.budget.Alloc(fixCost); err != nil {
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
			fixCost := eval.CostFix + int64(len(proto.Captures))*eval.CostPAPArg + int64(proto.NumLocals)*eval.CostPAPArg
			if err := vm.budget.Alloc(fixCost); err != nil {
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
			val, newCap, err := vm.callPrim(impl, frame.capEnv, vm.stack[base:vm.sp], frame.proto.SpanAt(frame.ip))
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
			if err := vm.budget.Alloc(eval.CostPAP + int64(arity)*eval.CostPAPArg); err != nil {
				return eval.EvalResult{}, err
			}
			args := make([]eval.Value, arity)
			for i := arity - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}
			vm.push(&eval.PrimVal{
				Name: name, Arity: arity,
				Effectful: true, Args: args,
				S:    frame.proto.SpanAt(frame.ip),
				Impl: impl,
			})

		default:
			return eval.EvalResult{}, vm.runtimeError(
				fmt.Sprintf("unknown opcode: %d", op), frame)
		}
	}
}
