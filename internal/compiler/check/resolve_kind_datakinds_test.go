// DataKinds tests — KData equality/arity, user kind resolution, constructor promotion.
// Does NOT cover: GADT (gadt_check_test.go), kind unification (unify/kind_unify_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- DataKinds tests ---

func TestKDataEquality(t *testing.T) {
	k1 := types.KData{Name: "Bool"}
	k2 := types.KData{Name: "Bool"}
	k3 := types.KData{Name: "DBState"}
	if !k1.Equal(k2) {
		t.Error("KData{Bool} should equal KData{Bool}")
	}
	if k1.Equal(k3) {
		t.Error("KData{Bool} should not equal KData{DBState}")
	}
	if k1.String() != "Bool" {
		t.Errorf("expected 'Bool', got %s", k1.String())
	}
}

func TestKDataArity(t *testing.T) {
	k := types.KData{Name: "DBState"}
	if types.Arity(k) != 0 {
		t.Errorf("KData arity should be 0, got %d", types.Arity(k))
	}
	if types.ResultKind(k) != k {
		t.Error("KData ResultKind should be itself")
	}
}

func TestResolveUserKind(t *testing.T) {
	// \ (s: DBState). T → the kind annotation DBState should resolve to KData{DBState}
	source := `form DBState := { Opened: DBState; Closed: DBState; }
form DB := \s. { MkDB: DB s; }
f :: \ (s: DBState). DB s -> DB s
f := \x. x
main := f (MkDB :: DB Opened)`
	checkSource(t, source, nil)
}

func TestPromoteNullaryConstructors(t *testing.T) {
	// form S := { A: S; B: S; } → A and B are promoted to type level with kind S
	source := `form S := { A: S; B: S; }
form Proxy := \s. { MkProxy: Proxy s; }
main := (MkProxy :: Proxy A)`
	checkSource(t, source, nil)
}

func TestPromoteSkipsFieldedConstructors(t *testing.T) {
	// form Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a; } → only Nothing is promoted, Just is not
	source := `form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a; }
form Proxy := \s. { MkProxy: Proxy s; }
main := (MkProxy :: Proxy Nothing)`
	checkSource(t, source, nil)
}

func TestNonNullaryPromotedConKind(t *testing.T) {
	// Non-nullary constructors should be promoted with kind arrows.
	// Just :: Type -> Maybe (i.e., KArrow{KType, KData{Maybe}})
	source := `form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a }
type Test :: Maybe := Just Int
main := 42`
	checkSource(t, source, nil)
}

func TestNonNullaryPromotedConInTypeFamily(t *testing.T) {
	// Non-nullary promoted con in type family pattern.
	source := `
form Bool := { True: Bool; False: Bool }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a }

type IsJust :: Bool := \(m: Maybe). case m {
  Just _ => True;
  Nothing => False
}

type T1 :: Bool := IsJust (Just Int)
type T2 :: Bool := IsJust Nothing

main := 42`
	checkSource(t, source, nil)
}

func TestPromotedInTypeSignature(t *testing.T) {
	// DB Opened => DB Closed should kind-check
	source := `form DBState := { Opened: DBState; Closed: DBState; }
form DB := \s. { MkDB: DB s; }
close :: DB Opened -> DB Closed
close := \_. MkDB
main := close MkDB`
	checkSource(t, source, nil)
}
