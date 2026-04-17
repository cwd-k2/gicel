package types

import "testing"

// PeelForalls kind substitution tests — verifies that the visitor receives
// Kind fields with accumulated substitutions applied for K>=2 binders.
// Does NOT cover: subst_test.go, subst_kind_test.go.

func TestPeelForalls_KindSubst_K2(t *testing.T) {
	// \(k: Kind). \(a: k). a -> a
	// When peeling, the visitor for "a" should see Kind = replacement-for-k,
	// not the raw TyVar{k}.
	ty := &TyForall{
		Var:  "k",
		Kind: SortZero,
		Body: &TyForall{
			Var:  "a",
			Kind: &TyVar{Name: "k"},
			Body: &TyArrow{From: &TyVar{Name: "a"}, To: &TyVar{Name: "a"}},
		},
	}

	replK := testOps.Con("ReplacementForK")
	var secondKind Type
	testOps.PeelForalls(ty, func(f *TyForall) (Type, LevelExpr) {
		if f.Var == "k" {
			return replK, nil
		}
		// Record the Kind the visitor sees for "a".
		secondKind = f.Kind
		return testOps.Con("ReplacementForA"), nil
	})

	if secondKind == nil {
		t.Fatal("visitor was not called for second binder")
	}
	if !testOps.Equal(secondKind, replK) {
		t.Errorf("expected Kind = %s, got %s", testOps.Pretty(replK), testOps.Pretty(secondKind))
	}
}

func TestPeelForalls_KindSubst_K3(t *testing.T) {
	// \(k: Kind). \(j: k). \(a: j). a -> a
	// The visitor for "a" should see Kind = replacement-for-j (which itself
	// was generated from the adjusted Kind of the "j" binder).
	ty := &TyForall{
		Var:  "k",
		Kind: SortZero,
		Body: &TyForall{
			Var:  "j",
			Kind: &TyVar{Name: "k"},
			Body: &TyForall{
				Var:  "a",
				Kind: &TyVar{Name: "j"},
				Body: &TyArrow{From: &TyVar{Name: "a"}, To: &TyVar{Name: "a"}},
			},
		},
	}

	replK := testOps.Con("K_Repl")
	replJ := testOps.Con("J_Repl")
	var thirdKind Type
	testOps.PeelForalls(ty, func(f *TyForall) (Type, LevelExpr) {
		switch f.Var {
		case "k":
			return replK, nil
		case "j":
			// Verify that the "j" binder sees Kind = replK (not TyVar{k}).
			if !testOps.Equal(f.Kind, replK) {
				t.Errorf("j binder: expected Kind = %s, got %s", testOps.Pretty(replK), testOps.Pretty(f.Kind))
			}
			return replJ, nil
		default:
			thirdKind = f.Kind
			return testOps.Con("A_Repl"), nil
		}
	})

	if thirdKind == nil {
		t.Fatal("visitor was not called for third binder")
	}
	if !testOps.Equal(thirdKind, replJ) {
		t.Errorf("expected Kind = %s, got %s", testOps.Pretty(replJ), testOps.Pretty(thirdKind))
	}
}

func TestPeelForalls_KindSubst_K1_Unchanged(t *testing.T) {
	// Single binder: no accumulated substitution, Kind should be raw.
	ty := &TyForall{
		Var:  "a",
		Kind: TypeOfTypes,
		Body: &TyArrow{From: &TyVar{Name: "a"}, To: &TyVar{Name: "a"}},
	}

	var receivedKind Type
	testOps.PeelForalls(ty, func(f *TyForall) (Type, LevelExpr) {
		receivedKind = f.Kind
		return testOps.Con("Repl"), nil
	})

	if !testOps.Equal(receivedKind, TypeOfTypes) {
		t.Errorf("expected Kind = Type, got %s", testOps.Pretty(receivedKind))
	}
}

func TestPeelForalls_KindSubst_NoStaleReference(t *testing.T) {
	// \(k: Kind). \(a: k -> Type). a Int -> a Int
	// The "a" binder's Kind should have k replaced, yielding ReplacementForK -> Type.
	ty := &TyForall{
		Var:  "k",
		Kind: SortZero,
		Body: &TyForall{
			Var:  "a",
			Kind: &TyArrow{From: &TyVar{Name: "k"}, To: TypeOfTypes},
			Body: &TyArrow{
				From: &TyApp{Fun: &TyVar{Name: "a"}, Arg: testOps.Con("Int")},
				To:   &TyApp{Fun: &TyVar{Name: "a"}, Arg: testOps.Con("Int")},
			},
		},
	}

	replK := TypeOfRows
	var secondKind Type
	testOps.PeelForalls(ty, func(f *TyForall) (Type, LevelExpr) {
		if f.Var == "k" {
			return replK, nil
		}
		secondKind = f.Kind
		return testOps.Con("Repl"), nil
	})

	expected := &TyArrow{From: TypeOfRows, To: TypeOfTypes}
	if !testOps.Equal(secondKind, expected) {
		t.Errorf("expected Kind = %s, got %s", testOps.Pretty(expected), testOps.Pretty(secondKind))
	}
}
