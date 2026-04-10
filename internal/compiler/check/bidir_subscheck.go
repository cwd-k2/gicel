package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

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
