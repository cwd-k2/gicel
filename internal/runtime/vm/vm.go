package vm

import (
	"context"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
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
	globalSlots map[ir.VarKey]int

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
	GlobalSlots map[ir.VarKey]int
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
		Apply:          vm.applyForPrim,
		ApplyN:         vm.applyNForPrim,
		ForceEffectful: vm.forceEffectfulForPrim,
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
