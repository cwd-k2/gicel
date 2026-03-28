// Row-level type family tests — Merge, Without, Lookup builtin type families.
// Does NOT cover: general type families (type_family_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
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

// =============================================
// Without type family
// =============================================

func TestWithoutRemovesLabel(t *testing.T) {
	// Without #a { a: Int, b: String } should reduce to { b: String }.
	source := `
f :: Without #a { a: Int, b: String } -> Int
f := \r. 42
g :: { b: String } -> Int
g := f
main := 42`
	checkSource(t, source, nil)
}

func TestWithoutLabelAbsentError(t *testing.T) {
	// Without #c { a: Int, b: String } should produce a type error (label absent).
	source := `
f :: Without #c { a: Int, b: String } -> Int
f := \r. 42
main := 42`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

func TestWithoutEmptyRow(t *testing.T) {
	// Without #a {} should produce an error (label not present).
	source := `
f :: Without #a {} -> Int
f := \r. 42
main := 42`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

func TestWithoutSingleField(t *testing.T) {
	// Without #a { a: Int } should reduce to {}.
	source := `
f :: Without #a { a: Int } -> Int
f := \r. 42
g :: {} -> Int
g := f
main := 42`
	checkSource(t, source, nil)
}

func TestWithoutRegisteredAsFamily(t *testing.T) {
	ch := newTestChecker()
	ch.installFamilyReducer()
	_, ok := ch.lookupFamily("Without")
	if !ok {
		t.Fatal("Without should be registered as a type family")
	}
}

func TestWithoutOpenRowStuck(t *testing.T) {
	// Without with an open row should be stuck (not error, not reduce).
	// Verify at the unit level: reduceWithout returns (nil, false) for open rows.
	ch := newTestChecker()
	ch.installFamilyReducer()

	labelArg := &types.TyCon{Name: "a", Level: types.L1}
	openRow := types.OpenRow(
		[]types.RowField{{Label: "a", Type: &types.TyCon{Name: "Int"}}},
		&types.TyVar{Name: "r"},
	)
	_, ok := ch.reduceTyFamily("Without", []types.Type{labelArg, openRow}, span.Span{})
	if ok {
		t.Fatal("Without with open row should be stuck (not reduced)")
	}
	if ch.errors.HasErrors() {
		t.Errorf("Without with open row should not produce errors, got: %s", ch.errors.Format())
	}
}

// =============================================
// Lookup type family
// =============================================

func TestLookupFindsLabel(t *testing.T) {
	// Lookup #a { a: Int, b: String } should reduce to Int.
	source := `
f :: Lookup #a { a: Int, b: String } -> Int
f := \x. x
main := f 42`
	checkSource(t, source, nil)
}

func TestLookupSecondField(t *testing.T) {
	// Lookup #b { a: Int, b: String } should reduce to String.
	source := `
f :: Lookup #b { a: Int, b: String } -> String
f := \x. x
main := f "hello"`
	checkSource(t, source, nil)
}

func TestLookupLabelAbsentError(t *testing.T) {
	// Lookup #c { a: Int, b: String } should produce a type error (label absent).
	source := `
f :: Lookup #c { a: Int, b: String } -> Int
f := \x. x
main := 42`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

func TestLookupEmptyRowError(t *testing.T) {
	// Lookup #a {} should produce a type error.
	source := `
f :: Lookup #a {} -> Int
f := \x. x
main := 42`
	checkSourceExpectCode(t, source, nil, diagnostic.ErrTypeMismatch)
}

func TestLookupRegisteredAsFamily(t *testing.T) {
	ch := newTestChecker()
	ch.installFamilyReducer()
	_, ok := ch.lookupFamily("Lookup")
	if !ok {
		t.Fatal("Lookup should be registered as a type family")
	}
}

func TestLookupOpenRowStuck(t *testing.T) {
	// Lookup with an open row should be stuck (not error, not reduce).
	// Verify at the unit level: reduceLookup returns (nil, false) for open rows.
	ch := newTestChecker()
	ch.installFamilyReducer()

	labelArg := &types.TyCon{Name: "a", Level: types.L1}
	openRow := types.OpenRow(
		[]types.RowField{{Label: "a", Type: &types.TyCon{Name: "Int"}}},
		&types.TyVar{Name: "r"},
	)
	_, ok := ch.reduceTyFamily("Lookup", []types.Type{labelArg, openRow}, span.Span{})
	if ok {
		t.Fatal("Lookup with open row should be stuck (not reduced)")
	}
	if ch.errors.HasErrors() {
		t.Errorf("Lookup with open row should not produce errors, got: %s", ch.errors.Format())
	}
}

// =============================================
// Without + Lookup combined
// =============================================

func TestWithoutAndLookupCombined(t *testing.T) {
	// Using both Without and Lookup together should work.
	source := `
f :: Lookup #a { a: Int, b: String } -> Int
f := \x. x
g :: Without #a { a: Int, b: String } -> Int
g := \r. 42
main := f (g { b: "hello" })`
	checkSource(t, source, nil)
}
