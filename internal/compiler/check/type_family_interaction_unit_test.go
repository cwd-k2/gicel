// Type family interaction tests — unit assertions on typeHasMeta and reduceFamilyApps
// operating directly on Go type objects.
// Does NOT cover: integration scenarios (type_family_interaction_cross_test.go,
//                 type_family_interaction_bug_test.go, type_family_interaction_error_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

func TestInteractionContainsMetaTyFamilyApp(t *testing.T) {
	// A TyFamilyApp with a TyMeta inside should be detected by typeHasMeta.
	meta := &types.TyMeta{ID: 999, Kind: types.TypeOfTypes}
	tfApp := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{meta},
		Kind: types.TypeOfTypes,
	}
	if !typeHasMeta(tfApp) {
		t.Error("typeHasMeta should detect TyMeta inside TyFamilyApp")
	}
}

func TestInteractionContainsMetaTyFamilyAppNoMeta(t *testing.T) {
	// A TyFamilyApp without TyMeta should return false.
	tfApp := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{&types.TyCon{Name: "Int"}},
		Kind: types.TypeOfTypes,
	}
	if typeHasMeta(tfApp) {
		t.Error("typeHasMeta should not detect meta in TyFamilyApp with only TyCon args")
	}
}

func TestInteractionContainsMetaTyFamilyAppNestedMeta(t *testing.T) {
	// TyFamilyApp with meta nested inside a TyApp argument.
	meta := &types.TyMeta{ID: 999, Kind: types.TypeOfTypes}
	tfApp := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{
			&types.TyApp{
				Fun: &types.TyCon{Name: "List"},
				Arg: meta,
			},
		},
		Kind: types.TypeOfTypes,
	}
	if !typeHasMeta(tfApp) {
		t.Error("typeHasMeta should detect TyMeta nested inside TyFamilyApp arg")
	}
}

func TestInteractionContainsMetaTyThunk(t *testing.T) {
	meta := &types.TyMeta{ID: 999, Kind: types.TypeOfTypes}
	thunk := testOps.Thunk(
		types.ClosedRow(),
		types.ClosedRow(),
		meta,
		nil,
	)
	if !typeHasMeta(thunk) {
		t.Error("typeHasMeta should detect TyMeta inside TyCBPV (Thunk) Result")
	}
}

func TestInteractionContainsMetaTyThunkPre(t *testing.T) {
	meta := &types.TyMeta{ID: 999, Kind: types.TypeOfRows}
	thunk := testOps.Thunk(
		meta,
		types.ClosedRow(),
		&types.TyCon{Name: "Unit"},
		nil,
	)
	if !typeHasMeta(thunk) {
		t.Error("typeHasMeta should detect TyMeta inside TyCBPV (Thunk) Pre")
	}
}

func TestInteractionReduceFamilyAppsEvidence(t *testing.T) {
	// Type family application nested inside a TyEvidence body should
	// be reduced by reduceFamilyApps.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

form Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Unit := {
  eq := \x y. True
}

testConstrainedBody :: Elem (List Unit) -> Elem (List Unit) -> Bool
testConstrainedBody := \x y. eq x y
`
	checkSource(t, source, nil)
}

func TestInteractionReduceFamilyAppsCapRow(t *testing.T) {
	// Type family application as the type of a capability in a row.
	// After the fix, reduceFamilyApps recurses into TyEvidenceRow fields.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a
}

step :: Computation { cap: Elem (List Unit) } {} Unit
step := assumption

main :: Computation { cap: Unit } {} Unit
main := step
`
	checkSource(t, source, nil)
}
