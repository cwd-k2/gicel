package types

import (
	"testing"
)

// --- Kind ---

func TestKindEquality(t *testing.T) {
	kt := TypeOfTypes
	kr := TypeOfRows
	if !testOps.Equal(kt, TypeOfTypes) {
		t.Error("Type != Type")
	}
	if testOps.Equal(kt, kr) {
		t.Error("Type == Row")
	}
	arrow1 := &TyArrow{From: kt, To: kr}
	arrow2 := &TyArrow{From: kt, To: kr}
	if !testOps.Equal(arrow1, arrow2) {
		t.Error("Arrow(Type,Row) != Arrow(Type,Row)")
	}
}

func TestKindArity(t *testing.T) {
	kt := TypeOfTypes
	if Arity(kt) != 0 {
		t.Error("KType arity should be 0")
	}
	if Arity(KindOfComputation) != 4 {
		t.Errorf("Computation kind arity should be 4, got %d", Arity(KindOfComputation))
	}
}

func TestKindString(t *testing.T) {
	kt := TypeOfTypes
	kr := TypeOfRows
	if kt.Name != "Type" {
		t.Error("Type name mismatch")
	}
	if kr.Name != "Row" {
		t.Error("Row name mismatch")
	}
	got := testOps.PrettyTypeAsKind(KindOfComputation)
	if got != "Type -> Row -> Row -> Type -> Type" {
		t.Errorf("Computation kind: got %q", got)
	}
}

// --- Row ---

func TestRowNormalize(t *testing.T) {
	r := &TyEvidenceRow{Entries: &CapabilityEntries{Fields: []RowField{
		{Label: "c", Type: testOps.Con("C")},
		{Label: "a", Type: testOps.Con("A")},
		{Label: "b", Type: testOps.Con("B")},
	}}}
	n, err := NormalizeRow(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fields := n.CapFields()
	if fields[0].Label != "a" || fields[1].Label != "b" || fields[2].Label != "c" {
		t.Error("normalization should sort by label")
	}
}

func TestRowNormalizeErrorsOnDuplicate(t *testing.T) {
	r := &TyEvidenceRow{Entries: &CapabilityEntries{Fields: []RowField{
		{Label: "a", Type: testOps.Con("A")},
		{Label: "a", Type: testOps.Con("B")},
	}}}
	_, err := NormalizeRow(r)
	if err == nil {
		t.Error("expected error on duplicate label")
	}
}

// --- Equality ---

func TestEqualSimple(t *testing.T) {
	if !testOps.Equal(testOps.Con("Int"), testOps.Con("Int")) {
		t.Error("Int != Int")
	}
	if testOps.Equal(testOps.Con("Int"), testOps.Con("String")) {
		t.Error("Int == String")
	}
	arrow1 := testOps.Arrow(testOps.Con("Int"), testOps.Con("Bool"))
	arrow2 := testOps.Arrow(testOps.Con("Int"), testOps.Con("Bool"))
	if !testOps.Equal(arrow1, arrow2) {
		t.Error("Int -> Bool should be equal")
	}
}

func TestEqualAlpha(t *testing.T) {
	// forall a. a -> a  ==  forall b. b -> b
	t1 := testOps.Forall("a", TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Var("a")))
	t2 := testOps.Forall("b", TypeOfTypes, testOps.Arrow(testOps.Var("b"), testOps.Var("b")))
	if !testOps.Equal(t1, t2) {
		t.Error("alpha-equivalent types should be equal")
	}
	// forall a. a -> a  !=  forall a. a -> Int
	t3 := testOps.Forall("a", TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Con("Int")))
	if testOps.Equal(t1, t3) {
		t.Error("different types should not be equal")
	}
	// forall a. a -> b  !=  forall b. b -> b
	// Left b is free; right b is bound. Must not be considered equal.
	t4 := testOps.Forall("a", TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Var("b")))
	t5 := testOps.Forall("b", TypeOfTypes, testOps.Arrow(testOps.Var("b"), testOps.Var("b")))
	if testOps.Equal(t4, t5) {
		t.Error("free vs bound variable collision should not be equal")
	}
}

