package unify

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Zonk — TyEvidenceRow / TyEvidence
// =============================================================================

func TestZonkConstraintRow(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	u.soln[1] = types.Con("Int")

	cr := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{m}},
			},
		},
	}
	result := u.Zonk(cr)
	rc, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	if !types.Equal(rc.ConEntries()[0].Args[0], types.Con("Int")) {
		t.Errorf("expected Eq Int, got Eq %s", types.Pretty(rc.ConEntries()[0].Args[0]))
	}
}

func TestZonkConstraintRowIdentity(t *testing.T) {
	u := NewUnifier()
	cr := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	result := u.Zonk(cr)
	if result != cr {
		t.Error("Zonk of meta-free constraint row should return same pointer")
	}
}

func TestZonkConstraintRowTail(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KConstraint{}}
	remaining := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Ord", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	u.soln[1] = remaining

	cr := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
			},
		},
		Tail: m,
	}
	result := u.Zonk(cr)
	rc, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	if !types.Equal(rc.Tail, remaining) {
		t.Errorf("tail should be zonked to remaining, got %s", types.Pretty(rc.Tail))
	}
}

func TestZonkEvidence(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	u.soln[1] = types.Con("Int")

	ev := &types.TyEvidence{
		Constraints: types.SingleConstraint("Eq", []types.Type{m}),
		Body:        types.MkArrow(m, types.Con("Bool")),
	}
	result := u.Zonk(ev)
	re, ok := result.(*types.TyEvidence)
	if !ok {
		t.Fatalf("expected TyEvidence, got %T", result)
	}
	if !types.Equal(re.Constraints.ConEntries()[0].Args[0], types.Con("Int")) {
		t.Errorf("constraint arg should be Int, got %s", types.Pretty(re.Constraints.ConEntries()[0].Args[0]))
	}
	arr, ok := re.Body.(*types.TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow body, got %T", re.Body)
	}
	if !types.Equal(arr.From, types.Con("Int")) {
		t.Errorf("body From should be Int, got %s", types.Pretty(arr.From))
	}
}

func TestZonkEvidenceIdentity(t *testing.T) {
	u := NewUnifier()
	ev := &types.TyEvidence{
		Constraints: types.SingleConstraint("Eq", []types.Type{types.Con("Int")}),
		Body:        types.Con("Bool"),
	}
	result := u.Zonk(ev)
	if result != ev {
		t.Error("Zonk of meta-free evidence should return same pointer")
	}
}

// =============================================================================
// Unify — TyEvidenceRow (Constraint)
// =============================================================================

func TestUnifyConstraintRowClosedClosed(t *testing.T) {
	u := NewUnifier()
	cr1 := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	cr2 := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	if err := u.Unify(cr1, cr2); err != nil {
		t.Errorf("{Eq Int} ~ {Eq Int} should succeed: %v", err)
	}
}

func TestUnifyConstraintRowClosedMismatch(t *testing.T) {
	u := NewUnifier()
	cr1 := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	cr2 := types.SingleConstraint("Ord", []types.Type{types.Con("Int")})
	if err := u.Unify(cr1, cr2); err == nil {
		t.Error("{Eq Int} ~ {Ord Int} should fail")
	}
}

func TestUnifyConstraintRowOpenClosed(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KConstraint{}}
	// { Eq a | c } ~ { Eq Int, Ord Int }
	mA := &types.TyMeta{ID: 2, Kind: types.KType{}}
	cr1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{mA}},
			},
		},
		Tail: m,
	}
	cr2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
				{ClassName: "Ord", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	if err := u.Unify(cr1, cr2); err != nil {
		t.Fatalf("should succeed: %v", err)
	}
	// a should be solved to Int.
	solnA := u.Zonk(mA)
	if !types.Equal(solnA, types.Con("Int")) {
		t.Errorf("a should be Int, got %s", types.Pretty(solnA))
	}
	// c should be solved to { Ord Int }.
	solnC := u.Zonk(m)
	rc, ok := solnC.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("c should be TyEvidenceRow, got %T: %s", solnC, types.Pretty(solnC))
	}
	ces := rc.ConEntries()
	if len(ces) != 1 || ces[0].ClassName != "Ord" {
		t.Errorf("c should be {Ord Int}, got %s", types.Pretty(rc))
	}
}

