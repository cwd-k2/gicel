package check

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Structural Stress Tests — exercises subsystem interactions the
// per-feature stress tests leave uncovered.
// =============================================================================

// TestStressHigherRankThreeLevels — three nested \ levels with subsumption.
func TestStressHigherRankThreeLevels(t *testing.T) {
	source := `
data Bool = True | False

-- Level 1: \ a. a -> a
id :: \ a. a -> a
id := \x -> x

-- Level 2: (\ a. a -> a) -> Bool
applyId :: (\ a. a -> a) -> Bool
applyId := \f -> f True

-- Level 3: ((\ a. a -> a) -> Bool) -> Bool
applyApplyId :: ((\ a. a -> a) -> Bool) -> Bool
applyApplyId := \g -> g id

main := applyApplyId applyId
`
	checkSource(t, source, nil)
}

// TestStressHigherRankNested — higher-rank nested in function arguments.
func TestStressHigherRankNested(t *testing.T) {
	source := `
data Bool = True | False
data Maybe a = Nothing | Just a

apply :: (\ a. a -> a) -> Bool -> Bool
apply := \f -> \x -> f x

applyMaybe :: (\ a. a -> Maybe a) -> Bool -> Maybe Bool
applyMaybe := \f -> \x -> f x

main := (apply (\x -> x) True, applyMaybe (\x -> Just x) False)
`
	checkSource(t, source, nil)
}

// TestStressManyUnannLetBindings — 15 unannotated let-bindings with class constraints.
func TestStressManyUnannLetBindings(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("data Bool = True | False\n")
	sb.WriteString("class Eq a { eq :: a -> a -> Bool }\n")
	sb.WriteString("instance Eq Bool { eq := \\x -> \\y -> True }\n\n")

	// 15 unannotated bindings, each using eq.
	for i := range 15 {
		sb.WriteString(fmt.Sprintf("f%d := eq True False\n", i))
	}
	sb.WriteString("main := f0\n")
	checkSource(t, sb.String(), nil)
}

// TestStressDeepNestedLambdas — 50 nested lambdas with explicit \.
func TestStressDeepNestedLambdas(t *testing.T) {
	const N = 50
	var sb strings.Builder
	sb.WriteString("data Bool = True | False\n")

	// Build signature: \ a0 ... a49. a0 -> a1 -> ... -> a49 -> a0
	sb.WriteString("f :: \\")
	for i := range N {
		sb.WriteString(fmt.Sprintf(" a%d", i))
	}
	sb.WriteString(". ")
	for i := range N {
		sb.WriteString(fmt.Sprintf("a%d -> ", i))
	}
	sb.WriteString("a0\n")

	// Build body: \x0 -> \x1 -> ... -> \x49 -> x0
	sb.WriteString("f := ")
	for i := range N {
		sb.WriteString(fmt.Sprintf("\\x%d -> ", i))
	}
	sb.WriteString("x0\n")

	// Apply with Bool for all positions.
	sb.WriteString("main := f")
	for range N {
		sb.WriteString(" True")
	}
	sb.WriteString("\n")
	checkSource(t, sb.String(), nil)
}

// TestStressRowHKTInteraction — Functor on Record with row polymorphism.
func TestStressRowHKTInteraction(t *testing.T) {
	source := `
data Bool = True | False
data Maybe a = Nothing | Just a

class Functor (f : Type -> Type) {
  fmap :: \ a b. (a -> b) -> f a -> f b
}

instance Functor Maybe {
  fmap := \g -> \mx -> case mx { Nothing -> Nothing; Just x -> Just (g x) }
}

-- Use Functor Maybe with a record argument.
not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }

main := fmap not (Just True)
`
	checkSource(t, source, nil)
}

// TestStressExhaustiveGADTManyCons — exhaustiveness with 10 GADT constructors.
func TestStressExhaustiveGADTManyCons(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("data Bool = True | False\n")
	sb.WriteString("data Expr a = {\n")
	// 10 constructors, separated by ;
	for i := range 10 {
		if i > 0 {
			sb.WriteString(";\n  ")
		} else {
			sb.WriteString("  ")
		}
		sb.WriteString(fmt.Sprintf("Con%d :: Bool -> Expr Bool", i))
	}
	sb.WriteString("\n}\n\n")
	// Match all 10 constructors, branches separated by ;
	sb.WriteString("eval :: Expr Bool -> Bool\neval := \\e -> case e {\n")
	for i := range 10 {
		if i > 0 {
			sb.WriteString(";\n")
		}
		sb.WriteString(fmt.Sprintf("  Con%d b -> b", i))
	}
	sb.WriteString("\n}\n")
	sb.WriteString("main := eval (Con0 True)\n")
	checkSource(t, sb.String(), nil)
}

