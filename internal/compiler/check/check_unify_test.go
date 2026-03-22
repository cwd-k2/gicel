// Unify tests — unification (simple, meta, arrow, occurs, row), normalize, zonk.
// Does NOT cover: type family reduction (type_family_reduction_test.go), errors (checker_error_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func TestUnifySimple(t *testing.T) {
	u := unify.NewUnifier()
	if err := u.Unify(types.Con("Int"), types.Con("Int")); err != nil {
		t.Errorf("Int ~ Int should succeed: %v", err)
	}
	if err := u.Unify(types.Con("Int"), types.Con("Bool")); err == nil {
		t.Error("Int ~ Bool should fail")
	}
}

func TestUnifyMeta(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Errorf("?1 ~ Int should succeed: %v", err)
	}
	soln := u.Solve(1)
	if soln == nil {
		t.Fatal("?1 should be solved")
	}
	if con, ok := soln.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected Int, got %v", soln)
	}
}

func TestUnifyArrow(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	a := types.MkArrow(types.Con("Int"), m)
	b := types.MkArrow(types.Con("Int"), types.Con("Bool"))
	if err := u.Unify(a, b); err != nil {
		t.Errorf("should unify: %v", err)
	}
	if !types.Equal(u.Zonk(m), types.Con("Bool")) {
		t.Error("?1 should be Bool")
	}
}

func TestUnifyOccursCheck(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(m, types.MkArrow(m, types.Con("Int"))); err == nil {
		t.Error("should fail: infinite type")
	}
}

func TestUnifyRow(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.ClosedRow(types.RowField{Label: "a", Type: types.Con("Int")})
	r2 := types.ClosedRow(types.RowField{Label: "a", Type: types.Con("Int")})
	if err := u.Unify(r1, r2); err != nil {
		t.Errorf("identical rows should unify: %v", err)
	}
}

func TestUnifyRowOpenOpen(t *testing.T) {
	u := unify.NewUnifier()

	// r1 = { a: Int, b: Bool | ?1 }
	// r2 = { a: Int, c: Str  | ?2 }
	// After unification:
	//   shared: a (Int ~ Int ok)
	//   onlyLeft:  b: Bool  (in r1 not r2)
	//   onlyRight: c: Str   (in r2 not r1)
	//   ?1 = { c: Str | ?fresh }
	//   ?2 = { b: Bool | ?fresh }
	m1 := &types.TyMeta{ID: 100, Kind: types.KRow{}}
	m2 := &types.TyMeta{ID: 101, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "a", Type: types.Con("Int")},
		{Label: "b", Type: types.Con("Bool")},
	}, m1)

	r2 := types.OpenRow([]types.RowField{
		{Label: "a", Type: types.Con("Int")},
		{Label: "c", Type: types.Con("Str")},
	}, m2)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-open row unification should succeed: %v", err)
	}

	// ?1 should be solved to { c: Str | ?fresh }
	soln1 := u.Zonk(m1)
	row1, ok := soln1.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("?1 should be solved to a row, got %T: %s", soln1, types.Pretty(soln1))
	}
	cap1 := row1.Entries.(*types.CapabilityEntries)
	if len(cap1.Fields) != 1 || cap1.Fields[0].Label != "c" {
		t.Errorf("?1 should have field 'c', got %s", types.Pretty(row1))
	}
	if !types.Equal(cap1.Fields[0].Type, types.Con("Str")) {
		t.Errorf("?1.c should be Str, got %s", types.Pretty(cap1.Fields[0].Type))
	}
	if row1.Tail == nil {
		t.Error("?1 should have an open tail (the fresh meta)")
	}

	// ?2 should be solved to { b: Bool | ?fresh }
	soln2 := u.Zonk(m2)
	row2, ok := soln2.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("?2 should be solved to a row, got %T: %s", soln2, types.Pretty(soln2))
	}
	cap2 := row2.Entries.(*types.CapabilityEntries)
	if len(cap2.Fields) != 1 || cap2.Fields[0].Label != "b" {
		t.Errorf("?2 should have field 'b', got %s", types.Pretty(row2))
	}
	if !types.Equal(cap2.Fields[0].Type, types.Con("Bool")) {
		t.Errorf("?2.b should be Bool, got %s", types.Pretty(cap2.Fields[0].Type))
	}
	if row2.Tail == nil {
		t.Error("?2 should have an open tail (the fresh meta)")
	}

	// Both tails should be the same fresh metavariable.
	if row1.Tail != nil && row2.Tail != nil {
		tail1, ok1 := row1.Tail.(*types.TyMeta)
		tail2, ok2 := row2.Tail.(*types.TyMeta)
		if ok1 && ok2 {
			if tail1.ID != tail2.ID {
				t.Errorf("both row tails should share the same fresh meta, got ?%d and ?%d", tail1.ID, tail2.ID)
			}
		}
	}
}

