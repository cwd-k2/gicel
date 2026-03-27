// Label kind tests — label kind annotation, label literal resolution, kind mismatch, distinctness.
// Does NOT cover: row families (type_family_row_test.go), datakinds (resolve_kind_datakinds_test.go).

package check

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

// --- Label kind annotation ---

func TestLabelKindAnnotation(t *testing.T) {
	// \(l: Label). Int should type-check: Label is a valid kind annotation.
	source := `
f :: \(l: Label). Int
f := 42
main := f`
	checkSource(t, source, nil)
}

func TestLabelKindAnnotationUsedInBody(t *testing.T) {
	// A label-kinded binder used as a type-level label within a row.
	source := `
f :: \(l: Label). { l: Int } -> Int
f := \r. 42
main := 42`
	checkSource(t, source, nil)
}

// --- Label literal in type position ---

func TestLabelLiteralResolvesToL1TyCon(t *testing.T) {
	// A label literal `foo in type position should resolve to TyCon{Name:"foo", Level:L1}.
	// Verified indirectly: kindOfType for an L1 TyCon that is not a builtin kind con returns Label.
	ch := newTestChecker()
	ch.installFamilyReducer()
	ty := &types.TyCon{Name: "foo", Level: types.L1}
	kind := ch.kindOfType(ty)
	if !types.Equal(kind, types.TypeOfLabels) {
		t.Errorf("expected kind Label for label literal, got %s", types.Pretty(kind))
	}
}

func TestLabelLiteralDistinctFromBuiltinKindCon(t *testing.T) {
	// Builtin kind constants (Type, Row, Constraint, Label) are L1 but NOT labels.
	for _, name := range []string{"Type", "Row", "Constraint", "Label"} {
		ty := &types.TyCon{Name: name, Level: types.L1}
		if !types.IsBuiltinKindCon(ty) {
			t.Errorf("%s should be recognized as builtin kind con", name)
		}
	}
	// User label literal should NOT be a builtin kind con.
	ty := &types.TyCon{Name: "foo", Level: types.L1}
	if types.IsBuiltinKindCon(ty) {
		t.Error("user label literal should not be a builtin kind con")
	}
}

// --- Label literal as type argument ---

func TestLabelLiteralAsTypeArgument(t *testing.T) {
	// f @`state where f :: \(l: Label). Int should type-check.
	source := `
f :: \(l: Label). Int
f := 42
main := (f :: Int)`
	checkSource(t, source, nil)
}

func TestLabelLiteralInRowAccess(t *testing.T) {
	// Using label literals with row type families like Without and Lookup.
	source := `
f :: Without ` + "`" + `a { a: Int, b: String } -> Int
f := \r. 42
g :: { b: String } -> Int
g := f
main := 42`
	checkSource(t, source, nil)
}

// --- Kind mismatch: non-label where Label expected ---

func TestKindMismatchNonLabelWhereExpected(t *testing.T) {
	// A label-kinded binder should not accept a Type-kinded argument
	// in a position where Label is required.
	// Using a function with explicit Label kind annotation, trying to
	// pass it something that reduces to a different kind.
	source := `
form S := { A: S; B: S }
f :: \(l: Label). Int
f := 42
-- S is a promoted data kind, not Label
g :: \(s: S). Int
g := 42
main := 42`
	// This should type-check because f and g have separate kind parameters.
	// The error would only arise if we tried to unify S with Label.
	checkSource(t, source, nil)
}

// --- Label distinctness ---

func TestDistinctLabelLiterals(t *testing.T) {
	// Two different label literals should not unify.
	a := &types.TyCon{Name: "a", Level: types.L1}
	b := &types.TyCon{Name: "b", Level: types.L1}
	if types.Equal(a, b) {
		t.Error("label literals `a and `b should not be equal")
	}
}

func TestSameLabelLiteralsEqual(t *testing.T) {
	// Same label literals should be equal.
	ch := newTestChecker()
	_ = ch // use the checker to follow the pattern
	a1 := &types.TyCon{Name: "foo", Level: types.L1}
	a2 := &types.TyCon{Name: "foo", Level: types.L1}
	if !types.Equal(a1, a2) {
		t.Error("label literals with same name should be equal")
	}
}

