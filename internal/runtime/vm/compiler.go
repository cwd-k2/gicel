package vm

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Compiler translates Core IR into bytecode Protos.
//
// codeBuf and spanBuf are scratchpad buffers reused across emitter lifetimes
// (goja pattern). Since child emitters are compiled sequentially, a single
// buffer per slice type suffices. newEmitter borrows the buffer; finalize
// copies the used portion into a right-sized slice and returns the backing
// array to the Compiler.
type Compiler struct {
	globalSlots map[string]int
	source      *span.Source
	codeBuf     []byte
	spanBuf     []SpanEntry
}

// NewCompiler creates a Compiler with the given global slot mapping.
func NewCompiler(globalSlots map[string]int, source *span.Source) *Compiler {
	return &Compiler{globalSlots: globalSlots, source: source}
}

// SetSource updates the source context for error attribution.
func (c *Compiler) SetSource(s *span.Source) {
	c.source = s
}

// CompileExpr compiles a top-level Core IR expression into a Proto.
// The resulting Proto has no captures and no parameter (it is a "script" body).
func (c *Compiler) CompileExpr(expr ir.Core) *Proto {
	e := newEmitter(c, nil)
	e.compileExpr(expr, false) // not tail: we add ForceEffectful + Return ourselves
	e.emit(OpForceEffectful)
	e.emit(OpReturn)
	return e.finalize(c.source)
}

// CompileBinding compiles a top-level binding's expression into a Proto.
// Unlike CompileExpr, this does NOT add ForceEffectful at the end —
// binding values should not be forced (they may be effectful PrimVals
// or closures that must remain as values, not be invoked).
func (c *Compiler) CompileBinding(b ir.Binding) *Proto {
	e := newEmitter(c, nil)
	e.compileExpr(b.Expr, false)
	e.emit(OpReturn)
	return e.finalize(c.source)
}

// --- emitter: bytecode generation state for a single Proto ---

// emitter accumulates bytecode, constants, and metadata for one Proto.
type emitter struct {
	compiler *Compiler
	parent   *emitter // enclosing emitter (for captures)

	code        []byte
	constants   []eval.Value
	strings     []string
	protos      []*Proto
	matchDescs  []MatchDesc
	recordDescs []RecordDesc
	mergeDescs  []MergeDesc
	spans       []SpanEntry

	// Local variable slots: maps de Bruijn name to slot index.
	// Built up during compilation; numLocals tracks the high-water mark.
	locals    []localEntry
	numLocals int

	// captures: indices in the enclosing scope's locals that this proto captures.
	captures []int

	// bindNames: records variable names for OpBind instructions (observer support).
	bindNames []BindInfo

	// Is this a thunk body?
	isThunk bool
	// Parameter name (for closures).
	paramName string
	// Fix self-reference slot.
	fixSelfSlot int
}

// localEntry tracks a named local variable.
type localEntry struct {
	name string
	slot int
}

func newEmitter(c *Compiler, parent *emitter) *emitter {
	// Borrow scratchpad buffers from the Compiler. Since child emitters
	// are compiled sequentially (depth-first), the buffer is always
	// available when a new emitter is created at the same nesting level.
	code := c.codeBuf
	c.codeBuf = nil
	if code == nil {
		code = make([]byte, 0, 64)
	} else {
		code = code[:0]
	}
	spans := c.spanBuf
	c.spanBuf = nil
	if spans == nil {
		spans = make([]SpanEntry, 0, 8)
	} else {
		spans = spans[:0]
	}
	return &emitter{
		compiler:    c,
		parent:      parent,
		fixSelfSlot: -1,
		code:        code,
		spans:       spans,
	}
}

// --- bytecode emission ---

// emitStep emits an OpStep with the node kind from the IR expression.
func (e *emitter) emitStep(expr ir.Core) {
	kind := irNodeKind(expr)
	idx := e.addString(kind)
	e.emitU16(OpStep, idx)
}

