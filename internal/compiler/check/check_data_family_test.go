package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// ==========================================
// Phase 8: Data Families — TDD tests
// ==========================================

// --- Parsing ---

func TestDataFamilyParseClassDecl(t *testing.T) {
	// data family declaration inside class body
	source := `
data Collection := \a. {
  data Elem a :: Type;
  empty: a
}
`
	checkSource(t, source, nil)
}

func TestDataFamilyParseInstanceDef(t *testing.T) {
	// data family instance inside instance body
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }

data Collection := \a. {
  data Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  data Elem := ListElem a;
  empty := Nil
}
`
	checkSource(t, source, nil)
}

// --- Constructor registration ---

func TestDataFamilyConstructorType(t *testing.T) {
	// The mangled constructor should be usable as a value
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }

data Collection := \a. {
  data Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  data Elem := ListElem a;
  empty := Nil
}

x :: Elem (List Unit)
x := ListElem Unit
`
	checkSource(t, source, nil)
}

func TestDataFamilyMultipleInstances(t *testing.T) {
	// Different instances define different constructors for the same family
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }

data Collection := \a. {
  data Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  data Elem := ListElem a;
  empty := Nil
}

impl Collection Unit := {
  data Elem := UnitElem;
  empty := Unit
}

x :: Elem (List Unit)
x := ListElem Unit

y :: Elem Unit
y := UnitElem
`
	checkSource(t, source, nil)
}

// --- Pattern matching ---

func TestDataFamilyPatternMatch(t *testing.T) {
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }

data Collection := \a. {
  data Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  data Elem := ListElem a;
  empty := Nil
}

unwrap :: \ a. Elem (List a) -> a
unwrap := \e. case e { ListElem x => x }
`
	checkSource(t, source, nil)
}

// --- Error cases ---

func TestDataFamilyNotDeclaredInClass(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }
data Foo := \a. {
  empty: a
}
impl Foo Unit := {
  data Elem := UnitElem;
  empty := Unit
}
`
	checkSourceExpectError(t, source, nil)
}

func TestDataFamilyArityMismatch(t *testing.T) {
	// Defining a data family member not declared in the class should error.
	source := `
data Unit := { Unit: Unit; }
data Collection := \a. {
  data Elem a :: Type;
  empty: a
}
impl Collection Unit := {
  data NotElem := Bad;
  empty := Unit
}
`
	checkSourceExpectError(t, source, nil)
}

// --- Reduction: Elem as data family reduces like type family ---

func TestDataFamilyTypeReduction(t *testing.T) {
	// Elem (List Unit) should be usable as a type that accepts ListElem
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	source := `
data List := \a. { Nil: List a; Cons: a -> List a -> List a; }
data Unit := { Unit: Unit; }

data Collection := \a. {
  data Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  data Elem := ListElem a;
  empty := Nil
}

wrap :: \ a. a -> Elem (List a)
wrap := \x. ListElem x

id :: Elem (List Int) -> Elem (List Int)
id := \x. x
`
	checkSource(t, source, config)
}

// --- Data family with multiple constructors ---

func TestDataFamilyMultipleConstructors(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }

data Container := \a. {
  data Entry a :: Type;
  empty: a
}

impl Container Unit := {
  data Entry := Singleton Unit | Empty;
  empty := Unit
}

f :: Entry Unit -> Unit
f := \e. case e {
  Singleton x => x;
  Empty => Unit
}
`
	checkSource(t, source, nil)
}

// --- Exhaustiveness ---

func TestDataFamilyExhaustiveness(t *testing.T) {
	source := `
data Unit := { Unit: Unit; }

data Container := \a. {
  data Entry a :: Type;
  empty: a
}

impl Container Unit := {
  data Entry := Singleton Unit | Empty;
  empty := Unit
}

f :: Entry Unit -> Unit
f := \e. case e {
  Singleton x => x
}
`
	// Missing Empty branch → non-exhaustive
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
}
