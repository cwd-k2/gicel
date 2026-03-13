package check

import (
	"fmt"
	"strconv"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/internal/types"
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
		case "pure":
			ch.addCodedError(errs.ErrSpecialForm, e.S, "pure is a special form; use 'pure <expr>'")
			return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}
		case "bind":
			ch.addCodedError(errs.ErrSpecialForm, e.S, "bind is a special form; use do blocks for computation sequencing")
			return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}
		case "force":
			ch.addCodedError(errs.ErrSpecialForm, e.S, "force is a special form; use 'force <expr>'")
			return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}
		}
		ty, ok := ch.ctx.LookupVar(e.Name)
		if !ok {
			ch.addCodedError(errs.ErrUnboundVar, e.S, fmt.Sprintf("unbound variable: %s", e.Name))
			return &types.TyError{S: e.S}, &core.Var{Name: e.Name, S: e.S}
		}
		ch.trace(TraceInfer, e.S, "infer: %s ⇒ %s", e.Name, types.Pretty(ty))
		ty, coreExpr := ch.instantiate(ty, &core.Var{Name: e.Name, S: e.S})
		return ty, coreExpr

	case *syntax.ExprCon:
		ty, ok := ch.conTypes[e.Name]
		if !ok {
			ch.addCodedError(errs.ErrUnboundCon, e.S, fmt.Sprintf("unknown constructor: %s", e.Name))
			return &types.TyError{S: e.S}, &core.Con{Name: e.Name, S: e.S}
		}
		ty, coreExpr := ch.instantiate(ty, &core.Con{Name: e.Name, S: e.S})
		return ty, coreExpr

	case *syntax.ExprApp:
		// Special forms: pure, bind, thunk, force elaborate directly to Core nodes.
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
		// bind takes two args: bind comp (\x -> e) → Core.Bind.
		// Detect App(App(Var("bind"), comp), cont).
		if inner, ok := e.Fun.(*syntax.ExprApp); ok {
			if v, ok := inner.Fun.(*syntax.ExprVar); ok && v.Name == "bind" {
				return ch.inferBind(inner.Arg, e.Arg, e.S)
			}
		}
		// Partial application of bind (bind <comp> without continuation).
		if v, ok := e.Fun.(*syntax.ExprVar); ok && v.Name == "bind" {
			ch.addCodedError(errs.ErrSpecialForm, e.S, "bind requires two arguments: bind <comp> (\\x -> <body>)")
			return &types.TyError{S: e.S}, &core.Var{Name: "bind", S: e.S}
		}
		funTy, funCore := ch.infer(e.Fun)
		argTy, retTy := ch.matchArrow(funTy, e.S)
		argCore := ch.check(e.Arg, argTy)
		return retTy, &core.App{Fun: funCore, Arg: argCore, S: e.S}

	case *syntax.ExprTyApp:
		innerTy, innerCore := ch.infer(e.Expr)
		ty := ch.resolveTypeExpr(e.TyArg)
		f, ok := innerTy.(*types.TyForall)
		if !ok {
			ch.addCodedError(errs.ErrBadTypeApp, e.S, "type application to non-polymorphic type")
			return &types.TyError{S: e.S}, innerCore
		}
		resultTy := types.Subst(f.Body, f.Var, ty)
		return resultTy, &core.TyApp{Expr: innerCore, TyArg: ty, S: e.S}

	case *syntax.ExprAnn:
		ty := ch.resolveTypeExpr(e.AnnType)
		coreExpr := ch.check(e.Expr, ty)
		return ty, coreExpr

	case *syntax.ExprInfix:
		// Desugar: a op b → App(App(Var(op), a), b)
		opTy, ok := ch.ctx.LookupVar(e.Op)
		if !ok {
			ch.addCodedError(errs.ErrUnboundVar, e.S, fmt.Sprintf("unbound operator: %s", e.Op))
			return &types.TyError{S: e.S}, &core.Var{Name: e.Op, S: e.S}
		}
		opTy, opCore := ch.instantiate(opTy, &core.Var{Name: e.Op, S: e.S})
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

	case *syntax.ExprLam:
		// In infer mode, generate fresh metas for param types.
		paramTy := ch.freshMeta(types.KType{})
		retTy := ch.freshMeta(types.KType{})
		lamCore := ch.checkLam(e, types.MkArrow(paramTy, retTy))
		return ch.unifier.Zonk(types.MkArrow(paramTy, retTy)), lamCore

	case *syntax.ExprCase:
		return ch.inferCase(e)

	case *syntax.ExprIntLit:
		val, err := strconv.ParseInt(e.Value, 10, 64)
		if err != nil {
			ch.addCodedError(errs.ErrTypeMismatch, e.S, fmt.Sprintf("invalid integer literal: %s", e.Value))
		}
		return ch.mkType("Int"), &core.Lit{Value: val, S: e.S}

	case *syntax.ExprStrLit:
		return ch.mkType("String"), &core.Lit{Value: e.Value, S: e.S}

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
	//   ⟦ e : forall a:K. T ⟧ = TyLam(a, K, ⟦e : T⟧)
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
	//   ⟦ e : { C1 a, C2 b } => T ⟧ = Lam($d1, Lam($d2, ⟦e : T⟧))
	if ev, ok := expected.(*types.TyEvidence); ok {
		type dictInfo struct {
			param string
			ty    types.Type
		}
		dicts := make([]dictInfo, len(ev.Constraints.Entries))
		for i, entry := range ev.Constraints.Entries {
			var dictTy types.Type
			var className string
			var args []types.Type
			if entry.Quantified != nil {
				dictTy = ch.buildQuantifiedDictType(entry.Quantified)
				className = entry.ClassName
				args = entry.Args
			} else if entry.ConstraintVar != nil && entry.ClassName == "" {
				// Constraint variable: decompose to get className + args.
				cv := ch.unifier.Zonk(entry.ConstraintVar)
				if cn, cArgs, ok := DecomposeConstraintType(cv); ok {
					className = cn
					args = cArgs
					dictTy = ch.buildDictType(cn, cArgs)
				} else {
					className = "?"
					dictTy = cv // fallback
				}
			} else {
				className = entry.ClassName
				args = entry.Args
				dictTy = ch.buildDictType(entry.ClassName, entry.Args)
			}
			dictParam := fmt.Sprintf("$d_%s_%d", className, ch.fresh())
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
		// Wrap in Lam from last to first (innermost dict last).
		for i := len(dicts) - 1; i >= 0; i-- {
			bodyCore = &core.Lam{Param: dicts[i].param, ParamType: dicts[i].ty, Body: bodyCore, S: expr.Span()}
		}
		return bodyCore
	}

	switch e := expr.(type) {
	case *syntax.ExprLam:
		return ch.checkLam(e, expected)

	case *syntax.ExprCase:
		return ch.checkCase(e, expected)

	case *syntax.ExprDo:
		return ch.checkDo(e, expected)

	default:
		// Subsumption: infer type, then check inferred ≤ expected.
		inferredTy, coreExpr := ch.infer(expr)
		coreExpr = ch.subsCheck(inferredTy, expected, coreExpr, expr.Span())
		return coreExpr
	}
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
		groupID := ch.fresh()
		for _, entry := range ev.Constraints.Entries {
			placeholder := fmt.Sprintf("$dict_%d", ch.fresh())
			ch.deferred = append(ch.deferred, deferredConstraint{
				placeholder:   placeholder,
				className:     entry.ClassName,
				args:          entry.Args,
				s:             s,
				group:         groupID,
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
	var alts []core.Alt
	for _, alt := range e.Alts {
		pat, bindings, skolemIDs, hasEvidence := ch.checkPattern(alt.Pattern, scrutTy)
		for name, ty := range bindings {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
		// Isolate deferred constraints when the branch introduces evidence bindings
		// (existential skolems or Dict-style reified evidence). Evidence is only
		// in scope within the alt body, so constraints must be resolved here.
		needsLocalResolve := len(skolemIDs) > 0 || hasEvidence
		savedDeferred := ch.deferred
		if needsLocalResolve {
			ch.deferred = nil
		}
		bodyCore := ch.check(alt.Body, resultTy)
		if needsLocalResolve {
			// Resolve only the constraints generated within this branch body.
			bodyCore = ch.resolveDeferredConstraints(bodyCore)
			// Restore outer deferred constraints.
			ch.deferred = append(savedDeferred, ch.deferred...)
		}
		for range bindings {
			ch.ctx.Pop()
		}
		// Existential escape check: skolems must not appear in the result type.
		if len(skolemIDs) > 0 {
			ch.checkSkolemEscape(ch.unifier.Zonk(resultTy), skolemIDs, alt.Body.Span())
		}
		alts = append(alts, core.Alt{Pattern: pat, Body: bodyCore, S: alt.S})
	}
	ch.checkExhaustive(scrutTy, alts, e.S)
	return &core.Case{Scrutinee: scrutCore, Alts: alts, S: e.S}
}

func (ch *Checker) checkPattern(pat syntax.Pattern, scrutTy types.Type) (core.Pattern, map[string]types.Type, map[int]string, bool) {
	switch p := pat.(type) {
	case *syntax.PatVar:
		return &core.PVar{Name: p.Name, S: p.S}, map[string]types.Type{p.Name: scrutTy}, nil, false
	case *syntax.PatWild:
		return &core.PWild{S: p.S}, nil, nil, false
	case *syntax.PatCon:
		conTy, ok := ch.conTypes[p.Con]
		if !ok {
			ch.addCodedError(errs.ErrUnboundCon, p.S, fmt.Sprintf("unknown constructor in pattern: %s", p.Con))
			return &core.PWild{S: p.S}, nil, nil, false
		}
		// Instantiate constructor type and match argument types.
		conTy = ch.unifier.Zonk(conTy)
		var args []core.Pattern
		bindings := make(map[string]types.Type)
		currentTy := conTy

		// 1. Collect forall vars.
		type forallVar struct {
			name string
			kind types.Kind
		}
		var forallVars []forallVar
		tmpTy := currentTy
		for {
			if f, ok := tmpTy.(*types.TyForall); ok {
				forallVars = append(forallVars, forallVar{name: f.Var, kind: f.Kind})
				tmpTy = f.Body
			} else {
				break
			}
		}

		// 2. Get the return type's free vars (strip arrows from after foralls).
		_, retTy := decomposeConSig(conTy)
		retFreeVars := types.FreeVars(retTy)

		// 3. Classify each forall var: universal (in return type) → meta, existential → skolem.
		skolemIDs := map[int]string{}
		for _, fv := range forallVars {
			if f, ok := currentTy.(*types.TyForall); ok {
				if _, isUniversal := retFreeVars[fv.name]; isUniversal {
					meta := ch.freshMeta(fv.kind)
					currentTy = types.Subst(f.Body, f.Var, meta)
				} else {
					skolem := ch.freshSkolem(fv.name, fv.kind)
					skolemIDs[skolem.ID] = fv.name
					currentTy = types.Subst(f.Body, f.Var, skolem)
				}
			}
		}

		// 4. Peel constraints — generate dict bindings and pattern args for existential constraints.
		// For ConstraintVar entries, the concrete className/args are unknown until
		// return type unification (step 6). Record them and resolve after unification.
		type pendingCV struct {
			constraintVar types.Type
			dictParam     string
		}
		var pendingCVs []pendingCV
		for {
			if ev, ok := currentTy.(*types.TyEvidence); ok {
				for _, entry := range ev.Constraints.Entries {
					if entry.ConstraintVar != nil && entry.ClassName == "" {
						dictParam := fmt.Sprintf("$d_cv_%d", ch.fresh())
						pendingCVs = append(pendingCVs, pendingCV{
							constraintVar: entry.ConstraintVar,
							dictParam:     dictParam,
						})
						args = append(args, &core.PVar{Name: dictParam, S: p.S})
					} else {
						dictParam := fmt.Sprintf("$d_%s_%d", entry.ClassName, ch.fresh())
						dictTy := ch.buildDictType(entry.ClassName, entry.Args)
						bindings[dictParam] = dictTy
						args = append(args, &core.PVar{Name: dictParam, S: p.S})
					}
				}
				currentTy = ev.Body
			} else {
				break
			}
		}

		// 5. Peel arrow arguments matching user-supplied pattern args.
		for _, argPat := range p.Args {
			argTy, restTy := ch.matchArrow(currentTy, p.S)
			corePat, argBindings, childSkolems, _ := ch.checkPattern(argPat, argTy)
			args = append(args, corePat)
			for k, v := range argBindings {
				bindings[k] = v
			}
			for k, v := range childSkolems {
				skolemIDs[k] = v
			}
			currentTy = restTy
		}
		// 6. Unify result type with scrutinee type.
		// GADT: if this constructor has a refined return type that is
		// incompatible with the scrutinee, the branch is inaccessible.
		// Suppress the error — exhaustiveness handles relevance.
		if info := ch.conInfo[p.Con]; info != nil {
			for _, c := range info.Constructors {
				if c.Name == p.Con && c.ReturnType != nil {
					if !ch.canUnifyWith(c.ReturnType, scrutTy) {
						return &core.PCon{Con: p.Con, Args: args, S: p.S}, bindings, skolemIDs, len(pendingCVs) > 0
					}
				}
			}
		}
		if err := ch.unifier.Unify(currentTy, scrutTy); err != nil {
			ch.addUnifyError(err, p.S, "constructor type mismatch")
			return &core.PCon{Con: p.Con, Args: args, S: p.S}, bindings, skolemIDs, len(pendingCVs) > 0
		}
		// 7. Resolve pending constraint variable entries now that metas are solved.
		for _, pcv := range pendingCVs {
			cv := ch.unifier.Zonk(pcv.constraintVar)
			if cn, cArgs, ok := DecomposeConstraintType(cv); ok {
				dictTy := ch.buildDictType(cn, cArgs)
				bindings[pcv.dictParam] = dictTy
			}
		}
		return &core.PCon{Con: p.Con, Args: args, S: p.S}, bindings, skolemIDs, len(pendingCVs) > 0
	case *syntax.PatRecord:
		return ch.checkRecordPattern(p, scrutTy)
	case *syntax.PatParen:
		return ch.checkPattern(p.Inner, scrutTy)
	default:
		return &core.PWild{S: pat.Span()}, nil, nil, false
	}
}

func (ch *Checker) inferBlock(e *syntax.ExprBlock) (types.Type, core.Core) {
	// Desugar: { x := e1; body } → App(Lam(x, body), e1)
	// Forward pass: infer each binding, add to context.
	type bindInfo struct {
		name string
		ty   types.Type
		core core.Core
		s    span.Span
	}
	binds := make([]bindInfo, len(e.Binds))
	for i, bind := range e.Binds {
		bindTy, bindCore := ch.infer(bind.Expr)
		binds[i] = bindInfo{name: bind.Var, ty: bindTy, core: bindCore, s: bind.S}
		ch.ctx.Push(&CtxVar{Name: bind.Var, Type: bindTy})
	}

	// Infer body with all bindings in scope.
	resultTy, result := ch.infer(e.Body)

	// Pop all bindings.
	for range e.Binds {
		ch.ctx.Pop()
	}

	// Backward pass: build Core IR desugaring.
	for i := len(binds) - 1; i >= 0; i-- {
		b := binds[i]
		lam := &core.Lam{Param: b.name, Body: result, S: b.s}
		result = &core.App{Fun: lam, Arg: b.core, S: b.s}
	}

	return resultTy, result
}

func (ch *Checker) inferDo(e *syntax.ExprDo) (types.Type, core.Core) {
	if len(e.Stmts) == 0 {
		ch.addCodedError(errs.ErrEmptyDo, e.S, "empty do block")
		return ch.errorPair(e.S)
	}
	return ch.elaborateStmts(e.Stmts, e.S)
}

func (ch *Checker) elaborateStmts(stmts []syntax.Stmt, s span.Span) (types.Type, core.Core) {
	if len(stmts) == 1 {
		// Last statement: must be an expression.
		switch st := stmts[0].(type) {
		case *syntax.StmtExpr:
			return ch.infer(st.Expr)
		case *syntax.StmtBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return ch.errorPair(st.S)
		case *syntax.StmtPureBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return ch.errorPair(st.S)
		}
	}

	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		// x <- c; rest  →  Bind(c, x, rest)
		compTy, compCore := ch.infer(st.Comp)
		resultTy := ch.extractCompResult(compTy, st.S)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: resultTy})
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		ch.ctx.Pop()
		return restTy, &core.Bind{Comp: compCore, Var: st.Var, Body: restCore, S: st.S}

	case *syntax.StmtPureBind:
		// x := e; rest  →  App(Lam(x, rest), e)
		bindTy, bindCore := ch.infer(st.Expr)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: bindTy})
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		ch.ctx.Pop()
		return restTy, &core.App{
			Fun: &core.Lam{Param: st.Var, Body: restCore, S: st.S},
			Arg: bindCore,
			S:   st.S,
		}

	case *syntax.StmtExpr:
		// c; rest  →  Bind(c, "_", rest)
		_, compCore := ch.infer(st.Expr)
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		return restTy, &core.Bind{Comp: compCore, Var: "_", Body: restCore, S: st.S}

	default:
		ch.addCodedError(errs.ErrBadComputation, s, "unexpected statement in do block")
		return ch.errorPair(s)
	}
}