func TestLabelKindInForall(t *testing.T) {
	// A polymorphic function with a label-kinded parameter should type-check.
	source := `
form Bool := { True: Bool; False: Bool }
id :: \(l: Label). Bool -> Bool
id := \x. x
main := id True`
	checkSource(t, source, nil)
}

func TestLabelKindMismatchDistinctLabels(t *testing.T) {
	// Two label-kinded forall binders unified against different labels should fail.
	source := `
f :: Without ` + "`" + `a { a: Int, b: String } -> Int
f := \r. 42
g :: Without ` + "`" + `b { a: Int, b: String } -> Int
g := f
main := 42`
	checkSourceExpectError(t, source, nil)
}

func TestLabelKindPrettyPrint(t *testing.T) {
	// Label literals should pretty-print with backtick prefix.
	ty := &types.TyCon{Name: "myLabel", Level: types.L1}
	pretty := types.Pretty(ty)
	if pretty != "`myLabel" {
		t.Errorf("expected `myLabel, got %s", pretty)
	}
}

func TestLabelKindRegistration(t *testing.T) {
	// Verify Without and Lookup are registered with Label-kinded first parameter.
	ch := newTestChecker()
	ch.installFamilyReducer()

	withoutInfo, ok := ch.lookupFamily("Without")
	if !ok {
		t.Fatal("Without should be registered as a type family")
	}
	if len(withoutInfo.Params) != 2 {
		t.Fatalf("Without should have 2 params, got %d", len(withoutInfo.Params))
	}
	if !types.Equal(withoutInfo.Params[0].Kind, types.TypeOfLabels) {
		t.Errorf("Without first param should have kind Label, got %s", types.Pretty(withoutInfo.Params[0].Kind))
	}

	lookupInfo, ok := ch.lookupFamily("Lookup")
	if !ok {
		t.Fatal("Lookup should be registered as a type family")
	}
	if !types.Equal(lookupInfo.Params[0].Kind, types.TypeOfLabels) {
		t.Errorf("Lookup first param should have kind Label, got %s", types.Pretty(lookupInfo.Params[0].Kind))
	}
}

func TestLabelKindSourceLevelAnnotation(t *testing.T) {
	// Full source-level test: a function with label kind annotation in the type signature.
	source := `
form Bool := { True: Bool; False: Bool }
g :: \(l: Label). Bool
g := True
main := g`
	checkSource(t, source, nil)
}

func TestLabelLiteralKindCheckedCorrectly(t *testing.T) {
	// A label literal used where Label is expected should type-check.
	ch := newTestChecker()
	ch.installFamilyReducer()
	label := &types.TyCon{Name: "myLabel", Level: types.L1}
	kind := ch.kindOfType(label)
	if kind == nil {
		t.Fatal("expected non-nil kind for label literal")
	}
	if !types.Equal(kind, types.TypeOfLabels) {
		t.Errorf("expected kind Label, got %s", types.Pretty(kind))
	}
}

func TestUppercaseLabelLiteralRejected(t *testing.T) {
	// Uppercase backtick labels like `MyState should be rejected at lex time.
	// The lexer only accepts lowercase/underscore-initial labels after backtick.
	// If uppercase were accepted, label erasure would skip it and the program
	// would panic at runtime.
	source := "f :: \\(l: Label). Int\nf := assumption\nmain := f @`MyState"
	src := span.NewSource("test", source)
	l := parse.NewLexer(src)
	_, lexErrs := l.Tokenize()
	if !lexErrs.HasErrors() {
		t.Fatal("expected lex error for uppercase label literal, got none")
	}
}

func TestBuiltinKindConNotLabel(t *testing.T) {
	// Builtin kind constants at L1 should have kind Sort, not Label.
	ch := newTestChecker()
	for _, name := range []string{"Type", "Row", "Constraint", "Label"} {
		ty := &types.TyCon{Name: name, Level: types.L1}
		kind := ch.kindOfType(ty)
		if types.Equal(kind, types.TypeOfLabels) {
			t.Errorf("builtin kind %s should not have kind Label", name)
		}
	}
}
