// Parse test helpers — shared lex/parse utilities for parse package tests.
// Does NOT cover: correctness testing (parse_test.go, lexer_test.go).

package parse

import (
	"context"
	"testing"

	. "github.com/cwd-k2/gicel/internal/lang/syntax" //nolint:revive

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

func lex(input string) []Token {
	src := span.NewSource("test", input)
	l := NewLexer(src)
	tokens, _ := l.Tokenize()
	return tokens
}

func parse(input string) (*AstProgram, *diagnostic.Errors) {
	src := span.NewSource("test", input)
	l := NewLexer(src)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		return nil, lexErrs
	}
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), tokens, es)
	prog := p.ParseProgram()
	return prog, es
}

func lexWithErrors(input string) ([]Token, *diagnostic.Errors) {
	src := span.NewSource("test", input)
	l := NewLexer(src)
	return l.Tokenize()
}

func parseMustSucceed(t *testing.T, source string) *AstProgram {
	t.Helper()
	prog, es := parse(source)
	if es.HasErrors() {
		t.Fatalf("unexpected parse error: %s", es.Format())
	}
	return prog
}

func parseMustFail(t *testing.T, source string) string {
	t.Helper()
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected parse error, got none")
	}
	return es.Format()
}