func TestEqualRow(t *testing.T) {
	r1 := ClosedRow(
		RowField{Label: "b", Type: testOps.Con("B")},
		RowField{Label: "a", Type: testOps.Con("A")},
	)
	r2 := ClosedRow(
		RowField{Label: "a", Type: testOps.Con("A")},
		RowField{Label: "b", Type: testOps.Con("B")},
	)
	if !testOps.Equal(r1, r2) {
		t.Error("rows with same labels in different order should be equal")
	}
}

// --- Substitution ---

func TestSubstSimple(t *testing.T) {
	// a[a := Int] = Int
	ty := testOps.Var("a")
	result := testOps.Subst(ty, "a", testOps.Con("Int"))
	if !testOps.Equal(result, testOps.Con("Int")) {
		t.Error("substitution failed")
	}
}

func TestSubstShadow(t *testing.T) {
	// forall a. a[a := Int] = forall a. a (shadowed, no substitution)
	ty := testOps.Forall("a", TypeOfTypes, testOps.Var("a"))
	result := testOps.Subst(ty, "a", testOps.Con("Int"))
	if !testOps.Equal(result, ty) {
		t.Error("should not substitute under shadowing forall")
	}
}

func TestSubstCaptureAvoidance(t *testing.T) {
	// forall b. a -> b   [a := b]
	// should rename bound b to avoid capture
	ty := testOps.Forall("b", TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Var("b")))
	result := testOps.Subst(ty, "a", testOps.Var("b"))
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
	ty := testOps.Arrow(testOps.Var("a"), testOps.Var("b"))
	fv := testOps.FreeVars(ty)
	if _, ok := fv["a"]; !ok {
		t.Error("'a' should be free")
	}
	if _, ok := fv["b"]; !ok {
		t.Error("'b' should be free")
	}
}

func TestFreeVarsForall(t *testing.T) {
	// forall a. a -> b  →  FV = {b}
	ty := testOps.Forall("a", TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Var("b")))
	fv := testOps.FreeVars(ty)
	if _, ok := fv["a"]; ok {
		t.Error("'a' should be bound")
	}
	if _, ok := fv["b"]; !ok {
		t.Error("'b' should be free")
	}
}

// --- KConstraint ---

func TestKConstraintEquality(t *testing.T) {
	kc := TypeOfConstraints
	if !testOps.Equal(kc, TypeOfConstraints) {
		t.Error("Constraint != Constraint")
	}
	if testOps.Equal(kc, TypeOfTypes) {
		t.Error("Constraint == Type")
	}
	if kc.Name != "Constraint" {
		t.Errorf("expected 'Constraint', got %q", kc.Name)
	}
}

// --- Skolem ---

func TestTySkolemEquality(t *testing.T) {
	s1 := &TySkolem{ID: 1, Name: "a", Kind: TypeOfTypes}
	s2 := &TySkolem{ID: 1, Name: "a", Kind: TypeOfTypes}
	s3 := &TySkolem{ID: 2, Name: "b", Kind: TypeOfTypes}
	if !testOps.Equal(s1, s2) {
		t.Error("same-ID skolems should be equal")
	}
	if testOps.Equal(s1, s3) {
		t.Error("different-ID skolems should not be equal")
	}
}

