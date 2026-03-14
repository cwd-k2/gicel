package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/span"
)

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
	limit         *Limit
	trace         TraceHook
	explain       ExplainHook
	source        *span.Source // for line/col in explain events; nil if unavailable
	suppress      int         // >0: suppress explain events (inside stdlib)
	cachedApplier Applier     // reused across all primitive invocations
	stats         EvalStats
}

// NewEvaluator creates an Evaluator for a single execution.
func NewEvaluator(ctx context.Context, prims *PrimRegistry, limit *Limit, trace TraceHook, explain ExplainHook) *Evaluator {
	ev := &Evaluator{ctx: ctx, prims: prims, limit: limit, trace: trace, explain: explain}
	ev.cachedApplier = func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error) {
		r, err := ev.apply(capEnv, fn, arg, &core.App{})
		if err != nil {
			return nil, capEnv, err
		}
		return r.Value, r.CapEnv, nil
	}
	return ev
}

// SetSource sets the source for line/col resolution in explain events.
func (ev *Evaluator) SetSource(src *span.Source) {
	ev.source = src
}

// explainAt emits an ExplainStep with line/col derived from a Span.
// Line/col are only set when the Span falls within the user's source text;
// spans from stdlib modules (compiled with a different Source) are excluded.
// Events are suppressed when inside stdlib/internal closures.
func (ev *Evaluator) explainAt(kind ExplainKind, msg string, s span.Span) {
	if ev.suppress > 0 {
		return
	}
	step := ExplainStep{Depth: ev.limit.Depth(), Kind: kind, Message: msg}
	if ev.source != nil && s.Start > 0 && int(s.Start) < len(ev.source.Text) {
		step.Line, step.Col = ev.source.Location(s.Start)
	}
	ev.explain(step)
}

