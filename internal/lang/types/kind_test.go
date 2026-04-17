package types

import "testing"

// Arity and ResultKind tests — these functions now operate on Type (TyArrow chains).

func TestArityOnType(t *testing.T) {
	// Type -> Row -> Type has arity 2
	k := &TyArrow{From: TypeOfTypes, To: &TyArrow{From: TypeOfRows, To: TypeOfTypes}}
	if a := Arity(k); a != 2 {
		t.Errorf("expected arity 2, got %d", a)
	}
	// TypeOfTypes has arity 0
	if a := Arity(TypeOfTypes); a != 0 {
		t.Errorf("expected arity 0, got %d", a)
	}
}

func TestResultKindOnType(t *testing.T) {
	// Type -> Row -> Constraint → result = Constraint
	k := &TyArrow{From: TypeOfTypes, To: &TyArrow{From: TypeOfRows, To: TypeOfConstraints}}
	rk := ResultKind(k)
	if !testOps.Equal(rk, TypeOfConstraints) {
		t.Errorf("expected Constraint, got %s", testOps.PrettyTypeAsKind(rk))
	}
}
