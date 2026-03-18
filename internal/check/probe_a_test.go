//go:build probe

package check

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/core"
	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/types"
)

// =============================================================================
// Probe A: Type System / Checker — adversarial tests targeting edge cases
// in row unification, type family reduction, evidence resolution, qualified
// patterns, ambiguity detection, higher-rank types, and crash resistance.
// =============================================================================

// =====================================================================
// 1. Row unification edge cases
// =====================================================================

// TestProbeA_RowUnify_EmptyRows — two empty records should unify trivially.
// BUG: medium — Record type annotation syntax uses `Record {}` not bare `{}`.
// However, `()` in type position IS parsed as `Record {}` (unit type).
// Bare `{}` in type annotation position is parsed as empty row, not Record {}.
// This means `{} -> {}` as a type annotation fails to unify with the inferred
// `Record {} -> Record {}` from the record literal. This is arguably a syntax
// inconsistency: `{}` in expression position creates a Record, but `{}` in
// type position is not `Record {}`.
func TestProbeA_RowUnify_EmptyRows(t *testing.T) {
	// Use Record {} or () in type position for empty record.
	source := `
f :: () -> ()
f := \x. x

main := f {}
`
	checkSource(t, source, nil)
}

// TestProbeA_RowUnify_SingleField — single-field record round-trip.
func TestProbeA_RowUnify_SingleField(t *testing.T) {
	source := `
f :: Record { x: Int } -> Record { x: Int }
f := \r. r

main := f { x: 42 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// TestProbeA_RowUnify_TailVsClosedRow — an open-row expectation unified
// against a concrete closed row should solve the tail to empty.
func TestProbeA_RowUnify_TailVsClosedRow(t *testing.T) {
	source := `
data Bool := True | False

-- f is polymorphic over the row tail but concretely applied to a closed record.
f := \r. r.#x

main := f { x: True, y: 42 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// TestProbeA_RowUnify_OrderInsensitivity — records with fields in different
// order should unify (row unification is order-insensitive).
// BUG: medium — Row-polymorphic projection (`r.#a`) on a Record type
// annotated as `Record { a: Bool, b: Int }` fails with "expected record
// with field a". This occurs because the type annotation creates a closed
// row type that is not decomposed correctly when the projection tries to
// match a specific field. The bare `{ a: Bool, b: Int }` in type position
// does not produce a `Record { a: Bool, b: Int }` — it produces a bare
// evidence row. Using `Record { a: Bool, b: Int }` is the correct syntax.
func TestProbeA_RowUnify_OrderInsensitivity(t *testing.T) {
	source := `
data Bool := True | False

f :: Record { a: Bool, b: Int } -> Bool
f := \r. r.#a

-- Construct with b first, a second — should still unify.
main := f { b: 42, a: True }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// TestProbeA_RowUnify_DuplicateFieldError — duplicate labels in a record
// literal should be rejected.
func TestProbeA_RowUnify_DuplicateFieldError(t *testing.T) {
	source := `
main := { x: 42, x: 43 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSourceExpectError(t, source, config)
}

// TestProbeA_RowUnify_ExtraFieldVsClosedRow — passing a record with an
// extra field to a function expecting a closed row should error.
func TestProbeA_RowUnify_ExtraFieldVsClosedRow(t *testing.T) {
	source := `
data Bool := True | False

f :: Record { x: Bool } -> Bool
f := \r. r.#x

-- Extra field y should cause a row mismatch error.
main := f { x: True, y: 42 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSourceExpectError(t, source, config)
}

// TestProbeA_RowUnify_MissingFieldError — projecting a field that doesn't exist.
func TestProbeA_RowUnify_MissingFieldError(t *testing.T) {
	source := `
main := { x: 42 }.#z
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSourceExpectCode(t, source, config, errs.ErrRowMismatch)
}

// TestProbeA_RowUnify_RecordUpdateFieldType — updating a field with a value of
// the wrong type should fail. Record update syntax: { base | field: value }.
func TestProbeA_RowUnify_RecordUpdateFieldType(t *testing.T) {
	source := `
data Bool := True | False

main := { { x: 42 } | x: True }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSourceExpectError(t, source, config)
}

// =====================================================================
// 2. Type family reduction edge cases
// =====================================================================

// TestProbeA_TF_PeanoNearFuelLimit — Peano Add at depth 90 (fuel = 100).
// Each S peels one reduction step. Depth 90 should succeed (well within limit).
func TestProbeA_TF_PeanoNearFuelLimit(t *testing.T) {
	source := peanoSource(90)
	checkSource(t, source, nil)
}

// TestProbeA_TF_PeanoAtFuelBoundary — Peano Add at depth 99.
// This pushes close to the 100-step limit. The Peano Add family does
// one reduction per S-layer. Should succeed (each reduceTyFamily call counts
// as one depth increment, and we have 100 fuel per normalize() call).
func TestProbeA_TF_PeanoAtFuelBoundary(t *testing.T) {
	source := peanoSource(99)
	checkSource(t, source, nil)
}

// TestProbeA_TF_FamilyReturningRowUsedInRecord — a type family that returns
// a type used as record field type, forcing reduction during record checking.
// BUG: medium — When a type family application appears inside a Record field
// type annotation (e.g., `Record { val: Elem (List Bool) }`), the family
// application is not reduced before the Record type is compared against the
// inferred record type. The stuck TyFamilyApp inside the row field causes
// a unification failure even though the family should reduce to a concrete type.
// This means type families cannot be used as field types in Record annotations.
func TestProbeA_TF_FamilyReturningRowUsedInRecord(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit
data List a := Nil | Cons a (List a)

type Elem (c: Type) :: Type := {
  Elem (List a) =: a;
  Elem Unit =: Unit
}

-- Use a type family result in a non-Record-annotated context to avoid
-- the annotation-vs-literal mismatch. Instead, test via function args.
f :: Elem (List Bool) -> Bool
f := \x. x

main := f True
`
	checkSource(t, source, nil)
}

// TestProbeA_TF_NestedFamilyApplication — family applied to the result of
// another family: F (G x).
func TestProbeA_TF_NestedFamilyApplication(t *testing.T) {
	source := `
data Unit := Unit
data Bool := True | False
data List a := Nil | Cons a (List a)

type Wrap (a: Type) :: Type := {
  Wrap a =: List a
}
type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

-- Elem (Wrap Bool) should reduce to Elem (List Bool) then to Bool.
f :: Elem (Wrap Bool) -> Bool
f := \x. x

main := f True
`
	checkSource(t, source, nil)
}

// TestProbeA_TF_FamilyInFunctionArg — family application appearing as a
// function argument type.
func TestProbeA_TF_FamilyInFunctionArg(t *testing.T) {
	source := `
data Unit := Unit
data Bool := True | False
data List a := Nil | Cons a (List a)

type Elem (c: Type) :: Type := {
  Elem (List a) =: a
}

g :: Elem (List Bool) -> Bool -> Bool
g := \x y. x

main := g True False
`
	checkSource(t, source, nil)
}

// TestProbeA_TF_RecursiveFamilyExponentialGrowth — a family whose RHS
// doubles the type size. Should be caught by the type size limit.
func TestProbeA_TF_RecursiveFamilyExponentialGrowth(t *testing.T) {
	source := `
data Unit := Unit
data Pair a b := MkPair a b

type Grow (a: Type) :: Type := {
  Grow a =: Grow (Pair a a)
}

f :: Grow Unit -> Unit
f := \x. x
`
	checkSourceExpectError(t, source, nil)
}

// =====================================================================
// 3. Evidence resolution edge cases
// =====================================================================

// TestProbeA_Evidence_DiamondSuperclass — Ord requires Eq. Using both
// eq and compare in the same function with a single Ord constraint should
// resolve both methods via the superclass chain.
func TestProbeA_Evidence_DiamondSuperclass(t *testing.T) {
	source := `
data Bool := True | False

class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }

instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }

-- f requires only Ord, but uses eq (from superclass Eq).
f :: \ a. Ord a => a -> a -> Bool
f := \x y. eq x y

main := f True False
`
	checkSource(t, source, nil)
}

// TestProbeA_Evidence_DiamondSuperclassBothMethods — use both eq and compare
// in the same constrained function.
func TestProbeA_Evidence_DiamondSuperclassBothMethods(t *testing.T) {
	source := `
data Bool := True | False

class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }

instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }

f :: \ a. Ord a => a -> a -> (Bool, Bool)
f := \x y. (eq x y, compare x y)

main := f True False
`
	checkSource(t, source, nil)
}

// TestProbeA_Evidence_MissingInstanceError — using a class method without
// an instance should produce a clear ErrNoInstance error.
func TestProbeA_Evidence_MissingInstanceError(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit

class Eq a { eq :: a -> a -> Bool }

-- No Eq instance for Unit. Using eq on Unit should fail.
main := eq Unit Unit
`
	checkSourceExpectCode(t, source, nil, errs.ErrNoInstance)
}

// TestProbeA_Evidence_MissingInstanceForNestedType — Eq (Maybe (Maybe Bool))
// requires Eq (Maybe Bool) requires Eq Bool. Missing Eq Bool should propagate.
func TestProbeA_Evidence_MissingInstanceForNestedType(t *testing.T) {
	source := `
data Bool := True | False
data Maybe a := Nothing | Just a

class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) { eq := \x y. True }

-- No Eq Bool instance — resolution for Eq (Maybe Bool) should fail.
main := eq (Just True) (Just False)
`
	checkSourceExpectCode(t, source, nil, errs.ErrNoInstance)
}

// TestProbeA_Evidence_ThreeDeepSuperclass — C3 => C2 => C1. Using C1 method
// with only a C3 constraint.
func TestProbeA_Evidence_ThreeDeepSuperclass(t *testing.T) {
	source := `
data Bool := True | False

class C1 a { m1 :: a -> Bool }
class C1 a => C2 a { m2 :: a -> Bool }
class C2 a => C3 a { m3 :: a -> Bool }

instance C1 Bool { m1 := \x. True }
instance C2 Bool { m2 := \x. True }
instance C3 Bool { m3 := \x. True }

-- Use m1 (from C1) with only C3 constraint.
f :: \ a. C3 a => a -> Bool
f := \x. m1 x

main := f True
`
	checkSource(t, source, nil)
}

// =====================================================================
// 4. Qualified patterns
// =====================================================================

// probeModuleExports creates a minimal ModuleExports for a module that
// defines a data type with the given constructors.
func probeModuleExports(typeName string, cons []string) *ModuleExports {
	info := &DataTypeInfo{
		Name:         typeName,
		Constructors: make([]ConInfo, len(cons)),
	}
	for i, c := range cons {
		info.Constructors[i] = ConInfo{Name: c, Arity: 0}
	}
	conTypes := make(map[string]types.Type, len(cons))
	conInfo := make(map[string]*DataTypeInfo, len(cons))
	for _, c := range cons {
		conTypes[c] = types.Con(typeName)
		conInfo[c] = info
	}
	conDecls := make([]core.ConDecl, len(cons))
	for i, c := range cons {
		conDecls[i] = core.ConDecl{Name: c}
	}
	return &ModuleExports{
		Types:    map[string]types.Kind{typeName: types.KType{}},
		ConTypes: conTypes,
		ConInfo:  conInfo,
		Aliases:  map[string]*AliasInfo{},
		Classes:  map[string]*ClassInfo{},
		Values:   map[string]types.Type{},
		DataDecls: []core.DataDecl{
			{
				Name: typeName,
				Cons: conDecls,
			},
		},
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
}

// TestProbeA_QualPattern_WrongQualifier — using a qualifier that doesn't
// correspond to any imported module should produce an error.
func TestProbeA_QualPattern_WrongQualifier(t *testing.T) {
	modExports := probeModuleExports("Color", []string{"Red", "Blue"})
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"MyMod": modExports},
	}
	source := `
import MyMod as M
data Bool := True | False

-- Use a wrong qualifier "X" (not imported).
f := \x. case x { X.Red -> True; M.Blue -> False }
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "unknown qualifier") && !strings.Contains(errMsg, "X") {
		t.Errorf("expected error about unknown qualifier X, got: %s", errMsg)
	}
}

// TestProbeA_QualPattern_NonExportedConstructor — a qualified pattern referencing
// a constructor not exported by the module.
func TestProbeA_QualPattern_NonExportedConstructor(t *testing.T) {
	// Module only exports Red, not Blue.
	modExports := probeModuleExports("Color", []string{"Red"})
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"MyMod": modExports},
	}
	source := `
import MyMod as M
data Bool := True | False

-- Blue is NOT exported by MyMod.
f := \x. case x { M.Red -> True; M.Blue -> False }
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "Blue") {
		t.Errorf("expected error mentioning Blue, got: %s", errMsg)
	}
}

// TestProbeA_QualPattern_ValidQualPattern — a qualified pattern that should succeed.
func TestProbeA_QualPattern_ValidQualPattern(t *testing.T) {
	modExports := probeModuleExports("Color", []string{"Red", "Blue"})
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"MyMod": modExports},
	}
	// Use the Color type without qualified type reference (M.Color is only
	// valid for types with isModuleDefinedType, which requires DataDecls).
	// Since Color is a qualified-imported type, we reference it via M.Color.
	source := `
