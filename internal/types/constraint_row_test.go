package types

import (
	"testing"
)

// =============================================================================
// TyConstraintRow — type node interface
// =============================================================================

func TestConstraintRowTypeNode(t *testing.T) {
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Var("a")}},
		}},
	}
	// Must satisfy Type interface.
	var _ Type = cr
	_ = cr.Span()
	_ = cr.Children()
}

func TestConstraintRowChildren(t *testing.T) {
	a := Var("a")
	b := Var("b")
	tail := Var("c")
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{a}},
			{ClassName: "Ord", Args: []Type{b}},
		}},
		Tail: tail,
	}
	ch := cr.Children()
	// Should include all Args from all entries + Tail.
	// Eq(a) + Ord(b) + tail = 3
	if len(ch) != 3 {
		t.Fatalf("expected 3 children, got %d", len(ch))
	}
	if ch[0] != a {
		t.Error("first child should be Eq's arg 'a'")
	}
	if ch[1] != b {
		t.Error("second child should be Ord's arg 'b'")
	}
	if ch[2] != tail {
		t.Error("third child should be tail")
	}
}

func TestConstraintRowChildrenNoTail(t *testing.T) {
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Var("a")}},
		}},
	}
	ch := cr.Children()
	if len(ch) != 1 {
		t.Fatalf("expected 1 child, got %d", len(ch))
	}
}

func TestConstraintRowChildrenMultiArgs(t *testing.T) {
	// A constraint with multiple args: Functor f a
	f := Var("f")
	a := Var("a")
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Functor", Args: []Type{f, a}},
		}},
	}
	ch := cr.Children()
	if len(ch) != 2 {
		t.Fatalf("expected 2 children, got %d", len(ch))
	}
}

// =============================================================================
// TyEvidence — type node interface
// =============================================================================

func TestEvidenceTypeNode(t *testing.T) {
	ev := &TyEvidence{
		Constraints: &TyEvidenceRow{
			Entries: &ConstraintEntries{Entries: []ConstraintEntry{
				{ClassName: "Eq", Args: []Type{Var("a")}},
			}},
		},
		Body: MkArrow(Var("a"), Con("Bool")),
	}
	var _ Type = ev
	_ = ev.Span()
	_ = ev.Children()
}

func TestEvidenceChildren(t *testing.T) {
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Var("a")}},
		}},
	}
	body := MkArrow(Var("a"), Con("Bool"))
	ev := &TyEvidence{Constraints: cr, Body: body}
	ch := ev.Children()
	// Constraints + Body = 2
	if len(ch) != 2 {
		t.Fatalf("expected 2 children, got %d", len(ch))
	}
	if ch[0] != cr {
		t.Error("first child should be Constraints")
	}
	if ch[1] != body {
		t.Error("second child should be Body")
	}
}

// =============================================================================
// Equality
// =============================================================================

func TestConstraintRowEqual(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Con("Int")}},
		}},
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Con("Int")}},
		}},
	}
	if !Equal(cr1, cr2) {
		t.Error("identical constraint rows should be equal")
	}
}

func TestConstraintRowNotEqualDifferentClass(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Con("Int")}},
		}},
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Ord", Args: []Type{Con("Int")}},
		}},
	}
	if Equal(cr1, cr2) {
		t.Error("different class names should not be equal")
	}
}

func TestConstraintRowNotEqualDifferentArgs(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Con("Int")}},
		}},
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Con("Bool")}},
		}},
	}
	if Equal(cr1, cr2) {
		t.Error("different args should not be equal")
	}
}

func TestConstraintRowEqualMultiEntry(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Var("a")}},
			{ClassName: "Ord", Args: []Type{Var("a")}},
		}},
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Var("a")}},
			{ClassName: "Ord", Args: []Type{Var("a")}},
		}},
	}
	if !Equal(cr1, cr2) {
		t.Error("identical multi-entry constraint rows should be equal")
	}
}

func TestConstraintRowEqualWithTail(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{{ClassName: "Eq", Args: []Type{Var("a")}}}},
		Tail:    Var("c"),
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{{ClassName: "Eq", Args: []Type{Var("a")}}}},
		Tail:    Var("c"),
	}
	if !Equal(cr1, cr2) {
		t.Error("constraint rows with same tail should be equal")
	}
}

func TestConstraintRowNotEqualTailMismatch(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{{ClassName: "Eq", Args: []Type{Var("a")}}}},
		Tail:    Var("c"),
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{{ClassName: "Eq", Args: []Type{Var("a")}}}},
	}
	if Equal(cr1, cr2) {
		t.Error("open vs closed tail should not be equal")
	}
}

