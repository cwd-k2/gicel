// Compiler — Core IR to bytecode.
// Package layout:
//   compiler.go          — struct, frame lifecycle, pool/scope management, entry points
//   compiler_emit.go     — bytecode emission primitives
//   compiler_expr.go     — IR node compilation (compileExpr and per-node handlers)
//   compiler_closure.go  — child proto builders (lambda/thunk/fix/merge) and capture resolution
//   compiler_match.go    — pattern match compilation

package vm

import (
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
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
	globalSlots   map[string]int
	globalArities map[string]int // global name → known arity (-1 or absent: unknown)
	source        *span.Source
	frames        []frame
}

// frame accumulates bytecode and metadata for a single Proto.
// All slice fields are transferred directly to the Proto in leaveFrame —
// no copy is needed because the frame is popped and its slices are not
// reused.
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
// localEntry tracks a named local variable.
type localEntry struct {
	name  string
	slot  int
	arity int // -1: unknown, 0+: known arity (len of Params in the closure's Proto)
}

// NewCompiler creates a Compiler with the given global slot mapping.
// NewCompiler creates a Compiler with the given global slot mapping.
func NewCompiler(globalSlots map[string]int, source *span.Source) *Compiler {
	return &Compiler{globalSlots: globalSlots, source: source}
}

// SetSource updates the source context for error attribution.
// SetSource updates the source context for error attribution.
func (c *Compiler) SetSource(s *span.Source) {
	c.source = s
}

// top returns a pointer to the current (topmost) compilation frame.
// top returns a pointer to the current (topmost) compilation frame.
func (c *Compiler) top() *frame {
	return &c.frames[len(c.frames)-1]
}

// enterFrame pushes a new zero-value compilation frame onto the stack.
// enterFrame pushes a new zero-value compilation frame onto the stack.
func (c *Compiler) enterFrame() {
	c.frames = append(c.frames, frame{fixSelfSlot: -1})
}

// leaveFrame pops the top frame and transfers its state into a Proto.
// All slice fields are moved directly — no copy.
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
// CompileExpr compiles a top-level Core IR expression into a Proto.
func (c *Compiler) CompileExpr(expr ir.Core) *Proto {
	c.enterFrame()
	c.compileExpr(expr, false)
	c.emit(OpForceEffectful)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// CompileBinding compiles a top-level binding's expression into a Proto.
// CompileBinding compiles a top-level binding's expression into a Proto.
func (c *Compiler) CompileBinding(b ir.Binding) *Proto {
	c.enterFrame()
	c.compileExpr(b.Expr, false)
	c.emit(OpReturn)
	return c.leaveFrame()
}

// RecordArity registers the known arity of a global binding.
// Called after CompileBinding so that subsequent references emit OpApplyN.
// RecordArity registers the known arity of a global binding.
// Called after CompileBinding so that subsequent references emit OpApplyN.
func (c *Compiler) RecordArity(name string, arity int) {
	if c.globalArities == nil {
		c.globalArities = make(map[string]int)
	}
	c.globalArities[name] = arity
}

// --- bytecode emission ---
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
// --- local variable management ---

func (c *Compiler) allocLocal(name string) int {
	return c.allocLocalWithArity(name, -1)
}
func (c *Compiler) allocLocalWithArity(name string, arity int) int {
	f := c.top()
	slot := f.numLocals
	f.numLocals++
	f.locals = append(f.locals, localEntry{name: name, slot: slot, arity: arity})
	return slot
}

// localArity returns the known arity of a local variable, or -1 if unknown.
// localArity returns the known arity of a local variable, or -1 if unknown.
func (c *Compiler) localArity(name string) int {
	f := c.top()
	for i := len(f.locals) - 1; i >= 0; i-- {
		if f.locals[i].name == name {
			return f.locals[i].arity
		}
	}
	return -1
}

// varArity returns the known arity of a variable (local or global).
// varArity returns the known arity of a variable (local or global).
func (c *Compiler) varArity(v *ir.Var) int {
	if v.Index >= 0 {
		return c.localArity(v.Name)
	}
	if c.globalArities != nil {
		key := v.Key
		if key == "" {
			key = ir.VarKey(v)
		}
		if a, ok := c.globalArities[key]; ok {
			return a
		}
	}
	return -1
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
