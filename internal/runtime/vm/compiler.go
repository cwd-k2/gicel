// Compiler — Core IR to bytecode.
// Package layout:
//   compiler.go          — struct, frame lifecycle, pool/scope management, entry points
//   compiler_emit.go     — bytecode emission primitives
//   compiler_expr.go     — IR node compilation (compileExpr and per-node handlers)
//   compiler_closure.go  — child proto builders (lambda/thunk/fix/merge) and capture resolution
//   compiler_match.go    — pattern match compilation

package vm

import (
	"strconv"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// Compiler translates Core IR into bytecode Protos.
//
// Compilation uses a scope-stack model: a single Compiler holds a stack of
// compilation frames. Each frame accumulates bytecode, constants, and
// metadata for one Proto. enterFrame pushes a new frame; leaveFrame pops
// the frame and finalizes its Proto.
//
// Per-NewRuntime typed pools (compilerPool) supply the backing arrays for
// Proto slice fields. Each frame holds (start, end) range markers into
// the active pools; leaveFrame takes a cap-clamped sub-slice of each pool
// (pool[start:end:end]) and stores it on the Proto. This amortizes the
// per-frame slice allocations across the whole NewRuntime.
//
// Pool lifetime: the Compiler owns the pools while compiling. After
// precompileVM completes, the Compiler reference is dropped and Go GC
// keeps the pool backing arrays alive transitively through the Protos
// that point into them. No explicit hand-off is required.
type Compiler struct {
	globalSlots map[string]int
	globalPrims map[string]primInfo // prim-alias binding key → resolved metadata
	source      *span.Source
	frames      []frame
	pool        compilerPool
}

// compilerPool holds the per-NewRuntime backing arrays for Proto slice
// fields. Each pool is a Go slice that grows by append. After compile,
// the backing arrays are kept alive by the Protos that hold sub-slices
// into them.
//
// Two kinds of pools:
//   - Proto-bound index-addressed (matchDescs, ...): bytecode references
//     entries by frame-local index, computed AFTER append (see addMatchDesc
//     for the rationale). Children's pool entries appear as gaps in the
//     parent's Proto.X view but are never referenced by parent's bytecode.
//   - Scratchpad (locals): compile-time only. leaveFrame shrinks the pool
//     back to the frame's start because nothing retains the entries past
//     frame scope. Peak size = max nesting depth × max locals per frame.
//
// Spans is NOT pooled — it requires offset-sorted ordering for binary
// search (Proto.SpanAt), and a shared pool would interleave child spans
// (with child-local offsets) into the parent's range, breaking the sort.
//
// Code is NOT pooled — it requires contiguous bytecode for the linear
// VM dispatch loop, and a shared pool would insert child bytes into the
// parent's logical code stream.
type compilerPool struct {
	matchDescs  []MatchDesc
	recordDescs []RecordDesc
	mergeDescs  []MergeDesc
	locals      []localEntry
}

// Initial capacities for each pool. Chosen empirically (2026-04-08) from
// the actual element counts observed across smallSource / largeSource(100)
// / largeSource(500) compiles, which are dominated by Prelude.
//
// Pre-sizing eliminates the intermediate geometric grow allocations that
// would otherwise dominate the pool's total allocated bytes, making the
// pool strictly better than per-frame append in both alloc count AND
// total bytes.
//
// If a compile exceeds the initial capacity, the pool grows normally via
// append — grow events during compile are safe because finalized Protos
// hold sub-slices into the old backing (kept alive by GC reachability)
// and the new backing receives subsequent appends.
const (
	poolInitMatchDescs  = 400 // measured 359 across all 3 workloads
	poolInitRecordDescs = 64  // small (record literals are uncommon)
	poolInitMergeDescs  = 32  // very rare
	poolInitLocals      = 32  // measured peak 13 (LIFO shrink keeps it tiny)
)

// sub*() returns a cap-clamped sub-slice of the corresponding pool.
// The cap clamp (pool[start:end:end]) prevents accidental append from
// corrupting an adjacent frame's data — a finalized Proto can only grow
// its slice fields by allocating its own backing array, leaving the pool
// unchanged.
func subMatchDescs(pool []MatchDesc, start, end int) []MatchDesc {
	if start == end {
		return nil
	}
	return pool[start:end:end]
}

func subRecordDescs(pool []RecordDesc, start, end int) []RecordDesc {
	if start == end {
		return nil
	}
	return pool[start:end:end]
}

func subMergeDescs(pool []MergeDesc, start, end int) []MergeDesc {
	if start == end {
		return nil
	}
	return pool[start:end:end]
}

// primInfo holds the metadata needed to emit a direct OpPrim/OpEffectPrim
// for a call site whose function position is a Var pointing at a bare
// prim-alias binding (an assumption declaration whose IR expression is a
// 0-arg *ir.PrimOp). Populated by RecordGlobalPrim before compileApp runs.
type primInfo struct {
	name      string
	arity     int
	effectful bool
}

// frame accumulates bytecode and metadata for a single Proto.
//
// Most slice fields are owned per-frame and transferred to the Proto in
// leaveFrame (zero copy). Pooled fields (currently: matchDescs and locals)
// live in the per-Compiler pool and the frame holds (start, end) markers
// into the pool — leaveFrame takes a cap-clamped sub-slice (matchDescs)
// or shrinks the pool back to Start (locals scratchpad).
type frame struct {
	code      []byte
	constants []eval.Value
	strings   []string
	protos    []*Proto
	spans     []SpanEntry
	// Pool ranges (pool.matchDescs/recordDescs/mergeDescs/locals).
	// enterFrame sets Start==End to the current pool length; add*() append
	// to the pool and advance End; leaveFrame takes pool[Start:End:End]
	// for Proto-bound index-addressed pools, and truncates back to Start
	// for scratchpad pools (locals).
	matchDescsStart  int
	matchDescsEnd    int
	recordDescsStart int
	recordDescsEnd   int
	mergeDescsStart  int
	mergeDescsEnd    int
	localsStart      int
	localsEnd        int
	numLocals        int
	captures         []int
	bindNames        []BindInfo
	isThunk          bool
	params           []string
	fixSelfSlot      int
}

// localEntry tracks a named local variable.
type localEntry struct {
	name string
	slot int
}

// NewCompiler creates a Compiler with the given global slot mapping.
// Pre-sizes typed pools to their empirically determined initial caps to
// avoid geometric grow allocs during compile.
func NewCompiler(globalSlots map[string]int, source *span.Source) *Compiler {
	return &Compiler{
		globalSlots: globalSlots,
		source:      source,
		pool: compilerPool{
			matchDescs:  make([]MatchDesc, 0, poolInitMatchDescs),
			recordDescs: make([]RecordDesc, 0, poolInitRecordDescs),
			mergeDescs:  make([]MergeDesc, 0, poolInitMergeDescs),
			locals:      make([]localEntry, 0, poolInitLocals),
		},
	}
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
// Pool range markers are set to the current pool length so that add*()
// adders append into the pool starting at the frame's local "zero".
func (c *Compiler) enterFrame() {
	matchDescsBase := len(c.pool.matchDescs)
	recordDescsBase := len(c.pool.recordDescs)
	mergeDescsBase := len(c.pool.mergeDescs)
	localsBase := len(c.pool.locals)
	c.frames = append(c.frames, frame{
		fixSelfSlot:      -1,
		matchDescsStart:  matchDescsBase,
		matchDescsEnd:    matchDescsBase,
		recordDescsStart: recordDescsBase,
		recordDescsEnd:   recordDescsBase,
		mergeDescsStart:  mergeDescsBase,
		mergeDescsEnd:    mergeDescsBase,
		localsStart:      localsBase,
		localsEnd:        localsBase,
	})
}

// leaveFrame pops the top frame and transfers its state into a Proto.
//
// Per-frame slice fields are moved directly into the Proto (zero copy).
// Pool-bound fields (spans, matchDescs) become cap-clamped sub-slices of
// the corresponding pools.
//
// Scratchpad pools (locals) are truncated back to this frame's start —
// the discarded entries are no longer reachable, so the pool's peak size
// is bounded by max nesting depth × max locals/frame.
func (c *Compiler) leaveFrame() *Proto {
	f := c.top()
	c.pool.locals = c.pool.locals[:f.localsStart]
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
		MatchDescs:  subMatchDescs(c.pool.matchDescs, f.matchDescsStart, f.matchDescsEnd),
		RecordDescs: subRecordDescs(c.pool.recordDescs, f.recordDescsStart, f.recordDescsEnd),
		MergeDescs:  subMergeDescs(c.pool.mergeDescs, f.mergeDescsStart, f.mergeDescsEnd),
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

// RecordGlobalPrim registers a prim-alias binding so that saturated call
// sites `Var{prim} a b ...` can bypass the loaded PrimVal stub and emit
// OpPrim/OpEffectPrim directly. Called before the main/entry compilation
// pass in precompileVM, scanning all module entries and main bindings.
//
// The invariant: the binding named `key` has IR expression
// `&ir.PrimOp{Name, Arity, Effectful, Args: nil}`. When compileApp sees a
// global Var with this key and enough arguments in the application spine,
// it emits a direct saturated prim call instead of loading the stub and
// going through applyPrim's partial-application path.
func (c *Compiler) RecordGlobalPrim(key, name string, arity int, effectful bool) {
	if c.globalPrims == nil {
		c.globalPrims = make(map[string]primInfo)
	}
	c.globalPrims[key] = primInfo{name: name, arity: arity, effectful: effectful}
}

// --- constant / string pool ---

const maxPoolSize = 1<<16 - 1

// PoolOverflowError is the typed panic raised when a per-Proto pool reaches
// the u16 capacity limit (65535 entries). It carries the offending pool name
// so that the recover site can produce a structured diagnostic instead of a
// raw runtime panic. Caught by the targeted recover in `precompileVM`; any
// other panic shape from this package is treated as a real bug and propagates.
type PoolOverflowError struct {
	Pool string // "constant" | "string" | "match desc" | "record desc" | "merge desc" | "proto"
}

func (e *PoolOverflowError) Error() string {
	return "bytecode " + e.Pool + " pool exceeded maximum of " +
		strconv.Itoa(maxPoolSize) + " entries"
}

func poolOverflow(pool string) {
	panic(&PoolOverflowError{Pool: pool})
}

func (c *Compiler) addConstant(v eval.Value) uint16 {
	f := c.top()
	for i, existing := range f.constants {
		if existing == v {
			return uint16(i)
		}
	}
	if len(f.constants) >= maxPoolSize {
		poolOverflow("constant")
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
		poolOverflow("string")
	}
	idx := len(f.strings)
	f.strings = append(f.strings, s)
	return uint16(idx)
}

// addMatchDesc appends a match descriptor to the per-Compiler pool and
// returns the descriptor's index relative to the start of the current
// frame's pool range. This index is what's stored in bytecode operands
// (which read Proto.MatchDescs[idx] at runtime).
//
// CRITICAL: the index must be computed AFTER the append, not before.
// Between the time this frame was entered and now, child frames may have
// appended their own descriptors to the same pool, advancing its length
// past this frame's stale matchDescsEnd. Using the pre-append count would
// produce a frame-local index that collides with a child's descriptor in
// the parent's Proto.MatchDescs view (= pool[start:end]). Computing the
// index AFTER append uses the actual pool position, which yields a unique
// (possibly non-contiguous) index in the parent's range — child entries
// occupy "gap" indices that the parent's bytecode never references.
func (c *Compiler) addMatchDesc(md MatchDesc) uint16 {
	f := c.top()
	c.pool.matchDescs = append(c.pool.matchDescs, md)
	f.matchDescsEnd = len(c.pool.matchDescs)
	idx := f.matchDescsEnd - 1 - f.matchDescsStart
	if idx >= maxPoolSize {
		poolOverflow("match desc")
	}
	return uint16(idx)
}
func (c *Compiler) addRecordDesc(rd RecordDesc) uint16 {
	f := c.top()
	c.pool.recordDescs = append(c.pool.recordDescs, rd)
	f.recordDescsEnd = len(c.pool.recordDescs)
	idx := f.recordDescsEnd - 1 - f.recordDescsStart
	if idx >= maxPoolSize {
		poolOverflow("record desc")
	}
	return uint16(idx)
}
func (c *Compiler) addMergeDesc(md MergeDesc) uint16 {
	f := c.top()
	c.pool.mergeDescs = append(c.pool.mergeDescs, md)
	f.mergeDescsEnd = len(c.pool.mergeDescs)
	idx := f.mergeDescsEnd - 1 - f.mergeDescsStart
	if idx >= maxPoolSize {
		poolOverflow("merge desc")
	}
	return uint16(idx)
}

// --- local variable management ---

// localCount returns the number of locals visible in the current frame.
// Used by compilePatternMatch to save a position for popLocals.
func (c *Compiler) localCount() int {
	f := c.top()
	return f.localsEnd - f.localsStart
}

func (c *Compiler) allocLocal(name string) int {
	f := c.top()
	slot := f.numLocals
	f.numLocals++
	c.pool.locals = append(c.pool.locals, localEntry{name: name, slot: slot})
	f.localsEnd = len(c.pool.locals)
	return slot
}

// varGlobalPrim returns the prim-alias metadata associated with a global
// Var reference, or (zero, false) if the Var is a local or names a
// non-prim-alias global.
func (c *Compiler) varGlobalPrim(v *ir.Var) (primInfo, bool) {
	if v.Index >= 0 || c.globalPrims == nil {
		return primInfo{}, false
	}
	key := v.Key
	if key == "" {
		key = ir.VarKey(v)
	}
	info, ok := c.globalPrims[key]
	return info, ok
}
func (c *Compiler) resolveLocal(name string) (int, bool) {
	f := c.top()
	locals := c.pool.locals
	for i := f.localsEnd - 1; i >= f.localsStart; i-- {
		if locals[i].name == name {
			return locals[i].slot, true
		}
	}
	return -1, false
}

// popLocals truncates the current frame's locals to the given count
// (frame-local index). The pool is shrunk to release the discarded
// entries; combined with leaveFrame's shrink, this keeps peak memory
// bounded by max nesting depth × max locals/frame.
func (c *Compiler) popLocals(count int) {
	f := c.top()
	target := f.localsStart + count
	c.pool.locals = c.pool.locals[:target]
	f.localsEnd = target
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
		poolOverflow("proto")
	}
	idx := len(f.protos)
	f.protos = append(f.protos, p)
	return uint16(idx)
}

// --- compilation ---
