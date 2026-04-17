// Type helper tests — constructors used by host embedders to declare
// assumption and binding types. These are thin wrappers over types.*
// but are part of the public Go API surface and must remain stable.
// Does NOT cover: type unification (internal/compiler/check/unify).

package engine

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

func TestTypeHelperConstructors(t *testing.T) {
	intTy := ConType("Int")
	if con, ok := intTy.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("ConType(Int) = %v, want *types.TyCon{Name: Int}", intTy)
	}

	arrow := ArrowType(ConType("Int"), ConType("Bool"))
	if _, ok := arrow.(*types.TyArrow); !ok {
		t.Errorf("ArrowType() = %T, want *types.TyArrow", arrow)
	}

	// CompType with and without grade.
	compNoGrade := CompType(ConType("A"), ConType("B"), ConType("C"), nil)
	if _, ok := compNoGrade.(*types.TyCBPV); !ok {
		t.Errorf("CompType(..., nil) = %T, want *types.TyCBPV", compNoGrade)
	}
	compGraded := CompType(ConType("A"), ConType("B"), ConType("C"), ConType("Linear"))
	if cbpv, ok := compGraded.(*types.TyCBPV); !ok || !cbpv.IsGraded() {
		t.Errorf("CompType(..., grade) = %v, want graded CBPV", compGraded)
	}

	thunk := ThunkType(ConType("P"), ConType("Q"), ConType("R"))
	if cbpv, ok := thunk.(*types.TyCBPV); !ok || cbpv.Tag != types.TagThunk {
		t.Errorf("ThunkType() = %v, want TagThunk", thunk)
	}

	forall := ForallType("a", ConType("a"))
	if _, ok := forall.(*types.TyForall); !ok {
		t.Errorf("ForallType() = %T, want *types.TyForall", forall)
	}
	forallRow := ForallRow("r", VarType("r"))
	if f, ok := forallRow.(*types.TyForall); !ok || !types.Equal(f.Kind, types.TypeOfRows) {
		t.Errorf("ForallRow() should quantify over Row kind, got %v", forallRow)
	}
	forallKind := ForallKind("k", KindType(), VarType("k"))
	if _, ok := forallKind.(*types.TyForall); !ok {
		t.Errorf("ForallKind() = %T, want *types.TyForall", forallKind)
	}

	if _, ok := VarType("a").(*types.TyVar); !ok {
		t.Errorf("VarType() should return *types.TyVar")
	}

	app := AppType(ConType("List"), ConType("Int"))
	if _, ok := app.(*types.TyApp); !ok {
		t.Errorf("AppType() = %T, want *types.TyApp", app)
	}
}

func TestRowBuilder(t *testing.T) {
	closed := NewRow().
		And("x", ConType("Int")).
		And("y", ConType("String")).
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

	open := NewRow().
		And("a", ConType("Int")).
		Open("r")
	orow := open.(*types.TyEvidenceRow)
	if orow.IsClosed() {
		t.Error("Open() should have a tail var")
	}
}

func TestKindHelpers(t *testing.T) {
	if !types.Equal(KindType(), types.TypeOfTypes) {
		t.Error("KindType() != TypeOfTypes")
	}
	if !types.Equal(KindRow(), types.TypeOfRows) {
		t.Error("KindRow() != TypeOfRows")
	}
	if _, ok := KindArrow(KindType(), KindType()).(*types.TyArrow); !ok {
		t.Error("KindArrow() should return *types.TyArrow")
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

	closed := ClosedRowType(types.RowField{Label: "k", Type: types.MkCon("Int")})
	crow := closed.(*types.TyEvidenceRow)
	if crow.IsOpen() {
		t.Error("ClosedRowType should produce a closed row")
	}
	if len(crow.CapFields()) != 1 {
		t.Errorf("ClosedRowType fields = %d, want 1", len(crow.CapFields()))
	}
}
