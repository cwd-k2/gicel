//go:build probe

// Declaration probe tests: data, type alias, impl, forall, fixity, separators.
// Rewritten for unified syntax (DeclForm/DeclTypeAlias/DeclImpl).
// Does NOT cover: lexer (lexer_probe_test.go), crash (parser_crash_probe_test.go).
package parse

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage
)

// ===== From probe_b =====

// TestProbeB_GADTDecl checks GADT-style data declaration with row body.
func TestProbeB_GADTDecl(t *testing.T) {
	src := `form Expr := \a. {
  Lit: Int -> Expr Int;
  Add: Expr Int -> Expr Int -> Expr Int
}`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclForm)
	if d.Name != "Expr" {
		t.Errorf("expected 'Expr', got %q", d.Name)
	}
	// Body is \a. { Lit: ...; Add: ... }
	fa, ok := d.Body.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall body, got %T", d.Body)
	}
	row, ok := fa.Body.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow inside forall, got %T", fa.Body)
	}
	if len(row.Fields) != 2 {
		t.Errorf("expected 2 row fields, got %d", len(row.Fields))
	}
}

// TestProbeB_ForallType checks forall type: \a. a -> a.
func TestProbeB_ForallType(t *testing.T) {
	prog := parseMustSucceed(t, `id :: \a. a -> a`)
	d := prog.Decls[0].(*DeclTypeAnn)
	fa, ok := d.Type.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall, got %T", d.Type)
	}
	if len(fa.Binders) != 1 || fa.Binders[0].Name != "a" {
		t.Errorf("expected binder 'a'")
	}
}

// TestProbeB_ForallKindedBinder checks kinded forall: \(a: Type). a -> a.
func TestProbeB_ForallKindedBinder(t *testing.T) {
	prog := parseMustSucceed(t, `id :: \(a: Type). a -> a`)
	d := prog.Decls[0].(*DeclTypeAnn)
	fa, ok := d.Type.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall, got %T", d.Type)
	}
	if len(fa.Binders) != 1 || fa.Binders[0].Name != "a" {
		t.Errorf("expected binder 'a'")
	}
	if fa.Binders[0].Kind == nil {
		t.Error("expected kinded binder, got nil kind")
	}
}

// TestProbeB_QualifiedConstraint checks qualified type: Eq a => a -> a -> Bool.
func TestProbeB_QualifiedConstraint(t *testing.T) {
	prog := parseMustSucceed(t, "eq :: Eq a => a -> a -> Bool")
	d := prog.Decls[0].(*DeclTypeAnn)
	qual, ok := d.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual, got %T", d.Type)
	}
	_ = qual
}

// TestProbeB_RowTypeWithTail checks { x: Int, y: Bool | r }.
func TestProbeB_RowTypeWithTail(t *testing.T) {
	prog := parseMustSucceed(t, "f :: { x: Int, y: Bool | r } -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr, ok := d.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", d.Type)
	}
	row, ok := arr.From.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", arr.From)
	}
	if len(row.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(row.Fields))
	}
	if row.Tail == nil {
		t.Error("expected row tail")
	}
}

// TestProbeB_EmptyRowType checks {}.
func TestProbeB_EmptyRowType(t *testing.T) {
	prog := parseMustSucceed(t, "f :: {} -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr, ok := d.Type.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow, got %T", d.Type)
	}
	row, ok := arr.From.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", arr.From)
	}
	if len(row.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(row.Fields))
	}
}

