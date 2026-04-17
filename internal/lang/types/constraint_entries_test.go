package types

import (
	"testing"
)

// testOps is the shared TypeOps instance for all test files in this package.
var testOps = &TypeOps{}

// =============================================================================
// TyEvidenceRow — type node interface
// =============================================================================

func TestConstraintRowTypeNode(t *testing.T) {
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
		}},
	}
	// Must satisfy Type interface.
	var _ Type = cr
	_ = cr.Span()
	_ = cr.Children()
}

func TestConstraintRowChildren(t *testing.T) {
	a := testOps.Var("a")
	b := testOps.Var("b")
	tail := testOps.Var("c")
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{a}},
			&ClassEntry{ClassName: "Ord", Args: []Type{b}},
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
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
		}},
	}
	ch := cr.Children()
	if len(ch) != 1 {
		t.Fatalf("expected 1 child, got %d", len(ch))
	}
}

func TestConstraintRowChildrenMultiArgs(t *testing.T) {
	// A constraint with multiple args: Functor f a
	f := testOps.Var("f")
	a := testOps.Var("a")
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Functor", Args: []Type{f, a}},
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
				&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
			}},
		},
		Body: testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
	}
	var _ Type = ev
	_ = ev.Span()
	_ = ev.Children()
}

func TestEvidenceChildren(t *testing.T) {
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
		}},
	}
	body := testOps.Arrow(testOps.Var("a"), testOps.Con("Bool"))
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
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Con("Int")}},
		}},
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Con("Int")}},
		}},
	}
	if !testOps.Equal(cr1, cr2) {
		t.Error("identical constraint rows should be equal")
	}
}

func TestConstraintRowNotEqualDifferentClass(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Con("Int")}},
		}},
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Ord", Args: []Type{testOps.Con("Int")}},
		}},
	}
	if testOps.Equal(cr1, cr2) {
		t.Error("different class names should not be equal")
	}
}

func TestConstraintRowNotEqualDifferentArgs(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Con("Int")}},
		}},
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Con("Bool")}},
		}},
	}
	if testOps.Equal(cr1, cr2) {
		t.Error("different args should not be equal")
	}
}

func TestConstraintRowEqualMultiEntry(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
			&ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("a")}},
		}},
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
			&ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("a")}},
		}},
	}
	if !testOps.Equal(cr1, cr2) {
		t.Error("identical multi-entry constraint rows should be equal")
	}
}

func TestConstraintRowEqualWithTail(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}}},
		Tail:    testOps.Var("c"),
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}}},
		Tail:    testOps.Var("c"),
	}
	if !testOps.Equal(cr1, cr2) {
		t.Error("constraint rows with same tail should be equal")
	}
}

func TestConstraintRowNotEqualTailMismatch(t *testing.T) {
	cr1 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}}},
		Tail:    testOps.Var("c"),
	}
	cr2 := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}}},
	}
	if testOps.Equal(cr1, cr2) {
		t.Error("open vs closed tail should not be equal")
	}
}

func TestConstraintRowAlphaEquivalence(t *testing.T) {
	// forall a. { Eq a } => a  ==  forall b. { Eq b } => b
	t1 := testOps.Forall("a", TypeOfTypes,
		&TyEvidence{
			Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
			Body:        testOps.Var("a"),
		})
	t2 := testOps.Forall("b", TypeOfTypes,
		&TyEvidence{
			Constraints: SingleConstraint("Eq", []Type{testOps.Var("b")}),
			Body:        testOps.Var("b"),
		})
	if !testOps.Equal(t1, t2) {
		t.Error("alpha-equivalent evidence types should be equal")
	}
}

func TestEvidenceEqual(t *testing.T) {
	ev1 := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
		Body:        testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
	}
	ev2 := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
		Body:        testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
	}
	if !testOps.Equal(ev1, ev2) {
		t.Error("identical evidence types should be equal")
	}
}

func TestEvidenceNotEqual(t *testing.T) {
	ev1 := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
		Body:        testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
	}
	ev2 := &TyEvidence{
		Constraints: SingleConstraint("Ord", []Type{testOps.Var("a")}),
		Body:        testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
	}
	if testOps.Equal(ev1, ev2) {
		t.Error("different constraints should not be equal")
	}
}

