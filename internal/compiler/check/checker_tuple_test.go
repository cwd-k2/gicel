package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// =============================================================================
// Tuple literals — type inference
// =============================================================================

func TestTupleUnit(t *testing.T) {
	// () should have type Record {}
	source := `main := ()`
	checkSource(t, source, nil)
}

func TestTuplePair(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
main := (42, True)`
	checkSource(t, source, nil)
}

func TestTupleTriple(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
main := (1, 2, True)`
	checkSource(t, source, nil)
}

func TestTupleNested(t *testing.T) {
	source := `main := ((1, 2), 3)`
	checkSource(t, source, nil)
}

// =============================================================================
// Tuple projection
// =============================================================================

func TestTupleFst(t *testing.T) {
	source := `main := (42, 0).#_1`
	checkSource(t, source, nil)
}

func TestTupleSnd(t *testing.T) {
	source := `main := (42, 0).#_2`
	checkSource(t, source, nil)
}

// =============================================================================
// Tuple type annotations
// =============================================================================

func TestTupleAnnotated(t *testing.T) {
	source := `f :: (Int, Int) -> Int
f := \p. p.#_1
main := f (1, 2)`
	checkSource(t, source, nil)
}

func TestUnitAnnotated(t *testing.T) {
	source := `f :: () -> Int
f := \u. 42
main := f ()`
	checkSource(t, source, nil)
}

// =============================================================================
// Tuple patterns
// =============================================================================

func TestTuplePatternPair(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
f := \p. case p { (a, b) => a }
main := f (42, True)`
	checkSource(t, source, nil)
}

func TestTuplePatternUnit(t *testing.T) {
	source := `f := \u. case u { () => 42 }
main := f ()`
	checkSource(t, source, nil)
}

// =============================================================================
// Tuple check mode
// =============================================================================

func TestTupleCheckMode(t *testing.T) {
	source := `f :: (Int, Int) -> Int
f := \p. p.#_1
main := f (1, 2)`
	checkSource(t, source, nil)
}

func TestTupleCheckModeTypeMismatch(t *testing.T) {
	source := `form Bool := { True: Bool; False: Bool; }
f :: (Int, Int) -> Int
f := \p. p.#_1
main := f (True, 2)`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

// =============================================================================
// Mixed: records and tuples
// =============================================================================

func TestMixedRecordInTuple(t *testing.T) {
	source := `main := ({ x: 42 }, 0)`
	checkSource(t, source, nil)
}

func TestMixedTupleInRecord(t *testing.T) {
	source := `main := { x: (1, 2) }`
	checkSource(t, source, nil)
}
