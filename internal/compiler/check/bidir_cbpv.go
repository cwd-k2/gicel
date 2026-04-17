package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/family"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// checkPure handles 'pure <expr>' in check mode.
// When expected type is Computation, delegates to the infer path (Core.Pure).
// When expected type is a non-Computation monad (e.g. Maybe via Lift),
// uses GIMonad class dispatch to resolve gipure.
func (ch *Checker) checkPure(e *syntax.ExprApp, expected types.Type) ir.Core {
	expected = ch.unifier.Zonk(expected)

	// Fast path: Computation type → existing infer + subsCheck.
	if comp, ok := expected.(*types.TyCBPV); ok && comp.Tag == types.TagComp {
		inferredTy, coreExpr := ch.infer(e)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	// Class dispatch: extract monad head, resolve GIMonad, use mkGIPure.
	// extractMonadHead guarantees expected is TyApp-spine (UnwindApp found args).
	monadHead := ch.extractMonadHead(expected)
	if monadHead != nil {
		head, _ := types.AppSpineHead(monadHead)
		valCore := ch.check(e.Arg, expected.(*types.TyApp).Arg)
		return ch.mkGIPure(head, valCore, e.S)
	}

	// Fallback: infer + subsCheck (metavar, error, etc.)
	inferredTy, coreExpr := ch.infer(e)
	return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
}

// inferPure handles the special form 'pure <expr>'.
// pure e: Computation r r a @GradeDrop, elaborated to Core.Pure.
func (ch *Checker) inferPure(e *syntax.ExprApp) (types.Type, ir.Core) {
	argTy, argCore := ch.infer(e.Arg)
	r := ch.freshMeta(types.TypeOfRows)
	grade := ch.resolveGradeDrop()
	if grade == nil {
		grade = ch.freshMeta(types.TypeOfTypes)
	}
	resultTy := ch.typeOps.Comp(r, r, argTy, grade, span.Span{})
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, e.S, "pure: %s ⇒ %s", ch.typeOps.Pretty(argTy), ch.typeOps.Pretty(resultTy))
	}
	return resultTy, &ir.Pure{Expr: argCore, S: e.S}
}

// inferBind handles the special form 'bind <comp> <cont>'.
// bind c (\x. e) : Computation r1 r3 b, elaborated to Core.Bind.
func (ch *Checker) inferBind(compExpr, contExpr syntax.Expr, s span.Span) (types.Type, ir.Core) {
	compTy, compCore := ch.infer(compExpr)
	// Auto-force: if the inferred type is a Thunk, wrap in ir.Force so
	// bind's first argument is always a Computation. Mirrors the CBPV
	// coercion in subsCheck, which does not fire here because inferBind
	// constructs its expected Computation type from fresh metas rather
	// than going through the subsCheck entry path.
	compTy, compCore = ch.autoForceIfThunk(compTy, compCore, compExpr.Span())

	g1 := ch.freshMeta(types.TypeOfTypes)
	r1 := ch.freshMeta(types.TypeOfRows)
	r2 := ch.freshMeta(types.TypeOfRows)
	a := ch.freshMeta(types.TypeOfTypes)
	if err := ch.unifier.Unify(compTy, ch.typeOps.Comp(r1, r2, a, g1, span.Span{})); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadComputation, err, compExpr.Span(), "bind: first argument must be a computation, got "+ch.typeOps.Pretty(compTy))
		return ch.errorPair(s)
	}

	g2 := ch.freshMeta(types.TypeOfTypes)
	r3 := ch.freshMeta(types.TypeOfRows)
	b := ch.freshMeta(types.TypeOfTypes)

	var bindVar string
	var bodyCore ir.Core

	if lam, ok := contExpr.(*syntax.ExprLam); ok && len(lam.Params) >= 1 {
		bindVar = ch.patternName(lam.Params[0])
		ch.ctx.Push(&CtxVar{Name: bindVar, Type: ch.unifier.Zonk(a)})
		bodyTy := ch.typeOps.Comp(ch.unifier.Zonk(r2), r3, b, g2, span.Span{})
		if len(lam.Params) == 1 {
			bodyCore = ch.check(lam.Body, bodyTy)
		} else {
			rest := &syntax.ExprLam{Params: lam.Params[1:], Body: lam.Body, S: lam.S}
			bodyCore = ch.check(rest, bodyTy)
		}
		ch.ctx.Pop()
	} else {
		bindVar = ch.freshName(prefixBind)
		contExpected := ch.typeOps.Arrow(ch.unifier.Zonk(a), ch.typeOps.Comp(ch.unifier.Zonk(r2), r3, b, g2, span.Span{}), span.Span{})
		contCore := ch.check(contExpr, contExpected)
		bodyCore = &ir.App{
			Fun: contCore,
			Arg: &ir.Var{Name: bindVar, S: s},
			S:   s,
		}
	}

	// Compose grades: g1 (comp) ∘ g2 (continuation) → result grade.
	compGrade := ch.extractCompGrade(compTy)
	bodyGrade := ch.unifier.Zonk(g2)
	grade := ch.composeGrades(compGrade, bodyGrade)
	if grade == nil {
		grade = ch.freshMeta(types.TypeOfTypes)
	}
	resultTy := ch.typeOps.Comp(ch.unifier.Zonk(r1), ch.unifier.Zonk(r3), ch.unifier.Zonk(b), grade, span.Span{})
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, s, "bind: ⇒ %s", ch.typeOps.Pretty(resultTy))
	}
	return resultTy, &ir.Bind{Comp: compCore, Var: bindVar, Body: bodyCore, Generated: ir.GenAutoBind, S: s}
}

