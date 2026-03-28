package vm

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// Compiler translates Core IR into bytecode Protos.
type Compiler struct {
	globalSlots map[string]int
	source      *span.Source
}

// NewCompiler creates a Compiler with the given global slot mapping.
func NewCompiler(globalSlots map[string]int, source *span.Source) *Compiler {
	return &Compiler{globalSlots: globalSlots, source: source}
}

// CompileExpr compiles a top-level Core IR expression into a Proto.
// The resulting Proto has no captures and no parameter (it is a "script" body).
func (c *Compiler) CompileExpr(expr ir.Core) *Proto {
	e := newEmitter(c, nil)
	e.compileExpr(expr, true)
	e.emit(OpReturn)
	return e.finalize(c.source)
}

// CompileBinding compiles a top-level binding's expression into a Proto.
func (c *Compiler) CompileBinding(b ir.Binding) *Proto {
	return c.CompileExpr(b.Expr)
}

// --- emitter: bytecode generation state for a single Proto ---

// emitter accumulates bytecode, constants, and metadata for one Proto.
type emitter struct {
	compiler *Compiler
	parent   *emitter // enclosing emitter (for captures)

	code       []byte
	constants  []eval.Value
	strings    []string
	protos     []*Proto
	matchDescs []MatchDesc
	recordDescs []RecordDesc
	spans      []SpanEntry

	// Local variable slots: maps de Bruijn name to slot index.
	// Built up during compilation; numLocals tracks the high-water mark.
	locals    []localEntry
	numLocals int

	// captures: indices in the enclosing scope's locals that this proto captures.
	captures []int

	// Is this a thunk body?
	isThunk bool
	// Parameter name (for closures).
	paramName string
	// Fix self-reference slot.
	fixSelfSlot int
}

// localEntry tracks a named local variable.
type localEntry struct {
	name  string
	slot  int
	depth int // scope depth for shadowing
}

func newEmitter(c *Compiler, parent *emitter) *emitter {
	return &emitter{
		compiler:    c,
		parent:      parent,
		fixSelfSlot: -1,
	}
}

// --- bytecode emission ---

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

// patchJumpTo patches a jump instruction at pos to target the current code position.
func (e *emitter) patchJumpTo(pos int) {
	offset := int16(len(e.code) - pos - 3) // relative to instruction end
	EncodeU16(e.code[pos+1:], uint16(offset))
}

// patchU16 overwrites the u16 at code[pos+1..pos+2].
func (e *emitter) patchU16(pos int, val uint16) {
	EncodeU16(e.code[pos+1:], val)
}

// --- constant / string pool ---

