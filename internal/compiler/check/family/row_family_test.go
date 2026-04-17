// Family row tests — builtin Merge, Without, Lookup type families.
// Does NOT cover: reduce_test.go (pattern matching), verify_test.go (injectivity).
package family

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- helpers for row family tests ---

// labelCon creates a label-kinded TyCon at L1.
func labelCon(name string) *types.TyCon {
	return &types.TyCon{Name: name, Level: types.L1, IsLabel: true}
}

// capRow creates a closed capability row from label-type pairs.
func capRow(pairs ...any) *types.TyEvidenceRow {
	fields := make([]types.RowField, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		fields = append(fields, types.RowField{
			Label: pairs[i].(string),
			Type:  pairs[i+1].(types.Type),
		})
	}
	return types.ClosedRow(fields...)
}

// --- Merge tests ---

func TestReduceMerge_Disjoint(t *testing.T) {
	h := newTestHarness(nil)
	lhs := capRow("Fail", con("Unit"))
	rhs := capRow("State", con("Int"))

	result, ok := h.env.reduceMerge([]types.Type{lhs, rhs}, span.Span{})
	if !ok {
		t.Fatal("expected Merge to succeed for disjoint rows")
	}

	row, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := row.CapFields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	// Fields should be sorted by label.
	if fields[0].Label != "Fail" || fields[1].Label != "State" {
		t.Fatalf("expected [Fail, State], got [%s, %s]", fields[0].Label, fields[1].Label)
	}
}

func TestReduceMerge_Overlapping(t *testing.T) {
	h := newTestHarness(nil)
	lhs := capRow("Fail", con("Unit"))
	rhs := capRow("Fail", con("String"))

	_, ok := h.env.reduceMerge([]types.Type{lhs, rhs}, span.Span{})
	if ok {
		t.Fatal("expected Merge to fail for overlapping labels")
	}
	if len(h.errors) == 0 {
		t.Fatal("expected overlapping label error")
	}
	if !strings.Contains(h.errors[0], "overlapping") {
		t.Fatalf("expected 'overlapping' in error, got: %s", h.errors[0])
	}
}

func TestReduceMerge_EmptyRows(t *testing.T) {
	h := newTestHarness(nil)
	lhs := capRow()
	rhs := capRow()

	result, ok := h.env.reduceMerge([]types.Type{lhs, rhs}, span.Span{})
	if !ok {
		t.Fatal("expected Merge of empty rows to succeed")
	}
	row := result.(*types.TyEvidenceRow)
	if len(row.CapFields()) != 0 {
		t.Fatal("expected empty result row")
	}
}

func TestReduceMerge_EmptyAndNonEmpty(t *testing.T) {
	h := newTestHarness(nil)
	lhs := capRow()
	rhs := capRow("IO", con("Unit"))

	result, ok := h.env.reduceMerge([]types.Type{lhs, rhs}, span.Span{})
	if !ok {
		t.Fatal("expected Merge to succeed")
	}
	row := result.(*types.TyEvidenceRow)
	fields := row.CapFields()
	if len(fields) != 1 || fields[0].Label != "IO" {
		t.Fatalf("expected [IO], got %v", fields)
	}
}

func TestReduceMerge_NonRowStuck(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceMerge([]types.Type{con("Int"), capRow("IO", con("Unit"))}, span.Span{})
	if ok {
		t.Fatal("expected stuck when lhs is not a row")
	}
}

func TestReduceMerge_MetaStuck(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceMerge([]types.Type{meta(1), capRow("IO", con("Unit"))}, span.Span{})
	if ok {
		t.Fatal("expected stuck when lhs is a meta")
	}
}

func TestReduceMerge_OpenRowStuck(t *testing.T) {
	h := newTestHarness(nil)
	// Create an open row (with tail).
	openRow := types.OpenRow([]types.RowField{{Label: "IO", Type: con("Unit")}}, meta(1))

	_, ok := h.env.reduceMerge([]types.Type{openRow, capRow("Fail", con("String"))}, span.Span{})
	if ok {
		t.Fatal("expected stuck for open row")
	}
}

func TestReduceMerge_WrongArity(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceMerge([]types.Type{capRow("IO", con("Unit"))}, span.Span{})
	if ok {
		t.Fatal("expected stuck for wrong arity")
	}
}

// --- Without tests ---

func TestReduceWithout_RemoveLabel(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("Unit"), "IO", con("Unit"), "State", con("Int"))

	result, ok := h.env.reduceWithout([]types.Type{labelCon("IO"), row}, span.Span{})
	if !ok {
		t.Fatal("expected Without to succeed")
	}
	rRow := result.(*types.TyEvidenceRow)
	fields := rRow.CapFields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 remaining fields, got %d", len(fields))
	}
	for _, f := range fields {
		if f.Label == "IO" {
			t.Fatal("IO should have been removed")
		}
	}
}

