//go:build probe

// Explicit type application (@) kind checking probe tests.
// Verifies that ill-kinded @-applications produce ErrKindMismatch.
// Does NOT cover: resolve_kind_probe_test.go, resolve_kind_hkt_test.go.
package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// --- Reject: ill-kinded @-application ---

// TestProbe_TyApp_KindMismatch_RowArg — explicitly applying an empty
// capability row (kind Row) where a Type is expected.
func TestProbe_TyApp_KindMismatch_RowArg(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }

id :: \ a. a -> a
id := \x. x

main := id @({}) Unit
`
	msg := checkSourceExpectCode(t, source, nil, diagnostic.ErrKindMismatch)
	if !strings.Contains(msg, "kind") {
		t.Errorf("expected kind-related error, got: %s", msg)
	}
}

// TestProbe_TyApp_KindMismatch_ConstraintArg — applying a Constraint-kinded
// class application to a Type-kinded binder.
func TestProbe_TyApp_KindMismatch_ConstraintArg(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool; }

id :: \ a. a -> a
id := \x. x

main := id @(Eq Bool) True
`
	msg := checkSourceExpectCode(t, source, nil, diagnostic.ErrKindMismatch)
	if !strings.Contains(msg, "kind") {
		t.Errorf("expected kind-related error, got: %s", msg)
	}
}

// TestProbe_TyApp_KindMismatch_HKTWrongArity — applying a Type-kinded
// argument where Type -> Type is expected.
func TestProbe_TyApp_KindMismatch_HKTWrongArity(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

hk :: \ (f: Type -> Type). \ a. f a -> f a
hk := \x. x

main := hk @Bool @Bool True
`
	msg := checkSourceExpectCode(t, source, nil, diagnostic.ErrKindMismatch)
	if !strings.Contains(msg, "kind") {
		t.Errorf("expected kind-related error, got: %s", msg)
	}
}

// --- Accept: well-kinded @-application ---

// TestProbe_TyApp_ValidTypeArg — Type-kinded argument to Type-kinded binder.
func TestProbe_TyApp_ValidTypeArg(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

id :: \ a. a -> a
id := \x. x

main := id @Bool True
`
	checkSource(t, source, nil)
}

// TestProbe_TyApp_ValidHKT — (Type -> Type)-kinded argument to HKT binder.
func TestProbe_TyApp_ValidHKT(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

hk :: \ (f: Type -> Type). \ a. f a -> f a
hk := \x. x

main := hk @List @Unit Nil
`
	checkSource(t, source, nil)
}

// TestProbe_TyApp_KindPoly_ExplicitTypeArg — kind-polymorphic functions
// with explicit type argument (kind is inferred implicitly).
func TestProbe_TyApp_KindPoly_ExplicitTypeArg(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

id_k :: \ (k: Kind). \ (a: k). a -> a
id_k := \x. x

main := id_k @Bool True
`
	// k:Kind is instantiated implicitly, @Bool applies to the a:k binder.
	checkSource(t, source, nil)
}

// TestProbe_TyApp_KindPoly_ImplicitInstantiation — kind-polymorphic
// functions should work through implicit instantiation without @.
func TestProbe_TyApp_KindPoly_ImplicitInstantiation(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

id_k :: \ (k: Kind). \ (a: k). a -> a
id_k := \x. x

main := id_k True
`
	checkSource(t, source, nil)
}

// TestProbe_TyApp_NonPolyTarget — @-application to a non-polymorphic type
// should produce ErrBadTypeApp.
func TestProbe_TyApp_NonPolyTarget(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }

x :: Bool
x := True

main := x @Bool
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadTypeApp)
}