func (e *emitter) addConstant(v eval.Value) uint16 {
	for i, c := range e.constants {
		if c == v {
			return uint16(i)
		}
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
	idx := len(e.strings)
	e.strings = append(e.strings, s)
	return uint16(idx)
}

func (e *emitter) addMatchDesc(md MatchDesc) uint16 {
	idx := len(e.matchDescs)
	e.matchDescs = append(e.matchDescs, md)
	return uint16(idx)
}

func (e *emitter) addRecordDesc(rd RecordDesc) uint16 {
	idx := len(e.recordDescs)
	e.recordDescs = append(e.recordDescs, rd)
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

// resolveCapture looks up a variable in enclosing scopes and records it
// as a capture. Returns (capture index, true) if found.
func (e *emitter) resolveCapture(name string) (int, bool) {
	if e.parent == nil {
		return -1, false
	}
	// Check parent's locals.
	if slot, ok := e.parent.resolveLocal(name); ok {
		return e.addCapture(slot), true
	}
	// Check parent's captures (transitive capture).
	if capIdx, ok := e.parent.resolveCapture(name); ok {
		// Parent has a capture at capIdx — we need to capture from
		// parent's local representation of that capture.
		// Since captures are laid out before params in the local array,
		// parent's capture at capIdx is at local slot capIdx.
		return e.addCapture(capIdx), true
	}
	return -1, false
}

// addCapture adds a capture (if not already present) and returns its index.
func (e *emitter) addCapture(parentSlot int) int {
	for i, c := range e.captures {
		if c == parentSlot {
			return i
		}
	}
	idx := len(e.captures)
	e.captures = append(e.captures, parentSlot)
	return idx
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
	e.emit(OpStep)

	switch node := expr.(type) {
	case *ir.Var:
		e.compileVar(node)

	case *ir.Lit:
		e.compileLit(node)

	case *ir.Lam:
		e.compileLam(node)

	case *ir.App:
		e.compileApp(node, tail)

	case *ir.TyApp:
		// Type application is erased at runtime.
		e.compileExpr(node.Expr, tail)
		return // skip the STEP we already emitted for this node

	case *ir.TyLam:
		// Type abstraction is erased at runtime.
		e.compileExpr(node.Body, tail)
		return

	case *ir.Con:
		e.compileCon(node)

	case *ir.Case:
		e.compileCase(node, tail)

	case *ir.Fix:
		e.compileFix(node)

	case *ir.Pure:
		// Pure is identity in CBV.
		e.compileExpr(node.Expr, tail)
		return

	case *ir.Bind:
		e.compileBind(node, tail)

	case *ir.Thunk:
		e.compileThunk(node)

	case *ir.Force:
		e.compileForce(node, tail)

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
		// Local variable. The IR uses de Bruijn indices (0 = innermost).
		// Our emitter tracks locals by name, which were assigned slots
		// during compilation. Look up the name.
		if slot, ok := e.resolveLocal(v.Name); ok {
			e.emitU16(OpLoadLocal, uint16(slot))
			return
		}
		// Try as a capture from enclosing scope.
		if capIdx, ok := e.resolveCapture(v.Name); ok {
			e.emitU16(OpLoadLocal, uint16(capIdx))
			return
		}
		// Fallback: shouldn't happen for well-formed IR with assigned indices.
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
		// Global not in slot map — this can happen for un-indexed globals.
		// Encode the key as a constant string and use LOAD_GLOBAL with a
		// sentinel or a separate instruction. For now, fall back to storing
		// the key string and loading by name at runtime.
		// We'll store the key in constants as a HostVal(string).
		idx := e.addConstant(&eval.HostVal{Inner: key})
		e.emitU16(OpConst, idx)
		// The VM will need to handle this case — but slot-based lookup is
		// the expected fast path. Globals not in the slot map are rare.
		// TODO: add OpLoadGlobalNamed if needed.
	}
}

func (e *emitter) compileLit(lit *ir.Lit) {
	e.addSpan(lit.S)
	idx := e.addConstant(&eval.HostVal{Inner: lit.Value})
	e.emitU16(OpConst, idx)
}

func (e *emitter) compileLam(lam *ir.Lam) {
	e.addSpan(lam.S)
	child := e.compileChildProto(lam.Param, lam.Body, false, lam.S)
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
		child := e.compileFixProto(fix.Name, b.Param, b.Body, false, fix.S)
		protoIdx := e.addProto(child)
		e.emitU16(OpFixClosure, protoIdx)
	case *ir.Thunk:
		child := e.compileFixProto(fix.Name, "", b.Comp, true, fix.S)
		protoIdx := e.addProto(child)
		e.emitU16(OpFixThunk, protoIdx)
	default:
		panic(fmt.Sprintf("vm/compiler: Fix body is %T, expected Lam or Thunk", body))
	}
}

func (e *emitter) compileBind(bind *ir.Bind, tail bool) {
	e.addSpan(bind.S)
	// Compile the computation.
	e.compileExpr(bind.Comp, false)
	// Bind: force effectful, store result in local slot.
	slot := e.allocLocal(bind.Var)
	e.emitU16(OpBind, uint16(slot))
	// Compile body (may be tail position).
	e.compileExpr(bind.Body, tail)
}

func (e *emitter) compileThunk(thunk *ir.Thunk) {
	e.addSpan(thunk.S)
	child := e.compileChildProto("", thunk.Comp, true, thunk.S)
	protoIdx := e.addProto(child)
	e.emitU16(OpThunk, protoIdx)
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
	if len(prim.Args) == 0 && prim.Effectful {
		// Unapplied effectful primitive — push as a stub.
		stub := &eval.PrimVal{
			Name: prim.Name, Arity: prim.Arity,
			Effectful: prim.Effectful, S: prim.S,
		}
		idx := e.addConstant(stub)
		e.emitU16(OpPrimPartial, idx)
		return
	}
	if len(prim.Args) == 0 {
		// Unapplied non-effectful — push as stub for partial application.
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
func (e *emitter) compileChildProto(paramName string, body ir.Core, isThunk bool, s span.Span) *Proto {
	child := newEmitter(e.compiler, e)
	child.isThunk = isThunk
	child.paramName = paramName

	// Captures are resolved lazily during body compilation.
	// After body compilation, child.captures holds the list of parent slots.

	// Reserve slot 0..N-1 for captures (filled at runtime by CLOSURE/THUNK).
	// We don't know how many captures yet — they are added during compileExpr.
	// So we compile the body first, then fix up local slot indices.

	// If this is a closure (not thunk), allocate a slot for the parameter.
	if paramName != "" {
		child.allocLocal(paramName)
	}

	child.compileExpr(body, true)
	child.emit(OpReturn)

	return child.finalize(e.compiler.source)
}

// compileFixProto compiles a Fix body (self-referential closure or thunk).
func (e *emitter) compileFixProto(selfName, paramName string, body ir.Core, isThunk bool, s span.Span) *Proto {
	child := newEmitter(e.compiler, e)
	child.isThunk = isThunk
	child.paramName = paramName

	// Self-reference gets a slot.
	selfSlot := child.allocLocal(selfName)
	child.fixSelfSlot = selfSlot

	// Parameter slot (for closures).
	if paramName != "" {
		child.allocLocal(paramName)
	}

	child.compileExpr(body, true)
	child.emit(OpReturn)

	return child.finalize(e.compiler.source)
}

func (e *emitter) addProto(p *Proto) uint16 {
	idx := len(e.protos)
	e.protos = append(e.protos, p)
	return uint16(idx)
}

// finalize produces a Proto from the accumulated state.
func (e *emitter) finalize(source *span.Source) *Proto {
	return &Proto{
		Code:        e.code,
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
		Spans:       e.spans,
		Source:      source,
	}
}
