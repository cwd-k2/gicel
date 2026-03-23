package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Type family fundep tests — type families, class instances, and associated types in combination.
// Does NOT cover: type family reduction (type_family_reduction_test.go), data family basics (data_family_test.go).
// Note: fundep annotations and data family constructors are not supported in unified syntax.

// --- 3a: Type family stress: nested families, recursive reduction, type-level computation ---

func TestAdvancedTypeFamilyStress(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	source := `
-- Peano naturals as DataKinds.
form Nat := { Z: (); S: Nat; }

-- Type-level addition (encoded via NatPair since multi-param families
-- require unified syntax encoding).
form NatPair := \(a: Nat) (b: Nat). { MkNatPair: NatPair a b; }
type Add :: Nat := \(p: Type). case p {
  (NatPair Z b) => b;
  (NatPair (S a) b) => S (Add (NatPair a b))
}

-- Phantom type indexed by Nat.
form NatProxy := \(n: Nat). { MkProxy: NatProxy n; }

-- Elem: container element extraction.
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  (Maybe a) => a
}

-- Wrap: type-level function.
type Wrap :: Type := \(a: Type). case a {
  a => Maybe a
}

-- Nested reduction: Elem (List (Wrap Int)) reduces as:
-- Wrap Int = Maybe Int, then Elem (List (Maybe Int)) = Maybe Int
nestedElem :: Elem (List (Wrap Int)) -> Maybe Int
nestedElem := \x. x

-- Season rotation: single-step NextSeason is fully reduced.
form Season := { Spring: Season; Summer: Season; Autumn: Season; Winter: Season; }

type NextSeason :: Season := \(s: Season). case s {
  Spring => Summer;
  Summer => Autumn;
  Autumn => Winter;
  Winter => Spring
}

form Tagged := \(s: Season). { MkTagged: Tagged s; }

-- NextSeason reduces for concrete season values.
nextProof :: Tagged (NextSeason Spring) -> Tagged Summer
nextProof := \x. x

-- Add (NatPair (S (S Z)) Z) = S (S Z): Peano addition at compile time.
addProof :: NatProxy (Add (NatPair (S (S Z)) Z)) -> NatProxy (S (S Z))
addProof := \x. x

-- Add (NatPair Z (S Z)) = S Z
addProof2 :: NatProxy (Add (NatPair Z (S Z))) -> NatProxy (S Z)
addProof2 := \x. x

-- Add (NatPair (S Z) (S Z)) = S (S Z)
addProof3 :: NatProxy (Add (NatPair (S Z) (S Z))) -> NatProxy (S (S Z))
addProof3 := \x. x
`
	checkSource(t, source, config)
}

// --- 3b: Associated type family polymorphism ---

func TestAdvancedDataFamilyPolymorphism(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"String": types.KType{},
		},
	}
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

-- Wrappable class: each type wraps into the same shape (associated type family).
-- Data family constructors are not supported in unified syntax.
form Wrappable := \w. {
  type Wrapped w :: Type;
  wrap: w -> Wrapped w;
  unwrap: Wrapped w -> w
}

impl Wrappable Int := {
  type Wrapped := Int;
  wrap := \n. n;
  unwrap := \w. w
}

impl Wrappable Bool := {
  type Wrapped := Bool;
  wrap := \b. b;
  unwrap := \w. w
}

impl Wrappable Unit := {
  type Wrapped := Unit;
  wrap := \_. Unit;
  unwrap := \_. Unit
}

-- Verify associated type reduces correctly.
boxedInt :: Wrapped Int
boxedInt := 42

boxedBool :: Wrapped Bool
boxedBool := True

