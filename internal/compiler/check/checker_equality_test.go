// Equality constraint checker tests — (a ~ T) => type signatures.
// Does NOT cover: parser (parse_equality_test.go), solver internals (solve/).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// --- Positive tests ---

func TestEqualityConstraintSimple(t *testing.T) {
	// (a ~ Int) => a -> Int: body can use a as Int.
	source := `
f :: \a. a ~ Int => a -> Int
f := \x. x
main := f 42`
	prog := checkSource(t, source, nil)
	for _, b := range prog.Bindings {
		if b.Name == "main" {
			pretty := testOps.Pretty(b.Type)
			if pretty != "Int" {
				t.Errorf("expected main :: Int, got %s", testOps.Pretty(b.Type))
			}
		}
	}
}

func TestEqualityConstraintWithClassConstraint(t *testing.T) {
	// (a ~ Int, Eq a) => should work with both equality and class constraints.
	source := `
form Eq := \a. { eq: a -> a -> Bool }
form Bool := { True: Bool; False: Bool }
impl Eq Int := { eq := \x y. True }
f :: \a. (a ~ Int, Eq a) => a -> Bool
f := \x. eq x x
main := f 42`
	checkSource(t, source, nil)
}

func TestEqualityConstraintBothDirections(t *testing.T) {
	// Int ~ a should also work (RHS is skolem).
	source := `
f :: \a. Int ~ a => a -> Int
f := \x. x
main := f 42`
	checkSource(t, source, nil)
}

// --- Negative tests ---

func TestEqualityConstraintMismatch(t *testing.T) {
	// (a ~ Int) => a -> Bool, but body returns a (which is Int), not Bool.
	source := `
form Bool := { True: Bool; False: Bool }
f :: \a. a ~ Int => a -> Bool
f := \x. x
main := f 42`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

func TestEqualityConstraintCallSiteWanted(t *testing.T) {
	// Calling with wrong type should fail: f expects a ~ Int,
	// calling with Bool should fail.
	source := `
form Bool := { True: Bool; False: Bool }
f :: \a. a ~ Int => a -> Int
f := \x. x
main := f True`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}
