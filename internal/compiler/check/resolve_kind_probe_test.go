//go:build probe

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// kindMeta creates a kind-level metavariable (TyMeta with Kind=SortZero).
func kindMeta(id int) *types.TyMeta {
	return &types.TyMeta{ID: id, Kind: types.SortZero}
}

// =============================================================================
// Kind unification probe tests — kind meta solving, kind occurs check,
// TyArrow chains, PromotedDataKind matching, HKT kind polymorphism, kind mismatch
// in type application.
// =============================================================================

// =====================================================================
// From probe_d: Kind unification
// =====================================================================

// TestProbeD_KindUnify_MismatchedKinds — unifying Type with Row should fail.
func TestProbeD_KindUnify_MismatchedKinds(t *testing.T) {
	u := unify.NewUnifier()
	err := u.Unify(types.TypeOfTypes, types.TypeOfRows)
	if err == nil {
		t.Fatal("expected kind mismatch, got nil")
	}
}

// TestProbeD_KindUnify_KindMetaSolving — a kind meta should unify with Type.
func TestProbeD_KindUnify_KindMetaSolving(t *testing.T) {
	u := unify.NewUnifier()
	km := kindMeta(1)
	if err := u.Unify(km, types.TypeOfTypes); err != nil {
		t.Fatal(err)
	}
	result := u.Zonk(km)
	if !types.Equal(result, types.TypeOfTypes) {
		t.Errorf("expected Type, got %s", types.PrettyTypeAsKind(result))
	}
}

// TestProbeD_KindUnify_KindArrow — unifying (meta -> Type) with (Row -> Type)
// should solve meta = Row.
func TestProbeD_KindUnify_KindArrow(t *testing.T) {
	u := unify.NewUnifier()
	km := kindMeta(1)
	a := &types.TyArrow{From: km, To: types.TypeOfTypes}
	b := &types.TyArrow{From: types.TypeOfRows, To: types.TypeOfTypes}
	if err := u.Unify(a, b); err != nil {
		t.Fatal(err)
	}
	result := u.Zonk(km)
	if !types.Equal(result, types.TypeOfRows) {
		t.Errorf("expected Row, got %s", types.PrettyTypeAsKind(result))
	}
}

// TestProbeD_KindUnify_OccursCheck — kind occurs check: ?k ~ (?k -> Type).
func TestProbeD_KindUnify_OccursCheck(t *testing.T) {
	u := unify.NewUnifier()
	km := kindMeta(1)
	cycle := &types.TyArrow{From: km, To: types.TypeOfTypes}
	err := u.Unify(km, cycle)
	if err == nil {
		t.Fatal("expected kind occurs check, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck for kind, got %v", err)
	}
}

// TestProbeD_KindUnify_KDataMismatch — PromotedDataKind "Nat" vs "Bool" should fail.
func TestProbeD_KindUnify_KDataMismatch(t *testing.T) {
	u := unify.NewUnifier()
	err := u.Unify(types.PromotedDataKind("Nat"), types.PromotedDataKind("Bool"))
	if err == nil {
		t.Fatal("expected kind mismatch for different PromotedDataKind, got nil")
	}
}

// TestProbeD_KindUnify_KDataMatch — same PromotedDataKind should unify.
func TestProbeD_KindUnify_KDataMatch(t *testing.T) {
	u := unify.NewUnifier()
	if err := u.Unify(types.PromotedDataKind("Nat"), types.PromotedDataKind("Nat")); err != nil {
		t.Fatal(err)
	}
}

// =====================================================================
// From probe_e: Kind unification edge cases
// =====================================================================

// TestProbeE_KindUnify_MetaSelf — kind meta self-unification.
func TestProbeE_KindUnify_MetaSelf(t *testing.T) {
	u := unify.NewUnifier()
	km := kindMeta(1)
	if err := u.Unify(km, km); err != nil {
		t.Errorf("kind meta self-unify should succeed: %v", err)
	}
}

