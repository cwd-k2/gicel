//go:build legacy_syntax

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/types"
)

// Type family fundep tests — type families, data families, and functional dependencies in combination.
// Does NOT cover: type family reduction (type_family_reduction_test.go), data family basics (data_family_test.go).

// --- 3a: Type family stress: nested families, recursive reduction, type-level computation ---

func TestAdvancedTypeFamilyStress(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	source := `
-- Peano naturals as DataKinds.
data Nat := { Z: (); S: Nat; }

-- Type-level addition.
type Add (a: Nat) (b: Nat) :: Nat := {
  Add Z b =: b;
  Add (S a) b =: S (Add a b)
}

-- Phantom type indexed by Nat.
data NatProxy := \(n: Nat). { MkProxy: NatProxy n; }

-- Elem: container element extraction.
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

type Elem (c: Type) :: Type := {
  Elem (List a) =: a;
  Elem (Maybe a) =: a
}

-- Wrap: type-level function.
type Wrap (a: Type) :: Type := {
  Wrap a =: Maybe a
}

-- Nested reduction: Elem (List (Wrap Int)) reduces as:
-- Wrap Int = Maybe Int, then Elem (List (Maybe Int)) = Maybe Int
nestedElem :: Elem (List (Wrap Int)) -> Maybe Int
nestedElem := \x. x

-- Season rotation: single-step NextSeason is fully reduced.
data Season := Spring | Summer | Autumn | Winter

type NextSeason (s: Season) :: Season := {
  NextSeason Spring =: Summer;
  NextSeason Summer =: Autumn;
  NextSeason Autumn =: Winter;
  NextSeason Winter =: Spring
}

data Tagged := \(s: Season). { MkTagged: Tagged s; }

-- NextSeason reduces for concrete season values.
nextProof :: Tagged (NextSeason Spring) -> Tagged Summer
nextProof := \x. x

-- Add (S (S Z)) Z = S (S Z): Peano addition at compile time.
addProof :: NatProxy (Add (S (S Z)) Z) -> NatProxy (S (S Z))
addProof := \x. x

-- Add Z (S Z) = S Z
addProof2 :: NatProxy (Add Z (S Z)) -> NatProxy (S Z)
addProof2 := \x. x

-- Add (S Z) (S Z) = S (S Z)
addProof3 :: NatProxy (Add (S Z) (S Z)) -> NatProxy (S (S Z))
addProof3 := \x. x
`
	checkSource(t, source, config)
}

// --- 3b: Data family polymorphism ---

func TestAdvancedDataFamilyPolymorphism(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"String": types.KType{},
		},
	}
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

-- Wrappable class: each type wraps into a different runtime shape.
data Wrappable := \w. {
  data Wrapped w :: Type;
  wrap: w -> Wrapped w;
  unwrap: Wrapped w -> w
}

impl Wrappable Int := {
  data Wrapped Int =: IntBox Int;
  wrap := \n. IntBox n;
  unwrap := \w. case w { IntBox n => n }
}

impl Wrappable Bool := {
  data Wrapped Bool =: BoolBit Bool;
  wrap := \b. BoolBit b;
  unwrap := \w. case w { BoolBit b => b }
}

impl Wrappable Unit := {
  data Wrapped Unit =: UnitBox;
  wrap := \_. UnitBox;
  unwrap := \_. Unit
}

impl Wrappable (Maybe a) := {
  data Wrapped (Maybe a) =: OptBox (Maybe a);
  wrap := \m. OptBox m;
  unwrap := \w. case w { OptBox m -> m }
}

-- Data family constructors are type-distinct.
boxedInt :: Wrapped Int
boxedInt := IntBox 42

boxedBool :: Wrapped Bool
boxedBool := BoolBit True

boxedUnit :: Wrapped Unit
boxedUnit := UnitBox

-- Pattern matching on data family types.
isIntBox :: Wrapped Int -> Int
isIntBox := \w. case w { IntBox n => n }