// checkDo type-checks a do block against an expected type.
// Uses direct Core.Bind for Computation types (fast path) and
// class dispatch via ixbind for other IxMonad instances.
func (ch *Checker) checkDo(e *syntax.ExprDo, expected types.Type) core.Core {
	if len(e.Stmts) == 0 {
		ch.addCodedError(errs.ErrEmptyDo, e.S, "empty do block")
		return &core.Var{Name: "<error>", S: e.S}
	}

	expected = ch.unifier.Zonk(expected)

	// Fast path: Computation types, metas (unknown), and errors use Core.Bind.
	switch expected.(type) {
	case *types.TyComp, *types.TyMeta, *types.TyError:
		inferredTy, coreExpr := ch.elaborateStmts(e.Stmts, e.S)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	// Class dispatch: extract monad head and elaborate with dictionary.
	monadHead := ch.extractMonadHead(expected)
	if monadHead != nil {
		return ch.elaborateDoMonadic(e.Stmts, monadHead, expected, e.S)
	}

	// Fallback: try Computation inference.
	inferredTy, coreExpr := ch.elaborateStmts(e.Stmts, e.S)
	return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
}

// extractMonadHead extracts the type constructor head from a monadic type.
// e.g. Maybe Int → Maybe, List Bool → List
func (ch *Checker) extractMonadHead(ty types.Type) types.Type {
	head, args := types.UnwindApp(ty)
	if _, ok := head.(*types.TyCon); ok && len(args) > 0 {
		// Reconstruct head with all but last arg (the result type).
		var result types.Type = head
		for _, a := range args[:len(args)-1] {
			result = &types.TyApp{Fun: result, Arg: a}
		}
		return result
	}
	return nil
}

// elaborateDoMonadic elaborates a do block using IxMonad class dispatch.
// The monad head is used to resolve the IxMonad (Lift m) instance.
func (ch *Checker) elaborateDoMonadic(stmts []syntax.Stmt, monadHead types.Type, expected types.Type, s span.Span) core.Core {
	if len(stmts) == 1 {
		switch st := stmts[0].(type) {
		case *syntax.StmtExpr:
			// Intercept `pure val` / `ixpure val` at the end of a monadic do block.
			if pureVal := extractPureArg(st.Expr); pureVal != nil {
				_, args := types.UnwindApp(expected)
				if len(args) > 0 {
					resultTy := args[len(args)-1]
					valCore := ch.check(pureVal, resultTy)
					return ch.mkIxPure(monadHead, valCore, s)
				}
			}
			return ch.check(st.Expr, expected)
		case *syntax.StmtBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return &core.Var{Name: "<error>", S: st.S}
		case *syntax.StmtPureBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return &core.Var{Name: "<error>", S: st.S}
		}
	}

	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		// x <- comp; rest  →  ixbind comp (\x -> rest)
		// Intercept `x <- pure val` / `x <- ixpure val`.
		var compCore core.Core
		var resultTy types.Type
		if pureVal := extractPureArg(st.Comp); pureVal != nil {
			rty, vc := ch.infer(pureVal)
			compCore = ch.mkIxPure(monadHead, vc, st.S)
			resultTy = rty
		} else {
			compTy, cc := ch.infer(st.Comp)
			compCore = cc
			resultTy = ch.extractMonadResult(compTy, monadHead, st.S)
		}
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: resultTy})
		restCore := ch.elaborateDoMonadic(stmts[1:], monadHead, expected, s)
		ch.ctx.Pop()
		return ch.mkIxBind(monadHead, compCore, st.Var, restCore, st.S)

	case *syntax.StmtExpr:
		// comp; rest  →  ixbind comp (\_ -> rest)
		var compCore core.Core
		if pureVal := extractPureArg(st.Expr); pureVal != nil {
			_, vc := ch.infer(pureVal)
			compCore = ch.mkIxPure(monadHead, vc, st.S)
		} else {
			_, cc := ch.infer(st.Expr)
			compCore = cc
		}
		restCore := ch.elaborateDoMonadic(stmts[1:], monadHead, expected, s)
		return ch.mkIxBind(monadHead, compCore, "_", restCore, st.S)

	case *syntax.StmtPureBind:
		// x := e; rest  →  (\x -> rest) e
		bindTy, bindCore := ch.infer(st.Expr)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: bindTy})
		restCore := ch.elaborateDoMonadic(stmts[1:], monadHead, expected, s)
		ch.ctx.Pop()
		return &core.App{
			Fun: &core.Lam{Param: st.Var, Body: restCore, S: st.S},
			Arg: bindCore,
			S:   st.S,
		}
	}

	return &core.Var{Name: "<error>", S: s}
}

