package vm

import (
	"context"
	"errors"

	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// resolvePrimImplFrame returns the cached PrimImpl for a PrimVal, falling
// back to a registry lookup. Used by frame-driven dispatch (forceEffectful,
// applyPrim, applyN PrimVal cases) where the caller has a current Frame for
// span attribution. The returned error, when non-nil, is already a structured
// *eval.RuntimeError carrying source location.
func (vm *VM) resolvePrimImplFrame(pv *eval.PrimVal, frame *Frame) (eval.PrimImpl, error) {
	if pv.Impl != nil {
		return pv.Impl, nil
	}
	impl, ok := vm.prims.Lookup(pv.Name)
	if !ok {
		return nil, vm.runtimeError("missing primitive: "+pv.Name, frame)
	}
	return impl, nil
}

// resolvePrimImplBare returns the cached PrimImpl for a PrimVal, falling
// back to a registry lookup. Used by barrier-driven dispatch (applyForPrim,
// applyNForPrim) where no Frame is available. The returned error, when
// non-nil, is a plain "missing primitive" error without source attribution
// — the host callback boundary is the right place to add context if needed.
//
// Two helpers (Frame vs Bare) instead of one with optional Frame: the return
// shapes are categorically different (RuntimeError with span vs plain error),
// and forcing callers to pick the right one preserves the layering between
// VM-internal and host-callback dispatch.
func (vm *VM) resolvePrimImplBare(pv *eval.PrimVal) (eval.PrimImpl, error) {
	if pv.Impl != nil {
		return pv.Impl, nil
	}
	impl, ok := vm.prims.Lookup(pv.Name)
	if !ok {
		return nil, errors.New("missing primitive: " + pv.Name)
	}
	return impl, nil
}

// asPartialPrim returns a PrimVal stub representing a partially-applied
// primitive. The args slice MUST outlive the call (it becomes the stub's
// Args field) — callers building it from scratch and from primScratch must
// distinguish: scratch is heap-managed and aliasable, so partial stubs
// MUST use a heap-allocated args slice.
func asPartialPrim(pv *eval.PrimVal, args []eval.Value) *eval.PrimVal {
	return &eval.PrimVal{
		Name:      pv.Name,
		Arity:     pv.Arity,
		Effectful: pv.Effectful,
		Args:      args,
		S:         pv.S,
		Impl:      pv.Impl,
	}
}

// asDeferredEffectful returns a PrimVal stub representing a saturated
// (or over-saturated) effectful primitive whose execution is deferred until
// `forceEffectful` runs it. Same args-lifetime contract as asPartialPrim.
func asDeferredEffectful(pv *eval.PrimVal, args []eval.Value) *eval.PrimVal {
	return &eval.PrimVal{
		Name:      pv.Name,
		Arity:     pv.Arity,
		Effectful: true,
		Args:      args,
		S:         pv.S,
		Impl:      pv.Impl,
	}
}

// applyPrim handles application to a PrimVal.
func (vm *VM) applyPrim(pv *eval.PrimVal, arg eval.Value, frame *Frame) error {
	newLen := len(pv.Args) + 1

	// Fast path: saturated non-effectful with small arity.
	// Uses the VM's scratch buffer to avoid a heap allocation for the
	// transient argument slice. Safe because non-effectful primitives
	// return before any re-entrant VM execution can alias the buffer.
	if newLen >= pv.Arity && !pv.Effectful && newLen <= len(vm.primScratch) {
		impl, err := vm.resolvePrimImplFrame(pv, frame)
		if err != nil {
			return err
		}
		args := vm.primScratch[:newLen]
		copy(args, pv.Args)
		args[len(pv.Args)] = arg
		val, newCap, callErr := vm.callPrim(impl, frame.capEnv, args, pv.S)
		clear(vm.primScratch[:newLen])
		if callErr != nil {
			return callErr
		}
		frame.capEnv = newCap
		vm.push(val)
		return nil
	}

	args := make([]eval.Value, newLen)
	copy(args, pv.Args)
	args[len(pv.Args)] = arg

	if newLen < pv.Arity {
		// Partially applied.
		vm.push(asPartialPrim(pv, args))
		return nil
	}
	if pv.Effectful {
		// Saturated effectful — defer (keep as PrimVal).
		vm.push(asDeferredEffectful(pv, args))
		return nil
	}
	// Saturated non-effectful, arity > scratch size.
	impl, err := vm.resolvePrimImplFrame(pv, frame)
	if err != nil {
		return err
	}
	val, newCap, callErr := vm.callPrim(impl, frame.capEnv, args, pv.S)
	if callErr != nil {
		return callErr
	}
	frame.capEnv = newCap
	vm.push(val)
	return nil
}

// callPrim invokes a host-registered PrimImpl. It is the single dispatch
// path for every prim invocation in the VM (OpPrim direct dispatch, applyN
// saturated, applyForPrim/applyNForPrim host re-entry, applyPrim slow path,
// forceEffectful). One way for one thing.
//
// Trust boundary: prim impls are host-registered code under our control,
// not user-supplied. A panic from a prim impl is a programming bug in the
// host code, not a recoverable runtime condition. We deliberately do NOT
// wrap the call in defer/recover — letting the panic propagate surfaces
// the bug at its source rather than masking it as a generic
// "primitive panicked: internal error" downstream. Stdlib regression tests
// must catch any such panic before reaching production.
//
// Errors from the impl pass through wrapPrimError, which preserves
// structured errors (RuntimeError, ctx cancellation, budget limits) and
// wraps plain errors as RuntimeError so the runtime error path is uniform.
func (vm *VM) callPrim(impl eval.PrimImpl, capEnv eval.CapEnv, args []eval.Value, _ span.Span) (eval.Value, eval.CapEnv, error) {
	val, newCap, err := impl(vm.ctx, capEnv, args, vm.cachedApplier)
	if err != nil {
		// Use a nearby frame's instruction span for the call site,
		// not the PrimVal definition span. This ensures errors point
		// at the user's code, not at the stdlib definition.
		// Guard against barrier frames (nil proto) that appear in
		// runCallee nesting — walk down until we find a real frame.
		var callSpan span.Span
		var source *span.Source
		for i := vm.fp; i >= 0; i-- {
			f := &vm.frames[i]
			if f.proto != nil {
				callSpan = f.proto.SpanAt(f.ip)
				source = f.source
				break
			}
		}
		return nil, capEnv, wrapPrimError(err, callSpan, source)
	}
	return val, newCap, nil
}

// wrapPrimError wraps plain errors from stdlib primitives into RuntimeError,
// attaching source location from the call site when available.
// Errors that already carry structured information (RuntimeError with Span,
// context cancellation, budget limits) pass through unchanged.
func wrapPrimError(err error, s span.Span, source *span.Source) error {
	var re *eval.RuntimeError
	if errors.As(err, &re) {
		// Attach location if the RuntimeError doesn't already have one.
		if re.Span.IsZero() && !s.IsZero() {
			re.Span = s
			re.Source = source
		}
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if budget.IsLimitError(err) {
		return err
	}
	return &eval.RuntimeError{Message: err.Error(), Span: s, Source: source}
}