boxedUnit :: Wrapped Unit
boxedUnit := Unit
`
	checkSource(t, source, config)
}

// --- 3c: Class with multiple params and instance resolution ---

func TestAdvancedFunDepInference(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"String": types.KType{},
		},
	}
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

-- Convert class (without fundep annotation in unified syntax).
form Convert := \a b. {
  convert: a -> b
}

impl Convert Bool Int := {
  convert := \b. case b { True => 1; False => 0 }
}

impl Convert Unit Bool := {
  convert := \_. True
}

-- Instance resolution: convert True with annotation deduces b = Int.
boolToInt :: Int
boolToInt := convert True

-- convert Unit with annotation deduces b = Bool.
unitToBool :: Bool
unitToBool := convert Unit

-- HasElem class.
form HasElem := \c e. {
  getFirst: c -> Maybe e
}

impl HasElem (List a) a := {
  getFirst := \xs. case xs { Cons x _ => Just x; Nil => Nothing }
}

impl HasElem (Maybe a) a := {
  getFirst := \m. case m { Just x => Just x; Nothing => Nothing }
}

-- getFirst (Cons Unit Nil): c = List Unit, e = Unit
firstOfList :: Maybe Unit
firstOfList := getFirst (Cons Unit Nil)

-- getFirst (Just True): c = Maybe Bool, e = Bool
firstOfMaybe :: Maybe Bool
firstOfMaybe := getFirst (Just True)

-- Bidirectional class (without fundep annotation).
form Iso := \a b. {
  forward: a -> b;
  backward: b -> a
}

impl Iso Bool Int := {
  forward := \b. case b { True => 1; False => 0 };
  backward := \_. True
}

-- forward True: a = Bool, b = Int
fwd :: Int
fwd := forward True

-- backward 0: b = Int, a = Bool
bwd :: Bool
bwd := backward 0
`
	checkSource(t, source, config)
}

// --- Additional: complex interactions ---

func TestAdvancedTypeFamilyWithDataFamily(t *testing.T) {
	// Combine closed type family with associated type family in the same program.
	source := `
form Unit := { Unit: Unit; }
form Bool := { True: Bool; False: Bool; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

type IsJust :: Bool := \(m: Type). case m {
  (Maybe a) => True;
  _ => False
}

form Container := \c. {
  type Elem c :: Type;
  cempty: c
}

impl Container (Maybe a) := {
  type Elem := a;
  cempty := Nothing
}

-- Both the closed TF and associated type work together.
v :: Elem (Maybe Unit) -> Unit
v := \x. x

-- IsJust reduces for a concrete type.
form Phantom := \(b: Bool). { MkPhantom: Phantom b; }
proof :: Phantom (IsJust (Maybe Unit)) -> Phantom True
proof := \x. x
`
	checkSource(t, source, nil)
}

func TestAdvancedFunDepWithTypeFamily(t *testing.T) {
	// Class interacts with type family reduction.
	source := `
form Unit := { Unit: Unit; }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  (Maybe a) => a
}

-- Class where one param is the element type.
form Extract := \c e. {
  extract: c -> Maybe e
}

impl Extract (List a) a := {
  extract := \xs. case xs { Cons x _ => Just x; Nil => Nothing }
}

-- extract (Cons Unit Nil): c = List Unit, e = Unit
result :: Maybe Unit
result := extract (Cons Unit Nil)
`
	checkSource(t, source, nil)
}

func TestAdvancedRecursiveTFWithPhantom(t *testing.T) {
	// Recursive type family Dual for session types with concrete usage.
	source := `
form Session := { Send: Session; Recv: Session; End: (); }

type Dual :: Session := \(s: Session). case s {
  (Send s) => Recv (Dual s);
  (Recv s) => Send (Dual s);
  End => End
}

form Chan := \(s: Session). { MkChan: Chan s; }

-- Dual (Send End) = Recv (Dual End) = Recv End
dualProof1 :: Chan (Dual (Send End)) -> Chan (Recv End)
dualProof1 := \x. x

-- Dual (Recv (Send End)) = Send (Dual (Send End)) = Send (Recv End)
dualProof2 :: Chan (Dual (Recv (Send End))) -> Chan (Send (Recv End))
dualProof2 := \x. x
`
	checkSource(t, source, nil)
}