// TestProbeB_DataWithSuperclass checks data with superclass constraint
// (unified syntax: form Ord := \a. Eq a => { compare: a -> a -> Int }).
func TestProbeB_DataWithSuperclass(t *testing.T) {
	src := `form Ord := \a. Eq a => { compare: a -> a -> Int }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclForm)
	if d.Name != "Ord" {
		t.Errorf("expected 'Ord', got %q", d.Name)
	}
	fa, ok := d.Body.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall body, got %T", d.Body)
	}
	qual, ok := fa.Body.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual for superclass constraint, got %T", fa.Body)
	}
	row, ok := qual.Body.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", qual.Body)
	}
	if len(row.Fields) != 1 {
		t.Errorf("expected 1 method field, got %d", len(row.Fields))
	}
}

// TestProbeB_ImplWithContext checks impl with context constraint
// (unified syntax: impl Eq a => Eq (Maybe a) := { eq := ... }).
func TestProbeB_ImplWithContext(t *testing.T) {
	src := `form Maybe := \a. { Nothing: (); Just: a }
form Eq := \a. { eq: a -> a -> a }
impl Eq a => Eq (Maybe a) := { eq := \x y. x }`
	prog := parseMustSucceed(t, src)
	for _, d := range prog.Decls {
		if impl, ok := d.(*DeclImpl); ok {
			// Ann should be a qualified type: Eq a => Eq (Maybe a)
			_, isQual := impl.Ann.(*TyExprQual)
			if !isQual {
				t.Errorf("expected TyExprQual for impl Ann, got %T", impl.Ann)
			}
			return
		}
	}
	t.Error("impl declaration not found")
}

// TestProbeB_TypeAnnotationInExpr verifies (42 :: Int) in expression position.
func TestProbeB_TypeAnnotationInExpr(t *testing.T) {
	src := `main := (42 :: Int)`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	paren, ok := d.Expr.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen, got %T", d.Expr)
	}
	ann, ok := paren.Inner.(*ExprAnn)
	if !ok {
		t.Fatalf("expected ExprAnn, got %T", paren.Inner)
	}
	_ = ann
}

// TestProbeB_TypeApplication checks @Type syntax in expressions.
func TestProbeB_TypeApplication(t *testing.T) {
	prog := parseMustSucceed(t, "main := f @Int")
	d := prog.Decls[0].(*DeclValueDef)
	tyApp, ok := d.Expr.(*ExprTyApp)
	if !ok {
		t.Fatalf("expected ExprTyApp, got %T", d.Expr)
	}
	v, ok := tyApp.Expr.(*ExprVar)
	if !ok || v.Name != "f" {
		t.Errorf("expected var 'f', got %T", tyApp.Expr)
	}
}

// TestProbeB_NewlineAsSeparator checks that newline works as separator.
func TestProbeB_NewlineAsSeparator(t *testing.T) {
	src := "x := 1\ny := 2\nz := 3"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_SemicolonAsSeparator checks that semicolon works as separator.
func TestProbeB_SemicolonAsSeparator(t *testing.T) {
	src := "x := 1; y := 2; z := 3"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_MixedNewlineAndSemicolon checks mixed separators.
func TestProbeB_MixedNewlineAndSemicolon(t *testing.T) {
	src := "x := 1\ny := 2; z := 3\nw := 4"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 4 {
		t.Errorf("expected 4 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_TupleTypeDesugaring checks (Int, Bool) type desugars to Record.
func TestProbeB_TupleTypeDesugaring(t *testing.T) {
	prog := parseMustSucceed(t, "f :: (Int, Bool) -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr := d.Type.(*TyExprArrow)
	app, ok := arr.From.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp for tuple type, got %T", arr.From)
	}
	con, ok := app.Fun.(*TyExprCon)
	if !ok || con.Name != "Record" {
		t.Fatalf("expected Record con, got %T %v", app.Fun, app.Fun)
	}
}

// TestProbeB_UnitTypeDesugaring checks () type desugars to Record {}.
func TestProbeB_UnitTypeDesugaring(t *testing.T) {
	prog := parseMustSucceed(t, "f :: () -> Int")
	d := prog.Decls[0].(*DeclTypeAnn)
	arr := d.Type.(*TyExprArrow)
	app, ok := arr.From.(*TyExprApp)
	if !ok {
		t.Fatalf("expected TyExprApp for unit type, got %T", arr.From)
	}
	con, ok := app.Fun.(*TyExprCon)
	if !ok || con.Name != "Record" {
		t.Fatalf("expected Record con, got %T", app.Fun)
	}
}

// TestProbeB_ConstraintTupleDesugaring checks (Eq a, Ord a) => T desugars correctly.
func TestProbeB_ConstraintTupleDesugaring(t *testing.T) {
	src := "f :: (Eq a, Ord a) => a -> a"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclTypeAnn)
	// Should desugar to Eq a => Ord a => a -> a
	qual1, ok := d.Type.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual for outer constraint, got %T", d.Type)
	}
	qual2, ok := qual1.Body.(*TyExprQual)
	if !ok {
		t.Fatalf("expected TyExprQual for inner constraint, got %T", qual1.Body)
	}
	_, ok = qual2.Body.(*TyExprArrow)
	if !ok {
		t.Fatalf("expected TyExprArrow as body, got %T", qual2.Body)
	}
}

// TestProbeB_ImplTupleConstraint checks impl with tuple constraint
// (unified syntax: impl (Eq a, Show a) => MyClass a := { ... }).
func TestProbeB_ImplTupleConstraint(t *testing.T) {
	src := `form Eq := \a. { eq: a -> a }
form Show := \a. { show: a -> a }
form MyClass := \a. { m: a -> a }
impl (Eq a, Show a) => MyClass a := { m := \x. x }`
	prog := parseMustSucceed(t, src)
	for _, d := range prog.Decls {
		if impl, ok := d.(*DeclImpl); ok {
			// Ann should be a qualified type with nested constraints
			qual, isQual := impl.Ann.(*TyExprQual)
			if !isQual {
				t.Errorf("expected TyExprQual for impl Ann, got %T", impl.Ann)
				continue
			}
			// The desugared constraint tuple becomes nested quals
			_, isQual2 := qual.Body.(*TyExprQual)
			if !isQual2 {
				// The body of the outer qual should be another qual (second constraint)
				// or the head type directly
				t.Logf("impl Ann shape: %T -> %T", impl.Ann, qual.Body)
			}
			return
		}
	}
	t.Error("MyClass impl not found")
}

// TestProbeFormSingleConShorthand pins the fix for B8: bare
// `form Name := Con` (single constructor, no pipe, no braces) must be parsed
// as an ADT shorthand registering Con as a zero-arg constructor of Name —
// matching the features.adt grammar `Con (| Con)*`. Previously this was
// parsed as a constructorless nominal type, causing "unknown constructor"
// at the use site. Discovered by field-test doc critic (2026-04-08).
//
// The new rule is: `form Name := BareUpper` (single upper token followed by
// end-of-decl) is ALWAYS single-con shorthand. Users who want a nominal
// wrapper over a concrete type must use the brace form
// `form Name := { MkName: T -> Name }`.
func TestProbeFormSingleConShorthand(t *testing.T) {
	// Different name: form End := MkEnd
	prog := parseMustSucceed(t, "form End := MkEnd")
	d := prog.Decls[0].(*DeclForm)
	if _, ok := d.Body.(*TyExprRow); !ok {
		t.Errorf("form End := MkEnd: expected TyExprRow body (ADT shorthand), got %T", d.Body)
	}
	// Same name: form Foo := Foo (phantom-type pattern, as in phantom.gicel)
	prog = parseMustSucceed(t, "form Foo := Foo")
	d = prog.Decls[0].(*DeclForm)
	if _, ok := d.Body.(*TyExprRow); !ok {
		t.Errorf("form Foo := Foo: expected TyExprRow body (ADT shorthand), got %T", d.Body)
	}
	// Bare Upper RHS (even when name exists elsewhere) is shorthand.
	prog = parseMustSucceed(t, "form Box := Int")
	d = prog.Decls[0].(*DeclForm)
	if _, ok := d.Body.(*TyExprRow); !ok {
		t.Errorf("form Box := Int: expected TyExprRow body (shorthand), got %T", d.Body)
	}
	// Multi-con still works (pipe form).
	prog = parseMustSucceed(t, "form Color := Red | Green | Blue")
	d = prog.Decls[0].(*DeclForm)
	if _, ok := d.Body.(*TyExprRow); !ok {
		t.Errorf("form Color := ...: expected TyExprRow body (multi-con), got %T", d.Body)
	}
	// Type application on RHS must NOT be shorthand (parses as type expression).
	// form Box := Map Int (no shorthand — this is a type, not a constructor).
	prog = parseMustSucceed(t, "form Box := List Int")
	d = prog.Decls[0].(*DeclForm)
	if _, ok := d.Body.(*TyExprApp); !ok {
		t.Errorf("form Box := List Int: expected TyExprApp body, got %T", d.Body)
	}
}

// TestProbeB_DataWithKindedParam checks data with kinded parameter in body.
// Unified syntax: form F := \(a: Type). { MkF: a }.
func TestProbeB_DataWithKindedParam(t *testing.T) {
	src := `form F := \(a: Type). { MkF: a }`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclForm)
	fa, ok := d.Body.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall body, got %T", d.Body)
	}
	if len(fa.Binders) != 1 {
		t.Fatalf("expected 1 binder, got %d", len(fa.Binders))
	}
	if fa.Binders[0].Kind == nil {
		t.Error("expected kinded binder, got nil kind")
	}
}

// ===== From probe_d =====

// TestProbeD_SemicolonTopLevel verifies semicolons as declaration separators.
func TestProbeD_SemicolonTopLevel(t *testing.T) {
	prog := parseMustSucceed(t, "x := 1; y := 2; z := 3")
	if len(prog.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(prog.Decls))
	}
}

// TestProbeD_NewlineTopLevel verifies newlines as declaration separators.
func TestProbeD_NewlineTopLevel(t *testing.T) {
	prog := parseMustSucceed(t, "x := 1\ny := 2\nz := 3")
	if len(prog.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(prog.Decls))
	}
}

// TestProbeD_SemicolonInDataBody verifies semicolons as separators in data row bodies.
func TestProbeD_SemicolonInDataBody(t *testing.T) {
	source := "form MyC := \\a. {\n  f: a -> a;\n  g: a -> a\n}"
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclForm)
	fa, ok := d.Body.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall body, got %T", d.Body)
	}
	row, ok := fa.Body.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", fa.Body)
	}
	if len(row.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(row.Fields))
	}
}

// TestProbeD_SemicolonInImplBody verifies semicolons as separators in impl bodies.
func TestProbeD_SemicolonInImplBody(t *testing.T) {
	source := "form MyC := \\a. { f: a -> a; g: a -> a }\nimpl MyC Int := {\n  f := \\x. x;\n  g := \\x. x\n}"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if _, ok := d.(*DeclImpl); ok {
			return
		}
	}
	t.Error("impl declaration not found")
}

// TestProbeD_MultipleSemicolonsSkipped verifies consecutive semicolons are harmless.
func TestProbeD_MultipleSemicolonsSkipped(t *testing.T) {
	prog := parseMustSucceed(t, ";;;x := 1;;;;y := 2;;;")
	if len(prog.Decls) != 2 {
		t.Errorf("expected 2 decls, got %d", len(prog.Decls))
	}
}

// TestProbeD_FixityPrec0 verifies precedence 0 is handled correctly.
func TestProbeD_FixityPrec0(t *testing.T) {
	source := "infixl 0 $$\n($$) := \\a b. a\nmain := 1 $$ 2 $$ 3"
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	// Prec 0, left-assoc: (1 $$ 2) $$ 3
	for _, d := range prog.Decls {
		if vd, ok := d.(*DeclValueDef); ok && vd.Name == "main" {
			inf, ok := vd.Expr.(*ExprInfix)
			if !ok {
				t.Fatalf("expected ExprInfix, got %T", vd.Expr)
			}
			// Left-associative: left should also be ExprInfix
			if _, ok := inf.Left.(*ExprInfix); !ok {
				t.Errorf("expected left-associative grouping: (1 $$ 2) $$ 3, left is %T", inf.Left)
			}
		}
	}
}

// TestProbeD_FixityPrecMax verifies high precedence (e.g., 20) works.
func TestProbeD_FixityPrecMax(t *testing.T) {
	source := "infixl 20 ##\n(##) := \\a b. a\nmain := 1 ## 2"
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	for _, d := range prog.Decls {
		if vd, ok := d.(*DeclValueDef); ok && vd.Name == "main" {
			_, ok := vd.Expr.(*ExprInfix)
			if !ok {
				t.Fatalf("expected ExprInfix for high-prec operator, got %T", vd.Expr)
			}
		}
	}
}

// TestProbeD_FixityNoneConflict verifies that two infixn operators of same precedence
// applied without grouping produce an error or well-defined behavior.
// BUG: low — infixn (non-associative) operators at the same precedence should reject
// `a == b /= c` because neither can associate with the other.
func TestProbeD_FixityNoneConflict(t *testing.T) {
	source := "infixn 5 ==\ninfixn 5 /=\n(==) := \\a b. a\n(/=) := \\a b. b\nmain := 1 == 2 /= 3"
	prog, es := parse(source)
	// Non-associative operators at same precedence should ideally error.
	if es.HasErrors() {
		t.Logf("infixn conflict produced error (expected): %s", es.Format())
		return
	}
	for _, d := range prog.Decls {
		if vd, ok := d.(*DeclValueDef); ok && vd.Name == "main" {
			// BUG: low — silently left-groups instead of rejecting
			t.Logf("BUG: low — infixn conflict silently parsed as %T (left-grouped)", vd.Expr)
		}
	}
}

// TestProbeD_FixityNegativePrec verifies negative precedence does not crash.
func TestProbeD_FixityNegativePrec(t *testing.T) {
	// The parser uses strconv.Atoi, so this should just parse as the
	// operator `-` followed by literal `1`, not as a negative precedence.
	source := "infixl -1 ##"
	_, es := parse(source)
	// Document behavior — might error or misparse.
	_ = es
}

// TestProbeD_DataEmptyRowBody verifies `form T := {}` does not crash.
func TestProbeD_DataEmptyRowBody(t *testing.T) {
	prog, es := parse("form T := {}")
	if es.HasErrors() {
		t.Logf("empty body errors: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if dd, ok := d.(*DeclForm); ok && dd.Name == "T" {
				row, ok := dd.Body.(*TyExprRow)
				if !ok {
					t.Logf("body is %T, not TyExprRow", dd.Body)
				} else if len(row.Fields) != 0 {
					t.Errorf("expected 0 fields for empty body, got %d", len(row.Fields))
				}
			}
		}
	}
}

// TestProbeD_DataRowMissingColon verifies a row field without `:` is handled.
func TestProbeD_DataRowMissingColon(t *testing.T) {
	source := "form T := { MkT Int }"
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for row field without `:`")
	}
}

// TestProbeD_DataSingleRowField verifies a single row field parses correctly.
func TestProbeD_DataSingleRowField(t *testing.T) {
	source := "form T := { MkT: Int -> T }"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclForm); ok && dd.Name == "T" {
			row, ok := dd.Body.(*TyExprRow)
			if !ok {
				t.Fatalf("expected TyExprRow body, got %T", dd.Body)
			}
			if len(row.Fields) != 1 {
				t.Errorf("expected 1 field, got %d", len(row.Fields))
			}
			if row.Fields[0].Label != "MkT" {
				t.Errorf("expected label MkT, got %s", row.Fields[0].Label)
			}
		}
	}
}

// TestProbeD_DataMultipleRowFieldsSemicolon verifies multiple row fields separated by semicolons.
func TestProbeD_DataMultipleRowFieldsSemicolon(t *testing.T) {
	source := "form T := { A: Int -> T; B: T }"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclForm); ok && dd.Name == "T" {
			row, ok := dd.Body.(*TyExprRow)
			if !ok {
				t.Fatalf("expected TyExprRow body, got %T", dd.Body)
			}
			if len(row.Fields) != 2 {
				t.Errorf("expected 2 fields, got %d", len(row.Fields))
			}
		}
	}
}

// TestProbeD_DataMultipleRowFieldsComma verifies row fields separated by commas.
func TestProbeD_DataMultipleRowFieldsComma(t *testing.T) {
	source := "form T := { A: Int -> T, B: T }"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclForm); ok && dd.Name == "T" {
			row, ok := dd.Body.(*TyExprRow)
			if !ok {
				t.Fatalf("expected TyExprRow body, got %T", dd.Body)
			}
			if len(row.Fields) != 2 {
				t.Errorf("expected 2 fields, got %d", len(row.Fields))
			}
		}
	}
}

// TestProbeD_TypeAliasEmptyCaseBody verifies `type F :: Type := \a. case a {}`.
func TestProbeD_TypeAliasEmptyCaseBody(t *testing.T) {
	source := `type F :: Type -> Type := \a. case a {}`
	prog, es := parse(source)
	if es.HasErrors() {
		t.Logf("empty type case body: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if ta, ok := d.(*DeclTypeAlias); ok && ta.Name == "F" {
				t.Logf("parsed type alias F with body %T", ta.Body)
			}
		}
	}
}

// TestProbeD_DataNoMethods checks `form C := \a. {}` (empty row body).
func TestProbeD_DataNoMethods(t *testing.T) {
	prog := parseMustSucceed(t, "form C := \\a. {}")
	d := prog.Decls[0].(*DeclForm)
	if d.Name != "C" {
		t.Errorf("expected 'C', got %q", d.Name)
	}
	fa, ok := d.Body.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall body, got %T", d.Body)
	}
	row, ok := fa.Body.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", fa.Body)
	}
	if len(row.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(row.Fields))
	}
}

// TestProbeD_ImplEmptyBody verifies `impl C Int := {}` (empty record body).
func TestProbeD_ImplEmptyBody(t *testing.T) {
	prog := parseMustSucceed(t, "impl C Int := {}")
	for _, d := range prog.Decls {
		if _, ok := d.(*DeclImpl); ok {
			return
		}
	}
	t.Error("impl declaration not found")
}

// TestProbeD_DataMultipleSuperclassChain verifies chained superclass constraints.
func TestProbeD_DataMultipleSuperclassChain(t *testing.T) {
	source := "form MyClass := \\a. Eq a => Ord a => { m: a -> a }"
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	d := prog.Decls[0].(*DeclForm)
	fa, ok := d.Body.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall body, got %T", d.Body)
	}
	qual1, ok := fa.Body.(*TyExprQual)
	if !ok {
		t.Fatalf("expected first TyExprQual, got %T", fa.Body)
	}
	qual2, ok := qual1.Body.(*TyExprQual)
	if !ok {
		t.Fatalf("expected second TyExprQual, got %T", qual1.Body)
	}
	_, ok = qual2.Body.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow after constraints, got %T", qual2.Body)
	}
}

// TestProbeD_ImplParenthesizedConstraints verifies `impl (Eq a, Ord a) => C a := {}`.
func TestProbeD_ImplParenthesizedConstraints(t *testing.T) {
	source := "impl (Eq a, Ord a) => C a := {}"
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	for _, d := range prog.Decls {
		if impl, ok := d.(*DeclImpl); ok {
			_, isQual := impl.Ann.(*TyExprQual)
			if !isQual {
				t.Errorf("expected TyExprQual for impl Ann, got %T", impl.Ann)
			}
			return
		}
	}
	t.Error("impl declaration not found")
}

// TestProbeD_DataNoBody verifies `form C := \a.` with no body after forall dot.
func TestProbeD_DataNoBody(t *testing.T) {
	_, es := parse("form C := \\a.")
	if !es.HasErrors() {
		t.Error("expected error for data with no body after forall")
	}
}

// TestProbeD_ImplNoBody verifies `impl C Int` with no := and no body.
func TestProbeD_ImplNoBody(t *testing.T) {
	_, es := parse("impl C Int")
	if !es.HasErrors() {
		t.Error("expected error for impl with no := and no body")
	}
}

// TestProbeD_RowTypeEmpty verifies `{}` in type position is empty row.
func TestProbeD_RowTypeEmpty(t *testing.T) {
	source := "x :: {}\nx := 42"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if ta, ok := d.(*DeclTypeAnn); ok && ta.Name == "x" {
			row, ok := ta.Type.(*TyExprRow)
			if !ok {
				t.Fatalf("expected TyExprRow for {}, got %T", ta.Type)
			}
			if len(row.Fields) != 0 {
				t.Errorf("expected 0 fields in empty row, got %d", len(row.Fields))
			}
		}
	}
}

// TestProbeD_RowTypeWithTail verifies `{ x: Int | r }`.
func TestProbeD_RowTypeWithTail(t *testing.T) {
	source := "x :: { x: Int | r }\nx := 42"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if ta, ok := d.(*DeclTypeAnn); ok && ta.Name == "x" {
			row, ok := ta.Type.(*TyExprRow)
			if !ok {
				t.Fatalf("expected TyExprRow, got %T", ta.Type)
			}
			if row.Tail == nil {
				t.Error("expected non-nil row tail")
			} else if v, ok := row.Tail.(*TyExprVar); !ok {
				t.Errorf("expected *TyExprVar tail, got %T", row.Tail)
			} else if v.Name != "r" {
				t.Errorf("expected tail name 'r', got %q", v.Name)
			}
		}
	}
}

// TestProbeD_ForallType verifies `\ a. a -> a` in type position.
func TestProbeD_ForallType(t *testing.T) {
	source := "id :: \\ a. a -> a\nid := \\x. x"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if ta, ok := d.(*DeclTypeAnn); ok && ta.Name == "id" {
			fa, ok := ta.Type.(*TyExprForall)
			if !ok {
				t.Fatalf("expected TyExprForall, got %T", ta.Type)
			}
			if len(fa.Binders) != 1 {
				t.Errorf("expected 1 binder, got %d", len(fa.Binders))
			}
		}
	}
}

// TestProbeD_DataNoConstructors verifies `form T :=` with nothing after `:=`.
func TestProbeD_DataNoConstructors(t *testing.T) {
	_, es := parse("form T :=")
	if !es.HasErrors() {
		t.Error("expected error for data with no body after :=")
	}
}

// TestProbeD_DataManyConstructors verifies a data type with many ADT constructors.
func TestProbeD_DataManyConstructors(t *testing.T) {
	var b strings.Builder
	b.WriteString("form T := C0")
	for i := 1; i < 100; i++ {
		b.WriteString(fmt.Sprintf(" | C%d", i))
	}
	prog := parseMustSucceed(t, b.String())
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclForm); ok && dd.Name == "T" {
			// ADT shorthand desugars to a TyExprRow with one field per constructor
			row, ok := dd.Body.(*TyExprRow)
			if !ok {
				t.Fatalf("expected TyExprRow body (ADT shorthand), got %T", dd.Body)
			}
			if len(row.Fields) != 100 {
				t.Errorf("expected 100 constructors, got %d", len(row.Fields))
			}
		}
	}
}

// ===== From probe_e =====

// TestProbeE_DataDeclNoConstructors verifies that `form T :=` with nothing after
// the `:=` either errors or produces an empty constructor list.
func TestProbeE_DataDeclNoConstructors(t *testing.T) {
	_, es := parse("form T :=")
	if !es.HasErrors() {
		t.Error("expected error for data decl with no body")
	}
}

// TestProbeE_DataDeclTrailingPipe verifies `form T := A |` with trailing pipe.
func TestProbeE_DataDeclTrailingPipe(t *testing.T) {
	_, es := parse("form T := A |")
	if !es.HasErrors() {
		t.Error("expected error for trailing pipe in data decl")
	}
}

// TestProbeE_DataDeclEmptyBody verifies `form C := \a. {}` parses (empty row).
func TestProbeE_DataDeclEmptyBody(t *testing.T) {
	prog := parseMustSucceed(t, "form C := \\a. {}")
	found := false
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclForm); ok && dd.Name == "C" {
			found = true
			fa, ok := dd.Body.(*TyExprForall)
			if !ok {
				t.Fatalf("expected TyExprForall body, got %T", dd.Body)
			}
			row, ok := fa.Body.(*TyExprRow)
			if !ok {
				t.Fatalf("expected TyExprRow, got %T", fa.Body)
			}
			if len(row.Fields) != 0 {
				t.Errorf("expected 0 fields, got %d", len(row.Fields))
			}
		}
	}
	if !found {
		t.Error("form C not found")
	}
}

// TestProbeE_DataDeclNoBody verifies `form C :=` with no body at all.
func TestProbeE_DataDeclNoBody(t *testing.T) {
	_, es := parse("form C :=")
	if !es.HasErrors() {
		t.Error("expected error for data without body")
	}
}

// TestProbeE_ImplDeclEmptyBody verifies `impl C Int := {}` parses.
func TestProbeE_ImplDeclEmptyBody(t *testing.T) {
	prog := parseMustSucceed(t, "impl C Int := {}")
	_ = prog
}

// TestProbeE_FixityDeclNoOp verifies `infixl 6` with no operator name.
func TestProbeE_FixityDeclNoOp(t *testing.T) {
	_, es := parse("infixl 6")
	if !es.HasErrors() {
		t.Error("expected error for fixity with no operator")
	}
}

// TestProbeE_FixityDeclNoPrec verifies `infixl +` without precedence uses default.
func TestProbeE_FixityDeclNoPrec(t *testing.T) {
	prog := parseMustSucceed(t, "infixl +")
	for _, d := range prog.Decls {
		if fix, ok := d.(*DeclFixity); ok && fix.Op == "+" {
			// Default prec is 9 (from parseFixityDecl when IntLit is missing).
			if fix.Prec != 9 {
				t.Errorf("expected default prec 9, got %d", fix.Prec)
			}
		}
	}
}

// TestProbeE_DuplicateDecls verifies duplicate top-level bindings are accepted
// by the parser (the checker will reject them).
func TestProbeE_DuplicateDecls(t *testing.T) {
	source := `x := 1
x := 2`
	prog := parseMustSucceed(t, source)
	if len(prog.Decls) != 2 {
		t.Errorf("expected 2 decls, got %d", len(prog.Decls))
	}
}

// TestProbeE_TypeAnnWithoutValue verifies type annotation without value definition.
func TestProbeE_TypeAnnWithoutValue(t *testing.T) {
	source := `x :: Int`
	parseMustSucceed(t, source)
}

// TestProbeE_NamedDeclExpectedColonColon verifies `x = 1` (should be `:=`).
func TestProbeE_NamedDeclExpectedColonColon(t *testing.T) {
	_, es := parse("x = 1")
	if !es.HasErrors() {
		t.Error("expected error for '=' instead of ':=' in decl")
	}
}

// TestProbeE_DataRowBasic verifies GADT-style row body syntax.
func TestProbeE_DataRowBasic(t *testing.T) {
	source := `form T := \a. {
  MkT: Int -> T Int;
  MkU: T Bool
}`
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclForm); ok && dd.Name == "T" {
			fa, ok := dd.Body.(*TyExprForall)
			if !ok {
				t.Fatalf("expected TyExprForall body, got %T", dd.Body)
			}
			row, ok := fa.Body.(*TyExprRow)
			if !ok {
				t.Fatalf("expected TyExprRow, got %T", fa.Body)
			}
			if len(row.Fields) != 2 {
				t.Errorf("expected 2 row fields, got %d", len(row.Fields))
			}
		}
	}
}

// TestProbeE_DataEmptyRow verifies `form T := {}` — empty row body.
func TestProbeE_DataEmptyRow(t *testing.T) {
	prog, es := parse("form T := {}")
	if es.HasErrors() {
		t.Logf("empty row body errors: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if dd, ok := d.(*DeclForm); ok && dd.Name == "T" {
				row, ok := dd.Body.(*TyExprRow)
				if !ok {
					t.Logf("body is %T, not TyExprRow", dd.Body)
				} else if len(row.Fields) != 0 {
					t.Errorf("expected 0 fields, got %d", len(row.Fields))
				}
			}
		}
	}
}

// TestProbeE_TypeAnnotationInExpr verifies `expr :: type` in expression position.
func TestProbeE_TypeAnnotationInExpr(t *testing.T) {
	source := `main := (42 :: Int)`
	parseMustSucceed(t, source)
}

// TestProbeE_TypeAnnotationForall verifies forall type syntax.
func TestProbeE_TypeAnnotationForall(t *testing.T) {
	source := `id :: \a. a -> a
id := \x. x`
	parseMustSucceed(t, source)
}

// TestProbeE_RowTypeEmpty verifies {} in type position.
func TestProbeE_RowTypeEmpty(t *testing.T) {
	source := `x :: {}
x := 1`
	parseMustSucceed(t, source)
}

// TestProbeE_RowTypeWithTail verifies { x: Int | r } syntax.
func TestProbeE_RowTypeWithTail(t *testing.T) {
	source := `x :: { x: Int | r }
x := 1`
	parseMustSucceed(t, source)
}

// TestProbeE_RowTypeNoFields verifies { | r } syntax.
func TestProbeE_RowTypeNoFields(t *testing.T) {
	source := `x :: { | r }
x := 1`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclTypeAnn)
	row, ok := d.Type.(*TyExprRow)
	if !ok {
		t.Fatalf("expected TyExprRow, got %T", d.Type)
	}
	if row.Tail == nil {
		t.Error("expected row tail, got nil")
	}
	if len(row.Fields) != 0 {
		t.Errorf("expected 0 fields, got %d", len(row.Fields))
	}
}

// TestProbeE_DataWithSuperclass verifies data with superclass constraint.
func TestProbeE_DataWithSuperclass(t *testing.T) {
	source := `form Ord := \a. Eq a => { compare: a -> a -> a }`
	parseMustSucceed(t, source)
}

// TestProbeE_DataWithMultipleSuperclasses verifies curried superclasses.
func TestProbeE_DataWithMultipleSuperclasses(t *testing.T) {
	source := `form MyClass := \a. Eq a => Ord a => { m: a -> a }`
	parseMustSucceed(t, source)
}

// TestProbeE_DataWithTupleSuperclass verifies tuple-style superclass.
func TestProbeE_DataWithTupleSuperclass(t *testing.T) {
	source := `form MyClass := \a. (Eq a, Ord a) => { m: a -> a }`
	parseMustSucceed(t, source)
}

// TestProbeE_ImplWithContext verifies impl with context constraint.
func TestProbeE_ImplWithContext(t *testing.T) {
	source := `form Maybe := \a. { Nothing: (); Just: a }
form Eq := \a. { eq: a -> a -> a }
impl Eq a => Eq (Maybe a) := { eq := \x y. x }`
	parseMustSucceed(t, source)
}

// TestProbeE_DataBodyWithKeyword verifies data body with just a keyword errors.
func TestProbeE_DataBodyWithKeyword(t *testing.T) {
	_, es := parse(`form C := \a. { import }`)
	if !es.HasErrors() {
		t.Error("expected error for keyword in data body")
	}
}

// TestProbeE_ImplMethodMissingColonEq verifies impl method without := errors.
func TestProbeE_ImplMethodMissingColonEq(t *testing.T) {
	source := `form C := \a. { m: a -> a }
impl C Int := { m = \x. x }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for '=' instead of ':=' in impl method")
	}
}

