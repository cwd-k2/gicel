//go:build probe

package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/check/unify"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// =============================================================================
// Probe D: Deep unifier, snapshot/restore, kind unification, instance resolution
// depth, type family reduction, constraint resolution, skolem rigidity,
// evidence row handling, zonk cycles, and module-scoped resolution.
// =============================================================================

// =====================================================================
// 1. Unification edge cases
// =====================================================================

// TestProbeD_Unify_OccursCheckDirect — unifying ?m with (List ?m) must
// trigger the occurs check and return an error.
func TestProbeD_Unify_OccursCheckDirect(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	listMeta := &types.TyApp{Fun: &types.TyCon{Name: "List"}, Arg: meta}
	err := u.Unify(meta, listMeta)
	if err == nil {
		t.Fatal("expected occurs check error, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck, got %v", err)
	}
}

// TestProbeD_Unify_OccursCheckDeepNesting — occurs check through a chain
// of three levels: ?m ~ F (G (H ?m)).
func TestProbeD_Unify_OccursCheckDeepNesting(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	nested := &types.TyApp{
		Fun: types.Con("F"),
		Arg: &types.TyApp{
			Fun: types.Con("G"),
			Arg: &types.TyApp{
				Fun: types.Con("H"),
				Arg: meta,
			},
		},
	}
	err := u.Unify(meta, nested)
	if err == nil {
		t.Fatal("expected occurs check error for deeply nested meta, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck, got %v", err)
	}
}

// TestProbeD_Unify_DeeplyNestedTyAppChain — unifying two deeply nested
// TyApp chains should succeed when structurally equal.
func TestProbeD_Unify_DeeplyNestedTyAppChain(t *testing.T) {
	u := unify.NewUnifier()
	// Build: F (F (F (... (F Int) ...)))  depth=20
	const depth = 20
	buildChain := func() types.Type {
		var ty types.Type = types.Con("Int")
		for i := 0; i < depth; i++ {
			ty = &types.TyApp{Fun: types.Con("F"), Arg: ty}
		}
		return ty
	}
	a := buildChain()
	b := buildChain()
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("unifying identical deep chains should succeed: %v", err)
	}
}

// TestProbeD_Unify_TyAppChainMetaSolving — unifying F ?m with F Int
// should solve ?m = Int, then Zonk should reveal Int.
func TestProbeD_Unify_TyAppChainMetaSolving(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	a := &types.TyApp{Fun: types.Con("F"), Arg: meta}
	b := &types.TyApp{Fun: types.Con("F"), Arg: types.Con("Int")}
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("expected success: %v", err)
	}
	solved := u.Zonk(meta)
	if con, ok := solved.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected meta solved to Int, got %s", types.Pretty(solved))
	}
}

// TestProbeD_Unify_RowOverlappingLabels — unifying two closed rows with
// overlapping labels. Fields with the same label must have compatible types.
func TestProbeD_Unify_RowOverlappingLabels(t *testing.T) {
	u := unify.NewUnifier()
	row1 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
		types.RowField{Label: "y", Type: types.Con("Bool")},
		types.RowField{Label: "z", Type: types.Con("String")},
	)
	row2 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
		types.RowField{Label: "y", Type: types.Con("Bool")},
		types.RowField{Label: "z", Type: types.Con("String")},
	)
	if err := u.Unify(row1, row2); err != nil {
		t.Fatalf("identical closed rows should unify: %v", err)
	}
}

// TestProbeD_Unify_RowMismatchedFieldType — unifying two closed rows where
// one field has a different type should fail.
func TestProbeD_Unify_RowMismatchedFieldType(t *testing.T) {
	u := unify.NewUnifier()
	row1 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Int")},
	)
	row2 := types.ClosedRow(
		types.RowField{Label: "x", Type: types.Con("Bool")},
	)
	err := u.Unify(row1, row2)
	if err == nil {
		t.Fatal("expected row type mismatch, got nil")
	}
}

