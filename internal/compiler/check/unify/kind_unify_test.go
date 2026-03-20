package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Kind unification — basic cases
// =============================================================================

func TestUnifyKindsTypeType(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KType{}, types.KType{}); err != nil {
		t.Errorf("Type ~ Type should succeed: %v", err)
	}
}

func TestUnifyKindsRowRow(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KRow{}, types.KRow{}); err != nil {
		t.Errorf("Row ~ Row should succeed: %v", err)
	}
}

func TestUnifyKindsMismatch(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KType{}, types.KRow{}); err == nil {
		t.Error("Type ~ Row should fail")
	}
}

func TestUnifyKindsArrow(t *testing.T) {
	u := NewUnifier()
	k1 := &types.KArrow{From: types.KType{}, To: types.KRow{}}
	k2 := &types.KArrow{From: types.KType{}, To: types.KRow{}}
	if err := u.UnifyKinds(k1, k2); err != nil {
		t.Errorf("(Type -> Row) ~ (Type -> Row) should succeed: %v", err)
	}
}

func TestUnifyKindsArrowMismatch(t *testing.T) {
	u := NewUnifier()
	k1 := &types.KArrow{From: types.KType{}, To: types.KRow{}}
	k2 := &types.KArrow{From: types.KType{}, To: types.KType{}}
	if err := u.UnifyKinds(k1, k2); err == nil {
		t.Error("(Type -> Row) ~ (Type -> Type) should fail")
	}
}

// =============================================================================
// Kind metavariable solving
// =============================================================================

func TestUnifyKindsMetaSolve(t *testing.T) {
	u := NewUnifier()
	m := &types.KMeta{ID: 1}
	if err := u.UnifyKinds(m, types.KType{}); err != nil {
		t.Errorf("?k1 ~ Type should succeed: %v", err)
	}
	soln := u.ZonkKind(m)
	if !soln.Equal(types.KType{}) {
		t.Errorf("?k1 should be solved to Type, got %s", soln)
	}
}

func TestUnifyKindsMetaSolveBoth(t *testing.T) {
	u := NewUnifier()
	m1 := &types.KMeta{ID: 1}
	m2 := &types.KMeta{ID: 2}
	if err := u.UnifyKinds(m1, m2); err != nil {
		t.Errorf("?k1 ~ ?k2 should succeed: %v", err)
	}
	// Solve m2 to Type; m1 should follow.
	if err := u.UnifyKinds(m2, types.KType{}); err != nil {
		t.Fatalf("?k2 ~ Type should succeed: %v", err)
	}
	if !u.ZonkKind(m1).Equal(types.KType{}) {
		t.Errorf("?k1 should resolve to Type via ?k2, got %s", u.ZonkKind(m1))
	}
}

func TestUnifyKindsMetaInArrow(t *testing.T) {
	u := NewUnifier()
	m := &types.KMeta{ID: 1}
	k1 := &types.KArrow{From: m, To: types.KType{}}
	k2 := &types.KArrow{From: types.KRow{}, To: types.KType{}}
	if err := u.UnifyKinds(k1, k2); err != nil {
		t.Errorf("(?k1 -> Type) ~ (Row -> Type) should succeed: %v", err)
	}
	if !u.ZonkKind(m).Equal(types.KRow{}) {
		t.Errorf("?k1 should be Row, got %s", u.ZonkKind(m))
	}
}

// =============================================================================
// Kind occurs check
// =============================================================================

func TestUnifyKindsOccursCheck(t *testing.T) {
	u := NewUnifier()
	m := &types.KMeta{ID: 1}
	// ?k1 ~ ?k1 -> Type should fail (infinite kind).
	k := &types.KArrow{From: m, To: types.KType{}}
	if err := u.UnifyKinds(m, k); err == nil {
		t.Error("?k1 ~ (?k1 -> Type) should fail with occurs check")
	}
}

// =============================================================================
// Kind variable handling
// =============================================================================

func TestUnifyKindsVarVar(t *testing.T) {
	u := NewUnifier()
	v1 := types.KVar{Name: "k"}
	v2 := types.KVar{Name: "k"}
	if err := u.UnifyKinds(v1, v2); err != nil {
		t.Errorf("k ~ k should succeed: %v", err)
	}
}

func TestUnifyKindsVarMismatch(t *testing.T) {
	u := NewUnifier()
	v1 := types.KVar{Name: "k"}
	v2 := types.KVar{Name: "j"}
	if err := u.UnifyKinds(v1, v2); err == nil {
		t.Error("k ~ j should fail")
	}
}

// =============================================================================
// ZonkKind
// =============================================================================

func TestZonkKindUnsolved(t *testing.T) {
	u := NewUnifier()
	m := &types.KMeta{ID: 1}
	result := u.ZonkKind(m)
	if !result.Equal(m) {
		t.Error("unsolved meta should return itself")
	}
}

func TestZonkKindArrow(t *testing.T) {
	u := NewUnifier()
	m := &types.KMeta{ID: 1}
	u.kindSoln[1] = types.KRow{}
	k := &types.KArrow{From: m, To: types.KType{}}
	result := u.ZonkKind(k)
	arrow, ok := result.(*types.KArrow)
	if !ok {
		t.Fatalf("expected KArrow, got %T", result)
	}
	if !arrow.From.Equal(types.KRow{}) {
		t.Errorf("expected Row, got %s", arrow.From)
	}
}

func TestZonkKindChain(t *testing.T) {
	u := NewUnifier()
	// ?k1 -> ?k2, ?k2 solved to Type
	m1 := &types.KMeta{ID: 1}
	m2 := &types.KMeta{ID: 2}
	u.kindSoln[1] = m2
	u.kindSoln[2] = types.KType{}
	result := u.ZonkKind(m1)
	if !result.Equal(types.KType{}) {
		t.Errorf("chain resolution should give Type, got %s", result)
	}
}
