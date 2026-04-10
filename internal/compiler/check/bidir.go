package check

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

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
		ch.addDiag(diagnostic.ErrNestingLimit, expr.Span(), diagWithErr{Context: "nesting limit", Err: err})
		return &types.TyError{S: expr.Span()}, &ir.Lit{Value: nil, S: expr.Span()}
	}
	defer ch.budget.Unnest()

	switch e := expr.(type) {
	case *syntax.ExprVar:
		// `thunk` and `force` are pure syntactic special forms with
		// no first-class runtime representation. `thunk e` elaborates
		// to ir.Thunk, `force e` elaborates to ir.Force, and all
		// indirect uses (do bindings, case arms, handler arguments,
		// entry-point bindings) are covered by the type-directed
		// CBPV auto-coercion. A bare reference is therefore a
		// surface-level mistake — `thunk` can never be a function in
		// CBV (it would capture its argument evaluated), and `force`
		// is kept symmetric for conceptual uniformity even though a
		// `\thk. force thk` lambda would be semantically valid. The
		// error message points at the applied form and at the
		// coercion path so users understand both options.
		if e.Name == "thunk" || e.Name == "force" {
			ch.addDiag(diagnostic.ErrSpecialForm, e.S,
				diagMsg(e.Name+" requires an argument: use `"+e.Name+" <expr>` or let the CBPV auto-coercion insert it at a Thunk/Computation mismatch"))
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
		// Transparent rewrite: `f $ x` is pure forward application
		// (`($) := \f x. f x` in Prelude). Unwrapping to the direct App
		// shape at the checker level aligns the compiler's view with
		// the operator's semantic definition — `fix $ lam` reaches the
		// same special-form detection path as `fix (lam)`, and the
		// checkFix / inferFix intercept fires identically. The inliner
		// cannot recover this flattening after the fact because `fix`
		// is a runtime builtin with no compile-time IR body. Mirrors
		// the merge/*** transparency pattern below and honors user
		// shadowing in the current module.
		if isDollarOp(e.Op) {
			if _, mod, ok := ch.ctx.LookupVarFull(e.Op); !ok || mod != "" {
				return ch.infer(&syntax.ExprApp{Fun: e.Left, Arg: e.Right, S: e.S})
			}
		}
		// Special form: merge / *** as infix operator.
		if isMergeOp(e.Op) {
			if _, mod, ok := ch.ctx.LookupVarFull(e.Op); !ok || mod != "" {
				return ch.inferMerge(e.Left, e.Right, e.S)
			}
		}
		// Desugar: a op b → App(App(Var(op), a), b)
		opTy, opMod, ok := ch.ctx.LookupVarFull(e.Op)
		if !ok {
			detail := diagUnknown{Kind: "operator", Name: e.Op}
			if hints := ch.suggestVar(e.Op); len(hints) > 0 {
				ch.addDiagHints(diagnostic.ErrUnboundVar, e.S, detail, hints)
			} else {
				ch.addDiag(diagnostic.ErrUnboundVar, e.S, detail)
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
			ch.addDiag(diagnostic.ErrTypeMismatch, e.S, diagMsg("invalid integer literal: "+e.Value))
			return ch.errorPair(e.S)
		}
		return types.Con("Int"), &ir.Lit{Value: val, S: e.S}

	case *syntax.ExprStrLit:
		return types.Con("String"), &ir.Lit{Value: e.Value, S: e.S}

	case *syntax.ExprDoubleLit:
		val, err := strconv.ParseFloat(strings.ReplaceAll(e.Value, "_", ""), 64)
		if err != nil {
			ch.addDiag(diagnostic.ErrTypeMismatch, e.S, diagMsg("invalid double literal: "+e.Value))
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
		ch.addDiag(diagnostic.ErrTypeMismatch, expr.Span(), diagMsg("cannot infer type of expression"))
		return ch.errorPair(expr.Span())
	}
}

// check verifies that an expression has a given type.
func (ch *Checker) check(expr syntax.Expr, expected types.Type) ir.Core {
	ch.depth++
	defer func() { ch.depth-- }()
	if err := ch.budget.Nest(); err != nil {
		ch.addDiag(diagnostic.ErrNestingLimit, expr.Span(), diagWithErr{Context: "nesting limit", Err: err})
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

	// If the expected type is a forall, introduce TyLams and check the body
	// against the quantified type. This implements the spec rule:
	//   ⟦ e : \ a:K. T ⟧ = TyLam(a, K, ⟦e: T⟧)
	//
	// The whole peel/check/escape-check/wrap protocol is owned by
	// withPeeledForallScope so push/pop balance is lexically scoped
	// rather than tracked via a runtime counter.
	if _, ok := expected.(*types.TyForall); ok {
		return ch.withPeeledForallScope(expected, expr.Span(), func(body types.Type) ir.Core {
			return ch.check(expr, body)
		})
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
		if eq, ok := entry.(*types.EqualityEntry); ok {
			lhs := ch.unifier.Zonk(eq.Lhs)
			rhs := ch.unifier.Zonk(eq.Rhs)
			if sk, ok := lhs.(*types.TySkolem); ok {
				ch.unifier.InstallGivenEq(sk.ID, rhs)
				ch.emitGivenEq(lhs, rhs, eq.S)
				givenEqSkolems = append(givenEqSkolems, sk.ID)
			} else if sk, ok := rhs.(*types.TySkolem); ok {
				ch.unifier.InstallGivenEq(sk.ID, lhs)
				ch.emitGivenEq(lhs, rhs, eq.S)
				givenEqSkolems = append(givenEqSkolems, sk.ID)
			} else if types.ContainsSkolemOrFamily(lhs) || types.ContainsSkolemOrFamily(rhs) {
				// Type family application or skolem present — emit as given.
				// The equality is assumed to hold at the definition site;
				// it becomes a wanted at the call site (bidir_lookup.go).
				ch.emitGivenEq(lhs, rhs, eq.S)
			} else {
				// Both sides are concrete or meta — emit wanted for checking.
				ch.emitEq(lhs, rhs, eq.S, nil)
			}
			continue
		}
		di, ok := ch.constraintDictInfo(entry)
		if !ok {
			continue
		}
		dictParam := ch.freshDictName(di.className)
		dicts = append(dicts, dictInfo{param: dictParam, ty: di.dictTy})
		ch.ctx.Push(&CtxVar{Name: dictParam, Type: di.dictTy, DictClassName: di.className})
		pushed++
		ch.ctx.Push(&CtxEvidence{
			ClassName:  di.className,
			Args:       di.args,
			DictName:   dictParam,
			DictType:   di.dictTy,
			Quantified: di.quantified,
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
		bodyCore = &ir.Lam{Param: dicts[i].param, ParamType: dicts[i].ty, Body: bodyCore, Generated: ir.GenDict, S: expr.Span()}
	}
	return bodyCore
}

// constraintDictResult holds the decomposed dict info for a class constraint entry.
type constraintDictResult struct {
	dictTy     types.Type
	className  string
	args       []types.Type
	quantified *types.QuantifiedConstraint
}

// constraintDictInfo maps a non-equality ConstraintEntry to its dict type,
// class name, and arguments. Returns false for entries that don't produce dicts.
func (ch *Checker) constraintDictInfo(entry types.ConstraintEntry) (constraintDictResult, bool) {
	switch e := entry.(type) {
	case *types.QuantifiedConstraint:
		r := constraintDictResult{
			dictTy:     ch.buildQuantifiedDictType(e),
			quantified: e,
		}
		if e.Head != nil {
			r.className = e.Head.ClassName
			r.args = e.Head.Args
		}
		return r, true
	case *types.VarEntry:
		cv := ch.unifier.Zonk(e.Var)
		if cn, cArgs, ok := types.DecomposeConstraintType(cv); ok {
			return constraintDictResult{dictTy: ch.buildDictType(cn, cArgs), className: cn, args: cArgs}, true
		}
		return constraintDictResult{dictTy: cv, className: "?"}, true
	case *types.ClassEntry:
		return constraintDictResult{dictTy: ch.buildDictType(e.ClassName, e.Args), className: e.ClassName, args: e.Args}, true
	}
	return constraintDictResult{}, false
}

// subsCheck performs the subsumption check: inferred ≤ expected.
// Handles forall on the inferred side by instantiation,
// and qualified types by deferring constraints.
// Falls back to Unify when no polymorphism is involved.
//
// Precondition: expected must already be zonked by the caller.
// inferred is zonked here because infer results may contain unresolved metas.
func (ch *Checker) subsCheck(inferred, expected types.Type, expr ir.Core, s span.Span) ir.Core {
	for {
		inferred = ch.unifier.Zonk(inferred)
		// Inferred ∀a. A ≤ B  →  instantiate a, check A[a:=?m] ≤ B.
		if _, ok := inferred.(*types.TyForall); !ok {
			// Inferred { C1, C2 } => A ≤ B  →  defer all constraints, check A ≤ B
			if ev, ok := inferred.(*types.TyEvidence); ok {
				for _, entry := range ev.Constraints.ConEntries() {
					placeholder := ch.freshName(prefixDictDefer)
					ch.emitClassConstraint(placeholder, entry, s)
					expr = &ir.App{Fun: expr, Arg: &ir.Var{Name: placeholder, S: s}, S: s}
				}
				inferred = ev.Body
				continue
			}
			// CBPV adjunction coercion: a Computation value reaching a
			// Thunk-expecting position (or vice versa) is wrapped in the
			// dual IR node silently. The structural Pre/Post/Result/Grade
			// must match; if any fails the unifier is rolled back and
			// the default unify path reports the real mismatch.
			if coerced, ok := ch.tryCBPVCoercion(inferred, expected, expr, s); ok {
				return coerced
			}
			// Default: unify eagerly. subsCheck is on the critical path for type
			// information flow — metas must be solved immediately for downstream code.
			if err := ch.unifier.Unify(inferred, expected); err != nil {
				ch.addUnifyError(err, s, "type mismatch: expected "+types.Pretty(expected)+", got "+types.Pretty(inferred))
			}
			return expr
		}
		inferred = types.PeelForalls(inferred, func(f *types.TyForall) (types.Type, types.LevelExpr) {
			if isLevelKind(f.Kind) {
				return ch.freshMeta(types.SortZero), ch.unifier.FreshLevelMeta()
			}
			if isSortKind(f.Kind) {
				return ch.freshMeta(types.SortZero), nil
			}
			meta := ch.freshMeta(f.Kind)
			expr = &ir.TyApp{Expr: expr, TyArg: meta, S: s}
			return meta, nil
		})
	}
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
	lamCore := &ir.Lam{Param: bindName, ParamType: dictTy, Body: bodyCore, Generated: ir.GenDict, S: e.S}
	return &ir.App{Fun: lamCore, Arg: dictCore, S: e.S}
}

// tyLamSpec records the binder name and kind for a single peeled forall,
// in source order. withPeeledForallScope wraps the body in TyLam in
// reverse order (innermost first) when the body check completes.
type tyLamSpec struct {
	name string
	kind types.Type
}

// withPeeledForallScope owns the entire forall-peeling check protocol:
// solver scope entry, batched skolem introduction via PeelForalls,
// CtxTyVar push, body check, CtxTyVar pop, solver scope exit, escape
// check, and TyLam wrapping. The body callback runs with the peeled
// type as input and returns the body's Core. push/pop balance is
// lexically scoped — the helper guarantees no leak even if the body
// callback panics — instead of being tracked via a runtime counter
// in the caller.
//
// A chain of forall binders (e.g. \a. \b. \c. T) is peeled in one
// PeelForalls pass: visit allocates a skolem (or fresh TyVar for
// level/sort binders) per binder, and PeelForalls applies the whole
// substitution to T via a single SubstMany walk. This avoids the N
// body walks (and the corresponding heap copies) that recursive
// per-binder dispatch would otherwise perform.
//
// Side effects per visit:
//   - level-kinded: substitute f.Var → fresh TyVar in both level and
//     type positions. No CtxTyVar push (level vars are not bound in
//     the term-level context).
//   - sort-kinded: substitute f.Var → fresh TyVar in type positions.
//     No CtxTyVar push.
//   - type-kinded: substitute f.Var → fresh skolem, push a CtxTyVar
//     for the body's term-level context, and record the skolem ID
//     for the batched escape check after the body check completes.
func (ch *Checker) withPeeledForallScope(expected types.Type, sp span.Span, checkBody func(body types.Type) ir.Core) ir.Core {
	ch.enterSolverScope()
	preID := ch.freshID // belt-and-suspenders scope boundary
	trailPos := ch.unifier.TrailLen()

	var skolemIDs map[int]string
	var lamSpecs []tyLamSpec
	var pushedCtxs int

	body := types.PeelForalls(expected, func(f *types.TyForall) (types.Type, types.LevelExpr) {
		lamSpecs = append(lamSpecs, tyLamSpec{name: f.Var, kind: f.Kind})
		if isLevelKind(f.Kind) {
			freshName := fmt.Sprintf("%s$%d", f.Var, ch.fresh())
			return &types.TyVar{Name: freshName}, &types.LevelVar{Name: freshName}
		}
		if isSortKind(f.Kind) {
			freshName := fmt.Sprintf("%s$%d", f.Var, ch.fresh())
			return &types.TyVar{Name: freshName}, nil
		}
		// Type-kinded: skolemize and track for escape check.
		skolem := ch.freshSkolem(f.Var, f.Kind)
		if skolemIDs == nil {
			skolemIDs = make(map[int]string, 4)
		}
		skolemIDs[skolem.ID] = skolem.Name
		ch.ctx.Push(&CtxTyVar{Name: f.Var, Kind: f.Kind})
		pushedCtxs++
		return skolem, nil
	})

	bodyCore := checkBody(body)

	for range pushedCtxs {
		ch.ctx.Pop()
	}
	ch.exitSolverScope()

	// Belt-and-suspenders: verify no skolem from the peeled chain
	// leaked into outer solutions. Touchability (when enabled)
	// prevents this structurally; this check detects level-system
	// bugs and trial-scope commits that bypassed touchability via
	// SolverLevel = -1. The trail-incremental walk inspects only
	// soln writes that happened during the body, not the full
	// Solutions() map, and the set form shares the trail walk
	// across all skolems in the chain.
	ch.checkSkolemSetEscapeSince(skolemIDs, preID, trailPos, sp)

	// Wrap the body in TyLam in source order (outermost first).
	for i := len(lamSpecs) - 1; i >= 0; i-- {
		bodyCore = &ir.TyLam{
			TyParam: lamSpecs[i].name,
			Kind:    lamSpecs[i].kind,
			Body:    bodyCore,
			S:       sp,
		}
	}
	return bodyCore
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