// TestProbeD_Unify_TwoTyFamilyAppNodes — unifying two TyFamilyApp nodes
// with the same family name should unify their arguments pairwise.
func TestProbeD_Unify_TwoTyFamilyAppNodes(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	fam1 := &types.TyFamilyApp{Name: "Elem", Args: []types.Type{meta}, Kind: types.KType{}}
	fam2 := &types.TyFamilyApp{Name: "Elem", Args: []types.Type{types.Con("List")}, Kind: types.KType{}}
	if err := u.Unify(fam1, fam2); err != nil {
		t.Fatalf("unifying same-family TyFamilyApp should succeed: %v", err)
	}
	solved := u.Zonk(meta)
	if con, ok := solved.(*types.TyCon); !ok || con.Name != "List" {
		t.Errorf("expected meta solved to List, got %s", types.Pretty(solved))
	}
}

// TestProbeD_Unify_DifferentTyFamilyAppNames — unifying two TyFamilyApp nodes
// with different names should fail.
func TestProbeD_Unify_DifferentTyFamilyAppNames(t *testing.T) {
	u := unify.NewUnifier()
	fam1 := &types.TyFamilyApp{Name: "Elem", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	fam2 := &types.TyFamilyApp{Name: "Wrap", Args: []types.Type{types.Con("Int")}, Kind: types.KType{}}
	err := u.Unify(fam1, fam2)
	if err == nil {
		t.Fatal("expected mismatch for different family names, got nil")
	}
}

// =====================================================================
// 2. Snapshot/Restore correctness
// =====================================================================

// TestProbeD_SnapshotRestore_SolutionRollback — unify a meta, snapshot,
// unify another meta, restore, verify second meta is unsolved.
func TestProbeD_SnapshotRestore_SolutionRollback(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}

	// Solve m1 = Int
	if err := u.Unify(m1, types.Con("Int")); err != nil {
		t.Fatal(err)
	}

	snap := u.Snapshot()

	// Solve m2 = Bool (after snapshot)
	if err := u.Unify(m2, types.Con("Bool")); err != nil {
		t.Fatal(err)
	}
	// Verify m2 is solved
	if _, ok := u.Zonk(m2).(*types.TyCon); !ok {
		t.Fatal("m2 should be solved before restore")
	}

	u.Restore(snap)

	// m1 should still be solved (it was before snapshot)
	z1 := u.Zonk(m1)
	if con, ok := z1.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("m1 should still be Int after restore, got %s", types.Pretty(z1))
	}

	// m2 should be unsolved (it was solved after snapshot)
	z2 := u.Zonk(m2)
	if _, ok := z2.(*types.TyMeta); !ok {
		t.Errorf("m2 should be unsolved after restore, got %s", types.Pretty(z2))
	}
}

// TestProbeD_SnapshotRestore_LabelContextRollback — label context registered
// after snapshot should be rolled back.
func TestProbeD_SnapshotRestore_LabelContextRollback(t *testing.T) {
	u := unify.NewUnifier()

	snap := u.Snapshot()

	// Register labels after snapshot
	u.RegisterLabelContext(42, map[string]struct{}{"x": {}, "y": {}})
	if len(u.Labels()[42]) != 2 {
		t.Fatal("expected 2 labels before restore")
	}

	u.Restore(snap)

	// Labels for meta 42 should be gone
	if _, exists := u.Labels()[42]; exists {
		t.Error("label context for meta 42 should be removed after restore")
	}
}

// TestProbeD_SnapshotRestore_KindSolutionRollback — kind meta solutions
// after snapshot should be rolled back.
func TestProbeD_SnapshotRestore_KindSolutionRollback(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 10}

	snap := u.Snapshot()

	if err := u.UnifyKinds(km, types.KType{}); err != nil {
		t.Fatal(err)
	}
	// Verify solved
	zonked := u.ZonkKind(km)
	if _, ok := zonked.(types.KType); !ok {
		t.Fatal("kind meta should be solved before restore")
	}

	u.Restore(snap)

	// Kind meta should be unsolved
	zonked = u.ZonkKind(km)
	if _, ok := zonked.(*types.KMeta); !ok {
		t.Errorf("kind meta should be unsolved after restore, got %s", zonked)
	}
}

