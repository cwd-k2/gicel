package eval

import (
	"context"
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/span"
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
	limit         *Limit
	trace         TraceHook
	obs           *ExplainObserver // nil when explain is disabled
	cachedApplier Applier          // reused across all primitive invocations
	stats         EvalStats
}

// NewEvaluator creates an Evaluator for a single execution.
func NewEvaluator(ctx context.Context, prims *PrimRegistry, limit *Limit, trace TraceHook, obs *ExplainObserver) *Evaluator {
	// Embed the Limit in the context so that stdlib primitives can charge
	// Go-level allocations via ChargeAlloc(ctx, bytes).
	ctx = ContextWithLimit(ctx, limit)
	ev := &Evaluator{ctx: ctx, prims: prims, limit: limit, trace: trace, obs: obs}
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
	ev.stats.Allocated = ev.limit.Allocated()
	return ev.stats
}

// Eval evaluates a Core expression using a trampoline loop for TCO.
// Tail-position expressions return a bounceVal instead of recursing,
// keeping the Go stack flat for deep recursion. Bounce sites include
// case alt bodies, closure application, LetRec bodies, and Force.
func (ev *Evaluator) Eval(env *Env, capEnv CapEnv, expr core.Core) (EvalResult, error) {
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
			ev.limit.Leave()
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
		// Detect let-encoding: (\y. body) expr → emit bind event.
		if ev.obs.Active() {
			if lam, ok := e.Fun.(*core.Lam); ok && isUserVisible(lam.Param) {
				ev.obs.Emit(ev.limit.Depth(), ExplainBind, bindDetail(lam.Param, PrettyValue(argR.Value), false), e.S)
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
				if ev.obs.Active() && !isInternalPattern(alt.Pattern) {
					ev.obs.Emit(ev.limit.Depth(), ExplainMatch, matchDetail(PrettyValue(scrutR.Value), formatPattern(alt.Pattern), bindings), e.S)
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
		// Optimize fix/rec pattern: letrec _x = \arg. (f _x) arg
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
		// Tail position: bounce instead of recursing.
		return EvalResult{Value: &bounceVal{
			env: bodyEnv, capEnv: capEnv, expr: e.Body,
			leaveDepth: 1,
		}}, nil

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
			ev.obs.Emit(ev.limit.Depth(), ExplainBind, bindDetail(e.Var, PrettyValue(compR.Value), true), e.S)
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
	// Mark shared unconditionally: external code may mutate, so protect the original.
	// Cost is negligible (sets one bool on a value-type copy).
	capForImpl := r.CapEnv.MarkShared()
	val, newCap, err := impl(ev.ctx, capForImpl, pv.Args, ev.applier())
	if err != nil {
		return EvalResult{}, err
	}
	if ev.obs.Active() {
		site := callSite
		if site.Start == 0 {
			site = pv.S
		}
		ev.obs.Emit(ev.limit.Depth(), ExplainEffect, effectDetail(pv.Name, pv.Args, val, capForImpl, newCap), site)
	}
	return EvalResult{val, newCap}, nil
}

// applyResolved calls apply and resolves any bounceVal before returning.
// Used by the cached Applier (exposed to primitives) which must not leak
// internal bounceVal values to external code.
func (ev *Evaluator) applyResolved(capEnv CapEnv, fn Value, arg Value, site *core.App) (EvalResult, error) {
	r, err := ev.apply(capEnv, fn, arg, site)
	if err != nil {
		return EvalResult{}, err
	}
	b, ok := r.Value.(*bounceVal)
	if !ok {
		return r, nil
	}
	// Unwind depth; observer LeaveInternal is handled by the inner Eval's
	// pendingLeaveObs since EnterInternal is already in effect.
	for range b.leaveDepth {
		ev.limit.Leave()
	}
	// Delegate to Eval which will handle pendingLeaveObs accumulation.
	// We must re-enter the suppression scope so Eval can leave it.
	if b.leaveObs {
		// EnterInternal was already called in apply; Eval will call
		// LeaveInternal when the continuation resolves.
		// We use a direct Eval call here; it starts with pendingLeaveObs=0,
		// so we must handle the leave ourselves after Eval returns.
		result, err := ev.Eval(b.env, b.capEnv, b.expr)
		ev.obs.LeaveInternal()
		return result, err
	}
	return ev.Eval(b.env, b.capEnv, b.expr)
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
		var leaveObs bool
		if ev.obs != nil && f.Name != "" {
			if ev.obs.IsInternal(f.Name) {
				ev.obs.EnterInternal()
				leaveObs = true
			} else if ev.obs.Active() {
				detail := labelDetail(f.Name, "enter")
				detail.Value = PrettyValue(arg)
				ev.obs.Emit(ev.limit.Depth(), ExplainLabel, detail, site.S)
			}
		}
		bodyEnv := f.Env.Extend(f.Param, arg)
		// Tail position: bounce instead of recursing.
		return EvalResult{Value: &bounceVal{
			env: bodyEnv, capEnv: capEnv, expr: f.Body,
			leaveDepth: 1, leaveObs: leaveObs,
		}}, nil
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
//	name = \arg. (f name) arg
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
