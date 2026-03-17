package gicel

import (
	"context"
	"errors"
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
)

// annotateError populates Line/Col on RuntimeError from the Source.
func (r *Runtime) annotateError(err error) error {
	var re *eval.RuntimeError
	if errors.As(err, &re) && r.source != nil && re.Span != (span.Span{}) {
		re.Line, re.Col = r.source.Location(re.Span.Start)
	}
	return err
}

// Runtime is an immutable, compiled GICEL program.
// It is goroutine-safe and can be executed concurrently.
type Runtime struct {
	prog          *core.Program
	prims         *eval.PrimRegistry
	stepLimit     int
	depthLimit    int
	allocLimit    int64
	source        *span.Source
	bindings      map[string]types.Type
	moduleEntries []moduleEntry // module programs in registration order
	builtinEnv    *eval.Env     // pre-built pure/bind/force/fix/rec closures
}

// moduleEntry pairs a module name with its compiled program.
type moduleEntry struct {
	name          string
	prog          *core.Program
	qualifiedOnly bool // true if imported as qualified (no plain-name registration)
}

// Program returns the compiled Core IR for debugging/inspection.
func (r *Runtime) Program() *CoreProgram {
	return &CoreProgram{prog: r.prog}
}

// RunResult holds the result of an execution.
type RunResult struct {
	Value  Value
	CapEnv CapEnv
	Stats  EvalStats
}

// initBuiltinEnv constructs the immutable base environment with builtins and constructors.
// Called once at NewRuntime time; the result is shared across all executions.
func (r *Runtime) initBuiltinEnv(gatedBuiltins map[string]bool) {
	env := eval.BuiltinEnv(gatedBuiltins["fix"], gatedBuiltins["rec"])

	// Constructors from imported modules (dual registration: plain + qualified key).
	for _, me := range r.moduleEntries {
		for _, d := range me.prog.DataDecls {
			for _, con := range d.Cons {
				cv := &eval.ConVal{Con: con.Name}
				env = env.Extend(con.Name, cv)
				env = env.Extend(me.name+"\x00"+con.Name, cv)
			}
		}
	}
	// Re-register open-imported constructors so their plain names win.
	for _, me := range r.moduleEntries {
		if me.qualifiedOnly {
			continue
		}
		for _, d := range me.prog.DataDecls {
			for _, con := range d.Cons {
				if v, ok := env.Lookup(me.name + "\x00" + con.Name); ok {
					env = env.Extend(con.Name, v)
				}
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

// runRequest groups per-execution parameters for the execute method.
type runRequest struct {
	caps      map[string]any
	bindings  map[string]Value
	entry     string
	obs       *eval.ExplainObserver
	traceHook eval.TraceHook
}

// execute is the unified execution core.
func (r *Runtime) execute(ctx context.Context, req *runRequest) (eval.EvalResult, EvalStats, error) {
	env, err := r.buildEnv(req.bindings)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	limit := eval.NewLimit(r.stepLimit, r.depthLimit)
	if r.allocLimit > 0 {
		limit.SetAllocLimit(r.allocLimit)
	}

	ev := eval.NewEvaluator(ctx, r.prims, limit, req.traceHook, req.obs)

	// Module bindings (internal — suppressed in explain).
	// All modules evaluated in registration order (preserves dependency ordering).
	// Each binding is registered under both plain name and qualified key.
	for _, me := range r.moduleEntries {
		env, err = r.evalBindingsCore(ev, env, me.prog.Bindings, me.name, false, req.obs)
		if err != nil {
			return eval.EvalResult{}, EvalStats{}, err
		}
	}

	// Re-register open-imported bindings so their plain names shadow
	// any qualified-only module's plain names from the step above.
	for _, me := range r.moduleEntries {
		if me.qualifiedOnly {
			continue
		}
		for _, b := range me.prog.Bindings {
			if v, ok := env.Lookup(me.name + "\x00" + b.Name); ok {
				env = env.Extend(b.Name, v)
			}
		}
	}

	// User bindings.
	var entryExpr core.Core
	var nonEntry []core.Binding
	for _, b := range r.prog.Bindings {
		if b.Name == req.entry {
			entryExpr = b.Expr
		} else {
			nonEntry = append(nonEntry, b)
		}
	}

	env, err = r.evalBindingsCore(ev, env, nonEntry, "", true, req.obs)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}

	if entryExpr == nil {
		return eval.EvalResult{}, EvalStats{}, fmt.Errorf("entry point %q not found", req.entry)
	}

	if req.obs != nil {
		req.obs.Section(req.entry)
	}
	capEnv := eval.NewCapEnv(req.caps)
	result, err := ev.Eval(env, capEnv, entryExpr)
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}
	result, err = ev.ForceEffectful(result, span.Span{})
	if err != nil {
		return eval.EvalResult{}, EvalStats{}, err
	}
	if req.obs != nil {
		req.obs.Result(eval.PrettyValue(result.Value))
	}
	return result, ev.Stats(), nil
}

// evalBindingsCore evaluates a slice of bindings using forward-reference cells.
// modulePrefix is "" for user bindings or "ModuleName" for module bindings.
// When modulePrefix is non-empty, each binding is also registered under a
// qualified key (module\x00name) for qualified import resolution.
// When userVisible is true, explain events mark each binding's evaluation
// boundary; otherwise closures are marked as internal.
func (r *Runtime) evalBindingsCore(ev *eval.Evaluator, env *eval.Env, bindings []core.Binding, modulePrefix string, userVisible bool, obs *eval.ExplainObserver) (*eval.Env, error) {
	bindings = core.SortBindings(bindings)
	cells := make(map[string]*eval.IndirectVal, len(bindings))
	for _, b := range bindings {
		cell := &eval.IndirectVal{}
		cells[b.Name] = cell
		env = env.Extend(b.Name, cell)
		if modulePrefix != "" {
			env = env.Extend(modulePrefix+"\x00"+b.Name, cell)
		}
	}
	for _, b := range bindings {
		if userVisible {
			obs.Section(b.Name)
		}
		result, err := ev.Eval(env, eval.NewCapEnv(nil), b.Expr)
		if err != nil {
			return nil, fmt.Errorf("evaluating %s: %w", b.Name, err)
		}
		v := result.Value
		if clo, ok := v.(*eval.Closure); ok {
			clo.Name = b.Name
			if !userVisible {
				obs.MarkInternal(b.Name)
			}
		}
		cells[b.Name].Ref = &v
	}
	return env, nil
}

// RunWith executes the program with the given options.
// A nil opts is equivalent to &RunOptions{} (entry "main", no hooks).
func (r *Runtime) RunWith(ctx context.Context, opts *RunOptions) (*RunResult, error) {
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
	result, stats, err := r.execute(ctx, &runRequest{
		caps:      opts.Caps,
		bindings:  opts.Bindings,
		entry:     entry,
		obs:       obs,
		traceHook: opts.Trace,
	})
	if err != nil {
		return nil, r.annotateError(err)
	}
	return &RunResult{Value: result.Value, CapEnv: result.CapEnv, Stats: stats}, nil
}