// TestProbeD_SnapshotRestore_MultipleSnapshotsNested — nested snapshot/restore
// should work correctly (restore inner, then outer).
func TestProbeD_SnapshotRestore_MultipleSnapshotsNested(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}
	m3 := &types.TyMeta{ID: 3, Kind: types.KType{}}

	if err := u.Unify(m1, types.Con("A")); err != nil {
		t.Fatal(err)
	}
	snap1 := u.Snapshot()

	if err := u.Unify(m2, types.Con("B")); err != nil {
		t.Fatal(err)
	}
	snap2 := u.Snapshot()

	if err := u.Unify(m3, types.Con("C")); err != nil {
		t.Fatal(err)
	}

	// Restore inner: m3 unsolved, m1 and m2 solved
	u.Restore(snap2)
	if _, ok := u.Zonk(m3).(*types.TyMeta); !ok {
		t.Error("m3 should be unsolved after restoring snap2")
	}
	if con, ok := u.Zonk(m2).(*types.TyCon); !ok || con.Name != "B" {
		t.Error("m2 should still be B after restoring snap2")
	}

	// Restore outer: m2 and m3 unsolved, m1 solved
	u.Restore(snap1)
	if _, ok := u.Zonk(m2).(*types.TyMeta); !ok {
		t.Error("m2 should be unsolved after restoring snap1")
	}
	if con, ok := u.Zonk(m1).(*types.TyCon); !ok || con.Name != "A" {
		t.Error("m1 should still be A after restoring snap1")
	}
}

// =====================================================================
// 3. Kind unification
// =====================================================================

// TestProbeD_KindUnify_MismatchedKinds — unifying KType with KRow should fail.
func TestProbeD_KindUnify_MismatchedKinds(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KType{}, types.KRow{})
	if err == nil {
		t.Fatal("expected kind mismatch, got nil")
	}
}

// TestProbeD_KindUnify_KindMetaSolving — a kind meta should unify with KType.
func TestProbeD_KindUnify_KindMetaSolving(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	if err := u.UnifyKinds(km, types.KType{}); err != nil {
		t.Fatal(err)
	}
	result := u.ZonkKind(km)
	if _, ok := result.(types.KType); !ok {
		t.Errorf("expected KType, got %s", result)
	}
}

// TestProbeD_KindUnify_KindArrow — unifying (KMeta -> KType) with (KRow -> KType)
// should solve KMeta = KRow.
func TestProbeD_KindUnify_KindArrow(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	a := &types.KArrow{From: km, To: types.KType{}}
	b := &types.KArrow{From: types.KRow{}, To: types.KType{}}
	if err := u.UnifyKinds(a, b); err != nil {
		t.Fatal(err)
	}
	result := u.ZonkKind(km)
	if _, ok := result.(types.KRow); !ok {
		t.Errorf("expected KRow, got %s", result)
	}
}

