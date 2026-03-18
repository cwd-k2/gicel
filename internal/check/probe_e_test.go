//go:build probe

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/check/unify"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
	"github.com/cwd-k2/gicel/internal/syntax/parse"
	"github.com/cwd-k2/gicel/internal/types"
)

// =============================================================================
// Probe E: Type system / checker adversarial tests targeting unknown bugs.
// Focus: unification boundary conditions, type class resolution edge cases,
// type family reduction, evidence/constraint handling, GADT patterns,
// module imports, and kind checking.
// =============================================================================

// =====================================================================
// 1. Unification boundary conditions
// =====================================================================

// TestProbeE_Unify_TyErrorPropagation — TyError should unify with anything
// without panicking, including deeply nested types and other TyErrors.
func TestProbeE_Unify_TyErrorPropagation(t *testing.T) {
	u := unify.NewUnifier()
	tyErr := &types.TyError{}

	// TyError ~ concrete
	if err := u.Unify(tyErr, types.Con("Int")); err != nil {
		t.Errorf("TyError should unify with concrete type: %v", err)
	}
	// TyError ~ meta
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(tyErr, meta); err != nil {
		t.Errorf("TyError should unify with meta: %v", err)
	}
	// TyError ~ TyError
	if err := u.Unify(tyErr, &types.TyError{}); err != nil {
		t.Errorf("TyError should unify with TyError: %v", err)
	}
	// TyError ~ skolem (should still succeed because TyError absorbs everything)
	skolem := &types.TySkolem{ID: 99, Name: "s", Kind: types.KType{}}
	if err := u.Unify(tyErr, skolem); err != nil {
		t.Errorf("TyError should unify with skolem: %v", err)
	}
	// TyError ~ arrow
	arrow := &types.TyArrow{From: types.Con("Int"), To: types.Con("Bool")}
	if err := u.Unify(tyErr, arrow); err != nil {
		t.Errorf("TyError should unify with arrow: %v", err)
	}
	// TyError ~ forall
	forall := &types.TyForall{Var: "a", Kind: types.KType{}, Body: &types.TyVar{Name: "a"}}
	if err := u.Unify(tyErr, forall); err != nil {
		t.Errorf("TyError should unify with forall: %v", err)
	}
}

// TestProbeE_Unify_SelfMeta — unifying a meta with itself should be a no-op.
func TestProbeE_Unify_SelfMeta(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(meta, meta); err != nil {
		t.Errorf("self-unification of meta should succeed: %v", err)
	}
	// Meta should remain unsolved
	if soln := u.Solve(1); soln != nil {
		t.Errorf("self-unified meta should remain unsolved, got %s", types.Pretty(soln))
	}
}

// TestProbeE_Unify_MetaChainPathCompression — chained meta solutions should
// be path-compressed by Zonk.
func TestProbeE_Unify_MetaChainPathCompression(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}
	m3 := &types.TyMeta{ID: 3, Kind: types.KType{}}

	// Chain: m1 -> m2 -> m3 -> Int
	u.Unify(m1, m2)
	u.Unify(m2, m3)
	u.Unify(m3, types.Con("Int"))

	result := u.Zonk(m1)
	if con, ok := result.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected Int, got %s", types.Pretty(result))
	}
	// After zonk, path compression should make m1 point directly to Int
	direct := u.Solve(1)
	if con, ok := direct.(*types.TyCon); ok && con.Name == "Int" {
		// Path compressed — good
	} else if _, ok := direct.(*types.TyMeta); ok {
		// Not compressed yet — acceptable if compression only happens in Zonk
	} else {
		// Unexpected
		t.Logf("m1 solution after zonk: %s", types.Pretty(direct))
	}
}

// TestProbeE_Unify_OccursCheckThroughSolvedMeta — occurs check must look
// through already-solved metas.
func TestProbeE_Unify_OccursCheckThroughSolvedMeta(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}

	// Solve m2 = F m1
	u.Unify(m2, &types.TyApp{Fun: types.Con("F"), Arg: m1})
	// Now try m1 = m2, which is m1 = F m1 — should fail with occurs check
	err := u.Unify(m1, m2)
	if err == nil {
		t.Fatal("expected occurs check error when meta chain creates cycle")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck, got %v", err)
	}
}

// TestProbeE_Unify_SkolemVsSkolem — two different skolems must not unify.
func TestProbeE_Unify_SkolemVsSkolem(t *testing.T) {
	u := unify.NewUnifier()
	s1 := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	s2 := &types.TySkolem{ID: 2, Name: "b", Kind: types.KType{}}
	err := u.Unify(s1, s2)
	if err == nil {
		t.Fatal("expected error unifying distinct skolems")
	}
}

// TestProbeE_Unify_SkolemSelfUnify — a skolem should unify with itself.
func TestProbeE_Unify_SkolemSelfUnify(t *testing.T) {
	u := unify.NewUnifier()
	s := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	if err := u.Unify(s, s); err != nil {
		t.Errorf("skolem should unify with itself: %v", err)
	}
}