// TestProbeE_TypeApplicationBasic verifies expr @Type syntax.
func TestProbeE_TypeApplicationBasic(t *testing.T) {
	source := `main := f @Int`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	tyApp, ok := d.Expr.(*ExprTyApp)
	if !ok {
		t.Fatalf("expected ExprTyApp, got %T", d.Expr)
	}
	_ = tyApp
}

// TestProbeE_TypeApplicationChain verifies expr @A @B syntax.
func TestProbeE_TypeApplicationChain(t *testing.T) {
	source := `main := f @Int @Bool`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	outer, ok := d.Expr.(*ExprTyApp)
	if !ok {
		t.Fatalf("expected outer ExprTyApp, got %T", d.Expr)
	}
	_, ok = outer.Expr.(*ExprTyApp)
	if !ok {
		t.Fatalf("expected inner ExprTyApp, got %T", outer.Expr)
	}
}

// TestProbeE_DeclBoundaryNewline verifies that newline-separated declarations
// at the top level are correctly split.
func TestProbeE_DeclBoundaryNewline(t *testing.T) {
	source := `x := 1
y := 2`
	prog := parseMustSucceed(t, source)
	if len(prog.Decls) != 2 {
		t.Errorf("expected 2 decls, got %d", len(prog.Decls))
	}
}

