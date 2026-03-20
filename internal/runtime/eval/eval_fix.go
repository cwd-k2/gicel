package eval

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/lang/ir"
)

// peelTyLam strips type abstractions from a Core node.
// Fix bindings produced by polymorphic fix elaboration wrap the Lam
// in one or more TyLam layers; since types are erased at runtime,
// we simply unwrap to reach the underlying Lam.
func peelTyLam(c ir.Core) ir.Core {
	for {
		if tl, ok := c.(*ir.TyLam); ok {
			c = tl.Body
			continue
		}
		return c
	}
}

// evalFix handles the Fix node: creates a self-referential closure.
//
// Fix { Name, Body } where Body (after TyLam peeling) must be a Lam.
// The result is a closure whose environment binds Name to itself.
func (ev *Evaluator) evalFix(env *Env, capEnv CapEnv, e *ir.Fix) (EvalResult, error) {
	if err := ev.budget.Alloc(costFix); err != nil {
		return EvalResult{}, err
	}
	lam, ok := peelTyLam(e.Body).(*ir.Lam)
	if !ok {
		return EvalResult{}, &RuntimeError{
			Message: fmt.Sprintf("fix binding %s is not a lambda", e.Name),
			Span:    e.S,
			Source:  ev.source,
		}
	}
	closureBase := env
	if lam.FV != nil {
		closureBase = env.TrimTo(lam.FV)
	}
	clo := &Closure{Env: nil, Param: lam.Param, Body: lam.Body, Source: ev.source}
	clo.Env = closureBase.Extend(e.Name, clo)
	return EvalResult{clo, capEnv}, nil
}