// TestProbeD_KindUnify_OccursCheck — kind occurs check: ?k ~ (?k -> Type).
func TestProbeD_KindUnify_OccursCheck(t *testing.T) {
	u := unify.NewUnifier()
	km := &types.KMeta{ID: 1}
	cycle := &types.KArrow{From: km, To: types.KType{}}
	err := u.UnifyKinds(km, cycle)
	if err == nil {
		t.Fatal("expected kind occurs check, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyOccursCheck {
		t.Errorf("expected unify.UnifyOccursCheck for kind, got %v", err)
	}
}

// TestProbeD_KindUnify_KDataMismatch — KData "Nat" vs KData "Bool" should fail.
func TestProbeD_KindUnify_KDataMismatch(t *testing.T) {
	u := unify.NewUnifier()
	err := u.UnifyKinds(types.KData{Name: "Nat"}, types.KData{Name: "Bool"})
	if err == nil {
		t.Fatal("expected kind mismatch for different KData, got nil")
	}
}

// TestProbeD_KindUnify_KDataMatch — same KData should unify.
func TestProbeD_KindUnify_KDataMatch(t *testing.T) {
	u := unify.NewUnifier()
	if err := u.UnifyKinds(types.KData{Name: "Nat"}, types.KData{Name: "Nat"}); err != nil {
		t.Fatal(err)
	}
}

// =====================================================================
// 4. Instance resolution depth
// =====================================================================

// TestProbeD_InstanceDepth_NearLimit — 15 levels of nested contextual
// instance resolution. maxResolveDepth = 64, so 15 is well within range.
func TestProbeD_InstanceDepth_NearLimit(t *testing.T) {
	const N = 15
	var sb strings.Builder
	sb.WriteString("data Bool := True | False\n")
	sb.WriteString("class Eq a { eq :: a -> a -> Bool }\n")
	sb.WriteString("instance Eq Bool { eq := \\x y. True }\n\n")

	for i := 0; i < N; i++ {
		sb.WriteString(fmt.Sprintf("data W%d a := MkW%d a\n", i, i))
		sb.WriteString(fmt.Sprintf("instance Eq a => Eq (W%d a) { eq := \\x y. True }\n\n", i))
	}

	// Build W0 (W1 (... (W14 Bool) ...))
	inner := "True"
	for i := N - 1; i >= 0; i-- {
		inner = fmt.Sprintf("(MkW%d %s)", i, inner)
	}
	sb.WriteString(fmt.Sprintf("main := eq %s %s\n", inner, inner))
	checkSource(t, sb.String(), nil)
}

// TestProbeD_InstanceDepth_SelfRecursiveSameType — an instance whose context
// requires itself at the same type should be rejected at instance registration.
func TestProbeD_InstanceDepth_SelfRecursiveSameType(t *testing.T) {
	source := `
data Bool := True | False
class C a { m :: a -> Bool }
instance C a => C a { m := \x. True }
main := m True
`
	checkSourceExpectError(t, source, nil)
}

// =====================================================================
// 5. Type family reduction
// =====================================================================

// TestProbeD_TF_StuckOnUnsolvedMeta — a type family application stuck on
// a meta should not crash and should leave the type as TyFamilyApp or meta.
func TestProbeD_TF_StuckOnUnsolvedMeta(t *testing.T) {
	source := `
data Unit := Unit
data Bool := True | False
data List a := Nil | Cons a (List a)

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

-- f has polymorphic arg, Elem reduction is deferred.
f :: \ a. a -> Elem a
f := \x. x
`
	// This should fail: Elem a cannot reduce because a is a bound variable
	// but the identity function tries to return x :: a as Elem a.
	// The type family remains stuck since a is a skolem, not List _,
	// producing a mismatch between a and Elem a (stuck TyFamilyApp).
	checkSourceExpectError(t, source, nil)
}

// TestProbeD_TF_RecursiveFamilyFuelExhausted — a recursive family that
// never terminates should either hit the depth/budget limit and error,
// or leave the family stuck (both sides equal) and type-check successfully.
// The node budget in reduceFamilyApps may curtail expansion before the
// fuel limit fires, leaving Loop Z stuck on both sides of the identity.
func TestProbeD_TF_RecursiveFamilyFuelExhausted(t *testing.T) {
	source := `
data Nat := Z | S Nat
data Phantom (n: Nat) := MkPhantom

type Loop (a: Nat) :: Nat := {
  Loop a =: Loop (S a)
}

f :: Phantom (Loop Z) -> Phantom (Loop Z)
f := \x. x
`
	// Either outcome is acceptable: error (fuel/budget exhausted during
	// reduction) or success (stuck type unifies with itself).
	_ = checkSource(t, source, nil)
}

// TestProbeD_TF_IdentityFamily — a trivial family that returns its argument.
func TestProbeD_TF_IdentityFamily(t *testing.T) {
	source := `
data Bool := True | False

type Id (a: Type) :: Type := {
  Id a =: a
}

f :: Id Bool -> Bool
f := \x. x

main := f True
`
	checkSource(t, source, nil)
}

// TestProbeD_TF_TwoArgumentFamily — a family with two parameters.
func TestProbeD_TF_TwoArgumentFamily(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit

type Fst (a: Type) (b: Type) :: Type := {
  Fst a b =: a
}

f :: Fst Bool Unit -> Bool
f := \x. x

main := f True
`
	checkSource(t, source, nil)
}

// =====================================================================
// 6. Constraint resolution
// =====================================================================

// TestProbeD_Constraint_MissingInstance — using a method without an instance
// should produce ErrNoInstance.
func TestProbeD_Constraint_MissingInstance(t *testing.T) {
	source := `
data Bool := True | False
data Pair a b := MkPair a b

class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }

-- No Eq instance for Pair.
main := eq (MkPair True False) (MkPair False True)
`
	checkSourceExpectCode(t, source, nil, errs.ErrNoInstance)
}

// TestProbeD_Constraint_OverlappingInstances — two instances for the exact
// same type should produce ErrOverlap.
func TestProbeD_Constraint_OverlappingInstances(t *testing.T) {
	source := `
data Bool := True | False
class Show a { show :: a -> a }
instance Show Bool { show := \x. x }
instance Show Bool { show := \x. True }
main := show False
`
	checkSourceExpectCode(t, source, nil, errs.ErrOverlap)
}

// TestProbeD_Constraint_SuperclassResolutionChain — C3 => C2 => C1,
// using a C1 method with only a C3 constraint, through 3 levels.
func TestProbeD_Constraint_SuperclassResolutionChain(t *testing.T) {
	source := `
data Bool := True | False

class C1 a { m1 :: a -> Bool }
class C1 a => C2 a { m2 :: a -> Bool }
class C2 a => C3 a { m3 :: a -> Bool }

instance C1 Bool { m1 := \x. True }
instance C2 Bool { m2 := \x. True }
instance C3 Bool { m3 := \x. True }

f :: \ a. C3 a => a -> Bool
f := \x. m1 x

main := f True
`
	checkSource(t, source, nil)
}

// TestProbeD_Constraint_MultiParamClass — a multi-parameter type class
// with functional dependency.
func TestProbeD_Constraint_MultiParamClass(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit

class Convert a b | a =: b { convert :: a -> b }
instance Convert Bool Unit { convert := \x. Unit }

main := convert True
`
	checkSource(t, source, nil)
}

