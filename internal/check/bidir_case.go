package check

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/types"
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
			ch.addCodedError(errs.ErrTypeMismatch, s,
				fmt.Sprintf("divergent post-states in case branches: %s vs %s",
					types.Pretty(result), types.Pretty(zonked[i])))
		}
	}
	return result
}

// intersectCapRows computes the intersection of capability rows.
// Labels present in ALL rows are kept; labels present in only some are dropped.
// For shared labels, field types and multiplicities are unified.
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
			// Unify the type and mult from all branches.
			resultField := types.RowField{Label: f.Label, Type: f.Type, Mult: f.Mult, S: f.S}
			for _, otherRow := range rows[1:] {
				for _, of := range otherRow.CapFields() {
					if of.Label == f.Label {
						if err := ch.unifier.Unify(resultField.Type, of.Type); err != nil {
							ch.addCodedError(errs.ErrTypeMismatch, s,
								fmt.Sprintf("divergent capability type for %s: %s vs %s",
									f.Label, types.Pretty(resultField.Type), types.Pretty(of.Type)))
						}
						if resultField.Mult != nil && of.Mult != nil {
							if err := ch.unifier.Unify(resultField.Mult, of.Mult); err != nil {
								ch.addCodedError(errs.ErrTypeMismatch, s,
									fmt.Sprintf("divergent multiplicity for %s", f.Label))
							}
						}
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
			if err := ch.unifier.Unify(firstRow.Tail, r.Tail); err != nil {
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