isBoolBit :: Wrapped Bool -> Bool
isBoolBit := \w. case w { BoolBit b => b }
`
	checkSource(t, source, config)
}

// --- 3c: Fundep inference ---

func TestAdvancedFunDepInference(t *testing.T) {
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"String": types.KType{},
		},
	}
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }

-- Convert class with fundep: source type determines target.
data Convert := \a b | a =: b. {
  convert: a -> b
}

impl Convert Bool Int := {
  convert := \b. case b { True => 1; False => 0 }
}

impl Convert Unit Bool := {
  convert := \_. True
}

-- Fundep inference: convert True → the checker deduces b = Int from a = Bool.
boolToInt :: Int
boolToInt := convert True

-- convert Unit → deduces b = Bool from a = Unit.
unitToBool :: Bool
unitToBool := convert Unit

-- HasElem class: container determines element type.
data HasElem := \c e | c =: e. {
  getFirst: c -> Maybe e
}

impl HasElem (List a) a := {
  getFirst := \xs. case xs { Cons x _ => Just x; Nil => Nothing }
}

impl HasElem (Maybe a) a := {
  getFirst := \m. case m { Just x => Just x; Nothing => Nothing }
}

-- getFirst (Cons Unit Nil): c = List Unit → e = Unit
firstOfList :: Maybe Unit
firstOfList := getFirst (Cons Unit Nil)

-- getFirst (Just True): c = Maybe Bool → e = Bool
firstOfMaybe :: Maybe Bool
firstOfMaybe := getFirst (Just True)

-- Bidirectional fundep.
data Iso := \a b | a =: b, b =: a. {
  forward: a -> b;
  backward: b -> a
}

impl Iso Bool Int := {
  forward := \b. case b { True => 1; False => 0 };
  backward := \_. True
}

-- forward True: a = Bool → b = Int
fwd :: Int
fwd := forward True

-- backward 0: b = Int → a = Bool
bwd :: Bool
bwd := backward 0
`
	checkSource(t, source, config)
}

// --- Additional: complex interactions ---

func TestAdvancedTypeFamilyWithDataFamily(t *testing.T) {
	// Combine closed type family with data family in the same program.
	source := `
data Unit := { Unit: Unit; }
data Bool := { True: Bool; False: Bool; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }

type IsJust (m: Type) :: Bool := {
  IsJust (Maybe a) =: True;
  IsJust _ =: False
}

data Container := \c. {
  data Elem c :: Type;
  cempty: c
}

impl Container (Maybe a) := {
  data Elem (Maybe a) =: MaybeElem a;
  cempty := Nothing
}

-- Both the closed TF and data family work together.
v :: Elem (Maybe Unit)
v := MaybeElem Unit

-- IsJust reduces for a concrete type.
data Phantom := \(b: Bool). { MkPhantom: Phantom b; }
proof :: Phantom (IsJust (Maybe Unit)) -> Phantom True
proof := \x. x
`
	checkSource(t, source, nil)
}

func TestAdvancedFunDepWithTypeFamily(t *testing.T) {
	// Fundep interacts with type family reduction.
	source := `
data Unit := { Unit: Unit; }
data Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a; }
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }

type Elem (c: Type) :: Type := {
  Elem (List a) =: a;
  Elem (Maybe a) =: a
}

-- Fundep class where one param is a type family application.
data Extract := \c e | c =: e. {
  extract: c -> Maybe e
}

impl Extract (List a) a := {
  extract := \xs. case xs { Cons x _ => Just x; Nil => Nothing }
}

-- extract (Cons Unit Nil): c = List Unit → e = Unit
result :: Maybe Unit
result := extract (Cons Unit Nil)
`
	checkSource(t, source, nil)
}

func TestAdvancedRecursiveTFWithPhantom(t *testing.T) {
	// Recursive type family Dual for session types with concrete usage.
	source := `
data Session := { Send: Session; Recv: Session; End: (); }

type Dual (s: Session) :: Session := {
  Dual (Send s) =: Recv (Dual s);
  Dual (Recv s) =: Send (Dual s);
  Dual End =: End
}

data Chan := \(s: Session). { MkChan: Chan s; }

-- Dual (Send End) = Recv (Dual End) = Recv End
dualProof1 :: Chan (Dual (Send End)) -> Chan (Recv End)
dualProof1 := \x. x

-- Dual (Recv (Send End)) = Send (Dual (Send End)) = Send (Recv End)
dualProof2 :: Chan (Dual (Recv (Send End))) -> Chan (Send (Recv End))
dualProof2 := \x. x
`
	checkSource(t, source, nil)
}
