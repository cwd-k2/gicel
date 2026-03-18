//go:build probe

package parse

import (
	"fmt"
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/errs"
	"github.com/cwd-k2/gicel/internal/span"
)

// =====================================================
// Probe D: Parser Crash Resistance, Recovery, Boundaries
// =====================================================

// ----- 1. Crash Resistance: Empty and Minimal Inputs -----

// TestProbeD_EmptyInput verifies the parser does not crash on empty source.
func TestProbeD_EmptyInput(t *testing.T) {
	prog, es := parse("")
	if es.HasErrors() {
		t.Logf("errors on empty input (acceptable): %s", es.Format())
	}
	if prog == nil {
		t.Fatal("ParseProgram returned nil on empty input")
	}
	if len(prog.Decls) != 0 {
		t.Errorf("expected 0 decls on empty input, got %d", len(prog.Decls))
	}
}

// TestProbeD_OnlyWhitespace verifies whitespace-only input does not crash.
func TestProbeD_OnlyWhitespace(t *testing.T) {
	prog, es := parse("   \n\n\t  \n  ")
	if es.HasErrors() {
		t.Logf("errors on whitespace-only (acceptable): %s", es.Format())
	}
	if prog == nil {
		t.Fatal("ParseProgram returned nil on whitespace-only input")
	}
}

// TestProbeD_SingleLBrace verifies that a lone `{` does not crash.
func TestProbeD_SingleLBrace(t *testing.T) {
	_, es := parse("{")
	if !es.HasErrors() {
		t.Error("expected error for lone `{`")
	}
}

// TestProbeD_SingleEq verifies that a lone `=` does not crash.
func TestProbeD_SingleEq(t *testing.T) {
	_, es := parse("=")
	if !es.HasErrors() {
		t.Error("expected error for lone `=`")
	}
}

// TestProbeD_SingleLParen verifies that a lone `(` does not crash.
func TestProbeD_SingleLParen(t *testing.T) {
	_, es := parse("(")
	if !es.HasErrors() {
		t.Error("expected error for lone `(`")
	}
}

// TestProbeD_SingleRParen verifies that a lone `)` does not crash.
func TestProbeD_SingleRParen(t *testing.T) {
	_, es := parse(")")
	if !es.HasErrors() {
		t.Error("expected error for lone `)`")
	}
}

// TestProbeD_SingleRBrace verifies that a lone `}` does not crash.
func TestProbeD_SingleRBrace(t *testing.T) {
	_, es := parse("}")
	if !es.HasErrors() {
		t.Error("expected error for lone `}`")
	}
}

// TestProbeD_SingleDot verifies that a lone `.` does not crash.
func TestProbeD_SingleDot(t *testing.T) {
	_, es := parse(".")
	if !es.HasErrors() {
		t.Error("expected error for lone `.`")
	}
}

// TestProbeD_SingleArrow verifies that a lone `->` does not crash.
func TestProbeD_SingleArrow(t *testing.T) {
	_, es := parse("->")
	if !es.HasErrors() {
		t.Error("expected error for lone `->`")
	}
}

// TestProbeD_SingleColonColon verifies that a lone `::` does not crash.
func TestProbeD_SingleColonColon(t *testing.T) {
	_, es := parse("::")
	if !es.HasErrors() {
		t.Error("expected error for lone `::`")
	}
}

// TestProbeD_SingleBackslash verifies that a lone `\` does not crash.
func TestProbeD_SingleBackslash(t *testing.T) {
	_, es := parse("\\")
	if !es.HasErrors() {
		t.Error("expected error for lone `\\`")
	}
}

// TestProbeD_KeywordAlone verifies that each keyword alone does not crash.
func TestProbeD_KeywordAlone(t *testing.T) {
	keywords := []string{"case", "do", "data", "type", "infixl", "infixr", "infixn", "class", "instance", "import"}
	for _, kw := range keywords {
		t.Run(kw, func(t *testing.T) {
			_, es := parse(kw)
			// All should produce errors, not panics.
			if !es.HasErrors() {
				t.Errorf("expected error for keyword %q alone", kw)
			}
		})
	}
}

// ----- 2. Crash Resistance: Deep Nesting -----

