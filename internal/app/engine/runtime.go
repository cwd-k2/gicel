package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
	"github.com/cwd-k2/gicel/internal/runtime/vm"
)

// ExplainDepth controls how deeply the explain trace instruments evaluation.
type ExplainDepth int

const (
	// ExplainUser traces user code only; stdlib internals are suppressed.
	ExplainUser ExplainDepth = iota
	// ExplainAll traces all code including stdlib internals.
	ExplainAll
)

// RunOptions configures a single execution.
//
// Caps is passed to CapEnv with copy-on-write semantics; the caller's
// map is never mutated. Bindings is read-only during execution.
// Both maps may be safely reused after the call returns.
type RunOptions struct {
	// Entry is the top-level binding to evaluate (default: DefaultEntryPoint).
	Entry string
	// Caps provides initial capability values.
	Caps map[string]any
	// Bindings provides host-injected value bindings.
	Bindings map[string]eval.Value
	// Explain receives semantic evaluation events. Nil disables explain.
	Explain eval.ExplainHook
	// ExplainDepth controls stdlib suppression (default: ExplainUser).
	ExplainDepth ExplainDepth
	// Trace receives low-level evaluation step events. Nil disables trace.
	Trace eval.TraceHook
}

// Runtime is an immutable, compiled GICEL program.
// It is goroutine-safe and can be executed concurrently.
type Runtime struct {
	prog               *ir.Program
	prims              *eval.PrimRegistry
	stepLimit          int
	depthLimit         int
	nestingLimit       int
	allocLimit         int64
	source             *span.Source
	bindings           map[string]types.Type
	moduleEntries      []moduleEntry         // ALL module programs in registration order
	builtinGlobals     map[string]eval.Value // pre-built pure/bind/force/fix/rec closures + constructors
	globalSlots        map[string]int        // global name → slot index (assigned at NewRuntime)
	numGlobals         int                   // total number of global slots
	sortedMainBindings []ir.Binding          // all main bindings, topologically pre-sorted
	entryName          string                // default entry point name
	entryExpr          ir.Core               // default entry point expression (nil if not found)

	// Cached bytecode (populated at NewRuntime time for VM backend).
	vmModuleProtos [][]vmBindingProto    // pre-compiled module binding protos
	vmMainProtos   []vmBindingProto      // pre-compiled non-entry main binding protos
	vmEntryProto   *vm.Proto             // pre-compiled entry expression proto
	vmBuiltins     map[string]eval.Value // pre-compiled builtin VMClosures

	// Cached global array template. Populated after the first RunWith
	// that evaluates module bindings successfully. Subsequent RunWith
	// calls clone the template and skip evalPrecompiledBindings.
	// Pure module bindings are referentially transparent — their results
	// depend only on other module bindings and builtins, all fixed at
	// NewRuntime time.
	cachedGlobals     []eval.Value
	cachedGlobalsOnce sync.Once
}

type vmBindingProto struct {
	name      string
	proto     *vm.Proto
	generated bool // true when introduced by the compiler (dict bindings)
}

type moduleEntry struct {
	name           string
	prog           *ir.Program
	sortedBindings []ir.Binding // pre-sorted bindings for evaluation
	source         *span.Source // source text for error attribution
}

// Program returns the compiled Core IR for debugging/inspection.
func (r *Runtime) Program() *CoreProgram {
	return &CoreProgram{prog: r.prog}
}

// RunResult holds the result of an execution.
type RunResult struct {
	Value  eval.Value
	CapEnv eval.CapEnv
	Stats  eval.EvalStats
}

// annotateError populates Line/Col on RuntimeError from the Source.
// If the error carries its own Source (from the evaluator's source context),
// that takes precedence over the Runtime's main source.
func (r *Runtime) annotateError(err error) error {
	var re *eval.RuntimeError
	if errors.As(err, &re) && re.Span != (span.Span{}) {
		src := re.Source
		if src == nil {
			src = r.source
		}
		if src != nil {
			re.Line, re.Col = src.Location(re.Span.Start)
		}
	}
	return err
}

// initBuiltinGlobals registers builtin names and data constructors in the
// globals map. Builtin values (pure, bind, force, fix, rec) are placeholder
// nils here; precompileVM replaces them with VMClosures.
func (r *Runtime) initBuiltinGlobals(gatedBuiltins map[string]bool) {
	globals := make(map[string]eval.Value, 8)
	// Register builtin names (values are set by precompileVM).
	for _, name := range []string{"pure", "bind", "force"} {
		globals[name] = nil
	}
	if gatedBuiltins["fix"] {
		globals["fix"] = nil
	}
	if gatedBuiltins["rec"] {
		globals["rec"] = nil
	}

	for _, me := range r.moduleEntries {
		for _, d := range me.prog.DataDecls {
			for _, con := range d.Cons {
				globals[ir.QualifiedKey(me.name, con.Name)] = &eval.ConVal{Con: con.Name}
			}
		}
	}

	for _, d := range r.prog.DataDecls {
		for _, con := range d.Cons {
			globals[con.Name] = &eval.ConVal{Con: con.Name}
		}
	}

	r.builtinGlobals = globals
}