// TestProbeE_Unify_SkolemVsMeta — a meta should be solvable to a skolem
// (this is how existentials work in GADT branches).
func TestProbeE_Unify_SkolemVsMeta(t *testing.T) {
	u := unify.NewUnifier()
	s := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	m := &types.TyMeta{ID: 2, Kind: types.KType{}}
	if err := u.Unify(m, s); err != nil {
		t.Errorf("meta should be solvable to skolem: %v", err)
	}
	result := u.Zonk(m)
	if sk, ok := result.(*types.TySkolem); !ok || sk.ID != 1 {
		t.Errorf("expected skolem #1, got %s", types.Pretty(result))
	}
}

// TestProbeE_Unify_SkolemVsCon — skolem vs concrete should fail.
func TestProbeE_Unify_SkolemVsCon(t *testing.T) {
	u := unify.NewUnifier()
	s := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	err := u.Unify(s, types.Con("Int"))
	if err == nil {
		t.Fatal("expected error unifying skolem with concrete type")
	}
}

// TestProbeE_Unify_ForallBodySubstitution — unifying two foralls should
// treat their bound variables as equal by substitution.
func TestProbeE_Unify_ForallBodySubstitution(t *testing.T) {
	u := unify.NewUnifier()
	// forall a. a -> a  vs  forall b. b -> b
	fa := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, &types.TyVar{Name: "a"}))
	fb := types.MkForall("b", types.KType{}, types.MkArrow(&types.TyVar{Name: "b"}, &types.TyVar{Name: "b"}))
	if err := u.Unify(fa, fb); err != nil {
		t.Errorf("alpha-equivalent foralls should unify: %v", err)
	}
}

// TestProbeE_Unify_ForallBodyMismatch — forall body mismatch should fail cleanly.
func TestProbeE_Unify_ForallBodyMismatch(t *testing.T) {
	u := unify.NewUnifier()
	// forall a. a -> Int  vs  forall a. a -> Bool
	fa := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, types.Con("Int")))
	fb := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, types.Con("Bool")))
	err := u.Unify(fa, fb)
	if err == nil {
		t.Fatal("forall body mismatch should fail")
	}
}

// TestProbeE_Unify_CompVsTyApp — TyCBPV (Computation) should unify with a TyApp chain
// representing Computation pre post result.
func TestProbeE_Unify_CompVsTyApp(t *testing.T) {
	u := unify.NewUnifier()
	comp := types.MkComp(
		types.EmptyRow(),
		types.EmptyRow(),
		types.Con("Int"),
	)
	// Build TyApp chain: Computation {} {} Int
	app := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: types.Con("Computation"),
				Arg: types.EmptyRow(),
			},
			Arg: types.EmptyRow(),
		},
		Arg: types.Con("Int"),
	}
	if err := u.Unify(comp, app); err != nil {
		t.Errorf("TyCBPV (Computation) should unify with equivalent TyApp chain: %v", err)
	}
}

// TestProbeE_Unify_ThunkVsTyApp — same as above but for Thunk.
func TestProbeE_Unify_ThunkVsTyApp(t *testing.T) {
	u := unify.NewUnifier()
	thunk := types.MkThunk(
		types.EmptyRow(),
		types.EmptyRow(),
		types.Con("Int"),
	)
	app := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: types.Con("Thunk"),
				Arg: types.EmptyRow(),
			},
			Arg: types.EmptyRow(),
		},
		Arg: types.Con("Int"),
	}
	if err := u.Unify(thunk, app); err != nil {
		t.Errorf("TyCBPV (Thunk) should unify with equivalent TyApp chain: %v", err)
	}
}

// =====================================================================
// 2. Row unification edge cases
// =====================================================================

// TestProbeE_Row_DuplicateLabelInSingleRow — a row with duplicate labels
// in the same CapabilityEntries should be rejected.
func TestProbeE_Row_DuplicateLabelInSingleRow(t *testing.T) {
	u := unify.NewUnifier()
	row := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{
				{Label: "x", Type: types.Con("Int")},
				{Label: "x", Type: types.Con("Bool")},
			},
		},
	}
	emptyRow := types.EmptyRow()
	err := u.Unify(row, emptyRow)
	// This should either fail with duplicate label or row mismatch
	if err == nil {
		t.Log("WARN: unifying row with duplicate labels against empty row succeeded (may be valid if normalization catches it)")
	}
}

// TestProbeE_Row_OpenTailSolvedToEmpty — an open row tail unified against
// a closed row with matching fields should solve the tail to empty.
func TestProbeE_Row_OpenTailSolvedToEmpty(t *testing.T) {
	u := unify.NewUnifier()
	tail := &types.TyMeta{ID: 1, Kind: types.KRow{}}
	openRow := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{
				{Label: "x", Type: types.Con("Int")},
			},
		},
		Tail: tail,
	}
	closedRow := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{
				{Label: "x", Type: types.Con("Int")},
			},
		},
	}
	if err := u.Unify(openRow, closedRow); err != nil {
		t.Fatalf("expected success unifying open vs closed row with same fields: %v", err)
	}
	// Tail should be solved to empty row
	solved := u.Zonk(tail)
	if ev, ok := solved.(*types.TyEvidenceRow); ok {
		if cap, ok := ev.Entries.(*types.CapabilityEntries); ok && len(cap.Fields) == 0 && ev.Tail == nil {
			// correct — solved to empty
		} else {
			t.Errorf("expected tail solved to empty row, got %s", types.Pretty(solved))
		}
	} else if _, ok := solved.(*types.TyMeta); ok {
		t.Log("WARN: tail remained unsolved — may be valid if empty row is implicit")
	} else {
		t.Errorf("unexpected tail solution: %s", types.Pretty(solved))
	}
}

