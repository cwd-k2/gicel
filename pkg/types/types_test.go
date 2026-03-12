package types

import (
	"testing"
)

// --- Kind ---

func TestKindEquality(t *testing.T) {
	kt := KType{}
	kr := KRow{}
	if !kt.Equal(KType{}) {
		t.Error("KType != KType")
	}
	if kt.Equal(kr) {
		t.Error("KType == KRow")
	}
	arrow1 := &KArrow{kt, kr}
	arrow2 := &KArrow{kt, kr}
	if !arrow1.Equal(arrow2) {
		t.Error("KArrow(Type,Row) != KArrow(Type,Row)")
	}
}

func TestKindArity(t *testing.T) {
	kt := KType{}
	if Arity(kt) != 0 {
		t.Error("KType arity should be 0")
	}
	if Arity(KindOfComputation) != 3 {
		t.Errorf("Computation kind arity should be 3, got %d", Arity(KindOfComputation))
	}
}

func TestKindString(t *testing.T) {
	kt := KType{}
	kr := KRow{}
	if kt.String() != "Type" {
		t.Error("KType string")
	}
	if kr.String() != "Row" {
		t.Error("KRow string")
	}
	got := KindOfComputation.String()
	if got != "Row -> Row -> Type -> Type" {
		t.Errorf("Computation kind: got %q", got)
	}
}

// --- Row ---

func TestRowNormalize(t *testing.T) {
	r := &TyRow{Fields: []RowField{
		{Label: "c", Type: Con("C")},
		{Label: "a", Type: Con("A")},
		{Label: "b", Type: Con("B")},
	}}
	n := Normalize(r)
	if n.Fields[0].Label != "a" || n.Fields[1].Label != "b" || n.Fields[2].Label != "c" {
		t.Error("normalization should sort by label")
	}
}

func TestRowNormalizePanicsOnDuplicate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate label")
		}
	}()
	r := &TyRow{Fields: []RowField{
		{Label: "a", Type: Con("A")},
		{Label: "a", Type: Con("B")},
	}}
	Normalize(r)
}

func TestExtendRow(t *testing.T) {
	r := EmptyRow()
	r2, err := ExtendRow(r, RowField{Label: "x", Type: Con("Int")})
	if err != nil {
		t.Fatal(err)
	}
	r3, err := ExtendRow(r2, RowField{Label: "a", Type: Con("Bool")})
	if err != nil {
		t.Fatal(err)
	}
	if r3.Fields[0].Label != "a" || r3.Fields[1].Label != "x" {
		t.Error("fields should be sorted after extend")
	}
	_, err = ExtendRow(r3, RowField{Label: "x", Type: Con("String")})
	if err == nil {
		t.Error("expected error on duplicate label")
	}
}

func TestRemoveLabel(t *testing.T) {
	r := ClosedRow(
		RowField{Label: "a", Type: Con("A")},
		RowField{Label: "b", Type: Con("B")},
	)
	f, rest, ok := RemoveLabel(r, "a")
	if !ok || f.Label != "a" {
		t.Error("should find 'a'")
	}
	if len(rest.Fields) != 1 || rest.Fields[0].Label != "b" {
		t.Error("remaining should have only 'b'")
	}
	_, _, ok = RemoveLabel(r, "z")
	if ok {
		t.Error("should not find 'z'")
	}
}

// --- Equality ---

func TestEqualSimple(t *testing.T) {
	if !Equal(Con("Int"), Con("Int")) {
		t.Error("Int != Int")
	}
	if Equal(Con("Int"), Con("String")) {
		t.Error("Int == String")
	}
	arrow1 := MkArrow(Con("Int"), Con("Bool"))
	arrow2 := MkArrow(Con("Int"), Con("Bool"))
	if !Equal(arrow1, arrow2) {
		t.Error("Int -> Bool should be equal")
	}
}

