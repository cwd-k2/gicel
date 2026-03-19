package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
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
	prog               *core.Program
	prims              *eval.PrimRegistry
	stepLimit          int
	depthLimit         int
	nestingLimit       int
	allocLimit         int64
	source             *span.Source
	bindings           map[string]types.Type
	moduleEntries      []moduleEntry  // ALL module programs in registration order
	builtinEnv         *eval.Env      // pre-built pure/bind/force/fix/rec closures
	sortedMainBindings []core.Binding // all main bindings, topologically pre-sorted
	entryName          string         // default entry point name
	entryExpr          core.Core      // default entry point expression (nil if not found)
}

type moduleEntry struct {
	name           string
	prog           *core.Program
	sortedBindings []core.Binding  // pre-sorted bindings for evaluation
	source         *span.Source    // source text for error attribution
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

// initBuiltinEnv constructs the immutable base environment with builtins and constructors.
func (r *Runtime) initBuiltinEnv(gatedBuiltins map[string]bool) {
	env := eval.BuiltinEnv(gatedBuiltins["fix"], gatedBuiltins["rec"])

	for _, me := range r.moduleEntries {
		for _, d := range me.prog.DataDecls {
			for _, con := range d.Cons {
				cv := &eval.ConVal{Con: con.Name}
				env = env.Extend(core.QualifiedKey(me.name, con.Name), cv)
			}
		}
	}

	for _, d := range r.prog.DataDecls {
		for _, con := range d.Cons {
			env = env.Extend(con.Name, &eval.ConVal{Con: con.Name})
		}
	}

	env.Flatten()
	r.builtinEnv = env
}

// buildEnv extends the pre-built builtin environment with user-provided bindings.
func (r *Runtime) buildEnv(bindings map[string]eval.Value) (*eval.Env, error) {
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

type runRequest struct {
	caps      map[string]any
	bindings  map[string]eval.Value
	entry     string
	obs       *eval.ExplainObserver
	traceHook eval.TraceHook
}

// execute is the unified execution core.
func (r *Runtime) execute(ctx context.Context, req *runRequest) (eval.EvalResult, eval.EvalStats, error) {
	env, err := r.buildEnv(req.bindings)
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

	for _, me := range r.moduleEntries {
		ev.SetSource(me.source)
		env, err = r.evalBindingsCore(ev, env, me.sortedBindings, me.name, req.obs)
		if err != nil {
			return eval.EvalResult{}, eval.EvalStats{}, err
		}
	}
	ev.SetSource(r.source)

	var entryExpr core.Core
	var nonEntry []core.Binding
	if req.entry == r.entryName && r.entryExpr != nil {
		entryExpr = r.entryExpr
		nonEntry = make([]core.Binding, 0, len(r.sortedMainBindings)-1)
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

	env, err = r.evalBindingsCore(ev, env, nonEntry, "", req.obs)
	if err != nil {
		return eval.EvalResult{}, eval.EvalStats{}, err
	}

	if entryExpr == nil {
		return eval.EvalResult{}, eval.EvalStats{}, fmt.Errorf("entry point %q not found", req.entry)
	}

	if req.obs != nil {
		req.obs.Section(req.entry)
	}
	capEnv := eval.NewCapEnv(req.caps)
	result, err := ev.Eval(env, capEnv, entryExpr)
	if err != nil {
		return eval.EvalResult{}, eval.EvalStats{}, err
	}
	result, err = ev.ForceEffectful(result, span.Span{})
	if err != nil {
		return eval.EvalResult{}, eval.EvalStats{}, err
	}
	if req.obs != nil {
		req.obs.Result(eval.PrettyValue(result.Value))
	}
	return result, ev.Stats(), nil
}

// evalBindingsCore evaluates a slice of pre-sorted bindings using forward-reference cells.
// Callers must pass bindings in dependency order (via core.SortBindings).
func (r *Runtime) evalBindingsCore(ev *eval.Evaluator, env *eval.Env, bindings []core.Binding, modulePrefix string, obs *eval.ExplainObserver) (*eval.Env, error) {
	cells := make(map[string]*eval.IndirectVal, len(bindings))
	for _, b := range bindings {
		cell := &eval.IndirectVal{}
		cells[b.Name] = cell
		if modulePrefix != "" {
			env = env.Extend(core.QualifiedKey(modulePrefix, b.Name), cell)
		} else {
			env = env.Extend(b.Name, cell)
		}
	}
	userVisible := modulePrefix == ""
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
		return nil, r.annotateError(err)
	}
	return &RunResult{Value: result.Value, CapEnv: result.CapEnv, Stats: stats}, nil
}
