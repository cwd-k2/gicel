//go:build probe

// Declaration probe tests: GADT, type families, class, instance, data, forall, fixity, separators.
package parse

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage
)

// ===== From probe_b =====

// TestProbeB_GADTDecl checks GADT-style data declaration.
func TestProbeB_GADTDecl(t *testing.T) {
	src := `data Expr a := {
  Lit :: Int -> Expr Int;
  Add :: Expr Int -> Expr Int -> Expr Int
}`
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclData)
	if d.Name != "Expr" {
		t.Errorf("expected 'Expr', got %q", d.Name)
	}
	if len(d.GADTCons) != 2 {
		t.Errorf("expected 2 GADT cons, got %d", len(d.GADTCons))
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

// TestProbeB_ClassWithSuperclass checks class with superclass: class Eq a => Ord a { ... }.
func TestProbeB_ClassWithSuperclass(t *testing.T) {
	src := `class Eq a => Ord a {
  compare :: a -> a -> Int
}`
	prog := parseMustSucceed(t, src)
	cl := prog.Decls[0].(*DeclClass)
	if cl.Name != "Ord" {
		t.Errorf("expected class 'Ord', got %q", cl.Name)
	}
	if len(cl.Supers) != 1 {
		t.Errorf("expected 1 superclass, got %d", len(cl.Supers))
	}
	if len(cl.Methods) != 1 {
		t.Errorf("expected 1 method, got %d", len(cl.Methods))
	}
}

// TestProbeB_InstanceWithContext checks instance with context.
func TestProbeB_InstanceWithContext(t *testing.T) {
	src := `data Maybe a := Nothing | Just a
class Eq a { eq :: a -> a -> Bool }
instance Eq a => Eq (Maybe a) {
  eq := \x y. True
}`
	prog := parseMustSucceed(t, src)
	// Find instance decl
	for _, d := range prog.Decls {
		if inst, ok := d.(*DeclInstance); ok {
			if inst.ClassName != "Eq" {
				t.Errorf("expected class 'Eq', got %q", inst.ClassName)
			}
			if len(inst.Context) != 1 {
				t.Errorf("expected 1 context constraint, got %d", len(inst.Context))
			}
			return
		}
	}
	t.Error("instance declaration not found")
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

// TestProbeB_InstanceTupleConstraint checks instance (Eq a, Show a) => MyClass a.
func TestProbeB_InstanceTupleConstraint(t *testing.T) {
	src := `class Eq a { eq :: a -> a }
class Show a { show :: a -> a }
class MyClass a { m :: a -> a }
instance (Eq a, Show a) => MyClass a {
  m := \x. x
}`
	prog := parseMustSucceed(t, src)
	for _, d := range prog.Decls {
		if inst, ok := d.(*DeclInstance); ok {
			if inst.ClassName != "MyClass" {
				continue
			}
			if len(inst.Context) != 2 {
				t.Errorf("expected 2 context constraints, got %d", len(inst.Context))
			}
			return
		}
	}
	t.Error("MyClass instance not found")
}

// TestProbeB_DataWithKindedParam checks data F (a: Type) := MkF a.
func TestProbeB_DataWithKindedParam(t *testing.T) {
	src := "data F (a: Type) := MkF a"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclData)
	if len(d.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(d.Params))
	}
	if d.Params[0].Kind == nil {
		t.Error("expected kinded parameter, got nil kind")
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

// TestProbeD_NewlineInClassBody verifies newlines as separators in class bodies.
// BUG: low — same root cause as do-block newline issue. After parsing `f :: a -> a`,
// the type parser's parseTypeArrow sees `g` as a type atom (TyExprVar), so the method
// type becomes `a -> a -> g` instead of `a -> a`. The second method `g` is partially
// consumed. Recovery succeeds (2 methods parsed) but with a spurious error.
func TestProbeD_NewlineInClassBody(t *testing.T) {
	source := "class MyC a {\n  f :: a -> a\n  g :: a -> a\n}"
	prog, es := parse(source)
	if es.HasErrors() {
		// BUG: low — class body newline separation produces errors
		t.Logf("BUG: low — class body newline separation produces errors: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if cl, ok := d.(*DeclClass); ok {
				t.Logf("class %s has %d methods", cl.Name, len(cl.Methods))
			}
		}
	}
}

// TestProbeD_NewlineInInstanceBody verifies newlines as separators in instance bodies.
// BUG: low — same root cause as do-block/class newline issue. After parsing `f := \x. x`,
// the expression parser sees `g` on the next line as a continuation atom and tries to
// apply it. Recovery succeeds but with spurious errors.
func TestProbeD_NewlineInInstanceBody(t *testing.T) {
	source := "instance MyC Int {\n  f := \\x. x\n  g := \\x. x\n}"
	prog, es := parse(source)
	if es.HasErrors() {
		// BUG: low — instance body newline separation produces errors
		t.Logf("BUG: low — instance body newline separation produces errors: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if inst, ok := d.(*DeclInstance); ok {
				t.Logf("instance has %d methods", len(inst.Methods))
			}
		}
	}
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

// TestProbeD_GADTEmptyBody verifies `data T := {}` does not crash.
func TestProbeD_GADTEmptyBody(t *testing.T) {
	prog, es := parse("data T := {}")
	if es.HasErrors() {
		t.Logf("GADT empty body errors: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if dd, ok := d.(*DeclData); ok && dd.Name == "T" {
				if len(dd.GADTCons) != 0 {
					t.Errorf("expected 0 GADT constructors for empty body, got %d", len(dd.GADTCons))
				}
			}
		}
	}
}

