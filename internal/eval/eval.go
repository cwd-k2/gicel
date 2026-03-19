package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/budget"
	"github.com/cwd-k2/gicel/internal/core"
)

// isCompilerGenerated reports whether a name was introduced by the compiler
// during elaboration (type class dictionaries, desugaring artifacts).
func isCompilerGenerated(name string) bool {
	return strings.Contains(name, "$")
}

// isUserVisible reports whether a binding name originated from user source.
// Compiler-generated names and explicit discards are excluded.
func isUserVisible(name string) bool {
	return name != "_" && !isCompilerGenerated(name)
}

// Allocation cost estimates (bytes per value type).
const (
	costClosure = 40               // Closure struct
	costConBase = 32               // ConVal struct
	costConArg  = 16               // per arg in []Value
	costThunk   = 24               // ThunkVal struct
	costRecBase = 56               // RecordVal struct + map header
	costRecFld  = 32               // per field in map[string]Value
	costLetRec  = costClosure + 40 // Closure + Env node per binding
)

// EvalResult is the result of evaluation.
type EvalResult struct {
	Value  Value
	CapEnv CapEnv
}

// Evaluator is the per-execution evaluation engine.
type Evaluator struct {
	ctx           context.Context
	prims         *PrimRegistry
	budget        *budget.Budget
	trace         TraceHook
	obs           *ExplainObserver // nil when explain is disabled
	cachedApplier Applier          // reused across all primitive invocations
	stats         EvalStats
}

// NewEvaluator creates an Evaluator for a single execution.
func NewEvaluator(b *budget.Budget, prims *PrimRegistry, trace TraceHook, obs *ExplainObserver) *Evaluator {
	// Embed the Budget in the context so that stdlib primitives can charge
	// Go-level allocations via budget.ChargeAlloc(ctx, bytes).
	ctx := budget.ContextWithBudget(b.Context(), b)
	ev := &Evaluator{ctx: ctx, prims: prims, budget: b, trace: trace, obs: obs}
	ev.cachedApplier = func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error) {
		r, err := ev.applyResolved(capEnv, fn, arg, &core.App{})
		if err != nil {
			return nil, capEnv, err
		}
		return r.Value, r.CapEnv, nil
	}
	return ev
}

// Stats returns the accumulated statistics.
func (ev *Evaluator) Stats() EvalStats {
	ev.stats.Allocated = ev.budget.Allocated()
	return ev.stats
}

// Eval evaluates a Core expression using a trampoline loop for TCO.
// Tail-position expressions return a bounceVal instead of recursing,
// keeping the Go stack flat for deep recursion. Bounce sites include
// case alt bodies, closure application, LetRec bodies, and Force.
func (ev *Evaluator) Eval(env *Env, capEnv CapEnv, expr core.Core) (EvalResult, error) {
	if err := ev.budget.Nest(); err != nil {
		return EvalResult{}, err
	}
	defer ev.budget.Unnest()

	var pendingLeaveObs int // accumulated LeaveInternal calls to fire on resolution
	for {
		r, err := ev.evalStep(env, capEnv, expr)
		if err != nil {
			// Unwind observer suppression even on error (matches defer semantics).
			for range pendingLeaveObs {
				ev.obs.LeaveInternal()
			}
			return EvalResult{}, err
		}
		b, ok := r.Value.(*bounceVal)
		if !ok {
			// Final result: unwind all pending observer leaves.
			for range pendingLeaveObs {
				ev.obs.LeaveInternal()
			}
			return r, nil
		}
		// Unwind depth from the frame that bounced (Enter was already called).
		for range b.leaveDepth {
			ev.budget.Leave()
		}
		// Observer LeaveInternal is deferred until the continuation fully
		// resolves, keeping suppression active during body evaluation.
		if b.leaveObs {
			pendingLeaveObs++
		}
		env, capEnv, expr = b.env, b.capEnv, b.expr
	}
}

