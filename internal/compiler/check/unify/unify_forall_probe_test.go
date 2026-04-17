//go:build probe

// TyForall unification probe tests — capture-avoiding substitution.
// Does NOT cover: unify_isolation_test.go, unify_constraint_test.go.
package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// TestForallUnify_SameBinder — ∀a.a ~ ∀a.a should succeed trivially.
func TestForallUnify_SameBinder(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	a := types.MkForall("a", types.TypeOfTypes,
		&types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "a"}})
	b := types.MkForall("a", types.TypeOfTypes,
		&types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "a"}})
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// TestForallUnify_DifferentBinder — ∀a.a ~ ∀b.b should succeed
// (alpha-equivalence).
func TestForallUnify_DifferentBinder(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	a := types.MkForall("a", types.TypeOfTypes,
		&types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "a"}})
	b := types.MkForall("b", types.TypeOfTypes,
		&types.TyArrow{From: &types.TyVar{Name: "b"}, To: &types.TyVar{Name: "b"}})
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// TestForallUnify_CaptureSameNameFreeVar — ∀a.(a -> b) ~ ∀b.(b -> b)
// must NOT unify: in the RHS, the outer `b` is free while the binder
// `b` is bound. If the unifier reuses `a` as the fresh name, it would
// conflate the bound and free occurrences of `b` in the RHS body,
// producing a false positive.
//
// This is the exact scenario that the capture-avoiding fix prevents.
func TestForallUnify_CaptureSameNameFreeVar(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	// ∀a. a -> b (b is free)
	lhs := types.MkForall("a", types.TypeOfTypes,
		&types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "b"}})
	// ∀b. b -> b (b is bound — same name as the free var in LHS)
	rhs := types.MkForall("b", types.TypeOfTypes,
		&types.TyArrow{From: &types.TyVar{Name: "b"}, To: &types.TyVar{Name: "b"}})
	err := u.Unify(lhs, rhs)
	if err == nil {
		t.Fatal("expected unification failure (capture would make it succeed incorrectly)")
	}
}

// TestForallUnify_NoCaptureDistinctFreeVars — ∀a.(a -> c) ~ ∀b.(b -> c)
// should succeed: `c` is free in both, and the binders are alpha-equivalent.
func TestForallUnify_NoCaptureDistinctFreeVars(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	lhs := types.MkForall("a", types.TypeOfTypes,
		&types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "c"}})
	rhs := types.MkForall("b", types.TypeOfTypes,
		&types.TyArrow{From: &types.TyVar{Name: "b"}, To: &types.TyVar{Name: "c"}})
	if err := u.Unify(lhs, rhs); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

// TestForallUnify_NestedCapture — ∀a.∀b.(a -> b) ~ ∀b.∀a.(b -> a)
// should succeed (double alpha-rename).
func TestForallUnify_NestedCapture(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	lhs := types.MkForall("a", types.TypeOfTypes,
		types.MkForall("b", types.TypeOfTypes,
			&types.TyArrow{From: &types.TyVar{Name: "a"}, To: &types.TyVar{Name: "b"}}))
	rhs := types.MkForall("b", types.TypeOfTypes,
		types.MkForall("a", types.TypeOfTypes,
			&types.TyArrow{From: &types.TyVar{Name: "b"}, To: &types.TyVar{Name: "a"}}))
	if err := u.Unify(lhs, rhs); err != nil {
		t.Fatalf("expected success (alpha-equivalent), got: %v", err)
	}
}