// extractMonadResult extracts the result type from a monadic type given the monad head.
// e.g. Maybe Int with head Maybe → Int
func (ch *Checker) extractMonadResult(ty types.Type, monadHead types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	_, args := types.UnwindApp(ty)
	if len(args) > 0 {
		return args[len(args)-1]
	}
	// Generate fresh meta as fallback.
	result := ch.freshMeta(types.KType{})
	headApp := &types.TyApp{Fun: monadHead, Arg: result}
	if err := ch.unifier.Unify(ty, headApp); err != nil {
		ch.addSemanticUnifyError(errs.ErrBadComputation, err, s, fmt.Sprintf("expected %s type, got %s",
			types.Pretty(monadHead), types.Pretty(ty)))
		return &types.TyError{S: s}
	}
	return result
}

// extractPureArg checks if an expression is `pure val` or `ixpure val` and returns val.
func extractPureArg(expr syntax.Expr) syntax.Expr {
	app, ok := expr.(*syntax.ExprApp)
	if !ok {
		return nil
	}
	v, ok := app.Fun.(*syntax.ExprVar)
	if !ok {
		return nil
	}
	if v.Name == "pure" || v.Name == "ixpure" {
		return app.Arg
	}
	return nil
}

// mkIxPure generates Core for monadic pure using the IxMonad dictionary.
func (ch *Checker) mkIxPure(monadHead types.Type, val core.Core, s span.Span) core.Core {
	liftedMonad := &types.TyApp{Fun: &types.TyCon{Name: "Lift"}, Arg: monadHead}
	dict := ch.resolveInstance("IxMonad", []types.Type{liftedMonad}, s)

	classInfo := ch.classes["IxMonad"]
	pureIdx := len(classInfo.Supers) // ixpure is the first method (index 0)
	allFields := len(classInfo.Supers) + len(classInfo.Methods)
	var patArgs []core.Pattern
	var pureExpr core.Core
	freshBase := ch.fresh()
	for j := 0; j < allFields; j++ {
		argName := fmt.Sprintf("$ixm_%d_%d", j, freshBase)
		patArgs = append(patArgs, &core.PVar{Name: argName, S: s})
		if j == pureIdx {
			pureExpr = &core.Var{Name: argName, S: s}
		}
	}
	selector := &core.Case{
		Scrutinee: dict,
		Alts: []core.Alt{{
			Pattern: &core.PCon{Con: classInfo.DictConName, Args: patArgs, S: s},
			Body:    pureExpr,
			S:       s,
		}},
		S: s,
	}

	return &core.App{Fun: selector, Arg: val, S: s}
}

