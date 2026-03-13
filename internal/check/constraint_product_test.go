package check

import (
	"testing"

	"github.com/cwd-k2/gomputation/internal/types"
)

// =============================================================================
// Phase 5B: Constraint Product Syntax — (C1, C2) => T
// =============================================================================

func TestConstraintProductBasic(t *testing.T) {
	// (Eq a, Ord a) => a -> Bool — two constraints in product.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
f :: forall a. (Eq a, Ord a) => a -> Bool
f := \x -> eq x x
main := f True`
	checkSource(t, source, nil)
}

func TestConstraintProductTriple(t *testing.T) {
	// (Eq a, Ord a, Show a) => a -> Bool — three constraints.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
class Show a { show :: a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
instance Show Bool { show := \x -> True }
f :: forall a. (Eq a, Ord a, Show a) => a -> Bool
f := \x -> eq x x
main := f True`
	checkSource(t, source, nil)
}

func TestConstraintProductUsesMethods(t *testing.T) {
	// Ensure both constraints are usable within the body.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
f :: forall a. (Eq a, Ord a) => a -> a -> Bool
f := \x y -> compare x y
main := f True False`
	checkSource(t, source, nil)
}

func TestConstraintProductElaboratesToTyEvidence(t *testing.T) {
	// The binding type should be TyEvidence with multiple entries.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
f :: forall a. (Eq a, Ord a) => a -> Bool
f := \x -> eq x x
main := f True`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "f" {
			ty := b.Type
			// Strip foralls.
			for {
				if fa, ok := ty.(*types.TyForall); ok {
					ty = fa.Body
				} else {
					break
				}
			}
			ev, ok := ty.(*types.TyEvidence)
			if !ok {
				t.Fatalf("expected TyEvidence, got %T", ty)
			}
			if len(ev.Constraints.Entries) != 2 {
				t.Fatalf("expected 2 constraint entries, got %d", len(ev.Constraints.Entries))
			}
			if ev.Constraints.Entries[0].ClassName != "Eq" {
				t.Errorf("expected Eq first, got %s", ev.Constraints.Entries[0].ClassName)
			}
			if ev.Constraints.Entries[1].ClassName != "Ord" {
				t.Errorf("expected Ord second, got %s", ev.Constraints.Entries[1].ClassName)
			}
		}
	}
}

func TestConstraintProductMixedWithCurried(t *testing.T) {
	// (Eq a, Ord a) => Show a => a -> Bool — product + curried.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
class Show a { show :: a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
instance Show Bool { show := \x -> True }
f :: forall a. (Eq a, Ord a) => Show a => a -> Bool
f := \x -> eq x x
main := f True`
	checkSource(t, source, nil)
}

func TestConstraintProductSingleParen(t *testing.T) {
	// (Eq a) => a -> Bool — single constraint in parens, not a product.
	// Should still work like a regular constraint.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
f :: forall a. (Eq a) => a -> Bool
f := \x -> eq x x
main := f True`
	checkSource(t, source, nil)
}

// =============================================================================
// Edge cases
// =============================================================================

func TestConstraintProductInInstanceContext(t *testing.T) {
	// instance (Eq a, Eq b) => Eq (Pair a b)
	// Instance declarations already parse parenthesized constraints separately,
	// but this tests that the surface syntax also works.
	source := `data Bool = True | False
data Pair a b = MkPair a b
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Eq a => Eq b => Eq (Pair a b) { eq := \x y -> True }
main := eq (MkPair True True) (MkPair False False)`
	checkSource(t, source, nil)
}

// =============================================================================
// Stress tests
// =============================================================================

func TestConstraintProductStressManyClasses(t *testing.T) {
	// Five independent classes, all constraints in a product.
	source := `data Bool = True | False
class C1 a { m1 :: a -> Bool }
class C2 a { m2 :: a -> Bool }
class C3 a { m3 :: a -> Bool }
class C4 a { m4 :: a -> Bool }
class C5 a { m5 :: a -> Bool }
instance C1 Bool { m1 := \x -> True }
instance C2 Bool { m2 := \x -> True }
instance C3 Bool { m3 := \x -> True }
instance C4 Bool { m4 := \x -> True }
instance C5 Bool { m5 := \x -> True }
f :: forall a. (C1 a, C2 a, C3 a, C4 a, C5 a) => a -> Bool
f := \x -> m1 x
main := f True`
	checkSource(t, source, nil)
}

func TestConstraintProductEquivCurried(t *testing.T) {
	// (Eq a, Ord a) => T should behave identically to Eq a => Ord a => T.
	sourceProd := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
f :: forall a. (Eq a, Ord a) => a -> a -> Bool
f := \x y -> eq x y
main := f True False`
	sourceCurr := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
f :: forall a. Eq a => Ord a => a -> a -> Bool
f := \x y -> eq x y
main := f True False`
	checkSource(t, sourceProd, nil)
	checkSource(t, sourceCurr, nil)
}
