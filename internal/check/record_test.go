package check

import (
	"testing"

	"github.com/cwd-k2/gomputation/internal/errs"
)

// =============================================================================
// Record literal — type inference
// =============================================================================

func TestRecordLitEmpty(t *testing.T) {
	// {} should have type Record {}
	source := `main := {}`
	checkSource(t, source, nil)
}

func TestRecordLitSimple(t *testing.T) {
	// { x = 42 } should infer Record { x : Int }
	source := `main := { x = 42 }`
	checkSource(t, source, nil)
}

func TestRecordLitMultiField(t *testing.T) {
	source := `data Bool = True | False
main := { x = 42, y = True }`
	checkSource(t, source, nil)
}

func TestRecordLitNested(t *testing.T) {
	source := `main := { x = { y = 42 } }`
	checkSource(t, source, nil)
}

// =============================================================================
// Record projection — type inference
// =============================================================================

func TestRecordProjSimple(t *testing.T) {
	source := `main := { x = 42 }!#x`
	checkSource(t, source, nil)
}

func TestRecordProjChained(t *testing.T) {
	source := `main := { x = { y = 42 } }!#x!#y`
	checkSource(t, source, nil)
}

func TestRecordProjMissing(t *testing.T) {
	// Projecting a field that doesn't exist should error.
	source := `main := { x = 42 }!#y`
	checkSourceExpectCode(t, source, nil, errs.ErrRowMismatch)
}

// =============================================================================
// Record update — type inference
// =============================================================================

func TestRecordUpdateSimple(t *testing.T) {
	source := `main := { { x = 42, y = 0 } | x = 100 }`
	checkSource(t, source, nil)
}

func TestRecordUpdateMulti(t *testing.T) {
	source := `data Bool = True | False
main := { { x = 42, y = True } | x = 0, y = False }`
	checkSource(t, source, nil)
}

// =============================================================================
// Record type annotations
// =============================================================================

func TestRecordAnnotated(t *testing.T) {
	source := `f :: Record { x : Int } -> Int
f := \r -> r!#x
main := f { x = 42 }`
	checkSource(t, source, nil)
}

func TestRecordRowPoly(t *testing.T) {
	// Row-polymorphic function: takes any record with field x : Int.
	source := `f :: forall r. Record { x : Int | r } -> Int
f := \r -> r!#x
main := f { x = 42, y = 0 }`
	checkSource(t, source, nil)
}

// =============================================================================
// Record patterns
// =============================================================================

func TestRecordPatternSimple(t *testing.T) {
	source := `f := \r -> case r { { x = a } -> a }
main := f { x = 42 }`
	checkSource(t, source, nil)
}

func TestRecordPatternMultiField(t *testing.T) {
	source := `data Bool = True | False
f := \r -> case r { { x = a, y = b } -> a }
main := f { x = 42, y = True }`
	checkSource(t, source, nil)
}

// =============================================================================
// Record check mode
// =============================================================================

func TestRecordCheckMode(t *testing.T) {
	// Check a record literal against an expected record type.
	source := `f :: Record { x : Int, y : Int } -> Int
f := \r -> r!#x
main := f { x = 1, y = 2 }`
	checkSource(t, source, nil)
}

func TestRecordCheckModeTypeMismatch(t *testing.T) {
	// Field type mismatch should error.
	source := `data Bool = True | False
f :: Record { x : Int } -> Int
f := \r -> r!#x
main := f { x = True }`
	checkSourceExpectCode(t, source, nil, errs.ErrTypeMismatch)
}
