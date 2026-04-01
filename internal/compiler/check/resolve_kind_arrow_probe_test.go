//go:build probe

// Arrow kind resolution probe tests — universe-polymorphic -> edge cases.
// Does NOT cover: basic arrow kind tests (resolve_kind_arrow_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// TyArrow kind with LevelMeta — level inference through arrows
// =============================================================================

func TestProbe_ArrowKind_LevelMetaFrom(t *testing.T) {
	// When From has kind Type(?l), the arrow kind should still resolve.
	lm := &types.LevelMeta{ID: 99}
	fromKind := &types.TyCon{Name: "Type", Level: lm}
	toKind := &types.TyCon{Name: "Type", Level: types.L0}
	result := typeAtMaxLevel(fromKind, toKind)
	tc, ok := result.(*types.TyCon)
	if !ok || tc.Name != "Type" {
		t.Fatalf("expected Type TyCon, got %T", result)
	}
	// Level should be max(?l99, L0), not a hard fallback.
	if _, isMax := tc.Level.(*types.LevelMax); !isMax {
		// Could also be the meta itself if LevelEqual considers them equal (it won't).
		if _, isMeta := tc.Level.(*types.LevelMeta); !isMeta {
			t.Errorf("expected LevelMax or LevelMeta, got %T (%s)", tc.Level, tc.Level.LevelString())
		}
	}
}

func TestProbe_ArrowKind_BothLevelMeta(t *testing.T) {
	// Both sides have LevelMeta — should produce LevelMax.
	lm1 := &types.LevelMeta{ID: 1}
	lm2 := &types.LevelMeta{ID: 2}
	fromKind := &types.TyCon{Name: "Type", Level: lm1}
	toKind := &types.TyCon{Name: "Type", Level: lm2}
	result := typeAtMaxLevel(fromKind, toKind)
	tc, ok := result.(*types.TyCon)
	if !ok || tc.Name != "Type" {
		t.Fatalf("expected Type TyCon, got %T", result)
	}
	lmax, ok := tc.Level.(*types.LevelMax)
	if !ok {
		t.Fatalf("expected LevelMax, got %T", tc.Level)
	}
	if _, ok := lmax.A.(*types.LevelMeta); !ok {
		t.Errorf("expected LevelMeta in A, got %T", lmax.A)
	}
	if _, ok := lmax.B.(*types.LevelMeta); !ok {
		t.Errorf("expected LevelMeta in B, got %T", lmax.B)
	}
}

func TestProbe_ArrowKind_SameLevelMeta(t *testing.T) {
	// Same LevelMeta on both sides — should elide to the meta itself.
	lm := &types.LevelMeta{ID: 1}
	fromKind := &types.TyCon{Name: "Type", Level: lm}
	toKind := &types.TyCon{Name: "Type", Level: lm}
	result := typeAtMaxLevel(fromKind, toKind)
	tc, ok := result.(*types.TyCon)
	if !ok || tc.Name != "Type" {
		t.Fatalf("expected Type TyCon, got %T", result)
	}
	meta, ok := tc.Level.(*types.LevelMeta)
	if !ok {
		t.Fatalf("expected LevelMeta (elided max), got %T", tc.Level)
	}
	if meta.ID != 1 {
		t.Errorf("expected meta ID 1, got %d", meta.ID)
	}
}

// =============================================================================
// Non-Type kinds in arrow positions — fallback to TypeOfTypes
// =============================================================================

func TestProbe_ArrowKind_RowKindFallback(t *testing.T) {
	// If From has kind Row, fallback to TypeOfTypes.
	result := typeAtMaxLevel(types.TypeOfRows, &types.TyCon{Name: "Type", Level: types.L0})
	if result != types.TypeOfTypes {
		t.Errorf("expected TypeOfTypes fallback, got %v", result)
	}
}

func TestProbe_ArrowKind_ConstraintKindFallback(t *testing.T) {
	result := typeAtMaxLevel(types.TypeOfConstraints, types.TypeOfConstraints)
	if result != types.TypeOfTypes {
		t.Errorf("expected TypeOfTypes fallback, got %v", result)
	}
}

// =============================================================================
// Source-level probes — level-polymorphic arrow in type aliases
// =============================================================================

func TestProbe_ArrowKind_LevelPolyAlias(t *testing.T) {
	// type Arrow := \a b. a -> b
	// used at concrete level: Arrow Int Int ~ Int -> Int
	source := `
form Bool := { True: Bool; False: Bool; }
type Arrow := \a b. a -> b
not :: Arrow Bool Bool
not := \b. case b { True => False; False => True }
main := not True
`
	checkSource(t, source, nil)
}

func TestProbe_ArrowKind_LevelPolyAliasHigherKinded(t *testing.T) {
	// Arrow used with higher-kinded arguments.
	source := `
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
type Arrow := \a b. a -> b
f :: Arrow (Maybe Int) (Maybe Int)
f := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

func TestProbe_ArrowKind_ExplicitLevelAnnotation(t *testing.T) {
	// Explicit level quantification with arrow.
	source := `
id :: \(l: Level) (a: Type l). a -> a
id := \x. x
f :: Int -> Int
f := id
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}
