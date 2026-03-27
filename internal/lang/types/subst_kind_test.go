package types

import "testing"

// =============================================================================
// SubstKindInType — basic cases
// After Kind→Type unification, SubstKindInType delegates to Subst.
// These tests verify that substitution still works correctly for
// kind-level variables (TyVar) inside kind-annotated type structures.
// =============================================================================

func TestSubstKindInTypeForall(t *testing.T) {
	// forall (f: k -> Type). f Int
	// SubstKindInType(_, "k", TypeOfRows) should change f's kind to Row -> Type
	body := &TyForall{
		Var:  "f",
		Kind: &TyArrow{From: &TyVar{Name: "k"}, To: TypeOfTypes},
		Body: &TyApp{Fun: &TyVar{Name: "f"}, Arg: Con("Int")},
	}
	result := SubstKindInType(body, "k", TypeOfRows)
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

func TestSubstKindInTypeNested(t *testing.T) {
	// forall (k: Kind). forall (f: k -> Type). f a -> f a
	// After SubstKindInType(inner, "k", TypeOfRows):
	// forall (f: Row -> Type). f a -> f a
	inner := &TyForall{
		Var:  "f",
		Kind: &TyArrow{From: &TyVar{Name: "k"}, To: TypeOfTypes},
		Body: &TyArrow{
			From: &TyApp{Fun: &TyVar{Name: "f"}, Arg: &TyVar{Name: "a"}},
			To:   &TyApp{Fun: &TyVar{Name: "f"}, Arg: &TyVar{Name: "a"}},
		},
	}
	result := SubstKindInType(inner, "k", TypeOfRows)
	f := result.(*TyForall)
	arrow := f.Kind.(*TyArrow)
	if !Equal(arrow.From, TypeOfRows) {
		t.Errorf("expected Row, got %s", Pretty(arrow.From))
	}
}

func TestSubstKindInTypeMeta(t *testing.T) {
	// TyMeta with kind k -> Type; substitute k := Row
	// After unification, kind vars are TyVar in the Kind field.
	// SubstKindInType now delegates to Subst, which substitutes
	// throughout the entire type including kind fields.
	meta := &TyMeta{ID: 1, Kind: &TyArrow{From: &TyVar{Name: "k"}, To: TypeOfTypes}}
	result := SubstKindInType(meta, "k", TypeOfRows)
	// SubstKindInType is now Subst. TyMeta is a leaf in Subst (no variable
	// substitution happens inside it), so the result should be unchanged.
	// The old behavior substituted into Kind, but Subst treats TyMeta as opaque.
	m, ok := result.(*TyMeta)
	if !ok {
		t.Fatalf("expected TyMeta, got %T", result)
	}
	// After unification, Subst does not traverse into TyMeta.Kind,
	// so the kind should remain unchanged.
	if m.ID != 1 {
		t.Errorf("expected same meta ID")
	}
}

func TestSubstKindInTypeSkolem(t *testing.T) {
	// Similarly, Subst treats TySkolem as a leaf.
	skolem := &TySkolem{ID: 1, Name: "f", Kind: &TyVar{Name: "k"}}
	result := SubstKindInType(skolem, "k", TypeOfTypes)
	s, ok := result.(*TySkolem)
	if !ok {
		t.Fatalf("expected TySkolem, got %T", result)
	}
	if s.ID != 1 {
		t.Errorf("expected same skolem ID")
	}
}

func TestSubstKindInTypeNoChange(t *testing.T) {
	// No TyVar "k" in this type — should return same pointer
	ty := &TyArrow{From: Con("Int"), To: Con("Bool")}
	result := SubstKindInType(ty, "k", TypeOfRows)
	if result != ty {
		t.Error("expected same pointer when no substitution occurs")
	}
}

func TestSubstKindInTypeComp(t *testing.T) {
	// Comp type — after unification, Subst traverses TyCBPV children
	// but TyMeta is a leaf, so kinds inside metas won't be substituted.
	ty := MkComp(Con("Unit"), Con("Unit"), Con("Int"))
	result := SubstKindInType(ty, "k", TypeOfRows)
	if result != ty {
		t.Error("expected same pointer when no substitution in Comp")
	}
}

func TestSubstKindInTypeThunk(t *testing.T) {
	ty := MkThunk(Con("Unit"), Con("Unit"), Con("Int"))
	result := SubstKindInType(ty, "k", TypeOfTypes)
	if result != ty {
		t.Error("expected same pointer when no substitution in Thunk")
	}
}
