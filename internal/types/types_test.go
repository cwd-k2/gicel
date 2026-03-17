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
	r := &TyEvidenceRow{Entries: &CapabilityEntries{Fields: []RowField{
		{Label: "c", Type: Con("C")},
		{Label: "a", Type: Con("A")},
		{Label: "b", Type: Con("B")},
	}}}
	n := NormalizeRow(r)
	fields := n.CapFields()
	if fields[0].Label != "a" || fields[1].Label != "b" || fields[2].Label != "c" {
		t.Error("normalization should sort by label")
	}
}

func TestRowNormalizePanicsOnDuplicate(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate label")
		}
	}()
	r := &TyEvidenceRow{Entries: &CapabilityEntries{Fields: []RowField{
		{Label: "a", Type: Con("A")},
		{Label: "a", Type: Con("B")},
	}}}
	NormalizeRow(r)
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
	// forall a. a -> b  !=  forall b. b -> b
	// Left b is free; right b is bound. Must not be considered equal.
	t4 := MkForall("a", KType{}, MkArrow(Var("a"), Var("b")))
	t5 := MkForall("b", KType{}, MkArrow(Var("b"), Var("b")))
	if Equal(t4, t5) {
		t.Error("free vs bound variable collision should not be equal")
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

// --- KConstraint ---

func TestKConstraintEquality(t *testing.T) {
	kc := KConstraint{}
	if !kc.Equal(KConstraint{}) {
		t.Error("KConstraint != KConstraint")
	}
	if kc.Equal(KType{}) {
		t.Error("KConstraint == KType")
	}
	if kc.String() != "Constraint" {
		t.Errorf("expected 'Constraint', got %q", kc.String())
	}
}

// --- Skolem ---

func TestTySkolemEquality(t *testing.T) {
	s1 := &TySkolem{ID: 1, Name: "a", Kind: KType{}}
	s2 := &TySkolem{ID: 1, Name: "a", Kind: KType{}}
	s3 := &TySkolem{ID: 2, Name: "b", Kind: KType{}}
	if !Equal(s1, s2) {
		t.Error("same-ID skolems should be equal")
	}
	if Equal(s1, s3) {
		t.Error("different-ID skolems should not be equal")
	}
}

func TestZonkSkolem(t *testing.T) {
	s := &TySkolem{ID: 99, Name: "z", Kind: KType{}}
	pretty := Pretty(s)
	if pretty != "#z" {
		t.Errorf("expected #z, got %s", pretty)
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
		{MkForall("a", KType{}, MkArrow(Var("a"), Var("a"))), `\a. a -> a`},
		{MkForall("a", KType{}, MkForall("b", KType{}, MkArrow(Var("a"), Var("b")))),
			`\a b. a -> b`},
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

func TestEqualAlphaNestedForallShadowing(t *testing.T) {
	// forall a. forall a. a  ==  forall b. forall c. c
	// Inner 'a' shadows outer 'a'; inner 'c' shadows nothing.
	// Both reduce to "innermost bound variable" — should be equal.
	t1 := MkForall("a", KType{}, MkForall("a", KType{}, Var("a")))
	t2 := MkForall("b", KType{}, MkForall("c", KType{}, Var("c")))
	if !Equal(t1, t2) {
		t.Error("nested forall with shadowing should be alpha-equivalent")
	}
	// forall a. forall a. a  !=  forall b. forall c. b
	// Left: inner bound. Right: outer bound. Not equivalent.
	t3 := MkForall("b", KType{}, MkForall("c", KType{}, Var("b")))
	if Equal(t1, t3) {
		t.Error("inner-bound vs outer-bound should not be alpha-equivalent")
	}
}

func TestEqualAlphaBindingLookupDirection(t *testing.T) {
	// Verifies that binding lookup traverses from most recent (innermost) first.
	// forall a. forall b. a -> b  ==  forall x. forall y. x -> y
	t1 := MkForall("a", KType{}, MkForall("b", KType{}, MkArrow(Var("a"), Var("b"))))
	t2 := MkForall("x", KType{}, MkForall("y", KType{}, MkArrow(Var("x"), Var("y"))))
	if !Equal(t1, t2) {
		t.Error("nested forall with distinct names should be alpha-equivalent")
	}
	// forall a. forall b. a -> b  !=  forall x. forall y. y -> x  (swapped)
	t3 := MkForall("x", KType{}, MkForall("y", KType{}, MkArrow(Var("y"), Var("x"))))
	if Equal(t1, t3) {
		t.Error("swapped variable usage should not be alpha-equivalent")
	}
}

func TestEvidenceRowConstraintEntriesEqualOrderIndependent(t *testing.T) {
	// TyEvidenceRow with ConstraintEntries — order should not matter for Equal.
	eqA := ConstraintEntry{ClassName: "Eq", Args: []Type{Var("a")}}
	ordB := ConstraintEntry{ClassName: "Ord", Args: []Type{Var("b")}}
	r1 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{eqA, ordB}}}
	r2 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{ordB, eqA}}}
	if !Equal(r1, r2) {
		t.Error("TyEvidenceRow with ConstraintEntries Equal should be order-independent")
	}
	// Different entries should not be equal.
	showC := ConstraintEntry{ClassName: "Show", Args: []Type{Var("c")}}
	r3 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{eqA, showC}}}
	if Equal(r1, r3) {
		t.Error("different TyEvidenceRow ConstraintEntries should not be equal")
	}
}

func TestConstraintRowEqualOrderIndependent(t *testing.T) {
	eqA := ConstraintEntry{ClassName: "Eq", Args: []Type{Var("a")}}
	ordB := ConstraintEntry{ClassName: "Ord", Args: []Type{Var("b")}}
	// Same entries, different order.
	r1 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{eqA, ordB}}}
	r2 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{ordB, eqA}}}
	if !Equal(r1, r2) {
		t.Error("TyEvidenceRow with ConstraintEntries Equal should be order-independent")
	}
	// Different entries should not be equal.
	showC := ConstraintEntry{ClassName: "Show", Args: []Type{Var("c")}}
	r3 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{eqA, showC}}}
	if Equal(r1, r3) {
		t.Error("different TyEvidenceRow ConstraintEntries should not be equal")
	}
}