func TestUnifyRowOpenOpenShared(t *testing.T) {
	// Open-Open where both rows have the same labels → tails unify to same fresh.
	u := unify.NewUnifier()

	m1 := &types.TyMeta{ID: 200, Kind: types.KRow{}}
	m2 := &types.TyMeta{ID: 201, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
	}, m1)

	r2 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
	}, m2)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-open row unification (same labels) should succeed: %v", err)
	}

	// Both tails should point to the same fresh meta (with no extra fields).
	soln1 := u.Zonk(m1)
	soln2 := u.Zonk(m2)

	// When both rows have identical fields, the solutions should both be a row
	// with no extra fields and a shared fresh tail.
	row1, ok1 := soln1.(*types.TyEvidenceRow)
	row2, ok2 := soln2.(*types.TyEvidenceRow)
	if ok1 && ok2 {
		ce1 := row1.Entries.(*types.CapabilityEntries)
		if len(ce1.Fields) != 0 {
			t.Errorf("?200 should have no extra fields, got %s", types.Pretty(row1))
		}
		ce2 := row2.Entries.(*types.CapabilityEntries)
		if len(ce2.Fields) != 0 {
			t.Errorf("?201 should have no extra fields, got %s", types.Pretty(row2))
		}
	}
}

func TestUnifyRowOpenOpenDisjoint(t *testing.T) {
	// Open-Open where rows have entirely different labels.
	u := unify.NewUnifier()

	m1 := &types.TyMeta{ID: 300, Kind: types.KRow{}}
	m2 := &types.TyMeta{ID: 301, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "a", Type: types.Con("Int")},
	}, m1)

	r2 := types.OpenRow([]types.RowField{
		{Label: "b", Type: types.Con("Bool")},
	}, m2)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open-open row unification (disjoint labels) should succeed: %v", err)
	}

	// ?1 = { b: Bool | ?fresh }
	soln1 := u.Zonk(m1)
	row1, ok := soln1.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("?300 should be solved to a row, got %s", types.Pretty(soln1))
	}
	cap1 := row1.Entries.(*types.CapabilityEntries)
	if len(cap1.Fields) != 1 || cap1.Fields[0].Label != "b" {
		t.Errorf("?300 should have field 'b', got %s", types.Pretty(row1))
	}

	// ?2 = { a: Int | ?fresh }
	soln2 := u.Zonk(m2)
	row2, ok := soln2.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("?301 should be solved to a row, got %s", types.Pretty(soln2))
	}
	cap2 := row2.Entries.(*types.CapabilityEntries)
	if len(cap2.Fields) != 1 || cap2.Fields[0].Label != "a" {
		t.Errorf("?301 should have field 'a', got %s", types.Pretty(row2))
	}
}

func TestNormalizeCompAppPrePostOrder(t *testing.T) {
	// Computation pre post result as TyApp chain: ((Computation pre) post) result
	// normalizeCompApp must preserve: Pre=pre, Post=post, Result=result.
	u := unify.NewUnifier()
	pre := types.Con("Pre")
	post := types.Con("Post")
	result := types.Con("Result")

	// Build TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)
	appChain := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: &types.TyCon{Name: "Computation"},
				Arg: pre,
			},
			Arg: post,
		},
		Arg: result,
	}

	// Unify with a TyCBPV — the normalize path converts the TyApp chain.
	comp := types.MkComp(pre, post, result)
	if err := u.Unify(appChain, comp); err != nil {
		t.Fatalf("should unify: %v", err)
	}

	// Now test with distinct pre/post — swapping should fail.
	comp2 := types.MkComp(post, pre, result)
	if err := u.Unify(appChain, comp2); err == nil {
		t.Fatal("should fail when pre and post are swapped")
	}
}

