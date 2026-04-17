package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

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
	// Warn receives runtime warnings, currently emitted only when
	// RunOptions.Bindings contains names not declared via DeclareBinding
	// (likely typos). Nil falls back to os.Stderr. Per-call by design:
	// *Runtime is shared via the process-global cache, so a callback
	// stored on the Runtime would leak warnings across embedders.
	Warn func(string)
}

// Runtime is an immutable, compiled GICEL program.
// It is goroutine-safe and can be executed concurrently.
type Runtime struct {
	prog               *ir.Program
	annots             *ir.FVAnnotations // FV metadata for main bindings
	prims              *eval.PrimRegistry
	typeOps            *types.TypeOps
	stepLimit          int
	depthLimit         int
	nestingLimit       int
	allocLimit         int64
	source             *span.Source
	bindings           map[string]types.Type
	moduleEntries      []moduleEntry            // ALL module programs in registration order
	builtinGlobals     map[ir.VarKey]eval.Value // pre-built pure/bind/force/fix/rec closures + constructors
	globalSlots        map[ir.VarKey]int        // global name → slot index (assigned at NewRuntime)
	numGlobals         int                      // total number of global slots
	sortedMainBindings []ir.Binding             // all main bindings, topologically pre-sorted
	entryName          string                   // default entry point name
	entryExpr          ir.Core                  // default entry point expression (nil if not found)

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
	cachedGlobals     atomic.Pointer[[]eval.Value]
	cachedGlobalsOnce sync.Once
}

type vmBindingProto struct {
	name      string
	proto     *vm.Proto
	generated ir.GenKind // non-zero when introduced by the compiler
}

type moduleEntry struct {
	name           string
	prog           *ir.Program
	annots         *ir.FVAnnotations // FV metadata shared with prog
	sortedBindings []ir.Binding      // pre-sorted bindings for evaluation
	source         *span.Source      // source text for error attribution
}

// Program returns the compiled Core IR for debugging/inspection.
func (r *Runtime) Program() *CoreProgram {
	return &CoreProgram{prog: r.prog, typeOps: r.typeOps}
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
	if errors.As(err, &re) && !re.Span.IsZero() {
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

type runRequest struct {
	caps      map[string]any
	bindings  map[string]eval.Value
	entry     string
	obs       *eval.ExplainObserver
	traceHook eval.TraceHook
	warn      func(string) // nil = stderr
}

// execute runs the program using the bytecode VM.
func (r *Runtime) execute(ctx context.Context, req *runRequest) (eval.EvalResult, eval.EvalStats, error) {
	// Catch pre-cancelled contexts before allocating execution state.
	// With amortized context checking in budget.Step(), programs with
	// fewer than 64 steps might never poll the channel.
	if err := ctx.Err(); err != nil {
		return eval.EvalResult{}, eval.EvalStats{}, budget.WrapCtxErr(err)
	}

	globalArray, err := r.buildGlobalArray(req.bindings, req.warn)
	if err != nil {
		return eval.EvalResult{}, eval.EvalStats{}, err
	}

	b := budget.New(ctx, r.stepLimit, r.depthLimit)
	if r.allocLimit > 0 {
		b.SetAllocLimit(r.allocLimit)
	}

	capEnv := eval.NewCapEnv(req.caps)

	// Embed budget in context so stdlib primitives can call budget.ChargeAlloc.
	vmCtx := budget.ContextWithBudget(ctx, b)
	return r.executeVM(vmCtx, b, globalArray, capEnv, req)
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

	if cached := r.cachedGlobals.Load(); cached != nil {
		// Fast path: clone cached template. Module bindings are pure
		// and referentially transparent — their results never change.
		copy(globalArray, *cached)
		// Re-install host bindings which may differ across RunWith calls.
		// buildGlobalArray already validated and set them in globalArray,
		// but copy() overwrote those slots with the cached (first-run) values.
		for name := range r.bindings {
			if v, ok := req.bindings[name]; ok {
				globalArray[r.globalSlots[ir.LocalKey(name)]] = v
			}
		}
	} else {
		// First execution: evaluate module + main bindings, then cache.

		// Install pre-compiled builtins.
		for name, val := range r.vmBuiltins {
			if slot, ok := r.globalSlots[ir.LocalKey(name)]; ok {
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
		// Store via atomic.Pointer so concurrent readers at the fast path
		// observe a fully published slice header (Go memory model compliance).
		r.cachedGlobalsOnce.Do(func() {
			template := make([]eval.Value, len(globalArray))
			copy(template, globalArray)
			r.cachedGlobals.Store(&template)
		})
	}

	// Handle non-default entry point.
	var entryProto *vm.Proto
	if req.entry == r.entryName {
		entryProto = r.vmEntryProto
	} else {
		compiler := vm.NewCompiler(r.globalSlots, r.source)
		compiler.SetFVAnnots(r.annots)
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

// RunWith executes the program with the given options.
//
// Nil-safe: returns a typed error (not a panic) when called on a nil Runtime
// or with a nil context. opts may be nil (treated as zero-value options).
func (r *Runtime) RunWith(ctx context.Context, opts *RunOptions) (*RunResult, error) {
	if r == nil {
		return nil, fmt.Errorf("engine: RunWith called on nil *Runtime")
	}
	if ctx == nil {
		return nil, fmt.Errorf("engine: RunWith called with nil context")
	}
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
		warn:      opts.Warn,
	})
	if err != nil {
		return &RunResult{Stats: stats}, r.annotateError(err)
	}
	return &RunResult{Value: result.Value, CapEnv: result.CapEnv, Stats: stats}, nil
}