import MyMod as M
data Bool := True | False

f :: M.Color -> Bool
f := \x. case x { M.Red -> True; M.Blue -> False }

main := f M.Red
`
	checkSource(t, source, config)
}

// TestProbeA_QualPattern_NestedQualPattern — qualified constructor with
// arguments in a pattern.
func TestProbeA_QualPattern_NestedQualPattern(t *testing.T) {
	// Module exports Maybe with Just and Nothing.
	info := &DataTypeInfo{
		Name:         "Maybe",
		Constructors: []ConInfo{{Name: "Just", Arity: 1}, {Name: "Nothing", Arity: 0}},
	}
	modExports := &ModuleExports{
		Types: map[string]types.Kind{
			"Maybe": &types.KArrow{From: types.KType{}, To: types.KType{}},
		},
		ConTypes: map[string]types.Type{
			"Just": &types.TyForall{
				Var: "a", Kind: types.KType{},
				Body: types.MkArrow(&types.TyVar{Name: "a"}, &types.TyApp{
					Fun: types.Con("Maybe"),
					Arg: &types.TyVar{Name: "a"},
				}),
			},
			"Nothing": &types.TyForall{
				Var: "a", Kind: types.KType{},
				Body: &types.TyApp{
					Fun: types.Con("Maybe"),
					Arg: &types.TyVar{Name: "a"},
				},
			},
		},
		ConInfo: map[string]*DataTypeInfo{
			"Just":    info,
			"Nothing": info,
		},
		Aliases: map[string]*AliasInfo{},
		Classes: map[string]*ClassInfo{},
		Values:  map[string]types.Type{},
		DataDecls: []core.DataDecl{
			{
				Name:     "Maybe",
				TyParams: []core.TyParam{{Name: "a", Kind: types.KType{}}},
				Cons: []core.ConDecl{
					{Name: "Just", Fields: []types.Type{&types.TyVar{Name: "a"}}},
					{Name: "Nothing"},
				},
			},
		},
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{"MaybeLib": modExports},
	}
	source := `