func TestNormalizeThunkAppPrePostOrder(t *testing.T) {
	u := unify.NewUnifier()
	pre := types.Con("Pre")
	post := types.Con("Post")
	result := types.Con("Result")

	appChain := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: &types.TyCon{Name: "Thunk"},
				Arg: pre,
			},
			Arg: post,
		},
		Arg: result,
	}

	thunk := types.MkThunk(pre, post, result)
	if err := u.Unify(appChain, thunk); err != nil {
		t.Fatalf("should unify: %v", err)
	}

	thunk2 := types.MkThunk(post, pre, result)
	if err := u.Unify(appChain, thunk2); err == nil {
		t.Fatal("should fail when pre and post are swapped")
	}
}

func TestPatternConArityTooMany(t *testing.T) {
	// Just takes one arg, pattern supplies two → should error.
	source := `data Maybe := \a. { Nothing: (); Just: a; }
f :: Maybe Int -> Int
f := \x. case x { Nothing => 0; Just a b => a }
main := f (Just 42)`
	checkSourceExpectError(t, source, nil)
}

func TestPatternConArityTooFew(t *testing.T) {
	// Pair takes two args, pattern supplies one → should error.
	source := `data Pair := \a b. { MkPair: (a, b); }
f :: Pair Int Int -> Int
f := \x. case x { MkPair a => a }
main := f (MkPair 1 2)`
	checkSourceExpectError(t, source, nil)
}

func TestUnifyRowOpenClosedExtraLabels(t *testing.T) {
	// Open row { x: Int, y: Bool | ?tail } vs closed { x: Int }
	// The open side has extra label y — tail can absorb nothing since closed.
	// But the open row's tail should solve to {} (empty), and y is extra → error.
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 400, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
		{Label: "y", Type: types.Con("Bool")},
	}, m)

	r2 := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})

	if err := u.Unify(r1, r2); err == nil {
		t.Fatal("open row with extra labels should not unify with closed row missing those labels")
	}
}

func TestUnifyRowClosedOpenAbsorbExtra(t *testing.T) {
	// Closed row { x: Int } vs open row { x: Int, y: Bool | ?tail }
	// Reversed direction: same constraint.
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 500, Kind: types.KRow{}}

	r1 := types.ClosedRow(types.RowField{Label: "x", Type: types.Con("Int")})

	r2 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
		{Label: "y", Type: types.Con("Bool")},
	}, m)

	if err := u.Unify(r1, r2); err == nil {
		t.Fatal("closed row should not unify with open row that has extra labels")
	}
}

func TestUnifyRowOpenClosedSubset(t *testing.T) {
	// Open row { x: Int | ?tail } vs closed { x: Int, y: Bool }
	// Closed has extra y — tail absorbs { y: Bool }.
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 600, Kind: types.KRow{}}

	r1 := types.OpenRow([]types.RowField{
		{Label: "x", Type: types.Con("Int")},
	}, m)

	r2 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
		types.RowField{Label: "y", Type: types.Con("Bool")},
	)

	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("open row should absorb extra closed labels into tail: %v", err)
	}
	soln := u.Zonk(m)
	row, ok := soln.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("tail should be solved to a row, got %s", types.Pretty(soln))
	}
	cap := row.Entries.(*types.CapabilityEntries)
	if len(cap.Fields) != 1 || cap.Fields[0].Label != "y" {
		t.Errorf("tail should have field 'y', got %s", types.Pretty(row))
	}
}

func TestZonkPathCompression(t *testing.T) {
	u := unify.NewUnifier()
	// Chain: m1 → m2 → Int
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}
	u.InstallTempSolution(1, m2)
	u.InstallTempSolution(2, types.Con("Int"))

	result := u.Zonk(m1)
	if con, ok := result.(*types.TyCon); !ok || con.Name != "Int" {
		t.Fatalf("expected Int, got %v", result)
	}
	// After path compression, Solve(1) should point directly to Int.
	direct := u.Solve(1)
	if con, ok := direct.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("path compression failed: Solve(1) = %v, expected Int", direct)
	}
}

func TestZonkNoAllocUnchanged(t *testing.T) {
	u := unify.NewUnifier()
	// A type with no metavariables should return the exact same pointer.
	ty := types.MkArrow(types.Con("Int"), types.Con("Bool"))
	result := u.Zonk(ty)
	if result != ty {
		t.Errorf("Zonk of meta-free type should return same pointer")
	}
}
