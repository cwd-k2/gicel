package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/budget"
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
}

type vmBindingProto struct {
	name  string
	proto *vm.Proto
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

// initBuiltinGlobals constructs the immutable base globals with builtins and constructors.
func (r *Runtime) initBuiltinGlobals(gatedBuiltins map[string]bool) {
	globals := eval.BuiltinGlobals(gatedBuiltins["fix"], gatedBuiltins["rec"])

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
// are left nil (filled by evalBindingsCore).
func (r *Runtime) buildGlobalArray(hostBindings map[string]eval.Value) ([]eval.Value, error) {
	arr := make([]eval.Value, r.numGlobals)
	for k, v := range r.builtinGlobals {
		arr[r.globalSlots[k]] = v
	}
	for name := range r.bindings {
		v, ok := hostBindings[name]
		if !ok {
			return nil, fmt.Errorf("missing binding: %s", name)
		}
		arr[r.globalSlots[name]] = v
	}
	return arr, nil
}

// buildNamedGlobals constructs the named globals map as a fallback for
// Var nodes whose Index remains -1 because assignGlobalSlots exceeded
// maxTraversalDepth. The map mirrors builtinGlobals + host bindings.
func (r *Runtime) buildNamedGlobals(hostBindings map[string]eval.Value) map[string]eval.Value {
	m := make(map[string]eval.Value, len(r.builtinGlobals)+len(hostBindings))
	for k, v := range r.builtinGlobals {
		m[k] = v
	}
	for k, v := range hostBindings {
		m[k] = v
	}
	return m
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

	namedGlobals := r.buildNamedGlobals(req.bindings)
	capEnv := eval.NewCapEnv(req.caps)

	return r.executeVM(ctx, b, globalArray, namedGlobals, capEnv, req)
}

// precompileVM compiles all module bindings, main bindings, entry expression,
// and builtins to bytecode at NewRuntime time. This avoids per-execution
// compilation overhead in RunWith.
func (r *Runtime) precompileVM(gates map[string]bool) {
	compiler := vm.NewCompiler(r.globalSlots, r.source)

	// Compile builtins.
	r.vmBuiltins = vm.CompileBuiltinGlobals(compiler,
		gates["fix"], gates["rec"])

	// Compile module bindings.
	r.vmModuleProtos = make([][]vmBindingProto, len(r.moduleEntries))
	for i, me := range r.moduleEntries {
		compiler.SetSource(me.source)
		protos := make([]vmBindingProto, len(me.sortedBindings))
		for j, b := range me.sortedBindings {
			protos[j] = vmBindingProto{name: b.Name, proto: compiler.CompileBinding(b)}
		}
		r.vmModuleProtos[i] = protos
	}

	// Compile main bindings (non-entry).
	compiler.SetSource(r.source)
	for _, b := range r.sortedMainBindings {
		if b.Name != r.entryName {
			r.vmMainProtos = append(r.vmMainProtos, vmBindingProto{
				name: b.Name, proto: compiler.CompileBinding(b),
			})
		}
	}

	// Compile entry expression.
	if r.entryExpr != nil {
		r.vmEntryProto = compiler.CompileExpr(r.entryExpr)
	}
}

// executeVM compiles the entry expression to bytecode and runs it on the VM.
// Module and non-entry bindings are evaluated by the tree-walker (stored in
// globalArray). The VM uses the same global array for variable lookup.
func (r *Runtime) executeVM(ctx context.Context, b *budget.Budget, globalArray []eval.Value, namedGlobals map[string]eval.Value, capEnv eval.CapEnv, req *runRequest) (eval.EvalResult, eval.EvalStats, error) {
	machine := vm.NewVM(vm.VMConfig{
		Globals:      globalArray,
		GlobalSlots:  r.globalSlots,
		NamedGlobals: namedGlobals,
		Prims:        r.prims,
		Budget:       b,
		Ctx:          ctx,
		Observer:     req.obs,
		Trace:        req.traceHook,
		Source:       r.source,
	})

	// Install pre-compiled builtins.
	for name, val := range r.vmBuiltins {
		if slot, ok := r.globalSlots[name]; ok {
			globalArray[slot] = val
			namedGlobals[name] = val
		}
	}

	// Evaluate pre-compiled module bindings.
	for i, me := range r.moduleEntries {
		req.obs.SetSource(me.source)
		if err := r.evalPrecompiledBindings(machine, r.vmModuleProtos[i], me.name, globalArray, namedGlobals, req.obs); err != nil {
			return eval.EvalResult{}, machine.Stats(), err
		}
	}

	// Evaluate pre-compiled non-entry main bindings.
	req.obs.SetSource(r.source)
	if err := r.evalPrecompiledBindings(machine, r.vmMainProtos, "", globalArray, namedGlobals, req.obs); err != nil {
		return eval.EvalResult{}, machine.Stats(), err
	}

	// Handle non-default entry point.
	entryProto := r.vmEntryProto
	if req.entry != r.entryName {
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
func (r *Runtime) evalPrecompiledBindings(machine *vm.VM, protos []vmBindingProto, modulePrefix string, globalArray []eval.Value, namedGlobals map[string]eval.Value, obs *eval.ExplainObserver) error {
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
		namedGlobals[key] = cell
		cells[i] = slotCell{slot, key, cell}
	}
	userVisible := modulePrefix == ""
	for i, bp := range protos {
		if userVisible {
			obs.Section(bp.name)
		}
		result, err := machine.Run(bp.proto, eval.NewCapEnv(nil))
		if err != nil {
			if budget.IsLimitError(err) || strings.Contains(bp.name, "$") {
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
		namedGlobals[cells[i].key] = v
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