// TestProbeD_GADTMissingColonColon verifies GADT constructor without `::` is handled.
func TestProbeD_GADTMissingColonColon(t *testing.T) {
	source := "data T := { MkT Int }"
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for GADT constructor without `::`")
	}
}

// TestProbeD_GADTSingleConstructor verifies a single GADT constructor parses correctly.
func TestProbeD_GADTSingleConstructor(t *testing.T) {
	source := "data T := { MkT :: Int -> T }"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclData); ok && dd.Name == "T" {
			if len(dd.GADTCons) != 1 {
				t.Errorf("expected 1 GADT constructor, got %d", len(dd.GADTCons))
			}
			if dd.GADTCons[0].Name != "MkT" {
				t.Errorf("expected constructor MkT, got %s", dd.GADTCons[0].Name)
			}
		}
	}
}

// TestProbeD_GADTMultipleSemicolon verifies multiple GADT constructors separated by semicolons.
func TestProbeD_GADTMultipleSemicolon(t *testing.T) {
	source := "data T := { A :: Int -> T; B :: T }"
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclData); ok && dd.Name == "T" {
			if len(dd.GADTCons) != 2 {
				t.Errorf("expected 2 GADT constructors, got %d", len(dd.GADTCons))
			}
		}
	}
}

// TestProbeD_GADTMultipleNewline verifies GADT constructors separated by newlines.
// BUG: low — same newline-as-separator issue.
func TestProbeD_GADTMultipleNewline(t *testing.T) {
	source := "data T := {\n  A :: Int -> T\n  B :: T\n}"
	prog, es := parse(source)
	if es.HasErrors() {
		// BUG: low — GADT newline separation produces errors
		t.Logf("BUG: low — GADT newline separation produces errors: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if dd, ok := d.(*DeclData); ok && dd.Name == "T" {
				t.Logf("GADT with newlines: %d constructors parsed", len(dd.GADTCons))
			}
		}
	}
}

// TestProbeD_TypeFamilyEmptyEquationBlock verifies empty `type F :: Type := {}`.
func TestProbeD_TypeFamilyEmptyEquationBlock(t *testing.T) {
	source := "type F :: Type := {}"
	prog, es := parse(source)
	if es.HasErrors() {
		t.Logf("empty type family equations: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if tf, ok := d.(*DeclTypeFamily); ok && tf.Name == "F" {
				if len(tf.Equations) != 0 {
					t.Errorf("expected 0 equations for empty body, got %d", len(tf.Equations))
				}
			}
		}
	}
}

