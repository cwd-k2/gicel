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
// Probe E: Parser / Syntax / Front-End Adversarial Tests
// =====================================================
//
// Focus areas:
//   - Lexer edge cases (escapes, unicode, null bytes, long tokens)
//   - Operator parsing (non-associative chaining, precedence boundaries, sections)
//   - Declaration parsing (malformed class/instance, empty bodies, duplicates)
//   - Pattern parsing (deep nesting, ambiguities)
//   - Do-notation (empty blocks, edge cases)
//   - Import parsing (malformed lists, edge cases)
//   - Error recovery and crash resistance
//

// ----- 1. Lexer Edge Cases -----

// TestProbeE_LexerUnterminatedString verifies the lexer reports and recovers
// from an unterminated string literal at end of source.
func TestProbeE_LexerUnterminatedString(t *testing.T) {
	src := span.NewSource("test", `"hello`)
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for unterminated string")
	}
	// Should still produce a TokStrLit token (partial) + EOF.
	if len(tokens) < 2 {
		t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
	}
	if tokens[0].Kind != TokStrLit {
		t.Errorf("expected TokStrLit, got %v", tokens[0].Kind)
	}
}

// TestProbeE_LexerUnterminatedStringNewline verifies that a newline inside
// a string terminates it as unterminated.
func TestProbeE_LexerUnterminatedStringNewline(t *testing.T) {
	src := span.NewSource("test", "\"hello\nworld\"")
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for string with embedded newline")
	}
	// The lexer should break on newline, producing a partial string token.
	foundStr := false
	for _, tok := range tokens {
		if tok.Kind == TokStrLit {
			foundStr = true
			if strings.Contains(tok.Text, "\n") {
				t.Error("string token should not contain the newline")
			}
		}
	}
	if !foundStr {
		t.Error("expected at least one TokStrLit token")
	}
}

// TestProbeE_LexerUnknownEscapeSequence verifies the lexer reports unknown escapes.
func TestProbeE_LexerUnknownEscapeSequence(t *testing.T) {
	src := span.NewSource("test", `"\q"`)
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for unknown escape \\q")
	}
	// The token text should contain 'q' (the escaped char is passed through).
	if tokens[0].Kind != TokStrLit || tokens[0].Text != "q" {
		t.Errorf("expected TokStrLit with text 'q', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

// TestProbeE_LexerNullByteInSource verifies null bytes don't cause panics.
func TestProbeE_LexerNullByteInSource(t *testing.T) {
	src := span.NewSource("test", "main\x00 := 42")
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	// Should not panic. The null byte is an unknown character — it should be skipped.
	if len(tokens) < 2 {
		t.Fatalf("expected tokens, got %d", len(tokens))
	}
}

// TestProbeE_LexerEscapeNull verifies \0 escape produces a null byte.
func TestProbeE_LexerEscapeNull(t *testing.T) {
	src := span.NewSource("test", `"\0"`)
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if tokens[0].Text != "\x00" {
		t.Errorf("expected null byte, got %q", tokens[0].Text)
	}
}

// TestProbeE_LexerVeryLongIdentifier verifies lexing a very long identifier.
func TestProbeE_LexerVeryLongIdentifier(t *testing.T) {
	name := strings.Repeat("a", 10000)
	source := name + " := 42"
	src := span.NewSource("test", source)
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	if tokens[0].Kind != TokLower || tokens[0].Text != name {
		t.Errorf("long identifier not preserved correctly")
	}
}

// TestProbeE_LexerVeryLongOperator verifies lexing a very long operator.
func TestProbeE_LexerVeryLongOperator(t *testing.T) {
	op := strings.Repeat("+", 5000)
	source := "x " + op + " y"
	src := span.NewSource("test", source)
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	found := false
	for _, tok := range tokens {
		if tok.Kind == TokOp && tok.Text == op {
			found = true
		}
	}
	if !found {
		t.Error("long operator not found in token stream")
	}
}

// TestProbeE_LexerUnicodeUpperStart verifies that unicode uppercase letters
// are NOT accepted as uppercase identifier starts (only ASCII A-Z).
func TestProbeE_LexerUnicodeUpperStart(t *testing.T) {
	// U+00C0 = 'A' with grave accent - unicode letter but not ASCII upper
	src := span.NewSource("test", "\u00C0bc := 42")
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	// The lexer should treat this as an unknown character since isLowerStart
	// checks 'a'-'z' | '_' and isUpperStart checks 'A'-'Z'.
	// So U+00C0 should trigger "unexpected character".
	if !es.HasErrors() {
		// If no error, the lexer might handle it — check the token kind.
		if tokens[0].Kind == TokUpper {
			t.Log("NOTE: Unicode uppercase letter accepted as TokUpper — may be design intent")
		}
	}
}

// TestProbeE_LexerUnterminatedBlockComment verifies unterminated block comment handling.
func TestProbeE_LexerUnterminatedBlockComment(t *testing.T) {
	src := span.NewSource("test", "x {- this is never closed")
	l := NewLexer(src)
	_, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for unterminated block comment")
	}
}

// TestProbeE_LexerNestedBlockCommentMismatch verifies deeply nested block comments.
func TestProbeE_LexerNestedBlockCommentMismatch(t *testing.T) {
	// Open 3 levels, close only 2.
	src := span.NewSource("test", "x {- {- {- inner -} -} still open")
	l := NewLexer(src)
	_, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for mismatched nested block comment")
	}
}

// TestProbeE_LexerEmptyRuneLiteral verifies ” is handled correctly.
func TestProbeE_LexerEmptyRuneLiteral(t *testing.T) {
	src := span.NewSource("test", "''")
	l := NewLexer(src)
	_, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for empty rune literal")
	}
}

// TestProbeE_LexerRuneUnterminated verifies an unterminated rune literal.
func TestProbeE_LexerRuneUnterminated(t *testing.T) {
	src := span.NewSource("test", "'a")
	l := NewLexer(src)
	_, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for unterminated rune literal")
	}
}

// TestProbeE_LexerRuneMultiChar verifies multi-character rune literal.
// In Haskell, 'ab' is invalid. Check behavior.
func TestProbeE_LexerRuneMultiChar(t *testing.T) {
	src := span.NewSource("test", "'ab'")
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	// The lexer reads one rune 'a', then expects closing '.
	// It will find 'b' instead, which means unterminated.
	if !es.HasErrors() {
		t.Log("NOTE: multi-char rune literal did not produce error — need to verify")
	}
	_ = tokens
}

// TestProbeE_LexerBinaryData verifies the lexer handles binary data gracefully.
func TestProbeE_LexerBinaryData(t *testing.T) {
	// Feed raw binary data.
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	src := span.NewSource("test", string(data))
	l := NewLexer(src)
	_, _ = l.Tokenize()
	// No panic is the success criterion.
}