func TestEvidenceNotEqualToArrow(t *testing.T) {
	ev := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
		Body:        testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
	}
	arr := testOps.Arrow(testOps.Var("a"), testOps.Con("Bool"))
	if testOps.Equal(ev, arr) {
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
			SingleConstraint("Eq", []Type{testOps.Var("a")}),
			"{ Eq a }",
		},
		{
			"multiple",
			&TyEvidenceRow{
				Entries: &ConstraintEntries{Entries: []ConstraintEntry{
					&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
					&ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("a")}},
				}},
			},
			"{ Eq a, Ord a }",
		},
		{
			"with tail",
			&TyEvidenceRow{
				Entries: &ConstraintEntries{Entries: []ConstraintEntry{
					&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
				}},
				Tail: testOps.Var("c"),
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
			SingleConstraint("Functor", []Type{testOps.Var("f"), testOps.Var("a")}),
			"{ Functor f a }",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := testOps.Pretty(tt.cr)
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
				Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
				Body:        testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
			},
			"{ Eq a } => a -> Bool",
		},
		{
			"multi constraint",
			&TyEvidence{
				Constraints: &TyEvidenceRow{
					Entries: &ConstraintEntries{Entries: []ConstraintEntry{
						&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
						&ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("a")}},
					}},
				},
				Body: testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
			},
			"{ Eq a, Ord a } => a -> Bool",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := testOps.Pretty(tt.ev)
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
	cr := SingleConstraint("Eq", []Type{testOps.Var("a")})
	result := testOps.Subst(cr, "a", testOps.Con("Int"))
	rc, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	cls := rc.ConEntries()[0].(*ClassEntry)
	if !testOps.Equal(cls.Args[0], testOps.Con("Int")) {
		t.Errorf("expected Eq Int, got Eq %s", testOps.Pretty(cls.Args[0]))
	}
}

func TestConstraintRowSubstTail(t *testing.T) {
	// { Eq a | c }[c := { Ord b }] — tail substitution
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}}},
		Tail:    testOps.Var("c"),
	}
	replacement := SingleConstraint("Ord", []Type{testOps.Var("b")})
	result := testOps.Subst(cr, "c", replacement)
	rc, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	if !testOps.Equal(rc.Tail, replacement) {
		t.Errorf("tail should be replaced, got %s", testOps.Pretty(rc.Tail))
	}
}

func TestConstraintRowSubstIdentity(t *testing.T) {
	// Substituting a non-occurring variable returns same pointer.
	cr := SingleConstraint("Eq", []Type{testOps.Var("a")})
	result := testOps.Subst(cr, "z", testOps.Con("Int"))
	if result != cr {
		t.Error("subst of non-occurring variable should return same pointer")
	}
}

func TestEvidenceSubst(t *testing.T) {
	// { Eq a } => a -> Bool  [a := Int]  =  { Eq Int } => Int -> Bool
	ev := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
		Body:        testOps.Arrow(testOps.Var("a"), testOps.Con("Bool")),
	}
	result := testOps.Subst(ev, "a", testOps.Con("Int"))
	re, ok := result.(*TyEvidence)
	if !ok {
		t.Fatalf("expected TyEvidence, got %T", result)
	}
	// Check constraints substituted.
	rc := re.Constraints.ConEntries()
	rcCls := rc[0].(*ClassEntry)
	if !testOps.Equal(rcCls.Args[0], testOps.Con("Int")) {
		t.Errorf("constraint arg: expected Int, got %s", testOps.Pretty(rcCls.Args[0]))
	}
	// Check body substituted.
	arr, ok := re.Body.(*TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow body, got %T", re.Body)
	}
	if !testOps.Equal(arr.From, testOps.Con("Int")) {
		t.Errorf("body from: expected Int, got %s", testOps.Pretty(arr.From))
	}
}