// irNodeKind returns a short human-readable name for the Core IR node type.
func irNodeKind(c ir.Core) string {
	switch c.(type) {
	case *ir.Var:
		return "Var"
	case *ir.Lit:
		return "Lit"
	case *ir.Lam:
		return "Lam"
	case *ir.App:
		return "App"
	case *ir.Con:
		return "Con"
	case *ir.Case:
		return "Case"
	case *ir.Fix:
		return "Fix"
	case *ir.Bind:
		return "Bind"
	case *ir.Thunk:
		return "Thunk"
	case *ir.Force:
		return "Force"
	case *ir.Merge:
		return "Merge"
	case *ir.PrimOp:
		return "PrimOp"
	case *ir.RecordLit:
		return types.TyConRecord
	case *ir.RecordProj:
		return "RecordProj"
	case *ir.RecordUpdate:
		return "RecordUpdate"
	default:
		return "?"
	}
}

func (e *emitter) emit(op Opcode) int {
	pos := len(e.code)
	e.code = append(e.code, byte(op))
	return pos
}

func (e *emitter) emitU16(op Opcode, operand uint16) int {
	pos := len(e.code)
	e.code = append(e.code, byte(op), 0, 0)
	EncodeU16(e.code[pos+1:], operand)
	return pos
}

func (e *emitter) emitU16U8(op Opcode, a uint16, b uint8) int {
	pos := len(e.code)
	e.code = append(e.code, byte(op), 0, 0, b)
	EncodeU16(e.code[pos+1:], a)
	return pos
}

func (e *emitter) emitU16U16(op Opcode, a, b uint16) int {
	pos := len(e.code)
	e.code = append(e.code, byte(op), 0, 0, 0, 0)
	EncodeU16(e.code[pos+1:], a)
	EncodeU16(e.code[pos+3:], b)
	return pos
}

// emitJump emits a jump with a placeholder offset, returns the patch position.
func (e *emitter) emitJump(op Opcode) int {
	pos := len(e.code)
	e.code = append(e.code, byte(op), 0, 0)
	return pos
}

// patchJumpTo patches a JUMP instruction at pos to target the current code position.
// Uses relative offset (signed i16 from end of instruction).
func (e *emitter) patchJumpTo(pos int) {
	offset := int16(len(e.code) - pos - 3) // relative to instruction end
	EncodeU16(e.code[pos+1:], uint16(offset))
}

// --- constant / string pool ---

const maxPoolSize = 1<<16 - 1

func (e *emitter) addConstant(v eval.Value) uint16 {
	for i, c := range e.constants {
		if c == v {
			return uint16(i)
		}
	}
	if len(e.constants) >= maxPoolSize {
		panic("vm/compiler: constant pool overflow (max 65535)")
	}
	idx := len(e.constants)
	e.constants = append(e.constants, v)
	return uint16(idx)
}

func (e *emitter) addString(s string) uint16 {
	for i, existing := range e.strings {
		if existing == s {
			return uint16(i)
		}
	}
	if len(e.strings) >= maxPoolSize {
		panic("vm/compiler: string pool overflow (max 65535)")
	}
	idx := len(e.strings)
	e.strings = append(e.strings, s)
	return uint16(idx)
}

func (e *emitter) addMatchDesc(md MatchDesc) uint16 {
	if len(e.matchDescs) >= maxPoolSize {
		panic("vm/compiler: match descriptor pool overflow")
	}
	idx := len(e.matchDescs)
	e.matchDescs = append(e.matchDescs, md)
	return uint16(idx)
}

func (e *emitter) addRecordDesc(rd RecordDesc) uint16 {
	if len(e.recordDescs) >= maxPoolSize {
		panic("vm/compiler: record descriptor pool overflow")
	}
	idx := len(e.recordDescs)
	e.recordDescs = append(e.recordDescs, rd)
	return uint16(idx)
}

func (e *emitter) addMergeDesc(md MergeDesc) uint16 {
	if len(e.mergeDescs) >= maxPoolSize {
		panic("vm/compiler: merge descriptor pool overflow")
	}
	idx := len(e.mergeDescs)
	e.mergeDescs = append(e.mergeDescs, md)
	return uint16(idx)
}

// --- local variable management ---

// allocLocal reserves the next slot and records the name.
func (e *emitter) allocLocal(name string) int {
	slot := e.numLocals
	e.numLocals++
	e.locals = append(e.locals, localEntry{name: name, slot: slot})
	return slot
}

