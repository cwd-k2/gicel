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
	traceHook   eval.TraceHook
	explainHook eval.ExplainHook
	explainSeq  int // shared monotonic counter for explain events
	source      *span.Source
	bindings    map[string]types.Type
	moduleProgs []*core.Program
	builtinEnv  *eval.Env // pre-built pure/bind/force/fix/rec closures
}

// Program returns the compiled Core IR for debugging/inspection.
func (r *Runtime) Program() *CoreProgram {
	return &CoreProgram{prog: r.prog}
}

// emitExplain sends an ExplainStep to the hook with an auto-incremented seq.
func (r *Runtime) emitExplain(step eval.ExplainStep) {
	r.explainSeq++
	step.Seq = r.explainSeq
	r.explainHook(step)
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

// runWith is the per-execution implementation with local hooks and seq.
func (r *Runtime) runWith(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string, explainHook eval.ExplainHook, traceHook eval.TraceHook, explainDepth ExplainDepth) (eval.EvalResult, EvalStats, error) {
	env, err := r.buildEnv(bindings)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	limit := eval.NewLimit(r.stepLimit, r.depthLimit)
	if r.allocLimit > 0 {
		limit.SetAllocLimit(r.allocLimit)
	}

	// Per-execution seq counter — no shared state with Runtime.
	var seq int
	emit := func(step eval.ExplainStep) {
		seq++
		step.Seq = seq
		explainHook(step)
	}

	ev := eval.NewEvaluator(ctx, r.prims, limit, traceHook, explainHook)
	ev.SetSource(r.source)
	if explainDepth == ExplainAll {
		ev.SetSuppressDisabled(true)
	}

	for _, modProg := range r.moduleProgs {
		env, err = r.evalBindingsWith(ev, env, modProg.Bindings, false, nil)
		if err != nil {
			return eval.EvalResult{}, EvalStats{}, err
		}
	}

	var entryExpr core.Core
	var nonEntry []core.Binding
	for _, b := range r.prog.Bindings {
		if b.Name == entry {
			entryExpr = b.Expr
		} else {
			nonEntry = append(nonEntry, b)
		}
	}

	env, err = r.evalBindingsWith(ev, env, nonEntry, true, emit)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	if entryExpr == nil {
		return eval.EvalResult{}, EvalStats{}, fmt.Errorf("entry point %q not found", entry)
	}

	if explainHook != nil {
		seq = ev.ExplainSeq()
		emit(eval.ExplainStep{
			Kind:    eval.ExplainLabel,
			Message: "── " + entry + " ──",
			Detail:  eval.LabelDetail(entry, "section"),
		})
		ev.SetExplainSeq(seq)
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
	if explainHook != nil {
		seq = ev.ExplainSeq()
		val := eval.PrettyValue(result.Value)
		emit(eval.ExplainStep{
			Kind:    eval.ExplainResult,
			Message: "→ " + val,
			Detail:  eval.ResultDetail(val),
		})
	}
	return result, ev.Stats(), nil
}

// run is the shared implementation for RunContext and RunContextFull.
func (r *Runtime) run(ctx context.Context, caps map[string]any, bindings map[string]Value, entry string) (eval.EvalResult, EvalStats, error) {
	env, err := r.buildEnv(bindings)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	// Single Evaluator shared across all binding evaluations and entry execution.
	// This ensures step/alloc/depth limits are cumulative, not per-binding.
	limit := eval.NewLimit(r.stepLimit, r.depthLimit)
	if r.allocLimit > 0 {
		limit.SetAllocLimit(r.allocLimit)
	}
	ev := eval.NewEvaluator(ctx, r.prims, limit, r.traceHook, r.explainHook)
	ev.SetSource(r.source)
	ev.SetExplainSeq(r.explainSeq) // sync seq from runtime → evaluator

	// Evaluate module bindings first (in registration order).
	for _, modProg := range r.moduleProgs {
		env, err = r.evalBindings(ev, env, modProg.Bindings, false)
		if err != nil {
			return eval.EvalResult{}, EvalStats{}, err
		}
	}

	// Pre-populate env with forward-reference cells for all non-entry bindings.
	var entryExpr core.Core
	var nonEntry []core.Binding
	for _, b := range r.prog.Bindings {
		if b.Name == entry {
			entryExpr = b.Expr
		} else {
			nonEntry = append(nonEntry, b)
		}
	}

	env, err = r.evalBindings(ev, env, nonEntry, true)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	if entryExpr == nil {
		return eval.EvalResult{}, EvalStats{}, fmt.Errorf("entry point %q not found", entry)
	}

	if r.explainHook != nil {
		r.explainSeq = ev.ExplainSeq()
		r.emitExplain(eval.ExplainStep{
			Kind:    eval.ExplainLabel,
			Message: "── " + entry + " ──",
			Detail:  eval.LabelDetail(entry, "section"),
		})
		ev.SetExplainSeq(r.explainSeq)
	}
	capEnv := eval.NewCapEnv(caps)
	result, err := ev.Eval(env, capEnv, entryExpr)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}
	// Force effectful PrimVals at top-level (e.g. main := get).
	result, err = ev.ForceEffectful(result, span.Span{})
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}
	if r.explainHook != nil {
		r.explainSeq = ev.ExplainSeq() // sync seq from evaluator → runtime
		val := eval.PrettyValue(result.Value)
		r.emitExplain(eval.ExplainStep{
			Kind:    eval.ExplainResult,
			Message: "→ " + val,
			Detail:  eval.ResultDetail(val),
		})
	}
	return result, ev.Stats(), nil
}

