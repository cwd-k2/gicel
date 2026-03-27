package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// checkPure handles 'pure <expr>' in check mode.
// When expected type is Computation, delegates to the infer path (Core.Pure).
// When expected type is a non-Computation monad (e.g. Maybe), uses IxMonad
// class dispatch to resolve ixpure — same logic as do-block elaboration.
func (ch *Checker) checkPure(e *syntax.ExprApp, expected types.Type) ir.Core {
	expected = ch.unifier.Zonk(expected)

	// Fast path: Computation type → existing infer + subsCheck.
	if comp, ok := expected.(*types.TyCBPV); ok && comp.Tag == types.TagComp {
		inferredTy, coreExpr := ch.infer(e)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	// Class dispatch: extract monad head, resolve IxMonad, use mkIxPure.
	monadHead := ch.extractMonadHead(expected)
	if monadHead != nil {
		_, args := types.UnwindApp(expected)
		if len(args) > 0 {
			resultTy := args[len(args)-1]
			valCore := ch.check(e.Arg, resultTy)
			return ch.mkIxPure(monadHead, valCore, e.S)
		}
	}

	// Fallback: infer + subsCheck (metavar, error, etc.)
	inferredTy, coreExpr := ch.infer(e)
	return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
}

// inferPure handles the special form 'pure <expr>'.
// pure e: Computation r r a, elaborated to Core.Pure.
func (ch *Checker) inferPure(e *syntax.ExprApp) (types.Type, ir.Core) {
	argTy, argCore := ch.infer(e.Arg)
	r := ch.freshMeta(types.TypeOfRows)
	resultTy := types.MkComp(r, r, argTy)
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, e.S, "pure: %s ⇒ %s", types.Pretty(argTy), types.Pretty(resultTy))
	}
	return resultTy, &ir.Pure{Expr: argCore, S: e.S}
}

// inferBind handles the special form 'bind <comp> <cont>'.
// bind c (\x. e) : Computation r1 r3 b, elaborated to Core.Bind.
func (ch *Checker) inferBind(compExpr, contExpr syntax.Expr, s span.Span) (types.Type, ir.Core) {
	compTy, compCore := ch.infer(compExpr)

	r1 := ch.freshMeta(types.TypeOfRows)
	r2 := ch.freshMeta(types.TypeOfRows)
	a := ch.freshMeta(types.TypeOfTypes)
	if err := ch.unifier.Unify(compTy, types.MkComp(r1, r2, a)); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadComputation, err, compExpr.Span(), fmt.Sprintf("bind: first argument must be a computation, got %s", types.Pretty(compTy)))
		return ch.errorPair(s)
	}

	r3 := ch.freshMeta(types.TypeOfRows)
	b := ch.freshMeta(types.TypeOfTypes)

	var bindVar string
	var bodyCore ir.Core

	if lam, ok := contExpr.(*syntax.ExprLam); ok && len(lam.Params) >= 1 {
		bindVar = ch.patternName(lam.Params[0])
		ch.ctx.Push(&CtxVar{Name: bindVar, Type: ch.unifier.Zonk(a)})
		bodyTy := types.MkComp(ch.unifier.Zonk(r2), r3, b)
		if len(lam.Params) == 1 {
			bodyCore = ch.check(lam.Body, bodyTy)
		} else {
			rest := &syntax.ExprLam{Params: lam.Params[1:], Body: lam.Body, S: lam.S}
			bodyCore = ch.check(rest, bodyTy)
		}
		ch.ctx.Pop()
	} else {
		bindVar = fmt.Sprintf("%s_%d", prefixBind, ch.fresh())
		contExpected := types.MkArrow(ch.unifier.Zonk(a), types.MkComp(ch.unifier.Zonk(r2), r3, b))
		contCore := ch.check(contExpr, contExpected)
		bodyCore = &ir.App{
			Fun: contCore,
			Arg: &ir.Var{Name: bindVar, S: s},
			S:   s,
		}
	}

	resultTy := types.MkComp(ch.unifier.Zonk(r1), ch.unifier.Zonk(r3), ch.unifier.Zonk(b))
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, s, "bind: ⇒ %s", types.Pretty(resultTy))
	}
	return resultTy, &ir.Bind{Comp: compCore, Var: bindVar, Body: bodyCore, Generated: true, S: s}
}

// cbpvTriple extracts (pre, post, result) from a computation or thunk type.
// Returns nil fields if the type is neither.
func cbpvTriple(ty types.Type) (pre, post, result types.Type) {
	if t, ok := ty.(*types.TyCBPV); ok {
		return t.Pre, t.Post, t.Result
	}
	return nil, nil, nil
}

// inferDualForm infers the CBPV dual: thunk (Comp→Thunk) or force (Thunk→Comp).
func (ch *Checker) inferDualForm(
	e *syntax.ExprApp, label string,
	mkExpected func(pre, post, result types.Type) types.Type,
	mkResult func(pre, post, result types.Type) types.Type,
	mkCore func(argCore ir.Core) ir.Core,
) (types.Type, ir.Core) {
	argTy, argCore := ch.infer(e.Arg)
	argTy = ch.unifier.Zonk(argTy)

	// Fast path: direct triple extraction.
	if pre, post, result := cbpvTriple(argTy); pre != nil {
		resultTy := mkResult(pre, post, result)
		if ch.config.Trace != nil {
			ch.trace(TraceInfer, e.S, "%s: %s ⇒ %s", label, types.Pretty(argTy), types.Pretty(resultTy))
		}
		return resultTy, mkCore(argCore)
	}

	// Fallback: unify with a fresh triple.
	pre := ch.freshMeta(types.TypeOfRows)
	post := ch.freshMeta(types.TypeOfRows)
	result := ch.freshMeta(types.TypeOfTypes)
	expected := mkExpected(pre, post, result)
	if err := ch.unifier.Unify(argTy, expected); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadThunk, err, e.S,
			fmt.Sprintf("%s requires a %s argument, got %s", label, types.Pretty(expected), types.Pretty(argTy)))
		return &types.TyError{S: e.S}, mkCore(argCore)
	}
	resultTy := mkResult(ch.unifier.Zonk(pre), ch.unifier.Zonk(post), ch.unifier.Zonk(result))
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, e.S, "%s: %s ⇒ %s", label, types.Pretty(argTy), types.Pretty(resultTy))
	}
	return resultTy, mkCore(argCore)
}

func (ch *Checker) inferThunk(e *syntax.ExprApp) (types.Type, ir.Core) {
	return ch.inferDualForm(e, "thunk",
		func(p, q, r types.Type) types.Type { return types.MkComp(p, q, r) },
		func(p, q, r types.Type) types.Type { return types.MkThunk(p, q, r) },
		func(c ir.Core) ir.Core { return &ir.Thunk{Comp: c, S: e.S} },
	)
}

func (ch *Checker) inferForce(e *syntax.ExprApp) (types.Type, ir.Core) {
	return ch.inferDualForm(e, "force",
		func(p, q, r types.Type) types.Type { return types.MkThunk(p, q, r) },
		func(p, q, r types.Type) types.Type { return types.MkComp(p, q, r) },
		func(c ir.Core) ir.Core { return &ir.Force{Expr: c, S: e.S} },
	)
}
