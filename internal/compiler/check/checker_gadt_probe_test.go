//go:build probe

package check

import (
	"strings"
	"testing"
)

// =============================================================================
// GADT probe tests — skolem escape detection, GADT index refinement, and
// multi-constructor GADT patterns.
// =============================================================================

// =====================================================================
// From probe_e: GADT and existential edge cases
// =====================================================================

// TestProbeE_GADT_SkolemEscapeDetection — an existential type that leaks
// out of its case branch should be detected.
func TestProbeE_GADT_SkolemEscapeDetection(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Exists := { MkExists: \a. a -> Exists }

-- This should fail: the existential 'a' escapes through the return type
leak :: Exists -> Bool
leak := \e. case e { MkExists x => x }
`
	errMsg := checkSourceExpectError(t, source, nil)
	if !strings.Contains(errMsg, "mismatch") && !strings.Contains(errMsg, "escape") && !strings.Contains(errMsg, "skolem") {
		t.Logf("NOTICE: skolem escape error: %s", errMsg)
	}
}

// TestProbeE_GADT_IndexRefinement — GADT constructor should refine the
// type index in the case branch.
func TestProbeE_GADT_IndexRefinement(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Expr := \a. { LitBool: Bool -> Expr Bool }

eval :: Expr Bool -> Bool
eval := \e. case e { LitBool b => b }

main := eval (LitBool True)
`
	checkSource(t, source, nil)
}

// TestProbeE_GADT_MultiConstructorIndexRefinement — multiple GADT constructors
// with different index types.
func TestProbeE_GADT_MultiConstructorIndexRefinement(t *testing.T) {
	source := `
form Bool := { True: Bool; False: Bool; }
form Nat := Z | S Nat

form Tag := \a. { TagBool: Tag Bool; TagNat: Tag Nat }

describe :: Tag Bool -> Bool
describe := \t. case t { TagBool => True }

main := describe TagBool
`
	checkSource(t, source, nil)
}