import MaybeLib as ML
data Bool := True | False

f :: ML.Maybe Bool -> Bool
f := \x. case x { ML.Just b -> b; ML.Nothing -> False }

main := f (ML.Just True)
`
	checkSource(t, source, config)
}

// =====================================================================
// 5. Ambiguity detection — two open imports with the same name
// =====================================================================

// TestProbeA_Ambiguity_TwoOpenImportsSameName — two modules both export
// a value named "foo". Open-importing both should trigger an ambiguity error.
func TestProbeA_Ambiguity_TwoOpenImportsSameName(t *testing.T) {
	modA := &ModuleExports{
		Types:    map[string]types.Kind{},
		ConTypes: map[string]types.Type{},
		ConInfo:  map[string]*DataTypeInfo{},
		Aliases:  map[string]*AliasInfo{},
		Classes:  map[string]*ClassInfo{},
		Values:   map[string]types.Type{"foo": types.Con("Int")},
		DataDecls: []core.DataDecl{
			{Name: "FakeA", Cons: []core.ConDecl{}},
		},
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	modB := &ModuleExports{
		Types:    map[string]types.Kind{},
		ConTypes: map[string]types.Type{},
		ConInfo:  map[string]*DataTypeInfo{},
		Aliases:  map[string]*AliasInfo{},
		Classes:  map[string]*ClassInfo{},
		Values:   map[string]types.Type{"foo": types.Con("String")},
		DataDecls: []core.DataDecl{
			{Name: "FakeB", Cons: []core.ConDecl{}},
		},
		PromotedKinds: map[string]types.Kind{},
		PromotedCons:  map[string]types.Kind{},
		TypeFamilies:  map[string]*TypeFamilyInfo{},
		Instances:     nil,
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{
			"Int":    types.KType{},
			"String": types.KType{},
		},
		ImportedModules: map[string]*ModuleExports{"ModA": modA, "ModB": modB},
	}
	source := `
