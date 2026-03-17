package check

import "testing"

// =============================================================================
// Phase 5A: Constraint Aliases via existing type alias mechanism
// =============================================================================

func TestConstraintAliasSimple(t *testing.T) {
	// type Eqable a = Eq a => a -> a -> Bool
	// f :: \ a. Eqable a
	// f := \x y. eq x y
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
type Eqable a = Eq a => a -> a -> Bool
f :: \ a. Eqable a
f := \x y. eq x y
main := f True False`
	checkSource(t, source, nil)
}

func TestConstraintAliasMulti(t *testing.T) {
	// type EqOrd a = Eq a => Ord a => a -> Bool
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }
type EqOrd a = Eq a => Ord a => a -> Bool
f :: \ a. EqOrd a
f := \x. eq x x
main := f True`
	checkSource(t, source, nil)
}
