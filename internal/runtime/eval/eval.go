package eval

import (
	"context"
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// Allocation cost estimates (bytes per value type).
const (
	costClosure = 48               // Closure struct (incl. Source pointer)
	costConBase = 32               // ConVal struct
	costConArg  = 16               // per arg in []Value
	costThunk   = 32               // ThunkVal struct (incl. Source pointer)
	costRecBase = 56               // RecordVal struct + map header
	costRecFld  = 32               // per field in map[string]Value
	costFix     = costClosure + 40 // Closure + Env node for fix binding
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
	globals       map[string]Value // named globals (test/fallback path)
	globalArray   []Value          // slot-indexed globals (production path)
	globalSlots   map[string]int   // name → slot index for global Var resolution
	trace         TraceHook
	obs           *ExplainObserver // nil when explain is disabled
	source        *span.Source     // current source context for error attribution
	cachedApplier Applier          // reused across all primitive invocations
	stats         EvalStats
}

// NewEvaluator creates an Evaluator for a single execution.
// source is the initial source context (typically the main program source).
func NewEvaluator(b *budget.Budget, prims *PrimRegistry, trace TraceHook, obs *ExplainObserver, source *span.Source) *Evaluator {
	// Embed the Budget in the context so that stdlib primitives can charge
	// Go-level allocations via budget.ChargeAlloc(ctx, bytes).
	ctx := budget.ContextWithBudget(b.Context(), b)
	ev := &Evaluator{ctx: ctx, prims: prims, budget: b, trace: trace, obs: obs, source: source}
	ev.cachedApplier = func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error) {
		r, err := ev.applyResolved(capEnv, fn, arg, &ir.App{})
		if err != nil {
			return nil, capEnv, err
		}
		return r.Value, r.CapEnv, nil
	}
	return ev
}

// SetSource updates the current source context on both the evaluator and the observer.
// Called by the Runtime when switching between module and main source contexts.
func (ev *Evaluator) SetSource(src *span.Source) {
	ev.source = src
	if ev.obs != nil {
		ev.obs.SetSource(src)
	}
}

// SetGlobals sets the named globals map for the evaluator.
// Used by tests and as the fallback for un-indexed globals.
func (ev *Evaluator) SetGlobals(g map[string]Value) {
	ev.globals = g
}

// SetGlobalSlots sets the name → slot mapping for global variables.
// At eval time, unresolved globals (Var.Index == -1) are looked up by
// name in this map and then resolved via the global array.
func (ev *Evaluator) SetGlobalSlots(s map[string]int) {
	ev.globalSlots = s
}

// SetGlobalArray sets the slot-indexed globals array for the evaluator.
func (ev *Evaluator) SetGlobalArray(g []Value) {
	ev.globalArray = g
}

// Globals returns the evaluator's named globals map.
func (ev *Evaluator) Globals() map[string]Value {
	return ev.globals
}

// SetGlobalSlot sets a single value in the globals array at the given slot.
// Used by evalBindingsCore to fill binding values during setup.
func (ev *Evaluator) SetGlobalSlot(slot int, v Value) {
	ev.globalArray[slot] = v
}

// Stats returns the accumulated statistics.
func (ev *Evaluator) Stats() EvalStats {
	ev.stats.Allocated = ev.budget.Allocated()
	return ev.stats
}

