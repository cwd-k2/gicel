// Kind cumulativity tests — ground kinds (Type, Row, Constraint, PromotedDataKind) unify with Sort₀.
// Does NOT cover: general kind unification (kind_unify_test.go).

package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

func TestCumulativityTypeUnifiesWithSort(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.TypeOfTypes, types.SortZero); err != nil {
		t.Fatalf("Type should unify with Kind (Sort₀): %v", err)
	}
}

func TestCumulativitySortUnifiesWithType(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.SortZero, types.TypeOfTypes); err != nil {
		t.Fatalf("Kind (Sort₀) should unify with Type: %v", err)
	}
}

func TestCumulativityRowUnifiesWithSort(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.TypeOfRows, types.SortZero); err != nil {
		t.Fatalf("Row should unify with Kind (Sort₀): %v", err)
	}
}

func TestCumulativityConstraintUnifiesWithSort(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.TypeOfConstraints, types.SortZero); err != nil {
		t.Fatalf("Constraint should unify with Kind (Sort₀): %v", err)
	}
}

func TestCumulativityKDataUnifiesWithSort(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.PromotedDataKind("Bool"), types.SortZero); err != nil {
		t.Fatalf("PromotedDataKind(Bool) should unify with Kind (Sort₀): %v", err)
	}
}

func TestCumulativitySort1DoesNotUnifyWithType(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	if err := u.Unify(types.TypeOfTypes, types.SortAt(1)); err == nil {
		t.Fatal("Type should NOT unify with Sort₃ (level 4)")
	}
}

func TestCumulativityMetaSolvesToGroundKindViaSort(t *testing.T) {
	// Kind meta should be solvable to a ground kind.
	u := NewUnifier(&types.TypeOps{})
	meta := kindMeta(1)
	if err := u.Unify(meta, types.TypeOfTypes); err != nil {
		t.Fatalf("kind meta should solve to Type: %v", err)
	}
	solved := u.Zonk(meta)
	if !types.Equal(solved, types.TypeOfTypes) {
		t.Fatalf("expected Type, got %v", types.PrettyTypeAsKind(solved))
	}
}