// mkIxBind generates Core for a monadic bind using the IxMonad dictionary.
func (ch *Checker) mkIxBind(monadHead types.Type, comp core.Core, varName string, body core.Core, s span.Span) core.Core {
	// Resolve IxMonad (Lift monadHead) instance.
	liftedMonad := &types.TyApp{Fun: &types.TyCon{Name: "Lift"}, Arg: monadHead}
	dict := ch.resolveInstance("IxMonad", []types.Type{liftedMonad}, s)

	// Extract ixbind from dictionary using pattern match.
	classInfo := ch.classes["IxMonad"]
	bindIdx := len(classInfo.Supers) + 1 // ixbind is the second method (index 1)
	allFields := len(classInfo.Supers) + len(classInfo.Methods)
	var patArgs []core.Pattern
	var bindExpr core.Core
	freshBase := ch.fresh()
	for j := 0; j < allFields; j++ {
		argName := fmt.Sprintf("$ixm_%d_%d", j, freshBase)
		patArgs = append(patArgs, &core.PVar{Name: argName, S: s})
		if j == bindIdx {
			bindExpr = &core.Var{Name: argName, S: s}
		}
	}
	selector := &core.Case{
		Scrutinee: dict,
		Alts: []core.Alt{{
			Pattern: &core.PCon{Con: classInfo.DictConName, Args: patArgs, S: s},
			Body:    bindExpr,
			S:       s,
		}},
		S: s,
	}

	// ixbind comp (\x -> body)
	return &core.App{
		Fun: &core.App{Fun: selector, Arg: comp, S: s},
		Arg: &core.Lam{Param: varName, Body: body, S: s},
		S:   s,
	}
}