// buildGlobalSlots collects all global names and assigns each a slot index.
// The resulting map is used by the evaluator to resolve global Var nodes
// (Index == -1) by name at eval time. No IR mutation occurs — the program
// IR remains immutable after compilation.
func (r *Runtime) buildGlobalSlots() {
	slots := make(map[string]int, len(r.builtinGlobals)+len(r.bindings))

	// Assign slots from builtinGlobals (builtins + constructors).
	// Sort keys for deterministic slot assignment — Go map iteration is
	// non-deterministic, and varying slot indices can expose latent bugs.
	builtinKeys := make([]string, 0, len(r.builtinGlobals))
	for k := range r.builtinGlobals {
		builtinKeys = append(builtinKeys, k)
	}
	sort.Strings(builtinKeys)
	for _, k := range builtinKeys {
		slots[k] = len(slots)
	}
	// Host binding names (values provided at RunWith time).
	hostKeys := make([]string, 0, len(r.bindings))
	for name := range r.bindings {
		hostKeys = append(hostKeys, name)
	}
	sort.Strings(hostKeys)
	for _, name := range hostKeys {
		if _, ok := slots[name]; !ok {
			slots[name] = len(slots)
		}
	}
	// Module binding names.
	for _, me := range r.moduleEntries {
		for _, b := range me.prog.Bindings {
			key := ir.QualifiedKey(me.name, b.Name)
			if _, ok := slots[key]; !ok {
				slots[key] = len(slots)
			}
		}
	}
	// Main binding names.
	for _, b := range r.prog.Bindings {
		if _, ok := slots[b.Name]; !ok {
			slots[b.Name] = len(slots)
		}
	}

	r.globalSlots = slots
	r.numGlobals = len(slots)
}