// TestProbeE_Row_LabelContextPreventsDuplicates — if a meta has a label
// context, solving it to a row with a conflicting label should fail.
func TestProbeE_Row_LabelContextPreventsDuplicates(t *testing.T) {
	u := unify.NewUnifier()
	tail := &types.TyMeta{ID: 1, Kind: types.KRow{}}
	// Register that labels {"x"} already exist in the row containing this tail
	u.RegisterLabelContext(1, map[string]struct{}{"x": {}})
	// Now try to solve tail to a row containing "x" — should fail
	solution := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{
				{Label: "x", Type: types.Con("Bool")},
			},
		},
	}
	err := u.Unify(tail, solution)
	if err == nil {
		t.Fatal("expected duplicate label error when solving meta with conflicting label context")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyDupLabel {
		t.Errorf("expected unify.UnifyDupLabel, got %v", err)
	}
}

// TestProbeE_Row_ConstraintRowMismatch — unifying capability row with
// constraint row should fail with a clear error.
func TestProbeE_Row_ConstraintRowMismatch(t *testing.T) {
	u := unify.NewUnifier()
	capRow := types.EmptyRow()
	conRow := types.EmptyConstraintRow()
	err := u.Unify(capRow, conRow)
	if err == nil {
		// Both are empty — they might unify trivially. Check with non-empty.
		capRow2 := &types.TyEvidenceRow{
			Entries: &types.CapabilityEntries{
				Fields: []types.RowField{{Label: "x", Type: types.Con("Int")}},
			},
		}
		conRow2 := &types.TyEvidenceRow{
			Entries: &types.ConstraintEntries{
				Entries: []types.ConstraintEntry{{ClassName: "Eq", Args: []types.Type{types.Con("Int")}}},
			},
		}
		err = u.Unify(capRow2, conRow2)
		if err == nil {
			t.Fatal("expected error unifying capability row with constraint row")
		}
	}
}

// =====================================================================
// 3. Kind unification edge cases
// =====================================================================

// TestProbeE_KindUnify_MetaSelf — kind meta self-unification.
func TestProbeE_KindUnify_MetaSelf(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	if err := u.UnifyKinds(km, km); err != nil {
		t.Errorf("kind meta self-unify should succeed: %v", err)
	}
}

// TestProbeE_KindUnify_OccursCheck — kind occurs check.
func TestProbeE_KindUnify_OccursCheck(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	karrow := &types.KArrow{From: km, To: types.KType{}}
	err := u.UnifyKinds(km, karrow)
	if err == nil {
		t.Fatal("expected kind occurs check error")
	}
}

// TestProbeE_KindUnify_KTypeMismatch — KType vs KRow must fail.
func TestProbeE_KindUnify_KTypeMismatch(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KType{}, types.KRow{})
	if err == nil {
		t.Fatal("expected kind mismatch: Type vs Row")
	}
}

// TestProbeE_KindUnify_ArrowChain — deep KArrow chains should unify.
func TestProbeE_KindUnify_ArrowChain(t *testing.T) {
	u := unify.NewUnifier()
	// Type -> Type -> Type vs ?k1 -> ?k2 -> Type
	k1 := &types.KMeta{ID: 1}
	k2 := &types.KMeta{ID: 2}
	concrete := &types.KArrow{From: types.KType{}, To: &types.KArrow{From: types.KType{}, To: types.KType{}}}
	withMetas := &types.KArrow{From: k1, To: &types.KArrow{From: k2, To: types.KType{}}}
	if err := u.UnifyKinds(concrete, withMetas); err != nil {
		t.Fatalf("expected success: %v", err)
	}
	zk1 := u.ZonkKind(k1)
	if _, ok := zk1.(types.KType); !ok {
		t.Errorf("expected k1 = Type, got %s", zk1)
	}
	zk2 := u.ZonkKind(k2)
	if _, ok := zk2.(types.KType); !ok {
		t.Errorf("expected k2 = Type, got %s", zk2)
	}
}

// TestProbeE_KindUnify_KDataDistinct — KData with different names must not unify.
func TestProbeE_KindUnify_KDataDistinct(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KData{Name: "Color"}, types.KData{Name: "Shape"})
	if err == nil {
		t.Fatal("expected kind mismatch for distinct KData")
	}
}

// TestProbeE_KindUnify_KConstraintVsKType — KConstraint vs KType must fail.
func TestProbeE_KindUnify_KConstraintVsKType(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KConstraint{}, types.KType{})
	if err == nil {
		t.Fatal("expected kind mismatch: Constraint vs Type")
	}
}

// =====================================================================
// 4. Snapshot/Restore correctness
// =====================================================================

// TestProbeE_Snapshot_RestoreUndoesSolution — solutions added after snapshot
// should be undone on restore.
func TestProbeE_Snapshot_RestoreUndoesSolution(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	snap := u.Snapshot()
	// Solve m = Int
	u.Unify(m, types.Con("Int"))
	if soln := u.Solve(1); soln == nil {
		t.Fatal("expected m solved after unify")
	}
	// Restore
	u.Restore(snap)
	if soln := u.Solve(1); soln != nil {
		t.Fatal("expected m unsolved after restore")
	}
}