// =====================================================================
// 7. Skolem rigidity
// =====================================================================

// TestProbeD_Skolem_RigidUnifyWithConcrete — trying to unify a skolem
// with a concrete type should produce an error.
func TestProbeD_Skolem_RigidUnifyWithConcrete(t *testing.T) {
	u := unify.NewUnifier()
	sk := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	err := u.Unify(sk, types.Con("Int"))
	if err == nil {
		t.Fatal("expected skolem rigidity error, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifySkolemRigid {
		t.Errorf("expected unify.UnifySkolemRigid, got %v", err)
	}
}

// TestProbeD_Skolem_SameSkolemUnifies — unifying a skolem with itself
// should succeed.
func TestProbeD_Skolem_SameSkolemUnifies(t *testing.T) {
	u := unify.NewUnifier()
	sk := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	if err := u.Unify(sk, sk); err != nil {
		t.Fatalf("same skolem should unify: %v", err)
	}
}

// TestProbeD_Skolem_DifferentSkolemsRefuse — two different skolems should
// not unify.
func TestProbeD_Skolem_DifferentSkolemsRefuse(t *testing.T) {
	u := unify.NewUnifier()
	sk1 := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	sk2 := &types.TySkolem{ID: 2, Name: "b", Kind: types.KType{}}
	err := u.Unify(sk1, sk2)
	if err == nil {
		t.Fatal("different skolems should not unify")
	}
}

// TestProbeD_Skolem_MetaSolvedToSkolemInRow — a meta inside a row solved
// to contain a skolem should be detected if the skolem escapes.
func TestProbeD_Skolem_MetaSolvedToSkolemInRow(t *testing.T) {
	u := unify.NewUnifier()
	sk := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	meta := &types.TyMeta{ID: 2, Kind: types.KType{}}

	// Solve meta = F skolem
	wrapped := &types.TyApp{Fun: types.Con("F"), Arg: sk}
	if err := u.Unify(meta, wrapped); err != nil {
		t.Fatal(err)
	}

	// Verify: zonking the meta reveals the skolem
	zonked := u.Zonk(meta)
	// Verify structurally that the skolem is present:
	app, ok := zonked.(*types.TyApp)
	if !ok {
		t.Fatal("expected TyApp after zonk")
	}
	if s, ok := app.Arg.(*types.TySkolem); !ok || s.ID != 1 {
		t.Errorf("expected skolem a in solution, got %s", types.Pretty(zonked))
	}
}

// TestProbeD_Skolem_EscapeInExistential — existential type variable should
// not escape through case pattern.
func TestProbeD_Skolem_EscapeInExistential(t *testing.T) {
	source := `
data Bool := True | False
data Exists := { MkExists :: \ a. a -> Exists }

-- Trying to return the existentially-bound value should fail.
bad :: Exists -> Bool
bad := \e. case e { MkExists x -> x }
`
	checkSourceExpectError(t, source, nil)
}

// =====================================================================
// 8. Evidence row handling
// =====================================================================

// TestProbeD_Evidence_EmptyConstraintRowUnify — two empty constraint rows
// should unify.
func TestProbeD_Evidence_EmptyConstraintRowUnify(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.EmptyConstraintRow()
	r2 := types.EmptyConstraintRow()
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("empty constraint rows should unify: %v", err)
	}
}