func (ch *Checker) extractCompResult(ty types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	if comp, ok := ty.(*types.TyComp); ok {
		return comp.Result
	}
	// Try to unify with a fresh Computation.
	pre := ch.freshMeta(types.KRow{})
	post := ch.freshMeta(types.KRow{})
	result := ch.freshMeta(types.KType{})
	expected := types.MkComp(pre, post, result)
	if err := ch.unifier.Unify(ty, expected); err != nil {
		ch.addSemanticUnifyError(errs.ErrBadComputation, err, s, fmt.Sprintf("expected computation type, got %s", types.Pretty(ty)))
		return &types.TyError{S: s}
	}
	return result
}

func (ch *Checker) matchArrow(ty types.Type, s span.Span) (types.Type, types.Type) {
	ty = ch.unifier.Zonk(ty)
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
			groupID := ch.fresh()
			for _, entry := range ev.Constraints.Entries {
				placeholder := fmt.Sprintf("$dict_%d", ch.fresh())
				ch.deferred = append(ch.deferred, deferredConstraint{
					placeholder:   placeholder,
					className:     entry.ClassName,
					args:          entry.Args,
					s:             expr.Span(),
					group:         groupID,
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

func (ch *Checker) resolveTypeExpr(texpr syntax.TypeExpr) types.Type {
	switch t := texpr.(type) {
	case *syntax.TyExprVar:
		return &types.TyVar{Name: t.Name, S: t.S}
	case *syntax.TyExprCon:
		if info, ok := ch.aliases[t.Name]; ok && len(info.params) == 0 {
			return info.body
		}
		// DataKinds: if the name is a promoted constructor, treat it as a TyCon
		// (it will be kind-checked later; for now it's just a name in type position).
		return &types.TyCon{Name: t.Name, S: t.S}
	case *syntax.TyExprApp:
		fun := ch.resolveTypeExpr(t.Fun)
		arg := ch.resolveTypeExpr(t.Arg)
		// Recognize Computation and Thunk constructor application.
		result := ch.tryExpandApp(fun, arg, t.S)
		if result != nil {
			return result
		}
		ch.checkTypeAppKind(fun, arg, t.S)
		return &types.TyApp{Fun: fun, Arg: arg, S: t.S}
	case *syntax.TyExprArrow:
		return &types.TyArrow{
			From: ch.resolveTypeExpr(t.From),
			To:   ch.resolveTypeExpr(t.To),
			S:    t.S,
		}
	case *syntax.TyExprForall:
		// Register kind variables (binders with Kind sort) before resolving the body,
		// so that kind variable references in inner kind annotations resolve correctly.
		var kindVarNames []string
		for _, b := range t.Binders {
			if _, ok := b.Kind.(*syntax.KindExprSort); ok {
				ch.kindVars[b.Name] = true
				kindVarNames = append(kindVarNames, b.Name)
			}
		}
		ty := ch.resolveTypeExpr(t.Body)
		for i := len(t.Binders) - 1; i >= 0; i-- {
			kind := ch.resolveKindExpr(t.Binders[i].Kind)
			ty = &types.TyForall{Var: t.Binders[i].Name, Kind: kind, Body: ty, S: t.S}
		}
		for _, name := range kindVarNames {
			delete(ch.kindVars, name)
		}
		return ty
	case *syntax.TyExprRow:
		fields := make([]types.RowField, len(t.Fields))
		for i, f := range t.Fields {
			fields[i] = types.RowField{Label: f.Label, Type: ch.resolveTypeExpr(f.Type), S: f.S}
		}
		var tail types.Type
		if t.Tail != nil {
			tail = &types.TyVar{Name: t.Tail.Name, S: t.Tail.S}
		}
		return &types.TyEvidenceRow{
			Entries: &types.CapabilityEntries{Fields: fields},
			Tail:    tail,
			S:       t.S,
		}
	case *syntax.TyExprQual:
		body := ch.resolveTypeExpr(t.Body)
		// Single constraint: C a => T
		constraint := ch.resolveTypeExpr(t.Constraint)
		// Quantified constraint: (forall a. C1 a => C2 (f a)) => T
		if qc := ch.decomposeQuantifiedConstraint(constraint); qc != nil {
			entry := types.ConstraintEntry{
				ClassName:  qc.Head.ClassName,
				Args:       qc.Head.Args,
				Quantified: qc,
				S:          t.S,
			}
			if ev, ok := body.(*types.TyEvidence); ok {
				entries := make([]types.ConstraintEntry, 0, 1+len(ev.Constraints.Entries))
				entries = append(entries, entry)
				entries = append(entries, ev.Constraints.Entries...)
				return &types.TyEvidence{
					Constraints: &types.TyConstraintRow{Entries: entries},
					Body:        ev.Body,
					S:           t.S,
				}
			}
			return &types.TyEvidence{
				Constraints: &types.TyConstraintRow{Entries: []types.ConstraintEntry{entry}},
				Body:        body,
				S:           t.S,
			}
		}
		head, args := types.UnwindApp(constraint)
		if con, ok := head.(*types.TyCon); ok {
			entry := types.ConstraintEntry{ClassName: con.Name, Args: args, S: t.S}
			// Fold chained constraints into a single TyEvidence.
			if ev, ok := body.(*types.TyEvidence); ok {
				entries := make([]types.ConstraintEntry, 0, 1+len(ev.Constraints.Entries))
				entries = append(entries, entry)
				entries = append(entries, ev.Constraints.Entries...)
				return &types.TyEvidence{
					Constraints: &types.TyConstraintRow{Entries: entries},
					Body:        ev.Body,
					S:           t.S,
				}
			}
			return &types.TyEvidence{
				Constraints: types.SingleConstraint(con.Name, args),
				Body:        body,
				S:           t.S,
			}
		}
		ch.addCodedError(errs.ErrNoInstance, t.S, fmt.Sprintf("invalid constraint: %s", types.Pretty(constraint)))
		return body
	case *syntax.TyExprParen:
		return ch.resolveTypeExpr(t.Inner)
	default:
		return &types.TyError{}
	}
}

// decomposeQuantifiedConstraint checks if a resolved type is a quantified constraint
// (forall vars. context => head) and decomposes it into a QuantifiedConstraint.
// Returns nil if the type is not a quantified constraint.
func (ch *Checker) decomposeQuantifiedConstraint(ty types.Type) *types.QuantifiedConstraint {
	// Peel forall binders.
	var vars []types.ForallBinder
	current := ty
	for {
		if f, ok := current.(*types.TyForall); ok {
			vars = append(vars, types.ForallBinder{Name: f.Var, Kind: f.Kind})
			current = f.Body
		} else {
			break
		}
	}
	if len(vars) == 0 {
		return nil // not a quantified constraint
	}
	// Extract evidence: must be TyEvidence with at least one constraint entry for the head.
	ev, ok := current.(*types.TyEvidence)
	if !ok {
		return nil // forall a. T without => is not a quantified constraint
	}
	if len(ev.Constraints.Entries) == 0 {
		return nil
	}
	// The body of the evidence is the head constraint (after the last =>).
	headTy := ev.Body
	headHead, headArgs := types.UnwindApp(headTy)
	headCon, ok := headHead.(*types.TyCon)
	if !ok {
		return nil // head is not a class constraint
	}
	head := types.ConstraintEntry{ClassName: headCon.Name, Args: headArgs}
	// All entries in the evidence are context (premise) constraints.
	return &types.QuantifiedConstraint{
		Vars:    vars,
		Context: ev.Constraints.Entries,
		Head:    head,
	}
}

// tryExpandApp recognizes fully-saturated Computation and Thunk applications
// and produces the dedicated TyComp/TyThunk nodes, and expands type aliases.
func (ch *Checker) tryExpandApp(fun types.Type, arg types.Type, s span.Span) types.Type {
	// Computation pre post result: TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
	if app2, ok := fun.(*types.TyApp); ok {
		if app1, ok := app2.Fun.(*types.TyApp); ok {
			if con, ok := app1.Fun.(*types.TyCon); ok {
				switch con.Name {
				case "Computation":
					return &types.TyComp{Pre: app1.Arg, Post: app2.Arg, Result: arg, S: s}
				case "Thunk":
					return &types.TyThunk{Pre: app1.Arg, Post: app2.Arg, Result: arg, S: s}
				}
			}
		}
	}
	// General alias expansion: collect the TyApp spine and check if the
	// head is an alias with matching parameter count.
	result := &types.TyApp{Fun: fun, Arg: arg, S: s}
	head, args := types.UnwindApp(result)
	if con, ok := head.(*types.TyCon); ok {
		if info, ok := ch.aliases[con.Name]; ok && len(info.params) == len(args) {
			body := info.body
			for i, p := range info.params {
				body = types.Subst(body, p, args[i])
			}
			return body
		}
	}
	return nil
}

// inferPure handles the special form 'pure <expr>'.
// pure e : Computation r r a, elaborated to Core.Pure.
func (ch *Checker) inferPure(e *syntax.ExprApp) (types.Type, core.Core) {
	argTy, argCore := ch.infer(e.Arg)
	r := ch.freshMeta(types.KRow{})
	resultTy := types.MkComp(r, r, argTy)
	ch.trace(TraceInfer, e.S, "pure: %s ⇒ %s", types.Pretty(argTy), types.Pretty(resultTy))
	return resultTy, &core.Pure{Expr: argCore, S: e.S}
}

// inferBind handles the special form 'bind <comp> <cont>'.
// bind c (\x -> e) : Computation r1 r3 b, elaborated to Core.Bind.
func (ch *Checker) inferBind(compExpr, contExpr syntax.Expr, s span.Span) (types.Type, core.Core) {
	compTy, compCore := ch.infer(compExpr)

	r1 := ch.freshMeta(types.KRow{})
	r2 := ch.freshMeta(types.KRow{})
	a := ch.freshMeta(types.KType{})
	if err := ch.unifier.Unify(compTy, types.MkComp(r1, r2, a)); err != nil {
		ch.addSemanticUnifyError(errs.ErrBadComputation, err, compExpr.Span(), fmt.Sprintf("bind: first argument must be a computation, got %s", types.Pretty(compTy)))
		return ch.errorPair(s)
	}

	r3 := ch.freshMeta(types.KRow{})
	b := ch.freshMeta(types.KType{})

	var bindVar string
	var bodyCore core.Core

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
		bindVar = fmt.Sprintf("_bind_%d", ch.fresh())
		contExpected := types.MkArrow(ch.unifier.Zonk(a), types.MkComp(ch.unifier.Zonk(r2), r3, b))
		contCore := ch.check(contExpr, contExpected)
		bodyCore = &core.App{
			Fun: contCore,
			Arg: &core.Var{Name: bindVar, S: s},
			S:   s,
		}
	}

	resultTy := types.MkComp(ch.unifier.Zonk(r1), ch.unifier.Zonk(r3), ch.unifier.Zonk(b))
	ch.trace(TraceInfer, s, "bind: ⇒ %s", types.Pretty(resultTy))
	return resultTy, &core.Bind{Comp: compCore, Var: bindVar, Body: bodyCore, S: s}
}

// inferThunk handles the special form 'thunk <expr>'.
// The argument must have computation type Computation pre post a,
// and the result is Thunk pre post a, elaborated to Core.Thunk.
func (ch *Checker) inferThunk(e *syntax.ExprApp) (types.Type, core.Core) {
	argTy, argCore := ch.infer(e.Arg)
	argTy = ch.unifier.Zonk(argTy)

	// The argument must be a computation type.
	if comp, ok := argTy.(*types.TyComp); ok {
		resultTy := types.MkThunk(comp.Pre, comp.Post, comp.Result)
		ch.trace(TraceInfer, e.S, "thunk: %s ⇒ %s", types.Pretty(argTy), types.Pretty(resultTy))
		return resultTy, &core.Thunk{Comp: argCore, S: e.S}
	}

	// Try unifying with a fresh Computation type.
	pre := ch.freshMeta(types.KRow{})
	post := ch.freshMeta(types.KRow{})
	result := ch.freshMeta(types.KType{})
	expected := types.MkComp(pre, post, result)
	if err := ch.unifier.Unify(argTy, expected); err != nil {
		ch.addSemanticUnifyError(errs.ErrBadThunk, err, e.S, fmt.Sprintf("thunk requires a computation argument, got %s", types.Pretty(argTy)))
		return &types.TyError{S: e.S}, &core.Thunk{Comp: argCore, S: e.S}
	}
	resultTy := types.MkThunk(
		ch.unifier.Zonk(pre),
		ch.unifier.Zonk(post),
		ch.unifier.Zonk(result),
	)
	ch.trace(TraceInfer, e.S, "thunk: %s ⇒ %s", types.Pretty(argTy), types.Pretty(resultTy))
	return resultTy, &core.Thunk{Comp: argCore, S: e.S}
}

// inferForce handles direct application 'force <expr>'.
// The argument must have type Thunk pre post a,
// and the result is Computation pre post a, elaborated to Core.Force.
func (ch *Checker) inferForce(e *syntax.ExprApp) (types.Type, core.Core) {
	argTy, argCore := ch.infer(e.Arg)
	argTy = ch.unifier.Zonk(argTy)

	// The argument must be a thunk type.
	if thunk, ok := argTy.(*types.TyThunk); ok {
		resultTy := types.MkComp(thunk.Pre, thunk.Post, thunk.Result)
		ch.trace(TraceInfer, e.S, "force: %s ⇒ %s", types.Pretty(argTy), types.Pretty(resultTy))
		return resultTy, &core.Force{Expr: argCore, S: e.S}
	}

	// Try unifying with a fresh Thunk type.
	pre := ch.freshMeta(types.KRow{})
	post := ch.freshMeta(types.KRow{})
	result := ch.freshMeta(types.KType{})
	expected := types.MkThunk(pre, post, result)
	if err := ch.unifier.Unify(argTy, expected); err != nil {
		ch.addSemanticUnifyError(errs.ErrBadThunk, err, e.S, fmt.Sprintf("force requires a thunk argument, got %s", types.Pretty(argTy)))
		return &types.TyError{S: e.S}, &core.Force{Expr: argCore, S: e.S}
	}
	resultTy := types.MkComp(
		ch.unifier.Zonk(pre),
		ch.unifier.Zonk(post),
		ch.unifier.Zonk(result),
	)
	ch.trace(TraceInfer, e.S, "force: %s ⇒ %s", types.Pretty(argTy), types.Pretty(resultTy))
	return resultTy, &core.Force{Expr: argCore, S: e.S}
}

// inferList handles list literal [e1, e2, ...] by desugaring to Cons/Nil chain.
func (ch *Checker) inferList(e *syntax.ExprList) (types.Type, core.Core) {
	elemTy := ch.freshMeta(types.KType{})
	listTy := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: elemTy}

	// Build from the end: Nil, then Cons e_n (Cons e_{n-1} ...)
	var result core.Core = &core.Con{Name: "Nil", S: e.S}
	for i := len(e.Elems) - 1; i >= 0; i-- {
		elemCore := ch.check(e.Elems[i], elemTy)
		result = &core.App{
			Fun: &core.App{
				Fun: &core.Con{Name: "Cons", S: e.S},
				Arg: elemCore,
				S:   e.S,
			},
			Arg: result,
			S:   e.S,
		}
	}

	return ch.unifier.Zonk(listTy), result
}

// inferRecord infers the type of a record literal { l1 = e1, ..., ln = en }.
// Type: Record { l1 : T1, ..., ln : Tn }
func (ch *Checker) inferRecord(e *syntax.ExprRecord) (types.Type, core.Core) {
	fields := make([]types.RowField, len(e.Fields))
	coreFields := make([]core.RecordField, len(e.Fields))
	seen := make(map[string]bool, len(e.Fields))
	for i, f := range e.Fields {
		if seen[f.Label] {
			ch.addCodedError(errs.ErrDuplicateLabel, f.S,
				fmt.Sprintf("duplicate label %q in record literal", f.Label))
		}
		seen[f.Label] = true
		ty, coreVal := ch.infer(f.Value)
		fields[i] = types.RowField{Label: f.Label, Type: ty, S: f.S}
		coreFields[i] = core.RecordField{Label: f.Label, Value: coreVal}
	}
	row := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{Fields: fields},
		S:       e.S,
	}
	recTy := &types.TyApp{Fun: &types.TyCon{Name: "Record"}, Arg: row, S: e.S}
	return ch.unifier.Zonk(recTy), &core.RecordLit{Fields: coreFields, S: e.S}
}

// inferProject infers the type of a record projection r!#label.
func (ch *Checker) inferProject(e *syntax.ExprProject) (types.Type, core.Core) {
	recTy, recCore := ch.infer(e.Record)
	fieldTy := ch.matchRecordField(recTy, e.Label, e.S)
	return fieldTy, &core.RecordProj{Record: recCore, Label: e.Label, S: e.S}
}

// matchRecordField extracts a field's type from a Record row type.
// If the type is not a Record or the field doesn't exist, reports an error and returns a meta.
func (ch *Checker) matchRecordField(ty types.Type, label string, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	// Decompose TyApp(TyCon("Record"), row).
	if app, ok := ty.(*types.TyApp); ok {
		if con, ok := app.Fun.(*types.TyCon); ok && con.Name == "Record" {
			row := ch.unifier.Zonk(app.Arg)
			// Try to find the label in the row.
			if evRow, ok := row.(*types.TyEvidenceRow); ok {
				if cap, ok := evRow.Entries.(*types.CapabilityEntries); ok {
					for _, f := range cap.Fields {
						if f.Label == label {
							return f.Type
						}
					}
				}
			}
			// Row might be a meta or open row — unify to extract the field.
			fieldMeta := ch.freshMeta(types.KType{})
			tailMeta := ch.freshMeta(types.KRow{})
			expectedRow := &types.TyEvidenceRow{
				Entries: &types.CapabilityEntries{
					Fields: []types.RowField{{Label: label, Type: fieldMeta}},
				},
				Tail: tailMeta,
				S:    s,
			}
			if err := ch.unifier.Unify(row, expectedRow); err != nil {
				ch.addCodedError(errs.ErrRowMismatch, s, fmt.Sprintf("record has no field %s: %s", label, err.Error()))
				return ch.freshMeta(types.KType{})
			}
			return ch.unifier.Zonk(fieldMeta)
		}
	}
	// Type might be a meta — try to unify with Record { label : ?m | ?tail }.
	fieldMeta := ch.freshMeta(types.KType{})
	tailMeta := ch.freshMeta(types.KRow{})
	expectedRow := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{{Label: label, Type: fieldMeta}},
		},
		Tail: tailMeta,
		S:    s,
	}
	expectedRecTy := &types.TyApp{Fun: &types.TyCon{Name: "Record"}, Arg: expectedRow, S: s}
	if err := ch.unifier.Unify(ty, expectedRecTy); err != nil {
		ch.addCodedError(errs.ErrRowMismatch, s, fmt.Sprintf("expected record with field %s, got %s", label, types.Pretty(ty)))
		return ch.freshMeta(types.KType{})
	}
	return ch.unifier.Zonk(fieldMeta)
}