// resolveLocal looks up a local variable by name (innermost first).
// Returns (slot, true) if found, (-1, false) otherwise.
func (e *emitter) resolveLocal(name string) (int, bool) {
	for i := len(e.locals) - 1; i >= 0; i-- {
		if e.locals[i].name == name {
			return e.locals[i].slot, true
		}
	}
	return -1, false
}

// popLocals removes locals added since the given count.
func (e *emitter) popLocals(count int) {
	e.locals = e.locals[:count]
}

// addSpan records a source span for the current bytecode offset.
func (e *emitter) addSpan(s span.Span) {
	if s.Start == 0 && s.End == 0 {
		return
	}
	e.spans = append(e.spans, SpanEntry{Offset: len(e.code), Span: s})
}

// --- compilation ---

// compileExpr compiles a Core IR expression. tail indicates whether the
// expression is in tail position.
func (e *emitter) compileExpr(expr ir.Core, tail bool) {
	// Type-erased nodes produce no runtime instructions — compile the
	// inner expression directly without emitting a step.
	switch node := expr.(type) {
	case *ir.TyApp:
		e.compileExpr(node.Expr, tail)
		return
	case *ir.TyLam:
		e.compileExpr(node.Body, tail)
		return
	case *ir.Pure:
		// Pure is identity in CBV.
		e.compileExpr(node.Expr, tail)
		return
	}

	e.emitStep(expr)

	switch node := expr.(type) {
	case *ir.Var:
		e.compileVar(node)

	case *ir.Lit:
		e.compileLit(node)

	case *ir.Lam:
		e.compileLam(node)

	case *ir.App:
		e.compileApp(node, tail)

	case *ir.Con:
		e.compileCon(node)

	case *ir.Case:
		e.compileCase(node, tail)

	case *ir.Fix:
		e.compileFix(node)

	case *ir.Bind:
		e.compileBind(node, tail)

	case *ir.Thunk:
		e.compileThunk(node)

	case *ir.Force:
		e.compileForce(node, tail)

	case *ir.Merge:
		e.compileMerge(node)

	case *ir.PrimOp:
		e.compilePrimOp(node)

	case *ir.RecordLit:
		e.compileRecordLit(node)

	case *ir.RecordProj:
		e.compileRecordProj(node)

	case *ir.RecordUpdate:
		e.compileRecordUpdate(node)

	default:
		panic(fmt.Sprintf("vm/compiler: unhandled Core node %T", expr))
	}
}

func (e *emitter) compileVar(v *ir.Var) {
	e.addSpan(v.S)
	if v.Index >= 0 {
		// Local variable — resolve by name. Captures are pre-allocated as
		// local slots in compileChildProto, so resolveLocal finds them.
		if slot, ok := e.resolveLocal(v.Name); ok {
			e.emitU16(OpLoadLocal, uint16(slot))
			return
		}
		// Fallback: shouldn't happen for well-formed IR.
		panic(fmt.Sprintf("vm/compiler: unresolved local variable %q (index %d)", v.Name, v.Index))
	}
	// Global variable.
	key := v.Key
	if key == "" {
		key = ir.VarKey(v)
	}
	if slot, ok := e.compiler.globalSlots[key]; ok {
		e.emitU16(OpLoadGlobal, uint16(slot))
	} else {
		panic(fmt.Sprintf("vm/compiler: global %q (key %q) not in globalSlots", v.Name, key))
	}
}

func (e *emitter) compileLit(lit *ir.Lit) {
	e.addSpan(lit.S)
	idx := e.addConstant(&eval.HostVal{Inner: lit.Value})
	e.emitU16(OpConst, idx)
}

func (e *emitter) compileLam(lam *ir.Lam) {
	e.addSpan(lam.S)
	child := e.compileChildProto(lam.Param, lam.Body, false, lam.FV, lam.FVIndices)
	protoIdx := e.addProto(child)
	e.emitU16(OpClosure, protoIdx)
}