// TestProbeE_DeclBoundarySemicolon verifies semicolons separate declarations.
func TestProbeE_DeclBoundarySemicolon(t *testing.T) {
	source := `x := 1; y := 2`
	prog := parseMustSucceed(t, source)
	if len(prog.Decls) != 2 {
		t.Errorf("expected 2 decls, got %d", len(prog.Decls))
	}
}

// TestProbeE_DeclBoundaryInNestedContext verifies semicolons separate case alts
// inside braces.
func TestProbeE_DeclBoundaryInNestedContext(t *testing.T) {
	source := `main := case x {
  Nothing => 0;
  Just y => y
}`
	parseMustSucceed(t, source)
}

// TestProbeE_SemicolonBetweenImports verifies semicolons between imports.
func TestProbeE_SemicolonBetweenImports(t *testing.T) {
	source := `import A; import B; main := 1`
	prog := parseMustSucceed(t, source)
	if len(prog.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(prog.Imports))
	}
}

// TestProbeE_LeadingSemicolons verifies leading semicolons are skipped.
func TestProbeE_LeadingSemicolons(t *testing.T) {
	source := `;;;main := 1;;;`
	prog := parseMustSucceed(t, source)
	if len(prog.Decls) != 1 {
		t.Errorf("expected 1 decl, got %d", len(prog.Decls))
	}
}

