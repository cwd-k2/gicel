//go:build probe

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Kind unification probe tests — kind meta solving, kind occurs check,
// KArrow chains, KData matching, HKT kind polymorphism, kind mismatch
// in type application.
// =============================================================================

// =====================================================================
// From probe_d: Kind unification
// =====================================================================

// TestProbeD_KindUnify_MismatchedKinds — unifying KType with KRow should fail.
func TestProbeD_KindUnify_MismatchedKinds(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KType{}, types.KRow{})
	if err == nil {
		t.Fatal("expected kind mismatch, got nil")
	}
}

// TestProbeD_KindUnify_KindMetaSolving — a kind meta should unify with KType.
func TestProbeD_KindUnify_KindMetaSolving(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	if err := u.UnifyKinds(km, types.KType{}); err != nil {
		t.Fatal(err)
	}
	result := u.ZonkKind(km)
	if _, ok := result.(types.KType); !ok {
		t.Errorf("expected KType, got %s", result)
	}
}

// TestProbeD_KindUnify_KindArrow — unifying (KMeta -> KType) with (KRow -> KType)
// should solve KMeta = KRow.
func TestProbeD_KindUnify_KindArrow(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	a := &types.KArrow{From: km, To: types.KType{}}
	b := &types.KArrow{From: types.KRow{}, To: types.KType{}}
	if err := u.UnifyKinds(a, b); err != nil {
		t.Fatal(err)
	}
	result := u.ZonkKind(km)
	if _, ok := result.(types.KRow); !ok {
		t.Errorf("expected KRow, got %s", result)
	}
}

// TestProbeD_KindUnify_OccursCheck — kind occurs check: ?k ~ (?k -> Type).
func TestProbeD_KindUnify_OccursCheck(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	cycle := &types.KArrow{From: km, To: types.KType{}}
	err := u.UnifyKinds(km, cycle)
	if err == nil {
		t.Fatal("expected kind occurs check, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck for kind, got %v", err)
	}
}

// TestProbeD_KindUnify_KDataMismatch — KData "Nat" vs KData "Bool" should fail.
func TestProbeD_KindUnify_KDataMismatch(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KData{Name: "Nat"}, types.KData{Name: "Bool"})
	if err == nil {
		t.Fatal("expected kind mismatch for different KData, got nil")
	}
}

// TestProbeD_KindUnify_KDataMatch — same KData should unify.
func TestProbeD_KindUnify_KDataMatch(t *testing.T) {
	u := unify.NewUnifier()
	if err := u.UnifyKinds(types.KData{Name: "Nat"}, types.KData{Name: "Nat"}); err != nil {
		t.Fatal(err)
	}
}

// =====================================================================
// From probe_e: Kind unification edge cases
// =====================================================================

// TestProbeE_KindUnify_MetaSelf — kind meta self-unification.
func TestProbeE_KindUnify_MetaSelf(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	if err := u.UnifyKinds(km, km); err != nil {
		t.Errorf("kind meta self-unify should succeed: %v", err)
	}
}

// TestProbeE_KindUnify_OccursCheck — kind occurs check.
func TestProbeE_KindUnify_OccursCheck(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	karrow := &types.KArrow{From: km, To: types.KType{}}
	err := u.UnifyKinds(km, karrow)
	if err == nil {
		t.Fatal("expected kind occurs check error")
	}
}

// TestProbeE_KindUnify_KTypeMismatch — KType vs KRow must fail.
func TestProbeE_KindUnify_KTypeMismatch(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KType{}, types.KRow{})
	if err == nil {
		t.Fatal("expected kind mismatch: Type vs Row")
	}
}

// TestProbeE_KindUnify_ArrowChain — deep KArrow chains should unify.
func TestProbeE_KindUnify_ArrowChain(t *testing.T) {
	u := unify.NewUnifier()
	// Type -> Type -> Type vs ?k1 -> ?k2 -> Type
	k1 := &types.KMeta{ID: 1}
	k2 := &types.KMeta{ID: 2}
	concrete := &types.KArrow{From: types.KType{}, To: &types.KArrow{From: types.KType{}, To: types.KType{}}}
	withMetas := &types.KArrow{From: k1, To: &types.KArrow{From: k2, To: types.KType{}}}
	if err := u.UnifyKinds(concrete, withMetas); err != nil {
		t.Fatalf("expected success: %v", err)
	}
	zk1 := u.ZonkKind(k1)
	if _, ok := zk1.(types.KType); !ok {
		t.Errorf("expected k1 = Type, got %s", zk1)
	}
	zk2 := u.ZonkKind(k2)
	if _, ok := zk2.(types.KType); !ok {
		t.Errorf("expected k2 = Type, got %s", zk2)
	}
}

// TestProbeE_KindUnify_KDataDistinct — KData with different names must not unify.
func TestProbeE_KindUnify_KDataDistinct(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KData{Name: "Color"}, types.KData{Name: "Shape"})
	if err == nil {
		t.Fatal("expected kind mismatch for distinct KData")
	}
}

// TestProbeE_KindUnify_KConstraintVsKType — KConstraint vs KType must fail.
func TestProbeE_KindUnify_KConstraintVsKType(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KConstraint{}, types.KType{})
	if err == nil {
		t.Fatal("expected kind mismatch: Constraint vs Type")
	}
}

// TestProbeE_HKT_KindPolymorphicClass — a class with a higher-kinded
// type parameter should compile.
func TestProbeE_HKT_KindPolymorphicClass(t *testing.T) {
	source := `
data Bool := { True: Bool; False: Bool; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

data Functor := \(f: k -> Type). {
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
data Bool := { True: Bool; False: Bool; }
-- Bool has kind Type, not Type -> Type, so Bool Int is a kind error
f :: Bool Int -> Bool
f := \x. True
main := True
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	// NOTE: This currently does NOT produce an error, which may be a bug.
	// The type annotation `Bool Int` should be a kind error since Bool :: Type,
	// not Type -> Type. We test that it doesn't panic at minimum.
	checkSourceNoPanic(t, source, config)
}
