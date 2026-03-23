package check

import (
	"fmt"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Stress: Large evidence rows — unification
// =============================================================================

func TestStressUnifyLargeCapRows(t *testing.T) {
	const N = 100
	u := unify.NewUnifier()

	fields1 := make([]types.RowField, N)
	fields2 := make([]types.RowField, N)
	for i := range N {
		label := fmt.Sprintf("f%03d", i)
		fields1[i] = types.RowField{Label: label, Type: types.Con("Int")}
		fields2[N-1-i] = types.RowField{Label: label, Type: types.Con("Int")}
	}
	r1 := types.ClosedRow(fields1...)
	r2 := types.ClosedRow(fields2...)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("100-field rows should unify: %v", err)
	}
}

func TestStressUnifyLargeConRows(t *testing.T) {
	const N = 50
	u := unify.NewUnifier()

	entries1 := make([]types.ConstraintEntry, N)
	entries2 := make([]types.ConstraintEntry, N)
	for i := range N {
		cn := fmt.Sprintf("C%03d", i)
		entries1[i] = types.ConstraintEntry{ClassName: cn, Args: []types.Type{types.Con("Int")}}
		entries2[N-1-i] = types.ConstraintEntry{ClassName: cn, Args: []types.Type{types.Con("Int")}}
	}
	r1 := &types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: entries1}}
	r2 := &types.TyEvidenceRow{Entries: &types.ConstraintEntries{Entries: entries2}}

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("50-constraint rows should unify: %v", err)
	}
}

// =============================================================================
// Stress: Open-open with disjoint fields
// =============================================================================

func TestStressOpenOpenDisjoint(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1000, Kind: types.KRow{}}
	m2 := &types.TyMeta{ID: 1001, Kind: types.KRow{}}

	leftFields := make([]types.RowField, 10)
	rightFields := make([]types.RowField, 10)
	for i := range 10 {
		leftFields[i] = types.RowField{Label: fmt.Sprintf("left%d", i), Type: types.Con("Int")}
		rightFields[i] = types.RowField{Label: fmt.Sprintf("right%d", i), Type: types.Con("Bool")}
	}

	r1 := types.OpenRow(leftFields, m1)
	r2 := types.OpenRow(rightFields, m2)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("disjoint open-open should succeed: %v", err)
	}

	soln1 := u.Zonk(m1)
	ev1, ok := soln1.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("m1 solution should be TyEvidenceRow, got %T", soln1)
	}
	if ev1.Entries.EntryCount() != 10 {
		t.Errorf("m1 should have 10 fields, got %d", ev1.Entries.EntryCount())
	}
}

// =============================================================================
// Stress: Deeply nested tail chains
// =============================================================================

func TestStressDeeplyNestedTails(t *testing.T) {
	u := unify.NewUnifier()
	const depth = 20
	metas := make([]*types.TyMeta, depth)
	for i := range depth {
		metas[i] = &types.TyMeta{ID: 2000 + i, Kind: types.KRow{}}
	}

	for i := range depth - 1 {
		r1 := types.OpenRow(
			[]types.RowField{{Label: fmt.Sprintf("f%d", i), Type: types.Con("Int")}},
			metas[i],
		)
		r2 := types.OpenRow(
			[]types.RowField{
				{Label: fmt.Sprintf("f%d", i), Type: types.Con("Int")},
				{Label: fmt.Sprintf("f%d", i+1), Type: types.Con("Int")},
			},
			metas[i+1],
		)
		if err := u.Unify(r1, r2); err != nil {
			t.Fatalf("depth %d unification failed: %v", i, err)
		}
	}

	for i := range depth - 1 {
		soln := u.Zonk(metas[i])
		if _, ok := soln.(*types.TyMeta); ok {
			t.Errorf("meta %d should be solved", i)
		}
	}
}

// =============================================================================
// Stress: Fiber isolation
// =============================================================================

func TestStressFiberIsolation(t *testing.T) {
	u := unify.NewUnifier()
	cap := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})
	con := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})

	if err := u.Unify(cap, con); err == nil {
		t.Error("capability and constraint rows must not unify")
	}
}