// inferRecordUpdate infers the type of a record update { r | l1 = e1, ..., ln = en }.
func (ch *Checker) inferRecordUpdate(e *syntax.ExprRecordUpdate) (types.Type, core.Core) {
	recTy, recCore := ch.infer(e.Record)
	coreUpdates := make([]core.RecordField, len(e.Updates))
	seen := make(map[string]bool, len(e.Updates))
	for i, upd := range e.Updates {
		if seen[upd.Label] {
			ch.addCodedError(errs.ErrDuplicateLabel, upd.S,
				fmt.Sprintf("duplicate label %q in record update", upd.Label))
		}
		seen[upd.Label] = true
		// Infer the update value type, then check it matches the existing field.
		fieldTy := ch.matchRecordField(recTy, upd.Label, upd.S)
		updCore := ch.check(upd.Value, fieldTy)
		coreUpdates[i] = core.RecordField{Label: upd.Label, Value: updCore}
	}
	return recTy, &core.RecordUpdate{Record: recCore, Updates: coreUpdates, S: e.S}
}

// checkRecordPattern checks a record pattern { l1 = p1, ..., ln = pn } against a scrutinee type.
func (ch *Checker) checkRecordPattern(p *syntax.PatRecord, scrutTy types.Type) (core.Pattern, map[string]types.Type, map[int]string, bool) {
	bindings := make(map[string]types.Type)
	coreFields := make([]core.PRecordField, len(p.Fields))
	seen := make(map[string]bool, len(p.Fields))
	for i, f := range p.Fields {
		if seen[f.Label] {
			ch.addCodedError(errs.ErrDuplicateLabel, f.S,
				fmt.Sprintf("duplicate label %q in record pattern", f.Label))
		}
		seen[f.Label] = true
		fieldTy := ch.matchRecordField(scrutTy, f.Label, f.S)
		corePat, fieldBindings, _, _ := ch.checkPattern(f.Pattern, fieldTy)
		coreFields[i] = core.PRecordField{Label: f.Label, Pattern: corePat}
		for k, v := range fieldBindings {
			bindings[k] = v
		}
	}
	return &core.PRecord{Fields: coreFields, S: p.S}, bindings, nil, false
}

