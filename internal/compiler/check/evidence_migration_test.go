package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// =============================================================================
// TyEvidence in check mode
// =============================================================================

func TestCheckTyEvidenceSingleConstraint(t *testing.T) {
	// Building a TyEvidence manually and checking against it.
	// This tests the check-mode handling of TyEvidence:
	// { Eq a } => a -> a -> Bool should introduce a dict Lam.
	source := `data Bool := { True: (); False: (); }
data Eq := \a. { eq :: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y
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
	// (Eq a, Ord a) => ... should work (constraint tuple).
	source := `data Bool := { True: (); False: (); }
data Eq := \a. { eq :: a -> a -> Bool }
data Ord := \a. Eq a => { compare :: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Ord Bool := { compare := \x y. True }
g :: \ a. (Eq a, Ord a) => a -> a -> Bool
g := \x y. eq x y
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
	source := `data Bool := { True: (); False: (); }
data Eq := \a. { eq :: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
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
	// resolveTypeExpr for "Eq a => a -> Bool" produces TyEvidence.
	source := `data Bool := { True: (); False: (); }
data Eq := \a. { eq :: a -> a -> Bool }
f :: Eq a => a -> Bool
f := \x. True`
	checkSource(t, source, nil)
}

// =============================================================================
// Class/Instance with TyEvidence
// =============================================================================

func TestClassSelectorTypeMigration(t *testing.T) {
	// Class selector type should use TyEvidence (not TyQual).
	source := `data Bool := { True: (); False: (); }
data Eq := \a. { eq :: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
main := eq True False`
	prog := checkSource(t, source, nil)
	found := false
	for _, b := range prog.Bindings {
		if b.Name == "eq" {
			found = true
			// The selector should be elaborated as a TyLam wrapping a Lam.
			if _, ok := b.Expr.(*ir.TyLam); !ok {
				if _, ok := b.Expr.(*ir.Lam); !ok {
					t.Errorf("expected TyLam or Lam for eq selector, got %T", b.Expr)
				}
			}
			// The binding type must be TyEvidence (not TyQual) after migration.
			ty := b.Type
			// Strip outer foralls.
			for {
				if f, ok := ty.(*types.TyForall); ok {
					ty = f.Body
				} else {
					break
				}
			}
			if _, ok := ty.(*types.TyEvidence); !ok {
				t.Errorf("selector type should be TyEvidence after stripping foralls, got %T", ty)
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
	source := `data Bool := { True: (); False: (); }
data Maybe := \a. { Just: a; Nothing: (); }
data Eq := \a. { eq :: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Eq a => Eq (Maybe a) := { eq := \x y. True }
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
	source := `data Bool := { True: (); False: (); }
data Eq := \a. { eq :: a -> a -> Bool }
data Ord := \a. Eq a => { compare :: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
impl Ord Bool := { compare := \x y. True }
f :: \ a. Ord a => a -> a -> Bool
f := \x y. eq x y
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
	source := `data Bool := { True: (); False: (); }
data Eq := \a. { eq :: a -> a -> Bool }
impl Eq Bool := { eq := \x y. True }
f :: \ a. Eq a => a -> a -> Bool
f := \x y. eq x y`
	checkSource(t, source, nil)
}

func TestTyEvidenceInBindingType(t *testing.T) {
	// After checking, the binding type for f should have TyForall → TyEvidence structure.
	source := `data Bool := { True: (); False: (); }
data Eq := \a. { eq :: a -> a -> Bool }
f :: \ a. Eq a => a -> Bool
f := \x. True`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "f" {
			ty := b.Type
			if ty == nil {
				t.Fatal("f should have a type")
			}
			// Expect TyForall wrapping a TyEvidence with Eq constraint.
			fa, ok := ty.(*types.TyForall)
			if !ok {
				t.Fatalf("expected TyForall, got %T: %s", ty, types.Pretty(ty))
			}
			ev, ok := fa.Body.(*types.TyEvidence)
			if !ok {
				t.Fatalf("expected TyEvidence under TyForall, got %T: %s", fa.Body, types.Pretty(fa.Body))
			}
			if ev.Constraints == nil {
				t.Fatal("expected non-nil Constraints")
			}
			if len(ev.Constraints.ConEntries()) == 0 {
				t.Fatal("expected at least one constraint entry")
			}
			if ev.Constraints.ConEntries()[0].ClassName != "Eq" {
				t.Errorf("expected Eq constraint, got %s", ev.Constraints.ConEntries()[0].ClassName)
			}
			return
		}
	}
	t.Error("expected binding 'f'")
}
