// Class and family kind resolution tests — kindOfType, hasDeterministicKind, checkTypeAppKind.
// Does NOT cover: label kinds (check_label_kind_test.go), DataKinds (resolve_kind_datakinds_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// kindOfType — unit tests
// =============================================================================

func TestKindOfTypeClass(t *testing.T) {
	// Eq (a: Type) → kind = Type → Constraint
	ch := newTestChecker()
	ch.reg.RegisterClass("Eq", &ClassInfo{
		Name:         "Eq",
		TyParams:     []string{"a"},
		TyParamKinds: []types.Type{types.TypeOfTypes},
	})
	kind := ch.kindOfType(testOps.Con("Eq"))
	expected := &types.TyArrow{From: types.TypeOfTypes, To: types.TypeOfConstraints}
	if !testOps.Equal(kind, expected) {
		t.Errorf("expected %s, got %s", testOps.PrettyTypeAsKind(expected), testOps.PrettyTypeAsKind(kind))
	}
}

func TestKindOfTypeClassHKT(t *testing.T) {
	// Functor (f: Type → Type) → kind = (Type → Type) → Constraint
	ch := newTestChecker()
	fKind := &types.TyArrow{From: types.TypeOfTypes, To: types.TypeOfTypes}
	ch.reg.RegisterClass("Functor", &ClassInfo{
		Name:         "Functor",
		TyParams:     []string{"f"},
		TyParamKinds: []types.Type{fKind},
	})
	kind := ch.kindOfType(testOps.Con("Functor"))
	expected := &types.TyArrow{From: fKind, To: types.TypeOfConstraints}
	if !testOps.Equal(kind, expected) {
		t.Errorf("expected %s, got %s", testOps.PrettyTypeAsKind(expected), testOps.PrettyTypeAsKind(kind))
	}
}

func TestKindOfTypeFamily(t *testing.T) {
	// Elem (a: Type) :: Type → kind = Type → Type
	ch := newTestChecker()
	ch.installFamilyReducer()
	_ = ch.reg.RegisterFamily("Elem", &TypeFamilyInfo{
		Name:       "Elem",
		Params:     []TFParam{{Name: "a", Kind: types.TypeOfTypes}},
		ResultKind: types.TypeOfTypes,
	})
	kind := ch.kindOfType(testOps.Con("Elem"))
	expected := &types.TyArrow{From: types.TypeOfTypes, To: types.TypeOfTypes}
	if !testOps.Equal(kind, expected) {
		t.Errorf("expected %s, got %s", testOps.PrettyTypeAsKind(expected), testOps.PrettyTypeAsKind(kind))
	}
}

func TestKindOfTypeFamilyRowResult(t *testing.T) {
	// Merge (r1: Row) (r2: Row) :: Row → kind = Row → Row → Row
	ch := newTestChecker()
	ch.installFamilyReducer()
	kind := ch.kindOfType(testOps.Con("Merge"))
	expected := &types.TyArrow{
		From: types.TypeOfRows,
		To:   &types.TyArrow{From: types.TypeOfRows, To: types.TypeOfRows},
	}
	if !testOps.Equal(kind, expected) {
		t.Errorf("expected %s, got %s", testOps.PrettyTypeAsKind(expected), testOps.PrettyTypeAsKind(kind))
	}
}

// =============================================================================
// hasDeterministicKind — unit tests
// =============================================================================

func TestHasDeterministicKindClass(t *testing.T) {
	ch := newTestChecker()
	ch.reg.RegisterClass("Eq", &ClassInfo{
		Name:         "Eq",
		TyParams:     []string{"a"},
		TyParamKinds: []types.Type{types.TypeOfTypes},
	})
	if !ch.hasDeterministicKind(testOps.Con("Eq")) {
		t.Error("class should have deterministic kind")
	}
}

func TestHasDeterministicKindFamily(t *testing.T) {
	ch := newTestChecker()
	ch.installFamilyReducer()
	if !ch.hasDeterministicKind(testOps.Con("Merge")) {
		t.Error("builtin row family should have deterministic kind")
	}
}

// =============================================================================
// Source-level — class kind checking
// =============================================================================

func TestKindCheckClassApplicationSource(t *testing.T) {
	// Functor Maybe should kind-check: Functor expects (Type → Type), Maybe has that kind.
	source := `
form Bool := { True: Bool; False: Bool }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a }
form Functor := \(f: Type -> Type). { fmap: \a b. (a -> b) -> f a -> f b }
impl Functor Maybe := { fmap := \fn ma. case ma { Nothing => Nothing; Just a => Just (fn a) } }
main := fmap (\x. True) (Just True)`
	checkSource(t, source, nil)
}

func TestKindMismatchClassArgSource(t *testing.T) {
	// Functor Int should fail: Functor expects (Type → Type), Int has kind Type.
	source := `
form Bool := { True: Bool; False: Bool }
form Functor := \(f: Type -> Type). { fmap: \a b. (a -> b) -> f a -> f b }
impl Functor Bool := { fmap := \fn x. True }
main := True`
	checkSourceExpectError(t, source, nil)
}

// =============================================================================
// Source-level — family kind checking
// =============================================================================

func TestKindCheckFamilyApplicationSource(t *testing.T) {
	// A type family with Row-kinded parameter applied correctly in a type signature.
	source := `
form Bool := { True: Bool; False: Bool }
f :: Without #a { a: Bool, b: Bool } -> Bool
f := \r. True
main := True`
	checkSource(t, source, nil)
}

func TestKindOfTypeFamilyWithLabelParam(t *testing.T) {
	// Without :: Label → Row → Row — verify the kind is computed correctly.
	ch := newTestChecker()
	ch.installFamilyReducer()
	kind := ch.kindOfType(testOps.Con("Without"))
	expected := &types.TyArrow{
		From: types.TypeOfLabels,
		To:   &types.TyArrow{From: types.TypeOfRows, To: types.TypeOfRows},
	}
	if !testOps.Equal(kind, expected) {
		t.Errorf("expected %s, got %s", testOps.PrettyTypeAsKind(expected), testOps.PrettyTypeAsKind(kind))
	}
}
