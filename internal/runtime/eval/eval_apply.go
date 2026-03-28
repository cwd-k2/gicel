package eval

import (
	"context"
	"fmt"
	"runtime"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// ForceEffectful invokes a saturated effectful PrimVal, passing the current CapEnv.
// Non-effectful values and unsaturated PrimVals are returned unchanged.
// callSite is the Span of the calling context (e.g. Bind node) for explain events.
func (ev *Evaluator) ForceEffectful(r EvalResult, callSite span.Span) (EvalResult, error) {
	// Auto-force rec self-referential ThunkVal in computation position.
	// Charge a step per auto-force so unconditional self-reference
	// (rec (\self. self)) hits the step limit instead of leaking <thunk>.
	if thv, ok := r.Value.(*ThunkVal); ok && thv.AutoForce {
		if err := ev.budget.Step(); err != nil {
			return EvalResult{}, err
		}
		return ev.Eval(thv.Locals, r.CapEnv, thv.Comp)
	}
	pv, ok := r.Value.(*PrimVal)
	if !ok || !pv.Effectful || len(pv.Args) < pv.Arity {
		return r, nil
	}
	impl, ok := ev.prims.Lookup(pv.Name)
	if !ok {
		return EvalResult{}, &RuntimeError{Message: fmt.Sprintf("missing primitive: %s", pv.Name), Source: ev.source}
	}
	// Mark shared unconditionally: external code may mutate, so protect the original.
	capForImpl := r.CapEnv.MarkShared()
	val, newCap, err := callPrim(ev.ctx, impl, capForImpl, pv.Args, ev.applier())
	if err != nil {
		return EvalResult{}, wrapPrimError(err, pv.S, ev.source)
	}
	if ev.obs.Active() {
		site := callSite
		if site.Start == 0 {
			site = pv.S
		}
		ev.obs.Emit(ev.budget.Depth(), ExplainEffect, effectDetail(pv.Name, pv.Args, val, capForImpl, newCap), site)
	}
	return EvalResult{val, newCap}, nil
}

// applyResolved calls apply and resolves any bounceVal before returning.
// Used by the cached Applier (exposed to primitives) which must not leak
// internal bounceVal values to external code.
func (ev *Evaluator) applyResolved(capEnv CapEnv, fn Value, arg Value, site *ir.App) (EvalResult, error) {
	r, err := ev.apply(capEnv, fn, arg, site)
	if err != nil {
		return EvalResult{}, err
	}
	b, ok := r.Value.(*bounceVal)
	if !ok {
		return r, nil
	}
	for range b.leaveDepth {
		ev.budget.Leave()
	}
	// Switch source context before entering the bounced body.
	if b.source != nil {
		ev.SetSource(b.source)
	}
	if b.leaveObs {
		result, err := ev.Eval(b.locals, b.capEnv, b.expr)
		ev.obs.LeaveInternal()
		return result, err
	}
	return ev.Eval(b.locals, b.capEnv, b.expr)
}

// callPrim safely invokes a PrimImpl, recovering from panics and nil returns.
func callPrim(ctx context.Context, impl PrimImpl, capEnv CapEnv, args []Value, applier Applier) (val Value, newCap CapEnv, err error) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			val, newCap, err = nil, capEnv, fmt.Errorf("primitive panicked: %v\n%s", r, buf[:n])
		}
	}()
	val, newCap, err = impl(ctx, capEnv, args, applier)
	if err == nil && val == nil {
		err = fmt.Errorf("primitive returned nil value")
	}
	return
}

// wrapPrimError wraps plain errors into RuntimeError with span attribution.
// Typed errors (RuntimeError, budget limits, context errors) pass through
// unchanged to preserve their concrete type for errors.As matching.
func wrapPrimError(err error, s span.Span, source *span.Source) error {
	if _, ok := err.(*RuntimeError); ok {
		return err
	}
	typeName := fmt.Sprintf("%T", err)
	if typeName == "*errors.errorString" || typeName == "*fmt.wrapError" {
		return &RuntimeError{Message: err.Error(), Span: s, Source: source}
	}
	return err
}

// applier returns the cached Applier that delegates to the evaluator's apply method.
func (ev *Evaluator) applier() Applier {
	return ev.cachedApplier
}

func (ev *Evaluator) apply(capEnv CapEnv, fn Value, arg Value, site *ir.App) (EvalResult, error) {
	switch f := fn.(type) {
	case *Closure:
		if err := ev.budget.Enter(); err != nil {
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
				ev.obs.Emit(ev.budget.Depth(), ExplainLabel, detail, site.S)
			}
		}
		bodyLocals := Push(f.Locals, arg)
		return EvalResult{Value: ev.bounceWith(bodyLocals, capEnv, f.Body, 1, leaveObs, f.Source, nil)}, nil
	case *ConVal:
		if err := ev.budget.Alloc(int64(costConBase + costConArg*(len(f.Args)+1))); err != nil {
			return EvalResult{}, err
		}
		args := make([]Value, len(f.Args)+1)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		return EvalResult{&ConVal{Con: f.Con, Args: args}, capEnv}, nil
	case *PrimVal:
		args := make([]Value, len(f.Args)+1)
		copy(args, f.Args)
		args[len(f.Args)] = arg
		if len(args) < f.Arity {
			return EvalResult{&PrimVal{Name: f.Name, Arity: f.Arity, Effectful: f.Effectful, Args: args, S: f.S}, capEnv}, nil
		}
		if f.Effectful {
			return EvalResult{&PrimVal{Name: f.Name, Arity: f.Arity, Effectful: true, Args: args, S: f.S}, capEnv}, nil
		}
		impl, ok := ev.prims.Lookup(f.Name)
		if !ok {
			return EvalResult{}, &RuntimeError{
				Message: fmt.Sprintf("missing primitive: %s", f.Name),
				Span:    site.S,
				Source:  ev.source,
			}
		}
		val, newCap, err := callPrim(ev.ctx, impl, capEnv, args, ev.applier())
		if err != nil {
			return EvalResult{}, wrapPrimError(err, site.S, ev.source)
		}
		return EvalResult{val, newCap}, nil
	default:
		msg := "application of non-function"
		if fn != nil {
			msg = fmt.Sprintf("application of non-function: %s", fn)
		}
		return EvalResult{}, &RuntimeError{
			Message: msg,
			Span:    site.S,
			Source:  ev.source,
		}
	}
}