func TestQuantifiedConstraintSubstCaptureAvoidance(t *testing.T) {
	// forall b. Eq a => Show b   [a := b]
	//
	// Naive substitution would produce `forall b. Eq b => Show b`, capturing
	// the replacement `b` under the forall. Capture avoidance must rename
	// the bound b first, yielding `forall b'. Eq b => Show b'`.
	//
	// Note: the pre-variant version of this test stored the quantified
	// constraint as an outer class entry with a Quantified back-reference,
	// and put the free `a` in the outer Args. Variant化 removed that hybrid
	// state, so the free `a` lives inside the Context directly now.
	entry := &QuantifiedConstraint{
		Vars:    []ForallBinder{{Name: "b", Kind: TypeOfTypes}},
		Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}},
		Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("b")}},
	}
	cr := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{entry}}}
	result := testOps.Subst(cr, "a", testOps.Var("b"))
	rc := result.(*TyEvidenceRow)
	qc, ok := rc.ConEntries()[0].(*QuantifiedConstraint)
	if !ok {
		t.Fatal("expected quantified constraint to be preserved")
	}
	// Bound variable should be renamed away from "b".
	if qc.Vars[0].Name == "b" {
		t.Error("should have renamed bound variable to avoid capture with replacement 'b'")
	}
	freshName := qc.Vars[0].Name
	// Context Eq a becomes Eq b (the replacement).
	ctxCls := qc.Context[0].(*ClassEntry)
	if !testOps.Equal(ctxCls.Args[0], testOps.Var("b")) {
		t.Errorf("context should hold the replacement Var(\"b\"), got %s", testOps.Pretty(ctxCls.Args[0]))
	}
	// Head Show b becomes Show b' (the renamed bound var).
	if !testOps.Equal(qc.Head.Args[0], testOps.Var(freshName)) {
		t.Errorf("head should reference renamed var %q, got %s", freshName, testOps.Pretty(qc.Head.Args[0]))
	}
}

func TestEvidenceSubstIdentity(t *testing.T) {
	ev := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{testOps.Con("Int")}),
		Body:        testOps.Con("Bool"),
	}
	result := testOps.Subst(ev, "z", testOps.Con("String"))
	if result != ev {
		t.Error("subst of non-occurring variable should return same pointer")
	}
}

// =============================================================================
// SubstMany — QuantifiedConstraint shadowing and capture avoidance.
//
// Before ConstraintEntries.SubstEntriesMany was rewritten to dispatch through
// substManyConstraintEntry, the parallel path delegated to MapChildren which
// walked QuantifiedConstraint children unconditionally. That variant silently
// dropped shadowing (substituting into binder-matching keys) and capture
// avoidance (free vars in replacements being captured by binders). The
// following tests pin that the two paths — single-var Subst (sequential) and
// batch-var SubstMany (parallel) — now produce semantically equal results.
// =============================================================================

func TestSubstManyQuantifiedConstraintShadowing(t *testing.T) {
	// forall a. Eq a => Show a   [a := Int]
	// The only binder shadows the only sub key, so the QC is untouched.
	entry := &QuantifiedConstraint{
		Vars:    []ForallBinder{{Name: "a", Kind: TypeOfTypes}},
		Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}},
		Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("a")}},
	}
	cr := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{entry}}}
	result := testOps.SubstMany(cr, map[string]Type{"a": testOps.Con("Int")}, nil)
	if result != cr {
		t.Errorf("fully shadowed substitution should return the same row pointer, got %s", testOps.Pretty(result))
	}
}

func TestSubstManyQuantifiedConstraintPartialShadowing(t *testing.T) {
	// forall a b. Eq a => Show b   [a := Int, b := Bool, c := Char]
	// Both a and b are shadowed; c does not occur anywhere. Net: unchanged.
	entry := &QuantifiedConstraint{
		Vars: []ForallBinder{
			{Name: "a", Kind: TypeOfTypes},
			{Name: "b", Kind: TypeOfTypes},
		},
		Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}},
		Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("b")}},
	}
	cr := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{entry}}}
	result := testOps.SubstMany(cr, map[string]Type{
		"a": testOps.Con("Int"),
		"b": testOps.Con("Bool"),
		"c": testOps.Con("Char"),
	}, nil)
	rc := result.(*TyEvidenceRow)
	qc := rc.ConEntries()[0].(*QuantifiedConstraint)
	if qc.Vars[0].Name != "a" || qc.Vars[1].Name != "b" {
		t.Errorf("binders should be untouched, got %v", qc.Vars)
	}
	ctxCls := qc.Context[0].(*ClassEntry)
	if !testOps.Equal(ctxCls.Args[0], testOps.Var("a")) {
		t.Errorf("context arg should still be Var(a), got %s", testOps.Pretty(ctxCls.Args[0]))
	}
	if !testOps.Equal(qc.Head.Args[0], testOps.Var("b")) {
		t.Errorf("head arg should still be Var(b), got %s", testOps.Pretty(qc.Head.Args[0]))
	}
}

