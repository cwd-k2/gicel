package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Kind metas are now TyMeta with Kind=SortZero.
func kindMeta(id int) *types.TyMeta {
	return &types.TyMeta{ID: id, Kind: types.SortZero}
}

// =============================================================================
// Kind unification — basic cases
// =============================================================================

func TestUnifyKindsTypeType(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.TypeOfTypes, types.TypeOfTypes); err != nil {
		t.Errorf("Type ~ Type should succeed: %v", err)
	}
}

func TestUnifyKindsRowRow(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.TypeOfRows, types.TypeOfRows); err != nil {
		t.Errorf("Row ~ Row should succeed: %v", err)
	}
}

func TestUnifyKindsMismatch(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.TypeOfTypes, types.TypeOfRows); err == nil {
		t.Error("Type ~ Row should fail")
	}
}

func TestUnifyKindsArrow(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	k1 := &types.TyArrow{From: types.TypeOfTypes, To: types.TypeOfRows}
	k2 := &types.TyArrow{From: types.TypeOfTypes, To: types.TypeOfRows}
	if err := u.Unify(k1, k2); err != nil {
		t.Errorf("(Type -> Row) ~ (Type -> Row) should succeed: %v", err)
	}
}

func TestUnifyKindsArrowMismatch(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	k1 := &types.TyArrow{From: types.TypeOfTypes, To: types.TypeOfRows}
	k2 := &types.TyArrow{From: types.TypeOfTypes, To: types.TypeOfTypes}
	if err := u.Unify(k1, k2); err == nil {
		t.Error("(Type -> Row) ~ (Type -> Type) should fail")
	}
}

// =============================================================================
// Kind metavariable solving
// =============================================================================

func TestUnifyKindsMetaSolve(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := kindMeta(1)
	if err := u.Unify(m, types.TypeOfTypes); err != nil {
		t.Errorf("?k1 ~ Type should succeed: %v", err)
	}
	soln := u.Zonk(m)
	if !types.Equal(soln, types.TypeOfTypes) {
		t.Errorf("?k1 should be solved to Type, got %s", types.PrettyTypeAsKind(soln))
	}
}

func TestUnifyKindsMetaSolveBoth(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m1 := kindMeta(1)
	m2 := kindMeta(2)
	if err := u.Unify(m1, m2); err != nil {
		t.Errorf("?k1 ~ ?k2 should succeed: %v", err)
	}
	// Solve m2 to Type; m1 should follow.
	if err := u.Unify(m2, types.TypeOfTypes); err != nil {
		t.Fatalf("?k2 ~ Type should succeed: %v", err)
	}
	if !types.Equal(u.Zonk(m1), types.TypeOfTypes) {
		t.Errorf("?k1 should resolve to Type via ?k2, got %s", types.PrettyTypeAsKind(u.Zonk(m1)))
	}
}

func TestUnifyKindsMetaInArrow(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := kindMeta(1)
	k1 := &types.TyArrow{From: m, To: types.TypeOfTypes}
	k2 := &types.TyArrow{From: types.TypeOfRows, To: types.TypeOfTypes}
	if err := u.Unify(k1, k2); err != nil {
		t.Errorf("(?k1 -> Type) ~ (Row -> Type) should succeed: %v", err)
	}
	if !types.Equal(u.Zonk(m), types.TypeOfRows) {
		t.Errorf("?k1 should be Row, got %s", types.PrettyTypeAsKind(u.Zonk(m)))
	}
}

// =============================================================================
// Kind occurs check
// =============================================================================

func TestUnifyKindsOccursCheck(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := kindMeta(1)
	// ?k1 ~ ?k1 -> Type should fail (infinite kind).
	k := &types.TyArrow{From: m, To: types.TypeOfTypes}
	if err := u.Unify(m, k); err == nil {
		t.Error("?k1 ~ (?k1 -> Type) should fail with occurs check")
	}
}

// =============================================================================
// Kind variable handling
// =============================================================================

func TestUnifyKindsVarVar(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	v1 := &types.TyVar{Name: "k"}
	v2 := &types.TyVar{Name: "k"}
	if err := u.Unify(v1, v2); err != nil {
		t.Errorf("k ~ k should succeed: %v", err)
	}
}

func TestUnifyKindsVarMismatch(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	v1 := &types.TyVar{Name: "k"}
	v2 := &types.TyVar{Name: "j"}
	if err := u.Unify(v1, v2); err == nil {
		t.Error("k ~ j should fail")
	}
}

// =============================================================================
// Zonk (kind metas)
// =============================================================================

func TestZonkKindUnsolved(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := kindMeta(1)
	result := u.Zonk(m)
	if !types.Equal(result, m) {
		t.Error("unsolved meta should return itself")
	}
}

func TestZonkKindArrow(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := kindMeta(1)
	// Install solution directly via the soln map (kind metas now live in soln).
	u.InstallTempSolution(1, types.TypeOfRows)
	k := &types.TyArrow{From: m, To: types.TypeOfTypes}
	result := u.Zonk(k)
	arrow, ok := result.(*types.TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow, got %T", result)
	}
	if !types.Equal(arrow.From, types.TypeOfRows) {
		t.Errorf("expected Row, got %s", types.PrettyTypeAsKind(arrow.From))
	}
}

func TestZonkKindChain(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	// ?k1 -> ?k2, ?k2 solved to Type
	m1 := kindMeta(1)
	m2 := kindMeta(2)
	// Install solutions directly via the soln map (kind metas now live in soln).
	u.InstallTempSolution(1, m2)
	u.InstallTempSolution(2, types.TypeOfTypes)
	result := u.Zonk(m1)
	if !types.Equal(result, types.TypeOfTypes) {
		t.Errorf("chain resolution should give Type, got %s", types.PrettyTypeAsKind(result))
	}
}
