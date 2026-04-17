// Type helper tests — constructors used by host embedders to declare
// assumption and binding types. These are thin wrappers over types.*
// but are part of the public Go API surface and must remain stable.
// Does NOT cover: type unification (internal/compiler/check/unify).

package engine

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

var testOps = &types.TypeOps{}

func TestTypeHelperConstructors(t *testing.T) {
	intTy := testOps.Con("Int")
	if intTy.Name != "Int" {
		t.Errorf("Con(Int).Name = %q, want Int", intTy.Name)
	}

	arrow := testOps.Arrow(testOps.Con("Int"), testOps.Con("Bool"))
	if arrow == nil {
		t.Error("Arrow() returned nil")
	}

	// Comp with and without grade.
	compNoGrade := testOps.Comp(testOps.Con("A"), testOps.Con("B"), testOps.Con("C"), nil)
	if compNoGrade == nil {
		t.Error("Comp(..., nil) returned nil")
	}
	compGraded := testOps.Comp(testOps.Con("A"), testOps.Con("B"), testOps.Con("C"), testOps.Con("Linear"))
	if !compGraded.IsGraded() {
		t.Errorf("Comp(..., grade) should be graded")
	}

	thunk := testOps.Thunk(testOps.Con("P"), testOps.Con("Q"), testOps.Con("R"), nil)
	if thunk.Tag != types.TagThunk {
		t.Errorf("Thunk().Tag = %v, want TagThunk", thunk.Tag)
	}

	forall := testOps.Forall("a", types.TypeOfTypes, testOps.Con("a"))
	if forall == nil {
		t.Error("Forall() returned nil")
	}
	forallRow := testOps.Forall("r", types.TypeOfRows, testOps.Var("r"))
	if !testOps.Equal(forallRow.Kind, types.TypeOfRows) {
		t.Errorf("Forall(Row) should quantify over Row kind")
	}
	forallKind := testOps.Forall("k", KindType(), testOps.Var("k"))
	if forallKind == nil {
		t.Error("Forall(Kind) returned nil")
	}

	v := testOps.Var("a")
	if v.Name != "a" {
		t.Errorf("Var(a).Name = %q, want a", v.Name)
	}

	app := testOps.App(testOps.Con("List"), testOps.Con("Int"))
	if app == nil {
		t.Error("App() returned nil")
	}
}

func TestRowBuilder(t *testing.T) {
	closed := NewRow(testOps).
		And("x", testOps.Con("Int")).
		And("y", testOps.Con("String")).
		Closed()
	row, ok := closed.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("Closed() = %T, want *types.TyEvidenceRow", closed)
	}
	if !row.IsCapabilityRow() {
		t.Error("RowBuilder should produce a capability row")
	}
	if row.IsOpen() {
		t.Error("Closed() should have nil tail")
	}
	if len(row.CapFields()) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(row.CapFields()))
	}

	open := NewRow(testOps).
		And("a", testOps.Con("Int")).
		Open("r")
	orow := open.(*types.TyEvidenceRow)
	if orow.IsClosed() {
		t.Error("Open() should have a tail var")
	}
}

func TestKindHelpers(t *testing.T) {
	if !testOps.Equal(KindType(), types.TypeOfTypes) {
		t.Error("KindType() != TypeOfTypes")
	}
	if !testOps.Equal(KindRow(), types.TypeOfRows) {
		t.Error("KindRow() != TypeOfRows")
	}
	ka := testOps.Arrow(KindType(), KindType())
	if ka == nil {
		t.Error("Arrow(Kind,Kind) returned nil")
	}
}

func TestEmptyAndClosedRowType(t *testing.T) {
	empty := EmptyRowType()
	row, ok := empty.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("EmptyRowType() = %T, want *types.TyEvidenceRow", empty)
	}
	if row.Entries.EntryCount() != 0 {
		t.Errorf("EmptyRowType entries count = %d, want 0", row.Entries.EntryCount())
	}

	closed := ClosedRowType(types.RowField{Label: "k", Type: testOps.Con("Int")})
	crow := closed.(*types.TyEvidenceRow)
	if crow.IsOpen() {
		t.Error("ClosedRowType should produce a closed row")
	}
	if len(crow.CapFields()) != 1 {
		t.Errorf("ClosedRowType fields = %d, want 1", len(crow.CapFields()))
	}
}
