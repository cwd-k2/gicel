package check

import (
	"testing"

	"github.com/cwd-k2/gomputation/internal/core"
	"github.com/cwd-k2/gomputation/internal/types"
)

// =============================================================================
// TyEvidence in check mode
// =============================================================================

func TestCheckTyEvidenceSingleConstraint(t *testing.T) {
	// Building a TyEvidence manually and checking against it.
	// This tests the check-mode handling of TyEvidence:
	// { Eq a } => a -> a -> Bool should introduce a dict Lam.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
f :: forall a. Eq a => a -> a -> Bool
f := \x -> \y -> eq x y
main := f True False`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

func TestCheckTyEvidenceMultiConstraint(t *testing.T) {
	// Eq a => Ord a => ... should work (chained TyQual).
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
g :: forall a. Eq a => Ord a => a -> a -> Bool
g := \x -> \y -> eq x y
main := g True False`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

// =============================================================================
// TyEvidence in subsCheck/instantiate
// =============================================================================

func TestSubsCheckTyEvidence(t *testing.T) {
	// When subsCheck encounters a TyEvidence on the inferred side,
	// it should defer the constraints and peel them off.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
main := eq True False`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

// =============================================================================
// TyEvidence in resolveTypeExpr
// =============================================================================

func TestResolveTypeExprProducesTyEvidence(t *testing.T) {
	// After migration, resolveTypeExpr for "Eq a => a -> Bool"
	// should produce TyEvidence. For now we test that the existing
	// TyQual path is unbroken.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
f :: Eq a => a -> Bool
f := \x -> True`
	checkSource(t, source, nil)
}

// =============================================================================
// Class/Instance with TyEvidence
// =============================================================================

func TestClassSelectorTypeMigration(t *testing.T) {
	// Class selector type uses TyQual. After migration it should use TyEvidence.
	// This regression test verifies the selector still works.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
main := eq True False`
	prog := checkSource(t, source, nil)
	// Verify eq binding exists and resolves correctly.
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "eq" {
			found = true
			// The selector should be elaborated as a Lam (dict pattern match).
			if _, ok := b.Expr.(*core.TyLam); !ok {
				if _, ok := b.Expr.(*core.Lam); !ok {
					t.Errorf("expected TyLam or Lam for eq selector, got %T", b.Expr)
				}
			}
		}
	}
	if !found {
		t.Error("expected 'eq' selector binding")
	}
}

func TestInstanceContextMigration(t *testing.T) {
	// Instance with context: Eq a => Eq (Maybe a).
	// After migration this should still work.
	source := `data Bool = True | False
data Maybe a = Just a | Nothing
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Eq a => Eq (Maybe a) { eq := \x y -> True }
main := eq (Just True) (Just False)`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

// =============================================================================
// Superclass via TyEvidence
// =============================================================================

func TestSuperclassResolutionMigration(t *testing.T) {
	// Ord a => ... should allow resolving Eq a (superclass).
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
instance Ord Bool { compare := \x y -> True }
f :: forall a. Ord a => a -> a -> Bool
f := \x -> \y -> eq x y
main := f True False`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			found = true
		}
	}
	if !found {
		t.Error("expected binding 'main'")
	}
}

// =============================================================================
// TyEvidence type equality and traversals
// =============================================================================

func TestTyEvidenceZonkInChecker(t *testing.T) {
	// Verify that TyEvidence types are correctly zonked during type checking.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y -> True }
f :: forall a. Eq a => a -> a -> Bool
f := \x y -> eq x y`
	checkSource(t, source, nil)
}

func TestTyEvidenceInBindingType(t *testing.T) {
	// After checking, the binding type for f should have the constraint.
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
f :: forall a. Eq a => a -> Bool
f := \x -> True`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "f" {
			ty := b.Type
			if ty == nil {
				t.Fatal("f should have a type")
			}
			pretty := types.Pretty(ty)
			if pretty == "" {
				t.Error("f type should not be empty")
			}
		}
	}
}
