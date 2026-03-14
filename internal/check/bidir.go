package check

import (
	"fmt"
	"strconv"

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
		return ch.checkWithEvidence(expr, ev)
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
		groupID := ch.fresh()
		for _, entry := range ev.Constraints.ConEntries() {
			placeholder := fmt.Sprintf("%s_%d", prefixDictDefer, ch.fresh())
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

	// Desugar structured patterns: \pat -> body  →  \$p -> case $p { pat -> body }
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
	var alts []core.Alt
	for _, alt := range e.Alts {
		pr := ch.checkPattern(alt.Pattern, scrutTy)
		for name, ty := range pr.Bindings {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
		// Isolate deferred constraints when the branch introduces evidence bindings
		// (existential skolems or Dict-style reified evidence). Evidence is only
		// in scope within the alt body, so constraints must be resolved here.
		needsLocalResolve := len(pr.SkolemIDs) > 0 || pr.HasEvidence
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
		for range pr.Bindings {
			ch.ctx.Pop()
		}
		// Existential escape check: skolems must not appear in the result type.
		if len(pr.SkolemIDs) > 0 {
			ch.checkSkolemEscape(ch.unifier.Zonk(resultTy), pr.SkolemIDs, alt.Body.Span())
		}
		alts = append(alts, core.Alt{Pattern: pr.Pattern, Body: bodyCore, S: alt.S})
	}
	ch.checkExhaustive(scrutTy, alts, e.S)
	return &core.Case{Scrutinee: scrutCore, Alts: alts, S: e.S}
}

// patternResult holds the outputs of pattern checking.
type patternResult struct {
	Pattern     core.Pattern
	Bindings    map[string]types.Type
	SkolemIDs   map[int]string
	HasEvidence bool
}

func (ch *Checker) checkPattern(pat syntax.Pattern, scrutTy types.Type) patternResult {
	switch p := pat.(type) {
	case *syntax.PatVar:
		return patternResult{
			Pattern:  &core.PVar{Name: p.Name, S: p.S},
			Bindings: map[string]types.Type{p.Name: scrutTy},
		}
	case *syntax.PatWild:
		return patternResult{Pattern: &core.PWild{S: p.S}}
	case *syntax.PatCon:
		return ch.checkConPattern(p, scrutTy)
	case *syntax.PatRecord:
		return ch.checkRecordPattern(p, scrutTy)
	case *syntax.PatParen:
		return ch.checkPattern(p.Inner, scrutTy)
	default:
		return patternResult{Pattern: &core.PWild{S: pat.Span()}}
	}
}

// pendingCV tracks a constraint variable entry whose class/args are unknown
// until return type unification resolves the meta.
type pendingCV struct {
	constraintVar types.Type
	dictParam     string
}

// instantiateConForalls peels outer foralls from a constructor type,
// classifying each variable as universal (meta) or existential (skolem).
// Returns the body type after substitution and a map of skolem IDs.
func (ch *Checker) instantiateConForalls(conTy types.Type) (types.Type, map[int]string) {
	// Collect forall vars.
	type fvar struct {
		name string
		kind types.Kind
	}
	var forallVars []fvar
	tmpTy := conTy
	for {
		if f, ok := tmpTy.(*types.TyForall); ok {
			forallVars = append(forallVars, fvar{name: f.Var, kind: f.Kind})
			tmpTy = f.Body
		} else {
			break
		}
	}

	// Get the return type's free vars (strip arrows from after foralls).
	_, retTy := decomposeConSig(conTy)
	retFreeVars := types.FreeVars(retTy)

	// Classify each forall var: universal (in return type) → meta, existential → skolem.
	currentTy := conTy
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
	return currentTy, skolemIDs
}

func (ch *Checker) checkConPattern(p *syntax.PatCon, scrutTy types.Type) patternResult {
	conTy, ok := ch.conTypes[p.Con]
	if !ok {
		ch.addCodedError(errs.ErrUnboundCon, p.S, fmt.Sprintf("unknown constructor in pattern: %s", p.Con))
		return patternResult{Pattern: &core.PWild{S: p.S}}
	}
	conTy = ch.unifier.Zonk(conTy)
	var args []core.Pattern
	bindings := make(map[string]types.Type)

	currentTy, skolemIDs := ch.instantiateConForalls(conTy)

	// 4. Peel constraints — generate dict bindings and pattern args for existential constraints.
	// For ConstraintVar entries, the concrete className/args are unknown until
	// return type unification (step 6). Record them and resolve after unification.
	var pendingCVs []pendingCV
	for {
		if ev, ok := currentTy.(*types.TyEvidence); ok {
			for _, entry := range ev.Constraints.ConEntries() {
				if entry.ConstraintVar != nil && entry.ClassName == "" {
					dictParam := fmt.Sprintf("%s_%d", prefixDictCV, ch.fresh())
					pendingCVs = append(pendingCVs, pendingCV{
						constraintVar: entry.ConstraintVar,
						dictParam:     dictParam,
					})
					args = append(args, &core.PVar{Name: dictParam, S: p.S})
				} else {
					dictParam := fmt.Sprintf("%s_%s_%d", prefixDict, entry.ClassName, ch.fresh())
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

	mkResult := func() patternResult {
		return patternResult{
			Pattern:     &core.PCon{Con: p.Con, Args: args, S: p.S},
			Bindings:    bindings,
			SkolemIDs:   skolemIDs,
			HasEvidence: len(pendingCVs) > 0,
		}
	}

	// 5. Peel arrow arguments matching user-supplied pattern args.
	for _, argPat := range p.Args {
		argTy, restTy := ch.matchArrow(currentTy, p.S)
		child := ch.checkPattern(argPat, argTy)
		args = append(args, child.Pattern)
		for k, v := range child.Bindings {
			bindings[k] = v
		}
		for k, v := range child.SkolemIDs {
			skolemIDs[k] = v
		}
		currentTy = restTy
	}
	// 6. Unify result type with scrutinee type.
	if ch.isInaccessibleGADTBranch(p.Con, scrutTy) {
		return mkResult()
	}
	if err := ch.unifier.Unify(currentTy, scrutTy); err != nil {
		ch.addUnifyError(err, p.S, "constructor type mismatch")
		return mkResult()
	}
	// 7. Resolve pending constraint variable entries now that metas are solved.
	ch.resolvePendingCVs(pendingCVs, bindings)
	return mkResult()
}

// isInaccessibleGADTBranch returns true if the constructor's return type
// cannot unify with the scrutinee, making the branch inaccessible.
func (ch *Checker) isInaccessibleGADTBranch(conName string, scrutTy types.Type) bool {
	info := ch.conInfo[conName]
	if info == nil {
		return false
	}
	for _, c := range info.Constructors {
		if c.Name == conName && c.ReturnType != nil {
			if !ch.canUnifyWith(c.ReturnType, scrutTy) {
				return true
			}
		}
	}
	return false
}

// resolvePendingCVs resolves deferred constraint variable entries after metas are solved.
func (ch *Checker) resolvePendingCVs(pending []pendingCV, bindings map[string]types.Type) {
	for _, pcv := range pending {
		cv := ch.unifier.Zonk(pcv.constraintVar)
		if cn, cArgs, ok := DecomposeConstraintType(cv); ok {
			dictTy := ch.buildDictType(cn, cArgs)
			bindings[pcv.dictParam] = dictTy
		}
	}
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
			for _, entry := range ev.Constraints.ConEntries() {
				placeholder := fmt.Sprintf("%s_%d", prefixDictDefer, ch.fresh())
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
		bindVar = fmt.Sprintf("%s_%d", prefixBind, ch.fresh())
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

// cbpvTriple extracts (pre, post, result) from a computation or thunk type.
// Returns nil fields if the type is neither.
func cbpvTriple(ty types.Type) (pre, post, result types.Type) {
	switch t := ty.(type) {
	case *types.TyComp:
		return t.Pre, t.Post, t.Result
	case *types.TyThunk:
		return t.Pre, t.Post, t.Result
	}
	return nil, nil, nil
}

// inferDualForm infers the CBPV dual: thunk (Comp→Thunk) or force (Thunk→Comp).
func (ch *Checker) inferDualForm(
	e *syntax.ExprApp, label string,
	mkExpected func(pre, post, result types.Type) types.Type,
	mkResult func(pre, post, result types.Type) types.Type,
	mkCore func(argCore core.Core) core.Core,
) (types.Type, core.Core) {
	argTy, argCore := ch.infer(e.Arg)
	argTy = ch.unifier.Zonk(argTy)

	// Fast path: direct triple extraction.
	if pre, post, result := cbpvTriple(argTy); pre != nil {
		resultTy := mkResult(pre, post, result)
		ch.trace(TraceInfer, e.S, "%s: %s ⇒ %s", label, types.Pretty(argTy), types.Pretty(resultTy))
		return resultTy, mkCore(argCore)
	}

	// Fallback: unify with a fresh triple.
	pre := ch.freshMeta(types.KRow{})
	post := ch.freshMeta(types.KRow{})
	result := ch.freshMeta(types.KType{})
	expected := mkExpected(pre, post, result)
	if err := ch.unifier.Unify(argTy, expected); err != nil {
		ch.addSemanticUnifyError(errs.ErrBadThunk, err, e.S,
			fmt.Sprintf("%s requires a %s argument, got %s", label, types.Pretty(expected), types.Pretty(argTy)))
		return &types.TyError{S: e.S}, mkCore(argCore)
	}
	resultTy := mkResult(ch.unifier.Zonk(pre), ch.unifier.Zonk(post), ch.unifier.Zonk(result))
	ch.trace(TraceInfer, e.S, "%s: %s ⇒ %s", label, types.Pretty(argTy), types.Pretty(resultTy))
	return resultTy, mkCore(argCore)
}

func (ch *Checker) inferThunk(e *syntax.ExprApp) (types.Type, core.Core) {
	return ch.inferDualForm(e, "thunk",
		func(p, q, r types.Type) types.Type { return types.MkComp(p, q, r) },
		func(p, q, r types.Type) types.Type { return types.MkThunk(p, q, r) },
		func(c core.Core) core.Core { return &core.Thunk{Comp: c, S: e.S} },
	)
}

func (ch *Checker) inferForce(e *syntax.ExprApp) (types.Type, core.Core) {
	return ch.inferDualForm(e, "force",
		func(p, q, r types.Type) types.Type { return types.MkThunk(p, q, r) },
		func(p, q, r types.Type) types.Type { return types.MkComp(p, q, r) },
		func(c core.Core) core.Core { return &core.Force{Expr: c, S: e.S} },
	)
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