// TestProbeD_DeeplyNestedParens verifies the recursion limit halts gracefully
// on 500 levels of parentheses (exceeds maxRecurseDepth=256).
func TestProbeD_DeeplyNestedParens(t *testing.T) {
	depth := 500
	var b strings.Builder
	for i := 0; i < depth; i++ {
		b.WriteByte('(')
	}
	b.WriteString("x")
	for i := 0; i < depth; i++ {
		b.WriteByte(')')
	}
	source := fmt.Sprintf("main := %s", b.String())
	src := span.NewSource("test", source)
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	prog := p.ParseProgram()
	// Must not panic. Errors are expected (recursion limit).
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	if !es.HasErrors() {
		t.Error("expected recursion limit error for 500-deep parens")
	}
}

// TestProbeD_DeeplyNestedBraces verifies deep brace nesting does not crash.
func TestProbeD_DeeplyNestedBraces(t *testing.T) {
	depth := 300
	var b strings.Builder
	// Build: main := { a := { b := { ... x ... } } }
	for i := 0; i < depth; i++ {
		b.WriteString(fmt.Sprintf("{ v%d := ", i))
	}
	b.WriteString("42")
	for i := 0; i < depth; i++ {
		b.WriteString(fmt.Sprintf("; v%d }", i))
	}
	source := fmt.Sprintf("main := %s", b.String())
	src := span.NewSource("test", source)
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	prog := p.ParseProgram()
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	// Just verify no panic. Errors acceptable.
}

// TestProbeD_LongTokenSequence verifies that many tokens (5000+) do not exhaust the step limit.
func TestProbeD_LongTokenSequence(t *testing.T) {
	var b strings.Builder
	// 2000 simple value definitions — each is 4 tokens (name := intlit ;)
	for i := 0; i < 2000; i++ {
		b.WriteString(fmt.Sprintf("v%d := %d\n", i, i))
	}
	prog, es := parse(b.String())
	if es.HasErrors() {
		t.Logf("errors on long sequence: %s", es.Format())
	}
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	if len(prog.Decls) < 1000 {
		t.Errorf("expected at least 1000 decls, got %d", len(prog.Decls))
	}
}

// ----- 3. Error Recovery -----

// TestProbeD_MultipleSyntaxErrors tests whether the parser recovers from
// a first error and continues parsing subsequent declarations.
// BUG: medium — error recovery after `name :=` with missing RHS swallows the next
// declaration. The parser sees `bad1 :=` then calls parseExpr() which consumes the
// newline-separated `bad2` as an expression (since parseExpr looks for atoms including
// lowercase identifiers). The second `bad2 :=` and `good2 := 99` are misidentified.
// Only 1 of 2 valid declarations is recovered.
func TestProbeD_MultipleSyntaxErrors(t *testing.T) {
	source := `bad1 :=
bad2 :=
good := 42
bad3 :=
good2 := 99`
	prog, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected errors for incomplete declarations")
	}
	// Check that at least one good declaration was recovered.
	goodCount := 0
	for _, d := range prog.Decls {
		if vd, ok := d.(*DeclValueDef); ok {
			if vd.Name == "good" || vd.Name == "good2" {
				goodCount++
			}
		}
	}
	t.Logf("recovered %d good declarations out of 5 total, errors: %s", goodCount, es.Format())
	// BUG: medium — only 1 of 2 valid decls recovered (good2 is swallowed by error recovery)
	if goodCount < 2 {
		t.Logf("BUG: medium — error recovery swallowed valid declaration 'good2'")
	}
}

