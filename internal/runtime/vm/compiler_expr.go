// Compiler — IR node compilation.
// Does NOT cover: bytecode emission (compiler_emit.go),
//                 child proto builders (compiler_closure.go),
//                 pattern matching (compiler_match.go).

package vm

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/ir"

	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// --- compilation ---

func (c *Compiler) compileExpr(expr ir.Core, tail bool) {
	switch node := expr.(type) {
	case *ir.TyApp:
		c.compileExpr(node.Expr, tail)
		return
	case *ir.TyLam:
		c.compileExpr(node.Body, tail)
		return
	case *ir.Pure:
		c.compileExpr(node.Expr, tail)
		return
	}

	// Emit step only for reductions (CBPV operational semantics).
	// Var/Lit are value forms; Lam/Thunk/Con/Record/Merge produce values
	// without reducing. Only App, Force, Bind, Case, PrimOp, and Fix
	// correspond to computational steps.
	switch expr.(type) {
	case *ir.App, *ir.Force, *ir.Bind, *ir.Case, *ir.PrimOp, *ir.Fix:
		c.emitStep(expr)
	}

	switch node := expr.(type) {
	case *ir.Var:
		c.compileVar(node)
	case *ir.Lit:
		c.compileLit(node)
	case *ir.Lam:
		c.compileLam(node)
	case *ir.App:
		c.compileApp(node, tail)
	case *ir.Con:
		c.compileCon(node)
	case *ir.Case:
		c.compileCase(node, tail)
	case *ir.Fix:
		c.compileFix(node)
	case *ir.Bind:
		c.compileBind(node, tail)
	case *ir.Thunk:
		c.compileThunk(node)
	case *ir.Force:
		c.compileForce(node, tail)
	case *ir.Merge:
		c.compileMerge(node)
	case *ir.PrimOp:
		c.compilePrimOp(node)
	case *ir.RecordLit:
		c.compileRecordLit(node)
	case *ir.RecordProj:
		c.compileRecordProj(node)
	case *ir.RecordUpdate:
		c.compileRecordUpdate(node)
	default:
		panic(fmt.Sprintf("vm/compiler: unhandled Core node %T", expr))
	}
}
func (c *Compiler) compileVar(v *ir.Var) {
	c.addSpan(v.S)
	if v.Index >= 0 {
		if slot, ok := c.resolveLocal(v.Name); ok {
			c.emitU16(OpLoadLocal, uint16(slot))
			return
		}
		panic(fmt.Sprintf("vm/compiler: unresolved local variable %q (index %d)", v.Name, v.Index))
	}
	key := v.Key
	if key == "" {
		key = ir.VarKey(v)
	}
	if slot, ok := c.globalSlots[key]; ok {
		c.emitU16(OpLoadGlobal, uint16(slot))
	} else {
		panic(fmt.Sprintf("vm/compiler: global %q (key %q) not in globalSlots", v.Name, key))
	}
}
func (c *Compiler) compileLit(lit *ir.Lit) {
	c.addSpan(lit.S)
	idx := c.addConstant(&eval.HostVal{Inner: lit.Value})
	c.emitU16(OpConst, idx)
}
func (c *Compiler) compileLam(lam *ir.Lam) {
	c.addSpan(lam.S)
	// Flatten lambda chain: \x. \y. \z. body → single Proto with Params=["x","y","z"].
	params, body, fv, fvIndices := flattenLamChain(lam)
	child := c.compileMultiParamProto(params, body, fv, fvIndices)
	protoIdx := c.addProto(child)
	c.emitU16(OpClosure, protoIdx)
}

// irLamArity returns the arity of a Lam chain, or -1 if expr is not a Lam.
// flattenLamChain collects a chain of nested Lam nodes into a parameter
// list and the innermost body. Returns the outermost Lam's FV/FVIndices
// (inner params become local slots in the flattened Proto, not captures).
func flattenLamChain(lam *ir.Lam) (params []string, body ir.Core, fv []string, fvIndices []int) {
	params = []string{lam.Param}
	fv = lam.FV
	fvIndices = lam.FVIndices
	body = lam.Body
	for {
		inner := ir.PeelTyLam(body)
		innerLam, ok := inner.(*ir.Lam)
		if !ok {
			break
		}
		params = append(params, innerLam.Param)
		body = innerLam.Body
	}
	return
}