// TestProbeD_Evidence_SingleConstraintRowUnify — two single-entry constraint
// rows with the same class and args should unify.
func TestProbeD_Evidence_SingleConstraintRowUnify(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.SingleConstraint("Eq", []types.Type{types.Con("Bool")})
	r2 := types.SingleConstraint("Eq", []types.Type{types.Con("Bool")})
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("matching single constraint rows should unify: %v", err)
	}
}

// TestProbeD_Evidence_ConstraintRowMismatch — two constraint rows with
// different class names should fail.
func TestProbeD_Evidence_ConstraintRowMismatch(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.SingleConstraint("Eq", []types.Type{types.Con("Bool")})
	r2 := types.SingleConstraint("Ord", []types.Type{types.Con("Bool")})
	err := u.Unify(r1, r2)
	if err == nil {
		t.Fatal("different constraint classes should not unify")
	}
}

// TestProbeD_Evidence_NestedEvidenceType — a TyEvidence containing a
// TyEvidence body should be handled without crash.
func TestProbeD_Evidence_NestedEvidenceType(t *testing.T) {
	source := `
data Bool := True | False

class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }

instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }

-- Function with nested constraint usage.
f :: \ a. (Eq a, Ord a) => a -> a -> (Bool, Bool)
f := \x y. (eq x y, compare x y)

main := f True False
`
	checkSource(t, source, nil)
}

// TestProbeD_Evidence_EmptyCapabilityRowUnify — two empty capability rows
// should unify.
func TestProbeD_Evidence_EmptyCapabilityRowUnify(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.EmptyRow()
	r2 := types.EmptyRow()
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("empty capability rows should unify: %v", err)
	}
}

// =====================================================================
// 9. Zonk with cycles and path compression
// =====================================================================

// TestProbeD_Zonk_MetaChainPathCompression — a chain m1 -> m2 -> m3 -> Int
// should be compressed so m1 points directly to Int after Zonk.
func TestProbeD_Zonk_MetaChainPathCompression(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.KType{}}
	m2 := &types.TyMeta{ID: 2, Kind: types.KType{}}
	m3 := &types.TyMeta{ID: 3, Kind: types.KType{}}

	// Build chain: m1 -> m2 -> m3 -> Int
	u.InstallTempSolution(1, m2)
	u.InstallTempSolution(2, m3)
	u.InstallTempSolution(3, types.Con("Int"))

	result := u.Zonk(m1)
	if con, ok := result.(*types.TyCon); !ok || con.Name != "Int" {
		t.Fatalf("expected Int, got %s", types.Pretty(result))
	}

	// After Zonk, m1's solution should be path-compressed to Int directly
	directSoln := u.Solve(1)
	if con, ok := directSoln.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("path compression failed: m1 still points to %s", types.Pretty(directSoln))
	}
}

// TestProbeD_Zonk_UnsolvedMetaPreserved — zonking an unsolved meta returns
// the meta itself.
func TestProbeD_Zonk_UnsolvedMetaPreserved(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 42, Kind: types.KType{}}
	result := u.Zonk(m)
	if tm, ok := result.(*types.TyMeta); !ok || tm.ID != 42 {
		t.Errorf("expected unsolved meta ?42, got %s", types.Pretty(result))
	}
}

// TestProbeD_Zonk_StructuralIdentity — zonking a type with no metas should
// return the identical pointer.
func TestProbeD_Zonk_StructuralIdentity(t *testing.T) {
	u := unify.NewUnifier()
	ty := types.MkArrow(types.Con("Int"), types.Con("Bool"))
	result := u.Zonk(ty)
	if result != ty {
		t.Error("Zonk should return the same pointer for meta-free types")
	}
}

