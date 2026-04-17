package check

import (
	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// checkDo type-checks a do block against an expected type.
// Uses direct Core.Bind for Computation types (fast path) and
// class dispatch via GIMonad for other indexed monad instances.
func (ch *Checker) checkDo(e *syntax.ExprDo, expected types.Type) ir.Core {
	if len(e.Stmts) == 0 {
		ch.addDiag(diagnostic.ErrEmptyDo, e.S, diagMsg("empty do block"))
		return &ir.Error{S: e.S}
	}

	expected = ch.unifier.Zonk(expected)

	// Fast path: Computation types use Core.Bind with expected pre/post threading.
	if comp, ok := expected.(*types.TyCBPV); ok && comp.Tag == types.TagComp {
		d := &doChecked{ch: ch, comp: comp}
		_, result := doElaborate(ch, d, e.Stmts, e.S)
		ch.checkGradeBoundary(comp, e.S)
		return result
	}
	switch expected.(type) {
	case *types.TyMeta, *types.TyError:
		d := &doInfer{ch: ch}
		inferredTy, coreExpr := doElaborate(ch, d, e.Stmts, e.S)
		return ch.subsCheck(inferredTy, expected, coreExpr, e.S)
	}

	// Class dispatch: extract monad head and elaborate with GIMonad dictionary.
	monadHead := ch.extractMonadHead(expected)
	if monadHead != nil {
		if _, _, ok := ch.resolveGIMonadDict(monadHead, e.S); ok {
			head, _ := types.AppSpineHead(monadHead)
			d := &doGraded{ch: ch, monadHead: head, expected: expected}
			_, result := doElaborate(ch, d, e.Stmts, e.S)
			return result
		}
		// Monad fallback: desugar to mbind/mpure for Type→Type monads.
		// GIMonad handles graded indexed monads; Monad handles plain monads
		// (Maybe, List, Cont, etc.) that lack row/grade parameters.
		if ch.hasMonadInstance(monadHead, e.S) {
			desugared := ch.desugarDoToMonad(e.Stmts, e.S)
			return ch.check(desugared, expected)
		}
		ch.addDiag(diagnostic.ErrNoInstance, e.S,
			diagFmt{Format: "do notation for %s requires a GIMonad or Monad instance", Args: []any{ch.typeOps.Pretty(expected)}})
		return &ir.Error{S: e.S}
	}

	// Fallback: try Computation inference.
	d := &doInfer{ch: ch}
	inferredTy, coreExpr := doElaborate(ch, d, e.Stmts, e.S)
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
			result = ch.typeOps.App(result, a)
		}
		return result
	}
	return nil
}

// extractMonadResult extracts the result type from a monadic type given the monad head.
// e.g. Maybe Int with head Maybe → Int
func (ch *Checker) extractMonadResult(ty types.Type, monadHead types.Type, s span.Span) types.Type {
	ty = ch.unifier.Zonk(ty)
	if app, ok := ty.(*types.TyApp); ok {
		return app.Arg
	}
	// Generate fresh meta as fallback.
	result := ch.freshMeta(types.TypeOfTypes)
	headApp := ch.typeOps.App(monadHead, result)
	ch.emitEq(ty, headApp, s, solve.WithLazyContext(diagnostic.ErrBadComputation, func() string {
		return "expected " + ch.typeOps.Pretty(monadHead) + " type, got " + ch.typeOps.Pretty(ty)
	}))
	return result
}

// extractPureArg checks if an expression is `pure val` or `gipure val` and returns val.
func extractPureArg(expr syntax.Expr) syntax.Expr {
	app, ok := expr.(*syntax.ExprApp)
	if !ok {
		return nil
	}
	v, ok := app.Fun.(*syntax.ExprVar)
	if !ok {
		return nil
	}
	if v.Name == "pure" || v.Name == "gipure" {
		return app.Arg
	}
	return nil
}

// --- GIMonad (graded indexed monad) dispatch ---

// GIMonad method indices (offset from the start of the methods block, after supers).
// GIMonad has 1 super (GradeAlgebra g).
const (
	giMethodPure = 0 // gipure
	giMethodBind = 1 // gibind
)