func TestSubstManyQuantifiedConstraintCaptureAvoidance(t *testing.T) {
	// forall b. Eq a => Show b   [a := b]
	// Naive parallel substitution captures the replacement `b`; the fix
	// must rename the bound b first, matching the single-var path.
	entry := &QuantifiedConstraint{
		Vars:    []ForallBinder{{Name: "b", Kind: TypeOfTypes}},
		Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}},
		Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("b")}},
	}
	cr := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{entry}}}
	result := testOps.SubstMany(cr, map[string]Type{"a": testOps.Var("b")}, nil)
	rc, ok := result.(*TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	qc, ok := rc.ConEntries()[0].(*QuantifiedConstraint)
	if !ok {
		t.Fatal("expected quantified constraint to be preserved")
	}
	if qc.Vars[0].Name == "b" {
		t.Error("bound variable should have been renamed away from 'b' to avoid capture")
	}
	freshName := qc.Vars[0].Name
	// Context Eq a becomes Eq b (the replacement).
	ctxCls := qc.Context[0].(*ClassEntry)
	if !testOps.Equal(ctxCls.Args[0], testOps.Var("b")) {
		t.Errorf("context should hold the replacement Var(\"b\"), got %s", testOps.Pretty(ctxCls.Args[0]))
	}
	// Head Show b becomes Show b' (the renamed bound var).
	if !testOps.Equal(qc.Head.Args[0], testOps.Var(freshName)) {
		t.Errorf("head should reference renamed var %q, got %s", freshName, testOps.Pretty(qc.Head.Args[0]))
	}
}

func TestSubstManyQuantifiedConstraintEquivalence(t *testing.T) {
	// Single-var sequential Subst vs parallel SubstMany must produce
	// semantically equal results on QCs that exercise capture avoidance.
	// Extends the SubstMany equivalence contract (already checked for
	// TyForall and label vars) to quantified constraints.
	mkRow := func() *TyEvidenceRow {
		return &TyEvidenceRow{
			Entries: &ConstraintEntries{Entries: []ConstraintEntry{
				&QuantifiedConstraint{
					Vars:    []ForallBinder{{Name: "b", Kind: TypeOfTypes}},
					Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}},
					Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("b")}},
				},
			}},
		}
	}
	seq := testOps.Subst(mkRow(), "a", testOps.Var("b"))
	many := testOps.SubstMany(mkRow(), map[string]Type{"a": testOps.Var("b")}, nil)
	if !testOps.Equal(seq, many) {
		t.Errorf("SubstMany != sequential Subst on QC:\n  seq:  %s\n  many: %s",
			testOps.Pretty(seq), testOps.Pretty(many))
	}
}

func TestSubstManyQuantifiedConstraintMultiBinderCapture(t *testing.T) {
	// forall b c. Eq a => Show x   [a := b, x := c]
	// Both binders appear free in replacements and must be renamed.
	// After rename: forall b' c'. Eq a => Show x
	// After substitute: forall b' c'. Eq b => Show c
	entry := &QuantifiedConstraint{
		Vars: []ForallBinder{
			{Name: "b", Kind: TypeOfTypes},
			{Name: "c", Kind: TypeOfTypes},
		},
		Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}},
		Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("x")}},
	}
	cr := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{entry}}}
	result := testOps.SubstMany(cr, map[string]Type{
		"a": testOps.Var("b"),
		"x": testOps.Var("c"),
	}, nil)
	rc := result.(*TyEvidenceRow)
	qc := rc.ConEntries()[0].(*QuantifiedConstraint)
	if qc.Vars[0].Name == "b" {
		t.Error("first binder should have been renamed to avoid capture with free 'b'")
	}
	if qc.Vars[1].Name == "c" {
		t.Error("second binder should have been renamed to avoid capture with free 'c'")
	}
	ctxCls := qc.Context[0].(*ClassEntry)
	if !testOps.Equal(ctxCls.Args[0], testOps.Var("b")) {
		t.Errorf("context should hold the free replacement Var(\"b\"), got %s", testOps.Pretty(ctxCls.Args[0]))
	}
	if !testOps.Equal(qc.Head.Args[0], testOps.Var("c")) {
		t.Errorf("head should hold the free replacement Var(\"c\"), got %s", testOps.Pretty(qc.Head.Args[0]))
	}
}