// buildGlobalArray creates the global value array from the slot map.
// Builtin values and host bindings are filled; module/main bindings
// are left nil (filled by evalPrecompiledBindings).
func (r *Runtime) buildGlobalArray(hostBindings map[string]eval.Value) ([]eval.Value, error) {
	arr := make([]eval.Value, r.numGlobals)
	for k, v := range r.builtinGlobals {
		arr[r.globalSlots[k]] = v
	}
	var missing []string
	for name := range r.bindings {
		v, ok := hostBindings[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		arr[r.globalSlots[name]] = v
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("missing host binding(s): %s (declared with DeclareBinding but not provided in RunOptions.Bindings)", strings.Join(missing, ", "))
	}
	return arr, nil
}

type runRequest struct {
	caps      map[string]any
	bindings  map[string]eval.Value
	entry     string
	obs       *eval.ExplainObserver
	traceHook eval.TraceHook
}

// execute runs the program using the bytecode VM.
func (r *Runtime) execute(ctx context.Context, req *runRequest) (eval.EvalResult, eval.EvalStats, error) {
	// Catch pre-cancelled contexts before allocating execution state.
	// With amortized context checking in budget.Step(), programs with
	// fewer than 64 steps might never poll the channel.
	if err := ctx.Err(); err != nil {
		return eval.EvalResult{}, eval.EvalStats{}, budget.WrapCtxErr(err)
	}

	globalArray, err := r.buildGlobalArray(req.bindings)
	if err != nil {
		return eval.EvalResult{}, eval.EvalStats{}, err
	}

	b := budget.New(ctx, r.stepLimit, r.depthLimit)
	if r.nestingLimit > 0 {
		b.SetNestingLimit(r.nestingLimit)
	}
	if r.allocLimit > 0 {
		b.SetAllocLimit(r.allocLimit)
	}

	capEnv := eval.NewCapEnv(req.caps)

	// Embed budget in context so stdlib primitives can call budget.ChargeAlloc.
	vmCtx := budget.ContextWithBudget(ctx, b)
	return r.executeVM(vmCtx, b, globalArray, capEnv, req)
}

// precompileVM compiles all module bindings, main bindings, entry expression,
// and builtins to bytecode at NewRuntime time. This avoids per-execution
// compilation overhead in RunWith.
//
// Pool overflow panics from the bytecode compiler (vm.PoolOverflowError) are
// caught and converted to a structured CompileError with code
// ErrCompilePoolOverflow. The recover deliberately type-asserts on
// *vm.PoolOverflowError so that any other panic shape (internal compiler
// invariant violations, nil dereferences) continues to propagate as a real
// crash — those are bugs, not user-recoverable conditions.
func (r *Runtime) precompileVM(gates map[string]bool) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			if poe, isOverflow := rec.(*vm.PoolOverflowError); isOverflow {
				diags := &diagnostic.Errors{Source: r.source}
				diags.Add(&diagnostic.Error{
					Code:    diagnostic.ErrCompilePoolOverflow,
					Phase:   diagnostic.PhaseCheck,
					Message: poe.Error(),
				})
				err = &CompileError{Errors: diags}
				return
			}
			panic(rec) // real bug — propagate
		}
	}()

	compiler := vm.NewCompiler(r.globalSlots, r.source)

	// Pre-pass: register prim-alias bindings (assumption declarations whose
	// IR expression is a bare 0-arg *ir.PrimOp) so that compileApp can emit
	// direct saturated OpPrim/OpEffectPrim calls instead of loading the
	// PrimVal stub and walking the OpApply partial-application chain. This
	// is the natural cross-module locus — precompileVM has every module
	// and main binding in scope simultaneously.
	for _, me := range r.moduleEntries {
		for _, b := range me.sortedBindings {
			if po, ok := b.Expr.(*ir.PrimOp); ok && len(po.Args) == 0 && po.Arity > 0 {
				compiler.RecordGlobalPrim(ir.QualifiedKey(me.name, b.Name), po.Name, po.Arity, po.Effectful)
			}
		}
	}
	for _, b := range r.sortedMainBindings {
		if po, ok := b.Expr.(*ir.PrimOp); ok && len(po.Args) == 0 && po.Arity > 0 {
			compiler.RecordGlobalPrim(b.Name, po.Name, po.Arity, po.Effectful)
		}
	}

	// Compile builtins.
	r.vmBuiltins = vm.CompileBuiltinGlobals(compiler,
		gates["fix"], gates["rec"])

	// Compile module bindings.
	r.vmModuleProtos = make([][]vmBindingProto, len(r.moduleEntries))
	for i, me := range r.moduleEntries {
		compiler.SetSource(me.source)
		protos := make([]vmBindingProto, len(me.sortedBindings))
		for j, b := range me.sortedBindings {
			p := compiler.CompileBinding(b)
			protos[j] = vmBindingProto{name: b.Name, proto: p, generated: b.Generated}
		}
		r.vmModuleProtos[i] = protos
	}

	// Compile main bindings (non-entry).
	compiler.SetSource(r.source)
	for _, b := range r.sortedMainBindings {
		if b.Name != r.entryName {
			p := compiler.CompileBinding(b)
			r.vmMainProtos = append(r.vmMainProtos, vmBindingProto{
				name: b.Name, proto: p, generated: b.Generated,
			})
		}
	}

	// Compile entry expression.
	if r.entryExpr != nil {
		r.vmEntryProto = compiler.CompileExpr(r.entryExpr)
	}

	// Resolve primitive implementations at link time.
	// After this, OpPrim/OpEffectPrim use Proto.ResolvedPrims[nameIdx]
	// instead of runtime map lookups.
	for _, mp := range r.vmModuleProtos {
		for _, bp := range mp {
			bp.proto.ResolvePrims(r.prims)
		}
	}
	for _, bp := range r.vmMainProtos {
		bp.proto.ResolvePrims(r.prims)
	}
	if r.vmEntryProto != nil {
		r.vmEntryProto.ResolvePrims(r.prims)
	}
	return nil
}

