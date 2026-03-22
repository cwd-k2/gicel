//go:build probe

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/check/unify"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// Evidence probe tests — superclass chains, missing instances, overlapping
// instances, diamond inheritance, evidence row unification, constraint row
// handling, multiple constraints at different types.
// =============================================================================

// =====================================================================
// From probe_a: Evidence resolution edge cases
// =====================================================================

// TestProbeA_Evidence_DiamondSuperclass — Ord requires Eq. Using both
// eq and compare in the same function with a single Ord constraint should
// resolve both methods via the superclass chain.
func TestProbeA_Evidence_DiamondSuperclass(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }

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
data Bool := { True: (); False: (); }

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
data Bool := { True: (); False: (); }
data Unit := { Unit: (); }

class Eq a { eq :: a -> a -> Bool }

-- No Eq instance for Unit. Using eq on Unit should fail.
main := eq Unit Unit
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNoInstance)
}

// TestProbeA_Evidence_MissingInstanceForNestedType — Eq (Maybe (Maybe Bool))
// requires Eq (Maybe Bool) requires Eq Bool. Missing Eq Bool should propagate.
func TestProbeA_Evidence_MissingInstanceForNestedType(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
data Maybe := \a. { Nothing: (); Just: a; }

class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) { eq := \x y. True }

-- No Eq Bool instance — resolution for Eq (Maybe Bool) should fail.
main := eq (Just True) (Just False)
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNoInstance)
}

// TestProbeA_Evidence_ThreeDeepSuperclass — C3 => C2 => C1. Using C1 method
// with only a C3 constraint.
func TestProbeA_Evidence_ThreeDeepSuperclass(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }

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

// TestProbeA_Evidence_MultipleConstraintsSameClass — two Eq constraints
// at different types in the same signature.
func TestProbeA_Evidence_MultipleConstraintsSameClass(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
data Unit := { Unit: (); }

class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq Unit { eq := \x y. True }

f :: \ a b. (Eq a, Eq b) => a -> b -> (Bool, Bool)
f := \x y. (eq x x, eq y y)

main := f True Unit
`
	checkSource(t, source, nil)
}

// TestProbeA_Evidence_OverlappingInstancesSameType — two instances for
// the exact same type should be rejected.
func TestProbeA_Evidence_OverlappingInstancesSameType(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
instance Eq Bool { eq := \x y. False }
main := eq True False
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

// =====================================================================
// From probe_d: Evidence row handling
// =====================================================================

// TestProbeD_Evidence_EmptyConstraintRowUnify — two empty constraint rows
// should unify.
func TestProbeD_Evidence_EmptyConstraintRowUnify(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.EmptyConstraintRow()
	r2 := types.EmptyConstraintRow()
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("empty constraint rows should unify: %v", err)
	}
}

// TestProbeD_Evidence_SingleConstraintRowUnify — two single-entry constraint
// rows with the same class and args should unify.
func TestProbeD_Evidence_SingleConstraintRowUnify(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.SingleConstraint("Eq", []types.Type{types.Con("Bool")})
	r2 := types.SingleConstraint("Eq", []types.Type{types.Con("Bool")})
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("matching single constraint rows should unify: %v", err)
	}
}

// TestProbeD_Evidence_ConstraintRowMismatch — two constraint rows with
// different class names should fail.
func TestProbeD_Evidence_ConstraintRowMismatch(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.SingleConstraint("Eq", []types.Type{types.Con("Bool")})
	r2 := types.SingleConstraint("Ord", []types.Type{types.Con("Bool")})
	err := u.Unify(r1, r2)
	if err == nil {
		t.Fatal("different constraint classes should not unify")
	}
}

// TestProbeD_Evidence_NestedEvidenceType — a TyEvidence containing a
// TyEvidence body should be handled without crash.
func TestProbeD_Evidence_NestedEvidenceType(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }

class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }

instance Eq Bool { eq := \x y. True }
instance Ord Bool { compare := \x y. True }

-- Function with nested constraint usage.
f :: \ a. (Eq a, Ord a) => a -> a -> (Bool, Bool)
f := \x y. (eq x y, compare x y)

main := f True False
`
	checkSource(t, source, nil)
}

// TestProbeD_Evidence_EmptyCapabilityRowUnify — two empty capability rows
// should unify.
func TestProbeD_Evidence_EmptyCapabilityRowUnify(t *testing.T) {
	u := unify.NewUnifier()
	r1 := types.EmptyRow()
	r2 := types.EmptyRow()
	if err := u.Unify(r1, r2); err != nil {
		t.Fatalf("empty capability rows should unify: %v", err)
	}
}

