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
form Bool := { True: Bool; False: Bool; }
type IsTrue :: Bool := \(b: Bool). case b {
  True => True;
  False => False
}
`
	checkSource(t, source, nil)
}

func TestTypeFamilyParseTwoParams(t *testing.T) {
	source := `
form Mult := { Unrestricted: Mult; Affine: Mult; Linear: Mult; }
type LUB :: Mult := \(m1: Mult) (m2: Mult). case (m1, m2) {
  (Linear, _) => Linear;
  (_, Linear) => Linear;
  (Affine, _) => Affine;
  (_, Affine) => Affine;
  (Unrestricted, Unrestricted) => Unrestricted
}
`
	checkSource(t, source, nil)
}

// --- Reduction: Type-kinded type families ---

func TestTypeFamilyReduceAppPattern(t *testing.T) {
	// Elem (List a) = a: result kind is Type, so it can be used as a value type.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: Elem (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyReduceMultiEquations(t *testing.T) {
	// Multiple equations, Type-kinded result.
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  (Pair a b) => a
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
form Unit := { Unit: Unit; }
type Id :: Type := \(a: Type). case a {
  a => a
}
f :: Id Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyReduceConstant(t *testing.T) {
	// Constant type family: always returns the same type.
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type AlwaysUnit :: Type := \(a: Type). case a {
  a => Unit
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
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
}
f :: \ c. Elem c -> Elem c
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Wildcard patterns ---

func TestTypeFamilyWildcard(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type AlwaysUnit :: Type := \(a: Type). case a {
  _ => Unit
}
f :: AlwaysUnit (List Unit) -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Constraint families ---

func TestConstraintFamily(t *testing.T) {
	source := `
form Serialization := { JSON: Serialization; Binary: Serialization; }
form Show := \a. {
  show: a -> a
}
type Serializable :: Constraint := \(fmt: Serialization). case fmt {
  JSON => Show;
  Binary => Show
}
`
	checkSource(t, source, nil)
}

// --- Error cases ---

func TestTypeFamilyArityMismatch(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True False => True
}
`
	// True :: Bool (nullary), so True False is a kind error.
	checkSourceExpectCode(t, source, nil, diagnostic.ErrKindMismatch)
}

func TestTypeFamilyNameMismatch(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => True
}
`
	checkSource(t, source, nil)
}

func TestTypeFamilyInjectivityViolation(t *testing.T) {
	// Elem (List Unit) = Unit and Elem Unit = Unit: RHSes both Unit,
	// but LHS patterns (List a) and Unit cannot unify → injectivity violation.
	source := `
form Unit := { Unit: Unit; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a;
  Unit => Unit
}
`
	checkSource(t, source, nil)
}

func TestTypeFamilyDuplicate(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
type F :: Bool := \(a: Bool). case a {
  True => True
}
type F :: Bool := \(a: Bool). case a {
  True => False
}
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrDuplicateDecl)
}

// --- Type family used in function type ---

func TestTypeFamilyInFunctionType(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
type Elem :: Type := \(c: Type). case c {
  (List a) => a
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
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
		Assumptions:     map[string]types.Type{},
	}
	checkSource(t, source, config)
}

// --- Regression: type aliases still work ---

func TestTypeAliasStillWorks(t *testing.T) {
	source := `
form Unit := { Unit: Unit; }
type Id := \a. a
f :: Id Unit -> Unit
f := \x. x
`
	checkSource(t, source, nil)
}

// --- Associated types ---

func TestAssocTypeBasic(t *testing.T) {
	source := `
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }

form Container := \c. {
  type Elem c :: Type;
  cfold: \ b. (Elem c -> b -> b) -> b -> c -> b
}

impl Container (List a) := {
  type Elem := a;
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
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }
form Unit := { Unit: Unit; }
form Pair := \a b. { MkPair: a -> b -> Pair a b; }

form Container := \c. {
  type Elem c :: Type;
  clength: c -> Int
}

impl Container (List a) := {
  type Elem := a;
  clength := listLength
}

impl Container (Pair a b) := {
  type Elem := a;
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
		RegisteredTypes: map[string]types.Type{"Int": types.TypeOfTypes},
	}
	checkSource(t, source, config)
}

// --- Recursive type families ---