// compileMultiParamProto compiles a closure body with zero or more parameters.
// compileMultiParamProto compiles a closure body with zero or more parameters.
func (c *Compiler) compileMultiParamProto(params []string, body ir.Core, fv []string, fvIndices []int) *Proto {
	capturedNames, captureSlots := c.resolveCapturesFiltered(fv, fvIndices)
	c.enterFrame()
	f := c.top()
	f.isThunk = false
	f.params = params
	f.captures = captureSlots
	for _, name := range capturedNames {
		c.allocLocal(name)
	}
	for _, param := range params {
		c.allocLocal(param)
	}
	c.compileExpr(body, true)
	c.emit(OpReturn)
	return c.leaveFrame()
}
func (c *Compiler) compileApp(app *ir.App, tail bool) {
	c.addSpan(app.S)
	// Collect application spine. Single OpApplyN dispatch handles every
	// runtime value type and every saturation case via applyN, so the
	// only structural decision left is "single arg vs multi arg".
	fn, args := collectAppSpine(app)

	// Direct prim-alias call: when the spine head is a Var pointing at a
	// bare prim-alias binding and we have at least the prim's arity in
	// args, emit OpPrim / OpEffectPrim directly. Bypasses the PrimVal
	// stub load + applyN dispatch entirely (Phase 1 optimization).
	if v, ok := fn.(*ir.Var); ok && len(args) > 0 {
		if info, ok := c.varGlobalPrim(v); ok && info.arity > 0 && info.arity <= 255 && len(args) >= info.arity {
			for _, arg := range args[:info.arity] {
				c.compileExpr(arg, false)
			}
			nameIdx := c.addString(info.name)
			if info.effectful {
				c.emitU16U8(OpEffectPrim, nameIdx, uint8(info.arity))
			} else {
				c.emitU16U8(OpPrim, nameIdx, uint8(info.arity))
			}
			// Over-application: remaining args applied sequentially.
			for i, extra := range args[info.arity:] {
				c.compileExpr(extra, false)
				if tail && i == len(args)-info.arity-1 {
					c.emit(OpTailApply)
				} else {
					c.emit(OpApply)
				}
			}
			return
		}
	}

	// Single-arg call: OpApply is cheapest (the dispatch loop pops fn
	// and arg directly without allocating an args slice).
	if len(args) == 1 {
		c.compileExpr(fn, false)
		c.compileExpr(args[0], false)
		if tail {
			c.emit(OpTailApply)
		} else {
			c.emit(OpApply)
		}
		return
	}

	// Multi-arg call: emit OpApplyN with the entire spine. applyN
	// handles every value type and every saturation case (saturated,
	// partial, over-applied), so the compiler does not need to know
	// the function's arity statically. This collapses the N-1
	// intermediate PrimVal/PAPVal allocations that the sequential
	// OpApply chain would have produced for unknown-arity calls.
	//
	// Args are emitted in 255-element chunks (the OpApplyN immediate is
	// a u8). For each chunk after the first, applyN's result is treated
	// as the new function being applied to the next chunk.
	c.compileExpr(fn, false)
	remaining := args
	for len(remaining) > 0 {
		chunk := remaining
		if len(chunk) > 255 {
			chunk = chunk[:255]
		}
		remaining = remaining[len(chunk):]
		for _, arg := range chunk {
			c.compileExpr(arg, false)
		}
		op := OpApplyN
		if tail && len(remaining) == 0 {
			op = OpTailApplyN
		}
		c.emitU8(op, uint8(len(chunk)))
	}
}

