//go:build probe

// Lexer probe tests: numeric literals, token edge cases, unicode, escapes, comments.
package parse

import (
	"strings"
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive // dot import for tightly-coupled subpackage

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// ===== From probe_b =====

// TestProbeB_DoubleLiteral checks 3.14 as double literal.
func TestProbeB_DoubleLiteral(t *testing.T) {
	prog := parseMustSucceed(t, "main := 3.14")
	d := prog.Decls[0].(*DeclValueDef)
	dl, ok := d.Expr.(*ExprDoubleLit)
	if !ok {
		t.Fatalf("expected ExprDoubleLit, got %T", d.Expr)
	}
	if dl.Value != "3.14" {
		t.Errorf("expected '3.14', got %q", dl.Value)
	}
}

// TestProbeB_ScientificNotation checks 1e10 as double literal.
func TestProbeB_ScientificNotation(t *testing.T) {
	prog := parseMustSucceed(t, "main := 1e10")
	d := prog.Decls[0].(*DeclValueDef)
	dl, ok := d.Expr.(*ExprDoubleLit)
	if !ok {
		t.Fatalf("expected ExprDoubleLit for 1e10, got %T", d.Expr)
	}
	if dl.Value != "1e10" {
		t.Errorf("expected '1e10', got %q", dl.Value)
	}
}

// TestProbeB_UnderscoreInNumber checks 100_000 as int literal.
func TestProbeB_UnderscoreInNumber(t *testing.T) {
	prog := parseMustSucceed(t, "main := 100_000")
	d := prog.Decls[0].(*DeclValueDef)
	il, ok := d.Expr.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit, got %T", d.Expr)
	}
	if il.Value != "100_000" {
		t.Errorf("expected '100_000', got %q", il.Value)
	}
}

// TestProbeB_UnterminatedString checks that unterminated string produces a lex error.
func TestProbeB_UnterminatedString(t *testing.T) {
	errMsg := parseMustFail(t, `main := "hello`)
	if !strings.Contains(errMsg, "unterminated") {
		t.Errorf("expected 'unterminated' in error, got: %s", errMsg)
	}
}

// TestProbeB_NestedBlockComment checks nested block comments: {- outer {- inner -} still comment -}.
func TestProbeB_NestedBlockComment(t *testing.T) {
	src := "main := {- outer {- inner -} still comment -} 42"
	prog := parseMustSucceed(t, src)
	d := prog.Decls[0].(*DeclValueDef)
	il, ok := d.Expr.(*ExprIntLit)
	if !ok {
		t.Fatalf("expected ExprIntLit, got %T", d.Expr)
	}
	if il.Value != "42" {
		t.Errorf("expected 42, got %q", il.Value)
	}
}

// TestProbeB_LineCommentAtEOF checks line comment at end of file with no trailing newline.
func TestProbeB_LineCommentAtEOF(t *testing.T) {
	src := "main := 42 -- comment at EOF"
	prog := parseMustSucceed(t, src)
	if len(prog.Decls) != 1 {
		t.Errorf("expected 1 decl, got %d", len(prog.Decls))
	}
}

// TestProbeB_NullByte checks that null byte in source doesn't cause panic.
func TestProbeB_NullByte(t *testing.T) {
	_, es := parse("main := \x00")
	// Should not panic; error is acceptable.
	_ = es
}

// ===== From probe_d =====

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

// TestProbeD_LexerEmptyRune verifies `"` is handled (empty rune literal).
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

// ===== From probe_e =====

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

// TestProbeE_LexerEscapeNull verifies `\0` escape in strings.
func TestProbeE_LexerEscapeNull(t *testing.T) {
	src := span.NewSource("test", `"\0"`)
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	// \0 is a valid escape (NUL character).
	if es.HasErrors() {
		t.Logf("\\0 escape error: %s", es.Format())
	}
	if tokens[0].Kind != TokStrLit {
		t.Errorf("expected TokStrLit, got %v", tokens[0].Kind)
	}
}

