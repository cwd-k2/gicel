package check

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func (ch *Checker) matchArrow(ty types.Type, s span.Span) (types.Type, types.Type) {
	ty = ch.unifier.Zonk(ty)
	// Peel foralls: a higher-rank return type (e.g., from mkId :: () -> \a. a -> a)
	// must be instantiated before arrow decomposition.
	for {
		if f, ok := ty.(*types.TyForall); ok {
			meta := ch.freshMeta(f.Kind)
			ty = types.Subst(f.Body, f.Var, meta)
		} else {
			break
		}
	}
	// Reduce type family applications before arrow decomposition.
	// check() already reduces the expected type, but matchArrow is also called
	// from infer paths where the type may not have been pre-reduced.
	ty = ch.reduceFamilyInType(ty)
	if arr, ok := ty.(*types.TyArrow); ok {
		return arr.From, arr.To
	}
	// Generate fresh metas and decompose eagerly.
	// Eager unification is required here: callers use argTy/retTy immediately
	// for downstream checking (e.g., check(arg, argTy)), so the metas must
	// be solved before control returns. The headIsMeta check in processCtEq
	// would correctly handle error detection, but deferral would leave the
	// decomposition metas unsolved when callers need them.
	argTy := ch.freshMeta(types.TypeOfTypes)
	retTy := ch.freshMeta(types.TypeOfTypes)
	if err := ch.unifier.Unify(ty, types.MkArrow(argTy, retTy)); err != nil {
		ch.addSemanticUnifyError(diagnostic.ErrBadApplication, err, s, fmt.Sprintf("expected function type, got %s", types.Pretty(ty)))
	}
	return argTy, retTy
}

// lookupVar resolves a variable name to its type and Core node.
func (ch *Checker) lookupVar(e *syntax.ExprVar) (types.Type, ir.Core, bool) {
	// Suppress errors for parser error-recovery sentinels.
	if e.Name == "<error>" {
		return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}, false
	}
	ty, mod, ok := ch.ctx.LookupVarFull(e.Name)
	if !ok {
		msg := fmt.Sprintf("unbound variable: %s", e.Name)
		if gatedBuiltins[e.Name] {
			msg += " (requires --recursion flag)"
		}
		if hints := ch.suggestVar(e.Name); len(hints) > 0 {
			ch.addCodedErrorWithHints(diagnostic.ErrUnboundVar, e.S, msg, hints)
		} else {
			ch.addCodedError(diagnostic.ErrUnboundVar, e.S, msg)
		}
		return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}, false
	}
	return ty, &ir.Var{Name: e.Name, Module: mod, S: e.S}, true
}

// lookupCon resolves a constructor name to its type and Core node.
func (ch *Checker) lookupCon(e *syntax.ExprCon) (types.Type, ir.Core, bool) {
	if e.Name == "<error>" {
		return &types.TyError{S: e.S}, &ir.Con{Name: e.Name, S: e.S}, false
	}
	ty, ok := ch.reg.LookupConType(e.Name)
	if !ok {
		msg := fmt.Sprintf("unknown constructor: %s", e.Name)
		if hints := ch.suggestCon(e.Name); len(hints) > 0 {
			ch.addCodedErrorWithHints(diagnostic.ErrUnboundCon, e.S, msg, hints)
		} else {
			ch.addCodedError(diagnostic.ErrUnboundCon, e.S, msg)
		}
		return &types.TyError{S: e.S}, &ir.Con{Name: e.Name, S: e.S}, false
	}
	mod, _ := ch.reg.LookupConModule(e.Name)
	return ty, &ir.Con{Name: e.Name, Module: mod, S: e.S}, true
}

// lookupQualVar resolves a qualified variable reference (N.add) to its type and Core node.
func (ch *Checker) lookupQualVar(e *syntax.ExprQualVar) (types.Type, ir.Core, bool) {
	qs, ok := ch.scope.LookupQualified(e.Qualifier)
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundVar, e.S, fmt.Sprintf("unknown qualifier: %s", e.Qualifier))
		return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}, false
	}
	ty, ok := qs.Exports.Values[e.Name]
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundVar, e.S,
			fmt.Sprintf("module %s does not export value: %s", qs.ModuleName, e.Name))
		return &types.TyError{S: e.S}, &ir.Var{Name: e.Name, S: e.S}, false
	}
	return ty, &ir.Var{Name: e.Name, Module: qs.ModuleName, S: e.S}, true
}

// lookupQualCon resolves a qualified constructor reference (N.Just) to its type and Core node.
func (ch *Checker) lookupQualCon(e *syntax.ExprQualCon) (types.Type, ir.Core, bool) {
	qs, ok := ch.scope.LookupQualified(e.Qualifier)
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundCon, e.S, fmt.Sprintf("unknown qualifier: %s", e.Qualifier))
		return &types.TyError{S: e.S}, &ir.Con{Name: e.Name, S: e.S}, false
	}
	ty, ok := qs.Exports.ConTypes[e.Name]
	if !ok {
		ch.addCodedError(diagnostic.ErrUnboundCon, e.S,
			fmt.Sprintf("module %s does not export constructor: %s", qs.ModuleName, e.Name))
		return &types.TyError{S: e.S}, &ir.Con{Name: e.Name, S: e.S}, false
	}
	return ty, &ir.Con{Name: e.Name, Module: qs.ModuleName, S: e.S}, true
}

