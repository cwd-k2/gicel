package check

import (
	"maps"

	"github.com/cwd-k2/gicel/internal/compiler/check/solve"
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
	// Flatten nested rows that arise from meta-tail solutions
	// (e.g., { fail: String | { state: Int | #r } } → { fail: String, state: Int | #r }).
	rows := make([]*types.TyEvidenceRow, 0, len(zonked))
	for _, z := range zonked {
		if ev, ok := z.(*types.TyEvidenceRow); ok {
			rows = append(rows, types.FlattenCapRow(ev))
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
			return "divergent post-states in case branches: " + ch.typeOps.Pretty(r) + " vs " + ch.typeOps.Pretty(z)
		}))
	}
	return result
}

// intersectCapRows computes the intersection of capability rows.
// Labels present in ALL rows are kept; labels present in only some are dropped.
// For shared labels, field types are unified and multiplicities are joined
// via LUB (if the LUB type family is defined) or unified as fallback.
//
// Open-tailed rows (those with a non-nil tail) are treated as potentially
// carrying any labels in the tail. When all tails unify to the same variable,
// a label that appears in some branches' concrete fields but not in others
// is still considered "present" in the open-tailed branches — the tail
// passes those labels through unchanged. This handles asymmetric branches
// where one branch (e.g., failWith) has more concrete labels than another
// (e.g., a state-only do-block).
func (ch *Checker) intersectCapRows(rows []*types.TyEvidenceRow, s span.Span) types.Type {
	if len(rows) == 0 {
		return types.ClosedRow()
	}

	// Handle tail: if all rows have the same tail variable, preserve it.
	// Done first so we can use the tail information for label intersection.
	var tail types.Type
	allSameTail := false
	firstRow := rows[0]
	if firstRow.IsOpen() {
		allSameTail = true
		for _, r := range rows[1:] {
			if r.IsClosed() {
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

	// Collect all labels across all rows.
	allLabels := make(map[string]bool)
	for _, r := range rows {
		for _, f := range r.CapFields() {
			allLabels[f.Label] = true
		}
	}

	// Count label occurrences. When all tails are unified (open rows with
	// a shared tail variable), open-tailed rows contribute to ALL labels:
	// their tail can carry any label not explicitly named, so they don't
	// constrain the intersection.
	labelCount := make(map[string]int)
	n := len(rows)
	for _, r := range rows {
		concreteLabels := make(map[string]bool)
		for _, f := range r.CapFields() {
			concreteLabels[f.Label] = true
			labelCount[f.Label]++
		}
		// Open-tailed row with shared tail: count it for all labels
		// it doesn't explicitly name (they pass through the tail).
		if allSameTail && r.IsOpen() {
			for label := range allLabels {
				if !concreteLabels[label] {
					labelCount[label]++
				}
			}
		}
	}

	// Shared labels: present in ALL rows (directly or via tail).
	var sharedFields []types.RowField
	for _, f := range firstRow.CapFields() {
		if labelCount[f.Label] == n {
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
	// Also collect labels that appear in other rows but not firstRow
	// (they were counted as present in firstRow via tail).
	if allSameTail && firstRow.IsOpen() {
		firstLabels := make(map[string]bool)
		for _, f := range firstRow.CapFields() {
			firstLabels[f.Label] = true
		}
		for _, r := range rows[1:] {
			for _, f := range r.CapFields() {
				if labelCount[f.Label] == n && !firstLabels[f.Label] {
					sharedFields = append(sharedFields, types.RowField{Label: f.Label, Type: f.Type, Grades: f.Grades, S: f.S})
					firstLabels[f.Label] = true // avoid duplicates
				}
			}
		}
	}

	if tail != nil {
		return types.OpenRow(sharedFields, tail)
	}
	return types.ClosedRow(sharedFields...)
}

// joinGrades joins a result field's grades with another branch's grades.
// joinGrades lives in grade.go alongside the other grade-algebra operations.

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
		boolTy := ch.typeOps.Con("Bool", span.Span{})
		if !ch.tryTrivialUnify(scrutTy, boolTy) {
			// Capture the actual type eagerly: once emitEq unifies the meta,
			// a lazy Zonk would return Bool (the expected type), not the actual type.
			actualPretty := ch.typeOps.Pretty(ch.unifier.Zonk(scrutTy))
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

	// Variant scrutinee: extract the index meta s from Variant choices s.
	// Per branch, the pre-state is refined by substituting s → Lookup tag choices.
	_, variantSMeta, isVariantCase := decomposeVariantType(ch.unifier.Zonk(scrutTy))

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
			// Re-zonk comp.Pre for each branch: prior branches may have solved
			// meta tails in comp.Pre (e.g., failWith solving the fail label into
			// the tail). Re-zonking ensures each branch sees the current state.
			zonkedPre := ch.unifier.Zonk(comp.Pre)

			// Variant per-branch pre-state: the index s in the pre-state
			// (from receiveAt's post-state) must become the specific field
			// type for this branch. We create a fresh pre-state meta per
			// branch and unify it with the field-type-substituted row.
			if isVariantCase && pr.VariantFieldTy != nil && variantSMeta != nil {
				zonkedPre = variantSubstPreState(zonkedPre, variantSMeta, pr.VariantFieldTy)
			}

			branchExpected = &types.TyCBPV{
				Tag: types.TagComp, Pre: zonkedPre, Post: freshPost, Result: comp.Result, Flags: types.MetaFreeFlags(zonkedPre, freshPost, comp.Result), S: comp.S,
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
			return "cannot unify case post-state: expected " + ch.typeOps.Pretty(comp.Post) + ", got " + ch.typeOps.Pretty(joinedPost)
		}))
	}

	ch.checkExhaustive(scrutTy, alts, e.S)
	return &ir.Case{Scrutinee: scrutCore, Alts: alts, S: e.S}
}

// variantSubstPreState replaces the Variant index meta in a pre-state row
// with the concrete field type for this branch. Comparison uses the meta
// variable's ID (stable across zonking) rather than pointer equality.
func variantSubstPreState(pre types.Type, sMeta types.Type, fieldTy types.Type) types.Type {
	row, ok := pre.(*types.TyEvidenceRow)
	if !ok || !row.IsCapabilityRow() {
		return pre
	}
	fields := row.CapFields()
	var changed bool
	newFields := make([]types.RowField, len(fields))
	for i, f := range fields {
		newFields[i] = f
		if sameMetaOrEqual(f.Type, sMeta) {
			newFields[i].Type = fieldTy
			changed = true
		}
	}
	if !changed {
		return pre
	}
	if row.IsOpen() {
		return types.OpenRow(newFields, row.Tail)
	}
	return types.ClosedRow(newFields...)
}

// sameMetaOrEqual compares two types for identity. When both are TyMeta,
// uses the stable ID (survives zonking). Otherwise falls back to
// types.Equal for structural comparison.
func sameMetaOrEqual(a, b types.Type) bool {
	if ma, ok := a.(*types.TyMeta); ok {
		if mb, ok := b.(*types.TyMeta); ok {
			return ma.ID == mb.ID
		}
	}
	return types.Equal(a, b)
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
		// NOTE: This ir.Bind is generated post-type-check. The Force/Bind
		// typing is sound (Force : Thunk a → Computation a, Bind sequences
		// computations), but is not verified by the type checker. A future
		// IR verification pass must account for these generated nodes.
		body = &ir.Bind{
			Comp:      &ir.Force{Expr: &ir.Var{Name: f.internalName, S: s}, S: s},
			Var:       f.userName,
			Body:      body,
			Generated: ir.GenAutoForce,
			S:         s,
		}
	}
	return body
}
