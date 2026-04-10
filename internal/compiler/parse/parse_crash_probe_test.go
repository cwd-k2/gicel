//go:build probe

// Parser crash probe tests — deep nesting, long inputs, error recovery, step limits.
package parse

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// ===== From probe_b =====

// TestProbeB_MismatchedParens checks mismatched parentheses.
func TestProbeB_MismatchedParens(t *testing.T) {
	errMsg := parseMustFail(t, "main := (1 + 2")
	// Should report missing )
	if !strings.Contains(errMsg, ")") && !strings.Contains(errMsg, "RParen") {
		t.Logf("error for mismatched parens: %s", errMsg)
	}
}

// TestProbeB_UnexpectedEOFInExpr checks unexpected EOF after := in a value definition.
// Fixed: parse_decl.go detects missing body and reports ErrMissingBody.
func TestProbeB_UnexpectedEOFInExpr(t *testing.T) {
	_, es := parse("main :=")
	if !es.HasErrors() {
		t.Fatal("expected error for 'main :=' with no expression body")
	}
	found := false
	for _, e := range es.Errs {
		if e.Code == diagnostic.ErrMissingBody {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ErrMissingBody (E0103), got: %s", es.Format())
	}
}

// TestProbeB_UnexpectedEOFInCase checks unexpected EOF in case expression.
func TestProbeB_UnexpectedEOFInCase(t *testing.T) {
	errMsg := parseMustFail(t, "main := case x {")
	if errMsg == "" {
		t.Error("expected non-empty error")
	}
}

// TestProbeB_UnexpectedEOFInDo checks unexpected EOF in do-block.
func TestProbeB_UnexpectedEOFInDo(t *testing.T) {
	errMsg := parseMustFail(t, "main := do {")
	if errMsg == "" {
		t.Error("expected non-empty error")
	}
}

// TestProbeB_MissingArrowInCaseAlt checks missing -> in case alternative.
func TestProbeB_MissingArrowInCaseAlt(t *testing.T) {
	errMsg := parseMustFail(t, "main := case x { True 1 }")
	if errMsg == "" {
		t.Error("expected error for missing ->")
	}
}

// TestProbeB_ExtraCommaInTuple checks trailing comma in tuple.
func TestProbeB_ExtraCommaInTuple(t *testing.T) {
	// (1, 2,) — trailing comma. The parser will try to parse another expr after the comma.
	_, es := parse("main := (1, 2,)")
	// Should either parse or produce a meaningful error, but not panic.
	_ = es
}

// TestProbeB_MissingColonInRecord checks missing : in record literal.
// NOTE: { x 1 } is ambiguous. The parser's block disambiguation sees
// "lower" followed by neither ":" nor "|", so falls through to the block
// expression path.
func TestProbeB_MissingColonInRecord(t *testing.T) {
	prog, es := parse("main := { x 1 }")
	if es.HasErrors() {
		t.Logf("parser rejected { x 1 }: %s", es.Format())
		return
	}
	d := prog.Decls[0].(*DeclValueDef)
	// It should parse as a block expression with body "x 1" (App(x, 1)).
	if blk, ok := d.Expr.(*ExprBlock); ok {
		t.Logf("parsed as block expression with %d binds", len(blk.Binds))
	} else {
		t.Logf("parsed { x 1 } as %T", d.Expr)
	}
}

// TestProbeB_DoubleColonColonInDecl checks malformed declaration: f :: :: Int.
func TestProbeB_DoubleColonColonInDecl(t *testing.T) {
	_, es := parse("f :: :: Int")
	// Should produce error but not panic.
	if !es.HasErrors() {
		t.Log("parser accepted 'f :: :: Int' — check if AST is sensible")
	}
}

// TestProbeB_DeepNestedParens200 checks 200 nested parens don't crash.
func TestProbeB_DeepNestedParens200(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 200; i++ {
		b.WriteByte('(')
	}
	b.WriteString("x")
	for i := 0; i < 200; i++ {
		b.WriteByte(')')
	}
	src := span.NewSource("test", b.String())
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
	_ = p.ParseProgram()
	// No panic = success. Errors are acceptable.
}

// TestProbeB_LongIdentifier100 checks a 100-char identifier parses correctly.
func TestProbeB_LongIdentifier100(t *testing.T) {
	name := strings.Repeat("x", 100)
	src := fmt.Sprintf("%s := 42", name)
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	d := prog.Decls[0].(*DeclValueDef)
	if d.Name != name {
		t.Errorf("expected 100-char name, got len=%d", len(d.Name))
	}
}

// TestProbeB_SequentialSemicolons50 checks that 50 sequential semicolons don't crash.
func TestProbeB_SequentialSemicolons50(t *testing.T) {
	src := "main := 1" + strings.Repeat(";", 50) + "\nf := 2"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) < 2 {
		t.Errorf("expected at least 2 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_OnlySemicolons checks a source with nothing but semicolons.
func TestProbeB_OnlySemicolons(t *testing.T) {
	_, es := parse(strings.Repeat(";", 50))
	// Should not panic. May or may not error.
	_ = es
}

// TestProbeB_EmptySource checks empty string input.
func TestProbeB_EmptySource(t *testing.T) {
	prog := parseMustSucceed(t, "")
	if len(prog.Decls) != 0 || len(prog.Imports) != 0 {
		t.Errorf("expected empty program")
	}
}

// TestProbeB_DeepNestedTypes checks deeply nested type: Maybe (Maybe (Maybe ... a ...)).
func TestProbeB_DeepNestedTypes(t *testing.T) {
	ty := "a"
	for i := 0; i < 100; i++ {
		ty = fmt.Sprintf("(Maybe %s)", ty)
	}
	src := fmt.Sprintf("f :: %s", ty)
	_, es := parse(src)
	// May hit recursion limit, but should not panic.
	_ = es
}

// TestProbeB_DeepNestedLambda checks deeply nested lambda: \x. \x. \x. ... x.
func TestProbeB_DeepNestedLambda(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 100; i++ {
		b.WriteString(`\x. `)
	}
	b.WriteString("x")
	_, es := parse(b.String())
	// May hit recursion limit, but should not panic.
	_ = es
}

// TestProbeB_DeepNestedCase checks deeply nested case expressions.
func TestProbeB_DeepNestedCase(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 50; i++ {
		b.WriteString("case x { True => ")
	}
	b.WriteString("1")
	for i := 0; i < 50; i++ {
		b.WriteString(" }")
	}
	_, es := parse(b.String())
	// Should not panic.
	_ = es
}

// TestProbeB_ManyDeclarations checks 100 declarations.
func TestProbeB_ManyDeclarations(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString(fmt.Sprintf("x%d := %d\n", i, i))
	}
	prog := parseMustSucceed(t, b.String())
	if len(prog.Decls) != 100 {
		t.Errorf("expected 100 decls, got %d", len(prog.Decls))
	}
}

// TestProbeB_StepLimitOnRepeatedTokens verifies the parser step limit
// prevents infinite loops on adversarial token streams.
func TestProbeB_StepLimitOnRepeatedTokens(t *testing.T) {
	// A long sequence of operators with no operands to parse — parser should hit step limit.
	var b strings.Builder
	b.WriteString("main := x")
	for i := 0; i < 500; i++ {
		b.WriteString(" + x")
	}
	_, es := parse(b.String())
	// Should succeed or produce step limit error, but not hang.
	_ = es
}

// ===== From probe_d =====

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
	keywords := []string{"case", "do", "form", "type", "infixl", "infixr", "infixn", "class", "instance", "import"}
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
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
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
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
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

// TestProbeD_MultipleSyntaxErrors tests whether the parser recovers from
// a first error and continues parsing subsequent declarations.
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
	// Fixed: error recovery now recovers both good and good2.
	if goodCount < 2 {
		t.Errorf("expected 2 good declarations recovered, got %d", goodCount)
	}
}

// TestProbeD_ErrorRecoveryCascade tests whether a deeply broken expression
// cascades into many errors or stays contained.
func TestProbeD_ErrorRecoveryCascade(t *testing.T) {
	source := `main := case { + => * ; } }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected errors")
	}
	// Count how many errors were produced. Excessive cascading is a quality concern.
	count := es.Len()
	t.Logf("cascade test: %d errors produced", count)
	// Fixed: syncToStmtBoundary limits cascade. Threshold lowered from 20 to 10.
	if count > 10 {
		t.Errorf("error cascade produced %d errors, expected ≤10 for 1 broken expression", count)
	}
}

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

// TestProbeD_StepLimitHalt verifies the parser halts when step limit is exceeded.
func TestProbeD_StepLimitHalt(t *testing.T) {
	// Build a pathological case with many operators that force reprocessing.
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 500; i++ {
		b.WriteString("+ ")
	}
	src := span.NewSource("test", b.String())
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
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
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
	prog := p.ParseProgram()
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	// Should hit recursion limit. Errors expected.
	if !es.HasErrors() {
		t.Log("300-deep type nesting parsed without hitting recursion limit")
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

// TestProbeD_TupleExpr verifies tuple expression `(1, 2, 3)` produces ExprTuple.
func TestProbeD_TupleExpr(t *testing.T) {
	prog := parseMustSucceed(t, "main := (1, 2, 3)")
	d := prog.Decls[0].(*DeclValueDef)
	tup, ok := d.Expr.(*ExprTuple)
	if !ok {
		t.Fatalf("expected ExprTuple for tuple, got %T", d.Expr)
	}
	if len(tup.Elems) != 3 {
		t.Errorf("expected 3 elements for 3-tuple, got %d", len(tup.Elems))
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

// TestProbeD_MixedDeclsStress verifies a program with all declaration kinds.
func TestProbeD_MixedDeclsStress(t *testing.T) {
	source := `
import Prelude
import Mod as M

form Bool := True | False

type Alias := Int

infixl 6 +
(+) := \a b. a

form Eq := \a. {
  eq: a -> a -> Bool
}

impl Eq Bool := {
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

// ===== From probe_e =====

// TestProbeE_CrashDeepNestedParens verifies deep parens don't crash.
func TestProbeE_CrashDeepNestedParens(t *testing.T) {
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
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
	_ = p.ParseProgram()
	// No panic = success.
}

// TestProbeE_CrashDeepNestedTypes verifies deep nested types don't crash.
func TestProbeE_CrashDeepNestedTypes(t *testing.T) {
	depth := 300
	ty := "Int"
	for i := 0; i < depth; i++ {
		ty = fmt.Sprintf("(Maybe %s)", ty)
	}
	source := fmt.Sprintf("x :: %s\nx := 1", ty)
	_, es := parse(source)
	// May hit recursion limit — that's fine.
	_ = es
}

// TestProbeE_CrashDeepNestedCase verifies deep case nesting don't crash.
func TestProbeE_CrashDeepNestedCase(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 50; i++ {
		b.WriteString("case x { True => ")
	}
	b.WriteString("1")
	for i := 0; i < 50; i++ {
		b.WriteString(" }")
	}
	_, es := parse(b.String())
	_ = es
}

// TestProbeE_CrashDeepNestedDo verifies deep do nesting doesn't crash.
func TestProbeE_CrashDeepNestedDo(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 50; i++ {
		b.WriteString("do { x <- ")
	}
	b.WriteString("pure 1")
	for i := 0; i < 50; i++ {
		b.WriteString("; pure x }")
	}
	_, es := parse(b.String())
	_ = es
}

// TestProbeE_CrashDeepNestedLambda verifies deep lambda nesting doesn't crash.
func TestProbeE_CrashDeepNestedLambda(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := ")
	for i := 0; i < 100; i++ {
		b.WriteString("\\x. ")
	}
	b.WriteString("x")
	_, es := parse(b.String())
	_ = es
}

// TestProbeE_CrashUnbalancedParens verifies various unbalanced paren combinations.
func TestProbeE_CrashUnbalancedParens(t *testing.T) {
	cases := []string{
		"main := (((",
		"main := )))",
		"main := ([{",
		"main := }])",
		"main := ((())(",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, es := parse(src)
			if !es.HasErrors() {
				t.Error("expected error for unbalanced parens")
			}
		})
	}
}

// TestProbeE_CrashVeryLongLine verifies the parser handles very long lines.
func TestProbeE_CrashVeryLongLine(t *testing.T) {
	// A very long expression: f a a a a ... a (10000 args)
	var b strings.Builder
	b.WriteString("main := f")
	for i := 0; i < 10000; i++ {
		b.WriteString(" a")
	}
	_, es := parse(b.String())
	// Should not panic. Might hit step limit.
	_ = es
}

// TestProbeE_CrashRepeatedSemicolons verifies many semicolons don't stall.
func TestProbeE_CrashRepeatedSemicolons(t *testing.T) {
	source := strings.Repeat(";", 1000) + "main := 1" + strings.Repeat(";", 1000)
	prog, es := parse(source)
	if es.HasErrors() {
		t.Logf("errors (acceptable): %s", es.Format())
	}
	if prog == nil {
		t.Fatal("prog is nil")
	}
}

// TestProbeE_CrashAllKeywordsAsExpr verifies keywords in expression position error gracefully.
func TestProbeE_CrashAllKeywordsAsExpr(t *testing.T) {
	keywords := []string{"form", "type", "impl", "import", "infixl", "infixr", "infixn"}
	for _, kw := range keywords {
		t.Run(kw, func(t *testing.T) {
			source := fmt.Sprintf("main := %s", kw)
			_, es := parse(source)
			// Should error, not panic.
			if !es.HasErrors() {
				t.Errorf("expected error for keyword %q in expression position", kw)
			}
		})
	}
}

// TestProbeCrash_LetAsAtomInAppPosition pins the fix for a nil-deref crash in
// parseApp's inner argument loop: `parseAtom` returned nil when it encountered
// `let` as an identifier in expression position, and parseApp then dereferenced
// arg.Span() without a guard. Discovered by field-test algorithm agent
// (2026-04-08) via `do { let := 1; pure let }`.
func TestProbeCrash_LetAsAtomInAppPosition(t *testing.T) {
	// Minimal repros — both previously panicked with nil pointer deref.
	sources := []string{
		`main := do { let := 1; pure let }`,
		`main := pure let`,
		`main := f let`,
		`main := let`,
	}
	for _, src := range sources {
		// Must not panic. Diagnostic must be reported.
		_, es := parse(src)
		if !es.HasErrors() {
			t.Errorf("expected parse error for %q, got none", src)
		}
	}
}

// TestProbeE_HaltedParserReturnsEOF verifies that once halted, all peek/advance
// return EOF.
func TestProbeE_HaltedParserReturnsEOF(t *testing.T) {
	// Create input that will trigger the step limit.
	var b strings.Builder
	b.WriteString("main := ")
	// A pattern that generates many steps: repeated error recovery.
	for i := 0; i < 5000; i++ {
		b.WriteString("( ")
	}
	src := span.NewSource("test", b.String())
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
	_ = p.ParseProgram()
	// Should have halted (step or recursion limit).
	if !es.HasErrors() {
		t.Error("expected parser errors from stress input")
	}
}