// TestProbeD_TypeFamilyWrongName verifies that an equation with a different name
// than the family is accepted by the parser (semantic check responsibility).
func TestProbeD_TypeFamilyWrongName(t *testing.T) {
	source := `data Unit := Unit
type F (a: Type) :: Type := {
  G a =: Unit
}`
	// The parser doesn't validate that equation names match the family name.
	prog, es := parse(source)
	if es.HasErrors() {
		t.Logf("wrong name in equation: %s", es.Format())
		return
	}
	for _, d := range prog.Decls {
		if tf, ok := d.(*DeclTypeFamily); ok && tf.Name == "F" {
			if len(tf.Equations) > 0 && tf.Equations[0].Name == "G" {
				t.Log("parser accepted equation with name 'G' in family 'F' (semantic check responsibility)")
			}
		}
	}
}

// TestProbeD_TypeFamilyDuplicateEquations verifies duplicate equations don't crash.
func TestProbeD_TypeFamilyDuplicateEquations(t *testing.T) {
	source := `data Unit := Unit
type F (a: Type) :: Type := {
  F Unit =: Unit;
  F Unit =: Unit
}`
	prog, es := parse(source)
	if es.HasErrors() {
		t.Logf("duplicate equations: %s", es.Format())
		return
	}
	for _, d := range prog.Decls {
		if tf, ok := d.(*DeclTypeFamily); ok && tf.Name == "F" {
			if len(tf.Equations) != 2 {
				t.Errorf("expected 2 (duplicate) equations, got %d", len(tf.Equations))
			}
		}
	}
}

// TestProbeD_ClassNoMethods verifies `class C a {}` (empty body).
func TestProbeD_ClassNoMethods(t *testing.T) {
	prog := parseMustSucceed(t, "class C a {}")
	for _, d := range prog.Decls {
		if cl, ok := d.(*DeclClass); ok && cl.Name == "C" {
			if len(cl.Methods) != 0 {
				t.Errorf("expected 0 methods, got %d", len(cl.Methods))
			}
		}
	}
}

// TestProbeD_InstanceEmptyBody verifies `instance C Int {}` (empty body).
func TestProbeD_InstanceEmptyBody(t *testing.T) {
	prog := parseMustSucceed(t, "instance C Int {}")
	for _, d := range prog.Decls {
		if inst, ok := d.(*DeclInstance); ok && inst.ClassName == "C" {
			if len(inst.Methods) != 0 {
				t.Errorf("expected 0 methods, got %d", len(inst.Methods))
			}
		}
	}
}

// TestProbeD_MultipleSuperclassChain verifies chained superclass constraints.
func TestProbeD_MultipleSuperclassChain(t *testing.T) {
	source := "class Eq a => Ord a => MyClass a {}"
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	for _, d := range prog.Decls {
		if cl, ok := d.(*DeclClass); ok && cl.Name == "MyClass" {
			if len(cl.Supers) != 2 {
				t.Errorf("expected 2 superclass constraints, got %d", len(cl.Supers))
			}
		}
	}
}

// TestProbeD_InstanceParenthesizedConstraints verifies `instance (Eq a, Ord a) => C a {}`.
func TestProbeD_InstanceParenthesizedConstraints(t *testing.T) {
	source := "instance (Eq a, Ord a) => C a {}"
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	for _, d := range prog.Decls {
		if inst, ok := d.(*DeclInstance); ok && inst.ClassName == "C" {
			if len(inst.Context) != 2 {
				t.Errorf("expected 2 context constraints, got %d", len(inst.Context))
			}
		}
	}
}

// TestProbeD_ClassNoBody verifies `class C a` with no braces at all.
func TestProbeD_ClassNoBody(t *testing.T) {
	_, es := parse("class C a")
	if !es.HasErrors() {
		t.Error("expected error for class with no body braces")
	}
}