func TestReduceWithout_LabelNotPresent(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("Unit"))

	_, ok := h.env.reduceWithout([]types.Type{labelCon("IO"), row}, span.Span{})
	if ok {
		t.Fatal("expected Without to fail when label is absent")
	}
	if len(h.errors) == 0 || !strings.Contains(h.errors[0], "not present") {
		t.Fatalf("expected 'not present' error, got %v", h.errors)
	}
}

func TestReduceWithout_NonConcreteLabelStuck(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("Unit"))

	_, ok := h.env.reduceWithout([]types.Type{meta(1), row}, span.Span{})
	if ok {
		t.Fatal("expected stuck when label is a meta")
	}
}

func TestReduceWithout_NonRowStuck(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceWithout([]types.Type{labelCon("IO"), con("Int")}, span.Span{})
	if ok {
		t.Fatal("expected stuck when row arg is not a row")
	}
}

func TestReduceWithout_OpenRowStuck(t *testing.T) {
	h := newTestHarness(nil)
	openRow := types.OpenRow([]types.RowField{{Label: "IO", Type: con("Unit")}}, meta(1))

	_, ok := h.env.reduceWithout([]types.Type{labelCon("IO"), openRow}, span.Span{})
	if ok {
		t.Fatal("expected stuck for open row")
	}
}

// --- Lookup tests ---

func TestReduceLookup_Found(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("String"), "IO", con("Unit"))

	result, ok := h.env.reduceLookup([]types.Type{labelCon("Fail"), row}, span.Span{})
	if !ok {
		t.Fatal("expected Lookup to succeed")
	}
	if !testOps.Equal(result, con("String")) {
		t.Fatalf("expected String, got %v", result)
	}
}

func TestReduceLookup_NotFound(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("String"))

	_, ok := h.env.reduceLookup([]types.Type{labelCon("IO"), row}, span.Span{})
	if ok {
		t.Fatal("expected Lookup to fail when label is absent")
	}
	if len(h.errors) == 0 || !strings.Contains(h.errors[0], "not present") {
		t.Fatalf("expected 'not present' error, got %v", h.errors)
	}
}

func TestReduceLookup_NonCapabilityRowStuck(t *testing.T) {
	h := newTestHarness(nil)
	cr := types.SingleConstraint("Eq", []types.Type{con("Int")})

	_, ok := h.env.reduceLookup([]types.Type{labelCon("Eq"), cr}, span.Span{})
	if ok {
		t.Fatal("expected stuck for non-capability row")
	}
}

func TestReduceLookup_NonConcreteStuck(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("String"))

	_, ok := h.env.reduceLookup([]types.Type{meta(1), row}, span.Span{})
	if ok {
		t.Fatal("expected stuck for non-concrete label")
	}
}

func TestReduceLookup_OpenRowStuck(t *testing.T) {
	h := newTestHarness(nil)
	openRow := types.OpenRow([]types.RowField{{Label: "IO", Type: con("Unit")}}, meta(1))

	_, ok := h.env.reduceLookup([]types.Type{labelCon("IO"), openRow}, span.Span{})
	if ok {
		t.Fatal("expected stuck for open row")
	}
}

func TestReduceWithout_WrongArity(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceWithout([]types.Type{labelCon("IO")}, span.Span{})
	if ok {
		t.Fatal("expected stuck for wrong arity")
	}
}

func TestReduceWithout_NonLabelTyCon(t *testing.T) {
	// L0 TyCon (not a label) should be stuck.
	h := newTestHarness(nil)
	row := capRow("Fail", con("Unit"))

	_, ok := h.env.reduceWithout([]types.Type{con("Int"), row}, span.Span{})
	if ok {
		t.Fatal("expected stuck for non-label TyCon")
	}
}

func TestReduceLookup_WrongArity(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceLookup([]types.Type{labelCon("IO")}, span.Span{})
	if ok {
		t.Fatal("expected stuck for wrong arity")
	}
}

func TestReduceLookup_NonLabelTyCon(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("String"))

	_, ok := h.env.reduceLookup([]types.Type{con("Int"), row}, span.Span{})
	if ok {
		t.Fatal("expected stuck for non-label TyCon")
	}
}

func TestReduceMerge_NonCapabilityRow(t *testing.T) {
	// Constraint rows are not capability rows → stuck.
	h := newTestHarness(nil)
	cr := types.SingleConstraint("Eq", []types.Type{con("Int")})

	_, ok := h.env.reduceMerge([]types.Type{cr, capRow("IO", con("Unit"))}, span.Span{})
	if ok {
		t.Fatal("expected stuck for non-capability row")
	}
}

// --- dispatch tests ---

func TestReduceBuiltinRowFamily_UnknownName(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceBuiltinRowFamily("NotARowFamily", nil, span.Span{})
	if ok {
		t.Fatal("expected false for unknown builtin name")
	}
}

func TestReduceBuiltinRowFamily_WithoutDispatch(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("Unit"), "IO", con("Unit"))

	result, ok := h.env.reduceBuiltinRowFamily("Without", []types.Type{labelCon("IO"), row}, span.Span{})
	if !ok {
		t.Fatal("expected Without to dispatch successfully")
	}
	rRow := result.(*types.TyEvidenceRow)
	if len(rRow.CapFields()) != 1 || rRow.CapFields()[0].Label != "Fail" {
		t.Fatalf("expected [Fail], got %v", rRow.CapFields())
	}
}