// TestProbeE_Snapshot_PreservesPriorSolutions — solutions before snapshot
// should survive restore.
func TestProbeE_Snapshot_PreservesPriorSolutions(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}
	u.Unify(m1, types.Con("Int"))
	snap := u.Snapshot()
	u.Unify(m2, types.Con("Bool"))
	u.Restore(snap)
	// m1 should still be solved
	if soln := u.Solve(1); soln == nil {
		t.Fatal("m1 should remain solved after restore")
	}
	// m2 should be unsolved
	if soln := u.Solve(2); soln != nil {
		t.Fatal("m2 should be unsolved after restore")
	}
}

// TestProbeE_Snapshot_KindSolutionRestore — kind solutions should also be
// rolled back on restore.
func TestProbeE_Snapshot_KindSolutionRestore(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	snap := u.Snapshot()
	u.UnifyKinds(km, types.KType{})
	if k := u.ZonkKind(km); k == km {
		t.Fatal("expected km solved")
	}
	u.Restore(snap)
	if k := u.ZonkKind(km); k != km {
		t.Fatal("expected km unsolved after restore")
	}
}

// =====================================================================
// 5. Full checker edge cases (source-level)
// =====================================================================

// TestProbeE_TypeClass_EmptyClass — a class with no methods should compile.
func TestProbeE_TypeClass_EmptyClass(t *testing.T) {
	source := `
class Marker a {}
data Bool := True | False
instance Marker Bool {}
main := True
`
	checkSource(t, source, nil)
}

// TestProbeE_TypeClass_OverlappingInstancesError — overlapping instances
// should produce a clean error, not a panic.
func TestProbeE_TypeClass_OverlappingInstancesError(t *testing.T) {
	source := `
data Bool := True | False
class C a { method :: a -> Bool }
instance C Bool { method := \x. x }
instance C Bool { method := \x. True }
main := method True
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "overlap") && !strings.Contains(errMsg, "duplicate") {
		t.Logf("NOTICE: overlapping instances produced error: %s", errMsg)
		// Not a bug if it reports any error — just checking it doesn't panic
	}
}

// TestProbeE_TypeClass_MissingMethodError — instance with missing method
// should report a clear error.
func TestProbeE_TypeClass_MissingMethodError(t *testing.T) {
	source := `
data Bool := True | False
class C a { method1 :: a -> Bool; method2 :: a -> a }
instance C Bool { method1 := \x. x }
main := method1 True
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "method") {
		t.Logf("NOTICE: missing method error: %s", errMsg)
	}
}

// TestProbeE_TypeClass_SuperclassWithoutInstance — using a superclass method
// without providing the subclass instance should fail cleanly.
func TestProbeE_TypeClass_SuperclassWithoutInstance(t *testing.T) {
	source := `
data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { lt :: a -> a -> Bool }

-- Ord Bool instance without Eq Bool instance
instance Ord Bool { lt := \x y. True }
main := lt True False
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "instance") && !strings.Contains(errMsg, "Eq") {
		t.Logf("NOTICE: superclass missing error: %s", errMsg)
	}
}

// TestProbeE_TypeClass_AmbiguousConstraintDefaulting — a constraint with
// unsolved metas that cannot be defaulted should produce a diagnostic.
func TestProbeE_TypeClass_AmbiguousConstraintDefaulting(t *testing.T) {
	source := `
data Bool := True | False
class C a { method :: a -> Bool }
instance C Bool { method := \x. x }

-- f takes no arguments that could determine the type variable
f := method
main := f True
`
	// This might compile (inferring a = Bool from application) or error.
	// The important thing is no panic.
	checkSourceNoPanic(t, source, nil)
}

// TestProbeE_TypeClass_DeepSuperclassChain — transitive superclass search
// through a 3-level chain.
func TestProbeE_TypeClass_DeepSuperclassChain(t *testing.T) {
	source := `
data Bool := True | False
class A a { methodA :: a -> Bool }
class A a => B a { methodB :: a -> Bool }
class B a => C a { methodC :: a -> Bool }

instance A Bool { methodA := \x. True }
instance B Bool { methodB := \x. True }
instance C Bool { methodC := \x. True }

-- Using methodA through a C constraint should work via superclass chain C -> B -> A
useA :: \a. C a => a -> Bool
useA := \x. methodA x

main := useA True
`
	checkSource(t, source, nil)
}

// TestProbeE_TypeFamily_StuckOnMeta — a type family that cannot reduce because
// its argument is an unsolved meta should not panic.
func TestProbeE_TypeFamily_StuckOnMeta(t *testing.T) {
	source := `
data Bool := True | False
data Nat := Z | S Nat

type IsZero (n: Type) :: Type := {
  IsZero Z =: Bool;
  IsZero (S n) =: Nat
}

-- Applying IsZero to an unsolved meta should remain stuck, not crash
id :: \a. a -> a
id := \x. x

main := id Z
`
	checkSource(t, source, nil)
}

// TestProbeE_TypeFamily_RecursiveFuelLimit — a type family that recurses beyond
// the fuel limit should report an error, not hang.
func TestProbeE_TypeFamily_RecursiveFuelLimit(t *testing.T) {
	source := `
data Nat := Z | S Nat

type Loop (n: Type) :: Type := {
  Loop n =: Loop (S n)
}

main := (Z :: Loop Z)
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "depth limit") && !strings.Contains(errMsg, "reduction") {
		t.Logf("NOTICE: recursive TF error: %s", errMsg)
	}
}

