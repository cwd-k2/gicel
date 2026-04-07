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
	ue, ok := err.(unify.UnifyError)
	if !ok {
		return diagnostic.ErrTypeMismatch
	}
	switch ue.Kind() {
	case unify.UnifyOccursCheck:
		return diagnostic.ErrOccursCheck
	case unify.UnifyDupLabel:
		return diagnostic.ErrDuplicateLabel
	case unify.UnifyRowMismatch:
		return diagnostic.ErrRowMismatch
	case unify.UnifySkolemRigid:
		return diagnostic.ErrSkolemRigid
	case unify.UnifyUntouchable:
		return diagnostic.ErrUntouchable
	default:
		return diagnostic.ErrTypeMismatch
	}
}

// addUnifyError maps a unification error to the appropriate structured error code.
// Used at general type-mismatch sites where the UnifyError kind IS the primary diagnosis.
func (s *CheckState) addUnifyError(err error, sp span.Span, ctx string) {
	// Avoid redundant detail when the unifier reports a simple type mismatch —
	// the context already contains the type expectation. Only the
	// MismatchError variant carries both type sides; other UnifyMismatch-kind
	// variants (Grade/Level/Message) need their detail message appended.
	if _, ok := err.(*unify.MismatchError); ok {
		s.addCodedError(unifyErrorCode(err), sp, ctx)
		return
	}
	s.addCodedError(unifyErrorCode(err), sp, ctx+": "+err.Error())
}

// addSemanticUnifyError reports a unification failure with a semantic error code.
// For simple mismatches, the semantic code and message are used as-is.
// For specific failures (occurs check, skolem rigidity, etc.), the underlying
// unification error overrides the semantic code — it reveals the root cause.
func (s *CheckState) addSemanticUnifyError(semanticCode diagnostic.Code, err error, sp span.Span, ctx string) {
	code := unifyErrorCode(err)
	if code == diagnostic.ErrTypeMismatch {
		s.addCodedError(semanticCode, sp, ctx)
		return
	}
	s.addCodedError(code, sp, ctx+": "+err.Error())
}

// recordType calls the TypeRecorder callback if configured.
// Used via defer with a pointer to the named return value so that
// the final type is captured regardless of which return path is taken.
func (ch *Checker) recordType(sp span.Span, ty *types.Type) {
	if ch.config.TypeRecorder != nil && *ty != nil {
		ch.config.TypeRecorder(sp, ch.unifier.Zonk(*ty))
	}
}