// TestProbeD_Zonk_TyForallBodyZonked — zonking a forall should zonk the body.
func TestProbeD_Zonk_TyForallBodyZonked(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.KType{}}
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Fatal(err)
	}
	forallTy := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, m))
	result := u.Zonk(forallTy)
	f, ok := result.(*types.TyForall)
	if !ok {
		t.Fatal("expected TyForall")
	}
	arr, ok := f.Body.(*types.TyArrow)
	if !ok {
		t.Fatal("expected TyArrow in body")
	}
	if con, ok := arr.To.(*types.TyCon); !ok || con.Name != "Int" {
		t.Errorf("expected Int in return position, got %s", types.Pretty(arr.To))
	}
}

// =====================================================================
// 10. Module-scoped resolution
// =====================================================================

// TestProbeD_Module_QualifiedVarLookup — qualified variable lookup
// from an imported module.
func TestProbeD_Module_QualifiedVarLookup(t *testing.T) {
	modExports := &ModuleExports{
		Types:         map[string]types.Kind{"Int": types.KType{}},
		ConTypes:      map[string]types.Type{},
		ConInfo:       map[string]*DataTypeInfo{},
		Aliases:       map[string]*AliasInfo{},
		Classes:       map[string]*ClassInfo{},
		Values:        map[string]types.Type{"add": types.MkArrow(types.Con("Int"), types.MkArrow(types.Con("Int"), types.Con("Int")))},
		DataDecls:     nil,
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"MathLib": modExports},
	}
	source := `
import MathLib as M

main := M.add 1 2
`
	checkSource(t, source, config)
}

// TestProbeD_Module_QualifiedConLookup — qualified constructor lookup.
func TestProbeD_Module_QualifiedConLookup(t *testing.T) {
	modExports := probeModuleExports("Color", []string{"Red", "Blue"})
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"Colors": modExports},
	}
	source := `
import Colors as C
data Bool := True | False

f :: C.Color -> Bool
f := \x. case x { C.Red -> True; C.Blue -> False }

main := f C.Red
`
	checkSource(t, source, config)
}

