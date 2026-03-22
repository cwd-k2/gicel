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
data Bool := { True: (); False: (); }

id_k :: \ (k: Kind). \ (a: k). a -> a
id_k := \x. x

chain := id_k (id_k (id_k True))
`
	checkSource(t, source, nil)
}

// TestStressKindUnificationLargeArrow — large kind arrow with variables
func TestStressKindUnificationLargeArrow(t *testing.T) {
	u := unify.NewUnifier()
	// Build k1 -> k2 -> ... -> k20 -> Type with each ki as a KMeta
	var metas []*types.KMeta
	var k1 types.Kind = types.KType{}
	for i := 19; i >= 0; i-- {
		m := &types.KMeta{ID: i + 100}
		metas = append([]*types.KMeta{m}, metas...)
		k1 = &types.KArrow{From: m, To: k1}
	}
	// Build Type -> Type -> ... -> Type -> Type (all concrete)
	var k2 types.Kind = types.KType{}
	for i := 0; i < 20; i++ {
		k2 = &types.KArrow{From: types.KType{}, To: k2}
	}
	if err := u.UnifyKinds(k1, k2); err != nil {
		t.Fatalf("large kind arrow unification should succeed: %v", err)
	}
	for i, m := range metas {
		solved := u.ZonkKind(m)
		if !solved.Equal(types.KType{}) {
			t.Errorf("meta %d should be Type, got %s", i, solved)
		}
	}
}

// TestStressKindMetaChain — chain of kind meta solutions: ?k1 -> ?k2 -> ... -> ?k50
func TestStressKindMetaChain(t *testing.T) {
	u := unify.NewUnifier()
	const n = 50
	metas := make([]*types.KMeta, n)
	for i := 0; i < n; i++ {
		metas[i] = &types.KMeta{ID: i + 200}
	}
	// Chain: ?k0 ~ ?k1, ?k1 ~ ?k2, ..., ?k48 ~ ?k49
	for i := 0; i < n-1; i++ {
		if err := u.UnifyKinds(metas[i], metas[i+1]); err != nil {
			t.Fatalf("chain unify %d ~ %d failed: %v", i, i+1, err)
		}
	}
	// Solve the last one
	if err := u.UnifyKinds(metas[n-1], types.KRow{}); err != nil {
		t.Fatalf("solve tail failed: %v", err)
	}
	// All should resolve to Row
	for i, m := range metas {
		solved := u.ZonkKind(m)
		if !solved.Equal(types.KRow{}) {
			t.Errorf("meta %d should be Row, got %s", i, solved)
		}
	}
}

// TestStressSubstKindInTypeLarge — SubstKindInType with many nested foralls
func TestStressSubstKindInTypeLarge(t *testing.T) {
	// Build \ (f0: k -> Type). \ (f1: k -> Type). ... \ (f19: k -> Type). Int
	var ty types.Type = types.Con("Int")
	for i := 19; i >= 0; i-- {
		name := fmt.Sprintf("f%d", i)
		ty = &types.TyForall{
			Var:  name,
			Kind: &types.KArrow{From: types.KVar{Name: "k"}, To: types.KType{}},
			Body: ty,
		}
	}
	result := types.SubstKindInType(ty, "k", types.KRow{})
	// Verify all foralls have kind Row -> Type
	current := result
	for i := 0; i < 20; i++ {
		f, ok := current.(*types.TyForall)
		if !ok {
			t.Fatalf("expected TyForall at depth %d, got %T", i, current)
		}
		arrow, ok := f.Kind.(*types.KArrow)
		if !ok {
			t.Fatalf("expected KArrow at depth %d, got %T", i, f.Kind)
		}
		if !arrow.From.Equal(types.KRow{}) {
			t.Errorf("depth %d: expected From=Row, got %s", i, arrow.From)
		}
		current = f.Body
	}
}

// TestStressPolyKindedClassManyInstances — poly-kinded class with many instances
func TestStressPolyKindedClassManyInstances(t *testing.T) {
	var sb strings.Builder
	// Build 10 data types: D0, D1, ..., D9, each with a type parameter
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&sb, "data D%d a := MkD%d a\n", i, i)
	}
	// Poly-kinded Functor
	sb.WriteString("\nclass Functor (f: k -> Type) {\n")
	sb.WriteString("  fmap :: \\ a b. (a -> b) -> f a -> f b\n")
	sb.WriteString("}\n\n")
	// 10 instances
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&sb, "instance Functor D%d {\n", i)
		fmt.Fprintf(&sb, "  fmap := \\g x. case x { MkD%d v => MkD%d (g v) }\n", i, i)
		sb.WriteString("}\n\n")
	}
	// Use each one
	sb.WriteString("data Bool := { True: (); False: (); }\n")
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&sb, "test%d := fmap (\\x. True) (MkD%d True)\n", i, i)
	}
	checkSource(t, sb.String(), nil)
}

// TestStressKindPolyClassWithContext — poly-kinded class with context constraints
func TestStressKindPolyClassWithContext(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
data Maybe := \a. { Nothing: (); Just: a; }

data Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Bool := {
  eq := \x y. True
}

data Functor := \(f: k -> Type). {
  fmap: \ a b. (a -> b) -> f a -> f b
}

impl Functor Maybe := {
  fmap := \g mx. case mx { Nothing => Nothing; Just x => Just (g x) }
}

data Applicative := \(f: k -> Type). Functor f => {
  pure: \ a. a -> f a
}

impl Applicative Maybe := {
  pure := \x. Just x
}

test_pure := pure True
`
	checkSource(t, source, nil)
}
