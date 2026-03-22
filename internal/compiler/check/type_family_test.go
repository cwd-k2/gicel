package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Parser: type family declarations ---

func TestTypeFamilyParseBasic(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
type IsTrue (b: Bool) :: Bool := {
  IsTrue True =: True;
  IsTrue False =: False
}
`
	checkSource(t, source, nil)
}

func TestTypeFamilyParseTwoParams(t *testing.T) {
	source := `
data Mult := { Unrestricted: (); Affine: (); Linear: (); }
type LUB (m1: Mult) (m2: Mult) :: Mult := {
  LUB Linear _ =: Linear;
  LUB _ Linear =: Linear;
  LUB Affine _ =: Affine;
  LUB _ Affine =: Affine;
  LUB Unrestricted Unrestricted =: Unrestricted
}
`
	checkSource(t, source, nil)
}

// --- Reduction: Type-kinded type families ---

func TestTypeFamilyReduceAppPattern(t *testing.T) {
	// Elem (List a) = a: result kind is Type, so it can be used as a value type.
	source := `
data List := \a. { Nil: (); Cons: (a, List a); }
data Unit := { Unit: (); }
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
f :: Elem (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyReduceMultiEquations(t *testing.T) {
	// Multiple equations, Type-kinded result.
	source := `
data List := \a. { Nil: (); Cons: (a, List a); }
data Unit := { Unit: (); }
data Pair := \a b. { MkPair: (a, b); }
type Elem (c: Type) :: Type := {
  Elem (List a) =: a;
  Elem (Pair a b) =: a
}
f :: Elem (List Unit) -> Unit
f := \x. x
g :: Elem (Pair Unit Unit) -> Unit
g := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyReduceIdentity(t *testing.T) {
	// Simple identity-like type family.
	source := `
data Unit := { Unit: (); }
type Id (a: Type) :: Type := {
  Id a =: a
}
f :: Id Unit => Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyReduceConstant(t *testing.T) {
	// Constant type family: always returns the same type.
	source := `
data Unit := { Unit: (); }
data List := \a. { Nil: (); Cons: (a, List a); }
type AlwaysUnit (a: Type) :: Type := {
  AlwaysUnit a =: Unit
}
f :: AlwaysUnit (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Stuck reduction ---

func TestTypeFamilyStuckOnMeta(t *testing.T) {
	// A Type-kinded family with a skolem argument: reduction is stuck.
	// The stuck TyFamilyApp should unify with itself.
	source := `
data List := \a. { Nil: (); Cons: (a, List a); }
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
f :: \ c. Elem c -> Elem c
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Wildcard patterns ---

func TestTypeFamilyWildcard(t *testing.T) {
	source := `
data Unit := { Unit: (); }
data List := \a. { Nil: (); Cons: (a, List a); }
type AlwaysUnit (a: Type) :: Type := {
  AlwaysUnit _ =: Unit
}
f :: AlwaysUnit (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Constraint families ---

func TestConstraintFamily(t *testing.T) {
	source := `
data Serialization := JSON | Binary
class Show a {
  show :: a -> a
}
type Serializable (fmt: Serialization) :: Constraint := {
  Serializable JSON =: Show;
  Serializable Binary =: Show
}
`
	checkSource(t, source, nil)
}

// --- Error cases ---

func TestTypeFamilyArityMismatch(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
type F (a: Bool) :: Bool := {
  F True False =: True
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyEquation)
}

func TestTypeFamilyNameMismatch(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
type F (a: Bool) :: Bool := {
  G True =: True
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeFamilyEquation)
}

func TestTypeFamilyInjectivityViolation(t *testing.T) {
	// Elem (List Unit) = Unit and Elem Unit = Unit: RHSes both Unit,
	// but LHS patterns (List a) and Unit cannot unify → injectivity violation.
	source := `
data Unit := { Unit: (); }
data List := \a. { Nil: (); Cons: (a, List a); }
type Elem (c: Type) :: (r: Type) | r =: c := {
  Elem (List a) =: a;
  Elem Unit =: Unit
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrInjectivity)
}

func TestTypeFamilyDuplicate(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
type F (a: Bool) :: Bool := {
  F True =: True
}
type F (a: Bool) :: Bool := {
  F True =: False
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

// --- Type family used in function type ---

func TestTypeFamilyInFunctionType(t *testing.T) {
	source := `
data List := \a. { Nil: (); Cons: (a, List a); }
data Unit := { Unit: (); }
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}
map :: \ a b. (a -> b) -> List a -> List b
map := assumption
length :: \ a. List a -> Int
length := assumption
first :: \ a. List a -> Elem (List a)
first := assumption
main :: Int
main := length (map (\x. x) (Cons Unit Nil))
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		Assumptions:     map[string]types.Type{},
	}
	checkSource(t, source, config)
}

// --- Regression: type aliases still work ---

func TestTypeAliasStillWorks(t *testing.T) {
	source := `
data Unit := { Unit: (); }
type Id a := a
f :: Id Unit => Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Associated types ---

func TestAssocTypeBasic(t *testing.T) {
	source := `
data List := \a. { Nil: (); Cons: (a, List a); }
data Unit := { Unit: (); }

class Container c {
  type Elem c :: Type;
  cfold :: \ b. (Elem c -> b -> b) -> b -> c -> b
}

instance Container (List a) {
  type Elem (List a) =: a;
  cfold := foldr
}

foldr :: \ a b. (a -> b -> b) -> b -> List a -> b
foldr := assumption

f :: Elem (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestAssocTypeMultipleInstances(t *testing.T) {
	source := `
data List := \a. { Nil: (); Cons: (a, List a); }
data Unit := { Unit: (); }
data Pair := \a b. { MkPair: (a, b); }

class Container c {
  type Elem c :: Type;
  clength :: c -> Int
}

instance Container (List a) {
  type Elem (List a) =: a;
  clength := listLength
}

instance Container (Pair a b) {
  type Elem (Pair a b) =: a;
  clength := pairLength
}

listLength :: \ a. List a -> Int
listLength := assumption

pairLength :: \ a b. Pair a b -> Int
pairLength := assumption

f :: Elem (List Unit) -> Unit
f := \x. x
g :: Elem (Pair Unit Unit) -> Unit
g := \x. x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// --- Recursive type families ---

func TestRecursiveTypeFamilyDual(t *testing.T) {
	source := `
data Session := { Send: Session; Recv: Session; End: (); }
type Dual (s: Session) :: Session := {
  Dual (Send s) =: Recv (Dual s);
  Dual (Recv s) =: Send (Dual s);
  Dual End =: End
}
`
	checkSource(t, source, nil)
}

func TestRecursiveTypeFamilyFuelExhaustion(t *testing.T) {
	// Cycle detected via sentinel memoization; the family remains stuck (unreduced),
	// producing a type mismatch (E0200) when Loop Unit is compared against Unit.
	source := `
data Unit := { Unit: (); }
type Loop (a: Type) :: Type := {
  Loop a =: Loop a
}
f :: Loop Unit => Unit
f := \x. x
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

// --- Functional dependencies ---

func TestFunDepParse(t *testing.T) {
	source := `
data Unit := { Unit: (); }
data List := \a. { Nil: (); Cons: (a, List a); }
class Elem c e | c =: e {
  cfold :: (e -> e) -> c -> c
}
instance Elem (List a) a {
  cfold := \f xs. xs
}
`
	checkSource(t, source, nil)
}

func TestFunDepUnknownParam(t *testing.T) {
	source := `
class Bad a b | z =: b {
  m :: a -> b
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrBadClass)
}

// --- Session types (Phase 7) ---

func TestSessionTypeDual(t *testing.T) {
	// Session types as a library feature on recursive TF + DataKinds.
	source := `
data Session := { Send: Session; Recv: Session; End: (); }
type Dual (s: Session) :: Session := {
  Dual (Send s) =: Recv (Dual s);
  Dual (Recv s) =: Send (Dual s);
  Dual End =: End
}
`
	checkSource(t, source, nil)
}

func TestSessionTypeDualOfDual(t *testing.T) {
	// Dual (Dual s) should reduce back to s for concrete sessions.
	// This tests recursive TF with promoted constructor patterns.
	source := `
data Session := { Send: Session; Recv: Session; End: (); }
type Dual (s: Session) :: Session := {
  Dual (Send s) =: Recv (Dual s);
  Dual (Recv s) =: Send (Dual s);
  Dual End =: End
}
`
	// Just verify parsing + registration — full reduction of
	// Dual(Dual(Send End)) requires multiple recursive steps.
	checkSource(t, source, nil)
}

// --- Divergent Post-States (Phase 6) ---

func TestDivergentPostStatesLUBDefined(t *testing.T) {
	// With LUB type family defined and multiplicity on capabilities,
	// case branches with different post-states should be joined via LUB.
	// This test verifies the structural readiness: lubPostStates falls back
	// to unification when LUB is not yet wired in.
	source := `
data Bool := { True: (); False: (); }
data Unit := { Unit: (); }
f :: Bool -> Unit
f := \b. case b {
  True => Unit;
  False => Unit
}
`
	checkSource(t, source, nil)
}

// --- Graded Evidence: RowField.Grades ---

func TestRowFieldGradesPretty(t *testing.T) {
	// RowField with Grades should pretty-print as "label: Type @ Grade"
	row := types.ClosedRow(types.RowField{
		Label:  "handle",
		Type:   &types.TyCon{Name: "FileHandle"},
		Grades: []types.Type{&types.TyCon{Name: "Linear"}},
	})
	s := types.Pretty(row)
	if !strings.Contains(s, "@ Linear") {
		t.Errorf("expected '@ Linear' in pretty output, got %q", s)
	}
}

func TestRowFieldGradesEquality(t *testing.T) {
	a := types.ClosedRow(types.RowField{Label: "x", Type: &types.TyCon{Name: "Int"}, Grades: []types.Type{&types.TyCon{Name: "Linear"}}})
	b := types.ClosedRow(types.RowField{Label: "x", Type: &types.TyCon{Name: "Int"}, Grades: []types.Type{&types.TyCon{Name: "Linear"}}})
	c := types.ClosedRow(types.RowField{Label: "x", Type: &types.TyCon{Name: "Int"}, Grades: []types.Type{&types.TyCon{Name: "Affine"}}})
	d := types.ClosedRow(types.RowField{Label: "x", Type: &types.TyCon{Name: "Int"}}) // no Grades

	if !types.Equal(a, b) {
		t.Error("same Grades should be equal")
	}
	if types.Equal(a, c) {
		t.Error("different Grades should not be equal")
	}
	if types.Equal(a, d) {
		t.Error("Grades vs no-Grades should not be equal")
	}
}

func TestRowFieldGradesSubst(t *testing.T) {
	row := types.ClosedRow(types.RowField{
		Label:  "cap",
		Type:   &types.TyVar{Name: "a"},
		Grades: []types.Type{&types.TyVar{Name: "m"}},
	})
	result := types.Subst(row, "m", &types.TyCon{Name: "Linear"})
	evRow, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := evRow.CapFields()
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if len(fields[0].Grades) != 1 {
		t.Fatalf("expected 1 grade, got %d", len(fields[0].Grades))
	}
	grade := fields[0].Grades[0]
	if con, ok := grade.(*types.TyCon); !ok || con.Name != "Linear" {
		t.Errorf("expected Grade = Linear, got %v", grade)
	}
}

func TestRowFieldGradesZonk(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	u.InstallTempSolution(1, &types.TyCon{Name: "Affine"})
	row := types.ClosedRow(types.RowField{
		Label:  "cap",
		Type:   &types.TyCon{Name: "Int"},
		Grades: []types.Type{meta},
	})
	result := u.Zonk(row)
	evRow, ok := result.(*types.TyEvidenceRow)
	if !ok {
		t.Fatalf("expected TyEvidenceRow, got %T", result)
	}
	fields := evRow.CapFields()
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if len(fields[0].Grades) != 1 {
		t.Fatalf("expected 1 grade, got %d", len(fields[0].Grades))
	}
	grade := fields[0].Grades[0]
	if con, ok := grade.(*types.TyCon); !ok || con.Name != "Affine" {
		t.Errorf("expected Grade = Affine after zonk, got %v", grade)
	}
}

// --- Unit tests for TyFamilyApp operations ---

func TestTyFamilyAppPretty(t *testing.T) {
	ty := &types.TyFamilyApp{
		Name: "Elem",
		Args: []types.Type{
			&types.TyApp{
				Fun: &types.TyCon{Name: "List"},
				Arg: &types.TyCon{Name: "Int"},
			},
		},
		Kind: types.KType{},
	}
	s := types.Pretty(ty)
	if !strings.Contains(s, "Elem") {
		t.Errorf("expected 'Elem' in pretty output, got %q", s)
	}
	if !strings.Contains(s, "List Int") {
		t.Errorf("expected 'List Int' in pretty output, got %q", s)
	}
}

func TestTyFamilyAppSubst(t *testing.T) {
	tf := &types.TyFamilyApp{
		Name: "F",
		Args: []types.Type{&types.TyVar{Name: "a"}},
		Kind: types.KType{},
	}
	result := types.Subst(tf, "a", &types.TyCon{Name: "Int"})
	fa, ok := result.(*types.TyFamilyApp)
	if !ok {
		t.Fatalf("expected TyFamilyApp, got %T", result)
	}
	con, ok := fa.Args[0].(*types.TyCon)
	if !ok || con.Name != "Int" {
		t.Errorf("expected arg to be Int, got %v", fa.Args[0])
	}
}

func TestTyFamilyAppFreeVars(t *testing.T) {
	tf := &types.TyFamilyApp{
		Name: "F",
		Args: []types.Type{
			&types.TyVar{Name: "a"},
			&types.TyCon{Name: "Int"},
		},
		Kind: types.KType{},
	}
	fv := types.FreeVars(tf)
	if _, ok := fv["a"]; !ok {
		t.Error("expected 'a' in free vars")
	}
	if len(fv) != 1 {
		t.Errorf("expected 1 free var, got %d", len(fv))
	}
}

func TestTyFamilyAppEqual(t *testing.T) {
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{&types.TyCon{Name: "Int"}}, Kind: types.KType{}}
	b := &types.TyFamilyApp{Name: "F", Args: []types.Type{&types.TyCon{Name: "Int"}}, Kind: types.KType{}}
	c := &types.TyFamilyApp{Name: "F", Args: []types.Type{&types.TyCon{Name: "Bool"}}, Kind: types.KType{}}
	if !types.Equal(a, b) {
		t.Error("equal TyFamilyApps should be equal")
	}
	if types.Equal(a, c) {
		t.Error("different TyFamilyApps should not be equal")
	}
}

func TestTyFamilyAppZonk(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	u.InstallTempSolution(1, &types.TyCon{Name: "Int"})
	tf := &types.TyFamilyApp{
		Name: "F",
		Args: []types.Type{meta},
		Kind: types.KType{},
	}
	result := u.Zonk(tf)
	fa, ok := result.(*types.TyFamilyApp)
	if !ok {
		t.Fatalf("expected TyFamilyApp, got %T", result)
	}
	con, ok := fa.Args[0].(*types.TyCon)
	if !ok || con.Name != "Int" {
		t.Errorf("expected arg to be Int after zonk, got %v", fa.Args[0])
	}
}

// --- Check-mode App: associated type family return-context reduction ---

func TestCheckAppAssocTypeFromReturnContext(t *testing.T) {
	// Associated type Elem depends on class parameter c.
	// fromList :: List (Elem c) -> c.
	// When called as `fromList (Cons True Nil) :: List Bool`,
	// the expected return type `List Bool` must solve ?c = List Bool
	// BEFORE checking the argument, so that Elem (List Bool) reduces to Bool.
	source := `
data Bool := { True: (); False: (); }
data List := \a. { Nil: (); Cons: (a, List a); }

class FromList c {
  type Elem c :: Type;
  fromList :: List (Elem c) -> c
}

instance FromList (List a) {
  type Elem (List a) =: a;
  fromList := \xs. xs
}

main :: List Bool
main := fromList (Cons True Nil)
`
	checkSource(t, source, nil)
}

func TestCheckAppAssocTypeInfixFromReturnContext(t *testing.T) {
	// Same scenario but using an infix operator.
	source := `
data Bool := { True: (); False: (); }
data List := \a. { Nil: (); Cons: (a, List a); }

class Conv c {
  type Elem c :: Type;
  conv :: List (Elem c) -> c
}

instance Conv (List a) {
  type Elem (List a) =: a;
  conv := \xs. xs
}

infixr 0 <|
(<|) :: \ a b. (a -> b) -> a -> b
(<|) := \f x. f x

main :: List Bool
main := conv <| (Cons True Nil)
`
	checkSource(t, source, nil)
}

func TestCheckAppAssocTypeAnnotationStillWorks(t *testing.T) {
	// Annotation-based workaround should still work alongside the fix.
	source := `
data Bool := { True: (); False: (); }
data List := \a. { Nil: (); Cons: (a, List a); }

class FromList c {
  type Elem c :: Type;
  fromList :: List (Elem c) -> c
}

instance FromList (List a) {
  type Elem (List a) =: a;
  fromList := \xs. xs
}

main := (fromList (Cons True Nil)) :: List Bool
`
	checkSource(t, source, nil)
}

func TestCheckAppNoRegressionPlainFunction(t *testing.T) {
	// Ensure plain function application still works correctly in check mode.
	source := `
data Bool := { True: (); False: (); }
data List := \a. { Nil: (); Cons: (a, List a); }

id :: \ a. a -> a
id := \x. x

main :: List Bool
main := id (Cons True Nil)
`
	checkSource(t, source, nil)
}

func TestCheckAppCBPVSpecialFormsFallback(t *testing.T) {
	// CBPV special forms (pure, thunk, force) must still work through
	// the infer + subsCheck path, not the new checkApp path.
	source := `
data Unit := { Unit: (); }

main :: Computation {} {} Unit
main := pure Unit
`
	checkSource(t, source, nil)
}

func TestCheckAppAssocTypeNestedApp(t *testing.T) {
	// Nested application where both inner and outer app benefit from
	// return-context type propagation.
	source := `
data Bool := { True: (); False: (); }
data List := \a. { Nil: (); Cons: (a, List a); }

class Convert c {
  type Elem c :: Type;
  convert :: Elem c -> c
}

instance Convert (List a) {
  type Elem (List a) =: a;
  convert := \x. Cons x Nil
}

wrap :: \ a. a -> a
wrap := \x. x

main :: List Bool
main := wrap (convert True)
`
	checkSource(t, source, nil)
}
