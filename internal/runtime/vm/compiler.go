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
// Compilation uses a scope-stack model: a single Compiler holds a stack of
// compilation frames. Each frame accumulates bytecode, constants, and
// metadata for one Proto. enterFrame pushes a new frame; leaveFrame pops
// the frame and transfers its slices directly into a Proto (zero copy).
// Child frames (for nested lambdas/thunks) are compiled depth-first and
// fully finalized before the parent frame resumes.
type Compiler struct {
	globalSlots map[string]int
	source      *span.Source
	frames      []frame
}

// frame accumulates bytecode and metadata for a single Proto.
// All slice fields are transferred directly to the Proto in leaveFrame —
// no copy is needed because the frame is popped and its slices are not
// reused.
type frame struct {
	code        []byte
	constants   []eval.Value
	strings     []string
	protos      []*Proto
	matchDescs  []MatchDesc
	recordDescs []RecordDesc
	mergeDescs  []MergeDesc
	spans       []SpanEntry
	locals      []localEntry
	numLocals   int
	captures    []int
	bindNames   []BindInfo
	isThunk     bool
	params      []string
	fixSelfSlot int
}

// localEntry tracks a named local variable.
type localEntry struct {
	name string
	slot int
}

// NewCompiler creates a Compiler with the given global slot mapping.
func NewCompiler(globalSlots map[string]int, source *span.Source) *Compiler {
	return &Compiler{globalSlots: globalSlots, source: source}
}

// SetSource updates the source context for error attribution.
func (c *Compiler) SetSource(s *span.Source) {
	c.source = s
}

// top returns a pointer to the current (topmost) compilation frame.
func (c *Compiler) top() *frame {
	return &c.frames[len(c.frames)-1]
}

// enterFrame pushes a new zero-value compilation frame onto the stack.
func (c *Compiler) enterFrame() {
	c.frames = append(c.frames, frame{fixSelfSlot: -1})
}

// leaveFrame pops the top frame and transfers its state into a Proto.
// All slice fields are moved directly — no copy.
func (c *Compiler) leaveFrame() *Proto {
	f := c.top()
	p := &Proto{
		Code:        f.code,
		Constants:   f.constants,
		Strings:     f.strings,
		Protos:      f.protos,
		NumLocals:   f.numLocals,
		Captures:    f.captures,
		IsThunk:     f.isThunk,
		Params:      f.params,
		FixSelfSlot: f.fixSelfSlot,
		MatchDescs:  f.matchDescs,
		RecordDescs: f.recordDescs,
		MergeDescs:  f.mergeDescs,
		Spans:       f.spans,
		Source:      c.source,
		BindNames:   f.bindNames,
	}
	c.frames = c.frames[:len(c.frames)-1]
	return p
}

// CompileExpr compiles a top-level Core IR expression into a Proto.
func (c *Compiler) CompileExpr(expr ir.Core) *Proto {
	c.enterFrame()
	c.compileExpr(expr, false)
	c.emit(OpForceEffectful)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// CompileBinding compiles a top-level binding's expression into a Proto.
func (c *Compiler) CompileBinding(b ir.Binding) *Proto {
	c.enterFrame()
	c.compileExpr(b.Expr, false)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// --- bytecode emission ---

func (c *Compiler) emitStep(expr ir.Core) {
	kind := irNodeKind(expr)
	idx := c.addString(kind)
	c.emitU16(OpStep, idx)
}

// irNodeKind returns a short human-readable name for the Core IR node type.
func irNodeKind(n ir.Core) string {
	switch n.(type) {
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

func (c *Compiler) emit(op Opcode) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op))
	return pos
}

func (c *Compiler) emitU16(op Opcode, operand uint16) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), 0, 0)
	EncodeU16(f.code[pos+1:], operand)
	return pos
}

func (c *Compiler) emitU16U8(op Opcode, a uint16, b uint8) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), 0, 0, b)
	EncodeU16(f.code[pos+1:], a)
	return pos
}

func (c *Compiler) emitU16U16(op Opcode, a, b uint16) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), 0, 0, 0, 0)
	EncodeU16(f.code[pos+1:], a)
	EncodeU16(f.code[pos+3:], b)
	return pos
}

func (c *Compiler) emitJump(op Opcode) int {
	f := c.top()
	pos := len(f.code)
	f.code = append(f.code, byte(op), 0, 0)
	return pos
}

func (c *Compiler) patchJumpTo(pos int) {
	f := c.top()
	offset := int16(len(f.code) - pos - 3)
	EncodeU16(f.code[pos+1:], uint16(offset))
}

// --- constant / string pool ---

const maxPoolSize = 1<<16 - 1

func (c *Compiler) addConstant(v eval.Value) uint16 {
	f := c.top()
	for i, existing := range f.constants {
		if existing == v {
			return uint16(i)
		}
	}
	if len(f.constants) >= maxPoolSize {
		panic("vm/compiler: constant pool overflow (max 65535)")
	}
	idx := len(f.constants)
	f.constants = append(f.constants, v)
	return uint16(idx)
}

func (c *Compiler) addString(s string) uint16 {
	f := c.top()
	for i, existing := range f.strings {
		if existing == s {
			return uint16(i)
		}
	}
	if len(f.strings) >= maxPoolSize {
		panic("vm/compiler: string pool overflow (max 65535)")
	}
	idx := len(f.strings)
	f.strings = append(f.strings, s)
	return uint16(idx)
}

