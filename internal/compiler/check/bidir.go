package check

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// unifyErrorCode maps a UnifyError to the corresponding diagnostic.Code.
// Returns ErrTypeMismatch for non-UnifyError or general mismatch.
func unifyErrorCode(err error) diagnostic.Code {
	ue, ok := err.(*unify.UnifyError)
	if !ok {
		return diagnostic.ErrTypeMismatch
	}
	switch ue.Kind {
	case unify.UnifyOccursCheck:
		return diagnostic.ErrOccursCheck
	case unify.UnifyDupLabel:
		return diagnostic.ErrDuplicateLabel
	case unify.UnifyRowMismatch:
		return diagnostic.ErrRowMismatch
	case unify.UnifySkolemRigid:
		return diagnostic.ErrSkolemRigid
	default:
		return diagnostic.ErrTypeMismatch
	}
}

// addUnifyError maps a unification error to the appropriate structured error code.
// Used at general type-mismatch sites where the UnifyError kind IS the primary diagnosis.
func (s *Session) addUnifyError(err error, sp span.Span, ctx string) {
	s.addCodedError(unifyErrorCode(err), sp, ctx+": "+err.Error())
}

// addSemanticUnifyError reports a unification failure with a semantic error code.
// For simple mismatches, the semantic code and message are used as-is.
// For specific failures (occurs check, skolem rigidity, etc.), the underlying
// unification error overrides the semantic code — it reveals the root cause.
func (s *Session) addSemanticUnifyError(semanticCode diagnostic.Code, err error, sp span.Span, ctx string) {
	code := unifyErrorCode(err)
	if code == diagnostic.ErrTypeMismatch {
		s.addCodedError(semanticCode, sp, ctx)
		return
	}
	s.addCodedError(code, sp, ctx+": "+err.Error())
}

// infer produces a type for an expression and a Core IR node.
func (ch *Checker) infer(expr syntax.Expr) (types.Type, ir.Core) {
	ch.depth++
	defer func() { ch.depth-- }()
	if err := ch.budget.Nest(); err != nil {
		ch.addCodedError(diagnostic.ErrNestingLimit, expr.Span(), err.Error())
		return &types.TyError{S: expr.Span()}, &ir.Lit{Value: nil, S: expr.Span()}
	}
	defer ch.budget.Unnest()

	switch e := expr.(type) {
	case *syntax.ExprVar:
		switch e.Name {
		case "thunk":
			ch.addCodedError(diagnostic.ErrSpecialForm, e.S, "thunk is a special form; use 'thunk <expr>'")
			return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}
		case "force":
			ch.addCodedError(diagnostic.ErrSpecialForm, e.S, "force is a special form; use 'force <expr>'")
			return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}
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
		return retTy, &ir.App{Fun: funCore, Arg: argCore, S: e.S}

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
			ch.addCodedError(diagnostic.ErrUnboundVar, e.S, fmt.Sprintf("unbound operator: %s", e.Op))
			return &types.TyError{S: e.S}, &ir.Var{Name: e.Op, S: e.S}
		}
		opTy, opCore := ch.instantiate(opTy, &ir.Var{Name: e.Op, Module: opMod, S: e.S})
		arg1Ty, ret1Ty := ch.matchArrow(opTy, e.S)
		arg1Core := ch.check(e.Left, arg1Ty)
		arg2Ty, ret2Ty := ch.matchArrow(ret1Ty, e.S)
		arg2Core := ch.check(e.Right, arg2Ty)
		return ret2Ty, &ir.App{
			Fun: &ir.App{Fun: opCore, Arg: arg1Core, S: e.S},
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
		return ch.infer(desugarSection(e))

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
			ch.addCodedError(diagnostic.ErrTypeMismatch, e.S, fmt.Sprintf("invalid integer literal: %s", e.Value))
			return ch.errorPair(e.S)
		}
		return ch.mkType("Int"), &ir.Lit{Value: val, S: e.S}

	case *syntax.ExprStrLit:
		return ch.mkType("String"), &ir.Lit{Value: e.Value, S: e.S}

	case *syntax.ExprDoubleLit:
		val, err := strconv.ParseFloat(strings.ReplaceAll(e.Value, "_", ""), 64)
		if err != nil {
			ch.addCodedError(diagnostic.ErrTypeMismatch, e.S, fmt.Sprintf("invalid double literal: %s", e.Value))
			return ch.errorPair(e.S)
		}
		return ch.mkType("Double"), &ir.Lit{Value: val, S: e.S}

	case *syntax.ExprRuneLit:
		return ch.mkType("Rune"), &ir.Lit{Value: e.Value, S: e.S}

	case *syntax.ExprList:
		return ch.inferList(e)

	case *syntax.ExprRecord:
		return ch.inferRecord(e)

	case *syntax.ExprRecordUpdate:
		return ch.inferRecordUpdate(e)

	case *syntax.ExprProject:
		return ch.inferProject(e)

	case *syntax.ExprError:
		return ch.errorPair(e.S)

	default:
		ch.addCodedError(diagnostic.ErrTypeMismatch, expr.Span(), "cannot infer type of expression")
		return ch.errorPair(expr.Span())
	}
}

