package eval

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// evalFix handles the Fix node: creates a self-referential closure.
//
// Fix { Name, Body } where Body (after TyLam peeling) must be a Lam.
// The result is a closure whose environment binds Name to itself.
func (ev *Evaluator) evalFix(locals []Value, capEnv CapEnv, e *ir.Fix) (EvalResult, error) {
	if err := ev.budget.Alloc(costFix); err != nil {
		return EvalResult{}, err
	}
	lam, ok := ir.PeelTyLam(e.Body).(*ir.Lam)
	if !ok {
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf(
				"fix binding %s requires a lambda body (got %T); "+
					"in CBV evaluation, fix creates a self-referential closure "+
					"and cannot produce data constructor values directly — "+
					"wrap the body in a lambda or use thunk/force",
				e.Name, ir.PeelTyLam(e.Body)),
			Span:   e.S,
			Source: ev.source,
		}
	}
	closureBase := CaptureLam(locals, lam.FVIndices, lam.FV, ExtraCapSelf)
	clo := &Closure{Locals: nil, Param: lam.Param, Body: lam.Body, Source: ev.source}
	clo.Locals = Push(closureBase, clo) // knot-tying: self-reference at index 1 (above param)
	return EvalResult{clo, capEnv}, nil
}