// TestStressConstraintAliasinContext — type alias used as constraint context.
func TestStressConstraintAliasInContext(t *testing.T) {
	source := `
data Bool = True | False

class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Bool }
instance Eq Bool { eq := \x -> \y -> True }
instance Ord Bool { compare := \x -> \y -> True }

type EqOrd a = Eq a => Ord a => a -> a -> Bool

-- Use the alias.
bothCheck :: \ a. EqOrd a
bothCheck := \x -> \y -> eq x y

main := bothCheck True False
`
	checkSource(t, source, nil)
}

// TestStressDeepSuperclassWithHKT — 4-level superclass chain + poly-kinded class.
func TestStressDeepSuperclassWithHKT(t *testing.T) {
	source := `
data Bool = True | False
data Maybe a = Nothing | Just a

class C1 a { m1 :: a -> Bool }
class C1 a => C2 a { m2 :: a -> Bool }
class C2 a => C3 a { m3 :: a -> Bool }
class C3 a => C4 a { m4 :: a -> Bool }

instance C1 Bool { m1 := \x -> True }
instance C2 Bool { m2 := \x -> True }
instance C3 Bool { m3 := \x -> True }
instance C4 Bool { m4 := \x -> True }

class Functor (f : Type -> Type) {
  fmap :: \ a b. (a -> b) -> f a -> f b
}

instance Functor Maybe {
  fmap := \g -> \mx -> case mx { Nothing -> Nothing; Just x -> Just (g x) }
}

-- Use deep superclass method via top-level constraint + HKT.
f :: \ a. C4 a => a -> Bool
f := \x -> m1 x

main := fmap f (Just True)
`
	checkSource(t, source, nil)
}

// TestStressManyContextualInstances — 8 contextual instances in a chain.
func TestStressManyContextualInstances(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("data Bool = True | False\n")
	sb.WriteString("class Eq a { eq :: a -> a -> Bool }\n")
	sb.WriteString("instance Eq Bool { eq := \\x -> \\y -> True }\n\n")

	// 8 wrapper types, each with a contextual Eq instance.
	for i := range 8 {
		sb.WriteString(fmt.Sprintf("data W%d a = MkW%d a\n", i, i))
		sb.WriteString(fmt.Sprintf("instance Eq a => Eq (W%d a) { eq := \\x -> \\y -> True }\n\n", i))
	}

	// Nested application: Eq (W0 (W1 (W2 (W3 (W4 (W5 (W6 (W7 Bool))))))))
	sb.WriteString("main := eq ")
	nested := "Bool"
	for i := 7; i >= 0; i-- {
		nested = fmt.Sprintf("(W%d %s)", i, nested)
	}
	sb.WriteString(fmt.Sprintf("(MkW0 (MkW1 (MkW2 (MkW3 (MkW4 (MkW5 (MkW6 (MkW7 True)))))))) "))
	sb.WriteString(fmt.Sprintf("(MkW0 (MkW1 (MkW2 (MkW3 (MkW4 (MkW5 (MkW6 (MkW7 False))))))))\n"))
	checkSource(t, sb.String(), nil)
}

// TestStressMultiParamClassManyArgs — multi-param class with 4 type parameters.
func TestStressMultiParamClassManyArgs(t *testing.T) {
	source := `
data Bool = True | False
data Unit = Unit

class Multi a b c d {
  multi :: a -> b -> c -> d -> Bool
}

instance Multi Bool Bool Bool Bool {
  multi := \a -> \b -> \c -> \d -> True
}

instance Multi Unit Unit Unit Unit {
  multi := \a -> \b -> \c -> \d -> False
}

main := (multi True True True True, multi Unit Unit Unit Unit)
`
	checkSource(t, source, nil)
}