func TestConstraintRowAlphaEquivalence(t *testing.T) {
	// forall a. { Eq a } => a  ==  forall b. { Eq b } => b
	t1 := MkForall("a", KType{},
		&TyEvidence{
			Constraints: SingleConstraint("Eq", []Type{Var("a")}),
			Body:        Var("a"),
		})
	t2 := MkForall("b", KType{},
		&TyEvidence{
			Constraints: SingleConstraint("Eq", []Type{Var("b")}),
			Body:        Var("b"),
		})
	if !Equal(t1, t2) {
		t.Error("alpha-equivalent evidence types should be equal")
	}
}

func TestEvidenceEqual(t *testing.T) {
	ev1 := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{Var("a")}),
		Body:        MkArrow(Var("a"), Con("Bool")),
	}
	ev2 := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{Var("a")}),
		Body:        MkArrow(Var("a"), Con("Bool")),
	}
	if !Equal(ev1, ev2) {
		t.Error("identical evidence types should be equal")
	}
}

func TestEvidenceNotEqual(t *testing.T) {
	ev1 := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{Var("a")}),
		Body:        MkArrow(Var("a"), Con("Bool")),
	}
	ev2 := &TyEvidence{
		Constraints: SingleConstraint("Ord", []Type{Var("a")}),
		Body:        MkArrow(Var("a"), Con("Bool")),
	}
	if Equal(ev1, ev2) {
		t.Error("different constraints should not be equal")
	}
}

func TestEvidenceNotEqualToArrow(t *testing.T) {
	ev := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{Var("a")}),
		Body:        MkArrow(Var("a"), Con("Bool")),
	}
	arr := MkArrow(Var("a"), Con("Bool"))
	if Equal(ev, arr) {
		t.Error("TyEvidence should not equal TyArrow")
	}
}

// =============================================================================
// Pretty
// =============================================================================

func TestConstraintRowPretty(t *testing.T) {
	tests := []struct {
		name string
		cr   *TyEvidenceRow
		want string
	}{
		{
			"single",
			SingleConstraint("Eq", []Type{Var("a")}),
			"{ Eq a }",
		},
		{
			"multiple",
			&TyEvidenceRow{
				Entries: &ConstraintEntries{Entries: []ConstraintEntry{
					{ClassName: "Eq", Args: []Type{Var("a")}},
					{ClassName: "Ord", Args: []Type{Var("a")}},
				}},
			},
			"{ Eq a, Ord a }",
		},
		{
			"with tail",
			&TyEvidenceRow{
				Entries: &ConstraintEntries{Entries: []ConstraintEntry{
					{ClassName: "Eq", Args: []Type{Var("a")}},
				}},
				Tail: Var("c"),
			},
			"{ Eq a | c }",
		},
		{
			"empty closed",
			EmptyConstraintRow(),
			"{}",
		},
		{
			"multi-arg constraint",
			SingleConstraint("Functor", []Type{Var("f"), Var("a")}),
			"{ Functor f a }",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pretty(tt.cr)
			if got != tt.want {
				t.Errorf("Pretty = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEvidencePretty(t *testing.T) {
	tests := []struct {
		name string
		ev   *TyEvidence
		want string
	}{
		{
			"single constraint",
			&TyEvidence{
				Constraints: SingleConstraint("Eq", []Type{Var("a")}),
				Body:        MkArrow(Var("a"), Con("Bool")),
			},
			"{ Eq a } => a -> Bool",
		},
		{
			"multi constraint",
			&TyEvidence{
				Constraints: &TyEvidenceRow{
					Entries: &ConstraintEntries{Entries: []ConstraintEntry{
						{ClassName: "Eq", Args: []Type{Var("a")}},
						{ClassName: "Ord", Args: []Type{Var("a")}},
					}},
				},
				Body: MkArrow(Var("a"), Con("Bool")),
			},
			"{ Eq a, Ord a } => a -> Bool",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pretty(tt.ev)
			if got != tt.want {
				t.Errorf("Pretty = %q, want %q", got, tt.want)
			}
		})
	}
}

// =============================================================================
// Subst
// =============================================================================

func TestConstraintRowSubst(t *testing.T) {
	// { Eq a }[a := Int] = { Eq Int }
	cr := SingleConstraint("Eq", []Type{Var("a")})
	result := Subst(cr, "a", Con("Int"))
	rc, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	if !Equal(rc.ConEntries()[0].Args[0], Con("Int")) {
		t.Errorf("expected Eq Int, got Eq %s", Pretty(rc.ConEntries()[0].Args[0]))
	}
}

func TestConstraintRowSubstTail(t *testing.T) {
	// { Eq a | c }[c := { Ord b }] — tail substitution
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{{ClassName: "Eq", Args: []Type{Var("a")}}}},
		Tail:    Var("c"),
	}
	replacement := SingleConstraint("Ord", []Type{Var("b")})
	result := Subst(cr, "c", replacement)
	rc, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	if !Equal(rc.Tail, replacement) {
		t.Errorf("tail should be replaced, got %s", Pretty(rc.Tail))
	}
}

func TestConstraintRowSubstIdentity(t *testing.T) {
	// Substituting a non-occurring variable returns same pointer.
	cr := SingleConstraint("Eq", []Type{Var("a")})
	result := Subst(cr, "z", Con("Int"))
	if result != cr {
		t.Error("subst of non-occurring variable should return same pointer")
	}
}

func TestEvidenceSubst(t *testing.T) {
	// { Eq a } => a -> Bool  [a := Int]  =  { Eq Int } => Int -> Bool
	ev := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{Var("a")}),
		Body:        MkArrow(Var("a"), Con("Bool")),
	}
	result := Subst(ev, "a", Con("Int"))
	re, ok := result.(*TyEvidence)
	if !ok {
		t.Fatalf("expected TyEvidence, got %T", result)
	}
	// Check constraints substituted.
	rc := re.Constraints.ConEntries()
	if !Equal(rc[0].Args[0], Con("Int")) {
		t.Errorf("constraint arg: expected Int, got %s", Pretty(rc[0].Args[0]))
	}
	// Check body substituted.
	arr, ok := re.Body.(*TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow body, got %T", re.Body)
	}
	if !Equal(arr.From, Con("Int")) {
		t.Errorf("body from: expected Int, got %s", Pretty(arr.From))
	}
}

