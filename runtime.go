package gicel

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// Runtime is an immutable, compiled GICEL program.
// It is goroutine-safe and can be executed concurrently.
type Runtime struct {
	prog        *core.Program
	prims       *eval.PrimRegistry
	stepLimit   int
	depthLimit  int
	allocLimit  int64
	source      *span.Source
	bindings    map[string]types.Type
	moduleProgs []*core.Program
	builtinEnv  *eval.Env // pre-built pure/bind/force/fix/rec closures
}

// Program returns the compiled Core IR for debugging/inspection.
func (r *Runtime) Program() *CoreProgram {
	return &CoreProgram{prog: r.prog}
}

// RunResult holds the result of a single execution.
type RunResult struct {
	Value Value
	Stats EvalStats
}

// RunResultFull holds the full result of an execution including the final capability environment.
type RunResultFull struct {
	Value  Value
	CapEnv CapEnv
	Stats  EvalStats
}

// initBuiltinEnv constructs the immutable base environment with builtins and constructors.
// Called once at NewRuntime time; the result is shared across all executions.
func (r *Runtime) initBuiltinEnv(gatedBuiltins map[string]bool) {
	env := eval.BuiltinEnv(gatedBuiltins["fix"], gatedBuiltins["rec"])

	// Constructors from imported modules.
	for _, modProg := range r.moduleProgs {
		for _, d := range modProg.DataDecls {
			for _, con := range d.Cons {
				env = env.Extend(con.Name, &eval.ConVal{Con: con.Name})
			}
		}
	}

	// Constructors from main program.
	for _, d := range r.prog.DataDecls {
		for _, con := range d.Cons {
			env = env.Extend(con.Name, &eval.ConVal{Con: con.Name})
		}
	}

	r.builtinEnv = env
}

// buildEnv extends the pre-built builtin environment with user-provided bindings.
func (r *Runtime) buildEnv(bindings map[string]Value) (*eval.Env, error) {
	env := r.builtinEnv
	for name := range r.bindings {
		v, ok := bindings[name]
		if !ok {
			return nil, fmt.Errorf("missing binding: %s", name)
		}
		env = env.Extend(name, v)
	}
	return env, nil
}

// execute is the unified execution core.
// All public Run methods delegate here, differing only in how the
// ExplainObserver is constructed (or left nil).
func (r *Runtime) execute(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string, obs *eval.ExplainObserver, traceHook eval.TraceHook) (eval.EvalResult, EvalStats, error) {
	env, err := r.buildEnv(bindings)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	limit := eval.NewLimit(r.stepLimit, r.depthLimit)
	if r.allocLimit > 0 {
		limit.SetAllocLimit(r.allocLimit)
	}

	ev := eval.NewEvaluator(ctx, r.prims, limit, traceHook, obs)

	// Module bindings (internal — suppressed in explain).
	for _, modProg := range r.moduleProgs {
		env, err = r.evalBindings(ev, env, modProg.Bindings, false, obs)
		if err != nil {
			return eval.EvalResult{}, EvalStats{}, err
		}
	}

	// User bindings.
	var entryExpr core.Core
	var nonEntry []core.Binding
	for _, b := range r.prog.Bindings {
		if b.Name == entry {
			entryExpr = b.Expr
		} else {
			nonEntry = append(nonEntry, b)
		}
	}

	env, err = r.evalBindings(ev, env, nonEntry, true, obs)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	if entryExpr == nil {
		return eval.EvalResult{}, EvalStats{}, fmt.Errorf("entry point %q not found", entry)
	}

	if obs != nil {
		obs.Section(entry)
	}
	capEnv := eval.NewCapEnv(caps)
	result, err := ev.Eval(env, capEnv, entryExpr)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}
	result, err = ev.ForceEffectful(result, span.Span{})
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}
	if obs != nil {
		obs.Result(eval.PrettyValue(result.Value))
	}
	return result, ev.Stats(), nil
}

// evalBindings evaluates a slice of bindings using forward-reference cells.
// The provided Evaluator is shared with the caller so that resource limits
// (steps, allocation, depth) are enforced cumulatively across all bindings.
// When label is true, explain events mark each binding's evaluation boundary
// and closures are treated as user-visible; otherwise they are internal.
func (r *Runtime) evalBindings(ev *eval.Evaluator, env *eval.Env, bindings []core.Binding, label bool, obs *eval.ExplainObserver) (*eval.Env, error) {
	bindings = core.SortBindings(bindings)
	cells := make(map[string]*eval.IndirectVal, len(bindings))
	for _, b := range bindings {
		cell := &eval.IndirectVal{}
		cells[b.Name] = cell
		env = env.Extend(b.Name, cell)
	}
	for _, b := range bindings {
		if label && obs != nil {
			obs.Section(b.Name)
		}
		result, err := ev.Eval(env, eval.NewCapEnv(nil), b.Expr)
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", b.Name, err)
		}
		v := result.Value
		if clo, ok := v.(*eval.Closure); ok {
			clo.Name = b.Name
			if !label {
				obs.MarkInternal(b.Name)
			}
		}
		cells[b.Name].Ref = &v
	}
	return env, nil
}

// run delegates to execute without hooks. Used by RunContext/RunContextFull.
func (r *Runtime) run(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string) (eval.EvalResult, EvalStats, error) {
	return r.execute(ctx, caps, bindings, entry, nil, nil)
}

// RunWith executes the program with per-execution options.
// Explain and trace hooks are scoped to this execution, not shared with
// the Runtime. This is the preferred API for agent and LSP integration.
func (r *Runtime) RunWith(ctx context.Context, opts *RunOptions) (*RunResultFull, error) {
	if opts == nil {
		opts = &RunOptions{}
	}
	entry := opts.Entry
	if entry == "" {
		entry = "main"
	}
	var obs *eval.ExplainObserver
	if opts.Explain != nil {
		obs = eval.NewExplainObserver(opts.Explain, r.source)
		if opts.ExplainDepth == ExplainAll {
			obs.SetAll(true)
		}
	}
	result, stats, err := r.execute(ctx, opts.Caps, opts.Bindings, entry, obs, opts.Trace)
	if err != nil {
		return nil, err
	}
	return &RunResultFull{Value: result.Value, CapEnv: result.CapEnv, Stats: stats}, nil
}

// RunContext executes the program with the given capabilities and bindings.
// entry specifies the top-level binding to evaluate (typically "main").
func (r *Runtime) RunContext(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string) (*RunResult, error) {
	result, stats, err := r.run(ctx, caps, bindings, entry)
	if err != nil {
		return nil, err
	}
	return &RunResult{Value: result.Value, Stats: stats}, nil
}

// RunContextFull is like RunContext but also returns the final capability environment.
func (r *Runtime) RunContextFull(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string) (*RunResultFull, error) {
	result, stats, err := r.run(ctx, caps, bindings, entry)
	if err != nil {
		return nil, err
	}
	return &RunResultFull{Value: result.Value, CapEnv: result.CapEnv, Stats: stats}, nil
}
