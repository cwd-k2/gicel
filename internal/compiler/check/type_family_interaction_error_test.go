// Type family interaction tests — error message quality and diagnostics.
// Does NOT cover: cross-feature combinations (type_family_interaction_cross_test.go),
//                 bug probes (type_family_interaction_bug_test.go),
//                 unit assertions (type_family_interaction_unit_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

func TestInteractionErrorTypeFamilyArityTooMany(t *testing.T) {
	// Applying too many arguments to a type family.
	source := `
form Unit := { Unit: Unit; }
type Id :: Type := \(a: Type). case a {
  a => a
}
f :: Id Unit Unit -> Unit
f := \x. x
`
	// Id takes 1 arg, but is applied to 2. This should produce an error.
	// Depending on how the parser/checker handles extra args, this might
	// be a kind error or a type family arity mismatch.
	errMsg := checkSourceExpectError(t, source, nil)
	t.Logf("Arity error message: %s", errMsg)
}

func TestInteractionErrorTypeFamilyEquationArityMismatch(t *testing.T) {
	// In unified syntax, each case alternative has a single pattern expression.
	// An application chain like `a b` is parsed as a single pattern.
	// This test verifies that such a pattern compiles.
	source := `
form Unit := { Unit: Unit; }
type Id :: Type := \(a: Type). case a {
  a => a
}
f :: Id Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestInteractionErrorTypeFamilyNoMatch(t *testing.T) {
	// Type family applied to a type that doesn't match any equation.
	// This should produce a type mismatch because the stuck TyFamilyApp
	// cannot unify with the expected type.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
f :: Elem Bool -> Unit
f := \x. x
`
	// Elem Bool is stuck (no equation matches). Trying to unify
	// the stuck Elem Bool with Unit should fail.
	errMsg := checkSourceExpectError(t, source, nil)
	t.Logf("No-match error: %s", errMsg)
}

func TestInteractionErrorInjectivityEquationNumbers(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
type Collapse :: Type := \(a: Type). case a {
  Unit => Unit;
  Bool => Unit
}
f :: Collapse Unit -> Unit
f := \x. x
`
	// In unified syntax, injectivity annotations are not supported.
	// The type family compiles successfully.
	checkSource(t, source, nil)
}

func TestInteractionErrorRecursiveDepthFamilyName(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type Loop :: Type := \(a: Type). case a {
  a => Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	// Cycle is detected via sentinel memoization; the family remains stuck (unreduced),
	// producing a type mismatch (E0200) when Loop Unit is compared against Unit.
	errMsg := checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
	// The error message should include the family name "Loop".
	if !strings.Contains(errMsg, "Loop") {
		t.Errorf("expected family name 'Loop' in type mismatch error, got: %s", errMsg)
	}
}

func TestInteractionErrorDataFamilyConstructorWithoutInstance(t *testing.T) {
	// Using a data family constructor name that doesn't exist
	// (simulates the case where the instance wasn't imported).
	source := `
form Unit := { Unit: Unit; }
form Container := \a. {
  form Elem a :: Type;
  empty: a
}
x :: Elem Unit
x := ListElem Unit
`
	// ListElem is not defined anywhere — should be an unbound constructor error.
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "ListElem") {
		t.Errorf("expected mention of ListElem in error, got: %s", errMsg)
	}
}

// ================================================================
// Additional interaction tests: stress the boundary
// ================================================================
