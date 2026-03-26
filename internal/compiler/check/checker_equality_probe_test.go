//go:build probe

// Equality constraint probe tests — adversarial edge cases.
// Does NOT cover: basic equality (checker_equality_test.go), parser (parse_equality_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

func TestProbe_EqualityNestedInClass(t *testing.T) {
	// Equality constraint mixed with class constraint in nested forall.
	source := `
form Eq := \a. { eq: a -> a -> Bool }
form Bool := { True: Bool; False: Bool }
impl Eq Int := { eq := \_ _. True }

f :: \a b. (a ~ Int, b ~ Int, Eq a) => a -> b -> Bool
f := \x y. eq x y
main := f 42 43`
	checkSource(t, source, nil)
}

func TestProbe_EqualityWithTypeFamilyConcrete(t *testing.T) {
	// Equality constraint with a concrete type family result.
	source := `
form Bool := { True: Bool; False: Bool }
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a }

type IsJust :: Bool := \(m: Maybe). case m {
  Just _ => True;
  Nothing => False
}

-- Concrete: IsJust (Just Int) reduces to True, then True ~ True succeeds.
f :: (IsJust (Just Int) ~ True) => Int
f := 42
main := 42`
	checkSource(t, source, nil)
}

func TestProbe_EqualityBothConcreteFail(t *testing.T) {
	// (Int ~ Bool) => should fail at definition site.
	source := `
form Bool := { True: Bool; False: Bool }
f :: Int ~ Bool => Int
f := 42
main := 42`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

func TestProbe_EqualitySkolemEscapeBlocked(t *testing.T) {
	// The equality should not leak the skolem outside its scope.
	source := `
f :: \a. a ~ Int => a -> Int
f := \x. x
g :: Int -> Int
g := f
main := g 42`
	checkSource(t, source, nil)
}

func TestProbe_EqualitySymmetric(t *testing.T) {
	// a ~ Int and Int ~ a should both work for the same body.
	source1 := `
f :: \a. a ~ Int => a -> Int
f := \x. x
main := f 42`
	checkSource(t, source1, nil)

	source2 := `
f :: \a. Int ~ a => a -> Int
f := \x. x
main := f 42`
	checkSource(t, source2, nil)
}
