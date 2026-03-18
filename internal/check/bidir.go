package check

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

// unifyErrorCode maps a UnifyError to the corresponding errs.Code.
// Returns ErrTypeMismatch for non-UnifyError or general mismatch.
func unifyErrorCode(err error) errs.Code {
	ue, ok := err.(*UnifyError)
	if !ok {
		return errs.ErrTypeMismatch
	}
	switch ue.Kind {
	case UnifyOccursCheck:
		return errs.ErrOccursCheck
	case UnifyDupLabel:
		return errs.ErrDuplicateLabel
	case UnifyRowMismatch:
		return errs.ErrRowMismatch
	case UnifySkolemRigid:
		return errs.ErrSkolemRigid
	default:
		return errs.ErrTypeMismatch
	}
}

// addUnifyError maps a unification error to the appropriate structured error code.
// Used at general type-mismatch sites where the UnifyError kind IS the primary diagnosis.
func (ch *Checker) addUnifyError(err error, s span.Span, ctx string) {
	ch.addCodedError(unifyErrorCode(err), s, ctx+": "+err.Error())
}

// addSemanticUnifyError reports a unification failure with a semantic error code.
// For simple mismatches, the semantic code and message are used as-is.
// For specific failures (occurs check, skolem rigidity, etc.), the underlying
// unification error overrides the semantic code — it reveals the root cause.
func (ch *Checker) addSemanticUnifyError(semanticCode errs.Code, err error, s span.Span, ctx string) {
	code := unifyErrorCode(err)
	if code == errs.ErrTypeMismatch {
		ch.addCodedError(semanticCode, s, ctx)
		return
	}
	ch.addCodedError(code, s, ctx+": "+err.Error())
}