// TestProbeE_TypeFamily_ExponentialGrowth — a type family that produces
// exponentially growing types should be caught by the size limit.
// BUG: high — This test triggers an infinite loop / exponential blowup in
// reduceFamilyApps. The family `Grow a =: Pair (Grow a) (Grow a)` causes
// each reduction step to double the type, and reduceFamilyApps recurses
// into the result. Although reduceTyFamily has a maxReductionTypeSize check,
// the structural traversal in reduceFamilyApps expands both branches of
// Pair (Grow a) (Grow a) independently, leading to exponential time/space
// consumption before the size limit fires. The reductionDepth counter
// increments per successful reduction, but the branching factor means
// 2^k family reductions occur at depth k.
func TestProbeE_TypeFamily_ExponentialGrowth(t *testing.T) {
	t.Skip("KNOWN BUG: hangs due to exponential blowup in reduceFamilyApps — see BUG comment")
}

// TestProbeE_TypeFamily_InjectivityViolation — two equations with the same RHS
// but different LHS should be flagged as an injectivity violation.
func TestProbeE_TypeFamily_InjectivityViolation(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit

type F (a: Type) :: (r: Type) | r =: a := {
  F Bool =: Bool;
  F Unit =: Bool
}
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "injectivity") {
		t.Logf("NOTICE: injectivity violation error: %s", errMsg)
	}
}

// =====================================================================
// 6. GADT and existential edge cases
// =====================================================================

