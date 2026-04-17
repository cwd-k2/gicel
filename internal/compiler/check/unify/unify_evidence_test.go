package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Zonk — TyEvidenceRow
// =============================================================================

func TestZonkEvidenceRowCapability(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	u.soln[1] = testOps.Con("Int")

	ev := types.ClosedRow(types.RowField{Label: "x", Type: m})
	result := u.Zonk(ev)
	re, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := re.CapFields()
	if !testOps.Equal(fields[0].Type, testOps.Con("Int")) {
		t.Errorf("expected Int, got %s", testOps.Pretty(fields[0].Type))
	}
}

func TestZonkEvidenceRowCapabilityIdentity(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	ev := types.ClosedRow(types.RowField{Label: "x", Type: testOps.Con("Int")})
	result := u.Zonk(ev)
	if result != ev {
		t.Error("Zonk of meta-free evidence row should return same pointer")
	}
}

func TestZonkEvidenceRowConstraint(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	u.soln[1] = testOps.Con("Int")

	ev := types.SingleConstraint("Eq", []types.Type{m})
	result := u.Zonk(ev)
	re, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	entries := re.ConEntries()
	cls := entries[0].(*types.ClassEntry)
	if !testOps.Equal(cls.Args[0], testOps.Con("Int")) {
		t.Errorf("expected Eq Int, got Eq %s", testOps.Pretty(cls.Args[0]))
	}
}

func TestZonkEvidenceRowConstraintIdentity(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	ev := types.SingleConstraint("Eq", []types.Type{testOps.Con("Int")})
	result := u.Zonk(ev)
	if result != ev {
		t.Error("Zonk of meta-free evidence row should return same pointer")
	}
}

func TestZonkEvidenceRowTail(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfRows}
	remaining := types.ClosedRow(types.RowField{Label: "y", Type: testOps.Con("Bool")})
	u.soln[1] = remaining

	ev := types.OpenRow([]types.RowField{{Label: "x", Type: testOps.Con("Int")}}, m)
	result := u.Zonk(ev)
	re, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	if !testOps.Equal(re.Tail, remaining) {
		t.Errorf("tail should be zonked, got %s", testOps.Pretty(re.Tail))
	}
}

// =============================================================================
// Unify — TyEvidenceRow (capability fiber)
// =============================================================================

func TestUnifyEvidenceRowCapClosedClosed(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	r1 := types.ClosedRow(
		types.RowField{Label: "x", Type: testOps.Con("Int")},
		types.RowField{Label: "y", Type: testOps.Con("Bool")},
	)
	r2 := types.ClosedRow(
		types.RowField{Label: "y", Type: testOps.Con("Bool")},
		types.RowField{Label: "x", Type: testOps.Con("Int")},
	)
	if err := u.Unify(r1, r2); err != nil {
		t.Errorf("same fields in different order should unify: %v", err)
	}
}

func TestUnifyEvidenceRowCapClosedMismatch(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	r1 := types.ClosedRow(types.RowField{Label: "x", Type: testOps.Con("Int")})
	r2 := types.ClosedRow(types.RowField{Label: "y", Type: testOps.Con("Int")})
	if err := u.Unify(r1, r2); err == nil {
		t.Error("different labels should not unify")
	}
}

func TestUnifyEvidenceRowCapOpenClosed(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfRows}
	r1 := types.OpenRow([]types.RowField{{Label: "x", Type: testOps.Con("Int")}}, m)
	r2 := types.ClosedRow(
		types.RowField{Label: "x", Type: testOps.Con("Int")},
		types.RowField{Label: "y", Type: testOps.Con("Bool")},
	)
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-closed should succeed: %v", err)
	}
	// m should be solved to { y: Bool }.
	soln := u.Zonk(m)
	re, ok := soln.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T: %s", soln, testOps.Pretty(soln))
	}
	fields := re.CapFields()
	if len(fields) != 1 || fields[0].Label != "y" {
		t.Errorf("expected { y: Bool }, got %s", testOps.Pretty(soln))
	}
}

func TestUnifyEvidenceRowCapOpenOpen(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
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
	if !testOps.Equal(u.Zonk(mA), u.Zonk(mB)) {
		t.Errorf("field types should be unified")
	}
}

// =============================================================================
// Unify — TyEvidenceRow (constraint fiber)
// =============================================================================

func TestUnifyEvidenceRowConClosedClosed(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	r1 := types.SingleConstraint("Eq", []types.Type{testOps.Con("Int")})
	r2 := types.SingleConstraint("Eq", []types.Type{testOps.Con("Int")})
	if err := u.Unify(r1, r2); err != nil {
		t.Errorf("{Eq Int} ~ {Eq Int} should succeed: %v", err)
	}
}