// infer produces a type for an expression and a Core IR node.
func (ch *Checker) infer(expr syntax.Expr) (types.Type, core.Core) {
	ch.depth++
	defer func() { ch.depth-- }()

	switch e := expr.(type) {
	case *syntax.ExprVar:
		switch e.Name {
		case "thunk":
			ch.addCodedError(errs.ErrSpecialForm, e.S, "thunk is a special form; use 'thunk <expr>'")
			return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}
		case "force":
			ch.addCodedError(errs.ErrSpecialForm, e.S, "force is a special form; use 'force <expr>'")
			return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}
		}
		ty, coreExpr, ok := ch.lookupVar(e)
		if !ok {
			return ty, coreExpr
		}
		ch.trace(TraceInfer, e.S, "infer: %s ⇒ %s", e.Name, types.Pretty(ty))
		return ch.instantiate(ty, coreExpr)

	case *syntax.ExprCon:
		ty, coreExpr, ok := ch.lookupCon(e)
		if !ok {
			return ty, coreExpr
		}
		return ch.instantiate(ty, coreExpr)

	case *syntax.ExprQualVar:
		ty, coreExpr, ok := ch.lookupQualVar(e)
		if !ok {
			return ty, coreExpr
		}
		ch.trace(TraceInfer, e.S, "infer: %s.%s ⇒ %s", e.Qualifier, e.Name, types.Pretty(ty))
		return ch.instantiate(ty, coreExpr)

	case *syntax.ExprQualCon:
		ty, coreExpr, ok := ch.lookupQualCon(e)
		if !ok {
			return ty, coreExpr
		}
		return ch.instantiate(ty, coreExpr)

	case *syntax.ExprApp:
		// Optimization: fully applied pure/bind elaborate directly to Core nodes.
		if v, ok := e.Fun.(*syntax.ExprVar); ok {
			switch v.Name {
			case "pure":
				return ch.inferPure(e)
			case "thunk":
				return ch.inferThunk(e)
			case "force":
				return ch.inferForce(e)
			}
		}
		// bind takes two args: bind comp (\x. e) → Core.Bind.
		// Detect App(App(Var("bind"), comp), cont).
		if inner, ok := e.Fun.(*syntax.ExprApp); ok {
			if v, ok := inner.Fun.(*syntax.ExprVar); ok && v.Name == "bind" {
				return ch.inferBind(inner.Arg, e.Arg, e.S)
			}
		}
		funTy, funCore := ch.infer(e.Fun)
		argTy, retTy := ch.matchArrow(funTy, e.S)
		argCore := ch.check(e.Arg, argTy)
		return retTy, &core.App{Fun: funCore, Arg: argCore, S: e.S}

	case *syntax.ExprTyApp:
		// Delegate to inferHead (which preserves foralls) then instantiate remaining.
		ty, coreExpr := ch.inferHead(e)
		return ch.instantiate(ty, coreExpr)

	case *syntax.ExprAnn:
		ty := ch.resolveTypeExpr(e.AnnType)
		coreExpr := ch.check(e.Expr, ty)
		return ty, coreExpr

	case *syntax.ExprInfix:
		// Desugar: a op b → App(App(Var(op), a), b)
		opTy, opMod, ok := ch.ctx.LookupVarFull(e.Op)
		if !ok {
			ch.addCodedError(errs.ErrUnboundVar, e.S, fmt.Sprintf("unbound operator: %s", e.Op))
			return &types.TyError{S: e.S}, &core.Var{Name: e.Op, S: e.S}
		}
		opTy, opCore := ch.instantiate(opTy, &core.Var{Name: e.Op, Module: opMod, S: e.S})
		arg1Ty, ret1Ty := ch.matchArrow(opTy, e.S)
		arg1Core := ch.check(e.Left, arg1Ty)
		arg2Ty, ret2Ty := ch.matchArrow(ret1Ty, e.S)
		arg2Core := ch.check(e.Right, arg2Ty)
		return ret2Ty, &core.App{
			Fun: &core.App{Fun: opCore, Arg: arg1Core, S: e.S},
			Arg: arg2Core,
			S:   e.S,
		}

	case *syntax.ExprBlock:
		return ch.inferBlock(e)

	case *syntax.ExprDo:
		return ch.inferDo(e)

	case *syntax.ExprParen:
		return ch.infer(e.Inner)

	case *syntax.ExprSection:
		// Desugar operator sections to lambda:
		// (+ 1)  → \$x. $x + 1   (IsRight=true)
		// (1 +)  → \$x. 1 + $x   (IsRight=false)
		param := "$sec"
		var body syntax.Expr
		paramVar := &syntax.ExprVar{Name: param, S: e.S}
		if e.IsRight {
			body = &syntax.ExprInfix{Left: paramVar, Op: e.Op, Right: e.Arg, S: e.S}
		} else {
			body = &syntax.ExprInfix{Left: e.Arg, Op: e.Op, Right: paramVar, S: e.S}
		}
		lam := &syntax.ExprLam{Params: []syntax.Pattern{&syntax.PatVar{Name: param, S: e.S}}, Body: body, S: e.S}
		return ch.infer(lam)

	case *syntax.ExprLam:
		// In infer mode, generate fresh metas for param types.
		paramTy := ch.freshMeta(types.KType{})
		retTy := ch.freshMeta(types.KType{})
		lamCore := ch.checkLam(e, types.MkArrow(paramTy, retTy))
		return ch.unifier.Zonk(types.MkArrow(paramTy, retTy)), lamCore

	case *syntax.ExprCase:
		return ch.inferCase(e)

	case *syntax.ExprIntLit:
		val, err := strconv.ParseInt(strings.ReplaceAll(e.Value, "_", ""), 10, 64)
		if err != nil {
			ch.addCodedError(errs.ErrTypeMismatch, e.S, fmt.Sprintf("invalid integer literal: %s", e.Value))
		}
		return ch.mkType("Int"), &core.Lit{Value: val, S: e.S}

	case *syntax.ExprStrLit:
		return ch.mkType("String"), &core.Lit{Value: e.Value, S: e.S}

	case *syntax.ExprDoubleLit:
		val, err := strconv.ParseFloat(strings.ReplaceAll(e.Value, "_", ""), 64)
		if err != nil {
			ch.addCodedError(errs.ErrTypeMismatch, e.S, fmt.Sprintf("invalid double literal: %s", e.Value))
		}
		return ch.mkType("Double"), &core.Lit{Value: val, S: e.S}

	case *syntax.ExprRuneLit:
		return ch.mkType("Rune"), &core.Lit{Value: e.Value, S: e.S}

	case *syntax.ExprList:
		return ch.inferList(e)

	case *syntax.ExprRecord:
		return ch.inferRecord(e)

	case *syntax.ExprRecordUpdate:
		return ch.inferRecordUpdate(e)

	case *syntax.ExprProject:
		return ch.inferProject(e)

	default:
		ch.addCodedError(errs.ErrTypeMismatch, expr.Span(), "cannot infer type of expression")
		return ch.errorPair(expr.Span())
	}
}

