package check

import "testing"

// =============================================================================
// Phase 5C: Constraint-Kinded Type Parameters
// =============================================================================

func TestConstraintKindedParamForall(t *testing.T) {
	// \ (c: Constraint). ... should parse and check without errors.
	source := `data Bool = True | False
f :: \ (c: Constraint). Bool
f := True`
	checkSource(t, source, nil)
}

func TestConstraintKindedParamInClassDecl(t *testing.T) {
	// A class can declare a Constraint-kinded type parameter.
	source := `data Bool = True | False
class Constrained (c: Constraint) { witness :: Bool }`
	checkSource(t, source, nil)
}

func TestConstraintKindedParamMultiple(t *testing.T) {
	// Multiple \ binders, one with Constraint kind.
	source := `data Bool = True | False
f :: \ a (c: Constraint). a -> Bool
f := \x. True`
	checkSource(t, source, nil)
}

func TestConstraintKindedParamArrowKind(t *testing.T) {
	// Constraint -> Type kind (higher-kinded constraint).
	source := `data Bool = True | False
f :: \ (f: Constraint -> Type). Bool
f := True`
	checkSource(t, source, nil)
}

func TestConstraintKindedParamInTypeAlias(t *testing.T) {
	// type WithEq a = (Eq a) — a constraint alias.
	// Already tested in Phase 5A, but now with explicit Constraint kind:
	source := `data Bool = True | False
class Eq a { eq :: a -> a -> Bool }
instance Eq Bool { eq := \x y. True }
type Eqable a = Eq a => a -> a -> Bool
f :: \ a. Eqable a
f := \x y. eq x y
main := f True False`
	checkSource(t, source, nil)
}