// collectAppSpine flattens left-nested App/TyApp chains into (fun, [arg0, arg1, ...]).
// collectAppSpine flattens left-nested App/TyApp chains into (fun, [arg0, arg1, ...]).
func collectAppSpine(node *ir.App) (ir.Core, []ir.Core) {
	var args []ir.Core
	var cur ir.Core = node
	for {
		a, ok := cur.(*ir.App)
		if !ok {
			break
		}
		args = append(args, a.Arg)
		cur = a.Fun
		// Peel type applications (erased at runtime).
		for {
			if ta, ok := cur.(*ir.TyApp); ok {
				cur = ta.Expr
				continue
			}
			break
		}
	}
	// Reverse: collected right-to-left.
	for i, j := 0, len(args)-1; i < j; i, j = i+1, j-1 {
		args[i], args[j] = args[j], args[i]
	}
	return cur, args
}
func (c *Compiler) compileCon(con *ir.Con) {
	c.addSpan(con.S)
	for _, arg := range con.Args {
		c.compileExpr(arg, false)
	}
	nameIdx := c.addString(con.Name)
	c.emitU16U8(OpCon, nameIdx, uint8(len(con.Args)))
}
func (c *Compiler) compileCase(cs *ir.Case, tail bool) {
	c.addSpan(cs.S)
	c.compileExpr(cs.Scrutinee, false)
	compilePatternMatch(c, cs, tail)
}
func (c *Compiler) compileFix(fix *ir.Fix) {
	c.addSpan(fix.S)
	body := ir.PeelTyLam(fix.Body)
	switch b := body.(type) {
	case *ir.Lam:
		params, innerBody, _, _ := flattenLamChain(b)
		child := c.compileFixMultiProto(fix.Name, params, innerBody, b.FV, b.FVIndices)
		protoIdx := c.addProto(child)
		c.emitU16(OpFixClosure, protoIdx)
	case *ir.Thunk:
		child := c.compileFixProto(fix.Name, "", b.Comp, true, b.FV, b.FVIndices)
		protoIdx := c.addProto(child)
		c.emitU16(OpFixThunk, protoIdx)
	default:
		msg := fmt.Sprintf(
			"fix binding %s requires a lambda or thunk body (got %T); "+
				"in CBV evaluation, fix creates a self-referential closure "+
				"and cannot produce data constructor values directly — "+
				"wrap the body in a lambda or use thunk/force",
			fix.Name, body)
		idx := c.addConstant(&eval.HostVal{Inner: msg})
		c.emitU16(OpConst, idx)
		c.emit(OpRaise)
	}
}
func (c *Compiler) compileBind(bind *ir.Bind, tail bool) {
	c.addSpan(bind.S)
	c.compileExpr(bind.Comp, false)
	slot := c.allocLocal(bind.Var)
	c.emitU16(OpBind, uint16(slot))
	if !bind.Discard && !bind.Generated {
		c.top().bindNames = append(c.top().bindNames, BindInfo{Slot: slot, Name: bind.Var})
	}
	c.compileExpr(bind.Body, tail)
}
func (c *Compiler) compileThunk(thunk *ir.Thunk) {
	c.addSpan(thunk.S)
	child := c.compileChildProto("", thunk.Comp, true, thunk.FV, thunk.FVIndices)
	protoIdx := c.addProto(child)
	c.emitU16(OpThunk, protoIdx)
}
func (c *Compiler) compileMerge(merge *ir.Merge) {
	c.addSpan(merge.S)

	leftProto := c.compileMergeChildProto(merge.Left, merge.LeftFV, merge.LeftFVIdx)
	leftIdx := c.addProto(leftProto)
	c.emitU16(OpThunk, leftIdx)

	rightProto := c.compileMergeChildProto(merge.Right, merge.RightFV, merge.RightFVIdx)
	rightIdx := c.addProto(rightProto)
	c.emitU16(OpThunk, rightIdx)

	descIdx := c.addMergeDesc(MergeDesc{
		LeftLabels:  merge.LeftLabels,
		RightLabels: merge.RightLabels,
	})
	c.emitU16(OpMerge, descIdx)
}
func (c *Compiler) compileForce(force *ir.Force, tail bool) {
	c.addSpan(force.S)
	c.compileExpr(force.Expr, false)
	if tail {
		c.emit(OpForceTail)
	} else {
		c.emit(OpForce)
	}
}
func (c *Compiler) compilePrimOp(prim *ir.PrimOp) {
	c.addSpan(prim.S)
	if len(prim.Args) == 0 {
		if prim.Arity == 0 && !prim.Effectful {
			nameIdx := c.addString(prim.Name)
			c.emitU16U8(OpPrim, nameIdx, 0)
			return
		}
		stub := &eval.PrimVal{
			Name: prim.Name, Arity: prim.Arity,
			Effectful: prim.Effectful, S: prim.S,
		}
		idx := c.addConstant(stub)
		c.emitU16(OpPrimPartial, idx)
		return
	}
	for _, arg := range prim.Args {
		c.compileExpr(arg, false)
	}
	if !prim.Effectful && len(prim.Args) == prim.Arity {
		// Saturated non-effectful: invoke directly.
		nameIdx := c.addString(prim.Name)
		c.emitU16U8(OpPrim, nameIdx, uint8(prim.Arity))
	} else if prim.Effectful && len(prim.Args) == prim.Arity {
		// Saturated effectful: construct deferred PrimVal in one step.
		nameIdx := c.addString(prim.Name)
		c.emitU16U8(OpEffectPrim, nameIdx, uint8(prim.Arity))
	} else {
		// Genuinely partial: partial-application path.
		stub := &eval.PrimVal{
			Name: prim.Name, Arity: prim.Arity,
			Effectful: prim.Effectful, S: prim.S,
		}
		idx := c.addConstant(stub)
		c.emitU16(OpPrimPartial, idx)
		for range prim.Args {
			c.emit(OpApply)
		}
	}
}
func (c *Compiler) compileRecordLit(rec *ir.RecordLit) {
	c.addSpan(rec.S)
	if len(rec.Fields) == 0 {
		c.emit(OpConstUnit)
		return
	}
	labels := make([]string, len(rec.Fields))
	for i, f := range rec.Fields {
		labels[i] = f.Label
		c.compileExpr(f.Value, false)
	}
	descIdx := c.addRecordDesc(RecordDesc{Labels: labels})
	c.emitU16(OpRecord, descIdx)
}
func (c *Compiler) compileRecordProj(proj *ir.RecordProj) {
	c.addSpan(proj.S)
	c.compileExpr(proj.Record, false)
	labelIdx := c.addString(proj.Label)
	c.emitU16(OpRecordProj, labelIdx)
}
func (c *Compiler) compileRecordUpdate(upd *ir.RecordUpdate) {
	c.addSpan(upd.S)
	c.compileExpr(upd.Record, false)
	labels := make([]string, len(upd.Updates))
	for i, f := range upd.Updates {
		labels[i] = f.Label
		c.compileExpr(f.Value, false)
	}
	descIdx := c.addRecordDesc(RecordDesc{Labels: labels})
	c.emitU16(OpRecordUpdate, descIdx)
}

// --- child proto compilation ---

// compileChildProto compiles a nested function or thunk body.
// Capture resolution runs on the current (parent) frame before pushing the child.
