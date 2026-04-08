// ClassifyConstraints contract tests — full coverage of all variant
// dispatch paths. Pinned because the only production caller (the unifier's
// constraint-row dispatch) currently filters EqualityEntry/VarEntry through
// checkWithEvidence before they reach the row, leaving the non-class
// branches without exercised coverage. Documenting the contract here so
// the function's exported behavior is preserved if a future caller starts
// passing non-class entries directly.
//
// Does NOT cover: row unification (unify_evidence_test.go), evidence row
// pretty printing (evidence_test.go).
package types

import "testing"

// classKeyOf is a small helper for the assertion phase: returns the head
// class name for class-headed entries, the canonical key for non-class
// entries, so test failures print something readable.
func classKeyOf(e ConstraintEntry) string {
	if name := HeadClassName(e); name != "" {
		return name
	}
	return ConstraintKey(e)
}

// --- Class-only matching (regression baseline) ---

func TestClassifyConstraints_ClassIdentical(t *testing.T) {
	a := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
	}
	b := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 1 {
		t.Fatalf("expected 1 shared, got %d", len(shared))
	}
	if len(onlyA) != 0 || len(onlyB) != 0 {
		t.Errorf("expected empty onlyA/onlyB, got %d/%d", len(onlyA), len(onlyB))
	}
}

func TestClassifyConstraints_ClassDifferentArgs(t *testing.T) {
	// Greedy match by class name: same class, different args still pairs.
	// The unifier loop is responsible for unifying the args afterwards.
	a := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
	}
	b := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Bool")}},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 1 {
		t.Fatalf("expected 1 shared (same class name), got %d", len(shared))
	}
	if len(onlyA) != 0 || len(onlyB) != 0 {
		t.Errorf("expected empty onlyA/onlyB, got %d/%d", len(onlyA), len(onlyB))
	}
}

func TestClassifyConstraints_ClassDifferentClasses(t *testing.T) {
	a := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
	}
	b := []ConstraintEntry{
		&ClassEntry{ClassName: "Ord", Args: []Type{Con("Int")}},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 0 {
		t.Errorf("expected 0 shared, got %d", len(shared))
	}
	if len(onlyA) != 1 || classKeyOf(onlyA[0]) != "Eq" {
		t.Errorf("expected onlyA=[Eq], got %v", onlyA)
	}
	if len(onlyB) != 1 || classKeyOf(onlyB[0]) != "Ord" {
		t.Errorf("expected onlyB=[Ord], got %v", onlyB)
	}
}

// --- EqualityEntry by-key matching ---

func TestClassifyConstraints_EqualityIdentical(t *testing.T) {
	// (a ~ Int) on both sides: structural key match → shared.
	a := []ConstraintEntry{
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Int")},
	}
	b := []ConstraintEntry{
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Int")},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 1 {
		t.Fatalf("expected 1 shared, got %d", len(shared))
	}
	if len(onlyA) != 0 || len(onlyB) != 0 {
		t.Errorf("expected empty onlyA/onlyB, got %d/%d", len(onlyA), len(onlyB))
	}
}

func TestClassifyConstraints_EqualityDifferentRhs(t *testing.T) {
	// (a ~ Int) vs (a ~ Bool): different RHS → different canonical keys → no match.
	a := []ConstraintEntry{
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Int")},
	}
	b := []ConstraintEntry{
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Bool")},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 0 {
		t.Errorf("expected 0 shared, got %d", len(shared))
	}
	if len(onlyA) != 1 || len(onlyB) != 1 {
		t.Errorf("expected onlyA/onlyB to each have 1 entry, got %d/%d", len(onlyA), len(onlyB))
	}
}