// executeVM runs the entry expression on the VM. On the first call, module
// and main bindings are evaluated and the resulting global array is cached
// as a template. Subsequent calls clone the template, skipping re-evaluation.
func (r *Runtime) executeVM(ctx context.Context, b *budget.Budget, globalArray []eval.Value, capEnv eval.CapEnv, req *runRequest) (eval.EvalResult, eval.EvalStats, error) {
	machine := vm.NewVM(vm.VMConfig{
		Globals:     globalArray,
		GlobalSlots: r.globalSlots,
		Prims:       r.prims,
		Budget:      b,
		Ctx:         ctx,
		Observer:    req.obs,
		Trace:       req.traceHook,
		Source:      r.source,
	})

	if r.cachedGlobals != nil {
		// Fast path: clone cached template. Module bindings are pure
		// and referentially transparent — their results never change.
		copy(globalArray, r.cachedGlobals)
		// Re-install host bindings which may differ across RunWith calls.
		// buildGlobalArray already validated and set them in globalArray,
		// but copy() overwrote those slots with the cached (first-run) values.
		for name := range r.bindings {
			if v, ok := req.bindings[name]; ok {
				globalArray[r.globalSlots[name]] = v
			}
		}
	} else {
		// First execution: evaluate module + main bindings, then cache.

		// Install pre-compiled builtins.
		for name, val := range r.vmBuiltins {
			if slot, ok := r.globalSlots[name]; ok {
				globalArray[slot] = val
			}
		}

		// Evaluate pre-compiled module bindings.
		for i, me := range r.moduleEntries {
			req.obs.SetSource(me.source)
			if err := r.evalPrecompiledBindings(machine, r.vmModuleProtos[i], me.name, globalArray, req.obs); err != nil {
				return eval.EvalResult{}, machine.Stats(), err
			}
		}

		// Evaluate pre-compiled non-entry main bindings.
		req.obs.SetSource(r.source)
		if err := r.evalPrecompiledBindings(machine, r.vmMainProtos, "", globalArray, req.obs); err != nil {
			return eval.EvalResult{}, machine.Stats(), err
		}

		// Cache the evaluated global array for subsequent RunWith calls.
		r.cachedGlobalsOnce.Do(func() {
			r.cachedGlobals = make([]eval.Value, len(globalArray))
			copy(r.cachedGlobals, globalArray)
		})
	}

	// Handle non-default entry point.
	var entryProto *vm.Proto
	if req.entry == r.entryName {
		entryProto = r.vmEntryProto
	} else {
		compiler := vm.NewCompiler(r.globalSlots, r.source)
		for _, bb := range r.sortedMainBindings {
			if bb.Name == req.entry {
				entryProto = compiler.CompileExpr(bb.Expr)
				break
			}
		}
	}
	if entryProto == nil {
		return eval.EvalResult{}, machine.Stats(), fmt.Errorf("entry point %q not found", req.entry)
	}

	if req.obs != nil {
		req.obs.Section(req.entry)
	}

	result, err := machine.Run(entryProto, capEnv)
	if err != nil {
		return eval.EvalResult{}, machine.Stats(), err
	}
	if req.obs != nil {
		req.obs.Result(eval.PrettyValue(result.Value))
	}
	return result, machine.Stats(), nil
}

// evalPrecompiledBindings runs pre-compiled binding protos on the VM.
func (r *Runtime) evalPrecompiledBindings(machine *vm.VM, protos []vmBindingProto, modulePrefix string, globalArray []eval.Value, obs *eval.ExplainObserver) error {
	type slotCell struct {
		slot int
		key  string
		cell *eval.IndirectVal
	}
	cells := make([]slotCell, len(protos))
	for i, bp := range protos {
		key := bp.name
		if modulePrefix != "" {
			key = ir.QualifiedKey(modulePrefix, bp.name)
		}
		slot := r.globalSlots[key]
		cell := &eval.IndirectVal{}
		globalArray[slot] = cell
		cells[i] = slotCell{slot, key, cell}
	}
	userVisible := modulePrefix == ""
	for i, bp := range protos {
		if userVisible {
			obs.Section(bp.name)
		}
		result, err := machine.Run(bp.proto, eval.NewCapEnv(nil))
		if err != nil {
			if budget.IsLimitError(err) || bp.generated {
				return err
			}
			return fmt.Errorf("evaluating %s: %w", bp.name, err)
		}
		v := result.Value
		if clo, ok := v.(*eval.VMClosure); ok {
			clo.Name = bp.name
			if !userVisible {
				obs.MarkInternal(bp.name)
			}
		}
		val := v
		cells[i].cell.Ref = &val
		globalArray[cells[i].slot] = v
	}
	return nil
}

// RunWith executes the program with the given options.
func (r *Runtime) RunWith(ctx context.Context, opts *RunOptions) (*RunResult, error) {
	if opts == nil {
		opts = &RunOptions{}
	}
	entry := opts.Entry
	if entry == "" {
		entry = r.entryName
	}
	var obs *eval.ExplainObserver
	if opts.Explain != nil {
		obs = eval.NewExplainObserver(opts.Explain, r.source)
		if opts.ExplainDepth == ExplainAll {
			obs.SetAll(true)
		}
	}
	result, stats, err := r.execute(ctx, &runRequest{
		caps:      opts.Caps,
		bindings:  opts.Bindings,
		entry:     entry,
		obs:       obs,
		traceHook: opts.Trace,
	})
	if err != nil {
		return &RunResult{Stats: stats}, r.annotateError(err)
	}
	return &RunResult{Value: result.Value, CapEnv: result.CapEnv, Stats: stats}, nil
}
