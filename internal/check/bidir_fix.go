package check

import (
	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// fixArgLam extracts an ExprLam from a fix/rec argument, peeling ExprParen.
func fixArgLam(e syntax.Expr) *syntax.ExprLam {
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

// checkFix elaborates `fix (\self args... . body)` into a Fix node:
//
//	Fix { self, \args... . body }
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

	return &core.Fix{
		Name: selfName,
		Body: bodyCore,
		S:    e.S,
	}
}
