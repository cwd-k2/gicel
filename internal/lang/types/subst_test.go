package types

import "testing"

func TestSubstVar(t *testing.T) {
	ty := &TyVar{Name: "a"}
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	if con, ok := result.(*TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected Int, got %v", result)
	}
}

func TestSubstVarNoMatch(t *testing.T) {
	ty := &TyVar{Name: "b"}
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	if result != ty {
		t.Error("expected unchanged type when variable doesn't match")
	}
}

func TestSubstCon(t *testing.T) {
	ty := &TyCon{Name: "Bool"}
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	if result != ty {
		t.Error("TyCon should be unchanged by substitution")
	}
}

func TestSubstArrow(t *testing.T) {
	// a -> b  [a := Int]  →  Int -> b
	ty := &TyArrow{From: &TyVar{Name: "a"}, To: &TyVar{Name: "b"}}
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	arr, ok := result.(*TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow, got %T", result)
	}
	if con, ok := arr.From.(*TyCon); !ok || con.Name != "Int" {
		t.Error("From should be Int")
	}
	if v, ok := arr.To.(*TyVar); !ok || v.Name != "b" {
		t.Error("To should remain b")
	}
}

func TestSubstArrowUnchanged(t *testing.T) {
	ty := &TyArrow{From: &TyCon{Name: "Int"}, To: &TyCon{Name: "Bool"}}
	result := Subst(ty, "a", &TyCon{Name: "String"})
	if result != ty {
		t.Error("arrow with no matching vars should be pointer-equal")
	}
}

func TestSubstApp(t *testing.T) {
	// f a  [a := Int]  →  f Int
	ty := &TyApp{Fun: &TyVar{Name: "f"}, Arg: &TyVar{Name: "a"}}
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	app, ok := result.(*TyApp)
	if !ok {
		t.Fatalf("expected TyApp, got %T", result)
	}
	if con, ok := app.Arg.(*TyCon); !ok || con.Name != "Int" {
		t.Error("Arg should be Int")
	}
}

func TestSubstForallShadow(t *testing.T) {
	// forall a. a  [a := Int]  →  forall a. a  (shadowed)
	ty := &TyForall{Var: "a", Kind: KType{}, Body: &TyVar{Name: "a"}}
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	if result != ty {
		t.Error("substitution should be blocked by shadowing forall")
	}
}

func TestSubstForallNoCapture(t *testing.T) {
	// forall a. (a -> b)  [b := a]  →  forall a$N. (a$N -> a)
	// Must rename bound var to avoid capturing the free 'a' in replacement.
	ResetFreshCounter()
	ty := &TyForall{
		Var:  "a",
		Kind: KType{},
		Body: &TyArrow{From: &TyVar{Name: "a"}, To: &TyVar{Name: "b"}},
	}
	result := Subst(ty, "b", &TyVar{Name: "a"})
	forall, ok := result.(*TyForall)
	if !ok {
		t.Fatalf("expected TyForall, got %T", result)
	}
	if forall.Var == "a" {
		t.Error("bound variable should have been renamed to avoid capture")
	}
	// The body's To should now be the free 'a' (the replacement).
	arr, ok := forall.Body.(*TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow body, got %T", forall.Body)
	}
	if v, ok := arr.To.(*TyVar); !ok || v.Name != "a" {
		t.Errorf("To should be free var 'a', got %v", arr.To)
	}
}

func TestSubstComp(t *testing.T) {
	ty := MkComp(&TyVar{Name: "r1"}, &TyVar{Name: "r2"}, &TyVar{Name: "a"})
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	comp, ok := result.(*TyCBPV)
	if !ok {
		t.Fatalf("expected TyCBPV, got %T", result)
	}
	if con, ok := comp.Result.(*TyCon); !ok || con.Name != "Int" {
		t.Error("Result should be Int")
	}
}

func TestSubstFamilyApp(t *testing.T) {
	ty := &TyFamilyApp{
		Name: "F",
		Args: []Type{&TyVar{Name: "a"}, &TyCon{Name: "Int"}},
		Kind: KType{},
	}
	result := Subst(ty, "a", &TyCon{Name: "Bool"})
	fam, ok := result.(*TyFamilyApp)
	if !ok {
		t.Fatalf("expected TyFamilyApp, got %T", result)
	}
	if con, ok := fam.Args[0].(*TyCon); !ok || con.Name != "Bool" {
		t.Error("first arg should be Bool")
	}
	if con, ok := fam.Args[1].(*TyCon); !ok || con.Name != "Int" {
		t.Error("second arg should remain Int")
	}
}

func TestSubstMeta(t *testing.T) {
	ty := &TyMeta{ID: 42, Kind: KType{}}
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	if result != ty {
		t.Error("TyMeta should be unchanged by substitution")
	}
}

