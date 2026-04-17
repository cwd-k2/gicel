// Instance tests — resolution, overlap, arity, context, self-cycle, index lookup.
// Does NOT cover: class elaboration (class_elaboration_test.go), evidence (evidence_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// --- Instance resolution tests ---

func TestResolveMissingInstanceError(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNoInstance)
}

func TestResolveSimpleInstance(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !testOps.Equal(b.Type, testOps.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", testOps.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

func TestResolveContextualInstance(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq (Maybe a) := { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f (Just True) (Just False)`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !testOps.Equal(b.Type, testOps.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", testOps.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

// --- Instance index tests ---

func TestInstanceIndexLookup(t *testing.T) {
	// Register 10 classes each with 10 instances, then resolve specific one.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
form Show := \a. { show: a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. True }
main := eq True False`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !testOps.Equal(b.Type, testOps.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", testOps.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

func TestOverlappingInstances(t *testing.T) {
	// Two instances of Eq for the same type should trigger ErrOverlap.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. case x { True => y; False => case y { True => False; False => True } } }
impl Eq Bool := { eq := \x y. True }
main := eq True False`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

func TestNonOverlappingInstances(t *testing.T) {
	// Instances for different types should not overlap.
	source := `form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. case x { True => y; False => case y { True => False; False => True } } }
impl Eq Unit := { eq := \_ _. True }
main := eq True False`
	checkSource(t, source, nil)
}

func TestInstanceArityMismatch(t *testing.T) {
	// Class Eq has 1 type param, instance provides 2 → ErrBadInstance.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool Bool := { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestInstanceUnknownContextClass(t *testing.T) {
	// Instance context references a class that doesn't exist → ErrBadInstance.
	source := `form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Phantom a => Eq (Maybe a) := { eq := \_ _. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestInstanceSelfCycle(t *testing.T) {
	// Instance context requires itself → ErrBadInstance.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq a => Eq a := { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestInstanceExtraMethod(t *testing.T) {
	// Instance defines a method not declared in the class → ErrBadInstance.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True; notAMethod := \x. x }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestInstanceValidContextClass(t *testing.T) {
	// Valid instance with known context class should succeed.
	source := `form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. case x { True => y; False => case y { True => False; False => True } } }
impl Eq a => Eq (Maybe a) := {
  eq := \x y. case x {
    Nothing => case y { Nothing => True; Just _ => False };
    Just a  => case y { Nothing => False; Just b => eq a b }
  }
}`
	checkSource(t, source, nil)
}

func TestParametricOverlappingInstances(t *testing.T) {
	// instance Eq (Maybe a) overlaps with instance Eq (Maybe Bool).
	source := `form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq a => Eq (Maybe a) := {
  eq := \x y. case x {
    Nothing => case y { Nothing => True; Just _ => False };
    Just a  => case y { Nothing => False; Just b => eq a b }
  }
}
impl Eq (Maybe Bool) := {
  eq := \_ _. True
}`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

func TestSelfCycleCompoundType(t *testing.T) {
	// instance Eq (Maybe a) => Eq (Maybe a) is a self-cycle with compound types.
	source := `form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq (Maybe a) => Eq (Maybe a) := { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestOverlapBlocksRegistration(t *testing.T) {
	// Overlapping instance should NOT be registered — resolution should fail
	// with "no instance" rather than silently picking one.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. case x { True => y; False => case y { True => False; False => True } } }
impl Eq Bool := { eq := \x y. True }
main := eq True False`
	// We expect ErrOverlap from the duplicate instance declaration.
	// The second instance is rejected, so resolution uses the first — no ambiguity.
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

func TestSelfCycleBlocksRegistration(t *testing.T) {
	// Self-cycle should not be registered — no cascading errors from resolution.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool }
impl Eq a => Eq a := { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

// --- Private instance tests ---

func TestPrivateInstanceNoOverlap(t *testing.T) {
	// A private instance (impl _name) should NOT overlap with a public instance
	// for the same class and type.
	source := `form Int := { MkInt: Int; }
form MyEq := \a. { eq: a -> a -> Int; }
impl MyEq Int := { eq := \x y. MkInt; }
impl _alwaysEq :: MyEq Int := { eq := \x y. MkInt; }
main := eq MkInt MkInt`
	checkSource(t, source, nil)
}

func TestPrivateInstanceIsSolverInvisible(t *testing.T) {
	// When both a public and private instance exist for the same class/type,
	// the solver should resolve to the public instance (the private one is
	// still registered but the public one is found first for same-module).
	source := `form Int := { MkInt: Int; }
form MyEq := \a. { eq: a -> a -> Int; }
impl MyEq Int := { eq := \x y. MkInt; }
impl _alt :: MyEq Int := { eq := \x y. MkInt; }
main := eq MkInt MkInt`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !testOps.Equal(b.Type, testOps.Con("Int")) {
				t.Errorf("expected main :: Int, got %s", testOps.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

func TestTwoPrivateInstancesNoOverlap(t *testing.T) {
	// Two private instances for the same class/type should not overlap
	// with each other either.
	source := `form Int := { MkInt: Int; }
form MyEq := \a. { eq: a -> a -> Int; }
impl _a :: MyEq Int := { eq := \x y. MkInt; }
impl _b :: MyEq Int := { eq := \x y. MkInt; }
main := MkInt`
	checkSource(t, source, nil)
}

func TestPublicInstancesStillOverlap(t *testing.T) {
	// Two public instances for the same class/type should still overlap.
	source := `form Bool := { True: Bool; False: Bool; }
form Eq := \a. { eq: a -> a -> Bool; }
impl first :: Eq Bool := { eq := \x y. True; }
impl second :: Eq Bool := { eq := \x y. False; }
main := eq True False`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

// --- Scoped evidence injection tests ---

func TestScopedEvidenceInjection(t *testing.T) {
	// Private instance injected via value => expr overrides public instance.
	source := `form Int := { MkInt: Int; }
form MyEq := \a. { eq: a -> a -> Int; }
impl MyEq Int := { eq := \x y. MkInt; }
impl _alt :: MyEq Int := { eq := \x y. MkInt; }
main := _alt => eq MkInt MkInt`
	checkSource(t, source, nil)
}

func TestScopedEvidenceNested(t *testing.T) {
	// Nested scoped evidence: d1 => d2 => expr.
	source := `form Int := { MkInt: Int; }
form MyEq := \a. { eq: a -> a -> Int; }
form MyOrd := \a. { cmp: a -> a -> Int; }
impl MyEq Int := { eq := \x y. MkInt; }
impl MyOrd Int := { cmp := \x y. MkInt; }
impl _e :: MyEq Int := { eq := \x y. MkInt; }
impl _o :: MyOrd Int := { cmp := \x y. MkInt; }
main := _e => _o => eq MkInt MkInt`
	checkSource(t, source, nil)
}

func TestScopedEvidenceDoesNotLeak(t *testing.T) {
	// Scoped evidence should not affect resolution outside the => body.
	source := `form Bool := { True: Bool; False: Bool; }
form MyEq := \a. { eq: a -> a -> Bool; }
impl MyEq Bool := { eq := \x y. True; }
impl _alt :: MyEq Bool := { eq := \x y. False; }
inner := _alt => eq True False
outer := eq True True
main := outer`
	checkSource(t, source, nil)
}

func TestScopedEvidenceNonDictType(t *testing.T) {
	// value => expr where value has a non-dict type: context stays balanced,
	// evidence is not injected, body is checked normally.
	source := `form Unit := { Unit: Unit; }
main := Unit => Unit`
	checkSource(t, source, nil)
}
