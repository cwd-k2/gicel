package check

import (
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
// Records the raw type without zonking — the PostGeneralize and
// recordType calls HoverRecorder.RecordType if configured. The HoverRecorder
// re-zonk passes (Rezonk) resolve metavariables later, when all unifications
// and generalizations have completed.
func (ch *Checker) recordType(sp span.Span, ty *types.Type) {
	if ch.config.HoverRecorder != nil && *ty != nil {
		ch.config.HoverRecorder.RecordType(sp, *ty)
	}
}

func (ch *Checker) recordOperator(sp span.Span, name, module string, ty *types.Type) {
	if sp.IsZero() || *ty == nil {
		return
	}
	if ch.config.HoverRecorder != nil {
		ch.config.HoverRecorder.RecordOperator(sp, name, module, *ty)
	}
}

func (ch *Checker) recordVarDoc(sp span.Span, name, module string) {
	if ch.config.HoverRecorder != nil && !sp.IsZero() {
		ch.config.HoverRecorder.RecordVarDoc(sp, name, module)
	}
}

// infer produces a type for an expression and a Core IR node.
func (ch *Checker) infer(expr syntax.Expr) (ty types.Type, core ir.Core) {
	defer ch.recordType(expr.Span(), &ty)
	ch.depth++
	defer func() { ch.depth-- }()
	if err := ch.budget.Nest(); err != nil {
		if !ch.nestingReported {
			ch.nestingReported = true
			ch.addDiag(diagnostic.ErrNestingLimit, expr.Span(), diagWithErr{Context: "nesting limit", Err: err})
		}
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
		// Extract module from the Core node (set by lookupVar from CtxVar.Module).
		varMod := ""
		if v, ok := coreExpr.(*ir.Var); ok {
			varMod = v.Module
		}
		ch.recordVarDoc(e.S, e.Name, varMod)
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
		ch.recordVarDoc(e.S, e.Name, e.Qualifier)
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
			if opTy, mod, ok := ch.ctx.LookupVarFull(e.Op); !ok || mod != "" {
				ch.recordOperator(e.OpSpan, e.Op, mod, &opTy)
				return ch.infer(&syntax.ExprApp{Fun: e.Left, Arg: e.Right, S: e.S})
			}
		}
		// Special form: merge / *** as infix operator.
		if isMergeOp(e.Op) {
			if opTy, mod, ok := ch.ctx.LookupVarFull(e.Op); !ok || mod != "" {
				ch.recordOperator(e.OpSpan, e.Op, mod, &opTy)
				return ch.inferMerge(e.Left, e.Right, e.S)
			}
		}
		// Hint: r.x when the user likely meant r.#x (record projection).
		// The dot operator (.) is function composition. When the RHS is a
		// bare variable not in scope, the user almost certainly intended
		// the record projection syntax .#.
		if e.Op == "." {
			if rv, isVar := e.Right.(*syntax.ExprVar); isVar {
				if _, _, ok := ch.ctx.LookupVarFull(rv.Name); !ok {
					ch.addDiagHints(diagnostic.ErrUnboundVar, e.S,
						diagMsg("use .# for record field access"),
						[]diagnostic.Hint{{Message: "did you mean '.#" + rv.Name + "'?"}})
					return &types.TyError{S: e.S}, &ir.Var{Name: e.Op, S: e.S}
				}
			}
		}
		// Desugar: a op b → App(App(Var(op), a), b)
		opTy, opMod, ok := ch.ctx.LookupVarFull(e.Op)
		if !ok {
			detail := diagUnknown{Kind: "operator", Name: e.Op}
			if hints := suggestImport(e.Op); len(hints) > 0 {
				ch.addDiagHints(diagnostic.ErrUnboundVar, e.S, detail, hints)
			} else if hints := ch.suggestVar(e.Op); len(hints) > 0 {
				ch.addDiagHints(diagnostic.ErrUnboundVar, e.S, detail, hints)
			} else {
				ch.addDiag(diagnostic.ErrUnboundVar, e.S, detail)
			}
			return &types.TyError{S: e.S}, &ir.Var{Name: e.Op, S: e.S}
		}
		opTy, opCore := ch.instantiate(opTy, &ir.Var{Name: e.Op, Module: opMod, S: e.S})
		// Record the operator's own type at its source span for hover.
		ch.recordOperator(e.OpSpan, e.Op, opMod, &opTy)
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
		if !ch.nestingReported {
			ch.nestingReported = true
			ch.addDiag(diagnostic.ErrNestingLimit, expr.Span(), diagWithErr{Context: "nesting limit", Err: err})
		}
		return &ir.Lit{Value: nil, S: expr.Span()}
	}
	defer ch.budget.Unnest()

	expected = ch.unifier.Zonk(expected)
	// Record the checked type for IDE hover. The raw expected type is
	// recorded without extra zonking — HoverRecorder.Rezonk re-zonks
	// all entries after unifications complete.
	if ch.config.HoverRecorder != nil {
		defer ch.config.HoverRecorder.RecordType(expr.Span(), expected)
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
