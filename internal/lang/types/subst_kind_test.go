package types

import "testing"

// =============================================================================
// SubstKindInType — basic cases
// =============================================================================

func TestSubstKindInTypeForall(t *testing.T) {
	// forall (f: k -> Type). f Int
	// SubstKindInType(_, "k", KRow{}) should change f's kind to Row -> Type
	body := &TyForall{
		Var:  "f",
		Kind: &KArrow{From: KVar{Name: "k"}, To: KType{}},
		Body: &TyApp{Fun: &TyVar{Name: "f"}, Arg: Con("Int")},
	}
	result := SubstKindInType(body, "k", KRow{})
	f, ok := result.(*TyForall)
	if !ok {
		t.Fatalf("expected TyForall, got %T", result)
	}
	arrow, ok := f.Kind.(*KArrow)
	if !ok {
		t.Fatalf("expected KArrow kind, got %T", f.Kind)
	}
	if !arrow.From.Equal(KRow{}) {
		t.Errorf("expected From=Row, got %s", arrow.From)
	}
	if !arrow.To.Equal(KType{}) {
		t.Errorf("expected To=Type, got %s", arrow.To)
	}
}

func TestSubstKindInTypeNested(t *testing.T) {
	// forall (k: Kind). forall (f: k -> Type). f a -> f a
	// After SubstKindInType(inner, "k", KRow{}):
	// forall (f: Row -> Type). f a -> f a
	inner := &TyForall{
		Var:  "f",
		Kind: &KArrow{From: KVar{Name: "k"}, To: KType{}},
		Body: &TyArrow{
			From: &TyApp{Fun: &TyVar{Name: "f"}, Arg: &TyVar{Name: "a"}},
			To:   &TyApp{Fun: &TyVar{Name: "f"}, Arg: &TyVar{Name: "a"}},
		},
	}
	result := SubstKindInType(inner, "k", KRow{})
	f := result.(*TyForall)
	arrow := f.Kind.(*KArrow)
	if !arrow.From.Equal(KRow{}) {
		t.Errorf("expected Row, got %s", arrow.From)
	}
}

func TestSubstKindInTypeMeta(t *testing.T) {
	// TyMeta with kind k -> Type; substitute k := Row
	meta := &TyMeta{ID: 1, Kind: &KArrow{From: KVar{Name: "k"}, To: KType{}}}
	result := SubstKindInType(meta, "k", KRow{})
	m, ok := result.(*TyMeta)
	if !ok {
		t.Fatalf("expected TyMeta, got %T", result)
	}
	arrow := m.Kind.(*KArrow)
	if !arrow.From.Equal(KRow{}) {
		t.Errorf("expected Row, got %s", arrow.From)
	}
}

func TestSubstKindInTypeSkolem(t *testing.T) {
	skolem := &TySkolem{ID: 1, Name: "f", Kind: KVar{Name: "k"}}
	result := SubstKindInType(skolem, "k", KType{})
	s, ok := result.(*TySkolem)
	if !ok {
		t.Fatalf("expected TySkolem, got %T", result)
	}
	if !s.Kind.Equal(KType{}) {
		t.Errorf("expected Type, got %s", s.Kind)
	}
}

func TestSubstKindInTypeNoChange(t *testing.T) {
	// No KVar "k" in this type — should return same pointer
	ty := &TyArrow{From: Con("Int"), To: Con("Bool")}
	result := SubstKindInType(ty, "k", KRow{})
	if result != ty {
		t.Error("expected same pointer when no substitution occurs")
	}
}

func TestSubstKindInTypeComp(t *testing.T) {
	// Comp with a meta whose kind contains k
	ty := MkComp(&TyMeta{ID: 1, Kind: KVar{Name: "k"}}, &TyMeta{ID: 2, Kind: KVar{Name: "k"}}, Con("Int"))
	result := SubstKindInType(ty, "k", KRow{})
	comp := result.(*TyCBPV)
	if !comp.Pre.(*TyMeta).Kind.Equal(KRow{}) {
		t.Errorf("Pre meta kind should be Row, got %s", comp.Pre.(*TyMeta).Kind)
	}
	if !comp.Post.(*TyMeta).Kind.Equal(KRow{}) {
		t.Errorf("Post meta kind should be Row, got %s", comp.Post.(*TyMeta).Kind)
	}
}

func TestSubstKindInTypeThunk(t *testing.T) {
	ty := MkThunk(&TyMeta{ID: 1, Kind: KVar{Name: "k"}}, Con("Unit"), Con("Int"))
	result := SubstKindInType(ty, "k", KType{})
	thunk := result.(*TyCBPV)
	if !thunk.Pre.(*TyMeta).Kind.Equal(KType{}) {
		t.Errorf("Pre meta kind should be Type, got %s", thunk.Pre.(*TyMeta).Kind)
	}
}
