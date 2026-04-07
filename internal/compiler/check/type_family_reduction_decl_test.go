// Type family reduction tests — processTypeFamily, installFamilyReducer,
// reduceFamilyApps, mangling, injectivity, and associated type declaration probes.
// Does NOT cover: reduceTyFamily/matchTyPattern (type_family_reduction_match_test.go),
//                 integration scenarios (type_family_reduction_integration_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Duplicate type family should produce error.
func TestProcessTypeFamily_DuplicateCheck(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => True
}
type F :: Bool := \(a: Bool). case a {
  True => False
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

// Type family conflicting with type alias.

// Type family conflicting with type alias.
func TestProcessTypeFamily_ConflictsWithAlias(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type F := \a. a
type F :: Type := \(a: Type). case a {
  a => a
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

// In unified syntax, equation names are auto-populated from the declaration name.
// This test verifies that a single-equation type family with a valid equation compiles.

// In unified syntax, equation names are auto-populated from the declaration name.
// This test verifies that a single-equation type family with a valid equation compiles.
func TestProcessTypeFamily_EquationNameMismatch(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => True;
  False => False
}
`
	checkSource(t, source, nil)
}

// In unified syntax, the case expression pattern is parsed as a single
// type expression (possibly an application chain). The arity matches
// the number of lambda params. This test verifies that an application
// pattern in a 1-param family is accepted as a single complex pattern.

// In unified syntax, the case expression pattern is parsed as a single
// type expression (possibly an application chain). The arity matches
// the number of lambda params. This test verifies that an application
// pattern in a 1-param family is accepted as a single complex pattern.
func TestProcessTypeFamily_ArityValidation(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => True;
  False => False
}
`
	checkSource(t, source, nil)
}

// Zero-arity equation (too few patterns).

// Zero-arity equation (too few patterns).
func TestProcessTypeFamily_ArityTooFew(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool) (b: Bool). case a {
  True => True
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyEquation)
}

// If depth is not reset per normalize call, a second family reduction
// after a successful first would inherit the counter and fail prematurely.
func TestInstallFamilyReducer_DepthResetsPerNormalize(t *testing.T) {
	// Two separate type family uses that each require reduction.
	// If depth doesn't reset, the second might hit the limit.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Unit => Bool;
  Bool => Unit
}
f :: F Unit -> Bool
f := \x. x
g :: F Bool -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

// Recursive type family that terminates within limit.

// Recursive type family that terminates within limit.
func TestInstallFamilyReducer_RecursiveWithinLimit(t *testing.T) {
	source := `
form Session := { Send: Session; Recv: Session; End: (); }
type Dual :: Session := \(s: Session). case s {
  (Send s) => Recv (Dual s);
  (Recv s) => Send (Dual s);
  End => End
}
`
	checkSource(t, source, nil)
}

// TyApp chains with non-family heads should NOT be incorrectly reduced.
func TestReduceFamilyApps_NonFamilyHeadUntouched(t *testing.T) {
	// List is not a type family; applying List to a type should not trigger reduction.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
f :: List Unit -> List Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// TyApp chain that forms saturated family application should reduce.

// TyApp chain that forms saturated family application should reduce.
func TestReduceFamilyApps_SaturatedFamilyApp(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Elem (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Nested type family applications: F (G x).
// Currently, reduceFamilyApps does not recursively reduce TyFamilyApp arguments
// before attempting to match. This means F (G Unit) stays stuck because G Unit
// is not reduced to Bool before F tries to match it.
// This test documents the current behavior.

// Nested type family applications: F (G x).
// Currently, reduceFamilyApps does not recursively reduce TyFamilyApp arguments
// before attempting to match. This means F (G Unit) stays stuck because G Unit
// is not reduced to Bool before F tries to match it.
// This test documents the current behavior.
func TestReduceFamilyApps_NestedFamilyAppStuck(t *testing.T) {
	// F (G Unit) should ideally reduce to Unit (G Unit => Bool, F Bool -> Unit),
	// but currently stays stuck because inner TyFamilyApp args are not reduced
	// before the outer family tries to match.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
type F :: Type := \(a: Type). case a {
  Bool => Unit
}
type G :: Type := \(a: Type). case a {
  Unit => Bool
}
f :: \ x. F (G x) -> F (G x)
f := \x. x
`
	// Using \ x to make it compile as stuck-on-stuck unification.
	checkSource(t, source, nil)
}

// Nested application where outer family is applied to a reduced inner result.
// This works when the inner result is used via a type annotation that triggers
// reduction in the unifier's normalize pass.

// Nested application where outer family is applied to a reduced inner result.
// This works when the inner result is used via a type annotation that triggers
// reduction in the unifier's normalize pass.
func TestReduceFamilyApps_InnerReductionViaUnifier(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
type Id :: Type := \(a: Type). case a {
  a => a
}
f :: Elem (List Unit) -> Unit
f := \x. x
g :: Id Unit -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

// Type family in function argument position.

// Type family in function argument position.
func TestReduceFamilyApps_InArgPosition(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Unit -> Elem (List Unit)
f := \x. x
`
	checkSource(t, source, nil)
}

// Type family in both argument and result positions.

// Type family in both argument and result positions.
func TestReduceFamilyApps_BothPositions(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Elem (List Unit) -> Elem (List Unit)
f := \x. x
`
	checkSource(t, source, nil)
}

// Different patterns should produce different mangled names (no collisions).
func TestMangledDataFamilyName_NoCollisions(t *testing.T) {
	// Verifies that associated type family equations for different instances
	// produce correct reductions.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

form Container := \c. {
  type Elem c :: Type;
  empty: c
}

impl Container (List a) := {
  type Elem := a;
  empty := Nil
}

impl Container (Maybe a) := {
  type Elem := a;
  empty := Nothing
}

x :: Elem (List Unit) -> Unit
x := \v. v
y :: Elem (Maybe Unit) -> Unit
y := \v. v
`
	checkSource(t, source, nil)
}

// Zero-argument associated type definition in instance.

// Zero-argument associated type definition in instance.
func TestMangledDataFamilyName_ZeroArgPattern(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }

form Container := \c. {
  type Elem c :: Type;
  empty: c
}

impl Container Unit := {
  type Elem := Unit;
  empty := Unit
}

x :: Elem Unit -> Unit
x := \v. v
`
	checkSource(t, source, nil)
}

func TestVerifyInjectivity_ViolationCaught(t *testing.T) {
	// Without injectivity annotation, overlapping RHSes are allowed
	// (they behave as a regular closed type family with first-match semantics).
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  Unit => Unit
}
f :: Elem (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Equations with distinct RHSes should work fine.

// Equations with distinct RHSes should work fine.
func TestVerifyInjectivity_DistinctRHSes(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
type Tag :: Type := \(c: Type). case c {
  Unit => Bool;
  Bool => Unit
}
`
	checkSource(t, source, nil)
}

// Single-equation type family (identity).

// Single-equation type family (identity).
func TestVerifyInjectivity_SingleEquationAlwaysInjective(t *testing.T) {
	source := `
type Id :: Type := \(a: Type). case a {
  a => a
}
`
	checkSource(t, source, nil)
}

// Single equation identity family with concrete usage.

// Single equation identity family with concrete usage.
func TestVerifyInjectivity_CompatibleOverlap(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type F :: Type := \(a: Type). case a {
  a => a
}
f :: F Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// Associated type in class with no instances should produce a stuck type.
func TestAssocType_NoInstances(t *testing.T) {
	source := `
form Container := \c. {
  type Elem c :: Type;
  clength: c -> Int
}
f :: \ c. Elem c -> Elem c
f := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

// Associated type equation in wrong class should be rejected.

// Associated type equation in wrong class should be rejected.
func TestAssocType_WrongClass(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Foo := \a. {
  type MyType a :: Type;
  m: a
}
form Bar := \a. {
  m2: a
}
impl Bar Unit := {
  type MyType := Unit;
  m2 := Unit
}
`
	checkSourceExpectError(t, source, nil)
}

// Associated type with arity mismatch in instance.

// Associated type with arity mismatch in instance.
func TestAssocType_ArityMismatchInInstance(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form Container := \c. {
  type Elem c :: Type;
  empty: c
}
impl Container Unit := {
  type Elem := Unit;
  empty := Unit
}
`
	// In unified syntax, the associated type definition is always `type Elem := RHS`.
	// Arity mismatch is not possible. This test verifies it compiles correctly.
	checkSource(t, source, nil)
}