// evalStep performs one evaluation step. Tail positions return bounceVal
// to be continued by the Eval trampoline.
func (ev *Evaluator) evalStep(env *Env, capEnv CapEnv, expr core.Core) (EvalResult, error) {
	// Check step limit (also checks context cancellation).
	if err := ev.budget.Step(); err != nil {
		return EvalResult{}, err
	}

	// Update stats.
	ev.stats.Steps++
	if d := ev.budget.Depth(); d > ev.stats.MaxDepth {
		ev.stats.MaxDepth = d
	}

	// Trace hook.
	if ev.trace != nil {
		if err := ev.trace(newTraceEvent(ev.budget.Depth(), expr, capEnv)); err != nil {
			return EvalResult{}, err
		}
	}

	switch e := expr.(type) {
	case *core.Var:
		key := core.VarKey(e)
		v, ok := env.Lookup(key)
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
		if err := ev.budget.Alloc(costClosure); err != nil {
			return EvalResult{}, err
		}
		closureEnv := env
		if e.FV != nil {
			closureEnv = env.TrimTo(e.FV)
		}
		return EvalResult{&Closure{Env: closureEnv, Param: e.Param, Body: e.Body}, capEnv}, nil

	case *core.App:
		funR, err := ev.Eval(env, capEnv, e.Fun)
		if err != nil {
			return EvalResult{}, err
		}
		argR, err := ev.Eval(env, funR.CapEnv, e.Arg)
		if err != nil {
			return EvalResult{}, err
		}
		// Detect let-encoding: (\y. body) expr → emit bind event.
		if ev.obs.Active() {
			if lam, ok := e.Fun.(*core.Lam); ok && isUserVisible(lam.Param) {
				ev.obs.Emit(ev.budget.Depth(), ExplainBind, bindDetail(lam.Param, PrettyValue(argR.Value), false), e.S)
			}
		}
		return ev.apply(argR.CapEnv, funR.Value, argR.Value, e)

	case *core.TyApp:
		// Type application is erased at runtime.
		return ev.Eval(env, capEnv, e.Expr)

	case *core.TyLam:
		// Type abstraction is erased at runtime.
		return ev.Eval(env, capEnv, e.Body)

	case *core.Con:
		if err := ev.budget.Alloc(int64(costConBase + costConArg*len(e.Args))); err != nil {
			return EvalResult{}, err
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
		return EvalResult{&ConVal{Con: e.Name, Args: args}, ce}, nil

	case *core.Case:
		scrutR, err := ev.Eval(env, capEnv, e.Scrutinee)
		if err != nil {
			return EvalResult{}, err
		}
		for _, alt := range e.Alts {
			bindings := Match(scrutR.Value, alt.Pattern)
			if bindings != nil {
				if ev.obs.Active() && !isInternalPattern(alt.Pattern) {
					ev.obs.Emit(ev.budget.Depth(), ExplainMatch, matchDetail(PrettyValue(scrutR.Value), formatPattern(alt.Pattern), bindings), e.S)
				}
				altEnv := env.ExtendMany(bindings)
				// Tail position: bounce instead of recursing.
				return EvalResult{Value: &bounceVal{env: altEnv, capEnv: scrutR.CapEnv, expr: alt.Body}}, nil
			}
		}
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf("non-exhaustive pattern match on %s", scrutR.Value),
			Span:    e.S,
		}

	case *core.LetRec:
		return ev.evalLetRec(env, capEnv, e)

	case *core.Pure:
		return ev.Eval(env, capEnv, e.Expr)

	case *core.Bind:
		compR, err := ev.Eval(env, capEnv, e.Comp)
		if err != nil {
			return EvalResult{}, err
		}
		// Force effectful PrimVals (e.g. get, failWith ()) at bind-time.
		compR, err = ev.ForceEffectful(compR, e.S)
		if err != nil {
			return EvalResult{}, err
		}
		if ev.obs.Active() && isUserVisible(e.Var) {
			ev.obs.Emit(ev.budget.Depth(), ExplainBind, bindDetail(e.Var, PrettyValue(compR.Value), true), e.S)
		}
		bodyEnv := env.Extend(e.Var, compR.Value)
		if err := ev.budget.Enter(); err != nil {
			return EvalResult{}, err
		}
		bodyR, err := ev.Eval(bodyEnv, compR.CapEnv, e.Body)
		ev.budget.Leave()
		if err != nil {
			return EvalResult{}, err
		}
		// Force effectful PrimVals in the body result too (e.g. do { put 42; get }).
		return ev.ForceEffectful(bodyR, e.S)

	case *core.Thunk:
		if err := ev.budget.Alloc(costThunk); err != nil {
			return EvalResult{}, err
		}
		thunkEnv := env
		if e.FV != nil {
			thunkEnv = env.TrimTo(e.FV)
		}
		// Mark capEnv as shared since ThunkVal captures it.
		return EvalResult{&ThunkVal{Env: thunkEnv, Comp: e.Comp}, capEnv.MarkShared()}, nil

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
		if err := ev.budget.Enter(); err != nil {
			return EvalResult{}, err
		}
		// Tail position: bounce instead of recursing.
		return EvalResult{Value: &bounceVal{
			env: thunk.Env, capEnv: exprR.CapEnv, expr: thunk.Comp,
			leaveDepth: 1,
		}}, nil

	case *core.Lit:
		return EvalResult{&HostVal{Inner: e.Value}, capEnv}, nil

	case *core.PrimOp:
		if len(e.Args) == 0 && (e.Arity > 0 || e.Effectful) {
			// Unapplied or effectful primitive: produce a PrimVal that accumulates args.
			// Effectful 0-arity PrimOps (e.g. get) are deferred until forced in Bind.
			return EvalResult{&PrimVal{Name: e.Name, Arity: e.Arity, Effectful: e.Effectful, S: e.S}, capEnv}, nil
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
		val, newCap, err := callPrim(ev.ctx, impl, ce, args, ev.applier())
		if err != nil {
			return EvalResult{}, err
		}
		return EvalResult{val, newCap}, nil

	case *core.RecordLit:
		if err := ev.budget.Alloc(int64(costRecBase + costRecFld*len(e.Fields))); err != nil {
			return EvalResult{}, err
		}
		fields := make(map[string]Value, len(e.Fields))
		ce := capEnv
		for _, f := range e.Fields {
			r, err := ev.Eval(env, ce, f.Value)
			if err != nil {
				return EvalResult{}, err
			}
			fields[f.Label] = r.Value
			ce = r.CapEnv
		}
		return EvalResult{&RecordVal{Fields: fields}, ce}, nil

	case *core.RecordProj:
		recR, err := ev.Eval(env, capEnv, e.Record)
		if err != nil {
			return EvalResult{}, err
		}
		rec, ok := recR.Value.(*RecordVal)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("projection on non-record: %s", recR.Value),
				Span:    e.S,
			}
		}
		v, ok := rec.Fields[e.Label]
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("record has no field: %s", e.Label),
				Span:    e.S,
			}
		}
		return EvalResult{v, recR.CapEnv}, nil

	case *core.RecordUpdate:
		recR, err := ev.Eval(env, capEnv, e.Record)
		if err != nil {
			return EvalResult{}, err
		}
		rec, ok := recR.Value.(*RecordVal)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("update on non-record: %s", recR.Value),
				Span:    e.S,
			}
		}
		if err := ev.budget.Alloc(int64(costRecBase + costRecFld*len(rec.Fields))); err != nil {
			return EvalResult{}, err
		}
		// Copy all fields, then overwrite with updates.
		newFields := make(map[string]Value, len(rec.Fields))
		for k, v := range rec.Fields {
			newFields[k] = v
		}
		ce := recR.CapEnv
		for _, upd := range e.Updates {
			r, err := ev.Eval(env, ce, upd.Value)
			if err != nil {
				return EvalResult{}, err
			}
			newFields[upd.Label] = r.Value
			ce = r.CapEnv
		}
		return EvalResult{&RecordVal{Fields: newFields}, ce}, nil

	default:
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf("unknown Core node: %T", expr),
		}
	}
}