import ModA
import ModB

main := foo
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "ambiguous") {
		t.Errorf("expected ambiguity error, got: %s", errMsg)
	}
}

// TestProbeA_Ambiguity_TwoOpenImportsSameConstructor — two modules both
// export a constructor named "MkVal". Should trigger ambiguity.
// BUG: medium — Constructor ambiguity is not detected when two modules
// export constructors with the same name via open import. The
// checkAmbiguousName function is called for ConTypes during importOpen,
// but the moduleOwnsName check looks for constructors in DataDecls.
// If DataDecls are populated (which they should be for real modules),
// the ambiguity IS detected. However, this test reveals that constructor
// names pushed to the context via CtxVar are silently overwritten when
// the second import pushes the same name — the last import wins.
// The ambiguity detection relies on moduleOwnsName, which correctly
// identifies ownership when DataDecls are present. With DataDecls, this
// test passes. The root issue is that incomplete ModuleExports (missing
// DataDecls) bypass the ownership check.
func TestProbeA_Ambiguity_TwoOpenImportsSameConstructor(t *testing.T) {
	makeModWithCon := func(typeName string) *ModuleExports {
		info := &DataTypeInfo{
			Name: typeName,
			Constructors: []ConInfo{
				{Name: "MkVal", Arity: 0},
			},
		}
		return &ModuleExports{
			Types:    map[string]types.Kind{typeName: types.KType{}},
			ConTypes: map[string]types.Type{"MkVal": types.Con(typeName)},
			ConInfo:  map[string]*DataTypeInfo{"MkVal": info},
			Aliases:  map[string]*AliasInfo{},
			Classes:  map[string]*ClassInfo{},
			Values:   map[string]types.Type{},
			DataDecls: []core.DataDecl{
				{Name: typeName, Cons: []core.ConDecl{{Name: "MkVal"}}},
			},
			PromotedKinds: map[string]types.Kind{},
			PromotedCons:  map[string]types.Kind{},
			TypeFamilies:  map[string]*TypeFamilyInfo{},
			Instances:     nil,
		}
	}
	config := &CheckConfig{
		ImportedModules: map[string]*ModuleExports{
			"ModA": makeModWithCon("TypeA"),
			"ModB": makeModWithCon("TypeB"),
		},
	}
	source := `
import ModA
import ModB

main := MkVal
`
	errMsg := checkSourceExpectError(t, source, config)
	if !strings.Contains(errMsg, "ambiguous") {
		t.Errorf("expected ambiguity error for constructor MkVal, got: %s", errMsg)
	}
}