// TestProbeE_LexerOnlyOperators verifies a source of only operators.
func TestProbeE_LexerOnlyOperators(t *testing.T) {
	src := span.NewSource("test", "++ -- ** // <<")
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	// -- starts a line comment, so only ++ should appear, then EOF
	if tokens[0].Kind != TokOp || tokens[0].Text != "++" {
		t.Errorf("expected TokOp '++', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
	// After --, everything till EOL is consumed by line comment.
	if tokens[1].Kind != TokEOF {
		t.Errorf("expected EOF after line comment, got %v %q", tokens[1].Kind, tokens[1].Text)
	}
}

// TestProbeE_LexerConsecutiveStrings verifies consecutive string literals.
func TestProbeE_LexerConsecutiveStrings(t *testing.T) {
	src := span.NewSource("test", `"a""b""c"`)
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	count := 0
	for _, tok := range tokens {
		if tok.Kind == TokStrLit {
			count++
		}
	}
	if count != 3 {
		t.Errorf("expected 3 string literals, got %d", count)
	}
}

// TestProbeE_LexerEqColonAmbiguity probes the =: vs = : distinction.
// =: is TokEqColon, but = followed by : should be TokEq + TokColon.
func TestProbeE_LexerEqColonAmbiguity(t *testing.T) {
	// =: should be a single TokEqColon token.
	tokens1 := lex("=:")
	if tokens1[0].Kind != TokEqColon {
		t.Errorf("expected TokEqColon for '=:', got %v", tokens1[0].Kind)
	}

	// = : (with space) should be TokEq + TokColon.
	tokens2 := lex("= :")
	if tokens2[0].Kind != TokEq {
		t.Errorf("expected TokEq for '= :', got %v", tokens2[0].Kind)
	}
	if tokens2[1].Kind != TokColon {
		t.Errorf("expected TokColon for '= :', got %v", tokens2[1].Kind)
	}
}

// TestProbeE_LexerColonEqVsColonColonEq probes :: vs := disambiguation.
func TestProbeE_LexerColonColonEq(t *testing.T) {
	// ::= should lex as TokColonColon then TokEq? or TokColon then TokColonEq?
	tokens := lex("::=")
	// The lexer checks :: first (2 chars), then = is a standalone TokEq.
	if tokens[0].Kind != TokColonColon {
		t.Errorf("expected TokColonColon, got %v %q", tokens[0].Kind, tokens[0].Text)
	}
	if tokens[1].Kind != TokEq {
		t.Errorf("expected TokEq, got %v %q", tokens[1].Kind, tokens[1].Text)
	}
}

// TestProbeE_LexerPipeVsPipeOp probes | vs || disambiguation.
func TestProbeE_LexerPipeVsPipeOp(t *testing.T) {
	// | alone (not followed by operator char) is TokPipe.
	tokens1 := lex("| x")
	if tokens1[0].Kind != TokPipe {
		t.Errorf("expected TokPipe for '|', got %v %q", tokens1[0].Kind, tokens1[0].Text)
	}

	// || is an operator.
	tokens2 := lex("|| x")
	if tokens2[0].Kind != TokOp || tokens2[0].Text != "||" {
		t.Errorf("expected TokOp '||', got %v %q", tokens2[0].Kind, tokens2[0].Text)
	}
}

// TestProbeE_LexerDoubleUnderscore verifies __ is an identifier, not two wildcards.
func TestProbeE_LexerDoubleUnderscore(t *testing.T) {
	tokens := lex("__")
	// _ followed by _ is isIdentCont, so __ is a lowercase identifier.
	if tokens[0].Kind != TokLower || tokens[0].Text != "__" {
		t.Errorf("expected TokLower '__', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

// TestProbeE_LexerUnderscoreAlone verifies _ alone is TokUnderscore.
func TestProbeE_LexerUnderscoreAlone(t *testing.T) {
	tokens := lex("_ x")
	if tokens[0].Kind != TokUnderscore {
		t.Errorf("expected TokUnderscore, got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

// TestProbeE_LexerIntSeparators verifies digit separators in int literals.
func TestProbeE_LexerIntSeparators(t *testing.T) {
	tokens := lex("1_000_000")
	if tokens[0].Kind != TokIntLit || tokens[0].Text != "1_000_000" {
		t.Errorf("expected TokIntLit '1_000_000', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
}

// TestProbeE_LexerDoubleLitEdge verifies floating point edge cases.
func TestProbeE_LexerDoubleLitEdge(t *testing.T) {
	// 1.e5 — the 'e' comes right after '.', but '.' is not followed by a digit,
	// so lexer sees 1 (int) then . (dot) then e5 (identifier).
	tokens := lex("1.e5")
	if tokens[0].Kind != TokIntLit || tokens[0].Text != "1" {
		t.Errorf("token[0]: expected TokIntLit '1', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
	if tokens[1].Kind != TokDot {
		t.Errorf("token[1]: expected TokDot, got %v %q", tokens[1].Kind, tokens[1].Text)
	}
}

// TestProbeE_LexerTrailingUnderscore verifies 123_ is handled.
func TestProbeE_LexerTrailingUnderscore(t *testing.T) {
	// 123_ — the underscore is a separator only if followed by a digit.
	// So this should be 123 (int) followed by _ (wildcard).
	tokens := lex("123_")
	if tokens[0].Kind != TokIntLit || tokens[0].Text != "123" {
		t.Errorf("token[0]: expected TokIntLit '123', got %v %q", tokens[0].Kind, tokens[0].Text)
	}
	if tokens[1].Kind != TokUnderscore {
		t.Errorf("token[1]: expected TokUnderscore, got %v %q", tokens[1].Kind, tokens[1].Text)
	}
}

// ----- 2. Operator Parsing: Non-Associative Chaining -----

// TestProbeE_InfixNonAssocSameOpChain verifies same infixn op cannot chain.
func TestProbeE_InfixNonAssocSameOpChain(t *testing.T) {
	source := `infixn 4 ==
(==) := \x y. x
main := 1 == 2 == 3`
	errMsg := parseMustFail(t, source)
	if !strings.Contains(errMsg, "non-associative") {
		t.Errorf("expected 'non-associative' in error, got: %s", errMsg)
	}
}

// TestProbeE_InfixNonAssocDiffOpChain verifies different infixn ops at same
// precedence cannot chain. This was the bug fixed in 4b793f9.
func TestProbeE_InfixNonAssocDiffOpChain(t *testing.T) {
	source := `infixn 4 ==
infixn 4 /=
(==) := \x y. x
(/=) := \x y. x
main := 1 == 2 /= 3`
	errMsg := parseMustFail(t, source)
	if !strings.Contains(errMsg, "non-associative") {
		t.Errorf("expected 'non-associative' in error, got: %s", errMsg)
	}
}

// TestProbeE_InfixNonAssocThenLeftAssoc verifies that after a non-assoc op,
// a left-assoc op at higher precedence is still allowed.
func TestProbeE_InfixNonAssocThenHigherPrec(t *testing.T) {
	source := `infixn 4 ==
infixl 6 +
(==) := \x y. x
(+) := \x y. x
main := 1 + 2 == 3 + 4`
	parseMustSucceed(t, source)
}

// TestProbeE_InfixNonAssocResetByLeftAssoc verifies that after a left-assoc op,
// the non-assoc tracking is reset, allowing a subsequent non-assoc op.
func TestProbeE_InfixNonAssocResetByLeftAssoc(t *testing.T) {
	// This is tricky: `1 == (2 + 3) == 4` would fail because the parenthesized
	// subexpression doesn't affect the outer chain. But what about:
	// `a + b == c` → (a + b) == c — the non-assoc is the outermost, no chain.
	source := `infixn 4 ==
infixl 6 +
(==) := \x y. x
(+) := \x y. x
main := 1 + 2 == 3`
	parseMustSucceed(t, source)
}

// BUG: medium - Non-associative chain detection does NOT propagate into
// recursive parseInfix calls. When a right-assoc op appears between two
// non-assoc ops of the same precedence, the chain is not detected because
// the recursive call starts fresh with prevNonePrec = -1.
//
// Example: `1 == 2 $ 3 /= 4` where $ is infixr 0 and ==, /= are infixn 4.
// This parses as `1 == (2 $ (3 /= 4))` — the /= is in a nested parseInfix
// call and doesn't see the outer ==. But is this actually a bug?
// In Haskell, this is allowed because operator precedence naturally nests.
// It's the SAME precedence level that's the problem.
func TestProbeE_InfixNonAssocWithRightAssocBetween(t *testing.T) {
	source := `infixn 4 ==
infixn 4 /=
infixr 0 $
(==) := \x y. x
(/=) := \x y. x
($) := \x y. x
main := 1 == 2 $ 3 /= 4`
	// This should parse: 1 == (2 $ (3 /= 4)) — the two infixn ops are
	// at different nesting levels, so it's OK.
	parseMustSucceed(t, source)
}

// TestProbeE_InfixNonAssocLowerPrecFollowedByHigher verifies that infixn at
// low prec followed by infixn at high prec works (different prec levels).
func TestProbeE_InfixNonAssocDiffPrec(t *testing.T) {
	source := `infixn 4 ==
infixn 6 %%
(==) := \x y. x
(%%) := \x y. x
main := 1 %% 2 == 3`
	// 1 %% 2 binds tighter, so it's (1 %% 2) == 3, which is a single non-assoc.
	parseMustSucceed(t, source)
}

// TestProbeE_InfixAllAssocMixed verifies mixed associativity at same precedence.
func TestProbeE_InfixMixedAssocSamePrec(t *testing.T) {
	// Left-assoc and right-assoc at same precedence should be problematic:
	// Haskell rejects this, but GICEL might allow it (left-assoc wins in Pratt).
	source := `infixl 6 +
infixr 6 ++
(+) := \x y. x
(++) := \x y. x
main := 1 + 2 ++ 3`
	// In a Pratt parser, left-assoc uses nextMin = prec+1, right-assoc uses nextMin = prec.
	// When the outer loop sees + at prec 6 (left), it calls parseInfix(7).
	// Then it sees ++ at prec 6 (right), but 6 < 7, so it stops.
	// Result: (1 + 2) ++ ... — but wait, the outer loop continues and sees ++.
	// Actually, + consumes the right operand with parseInfix(7), which sees ++ at 6 < 7,
	// so ++ is not consumed there. The outer loop then sees ++ at 6 >= 0 (minPrec),
	// and consumes it with parseInfix(6) (right-assoc). This gives (1 + 2) ++ 3.
	// This is legal in Pratt parsing — no error expected.
	_, es := parse(source)
	// Record: does it error or succeed? Both are defensible.
	_ = es
}

// ----- 3. Operator Section Edge Cases -----

// TestProbeE_LeftSectionInParen verifies (expr op) is a left section.
func TestProbeE_LeftSectionInParen(t *testing.T) {
	source := `infixl 6 +
(+) := \x y. x
main := (1 +)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if sec.IsRight {
		t.Error("expected left section (IsRight=false)")
	}
	if sec.Op != "+" {
		t.Errorf("expected op '+', got %q", sec.Op)
	}
}

// TestProbeE_RightSectionInParen verifies (op expr) is a right section.
func TestProbeE_RightSectionInParen(t *testing.T) {
	source := `infixl 6 +
(+) := \x y. x
main := (+ 1)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if !sec.IsRight {
		t.Error("expected right section (IsRight=true)")
	}
}

// TestProbeE_SectionWithDot verifies (. expr) and (expr .) work.
func TestProbeE_SectionWithDot(t *testing.T) {
	source := `main := (. f)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection for (. f), got %T", d.Expr)
	}
	if sec.Op != "." {
		t.Errorf("expected op '.', got %q", sec.Op)
	}
}

// TestProbeE_OperatorAsValueInParen verifies (op) is an ExprVar.
func TestProbeE_OperatorAsValueInParen(t *testing.T) {
	source := `infixl 6 +
(+) := \x y. x
main := (+)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	v, ok := d.Expr.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar for (+), got %T", d.Expr)
	}
	if v.Name != "+" {
		t.Errorf("expected name '+', got %q", v.Name)
	}
}

// TestProbeE_NestedSections verifies nested sections like ((+) 1) — not a section,
// just function application of the operator value.
func TestProbeE_NestedSections(t *testing.T) {
	source := `infixl 6 +
(+) := \x y. x
main := ((+) 1)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	// Should be ExprParen { ExprApp { ExprVar(+), ExprIntLit(1) } }
	paren, ok := d.Expr.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen, got %T", d.Expr)
	}
	app, ok := paren.Inner.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp inside, got %T", paren.Inner)
	}
	v, ok := app.Fun.(*ExprVar)
	if !ok {
		t.Fatalf("expected ExprVar as function, got %T", app.Fun)
	}
	if v.Name != "+" {
		t.Errorf("expected '+', got %q", v.Name)
	}
}

// TestProbeE_EmptyParens verifies () is unit (empty record).
func TestProbeE_EmptyParens(t *testing.T) {
	source := `main := ()`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for (), got %T", d.Expr)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields for (), got %d", len(rec.Fields))
	}
}

// ----- 4. Declaration Parsing Edge Cases -----

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
		t.Error("class C not found in AST")
	}
}

// TestProbeE_ClassDeclNoBody verifies `class C a` (missing braces) errors.
func TestProbeE_ClassDeclNoBody(t *testing.T) {
	_, es := parse("class C a")
	if !es.HasErrors() {
		t.Error("expected error for class decl without body")
	}
}

// TestProbeE_InstanceDeclEmptyBody verifies `instance C Int {}` parses.
func TestProbeE_InstanceDeclEmptyBody(t *testing.T) {
	source := `class C a { m :: a -> a }
instance C Int {}`
	// The instance body is empty — this is syntactically valid.
	// The checker may reject missing methods.
	parseMustSucceed(t, source)
}

// TestProbeE_FixityDeclNoOp verifies `infixl 6` without operator errors.
func TestProbeE_FixityDeclNoOp(t *testing.T) {
	_, es := parse("infixl 6")
	if !es.HasErrors() {
		t.Error("expected error for fixity decl without operator")
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

// TestProbeE_DuplicateDecls verifies the parser handles duplicate declarations
// without crashing (semantic check is the checker's job).
func TestProbeE_DuplicateDecls(t *testing.T) {
	source := `x := 1
x := 2`
	prog := parseMustSucceed(t, source)
	count := 0
	for _, d := range prog.Decls {
		if vd, ok := d.(*DeclValueDef); ok && vd.Name == "x" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 value defs for x, got %d", count)
	}
}

// TestProbeE_TypeAnnWithoutValue verifies a type annotation without a corresponding
// value definition is syntactically valid.
func TestProbeE_TypeAnnWithoutValue(t *testing.T) {
	source := `f :: Int -> Int`
	parseMustSucceed(t, source)
}

// TestProbeE_NamedDeclExpectedColonColon verifies error on malformed named decl.
func TestProbeE_NamedDeclExpectedColonColon(t *testing.T) {
	// `f = 1` — should expect := not =
	_, es := parse("f = 1")
	if !es.HasErrors() {
		t.Error("expected error: 'f = 1' should require ':=' not '='")
	}
}

// TestProbeE_GADTDeclBasic verifies basic GADT syntax.
// Note: inside braces, semicolons are required between GADT constructors.
func TestProbeE_GADTDeclBasic(t *testing.T) {
	source := `data Expr a := {
  LitI :: Int -> Expr Int;
  LitB :: Bool -> Expr Bool
}`
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclData); ok && dd.Name == "Expr" {
			if len(dd.GADTCons) != 2 {
				t.Errorf("expected 2 GADT cons, got %d", len(dd.GADTCons))
			}
		}
	}
}

// TestProbeE_GADTEmptyBody verifies `data T := {}` parses with 0 GADT constructors.
func TestProbeE_GADTEmptyBody(t *testing.T) {
	source := `data T := {}`
	prog := parseMustSucceed(t, source)
	for _, d := range prog.Decls {
		if dd, ok := d.(*DeclData); ok && dd.Name == "T" {
			if len(dd.GADTCons) != 0 {
				t.Errorf("expected 0 GADT cons, got %d", len(dd.GADTCons))
			}
		}
	}
}

// ----- 5. Pattern Parsing Edge Cases -----

// TestProbeE_DeeplyNestedPattern verifies deeply nested constructor patterns
// don't cause stack overflow.
func TestProbeE_DeeplyNestedPattern(t *testing.T) {
	// Build: Just (Just (Just ... (Just x) ...))
	pat := "x"
	for i := 0; i < 100; i++ {
		pat = fmt.Sprintf("(Just %s)", pat)
	}
	source := fmt.Sprintf(`data Maybe a := Nothing | Just a
f := case Nothing { %s -> 1 }`, pat)
	_, es := parse(source)
	// Should not panic. May hit recursion limit — that's acceptable.
	_ = es
}

// TestProbeE_WildcardAsConstructorArg verifies `Just _` pattern.
func TestProbeE_WildcardAsConstructorArg(t *testing.T) {
	source := `data Maybe a := Nothing | Just a
f := case Nothing { Just _ -> 1; Nothing -> 0 }`
	parseMustSucceed(t, source)
}

// TestProbeE_NestedTuplePattern verifies ((x, y), z) pattern.
func TestProbeE_NestedTuplePattern(t *testing.T) {
	source := `main := case (1, 2, 3) { (x, y, z) -> x }`
	parseMustSucceed(t, source)
}

// TestProbeE_PatternLiterals verifies literal patterns.
func TestProbeE_PatternLiterals(t *testing.T) {
	source := `f := case 42 { 0 -> "zero"; 42 -> "answer"; _ -> "other" }`
	parseMustSucceed(t, source)
}

// TestProbeE_RecordPattern verifies record pattern matching.
func TestProbeE_RecordPattern(t *testing.T) {
	source := `f := case () { {x: a, y: b} -> a }`
	parseMustSucceed(t, source)
}

// TestProbeE_EmptyRecordPattern verifies {} pattern (unit pattern).
func TestProbeE_EmptyRecordPattern(t *testing.T) {
	source := `f := case () { {} -> 1 }`
	parseMustSucceed(t, source)
}

// TestProbeE_PatternParenUnit verifies () pattern.
func TestProbeE_PatternParenUnit(t *testing.T) {
	source := `f := case () { () -> 1 }`
	parseMustSucceed(t, source)
}

// ----- 6. Do-Notation Edge Cases -----

// TestProbeE_DoBlockEmpty verifies `do {}` is syntactically valid.
func TestProbeE_DoBlockEmpty(t *testing.T) {
	source := `main := do {}`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr, ok := d.Expr.(*ExprDo)
	if !ok {
		t.Fatalf("expected ExprDo, got %T", d.Expr)
	}
	if len(doExpr.Stmts) != 0 {
		t.Errorf("expected 0 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeE_DoBlockOnlyBind verifies `do { x <- expr }`.
func TestProbeE_DoBlockOnlyBind(t *testing.T) {
	source := `main := do { x <- pure 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(doExpr.Stmts))
	}
	bind, ok := doExpr.Stmts[0].(*StmtBind)
	if !ok {
		t.Fatalf("expected StmtBind, got %T", doExpr.Stmts[0])
	}
	if bind.Var != "x" {
		t.Errorf("expected var 'x', got %q", bind.Var)
	}
}

// TestProbeE_DoBlockOnlyPureBind verifies `do { x := expr }`.
func TestProbeE_DoBlockOnlyPureBind(t *testing.T) {
	source := `main := do { x := 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(doExpr.Stmts))
	}
	_, ok := doExpr.Stmts[0].(*StmtPureBind)
	if !ok {
		t.Fatalf("expected StmtPureBind, got %T", doExpr.Stmts[0])
	}
}

// TestProbeE_DoBlockOnlyExpr verifies `do { expr }`.
func TestProbeE_DoBlockOnlyExpr(t *testing.T) {
	source := `main := do { pure 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(doExpr.Stmts))
	}
	_, ok := doExpr.Stmts[0].(*StmtExpr)
	if !ok {
		t.Fatalf("expected StmtExpr, got %T", doExpr.Stmts[0])
	}
}

// TestProbeE_DoBlockWildcardBind verifies `do { _ <- expr }`.
func TestProbeE_DoBlockWildcardBind(t *testing.T) {
	source := `main := do { _ <- pure 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 1 {
		t.Fatalf("expected 1 stmt, got %d", len(doExpr.Stmts))
	}
	bind, ok := doExpr.Stmts[0].(*StmtBind)
	if !ok {
		t.Fatalf("expected StmtBind, got %T", doExpr.Stmts[0])
	}
	if bind.Var != "_" {
		t.Errorf("expected var '_', got %q", bind.Var)
	}
}

// TestProbeE_DoBlockNestedDo verifies nested do blocks.
func TestProbeE_DoBlockNestedDo(t *testing.T) {
	source := `main := do {
  x <- do {
    pure 1
  }
  pure x
}`
	parseMustSucceed(t, source)
}

// TestProbeE_DoBlockSemicolonSeparated verifies semicolons as statement separators.
func TestProbeE_DoBlockSemicolonSeparated(t *testing.T) {
	source := `main := do { x <- pure 1; y <- pure 2; pure x }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	doExpr := d.Expr.(*ExprDo)
	if len(doExpr.Stmts) != 3 {
		t.Errorf("expected 3 stmts, got %d", len(doExpr.Stmts))
	}
}

// TestProbeE_DoBlockMissingBrace verifies do without closing brace errors.
func TestProbeE_DoBlockMissingBrace(t *testing.T) {
	_, es := parse("main := do { pure 1")
	if !es.HasErrors() {
		t.Error("expected error for missing closing brace in do")
	}
}

// ----- 7. Import Parsing Edge Cases -----

// TestProbeE_ImportBasic verifies basic import.
func TestProbeE_ImportBasic(t *testing.T) {
	prog := parseMustSucceed(t, "import Prelude\nmain := 1")
	if len(prog.Imports) != 1 {
		t.Fatalf("expected 1 import, got %d", len(prog.Imports))
	}
	if prog.Imports[0].ModuleName != "Prelude" {
		t.Errorf("expected Prelude, got %s", prog.Imports[0].ModuleName)
	}
}

// TestProbeE_ImportQualified verifies qualified import.
func TestProbeE_ImportQualified(t *testing.T) {
	prog := parseMustSucceed(t, "import Prelude as P\nmain := 1")
	if prog.Imports[0].Alias != "P" {
		t.Errorf("expected alias P, got %s", prog.Imports[0].Alias)
	}
}

// TestProbeE_ImportSelective verifies selective import.
func TestProbeE_ImportSelective(t *testing.T) {
	prog := parseMustSucceed(t, "import Prelude (map, Bool(..))\nmain := 1")
	if len(prog.Imports[0].Names) != 2 {
		t.Fatalf("expected 2 import names, got %d", len(prog.Imports[0].Names))
	}
	if prog.Imports[0].Names[0].Name != "map" {
		t.Errorf("expected 'map', got %s", prog.Imports[0].Names[0].Name)
	}
	if !prog.Imports[0].Names[1].AllSubs {
		t.Error("expected Bool(..) to have AllSubs=true")
	}
}

// TestProbeE_ImportSelectiveEmpty verifies import M () — empty selective list.
func TestProbeE_ImportSelectiveEmpty(t *testing.T) {
	prog := parseMustSucceed(t, "import M ()\nmain := 1")
	if prog.Imports[0].Names == nil {
		t.Error("expected non-nil Names for import M ()")
	}
	if len(prog.Imports[0].Names) != 0 {
		t.Errorf("expected 0 import names, got %d", len(prog.Imports[0].Names))
	}
}

// TestProbeE_ImportOperator verifies importing operators.
func TestProbeE_ImportOperator(t *testing.T) {
	prog := parseMustSucceed(t, "import Prelude ((+))\nmain := 1")
	if len(prog.Imports[0].Names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(prog.Imports[0].Names))
	}
	if prog.Imports[0].Names[0].Name != "+" {
		t.Errorf("expected '+', got %s", prog.Imports[0].Names[0].Name)
	}
}

// TestProbeE_ImportDottedModule verifies dotted module names.
func TestProbeE_ImportDottedModule(t *testing.T) {
	prog := parseMustSucceed(t, "import Std.Num\nmain := 1")
	if prog.Imports[0].ModuleName != "Std.Num" {
		t.Errorf("expected Std.Num, got %s", prog.Imports[0].ModuleName)
	}
}

// TestProbeE_ImportMalformedModuleName verifies import with lowercase module name errors.
func TestProbeE_ImportMalformedModuleName(t *testing.T) {
	_, es := parse("import prelude\nmain := 1")
	if !es.HasErrors() {
		t.Error("expected error for lowercase module name")
	}
}

// TestProbeE_ImportNoModuleName verifies `import` alone errors.
func TestProbeE_ImportNoModuleName(t *testing.T) {
	_, es := parse("import\nmain := 1")
	if !es.HasErrors() {
		t.Error("expected error for import without module name")
	}
}

// TestProbeE_ImportDuplicates verifies duplicate imports are syntactically valid.
func TestProbeE_ImportDuplicates(t *testing.T) {
	source := `import Prelude
import Prelude
main := 1`
	prog := parseMustSucceed(t, source)
	if len(prog.Imports) != 2 {
		t.Errorf("expected 2 imports, got %d", len(prog.Imports))
	}
}

// TestProbeE_ImportSelectiveSubList verifies import M (T(A, B)).
func TestProbeE_ImportSelectiveSubList(t *testing.T) {
	prog := parseMustSucceed(t, "import M (Maybe(Nothing, Just))\nmain := 1")
	names := prog.Imports[0].Names
	if len(names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(names))
	}
	if !names[0].HasSub {
		t.Error("expected HasSub=true")
	}
	if names[0].AllSubs {
		t.Error("expected AllSubs=false for explicit sub-list")
	}
	if len(names[0].SubList) != 2 {
		t.Errorf("expected 2 subs, got %d", len(names[0].SubList))
	}
}

// TestProbeE_ImportAfterDecl verifies that imports after declarations are NOT parsed as imports.
// The parser collects imports first, then declarations.
func TestProbeE_ImportAfterDecl(t *testing.T) {
	source := `main := 1
import Prelude`
	prog, es := parse(source)
	// The second import should be parsed as a declaration, which will fail.
	// Imports are only collected before the first non-import declaration.
	if len(prog.Imports) != 0 {
		t.Errorf("expected 0 imports (import after decl), got %d", len(prog.Imports))
	}
	if !es.HasErrors() {
		t.Error("expected error for import after declaration")
	}
}

// ----- 8. Error Recovery and Crash Resistance -----

// TestProbeE_CrashDeepNestedParens verifies deep paren nesting doesn't overflow.
func TestProbeE_CrashDeepNestedParens(t *testing.T) {
	var b strings.Builder
	depth := 500
	for i := 0; i < depth; i++ {
		b.WriteByte('(')
	}
	b.WriteString("x")
	for i := 0; i < depth; i++ {
		b.WriteByte(')')
	}
	source := fmt.Sprintf("main := %s", b.String())
	_, es := parse(source)
	// Should not panic. May produce recursion limit error.
	_ = es
}

// TestProbeE_CrashDeepNestedTypes verifies deep type nesting doesn't overflow.
func TestProbeE_CrashDeepNestedTypes(t *testing.T) {
	var b strings.Builder
	depth := 500
	for i := 0; i < depth; i++ {
		b.WriteString("(")
	}
	b.WriteString("Int")
	for i := 0; i < depth; i++ {
		b.WriteString(")")
	}
	source := fmt.Sprintf("x :: %s\nx := 1", b.String())
	_, es := parse(source)
	_ = es
}

// TestProbeE_CrashDeepNestedCase verifies deeply nested case expressions.
func TestProbeE_CrashDeepNestedCase(t *testing.T) {
	// case x { y -> case x { y -> ... } }
	source := "main := "
	for i := 0; i < 50; i++ {
		source += "case x { y -> "
	}
	source += "1"
	for i := 0; i < 50; i++ {
		source += " }"
	}
	_, es := parse(source)
	_ = es
}

// TestProbeE_CrashDeepNestedDo verifies deeply nested do blocks.
func TestProbeE_CrashDeepNestedDo(t *testing.T) {
	source := "main := "
	for i := 0; i < 50; i++ {
		source += "do { x <- "
	}
	source += "pure 1"
	for i := 0; i < 50; i++ {
		source += " }"
	}
	_, es := parse(source)
	_ = es
}

// TestProbeE_CrashDeepNestedLambda verifies deeply nested lambdas.
func TestProbeE_CrashDeepNestedLambda(t *testing.T) {
	source := "main := "
	for i := 0; i < 100; i++ {
		source += "\\x. "
	}
	source += "x"
	_, es := parse(source)
	_ = es
}

// TestProbeE_CrashUnbalancedParens verifies mismatched parens don't cause infinite loop.
func TestProbeE_CrashUnbalancedParens(t *testing.T) {
	sources := []string{
		"main := (((",
		"main := )))",
		"main := ((())",
		"main := ())",
		"main := ({)}",
		"main := do { ( }",
	}
	for _, src := range sources {
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
	keywords := []string{"data", "type", "class", "instance", "import", "infixl", "infixr", "infixn"}
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

// ----- 9. Block Expression / Record Ambiguity -----

// TestProbeE_BlockExprSimple verifies block expression parsing.
func TestProbeE_BlockExprSimple(t *testing.T) {
	source := `main := { x := 1; x }`
	parseMustSucceed(t, source)
}

// TestProbeE_RecordLiteralSimple verifies record literal parsing.
func TestProbeE_RecordLiteralSimple(t *testing.T) {
	source := `main := { x: 1, y: 2 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord, got %T", d.Expr)
	}
	if len(rec.Fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(rec.Fields))
	}
}

// TestProbeE_EmptyRecord verifies `{}` is an empty record.
func TestProbeE_EmptyRecord(t *testing.T) {
	source := `main := {}`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for {}, got %T", d.Expr)
	}
	if len(rec.Fields) != 0 {
		t.Errorf("expected 0 fields for {}, got %d", len(rec.Fields))
	}
}

// TestProbeE_RecordUpdateBasic verifies record update syntax.
func TestProbeE_RecordUpdateBasic(t *testing.T) {
	source := `main := { r | x: 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	_, ok := d.Expr.(*ExprRecordUpdate)
	if !ok {
		t.Fatalf("expected ExprRecordUpdate, got %T", d.Expr)
	}
}

// ----- 10. Tuple Expression Edge Cases -----

// TestProbeE_TupleSingleton verifies (x) is just grouping, not a 1-tuple.
func TestProbeE_TupleSingleton(t *testing.T) {
	source := `main := (42)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	paren, ok := d.Expr.(*ExprParen)
	if !ok {
		t.Fatalf("expected ExprParen for (42), got %T", d.Expr)
	}
	_, ok = paren.Inner.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit inside parens, got %T", paren.Inner)
	}
}

// TestProbeE_TuplePair verifies (x, y) is a record with _1, _2.
func TestProbeE_TuplePair(t *testing.T) {
	source := `main := (1, 2)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	rec, ok := d.Expr.(*ExprRecord)
	if !ok {
		t.Fatalf("expected ExprRecord for (1, 2), got %T", d.Expr)
	}
	if len(rec.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(rec.Fields))
	}
	if rec.Fields[0].Label != "_1" || rec.Fields[1].Label != "_2" {
		t.Errorf("expected _1, _2; got %q, %q", rec.Fields[0].Label, rec.Fields[1].Label)
	}
}

// TestProbeE_TupleTrailingComma verifies (x, y,) behavior.
func TestProbeE_TupleTrailingComma(t *testing.T) {
	source := `main := (1, 2,)`
	_, es := parse(source)
	// Trailing comma: parseExpr will try to parse `)` as expr after the comma.
	// This should produce an error.
	if !es.HasErrors() {
		t.Error("expected error for trailing comma in tuple")
	}
}

// ----- 11. List Literal Edge Cases -----

// TestProbeE_ListEmpty verifies [].
func TestProbeE_ListEmpty(t *testing.T) {
	source := `main := []`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	lst, ok := d.Expr.(*ExprList)
	if !ok {
		t.Fatalf("expected ExprList, got %T", d.Expr)
	}
	if len(lst.Elems) != 0 {
		t.Errorf("expected 0 elements, got %d", len(lst.Elems))
	}
}

// TestProbeE_ListSingleton verifies [x].
func TestProbeE_ListSingleton(t *testing.T) {
	source := `main := [1]`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	lst := d.Expr.(*ExprList)
	if len(lst.Elems) != 1 {
		t.Errorf("expected 1 element, got %d", len(lst.Elems))
	}
}

// TestProbeE_ListMultiple verifies [a, b, c].
func TestProbeE_ListMultiple(t *testing.T) {
	source := `main := [1, 2, 3]`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	lst := d.Expr.(*ExprList)
	if len(lst.Elems) != 3 {
		t.Errorf("expected 3 elements, got %d", len(lst.Elems))
	}
}

// TestProbeE_ListTrailingComma verifies [a,] behavior.
func TestProbeE_ListTrailingComma(t *testing.T) {
	source := `main := [1,]`
	_, es := parse(source)
	// After the comma, parseExpr sees ']' which is not an atom start.
	// This should produce an error.
	if !es.HasErrors() {
		t.Error("expected error for trailing comma in list")
	}
}

// ----- 12. Lambda Edge Cases -----

// TestProbeE_LambdaNoParams verifies `\. expr` — lambda with no params.
func TestProbeE_LambdaNoParams(t *testing.T) {
	source := `main := \. 42`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	lam, ok := d.Expr.(*ExprLam)
	if !ok {
		t.Fatalf("expected ExprLam, got %T", d.Expr)
	}
	if len(lam.Params) != 0 {
		t.Errorf("expected 0 params, got %d", len(lam.Params))
	}
}

// TestProbeE_LambdaMissingBody verifies `\x.` with missing body errors.
func TestProbeE_LambdaMissingBody(t *testing.T) {
	_, es := parse(`main := \x.`)
	if !es.HasErrors() {
		t.Error("expected error for lambda with missing body")
	}
}

// TestProbeE_LambdaMissingDot verifies `\x x` with missing dot errors.
func TestProbeE_LambdaMissingDot(t *testing.T) {
	// \x x — the parser expects a dot after params. 'x' is a pattern atom,
	// so it consumes both x's as params, then expects dot, finds EOF.
	_, es := parse(`main := \x y`)
	if !es.HasErrors() {
		t.Error("expected error for lambda with missing dot")
	}
}

// ----- 13. Case Expression Edge Cases -----

// TestProbeE_CaseEmptyAlts verifies `case x {}` — case with no alternatives.
func TestProbeE_CaseEmptyAlts(t *testing.T) {
	source := `main := case x {}`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	caseExpr, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	if len(caseExpr.Alts) != 0 {
		t.Errorf("expected 0 alts, got %d", len(caseExpr.Alts))
	}
}

// TestProbeE_CaseMissingArrow verifies case with missing -> in alternative.
func TestProbeE_CaseMissingArrow(t *testing.T) {
	_, es := parse(`main := case x { y 1 }`)
	if !es.HasErrors() {
		t.Error("expected error for missing -> in case alt")
	}
}

// TestProbeE_CaseNoBraces verifies case without braces errors.
func TestProbeE_CaseNoBraces(t *testing.T) {
	_, es := parse(`main := case x y -> 1`)
	if !es.HasErrors() {
		t.Error("expected error for case without braces")
	}
}

// ----- 14. Type Annotation Edge Cases -----

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

// ----- 15. Record Projection Edge Cases -----

// TestProbeE_RecordProjection verifies r.#field syntax.
func TestProbeE_RecordProjection(t *testing.T) {
	source := `main := r.#x`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	proj, ok := d.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected ExprProject, got %T", d.Expr)
	}
	if proj.Label != "x" {
		t.Errorf("expected label 'x', got %q", proj.Label)
	}
}

// TestProbeE_RecordProjectionChain verifies r.#x.#y chaining.
func TestProbeE_RecordProjectionChain(t *testing.T) {
	source := `main := r.#x.#y`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	outer, ok := d.Expr.(*ExprProject)
	if !ok {
		t.Fatalf("expected outer ExprProject, got %T", d.Expr)
	}
	if outer.Label != "y" {
		t.Errorf("expected outer label 'y', got %q", outer.Label)
	}
	inner, ok := outer.Record.(*ExprProject)
	if !ok {
		t.Fatalf("expected inner ExprProject, got %T", outer.Record)
	}
	if inner.Label != "x" {
		t.Errorf("expected inner label 'x', got %q", inner.Label)
	}
}

// ----- 16. Row Type / Record Type Edge Cases -----

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

// ----- 17. Qualified Name Edge Cases -----

// TestProbeE_QualifiedVarWithSpace verifies M . x (spaces) is composition, not qualified.
func TestProbeE_QualifiedVarWithSpace(t *testing.T) {
	source := `main := M . x`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	// Should be: ExprInfix { Left: ExprCon(M), Op: ".", Right: ExprVar(x) }
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix for 'M . x', got %T", d.Expr)
	}
	if inf.Op != "." {
		t.Errorf("expected op '.', got %q", inf.Op)
	}
}

// TestProbeE_QualifiedConAdjacent verifies M.Just (no spaces) is qualified con.
func TestProbeE_QualifiedConAdjacent(t *testing.T) {
	source := `main := M.Just`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	qc, ok := d.Expr.(*ExprQualCon)
	if !ok {
		t.Fatalf("expected ExprQualCon, got %T", d.Expr)
	}
	if qc.Qualifier != "M" || qc.Name != "Just" {
		t.Errorf("expected M.Just, got %s.%s", qc.Qualifier, qc.Name)
	}
}

// ----- 18. Stress: Parser Step Limit -----

// TestProbeE_StepLimitLongInfixChain verifies the parser handles extremely long
// infix chains within the step limit.
func TestProbeE_StepLimitLongInfixChain(t *testing.T) {
	var b strings.Builder
	b.WriteString("infixl 6 +\n(+) := \\x y. x\nmain := 1")
	for i := 0; i < 500; i++ {
		b.WriteString(" + 1")
	}
	_, es := parse(b.String())
	// May hit step limit — that's acceptable.
	_ = es
}

// TestProbeE_StepLimitLongApplicationChain verifies long application chains.
func TestProbeE_StepLimitLongApplicationChain(t *testing.T) {
	var b strings.Builder
	b.WriteString("main := f")
	for i := 0; i < 2000; i++ {
		b.WriteString(" x")
	}
	_, es := parse(b.String())
	_ = es
}

// ----- 19. Fixity Collection Edge Cases -----

// TestProbeE_FixityCollectionPrePass verifies that fixity declarations anywhere in the
// source are collected in the pre-pass before expression parsing.
func TestProbeE_FixityCollectionPrePass(t *testing.T) {
	source := `(+) := \x y. x
main := 1 + 2
infixl 6 +`
	prog := parseMustSucceed(t, source)
	// The fixity declaration comes AFTER the usage, but the pre-pass should
	// have collected it. Verify by checking the parse tree.
	d := prog.Decls[len(prog.Decls)-2].(*DeclValueDef)
	inf, ok := d.Expr.(*ExprInfix)
	if !ok {
		t.Fatalf("expected ExprInfix, got %T", d.Expr)
	}
	if inf.Op != "+" {
		t.Errorf("expected op '+', got %q", inf.Op)
	}
}

// TestProbeE_FixityOutOfRange verifies fixity with precedence > 9 or < 0.
func TestProbeE_FixityOutOfRange(t *testing.T) {
	// The parser just reads an int — it doesn't validate range.
	source := `infixl 100 ++
(++) := \x y. x
main := 1 ++ 2`
	prog, es := parse(source)
	if es.HasErrors() {
		t.Logf("errors (might be expected): %s", es.Format())
	}
	// Verify the fixity was stored even with out-of-range precedence.
	if prog != nil {
		found := false
		for _, d := range prog.Decls {
			if fix, ok := d.(*DeclFixity); ok && fix.Op == "++" {
				found = true
				if fix.Prec != 100 {
					t.Errorf("expected prec 100, got %d", fix.Prec)
				}
			}
		}
		if !found {
			t.Error("fixity decl for ++ not found")
		}
	}
}

// ----- 20. Class/Instance Declaration Edge Cases -----

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

// TestProbeE_ClassMethodOnlyKeyword verifies class body with just a keyword errors.
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

// ----- 21. Type Application Edge Cases -----

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

// ----- 22. Declaration Boundary Detection -----

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

// ----- 23. Adversarial Combinatorics -----

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

// TestProbeE_DoInsideCaseInsideLambda verifies complex nesting.
func TestProbeE_DoInsideCaseInsideLambda(t *testing.T) {
	source := `main := \x. case x {
  True -> do {
    y <- pure 1;
    pure y
  };
  False -> pure 0
}`
	parseMustSucceed(t, source)
}

// ----- 24. Pratt Parser Backtrack / Section Detection Edge Cases -----

// TestProbeE_SectionBacktrackOnComplexExpr verifies section detection backtracking
// when the expression after the operator is complex and doesn't end with ).
// The parser's right-section detection tries (op expr), and backtracks if no ).
func TestProbeE_SectionBacktrackOnComplexExpr(t *testing.T) {
	// (+ 1 2) — this could be ambiguous: is it a right section (+ (1 2))
	// or something else? The parser tries right section: op=+, expr=1 2.
	// After parsing "1 2" as App(1, 2), it checks for ). If found, it's a section.
	source := `infixl 6 +
(+) := \x y. x
main := (+ 1 2)`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[len(prog.Decls)-1].(*DeclValueDef)
	sec, ok := d.Expr.(*ExprSection)
	if !ok {
		t.Fatalf("expected ExprSection, got %T", d.Expr)
	}
	if !sec.IsRight {
		t.Error("expected right section")
	}
	// The arg should be App(1, 2).
	_, ok = sec.Arg.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp as section arg, got %T", sec.Arg)
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

// ----- 25. peekAt boundary checks in Lexer -----

// TestProbeE_LexerPeekAtBoundary verifies the lexer doesn't panic when
// multi-char tokens appear at end of source.
func TestProbeE_LexerPeekAtBoundary(t *testing.T) {
	// Each of these sources ends with the start of a multi-char token.
	// The peekAt(1) call should return 0 (EOF) gracefully.
	edgeCases := []string{
		"-", // could be start of -> or -- but just -
		"<", // could be start of <-
		":", // could be start of :: or :=
		"=", // could be start of => or =:
		".", // could be start of .#
		"{", // could be start of {-
	}
	for _, src := range edgeCases {
		t.Run(src, func(t *testing.T) {
			s := span.NewSource("test", src)
			l := NewLexer(s)
			tokens, _ := l.Tokenize()
			// Just verify no panic and at least one token.
			if len(tokens) < 1 {
				t.Error("expected at least 1 token")
			}
		})
	}
}

// ----- 26. Collectivity of fixity: does it survive the second pass? -----

// TestProbeE_FixityInClassBody verifies fixity declared inside a class body...
// Actually, fixity declarations only live at the top level. But what if someone
// writes one inside a class body?
func TestProbeE_FixityInsideClassBody(t *testing.T) {
	source := `class C a {
  infixl 6 +
}`
	_, es := parse(source)
	// The class body parser expects lower (method name), type, or data.
	// infixl is a keyword, so it should error.
	if !es.HasErrors() {
		t.Error("expected error for fixity inside class body")
	}
}

// ----- 27. Token adjacency edge cases -----

// TestProbeE_DotHashNotAdjacent verifies .# with space doesn't form TokDotHash.
func TestProbeE_DotHashNotAdjacent(t *testing.T) {
	// ". #" — two separate tokens.
	tokens := lex(". #x")
	if tokens[0].Kind != TokDot {
		t.Errorf("expected TokDot, got %v", tokens[0].Kind)
	}
	// #x is an operator token.
	if tokens[1].Kind != TokOp {
		t.Errorf("expected TokOp for '#x', got %v %q", tokens[1].Kind, tokens[1].Text)
	}
}

// ----- 28. Instance body stall detection -----

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

// TestProbeE_CaseAltStall verifies that garbage in case alternatives
// doesn't cause infinite loop.
func TestProbeE_CaseAltStall(t *testing.T) {
	source := `main := case x { + + + }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for garbage in case alts")
	}
}

// TestProbeE_DoBlockStall verifies that garbage in do block
// doesn't cause infinite loop.
func TestProbeE_DoBlockStall(t *testing.T) {
	source := `main := do { + + + }`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Error("expected error for garbage in do block")
	}
}

// ----- 29. Interaction: noBraceAtom in case scrutinee -----

// TestProbeE_CaseScrutineeNoBrace verifies that `{` is NOT an atom start
// inside a case scrutinee (noBraceAtom flag).
func TestProbeE_CaseScrutineeNoBrace(t *testing.T) {
	// `case x { ... }` — the `{` should start the case body, not be parsed
	// as a record/block expression.
	source := `main := case x { True -> 1; False -> 0 }`
	parseMustSucceed(t, source)
}

// TestProbeE_CaseScrutineeComplexExpr verifies complex scrutinee parsing.
func TestProbeE_CaseScrutineeComplexExpr(t *testing.T) {
	source := `main := case f x y { True -> 1 }`
	prog := parseMustSucceed(t, source)
	d := prog.Decls[0].(*DeclValueDef)
	caseExpr, ok := d.Expr.(*ExprCase)
	if !ok {
		t.Fatalf("expected ExprCase, got %T", d.Expr)
	}
	// The scrutinee should be App(App(f, x), y).
	app, ok := caseExpr.Scrutinee.(*ExprApp)
	if !ok {
		t.Fatalf("expected ExprApp as scrutinee, got %T", caseExpr.Scrutinee)
	}
	_ = app
}

// ----- 30. Adversarial: semicolons in various positions -----

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

// ----- 31. Instance body with associated type definitions -----

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

// ----- 32. Adversarial: parser halting -----

// TestProbeE_HaltedParserReturnsEOF verifies that once halted, all peek/advance
// return EOF. We can't easily test this directly, but we can trigger it
// via deep nesting and verify the parser still terminates.
func TestProbeE_HaltedParserReturnsEOF(t *testing.T) {
	// Create input that will trigger the step limit.
	var b strings.Builder
	b.WriteString("main := ")
	// A pattern that generates many steps: repeated error recovery.
	for i := 0; i < 5000; i++ {
		b.WriteString("( ")
	}
	src := span.NewSource("test", b.String())
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	es := &errs.Errors{Source: src}
	p := NewParser(tokens, es)
	_ = p.ParseProgram()
	// Should have halted (step or recursion limit).
	if !es.HasErrors() {
		t.Error("expected parser errors from stress input")
	}
}

// ----- 33. Edge: double-primed identifiers -----

// TestProbeE_PrimedIdentifiers verifies identifiers with primes: x', x”, etc.
func TestProbeE_PrimedIdentifiers(t *testing.T) {
	source := `x' := 1
x'' := 2
main := x'`
	prog := parseMustSucceed(t, source)
	if len(prog.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(prog.Decls))
	}
}

// ----- 34. Type family equation with conflicting name -----

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

// ----- 35. Import of dot operator -----

// TestProbeE_ImportDotOperator verifies import M ((.)) works.
func TestProbeE_ImportDotOperator(t *testing.T) {
	prog := parseMustSucceed(t, "import M ((.));\nmain := 1")
	if len(prog.Imports[0].Names) != 1 {
		t.Fatalf("expected 1 import name, got %d", len(prog.Imports[0].Names))
	}
	if prog.Imports[0].Names[0].Name != "." {
		t.Errorf("expected '.', got %q", prog.Imports[0].Names[0].Name)
	}
}

// ----- 36. Forall type with no binders -----

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