func TestUnifyConstraintRowOpenOpen(t *testing.T) {
	u := NewUnifier()
	m1 := &types.TyMeta{ID: 100, Kind: types.KConstraint{}}
	m2 := &types.TyMeta{ID: 101, Kind: types.KConstraint{}}
	mA := &types.TyMeta{ID: 102, Kind: types.KType{}}
	mB := &types.TyMeta{ID: 103, Kind: types.KType{}}

	// { Eq ?a | c1 } ~ { Eq ?b | c2 }
	cr1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{mA}},
			},
		},
		Tail: m1,
	}
	cr2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{mB}},
			},
		},
		Tail: m2,
	}
	// After unification: ?a ~ ?b, c1 = { | fresh }, c2 = { | fresh }
	if err := u.Unify(cr1, cr2); err != nil {
		t.Fatalf("open-open should succeed: %v", err)
	}
	// ?a and ?b should be unified.
	if !types.Equal(u.Zonk(mA), u.Zonk(mB)) {
		t.Errorf("?a and ?b should be unified, got %s and %s",
			types.Pretty(u.Zonk(mA)), types.Pretty(u.Zonk(mB)))
	}
}

func TestUnifyConstraintRowMultiEntry(t *testing.T) {
	u := NewUnifier()
	// { Eq Int, Ord Int } ~ { Ord Int, Eq Int }
	cr1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
				{ClassName: "Ord", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	cr2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Ord", Args: []types.Type{types.Con("Int")}},
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	if err := u.Unify(cr1, cr2); err != nil {
		t.Errorf("same entries in different order should unify: %v", err)
	}
}

func TestUnifyConstraintRowSameClassDifferentArgs(t *testing.T) {
	u := NewUnifier()
	// { Eq Int, Eq Bool } ~ { Eq Bool, Eq Int }
	cr1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
				{ClassName: "Eq", Args: []types.Type{types.Con("Bool")}},
			},
		},
	}
	cr2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Bool")}},
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	if err := u.Unify(cr1, cr2); err != nil {
		t.Errorf("same entries (Eq Int, Eq Bool) in different order should unify: %v", err)
	}
}

func TestUnifyConstraintRowOpenClosedExtraOnOpenSide(t *testing.T) {
	// Open { Eq a, Ord a | c } vs closed { Eq Int }
	// The open side has extra Ord — closed has no tail to absorb it → error.
	u := NewUnifier()
	m := &types.TyMeta{ID: 900, Kind: types.KConstraint{}}
	mA := &types.TyMeta{ID: 901, Kind: types.KType{}}

	cr1 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{mA}},
				{ClassName: "Ord", Args: []types.Type{mA}},
			},
		},
		Tail: m,
	}
	cr2 := &types.TyEvidenceRow{
		Entries: &types.ConstraintEntries{
			Entries: []types.ConstraintEntry{
				{ClassName: "Eq", Args: []types.Type{types.Con("Int")}},
			},
		},
	}
	if err := u.Unify(cr1, cr2); err == nil {
		t.Fatal("open row with extra constraints should not unify with closed row missing them")
	}
}

func TestUnifyEvidence(t *testing.T) {
	u := NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}

	ev1 := &types.TyEvidence{
		Constraints: types.SingleConstraint("Eq", []types.Type{m}),
		Body:        types.MkArrow(m, types.Con("Bool")),
	}
	ev2 := &types.TyEvidence{
		Constraints: types.SingleConstraint("Eq", []types.Type{types.Con("Int")}),
		Body:        types.MkArrow(types.Con("Int"), types.Con("Bool")),
	}
	if err := u.Unify(ev1, ev2); err != nil {
		t.Errorf("evidence unify should succeed: %v", err)
	}
	soln := u.Zonk(m)
	if !types.Equal(soln, types.Con("Int")) {
		t.Errorf("m should be Int, got %s", types.Pretty(soln))
	}
}

func TestUnifyEvidenceMismatch(t *testing.T) {
	u := NewUnifier()
	ev1 := &types.TyEvidence{
		Constraints: types.SingleConstraint("Eq", []types.Type{types.Con("Int")}),
		Body:        types.Con("Bool"),
	}
	ev2 := &types.TyEvidence{
		Constraints: types.SingleConstraint("Ord", []types.Type{types.Con("Int")}),
		Body:        types.Con("Bool"),
	}
	if err := u.Unify(ev1, ev2); err == nil {
		t.Error("different constraints should not unify")
	}
}
