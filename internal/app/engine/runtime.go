package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
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

// assignGlobalSlots collects all global names and assigns each a slot index.
// Then walks all IR (module + main) to convert Var.Index = -1 to -(slot+2).
// This completes the de Bruijn unification: all variables are integer-indexed.
func (r *Runtime) assignGlobalSlots() {
	slots := make(map[string]int, len(r.builtinGlobals)+len(r.bindings))

	// Assign slots from builtinGlobals (builtins + constructors).
	for k := range r.builtinGlobals {
		slots[k] = len(slots)
	}
	// Host binding names (values provided at RunWith time).
	for name := range r.bindings {
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

	// Walk all IR to assign global slot indices.
	for _, me := range r.moduleEntries {
		ir.AssignGlobalSlotsProgram(me.prog, slots)
	}
	ir.AssignGlobalSlotsProgram(r.prog, slots)
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

// execute is the unified execution core.
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

	ev := eval.NewEvaluator(b, r.prims, req.traceHook, req.obs, r.source)
	ev.SetGlobalArray(globalArray)
	ev.SetGlobals(r.buildNamedGlobals(req.bindings))

	for _, me := range r.moduleEntries {
		ev.SetSource(me.source)
		if err := r.evalBindingsCore(ev, me.sortedBindings, me.name, req.obs); err != nil {
			return eval.EvalResult{}, ev.Stats(), err
		}
	}
	ev.SetSource(r.source)

	var entryExpr ir.Core
	var nonEntry []ir.Binding
	if req.entry == r.entryName && r.entryExpr != nil {
		entryExpr = r.entryExpr
		nonEntry = make([]ir.Binding, 0, len(r.sortedMainBindings)-1)
		for _, b := range r.sortedMainBindings {
			if b.Name != r.entryName {
				nonEntry = append(nonEntry, b)
			}
		}
	} else {
		for _, b := range r.sortedMainBindings {
			if b.Name == req.entry {
				entryExpr = b.Expr
			} else {
				nonEntry = append(nonEntry, b)
			}
		}
	}

	if err := r.evalBindingsCore(ev, nonEntry, "", req.obs); err != nil {
		return eval.EvalResult{}, ev.Stats(), err
	}

	if entryExpr == nil {
		return eval.EvalResult{}, ev.Stats(), fmt.Errorf("entry point %q not found", req.entry)
	}

	if req.obs != nil {
		req.obs.Section(req.entry)
	}
	capEnv := eval.NewCapEnv(req.caps)
	result, err := ev.Eval(nil, capEnv, entryExpr)
	if err != nil {
		return eval.EvalResult{}, ev.Stats(), err
	}
	result, err = ev.ForceEffectful(result, span.Span{})
	if err != nil {
		return eval.EvalResult{}, ev.Stats(), err
	}
	if req.obs != nil {
		req.obs.Result(eval.PrettyValue(result.Value))
	}
	return result, ev.Stats(), nil
}

// evalBindingsCore evaluates a slice of pre-sorted bindings.
// Callers must pass bindings in dependency order (via ir.SortBindings).
// Values are stored directly in the evaluator's global array at pre-assigned slots.
func (r *Runtime) evalBindingsCore(ev *eval.Evaluator, bindings []ir.Binding, modulePrefix string, obs *eval.ExplainObserver) error {
	// Phase 1: place IndirectVal sentinels for forward references.
	type slotCell struct {
		slot int
		key  string
		cell *eval.IndirectVal
	}
	cells := make([]slotCell, len(bindings))
	globals := ev.Globals()
	for i, b := range bindings {
		key := b.Name
		if modulePrefix != "" {
			key = ir.QualifiedKey(modulePrefix, b.Name)
		}
		slot := r.globalSlots[key]
		cell := &eval.IndirectVal{}
		ev.SetGlobalSlot(slot, cell)
		globals[key] = cell
		cells[i] = slotCell{slot, key, cell}
	}
	// Phase 2: evaluate and fill slots.
	userVisible := modulePrefix == ""
	for i, b := range bindings {
		if userVisible {
			obs.Section(b.Name)
		}
		result, err := ev.Eval(nil, eval.NewCapEnv(nil), b.Expr)
		if err != nil {
			if strings.Contains(b.Name, "$") {
				return err
			}
			return fmt.Errorf("evaluating %s: %w", b.Name, err)
		}
		v := result.Value
		if clo, ok := v.(*eval.Closure); ok {
			clo.Name = b.Name
			if !userVisible {
				obs.MarkInternal(b.Name)
			}
		}
		// Fill IndirectVal (for closures that captured the cell) and replace slot.
		val := v
		cells[i].cell.Ref = &val
		ev.SetGlobalSlot(cells[i].slot, v)
		globals[cells[i].key] = v
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
