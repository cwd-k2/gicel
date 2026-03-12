package eval

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gomputation/internal/core"
)

// EvalResult is the result of evaluation.
type EvalResult struct {
	Value  Value
	CapEnv CapEnv
}

// Evaluator is the per-execution evaluation engine.
type Evaluator struct {
	ctx   context.Context
	prims *PrimRegistry
	limit *Limit
	trace TraceHook
	stats EvalStats
}

// NewEvaluator creates an Evaluator for a single execution.
func NewEvaluator(ctx context.Context, prims *PrimRegistry, limit *Limit, trace TraceHook) *Evaluator {
	return &Evaluator{ctx: ctx, prims: prims, limit: limit, trace: trace}
}

// Stats returns the accumulated statistics.
func (ev *Evaluator) Stats() EvalStats {
	return ev.stats
}

// Eval evaluates a Core expression.
func (ev *Evaluator) Eval(env *Env, capEnv CapEnv, expr core.Core) (EvalResult, error) {
	// Check context cancellation.
	select {
	case <-ev.ctx.Done():
		return EvalResult{}, ev.ctx.Err()
	default:
	}

	// Check step limit.
	if err := ev.limit.Step(); err != nil {
		return EvalResult{}, err
	}

	// Update stats.
	ev.stats.Steps++
	if d := ev.limit.Depth(); d > ev.stats.MaxDepth {
		ev.stats.MaxDepth = d
	}

	// Trace hook.
	if ev.trace != nil {
		if err := ev.trace(TraceEvent{
			Depth: ev.limit.Depth(), Node: expr, Env: env, CapEnv: capEnv,
		}); err != nil {
			return EvalResult{}, err
		}
	}

	switch e := expr.(type) {
	case *core.Var:
		v, ok := env.Lookup(e.Name)
		if !ok {
			return EvalResult{}, &RuntimeError{Message: fmt.Sprintf("unbound variable: %s", e.Name), Span: e.S}
		}
		// Dereference forward-reference cells (used for mutually-recursive top-level bindings).
		if ind, ok := v.(*IndirectVal); ok {
			if ind.Ref == nil {
				return EvalResult{}, &RuntimeError{Message: fmt.Sprintf("uninitialized forward reference: %s", e.Name), Span: e.S}
			}
			return EvalResult{*ind.Ref, capEnv}, nil
		}
		return EvalResult{v, capEnv}, nil

	case *core.Lam:
		return EvalResult{&Closure{Env: env, Param: e.Param, Body: e.Body}, capEnv}, nil

	case *core.App:
		funR, err := ev.Eval(env, capEnv, e.Fun)
		if err != nil {
			return EvalResult{}, err
		}
		argR, err := ev.Eval(env, funR.CapEnv, e.Arg)
		if err != nil {
			return EvalResult{}, err
		}
		return ev.apply(argR.CapEnv, funR.Value, argR.Value, e)

	case *core.TyApp:
		// Type application is erased at runtime.
		return ev.Eval(env, capEnv, e.Expr)

	case *core.TyLam:
		// Type abstraction is erased at runtime.
		return ev.Eval(env, capEnv, e.Body)

	case *core.Con:
		args := make([]Value, len(e.Args))
		ce := capEnv
		for i, arg := range e.Args {
			r, err := ev.Eval(env, ce, arg)
			if err != nil {
				return EvalResult{}, err
			}
			args[i] = r.Value
			ce = r.CapEnv
		}
		return EvalResult{&ConVal{Con: e.Name, Args: args}, ce}, nil

	case *core.Case:
		scrutR, err := ev.Eval(env, capEnv, e.Scrutinee)
		if err != nil {
			return EvalResult{}, err
		}
		for _, alt := range e.Alts {
			bindings := Match(scrutR.Value, alt.Pattern)
			if bindings != nil {
				altEnv := env.ExtendMany(bindings)
				return ev.Eval(altEnv, scrutR.CapEnv, alt.Body)
			}
		}
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf("non-exhaustive pattern match on %s", scrutR.Value),
			Span:    e.S,
		}

	case *core.LetRec:
		// Knot-tying for recursive bindings.
		recEnv := env
		closures := make([]*Closure, len(e.Bindings))
		for i, b := range e.Bindings {
			lam, ok := b.Expr.(*core.Lam)
			if !ok {
				return EvalResult{}, &RuntimeError{
					Message: fmt.Sprintf("letrec binding %s is not a lambda", b.Name),
					Span:    b.S,
				}
			}
			clo := &Closure{Env: nil, Param: lam.Param, Body: lam.Body}
			closures[i] = clo
			recEnv = recEnv.Extend(b.Name, clo)
		}
		for _, clo := range closures {
			clo.Env = recEnv
		}
		return ev.Eval(recEnv, capEnv, e.Body)

	case *core.Pure:
		return ev.Eval(env, capEnv, e.Expr)

	case *core.Bind:
		compR, err := ev.Eval(env, capEnv, e.Comp)
		if err != nil {
			return EvalResult{}, err
		}
		bodyEnv := env.Extend(e.Var, compR.Value)
		return ev.Eval(bodyEnv, compR.CapEnv, e.Body)

	case *core.Thunk:
		// Mark capEnv as shared since ThunkVal captures it.
		return EvalResult{&ThunkVal{Env: env, Comp: e.Comp}, capEnv.MarkShared()}, nil

	case *core.Force:
		exprR, err := ev.Eval(env, capEnv, e.Expr)
		if err != nil {
			return EvalResult{}, err
		}
		thunk, ok := exprR.Value.(*ThunkVal)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("force applied to non-thunk: %s", exprR.Value),
				Span:    e.S,
			}
		}
		if err := ev.limit.Enter(); err != nil {
			return EvalResult{}, err
		}
		result, err := ev.Eval(thunk.Env, exprR.CapEnv, thunk.Comp)
		ev.limit.Leave()
		return result, err

	case *core.PrimOp:
		if len(e.Args) == 0 && e.Arity > 0 {
			// Unapplied primitive: produce a PrimVal that accumulates args.
			return EvalResult{&PrimVal{Name: e.Name, Arity: e.Arity}, capEnv}, nil
		}
		args := make([]Value, len(e.Args))
		ce := capEnv
		for i, arg := range e.Args {
			r, err := ev.Eval(env, ce, arg)
			if err != nil {
				return EvalResult{}, err
			}
			args[i] = r.Value
			ce = r.CapEnv
		}
		impl, ok := ev.prims.Lookup(e.Name)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("missing primitive: %s", e.Name),
				Span:    e.S,
			}
		}
		val, newCap, err := impl(ev.ctx, ce, args)
		if err != nil {
			return EvalResult{}, err
		}
		return EvalResult{val, newCap}, nil

	default:
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf("unknown Core node: %T", expr),
		}
	}
}