// =============================================================================
// Stress: Zonk large evidence rows
// =============================================================================

func TestStressZonkLargeRow(t *testing.T) {
	u := unify.NewUnifier()
	const N = 100
	metas := make([]*types.TyMeta, N)
	for i := range N {
		metas[i] = &types.TyMeta{ID: 3000 + i, Kind: types.KType{}}
		u.InstallTempSolution(3000+i, types.Con(fmt.Sprintf("T%d", i)))
	}

	fields := make([]types.RowField, N)
	for i := range N {
		fields[i] = types.RowField{Label: fmt.Sprintf("f%d", i), Type: metas[i]}
	}
	r := types.ClosedRow(fields...)
	result := u.Zonk(r)

	ev, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	// Fields are sorted by label after ClosedRow normalization.
	// Verify by looking up each field's label → type mapping.
	capFields := ev.CapFields()
	fieldMap := make(map[string]string, len(capFields))
	for _, f := range capFields {
		if c, ok := f.Type.(*types.TyCon); ok {
			fieldMap[f.Label] = c.Name
		}
	}
	for i := range N {
		label := fmt.Sprintf("f%d", i)
		expected := fmt.Sprintf("T%d", i)
		if got, ok := fieldMap[label]; !ok || got != expected {
			t.Errorf("field %s: expected %s, got %s", label, expected, got)
		}
	}
}

// =============================================================================
// Stress: Subst + Equal + FreeVars on large evidence rows
// =============================================================================

func TestStressSubstLargeRow(t *testing.T) {
	const N = 50
	fields := make([]types.RowField, N)
	for i := range N {
		fields[i] = types.RowField{Label: fmt.Sprintf("f%d", i), Type: types.Var("a")}
	}
	r := types.ClosedRow(fields...)
	result := types.Subst(r, "a", types.Con("Int"))

	ev, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	for _, f := range ev.CapFields() {
		if c, ok := f.Type.(*types.TyCon); !ok || c.Name != "Int" {
			t.Errorf("field %s: expected Int, got %s", f.Label, types.Pretty(f.Type))
		}
	}
}

func TestStressEqualLargeRows(t *testing.T) {
	const N = 100
	fields1 := make([]types.RowField, N)
	fields2 := make([]types.RowField, N)
	for i := range N {
		label := fmt.Sprintf("f%03d", i)
		fields1[i] = types.RowField{Label: label, Type: types.Con("Int")}
		fields2[N-1-i] = types.RowField{Label: label, Type: types.Con("Int")}
	}
	r1 := types.ClosedRow(fields1...)
	r2 := types.ClosedRow(fields2...)

	if !types.Equal(r1, r2) {
		t.Error("100-field rows with reversed order should be equal")
	}
}

func TestStressFreeVarsLargeRow(t *testing.T) {
	const N = 50
	fields := make([]types.RowField, N)
	for i := range N {
		fields[i] = types.RowField{Label: fmt.Sprintf("f%d", i), Type: types.Var(fmt.Sprintf("a%d", i))}
	}
	r := types.OpenRow(fields, types.Var("tail"))
	fv := types.FreeVars(r)

	if len(fv) != N+1 {
		t.Errorf("expected %d free vars, got %d", N+1, len(fv))
	}
	if _, ok := fv["tail"]; !ok {
		t.Error("missing tail var")
	}
}

// =============================================================================
// Integration: Evidence rows in full programs
// =============================================================================

func TestSortIntegrationCapabilityProgram(t *testing.T) {
	// Capability rows now flow through TyEvidenceRow.
	source := `form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
f :: Computation { x: Int } { x: Int } Bool
f := do { pure True }
main := f`
	checkSource(t, source, nil)
}

func TestSortIntegrationCapabilityMultiField(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
g :: Computation { x: Int, y: Bool } { x: Int, y: Bool } Bool
g := do { pure True }
main := g`
	checkSource(t, source, nil)
}

func TestSortIntegrationCapabilityRowVar(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
h :: \ (r: Row). Computation { x: Int | r } { x: Int | r } Bool
h := do { pure True }
main := h`
	checkSource(t, source, nil)
}
