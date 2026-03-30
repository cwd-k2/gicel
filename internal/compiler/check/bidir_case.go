package check

import (
	"fmt"
	"maps"

	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/syntax"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// lubPostStates computes the join of multiple post-states from case branches.
// Strategy: intersect labels present in all branches. Labels present in only
// some branches are dropped (capability was consumed in those branches).
// For shared labels, types and multiplicities are unified.
func (ch *Checker) lubPostStates(posts []types.Type, s span.Span) types.Type {
	if len(posts) == 0 {
		return ch.freshMeta(types.TypeOfRows)
	}
	if len(posts) == 1 {
		return posts[0]
	}

	// Zonk all post-states to resolve metas.
	zonked := make([]types.Type, len(posts))
	for i, p := range posts {
		zonked[i] = ch.unifier.Zonk(p)
	}

	// Try to extract capability rows from each post-state.
	rows := make([]*types.TyEvidenceRow, 0, len(zonked))
	for _, z := range zonked {
		if ev, ok := z.(*types.TyEvidenceRow); ok {
			rows = append(rows, ev)
		}
	}

	// If all posts resolved to capability rows, compute intersection.
	if len(rows) == len(zonked) {
		return ch.intersectCapRows(rows, s)
	}

	// Fallback: emit equality constraints for all posts (v0 behavior).
	result := zonked[0]
	for i := 1; i < len(zonked); i++ {
		r, z := result, zonked[i]
		ch.emitEq(r, z, s, solve.WithLazyContext(0, func() string {
			return "divergent post-states in case branches: " + types.Pretty(r) + " vs " + types.Pretty(z)
		}))
	}
	return result
}

// intersectCapRows computes the intersection of capability rows.
// Labels present in ALL rows are kept; labels present in only some are dropped.
// For shared labels, field types are unified and multiplicities are joined
// via LUB (if the LUB type family is defined) or unified as fallback.
func (ch *Checker) intersectCapRows(rows []*types.TyEvidenceRow, s span.Span) types.Type {
	if len(rows) == 0 {
		return types.ClosedRow()
	}

	// Count label occurrences across all rows.
	labelCount := make(map[string]int)
	for _, r := range rows {
		for _, f := range r.CapFields() {
			labelCount[f.Label]++
		}
	}

	// Shared labels: present in ALL rows.
	n := len(rows)
	var sharedFields []types.RowField
	firstRow := rows[0]
	for _, f := range firstRow.CapFields() {
		if labelCount[f.Label] == n {
			// This label is in all branches — keep it.
			// Unify the type; join grades via LUB.
			resultField := types.RowField{Label: f.Label, Type: f.Type, Grades: f.Grades, S: f.S}
			for _, otherRow := range rows[1:] {
				if of := types.RowFieldByLabel(otherRow.CapFields(), f.Label); of != nil {
					ch.emitEq(resultField.Type, of.Type, s, solve.WithContext(0, "conflicting field types for label "+f.Label))
					ch.joinGrades(&resultField, of.Grades, s)
				}
			}
			sharedFields = append(sharedFields, resultField)
		}
	}

	// Handle tail: if all rows have the same tail variable, preserve it.
	var tail types.Type
	if firstRow.Tail != nil {
		allSameTail := true
		for _, r := range rows[1:] {
			if r.Tail == nil {
				allSameTail = false
				break
			}
			if !ch.tryUnify(firstRow.Tail, r.Tail) {
				allSameTail = false
				break
			}
		}
		if allSameTail {
			tail = firstRow.Tail
		}
	}

	if tail != nil {
		return types.OpenRow(sharedFields, tail)
	}
	return types.ClosedRow(sharedFields...)
}

// joinGrades joins a result field's grades with another branch's grades.
// Uses the LUB type family when available; falls back to unification.
func (ch *Checker) joinGrades(result *types.RowField, other []types.Type, s span.Span) {
	if len(result.Grades) == 0 && len(other) == 0 {
		return
	}

	// One side annotated, other unrestricted → take the annotation (more restrictive).
	if len(result.Grades) == 0 && len(other) > 0 {
		result.Grades = other
		return
	}
	if len(result.Grades) > 0 && len(other) == 0 {
		return // keep result grades
	}

	// Both annotated: grade counts must match.
	if len(result.Grades) != len(other) {
		ch.addCodedError(diagnostic.ErrTypeMismatch, s,
			fmt.Sprintf("grade count mismatch for %s: %d vs %d",
				result.Label, len(result.Grades), len(other)))
		return
	}
	for i := range result.Grades {
		a := ch.unifier.Zonk(result.Grades[i])
		b := ch.unifier.Zonk(other[i])
		lubResult, ok := ch.reduceTyFamily("LUB", []types.Type{a, b}, s)
		if ok {
			result.Grades[i] = lubResult
			continue
		}
		// Stuck: emit CtFunEq for deferred LUB reduction.
		args := []types.Type{a, b}
		blocking := ch.unifier.CollectBlockingMetas(args)
		if len(blocking) > 0 {
			if _, lubFam := ch.reg.LookupFamily("LUB"); lubFam {
				resultMeta := ch.freshMeta(types.TypeOfTypes)
				ct := &CtFunEq{
					FamilyName: "LUB",
					Args:       args,
					ResultMeta: resultMeta,
					BlockingOn: blocking,
					S:          s,
				}
				ch.registerStuckFunEq(ct)
				result.Grades[i] = resultMeta
				continue
			}
		}
		// No LUB family or no blocking metas: fall back to equality constraint.
		ch.emitEq(a, b, s, nil)
	}
}

func isStructuredPattern(p syntax.Pattern) bool {
	switch pat := p.(type) {
	case *syntax.PatVar, *syntax.PatWild:
		return false
	case *syntax.PatParen:
		return isStructuredPattern(pat.Inner)
	default:
		return true
	}
}

func (ch *Checker) inferCase(e *syntax.ExprCase) (types.Type, ir.Core) {
	scrutTy, scrutCore := ch.infer(e.Scrutinee)
	resultTy := ch.freshMeta(types.TypeOfTypes)
	caseCore := ch.checkCaseAlts(scrutTy, resultTy, scrutCore, e)
	return ch.unifier.Zonk(resultTy), caseCore
}

func (ch *Checker) checkCase(e *syntax.ExprCase, expected types.Type) ir.Core {
	scrutTy, scrutCore := ch.infer(e.Scrutinee)
	return ch.checkCaseAlts(scrutTy, expected, scrutCore, e)
}

func (ch *Checker) checkCaseAlts(scrutTy, resultTy types.Type, scrutCore ir.Core, e *syntax.ExprCase) ir.Core {
	// If-desugar: the scrutinee must be Bool. Check once and emit a clear message
	// on failure, then override scrutTy so the per-branch pattern checks do not
	// produce duplicate "constructor type mismatch" errors.
	if e.IfDesugar {
		boolTy := types.Con("Bool")
		if !ch.tryTrivialUnify(scrutTy, boolTy) {
			// Capture the actual type eagerly: once emitEq unifies the meta,
			// a lazy Zonk would return Bool (the expected type), not the actual type.
			actualPretty := types.Pretty(ch.unifier.Zonk(scrutTy))
			ch.emitEq(scrutTy, boolTy, e.Scrutinee.Span(), solve.WithLazyContext(0, func() string {
				return "type mismatch in if-condition: expected Bool, got " + actualPretty
			}))
		}
		scrutTy = boolTy
	}

	// Divergent post-states: when result is TyCBPV (Computation), each branch gets a
	// fresh post-state meta. After all branches, post-states are joined.
	comp, isComp := ch.unifier.Zonk(resultTy).(*types.TyCBPV)
	if isComp && comp.Tag != types.TagComp {
		isComp = false
	}
	var branchPosts []types.Type

	var alts []ir.Alt
	for _, alt := range e.Alts {
		pr := ch.checkPattern(alt.Pattern, scrutTy)
		for name, ty := range pr.Bindings {
			ch.ctx.Push(&CtxVar{Name: name, Type: ty})
		}
		needsLocalResolve := len(pr.SkolemIDs) > 0 || pr.HasEvidence

		// Per-branch expected type: same resultTy for non-Comp,
		// or TyCBPV with fresh post-state meta for Comp.
		branchExpected := resultTy
		if isComp {
			freshPost := ch.freshMeta(types.TypeOfRows)
			branchExpected = &types.TyCBPV{
				Tag: types.TagComp, Pre: comp.Pre, Post: freshPost, Result: comp.Result, S: comp.S,
			}
			branchPosts = append(branchPosts, freshPost)
		}

		var bodyCore ir.Core
		if needsLocalResolve {
			bodyCore = ch.checkWithLocalScope(alt.Body, branchExpected, pr.SkolemIDs)
		} else {
			bodyCore = ch.check(alt.Body, branchExpected)
		}
		for range pr.Bindings {
			ch.ctx.Pop()
		}
		if len(pr.SkolemIDs) > 0 {
			escapable := maps.Clone(pr.SkolemIDs)
			for _, ty := range pr.GivenEqs {
				removeSkolemIDsFrom(escapable, ty)
			}
			ch.checkSkolemEscape(ch.unifier.Zonk(resultTy), escapable, alt.Body.Span())
		}
		// Auto-force: for lazy co-data constructors, wrap body with
		// Bind(Force($lzN), userName, ...) so pattern-bound ThunkVals
		// are forced before user code sees them.
		bodyCore = ch.autoForceLazy(pr.Pattern, bodyCore, alt.S)

		alts = append(alts, ir.Alt{Pattern: pr.Pattern, Body: bodyCore, S: alt.S})

		// Remove GADT given equalities scoped to this branch.
		// Meta solutions from the branch body are intentionally preserved.
		for skolemID := range pr.GivenEqs {
			ch.unifier.RemoveGivenEq(skolemID)
		}
	}

	// Join divergent post-states.
	if isComp && len(branchPosts) > 0 {
		joinedPost := ch.lubPostStates(branchPosts, e.S)
		ch.emitEq(comp.Post, joinedPost, e.S, solve.WithLazyContext(0, func() string {
			return "cannot unify case post-state: expected " + types.Pretty(comp.Post) + ", got " + types.Pretty(joinedPost)
		}))
	}

	ch.checkExhaustive(scrutTy, alts, e.S)
	return &ir.Case{Scrutinee: scrutCore, Alts: alts, S: e.S}
}

// autoForceLazy handles lazy co-data pattern matching. For lazy constructors,
// pattern-bound variables hold ThunkVals at runtime. This function:
// 1. Renames pattern variables to internal names ($lzN)
// 2. Wraps the body with Bind(Force($lzN), originalName, body)
// Using ir.Bind instead of App(Lam(...)) avoids creating a child closure,
// which would cause de Bruijn index → bytecode slot mapping errors.
func (ch *Checker) autoForceLazy(pat ir.Pattern, body ir.Core, s span.Span) ir.Core {
	pcon, ok := pat.(*ir.PCon)
	if !ok {
		return body
	}
	info, ok := ch.reg.LookupConInfo(pcon.Con)
	if !ok || !info.IsLazy {
		return body
	}
	// Collect pattern variables to force (reverse order for correct nesting).
	type lazyField struct {
		index        int
		internalName string
		userName     string
		s            span.Span
	}
	var fields []lazyField
	for i, arg := range pcon.Args {
		pv, ok := arg.(*ir.PVar)
		if !ok || pv.Name == "_" {
			continue
		}
		fields = append(fields, lazyField{
			index:        i,
			internalName: ch.freshName("$lz"),
			userName:     pv.Name,
			s:            pv.S,
		})
	}
	// Wrap body inside-out: innermost Bind is the last field.
	for i := len(fields) - 1; i >= 0; i-- {
		f := fields[i]
		pcon.Args[f.index] = &ir.PVar{Name: f.internalName, S: f.s}
		body = &ir.Bind{
			Comp: &ir.Force{Expr: &ir.Var{Name: f.internalName, S: s}, S: s},
			Var:  f.userName,
			Body: body,
			S:    s,
		}
	}
	return body
}