// check verifies that an expression has a given type.
func (ch *Checker) check(expr syntax.Expr, expected types.Type) core.Core {
	ch.depth++
	defer func() { ch.depth-- }()

	expected = ch.unifier.Zonk(expected)

	// If the expected type is a forall, introduce a TyLam and check the body
	// against the quantified type. This implements the spec rule:
	//   ⟦ e : \ a:K. T ⟧ = TyLam(a, K, ⟦e: T⟧)
	if f, ok := expected.(*types.TyForall); ok {
		if _, isSort := f.Kind.(types.KSort); isSort {
			// Kind-level quantifier: introduce a fresh kind skolem (KVar)
			// and substitute in all kind positions.
			freshName := fmt.Sprintf("%s$%d", f.Var, ch.fresh())
			body := types.SubstKindInType(f.Body, f.Var, types.KVar{Name: freshName})
			bodyCore := ch.check(expr, body)
			return &core.TyLam{TyParam: f.Var, Kind: f.Kind, Body: bodyCore, S: expr.Span()}
		}
		preID := ch.freshID // track scope boundary
		skolem := ch.freshSkolem(f.Var, f.Kind)
		ch.ctx.Push(&CtxTyVar{Name: f.Var, Kind: f.Kind})
		bodyCore := ch.check(expr, types.Subst(f.Body, f.Var, skolem))
		ch.ctx.Pop()
		// Escape check: skolem must not appear in solutions for
		// metas created before the skolem (outside the scope).
		ch.checkSkolemEscapeInSolutions(skolem, preID, expr.Span())
		return &core.TyLam{TyParam: f.Var, Kind: f.Kind, Body: bodyCore, S: expr.Span()}
	}

	// If the expected type is a TyEvidence, introduce implicit dict parameters
	// for each constraint entry.
	//   ⟦ e : { C1 a, C2 b } => T ⟧ = Lam($d1, Lam($d2, ⟦e: T⟧))
	if ev, ok := expected.(*types.TyEvidence); ok {
		return ch.checkWithEvidence(expr, ev)
	}

	switch e := expr.(type) {
	case *syntax.ExprLam:
		return ch.checkLam(e, expected)

	case *syntax.ExprCase:
		return ch.checkCase(e, expected)

	case *syntax.ExprDo:
		return ch.checkDo(e, expected)

	case *syntax.ExprRecord:
		return ch.checkRecord(e, expected)

	case *syntax.ExprApp:
		return ch.checkApp(e, expected)

	case *syntax.ExprInfix:
		return ch.checkInfix(e, expected)

	case *syntax.ExprSection:
		return ch.checkSection(e, expected)

	case *syntax.ExprParen:
		return ch.check(e.Inner, expected)

	default:
		// Subsumption: infer type, then check inferred ≤ expected.
		inferredTy, coreExpr := ch.infer(expr)
		coreExpr = ch.subsCheck(inferredTy, expected, coreExpr, expr.Span())
		return coreExpr
	}
}

