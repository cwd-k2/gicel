// Row-level type family tests — Merge builtin type family.
// Does NOT cover: general type families (type_family_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

func TestMergeDisjointRows(t *testing.T) {
	// Merge { a: Int } { b: Bool } should unify with { a: Int, b: Bool }.
	source := `
form Bool := { True: Bool; False: Bool }
f :: Merge { a: Int } { b: Bool } -> Int
f := \r. 42
g :: { a: Int, b: Bool } -> Int
g := f
main := 42`
	checkSource(t, source, nil)
}

func TestMergeOverlapError(t *testing.T) {
	// Overlapping labels should produce an error when Merge is reduced.
	source := `
f :: Merge { a: Int } { a: Bool } -> Int
f := \r. 42
main := 42`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

func TestMergeEmptyRows(t *testing.T) {
	// Merge {} {} should reduce to {}.
	source := `
f :: Merge {} {} -> Int
f := \r. 42
g :: {} -> Int
g := f
main := 42`
	checkSource(t, source, nil)
}

func TestMergeOneEmpty(t *testing.T) {
	// Merge { a: Int } {} should reduce to { a: Int }.
	source := `
f :: Merge { a: Int } {} -> Int
f := \r. 42
g :: { a: Int } -> Int
g := f
main := 42`
	checkSource(t, source, nil)
}

func TestMergeRegisteredAsFamily(t *testing.T) {
	// Merge should be recognized as a type family.
	ch := newTestChecker()
	ch.installFamilyReducer()
	_, ok := ch.lookupFamily("Merge")
	if !ok {
		t.Fatal("Merge should be registered as a type family")
	}
}