// TestProbeD_ErrorRecoveryCascade tests whether a deeply broken expression
// cascades into many errors or stays contained.
func TestProbeD_ErrorRecoveryCascade(t *testing.T) {
	source := `main := case { + -> * ; } }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected errors")
	}
	// Count how many errors were produced. Excessive cascading is a quality concern.
	count := es.Len()
	t.Logf("cascade test: %d errors produced", count)
	if count > 20 {
		// BUG: medium — error cascade produces excessive diagnostics
		t.Logf("BUG: medium — error cascade produced %d errors, excessive for 1 broken expression", count)
	}
}

// ----- 4. Boundary Tokens -----

// TestProbeD_OperatorAtEOF verifies the parser handles an operator at end of input.
func TestProbeD_OperatorAtEOF(t *testing.T) {
	_, es := parse("main := 1 +")
	if !es.HasErrors() {
		t.Error("expected error for operator at EOF")
	}
}

// TestProbeD_UnclosedParenAtEOF verifies unclosed paren at EOF is handled.
func TestProbeD_UnclosedParenAtEOF(t *testing.T) {
	_, es := parse("main := (1 + 2")
	if !es.HasErrors() {
		t.Error("expected error for unclosed paren at EOF")
	}
}

// TestProbeD_UnclosedBraceAtEOF verifies unclosed brace at EOF is handled.
func TestProbeD_UnclosedBraceAtEOF(t *testing.T) {
	_, es := parse("main := do { x <- pure 1")
	if !es.HasErrors() {
		t.Error("expected error for unclosed brace at EOF")
	}
}

// TestProbeD_UnclosedBracketAtEOF verifies unclosed bracket at EOF is handled.
func TestProbeD_UnclosedBracketAtEOF(t *testing.T) {
	_, es := parse("main := [1, 2, 3")
	if !es.HasErrors() {
		t.Error("expected error for unclosed bracket at EOF")
	}
}

// TestProbeD_ImportNoModuleName verifies `import` with nothing after it.
func TestProbeD_ImportNoModuleName(t *testing.T) {
	_, es := parse("import")
	if !es.HasErrors() {
		t.Error("expected error for `import` with no module name")
	}
}

// TestProbeD_ImportLowercaseName verifies `import foo` is rejected (expects Upper).
func TestProbeD_ImportLowercaseName(t *testing.T) {
	_, es := parse("import foo")
	if !es.HasErrors() {
		t.Error("expected error for `import foo` (lowercase module name)")
	}
}

// ----- 5. Dot Disambiguation Edge Cases -----

// TestProbeD_DotAtEOF verifies `M.` at end of input does not crash.
func TestProbeD_DotAtEOF(t *testing.T) {
	_, es := parse("main := M.")
	// Should error — dot with nothing after it.
	if !es.HasErrors() {
		t.Error("expected error for M. at EOF")
	}
}

// TestProbeD_DotSpaceLower verifies `M. x` (space after dot) is composition, not qualified.
func TestProbeD_DotSpaceLower(t *testing.T) {
	// M . x — the lexer inserts space so tokens are not adjacent.
	// This should parse as infix `.` application.
	prog, es := parse("main := M . x")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix for `M . x`, got %T", d.Expr)
	}
	if inf.Op != "." {
		t.Errorf("expected op '.', got %q", inf.Op)
	}
}

// TestProbeD_DotBetweenTwoLowers verifies `f.g` is infix composition.
func TestProbeD_DotBetweenTwoLowers(t *testing.T) {
	prog, es := parse("main := f.g")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	d := prog.Decls[0].(*DeclValueDef)
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix for `f.g`, got %T", d.Expr)
	}
	if inf.Op != "." {
		t.Errorf("expected op '.', got %q", inf.Op)
	}
}

// TestProbeD_DotChainQualified verifies `M.N.x` — double qualification.
// Current behavior: M.N is ExprQualCon, then `.x` needs infix continuation.
func TestProbeD_DotChainQualified(t *testing.T) {
	prog, es := parse("main := M.N.x")
	// Should not panic. Document actual behavior.
	if es.HasErrors() {
		t.Logf("errors (acceptable for double qualification): %s", es.Format())
		return
	}
	d := prog.Decls[0].(*DeclValueDef)
	t.Logf("M.N.x parsed as %T", d.Expr)
}

// TestProbeD_DotNumber verifies `M.42` does not crash (dot followed by number).
func TestProbeD_DotNumber(t *testing.T) {
	_, es := parse("main := M.42")
	// Should not crash. Might produce a non-qualified parse.
	_ = es
}

// ----- 6. Semicolon / Newline Interchangeability -----

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

// TestProbeD_NewlineInDoBlock verifies newlines as statement separators in do-blocks.
// BUG: medium — newlines inside do-blocks are not treated as semicolons.
// The design says "`;` and newline interchangeable at all levels" but inside
// braces (depth > 0), newlines are only tracked as NewlineBefore on the next token.
// The do-block parser does not use NewlineBefore to terminate expression parsing;
// instead, parseExpr/parseApp greedily consumes the next line's identifier as an
// argument. For `x <- pure 1\ny <- pure 2`, after parsing `pure 1`, the parser
// sees `y` as an atom start and tries to apply `1 y`, producing a spurious error.
// Root cause: no layout rule or newline-as-separator logic inside braced blocks.
func TestProbeD_NewlineInDoBlock(t *testing.T) {
	source := "main := do {\n  x <- pure 1\n  y <- pure 2\n  pure x\n}"
	prog, es := parse(source)
	if es.HasErrors() {
		// BUG: medium — newline-separated do-block statements produce errors
		t.Logf("BUG: medium — do-block newline separation produces errors: %s", es.Format())
	}
	if prog != nil && len(prog.Decls) > 0 {
		d := prog.Decls[0].(*DeclValueDef)
		doExpr, ok := d.Expr.(*ExprDo)
		if ok {
			t.Logf("do-block parsed %d statements", len(doExpr.Stmts))
		}
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

// ----- 7. Fixity Edge Cases -----

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
// `a == b /= c` because neither can associate with the other. The Pratt parser's
// continueInfix loop uses `fix.Prec < minPrec` to break, and for AssocNone sets
// `nextMin = fix.Prec + 1`. This means after parsing `1 == 2`, the second operator
// `/=` at same precedence 5 satisfies `5 >= 5` (not less than minPrec=0) so it
// continues, silently left-grouping as `(1 == 2) /= 3`. A correct implementation
// would detect the conflict and report an error.
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

// ----- 8. GADT Syntax -----

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
// BUG: low — same newline-as-separator issue. After parsing `A :: Int -> T`, the type
// parser sees `B` on the next line as a type atom continuation (TyExprCon), extending
// the first constructor's type to `Int -> T B`. Recovery succeeds (2 constructors)
// but with a spurious "expected uppercase identifier" error on `::`.
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

// ----- 9. Type Family Equations -----

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

// ----- 10. Import Edge Cases -----

// TestProbeD_ImportDottedNameEOF verifies `import M.` (dot at boundary).
func TestProbeD_ImportDottedNameEOF(t *testing.T) {
	_, es := parse("import M.")
	if !es.HasErrors() {
		t.Error("expected error for `import M.` (no name after dot)")
	}
}

// TestProbeD_ImportAsNoAlias verifies `import M as` with no alias.
func TestProbeD_ImportAsNoAlias(t *testing.T) {
	_, es := parse("import M as")
	if !es.HasErrors() {
		t.Error("expected error for `import M as` (no alias)")
	}
}

// TestProbeD_ImportAsLowercase verifies `import M as n` (lowercase alias).
func TestProbeD_ImportAsLowercase(t *testing.T) {
	_, es := parse("import M as n")
	// `as` is matched by TokLower + text=="as", then expectUpper() for alias.
	// `n` is lowercase, so expectUpper should fail.
	if !es.HasErrors() {
		t.Error("expected error for lowercase alias in qualified import")
	}
}

// TestProbeD_ImportEmptyParens verifies `import M ()` — selective import with no names.
func TestProbeD_ImportEmptyParens(t *testing.T) {
	prog, es := parse("import M ()")
	if es.HasErrors() {
		t.Logf("import M (): %s", es.Format())
		return
	}
	// Parser should produce a DeclImport with empty Names slice (non-nil).
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	imp := prog.Imports[0]
	if imp.Names == nil {
		// BUG: low — import M () should produce non-nil empty Names to distinguish from import M
		t.Log("BUG: low — import M () produced nil Names, indistinguishable from open import")
	} else if len(imp.Names) != 0 {
		t.Errorf("expected 0 names in import M (), got %d", len(imp.Names))
	}
}

// TestProbeD_ImportDottedMultiple verifies `import A.B.C` — deep dotted module name.
func TestProbeD_ImportDottedMultiple(t *testing.T) {
	prog, es := parse("import A.B.C")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if prog.Imports[0].ModuleName != "A.B.C" {
		t.Errorf("expected module name A.B.C, got %q", prog.Imports[0].ModuleName)
	}
}

// TestProbeD_ImportSelectiveOperator verifies `import M ((+))`.
func TestProbeD_ImportSelectiveOperator(t *testing.T) {
	prog, es := parse("import M ((+))")
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	names := prog.Imports[0].Names
	if len(names) != 1 || names[0].Name != "+" {
		t.Errorf("expected import of operator +, got %v", names)
	}
}

// TestProbeD_ImportSelectiveDot verifies `import M ((.))` — importing the dot operator.
func TestProbeD_ImportSelectiveDot(t *testing.T) {
	prog, es := parse("import M ((.))") // (.) in import list
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	names := prog.Imports[0].Names
	if len(names) != 1 || names[0].Name != "." {
		t.Errorf("expected import of operator '.', got %v", names)
	}
}

// ----- 11. Class / Instance Edge Cases -----

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

// ----- 12. Unicode Identifiers -----

// TestProbeD_UnicodeUpperIdent checks if unicode uppercase letters are accepted as Upper identifiers.
func TestProbeD_UnicodeUpperIdent(t *testing.T) {
	// isUpperStart only checks A-Z (ASCII). Unicode uppercase should be rejected.
	_, es := parse("data \u00C9tat := \u00C9tat") // data État := État
	if !es.HasErrors() {
		// If this succeeds, the lexer accepts Unicode uppercase as identifiers.
		t.Log("Unicode uppercase identifier accepted (isUpperStart accepts only ASCII A-Z)")
	}
}

// TestProbeD_UnicodeLowerIdent checks unicode lowercase letters in identifiers.
func TestProbeD_UnicodeLowerIdent(t *testing.T) {
	// isLowerStart checks a-z or _. Unicode lowercase should be rejected.
	_, es := parse("main := \u00E9tat") // main := état
	if !es.HasErrors() {
		t.Log("Unicode lowercase identifier accepted (isLowerStart checks a-z and _)")
	}
}

// TestProbeD_UnicodeInIdentCont checks unicode continuation characters.
// isIdentCont uses unicode.IsLetter, so unicode letters in continuation position
// should be accepted (e.g., `café`).
func TestProbeD_UnicodeInIdentCont(t *testing.T) {
	prog, es := parse("caf\u00E9 := 42") // café := 42
	if es.HasErrors() {
		t.Logf("unicode continuation rejected: %s", es.Format())
		return
	}
	if len(prog.Decls) > 0 {
		if vd, ok := prog.Decls[0].(*DeclValueDef); ok {
			if vd.Name != "caf\u00E9" {
				t.Errorf("expected name 'café', got %q", vd.Name)
			}
		}
	}
}

// ----- 13. Halting Behavior -----

// TestProbeD_StepLimitHalt verifies the parser halts when step limit is exceeded.
// We construct input that causes many advance() calls per token.
func TestProbeD_StepLimitHalt(t *testing.T) {
	// Build a pathological case with many operators that force reprocessing.
	// A sequence of `+ + + + ...` should cause many advances in error recovery.
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 500; i++ {
		b.WriteString("+ ")
	}
	src := span.NewSource("test", b.String())
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	prog := p.ParseProgram()
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	// Check the parser halted (errors present).
	if !es.HasErrors() {
		t.Error("expected errors from pathological operator sequence")
	}
}

// TestProbeD_RecursionLimitOnNestedTypes verifies deep type nesting triggers limit.
func TestProbeD_RecursionLimitOnNestedTypes(t *testing.T) {
	depth := 300
	var ty strings.Builder
	for i := 0; i < depth; i++ {
		ty.WriteString("(")
	}
	ty.WriteString("Int")
	for i := 0; i < depth; i++ {
		ty.WriteString(")")
	}
	source := fmt.Sprintf("x :: %s\nx := 42", ty.String())
	src := span.NewSource("test", source)
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	prog := p.ParseProgram()
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	// Should hit recursion limit. Errors expected.
	if !es.HasErrors() {
		t.Log("300-deep type nesting parsed without hitting recursion limit")
	}
}

// ----- 14. Expression Edge Cases -----

// TestProbeD_EmptyDoBlock verifies `do {}` does not crash.
func TestProbeD_EmptyDoBlock(t *testing.T) {
	prog, es := parse("main := do {}")
	if es.HasErrors() {
		t.Logf("empty do block: %s", es.Format())
	}
	if prog != nil && len(prog.Decls) > 0 {
		if vd, ok := prog.Decls[0].(*DeclValueDef); ok {
			if doExpr, ok := vd.Expr.(*ExprDo); ok {
				if len(doExpr.Stmts) != 0 {
					t.Errorf("expected 0 statements in empty do, got %d", len(doExpr.Stmts))
				}
			}
		}
	}
}

// TestProbeD_EmptyCaseBlock verifies `case x {}` does not crash.
func TestProbeD_EmptyCaseBlock(t *testing.T) {
	prog, es := parse("main := case x {}")
	if es.HasErrors() {
		t.Logf("empty case block: %s", es.Format())
	}
	if prog != nil && len(prog.Decls) > 0 {
		if vd, ok := prog.Decls[0].(*DeclValueDef); ok {
			if caseExpr, ok := vd.Expr.(*ExprCase); ok {
				if len(caseExpr.Alts) != 0 {
					t.Errorf("expected 0 alternatives in empty case, got %d", len(caseExpr.Alts))
				}
			}
		}
	}
}

// TestProbeD_LambdaNoParams verifies `\ . x` (lambda with no parameters).
func TestProbeD_LambdaNoParams(t *testing.T) {
	prog, es := parse("main := \\. x")
	if es.HasErrors() {
		t.Logf("lambda no params: %s", es.Format())
		return
	}
	// Parser may accept it — lambda with empty param list.
	if prog != nil && len(prog.Decls) > 0 {
		if vd, ok := prog.Decls[0].(*DeclValueDef); ok {
			if lam, ok := vd.Expr.(*ExprLam); ok {
				if len(lam.Params) != 0 {
					t.Errorf("expected 0 params for \\. x, got %d", len(lam.Params))
				}
			}
		}
	}
}

// TestProbeD_TupleExpr verifies tuple expression `(1, 2, 3)`.
func TestProbeD_TupleExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := (1, 2, 3)")
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for tuple, got %T", d.Expr)
	}
	if len(rec.Fields) != 3 {
		t.Errorf("expected 3 fields for 3-tuple, got %d", len(rec.Fields))
	}
	for i, f := range rec.Fields {
		expected := fmt.Sprintf("_%d", i+1)
		if f.Label != expected {
			t.Errorf("field %d: expected label %q, got %q", i, expected, f.Label)
		}
	}
}

// TestProbeD_UnitExpr verifies `()` parses as empty record (unit).
func TestProbeD_UnitExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := ()")
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for unit, got %T", d.Expr)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields for unit, got %d", len(rec.Fields))
	}
}

// TestProbeD_EmptyRecord verifies `{}` parses as empty record.
func TestProbeD_EmptyRecord(t *testing.T) {
	prog := parseMustSucceed(t, "main := {}")
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for {}, got %T", d.Expr)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields for empty record, got %d", len(rec.Fields))
	}
}

// TestProbeD_OperatorSection verifies `(+ 1)` as right section.
func TestProbeD_OperatorSection(t *testing.T) {
	prog := parseMustSucceed(t, "main := (+ 1)")
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if !sec.IsRight {
		t.Error("expected right section")
	}
	if sec.Op != "+" {
		t.Errorf("expected operator '+', got %q", sec.Op)
	}
}

// TestProbeD_LeftSection verifies `(1 +)` as left section.
func TestProbeD_LeftSection(t *testing.T) {
	prog := parseMustSucceed(t, "main := (1 +)")
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if sec.IsRight {
		t.Error("expected left section")
	}
}

// TestProbeD_ListLiteral verifies `[1, 2, 3]`.
func TestProbeD_ListLiteral(t *testing.T) {
	prog := parseMustSucceed(t, "main := [1, 2, 3]")
	d := prog.Decls[0].(*DeclValueDef)
	lst, ok := d.Expr.(*ExprList)
	if !ok {
		t.Fatalf("expected ExprList, got %T", d.Expr)
	}
	if len(lst.Elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(lst.Elems))
	}
}

// TestProbeD_EmptyList verifies `[]`.
func TestProbeD_EmptyList(t *testing.T) {
	prog := parseMustSucceed(t, "main := []")
	d := prog.Decls[0].(*DeclValueDef)
	lst, ok := d.Expr.(*ExprList)
	if !ok {
		t.Fatalf("expected ExprList, got %T", d.Expr)
	}
	if len(lst.Elems) != 0 {
		t.Errorf("expected 0 elements, got %d", len(lst.Elems))
	}
}

// ----- 15. Type Edge Cases -----

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

// ----- 16. Lexer Edge Cases -----

// TestProbeD_LexerUnterminatedString verifies unterminated string produces error.
func TestProbeD_LexerUnterminatedString(t *testing.T) {
	_, es := parse(`main := "hello`)
	if !es.HasErrors() {
		t.Error("expected error for unterminated string")
	}
}