// inferDualForm infers the CBPV dual: thunk (Comp→Thunk) or force (Thunk→Comp).
// sourceTag is the expected tag of the argument (TagComp for thunk, TagThunk for force).
func (ch *Checker) inferDualForm(
	e *syntax.ExprApp, label string,
	sourceTag types.CBPVTag,
	mkExpected func(pre, post, result, grade types.Type) types.Type,
	mkResult func(pre, post, result, grade types.Type) types.Type,
	mkCore func(argCore ir.Core) ir.Core,
) (types.Type, ir.Core) {
	argTy, argCore := ch.infer(e.Arg)
	argTy = ch.unifier.Zonk(argTy)

	// Fast path: direct extraction with tag verification.
	if t, ok := argTy.(*types.TyCBPV); ok && t.Tag == sourceTag {
		grade := t.Grade
		if grade == nil {
			grade = ch.freshMeta(types.TypeOfTypes)
		}
		resultTy := mkResult(t.Pre, t.Post, t.Result, grade)
		if ch.config.Trace != nil {
			ch.trace(TraceInfer, e.S, "%s: %s ⇒ %s", label, ch.typeOps.Pretty(argTy), ch.typeOps.Pretty(resultTy))
		}
		return resultTy, mkCore(argCore)
	}

	// Fallback: unify with a fresh quadruple.
	pre := ch.freshMeta(types.TypeOfRows)
	post := ch.freshMeta(types.TypeOfRows)
	result := ch.freshMeta(types.TypeOfTypes)
	grade := ch.freshMeta(types.TypeOfTypes)
	expected := mkExpected(pre, post, result, grade)
	if err := ch.unifier.Unify(argTy, expected); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadThunk, err, e.S,
			label+" requires a "+ch.typeOps.Pretty(expected)+" argument, got "+ch.typeOps.Pretty(argTy))
		return &types.TyError{S: e.S}, mkCore(argCore)
	}
	resultTy := mkResult(ch.unifier.Zonk(pre), ch.unifier.Zonk(post), ch.unifier.Zonk(result), ch.unifier.Zonk(grade))
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, e.S, "%s: %s ⇒ %s", label, ch.typeOps.Pretty(argTy), ch.typeOps.Pretty(resultTy))
	}
	return resultTy, mkCore(argCore)
}

// isMergeOp reports whether name is one of the merge/*** special form identifiers.
func isMergeOp(name string) bool {
	return name == "merge" || name == "(***)" || name == "***"
}

