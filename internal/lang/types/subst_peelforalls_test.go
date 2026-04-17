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

	replK := MkCon("ReplacementForK")
	var secondKind Type
	PeelForalls(ty, func(f *TyForall) (Type, LevelExpr) {
		if f.Var == "k" {
			return replK, nil
		}
		// Record the Kind the visitor sees for "a".
		secondKind = f.Kind
		return MkCon("ReplacementForA"), nil
	})

	if secondKind == nil {
		t.Fatal("visitor was not called for second binder")
	}
	if !Equal(secondKind, replK) {
		t.Errorf("expected Kind = %s, got %s", Pretty(replK), Pretty(secondKind))
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

	replK := MkCon("K_Repl")
	replJ := MkCon("J_Repl")
	var thirdKind Type
	PeelForalls(ty, func(f *TyForall) (Type, LevelExpr) {
		switch f.Var {
		case "k":
			return replK, nil
		case "j":
			// Verify that the "j" binder sees Kind = replK (not TyVar{k}).
			if !Equal(f.Kind, replK) {
				t.Errorf("j binder: expected Kind = %s, got %s", Pretty(replK), Pretty(f.Kind))
			}
			return replJ, nil
		default:
			thirdKind = f.Kind
			return MkCon("A_Repl"), nil
		}
	})

	if thirdKind == nil {
		t.Fatal("visitor was not called for third binder")
	}
	if !Equal(thirdKind, replJ) {
		t.Errorf("expected Kind = %s, got %s", Pretty(replJ), Pretty(thirdKind))
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
	PeelForalls(ty, func(f *TyForall) (Type, LevelExpr) {
		receivedKind = f.Kind
		return MkCon("Repl"), nil
	})

	if !Equal(receivedKind, TypeOfTypes) {
		t.Errorf("expected Kind = Type, got %s", Pretty(receivedKind))
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
				From: &TyApp{Fun: &TyVar{Name: "a"}, Arg: MkCon("Int")},
				To:   &TyApp{Fun: &TyVar{Name: "a"}, Arg: MkCon("Int")},
			},
		},
	}

	replK := TypeOfRows
	var secondKind Type
	PeelForalls(ty, func(f *TyForall) (Type, LevelExpr) {
		if f.Var == "k" {
			return replK, nil
		}
		secondKind = f.Kind
		return MkCon("Repl"), nil
	})

	expected := &TyArrow{From: TypeOfRows, To: TypeOfTypes}
	if !Equal(secondKind, expected) {
		t.Errorf("expected Kind = %s, got %s", Pretty(expected), Pretty(secondKind))
	}
}