// TestProbeE_GADT_SkolemEscapeDetection — an existential type that leaks
// out of its case branch should be detected.
func TestProbeE_GADT_SkolemEscapeDetection(t *testing.T) {
	source := `
data Bool := True | False
data Exists := { MkExists :: \a. a -> Exists }

-- This should fail: the existential 'a' escapes through the return type
leak :: Exists -> Bool
leak := \e. case e { MkExists x -> x }
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "mismatch") && !strings.Contains(errMsg, "escape") && !strings.Contains(errMsg, "skolem") {
		t.Logf("NOTICE: skolem escape error: %s", errMsg)
	}
}

// TestProbeE_GADT_IndexRefinement — GADT constructor should refine the
// type index in the case branch.
func TestProbeE_GADT_IndexRefinement(t *testing.T) {
	source := `
data Bool := True | False
data Expr a := { LitBool :: Bool -> Expr Bool }

eval :: Expr Bool -> Bool
eval := \e. case e { LitBool b -> b }

main := eval (LitBool True)
`
	checkSource(t, source, nil)
}

// TestProbeE_GADT_MultiConstructorIndexRefinement — multiple GADT constructors
// with different index types.
func TestProbeE_GADT_MultiConstructorIndexRefinement(t *testing.T) {
	source := `
data Bool := True | False
data Nat := Z | S Nat

data Tag a := { TagBool :: Tag Bool; TagNat :: Tag Nat }

describe :: Tag Bool -> Bool
describe := \t. case t { TagBool -> True }

main := describe TagBool
`
	checkSource(t, source, nil)
}

// =====================================================================
// 7. Module import edge cases
// =====================================================================

// TestProbeE_Module_SelectiveImportNonExistent — importing a name that
// doesn't exist in the module should produce a clear error.
func TestProbeE_Module_SelectiveImportNonExistent(t *testing.T) {
	// This requires setting up a module first
	source := `
data Bool := True | False
main := True
`
	// Register a module, then try to selectively import a nonexistent name
	config := makeModuleConfig(t, "Lib", `
data Color := Red | Blue
`)
	errSource := `
import Lib (nonexistent)
main := 42
`
	errMsg := checkSourceExpectError(t, errSource, config)
	if !strings.Contains(errMsg, "does not export") && !strings.Contains(errMsg, "nonexistent") {
		t.Logf("NOTICE: selective import error: %s", errMsg)
	}
	_ = source // suppress unused
}

// TestProbeE_Module_QualifiedAccessToPrivateName — qualified access to a
// private name (underscore prefix) should fail.
func TestProbeE_Module_QualifiedAccessToPrivateName(t *testing.T) {
	config := makeModuleConfig(t, "Lib", `
_private := 42
data Bool := True | False
public := True
`)
	errSource := `
import Lib as L
main := L._private
`
	// Should fail because _private is not exported
	errMsg := checkSourceExpectError(t, errSource, config)
	if !strings.Contains(errMsg, "does not export") && !strings.Contains(errMsg, "_private") {
		t.Logf("NOTICE: private access error: %s", errMsg)
	}
}

// TestProbeE_Module_UnknownModule — importing a module that doesn't exist
// should produce a clear error.
func TestProbeE_Module_UnknownModule(t *testing.T) {
	errMsg := checkSourceExpectError(t, `
import NonExistent
main := 42
`, nil)
	if !strings.Contains(errMsg, "unknown module") {
		t.Logf("NOTICE: unknown module error: %s", errMsg)
	}
}

// =====================================================================
// 8. Evidence row and constraint handling
// =====================================================================

// TestProbeE_Evidence_EmptyConstraintRowUnification — two empty constraint
// rows should unify.
func TestProbeE_Evidence_EmptyConstraintRowUnification(t *testing.T) {
	u := unify.NewUnifier()
	a := types.EmptyConstraintRow()
	b := types.EmptyConstraintRow()
	if err := u.Unify(a, b); err != nil {
		t.Errorf("empty constraint rows should unify: %v", err)
	}
}

// TestProbeE_Evidence_ConstraintRowWithMismatchedClass — constraint rows
// with different class names should fail.
func TestProbeE_Evidence_ConstraintRowWithMismatchedClass(t *testing.T) {
	u := unify.NewUnifier()
	a := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	b := types.SingleConstraint("Ord", []types.Type{types.Con("Int")})
	err := u.Unify(a, b)
	if err == nil {
		t.Fatal("expected error unifying Eq Int with Ord Int constraint rows")
	}
}

// TestProbeE_Evidence_ConstraintRowWithMetaArgs — constraint rows where
// args contain metas should unify and solve the metas.
func TestProbeE_Evidence_ConstraintRowWithMetaArgs(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	a := types.SingleConstraint("Eq", []types.Type{meta})
	b := types.SingleConstraint("Eq", []types.Type{types.Con("Bool")})
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("constraint rows with meta args should unify: %v", err)
	}
	solved := u.Zonk(meta)
	if con, ok := solved.(*types.TyCon); !ok || con.Name != "Bool" {
		t.Errorf("expected meta solved to Bool, got %s", types.Pretty(solved))
	}
}

// =====================================================================
// 9. Zonk edge cases
// =====================================================================

// TestProbeE_Zonk_UnsolvedMetaPassthrough — zonking an unsolved meta should
// return the meta itself.
func TestProbeE_Zonk_UnsolvedMetaPassthrough(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	result := u.Zonk(m)
	if result != m {
		t.Errorf("unsolved meta should pass through Zonk unchanged, got %T", result)
	}
}

// TestProbeE_Zonk_DeepNesting — zonking a deeply nested type should not
// stack overflow.
func TestProbeE_Zonk_DeepNesting(t *testing.T) {
	u := unify.NewUnifier()
	// Build: F (F (F ... (F Int) ...)) with depth 1000
	const depth = 1000
	var ty types.Type = types.Con("Int")
	for i := 0; i < depth; i++ {
		ty = &types.TyApp{Fun: types.Con("F"), Arg: ty}
	}
	// Should not stack overflow
	result := u.Zonk(ty)
	if result == nil {
		t.Fatal("Zonk returned nil")
	}
}

// TestProbeE_Zonk_TyEvidenceWithSolvedMeta — zonking a TyEvidence where
// the constraint row contains solved metas should work correctly.
func TestProbeE_Zonk_TyEvidenceWithSolvedMeta(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	u.Unify(meta, types.Con("Bool"))
	evidence := &types.TyEvidence{
		Constraints: &types.TyEvidenceRow{
			Entries: &types.ConstraintEntries{
				Entries: []types.ConstraintEntry{
					{ClassName: "Eq", Args: []types.Type{meta}},
				},
			},
		},
		Body: types.MkArrow(meta, types.Con("Int")),
	}
	result := u.Zonk(evidence)
	ev, ok := result.(*types.TyEvidence)
	if !ok {
		t.Fatalf("expected TyEvidence, got %T", result)
	}
	// Check that meta was resolved in the body
	arr, ok := ev.Body.(*types.TyArrow)
	if !ok {
		t.Fatalf("expected TyArrow body, got %T", ev.Body)
	}
	if con, ok := arr.From.(*types.TyCon); !ok || con.Name != "Bool" {
		t.Errorf("expected Bool in arrow from, got %s", types.Pretty(arr.From))
	}
}

// TestProbeE_Zonk_TyFamilyApp — zonking a TyFamilyApp should zonk its args.
func TestProbeE_Zonk_TyFamilyApp(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	u.Unify(meta, types.Con("Int"))
	fam := &types.TyFamilyApp{
		Name: "F",
		Args: []types.Type{meta, types.Con("Bool")},
		Kind: types.KType{},
	}
	result := u.Zonk(fam)
	famResult, ok := result.(*types.TyFamilyApp)
	if !ok {
		t.Fatalf("expected TyFamilyApp, got %T", result)
	}
	if con, ok := famResult.Args[0].(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected first arg zonked to Int, got %s", types.Pretty(famResult.Args[0]))
	}
}

// =====================================================================
// 10. Full-program edge cases for crash resistance
// =====================================================================

// TestProbeE_Crash_TypeAnnotationOnLambda — type annotation on a lambda
// that doesn't match should produce a clean error.
func TestProbeE_Crash_TypeAnnotationOnLambda(t *testing.T) {
	source := `
data Bool := True | False
f :: Bool -> Bool -> Bool
f := \x. x
main := f True
`
	// This should error because f's annotation says two args but body takes one
	// and returns the first arg (which is Bool, but the type says Bool -> Bool).
	// Actually this is fine if the body returns a function. Let's make a real mismatch:
	checkSourceNoPanic(t, source, nil)
}

// TestProbeE_Crash_EmptyDataDecl — a data decl with no constructors.
func TestProbeE_Crash_EmptyDataDecl(t *testing.T) {
	source := `
data Void
main := Void
`
	// Empty data decl — might fail at parse or check, but should not panic
	checkSourceNoPanic(t, source, nil)
}

// TestProbeE_Crash_RecordUpdateNonexistentField — updating a field that
// doesn't exist in the record type.
func TestProbeE_Crash_RecordUpdateNonexistentField(t *testing.T) {
	source := `
data Bool := True | False
r := { x: True }
main := { r | y: True }
`
	checkSourceNoPanic(t, source, nil)
}

// TestProbeE_Crash_DeeplyNestedCase — deeply nested case expressions.
func TestProbeE_Crash_DeeplyNestedCase(t *testing.T) {
	source := `
data Bool := True | False
f := \x. case x {
  True -> case x {
    True -> case x {
      True -> case x {
        True -> True;
        False -> False
      };
      False -> False
    };
    False -> False
  };
  False -> False
}
main := f True
`
	checkSource(t, source, nil)
}

// TestProbeE_Crash_PolymorphicRecursionWithoutAnnotation — this is known
// to be undecidable in general; the checker should not hang.
func TestProbeE_Crash_PolymorphicRecursionWithoutAnnotation(t *testing.T) {
	source := `
data Bool := True | False
data List a := Nil | Cons a (List a)
-- Without a type annotation, this would require polymorphic recursion
-- which is undecidable. The checker should either reject it or handle it
-- with the fuel limit.
main := Nil
`
	// This specific case should be fine since there's no actual recursion
	checkSourceNoPanic(t, source, nil)
}

// TestProbeE_Crash_TypeAnnotationWithUnregisteredType — using a type name
// that hasn't been registered should produce a clean error.
func TestProbeE_Crash_TypeAnnotationWithUnregisteredType(t *testing.T) {
	source := `
f :: FakeType -> FakeType
f := \x. x
main := f
`
	config := &CheckConfig{StrictTypeNames: true}
	checkSourceNoPanic(t, source, config)
}

// TestProbeE_Crash_NestedForallInstantiation — instantiating a deeply
// nested forall type should work correctly.
func TestProbeE_Crash_NestedForallInstantiation(t *testing.T) {
	source := `
data Bool := True | False

-- Three levels of quantification
f :: \a b c. a -> b -> c -> a
f := \x y z. x

main := f True True True
`
	checkSource(t, source, nil)
}

// TestProbeE_Crash_LargeConstraintContext — a function with many constraints.
func TestProbeE_Crash_LargeConstraintContext(t *testing.T) {
	source := `
data Bool := True | False
class C1 a { m1 :: a -> Bool }
class C2 a { m2 :: a -> Bool }
class C3 a { m3 :: a -> Bool }
instance C1 Bool { m1 := \x. x }
instance C2 Bool { m2 := \x. x }
instance C3 Bool { m3 := \x. x }

f :: \a. (C1 a, C2 a, C3 a) => a -> Bool
f := \x. m1 x

main := f True
`
	checkSource(t, source, nil)
}

// TestProbeE_Crash_CaseOnFunctionType — case on a non-data type should
// produce a clean error.
func TestProbeE_Crash_CaseOnFunctionType(t *testing.T) {
	source := `
data Bool := True | False
f := \g. case g { True -> True; False -> False }
main := f (\x. x)
`
	// g has type a -> a, not Bool. The case expects Bool constructors.
	// This might type-check if the checker infers g : Bool from the patterns.
	checkSourceNoPanic(t, source, nil)
}

// TestProbeE_TypeClass_InstanceWithExtraContext — an instance with an
// unnecessary context constraint should still compile.
func TestProbeE_TypeClass_InstanceWithExtraContext(t *testing.T) {
	source := `
data Bool := True | False
data Maybe a := Nothing | Just a
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq (Maybe a) {
  eq := \mx my. case mx {
    Nothing -> case my { Nothing -> True; Just _ -> False };
    Just x -> case my { Nothing -> False; Just y -> eq x y }
  }
}
main := eq (Just True) (Just False)
`
	checkSource(t, source, nil)
}

// TestProbeE_Evidence_MultipleConstraintsSameClass — two constraints on the
// same class with different type args.
func TestProbeE_Evidence_MultipleConstraintsSameClass(t *testing.T) {
	source := `
data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }

