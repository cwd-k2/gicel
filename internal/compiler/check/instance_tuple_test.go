package check

import (
	"testing"
)

// Verify type class instances can be defined for tuple/unit types.

func TestInstanceEqUnit(t *testing.T) {
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq () { eq := \_ _. True }
main := eq () ()`
	checkSource(t, source, nil)
}

func TestInstanceEqPairTuple(t *testing.T) {
	source := `data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq b => Eq (a, b) {
  eq := \x y. case x {
    (a1, b1) -> case y {
      (a2, b2) -> case eq a1 a2 { True => eq b1 b2; False => False }
    }
  }
}
main := eq (True, False) (True, True)`
	checkSource(t, source, nil)
}