// inferHead infers the type of an expression without instantiating outer foralls.
// Used by ExprTyApp to preserve the forall for explicit type application (@).
func (ch *Checker) inferHead(expr syntax.Expr) (types.Type, ir.Core) {
	switch e := expr.(type) {
	case *syntax.ExprVar:
		ty, coreExpr, _ := ch.lookupVar(e)
		return ty, coreExpr
	case *syntax.ExprCon:
		ty, coreExpr, _ := ch.lookupCon(e)
		return ty, coreExpr
	case *syntax.ExprQualVar:
		ty, coreExpr, _ := ch.lookupQualVar(e)
		return ty, coreExpr
	case *syntax.ExprQualCon:
		ty, coreExpr, _ := ch.lookupQualCon(e)
		return ty, coreExpr
	case *syntax.ExprTyApp:
		innerTy, innerCore := ch.inferHead(e.Expr)
		ty := ch.resolveTypeExpr(e.TyArg)
		innerTy = ch.unifier.Zonk(innerTy)
		f, ok := innerTy.(*types.TyForall)
		if !ok {
			ch.addCodedError(diagnostic.ErrBadTypeApp, e.S, "type application to non-polymorphic type")
			return &types.TyError{S: e.S}, innerCore
		}
		resultTy := types.Subst(f.Body, f.Var, ty)
		return resultTy, &ir.TyApp{Expr: innerCore, TyArg: ty, S: e.S}
	default:
		// Non-variable/constructor/TyApp expressions cannot be targets of explicit
		// type application. Falling through to infer (which instantiates) is correct:
		// the caller's instantiate call becomes a no-op since foralls are already gone.
		return ch.infer(expr)
	}
}

func (ch *Checker) instantiate(ty types.Type, expr ir.Core) (types.Type, ir.Core) {
	for {
		ty = ch.unifier.Zonk(ty)
		if f, ok := ty.(*types.TyForall); ok {
			meta := ch.freshMeta(f.Kind)
			if ch.config.Trace != nil {
				ch.trace(TraceInstantiate, span.Span{}, "instantiate: %s → %s[%s := ?%d]",
					types.Pretty(ty), f.Var, types.Pretty(meta), meta.ID)
			}
			ty = types.Subst(f.Body, f.Var, meta)
			expr = &ir.TyApp{Expr: expr, TyArg: meta, S: expr.Span()}
			continue
		}
		if ev, ok := ty.(*types.TyEvidence); ok {
			for _, entry := range ev.Constraints.ConEntries() {
				if entry.IsEquality {
					ch.emitEq(entry.EqLhs, entry.EqRhs, entry.S, nil)
					continue
				}
				placeholder := ch.freshName(prefixDictDefer)
				ch.emitClassConstraint(placeholder, entry, expr.Span())
				expr = &ir.App{Fun: expr, Arg: &ir.Var{Name: placeholder, S: expr.Span()}, S: expr.Span()}
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

// inferList handles list literal [e1, e2, ...] by desugaring to Cons/Nil chain.
func (ch *Checker) inferList(e *syntax.ExprList) (types.Type, ir.Core) {
	elemTy := ch.freshMeta(types.TypeOfTypes)
	listTy := &types.TyApp{Fun: types.Con("List"), Arg: elemTy}

	// Build from the end: Nil, then Cons e_n (Cons e_{n-1} ...)
	nilMod, nilOk := ch.reg.LookupConModule("Nil")
	consMod, consOk := ch.reg.LookupConModule("Cons")
	if !nilOk || !consOk {
		ch.addCodedError(diagnostic.ErrUnboundCon, e.S, "list literals require Prelude; add 'import Prelude'")
		return ch.errorPair(e.S)
	}
	var result ir.Core = &ir.Con{Name: "Nil", Module: nilMod, S: e.S}
	for i := len(e.Elems) - 1; i >= 0; i-- {
		elemCore := ch.check(e.Elems[i], elemTy)
		result = &ir.App{
			Fun: &ir.App{
				Fun: &ir.Con{Name: "Cons", Module: consMod, S: e.S},
				Arg: elemCore,
				S:   e.S,
			},
			Arg: result,
			S:   e.S,
		}
	}

	return ch.unifier.Zonk(listTy), result
}

// suggestVar returns hint(s) for an unbound variable by searching the context
// for similar names.
func (ch *Checker) suggestVar(name string) []diagnostic.Hint {
	seen := make(map[string]bool)
	var candidates []string
	ch.ctx.Scan(func(entry CtxEntry) bool {
		if v, ok := entry.(*CtxVar); ok && !seen[v.Name] && v.Name != "" && v.Name[0] != '$' {
			seen[v.Name] = true
			candidates = append(candidates, v.Name)
		}
		return true
	})
	return suggestHints(name, candidates)
}

// suggestCon returns hint(s) for an unknown constructor by searching the registry.
func (ch *Checker) suggestCon(name string) []diagnostic.Hint {
	var candidates []string
	for c := range ch.reg.AllConTypes() {
		candidates = append(candidates, c)
	}
	return suggestHints(name, candidates)
}

func suggestHints(name string, candidates []string) []diagnostic.Hint {
	matches := diagnostic.Suggest(name, candidates, 2, 3)
	if len(matches) == 0 {
		return nil
	}
	quoted := make([]string, len(matches))
	for i, m := range matches {
		quoted[i] = "'" + m + "'"
	}
	return []diagnostic.Hint{{Message: "did you mean " + strings.Join(quoted, ", ") + "?"}}
}