func TestReduceBuiltinRowFamily_LookupDispatch(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("Fail", con("String"))

	result, ok := h.env.reduceBuiltinRowFamily("Lookup", []types.Type{labelCon("Fail"), row}, span.Span{})
	if !ok {
		t.Fatal("expected Lookup to dispatch successfully")
	}
	if !testOps.Equal(result, con("String")) {
		t.Fatalf("expected String, got %v", result)
	}
}

// --- MapRow tests ---

func TestReduceMapRow_EmptyRow(t *testing.T) {
	h := newTestHarness(nil)
	f := con("Dual") // any concrete function
	row := capRow()

	result, ok := h.env.reduceMapRow([]types.Type{f, row}, span.Span{})
	if !ok {
		t.Fatal("expected MapRow on empty row to succeed")
	}
	rRow := result.(*types.TyEvidenceRow)
	if len(rRow.CapFields()) != 0 {
		t.Fatal("expected empty result row")
	}
}

func TestReduceMapRow_MultiField(t *testing.T) {
	h := newTestHarness(nil)
	f := con("Dual")
	row := capRow("a", con("Send"), "b", con("Recv"))

	result, ok := h.env.reduceMapRow([]types.Type{f, row}, span.Span{})
	if !ok {
		t.Fatal("expected MapRow on multi-field row to succeed")
	}
	rRow := result.(*types.TyEvidenceRow)
	fields := rRow.CapFields()
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	// Each field type should be TyApp(Dual, original).
	for _, field := range fields {
		a, ok := field.Type.(*types.TyApp)
		if !ok {
			t.Fatalf("expected TyApp for field %s, got %T", field.Label, field.Type)
		}
		if !testOps.Equal(a.Fun, f) {
			t.Fatalf("expected Dual as function, got %v", a.Fun)
		}
	}
	// Verify specific args.
	if !testOps.Equal(fields[0].Type.(*types.TyApp).Arg, con("Send")) {
		t.Fatalf("field 'a' should have arg Send")
	}
	if !testOps.Equal(fields[1].Type.(*types.TyApp).Arg, con("Recv")) {
		t.Fatalf("field 'b' should have arg Recv")
	}
}

func TestReduceMapRow_PreservesGrades(t *testing.T) {
	h := newTestHarness(nil)
	f := con("Dual")
	graded := types.ClosedRow(types.RowField{
		Label:  "ch",
		Type:   con("Send"),
		Grades: []types.Type{con("Linear")},
	})

	result, ok := h.env.reduceMapRow([]types.Type{f, graded}, span.Span{})
	if !ok {
		t.Fatal("expected MapRow to succeed")
	}
	rRow := result.(*types.TyEvidenceRow)
	fields := rRow.CapFields()
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if len(fields[0].Grades) != 1 || !testOps.Equal(fields[0].Grades[0], con("Linear")) {
		t.Fatal("grade annotation not preserved")
	}
}

func TestReduceMapRow_OpenRowStuck(t *testing.T) {
	h := newTestHarness(nil)
	f := con("Dual")
	openRow := types.OpenRow([]types.RowField{{Label: "a", Type: con("Send")}}, meta(1))

	_, ok := h.env.reduceMapRow([]types.Type{f, openRow}, span.Span{})
	if ok {
		t.Fatal("expected stuck for open row")
	}
}

func TestReduceMapRow_MetaFStuck(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("a", con("Send"))

	_, ok := h.env.reduceMapRow([]types.Type{meta(1), row}, span.Span{})
	if ok {
		t.Fatal("expected stuck when f is a meta")
	}
}

func TestReduceMapRow_WrongArity(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceMapRow([]types.Type{con("Dual")}, span.Span{})
	if ok {
		t.Fatal("expected stuck for wrong arity")
	}
}

func TestReduceMapRow_NonRowStuck(t *testing.T) {
	h := newTestHarness(nil)

	_, ok := h.env.reduceMapRow([]types.Type{con("Dual"), con("Int")}, span.Span{})
	if ok {
		t.Fatal("expected stuck when row arg is not a row")
	}
}

func TestReduceBuiltinRowFamily_MapRowDispatch(t *testing.T) {
	h := newTestHarness(nil)
	row := capRow("a", con("Send"))

	result, ok := h.env.reduceBuiltinRowFamily("MapRow", []types.Type{con("Dual"), row}, span.Span{})
	if !ok {
		t.Fatal("expected MapRow to dispatch successfully")
	}
	rRow := result.(*types.TyEvidenceRow)
	fields := rRow.CapFields()
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	a, ok := fields[0].Type.(*types.TyApp)
	if !ok {
		t.Fatalf("expected TyApp, got %T", fields[0].Type)
	}
	if !testOps.Equal(a.Fun, con("Dual")) || !testOps.Equal(a.Arg, con("Send")) {
		t.Fatalf("expected Dual Send, got %v %v", a.Fun, a.Arg)
	}
}