// TestProbeE_TypeAliasWithCase verifies type alias with case body (type family replacement).
func TestProbeE_TypeAliasWithCase(t *testing.T) {
	source := `form Unit := Unit
type F :: Type -> Type := \a. case a {
  Unit => Unit
}`
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if ta, ok := d.(*DeclTypeAlias); ok && ta.Name == "F" {
			fa, ok := ta.Body.(*TyExprForall)
			if !ok {
				t.Fatalf("expected TyExprForall body, got %T", ta.Body)
			}
			cs, ok := fa.Body.(*TyExprCase)
			if !ok {
				t.Fatalf("expected TyExprCase, got %T", fa.Body)
			}
			if len(cs.Alts) != 1 {
				t.Errorf("expected 1 alt, got %d", len(cs.Alts))
			}
		}
	}
}

// TestProbeE_ForallNoBinders verifies `\. T` in type position.
func TestProbeE_ForallNoBinders(t *testing.T) {
	source := `x :: \. Int
x := 1`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclTypeAnn)
	forall, ok := d.Type.(*TyExprForall)
	if !ok {
		t.Fatalf("expected TyExprForall, got %T", d.Type)
	}
	if len(forall.Binders) != 0 {
		t.Errorf("expected 0 binders, got %d", len(forall.Binders))
	}
}

