package family

import (
	"fmt"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Builtin row-level type family names.
const (
	RowFamilyMerge   = "Merge"
	RowFamilyWithout = "Without"
	RowFamilyLookup  = "Lookup"
	RowFamilyMapRow  = "MapRow"
)

// reduceBuiltinRowFamily dispatches to builtin row family reducers.
// Returns (result, true) on successful reduction, or (nil, false) if
// the name is not a builtin or the arguments are not reducible.
func (e *ReduceEnv) reduceBuiltinRowFamily(name string, args []types.Type, s span.Span) (types.Type, bool) {
	switch name {
	case RowFamilyMerge:
		return e.reduceMerge(args, s)
	case RowFamilyWithout:
		return e.reduceWithout(args, s)
	case RowFamilyLookup:
		return e.reduceLookup(args, s)
	case RowFamilyMapRow:
		return e.reduceMapRow(args, s)
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

// reduceWithout implements Without :: Label -> Row -> Row.
// Removes the field with the given label from a capability row.
func (e *ReduceEnv) reduceWithout(args []types.Type, s span.Span) (types.Type, bool) {
	if len(args) != 2 {
		return nil, false
	}
	labelArg := e.Unifier.Zonk(args[0])
	rowArg := e.Unifier.Zonk(args[1])

	// Label must be a concrete label-kinded TyCon (Level L1).
	labelCon, ok := labelArg.(*types.TyCon)
	if !ok || !types.IsKindLevel(labelCon.Level) {
		return nil, false // stuck: label not concrete
	}
	label := labelCon.Name

	// Row must be a concrete capability row.
	row, ok := rowArg.(*types.TyEvidenceRow)
	if !ok || !row.IsCapabilityRow() {
		return nil, false // stuck: not a concrete capability row
	}
	if row.Tail != nil {
		return nil, false // stuck: open row
	}

	_, remaining, found := types.RemoveLabel(row, label)
	if !found {
		e.AddError(diagnostic.ErrTypeMismatch, s,
			fmt.Sprintf("Without: label %q not present in row", label))
		return nil, false
	}
	return remaining, true
}

// reduceLookup implements Lookup :: Label -> Row -> Type.
// Returns the type of the field with the given label in a capability row.
func (e *ReduceEnv) reduceLookup(args []types.Type, s span.Span) (types.Type, bool) {
	if len(args) != 2 {
		return nil, false
	}
	labelArg := e.Unifier.Zonk(args[0])
	rowArg := e.Unifier.Zonk(args[1])

	labelCon, ok := labelArg.(*types.TyCon)
	if !ok || !types.IsKindLevel(labelCon.Level) {
		return nil, false // stuck: label not concrete
	}
	label := labelCon.Name

	row, ok := rowArg.(*types.TyEvidenceRow)
	if !ok || !row.IsCapabilityRow() {
		return nil, false // stuck
	}
	if row.Tail != nil {
		return nil, false // stuck: open row
	}

	ty := types.RowFieldType(row.CapFields(), label)
	if ty == nil {
		e.AddError(diagnostic.ErrTypeMismatch, s,
			fmt.Sprintf("Lookup: label %q not present in row", label))
		return nil, false
	}
	return ty, true
}

// reduceMapRow implements MapRow :: (Type -> Type) -> Row -> Row.
// Applies a type function f to every field type in a capability row.
// Both f and the row must be concrete for reduction to proceed.
func (e *ReduceEnv) reduceMapRow(args []types.Type, s span.Span) (types.Type, bool) {
	if len(args) != 2 {
		return nil, false
	}
	fArg := e.Unifier.Zonk(args[0])
	rowArg := e.Unifier.Zonk(args[1])

	// f must be concrete (not a meta).
	if _, isMeta := fArg.(*types.TyMeta); isMeta {
		return nil, false // stuck: f not resolved
	}

	row, ok := rowArg.(*types.TyEvidenceRow)
	if !ok || !row.IsCapabilityRow() {
		return nil, false // stuck: not a concrete capability row
	}
	if row.Tail != nil {
		return nil, false // stuck: open row
	}

	fields := row.CapFields()
	if len(fields) == 0 {
		return types.ClosedRow(), true // MapRow f {} = {}
	}

	newFields := make([]types.RowField, len(fields))
	for i, field := range fields {
		applied := &types.TyApp{Fun: fArg, Arg: field.Type, S: s}
		reduced := e.reduceFamilyAppsN(applied)
		newFields[i] = types.RowField{
			Label:  field.Label,
			Type:   reduced,
			Grades: field.Grades,
			S:      field.S,
		}
	}
	return types.ClosedRow(newFields...), true
}