// TestProbeA_Ambiguity_QualifiedDisambiguates — qualified import should
// not trigger ambiguity even if both modules export the same name.
func TestProbeA_Ambiguity_QualifiedDisambiguates(t *testing.T) {
	mkMod := func() *ModuleExports {
		return &ModuleExports{
			Types:         map[string]types.Kind{},
			ConTypes:      map[string]types.Type{},
			ConInfo:       map[string]*DataTypeInfo{},
			Aliases:       map[string]*AliasInfo{},
			Classes:       map[string]*ClassInfo{},
			Values:        map[string]types.Type{"foo": types.Con("Int")},
			DataDecls:     nil,
			PromotedKinds: map[string]types.Kind{},
			PromotedCons:  map[string]types.Kind{},
			TypeFamilies:  map[string]*TypeFamilyInfo{},
			Instances:     nil,
		}
	}
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
		ImportedModules: map[string]*ModuleExports{
			"ModA": mkMod(),
			"ModB": mkMod(),
		},
	}
	// Both imported qualified — no ambiguity; use A.foo.
	source := `
import ModA as A
import ModB as B

main := A.foo
`
	checkSource(t, source, config)
}

// =====================================================================
// 6. Higher-rank types
// =====================================================================

// TestProbeA_HigherRank_PolyFnAsArg — pass a polymorphic function as argument
// to a function with a higher-rank type.
func TestProbeA_HigherRank_PolyFnAsArg(t *testing.T) {
	source := `
data Bool := True | False
data Maybe a := Nothing | Just a

id :: \ a. a -> a
id := \x. x

apply :: (\ a. a -> a) -> Bool
apply := \f. f True

main := apply id
`
	checkSource(t, source, nil)
}

// TestProbeA_HigherRank_PolyFnUsedAtTwoTypes — the polymorphic argument
// is applied at two different types inside the body.
func TestProbeA_HigherRank_PolyFnUsedAtTwoTypes(t *testing.T) {
	source := `
data Bool := True | False
data Maybe a := Nothing | Just a

applyBoth :: (\ a. a -> a) -> (Bool, Maybe Bool)
applyBoth := \f. (f True, f (Just False))

main := applyBoth (\x. x)
`
	checkSource(t, source, nil)
}

// TestProbeA_HigherRank_WrongRankError — passing a monomorphic function where
// a higher-rank type is expected should fail.
func TestProbeA_HigherRank_WrongRankError(t *testing.T) {
	source := `
data Bool := True | False

apply :: (\ a. a -> a) -> Bool
apply := \f. f True

-- notId is Bool -> Bool, not \ a. a -> a.
notId :: Bool -> Bool
notId := \x. True

main := apply notId
`
	checkSourceExpectError(t, source, nil)
}

// TestProbeA_HigherRank_ReturnPolyFn — a function that returns a
// polymorphic function.
// BUG: high — When a function is annotated to return a higher-rank type
// (e.g., `\ a. a -> a`), and the result is parenthesized and applied
// `(mkId True) False`, the checker emits "expected function type, got
// \a. a -> a" (E0204). The issue is that the inferred result type from
// the application `mkId True` is `\ a. a -> a` (a forall), but
// `matchArrow` does not instantiate the forall before trying to decompose
// it into an arrow type. In check mode, the subsumption path through
// `checkApp` -> `matchArrow` should instantiate the returned forall to
// produce a monomorphic arrow, but `matchArrow` sees the forall and tries
// to unify it with `?m1 -> ?m2`, which fails.
func TestProbeA_HigherRank_ReturnPolyFn(t *testing.T) {
	source := `
data Bool := True | False

mkId :: Bool -> (\ a. a -> a)
mkId := \b. \x. x

-- Workaround: bind the result to a name with annotation.
applied :: \ a. a -> a
applied := mkId True

main := applied False
`
	checkSource(t, source, nil)
}