// resolveGIMonadDict resolves a GIMonad dictionary for monadHead,
// trying direct resolution first, then Lift-wrapped fallback for Type→Type monads.
// Returns (dict, classInfo, true) on success, (nil, nil, false) on failure.
func (ch *Checker) resolveGIMonadDict(monadHead types.Type, s span.Span) (ir.Core, *ClassInfo, bool) {
	classInfo, ok := ch.reg.LookupClass("GIMonad")
	if !ok {
		return nil, nil, false
	}
	head, _ := types.AppSpineHead(monadHead)
	// Try direct GIMonad instance.
	gradeMeta := ch.freshMeta(types.TypeOfTypes)
	if dict, ok := ch.tryResolveInstance("GIMonad", []types.Type{gradeMeta, head}, s); ok {
		return dict, classInfo, true
	}
	// Lift fallback: GIMonad g (Lift head).
	// Safe for Type→Type monads; do-block elaboration for Lift-wrapped types
	// produces correct results because these monads have no row parameters.
	liftedHead := ch.typeOps.App(ch.typeOps.Con("Lift"), head)
	gradeMeta2 := ch.freshMeta(types.TypeOfTypes)
	if dict, ok := ch.tryResolveInstance("GIMonad", []types.Type{gradeMeta2, liftedHead}, s); ok {
		return dict, classInfo, true
	}
	return nil, nil, false
}

// extractGIMethod resolves a GIMonad dictionary for monadHead and extracts
// the method at methodIdx.
func (ch *Checker) extractGIMethod(monadHead types.Type, methodIdx int, s span.Span) ir.Core {
	dict, classInfo, ok := ch.resolveGIMonadDict(monadHead, s)
	if !ok {
		ch.addDiag(diagnostic.ErrNoInstance, s, diagWithType{Context: "no GIMonad instance for ", Type: monadHead})
		return &ir.Error{S: s}
	}
	fieldIdx := len(classInfo.Supers) + methodIdx
	return ch.solver.ExtractDictField(classInfo, dict, fieldIdx, "gim", s)
}

// mkGIPure generates Core for graded monadic pure using the GIMonad dictionary.
func (ch *Checker) mkGIPure(monadHead types.Type, val ir.Core, s span.Span) ir.Core {
	selector := ch.extractGIMethod(monadHead, giMethodPure, s)
	return &ir.App{Fun: selector, Arg: val, S: s}
}

// mkGIBind generates Core for a graded monadic bind using the GIMonad dictionary.
func (ch *Checker) mkGIBind(monadHead types.Type, comp ir.Core, varName string, body ir.Core, s span.Span) ir.Core {
	selector := ch.extractGIMethod(monadHead, giMethodBind, s)
	return &ir.App{
		Fun: &ir.App{Fun: selector, Arg: comp, S: s},
		Arg: &ir.Lam{Param: varName, Body: body, S: s},
		S:   s,
	}
}

// --- Monad (Type→Type) fallback ---

// hasMonadInstance checks whether the Monad class has a resolvable instance
// for the given monadHead, without emitting errors.
func (ch *Checker) hasMonadInstance(monadHead types.Type, s span.Span) bool {
	if _, ok := ch.reg.LookupClass("Monad"); !ok {
		return false
	}
	_, ok := ch.tryResolveInstance("Monad", []types.Type{monadHead}, s)
	return ok
}

// rewritePureToMpure rewrites `pure expr` to `mpure expr` at the AST level.
// The `pure` builtin is linked to GIMonad/Computation; for Monad dispatch
// we need `mpure` instead. Non-pure expressions are returned as-is.
func rewritePureToMpure(expr syntax.Expr) syntax.Expr {
	if app, ok := expr.(*syntax.ExprApp); ok {
		if v, ok := app.Fun.(*syntax.ExprVar); ok {
			if v.Name == "pure" || v.Name == "gipure" {
				return &syntax.ExprApp{
					Fun: &syntax.ExprVar{Name: "mpure", S: v.S},
					Arg: app.Arg,
					S:   app.S,
				}
			}
		}
	}
	return expr
}