// inferMerge handles merge left right → ir.Merge with CapEnv label extraction.
// merge :: Computation pre1 post1 a -> Computation pre2 post2 b
//
//	-> Computation (Merge pre1 pre2) (Merge post1 post2) (a, b)
func (ch *Checker) inferMerge(leftExpr, rightExpr syntax.Expr, s span.Span) (types.Type, ir.Core) {
	g1 := ch.freshMeta(types.TypeOfTypes)
	pre1 := ch.freshMeta(types.TypeOfRows)
	post1 := ch.freshMeta(types.TypeOfRows)
	a := ch.freshMeta(types.TypeOfTypes)
	g2 := ch.freshMeta(types.TypeOfTypes)
	pre2 := ch.freshMeta(types.TypeOfRows)
	post2 := ch.freshMeta(types.TypeOfRows)
	b := ch.freshMeta(types.TypeOfTypes)

	leftCore := ch.check(leftExpr, ch.typeOps.Comp(pre1, post1, a, g1, span.Span{}))
	rightCore := ch.check(rightExpr, ch.typeOps.Comp(pre2, post2, b, g2, span.Span{}))

	// Extract row labels from the resolved pre-states.
	leftLabels := ch.extractRowLabels(ch.unifier.Zonk(pre1))
	rightLabels := ch.extractRowLabels(ch.unifier.Zonk(pre2))

	// Result type: Computation (Merge pre1 pre2) (Merge post1 post2) (a, b)
	// Tuple = Record { _1: a, _2: b }, matching the runtime OpMerge output.
	mergedPre := ch.applyMergeFamily(pre1, pre2)
	mergedPost := ch.applyMergeFamily(post1, post2)
	result := ch.typeOps.App(
		ch.typeOps.Con(types.TyConRecord, span.Span{}),
		types.ClosedRow(
			types.RowField{Label: types.TupleLabel(1), Type: a},
			types.RowField{Label: types.TupleLabel(2), Type: b},
		),
		span.Span{},
	)
	mergedGrade := ch.composeGrades(g1, g2)
	if mergedGrade == nil {
		mergedGrade = ch.freshMeta(types.TypeOfTypes)
	}
	resultTy := ch.typeOps.Comp(mergedPre, mergedPost, result, mergedGrade, span.Span{})

	mergeNode := &ir.Merge{
		Left:        leftCore,
		Right:       rightCore,
		LeftLabels:  leftLabels,
		RightLabels: rightLabels,
		S:           s,
	}
	// Stash the pre-state row types in the checker-local side table so
	// refineMergeLabels can re-extract labels after constraint resolution.
	if ch.pipeState != nil {
		if ch.pipeState.pendingMergeLabels == nil {
			ch.pipeState.pendingMergeLabels = make(map[*ir.Merge]pendingMergePre)
		}
		ch.pipeState.pendingMergeLabels[mergeNode] = pendingMergePre{Pre1: pre1, Pre2: pre2}
	}
	return resultTy, mergeNode
}

// extractRowLabels returns sorted label names from a resolved capability row.
func (ch *Checker) extractRowLabels(ty types.Type) []string {
	ty = ch.unifier.Zonk(ty)
	row, ok := ty.(*types.TyEvidenceRow)
	if !ok {
		return nil
	}
	fields := row.CapFields()
	labels := make([]string, len(fields))
	for i, f := range fields {
		labels[i] = f.Label
	}
	return labels
}

// applyMergeFamily applies the Merge type family to two row types.
func (ch *Checker) applyMergeFamily(r1, r2 types.Type) types.Type {
	r1 = ch.unifier.Zonk(r1)
	r2 = ch.unifier.Zonk(r2)
	// Merge type family is applied via TyApp(TyApp(TyCon("Merge"), r1), r2).
	merged := ch.typeOps.App(ch.typeOps.App(ch.typeOps.Con(family.RowFamilyMerge, span.Span{}), r1, span.Span{}), r2, span.Span{})
	return ch.reduceFamilyInType(merged)
}

func (ch *Checker) inferThunk(e *syntax.ExprApp) (types.Type, ir.Core) {
	return ch.inferDualForm(e, "thunk",
		types.TagComp, // thunk expects a Computation argument
		func(p, q, r, g types.Type) types.Type { return ch.typeOps.Comp(p, q, r, g, span.Span{}) },
		func(p, q, r, _ types.Type) types.Type { return ch.typeOps.Thunk(p, q, r, span.Span{}) },
		func(c ir.Core) ir.Core { return &ir.Thunk{Comp: c, S: e.S} },
	)
}

func (ch *Checker) inferForce(e *syntax.ExprApp) (types.Type, ir.Core) {
	return ch.inferDualForm(e, "force",
		types.TagThunk, // force expects a Thunk argument
		func(p, q, r, _ types.Type) types.Type { return ch.typeOps.Thunk(p, q, r, span.Span{}) },
		func(p, q, r, g types.Type) types.Type { return ch.typeOps.Comp(p, q, r, g, span.Span{}) },
		func(c ir.Core) ir.Core { return &ir.Force{Expr: c, S: e.S} },
	)
}

// autoForceIfThunk applies the Thunk → Computation direction of the
// CBPV coercion: if inferred is a Thunk, wrap core in ir.Force and
// return the corresponding Computation view of the structural parts.
// Otherwise returns the inputs unchanged. Used by inferBind (and other
// entry points that construct a Computation expectation from fresh
// metas) to align with the subsCheck-based coercion.
func (ch *Checker) autoForceIfThunk(inferred types.Type, core ir.Core, s span.Span) (types.Type, ir.Core) {
	zonked := ch.unifier.Zonk(inferred)
	thunk, ok := zonked.(*types.TyCBPV)
	if !ok || thunk.Tag != types.TagThunk {
		return inferred, core
	}
	asComp := ch.typeOps.Comp(thunk.Pre, thunk.Post, thunk.Result, thunk.Grade, span.Span{})
	return asComp, &ir.Force{Expr: core, S: s}
}