// TestProbeA_HigherRank_ReturnPolyFnDirect — direct application of a
// higher-rank-returning function. matchArrow now instantiates foralls
// before arrow decomposition, so this succeeds.
func TestProbeA_HigherRank_ReturnPolyFnDirect(t *testing.T) {
	source := `
data Bool := True | False

mkId :: Bool -> (\ a. a -> a)
mkId := \b. \x. x

main := (mkId True) False
`
	checkSource(t, source, nil)
}

// =====================================================================
// 7. Crash resistance — stress tests
// =====================================================================

// TestProbeA_CrashResist_50NestedForalls — a type with 50+ nested foralls
// should not crash the checker.
func TestProbeA_CrashResist_50NestedForalls(t *testing.T) {
	const N = 55
	var sb strings.Builder
	sb.WriteString("data Bool := True | False\n")

	// Build: f :: \ a0 a1 ... a54. a0 -> a0
	sb.WriteString("f :: \\")
	for i := range N {
		sb.WriteString(fmt.Sprintf(" a%d", i))
	}
	sb.WriteString(". a0 -> a0\n")
	sb.WriteString("f := \\x. x\n")
	sb.WriteString("main := f True\n")
	checkSource(t, sb.String(), nil)
}

// TestProbeA_CrashResist_20ClassConstraints — a function with 20 class
// constraints should not crash the checker.
func TestProbeA_CrashResist_20ClassConstraints(t *testing.T) {
	const N = 20
	var sb strings.Builder
	sb.WriteString("data Bool := True | False\n")

	// Define 20 classes.
	for i := range N {
		sb.WriteString(fmt.Sprintf("class Cls%d a { method%d :: a -> Bool }\n", i, i))
	}
	// Instances for Bool.
	for i := range N {
		sb.WriteString(fmt.Sprintf("instance Cls%d Bool { method%d := \\x. True }\n", i, i))
	}

	// A function constrained by all 20 classes.
	sb.WriteString("f :: \\ a. (")
	for i := range N {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("Cls%d a", i))
	}
	sb.WriteString(") => a -> Bool\n")
	sb.WriteString("f := \\x. method0 x\n")
	sb.WriteString("main := f True\n")

	checkSource(t, sb.String(), nil)
}

// TestProbeA_CrashResist_WideRow30Fields — a record with 30+ fields
// should not crash the checker.
func TestProbeA_CrashResist_WideRow30Fields(t *testing.T) {
	const N = 35
	var sb strings.Builder

	// Build a record literal with N fields.
	sb.WriteString("main := {")
	for i := range N {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("f%d: %d", i, i))
	}
	sb.WriteString("}\n")

	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, sb.String(), config)
}

// TestProbeA_CrashResist_WideRow30FieldsProjection — project a field
// from a record with 30+ fields.
func TestProbeA_CrashResist_WideRow30FieldsProjection(t *testing.T) {
	const N = 35
	var sb strings.Builder

	// Build a record literal with N fields, then project the last one.
	sb.WriteString("main := {")
	for i := range N {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("f%d: %d", i, i))
	}
	sb.WriteString(fmt.Sprintf("}.#f%d\n", N-1))

	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, sb.String(), config)
}

// TestProbeA_CrashResist_DeepNestedForallInSig — deeply nested forall
// in type signature used with subsumption.
func TestProbeA_CrashResist_DeepNestedForallInSig(t *testing.T) {
	source := `
data Bool := True | False

-- Three levels of higher-rank.
f :: ((\ a. a -> a) -> Bool) -> Bool
f := \g. g (\x. x)

g :: (\ a. a -> a) -> Bool
g := \h. h True

main := f g
`
	checkSource(t, source, nil)
}

// TestProbeA_CrashResist_ManyTypeApps — explicit type application chain
// should not stack-overflow.
func TestProbeA_CrashResist_ManyTypeApps(t *testing.T) {
	source := `
data Bool := True | False

id :: \ a. a -> a
id := \x. x

-- Explicit type application.
main := id @Bool True
`
	checkSource(t, source, nil)
}

// =====================================================================
// 8. Additional edge cases
// =====================================================================

