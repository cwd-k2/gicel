package eval

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
)

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

// evalLetRec handles the LetRec case of evalStep: knot-tying for recursive
// bindings with FV trimming and fixpoint optimization.
func (ev *Evaluator) evalLetRec(env *Env, capEnv CapEnv, e *core.LetRec) (EvalResult, error) {
	if err := ev.budget.Alloc(int64(costLetRec * len(e.Bindings))); err != nil {
		return EvalResult{}, err
	}
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
	if err := ev.budget.Enter(); err != nil {
		return EvalResult{}, err
	}
	return EvalResult{Value: &bounceVal{
		env: bodyEnv, capEnv: capEnv, expr: e.Body,
		leaveDepth: 1,
	}}, nil
}