// checkWithEvidence introduces implicit dict parameters for each constraint entry
// and checks the body against the evidence-stripped type.
func (ch *Checker) checkWithEvidence(expr syntax.Expr, ev *types.TyEvidence) core.Core {
	type dictInfo struct {
		param string
		ty    types.Type
	}
	dicts := make([]dictInfo, len(ev.Constraints.ConEntries()))
	for i, entry := range ev.Constraints.ConEntries() {
		var dictTy types.Type
		var className string
		var args []types.Type
		if entry.Quantified != nil {
			dictTy = ch.buildQuantifiedDictType(entry.Quantified)
			className = entry.ClassName
			args = entry.Args
		} else if entry.ConstraintVar != nil && entry.ClassName == "" {
			cv := ch.unifier.Zonk(entry.ConstraintVar)
			if cn, cArgs, ok := DecomposeConstraintType(cv); ok {
				className = cn
				args = cArgs
				dictTy = ch.buildDictType(cn, cArgs)
			} else {
				className = "?"
				dictTy = cv
			}
		} else {
			className = entry.ClassName
			args = entry.Args
			dictTy = ch.buildDictType(entry.ClassName, entry.Args)
		}
		dictParam := fmt.Sprintf("%s_%s_%d", prefixDict, className, ch.fresh())
		dicts[i] = dictInfo{param: dictParam, ty: dictTy}
		ch.ctx.Push(&CtxVar{Name: dictParam, Type: dictTy})
		ch.ctx.Push(&CtxEvidence{
			ClassName:  className,
			Args:       args,
			DictName:   dictParam,
			DictType:   dictTy,
			Quantified: entry.Quantified,
		})
	}
	bodyCore := ch.check(expr, ev.Body)
	bodyCore = ch.resolveDeferredConstraints(bodyCore)
	for i := 0; i < len(dicts)*2; i++ {
		ch.ctx.Pop()
	}
	for i := len(dicts) - 1; i >= 0; i-- {
		bodyCore = &core.Lam{Param: dicts[i].param, ParamType: dicts[i].ty, Body: bodyCore, S: expr.Span()}
	}
	return bodyCore
}

// subsCheck performs the subsumption check: inferred ≤ expected.
// Handles forall on the inferred side by instantiation,
// and qualified types by deferring constraints.
// Falls back to Unify when no polymorphism is involved.
func (ch *Checker) subsCheck(inferred, expected types.Type, expr core.Core, s span.Span) core.Core {
	inferred = ch.unifier.Zonk(inferred)
	expected = ch.unifier.Zonk(expected)

	// Inferred ∀a. A ≤ B  →  instantiate a, check A[a:=?m] ≤ B
	if f, ok := inferred.(*types.TyForall); ok {
		if _, isSort := f.Kind.(types.KSort); isSort {
			// Kind-level quantifier: instantiate with a fresh kind metavariable
			km := ch.freshKindMeta()
			body := types.SubstKindInType(f.Body, f.Var, km)
			return ch.subsCheck(body, expected, expr, s)
		}
		meta := ch.freshMeta(f.Kind)
		body := types.Subst(f.Body, f.Var, meta)
		expr = &core.TyApp{Expr: expr, TyArg: meta, S: s}
		return ch.subsCheck(body, expected, expr, s)
	}

	// Inferred { C1, C2 } => A ≤ B  →  defer all constraints, check A ≤ B
	if ev, ok := inferred.(*types.TyEvidence); ok {
		for _, entry := range ev.Constraints.ConEntries() {
			placeholder := fmt.Sprintf("%s_%d", prefixDictDefer, ch.fresh())
			ch.deferred = append(ch.deferred, deferredConstraint{
				placeholder:   placeholder,
				className:     entry.ClassName,
				args:          entry.Args,
				s:             s,
				quantified:    entry.Quantified,
				constraintVar: entry.ConstraintVar,
			})
			expr = &core.App{Fun: expr, Arg: &core.Var{Name: placeholder, S: s}, S: s}
		}
		return ch.subsCheck(ev.Body, expected, expr, s)
	}

	// Default: unify
	if err := ch.unifier.Unify(inferred, expected); err != nil {
		ch.addUnifyError(err, s, fmt.Sprintf("type mismatch: expected %s, got %s",
			types.Pretty(expected), types.Pretty(inferred)))
	}
	return expr
}

