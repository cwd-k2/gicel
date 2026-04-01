// Arrow kind resolution tests — universe-polymorphic -> kind computation.
// Does NOT cover: kind unification (resolve_kind_probe_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// extractTypeLevel
// =============================================================================

func TestExtractTypeLevelFromType(t *testing.T) {
	tc := &types.TyCon{Name: "Type", Level: types.L0}
	level, ok := extractTypeLevel(tc)
	if !ok {
		t.Fatal("expected ok for Type TyCon")
	}
	if !types.LevelEqual(level, types.L0) {
		t.Errorf("expected L0, got %s", level.LevelString())
	}
}

func TestExtractTypeLevelFromTypeL1(t *testing.T) {
	level, ok := extractTypeLevel(types.TypeOfTypes) // Type at L1
	if !ok {
		t.Fatal("expected ok for TypeOfTypes")
	}
	if !types.LevelEqual(level, types.L1) {
		t.Errorf("expected L1, got %s", level.LevelString())
	}
}

func TestExtractTypeLevelNilLevel(t *testing.T) {
	tc := &types.TyCon{Name: "Type"} // Level == nil → L0
	level, ok := extractTypeLevel(tc)
	if !ok {
		t.Fatal("expected ok for Type TyCon with nil level")
	}
	if !types.LevelEqual(level, types.L0) {
		t.Errorf("expected L0, got %s", level.LevelString())
	}
}

func TestExtractTypeLevelNotType(t *testing.T) {
	_, ok := extractTypeLevel(types.TypeOfRows) // Row :: Kind
	if ok {
		t.Error("expected !ok for Row")
	}
}

func TestExtractTypeLevelNonTyCon(t *testing.T) {
	_, ok := extractTypeLevel(&types.TyVar{Name: "a"})
	if ok {
		t.Error("expected !ok for TyVar")
	}
}

// =============================================================================
// joinLevel
// =============================================================================

func TestMaxLevelEqual(t *testing.T) {
	result := joinLevel(types.L0, types.L0)
	if !types.LevelEqual(result, types.L0) {
		t.Errorf("expected L0, got %s", result.LevelString())
	}
}

func TestMaxLevelDifferent(t *testing.T) {
	result := joinLevel(types.L0, types.L1)
	lm, ok := result.(*types.LevelMax)
	if !ok {
		t.Fatalf("expected LevelMax, got %T", result)
	}
	if !types.LevelEqual(lm.A, types.L0) || !types.LevelEqual(lm.B, types.L1) {
		t.Errorf("expected max(L0, L1), got max(%s, %s)", lm.A.LevelString(), lm.B.LevelString())
	}
}

func TestMaxLevelVarEqual(t *testing.T) {
	v := &types.LevelVar{Name: "l"}
	result := joinLevel(v, v)
	// max(l, l) = l
	lv, ok := result.(*types.LevelVar)
	if !ok {
		t.Fatalf("expected LevelVar, got %T", result)
	}
	if lv.Name != "l" {
		t.Errorf("expected l, got %s", lv.Name)
	}
}

// =============================================================================
// typeAtMaxLevel
// =============================================================================

func TestTypeAtMaxLevelBothL0(t *testing.T) {
	kindA := &types.TyCon{Name: "Type", Level: types.L0}
	kindB := &types.TyCon{Name: "Type", Level: types.L0}
	result := typeAtMaxLevel(kindA, kindB)
	tc, ok := result.(*types.TyCon)
	if !ok || tc.Name != "Type" {
		t.Fatalf("expected Type TyCon, got %T", result)
	}
	if !types.LevelEqual(tc.Level, types.L0) {
		t.Errorf("expected L0, got %s", tc.Level.LevelString())
	}
}

func TestTypeAtMaxLevelMixed(t *testing.T) {
	kindA := &types.TyCon{Name: "Type", Level: types.L0}
	kindB := &types.TyCon{Name: "Type", Level: types.L1}
	result := typeAtMaxLevel(kindA, kindB)
	tc, ok := result.(*types.TyCon)
	if !ok || tc.Name != "Type" {
		t.Fatalf("expected Type TyCon, got %T", result)
	}
	_, isMax := tc.Level.(*types.LevelMax)
	if !isMax {
		t.Errorf("expected LevelMax, got %T", tc.Level)
	}
}

func TestTypeAtMaxLevelNonType(t *testing.T) {
	// When either kind is non-Type, fall back to TypeOfTypes.
	kindA := &types.TyCon{Name: "Type", Level: types.L0}
	kindB := types.TypeOfRows // Row
	result := typeAtMaxLevel(kindA, kindB)
	if result != types.TypeOfTypes {
		t.Errorf("expected TypeOfTypes, got %v", result)
	}
}

func TestTypeAtMaxLevelNilKind(t *testing.T) {
	kindA := &types.TyCon{Name: "Type", Level: types.L0}
	result := typeAtMaxLevel(kindA, nil)
	if result != types.TypeOfTypes {
		t.Errorf("expected TypeOfTypes for nil kind, got %v", result)
	}
}

// =============================================================================
// Integration: source-level arrow kind with level polymorphism
// =============================================================================

func TestArrowKindBasic(t *testing.T) {
	// Basic function type annotation.
	source := `
form Unit := { MkUnit: Unit; }
f :: Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestArrowKindLevelPolyId(t *testing.T) {
	// Level-polymorphic identity through arrow.
	source := `
id :: \(l: Level) (a: Type l). a -> a
id := \x. x
`
	checkSource(t, source, nil)
}

func TestArrowKindTypeAliasArrow(t *testing.T) {
	// type Arrow := \a b. a -> b should be a valid type alias.
	source := `
type Arrow := \a b. a -> b
f :: Arrow Int Int
f := \x. x
`
	checkSource(t, source, nil)
}

func TestArrowKindFormWithArrowField(t *testing.T) {
	// A form whose constructor field is a function type.
	source := `
form Wrap := \a b. { MkWrap: (a -> b) -> Wrap a b; }
`
	checkSource(t, source, nil)
}

func TestArrowKindNestedArrow(t *testing.T) {
	// Nested arrows: (a -> a) -> a -> a
	source := `
twice :: \a. (a -> a) -> a -> a
twice := \f x. f (f x)
`
	checkSource(t, source, nil)
}

func TestArrowKindDashPipeWithArrow(t *testing.T) {
	// -| combined with ->: F -| A -> F -| A
	source := `
form Box := \a. { MkBox: a -> Box a; }
f :: Box -| Int -> Box -| Int
f := \x. x
`
	checkSource(t, source, nil)
}