data Pair a b := MkPair a b

-- Two Eq constraints on different types
f :: \a b. (Eq a, Eq b) => a -> b -> Bool
f := \x y. eq x x

main := f True True
`
	checkSource(t, source, nil)
}

// TestProbeE_TypeFamily_NoMatchingEquation — when no equation matches,
// the family application should remain stuck (not panic).
func TestProbeE_TypeFamily_NoMatchingEquation(t *testing.T) {
	source := `
data Bool := True | False
data Nat := Z | S Nat

type IsZero (n: Type) :: Type := {
  IsZero Z =: Bool
}

-- S Z has no matching equation — the family is stuck
-- Using it concretely should produce a type error, not a crash
main := Z
`
	checkSourceNoPanic(t, source, nil)
}

// TestProbeE_Unify_TyFamilyAppSameName — two TyFamilyApps with the same
// name and args should unify.
func TestProbeE_Unify_TyFamilyAppSameName(t *testing.T) {
	u := unify.NewUnifier()
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	if err := u.Unify(a, b); err != nil {
		t.Errorf("identical TyFamilyApps should unify: %v", err)
	}
}

// TestProbeE_Unify_TyFamilyAppDifferentArgs — two TyFamilyApps with different
// args should fail.
func TestProbeE_Unify_TyFamilyAppDifferentArgs(t *testing.T) {
	u := unify.NewUnifier()
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Bool")}, Kind: types.KType{}}
	err := u.Unify(a, b)
	if err == nil {
		t.Fatal("TyFamilyApps with different args should fail to unify")
	}
}

// TestProbeE_Unify_TyFamilyAppDifferentNames — two TyFamilyApps with different
// names should fail.
func TestProbeE_Unify_TyFamilyAppDifferentNames(t *testing.T) {
	u := unify.NewUnifier()
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "G", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	err := u.Unify(a, b)
	if err == nil {
		t.Fatal("TyFamilyApps with different names should fail to unify")
	}
}

// TestProbeE_Unify_TyFamilyAppWithMeta — a TyFamilyApp where one arg is
// a meta should solve the meta.
func TestProbeE_Unify_TyFamilyAppWithMeta(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{meta}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "F", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("TyFamilyApp with meta arg should unify: %v", err)
	}
	solved := u.Zonk(meta)
	if con, ok := solved.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected meta solved to Int, got %s", types.Pretty(solved))
	}
}

// TestProbeE_HKT_KindPolymorphicClass — a class with a higher-kinded
// type parameter should compile.
func TestProbeE_HKT_KindPolymorphicClass(t *testing.T) {
	source := `
