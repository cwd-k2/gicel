// Exhaustiveness tests — complete/incomplete/wildcard/nested/redundant/record patterns.
// Does NOT cover: GADT exhaustiveness (gadt_check_test.go), exhaust algorithm (exhaust/exhaust_test.go).

package check

import (
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

func TestExhaustiveComplete(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { True -> True; False -> False }`
	checkSource(t, source, nil)
}

func TestExhaustiveIncomplete(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { True -> True }`
	errMsg := checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
	if !strings.Contains(errMsg, "False") {
		t.Errorf("expected missing constructor 'False' in error, got: %s", errMsg)
	}
}

func TestExhaustiveWildcard(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { _ -> True }`
	checkSource(t, source, nil)
}

func TestExhaustiveVarPattern(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { x -> x }`
	checkSource(t, source, nil)
}

func TestExhaustiveNestedComplete(t *testing.T) {
	source := `data Maybe a := Just a | Nothing
data Bool := True | False
main := \m. case m { Just (Just _) -> 1; Just (Nothing) -> 2; Nothing -> 3 }`
	checkSource(t, source, nil)
}

func TestExhaustiveNestedIncomplete(t *testing.T) {
	source := `data Maybe a := Just a | Nothing
data Bool := True | False
main := \m. case m { Just (Just _) -> 1; Nothing -> 3 }`
	errMsg := checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
	if !strings.Contains(errMsg, "Nothing") && !strings.Contains(errMsg, "Just") {
		t.Errorf("expected mention of missing pattern, got: %s", errMsg)
	}
}

func TestRedundantPattern(t *testing.T) {
	source := `data Bool := True | False
main := \b. case b { _ -> 1; True -> 2 }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrRedundantPattern)
}

// --- Exhaustiveness: additional coverage ---

func TestExhaustiveRecordPatterns(t *testing.T) {
	// Record patterns should be handled by the exhaustiveness checker.
	source := `data Bool := True | False
main := \r. case r { { x: True, y: _ } -> 1; { x: False, y: _ } -> 2 }`
	checkSource(t, source, nil)
}

func TestExhaustiveWildcardOnly(t *testing.T) {
	// A single wildcard always covers all cases.
	source := `data Color := Red | Green | Blue
main := \c. case c { _ -> 1 }`
	checkSource(t, source, nil)
}

func TestExhaustiveMultiConComplete(t *testing.T) {
	// Three-constructor type fully covered.
	source := `data Tri := A | B | C
main := \t. case t { A -> 1; B -> 2; C -> 3 }`
	checkSource(t, source, nil)
}

func TestExhaustiveMultiConIncomplete(t *testing.T) {
	// Missing constructor C should be reported.
	source := `data Tri := A | B | C
main := \t. case t { A -> 1; B -> 2 }`
	errMsg := checkSourceExpectCode(t, source, nil, diagnostic.ErrNonExhaustive)
	if !strings.Contains(errMsg, "C") {
		t.Errorf("expected missing constructor 'C' in error, got: %s", errMsg)
	}
}

func TestRedundantPatternMiddle(t *testing.T) {
	// Wildcard before specific constructors: second alt is redundant.
	source := `data Bool := True | False
main := \b. case b { True -> 1; True -> 2; False -> 3 }`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrRedundantPattern)
}

func TestExhaustiveGADTFiltering(t *testing.T) {
	// GADT: only constructors applicable to the scrutinee type should be required.
	source := `data Bool := True | False
data Unit := Unit
data Tag a := { TagBool :: Tag Bool; TagUnit :: Tag Unit }
f :: Tag Bool -> Bool
f := \t. case t { TagBool -> True }`
	checkSource(t, source, nil)
}
