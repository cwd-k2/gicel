// Family verify tests — injectivity verification, pattern variable collection.
// Does NOT cover: reduce_test.go (pattern matching), row_family_test.go (row families).
package family

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/env"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- CollectPatternVars tests ---

func TestCollectPatternVars_Simple(t *testing.T) {
	vars := CollectPatternVars([]types.Type{tyvar("a"), tyvar("b")})
	if len(vars) != 2 || vars[0] != "a" || vars[1] != "b" {
		t.Fatalf("expected [a, b], got %v", vars)
	}
}

func TestCollectPatternVars_Wildcard(t *testing.T) {
	vars := CollectPatternVars([]types.Type{tyvar("_"), tyvar("a")})
	if len(vars) != 1 || vars[0] != "a" {
		t.Fatalf("expected [a], got %v", vars)
	}
}

func TestCollectPatternVars_Dedup(t *testing.T) {
	vars := CollectPatternVars([]types.Type{tyvar("a"), tyvar("a")})
	if len(vars) != 1 {
		t.Fatalf("expected 1 unique var, got %v", vars)
	}
}

func TestCollectPatternVars_Nested(t *testing.T) {
	// List a → TyApp(TyCon "List", TyVar "a")
	pat := app(con("List"), tyvar("a"))
	vars := CollectPatternVars([]types.Type{pat})
	if len(vars) != 1 || vars[0] != "a" {
		t.Fatalf("expected [a], got %v", vars)
	}
}

func TestCollectPatternVars_Empty(t *testing.T) {
	vars := CollectPatternVars(nil)
	if len(vars) != 0 {
		t.Fatalf("expected empty, got %v", vars)
	}
}

func TestCollectPatternVars_ConOnly(t *testing.T) {
	vars := CollectPatternVars([]types.Type{con("Int")})
	if len(vars) != 0 {
		t.Fatalf("expected no vars from TyCon, got %v", vars)
	}
}

// --- VerifyInjectivity tests ---

func TestVerifyInjectivity_Injective(t *testing.T) {
	// type family F a | result -> a where
	//   F Int  = Bool
	//   F Bool = Int
	// Injective: different RHS → different LHS.
	h := newTestHarness(nil)
	info := &env.TypeFamilyInfo{
		Name:       "F",
		Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
		ResultName: "result",
		Equations: []env.TFEquation{
			{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
			{Patterns: []types.Type{con("Bool")}, RHS: con("Int")},
		},
	}

	h.env.VerifyInjectivity(info)
	if len(h.errors) > 0 {
		t.Fatalf("expected no injectivity errors, got: %v", h.errors)
	}
}

func TestVerifyInjectivity_Violation(t *testing.T) {
	// type family F a | result -> a where
	//   F Int  = Bool
	//   F Bool = Bool
	// NOT injective: same RHS (Bool) but different LHS (Int vs Bool).
	h := newTestHarness(nil)
	info := &env.TypeFamilyInfo{
		Name:       "F",
		Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
		ResultName: "result",
		Equations: []env.TFEquation{
			{Patterns: []types.Type{con("Int")}, RHS: con("Bool")},
			{Patterns: []types.Type{con("Bool")}, RHS: con("Bool")},
		},
	}

	h.env.VerifyInjectivity(info)
	if len(h.errors) == 0 {
		t.Fatal("expected injectivity violation error")
	}
	if !strings.Contains(h.errors[0], "injectivity violation") {
		t.Fatalf("expected 'injectivity violation' in error, got: %s", h.errors[0])
	}
}

func TestVerifyInjectivity_ParametricInjective(t *testing.T) {
	// type family Wrap a | result -> a where
	//   Wrap a = Maybe a
	// Injective: parametric in a.
	h := newTestHarness(nil)
	info := &env.TypeFamilyInfo{
		Name:       "Wrap",
		Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
		ResultName: "result",
		Equations: []env.TFEquation{
			{Patterns: []types.Type{tyvar("a")}, RHS: app(con("Maybe"), tyvar("a"))},
		},
	}

	h.env.VerifyInjectivity(info)
	if len(h.errors) > 0 {
		t.Fatalf("expected no injectivity errors, got: %v", h.errors)
	}
}

func TestVerifyInjectivity_MultiParam(t *testing.T) {
	// type family F a b | result -> a b where
	//   F Int  Bool   = String
	//   F Bool String = String
	// NOT injective: same RHS (String) but different LHS pairs.
	h := newTestHarness(nil)
	info := &env.TypeFamilyInfo{
		Name:       "F",
		Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}, {Name: "b", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
		ResultName: "result",
		Equations: []env.TFEquation{
			{Patterns: []types.Type{con("Int"), con("Bool")}, RHS: con("String")},
			{Patterns: []types.Type{con("Bool"), con("String")}, RHS: con("String")},
		},
	}

	h.env.VerifyInjectivity(info)
	if len(h.errors) == 0 {
		t.Fatal("expected injectivity violation for multi-param family")
	}
}

func TestVerifyInjectivity_NestedPatterns(t *testing.T) {
	// type family F a | result -> a where
	//   F (Maybe a) = a
	//   F (List a)  = a
	// NOT injective: F (Maybe Int) = Int, F (List Int) = Int → same RHS, different LHS.
	h := newTestHarness(nil)
	info := &env.TypeFamilyInfo{
		Name:       "F",
		Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
		ResultName: "result",
		Equations: []env.TFEquation{
			{Patterns: []types.Type{app(con("Maybe"), tyvar("a"))}, RHS: tyvar("a")},
			{Patterns: []types.Type{app(con("List"), tyvar("a"))}, RHS: tyvar("a")},
		},
	}

	h.env.VerifyInjectivity(info)
	if len(h.errors) == 0 {
		t.Fatal("expected injectivity violation for nested patterns")
	}
}

func TestVerifyInjectivity_SingleEquation(t *testing.T) {
	// Single equation is trivially injective.
	h := newTestHarness(nil)
	info := &env.TypeFamilyInfo{
		Name:       "F",
		Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
		ResultName: "result",
		Equations: []env.TFEquation{
			{Patterns: []types.Type{tyvar("a")}, RHS: tyvar("a")},
		},
	}

	h.env.VerifyInjectivity(info)
	if len(h.errors) > 0 {
		t.Fatalf("single equation should be trivially injective, got: %v", h.errors)
	}
}

func TestVerifyInjectivity_ExtraPatterns(t *testing.T) {
	// More patterns than declared params → fallback kind (TypeOfTypes).
	// Should not panic and should still verify correctly.
	h := newTestHarness(nil)
	info := &env.TypeFamilyInfo{
		Name:       "F",
		Params:     []env.TFParam{{Name: "a", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
		ResultName: "result",
		Equations: []env.TFEquation{
			{Patterns: []types.Type{con("Int"), tyvar("x")}, RHS: con("Bool")},
			{Patterns: []types.Type{con("Bool"), tyvar("y")}, RHS: con("Bool")},
		},
	}

	h.env.VerifyInjectivity(info)
	if len(h.errors) == 0 {
		t.Fatal("expected injectivity violation (same RHS, different first pattern)")
	}
}