func TestQuantifiedConstraintSubstCaptureAvoidance(t *testing.T) {
	// forall b. Eq b => Show b   [a := b]
	// should alpha-rename bound b to avoid capture.
	entry := ConstraintEntry{
		ClassName: "Show",
		Args:      []Type{Var("a")},
		Quantified: &QuantifiedConstraint{
			Vars:    []ForallBinder{{Name: "b", Kind: KType{}}},
			Context: []ConstraintEntry{{ClassName: "Eq", Args: []Type{Var("b")}}},
			Head:    ConstraintEntry{ClassName: "Show", Args: []Type{Var("b")}},
		},
	}
	cr := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{entry}}}
	result := Subst(cr, "a", Var("b"))
	rc := result.(*TyEvidenceRow)
	qc := rc.ConEntries()[0].Quantified
	if qc == nil {
		t.Fatal("expected quantified constraint to be preserved")
	}
	// Bound variable should be renamed away from "b".
	if qc.Vars[0].Name == "b" {
		t.Error("should have renamed bound variable to avoid capture with replacement 'b'")
	}
	// Context and head should use the renamed variable.
	freshName := qc.Vars[0].Name
	if !Equal(qc.Context[0].Args[0], Var(freshName)) {
		t.Errorf("context should reference renamed var %q, got %s", freshName, Pretty(qc.Context[0].Args[0]))
	}
	if !Equal(qc.Head.Args[0], Var(freshName)) {
		t.Errorf("head should reference renamed var %q, got %s", freshName, Pretty(qc.Head.Args[0]))
	}
}

func TestEvidenceSubstIdentity(t *testing.T) {
	ev := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{Con("Int")}),
		Body:        Con("Bool"),
	}
	result := Subst(ev, "z", Con("String"))
	if result != ev {
		t.Error("subst of non-occurring variable should return same pointer")
	}
}

// =============================================================================
// FreeVars
// =============================================================================

func TestConstraintRowFreeVars(t *testing.T) {
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Var("a")}},
			{ClassName: "Ord", Args: []Type{Var("b")}},
		}},
		Tail: Var("c"),
	}
	fv := FreeVars(cr)
	for _, v := range []string{"a", "b", "c"} {
		if _, ok := fv[v]; !ok {
			t.Errorf("'%s' should be free", v)
		}
	}
}

func TestEvidenceFreeVars(t *testing.T) {
	ev := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{Var("a")}),
		Body:        MkArrow(Var("a"), Var("b")),
	}
	fv := FreeVars(ev)
	for _, v := range []string{"a", "b"} {
		if _, ok := fv[v]; !ok {
			t.Errorf("'%s' should be free", v)
		}
	}
}