// =====================================================================
// From probe_d: Constraint resolution
// =====================================================================

// TestProbeD_Constraint_MissingInstance — using a method without an instance
// should produce ErrNoInstance.
func TestProbeD_Constraint_MissingInstance(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
data Pair := \a b. { MkPair: (a, b); }

class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }

-- No Eq instance for Pair.
main := eq (MkPair True False) (MkPair False True)
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNoInstance)
}

// TestProbeD_Constraint_OverlappingInstances — two instances for the exact
// same type should produce ErrOverlap.
func TestProbeD_Constraint_OverlappingInstances(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
class Show a { show :: a -> a }
instance Show Bool { show := \x. x }
instance Show Bool { show := \x. True }
main := show False
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrOverlap)
}

// TestProbeD_Constraint_SuperclassResolutionChain — C3 => C2 => C1,
// using a C1 method with only a C3 constraint, through 3 levels.
func TestProbeD_Constraint_SuperclassResolutionChain(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }

class C1 a { m1 :: a -> Bool }
class C1 a => C2 a { m2 :: a -> Bool }
class C2 a => C3 a { m3 :: a -> Bool }

instance C1 Bool { m1 := \x. True }
instance C2 Bool { m2 := \x. True }
instance C3 Bool { m3 := \x. True }

f :: \ a. C3 a => a -> Bool
f := \x. m1 x

main := f True
`
	checkSource(t, source, nil)
}

// TestProbeD_Constraint_MultiParamClass — a multi-parameter type class
// with functional dependency.
func TestProbeD_Constraint_MultiParamClass(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
data Unit := { Unit: (); }

class Convert a b | a =: b { convert :: a -> b }
instance Convert Bool Unit { convert := \x. Unit }

main := convert True
`
	checkSource(t, source, nil)
}

// =====================================================================
// From probe_e: Evidence row and constraint handling
// =====================================================================

// TestProbeE_Evidence_EmptyConstraintRowUnification — two empty constraint
// rows should unify.
func TestProbeE_Evidence_EmptyConstraintRowUnification(t *testing.T) {
	u := unify.NewUnifier()
	a := types.EmptyConstraintRow()
	b := types.EmptyConstraintRow()
	if err := u.Unify(a, b); err != nil {
		t.Errorf("empty constraint rows should unify: %v", err)
	}
}

// TestProbeE_Evidence_ConstraintRowWithMismatchedClass — constraint rows
// with different class names should fail.
func TestProbeE_Evidence_ConstraintRowWithMismatchedClass(t *testing.T) {
	u := unify.NewUnifier()
	a := types.SingleConstraint("Eq", []types.Type{types.Con("Int")})
	b := types.SingleConstraint("Ord", []types.Type{types.Con("Int")})
	err := u.Unify(a, b)
	if err == nil {
		t.Fatal("expected error unifying Eq Int with Ord Int constraint rows")
	}
}

// TestProbeE_Evidence_ConstraintRowWithMetaArgs — constraint rows where
// args contain metas should unify and solve the metas.
func TestProbeE_Evidence_ConstraintRowWithMetaArgs(t *testing.T) {
	u := unify.NewUnifier()
	meta := &types.TyMeta{ID: 1, Kind: types.KType{}}
	a := types.SingleConstraint("Eq", []types.Type{meta})
	b := types.SingleConstraint("Eq", []types.Type{types.Con("Bool")})
	if err := u.Unify(a, b); err != nil {
		t.Fatalf("constraint rows with meta args should unify: %v", err)
	}
	solved := u.Zonk(meta)
	if con, ok := solved.(*types.TyCon); !ok || con.Name != "Bool" {
		t.Errorf("expected meta solved to Bool, got %s", types.Pretty(solved))
	}
}

// TestProbeE_Evidence_MultipleConstraintsSameClass — two constraints on the
// same class with different type args.
func TestProbeE_Evidence_MultipleConstraintsSameClass(t *testing.T) {
	source := `
data Bool := { True: (); False: (); }
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }

data Pair := \a b. { MkPair: (a, b); }

-- Two Eq constraints on different types
f :: \a b. (Eq a, Eq b) => a -> b -> Bool
f := \x y. eq x x

main := f True True
`
	checkSource(t, source, nil)
}
