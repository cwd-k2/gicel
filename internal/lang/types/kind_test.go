package types

import "testing"

// --- Group 2A: Kind representation extensions ---

func TestKindMetaConstruction(t *testing.T) {
	m := &KMeta{ID: 1}
	if m.ID != 1 {
		t.Errorf("expected ID 1, got %d", m.ID)
	}
}

func TestKindMetaEqual(t *testing.T) {
	m1 := &KMeta{ID: 1}
	m2 := &KMeta{ID: 1}
	m3 := &KMeta{ID: 2}
	if !m1.Equal(m2) {
		t.Error("same ID should be equal")
	}
	if m1.Equal(m3) {
		t.Error("different ID should not be equal")
	}
	if m1.Equal(KType{}) {
		t.Error("KMeta should not equal KType")
	}
}

func TestKindMetaString(t *testing.T) {
	m := &KMeta{ID: 42}
	if m.String() != "?k42" {
		t.Errorf("expected ?k42, got %s", m.String())
	}
}

func TestKindVarConstruction(t *testing.T) {
	v := KVar{Name: "k"}
	if v.Name != "k" {
		t.Errorf("expected k, got %s", v.Name)
	}
}

func TestKindVarEqual(t *testing.T) {
	v1 := KVar{Name: "k"}
	v2 := KVar{Name: "k"}
	v3 := KVar{Name: "j"}
	if !v1.Equal(v2) {
		t.Error("same name should be equal")
	}
	if v1.Equal(v3) {
		t.Error("different name should not be equal")
	}
	if v1.Equal(KType{}) {
		t.Error("KVar should not equal KType")
	}
}

func TestKindVarString(t *testing.T) {
	v := KVar{Name: "k"}
	if v.String() != "k" {
		t.Errorf("expected k, got %s", v.String())
	}
}

func TestKindSortConstruction(t *testing.T) {
	s := KSort{}
	_ = s
}

func TestKindSortEqual(t *testing.T) {
	s1 := KSort{}
	s2 := KSort{}
	if !s1.Equal(s2) {
		t.Error("KSort should be equal to itself")
	}
	if s1.Equal(KType{}) {
		t.Error("KSort should not equal KType")
	}
}

func TestKindSortString(t *testing.T) {
	s := KSort{}
	if s.String() != "Kind" {
		t.Errorf("expected Kind, got %s", s.String())
	}
}

func TestKindSubst(t *testing.T) {
	// k -> Type with k := Row → k -> Type becomes Row -> Type
	k := &KArrow{From: KVar{Name: "k"}, To: KType{}}
	result := KindSubst(k, "k", KRow{})
	arrow, ok := result.(*KArrow)
	if !ok {
		t.Fatalf("expected KArrow, got %T", result)
	}
	if !arrow.From.Equal(KRow{}) {
		t.Errorf("expected Row, got %s", arrow.From)
	}
	if !arrow.To.Equal(KType{}) {
		t.Errorf("expected Type, got %s", arrow.To)
	}
}

func TestKindSubstNoMatch(t *testing.T) {
	k := KVar{Name: "k"}
	result := KindSubst(k, "j", KRow{})
	if !result.Equal(k) {
		t.Errorf("unmatched var should remain, got %s", result)
	}
}

func TestKindSubstMeta(t *testing.T) {
	// KMeta is not affected by variable substitution.
	m := &KMeta{ID: 1}
	result := KindSubst(m, "k", KRow{})
	if !result.Equal(m) {
		t.Error("KMeta should not be affected by KindSubst")
	}
}

func TestKindSubstNested(t *testing.T) {
	// (k -> k) -> Type with k := Row → (Row -> Row) -> Type
	inner := &KArrow{From: KVar{Name: "k"}, To: KVar{Name: "k"}}
	k := &KArrow{From: inner, To: KType{}}
	result := KindSubst(k, "k", KRow{})
	arrow, ok := result.(*KArrow)
	if !ok {
		t.Fatalf("expected KArrow, got %T", result)
	}
	innerResult, ok := arrow.From.(*KArrow)
	if !ok {
		t.Fatalf("expected inner KArrow, got %T", arrow.From)
	}
	if !innerResult.From.Equal(KRow{}) || !innerResult.To.Equal(KRow{}) {
		t.Errorf("expected Row -> Row, got %s -> %s", innerResult.From, innerResult.To)
	}
}

func TestKindArrowWithMeta(t *testing.T) {
	// ?k1 -> Type is a valid kind
	m := &KMeta{ID: 1}
	arrow := &KArrow{From: m, To: KType{}}
	if arrow.From.String() != "?k1" {
		t.Errorf("expected ?k1, got %s", arrow.From)
	}
}

func TestKindArrowParensPrinting(t *testing.T) {
	// (k -> Type) -> Type should parenthesize the left arrow
	inner := &KArrow{From: KVar{Name: "k"}, To: KType{}}
	outer := &KArrow{From: inner, To: KType{}}
	expected := "(k -> Type) -> Type"
	if outer.String() != expected {
		t.Errorf("expected %s, got %s", expected, outer.String())
	}
}

// --- KSort Level tests ---

func TestKSortLevelEquality(t *testing.T) {
	s0a := KSort{Level: 0}
	s0b := KSort{} // zero-value: Level 0
	s1 := KSort{Level: 1}

	if !s0a.Equal(s0b) {
		t.Error("KSort{Level: 0} should equal KSort{}")
	}
	if s0a.Equal(s1) {
		t.Error("KSort{Level: 0} should not equal KSort{Level: 1}")
	}
	if s1.Equal(s0a) {
		t.Error("KSort{Level: 1} should not equal KSort{Level: 0}")
	}
	if !s0a.Equal(KSort{}) {
		t.Error("KSort zero-value should equal itself")
	}
}

func TestKSortLevelString(t *testing.T) {
	if s := (KSort{}).String(); s != "Kind" {
		t.Errorf("KSort{} should be Kind, got %s", s)
	}
	if s := (KSort{Level: 0}).String(); s != "Kind" {
		t.Errorf("KSort{Level: 0} should be Kind, got %s", s)
	}
	if s := (KSort{Level: 1}).String(); s != "Sort2" {
		t.Errorf("KSort{Level: 1} should be Sort2, got %s", s)
	}
	if s := (KSort{Level: 2}).String(); s != "Sort3" {
		t.Errorf("KSort{Level: 2} should be Sort3, got %s", s)
	}
}