// TestProbeD_LexerUnterminatedBlockComment verifies unterminated {- does not hang.
func TestProbeD_LexerUnterminatedBlockComment(t *testing.T) {
	_, es := parse("{- this comment never ends")
	if !es.HasErrors() {
		t.Error("expected error for unterminated block comment")
	}
}

// TestProbeD_LexerNestedBlockComment verifies nested {- {- -} -} works.
func TestProbeD_LexerNestedBlockComment(t *testing.T) {
	prog := parseMustSucceed(t, "{- outer {- inner -} outer -} x := 42")
	if len(prog.Decls) != 1 {
		t.Errorf("expected 1 decl after nested comment, got %d", len(prog.Decls))
	}
}

// TestProbeD_LexerEmptyRune verifies `”` is handled (empty rune literal).
func TestProbeD_LexerEmptyRune(t *testing.T) {
	_, es := parse("main := ''")
	if !es.HasErrors() {
		t.Error("expected error for empty rune literal")
	}
}

// TestProbeD_LexerDoubleWithExponent verifies `1.5e10` is TokDoubleLit.
func TestProbeD_LexerDoubleWithExponent(t *testing.T) {
	tokens := lex("1.5e10")
	if len(tokens) < 1 || tokens[0].Kind != TokDoubleLit {
		t.Errorf("expected TokDoubleLit for 1.5e10, got %v", tokens[0].Kind)
	}
}

