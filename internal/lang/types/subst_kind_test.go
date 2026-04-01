package types

import "testing"

// =============================================================================
// Subst — kind-level variable substitution
// After Kind→Type unification, kind substitution uses Subst directly.
// These tests verify that substitution works correctly for kind-level
// variables (TyVar) inside kind-annotated type structures.
// =============================================================================

func TestSubstKindForallArrow(t *testing.T) {
	// forall (f: k -> Type). f Int
	// Subst(_, "k", TypeOfRows) should change f's kind to Row -> Type
	body := &TyForall{
		Var:  "f",
		Kind: &TyArrow{From: &TyVar{Name: "k"}, To: TypeOfTypes},
		Body: &TyApp{Fun: &TyVar{Name: "f"}, Arg: Con("Int")},
	}
	result := Subst(body, "k", TypeOfRows)
	f, ok := result.(*TyForall)
	if !ok {
		t.Fatalf("expected TyForall, got %T", result)
	}
	arrow, ok := f.Kind.(*TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow kind, got %T", f.Kind)
	}
	if !Equal(arrow.From, TypeOfRows) {
		t.Errorf("expected From=Row, got %s", Pretty(arrow.From))
	}
	if !Equal(arrow.To, TypeOfTypes) {
		t.Errorf("expected To=Type, got %s", Pretty(arrow.To))
	}
}

func TestSubstKindNested(t *testing.T) {
	// forall (k: Kind). forall (f: k -> Type). f a -> f a
	// After Subst(inner, "k", TypeOfRows):
	// forall (f: Row -> Type). f a -> f a
	inner := &TyForall{
		Var:  "f",
		Kind: &TyArrow{From: &TyVar{Name: "k"}, To: TypeOfTypes},
		Body: &TyArrow{
			From: &TyApp{Fun: &TyVar{Name: "f"}, Arg: &TyVar{Name: "a"}},
			To:   &TyApp{Fun: &TyVar{Name: "f"}, Arg: &TyVar{Name: "a"}},
		},
	}
	result := Subst(inner, "k", TypeOfRows)
	f := result.(*TyForall)
	arrow := f.Kind.(*TyArrow)
	if !Equal(arrow.From, TypeOfRows) {
		t.Errorf("expected Row, got %s", Pretty(arrow.From))
	}
}

func TestSubstKindMetaOpaque(t *testing.T) {
	// TyMeta is a leaf in Subst — kind field is not traversed.
	meta := &TyMeta{ID: 1, Kind: &TyArrow{From: &TyVar{Name: "k"}, To: TypeOfTypes}}
	result := Subst(meta, "k", TypeOfRows)
	m, ok := result.(*TyMeta)
	if !ok {
		t.Fatalf("expected TyMeta, got %T", result)
	}
	if m.ID != 1 {
		t.Errorf("expected same meta ID")
	}
}

func TestSubstKindSkolemOpaque(t *testing.T) {
	// Subst treats TySkolem as a leaf.
	skolem := &TySkolem{ID: 1, Name: "f", Kind: &TyVar{Name: "k"}}
	result := Subst(skolem, "k", TypeOfTypes)
	s, ok := result.(*TySkolem)
	if !ok {
		t.Fatalf("expected TySkolem, got %T", result)
	}
	if s.ID != 1 {
		t.Errorf("expected same skolem ID")
	}
}

func TestSubstKindNoChange(t *testing.T) {
	// No TyVar "k" in this type — should return same pointer
	ty := &TyArrow{From: Con("Int"), To: Con("Bool")}
	result := Subst(ty, "k", TypeOfRows)
	if result != ty {
		t.Error("expected same pointer when no substitution occurs")
	}
}

func TestSubstKindComp(t *testing.T) {
	ty := MkComp(Con("Unit"), Con("Unit"), Con("Int"))
	result := Subst(ty, "k", TypeOfRows)
	if result != ty {
		t.Error("expected same pointer when no substitution in Comp")
	}
}

func TestSubstKindThunk(t *testing.T) {
	ty := MkThunk(Con("Unit"), Con("Unit"), Con("Int"))
	result := Subst(ty, "k", TypeOfTypes)
	if result != ty {
		t.Error("expected same pointer when no substitution in Thunk")
	}
}