func (e *emitter) compileApp(app *ir.App, tail bool) {
	e.addSpan(app.S)
	// Compile function and argument (non-tail).
	e.compileExpr(app.Fun, false)
	e.compileExpr(app.Arg, false)
	if tail {
		e.emit(OpTailApply)
	} else {
		e.emit(OpApply)
	}
}

func (e *emitter) compileCon(con *ir.Con) {
	e.addSpan(con.S)
	// Evaluate all constructor arguments (left to right).
	for _, arg := range con.Args {
		e.compileExpr(arg, false)
	}
	nameIdx := e.addString(con.Name)
	e.emitU16U8(OpCon, nameIdx, uint8(len(con.Args)))
}

func (e *emitter) compileCase(cs *ir.Case, tail bool) {
	e.addSpan(cs.S)
	// Compile scrutinee.
	e.compileExpr(cs.Scrutinee, false)

	compilePatternMatch(e, cs, tail)
}

func (e *emitter) compileFix(fix *ir.Fix) {
	e.addSpan(fix.S)
	body := ir.PeelTyLam(fix.Body)
	switch b := body.(type) {
	case *ir.Lam:
		child := e.compileFixProto(fix.Name, b.Param, b.Body, false, b.FV, b.FVIndices)
		protoIdx := e.addProto(child)
		e.emitU16(OpFixClosure, protoIdx)
	case *ir.Thunk:
		child := e.compileFixProto(fix.Name, "", b.Comp, true, b.FV, b.FVIndices)
		protoIdx := e.addProto(child)
		e.emitU16(OpFixThunk, protoIdx)
	default:
		// Fix body is neither Lam nor Thunk. Emit code that raises a runtime
		// error, matching the tree-walker's evalFix behavior.
		msg := fmt.Sprintf(
			"fix binding %s requires a lambda or thunk body (got %T); "+
				"in CBV evaluation, fix creates a self-referential closure "+
				"and cannot produce data constructor values directly — "+
				"wrap the body in a lambda or use thunk/force",
			fix.Name, body)
		idx := e.addConstant(&eval.HostVal{Inner: msg})
		e.emitU16(OpConst, idx)
		e.emit(OpRaise)
	}
}

func (e *emitter) compileBind(bind *ir.Bind, tail bool) {
	e.addSpan(bind.S)
	e.compileExpr(bind.Comp, false)
	slot := e.allocLocal(bind.Var)
	e.emitU16(OpBind, uint16(slot))
	if !bind.Discard && !bind.Generated {
		e.bindNames = append(e.bindNames, BindInfo{Slot: slot, Name: bind.Var})
	}
	// Body is in tail position if the Bind itself is. ForceEffectful for
	// the final result is handled by CompileExpr's trailing OpForceEffectful.
	e.compileExpr(bind.Body, tail)
}

func (e *emitter) compileThunk(thunk *ir.Thunk) {
	e.addSpan(thunk.S)
	child := e.compileChildProto("", thunk.Comp, true, thunk.FV, thunk.FVIndices)
	protoIdx := e.addProto(child)
	e.emitU16(OpThunk, protoIdx)
}

func (e *emitter) compileMerge(merge *ir.Merge) {
	e.addSpan(merge.S)
	// Compile Left and Right as effectful thunk protos.
	// Unlike regular thunks, merge children add ForceEffectful before Return
	// to fully execute the computation (not just return a deferred PrimVal).
	leftProto := e.compileMergeChildProto(merge.Left, merge.LeftFV, merge.LeftFVIdx)
	leftIdx := e.addProto(leftProto)
	e.emitU16(OpThunk, leftIdx)

	rightProto := e.compileMergeChildProto(merge.Right, merge.RightFV, merge.RightFVIdx)
	rightIdx := e.addProto(rightProto)
	e.emitU16(OpThunk, rightIdx)

	descIdx := e.addMergeDesc(MergeDesc{
		LeftLabels:  merge.LeftLabels,
		RightLabels: merge.RightLabels,
	})
	e.emitU16(OpMerge, descIdx)
}