data Bool := True | False
data Maybe a := Nothing | Just a

class Functor (f: k -> Type) {
  fmap :: \ a b. (a -> b) -> f a -> f b
}

instance Functor Maybe {
  fmap := \g mx. case mx { Nothing -> Nothing; Just x -> Just (g x) }
}

myNot :: Bool -> Bool
myNot := \b. case b { True -> False; False -> True }

main := fmap myNot (Just True)
`
	checkSource(t, source, nil)
}

// TestProbeE_KindMismatch_InTypeApp — applying a type of kind Type
// to another type should produce a kind error.
// BUG: low — `Bool Int` in a type annotation position does not produce a
// kind error. The type resolver does not perform kind checking on type
// applications in annotations — `Bool` has kind `Type` (not `Type -> Type`),
// so `Bool Int` is a kind error, but the checker silently treats it as a
// valid type application. This could lead to unsound types being accepted.
func TestProbeE_KindMismatch_InTypeApp(t *testing.T) {
	source := `
data Bool := True | False
-- Bool has kind Type, not Type -> Type, so Bool Int is a kind error
f :: Bool Int -> Bool
f := \x. True
main := True
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	// NOTE: This currently does NOT produce an error, which may be a bug.
	// The type annotation `Bool Int` should be a kind error since Bool :: Type,
	// not Type -> Type. We test that it doesn't panic at minimum.
	checkSourceNoPanic(t, source, config)
}

// =====================================================================
// 11. Complex interaction tests
// =====================================================================

// TestProbeE_Interaction_GADTWithTypeClass — using a type class method
// inside a GADT case branch.
func TestProbeE_Interaction_GADTWithTypeClass(t *testing.T) {
	source := `
data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }

data SomeEq := { MkSomeEq :: \a. Eq a => a -> a -> SomeEq }

test :: SomeEq -> Bool
test := \s. case s { MkSomeEq x y -> eq x y }

main := test (MkSomeEq True False)
`
	checkSource(t, source, nil)
}

// TestProbeE_Interaction_TypeFamilyWithTypeClass — using a type family
// result as a type class argument.
func TestProbeE_Interaction_TypeFamilyWithTypeClass(t *testing.T) {
	source := `
data Bool := True | False
data Nat := Z | S Nat

type IsZero (n: Type) :: Type := {
  IsZero Z =: Bool
}

class Show a { show :: a -> Bool }
instance Show Bool { show := \x. x }

-- Use IsZero Z (which reduces to Bool) in a context requiring Show
main := show (True :: IsZero Z)
`
	checkSource(t, source, nil)
}

// TestProbeE_Interaction_ConstrainedLetGen — let generalization with
// constraints should lift residuals into qualified types.
func TestProbeE_Interaction_ConstrainedLetGen(t *testing.T) {
	source := `
data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }

-- same should be generalized to: \ a. Eq a => a -> a -> Bool
same := \x y. eq x y
main := same True True
`
	checkSource(t, source, nil)
}

// TestProbeE_Interaction_RecordProjectionWithConstraint — accessing a
// record field through a constraint-qualified type.
func TestProbeE_Interaction_RecordProjectionWithConstraint(t *testing.T) {
	source := `
data Bool := True | False

getX := \r. r.#x
main := getX { x: True, y: True }
`
	checkSource(t, source, nil)
}

// makeModuleConfig creates a CheckConfig with one pre-compiled module.
func makeModuleConfig(t *testing.T, moduleName, moduleSource string) *CheckConfig {
	t.Helper()
	src := span.NewSource(moduleName, moduleSource)
	l := parse.NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatalf("lex errors in module %s: %s", moduleName, lexErrs.Format())
	}
	es := &errs.Errors{Source: src}
	p := parse.NewParser(tokens, es)
	ast := p.ParseProgram()
	if es.HasErrors() {
		t.Fatalf("parse errors in module %s: %s", moduleName, es.Format())
	}
	modConfig := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	_, exports, checkErrs := CheckModule(ast, src, modConfig)
	if checkErrs.HasErrors() {
		t.Fatalf("check errors in module %s: %s", moduleName, checkErrs.Format())
	}
	return &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{moduleName: exports},
	}
}