func TestClassifyConstraints_EqualityDifferentLhs(t *testing.T) {
	// (a ~ Int) vs (b ~ Int): different LHS → no match.
	a := []ConstraintEntry{
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Int")},
	}
	b := []ConstraintEntry{
		&EqualityEntry{Lhs: Var("b"), Rhs: Con("Int")},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 0 {
		t.Errorf("expected 0 shared, got %d", len(shared))
	}
	if len(onlyA) != 1 || len(onlyB) != 1 {
		t.Errorf("expected onlyA/onlyB to each have 1 entry, got %d/%d", len(onlyA), len(onlyB))
	}
}

// --- VarEntry by-key matching ---

func TestClassifyConstraints_VarIdentical(t *testing.T) {
	a := []ConstraintEntry{
		&VarEntry{Var: Var("c")},
	}
	b := []ConstraintEntry{
		&VarEntry{Var: Var("c")},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 1 {
		t.Fatalf("expected 1 shared, got %d", len(shared))
	}
	if len(onlyA) != 0 || len(onlyB) != 0 {
		t.Errorf("expected empty onlyA/onlyB, got %d/%d", len(onlyA), len(onlyB))
	}
}

func TestClassifyConstraints_VarDifferent(t *testing.T) {
	a := []ConstraintEntry{
		&VarEntry{Var: Var("c1")},
	}
	b := []ConstraintEntry{
		&VarEntry{Var: Var("c2")},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 0 {
		t.Errorf("expected 0 shared, got %d", len(shared))
	}
	if len(onlyA) != 1 || len(onlyB) != 1 {
		t.Errorf("expected onlyA/onlyB to each have 1 entry, got %d/%d", len(onlyA), len(onlyB))
	}
}

// --- Mixed variants ---

func TestClassifyConstraints_MixedAllMatched(t *testing.T) {
	// (Eq Int, a ~ Int) vs (Eq Int, a ~ Int): all matched.
	a := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Int")},
	}
	b := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Int")},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 2 {
		t.Fatalf("expected 2 shared, got %d", len(shared))
	}
	if len(onlyA) != 0 || len(onlyB) != 0 {
		t.Errorf("expected empty onlyA/onlyB, got %d/%d", len(onlyA), len(onlyB))
	}
}

func TestClassifyConstraints_MixedPartialMatch(t *testing.T) {
	// (Eq Int, a ~ Int) vs (Eq Int, a ~ Bool): only the class matches.
	a := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Int")},
	}
	b := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Bool")},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 1 {
		t.Fatalf("expected 1 shared, got %d", len(shared))
	}
	if len(onlyA) != 1 || classKeyOf(onlyA[0]) == "Eq" {
		t.Errorf("expected onlyA to contain the equality, got %v", onlyA)
	}
	if len(onlyB) != 1 || classKeyOf(onlyB[0]) == "Eq" {
		t.Errorf("expected onlyB to contain the equality, got %v", onlyB)
	}
}

func TestClassifyConstraints_OrderIndependence(t *testing.T) {
	// (Eq Int, Ord Bool) vs (Ord Bool, Eq Int): both orderings should give
	// the same set of shared matches.
	a := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
		&ClassEntry{ClassName: "Ord", Args: []Type{Con("Bool")}},
	}
	b := []ConstraintEntry{
		&ClassEntry{ClassName: "Ord", Args: []Type{Con("Bool")}},
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 2 {
		t.Fatalf("expected 2 shared, got %d", len(shared))
	}
	if len(onlyA) != 0 || len(onlyB) != 0 {
		t.Errorf("expected empty onlyA/onlyB, got %d/%d", len(onlyA), len(onlyB))
	}
}

// --- Empty / asymmetric cases ---

func TestClassifyConstraints_EmptyA(t *testing.T) {
	a := []ConstraintEntry{}
	b := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
		&EqualityEntry{Lhs: Var("a"), Rhs: Con("Int")},
	}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 0 {
		t.Errorf("expected 0 shared, got %d", len(shared))
	}
	if len(onlyA) != 0 {
		t.Errorf("expected empty onlyA, got %d", len(onlyA))
	}
	if len(onlyB) != 2 {
		t.Errorf("expected onlyB to have 2 entries, got %d", len(onlyB))
	}
}

func TestClassifyConstraints_EmptyB(t *testing.T) {
	a := []ConstraintEntry{
		&ClassEntry{ClassName: "Eq", Args: []Type{Con("Int")}},
		&VarEntry{Var: Var("c")},
	}
	b := []ConstraintEntry{}
	shared, onlyA, onlyB := ClassifyConstraints(a, b)
	if len(shared) != 0 {
		t.Errorf("expected 0 shared, got %d", len(shared))
	}
	if len(onlyA) != 2 {
		t.Errorf("expected onlyA to have 2 entries, got %d", len(onlyA))
	}
	if len(onlyB) != 0 {
		t.Errorf("expected empty onlyB, got %d", len(onlyB))
	}
}