// compileMergeChildProto compiles a merge child computation as an effectful thunk.
// Adds ForceEffectful before Return to fully execute the computation.
func (e *emitter) compileMergeChildProto(body ir.Core, fv []string, fvIndices []int) *Proto {
	child := newEmitter(e.compiler, e)
	child.isThunk = true

	capturedNames, captureSlots := e.resolveCapturesFiltered(fv, fvIndices)
	child.captures = captureSlots
	for _, name := range capturedNames {
		child.allocLocal(name)
	}

	child.compileExpr(body, false)
	child.emit(OpForceEffectful) // force the computation result
	child.emit(OpReturn)

	return child.finalize(e.compiler.source)
}

func (e *emitter) compileForce(force *ir.Force, tail bool) {
	e.addSpan(force.S)
	e.compileExpr(force.Expr, false)
	if tail {
		e.emit(OpForceTail)
	} else {
		e.emit(OpForce)
	}
}

func (e *emitter) compilePrimOp(prim *ir.PrimOp) {
	e.addSpan(prim.S)
	if len(prim.Args) == 0 {
		if prim.Arity == 0 && !prim.Effectful {
			// 0-arity non-effectful: invoke immediately (matches tree-walker).
			nameIdx := e.addString(prim.Name)
			e.emitU16U8(OpPrim, nameIdx, 0)
			return
		}
		// Unapplied or effectful: push as PrimVal stub.
		stub := &eval.PrimVal{
			Name: prim.Name, Arity: prim.Arity,
			Effectful: prim.Effectful, S: prim.S,
		}
		idx := e.addConstant(stub)
		e.emitU16(OpPrimPartial, idx)
		return
	}
	// Has arguments. Compile all args.
	for _, arg := range prim.Args {
		e.compileExpr(arg, false)
	}
	if !prim.Effectful && len(prim.Args) == prim.Arity {
		// Fully-applied non-effectful: invoke immediately.
		nameIdx := e.addString(prim.Name)
		e.emitU16U8(OpPrim, nameIdx, uint8(prim.Arity))
	} else {
		// Partially applied or effectful: build PrimVal.
		stub := &eval.PrimVal{
			Name: prim.Name, Arity: prim.Arity,
			Effectful: prim.Effectful, S: prim.S,
		}
		idx := e.addConstant(stub)
		e.emitU16(OpPrimPartial, idx)
		// The compiled args are on the stack — the VM must apply them
		// to the PrimVal. We emit APPLY for each arg.
		for range prim.Args {
			e.emit(OpApply)
		}
	}
}

func (e *emitter) compileRecordLit(rec *ir.RecordLit) {
	e.addSpan(rec.S)
	if len(rec.Fields) == 0 {
		e.emit(OpConstUnit)
		return
	}
	labels := make([]string, len(rec.Fields))
	for i, f := range rec.Fields {
		labels[i] = f.Label
		e.compileExpr(f.Value, false)
	}
	descIdx := e.addRecordDesc(RecordDesc{Labels: labels})
	e.emitU16(OpRecord, descIdx)
}

func (e *emitter) compileRecordProj(proj *ir.RecordProj) {
	e.addSpan(proj.S)
	e.compileExpr(proj.Record, false)
	labelIdx := e.addString(proj.Label)
	e.emitU16(OpRecordProj, labelIdx)
}

func (e *emitter) compileRecordUpdate(upd *ir.RecordUpdate) {
	e.addSpan(upd.S)
	e.compileExpr(upd.Record, false)
	labels := make([]string, len(upd.Updates))
	for i, f := range upd.Updates {
		labels[i] = f.Label
		e.compileExpr(f.Value, false)
	}
	descIdx := e.addRecordDesc(RecordDesc{Labels: labels})
	e.emitU16(OpRecordUpdate, descIdx)
}

// --- child proto compilation ---