// TestProbeE_ImplBodyStall verifies that garbage in impl body
// doesn't cause infinite loop (the p.pos == before guard should catch it).
func TestProbeE_ImplBodyStall(t *testing.T) {
	source := `form C := \a. { m: a }
impl C Int := { + + + }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for garbage in impl body")
	}
}

// TestProbeE_DataBodyStall verifies that garbage in data body row
// doesn't cause infinite loop.
func TestProbeE_DataBodyStall(t *testing.T) {
	source := `form C := \a. { + + + }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for garbage in data body")
	}
}

// TestProbeE_ParenWithAnnotation verifies (expr :: type) inside parens.
func TestProbeE_ParenWithAnnotation(t *testing.T) {
	source := `main := (42 :: Int)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	paren, ok := d.Expr.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen, got %T", d.Expr)
	}
	ann, ok := paren.Inner.(*ExprAnn)
	if !ok {
		t.Fatalf("expected ExprAnn inside parens, got %T", paren.Inner)
	}
	_ = ann
}

// TestProbeE_AllKeywordsInOneSource verifies a source using all declaration keywords.
func TestProbeE_AllKeywordsInOneSource(t *testing.T) {
	source := `import Prelude
form Bool := True | False
type MyBool := Bool
form MyClass := \a. { m: a -> a }
impl MyClass Bool := { m := \x. x }
infixl 6 +
infixr 5 ++
infixn 4 ==
main := 1`
	parseMustSucceed(t, source)
}
