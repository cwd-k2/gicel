package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func (ch *Checker) checkLam(e *syntax.ExprLam, expected types.Type) ir.Core {
	if len(e.Params) == 0 {
		return ch.check(e.Body, expected)
	}
	argTy, retTy := ch.matchArrow(expected, e.S)

	// Desugar structured patterns: \pat. body  →  \$p. case $p { pat -> body }
	if isStructuredPattern(e.Params[0]) {
		freshName := ch.freshName(prefixPat)
		var innerBody syntax.Expr
		if len(e.Params) == 1 {
			innerBody = e.Body
		} else {
			innerBody = &syntax.ExprLam{Params: e.Params[1:], Body: e.Body, S: e.S}
		}
		caseExpr := &syntax.ExprCase{
			Scrutinee: &syntax.ExprVar{Name: freshName, S: e.S},
			Alts: []syntax.AstAlt{{
				Pattern: e.Params[0],
				Body:    innerBody,
			}},
			S: e.S,
		}
		ch.ctx.Push(&CtxVar{Name: freshName, Type: argTy})
		bodyCore := ch.check(caseExpr, retTy)
		ch.ctx.Pop()
		return &ir.Lam{Param: freshName, ParamType: argTy, Body: bodyCore, Generated: true, S: e.S}
	}

	paramName := ch.patternName(e.Params[0])
	generated := false
	if pv, ok := e.Params[0].(*syntax.PatVar); ok {
		generated = pv.Generated
	}
	ch.ctx.Push(&CtxVar{Name: paramName, Type: argTy})
	var bodyCore ir.Core
	if len(e.Params) == 1 {
		bodyCore = ch.check(e.Body, retTy)
	} else {
		rest := &syntax.ExprLam{Params: e.Params[1:], Body: e.Body, S: e.S}
		bodyCore = ch.check(rest, retTy)
	}
	ch.ctx.Pop()
	return &ir.Lam{Param: paramName, ParamType: argTy, Body: bodyCore, Generated: generated, S: e.S}
}

// checkApp handles function application in check mode.
// Pre-unifies retTy with expected before checking the argument, so that
// return-position metavariables are solved and type families in argTy can reduce.
// Special forms (pure, thunk, force, bind) fall back to infer + subsCheck.
func (ch *Checker) checkApp(e *syntax.ExprApp, expected types.Type) ir.Core {
	// Special forms: delegate to infer + subsCheck (they have dedicated CBPV logic).
	if v, ok := e.Fun.(*syntax.ExprVar); ok {
		switch v.Name {
		case "pure":
			return ch.checkPure(e, expected)
		case "thunk", "force":
			inferredTy, coreExpr := ch.infer(e)
			return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
		}
	}
	if inner, ok := e.Fun.(*syntax.ExprApp); ok {
		if v, ok := inner.Fun.(*syntax.ExprVar); ok && v.Name == "bind" {
			inferredTy, coreExpr := ch.infer(e)
			return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
		}
	}

	// General case: infer function, decompose arrow, pre-unify return type.
	funTy, funCore := ch.infer(e.Fun)
	argTy, retTy := ch.matchArrow(funTy, e.S)

	// Trial pre-unification: solve metas in retTy from expected.
	// Rollback on failure (retTy may be forall/evidence, handled by subsCheck).
	ch.tryUnify(retTy, expected)

	argCore := ch.check(e.Arg, argTy)
	appCore := &ir.App{Fun: funCore, Arg: argCore, S: e.S}
	return ch.subsCheck(retTy, expected, appCore, e.S)
}

// checkInfix handles infix expressions in check mode.
// Pre-unifies the final return type with expected before checking arguments.
func (ch *Checker) checkInfix(e *syntax.ExprInfix, expected types.Type) ir.Core {
	opTy, opMod, ok := ch.ctx.LookupVarFull(e.Op)
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundVar, e.S, fmt.Sprintf("unbound operator: %s", e.Op))
		return &ir.Var{Name: e.Op, S: e.S}
	}
	opTy, opCore := ch.instantiate(opTy, &ir.Var{Name: e.Op, Module: opMod, S: e.S})
	arg1Ty, ret1Ty := ch.matchArrow(opTy, e.S)
	arg2Ty, ret2Ty := ch.matchArrow(ret1Ty, e.S)

	// Trial pre-unification: solve metas in ret2Ty from expected.
	ch.tryUnify(ret2Ty, expected)

	arg1Core := ch.check(e.Left, arg1Ty)
	arg2Core := ch.check(e.Right, arg2Ty)
	infixCore := &ir.App{
		Fun: &ir.App{Fun: opCore, Arg: arg1Core, S: e.S},
		Arg: arg2Core,
		S:   e.S,
	}
	return ch.subsCheck(ret2Ty, expected, infixCore, e.S)
}

// checkSection handles operator sections in check mode.
// Desugars to a lambda and delegates to check (checkLam propagates expected).
func (ch *Checker) checkSection(e *syntax.ExprSection, expected types.Type) ir.Core {
	return ch.check(desugarSection(e), expected)
}

// desugarSection rewrites an operator section to a lambda expression:
//
//	(+ 1)  → \$sec. $sec + 1   (IsRight=true)
//	(1 +)  → \$sec. 1 + $sec   (IsRight=false)
func desugarSection(e *syntax.ExprSection) *syntax.ExprLam {
	param := prefixSec
	paramVar := &syntax.ExprVar{Name: param, S: e.S}
	var body syntax.Expr
	if e.IsRight {
		body = &syntax.ExprInfix{Left: paramVar, Op: e.Op, Right: e.Arg, S: e.S}
	} else {
		body = &syntax.ExprInfix{Left: e.Arg, Op: e.Op, Right: paramVar, S: e.S}
	}
	return &syntax.ExprLam{Params: []syntax.Pattern{&syntax.PatVar{Name: param, Generated: true, S: e.S}}, Body: body, S: e.S}
}