// TestProbeD_Module_UnknownQualifierError — referencing a qualifier not
// in scope should produce a clear error.
func TestProbeD_Module_UnknownQualifierError(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	source := `
main := X.foo
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "unknown qualifier") && !strings.Contains(errMsg, "X") {
		t.Errorf("expected error about unknown qualifier X, got: %s", errMsg)
	}
}

// TestProbeD_Module_QualifiedShadowLocal — a local binding should NOT
// shadow a qualified import (qualified names are always explicit).
func TestProbeD_Module_QualifiedShadowLocal(t *testing.T) {
	modExports := &ModuleExports{
		Types:         map[string]types.Kind{"Int": types.KType{}},
		ConTypes:      map[string]types.Type{},
		ConInfo:       map[string]*DataTypeInfo{},
		Aliases:       map[string]*AliasInfo{},
		Classes:       map[string]*ClassInfo{},
		Values:        map[string]types.Type{"val": types.Con("Int")},
		DataDecls:     nil,
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}, "Bool": types.KType{}},
		ImportedModules: map[string]*ModuleExports{"Lib": modExports},
	}
	source := `
import Lib as L

-- Local "val" of different type
data Bool := True | False
val :: Bool
val := True

-- L.val should still refer to the module's Int-typed val, not local Bool-typed val
main := L.val
`
	checkSource(t, source, config)
}

// =====================================================================
// Additional edge cases
// =====================================================================

// TestProbeD_Unify_SkolemVsMeta — a meta should be solvable to a skolem
// (metas CAN be solved to skolems; the escape check is separate).
func TestProbeD_Unify_SkolemVsMeta(t *testing.T) {
	u := unify.NewUnifier()
	sk := &types.TySkolem{ID: 1, Name: "a", Kind: types.KType{}}
	meta := &types.TyMeta{ID: 2, Kind: types.KType{}}
	if err := u.Unify(meta, sk); err != nil {
		t.Fatalf("meta should be solvable to skolem: %v", err)
	}
	solved := u.Zonk(meta)
	if s, ok := solved.(*types.TySkolem); !ok || s.ID != 1 {
		t.Errorf("expected skolem a, got %s", types.Pretty(solved))
	}
}

// TestProbeD_Unify_TyCompCrossCase — unifying TyCBPV (Computation) with an equivalent
// TyApp chain should succeed through the cross-case unification path.
func TestProbeD_Unify_TyCompCrossCase(t *testing.T) {
	u := unify.NewUnifier()
	comp := types.MkComp(
		types.EmptyRow(),
		types.EmptyRow(),
		types.Con("Int"),
	)
	// Build: TyApp(TyApp(TyApp(TyCon("Computation"), emptyRow), emptyRow), Int)
	appChain := &types.TyApp{
		Fun: &types.TyApp{
			Fun: &types.TyApp{
				Fun: types.Con("Computation"),
				Arg: types.EmptyRow(),
			},
			Arg: types.EmptyRow(),
		},
		Arg: types.Con("Int"),
	}
	if err := u.Unify(comp, appChain); err != nil {
		t.Fatalf("TyCBPV (Computation) should unify with equivalent TyApp chain: %v", err)
	}
}

// TestProbeD_Unify_TyErrorAbsorbs — TyError should unify with anything
// (error recovery behavior).
func TestProbeD_Unify_TyErrorAbsorbs(t *testing.T) {
	u := unify.NewUnifier()
	if err := u.Unify(&types.TyError{}, types.Con("Int")); err != nil {
		t.Fatalf("TyError should absorb: %v", err)
	}
	if err := u.Unify(types.Con("Bool"), &types.TyError{}); err != nil {
		t.Fatalf("TyError should absorb (reversed): %v", err)
	}
}

// TestProbeD_Unify_ForallBodiesAlphaEquivalent — unifying two foralls
// with different bound variable names but alpha-equivalent bodies.
func TestProbeD_Unify_ForallBodiesAlphaEquivalent(t *testing.T) {
	u := unify.NewUnifier()
	// \a. a -> a
	f1 := types.MkForall("a", types.KType{}, types.MkArrow(&types.TyVar{Name: "a"}, &types.TyVar{Name: "a"}))
	// \b. b -> b
	f2 := types.MkForall("b", types.KType{}, types.MkArrow(&types.TyVar{Name: "b"}, &types.TyVar{Name: "b"}))
	if err := u.Unify(f1, f2); err != nil {
		t.Fatalf("alpha-equivalent foralls should unify: %v", err)
	}
}

// TestProbeD_RowUnify_DuplicateLabelDetection — duplicate labels in the same
// row should be detected by the label context mechanism.
func TestProbeD_RowUnify_DuplicateLabelDetection(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KRow{}}
	// Register "x" as already present in the surrounding context
	u.RegisterLabelContext(1, map[string]struct{}{"x": {}})

	// Try to solve meta with a row containing "x" — should trigger dup label
	solution := &types.TyEvidenceRow{
		Entries: &types.CapabilityEntries{
			Fields: []types.RowField{{Label: "x", Type: types.Con("Int")}},
		},
	}
	err := u.Unify(meta, solution)
	if err == nil {
		t.Fatal("expected duplicate label error, got nil")
	}
	ue, ok := err.(*unify.UnifyError)
	if !ok || ue.Kind != unify.UnifyDupLabel {
		t.Errorf("expected unify.UnifyDupLabel, got %v", err)
	}
}

// TestProbeD_StuckFamily_ReactivationOnMetaSolve — registering a stuck
// family, then reactivating the blocking meta, should move it to the rework queue.
func TestProbeD_StuckFamily_ReactivationOnMetaSolve(t *testing.T) {
	var idx stuckFamilyIndex
	resultMeta := &types.TyMeta{ID: 2, Kind: types.KType{}}

	entry := &stuckFamilyEntry{
		familyName: "Elem",
		args:       []types.Type{&types.TyMeta{ID: 1, Kind: types.KType{}}},
		resultMeta: resultMeta,
		blockingOn: []int{1},
	}
	idx.register(entry)

	if idx.hasRework() {
		t.Fatal("should not have rework before reactivation")
	}

	// Reactivate the blocking meta
	idx.reactivate(1)

	if !idx.hasRework() {
		t.Fatal("expected rework after reactivation")
	}

	entries := idx.drainRework()
	if len(entries) != 1 || entries[0].familyName != "Elem" {
		t.Errorf("expected 1 Elem rework entry, got %d", len(entries))
	}
}
