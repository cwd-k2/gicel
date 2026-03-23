//go:build probe

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
form Bool := { True: Bool; False: Bool; }
form Exists := { MkExists: \ a. a -> Exists }

-- Trying to return the existentially-bound value should fail.
bad :: Exists -> Bool
bad := \e. case e { MkExists x => x }
`
	checkSourceExpectError(t, source, nil)
}
