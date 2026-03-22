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
	source := `data DBState := Opened | Closed
data DB s := MkDB
f :: \ (s: DBState). DB s -> DB s
f := \x. x
main := f (MkDB :: DB Opened)`
	checkSource(t, source, nil)
}

func TestPromoteNullaryConstructors(t *testing.T) {
	// data S := A | B → A and B are promoted to type level with kind S
	source := `data S := A | B
data Proxy s := MkProxy
main := (MkProxy :: Proxy A)`
	checkSource(t, source, nil)
}

func TestPromoteSkipsFieldedConstructors(t *testing.T) {
	// data Maybe := \a. { Just: a; Nothing: (); } → only Nothing is promoted, Just is not
	source := `data Bool := { True: (); False: (); }
data Maybe := \a. { Just: a; Nothing: (); }
data Proxy s := MkProxy
main := (MkProxy :: Proxy Nothing)`
	checkSource(t, source, nil)
}

func TestPromotedInTypeSignature(t *testing.T) {
	// DB Opened => DB Closed should kind-check
	source := `data DBState := Opened | Closed
data DB s := MkDB
close :: DB Opened => DB Closed
close := \_. MkDB
main := close MkDB`
	checkSource(t, source, nil)
}