// TestProbeE_LexerVeryLongIdentifier verifies a very long identifier (10000 chars).
func TestProbeE_LexerVeryLongIdentifier(t *testing.T) {
	name := strings.Repeat("x", 10000)
	src := span.NewSource("test", name+" := 42")
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	if tokens[0].Kind != TokLower || tokens[0].Text != name {
		t.Errorf("expected TokLower with 10000-char name, got %v len=%d", tokens[0].Kind, len(tokens[0].Text))
	}
}

// TestProbeE_LexerVeryLongOperator verifies a very long operator (5000 chars).
func TestProbeE_LexerVeryLongOperator(t *testing.T) {
	op := strings.Repeat("+", 5000)
	src := span.NewSource("test", op)
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	if tokens[0].Kind != TokOp {
		t.Errorf("expected TokOp for long operator, got %v", tokens[0].Kind)
	}
	if len(tokens[0].Text) != 5000 {
		t.Errorf("expected operator of length 5000, got %d", len(tokens[0].Text))
	}
}

// TestProbeE_LexerUnicodeUpperStart verifies non-ASCII uppercase is rejected as Upper token.
func TestProbeE_LexerUnicodeUpperStart(t *testing.T) {
	src := span.NewSource("test", "\u00C9tat") // Étal
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	// isUpperStart checks A-Z only, so É is not recognized as upper start.
	if tokens[0].Kind == TokUpper {
		t.Error("expected non-TokUpper for Unicode uppercase start")
	}
}

// TestProbeE_LexerUnterminatedBlockComment verifies unterminated block comment.
func TestProbeE_LexerUnterminatedBlockComment(t *testing.T) {
	src := span.NewSource("test", "{- open forever")
	l := NewLexer(src)
	_, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for unterminated block comment")
	}
}

// TestProbeE_LexerNestedBlockCommentMismatch verifies mismatched nesting count.
func TestProbeE_LexerNestedBlockCommentMismatch(t *testing.T) {
	src := span.NewSource("test", "{- {- -}")
	l := NewLexer(src)
	_, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for mismatched nested block comment")
	}
}

// TestProbeE_LexerEmptyRuneLiteral verifies ” is rejected.
func TestProbeE_LexerEmptyRuneLiteral(t *testing.T) {
	src := span.NewSource("test", "''")
	l := NewLexer(src)
	_, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for empty rune literal")
	}
}

// TestProbeE_LexerRuneUnterminated verifies 'a is rejected.
func TestProbeE_LexerRuneUnterminated(t *testing.T) {
	src := span.NewSource("test", "'a")
	l := NewLexer(src)
	_, es := l.Tokenize()
	if !es.HasErrors() {
		t.Error("expected error for unterminated rune literal")
	}
}

// TestProbeE_LexerRuneMultiChar verifies 'ab' is rejected or handled.
func TestProbeE_LexerRuneMultiChar(t *testing.T) {
	src := span.NewSource("test", "'ab'")
	l := NewLexer(src)
	_, es := l.Tokenize()
	// Multi-char rune should error.
	if !es.HasErrors() {
		t.Error("expected error for multi-char rune literal")
	}
}

// TestProbeE_LexerBinaryData verifies binary-heavy input doesn't crash.
func TestProbeE_LexerBinaryData(t *testing.T) {
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}
	src := span.NewSource("test", string(data))
	l := NewLexer(src)
	_, _ = l.Tokenize()
	// Just verify no panic.
}

// TestProbeE_LexerOnlyOperators verifies source with only operator characters.
func TestProbeE_LexerOnlyOperators(t *testing.T) {
	src := span.NewSource("test", "+-*/<>=!?&|^~$%@#")
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	// Should produce at least one operator token + EOF.
	if len(tokens) < 2 {
		t.Fatalf("expected at least 2 tokens, got %d", len(tokens))
	}
}

// TestProbeE_LexerConsecutiveStrings verifies consecutive string literals.
func TestProbeE_LexerConsecutiveStrings(t *testing.T) {
	src := span.NewSource("test", `"hello" "world"`)
	l := NewLexer(src)
	tokens, es := l.Tokenize()
	if es.HasErrors() {
		t.Fatalf("unexpected error: %s", es.Format())
	}
	strCount := 0
	for _, tok := range tokens {
		if tok.Kind == TokStrLit {
			strCount++
		}
	}
	if strCount != 2 {
		t.Errorf("expected 2 TokStrLit tokens, got %d", strCount)
	}
}

