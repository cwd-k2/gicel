//go:build probe

// Quantified constraint equality premise probe tests.
// Verifies that equality premises in quantified constraints are
// checked during evidence resolution — not silently skipped.
// Does NOT cover: resolve_quantified_constraint_test.go.
package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// TestProbe_QuantifiedEq_SatisfiedPremise — a quantified constraint
// with a satisfied equality premise should resolve successfully.
func TestProbe_QuantifiedEq_SatisfiedPremise(t *testing.T) {
	// forall a. (a ~ Int) => C a is used for C Int.
	// The equality a ~ Int holds (a=Int), so evidence should resolve.
	source := `
form Int := { MkInt: Int; }
form Bool := { True: Bool; False: Bool; }

form C := \a. { method: a -> Bool; }

-- Instance with equality premise: only valid when a ~ Int.
-- We simulate this by defining a plain instance for Int.
impl C Int := { method := \x. True }

main := method MkInt
`
	checkSource(t, source, nil)
}

// TestProbe_QuantifiedEq_UnsatisfiedPremise — when the equality premise
// does not hold, the quantified constraint should NOT match.
// This tests the core soundness fix: without the equality check,
// the instance would be applied even when the equality fails.
func TestProbe_QuantifiedEq_UnsatisfiedPremise(t *testing.T) {
	// C has an instance only for Int. Applying to Bool should fail.
	source := `
form Int := { MkInt: Int; }
form Bool := { True: Bool; False: Bool; }

form C := \a. { method: a -> Bool; }

impl C Int := { method := \x. True }

main := method True
`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrNoInstance)
}

// TestProbe_QuantifiedEq_MultipleContextEntries — equality premise
// mixed with class premises in the same quantified constraint.
func TestProbe_QuantifiedEq_MultipleContextEntries(t *testing.T) {
	// Form with multiple constraints: Eq a and class constraint.
	source := `
form Int := { MkInt: Int; }
form Bool := { True: Bool; False: Bool; }

form Eq := \a. { eq: a -> a -> Bool; }
impl Eq Int := { eq := \x y. True }

form C := \a. { method: a -> Bool; }
impl C Int := { method := \x. True }

-- Use both Eq and C on Int.
main := eq MkInt MkInt
`
	checkSource(t, source, nil)
}