// TestProbeD_InstanceNoBody verifies `instance C Int` with no braces.
func TestProbeD_InstanceNoBody(t *testing.T) {
	_, es := parse("instance C Int")
	if !es.HasErrors() {
		t.Error("expected error for instance with no body braces")
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
			} else if row.Tail.Name != "r" {
				t.Errorf("expected tail name 'r', got %q", row.Tail.Name)
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

// TestProbeD_DataNoConstructors verifies `data T :=` with nothing after `:=`.
func TestProbeD_DataNoConstructors(t *testing.T) {
	_, es := parse("data T :=")
	if !es.HasErrors() {
		t.Error("expected error for data with no constructors")
	}
}

// TestProbeD_DataManyConstructors verifies a data type with many constructors.
func TestProbeD_DataManyConstructors(t *testing.T) {
	var b strings.Builder
	b.WriteString("data T := C0")
	for i := 1; i < 100; i++ {
		b.WriteString(fmt.Sprintf(" | C%d", i))
	}
	prog := parseMustSucceed(t, b.String())
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclData); ok && dd.Name == "T" {
			if len(dd.Cons) != 100 {
				t.Errorf("expected 100 constructors, got %d", len(dd.Cons))
			}
		}
	}
}

// ===== From probe_e =====

// TestProbeE_DataDeclNoConstructors verifies that `data T :=` with nothing after
// the `:=` either errors or produces an empty constructor list.
func TestProbeE_DataDeclNoConstructors(t *testing.T) {
	// After `:=`, the parser calls parseConDecl which calls expectUpper().
	// If the next token is not upper, it should error.
	_, es := parse("data T :=")
	if !es.HasErrors() {
		t.Error("expected error for data decl with no constructors")
	}
}

// TestProbeE_DataDeclTrailingPipe verifies `data T := A |` with trailing pipe.
func TestProbeE_DataDeclTrailingPipe(t *testing.T) {
	_, es := parse("data T := A |")
	if !es.HasErrors() {
		t.Error("expected error for trailing pipe in data decl")
	}
}

// TestProbeE_ClassDeclEmptyBody verifies `class C a {}` parses.
func TestProbeE_ClassDeclEmptyBody(t *testing.T) {
	prog := parseMustSucceed(t, "class C a {}")
	found := false
	for _, d := range prog.Decls {
		if cl, ok := d.(*DeclClass); ok && cl.Name == "C" {
			found = true
			if len(cl.Methods) != 0 {
				t.Errorf("expected 0 methods, got %d", len(cl.Methods))
			}
		}
	}
	if !found {
		t.Error("class C not found")
	}
}

// TestProbeE_ClassDeclNoBody verifies `class C a` with no braces.
func TestProbeE_ClassDeclNoBody(t *testing.T) {
	_, es := parse("class C a")
	if !es.HasErrors() {
		t.Error("expected error for class without body braces")
	}
}

// TestProbeE_InstanceDeclEmptyBody verifies `instance C Int {}` parses.
func TestProbeE_InstanceDeclEmptyBody(t *testing.T) {
	prog := parseMustSucceed(t, "instance C Int {}")
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

// TestProbeE_GADTDeclBasic verifies GADT syntax.
func TestProbeE_GADTDeclBasic(t *testing.T) {
	source := `data T a := {
  MkT :: Int -> T Int;
  MkU :: T Bool
}`
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclData); ok && dd.Name == "T" {
			if len(dd.GADTCons) != 2 {
				t.Errorf("expected 2 GADT constructors, got %d", len(dd.GADTCons))
			}
		}
	}
}

