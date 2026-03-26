// Kind cumulativity tests — ground kinds (Type, Row, Constraint, KData) unify with Sort₀.
// Does NOT cover: general kind unification (kind_unify_test.go).

package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

func TestCumulativityTypeUnifiesWithSort(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KType{}, types.KSort{}); err != nil {
		t.Fatalf("KType should unify with KSort{0}: %v", err)
	}
}

func TestCumulativitySortUnifiesWithType(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KSort{}, types.KType{}); err != nil {
		t.Fatalf("KSort{0} should unify with KType: %v", err)
	}
}

func TestCumulativityRowUnifiesWithSort(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KRow{}, types.KSort{}); err != nil {
		t.Fatalf("KRow should unify with KSort{0}: %v", err)
	}
}

func TestCumulativityConstraintUnifiesWithSort(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KConstraint{}, types.KSort{}); err != nil {
		t.Fatalf("KConstraint should unify with KSort{0}: %v", err)
	}
}

func TestCumulativityKDataUnifiesWithSort(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KData{Name: "Bool"}, types.KSort{}); err != nil {
		t.Fatalf("KData{Bool} should unify with KSort{0}: %v", err)
	}
}

func TestCumulativitySort1DoesNotUnifyWithType(t *testing.T) {
	u := NewUnifier()
	if err := u.UnifyKinds(types.KType{}, types.KSort{Level: 1}); err == nil {
		t.Fatal("KType should NOT unify with KSort{1}")
	}
}

func TestCumulativityMetaSolvesToGroundKindViaSort(t *testing.T) {
	// KMeta should be solvable to a ground kind even when compared via Sort.
	u := NewUnifier()
	meta := &types.KMeta{ID: 1}
	if err := u.UnifyKinds(meta, types.KType{}); err != nil {
		t.Fatalf("KMeta should solve to KType: %v", err)
	}
	solved := u.ZonkKind(meta)
	if _, ok := solved.(types.KType); !ok {
		t.Fatalf("expected KType, got %v", solved)
	}
}