// compileChildProto compiles a nested function or thunk body.
// fv is the list of free variable names from the IR (Lam.FV or Thunk.FV).
// fvIndices is the list of de Bruijn indices in the enclosing env.
// Callers must ensure FV annotation has been run (ir.AnnotateFreeVars);
// fv==nil is accepted only for top-level lambdas with no captures.
func (e *emitter) compileChildProto(paramName string, body ir.Core, isThunk bool, fv []string, fvIndices []int) *Proto {
	child := newEmitter(e.compiler, e)
	child.isThunk = isThunk
	child.paramName = paramName

	// Phase 1: Resolve captures — only local (non-global) free variables.
	capturedNames, captureSlots := e.resolveCapturesFiltered(fv, fvIndices)
	child.captures = captureSlots
	for _, name := range capturedNames {
		child.allocLocal(name) // slot 0..N-1 for captures
	}

	// Phase 2: Reserve slot for parameter (closures only).
	if paramName != "" {
		child.allocLocal(paramName)
	}

	// Phase 3: Compile body.
	child.compileExpr(body, true)
	child.emit(OpReturn)

	return child.finalize(e.compiler.source)
}

// compileFixProto compiles a Fix body (self-referential closure or thunk).
func (e *emitter) compileFixProto(selfName, paramName string, body ir.Core, isThunk bool, fv []string, fvIndices []int) *Proto {
	child := newEmitter(e.compiler, e)
	child.isThunk = isThunk
	child.paramName = paramName

	// Captures first — only local (non-global) free variables.
	capturedNames, captureSlots := e.resolveCapturesFiltered(fv, fvIndices)
	child.captures = captureSlots
	for _, name := range capturedNames {
		child.allocLocal(name)
	}

	// Self-reference slot (comes after captures).
	selfSlot := child.allocLocal(selfName)
	child.fixSelfSlot = selfSlot

	// Parameter slot (closures only).
	if paramName != "" {
		child.allocLocal(paramName)
	}

	child.compileExpr(body, true)
	child.emit(OpReturn)

	return child.finalize(e.compiler.source)
}

// resolveCapturesFiltered resolves free variable names to parent local slots,
// filtering out globals (which the child accesses via LOAD_GLOBAL).
// Returns (capturedNames, parentSlots) in parallel.
func (e *emitter) resolveCapturesFiltered(fv []string, fvIndices []int) ([]string, []int) {
	if len(fv) == 0 {
		return nil, nil
	}
	var names []string
	var slots []int
	for i, name := range fv {
		if slot, ok := e.resolveLocal(name); ok {
			names = append(names, name)
			slots = append(slots, slot)
		} else if fvIndices != nil && i < len(fvIndices) && fvIndices[i] >= 0 {
			names = append(names, name)
			slots = append(slots, fvIndices[i])
		}
		// else: global variable — skip.
	}
	return names, slots
}

func (e *emitter) addProto(p *Proto) uint16 {
	if len(e.protos) >= maxPoolSize {
		panic("vm/compiler: proto pool overflow")
	}
	idx := len(e.protos)
	e.protos = append(e.protos, p)
	return uint16(idx)
}

// finalize produces a Proto from the accumulated state.
// The code and spans backing arrays are returned to the Compiler scratchpad
// (keeping the larger of current scratchpad vs. this emitter's buffer).
// The Proto receives right-sized copies.
func (e *emitter) finalize(source *span.Source) *Proto {
	// Copy used portions into right-sized slices for the Proto.
	var code []byte
	if len(e.code) > 0 {
		code = make([]byte, len(e.code))
		copy(code, e.code)
	}
	var spans []SpanEntry
	if len(e.spans) > 0 {
		spans = make([]SpanEntry, len(e.spans))
		copy(spans, e.spans)
	}
	// Return backing arrays to the Compiler scratchpad (keep the larger).
	if cap(e.code) > cap(e.compiler.codeBuf) {
		e.compiler.codeBuf = e.code
	}
	if cap(e.spans) > cap(e.compiler.spanBuf) {
		e.compiler.spanBuf = e.spans
	}
	return &Proto{
		Code:        code,
		Constants:   e.constants,
		Strings:     e.strings,
		Protos:      e.protos,
		NumLocals:   e.numLocals,
		Captures:    e.captures,
		IsThunk:     e.isThunk,
		ParamName:   e.paramName,
		FixSelfSlot: e.fixSelfSlot,
		MatchDescs:  e.matchDescs,
		RecordDescs: e.recordDescs,
		MergeDescs:  e.mergeDescs,
		Spans:       spans,
		Source:      source,
		BindNames:   e.bindNames,
	}
}
