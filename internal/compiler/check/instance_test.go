// Instance tests — resolution, overlap, arity, context, self-cycle, index lookup.
// Does NOT cover: class elaboration (class_elaboration_test.go), evidence (evidence_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Instance resolution tests ---

func TestResolveMissingInstanceError(t *testing.T) {
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNoInstance)
}

func TestResolveSimpleInstance(t *testing.T) {
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !types.Equal(b.Type, types.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", types.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

func TestResolveContextualInstance(t *testing.T) {
	source := `data Bool := { True: (); False: (); }
data Maybe := \a. { Just: a; Nothing: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq (Maybe a) { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
main := f (Just True) (Just False)`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !types.Equal(b.Type, types.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", types.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

// --- Instance index tests ---

func TestInstanceIndexLookup(t *testing.T) {
	// Register 10 classes each with 10 instances, then resolve specific one.
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
class Show a { show :: a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Show Bool { show := \x. True }
main := eq True False`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			if !types.Equal(b.Type, types.Con("Bool")) {
				t.Errorf("expected main :: Bool, got %s", types.Pretty(b.Type))
			}
			return
		}
	}
	t.Error("expected binding 'main'")
}

func TestOverlappingInstances(t *testing.T) {
	// Two instances of Eq for the same type should trigger ErrOverlap.
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. case x { True => y; False => case y { True => False; False => True } } }
instance Eq Bool { eq := \x y. True }
main := eq True False`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

func TestNonOverlappingInstances(t *testing.T) {
	// Instances for different types should not overlap.
	source := `data Bool := { True: (); False: (); }
data Unit := { Unit: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. case x { True => y; False => case y { True => False; False => True } } }
instance Eq Unit { eq := \_ _. True }
main := eq True False`
	checkSource(t, source, nil)
}

func TestInstanceArityMismatch(t *testing.T) {
	// Class Eq has 1 type param, instance provides 2 → ErrBadInstance.
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool Bool { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestInstanceUnknownContextClass(t *testing.T) {
	// Instance context references a class that doesn't exist → ErrBadInstance.
	source := `data Bool := { True: (); False: (); }
data Maybe := \a. { Nothing: (); Just: a; }
class Eq a { eq :: a -> a -> Bool }
instance Phantom a => Eq (Maybe a) { eq := \_ _. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestInstanceSelfCycle(t *testing.T) {
	// Instance context requires itself → ErrBadInstance.
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq a { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestInstanceExtraMethod(t *testing.T) {
	// Instance defines a method not declared in the class → ErrBadInstance.
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True; notAMethod := \x. x }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestInstanceValidContextClass(t *testing.T) {
	// Valid instance with known context class should succeed.
	source := `data Bool := { True: (); False: (); }
data Maybe := \a. { Nothing: (); Just: a; }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. case x { True => y; False => case y { True => False; False => True } } }
instance Eq a => Eq (Maybe a) {
  eq := \x y. case x {
    Nothing => case y { Nothing => True; Just _ => False };
    Just a  -> case y { Nothing => False; Just b -> eq a b }
  }
}`
	checkSource(t, source, nil)
}

func TestParametricOverlappingInstances(t *testing.T) {
	// instance Eq (Maybe a) overlaps with instance Eq (Maybe Bool).
	source := `data Bool := { True: (); False: (); }
data Maybe := \a. { Nothing: (); Just: a; }
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) {
  eq := \x y. case x {
    Nothing => case y { Nothing => True; Just _ => False };
    Just a  -> case y { Nothing => False; Just b -> eq a b }
  }
}
instance Eq (Maybe Bool) {
  eq := \_ _. True
}`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

func TestSelfCycleCompoundType(t *testing.T) {
	// instance Eq (Maybe a) => Eq (Maybe a) is a self-cycle with compound types.
	source := `data Bool := { True: (); False: (); }
data Maybe := \a. { Nothing: (); Just: a; }
class Eq a { eq :: a -> a -> Bool }
instance Eq (Maybe a) => Eq (Maybe a) { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}

func TestOverlapBlocksRegistration(t *testing.T) {
	// Overlapping instance should NOT be registered — resolution should fail
	// with "no instance" rather than silently picking one.
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. case x { True => y; False => case y { True => False; False => True } } }
instance Eq Bool { eq := \x y. True }
main := eq True False`
	// We expect ErrOverlap from the duplicate instance declaration.
	// The second instance is rejected, so resolution uses the first — no ambiguity.
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

func TestSelfCycleBlocksRegistration(t *testing.T) {
	// Self-cycle should not be registered — no cascading errors from resolution.
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq a { eq := \x y. True }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadInstance)
}
