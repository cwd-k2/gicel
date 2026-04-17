package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// HKT Stress Tests
// =============================================================================

// TestStressDeepKindNesting — deeply nested kind applications
func TestStressDeepKindNesting(t *testing.T) {
	// \ (k: Kind). \ (j: Kind). \ (f: k -> j -> Type). Int
	source := `
f :: \ (k: Kind). \ (j: Kind). \ (f: k -> j -> Type). Int -> Int
f := \x. x
`
	checkSource(t, source, nil)
}

// TestStressKindPolyIdentityChain — chain of kind-polymorphic identity applications
func TestStressKindPolyIdentityChain(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

id_k :: \ (k: Kind). \ (a: k). a -> a
id_k := \x. x

chain := id_k (id_k (id_k True))
`
	checkSource(t, source, nil)
}

// TestStressKindUnificationLargeArrow — large kind arrow with variables
func TestStressKindUnificationLargeArrow(t *testing.T) {
	u := unify.NewUnifier(&types.TypeOps{})
	// Build k1 -> k2 -> ... -> k20 -> Type with each ki as a kind meta
	var metas []*types.TyMeta
	var k1 types.Type = types.TypeOfTypes
	for i := 19; i >= 0; i-- {
		m := &types.TyMeta{ID: i + 100, Kind: types.SortZero}
		metas = append([]*types.TyMeta{m}, metas...)
		k1 = &types.TyArrow{From: m, To: k1}
	}
	// Build Type -> Type -> ... -> Type -> Type (all concrete)
	var k2 types.Type = types.TypeOfTypes
	for range 20 {
		k2 = &types.TyArrow{From: types.TypeOfTypes, To: k2}
	}
	if err := u.Unify(k1, k2); err != nil {
		t.Fatalf("large kind arrow unification should succeed: %v", err)
	}
	for i, m := range metas {
		solved := u.Zonk(m)
		if !types.Equal(solved, types.TypeOfTypes) {
			t.Errorf("meta %d should be Type, got %s", i, types.PrettyTypeAsKind(solved))
		}
	}
}

// TestStressKindMetaChain — chain of kind meta solutions: ?k1 -> ?k2 -> ... -> ?k50
func TestStressKindMetaChain(t *testing.T) {
	u := unify.NewUnifier(&types.TypeOps{})
	const n = 50
	metas := make([]*types.TyMeta, n)
	for i := range n {
		metas[i] = &types.TyMeta{ID: i + 200, Kind: types.SortZero}
	}
	// Chain: ?k0 ~ ?k1, ?k1 ~ ?k2, ..., ?k48 ~ ?k49
	for i := range n - 1 {
		if err := u.Unify(metas[i], metas[i+1]); err != nil {
			t.Fatalf("chain unify %d ~ %d failed: %v", i, i+1, err)
		}
	}
	// Solve the last one
	if err := u.Unify(metas[n-1], types.TypeOfRows); err != nil {
		t.Fatalf("solve tail failed: %v", err)
	}
	// All should resolve to Row
	for i, m := range metas {
		solved := u.Zonk(m)
		if !types.Equal(solved, types.TypeOfRows) {
			t.Errorf("meta %d should be Row, got %s", i, types.PrettyTypeAsKind(solved))
		}
	}
}

// TestStressSubstKindLarge — Subst with many nested kind-polymorphic foralls
func TestStressSubstKindLarge(t *testing.T) {
	// Build \ (f0: k -> Type). \ (f1: k -> Type). ... \ (f19: k -> Type). Int
	var ty types.Type = types.Con("Int")
	for i := 19; i >= 0; i-- {
		name := fmt.Sprintf("f%d", i)
		ty = &types.TyForall{
			Var:  name,
			Kind: &types.TyArrow{From: &types.TyVar{Name: "k"}, To: types.TypeOfTypes},
			Body: ty,
		}
	}
	result := types.Subst(ty, "k", types.TypeOfRows)
	// Verify all foralls have kind Row -> Type
	current := result
	for i := range 20 {
		f, ok := current.(*types.TyForall)
		if !ok {
			t.Fatalf("expected TyForall at depth %d, got %T", i, current)
		}
		arrow, ok := f.Kind.(*types.TyArrow)
		if !ok {
			t.Fatalf("expected TyArrow at depth %d, got %T", i, f.Kind)
		}
		if !types.Equal(arrow.From, types.TypeOfRows) {
			t.Errorf("depth %d: expected From=Row, got %s", i, types.PrettyTypeAsKind(arrow.From))
		}
		current = f.Body
	}
}

// TestStressPolyKindedClassManyInstances — poly-kinded class with many instances
func TestStressPolyKindedClassManyInstances(t *testing.T) {
	var sb strings.Builder
	// Build 10 data types: D0, D1, ..., D9, each with a type parameter
	for i := range 10 {
		fmt.Fprintf(&sb, "form D%d := \\a. { MkD%d: a -> D%d a; }\n", i, i, i)
	}
	// Poly-kinded Functor
	sb.WriteString("\nform Functor := \\(f: k -> Type). {\n")
	sb.WriteString("  fmap: \\ a b. (a -> b) -> f a -> f b\n")
	sb.WriteString("}\n\n")
	// 10 instances
	for i := range 10 {
		fmt.Fprintf(&sb, "impl Functor D%d := {\n", i)
		fmt.Fprintf(&sb, "  fmap := \\g x. case x { MkD%d v => MkD%d (g v) }\n", i, i)
		sb.WriteString("}\n\n")
	}
	// Use each one
	sb.WriteString("form Bool := { True: Bool; False: Bool; }\n")
	for i := range 10 {
		fmt.Fprintf(&sb, "test%d := fmap (\\x. True) (MkD%d True)\n", i, i)
	}
	checkSource(t, sb.String(), nil)
}

// TestStressKindPolyClassWithContext — poly-kinded class with context constraints
func TestStressKindPolyClassWithContext(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Bool := {
  eq := \x y. True
}

form Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}

form Applicative := \(f: k -> Type). Functor f => {
  pure: \ a. a -> f a
}

impl Applicative Maybe := {
  pure := \x. Just x
}

test_pure := pure True
`
	checkSource(t, source, nil)
}
