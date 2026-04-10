//go:build probe

// Zonk probe tests — meta chain path compression, unsolved meta preservation, structural identity.
// Does NOT cover: check_unify_test.go, check_unify_probe_test.go.
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Zonk probe tests — meta chain path compression, unsolved meta preservation,
// structural identity, forall body zonking, deep nesting, TyEvidence with
// solved metas, TyFamilyApp arg zonking.
// =============================================================================

// =====================================================================
// From probe_d: Zonk with cycles and path compression
// =====================================================================

// TestProbeD_Zonk_MetaChainPathCompression — a chain m1 -> m2 -> m3 -> Int
// should be compressed so m1 points directly to Int after Zonk.
func TestProbeD_Zonk_MetaChainPathCompression(t *testing.T) {
	u := unify.NewUnifier()
	m1 := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	m2 := &types.TyMeta{ID: 2, Kind: types.TypeOfTypes}
	m3 := &types.TyMeta{ID: 3, Kind: types.TypeOfTypes}

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
	m := &types.TyMeta{ID: 42, Kind: types.TypeOfTypes}
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
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	if err := u.Unify(m, types.Con("Int")); err != nil {
		t.Fatal(err)
	}
	forallTy := types.MkForall("a", types.TypeOfTypes, types.MkArrow(&types.TyVar{Name: "a"}, m))
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
// From probe_e: Zonk edge cases
// =====================================================================

// TestProbeE_Zonk_UnsolvedMetaPassthrough — zonking an unsolved meta should
// return the meta itself.
func TestProbeE_Zonk_UnsolvedMetaPassthrough(t *testing.T) {
	u := unify.NewUnifier()
	m := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
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
	meta := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	u.Unify(meta, types.Con("Bool"))
	evidence := &types.TyEvidence{
		Constraints: &types.TyEvidenceRow{
			Entries: &types.ConstraintEntries{
				Entries: []types.ConstraintEntry{
					&types.ClassEntry{ClassName: "Eq", Args: []types.Type{meta}},
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
	meta := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	u.Unify(meta, types.Con("Int"))
	fam := &types.TyFamilyApp{
		Name: "F",
		Args: []types.Type{meta, types.Con("Bool")},
		Kind: types.TypeOfTypes,
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
