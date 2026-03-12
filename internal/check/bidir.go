package check

import (
	"fmt"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
	"github.com/cwd-k2/gomputation/internal/syntax"
	"github.com/cwd-k2/gomputation/pkg/types"
)

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

	default:
		ch.addError(expr.Span(), "cannot infer type of expression")
		return &types.TyError{S: expr.Span()}, &core.Var{Name: "<error>", S: expr.Span()}
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
		ch.ctx.Push(&CtxTyVar{Name: f.Var, Kind: f.Kind})
		bodyCore := ch.check(expr, f.Body)
		ch.ctx.Pop()
		return &core.TyLam{TyParam: f.Var, Kind: f.Kind, Body: bodyCore, S: expr.Span()}
	}

	switch e := expr.(type) {
	case *syntax.ExprLam:
		return ch.checkLam(e, expected)

	case *syntax.ExprCase:
		return ch.checkCase(e, expected)

	default:
		// Subsumption: infer type, then check inferred ≤ expected.
		inferredTy, coreExpr := ch.infer(expr)
		if err := ch.unifier.Unify(inferredTy, expected); err != nil {
			ch.addError(expr.Span(), fmt.Sprintf("type mismatch: expected %s, got %s",
				types.Pretty(expected), types.Pretty(inferredTy)))
		}
		return coreExpr
	}
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
		pat, bindings := ch.checkPattern(alt.Pattern, scrutTy)
		for name, ty := range bindings {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
		bodyCore := ch.check(alt.Body, resultTy)
		for range bindings {
			ch.ctx.Pop()
		}
		alts = append(alts, core.Alt{Pattern: pat, Body: bodyCore, S: alt.S})
	}
	ch.checkExhaustive(scrutTy, alts, e.S)
	return &core.Case{Scrutinee: scrutCore, Alts: alts, S: e.S}
}

func (ch *Checker) checkPattern(pat syntax.Pattern, scrutTy types.Type) (core.Pattern, map[string]types.Type) {
	switch p := pat.(type) {
	case *syntax.PatVar:
		return &core.PVar{Name: p.Name, S: p.S}, map[string]types.Type{p.Name: scrutTy}
	case *syntax.PatWild:
		return &core.PWild{S: p.S}, nil
	case *syntax.PatCon:
		conTy, ok := ch.conTypes[p.Con]
		if !ok {
			ch.addError(p.S, fmt.Sprintf("unknown constructor in pattern: %s", p.Con))
			return &core.PWild{S: p.S}, nil
		}
		// Instantiate constructor type and match argument types.
		conTy = ch.unifier.Zonk(conTy)
		var args []core.Pattern
		bindings := make(map[string]types.Type)
		currentTy := conTy
		// Peel off foralls.
		for {
			if f, ok := currentTy.(*types.TyForall); ok {
				meta := ch.freshMeta(f.Kind)
				currentTy = types.Subst(f.Body, f.Var, meta)
			} else {
				break
			}
		}
		for _, argPat := range p.Args {
			argTy, restTy := ch.matchArrow(currentTy, p.S)
			corePat, argBindings := ch.checkPattern(argPat, argTy)
			args = append(args, corePat)
			for k, v := range argBindings {
				bindings[k] = v
			}
			currentTy = restTy
		}
		// Unify result type with scrutinee type.
		if err := ch.unifier.Unify(currentTy, scrutTy); err != nil {
			ch.addError(p.S, fmt.Sprintf("constructor type mismatch: %s", err))
		}
		return &core.PCon{Con: p.Con, Args: args, S: p.S}, bindings
	case *syntax.PatParen:
		return ch.checkPattern(p.Inner, scrutTy)
	default:
		return &core.PWild{S: pat.Span()}, nil
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
		return &types.TyError{S: e.S}, &core.Var{Name: "<error>", S: e.S}
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
			return &types.TyError{S: st.S}, &core.Var{Name: "<error>", S: st.S}
		case *syntax.StmtPureBind:
			ch.addCodedError(errs.ErrBadDoEnding, st.S, "do block must end with an expression")
			return &types.TyError{S: st.S}, &core.Var{Name: "<error>", S: st.S}
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
		ch.addError(s, "unexpected statement in do block")
		return &types.TyError{S: s}, &core.Var{Name: "<error>", S: s}
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
		ch.addCodedError(errs.ErrBadComputation, s, fmt.Sprintf("expected computation type, got %s", types.Pretty(ty)))
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
		ch.addCodedError(errs.ErrBadApplication, s, fmt.Sprintf("expected function type, got %s", types.Pretty(ty)))
	}
	return argTy, retTy
}

func (ch *Checker) instantiate(ty types.Type, expr core.Core) (types.Type, core.Core) {
	for {
		ty = ch.unifier.Zonk(ty)
		f, ok := ty.(*types.TyForall)
		if !ok {
			return ty, expr
		}
		meta := ch.freshMeta(f.Kind)
		ch.trace(TraceInstantiate, span.Span{}, "instantiate: %s → %s[%s := ?%d]",
			types.Pretty(ty), f.Var, types.Pretty(meta), meta.ID)
		ty = types.Subst(f.Body, f.Var, meta)
		expr = &core.TyApp{Expr: expr, TyArg: meta, S: expr.Span()}
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
		return &types.TyCon{Name: t.Name, S: t.S}
	case *syntax.TyExprApp:
		fun := ch.resolveTypeExpr(t.Fun)
		arg := ch.resolveTypeExpr(t.Arg)
		// Recognize Computation and Thunk constructor application.
		result := ch.tryExpandApp(fun, arg, t.S)
		if result != nil {
			return result
		}
		return &types.TyApp{Fun: fun, Arg: arg, S: t.S}
	case *syntax.TyExprArrow:
		return &types.TyArrow{
			From: ch.resolveTypeExpr(t.From),
			To:   ch.resolveTypeExpr(t.To),
			S:    t.S,
		}
	case *syntax.TyExprForall:
		ty := ch.resolveTypeExpr(t.Body)
		for i := len(t.Binders) - 1; i >= 0; i-- {
			kind := ch.resolveKindExpr(t.Binders[i].Kind)
			ty = &types.TyForall{Var: t.Binders[i].Name, Kind: kind, Body: ty, S: t.S}
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
		return &types.TyRow{Fields: fields, Tail: tail, S: t.S}
	case *syntax.TyExprParen:
		return ch.resolveTypeExpr(t.Inner)
	default:
		return &types.TyError{}
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
		ch.addCodedError(errs.ErrBadComputation, compExpr.Span(), fmt.Sprintf("bind: first argument must be a computation, got %s", types.Pretty(compTy)))
		return &types.TyError{S: s}, &core.Var{Name: "<error>", S: s}
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
		ch.addCodedError(errs.ErrBadThunk, e.S, fmt.Sprintf("thunk requires a computation argument, got %s", types.Pretty(argTy)))
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
		ch.addCodedError(errs.ErrBadThunk, e.S, fmt.Sprintf("force requires a thunk argument, got %s", types.Pretty(argTy)))
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

func (ch *Checker) resolveKindExpr(k syntax.KindExpr) types.Kind {
	if k == nil {
		return types.KType{}
	}
	switch ke := k.(type) {
	case *syntax.KindExprType:
		return types.KType{}
	case *syntax.KindExprRow:
		return types.KRow{}
	case *syntax.KindExprArrow:
		return &types.KArrow{From: ch.resolveKindExpr(ke.From), To: ch.resolveKindExpr(ke.To)}
	default:
		return types.KType{}
	}
}
