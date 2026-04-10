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
form Collection := \a. {
  form Elem a :: Type;
  empty: a
}
`
	checkSource(t, source, nil)
}

func TestDataFamilyParseInstanceDef(t *testing.T) {
	// data family instance inside instance body
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

form Collection := \a. {
  form Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  form Elem := ListElem a;
  empty := Nil
}
`
	checkSource(t, source, nil)
}

// --- Constructor registration ---

func TestDataFamilyConstructorType(t *testing.T) {
	// t.Skip("form family constructor registration: type mismatch in mangled name")
	// The mangled constructor should be usable as a value
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

form Collection := \a. {
  form Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  form Elem := ListElem a;
  empty := Nil
}

x :: Elem (List Unit)
x := ListElem Unit
`
	checkSource(t, source, nil)
}

func TestDataFamilyMultipleInstances(t *testing.T) {
	// Formerly skipped: "form family constructor registration: type mismatch
	// in mangled name". Resolved by kindOfType universe stratification (S-1).
	// Different instances define different constructors for the same family
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

form Collection := \a. {
  form Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  form Elem := ListElem a;
  empty := Nil
}

impl Collection Unit := {
  form Elem := UnitElem;
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
	// Formerly skipped: "form family constructor registration: type mismatch
	// in mangled name". Resolved by kindOfType universe stratification (S-1).
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

form Collection := \a. {
  form Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  form Elem := ListElem a;
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
form Unit := { Unit: Unit; }
form Foo := \a. {
  empty: a
}
impl Foo Unit := {
  form Elem := UnitElem;
  empty := Unit
}
`
	checkSourceExpectError(t, source, nil)
}

func TestDataFamilyArityMismatch(t *testing.T) {
	// Defining a data family member not declared in the class should error.
	source := `
form Unit := { Unit: Unit; }
form Collection := \a. {
  form Elem a :: Type;
  empty: a
}
impl Collection Unit := {
  form NotElem := Bad;
  empty := Unit
}
`
	checkSourceExpectError(t, source, nil)
}

// --- Reduction: Elem as data family reduces like type family ---

func TestDataFamilyTypeReduction(t *testing.T) {
	// Formerly skipped: "form family constructor registration: type mismatch
	// in mangled name". Resolved by kindOfType universe stratification (S-1).
	// Elem (List Unit) should be usable as a type that accepts ListElem
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

form Collection := \a. {
  form Elem a :: Type;
  empty: a
}

impl Collection (List a) := {
  form Elem := ListElem a;
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
	t.Skip("multi-constructor associated form families not yet supported by parser")
	source := `
form Unit := { Unit: Unit; }

form Container := \a. {
  form Entry a :: Type;
  empty: a
}

impl Container Unit := {
  form Entry := Singleton Unit | Empty;
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
	t.Skip("multi-constructor associated form families not yet supported by parser")
	source := `
form Unit := { Unit: Unit; }

form Container := \a. {
  form Entry a :: Type;
  empty: a
}

impl Container Unit := {
  form Entry := Singleton Unit | Empty;
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