// desugarDoToMonad desugars a do-block into explicit mbind/mpure calls.
// do { x <- a; b }  →  mbind a (\x. b)
// do { a; b }        →  mbind a (\_ . b)
// do { a }           →  a
func (ch *Checker) desugarDoToMonad(stmts []syntax.Stmt, s span.Span) syntax.Expr {
	if len(stmts) == 0 {
		return &syntax.ExprError{S: s}
	}
	if len(stmts) == 1 {
		switch st := stmts[0].(type) {
		case *syntax.StmtExpr:
			return rewritePureToMpure(st.Expr)
		case *syntax.StmtBind:
			return st.Comp
		case *syntax.StmtPureBind:
			return st.Expr
		}
		return &syntax.ExprError{S: s}
	}
	rest := ch.desugarDoToMonad(stmts[1:], s)
	switch st := stmts[0].(type) {
	case *syntax.StmtBind:
		var lamBody syntax.Expr
		var lamParam syntax.Pattern
		if name, ok := syntax.PatVarName(st.Pat); ok {
			lamParam = &syntax.PatVar{Name: name, S: st.S}
			lamBody = rest
		} else {
			freshName := ch.freshName("$p")
			lamParam = &syntax.PatVar{Name: freshName, S: st.S}
			lamBody = &syntax.ExprCase{
				Scrutinee: &syntax.ExprVar{Name: freshName, S: st.S},
				Alts:      []syntax.Alt{{Pattern: st.Pat, Body: rest, S: st.S}},
				S:         st.S,
			}
		}
		return &syntax.ExprApp{
			Fun: &syntax.ExprApp{
				Fun: &syntax.ExprVar{Name: "mbind", S: st.S},
				Arg: rewritePureToMpure(st.Comp),
				S:   st.S,
			},
			Arg: &syntax.ExprLam{
				Params: []syntax.Pattern{lamParam},
				Body:   lamBody,
				S:      st.S,
			},
			S: st.S,
		}
	case *syntax.StmtExpr:
		return &syntax.ExprApp{
			Fun: &syntax.ExprApp{
				Fun: &syntax.ExprVar{Name: "mbind", S: st.S},
				Arg: rewritePureToMpure(st.Expr),
				S:   st.S,
			},
			Arg: &syntax.ExprLam{
				Params: []syntax.Pattern{&syntax.PatWild{S: st.S}},
				Body:   rest,
				S:      st.S,
			},
			S: st.S,
		}
	case *syntax.StmtPureBind:
		var lamBody syntax.Expr
		var lamParam syntax.Pattern
		if name, ok := syntax.PatVarName(st.Pat); ok {
			lamParam = &syntax.PatVar{Name: name, S: st.S}
			lamBody = rest
		} else {
			freshName := ch.freshName("$p")
			lamParam = &syntax.PatVar{Name: freshName, S: st.S}
			lamBody = &syntax.ExprCase{
				Scrutinee: &syntax.ExprVar{Name: freshName, S: st.S},
				Alts:      []syntax.Alt{{Pattern: st.Pat, Body: rest, S: st.S}},
				S:         st.S,
			}
		}
		return &syntax.ExprApp{
			Fun: &syntax.ExprLam{
				Params: []syntax.Pattern{lamParam},
				Body:   lamBody,
				S:      st.S,
			},
			Arg: st.Expr,
			S:   st.S,
		}
	}
	return &syntax.ExprError{S: s}
}

// --- doGraded: GIMonad class dispatch (grade-aware) ---

type doGraded struct {
	ch        *Checker
	monadHead types.Type
	expected  types.Type
}

func (d *doGraded) errPair(s span.Span) (types.Type, ir.Core) {
	return d.expected, &ir.Error{S: s}
}

func (d *doGraded) elaborateBase(expr syntax.Expr, s span.Span) (types.Type, ir.Core) {
	ch := d.ch
	if pureVal := extractPureArg(expr); pureVal != nil {
		if app, ok := d.expected.(*types.TyApp); ok {
			valCore := ch.check(pureVal, app.Arg)
			return d.expected, ch.mkGIPure(d.monadHead, valCore, s)
		}
	}
	return d.expected, ch.check(expr, d.expected)
}

func (d *doGraded) elaborateBind(varName string, comp syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch

	var compCore ir.Core
	var resultTy types.Type

	if pureVal := extractPureArg(comp); pureVal != nil {
		rty, vc := ch.infer(pureVal)
		compCore = ch.mkGIPure(d.monadHead, vc, stmtS)
		resultTy = rty
	} else {
		compTy, cc := ch.infer(comp)
		compCore = cc
		resultTy = ch.extractMonadResult(compTy, d.monadHead, stmtS)
	}

	ch.ctx.Push(&CtxVar{Name: varName, Type: resultTy})
	_, restCore := doElaborate(ch, d, rest, doS)
	ch.ctx.Pop()
	return d.expected, ch.mkGIBind(d.monadHead, compCore, varName, restCore, stmtS)
}

func (d *doGraded) elaborateExprStmt(expr syntax.Expr, rest []syntax.Stmt, stmtS, doS span.Span) (types.Type, ir.Core) {
	ch := d.ch

	var compCore ir.Core
	if pureVal := extractPureArg(expr); pureVal != nil {
		_, vc := ch.infer(pureVal)
		compCore = ch.mkGIPure(d.monadHead, vc, stmtS)
	} else {
		_, cc := ch.infer(expr)
		compCore = cc
	}

	_, restCore := doElaborate(ch, d, rest, doS)
	return d.expected, ch.mkGIBind(d.monadHead, compCore, "_", restCore, stmtS)
}