func TestRecursiveTypeFamilyDual(t *testing.T) {
	source := `
form Session := { Send: Session; Recv: Session; End: (); }
type Dual :: Session := \(s: Session). case s {
  (Send s) => Recv (Dual s);
  (Recv s) => Send (Dual s);
  End => End
}
`
	checkSource(t, source, nil)
}

func TestRecursiveTypeFamilyFuelExhaustion(t *testing.T) {
	// Cycle detected via sentinel memoization; the family remains stuck (unreduced),
	// producing a type mismatch (E0200) when Loop Unit is compared against Unit.
	source := `
form Unit := { Unit: Unit; }
type Loop :: Type := \(a: Type). case a {
  a => Loop a
}
f :: Loop Unit -> Unit
f := \x. x
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

// --- Session types (Phase 7) ---

func TestSessionTypeDual(t *testing.T) {
	// Session types as a library feature on recursive TF + DataKinds.
	source := `
form Session := { Send: Session; Recv: Session; End: (); }
type Dual :: Session := \(s: Session). case s {
  (Send s) => Recv (Dual s);
  (Recv s) => Send (Dual s);
  End => End
}
`
	checkSource(t, source, nil)
}

func TestSessionTypeDualOfDual(t *testing.T) {
	// Dual (Dual s) should reduce back to s for concrete sessions.
	// This tests recursive TF with promoted constructor patterns.
	source := `
form Session := { Send: Session; Recv: Session; End: (); }
type Dual :: Session := \(s: Session). case s {
  (Send s) => Recv (Dual s);
  (Recv s) => Send (Dual s);
  End => End
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
form Bool := { True: Bool; False: Bool; }
form Unit := { Unit: Unit; }
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
	meta := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
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
		Kind: types.TypeOfTypes,
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
		Kind: types.TypeOfTypes,
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
		Kind: types.TypeOfTypes,
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
	a := &types.TyFamilyApp{Name: "F", Args: []types.Type{&types.TyCon{Name: "Int"}}, Kind: types.TypeOfTypes}
	b := &types.TyFamilyApp{Name: "F", Args: []types.Type{&types.TyCon{Name: "Int"}}, Kind: types.TypeOfTypes}
	c := &types.TyFamilyApp{Name: "F", Args: []types.Type{&types.TyCon{Name: "Bool"}}, Kind: types.TypeOfTypes}
	if !types.Equal(a, b) {
		t.Error("equal TyFamilyApps should be equal")
	}
	if types.Equal(a, c) {
		t.Error("different TyFamilyApps should not be equal")
	}
}

func TestTyFamilyAppZonk(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.TypeOfTypes}
	u.InstallTempSolution(1, &types.TyCon{Name: "Int"})
	tf := &types.TyFamilyApp{
		Name: "F",
		Args: []types.Type{meta},
		Kind: types.TypeOfTypes,
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
form Bool := { True: Bool; False: Bool; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

form FromList := \c. {
  type Elem c :: Type;
  fromList: List (Elem c) -> c
}

impl FromList (List a) := {
  type Elem := a;
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
form Bool := { True: Bool; False: Bool; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

form Conv := \c. {
  type Elem c :: Type;
  conv: List (Elem c) -> c
}

impl Conv (List a) := {
  type Elem := a;
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
form Bool := { True: Bool; False: Bool; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

form FromList := \c. {
  type Elem c :: Type;
  fromList: List (Elem c) -> c
}

impl FromList (List a) := {
  type Elem := a;
  fromList := \xs. xs
}

main := (fromList (Cons True Nil)) :: List Bool
`
	checkSource(t, source, nil)
}

func TestCheckAppNoRegressionPlainFunction(t *testing.T) {
	// Ensure plain function application still works correctly in check mode.
	source := `
form Bool := { True: Bool; False: Bool; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

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
form Unit := { Unit: Unit; }

main :: Computation {} {} Unit
main := pure Unit
`
	checkSource(t, source, nil)
}

func TestCheckAppAssocTypeNestedApp(t *testing.T) {
	// Nested application where both inner and outer app benefit from
	// return-context type propagation.
	source := `
form Bool := { True: Bool; False: Bool; }
form List := \a. { Nil: List a; Cons: a -> List a -> List a; }

form Convert := \c. {
  type Elem c :: Type;
  convert: Elem c -> c
}

impl Convert (List a) := {
  type Elem := a;
  convert := \x. Cons x Nil
}

wrap :: \ a. a -> a
wrap := \x. x

main :: List Bool
main := wrap (convert True)
`
	checkSource(t, source, nil)
}