func (ch *Checker) checkLam(e *syntax.ExprLam, expected types.Type) core.Core {
	if len(e.Params) == 0 {
		return ch.check(e.Body, expected)
	}
	argTy, retTy := ch.matchArrow(expected, e.S)

	// Desugar structured patterns: \pat. body  →  \$p. case $p { pat -> body }
	if isStructuredPattern(e.Params[0]) {
		freshName := fmt.Sprintf("%s_%d", prefixPat, ch.fresh())
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
		return &core.Lam{Param: freshName, ParamType: argTy, Body: bodyCore, S: e.S}
	}

	paramName := ch.patternName(e.Params[0])
	ch.ctx.Push(&CtxVar{Name: paramName, Type: argTy})
	var bodyCore core.Core
	if len(e.Params) == 1 {
		bodyCore = ch.check(e.Body, retTy)
	} else {
		rest := &syntax.ExprLam{Params: e.Params[1:], Body: e.Body, S: e.S}
		bodyCore = ch.check(rest, retTy)
	}
	ch.ctx.Pop()
	return &core.Lam{Param: paramName, ParamType: argTy, Body: bodyCore, S: e.S}
}

// checkApp handles function application in check mode.
// Pre-unifies retTy with expected before checking the argument, so that
// return-position metavariables are solved and type families in argTy can reduce.
// Special forms (pure, thunk, force, bind) fall back to infer + subsCheck.
func (ch *Checker) checkApp(e *syntax.ExprApp, expected types.Type) core.Core {
	// Special forms: delegate to infer + subsCheck (they have dedicated CBPV logic).
	if v, ok := e.Fun.(*syntax.ExprVar); ok {
		switch v.Name {
		case "pure", "thunk", "force":
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
	appCore := &core.App{Fun: funCore, Arg: argCore, S: e.S}
	return ch.subsCheck(retTy, expected, appCore, e.S)
}

// checkInfix handles infix expressions in check mode.
// Pre-unifies the final return type with expected before checking arguments.
func (ch *Checker) checkInfix(e *syntax.ExprInfix, expected types.Type) core.Core {
	opTy, opMod, ok := ch.ctx.LookupVarFull(e.Op)
	if !ok {
		ch.addCodedError(errs.ErrUnboundVar, e.S, fmt.Sprintf("unbound operator: %s", e.Op))
		return &core.Var{Name: e.Op, S: e.S}
	}
	opTy, opCore := ch.instantiate(opTy, &core.Var{Name: e.Op, Module: opMod, S: e.S})
	arg1Ty, ret1Ty := ch.matchArrow(opTy, e.S)
	arg2Ty, ret2Ty := ch.matchArrow(ret1Ty, e.S)

	// Trial pre-unification: solve metas in ret2Ty from expected.
	ch.tryUnify(ret2Ty, expected)

	arg1Core := ch.check(e.Left, arg1Ty)
	arg2Core := ch.check(e.Right, arg2Ty)
	infixCore := &core.App{
		Fun: &core.App{Fun: opCore, Arg: arg1Core, S: e.S},
		Arg: arg2Core,
		S:   e.S,
	}
	return ch.subsCheck(ret2Ty, expected, infixCore, e.S)
}

// checkSection handles operator sections in check mode.
// Desugars to a lambda and delegates to check (checkLam propagates expected).
func (ch *Checker) checkSection(e *syntax.ExprSection, expected types.Type) core.Core {
	param := "$sec"
	var body syntax.Expr
	paramVar := &syntax.ExprVar{Name: param, S: e.S}
	if e.IsRight {
		body = &syntax.ExprInfix{Left: paramVar, Op: e.Op, Right: e.Arg, S: e.S}
	} else {
		body = &syntax.ExprInfix{Left: e.Arg, Op: e.Op, Right: paramVar, S: e.S}
	}
	lam := &syntax.ExprLam{Params: []syntax.Pattern{&syntax.PatVar{Name: param, S: e.S}}, Body: body, S: e.S}
	return ch.check(lam, expected)
}

func isStructuredPattern(p syntax.Pattern) bool {
	switch pat := p.(type) {
	case *syntax.PatVar, *syntax.PatWild:
		return false
	case *syntax.PatParen:
		return isStructuredPattern(pat.Inner)
	default:
		return true
	}
}

func (ch *Checker) inferCase(e *syntax.ExprCase) (types.Type, core.Core) {
	scrutTy, scrutCore := ch.infer(e.Scrutinee)
	resultTy := ch.freshMeta(types.KType{})
	caseCore := ch.checkCaseAlts(scrutTy, resultTy, scrutCore, e)
	return ch.unifier.Zonk(resultTy), caseCore
}

func (ch *Checker) checkCase(e *syntax.ExprCase, expected types.Type) core.Core {
	scrutTy, scrutCore := ch.infer(e.Scrutinee)
	return ch.checkCaseAlts(scrutTy, expected, scrutCore, e)
}

func (ch *Checker) checkCaseAlts(scrutTy, resultTy types.Type, scrutCore core.Core, e *syntax.ExprCase) core.Core {
	// Divergent post-states: when result is TyComp, each branch gets a
	// fresh post-state meta. After all branches, post-states are joined.
	comp, isComp := ch.unifier.Zonk(resultTy).(*types.TyComp)
	var branchPosts []types.Type

	var alts []core.Alt
	for _, alt := range e.Alts {
		pr := ch.checkPattern(alt.Pattern, scrutTy)
		for name, ty := range pr.Bindings {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
		needsLocalResolve := len(pr.SkolemIDs) > 0 || pr.HasEvidence
		savedDeferred := ch.deferred
		if needsLocalResolve {
			ch.deferred = nil
		}

		// Per-branch expected type: same resultTy for non-Comp,
		// or TyComp with fresh post-state meta for Comp.
		branchExpected := resultTy
		if isComp {
			freshPost := ch.freshMeta(types.KRow{})
			branchExpected = &types.TyComp{
				Pre: comp.Pre, Post: freshPost, Result: comp.Result, S: comp.S,
			}
			branchPosts = append(branchPosts, freshPost)
		}

		bodyCore := ch.check(alt.Body, branchExpected)
		if needsLocalResolve {
			bodyCore = ch.resolveDeferredConstraints(bodyCore)
			ch.deferred = append(savedDeferred, ch.deferred...)
		}
		for range pr.Bindings {
			ch.ctx.Pop()
		}
		if len(pr.SkolemIDs) > 0 {
			ch.checkSkolemEscape(ch.unifier.Zonk(resultTy), pr.SkolemIDs, alt.Body.Span())
		}
		alts = append(alts, core.Alt{Pattern: pr.Pattern, Body: bodyCore, S: alt.S})
	}

	// Join divergent post-states.
	if isComp && len(branchPosts) > 0 {
		joinedPost := ch.lubPostStates(branchPosts, e.S)
		if err := ch.unifier.Unify(comp.Post, joinedPost); err != nil {
			ch.addUnifyError(err, e.S, fmt.Sprintf(
				"cannot unify case post-state: expected %s, got %s",
				types.Pretty(comp.Post), types.Pretty(joinedPost)))
		}
	}

	ch.checkExhaustive(scrutTy, alts, e.S)
	return &core.Case{Scrutinee: scrutCore, Alts: alts, S: e.S}
}

func (ch *Checker) matchArrow(ty types.Type, s span.Span) (types.Type, types.Type) {
	ty = ch.unifier.Zonk(ty)
	// Peel foralls: a higher-rank return type (e.g., from mkId :: () -> \a. a -> a)
	// must be instantiated before arrow decomposition.
	for {
		if f, ok := ty.(*types.TyForall); ok {
			meta := ch.freshMeta(f.Kind)
			ty = types.Subst(f.Body, f.Var, meta)
		} else {
			break
		}
	}
	if arr, ok := ty.(*types.TyArrow); ok {
		return arr.From, arr.To
	}
	// Generate fresh metas.
	argTy := ch.freshMeta(types.KType{})
	retTy := ch.freshMeta(types.KType{})
	if err := ch.unifier.Unify(ty, types.MkArrow(argTy, retTy)); err != nil {
		ch.addSemanticUnifyError(errs.ErrBadApplication, err, s, fmt.Sprintf("expected function type, got %s", types.Pretty(ty)))
	}
	return argTy, retTy
}

// lookupVar resolves a variable name to its type and Core node.
func (ch *Checker) lookupVar(e *syntax.ExprVar) (types.Type, core.Core, bool) {
	// Suppress errors for parser error-recovery sentinels.
	if e.Name == "<error>" {
		return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}, false
	}
	ty, mod, ok := ch.ctx.LookupVarFull(e.Name)
	if !ok {
		ch.addCodedError(errs.ErrUnboundVar, e.S, fmt.Sprintf("unbound variable: %s", e.Name))
		return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}, false
	}
	return ty, &core.Var{Name: e.Name, Module: mod, S: e.S}, true
}

// lookupCon resolves a constructor name to its type and Core node.
func (ch *Checker) lookupCon(e *syntax.ExprCon) (types.Type, core.Core, bool) {
	if e.Name == "<error>" {
		return &types.TyError{S: e.S}, &core.Con{Name: e.Name, S: e.S}, false
	}
	ty, ok := ch.conTypes[e.Name]
	if !ok {
		ch.addCodedError(errs.ErrUnboundCon, e.S, fmt.Sprintf("unknown constructor: %s", e.Name))
		return &types.TyError{S: e.S}, &core.Con{Name: e.Name, S: e.S}, false
	}
	mod := ch.conModules[e.Name] // "" if from current module or builtin
	return ty, &core.Con{Name: e.Name, Module: mod, S: e.S}, true
}

// lookupQualVar resolves a qualified variable reference (N.add) to its type and Core node.
func (ch *Checker) lookupQualVar(e *syntax.ExprQualVar) (types.Type, core.Core, bool) {
	qs, ok := ch.qualifiedScopes[e.Qualifier]
	if !ok {
		ch.addCodedError(errs.ErrUnboundVar, e.S, fmt.Sprintf("unknown qualifier: %s", e.Qualifier))
		return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}, false
	}
	ty, ok := qs.exports.Values[e.Name]
	if !ok {
		ch.addCodedError(errs.ErrUnboundVar, e.S,
			fmt.Sprintf("module %s does not export value: %s", qs.moduleName, e.Name))
		return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}, false
	}
	return ty, &core.Var{Name: e.Name, Module: qs.moduleName, S: e.S}, true
}