// TestProbeD_LexerUnderscoreSeparator verifies `100_000` parses as single int.
func TestProbeD_LexerUnderscoreSeparator(t *testing.T) {
	tokens := lex("100_000")
	if len(tokens) < 1 || tokens[0].Kind != TokIntLit {
		t.Errorf("expected TokIntLit for 100_000, got %v", tokens[0].Kind)
	}
	if tokens[0].Text != "100_000" {
		t.Errorf("expected text '100_000', got %q", tokens[0].Text)
	}
}

// ----- 17. Data Declaration Edge Cases -----

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

// ----- 18. Mixed / Stress -----

// TestProbeD_MixedDeclsStress verifies a program with all declaration kinds.
func TestProbeD_MixedDeclsStress(t *testing.T) {
	source := `
import Prelude
import Mod as M

data Bool := True | False

type Alias := Int

infixl 6 +
(+) := \a b. a

class Eq a {
  eq :: a -> a -> Bool
}

instance Eq Bool {
  eq := \a b. True
}

id :: \a. a -> a
id := \x. x

main := id True
`
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected error in mixed decl stress: %s", es.Format())
	}
	if len(prog.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(prog.Imports))
	}
	if len(prog.Decls) < 6 {
		t.Errorf("expected at least 6 decls, got %d", len(prog.Decls))
	}
}

// TestProbeD_CommentOnlyInput verifies input that is only comments.
func TestProbeD_CommentOnlyInput(t *testing.T) {
	prog, es := parse("-- just a comment\n{- block -}\n-- another")
	if es.HasErrors() {
		t.Logf("comment-only input errors: %s", es.Format())
	}
	if prog == nil {
		t.Fatal("ParseProgram returned nil for comment-only input")
	}
	if len(prog.Decls) != 0 {
		t.Errorf("expected 0 decls for comment-only input, got %d", len(prog.Decls))
	}
}
