package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// --- Parser: type family declarations ---

func TestTypeFamilyParseBasic(t *testing.T) {
	source := `
data Bool = True | False
type IsTrue (b : Bool) :: Bool = {
  IsTrue True = True;
  IsTrue False = False
}
`
	checkSource(t, source, nil)
}

func TestTypeFamilyParseTwoParams(t *testing.T) {
	source := `
data Mult = Unrestricted | Affine | Linear
type LUB (m1 : Mult) (m2 : Mult) :: Mult = {
  LUB Linear _ = Linear;
  LUB _ Linear = Linear;
  LUB Affine _ = Affine;
  LUB _ Affine = Affine;
  LUB Unrestricted Unrestricted = Unrestricted
}
`
	checkSource(t, source, nil)
}

// --- Reduction: Type-kinded type families ---

func TestTypeFamilyReduceAppPattern(t *testing.T) {
	// Elem (List a) = a: result kind is Type, so it can be used as a value type.
	source := `
data List a = Nil | Cons a (List a)
data Unit = Unit
type Elem (c : Type) :: Type = {
  Elem (List a) = a
}
f :: Elem (List Unit) -> Unit
f := \x -> x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyReduceMultiEquations(t *testing.T) {
	// Multiple equations, Type-kinded result.
	source := `
data List a = Nil | Cons a (List a)
data Unit = Unit
data Pair a b = MkPair a b
type Elem (c : Type) :: Type = {
  Elem (List a) = a;
  Elem (Pair a b) = a
}
f :: Elem (List Unit) -> Unit
f := \x -> x
g :: Elem (Pair Unit Unit) -> Unit
g := \x -> x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyReduceIdentity(t *testing.T) {
	// Simple identity-like type family.
	source := `
data Unit = Unit
type Id (a : Type) :: Type = {
  Id a = a
}
f :: Id Unit -> Unit
f := \x -> x
`
	checkSource(t, source, nil)
}

func TestTypeFamilyReduceConstant(t *testing.T) {
	// Constant type family: always returns the same type.
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type AlwaysUnit (a : Type) :: Type = {
  AlwaysUnit a = Unit
}
f :: AlwaysUnit (List Unit) -> Unit
f := \x -> x
`
	checkSource(t, source, nil)
}

// --- Stuck reduction ---

func TestTypeFamilyStuckOnMeta(t *testing.T) {
	// A Type-kinded family with a skolem argument: reduction is stuck.
	// The stuck TyFamilyApp should unify with itself.
	source := `
data List a = Nil | Cons a (List a)
type Elem (c : Type) :: Type = {
  Elem (List a) = a
}
f :: forall c. Elem c -> Elem c
f := \x -> x
`
	checkSource(t, source, nil)
}

// --- Wildcard patterns ---

func TestTypeFamilyWildcard(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type AlwaysUnit (a : Type) :: Type = {
  AlwaysUnit _ = Unit
}
f :: AlwaysUnit (List Unit) -> Unit
f := \x -> x
`
	checkSource(t, source, nil)
}

// --- Constraint families ---

func TestConstraintFamily(t *testing.T) {
	source := `
data Serialization = JSON | Binary
class Show a {
  show :: a -> a
}
type Serializable (fmt : Serialization) :: Constraint = {
  Serializable JSON = Show;
  Serializable Binary = Show
}
`
	checkSource(t, source, nil)
}

// --- Error cases ---

func TestTypeFamilyArityMismatch(t *testing.T) {
	source := `
data Bool = True | False
type F (a : Bool) :: Bool = {
  F True False = True
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyEquation)
}

func TestTypeFamilyNameMismatch(t *testing.T) {
	source := `
data Bool = True | False
type F (a : Bool) :: Bool = {
  G True = True
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrTypeFamilyEquation)
}

func TestTypeFamilyInjectivityViolation(t *testing.T) {
	// Elem (List Unit) = Unit and Elem Unit = Unit: RHSes both Unit,
	// but LHS patterns (List a) and Unit cannot unify → injectivity violation.
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
type Elem (c : Type) :: (r :: Type) | r -> c = {
  Elem (List a) = a;
  Elem Unit = Unit
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrInjectivity)
}

func TestTypeFamilyDuplicate(t *testing.T) {
	source := `
data Bool = True | False
type F (a : Bool) :: Bool = {
  F True = True
}
type F (a : Bool) :: Bool = {
  F True = False
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrDuplicateDecl)
}

// --- Type family used in function type ---

func TestTypeFamilyInFunctionType(t *testing.T) {
	source := `
data List a = Nil | Cons a (List a)
data Unit = Unit
type Elem (c : Type) :: Type = {
  Elem (List a) = a
}
map :: forall a b. (a -> b) -> List a -> List b
map := assumption
length :: forall a. List a -> Int
length := assumption
first :: forall a. List a -> Elem (List a)
first := assumption
main :: Int
main := length (map (\x -> x) (Cons Unit Nil))
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
data Unit = Unit
type Id a = a
f :: Id Unit -> Unit
f := \x -> x
`
	checkSource(t, source, nil)
}

// --- Associated types ---

func TestAssocTypeBasic(t *testing.T) {
	source := `
data List a = Nil | Cons a (List a)
data Unit = Unit

class Container c {
  type Elem c :: Type;
  cfold :: forall b. (Elem c -> b -> b) -> b -> c -> b
}

instance Container (List a) {
  type Elem (List a) = a;
  cfold := foldr
}

foldr :: forall a b. (a -> b -> b) -> b -> List a -> b
foldr := assumption

f :: Elem (List Unit) -> Unit
f := \x -> x
`
	checkSource(t, source, nil)
}

func TestAssocTypeMultipleInstances(t *testing.T) {
	source := `
data List a = Nil | Cons a (List a)
data Unit = Unit
data Pair a b = MkPair a b

class Container c {
  type Elem c :: Type;
  clength :: c -> Int
}

instance Container (List a) {
  type Elem (List a) = a;
  clength := listLength
}

instance Container (Pair a b) {
  type Elem (Pair a b) = a;
  clength := pairLength
}

listLength :: forall a. List a -> Int
listLength := assumption

pairLength :: forall a b. Pair a b -> Int
pairLength := assumption

f :: Elem (List Unit) -> Unit
f := \x -> x
g :: Elem (Pair Unit Unit) -> Unit
g := \x -> x
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// --- Functional dependencies ---

func TestFunDepParse(t *testing.T) {
	source := `
data Unit = Unit
data List a = Nil | Cons a (List a)
class Elem c e | c -> e {
  cfold :: (e -> e) -> c -> c
}
instance Elem (List a) a {
  cfold := \f -> \xs -> xs
}
`
	checkSource(t, source, nil)
}

func TestFunDepUnknownParam(t *testing.T) {
	source := `
class Bad a b | z -> b {
  m :: a -> b
}
`
	checkSourceExpectCode(t, source, nil, errs.ErrBadClass)
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
	u := NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	u.soln[1] = &types.TyCon{Name: "Int"}
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