// lookupQualCon resolves a qualified constructor reference (N.Just) to its type and Core node.
func (ch *Checker) lookupQualCon(e *syntax.ExprQualCon) (types.Type, core.Core, bool) {
	qs, ok := ch.qualifiedScopes[e.Qualifier]
	if !ok {
		ch.addCodedError(errs.ErrUnboundCon, e.S, fmt.Sprintf("unknown qualifier: %s", e.Qualifier))
		return &types.TyError{S: e.S}, &core.Con{Name: e.Name, S: e.S}, false
	}
	ty, ok := qs.exports.ConTypes[e.Name]
	if !ok {
		ch.addCodedError(errs.ErrUnboundCon, e.S,
			fmt.Sprintf("module %s does not export constructor: %s", qs.moduleName, e.Name))
		return &types.TyError{S: e.S}, &core.Con{Name: e.Name, S: e.S}, false
	}
	return ty, &core.Con{Name: e.Name, Module: qs.moduleName, S: e.S}, true
}

// inferHead infers the type of an expression without instantiating outer foralls.
// Used by ExprTyApp to preserve the forall for explicit type application (@).
func (ch *Checker) inferHead(expr syntax.Expr) (types.Type, core.Core) {
	switch e := expr.(type) {
	case *syntax.ExprVar:
		ty, coreExpr, _ := ch.lookupVar(e)
		return ty, coreExpr
	case *syntax.ExprCon:
		ty, coreExpr, _ := ch.lookupCon(e)
		return ty, coreExpr
	case *syntax.ExprQualVar:
		ty, coreExpr, _ := ch.lookupQualVar(e)
		return ty, coreExpr
	case *syntax.ExprQualCon:
		ty, coreExpr, _ := ch.lookupQualCon(e)
		return ty, coreExpr
	case *syntax.ExprTyApp:
		innerTy, innerCore := ch.inferHead(e.Expr)
		ty := ch.resolveTypeExpr(e.TyArg)
		innerTy = ch.unifier.Zonk(innerTy)
		f, ok := innerTy.(*types.TyForall)
		if !ok {
			ch.addCodedError(errs.ErrBadTypeApp, e.S, "type application to non-polymorphic type")
			return &types.TyError{S: e.S}, innerCore
		}
		resultTy := types.Subst(f.Body, f.Var, ty)
		return resultTy, &core.TyApp{Expr: innerCore, TyArg: ty, S: e.S}
	default:
		// Non-variable/constructor/TyApp expressions cannot be targets of explicit
		// type application. Falling through to infer (which instantiates) is correct:
		// the caller's instantiate call becomes a no-op since foralls are already gone.
		return ch.infer(expr)
	}
}