// TestProbeE_GADTEmptyBody verifies `data T := {}` — empty GADT body.
func TestProbeE_GADTEmptyBody(t *testing.T) {
	prog, es := parse("data T := {}")
	if es.HasErrors() {
		t.Logf("GADT empty body errors: %s", es.Format())
	}
	if prog != nil {
		for _, d := range prog.Decls {
			if dd, ok := d.(*DeclData); ok && dd.Name == "T" {
				if len(dd.GADTCons) != 0 {
					t.Errorf("expected 0 GADT cons, got %d", len(dd.GADTCons))
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

// TestProbeE_ClassWithSuperclass verifies class with superclass constraint.
func TestProbeE_ClassWithSuperclass(t *testing.T) {
	source := `class Eq a => Ord a {
  compare :: a -> a -> a
}`
	parseMustSucceed(t, source)
}

// TestProbeE_ClassWithMultipleSuperclasses verifies curried superclasses.
func TestProbeE_ClassWithMultipleSuperclasses(t *testing.T) {
	source := `class Eq a => Ord a => MyClass a {
  m :: a -> a
}`
	parseMustSucceed(t, source)
}

// TestProbeE_ClassWithTupleSuperclass verifies tuple-style superclass.
func TestProbeE_ClassWithTupleSuperclass(t *testing.T) {
	source := `class (Eq a, Ord a) => MyClass a {
  m :: a -> a
}`
	parseMustSucceed(t, source)
}

// TestProbeE_InstanceWithContext verifies instance with context constraint.
func TestProbeE_InstanceWithContext(t *testing.T) {
	source := `data Maybe a := Nothing | Just a
class Eq a { eq :: a -> a -> a }
instance Eq a => Eq (Maybe a) {
  eq := \x y. x
}`
	parseMustSucceed(t, source)
}

// TestProbeE_ClassBodyWithKeyword verifies class body with just a keyword errors.
func TestProbeE_ClassBodyWithKeyword(t *testing.T) {
	_, es := parse(`class C a { import }`)
	if !es.HasErrors() {
		t.Error("expected error for keyword in class body")
	}
}

// TestProbeE_InstanceMethodMissingColonEq verifies instance method without := errors.
func TestProbeE_InstanceMethodMissingColonEq(t *testing.T) {
	source := `class C a { m :: a -> a }
instance C Int { m = \x. x }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for '=' instead of ':=' in instance method")
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
// inside braces (newlines alone are not separators inside braces).
func TestProbeE_DeclBoundaryInNestedContext(t *testing.T) {
	source := `main := case x {
  Nothing -> 0;
  Just y -> y
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

// TestProbeE_InstanceAssocTypeDef verifies type equation in instance body.
func TestProbeE_InstanceAssocTypeDef(t *testing.T) {
	source := `data Unit := Unit
class C a {
  type Elem a :: Type
}
instance C Unit {
  type Elem Unit =: Unit
}`
	parseMustSucceed(t, source)
}

// TestProbeE_InstanceAssocDataDef verifies data definition in instance body.
func TestProbeE_InstanceAssocDataDef(t *testing.T) {
	source := `data Unit := Unit
class C a {
  data Container a :: Type
}
instance C Unit {
  data Container Unit =: UnitContainer
}`
	parseMustSucceed(t, source)
}

// TestProbeE_TypeFamilyEquationWrongName verifies that a type family equation
// with a name different from the family name is still accepted by the parser
// (the checker will reject it).
func TestProbeE_TypeFamilyEquationWrongName(t *testing.T) {
	source := `data Unit := Unit
type F (a: Type) :: Type := {
  G a =: Unit
}`
	// The parser doesn't enforce equation name matching the family name.
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if tf, ok := d.(*DeclTypeFamily); ok && tf.Name == "F" {
			if tf.Equations[0].Name != "G" {
				t.Errorf("expected equation name 'G', got %q", tf.Equations[0].Name)
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

// TestProbeE_InstanceBodyStall verifies that garbage in instance body
// doesn't cause infinite loop (the p.pos == before guard should catch it).
func TestProbeE_InstanceBodyStall(t *testing.T) {
	source := `class C a { m :: a }
instance C Int { + + + }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for garbage in instance body")
	}
}

// TestProbeE_ClassBodyStall verifies that garbage in class body
// doesn't cause infinite loop.
func TestProbeE_ClassBodyStall(t *testing.T) {
	source := `class C a { + + + }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for garbage in class body")
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

// TestProbeE_AllKeywordsInOneSource verifies a source using all keywords.
func TestProbeE_AllKeywordsInOneSource(t *testing.T) {
	source := `import Prelude
data Bool := True | False
type MyBool := Bool
class MyClass a {
  m :: a -> a
}
instance MyClass Bool {
  m := \x. x
}
infixl 6 +
infixr 5 ++
infixn 4 ==
main := case True {
  True -> do { pure 1 };
  False -> 0
}`
	parseMustSucceed(t, source)
}