func TestEqualAlpha(t *testing.T) {
	// forall a. a -> a  ==  forall b. b -> b
	t1 := MkForall("a", KType{}, MkArrow(Var("a"), Var("a")))
	t2 := MkForall("b", KType{}, MkArrow(Var("b"), Var("b")))
	if !Equal(t1, t2) {
		t.Error("alpha-equivalent types should be equal")
	}
	// forall a. a -> a  !=  forall a. a -> Int
	t3 := MkForall("a", KType{}, MkArrow(Var("a"), Con("Int")))
	if Equal(t1, t3) {
		t.Error("different types should not be equal")
	}
}

func TestEqualRow(t *testing.T) {
	r1 := ClosedRow(
		RowField{Label: "b", Type: Con("B")},
		RowField{Label: "a", Type: Con("A")},
	)
	r2 := ClosedRow(
		RowField{Label: "a", Type: Con("A")},
		RowField{Label: "b", Type: Con("B")},
	)
	if !Equal(r1, r2) {
		t.Error("rows with same labels in different order should be equal")
	}
}

// --- Substitution ---

func TestSubstSimple(t *testing.T) {
	// a[a := Int] = Int
	ty := Var("a")
	result := Subst(ty, "a", Con("Int"))
	if !Equal(result, Con("Int")) {
		t.Error("substitution failed")
	}
}

func TestSubstShadow(t *testing.T) {
	// forall a. a[a := Int] = forall a. a (shadowed, no substitution)
	ty := MkForall("a", KType{}, Var("a"))
	result := Subst(ty, "a", Con("Int"))
	if !Equal(result, ty) {
		t.Error("should not substitute under shadowing forall")
	}
}

func TestSubstCaptureAvoidance(t *testing.T) {
	// forall b. a -> b   [a := b]
	// should rename bound b to avoid capture
	ty := MkForall("b", KType{}, MkArrow(Var("a"), Var("b")))
	result := Subst(ty, "a", Var("b"))
	// The result should be forall b'. b -> b' (not forall b. b -> b)
	f, ok := result.(*TyForall)
	if !ok {
		t.Fatal("expected TyForall")
	}
	if f.Var == "b" {
		t.Error("should have renamed bound variable to avoid capture")
	}
}

// --- Free Variables ---

func TestFreeVars(t *testing.T) {
	ty := MkArrow(Var("a"), Var("b"))
	fv := FreeVars(ty)
	if _, ok := fv["a"]; !ok {
		t.Error("'a' should be free")
	}
	if _, ok := fv["b"]; !ok {
		t.Error("'b' should be free")
	}
}

func TestFreeVarsForall(t *testing.T) {
	// forall a. a -> b  →  FV = {b}
	ty := MkForall("a", KType{}, MkArrow(Var("a"), Var("b")))
	fv := FreeVars(ty)
	if _, ok := fv["a"]; ok {
		t.Error("'a' should be bound")
	}
	if _, ok := fv["b"]; !ok {
		t.Error("'b' should be free")
	}
}

// --- Pretty Printing ---

func TestPretty(t *testing.T) {
	tests := []struct {
		ty   Type
		want string
	}{
		{Con("Int"), "Int"},
		{Var("a"), "a"},
		{MkArrow(Con("Int"), Con("Bool")), "Int -> Bool"},
		{MkArrow(MkArrow(Con("A"), Con("B")), Con("C")), "(A -> B) -> C"},
		{MkForall("a", KType{}, MkArrow(Var("a"), Var("a"))), "forall a. a -> a"},
		{MkForall("a", KType{}, MkForall("b", KType{}, MkArrow(Var("a"), Var("b")))),
			"forall a b. a -> b"},
		{EmptyRow(), "{}"},
		{ClosedRow(RowField{Label: "x", Type: Con("Int")}), "{ x : Int }"},
	}
	for _, tt := range tests {
		got := Pretty(tt.ty)
		if got != tt.want {
			t.Errorf("Pretty(%v) = %q, want %q", tt.ty, got, tt.want)
		}
	}
}