func (ch *Checker) instantiate(ty types.Type, expr core.Core) (types.Type, core.Core) {
	for {
		ty = ch.unifier.Zonk(ty)
		if f, ok := ty.(*types.TyForall); ok {
			meta := ch.freshMeta(f.Kind)
			ch.trace(TraceInstantiate, span.Span{}, "instantiate: %s → %s[%s := ?%d]",
				types.Pretty(ty), f.Var, types.Pretty(meta), meta.ID)
			ty = types.Subst(f.Body, f.Var, meta)
			expr = &core.TyApp{Expr: expr, TyArg: meta, S: expr.Span()}
			continue
		}
		if ev, ok := ty.(*types.TyEvidence); ok {
			for _, entry := range ev.Constraints.ConEntries() {
				placeholder := fmt.Sprintf("%s_%d", prefixDictDefer, ch.fresh())
				ch.deferred = append(ch.deferred, deferredConstraint{
					placeholder:   placeholder,
					className:     entry.ClassName,
					args:          entry.Args,
					s:             expr.Span(),
					quantified:    entry.Quantified,
					constraintVar: entry.ConstraintVar,
				})
				expr = &core.App{Fun: expr, Arg: &core.Var{Name: placeholder, S: expr.Span()}, S: expr.Span()}
			}
			ty = ev.Body
			continue
		}
		return ty, expr
	}
}