func (ch *Checker) resolveKindExpr(k syntax.KindExpr) types.Kind {
	if k == nil {
		return types.KType{}
	}
	switch ke := k.(type) {
	case *syntax.KindExprType:
		return types.KType{}
	case *syntax.KindExprRow:
		return types.KRow{}
	case *syntax.KindExprConstraint:
		return types.KConstraint{}
	case *syntax.KindExprArrow:
		return &types.KArrow{From: ch.resolveKindExpr(ke.From), To: ch.resolveKindExpr(ke.To)}
	case *syntax.KindExprName:
		if ch.kindVars[ke.Name] {
			return types.KVar{Name: ke.Name}
		}
		if pk, ok := ch.promotedKinds[ke.Name]; ok {
			return pk
		}
		return types.KType{}
	case *syntax.KindExprSort:
		return types.KSort{}
	default:
		return types.KType{}
	}
}

// checkTypeAppKind validates that a type application F A is kind-correct.
// Only checks when:
//   - F has an explicitly annotated parameter kind (not the default KType)
//   - A is a concrete type constructor (TyCon or TyApp) with a deterministic kind
//
// This avoids false positives from type variables whose kind isn't yet in context.
func (ch *Checker) checkTypeAppKind(fun, arg types.Type, s span.Span) {
	// Only check when arg has a deterministic kind (concrete TyCon, not TyVar).
	if !ch.hasDeterministicKind(arg) {
		return
	}
	funKind := ch.kindOfType(fun)
	if funKind == nil {
		return
	}
	funKind = ch.unifier.ZonkKind(funKind)
	ka, ok := funKind.(*types.KArrow)
	if !ok {
		return
	}
	// Skip if the parameter kind is the default KType (unannotated parameter).
	if _, isType := ka.From.(types.KType); isType {
		return
	}
	argKind := ch.kindOfType(arg)
	if argKind == nil {
		return
	}
	argKind = ch.unifier.ZonkKind(argKind)
	if _, isMeta := argKind.(*types.KMeta); isMeta {
		return
	}
	if err := ch.unifier.UnifyKinds(ka.From, argKind); err != nil {
		ch.addCodedError(errs.ErrKindMismatch, s,
			fmt.Sprintf("kind mismatch in type application: expected kind %s, got %s", ka.From, argKind))
	}
}

// hasDeterministicKind returns true if the type's kind is deterministic
// (i.e., derived from a registered type constructor, not a defaulted TyVar).
func (ch *Checker) hasDeterministicKind(ty types.Type) bool {
	switch t := ty.(type) {
	case *types.TyCon:
		_, inReg := ch.config.RegisteredTypes[t.Name]
		_, inProm := ch.promotedCons[t.Name]
		_, isAlias := ch.aliases[t.Name]
		return inReg || inProm || isAlias
	case *types.TyApp:
		// Recurse on the head to check if it's deterministic.
		head, _ := types.UnwindApp(ty)
		if head != ty {
			return ch.hasDeterministicKind(head)
		}
		return false
	case *types.TyMeta:
		return true
	case *types.TySkolem:
		return true
	default:
		return false
	}
}
