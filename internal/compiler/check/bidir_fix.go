package check

import (
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
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

// inferFix elaborates `fix (\self args... . body)` or `rec (\self. body)` in infer mode.
// Unlike checkFix which receives an expected type, inferFix generates a fresh meta
// for the result type. The produced IR is identical: ir.Fix (or Force(Fix(Thunk(...)))).
func (ch *Checker) inferFix(e *syntax.ExprApp, lam *syntax.ExprLam, isRec bool) (types.Type, ir.Core) {
	if len(lam.Params) == 0 {
		return ch.infer(e)
	}
	selfName := ch.patternName(lam.Params[0])

	// Generate fresh meta for the result type.
	resultTy := ch.freshMeta(types.TypeOfTypes)

	ch.ctx.Push(&CtxVar{Name: selfName, Type: resultTy})

	var bodyExpr syntax.Expr
	if len(lam.Params) == 1 {
		bodyExpr = lam.Body
	} else {
		bodyExpr = &syntax.ExprLam{Params: lam.Params[1:], Body: lam.Body, S: lam.S}
	}
	bodyCore := ch.check(bodyExpr, resultTy)

	ch.ctx.Pop()

	if isRec {
		return resultTy, &ir.Force{
			Expr: &ir.Fix{
				Name: selfName,
				Body: &ir.Thunk{Comp: bodyCore, S: e.S},
				S:    e.S,
			},
			S: e.S,
		}
	}
	ch.checkFixBodyForm(bodyCore, e.S)
	return resultTy, &ir.Fix{
		Name: selfName,
		Body: bodyCore,
		S:    e.S,
	}
}

// checkFix elaborates `fix (\self args... . body)` into a Fix node:
//
//	Fix { self, \args... . body }
//
// By giving self the full expected type (including forall), each
// reference to self in body triggers instantiation with fresh metas,
// enabling polymorphic recursion over GADTs.
//
// checkRec elaborates `rec (\self. body)` into Force(Fix { self, Thunk(body) }).
// In CBV, computation-level fixpoints require thunk/force to avoid eager
// infinite recursion. The self-reference is a ThunkVal that is auto-forced
// by ForceEffectful when it appears in a Bind chain.
func (ch *Checker) checkFix(e *syntax.ExprApp, lam *syntax.ExprLam, expected types.Type, isRec bool) ir.Core {
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

	if isRec {
		// rec (\self. body) → Force(Fix { self, Thunk(body) })
		return &ir.Force{
			Expr: &ir.Fix{
				Name: selfName,
				Body: &ir.Thunk{Comp: bodyCore, S: e.S},
				S:    e.S,
			},
			S: e.S,
		}
	}

	ch.checkFixBodyForm(bodyCore, e.S)
	return &ir.Fix{
		Name: selfName,
		Body: bodyCore,
		S:    e.S,
	}
}

// checkFixBodyForm verifies that a Fix body (after peeling TyLam) is a Lam
// or Thunk. In CBV evaluation, fix creates a self-referential closure and
// requires the body to be a closure-forming node.
func (ch *Checker) checkFixBodyForm(body ir.Core, s span.Span) {
	inner := ir.PeelTyLam(body)
	switch inner.(type) {
	case *ir.Lam, *ir.Thunk:
		// OK
	default:
		ch.addDiag(diagnostic.ErrSpecialForm, s,
			diagMsg("fix requires a function body (\\self args. ...); "+
				"wrap the body in a lambda or use rec for computation-level recursion"))
	}
}