func TestEvidenceFreeVarsUnderForall(t *testing.T) {
	// forall a. { Eq a } => a -> b — FV = {b}
	ty := MkForall("a", KType{},
		&TyEvidence{
			Constraints: SingleConstraint("Eq", []Type{Var("a")}),
			Body:        MkArrow(Var("a"), Var("b")),
		})
	fv := FreeVars(ty)
	if _, ok := fv["a"]; ok {
		t.Error("'a' should be bound by forall")
	}
	if _, ok := fv["b"]; !ok {
		t.Error("'b' should be free")
	}
}

// =============================================================================
// Builders
// =============================================================================

func TestEmptyConstraintRow(t *testing.T) {
	cr := EmptyConstraintRow()
	if len(cr.ConEntries()) != 0 {
		t.Error("empty constraint row should have no entries")
	}
	if cr.Tail != nil {
		t.Error("empty constraint row should have nil tail")
	}
}

func TestSingleConstraint(t *testing.T) {
	cr := SingleConstraint("Eq", []Type{Var("a")})
	if len(cr.ConEntries()) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(cr.ConEntries()))
	}
	if cr.ConEntries()[0].ClassName != "Eq" {
		t.Errorf("expected class 'Eq', got %q", cr.ConEntries()[0].ClassName)
	}
	if len(cr.ConEntries()[0].Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(cr.ConEntries()[0].Args))
	}
}

func TestMkEvidence(t *testing.T) {
	entries := []ConstraintEntry{
		{ClassName: "Eq", Args: []Type{Var("a")}},
	}
	body := MkArrow(Var("a"), Con("Bool"))
	ev := MkEvidence(entries, body)
	if ev.Constraints == nil {
		t.Fatal("Constraints should not be nil")
	}
	if len(ev.Constraints.ConEntries()) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ev.Constraints.ConEntries()))
	}
	if ev.Body != body {
		t.Error("Body mismatch")
	}
}

// =============================================================================
// Constraint Row Operations
// =============================================================================

func TestNormalizeConstraints(t *testing.T) {
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Ord", Args: []Type{Var("a")}},
			{ClassName: "Eq", Args: []Type{Var("a")}},
		}},
	}
	norm := NormalizeConstraints(cr)
	if norm.ConEntries()[0].ClassName != "Eq" {
		t.Errorf("first entry should be Eq, got %s", norm.ConEntries()[0].ClassName)
	}
	if norm.ConEntries()[1].ClassName != "Ord" {
		t.Errorf("second entry should be Ord, got %s", norm.ConEntries()[1].ClassName)
	}
}

func TestNormalizeConstraintsSameClassName(t *testing.T) {
	// Eq a, Eq b — same className, different args. Sort by canonical key.
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			{ClassName: "Eq", Args: []Type{Var("b")}},
			{ClassName: "Eq", Args: []Type{Var("a")}},
		}},
	}
	norm := NormalizeConstraints(cr)
	// "Eq a" < "Eq b" canonically.
	if Pretty(norm.ConEntries()[0].Args[0]) != "a" {
		t.Errorf("expected Eq a first, got Eq %s", Pretty(norm.ConEntries()[0].Args[0]))
	}
}

func TestNormalizeConstraintsSingle(t *testing.T) {
	cr := SingleConstraint("Eq", []Type{Var("a")})
	norm := NormalizeConstraints(cr)
	if norm != cr {
		t.Error("single-entry normalization should return same pointer")
	}
}

func TestConstraintKey(t *testing.T) {
	e1 := ConstraintEntry{ClassName: "Eq", Args: []Type{Var("a")}}
	e2 := ConstraintEntry{ClassName: "Eq", Args: []Type{Var("b")}}
	e3 := ConstraintEntry{ClassName: "Ord", Args: []Type{Var("a")}}

	k1 := ConstraintKey(e1)
	k2 := ConstraintKey(e2)
	k3 := ConstraintKey(e3)

	if k1 == k2 {
		t.Error("Eq a and Eq b should have different keys")
	}
	if k1 == k3 {
		t.Error("Eq a and Ord a should have different keys")
	}
	// Deterministic.
	if k1 != ConstraintKey(e1) {
		t.Error("key should be deterministic")
	}
}

func TestExtendConstraint(t *testing.T) {
	cr := SingleConstraint("Eq", []Type{Var("a")})
	extended := ExtendConstraint(cr, ConstraintEntry{ClassName: "Ord", Args: []Type{Var("a")}})
	if len(extended.ConEntries()) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(extended.ConEntries()))
	}
}