// Stats returns the accumulated statistics.
func (ev *Evaluator) Stats() EvalStats {
	ev.stats.Allocated = ev.limit.Allocated()
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
		if err := ev.limit.Alloc(costClosure); err != nil {
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
		// Detect let-encoding: (\y -> body) expr → emit "y = value".
		if ev.explain != nil && ev.suppress == 0 {
			if lam, ok := e.Fun.(*core.Lam); ok && lam.Param != "_" && !strings.HasPrefix(lam.Param, "$") {
				ev.explainAt(ExplainBind, lam.Param+" = "+PrettyValue(argR.Value), e.S)
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
		if err := ev.limit.Alloc(int64(costConBase + costConArg*len(e.Args))); err != nil {
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
				if ev.explain != nil && ev.suppress == 0 && !isInternalPattern(alt.Pattern) {
					msg := "match " + PrettyValue(scrutR.Value) + " → " + FormatPattern(alt.Pattern)
					if bs := FormatBindings(bindings); bs != "" {
						msg += "    " + bs
					}
					ev.explainAt(ExplainMatch, msg, e.S)
				}
				altEnv := env.ExtendMany(bindings)
				return ev.Eval(altEnv, scrutR.CapEnv, alt.Body)
			}
		}
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf("non-exhaustive pattern match on %s", scrutR.Value),
			Span:    e.S,
		}

	case *core.LetRec:
		if err := ev.limit.Alloc(int64(costLetRec * len(e.Bindings))); err != nil {
			return EvalResult{}, err
		}
		// Knot-tying for recursive bindings.
		// Trim closure environments to union of FV for safe-for-space.
		closureBase := env
		if fv := letRecGroupFV(e); fv != nil {
			closureBase = env.TrimTo(fv)
		}
		recEnv := closureBase
		bodyEnv := env
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
			bodyEnv = bodyEnv.Extend(b.Name, clo)
		}
		for _, clo := range closures {
			clo.Env = recEnv
		}
		// Optimize fix/rec pattern: letrec _x = \arg -> (f _x) arg
		// Evaluate (f _x) once and inline the result closure into _x,
		// eliminating the redundant application on every recursive call.
		if len(closures) == 1 {
			if inner, ok := isFixpointBody(e.Bindings[0]); ok {
				if r, err := ev.Eval(closures[0].Env, capEnv, inner); err == nil {
					if rc, ok := r.Value.(*Closure); ok {
						closures[0].Param = rc.Param
						closures[0].Body = rc.Body
						closures[0].Env = rc.Env
					}
				}
			}
		}
		if err := ev.limit.Enter(); err != nil {
			return EvalResult{}, err
		}
		result, err := ev.Eval(bodyEnv, capEnv, e.Body)
		ev.limit.Leave()
		return result, err

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
		if ev.explain != nil && ev.suppress == 0 && e.Var != "_" && !strings.HasPrefix(e.Var, "$") {
			ev.explainAt(ExplainBind, e.Var+" ← "+PrettyValue(compR.Value), e.S)
		}
		bodyEnv := env.Extend(e.Var, compR.Value)
		if err := ev.limit.Enter(); err != nil {
			return EvalResult{}, err
		}
		bodyR, err := ev.Eval(bodyEnv, compR.CapEnv, e.Body)
		ev.limit.Leave()
		if err != nil {
			return EvalResult{}, err
		}
		// Force effectful PrimVals in the body result too (e.g. do { put 42; get }).
		return ev.ForceEffectful(bodyR, e.S)

	case *core.Thunk:
		if err := ev.limit.Alloc(costThunk); err != nil {
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
		if err := ev.limit.Enter(); err != nil {
			return EvalResult{}, err
		}
		result, err := ev.Eval(thunk.Env, exprR.CapEnv, thunk.Comp)
		ev.limit.Leave()
		return result, err

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
		val, newCap, err := impl(ev.ctx, ce, args, ev.applier())
		if err != nil {
			return EvalResult{}, err
		}
		return EvalResult{val, newCap}, nil

	case *core.RecordLit:
		if err := ev.limit.Alloc(int64(costRecBase + costRecFld*len(e.Fields))); err != nil {
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
		if err := ev.limit.Alloc(int64(costRecBase + costRecFld*len(rec.Fields))); err != nil {
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

// ForceEffectful invokes a saturated effectful PrimVal, passing the current CapEnv.
// Non-effectful values and unsaturated PrimVals are returned unchanged.
// callSite is the Span of the calling context (e.g. Bind node) for explain events.
func (ev *Evaluator) ForceEffectful(r EvalResult, callSite span.Span) (EvalResult, error) {
	pv, ok := r.Value.(*PrimVal)
	if !ok || !pv.Effectful || len(pv.Args) < pv.Arity {
		return r, nil
	}
	impl, ok := ev.prims.Lookup(pv.Name)
	if !ok {
		return EvalResult{}, &RuntimeError{Message: fmt.Sprintf("missing primitive: %s", pv.Name)}
	}
	val, newCap, err := impl(ev.ctx, r.CapEnv, pv.Args, ev.applier())
	if err != nil {
		return EvalResult{}, err
	}
	if ev.explain != nil && ev.suppress == 0 {
		// Prefer callSite (user's code) over pv.S (may be stdlib module).
		site := callSite
		if site.Start == 0 {
			site = pv.S
		}
		ev.explainAt(ExplainEffect, FormatEffect(pv.Name, pv.Args, val, r.CapEnv, newCap), site)
	}
	return EvalResult{val, newCap}, nil
}

// applier returns the cached Applier that delegates to the evaluator's apply method.
func (ev *Evaluator) applier() Applier {
	return ev.cachedApplier
}

func (ev *Evaluator) apply(capEnv CapEnv, fn Value, arg Value, site *core.App) (EvalResult, error) {
	switch f := fn.(type) {
	case *Closure:
		if err := ev.limit.Enter(); err != nil {
			return EvalResult{}, err
		}
		// Explain: function call boundaries and stdlib suppression.
		if ev.explain != nil && f.Name != "" {
			if f.Internal {
				ev.suppress++
				defer func() { ev.suppress-- }()
			} else {
				ev.explainAt(ExplainLabel, "enter "+f.Name, site.S)
			}
		}
		bodyEnv := f.Env.Extend(f.Param, arg)
		result, err := ev.Eval(bodyEnv, capEnv, f.Body)
		ev.limit.Leave()
		return result, err
	case *ConVal:
		if err := ev.limit.Alloc(int64(costConBase + costConArg*(len(f.Args)+1))); err != nil {
			return EvalResult{}, err
		}
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
			return EvalResult{&PrimVal{Name: f.Name, Arity: f.Arity, Effectful: f.Effectful, Args: args, S: f.S}, capEnv}, nil
		}
		if f.Effectful {
			// Effectful primitives are deferred until forced in Bind or top-level.
			return EvalResult{&PrimVal{Name: f.Name, Arity: f.Arity, Effectful: true, Args: args, S: f.S}, capEnv}, nil
		}
		impl, ok := ev.prims.Lookup(f.Name)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("missing primitive: %s", f.Name),
				Span:    site.S,
			}
		}
		val, newCap, err := impl(ev.ctx, capEnv, args, ev.applier())
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

// isFixpointBody detects the fix/rec pattern in a LetRec binding:
//
//	name = \arg -> (f name) arg
//
// Returns the inner application (f name) so it can be evaluated once
// and inlined into the closure, avoiding redundant re-evaluation.
func isFixpointBody(b core.Binding) (inner core.Core, ok bool) {
	lam, isLam := b.Expr.(*core.Lam)
	if !isLam {
		return nil, false
	}
	outerApp, isApp := lam.Body.(*core.App)
	if !isApp {
		return nil, false
	}
	// Check: outer arg == lambda param (the eta-expanded argument)
	argVar, isVar := outerApp.Arg.(*core.Var)
	if !isVar || argVar.Name != lam.Param {
		return nil, false
	}
	// Check: fun is (f name) — an application whose arg is the binding name
	innerApp, isInner := outerApp.Fun.(*core.App)
	if !isInner {
		return nil, false
	}
	selfVar, isSelf := innerApp.Arg.(*core.Var)
	if !isSelf || selfVar.Name != b.Name {
		return nil, false
	}
	return outerApp.Fun, true
}

// letRecGroupFV collects the union of free variables from all Lam bindings
// in a LetRec group. Returns nil if no FV annotations are present.
func letRecGroupFV(e *core.LetRec) []string {
	var hasAnnotation bool
	fvSet := make(map[string]struct{})
	for _, b := range e.Bindings {
		if lam, ok := b.Expr.(*core.Lam); ok && lam.FV != nil {
			hasAnnotation = true
			for _, v := range lam.FV {
				fvSet[v] = struct{}{}
			}
		}
	}
	if !hasAnnotation {
		return nil
	}
	result := make([]string, 0, len(fvSet))
	for v := range fvSet {
		result = append(result, v)
	}
	return result
}