// autoThunkComputation applies the Computation → Thunk direction of
// the CBPV coercion, descending through forall / evidence layers to
// reach the underlying TyCBPV. At non-entry top-level bindings a bare
// Computation cannot be run directly (only the entry point is driven
// by the runtime), so the only semantically meaningful form is a
// stored Thunk — the checker inserts the wrap silently when no
// annotation pins the binding as Computation. Dual of
// autoForceIfThunk, used by decl.go's binding elaboration.
//
// For polymorphic bindings (type ∀a. Computation r r a), the inferred
// core already carries matching ir.TyLam layers after generalization;
// the helper descends under each pair in lock-step and wraps the
// innermost computation node in ir.Thunk. For bindings with evidence
// (type C => Computation r r a), the core has a generated dict lambda
// that we walk through analogously.
func (ch *Checker) autoThunkComputation(inferred types.Type, core ir.Core, s span.Span) (types.Type, ir.Core) {
	return thunkIfBareComputation(ch.typeOps, ch.unifier.Zonk(inferred), core, s)
}

func thunkIfBareComputation(ops *types.TypeOps, ty types.Type, core ir.Core, s span.Span) (types.Type, ir.Core) {
	switch t := ty.(type) {
	case *types.TyForall:
		if tl, ok := core.(*ir.TyLam); ok {
			innerTy, innerCore := thunkIfBareComputation(ops, t.Body, tl.Body, s)
			newTy := ops.Forall(t.Var, t.Kind, innerTy, t.S)
			newCore := &ir.TyLam{TyParam: tl.TyParam, Kind: tl.Kind, Body: innerCore, S: tl.S}
			return newTy, newCore
		}
	case *types.TyEvidence:
		if lam, ok := core.(*ir.Lam); ok && lam.Generated.IsGenerated() {
			innerTy, innerCore := thunkIfBareComputation(ops, t.Body, lam.Body, s)
			newTy := ops.EvidenceWrap(t.Constraints, innerTy, t.S)
			newCore := &ir.Lam{
				Param: lam.Param, ParamType: lam.ParamType,
				Body: innerCore, Generated: lam.Generated, S: lam.S,
			}
			return newTy, newCore
		}
	case *types.TyCBPV:
		if t.Tag == types.TagComp {
			asThunk := ops.ThunkGraded(t.Pre, t.Post, t.Result, t.Grade, t.S)
			return asThunk, &ir.Thunk{Comp: core, S: s}
		}
	}
	return ty, core
}

// tryCBPVCoercion attempts an implicit Computation ↔ Thunk coercion when
// the inferred and expected types are both CBPV but carry opposite tags.
// This witnesses the CBPV adjunction (thunk ⊣ force) at the type level:
// whenever a Computation value reaches a Thunk-expecting position (or
// vice versa), the checker silently inserts the matching ir.Thunk or
// ir.Force node, leaving the rest of the CBPV structure (Pre, Post,
// Result, Grade) unified exactly as a direct match would require.
//
// Returns (coercedExpr, true) when a coercion was applied, or
// (nil, false) when no coercion is warranted — in which case the caller
// falls through to the normal unification path so the existing type
// error surfaces unchanged.
//
// The structural unification is attempted eagerly; if any part fails,
// the unifier state is rolled back via saveState/restoreState so the
// caller's subsequent unification attempt sees a clean slate.
func (ch *Checker) tryCBPVCoercion(inferred, expected types.Type, expr ir.Core, s span.Span) (ir.Core, bool) {
	iCBPV, ok1 := inferred.(*types.TyCBPV)
	if !ok1 {
		return nil, false
	}
	expected = ch.unifier.Zonk(expected)
	eCBPV, ok2 := expected.(*types.TyCBPV)
	if !ok2 {
		return nil, false
	}
	pairs, ok := types.CBPVAdjunctionParts(iCBPV, eCBPV)
	if !ok {
		return nil, false
	}

	// Save state: if any structural unification fails we must roll back
	// so the caller's normal unify path reports the real mismatch on
	// untouched types.
	saved := ch.saveState()

	for _, p := range pairs {
		if err := ch.unifier.Unify(p[0], p[1]); err != nil {
			ch.restoreState(saved)
			return nil, false
		}
	}

	switch {
	case iCBPV.Tag == types.TagComp && eCBPV.Tag == types.TagThunk:
		return &ir.Thunk{Comp: expr, S: s}, true
	case iCBPV.Tag == types.TagThunk && eCBPV.Tag == types.TagComp:
		return &ir.Force{Expr: expr, S: s}, true
	}
	// Unreachable: tags differ (asserted above) and only two tags exist.
	ch.restoreState(saved)
	return nil, false
}
