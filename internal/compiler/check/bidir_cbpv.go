package check

import (
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
	resultTy := types.MkCompGraded(r, r, argTy, grade)
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, e.S, "pure: %s ⇒ %s", types.Pretty(argTy), types.Pretty(resultTy))
	}
	return resultTy, &ir.Pure{Expr: argCore, S: e.S}
}

// inferBind handles the special form 'bind <comp> <cont>'.
// bind c (\x. e) : Computation r1 r3 b, elaborated to Core.Bind.
func (ch *Checker) inferBind(compExpr, contExpr syntax.Expr, s span.Span) (types.Type, ir.Core) {
	compTy, compCore := ch.infer(compExpr)

	g1 := ch.freshMeta(types.TypeOfTypes)
	r1 := ch.freshMeta(types.TypeOfRows)
	r2 := ch.freshMeta(types.TypeOfRows)
	a := ch.freshMeta(types.TypeOfTypes)
	if err := ch.unifier.Unify(compTy, types.MkCompGraded(r1, r2, a, g1)); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadComputation, err, compExpr.Span(), "bind: first argument must be a computation, got "+types.Pretty(compTy))
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
		bodyTy := types.MkCompGraded(ch.unifier.Zonk(r2), r3, b, g2)
		if len(lam.Params) == 1 {
			bodyCore = ch.check(lam.Body, bodyTy)
		} else {
			rest := &syntax.ExprLam{Params: lam.Params[1:], Body: lam.Body, S: lam.S}
			bodyCore = ch.check(rest, bodyTy)
		}
		ch.ctx.Pop()
	} else {
		bindVar = ch.freshName(prefixBind)
		contExpected := types.MkArrow(ch.unifier.Zonk(a), types.MkCompGraded(ch.unifier.Zonk(r2), r3, b, g2))
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
	resultTy := types.MkCompGraded(ch.unifier.Zonk(r1), ch.unifier.Zonk(r3), ch.unifier.Zonk(b), grade)
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, s, "bind: ⇒ %s", types.Pretty(resultTy))
	}
	return resultTy, &ir.Bind{Comp: compCore, Var: bindVar, Body: bodyCore, Generated: true, S: s}
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
			ch.trace(TraceInfer, e.S, "%s: %s ⇒ %s", label, types.Pretty(argTy), types.Pretty(resultTy))
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
			label+" requires a "+types.Pretty(expected)+" argument, got "+types.Pretty(argTy))
		return &types.TyError{S: e.S}, mkCore(argCore)
	}
	resultTy := mkResult(ch.unifier.Zonk(pre), ch.unifier.Zonk(post), ch.unifier.Zonk(result), ch.unifier.Zonk(grade))
	if ch.config.Trace != nil {
		ch.trace(TraceInfer, e.S, "%s: %s ⇒ %s", label, types.Pretty(argTy), types.Pretty(resultTy))
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

	leftCore := ch.check(leftExpr, types.MkCompGraded(pre1, post1, a, g1))
	rightCore := ch.check(rightExpr, types.MkCompGraded(pre2, post2, b, g2))

	// Extract row labels from the resolved pre-states.
	leftLabels := ch.extractRowLabels(ch.unifier.Zonk(pre1))
	rightLabels := ch.extractRowLabels(ch.unifier.Zonk(pre2))

	// Result type: Computation (Merge pre1 pre2) (Merge post1 post2) (a, b)
	// Tuple = Record { _1: a, _2: b }, matching the runtime OpMerge output.
	mergedPre := ch.applyMergeFamily(pre1, pre2)
	mergedPost := ch.applyMergeFamily(post1, post2)
	result := &types.TyApp{
		Fun: types.Con(types.TyConRecord),
		Arg: types.ClosedRow(
			types.RowField{Label: ir.TupleLabel(1), Type: a},
			types.RowField{Label: ir.TupleLabel(2), Type: b},
		),
	}
	mergedGrade := ch.composeGrades(g1, g2)
	if mergedGrade == nil {
		mergedGrade = ch.freshMeta(types.TypeOfTypes)
	}
	resultTy := types.MkCompGraded(mergedPre, mergedPost, result, mergedGrade)

	return resultTy, &ir.Merge{
		Left:        leftCore,
		Right:       rightCore,
		LeftLabels:  leftLabels,
		RightLabels: rightLabels,
		PreLeft:     pre1,
		PreRight:    pre2,
		S:           s,
	}
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
	merged := &types.TyApp{Fun: &types.TyApp{Fun: types.Con("Merge"), Arg: r1}, Arg: r2}
	return ch.reduceFamilyInType(merged)
}

func (ch *Checker) inferThunk(e *syntax.ExprApp) (types.Type, ir.Core) {
	return ch.inferDualForm(e, "thunk",
		types.TagComp, // thunk expects a Computation argument
		func(p, q, r, g types.Type) types.Type { return types.MkCompGraded(p, q, r, g) },
		func(p, q, r, _ types.Type) types.Type { return types.MkThunk(p, q, r) },
		func(c ir.Core) ir.Core { return &ir.Thunk{Comp: c, S: e.S} },
	)
}

func (ch *Checker) inferForce(e *syntax.ExprApp) (types.Type, ir.Core) {
	return ch.inferDualForm(e, "force",
		types.TagThunk, // force expects a Thunk argument
		func(p, q, r, _ types.Type) types.Type { return types.MkThunk(p, q, r) },
		func(p, q, r, g types.Type) types.Type { return types.MkCompGraded(p, q, r, g) },
		func(c ir.Core) ir.Core { return &ir.Force{Expr: c, S: e.S} },
	)
}
