//go:build probe

// Registry merge probe tests — type family equation coherence on merge.
// Does NOT cover: type_family_interaction_cross_test.go.
package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func testRegistry() *Registry {
	return newRegistry(&CheckConfig{})
}

// TestMergeFamily_IdenticalEquations — merging identical equations should
// succeed without error (diamond import deduplication).
func TestMergeFamily_IdenticalEquations(t *testing.T) {
	r := testRegistry()
	fam1 := &env.TypeFamilyInfo{
		Name:       "Elem",
		ResultKind: types.TypeOfTypes,
		Equations: []env.TFEquation{
			{Patterns: []types.Type{testOps.Con("List")}, RHS: testOps.Con("Int")},
		},
	}
	if err := r.RegisterFamily("Elem", fam1); err != nil {
		t.Fatalf("first register: %v", err)
	}
	fam2 := &env.TypeFamilyInfo{
		Name:       "Elem",
		ResultKind: types.TypeOfTypes,
		Equations: []env.TFEquation{
			{Patterns: []types.Type{testOps.Con("List")}, RHS: testOps.Con("Int")},
		},
	}
	if err := r.RegisterFamily("Elem", fam2); err != nil {
		t.Fatalf("merge with identical equation should succeed: %v", err)
	}
}

// TestMergeFamily_ConflictingRHS — merging equations with same LHS but
// different RHS must produce an error (coherence violation).
func TestMergeFamily_ConflictingRHS(t *testing.T) {
	r := testRegistry()
	fam1 := &env.TypeFamilyInfo{
		Name:       "Elem",
		ResultKind: types.TypeOfTypes,
		Equations: []env.TFEquation{
			{Patterns: []types.Type{testOps.Con("List")}, RHS: testOps.Con("Int")},
		},
	}
	if err := r.RegisterFamily("Elem", fam1); err != nil {
		t.Fatalf("first register: %v", err)
	}
	fam2 := &env.TypeFamilyInfo{
		Name:       "Elem",
		ResultKind: types.TypeOfTypes,
		Equations: []env.TFEquation{
			{Patterns: []types.Type{testOps.Con("List")}, RHS: testOps.Con("String")},
		},
	}
	err := r.RegisterFamily("Elem", fam2)
	if err == nil {
		t.Fatal("expected error for conflicting RHS, got nil")
	}
	if !strings.Contains(err.Error(), "conflicting") {
		t.Errorf("expected 'conflicting' in error, got: %v", err)
	}
}

// TestMergeFamily_DisjointEquations — merging equations with different LHS
// should succeed (non-overlapping enrichment).
func TestMergeFamily_DisjointEquations(t *testing.T) {
	r := testRegistry()
	fam1 := &env.TypeFamilyInfo{
		Name:       "Elem",
		ResultKind: types.TypeOfTypes,
		Equations: []env.TFEquation{
			{Patterns: []types.Type{testOps.Con("List")}, RHS: testOps.Con("Int")},
		},
	}
	if err := r.RegisterFamily("Elem", fam1); err != nil {
		t.Fatalf("first register: %v", err)
	}
	fam2 := &env.TypeFamilyInfo{
		Name:       "Elem",
		ResultKind: types.TypeOfTypes,
		Equations: []env.TFEquation{
			{Patterns: []types.Type{testOps.Con("Set")}, RHS: testOps.Con("String")},
		},
	}
	if err := r.RegisterFamily("Elem", fam2); err != nil {
		t.Fatalf("merge with disjoint LHS should succeed: %v", err)
	}
}
