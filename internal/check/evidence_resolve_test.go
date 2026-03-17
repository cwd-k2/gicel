package check

import (
	"testing"
)

// =============================================================================
// Evidence Resolution — Level 8 integration tests
// =============================================================================

func TestEvidenceResolveSingleConstraint(t *testing.T) {
	// Basic: Eq Bool resolved from global instance.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
main := eq True False`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

func TestEvidenceResolveMultiConstraint(t *testing.T) {
	// Multiple constraints in one TyEvidence: { Eq a, Ord a } => ...
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
f :: \ a. (Eq a, Ord a) => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

func TestEvidenceResolveSuperclass(t *testing.T) {
	// Ord a => ... should resolve Eq a via superclass.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
f :: \ a. Ord a => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	checkSource(t, source, nil)
}

func TestEvidenceResolveContextual(t *testing.T) {
	// Instance with context: Eq a => Eq (Maybe a).
	source := `data Bool = True | False
data Maybe a = Just a | Nothing
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq (Maybe a) { eq := \x y. True }
main := eq (Just True) (Just False)`
	checkSource(t, source, nil)
}

func TestEvidenceResolveMultipleClasses(t *testing.T) {
	// Multiple independent classes: Eq and Show.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Show a { show :: a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Show Bool { show := \x. True }
f :: \ a. (Eq a, Show a) => a -> Bool
f := \x. eq x x
main := f True`
	checkSource(t, source, nil)
}

func TestEvidenceResolveNested(t *testing.T) {
	// Nested evidence: using a method inside an instance method.
	source := `data Bool = True | False
data Pair a b = MkPair a b
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq b => Eq (Pair a b) { eq := \x y. True }
main := eq (MkPair True True) (MkPair False False)`
	checkSource(t, source, nil)
}

func TestEvidenceResolveTransitiveSuperclass(t *testing.T) {
	// Transitive superclass: Bounded a => ... should resolve Eq a
	// via Bounded => Ord => Eq chain.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
class Ord a => Bounded a { minBound :: a }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
instance Bounded Bool { minBound := True }
f :: \ a. Bounded a => a -> a -> Bool
f := \x y. eq x y
main := f True False`
	checkSource(t, source, nil)
}

func TestEvidenceResolveStressMultiInstance(t *testing.T) {
	// Multiple instances for the same class.
	source := `data Bool = True | False
data Unit = Unit
data Maybe a = Just a | Nothing
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq Unit { eq := \x y. True }
instance Eq a => Eq (Maybe a) { eq := \x y. True }
main := eq (Just Unit) (Just Unit)`
	checkSource(t, source, nil)
}
