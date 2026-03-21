package check

import (
	"fmt"
	"maps"

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
		return ch.freshMeta(types.KRow{})
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

	// Fallback: unify all posts (v0 behavior).
	result := zonked[0]
	for i := 1; i < len(zonked); i++ {
		if err := ch.unifier.Unify(result, zonked[i]); err != nil {
			ch.addCodedError(diagnostic.ErrTypeMismatch, s,
				fmt.Sprintf("divergent post-states in case branches: %s vs %s",
					types.Pretty(result), types.Pretty(zonked[i])))
		}
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
			// Unify the type; join multiplicities via LUB.
			resultField := types.RowField{Label: f.Label, Type: f.Type, Mult: f.Mult, S: f.S}
			for _, otherRow := range rows[1:] {
				for _, of := range otherRow.CapFields() {
					if of.Label == f.Label {
						if err := ch.unifier.Unify(resultField.Type, of.Type); err != nil {
							ch.addCodedError(diagnostic.ErrTypeMismatch, s,
								fmt.Sprintf("divergent capability type for %s: %s vs %s",
									f.Label, types.Pretty(resultField.Type), types.Pretty(of.Type)))
						}
						ch.joinMult(&resultField, of.Mult, s)
						break
					}
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

// joinMult joins a result field's multiplicity with another branch's multiplicity.
// Uses the LUB type family when available; falls back to unification.
func (ch *Checker) joinMult(result *types.RowField, other types.Type, s span.Span) {
	m1 := result.Mult
	m2 := other

	if m1 == nil && m2 == nil {
		return
	}

	// One side annotated, other unrestricted → take the annotation (more restrictive).
	if m1 == nil && m2 != nil {
		result.Mult = m2
		return
	}
	if m1 != nil && m2 == nil {
		return // keep m1
	}

	// Both annotated: try LUB type family, fall back to unification.
	lubResult, ok := ch.reduceTyFamily("LUB", []types.Type{m1, m2}, s)
	if ok {
		result.Mult = lubResult
		return
	}
	if err := ch.unifier.Unify(m1, m2); err != nil {
		ch.addCodedError(diagnostic.ErrTypeMismatch, s,
			fmt.Sprintf("divergent multiplicity for %s: %s vs %s",
				result.Label, types.Pretty(m1), types.Pretty(m2)))
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
	resultTy := ch.freshMeta(types.KType{})
	caseCore := ch.checkCaseAlts(scrutTy, resultTy, scrutCore, e)
	return ch.unifier.Zonk(resultTy), caseCore
}

func (ch *Checker) checkCase(e *syntax.ExprCase, expected types.Type) ir.Core {
	scrutTy, scrutCore := ch.infer(e.Scrutinee)
	return ch.checkCaseAlts(scrutTy, expected, scrutCore, e)
}

func (ch *Checker) checkCaseAlts(scrutTy, resultTy types.Type, scrutCore ir.Core, e *syntax.ExprCase) ir.Core {
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
			freshPost := ch.freshMeta(types.KRow{})
			branchExpected = &types.TyCBPV{
				Tag: types.TagComp, Pre: comp.Pre, Post: freshPost, Result: comp.Result, S: comp.S,
			}
			branchPosts = append(branchPosts, freshPost)
		}

		var bodyCore ir.Core
		if needsLocalResolve {
			bodyCore = ch.withDeferredScope(func() ir.Core {
				return ch.check(alt.Body, branchExpected)
			})
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
		if err := ch.unifier.Unify(comp.Post, joinedPost); err != nil {
			ch.addUnifyError(err, e.S, fmt.Sprintf(
				"cannot unify case post-state: expected %s, got %s",
				types.Pretty(comp.Post), types.Pretty(joinedPost)))
		}
	}

	ch.checkExhaustive(scrutTy, alts, e.S)
	return &ir.Case{Scrutinee: scrutCore, Alts: alts, S: e.S}
}