// Eval evaluates a Core expression using a trampoline loop for TCO.
// Tail-position expressions return a bounceVal instead of recursing,
// keeping the Go stack flat for deep recursion. Bounce sites include
// case alt bodies, closure application, and Force.
//
// Source context is saved on entry and restored on return, so nested
// Eval calls (subexpressions) cannot leak source changes to the caller.
func (ev *Evaluator) Eval(locals []Value, capEnv CapEnv, expr ir.Core) (EvalResult, error) {
	if err := ev.budget.Nest(); err != nil {
		return EvalResult{}, err
	}
	savedSource := ev.source
	defer func() {
		ev.budget.Unnest()
		ev.SetSource(savedSource)
	}()

	var pendingLeaveObs int         // accumulated LeaveInternal calls to fire on resolution
	var pendingForceSpan *span.Span // deferred ForceEffectful from Bind bounce
	for {
		r, err := ev.evalStep(locals, capEnv, expr)
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
			// Apply deferred ForceEffectful from Bind bounce.
			if pendingForceSpan != nil {
				r, err = ev.ForceEffectful(r, *pendingForceSpan)
				if err != nil {
					return EvalResult{}, err
				}
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
		// Follow source context through the bounce chain.
		if b.source != nil {
			ev.SetSource(b.source)
		}
		// Track deferred ForceEffectful from Bind bounces.
		if b.forceSpan != nil {
			pendingForceSpan = b.forceSpan
		}
		locals, capEnv, expr = b.locals, b.capEnv, b.expr
	}
}

// evalStep performs one evaluation step. Tail positions return bounceVal
// to be continued by the Eval trampoline.
func (ev *Evaluator) evalStep(locals []Value, capEnv CapEnv, expr ir.Core) (EvalResult, error) {
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
	case *ir.Var:
		var v Value
		if e.Index >= 0 {
			// Local variable — de Bruijn indexed.
			v = LookupLocal(locals, e.Index)
		} else {
			// Global variable — resolve name to slot, then array lookup.
			key := e.Key
			if key == "" {
				key = ir.VarKey(e)
			}
			if slot, ok := ev.globalSlots[key]; ok {
				v = ev.globalArray[slot]
			} else if v2, ok2 := ev.globals[key]; ok2 {
				// Fallback: named lookup (tests, un-indexed IR).
				v = v2
			} else {
				return EvalResult{}, &RuntimeError{Message: fmt.Sprintf("unbound variable: %s", e.Name), Span: e.S, Source: ev.source}
			}
		}
		// Dereference forward-reference cells (used for mutually-recursive top-level bindings).
		if ind, ok := v.(*IndirectVal); ok {
			if ind.Ref == nil {
				return EvalResult{}, &RuntimeError{Message: fmt.Sprintf("uninitialized forward reference: %s", e.Name), Span: e.S, Source: ev.source}
			}
			return EvalResult{*ind.Ref, capEnv}, nil
		}
		return EvalResult{v, capEnv}, nil

	case *ir.Lam:
		if err := ev.budget.Alloc(costClosure); err != nil {
			return EvalResult{}, err
		}
		closureLocals := CaptureLam(locals, e.FVIndices, e.FV, ExtraCapParam)
		return EvalResult{&Closure{Locals: closureLocals, Param: e.Param, Body: e.Body, Source: ev.source}, capEnv}, nil

	case *ir.App:
		funR, err := ev.Eval(locals, capEnv, e.Fun)
		if err != nil {
			return EvalResult{}, err
		}
		argR, err := ev.Eval(locals, funR.CapEnv, e.Arg)
		if err != nil {
			return EvalResult{}, err
		}
		// Detect let-encoding: (\y. body) expr → emit bind event.
		if ev.obs.Active() {
			if lam, ok := e.Fun.(*ir.Lam); ok && !lam.Generated && lam.Param != "_" {
				ev.obs.Emit(ev.budget.Depth(), ExplainBind, bindDetail(lam.Param, PrettyValue(argR.Value), false), e.S)
			}
		}
		return ev.apply(argR.CapEnv, funR.Value, argR.Value, e)

	case *ir.TyApp:
		// Type application is erased at runtime.
		return ev.Eval(locals, capEnv, e.Expr)

	case *ir.TyLam:
		// Type abstraction is erased at runtime.
		return ev.Eval(locals, capEnv, e.Body)

	case *ir.Con:
		if err := ev.budget.Alloc(int64(costConBase + costConArg*len(e.Args))); err != nil {
			return EvalResult{}, err
		}
		args := make([]Value, len(e.Args))
		ce := capEnv
		for i, arg := range e.Args {
			r, err := ev.Eval(locals, ce, arg)
			if err != nil {
				return EvalResult{}, err
			}
			args[i] = r.Value
			ce = r.CapEnv
		}
		return EvalResult{&ConVal{Con: e.Name, Args: args}, ce}, nil

	case *ir.Case:
		scrutR, err := ev.Eval(locals, capEnv, e.Scrutinee)
		if err != nil {
			return EvalResult{}, err
		}
		for _, alt := range e.Alts {
			matchVals := Match(scrutR.Value, alt.Pattern)
			if matchVals != nil {
				if ev.obs.Active() && !alt.Generated {
					namedBindings := MatchNamed(scrutR.Value, alt.Pattern)
					ev.obs.Emit(ev.budget.Depth(), ExplainMatch, matchDetail(PrettyValue(scrutR.Value), formatPattern(alt.Pattern), namedBindings), e.S)
				}
				altLocals := PushMany(locals, matchVals)
				// Tail position: bounce instead of recursing.
				return EvalResult{Value: &bounceVal{locals: altLocals, capEnv: scrutR.CapEnv, expr: alt.Body}}, nil
			}
		}
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf("non-exhaustive pattern match on %s", scrutR.Value),
			Span:    e.S,
			Source:  ev.source,
		}

	case *ir.Fix:
		return ev.evalFix(locals, capEnv, e)

	case *ir.Pure:
		return ev.Eval(locals, capEnv, e.Expr)

	case *ir.Bind:
		compR, err := ev.Eval(locals, capEnv, e.Comp)
		if err != nil {
			return EvalResult{}, err
		}
		// Force effectful PrimVals (e.g. get, failWith ()) at bind-time.
		compR, err = ev.ForceEffectful(compR, e.S)
		if err != nil {
			return EvalResult{}, err
		}
		if ev.obs.Active() && !e.Generated && e.Var != "_" {
			ev.obs.Emit(ev.budget.Depth(), ExplainBind, bindDetail(e.Var, PrettyValue(compR.Value), true), e.S)
		}
		bodyLocals := Push(locals, compR.Value)
		// Tail position: bounce body instead of recursing.
		// ForceEffectful on the body result is deferred to the trampoline via forceSpan.
		s := e.S
		return EvalResult{Value: &bounceVal{
			locals: bodyLocals, capEnv: compR.CapEnv, expr: e.Body,
			forceSpan: &s,
		}}, nil

	case *ir.Thunk:
		if err := ev.budget.Alloc(costThunk); err != nil {
			return EvalResult{}, err
		}
		thunkLocals := CaptureLam(locals, e.FVIndices, e.FV, ExtraCapNone)
		// Mark capEnv as shared since ThunkVal captures it.
		return EvalResult{&ThunkVal{Locals: thunkLocals, Comp: e.Comp, Source: ev.source}, capEnv.MarkShared()}, nil

	case *ir.Force:
		exprR, err := ev.Eval(locals, capEnv, e.Expr)
		if err != nil {
			return EvalResult{}, err
		}
		thunk, ok := exprR.Value.(*ThunkVal)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("force applied to non-thunk: %s", exprR.Value),
				Span:    e.S,
				Source:  ev.source,
			}
		}
		if err := ev.budget.Enter(); err != nil {
			return EvalResult{}, err
		}
		// Tail position: bounce instead of recursing.
		return EvalResult{Value: &bounceVal{
			locals: thunk.Locals, capEnv: exprR.CapEnv, expr: thunk.Comp,
			leaveDepth: 1, source: thunk.Source,
		}}, nil

	case *ir.Lit:
		if s, ok := e.Value.(string); ok && len(s) > 0 {
			if err := ev.budget.Alloc(int64(len(s))); err != nil {
				return EvalResult{}, err
			}
		}
		return EvalResult{&HostVal{Inner: e.Value}, capEnv}, nil

	case *ir.PrimOp:
		if len(e.Args) == 0 && (e.Arity > 0 || e.Effectful) {
			// Unapplied or effectful primitive: produce a PrimVal that accumulates args.
			// Effectful 0-arity PrimOps (e.g. get) are deferred until forced in Bind.
			return EvalResult{&PrimVal{Name: e.Name, Arity: e.Arity, Effectful: e.Effectful, S: e.S}, capEnv}, nil
		}
		args := make([]Value, len(e.Args))
		ce := capEnv
		for i, arg := range e.Args {
			r, err := ev.Eval(locals, ce, arg)
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
				Source:  ev.source,
			}
		}
		val, newCap, err := callPrim(ev.ctx, impl, ce, args, ev.applier())
		if err != nil {
			return EvalResult{}, wrapPrimError(err, e.S, ev.source)
		}
		return EvalResult{val, newCap}, nil

	case *ir.RecordLit:
		if err := ev.budget.Alloc(int64(costRecBase + costRecFld*len(e.Fields))); err != nil {
			return EvalResult{}, err
		}
		fields := make(map[string]Value, len(e.Fields))
		ce := capEnv
		for _, f := range e.Fields {
			r, err := ev.Eval(locals, ce, f.Value)
			if err != nil {
				return EvalResult{}, err
			}
			fields[f.Label] = r.Value
			ce = r.CapEnv
		}
		return EvalResult{&RecordVal{Fields: fields}, ce}, nil

	case *ir.RecordProj:
		recR, err := ev.Eval(locals, capEnv, e.Record)
		if err != nil {
			return EvalResult{}, err
		}
		rec, ok := recR.Value.(*RecordVal)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("projection on non-record: %s", recR.Value),
				Span:    e.S,
				Source:  ev.source,
			}
		}
		v, ok := rec.Fields[e.Label]
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("record has no field: %s", e.Label),
				Span:    e.S,
				Source:  ev.source,
			}
		}
		return EvalResult{v, recR.CapEnv}, nil

	case *ir.RecordUpdate:
		recR, err := ev.Eval(locals, capEnv, e.Record)
		if err != nil {
			return EvalResult{}, err
		}
		rec, ok := recR.Value.(*RecordVal)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("update on non-record: %s", recR.Value),
				Span:    e.S,
				Source:  ev.source,
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
			r, err := ev.Eval(locals, ce, upd.Value)
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
			Source:  ev.source,
		}
	}
}
