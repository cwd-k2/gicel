package family

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Builtin row-level type family names.
const (
	RowFamilyMerge = "Merge"
)

// reduceBuiltinRowFamily dispatches to builtin row family reducers.
// Returns (result, true) on successful reduction, or (nil, false) if
// the name is not a builtin or the arguments are not reducible.
func (e *ReduceEnv) reduceBuiltinRowFamily(name string, args []types.Type, s span.Span) (types.Type, bool) {
	switch name {
	case RowFamilyMerge:
		return e.reduceMerge(args, s)
	default:
		return nil, false
	}
}

// reduceMerge implements the Merge :: Row -> Row -> Row builtin type family.
// Performs disjoint merge of two capability rows. Overlapping labels are an error.
// If either argument contains unsolved metas or is not a concrete row, returns stuck.
func (e *ReduceEnv) reduceMerge(args []types.Type, s span.Span) (types.Type, bool) {
	if len(args) != 2 {
		return nil, false
	}
	lhs := e.Unifier.Zonk(args[0])
	rhs := e.Unifier.Zonk(args[1])

	lRow, lOk := lhs.(*types.TyEvidenceRow)
	rRow, rOk := rhs.(*types.TyEvidenceRow)

	// Both must be concrete capability rows.
	if !lOk || !rOk {
		return nil, false // stuck: not concrete rows
	}
	if !lRow.IsCapabilityRow() || !rRow.IsCapabilityRow() {
		return nil, false // stuck: not capability rows
	}

	lFields := lRow.CapFields()
	rFields := rRow.CapFields()

	// Check for overlapping labels.
	shared, _, _ := types.ClassifyRowFields(lFields, rFields)
	if len(shared) > 0 {
		e.AddError(diagnostic.ErrTypeMismatch, s,
			fmt.Sprintf("Merge: overlapping labels %v", shared))
		return nil, false
	}

	// Merge fields (both sorted, produce sorted result).
	merged := make([]types.RowField, 0, len(lFields)+len(rFields))
	li, ri := 0, 0
	for li < len(lFields) && ri < len(rFields) {
		if lFields[li].Label < rFields[ri].Label {
			merged = append(merged, lFields[li])
			li++
		} else {
			merged = append(merged, rFields[ri])
			ri++
		}
	}
	merged = append(merged, lFields[li:]...)
	merged = append(merged, rFields[ri:]...)

	// Resolve tail: both closed → closed; one open → stuck (need meta resolution).
	lTail := lRow.Tail
	rTail := rRow.Tail
	if lTail != nil || rTail != nil {
		// Open rows: cannot merge statically. Return stuck for solver re-activation.
		return nil, false
	}

	return types.ClosedRow(merged...), true
}