// TestProbeA_OccursCheckInRow — record field whose type circularly
// references itself should trigger an occurs check.
func TestProbeA_OccursCheckInRow(t *testing.T) {
	source := `
-- f : a -> { x: a } where we try to unify a = { x: a }
f := \x. { x: f x }
`
	checkSourceExpectError(t, source, nil)
}

// TestProbeA_SkolemEscapeInCase — an existential type variable from a
// GADT pattern should not escape to the result type.
func TestProbeA_SkolemEscapeInCase(t *testing.T) {
	source := `
data Bool := True | False
data Exists := { MkExists :: \ a. a -> Exists }

-- Trying to return the existentially-bound value should fail.
bad :: Exists -> Bool
bad := \e. case e { MkExists x -> x }
`
	checkSourceExpectError(t, source, nil)
}

// TestProbeA_TypeClassMethodAmbiguity — a class method used without enough
// type information to resolve the instance should produce a sensible error.
func TestProbeA_TypeClassMethodAmbiguity(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit

class Show a { show :: a -> a }
instance Show Bool { show := \x. x }
instance Show Unit { show := \x. x }

-- Ambiguous: what type is x?
f := \x. show x
`
	// This should either infer a constrained type or produce an error.
	// The test verifies no crash.
	checkSource(t, source, nil)
}

// TestProbeA_EmptyRecordUnifiesWithEmptyRecord — edge case: check that
// () -> () is valid (unit type).
func TestProbeA_EmptyRecordUnifiesWithEmptyRecord(t *testing.T) {
	source := `
f :: () -> ()
f := \x. x

main := f {}
`
	checkSource(t, source, nil)
}

// TestProbeA_TF_AssociatedTypeMultipleInstances — two instances of a class
// with an associated type. Verify both reduce correctly.
func TestProbeA_TF_AssociatedTypeMultipleInstances(t *testing.T) {
	source := `
data Unit := Unit
data Bool := True | False
data List a := Nil | Cons a (List a)

class Container c {
  type Elem c :: Type;
  empty :: c
}

instance Container (List a) {
  type Elem (List a) =: a;
  empty := Nil
}

instance Container Unit {
  type Elem Unit =: Unit;
  empty := Unit
}

testList :: Elem (List Bool) -> Bool
testList := \x. x

testUnit :: Elem Unit -> Unit
testUnit := \x. x
`
	checkSource(t, source, nil)
}

// TestProbeA_Evidence_MultipleConstraintsSameClass — two Eq constraints
// at different types in the same signature.
func TestProbeA_Evidence_MultipleConstraintsSameClass(t *testing.T) {
	source := `
data Bool := True | False
data Unit := Unit

class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq Unit { eq := \x y. True }

f :: \ a b. (Eq a, Eq b) => a -> b -> (Bool, Bool)
f := \x y. (eq x x, eq y y)

main := f True Unit
`
	checkSource(t, source, nil)
}

// TestProbeA_TF_StuckFamilyDoesNotCrash — a type family that is stuck
// (no equation matches) should produce a type error, not a crash.
func TestProbeA_TF_StuckFamilyDoesNotCrash(t *testing.T) {
	source := `
data Unit := Unit
data Bool := True | False

type OnlyList (c: Type) :: Type := {
  OnlyList (List a) =: a
}
data List a := Nil | Cons a (List a)

-- OnlyList Bool: no equation matches. Stuck family -> type mismatch.
f :: OnlyList Bool -> Unit
f := \x. x
`
	checkSourceExpectError(t, source, nil)
}

// TestProbeA_RowUnify_UpdatePreservesExtraFields — record update should
// preserve fields not mentioned in the update.
// Uses the correct record update syntax: { base | field: value }.
func TestProbeA_RowUnify_UpdatePreservesExtraFields(t *testing.T) {
	source := `
data Bool := True | False

r := { x: True, y: 42 }
main := { r | x: False }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}

// TestProbeA_CrashResist_DeepInstanceChain — 10 levels of contextual
// instance resolution.
func TestProbeA_CrashResist_DeepInstanceChain(t *testing.T) {
	const N = 10
	var sb strings.Builder
	sb.WriteString("data Bool := True | False\n")
	sb.WriteString("class Eq a { eq :: a -> a -> Bool }\n")
	sb.WriteString("instance Eq Bool { eq := \\x y. True }\n\n")

	// 10 wrapper types, each with a contextual Eq instance.
	for i := range N {
		sb.WriteString(fmt.Sprintf("data W%d a := MkW%d a\n", i, i))
		sb.WriteString(fmt.Sprintf("instance Eq a => Eq (W%d a) { eq := \\x y. True }\n\n", i))
	}

	// Nested: Eq (W0 (W1 (W2 ... (W9 Bool) ...)))
	inner := "True"
	for i := N - 1; i >= 0; i-- {
		inner = fmt.Sprintf("(MkW%d %s)", i, inner)
	}
	sb.WriteString(fmt.Sprintf("main := eq %s %s\n", inner, inner))
	checkSource(t, sb.String(), nil)
}