func TestSubstSkolem(t *testing.T) {
	ty := &TySkolem{ID: 1, Name: "sk", Kind: KType{}}
	result := Subst(ty, "a", &TyCon{Name: "Int"})
	if result != ty {
		t.Error("TySkolem should be unchanged by substitution")
	}
}

func TestSubstEvidenceRow(t *testing.T) {
	row := &TyEvidenceRow{
		Entries: &CapabilityEntries{
			Fields: []RowField{
				{Label: "x", Type: &TyVar{Name: "a"}},
			},
		},
	}
	result := Subst(row, "a", &TyCon{Name: "Int"})
	r, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := r.Entries.(*CapabilityEntries).Fields
	if con, ok := fields[0].Type.(*TyCon); !ok || con.Name != "Int" {
		t.Error("field type should be Int")
	}
}

func TestSubstEvidenceRowTail(t *testing.T) {
	row := &TyEvidenceRow{
		Entries: &CapabilityEntries{},
		Tail:    &TyVar{Name: "r"},
	}
	result := Subst(row, "r", &TyVar{Name: "s"})
	r := result.(*TyEvidenceRow)
	if v, ok := r.Tail.(*TyVar); !ok || v.Name != "s" {
		t.Errorf("tail should be 's', got %v", r.Tail)
	}
}

// --- SubstMany ---

func TestSubstManySimple(t *testing.T) {
	ty := &TyArrow{From: &TyVar{Name: "a"}, To: &TyVar{Name: "b"}}
	result := SubstMany(ty, map[string]Type{
		"a": &TyCon{Name: "Int"},
		"b": &TyCon{Name: "Bool"},
	})
	arr := result.(*TyArrow)
	if con, ok := arr.From.(*TyCon); !ok || con.Name != "Int" {
		t.Error("From should be Int")
	}
	if con, ok := arr.To.(*TyCon); !ok || con.Name != "Bool" {
		t.Error("To should be Bool")
	}
}

func TestSubstManyEmpty(t *testing.T) {
	ty := &TyVar{Name: "a"}
	result := SubstMany(ty, map[string]Type{})
	if result != ty {
		t.Error("empty substitution should return same type")
	}
}

func TestSubstManyNoInterference(t *testing.T) {
	// Simultaneous: [a := b, b := a] should swap, not collapse.
	ty := &TyArrow{From: &TyVar{Name: "a"}, To: &TyVar{Name: "b"}}
	result := SubstMany(ty, map[string]Type{
		"a": &TyVar{Name: "b"},
		"b": &TyVar{Name: "a"},
	})
	arr := result.(*TyArrow)
	if v, ok := arr.From.(*TyVar); !ok || v.Name != "b" {
		t.Errorf("From should be b (swapped), got %v", arr.From)
	}
	if v, ok := arr.To.(*TyVar); !ok || v.Name != "a" {
		t.Errorf("To should be a (swapped), got %v", arr.To)
	}
}

func TestSubstManyShadowing(t *testing.T) {
	// forall a. (a -> b)  [a := Int, b := Bool]
	// a is shadowed, only b should be substituted.
	ty := &TyForall{
		Var:  "a",
		Kind: KType{},
		Body: &TyArrow{From: &TyVar{Name: "a"}, To: &TyVar{Name: "b"}},
	}
	result := SubstMany(ty, map[string]Type{
		"a": &TyCon{Name: "Int"},
		"b": &TyCon{Name: "Bool"},
	})
	forall := result.(*TyForall)
	arr := forall.Body.(*TyArrow)
	if _, ok := arr.From.(*TyVar); !ok {
		t.Error("From should still be var a (shadowed)")
	}
	if con, ok := arr.To.(*TyCon); !ok || con.Name != "Bool" {
		t.Error("To should be Bool")
	}
}

// --- SubstKindInType ---

func TestSubstKindInTypeForallKind(t *testing.T) {
	ty := &TyForall{
		Var:  "a",
		Kind: KVar{Name: "k"},
		Body: &TyVar{Name: "a"},
	}
	result := SubstKindInType(ty, "k", KType{})
	forall := result.(*TyForall)
	if _, ok := forall.Kind.(KType); !ok {
		t.Errorf("kind should be KType, got %v", forall.Kind)
	}
}

func TestSubstKindInTypeMetaKind(t *testing.T) {
	ty := &TyMeta{ID: 1, Kind: KVar{Name: "k"}}
	result := SubstKindInType(ty, "k", KType{})
	meta := result.(*TyMeta)
	if _, ok := meta.Kind.(KType); !ok {
		t.Errorf("kind should be KType, got %v", meta.Kind)
	}
}

// --- ResetFreshCounter ---

func TestResetFreshCounter(t *testing.T) {
	ResetFreshCounter()
	n1 := freshName("x")
	n2 := freshName("x")
	ResetFreshCounter()
	n3 := freshName("x")
	if n1 != n3 {
		t.Errorf("after reset, first name should repeat: %s vs %s", n1, n3)
	}
	if n1 == n2 {
		t.Error("sequential names should differ")
	}
}