// infer produces a type for an expression and a Core IR node.
func (ch *Checker) infer(expr syntax.Expr) (ty types.Type, core ir.Core) {
	defer ch.recordType(expr.Span(), &ty)
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
		if ch.config.Trace != nil {
			ch.trace(TraceInfer, e.S, "infer: %s ⇒ %s", e.Name, types.Pretty(ty))
		}
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
		if ch.config.Trace != nil {
			ch.trace(TraceInfer, e.S, "infer: %s.%s ⇒ %s", e.Qualifier, e.Name, types.Pretty(ty))
		}
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
			if v, ok := inner.Fun.(*syntax.ExprVar); ok {
				switch v.Name {
				case "bind":
					return ch.inferBind(inner.Arg, e.Arg, e.S)
				default:
					if isMergeOp(v.Name) {
						// Only intercept if merge/*** is from Core (not user-defined in current module).
						if _, mod, ok := ch.ctx.LookupVarFull(v.Name); !ok || mod != "" {
							return ch.inferMerge(inner.Arg, e.Arg, e.S)
						}
					}
				}
			}
		}
		// fix/rec in infer context: produce ir.Fix nodes directly.
		if v, ok := e.Fun.(*syntax.ExprVar); ok && (v.Name == "fix" || v.Name == "rec") {
			if ch.config.GatedBuiltins != nil && ch.config.GatedBuiltins[v.Name] {
				if lam := fixArgLam(e.Arg); lam != nil {
					return ch.inferFix(e, lam, v.Name == "rec")
				}
			}
		}
		funTy, funCore := ch.infer(e.Fun)
		return ch.inferApply(funTy, funCore, e.Arg, e.S)

	case *syntax.ExprTyApp:
		// Delegate to inferHead (which preserves foralls) then instantiate remaining.
		ty, coreExpr := ch.inferHead(e)
		return ch.instantiate(ty, coreExpr)

	case *syntax.ExprAnn:
		ty := ch.resolveTypeExpr(e.AnnType)
		coreExpr := ch.check(e.Expr, ty)
		return ty, coreExpr

	case *syntax.ExprInfixSpine:
		panic("internal: unresolved ExprInfixSpine reached type checker")

	case *syntax.ExprInfix:
		// Special form: merge / *** as infix operator.
		if isMergeOp(e.Op) {
			if _, mod, ok := ch.ctx.LookupVarFull(e.Op); !ok || mod != "" {
				return ch.inferMerge(e.Left, e.Right, e.S)
			}
		}
		// Desugar: a op b → App(App(Var(op), a), b)
		opTy, opMod, ok := ch.ctx.LookupVarFull(e.Op)
		if !ok {
			msg := "unbound operator: " + e.Op
			if hints := ch.suggestVar(e.Op); len(hints) > 0 {
				ch.addCodedErrorWithHints(diagnostic.ErrUnboundVar, e.S, msg, hints)
			} else {
				ch.addCodedError(diagnostic.ErrUnboundVar, e.S, msg)
			}
			return &types.TyError{S: e.S}, &ir.Var{Name: e.Op, S: e.S}
		}
		opTy, opCore := ch.instantiate(opTy, &ir.Var{Name: e.Op, Module: opMod, S: e.S})
		ret1Ty, app1Core := ch.inferApply(opTy, opCore, e.Left, e.S)
		return ch.inferApply(ret1Ty, app1Core, e.Right, e.S)

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
		paramTy := ch.freshMeta(types.TypeOfTypes)
		retTy := ch.freshMeta(types.TypeOfTypes)
		lamCore := ch.checkLam(e, types.MkArrow(paramTy, retTy))
		return ch.unifier.Zonk(types.MkArrow(paramTy, retTy)), lamCore

	case *syntax.ExprCase:
		return ch.inferCase(e)

	case *syntax.ExprIntLit:
		val, err := strconv.ParseInt(strings.ReplaceAll(e.Value, "_", ""), 10, 64)
		if err != nil {
			ch.addCodedError(diagnostic.ErrTypeMismatch, e.S, "invalid integer literal: "+e.Value)
			return ch.errorPair(e.S)
		}
		return types.Con("Int"), &ir.Lit{Value: val, S: e.S}

	case *syntax.ExprStrLit:
		return types.Con("String"), &ir.Lit{Value: e.Value, S: e.S}

	case *syntax.ExprDoubleLit:
		val, err := strconv.ParseFloat(strings.ReplaceAll(e.Value, "_", ""), 64)
		if err != nil {
			ch.addCodedError(diagnostic.ErrTypeMismatch, e.S, "invalid double literal: "+e.Value)
			return ch.errorPair(e.S)
		}
		return types.Con("Double"), &ir.Lit{Value: val, S: e.S}

	case *syntax.ExprRuneLit:
		return types.Con("Rune"), &ir.Lit{Value: e.Value, S: e.S}

	case *syntax.ExprList:
		return ch.inferList(e)

	case *syntax.ExprRecord:
		return ch.inferRecord(e)

	case *syntax.ExprRecordUpdate:
		return ch.inferRecordUpdate(e)

	case *syntax.ExprProject:
		return ch.inferProject(e)

	case *syntax.ExprEvidence:
		return ch.inferEvidence(e)

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
	// Record the checked type for IDE hover. The expected type (post-zonk)
	// is the type this expression is used at.
	if ch.config.TypeRecorder != nil {
		defer ch.config.TypeRecorder(expr.Span(), ch.unifier.Zonk(expected))
	}

	// Reduce type family applications in the expected type so that checking
	// can decompose the result (e.g., `F (Pi Set Set)` → `Unit -> Unit`).
	// NOTE: Type family reduction is NOT done here globally because it can
	// change type identity and break computation boundary checks (e.g.,
	// DualDual(S) ≠ S after reduction). Reduction happens on demand in
	// matchArrow and the unifier.

	// Polymorphic fix/rec: intercept before forall peeling so self
	// gets the full expected type, enabling polymorphic recursion.
	if app, ok := expr.(*syntax.ExprApp); ok {
		if v, ok := app.Fun.(*syntax.ExprVar); ok && (v.Name == "fix" || v.Name == "rec") {
			if ch.config.GatedBuiltins != nil && ch.config.GatedBuiltins[v.Name] {
				if lam := fixArgLam(app.Arg); lam != nil {
					return ch.checkFix(app, lam, expected, v.Name == "rec")
				}
			}
		}
	}

	// If the expected type is a forall, introduce a TyLam and check the body
	// against the quantified type. This implements the spec rule:
	//   ⟦ e : \ a:K. T ⟧ = TyLam(a, K, ⟦e: T⟧)
	if f, ok := expected.(*types.TyForall); ok {
		if isLevelKind(f.Kind) {
			// Level quantifier: introduce a fresh skolem.
			// Substitute in both level positions (TyCon.Level via SubstLevel)
			// and type positions (TyVar via Subst with a kind-level TyVar).
			freshName := fmt.Sprintf("%s$%d", f.Var, ch.fresh())
			body := types.SubstLevel(f.Body, f.Var, &types.LevelVar{Name: freshName})
			body = types.Subst(body, f.Var, &types.TyVar{Name: freshName})
			bodyCore := ch.check(expr, body)
			return &ir.TyLam{TyParam: f.Var, Kind: f.Kind, Body: bodyCore, S: expr.Span()}
		}
		if isSortKind(f.Kind) {
			// Kind-level quantifier: introduce a fresh kind skolem (TyVar)
			// and substitute in all kind positions.
			freshName := fmt.Sprintf("%s$%d", f.Var, ch.fresh())
			body := types.Subst(f.Body, f.Var, &types.TyVar{Name: freshName})
			bodyCore := ch.check(expr, body)
			return &ir.TyLam{TyParam: f.Var, Kind: f.Kind, Body: bodyCore, S: expr.Span()}
		}
		ch.enterSolverScope()
		preID := ch.freshID // belt-and-suspenders scope boundary
		skolem := ch.freshSkolem(f.Var, f.Kind)
		ch.ctx.Push(&CtxTyVar{Name: f.Var, Kind: f.Kind})
		bodyCore := ch.check(expr, types.Subst(f.Body, f.Var, skolem))
		ch.ctx.Pop()
		ch.exitSolverScope()
		// Belt-and-suspenders: verify skolem didn't leak into outer solutions.
		// Touchability (when enabled) prevents this structurally; this check
		// detects level-system bugs.
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

	case *syntax.ExprInfixSpine:
		panic("internal: unresolved ExprInfixSpine reached type checker")

	case *syntax.ExprInfix:
		return ch.checkInfix(e, expected)

	case *syntax.ExprSection:
		return ch.checkSection(e, expected)

	case *syntax.ExprParen:
		return ch.check(e.Inner, expected)

	case *syntax.ExprEvidence:
		return ch.checkEvidence(e, expected)

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
	var dicts []dictInfo
	var givenEqSkolems []int // skolem IDs with installed given equalities
	pushed := 0
	for _, entry := range ev.Constraints.ConEntries() {
		// Equality constraint: install as a given equality (definition site).
		// If one side is a skolem, use InstallGivenEq so that the skolem
		// is locally equal to the other side within this body.
		// At the call site (bidir_lookup.go), this becomes a wanted CtEq.
		if entry.IsEquality {
			lhs := ch.unifier.Zonk(entry.EqLhs)
			rhs := ch.unifier.Zonk(entry.EqRhs)
			if sk, ok := lhs.(*types.TySkolem); ok {
				ch.unifier.InstallGivenEq(sk.ID, rhs)
				ch.emitGivenEq(lhs, rhs, entry.S)
				givenEqSkolems = append(givenEqSkolems, sk.ID)
			} else if sk, ok := rhs.(*types.TySkolem); ok {
				ch.unifier.InstallGivenEq(sk.ID, lhs)
				ch.emitGivenEq(lhs, rhs, entry.S)
				givenEqSkolems = append(givenEqSkolems, sk.ID)
			} else if types.ContainsSkolemOrFamily(lhs) || types.ContainsSkolemOrFamily(rhs) {
				// Type family application or skolem present — emit as given.
				// The equality is assumed to hold at the definition site;
				// it becomes a wanted at the call site (bidir_lookup.go).
				ch.emitGivenEq(lhs, rhs, entry.S)
			} else {
				// Both sides are concrete or meta — emit wanted for checking.
				ch.emitEq(lhs, rhs, entry.S, nil)
			}
			continue
		}
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
		dictParam := ch.freshDictName(className)
		dicts = append(dicts, dictInfo{param: dictParam, ty: dictTy})
		ch.ctx.Push(&CtxVar{Name: dictParam, Type: dictTy, DictClassName: className})
		pushed++
		ch.ctx.Push(&CtxEvidence{
			ClassName:  className,
			Args:       args,
			DictName:   dictParam,
			DictType:   dictTy,
			Quantified: entry.Quantified,
		})
		pushed++
	}
	savedWorklist := ch.solver.SaveWorklist()
	bodyCore := ch.check(expr, ev.Body)
	bodyCore = ch.resolveDeferredConstraints(bodyCore)
	ch.solver.RestoreWorklistAppend(savedWorklist)
	for range pushed {
		ch.ctx.Pop()
	}
	// Remove given equalities scoped to this evidence body.
	for _, skolemID := range givenEqSkolems {
		ch.unifier.RemoveGivenEq(skolemID)
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
//
// Precondition: expected must already be zonked by the caller.
// inferred is zonked here because infer results may contain unresolved metas.
func (ch *Checker) subsCheck(inferred, expected types.Type, expr ir.Core, s span.Span) ir.Core {
	inferred = ch.unifier.Zonk(inferred)

	// Inferred ∀a. A ≤ B  →  instantiate a, check A[a:=?m] ≤ B
	if f, ok := inferred.(*types.TyForall); ok {
		if isLevelKind(f.Kind) {
			// Level quantifier: instantiate with a fresh LevelMeta + TyMeta.
			lm := ch.unifier.FreshLevelMeta()
			km := ch.freshMeta(types.SortZero)
			body := types.SubstLevel(f.Body, f.Var, lm)
			body = types.Subst(body, f.Var, km)
			return ch.subsCheck(body, expected, expr, s)
		}
		if isSortKind(f.Kind) {
			// Kind-level quantifier: instantiate with a fresh kind metavariable
			km := ch.freshMeta(types.SortZero)
			body := types.Subst(f.Body, f.Var, km)
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
			placeholder := ch.freshName(prefixDictDefer)
			ch.emitClassConstraint(placeholder, entry, s)
			expr = &ir.App{Fun: expr, Arg: &ir.Var{Name: placeholder, S: s}, S: s}
		}
		return ch.subsCheck(ev.Body, expected, expr, s)
	}

	// Default: unify eagerly. subsCheck is on the critical path for type
	// information flow — metas must be solved immediately for downstream code.
	if err := ch.unifier.Unify(inferred, expected); err != nil {
		ch.addUnifyError(err, s, "type mismatch: expected "+types.Pretty(expected)+", got "+types.Pretty(inferred))
	}
	return expr
}

// inferEvidence handles `value => expr` in infer mode.
func (ch *Checker) inferEvidence(e *syntax.ExprEvidence) (types.Type, ir.Core) {
	var bodyTy types.Type
	core := ch.withEvidenceScope(e, func() ir.Core {
		var bodyCore ir.Core
		bodyTy, bodyCore = ch.infer(e.Body)
		return bodyCore
	})
	return bodyTy, core
}

// checkEvidence handles `value => expr` in check mode.
func (ch *Checker) checkEvidence(e *syntax.ExprEvidence, expected types.Type) ir.Core {
	return ch.withEvidenceScope(e, func() ir.Core {
		return ch.check(e.Body, expected)
	})
}

// withEvidenceScope handles the shared push/pop/resolve protocol for
// scoped evidence injection. The body callback runs with the evidence
// in scope; deferred constraints are resolved before cleanup.
func (ch *Checker) withEvidenceScope(e *syntax.ExprEvidence, body func() ir.Core) ir.Core {
	dictTy, dictCore := ch.infer(e.Dict)
	bindName := ch.freshName("$ev")
	ch.ctx.Push(&CtxVar{Name: bindName, Type: dictTy})
	pushedEvidence := ch.pushEvidenceFromDictType(bindName, dictTy)
	bodyCore := body()
	bodyCore = ch.resolveDeferredConstraints(bodyCore)
	if pushedEvidence {
		ch.ctx.Pop() // pop CtxEvidence
	}
	ch.ctx.Pop() // pop CtxVar
	lamCore := &ir.Lam{Param: bindName, ParamType: dictTy, Body: bodyCore, Generated: true, S: e.S}
	return &ir.App{Fun: lamCore, Arg: dictCore, S: e.S}
}

// pushEvidenceFromDictType decomposes a dictionary type into class name + args
// and pushes a CtxEvidence entry. Returns true if a CtxEvidence was pushed.
func (ch *Checker) pushEvidenceFromDictType(bindName string, dictTy types.Type) bool {
	dictTy = ch.unifier.Zonk(dictTy)
	head, args := types.UnwindApp(dictTy)
	if con, ok := head.(*types.TyCon); ok {
		if className, ok := ch.reg.ClassFromDict(con.Name); ok {
			ch.ctx.Push(&CtxEvidence{
				ClassName: className,
				Args:      args,
				DictName:  bindName,
				DictType:  dictTy,
			})
			return true
		}
	}
	return false
}