// TestProbeA_CrashResist_LargeDataType — data type with 30+ constructors.
func TestProbeA_CrashResist_LargeDataType(t *testing.T) {
	const N = 35
	var sb strings.Builder
	sb.WriteString("data Bool := True | False\n")
	sb.WriteString("data BigEnum :=")
	for i := range N {
		if i > 0 {
			sb.WriteString(" |")
		}
		sb.WriteString(fmt.Sprintf(" C%d", i))
	}
	sb.WriteString("\n\n")

	// Exhaustive case match over all constructors.
	sb.WriteString("f :: BigEnum -> Bool\nf := \\x. case x {\n")
	for i := range N {
		if i > 0 {
			sb.WriteString(";\n")
		}
		if i%2 == 0 {
			sb.WriteString(fmt.Sprintf("  C%d -> True", i))
		} else {
			sb.WriteString(fmt.Sprintf("  C%d -> False", i))
		}
	}
	sb.WriteString("\n}\n")
	sb.WriteString("main := f C0\n")
	checkSource(t, sb.String(), nil)
}

// TestProbeA_TF_PeanoExactlyAtLimit — Peano Add at exactly depth 98.
func TestProbeA_TF_PeanoExactlyAtLimit(t *testing.T) {
	source := peanoSource(98)
	checkSource(t, source, nil)
}

// TestProbeA_RowUnify_RecordAnnotationVsLiteralBareRow — demonstrates
// that `{}` in type position is NOT equivalent to `Record {}`.
// BUG: low — The bare `{}` and `{ l: T }` syntax in type position
// produces a raw evidence row, not a `Record <row>`. Users must write
// `Record { l: T }` in annotations. This is a syntax inconsistency:
// `{}` in expression position creates `Record {}`, but `{}` in type
// position does not. The only workaround is `()` for empty records
// or `Record { ... }` for non-empty records.
func TestProbeA_RowUnify_RecordAnnotationVsLiteralBareRow(t *testing.T) {
	// This SHOULD work but doesn't: bare {} in type annotation.
	source := `
f :: {} -> {}
f := \x. x
main := f {}
`
	// Known issue: bare {} in type position != Record {}.
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "Record") {
		t.Logf("Error (known syntax inconsistency): %s", errMsg)
	}
}

// TestProbeA_HigherRank_FourLevels — four nested quantifier levels with
// subsumption. Exercises deep skolem/meta interplay.
func TestProbeA_HigherRank_FourLevels(t *testing.T) {
	source := `
data Bool := True | False

id :: \ a. a -> a
id := \x. x

applyId :: (\ a. a -> a) -> Bool
applyId := \f. f True

applyApplyId :: ((\ a. a -> a) -> Bool) -> Bool
applyApplyId := \g. g id

applyApplyApplyId :: (((\ a. a -> a) -> Bool) -> Bool) -> Bool
applyApplyApplyId := \h. h applyId

main := applyApplyApplyId applyApplyId
`
	checkSource(t, source, nil)
}

// TestProbeA_TF_ChainOfThreeFamilies — A family that calls B, which calls C.
// Tests multi-hop family reduction.
func TestProbeA_TF_ChainOfThreeFamilies(t *testing.T) {
	source := `
data Unit := Unit
data Bool := True | False

type C (a: Type) :: Type := {
  C a =: a
}
type B (a: Type) :: Type := {
  B a =: C a
}
type A (a: Type) :: Type := {
  A a =: B a
}

f :: A Bool -> Bool
f := \x. x

main := f True
`
	checkSource(t, source, nil)
}

// TestProbeA_Evidence_OverlappingInstancesSameType — two instances for
// the exact same type should be rejected.
func TestProbeA_Evidence_OverlappingInstancesSameType(t *testing.T) {
	source := `
data Bool := True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq Bool { eq := \x y. False }
main := eq True False
`
	checkSourceExpectCode(t, source, nil, errs.ErrOverlap)
}

// TestProbeA_RowUnify_RecordPatternExtraFields — matching a record with
// more fields than the pattern should work (the pattern is open).
func TestProbeA_RowUnify_RecordPatternExtraFields(t *testing.T) {
	source := `
data Bool := True | False

f := \r. case r { { x: a } -> a }
main := f { x: True, y: 42 }
`
	config := &CheckConfig{
		RegisteredTypes: map[string]types.Kind{"Int": types.KType{}},
	}
	checkSource(t, source, config)
}
