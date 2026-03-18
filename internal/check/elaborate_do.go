package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax"
	"github.com/cwd-k2/gicel/internal/types"
)

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

// elaborateStmtsChecked elaborates a do block against a known Computation type.
// This threads the pre/post state through the bind chain, unlike elaborateStmts
// which infers fresh metas for pre/post.
func (ch *Checker) elaborateStmtsChecked(stmts []syntax.Stmt, comp *types.TyCBPV, s span.Span) core.Core {
	if len(stmts) == 1 {
		switch st := stmts[0].(type) {
		case *syntax.StmtExpr:
			return ch.check(st.Expr, comp)
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
		// x <- c; rest
		// c: Computation pre mid a — infer, but pre must match comp.Pre
		compTy, compCore := ch.infer(st.Comp)
		compTy = ch.unifier.Zonk(compTy)
		if inferredComp, ok := compTy.(*types.TyCBPV); ok {
			// Record step for multiplicity analysis.
			ch.multSteps = append(ch.multSteps, multStep{pre: inferredComp.Pre, post: inferredComp.Post, s: st.S})
			// Unify inferred pre with expected pre.
			if err := ch.unifier.Unify(inferredComp.Pre, comp.Pre); err != nil {
				ch.addUnifyError(err, st.S, fmt.Sprintf(
					"do bind: pre-state mismatch: expected %s, got %s",
					types.Pretty(comp.Pre), types.Pretty(inferredComp.Pre)))
			}
			resultTy := inferredComp.Result
			ch.ctx.Push(&CtxVar{Name: st.Var, Type: resultTy})
			// Rest: Computation mid post result — mid from inferred post, post/result from expected.
			restComp := &types.TyCBPV{Tag: types.TagComp, Pre: inferredComp.Post, Post: comp.Post, Result: comp.Result, S: comp.S}
			restCore := ch.elaborateStmtsChecked(stmts[1:], restComp, s)
			ch.ctx.Pop()
			return &core.Bind{Comp: compCore, Var: st.Var, Body: restCore, S: st.S}
		}
		// Fallback: infer didn't give TyCBPV, extract result and continue.
		resultTy := ch.extractCompResult(compTy, st.S)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: resultTy})
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		ch.ctx.Pop()
		// Best-effort: infer didn't produce TyCBPV, so pre/post threading
		// is unavailable. Unifying the inferred rest type with the expected
		// computation type is advisory — failure means the do block types
		// are already inconsistent and errors will surface elsewhere.
		_ = ch.unifier.Unify(restTy, comp)
		return &core.Bind{Comp: compCore, Var: st.Var, Body: restCore, S: st.S}

	case *syntax.StmtPureBind:
		// x := e; rest
		bindTy, bindCore := ch.infer(st.Expr)
		ch.ctx.Push(&CtxVar{Name: st.Var, Type: bindTy})
		restCore := ch.elaborateStmtsChecked(stmts[1:], comp, s)
		ch.ctx.Pop()
		return &core.App{
			Fun: &core.Lam{Param: st.Var, Body: restCore, S: st.S},
			Arg: bindCore,
			S:   st.S,
		}

	case *syntax.StmtExpr:
		// c; rest
		compTy, compCore := ch.infer(st.Expr)
		compTy = ch.unifier.Zonk(compTy)
		if inferredComp, ok := compTy.(*types.TyCBPV); ok {
			// Record step for multiplicity analysis.
			ch.multSteps = append(ch.multSteps, multStep{pre: inferredComp.Pre, post: inferredComp.Post, s: st.S})
			if err := ch.unifier.Unify(inferredComp.Pre, comp.Pre); err != nil {
				ch.addUnifyError(err, st.S, fmt.Sprintf(
					"do statement: pre-state mismatch: expected %s, got %s",
					types.Pretty(comp.Pre), types.Pretty(inferredComp.Pre)))
			}
			restComp := &types.TyCBPV{Tag: types.TagComp, Pre: inferredComp.Post, Post: comp.Post, Result: comp.Result, S: comp.S}
			restCore := ch.elaborateStmtsChecked(stmts[1:], restComp, s)
			return &core.Bind{Comp: compCore, Var: "_", Body: restCore, S: st.S}
		}
		restTy, restCore := ch.elaborateStmts(stmts[1:], s)
		// Best-effort: infer didn't produce TyCBPV, so pre/post threading
		// is unavailable. Unifying the inferred rest type with the expected
		// computation type is advisory — failure means the do block types
		// are already inconsistent and errors will surface elsewhere.
		_ = ch.unifier.Unify(restTy, comp)
		return &core.Bind{Comp: compCore, Var: "_", Body: restCore, S: st.S}
	}

	ch.addCodedError(errs.ErrBadComputation, s, "unexpected statement in do block")
	return &core.Var{Name: "<error>", S: s}
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

	// Fast path: Computation types use Core.Bind with expected pre/post threading.
	if comp, ok := expected.(*types.TyCBPV); ok && comp.Tag == types.TagComp {
		saved := ch.multSteps
		ch.multSteps = nil
		result := ch.elaborateStmtsChecked(e.Stmts, comp, e.S)
		ch.checkMultiplicity(comp, e.S)
		ch.multSteps = saved
		return result
	}
	switch expected.(type) {
	case *types.TyMeta, *types.TyError:
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
		// x <- comp; rest  →  ixbind comp (\x. rest)
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
		// comp; rest  →  ixbind comp (\_. rest)
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
		// x := e; rest  →  (\x. rest) e
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

// IxMonad method indices (offset from the start of the methods block, after supers).
const (
	ixMethodPure = 0 // ixpure
	ixMethodBind = 1 // ixbind
)

// extractIxMethod resolves the IxMonad (Lift monadHead) dictionary and
// extracts the method at the given index via pattern matching.
func (ch *Checker) extractIxMethod(monadHead types.Type, methodIdx int, s span.Span) core.Core {
	classInfo := ch.classes["IxMonad"]
	if classInfo == nil {
		ch.errors.Add(&errs.Error{Code: errs.ErrNoInstance, Span: s, Message: "IxMonad class not available (missing Prelude?)"})
		return &core.Var{Name: "<error>", S: s}
	}
	liftedMonad := &types.TyApp{Fun: &types.TyCon{Name: "Lift"}, Arg: monadHead}
	dict := ch.resolveInstance("IxMonad", []types.Type{liftedMonad}, s)
	fieldIdx := len(classInfo.Supers) + methodIdx
	return ch.extractDictField(classInfo, dict, fieldIdx, "ixm", s)
}

// mkIxPure generates Core for monadic pure using the IxMonad dictionary.
func (ch *Checker) mkIxPure(monadHead types.Type, val core.Core, s span.Span) core.Core {
	selector := ch.extractIxMethod(monadHead, ixMethodPure, s)
	return &core.App{Fun: selector, Arg: val, S: s}
}

// mkIxBind generates Core for a monadic bind using the IxMonad dictionary.
func (ch *Checker) mkIxBind(monadHead types.Type, comp core.Core, varName string, body core.Core, s span.Span) core.Core {
	selector := ch.extractIxMethod(monadHead, ixMethodBind, s)
	return &core.App{
		Fun: &core.App{Fun: selector, Arg: comp, S: s},
		Arg: &core.Lam{Param: varName, Body: body, S: s},
		S:   s,
	}
}

// --- Multiplicity enforcement ---

// multStep records a single do-chain step's inferred pre/post for multiplicity analysis.
type multStep struct {
	pre  types.Type
	post types.Type
	s    span.Span
}

// checkMultiplicity verifies multiplicity constraints on @Mult-annotated labels.
// For each label annotated with @Linear or @Affine, counts the number of
// same-type preservation events (steps where the label appears in both pre
// and post with structurally equal types). Type-changing preservations
// (protocol state transitions) and consumption events do not count.
func (ch *Checker) checkMultiplicity(comp *types.TyCBPV, s span.Span) {
	steps := ch.multSteps
	if len(steps) == 0 {
		return
	}

	// Collect all @Mult-annotated labels from step pre/post states
	// and the overall computation's pre-state.
	mults := make(map[string]types.Type) // label → zonked multiplicity
	for _, step := range steps {
		collectMultLabels(ch, step.pre, mults)
		collectMultLabels(ch, step.post, mults)
	}
	collectMultLabels(ch, comp.Pre, mults)

	if len(mults) == 0 {
		return
	}

	// For each @Mult label, count same-type preservations.
	for label, mult := range mults {
		limit := multLimit(mult)
		if limit < 0 {
			continue
		}

		count := 0
		for _, step := range steps {
			preTy := capFieldType(ch, step.pre, label)
			postTy := capFieldType(ch, step.post, label)
			if preTy != nil && postTy != nil && types.Equal(preTy, postTy) {
				count++
			}
		}

		if count > limit {
			ch.addCodedError(errs.ErrMultiplicity, s,
				fmt.Sprintf("@%s capability %q accessed %d times (maximum %d)",
					types.Pretty(mult), label, count, limit))
		}
	}
}

// collectMultLabels extracts @Mult-annotated labels from a zonked capability row.
func collectMultLabels(ch *Checker, ty types.Type, out map[string]types.Type) {
	ty = ch.unifier.Zonk(ty)
	ev, ok := ty.(*types.TyEvidenceRow)
	if !ok {
		return
	}
	cap, ok := ev.Entries.(*types.CapabilityEntries)
	if !ok {
		return
	}
	for _, f := range cap.Fields {
		if f.Mult != nil {
			if _, exists := out[f.Label]; !exists {
				out[f.Label] = ch.unifier.Zonk(f.Mult)
			}
		}
	}
}

// capFieldType returns the zonked type for a label in a capability row, or nil.
func capFieldType(ch *Checker, ty types.Type, label string) types.Type {
	ty = ch.unifier.Zonk(ty)
	ev, ok := ty.(*types.TyEvidenceRow)
	if !ok {
		return nil
	}
	cap, ok := ev.Entries.(*types.CapabilityEntries)
	if !ok {
		return nil
	}
	for _, f := range cap.Fields {
		if f.Label == label {
			return ch.unifier.Zonk(f.Type)
		}
	}
	return nil
}

// multLimit returns the maximum allowed same-type preservations for a multiplicity.
// Returns -1 for unrestricted (no limit).
func multLimit(mult types.Type) int {
	if con, ok := mult.(*types.TyCon); ok {
		switch con.Name {
		case "Linear":
			return 1
		case "Affine":
			return 1
		}
	}
	return -1
}

func (ch *Checker) extractCompResult(ty types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	if comp, ok := ty.(*types.TyCBPV); ok {
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