// evalBindingsWith is like evalBindings but uses a per-call emit function
// for explain events, decoupling explain state from the Runtime.
func (r *Runtime) evalBindingsWith(ev *eval.Evaluator, env *eval.Env, bindings []core.Binding, label bool, emit func(eval.ExplainStep)) (*eval.Env, error) {
	bindings = core.SortBindings(bindings)
	cells := make(map[string]*eval.IndirectVal, len(bindings))
	for _, b := range bindings {
		cell := &eval.IndirectVal{}
		cells[b.Name] = cell
		env = env.Extend(b.Name, cell)
	}
	for _, b := range bindings {
		if label && emit != nil {
			seq := ev.ExplainSeq()
			emit(eval.ExplainStep{
				Kind:    eval.ExplainLabel,
				Message: "── " + b.Name + " ──",
				Detail:  eval.LabelDetail(b.Name, "section"),
			})
			ev.SetExplainSeq(seq + 1) // sync back
		}
		result, err := ev.Eval(env, eval.NewCapEnv(nil), b.Expr)
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", b.Name, err)
		}
		v := result.Value
		if clo, ok := v.(*eval.Closure); ok {
			clo.Name = b.Name
			clo.Internal = !label
		}
		cells[b.Name].Ref = &v
	}
	return env, nil
}

// evalBindings evaluates a slice of bindings using forward-reference cells.
// The provided Evaluator is shared with the caller so that resource limits
// (steps, allocation, depth) are enforced cumulatively across all bindings.
// When label is true, explain events mark each binding's evaluation boundary.
func (r *Runtime) evalBindings(ev *eval.Evaluator, env *eval.Env, bindings []core.Binding, label bool) (*eval.Env, error) {
	// Sort bindings by dependency so that each binding is evaluated
	// after the bindings it references, eliminating forward-reference
	// errors for non-cyclic dependencies.
	bindings = core.SortBindings(bindings)
	cells := make(map[string]*eval.IndirectVal, len(bindings))
	for _, b := range bindings {
		cell := &eval.IndirectVal{}
		cells[b.Name] = cell
		env = env.Extend(b.Name, cell)
	}
	for _, b := range bindings {
		if label && r.explainHook != nil {
			r.explainSeq = ev.ExplainSeq()
			r.emitExplain(eval.ExplainStep{
				Kind:    eval.ExplainLabel,
				Message: "── " + b.Name + " ──",
				Detail:  eval.LabelDetail(b.Name, "section"),
			})
			ev.SetExplainSeq(r.explainSeq)
		}
		result, err := ev.Eval(env, eval.NewCapEnv(nil), b.Expr)
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", b.Name, err)
		}
		v := result.Value
		// Annotate closures with their binding name and module origin.
		if clo, ok := v.(*eval.Closure); ok {
			clo.Name = b.Name
			clo.Internal = !label // module bindings are internal
		}
		cells[b.Name].Ref = &v
	}
	return env, nil
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
	result, stats, err := r.runWith(ctx, opts.Caps, opts.Bindings, entry, opts.Explain, opts.Trace, opts.ExplainDepth)
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