// TestProbeE_KindUnify_OccursCheck — kind occurs check.
func TestProbeE_KindUnify_OccursCheck(t *testing.T) {
	u := unify.NewUnifier()
	km := kindMeta(1)
	karrow := &types.TyArrow{From: km, To: types.TypeOfTypes}
	err := u.Unify(km, karrow)
	if err == nil {
		t.Fatal("expected kind occurs check error")
	}
}

// TestProbeE_KindUnify_KTypeMismatch — Type vs Row must fail.
func TestProbeE_KindUnify_KTypeMismatch(t *testing.T) {
	u := unify.NewUnifier()
	err := u.Unify(types.TypeOfTypes, types.TypeOfRows)
	if err == nil {
		t.Fatal("expected kind mismatch: Type vs Row")
	}
}

// TestProbeE_KindUnify_ArrowChain — deep TyArrow chains should unify.
func TestProbeE_KindUnify_ArrowChain(t *testing.T) {
	u := unify.NewUnifier()
	// Type -> Type -> Type vs ?k1 -> ?k2 -> Type
	k1 := kindMeta(1)
	k2 := kindMeta(2)
	concrete := &types.TyArrow{From: types.TypeOfTypes, To: &types.TyArrow{From: types.TypeOfTypes, To: types.TypeOfTypes}}
	withMetas := &types.TyArrow{From: k1, To: &types.TyArrow{From: k2, To: types.TypeOfTypes}}
	if err := u.Unify(concrete, withMetas); err != nil {
		t.Fatalf("expected success: %v", err)
	}
	zk1 := u.Zonk(k1)
	if !types.Equal(zk1, types.TypeOfTypes) {
		t.Errorf("expected k1 = Type, got %s", types.PrettyTypeAsKind(zk1))
	}
	zk2 := u.Zonk(k2)
	if !types.Equal(zk2, types.TypeOfTypes) {
		t.Errorf("expected k2 = Type, got %s", types.PrettyTypeAsKind(zk2))
	}
}

// TestProbeE_KindUnify_KDataDistinct — PromotedDataKind with different names must not unify.
func TestProbeE_KindUnify_KDataDistinct(t *testing.T) {
	u := unify.NewUnifier()
	err := u.Unify(types.PromotedDataKind("Color"), types.PromotedDataKind("Shape"))
	if err == nil {
		t.Fatal("expected kind mismatch for distinct PromotedDataKind")
	}
}

// TestProbeE_KindUnify_KConstraintVsKType — Constraint vs Type (at level 1) must fail.
func TestProbeE_KindUnify_KConstraintVsKType(t *testing.T) {
	u := unify.NewUnifier()
	err := u.Unify(types.TypeOfConstraints, types.TypeOfTypes)
	if err == nil {
		t.Fatal("expected kind mismatch: Constraint vs Type")
	}
}

// TestProbeE_HKT_KindPolymorphicClass — a class with a higher-kinded
// type parameter should compile.
func TestProbeE_HKT_KindPolymorphicClass(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}

myNot :: Bool -> Bool
myNot := \b. case b { True => False; False => True }

main := fmap myNot (Just True)
`
	checkSource(t, source, nil)
}

// TestProbeE_KindMismatch_InTypeApp — applying a type of kind Type
// to another type should produce a kind error.
// BUG: low — `Bool Int` in a type annotation position does not produce a
// kind error. The type resolver does not perform kind checking on type
// applications in annotations — `Bool` has kind `Type` (not `Type -> Type`),
// so `Bool Int` is a kind error, but the checker silently treats it as a
// valid type application. This could lead to unsound types being accepted.
func TestProbeE_KindMismatch_InTypeApp(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
-- Bool has kind Type, not Type -> Type, so Bool Int is a kind error
f :: Bool Int -> Bool
f := \x. True
main := True
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	// NOTE: This currently does NOT produce an error, which may be a bug.
	// The type annotation `Bool Int` should be a kind error since Bool :: Type,
	// not Type -> Type. We test that it doesn't panic at minimum.
	checkSourceNoPanic(t, source, config)
}
