//go:build probe

// Skolem probe tests — skolem rigidity, same-skolem unification, different-skolem rejection.
// Does NOT cover: check_unify_test.go, check_unify_probe_test.go, checker_gadt_probe_test.go.
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Skolem probe tests — skolem rigidity, same-skolem unification, different-
// skolem rejection, meta solved to skolem in row, and existential escape.
// =============================================================================

// =====================================================================
// From probe_d: Skolem rigidity
// =====================================================================

// TestProbeD_Skolem_RigidUnifyWithConcrete — trying to unify a skolem
// with a concrete type should produce an error.
func TestProbeD_Skolem_RigidUnifyWithConcrete(t *testing.T) {
	u := unify.NewUnifier(&types.TypeOps{})
	sk := &types.TySkolem{ID: 1, Name: "a", Kind: types.TypeOfTypes}
	err := u.Unify(sk, types.MkCon("Int"))
	if err == nil {
		t.Fatal("expected skolem rigidity error, got nil")
	}
	ue, ok := err.(unify.UnifyError)
	if !ok || ue.Kind() != unify.UnifySkolemRigid {
		t.Errorf("expected unify.UnifySkolemRigid, got %v", err)
	}
}

// TestProbeD_Skolem_SameSkolemUnifies — unifying a skolem with itself
// should succeed.
func TestProbeD_Skolem_SameSkolemUnifies(t *testing.T) {
	u := unify.NewUnifier(&types.TypeOps{})
	sk := &types.TySkolem{ID: 1, Name: "a", Kind: types.TypeOfTypes}
	if err := u.Unify(sk, sk); err != nil {
		t.Fatalf("same skolem should unify: %v", err)
	}
}

// TestProbeD_Skolem_DifferentSkolemsRefuse — two different skolems should
// not unify.
func TestProbeD_Skolem_DifferentSkolemsRefuse(t *testing.T) {
	u := unify.NewUnifier(&types.TypeOps{})
	sk1 := &types.TySkolem{ID: 1, Name: "a", Kind: types.TypeOfTypes}
	sk2 := &types.TySkolem{ID: 2, Name: "b", Kind: types.TypeOfTypes}
	err := u.Unify(sk1, sk2)
	if err == nil {
		t.Fatal("different skolems should not unify")
	}
}

// TestProbeD_Skolem_MetaSolvedToSkolemInRow — a meta inside a row solved
// to contain a skolem should be detected if the skolem escapes.
func TestProbeD_Skolem_MetaSolvedToSkolemInRow(t *testing.T) {
	u := unify.NewUnifier(&types.TypeOps{})
	sk := &types.TySkolem{ID: 1, Name: "a", Kind: types.TypeOfTypes}
	meta := &types.TyMeta{ID: 2, Kind: types.TypeOfTypes}

	// Solve meta = F skolem
	wrapped := &types.TyApp{Fun: types.MkCon("F"), Arg: sk}
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
form Bool := { True: Bool; False: Bool; }
form Exists := { MkExists: \ a. a -> Exists }

-- Trying to return the existentially-bound value should fail.
bad :: Exists -> Bool
bad := \e. case e { MkExists x => x }
`
	checkSourceExpectError(t, source, nil)
}

// TestProbeD_Skolem_TrailBasedCheckCatchesEscape verifies that the
// trail-incremental check (checkSkolemEscapeSince) catches the same
// classes of leaks the previous full-iteration version did. This is the
// regression guard for the P1 optimization: shrinking the iteration
// scope from "all soln" to "writes since trailPos" must NOT lose any
// real escape detection.
//
// We construct a scenario that causes a skolem to be written into a
// pre-existing meta and verify the diagnostic fires. The mechanism:
// the existential's hidden type appears in a context where the checker
// instantiates a polymorphic helper, and the metas of the helper end
// up referencing the skolem.
func TestProbeD_Skolem_TrailBasedCheckCatchesEscape(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Exists := { MkExists: \ a. a -> Exists }

id :: \ a. a -> a
id := \x. x

-- Like the existential escape above, but routed through a polymorphic
-- helper. The helper's fresh metas may end up holding the skolem,
-- which the trail-based check must still catch.
bad :: Exists -> Bool
bad := \e. case e { MkExists x => id x }
`
	checkSourceExpectError(t, source, nil)
}

// TestProbeD_Skolem_NoFalsePositiveOnPolymorphicBody verifies that the
// trail-based check does NOT raise false positives on a polymorphic
// body whose metas all reference only in-scope skolems. The previous
// full-iteration check would walk all solns; the trail-based version
// walks only writes within the body. Both must agree on "no escape" for
// well-typed polymorphic code.
func TestProbeD_Skolem_NoFalsePositiveOnPolymorphicBody(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

-- A polymorphic body with multiple metas being solved during checking.
-- All solutions should reference only the local skolem 'a' or be ground.
poly :: \ a. a -> a -> a
poly := \x. \y. x
`
	checkSource(t, source, nil)
}

// TestProbeD_Skolem_NestedForallNoLeak verifies that nested foralls
// each get their own trailPos and the inner check does not see writes
// from the outer scope as leaks.
func TestProbeD_Skolem_NestedForallNoLeak(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

-- const: outer skolem 'a', inner skolem 'b'. Both must be in scope of
-- their respective foralls; the trail-based check at each level must
-- only see writes inside that level's body.
constant :: \ a. \ b. a -> b -> a
constant := \x. \y. x
`
	checkSource(t, source, nil)
}