// check verifies that an expression has a given type.
func (ch *Checker) check(expr syntax.Expr, expected types.Type) ir.Core {
	ch.depth++
	defer func() { ch.depth-- }()
	if err := ch.budget.Nest(); err != nil {
		ch.addCodedError(diagnostic.ErrNestingLimit, expr.Span(), err.Error())
		return &ir.Lit{Value: nil, S: expr.Span()}
	}
	defer ch.budget.Unnest()

	expected = ch.unifier.Zonk(expected)

	// Polymorphic fix/rec: intercept before forall peeling so self
	// gets the full expected type, enabling polymorphic recursion.
	if app, ok := expr.(*syntax.ExprApp); ok {
		if v, ok := app.Fun.(*syntax.ExprVar); ok && (v.Name == "fix" || v.Name == "rec") {
			if ch.config.GatedBuiltins != nil && ch.config.GatedBuiltins[v.Name] {
				if lam := fixArgLam(app.Arg); lam != nil {
					return ch.checkFix(app, lam, expected)
				}
			}
		}
	}

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
			return &ir.TyLam{TyParam: f.Var, Kind: f.Kind, Body: bodyCore, S: expr.Span()}
		}
		preID := ch.freshID // track scope boundary
		skolem := ch.freshSkolem(f.Var, f.Kind)
		ch.ctx.Push(&CtxTyVar{Name: f.Var, Kind: f.Kind})
		bodyCore := ch.check(expr, types.Subst(f.Body, f.Var, skolem))
		ch.ctx.Pop()
		// Escape check: skolem must not appear in solutions for
		// metas created before the skolem (outside the scope).
		ch.checkSkolemEscapeInSolutions(skolem, preID, expr.Span())
		return &ir.TyLam{TyParam: f.Var, Kind: f.Kind, Body: bodyCore, S: expr.Span()}
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
func (ch *Checker) checkWithEvidence(expr syntax.Expr, ev *types.TyEvidence) ir.Core {
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
			if cn, cArgs, ok := types.DecomposeConstraintType(cv); ok {
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
		bodyCore = &ir.Lam{Param: dicts[i].param, ParamType: dicts[i].ty, Body: bodyCore, Generated: true, S: expr.Span()}
	}
	return bodyCore
}

// subsCheck performs the subsumption check: inferred ≤ expected.
// Handles forall on the inferred side by instantiation,
// and qualified types by deferring constraints.
// Falls back to Unify when no polymorphism is involved.
func (ch *Checker) subsCheck(inferred, expected types.Type, expr ir.Core, s span.Span) ir.Core {
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
		expr = &ir.TyApp{Expr: expr, TyArg: meta, S: s}
		return ch.subsCheck(body, expected, expr, s)
	}

	// Inferred { C1, C2 } => A ≤ B  →  defer all constraints, check A ≤ B
	if ev, ok := inferred.(*types.TyEvidence); ok {
		for _, entry := range ev.Constraints.ConEntries() {
			placeholder := fmt.Sprintf("%s_%d", prefixDictDefer, ch.fresh())
			ch.emitClassConstraint(placeholder, entry, s)
			expr = &ir.App{Fun: expr, Arg: &ir.Var{Name: placeholder, S: s}, S: s}
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
