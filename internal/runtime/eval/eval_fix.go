package eval

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// evalFix handles the Fix node: creates a self-referential value.
//
// Fix { Name, Body } where Body (after TyLam peeling) is a Lam or Thunk.
//   - Lam body: creates a self-referential closure (value-level fixpoint for fix).
//   - Thunk body: creates a self-referential ThunkVal (computation-level fixpoint for rec).
func (ev *Evaluator) evalFix(locals []Value, capEnv CapEnv, e *ir.Fix) (EvalResult, error) {
	if err := ev.budget.Alloc(CostFix); err != nil {
		return EvalResult{}, err
	}
	body := ir.PeelTyLam(e.Body)
	switch b := body.(type) {
	case *ir.Lam:
		closureBase := CaptureLam(locals, b.FVIndices, b.FV, ExtraCapSelf)
		clo := &Closure{Locals: nil, Param: b.Param, Body: b.Body, Source: ev.source}
		clo.Locals = Push(closureBase, clo)
		return EvalResult{clo, capEnv}, nil
	case *ir.Thunk:
		thunkBase := CaptureLam(locals, b.FVIndices, b.FV, ExtraCapSelf)
		thv := &ThunkVal{Locals: nil, Comp: b.Comp, Source: ev.source, AutoForce: true}
		thv.Locals = Push(thunkBase, thv)
		return EvalResult{thv, capEnv}, nil
	default:
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf(
				"fix binding %s requires a lambda or thunk body (got %T); "+
					"in CBV evaluation, fix creates a self-referential closure "+
					"and cannot produce data constructor values directly — "+
					"wrap the body in a lambda or use thunk/force",
				e.Name, body),
			Span:   e.S,
			Source: ev.source,
		}
	}
}