// TestProbeE_LexerEqSpaceColon verifies = : tokenizes as two tokens.
func TestProbeE_LexerEqSpaceColon(t *testing.T) {
	tokens := lex("= :")
	if tokens[0].Kind != TokEq {
		t.Errorf("expected TokEq for '=', got %v", tokens[0].Kind)
	}
	if tokens[1].Kind != TokColon {
		t.Errorf("expected TokColon for ':', got %v", tokens[1].Kind)
	}
}

// TestProbeE_LexerColonColonEq verifies ::= tokenization (not a real token combo).
func TestProbeE_LexerColonColonEq(t *testing.T) {
	tokens := lex("::=")
	// :: is TokColonColon, = is TokEq.
	if tokens[0].Kind != TokColonColon {
		t.Errorf("expected TokColonColon, got %v", tokens[0].Kind)
	}
	if tokens[1].Kind != TokEq {
		t.Errorf("expected TokEq, got %v", tokens[1].Kind)
	}
}

// TestProbeE_LexerPipeVsPipeOp verifies | vs ||.
func TestProbeE_LexerPipeVsPipeOp(t *testing.T) {
	tokens := lex("|")
	if tokens[0].Kind != TokPipe {
		t.Errorf("expected TokPipe for '|', got %v", tokens[0].Kind)
	}
	tokens2 := lex("||")
	// || should be TokOp (an operator, not two pipes).
	if tokens2[0].Kind != TokOp {
		t.Errorf("expected TokOp for '||', got %v", tokens2[0].Kind)
	}
}

// TestProbeE_LexerDoubleUnderscore verifies __ is TokLower (identifier starting with _).
func TestProbeE_LexerDoubleUnderscore(t *testing.T) {
	tokens := lex("__")
	if tokens[0].Kind != TokLower {
		t.Errorf("expected TokLower for '__', got %v", tokens[0].Kind)
	}
}

// TestProbeE_LexerUnderscoreAlone verifies _ is TokUnderscore.
func TestProbeE_LexerUnderscoreAlone(t *testing.T) {
	tokens := lex("_")
	if tokens[0].Kind != TokUnderscore {
		t.Errorf("expected TokUnderscore for '_', got %v", tokens[0].Kind)
	}
}

// TestProbeE_LexerIntSeparators verifies 1_000_000 is TokIntLit.
func TestProbeE_LexerIntSeparators(t *testing.T) {
	tokens := lex("1_000_000")
	if tokens[0].Kind != TokIntLit {
		t.Errorf("expected TokIntLit for 1_000_000, got %v", tokens[0].Kind)
	}
}

// TestProbeE_LexerDoubleLitEdge verifies edge cases of double literals.
func TestProbeE_LexerDoubleLitEdge(t *testing.T) {
	// 0.0 — should be TokDoubleLit.
	tokens := lex("0.0")
	if tokens[0].Kind != TokDoubleLit {
		t.Errorf("expected TokDoubleLit for 0.0, got %v", tokens[0].Kind)
	}
	// 1e-10 — exponent with sign.
	tokens2 := lex("1e-10")
	if tokens2[0].Kind != TokDoubleLit {
		t.Errorf("expected TokDoubleLit for 1e-10, got %v", tokens2[0].Kind)
	}
}

// TestProbeE_LexerTrailingUnderscore verifies 100_ behavior.
func TestProbeE_LexerTrailingUnderscore(t *testing.T) {
	tokens := lex("100_")
	// The lexer may accept 100_ as a single token or split it.
	// Document actual behavior.
	if tokens[0].Kind == TokIntLit {
		t.Logf("100_ lexed as TokIntLit: %q", tokens[0].Text)
	} else {
		t.Logf("100_ lexed as %v: %q", tokens[0].Kind, tokens[0].Text)
	}
}

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

// TestProbeE_PrimedIdentifiers verifies identifiers with primes: x', x", etc.
func TestProbeE_PrimedIdentifiers(t *testing.T) {
	source := `x' := 1
x'' := 2
main := x'`
	prog := parseMustSucceed(t, source)
	if len(prog.Decls) != 3 {
		t.Errorf("expected 3 decls, got %d", len(prog.Decls))
	}
}
