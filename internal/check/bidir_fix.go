package check

import (
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// unwrapLam extracts an ExprLam from an expression, peeling ExprParen.
func unwrapLam(e syntax.Expr) *syntax.ExprLam {
	for {
		switch x := e.(type) {
		case *syntax.ExprLam:
			return x
		case *syntax.ExprParen:
			e = x.Inner
		default:
			return nil
		}
	}
}

// checkFix desugars `fix (\self args... . body)` into a LetRec:
//
//	letrec self = \args... . body in self
//
// By giving self the full expected type (including forall), each
// reference to self in body triggers instantiation with fresh metas,
// enabling polymorphic recursion over GADTs.
func (ch *Checker) checkFix(e *syntax.ExprApp, lam *syntax.ExprLam, expected types.Type) core.Core {
	if len(lam.Params) == 0 {
		// No self parameter — fall back to normal checking.
		inferredTy, coreExpr := ch.infer(e)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	selfName := ch.patternName(lam.Params[0])

	// Push self with the full polymorphic expected type.
	ch.ctx.Push(&CtxVar{Name: selfName, Type: expected})

	// Remaining params form the body expression.
	var bodyExpr syntax.Expr
	if len(lam.Params) == 1 {
		bodyExpr = lam.Body
	} else {
		bodyExpr = &syntax.ExprLam{Params: lam.Params[1:], Body: lam.Body, S: lam.S}
	}
	bodyCore := ch.check(bodyExpr, expected)

	ch.ctx.Pop()

	return &core.LetRec{
		Bindings: []core.Binding{{
			Name: selfName, Type: expected, Expr: bodyCore, S: e.S,
		}},
		Body: &core.Var{Name: selfName, S: e.S},
		S:    e.S,
	}
}
