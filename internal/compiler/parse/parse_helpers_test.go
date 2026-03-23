package parse

import (
	"context"

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