func TestSubstManyQuantifiedConstraintMultiBinderCaptureEquivalence(t *testing.T) {
	// Same multi-binder capture case — verify sequential and parallel agree.
	mkRow := func() *TyEvidenceRow {
		return &TyEvidenceRow{
			Entries: &ConstraintEntries{Entries: []ConstraintEntry{
				&QuantifiedConstraint{
					Vars: []ForallBinder{
						{Name: "b", Kind: TypeOfTypes},
						{Name: "c", Kind: TypeOfTypes},
					},
					Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}},
					Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("x")}},
				},
			}},
		}
	}
	seq := testOps.Subst(mkRow(), "a", testOps.Var("b"))
	seq = testOps.Subst(seq, "x", testOps.Var("c"))
	many := testOps.SubstMany(mkRow(), map[string]Type{
		"a": testOps.Var("b"),
		"x": testOps.Var("c"),
	}, nil)
	if !testOps.Equal(seq, many) {
		t.Errorf("SubstMany != sequential Subst on multi-binder QC capture:\n  seq:  %s\n  many: %s",
			testOps.Pretty(seq), testOps.Pretty(many))
	}
}

func TestSubstManyQuantifiedConstraintDescendWithoutCapture(t *testing.T) {
	// forall a. Eq b => Show c   [b := Int, c := Bool]
	// No shadowing, no capture. Substitution descends into context and head.
	entry := &QuantifiedConstraint{
		Vars:    []ForallBinder{{Name: "a", Kind: TypeOfTypes}},
		Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("b")}}},
		Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("c")}},
	}
	cr := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{entry}}}
	result := testOps.SubstMany(cr, map[string]Type{
		"b": testOps.Con("Int"),
		"c": testOps.Con("Bool"),
	}, nil)
	rc := result.(*TyEvidenceRow)
	qc := rc.ConEntries()[0].(*QuantifiedConstraint)
	if qc.Vars[0].Name != "a" {
		t.Errorf("binder 'a' should be preserved (not in replacement fv), got %q", qc.Vars[0].Name)
	}
	ctxCls := qc.Context[0].(*ClassEntry)
	if !testOps.Equal(ctxCls.Args[0], testOps.Con("Int")) {
		t.Errorf("context should be Eq Int, got %s", testOps.Pretty(ctxCls.Args[0]))
	}
	if !testOps.Equal(qc.Head.Args[0], testOps.Con("Bool")) {
		t.Errorf("head should be Show Bool, got %s", testOps.Pretty(qc.Head.Args[0]))
	}
}

func TestSubstManyQuantifiedConstraintIdentity(t *testing.T) {
	// No key in subs touches the QC — pointer should come back unchanged.
	entry := &QuantifiedConstraint{
		Vars:    []ForallBinder{{Name: "a", Kind: TypeOfTypes}},
		Context: []ConstraintEntry{&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}},
		Head:    &ClassEntry{ClassName: "Show", Args: []Type{testOps.Var("a")}},
	}
	cr := &TyEvidenceRow{Entries: &ConstraintEntries{Entries: []ConstraintEntry{entry}}}
	result := testOps.SubstMany(cr, map[string]Type{"z": testOps.Con("String")}, nil)
	if result != cr {
		t.Errorf("non-occurring SubstMany should return the same row pointer, got %s", testOps.Pretty(result))
	}
}