func TestUnifyEvidenceRowConClosedMismatch(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	r1 := types.SingleConstraint("Eq", []types.Type{testOps.Con("Int")})
	r2 := types.SingleConstraint("Ord", []types.Type{testOps.Con("Int")})
	if err := u.Unify(r1, r2); err == nil {
		t.Error("{Eq Int} ~ {Ord Int} should fail")
	}
}

func TestUnifyEvidenceRowConOpenClosed(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfConstraints}
	mA := &types.TyMeta{ID: 2, Kind: types.TypeOfTypes}
	r1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				&types.ClassEntry{ClassName: "Eq", Args: []types.Type{mA}},
			},
		},
		Tail: m,
	}
	r2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				&types.ClassEntry{ClassName: "Eq", Args: []types.Type{testOps.Con("Int")}},
				&types.ClassEntry{ClassName: "Ord", Args: []types.Type{testOps.Con("Int")}},
			},
		},
	}
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("should succeed: %v", err)
	}
	// mA should be solved to Int.
	solnA := u.Zonk(mA)
	if !testOps.Equal(solnA, testOps.Con("Int")) {
		t.Errorf("a should be Int, got %s", testOps.Pretty(solnA))
	}
	// m should be solved to { Ord Int }.
	solnC := u.Zonk(m)
	re, ok := solnC.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("c should be TyEvidenceRow, got %T: %s", solnC, testOps.Pretty(solnC))
	}
	entries := re.ConEntries()
	if len(entries) != 1 || types.HeadClassName(entries[0]) != "Ord" {
		t.Errorf("c should be {Ord Int}, got %s", testOps.Pretty(solnC))
	}
}

func TestUnifyEvidenceRowConMultiEntry(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	r1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				&types.ClassEntry{ClassName: "Eq", Args: []types.Type{testOps.Con("Int")}},
				&types.ClassEntry{ClassName: "Ord", Args: []types.Type{testOps.Con("Int")}},
			},
		},
	}
	r2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				&types.ClassEntry{ClassName: "Ord", Args: []types.Type{testOps.Con("Int")}},
				&types.ClassEntry{ClassName: "Eq", Args: []types.Type{testOps.Con("Int")}},
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
	// Open side has extra y — error with label name.
	u := NewUnifier(&types.TypeOps{})
	m := &types.TyMeta{ID: 800, Kind: types.TypeOfRows}
	r1 := types.OpenRow([]types.RowField{
		{Label: "x", Type: testOps.Con("Int")},
		{Label: "y", Type: testOps.Con("Bool")},
	}, m)
	r2 := types.ClosedRow(types.RowField{Label: "x", Type: testOps.Con("Int")})
	err := u.Unify(r1, r2)
	if err == nil {
		t.Fatal("open row with extra labels should not unify with closed row")
	}
	rme, ok := err.(*RowMismatchError)
	if !ok {
		t.Fatalf("expected *RowMismatchError, got %T", err)
	}
	if len(rme.Labels) != 1 || rme.Labels[0] != "y" {
		t.Errorf("expected Labels=[y], got %v", rme.Labels)
	}
}

func TestUnifyEvidenceRowCapClosedMismatchLabels(t *testing.T) {
	// Closed { x: Int } vs closed { y: Bool } — both sides unmatched.
	u := NewUnifier(&types.TypeOps{})
	r1 := types.ClosedRow(types.RowField{Label: "x", Type: testOps.Con("Int")})
	r2 := types.ClosedRow(types.RowField{Label: "y", Type: testOps.Con("Bool")})
	err := u.Unify(r1, r2)
	if err == nil {
		t.Fatal("different labels should not unify")
	}
	rme, ok := err.(*RowMismatchError)
	if !ok {
		t.Fatalf("expected *RowMismatchError, got %T", err)
	}
	if len(rme.Labels) != 2 {
		t.Errorf("expected 2 labels, got %v", rme.Labels)
	}
}

func TestCapFieldLabelsNil(t *testing.T) {
	labels := capFieldLabels(nil, nil)
	if len(labels) != 0 {
		t.Errorf("expected empty, got %v", labels)
	}
}

func TestCapFieldLabelsOneSide(t *testing.T) {
	a := &types.CapabilityEntries{Fields: []types.RowField{
		{Label: "x"}, {Label: "y"},
	}}
	labels := capFieldLabels(a, nil)
	if len(labels) != 2 || labels[0] != "x" || labels[1] != "y" {
		t.Errorf("expected [x, y], got %v", labels)
	}
}

func TestUnifyEvidenceRowFiberMismatch(t *testing.T) {
	u := NewUnifier(&types.TypeOps{})
	cap := types.EmptyRow()
	con := types.EmptyConstraintRow()
	if err := u.Unify(cap, con); err == nil {
		t.Error("capability and constraint rows should not unify")
	}
}