func (c *Compiler) addMatchDesc(md MatchDesc) uint16 {
	f := c.top()
	if len(f.matchDescs) >= maxPoolSize {
		panic("vm/compiler: match descriptor pool overflow")
	}
	idx := len(f.matchDescs)
	f.matchDescs = append(f.matchDescs, md)
	return uint16(idx)
}

func (c *Compiler) addRecordDesc(rd RecordDesc) uint16 {
	f := c.top()
	if len(f.recordDescs) >= maxPoolSize {
		panic("vm/compiler: record descriptor pool overflow")
	}
	idx := len(f.recordDescs)
	f.recordDescs = append(f.recordDescs, rd)
	return uint16(idx)
}

func (c *Compiler) addMergeDesc(md MergeDesc) uint16 {
	f := c.top()
	if len(f.mergeDescs) >= maxPoolSize {
		panic("vm/compiler: merge descriptor pool overflow")
	}
	idx := len(f.mergeDescs)
	f.mergeDescs = append(f.mergeDescs, md)
	return uint16(idx)
}

// --- local variable management ---

func (c *Compiler) allocLocal(name string) int {
	f := c.top()
	slot := f.numLocals
	f.numLocals++
	f.locals = append(f.locals, localEntry{name: name, slot: slot})
	return slot
}

func (c *Compiler) resolveLocal(name string) (int, bool) {
	f := c.top()
	for i := len(f.locals) - 1; i >= 0; i-- {
		if f.locals[i].name == name {
			return f.locals[i].slot, true
		}
	}
	return -1, false
}

func (c *Compiler) popLocals(count int) {
	c.top().locals = c.top().locals[:count]
}

func (c *Compiler) addSpan(s span.Span) {
	if s.Start == 0 && s.End == 0 {
		return
	}
	f := c.top()
	f.spans = append(f.spans, SpanEntry{Offset: len(f.code), Span: s})
}

func (c *Compiler) addProto(p *Proto) uint16 {
	f := c.top()
	if len(f.protos) >= maxPoolSize {
		panic("vm/compiler: proto pool overflow")
	}
	idx := len(f.protos)
	f.protos = append(f.protos, p)
	return uint16(idx)
}

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
	child := c.compileChildProto(lam.Param, lam.Body, false, lam.FV, lam.FVIndices)
	protoIdx := c.addProto(child)
	c.emitU16(OpClosure, protoIdx)
}

func (c *Compiler) compileApp(app *ir.App, tail bool) {
	c.addSpan(app.S)
	c.compileExpr(app.Fun, false)
	c.compileExpr(app.Arg, false)
	if tail {
		c.emit(OpTailApply)
	} else {
		c.emit(OpApply)
	}
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
		child := c.compileFixProto(fix.Name, b.Param, b.Body, false, b.FV, b.FVIndices)
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

func (c *Compiler) compileMergeChildProto(body ir.Core, fv []string, fvIndices []int) *Proto {
	capturedNames, captureSlots := c.resolveCapturesFiltered(fv, fvIndices)
	c.enterFrame()
	f := c.top()
	f.isThunk = true
	f.captures = captureSlots
	for _, name := range capturedNames {
		c.allocLocal(name)
	}
	c.compileExpr(body, false)
	c.emit(OpForceEffectful)
	c.emit(OpReturn)
	return c.leaveFrame()
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
func (c *Compiler) compileChildProto(paramName string, body ir.Core, isThunk bool, fv []string, fvIndices []int) *Proto {
	capturedNames, captureSlots := c.resolveCapturesFiltered(fv, fvIndices)
	c.enterFrame()
	f := c.top()
	f.isThunk = isThunk
	if paramName != "" {
		f.params = []string{paramName}
	}
	f.captures = captureSlots
	for _, name := range capturedNames {
		c.allocLocal(name)
	}
	if paramName != "" {
		c.allocLocal(paramName)
	}
	c.compileExpr(body, true)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// compileFixProto compiles a Fix body (self-referential closure or thunk).
func (c *Compiler) compileFixProto(selfName, paramName string, body ir.Core, isThunk bool, fv []string, fvIndices []int) *Proto {
	capturedNames, captureSlots := c.resolveCapturesFiltered(fv, fvIndices)
	c.enterFrame()
	f := c.top()
	f.isThunk = isThunk
	if paramName != "" {
		f.params = []string{paramName}
	}
	f.captures = captureSlots
	for _, name := range capturedNames {
		c.allocLocal(name)
	}
	selfSlot := c.allocLocal(selfName)
	f.fixSelfSlot = selfSlot
	if paramName != "" {
		c.allocLocal(paramName)
	}
	c.compileExpr(body, true)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// resolveCapturesFiltered resolves free variable names to parent-frame local slots,
// filtering out globals (which the child accesses via LOAD_GLOBAL).
func (c *Compiler) resolveCapturesFiltered(fv []string, fvIndices []int) ([]string, []int) {
	if len(fv) == 0 {
		return nil, nil
	}
	f := c.top() // current frame = parent of the about-to-be-created child
	var names []string
	var slots []int
	for i, name := range fv {
		if slot, ok := resolveLocalInFrame(f, name); ok {
			names = append(names, name)
			slots = append(slots, slot)
		} else if fvIndices != nil && i < len(fvIndices) && fvIndices[i] >= 0 {
			names = append(names, name)
			slots = append(slots, fvIndices[i])
		}
	}
	return names, slots
}

// resolveLocalInFrame searches a specific frame's locals by name.
func resolveLocalInFrame(f *frame, name string) (int, bool) {
	for i := len(f.locals) - 1; i >= 0; i-- {
		if f.locals[i].name == name {
			return f.locals[i].slot, true
		}
	}
	return -1, false
}