// =============================================================================
// FreeVars
// =============================================================================

func TestConstraintRowFreeVars(t *testing.T) {
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
			&ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("b")}},
		}},
		Tail: testOps.Var("c"),
	}
	fv := testOps.FreeVars(cr)
	for _, v := range []string{"a", "b", "c"} {
		if _, ok := fv[v]; !ok {
			t.Errorf("'%s' should be free", v)
		}
	}
}

func TestEvidenceFreeVars(t *testing.T) {
	ev := &TyEvidence{
		Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
		Body:        testOps.Arrow(testOps.Var("a"), testOps.Var("b")),
	}
	fv := testOps.FreeVars(ev)
	for _, v := range []string{"a", "b"} {
		if _, ok := fv[v]; !ok {
			t.Errorf("'%s' should be free", v)
		}
	}
}

func TestEvidenceFreeVarsUnderForall(t *testing.T) {
	// forall a. { Eq a } => a -> b — FV = {b}
	ty := testOps.Forall("a", TypeOfTypes,
		&TyEvidence{
			Constraints: SingleConstraint("Eq", []Type{testOps.Var("a")}),
			Body:        testOps.Arrow(testOps.Var("a"), testOps.Var("b")),
		})
	fv := testOps.FreeVars(ty)
	if _, ok := fv["a"]; ok {
		t.Error("'a' should be bound by forall")
	}
	if _, ok := fv["b"]; !ok {
		t.Error("'b' should be free")
	}
}

func TestMkEvidence(t *testing.T) {
	entries := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
	}
	body := testOps.Arrow(testOps.Var("a"), testOps.Con("Bool"))
	ev := testOps.Evidence(entries, body)
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
			&ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("a")}},
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
		}},
	}
	norm := testOps.NormalizeConstraints(cr)
	if HeadClassName(norm.ConEntries()[0]) != "Eq" {
		t.Errorf("first entry should be Eq, got %s", HeadClassName(norm.ConEntries()[0]))
	}
	if HeadClassName(norm.ConEntries()[1]) != "Ord" {
		t.Errorf("second entry should be Ord, got %s", HeadClassName(norm.ConEntries()[1]))
	}
}

func TestNormalizeConstraintsSameClassName(t *testing.T) {
	// Eq a, Eq b — same className, different args. Sort by canonical key.
	cr := &TyEvidenceRow{
		Entries: &ConstraintEntries{Entries: []ConstraintEntry{
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("b")}},
			&ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}},
		}},
	}
	norm := testOps.NormalizeConstraints(cr)
	// "Eq a" < "Eq b" canonically.
	first := norm.ConEntries()[0].(*ClassEntry)
	if testOps.Pretty(first.Args[0]) != "a" {
		t.Errorf("expected Eq a first, got Eq %s", testOps.Pretty(first.Args[0]))
	}
}

func TestNormalizeConstraintsSingle(t *testing.T) {
	cr := SingleConstraint("Eq", []Type{testOps.Var("a")})
	norm := testOps.NormalizeConstraints(cr)
	if norm != cr {
		t.Error("single-entry normalization should return same pointer")
	}
}

func TestConstraintKey(t *testing.T) {
	e1 := &ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("a")}}
	e2 := &ClassEntry{ClassName: "Eq", Args: []Type{testOps.Var("b")}}
	e3 := &ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("a")}}

	k1 := testOps.ConstraintKey(e1)
	k2 := testOps.ConstraintKey(e2)
	k3 := testOps.ConstraintKey(e3)

	if k1 == k2 {
		t.Error("Eq a and Eq b should have different keys")
	}
	if k1 == k3 {
		t.Error("Eq a and Ord a should have different keys")
	}
	// Deterministic.
	if k1 != testOps.ConstraintKey(e1) {
		t.Error("key should be deterministic")
	}
}

func TestExtendConstraint(t *testing.T) {
	cr := SingleConstraint("Eq", []Type{testOps.Var("a")})
	extended := testOps.ExtendConstraint(cr, &ClassEntry{ClassName: "Ord", Args: []Type{testOps.Var("a")}})
	if len(extended.ConEntries()) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(extended.ConEntries()))
	}
}
