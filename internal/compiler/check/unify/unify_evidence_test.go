package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Zonk — TyEvidenceRow
// =============================================================================

func TestZonkEvidenceRowCapability(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	u.soln[1] = types.Con("Int")

	ev := types.ClosedRow(types.RowField{Label: "x", Type: m})
	result := u.Zonk(ev)
	re, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := re.CapFields()
	if !types.Equal(fields[0].Type, types.Con("Int")) {
		t.Errorf("expected Int, got %s", types.Pretty(fields[0].Type))
	}
}

func TestZonkEvidenceRowCapabilityIdentity(t *testing.T) {
	u := NewUnifier()
	ev := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})
	result := u.Zonk(ev)
	if result != ev {
		t.Error("Zonk of meta-free evidence row should return same pointer")
	}
}

func TestZonkEvidenceRowConstraint(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	u.soln[1] = types.Con("Int")

	ev := types.SingleConstraint("Eq", []types.Type{m})
	result := u.Zonk(ev)
	re, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	entries := re.ConEntries()
	if !types.Equal(entries[0].Args[0], types.Con("Int")) {
		t.Errorf("expected Eq Int, got Eq %s", types.Pretty(entries[0].Args[0]))
	}
}

func TestZonkEvidenceRowConstraintIdentity(t *testing.T) {
	u := NewUnifier()
	ev := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	result := u.Zonk(ev)
	if result != ev {
		t.Error("Zonk of meta-free evidence row should return same pointer")
	}
}

func TestZonkEvidenceRowTail(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfRows}
	remaining := types.ClosedRow(types.RowField{Label: "y", Type: types.Con("Bool")})
	u.soln[1] = remaining

	ev := types.OpenRow([]types.RowField{{Label: "x", Type: types.Con("Int")}}, m)
	result := u.Zonk(ev)
	re, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	if !types.Equal(re.Tail, remaining) {
		t.Errorf("tail should be zonked, got %s", types.Pretty(re.Tail))
	}
}

// =============================================================================
// Unify — TyEvidenceRow (capability fiber)
// =============================================================================

func TestUnifyEvidenceRowCapClosedClosed(t *testing.T) {
	u := NewUnifier()
	r1 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
		types.RowField{Label: "y", Type: types.Con("Bool")},
	)
	r2 := types.ClosedRow(
		types.RowField{Label: "y", Type: types.Con("Bool")},
		types.RowField{Label: "x", Type: types.Con("Int")},
	)
	if err := u.Unify(r1, r2); err != nil {
		t.Errorf("same fields in different order should unify: %v", err)
	}
}

func TestUnifyEvidenceRowCapClosedMismatch(t *testing.T) {
	u := NewUnifier()
	r1 := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})
	r2 := types.ClosedRow(types.RowField{Label: "y", Type: types.Con("Int")})
	if err := u.Unify(r1, r2); err == nil {
		t.Error("different labels should not unify")
	}
}

func TestUnifyEvidenceRowCapOpenClosed(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfRows}
	r1 := types.OpenRow([]types.RowField{{Label: "x", Type: types.Con("Int")}}, m)
	r2 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
		types.RowField{Label: "y", Type: types.Con("Bool")},
	)
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-closed should succeed: %v", err)
	}
	// m should be solved to { y: Bool }.
	soln := u.Zonk(m)
	re, ok := soln.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T: %s", soln, types.Pretty(soln))
	}
	fields := re.CapFields()
	if len(fields) != 1 || fields[0].Label != "y" {
		t.Errorf("expected { y: Bool }, got %s", types.Pretty(soln))
	}
}

func TestUnifyEvidenceRowCapOpenOpen(t *testing.T) {
	u := NewUnifier()
	m1 := &types.TyMeta{ID: 100, Kind: types.TypeOfRows}
	m2 := &types.TyMeta{ID: 101, Kind: types.TypeOfRows}
	mA := &types.TyMeta{ID: 102, Kind: types.TypeOfTypes}
	mB := &types.TyMeta{ID: 103, Kind: types.TypeOfTypes}

	r1 := types.OpenRow([]types.RowField{{Label: "x", Type: mA}}, m1)
	r2 := types.OpenRow([]types.RowField{{Label: "x", Type: mB}}, m2)
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-open should succeed: %v", err)
	}
	// mA and mB should be unified.
	if !types.Equal(u.Zonk(mA), u.Zonk(mB)) {
		t.Errorf("field types should be unified")
	}
}

// =============================================================================
// Unify — TyEvidenceRow (constraint fiber)
// =============================================================================

func TestUnifyEvidenceRowConClosedClosed(t *testing.T) {
	u := NewUnifier()
	r1 := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	r2 := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	if err := u.Unify(r1, r2); err != nil {
		t.Errorf("{Eq Int} ~ {Eq Int} should succeed: %v", err)
	}
}

func TestUnifyEvidenceRowConClosedMismatch(t *testing.T) {
	u := NewUnifier()
	r1 := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	r2 := types.SingleConstraint("Ord", []types.Type{types.Con("Int")})
	if err := u.Unify(r1, r2); err == nil {
		t.Error("{Eq Int} ~ {Ord Int} should fail")
	}
}

func TestUnifyEvidenceRowConOpenClosed(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfConstraints}
	mA := &types.TyMeta{ID: 2, Kind: types.TypeOfTypes}
	r1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{mA}},
			},
		},
		Tail: m,
	}
	r2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
				{ClassName: "Ord", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("should succeed: %v", err)
	}
	// mA should be solved to Int.
	solnA := u.Zonk(mA)
	if !types.Equal(solnA, types.Con("Int")) {
		t.Errorf("a should be Int, got %s", types.Pretty(solnA))
	}
	// m should be solved to { Ord Int }.
	solnC := u.Zonk(m)
	re, ok := solnC.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("c should be TyEvidenceRow, got %T: %s", solnC, types.Pretty(solnC))
	}
	entries := re.ConEntries()
	if len(entries) != 1 || entries[0].ClassName != "Ord" {
		t.Errorf("c should be {Ord Int}, got %s", types.Pretty(solnC))
	}
}

func TestUnifyEvidenceRowConMultiEntry(t *testing.T) {
	u := NewUnifier()
	r1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
				{ClassName: "Ord", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	r2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Ord", Args: []types.Type{types.Con("Int")}},
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	if err := u.Unify(r1, r2); err != nil {
		t.Errorf("same entries in different order should unify: %v", err)
	}
}

// =============================================================================
// Unify — TyEvidenceRow fiber mismatch
// =============================================================================

func TestUnifyEvidenceRowCapOpenClosedExtraOnOpenSide(t *testing.T) {
	// Open { x: Int, y: Bool | tail } vs closed { x: Int }
	// Open side has extra y — error.
	u := NewUnifier()
	m := &types.TyMeta{ID: 800, Kind: types.TypeOfRows}
	r1 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
		{Label: "y", Type: types.Con("Bool")},
	}, m)
	r2 := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})
	if err := u.Unify(r1, r2); err == nil {
		t.Fatal("open row with extra labels should not unify with closed row")
	}
}

func TestUnifyEvidenceRowFiberMismatch(t *testing.T) {
	u := NewUnifier()
	cap := types.EmptyRow()
	con := types.EmptyConstraintRow()
	if err := u.Unify(cap, con); err == nil {
		t.Error("capability and constraint rows should not unify")
	}
}