func (ch *Checker) patternName(p syntax.Pattern) string {
	switch pat := p.(type) {
	case *syntax.PatVar:
		return pat.Name
	case *syntax.PatWild:
		return "_"
	case *syntax.PatParen:
		return ch.patternName(pat.Inner)
	default:
		return "_"
	}
}

// inferList handles list literal [e1, e2, ...] by desugaring to Cons/Nil chain.
func (ch *Checker) inferList(e *syntax.ExprList) (types.Type, core.Core) {
	elemTy := ch.freshMeta(types.KType{})
	listTy := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: elemTy}

	// Build from the end: Nil, then Cons e_n (Cons e_{n-1} ...)
	nilMod := ch.conModules["Nil"]
	consMod := ch.conModules["Cons"]
	var result core.Core = &core.Con{Name: "Nil", Module: nilMod, S: e.S}
	for i := len(e.Elems) - 1; i >= 0; i-- {
		elemCore := ch.check(e.Elems[i], elemTy)
		result = &core.App{
			Fun: &core.App{
				Fun: &core.Con{Name: "Cons", Module: consMod, S: e.S},
				Arg: elemCore,
				S:   e.S,
			},
			Arg: result,
			S:   e.S,
		}
	}

	return ch.unifier.Zonk(listTy), result
}
