package check

import (
	"strings"
	"testing"
)

// =============================================================================
// Phase 5D: Quantified Constraints
//
// A quantified constraint `\ a. C1 a => C2 (f a)` introduces evidence
// that is a *function*: given evidence for C1 a, it produces evidence for C2 (f a).
// =============================================================================

func TestQuantifiedConstraintBasic(t *testing.T) {
	// Simplest case: a function with a quantified constraint parameter.
	// (\ a. Eq a => Eq (F a)) provides evidence to resolve Eq (F Bool).
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
f :: \ (g: Type -> Type). (\ a. Eq a => Eq (g a)) => g Bool -> g Bool -> Bool
f := \x y. eq x y
impl Eq a => Eq (F a) := { eq := \x y. True }
main := f (MkF True) (MkF False)`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintMultiplePremises(t *testing.T) {
	// Quantified constraint with multiple premises.
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
form Show := \a. { show: a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. True }
f :: \ (g: Type -> Type). (\ a. (Eq a, Show a) => Eq (g a)) => g Bool -> g Bool -> Bool
f := \x y. eq x y
impl Eq a => Show a => Eq (F a) := { eq := \x y. True }
main := f (MkF True) (MkF False)`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintWithOtherConstraints(t *testing.T) {
	// Quantified constraint alongside regular constraints.
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
form Show := \a. { show: a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. True }
impl Eq a => Eq (F a) := { eq := \x y. True }
f :: \ (g: Type -> Type). (Show Bool, (\ a. Eq a => Eq (g a))) => g Bool -> g Bool -> Bool
f := \x y. eq x y
main := f (MkF True) (MkF False)`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintInProduct(t *testing.T) {
	// Quantified constraint alongside another constraint.
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
form Show := \a. { show: a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. True }
impl Eq a => Eq (F a) := { eq := \x y. True }
f :: \ (g: Type -> Type). (Show Bool, (\ a. Eq a => Eq (g a))) => g Bool -> g Bool -> Bool
f := \x y. eq x y
main := f (MkF True) (MkF False)`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintNested(t *testing.T) {
	// Two quantified constraints in scope.
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form G := \a. { MkG: a -> G a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq (F a) := { eq := \x y. True }
impl Eq a => Eq (G a) := { eq := \x y. True }
f :: \ (g: Type -> Type) (h: Type -> Type). ((\ a. Eq a => Eq (g a)), (\ a. Eq a => Eq (h a))) => g Bool -> h Bool -> Bool
f := \x y. eq x x
main := f (MkF True) (MkG False)`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintErrorMissingPremise(t *testing.T) {
	// Quantified constraint premise can't be satisfied — should error.
	source := `form Bool := { True: Bool; False: Bool; }
form MyType := { MyType: MyType; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
f :: \ (g: Type -> Type). (\ a. Eq a => Eq (g a)) => g MyType -> g MyType -> Bool
f := \x y. eq x y
impl Eq a => Eq (F a) := { eq := \x y. True }
main := f (MkF MyType) (MkF MyType)`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "no instance") {
		t.Errorf("expected 'no instance' error, got: %s", errMsg)
	}
}

// =============================================================================
// Stress Tests — Quantified Constraints
// =============================================================================

func TestStressQuantifiedConstraintDeepChain(t *testing.T) {
	// Quantified constraint where premise itself requires another instance.
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form G := \a. { MkG: a -> G a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq (G a) := { eq := \x y. True }
f :: \ (h: Type -> Type). (\ a. Eq a => Eq (h a)) => h (G Bool) -> h (G Bool) -> Bool
f := \x y. eq x y
impl Eq a => Eq (F a) := { eq := \x y. True }
main := f (MkF (MkG True)) (MkF (MkG False))`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintUsedInBody(t *testing.T) {
	// The quantified evidence is used within the function body to resolve
	// the constraint at a specific type.
	source := `form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq Unit := { eq := \x y. True }
impl Eq a => Eq (F a) := { eq := \x y. True }
f :: \ (g: Type -> Type). (\ a. Eq a => Eq (g a)) => g Bool -> g Unit -> Bool
f := \x y. eq x x
main := f (MkF True) (MkF Unit)`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintParseDisplay(t *testing.T) {
	// Verify the constraint parses, resolves, and the checker doesn't crash.
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq (F a) := { eq := \x y. True }
id :: \ a. a -> a
id := \x. x
f :: \ (g: Type -> Type). (\ a. Eq a => Eq (g a)) => g Bool -> Bool
f := \x. eq x x
main := f (MkF True)`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintMixedWithCurried(t *testing.T) {
	// Quantified constraint mixed with curried regular constraints.
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
form Show := \a. { show: a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. True }
impl Eq a => Eq (F a) := { eq := \x y. True }
f :: \ (g: Type -> Type). (Show Bool, (\ a. Eq a => Eq (g a))) => g Bool -> Bool
f := \x. eq x x
main := f (MkF True)`
	checkSource(t, source, nil)
}

func TestQuantifiedConstraintMixed(t *testing.T) {
	// Curried constraints mixing quantified and simple constraints.
	source := `form Bool := { True: Bool; False: Bool; }
form F := \a. { MkF: a -> F a; }
form Eq := \a. { eq: a -> a -> Bool }
form Show := \a. { show: a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. True }
impl Eq a => Eq (F a) := { eq := \x y. True }
f :: \ (g: Type -> Type). (Show Bool, (\ a. Eq a => Eq (g a))) => g Bool -> Bool
f := \x. eq x x
main := f (MkF True)`
	checkSource(t, source, nil)
}
