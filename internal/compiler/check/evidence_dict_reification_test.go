package check

import (
	"strings"
	"testing"
)

// =============================================================================
// Phase 5E: Dict Reification
//
// data Dict (c: Constraint) := Dict c
//
// Dict c carries a class dictionary as a first-class value. The constructor
// field `c` is Constraint-kinded; it elaborates to the dictionary type.
// =============================================================================

func TestDictReificationBasic(t *testing.T) {
	// Declare Dict with a Constraint-kinded parameter and create a value.
	source := `data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
data Dict := \(c: Constraint). { MkDict: Dict c; }
mkDict :: Dict (Eq Bool)
mkDict := MkDict`
	checkSource(t, source, nil)
}

func TestDictReificationPatternMatch(t *testing.T) {
	// Pattern matching on Dict brings the evidence into scope.
	source := `data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
data Dict := \(c: Constraint). { MkDict: Dict c; }
useDict :: Dict (Eq Bool) -> Bool -> Bool -> Bool
useDict := \d x y. case d { MkDict => eq x y }
main := useDict MkDict True False`
	checkSource(t, source, nil)
}

func TestDictReificationPolymorphic(t *testing.T) {
	// Dict used polymorphically with explicit constraint.
	source := `data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
data Dict := \(c: Constraint). { MkDict: Dict c; }
withDict :: \ a. Eq a => Dict (Eq a) -> a -> a -> Bool
withDict := \d x y. case d { MkDict => eq x y }
main := withDict (MkDict :: Dict (Eq Bool)) True False`
	checkSource(t, source, nil)
}

func TestDictReificationMultipleConstraints(t *testing.T) {
	// Two Dict values with different constraints used together.
	source := `data Bool := { True: Bool; False: Bool; }
data Unit := { MkUnit: Unit; }
data Eq := \a. { eq: a -> a -> Bool }
data Show := \a. { show: a -> Unit }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. MkUnit }
data Dict := \(c: Constraint). { MkDict: Dict c; }
useEq :: Dict (Eq Bool) -> Bool -> Bool -> Bool
useEq := \d x y. case d { MkDict => eq x y }
useShow :: Dict (Show Bool) -> Bool -> Unit
useShow := \d x. case d { MkDict => show x }`
	checkSource(t, source, nil)
}

func TestDictReificationSuperclass(t *testing.T) {
	// Dict carries a subclass constraint; superclass evidence should be extractable.
	source := `data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
data Ord := \a. Eq a => { compare: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Ord Bool := { compare := \x y. True }
data Dict := \(c: Constraint). { MkDict: Dict c; }
useOrd :: Dict (Ord Bool) -> Bool -> Bool -> Bool
useOrd := \d x y. case d { MkDict => eq x y }`
	checkSource(t, source, nil)
}

func TestDictReificationPassThrough(t *testing.T) {
	// Dict value passed through a function without pattern matching.
	source := `data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
data Dict := \(c: Constraint). { MkDict: Dict c; }
passDict :: Dict (Eq Bool) -> Dict (Eq Bool)
passDict := \d. d`
	checkSource(t, source, nil)
}

func TestDictReificationNested(t *testing.T) {
	// Dict inside another data type.
	source := `data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
data Dict := \(c: Constraint). { MkDict: Dict c; }
data Pair := \a b. { MkPair: a -> b -> Pair a b; }
wrapDict :: Dict (Eq Bool) -> Pair (Dict (Eq Bool)) Bool
wrapDict := \d. MkPair d True`
	checkSource(t, source, nil)
}

func TestDictReificationErrorNoInstance(t *testing.T) {
	// Using eq on a type with no Eq instance should fail.
	source := `data Bool := { True: Bool; False: Bool; }
data Nat := { Zero: Nat; Succ: Nat -> Nat; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
bad := eq Zero Zero`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "no instance") {
		t.Errorf("expected 'no instance' error, got: %s", errMsg)
	}
}

func TestDictReificationMultipleFields(t *testing.T) {
	// Dict with a Constraint-kinded param alongside regular params.
	source := `data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
data Evidence := \(c: Constraint) a. { MkEvidence: a -> Evidence c a; }
useEvidence :: Evidence (Eq Bool) Bool -> Bool
useEvidence := \e. case e { MkEvidence x => eq x True }`
	checkSource(t, source, nil)
}

func TestDictReificationStressChain(t *testing.T) {
	// Chain of Dict pattern matches.
	source := `data Bool := { True: Bool; False: Bool; }
data Unit := { MkUnit: Unit; }
data Eq := \a. { eq: a -> a -> Bool }
data Show := \a. { show: a -> Unit }
impl Eq Bool := { eq := \x y. True }
impl Show Bool := { show := \x. MkUnit }
data Dict := \(c: Constraint). { MkDict: Dict c; }
chain :: Dict (Eq Bool) -> Dict (Show Bool) -> Bool -> Unit
chain := \d1 d2 x. case d1 { MkDict => case d2 { MkDict => show x } }`
	checkSource(t, source, nil)
}

func TestDictReificationInferredType(t *testing.T) {
	// Dict creation without explicit type annotation (inferred).
	source := `data Bool := { True: Bool; False: Bool; }
data Eq := \a. { eq: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
data Dict := \(c: Constraint). { MkDict: Dict c; }
useInferred := case (MkDict :: Dict (Eq Bool)) { MkDict => eq True False }`
	checkSource(t, source, nil)
}