func TestZonkSkolem(t *testing.T) {
	s := &TySkolem{ID: 99, Name: "z", Kind: TypeOfTypes}
	pretty := testOps.Pretty(s)
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
		{testOps.Con("Int"), "Int"},
		{testOps.Var("a"), "a"},
		{testOps.Arrow(testOps.Con("Int"), testOps.Con("Bool")), "Int -> Bool"},
		{testOps.Arrow(testOps.Arrow(testOps.Con("A"), testOps.Con("B")), testOps.Con("C")), "(A -> B) -> C"},
		{testOps.Forall("a", TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Var("a"))), `\a. a -> a`},
		{testOps.Forall("a", TypeOfTypes, testOps.Forall("b", TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Var("b")))),
			`\a b. a -> b`},
		{EmptyRow(), "{}"},
		{ClosedRow(RowField{Label: "x", Type: testOps.Con("Int")}), "{ x: Int }"},
		// Tuple sugar
		{&TyApp{Fun: testOps.Con(TyConRecord), Arg: EmptyRow()}, "()"},
		{&TyApp{Fun: testOps.Con(TyConRecord), Arg: ClosedRow(
			RowField{Label: "_1", Type: testOps.Con("Int")},
			RowField{Label: "_2", Type: testOps.Con("Bool")},
		)}, "(Int, Bool)"},
	}
	for _, tt := range tests {
		got := testOps.Pretty(tt.ty)
		if got != tt.want {
			t.Errorf("Pretty(%v) = %q, want %q", tt.ty, got, tt.want)
		}
	}
}

func TestEqualAlphaNestedForallShadowing(t *testing.T) {
	// forall a. forall a. a  ==  forall b. forall c. c
	// Inner 'a' shadows outer 'a'; inner 'c' shadows nothing.
	// Both reduce to "innermost bound variable" — should be equal.
	t1 := testOps.Forall("a", TypeOfTypes, testOps.Forall("a", TypeOfTypes, testOps.Var("a")))
	t2 := testOps.Forall("b", TypeOfTypes, testOps.Forall("c", TypeOfTypes, testOps.Var("c")))
	if !testOps.Equal(t1, t2) {
		t.Error("nested forall with shadowing should be alpha-equivalent")
	}
	// forall a. forall a. a  !=  forall b. forall c. b
	// Left: inner bound. Right: outer bound. Not equivalent.
	t3 := testOps.Forall("b", TypeOfTypes, testOps.Forall("c", TypeOfTypes, testOps.Var("b")))
	if testOps.Equal(t1, t3) {
		t.Error("inner-bound vs outer-bound should not be alpha-equivalent")
	}
}

func TestEqualAlphaBindingLookupDirection(t *testing.T) {
	// Verifies that binding lookup traverses from most recent (innermost) first.
	// forall a. forall b. a -> b  ==  forall x. forall y. x -> y
	t1 := testOps.Forall("a", TypeOfTypes, testOps.Forall("b", TypeOfTypes, testOps.Arrow(testOps.Var("a"), testOps.Var("b"))))
	t2 := testOps.Forall("x", TypeOfTypes, testOps.Forall("y", TypeOfTypes, testOps.Arrow(testOps.Var("x"), testOps.Var("y"))))
	if !testOps.Equal(t1, t2) {
		t.Error("nested forall with distinct names should be alpha-equivalent")
	}
	// forall a. forall b. a -> b  !=  forall x. forall y. y -> x  (swapped)
	t3 := testOps.Forall("x", TypeOfTypes, testOps.Forall("y", TypeOfTypes, testOps.Arrow(testOps.Var("y"), testOps.Var("x"))))
	if testOps.Equal(t1, t3) {
		t.Error("swapped variable usage should not be alpha-equivalent")
	}
}

func TestEvidenceRowConstraintEntriesEqualOrderIndependent(t *testing.T) {
	// TyEvidenceRow with ConstraintEntries — order should not matter for Equal.
	var eqA ConstraintEntry = &ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}
	var ordB ConstraintEntry = &ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("b")}}
	r1 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{eqA, ordB}}}
	r2 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{ordB, eqA}}}
	if !testOps.Equal(r1, r2) {
		t.Error("TyEvidenceRow with ConstraintEntries Equal should be order-independent")
	}
	// Different entries should not be equal.
	var showC ConstraintEntry = &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("c")}}
	r3 := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{eqA, showC}}}
	if testOps.Equal(r1, r3) {
		t.Error("different TyEvidenceRow ConstraintEntries should not be equal")
	}
}
