package types

import "testing"

func TestLevelLitEqual(t *testing.T) {
	if !LevelEqual(L0, L0) {
		t.Error("L0 should equal L0")
	}
	if !LevelEqual(L1, L1) {
		t.Error("L1 should equal L1")
	}
	if LevelEqual(L0, L1) {
		t.Error("L0 should not equal L1")
	}
	if LevelEqual(L1, L2) {
		t.Error("L1 should not equal L2")
	}
}

func TestLevelNilEqualsL0(t *testing.T) {
	if !LevelEqual(nil, L0) {
		t.Error("nil should equal L0")
	}
	if !LevelEqual(L0, nil) {
		t.Error("L0 should equal nil")
	}
	if !LevelEqual(nil, nil) {
		t.Error("nil should equal nil")
	}
	if LevelEqual(nil, L1) {
		t.Error("nil should not equal L1")
	}
}

func TestLevelString(t *testing.T) {
	if L0.LevelString() != "0" {
		t.Errorf("L0 string: got %q", L0.LevelString())
	}
	if L1.LevelString() != "1" {
		t.Errorf("L1 string: got %q", L1.LevelString())
	}
	if L2.LevelString() != "2" {
		t.Errorf("L2 string: got %q", L2.LevelString())
	}
}

func TestLevelMetaEqual(t *testing.T) {
	m1 := &LevelMeta{ID: 1}
	m2 := &LevelMeta{ID: 1}
	m3 := &LevelMeta{ID: 2}
	if !LevelEqual(m1, m2) {
		t.Error("same ID metas should be equal")
	}
	if LevelEqual(m1, m3) {
		t.Error("different ID metas should not be equal")
	}
	if LevelEqual(m1, L0) {
		t.Error("LevelMeta should not equal LevelLit")
	}
}

func TestLevelVarEqual(t *testing.T) {
	v1 := &LevelVar{Name: "l"}
	v2 := &LevelVar{Name: "l"}
	v3 := &LevelVar{Name: "k"}
	if !LevelEqual(v1, v2) {
		t.Error("same name vars should be equal")
	}
	if LevelEqual(v1, v3) {
		t.Error("different name vars should not be equal")
	}
}

func TestLevelMaxEqual(t *testing.T) {
	m1 := &LevelMax{A: L0, B: L1}
	m2 := &LevelMax{A: L0, B: L1}
	m3 := &LevelMax{A: L1, B: L0}
	if !LevelEqual(m1, m2) {
		t.Error("structurally equal max should be equal")
	}
	if LevelEqual(m1, m3) {
		t.Error("structurally different max should not be equal (no commutativity)")
	}
}

func TestLevelSuccEqual(t *testing.T) {
	s1 := &LevelSucc{E: L0}
	s2 := &LevelSucc{E: L0}
	s3 := &LevelSucc{E: L1}
	if !LevelEqual(s1, s2) {
		t.Error("same succ should be equal")
	}
	if LevelEqual(s1, s3) {
		t.Error("different succ should not be equal")
	}
}

func TestIsValueLevel(t *testing.T) {
	if !IsValueLevel(nil) {
		t.Error("nil should be value level")
	}
	if !IsValueLevel(L0) {
		t.Error("L0 should be value level")
	}
	if IsValueLevel(L1) {
		t.Error("L1 should not be value level")
	}
}

func TestIsKindLevel(t *testing.T) {
	if !IsKindLevel(L1) {
		t.Error("L1 should be kind level")
	}
	if IsKindLevel(L0) {
		t.Error("L0 should not be kind level")
	}
	if IsKindLevel(nil) {
		t.Error("nil should not be kind level")
	}
}