func (ev *Evaluator) apply(capEnv CapEnv, fn Value, arg Value, site *core.App) (EvalResult, error) {
	switch f := fn.(type) {
	case *Closure:
		if err := ev.limit.Enter(); err != nil {
			return EvalResult{}, err
		}
		bodyEnv := f.Env.Extend(f.Param, arg)
		result, err := ev.Eval(bodyEnv, capEnv, f.Body)
		ev.limit.Leave()
		return result, err
	case *ConVal:
		// Constructor application: accumulate arguments.
		args := make([]Value, len(f.Args)+1)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		return EvalResult{&ConVal{Con: f.Con, Args: args}, capEnv}, nil
	case *PrimVal:
		// Primitive application: accumulate arg, call when saturated.
		args := make([]Value, len(f.Args)+1)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		if len(args) < f.Arity {
			return EvalResult{&PrimVal{Name: f.Name, Arity: f.Arity, Args: args}, capEnv}, nil
		}
		impl, ok := ev.prims.Lookup(f.Name)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("missing primitive: %s", f.Name),
				Span:    site.S,
			}
		}
		val, newCap, err := impl(ev.ctx, capEnv, args)
		if err != nil {
			return EvalResult{}, err
		}
		return EvalResult{val, newCap}, nil
	default:
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf("application of non-function: %s", fn),
			Span:    site.S,
		}
	}
}
