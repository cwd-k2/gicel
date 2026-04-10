package parse

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// Scanner produces tokens on demand from source text.
type Scanner struct {
	source     *span.Source
	pos        int
	errors     *diagnostic.Errors
	sawNewline bool
}

// NewScanner creates a streaming token scanner for the given source.
func NewScanner(source *span.Source) *Scanner {
	start := 0
	text := source.Text
	// Skip BOM.
	if len(text) >= 3 && text[0] == 0xEF && text[1] == 0xBB && text[2] == 0xBF {
		start = 3
	}
	// Skip shebang line (e.g. #!/usr/bin/env gicel run).
	if start < len(text)-1 && text[start] == '#' && text[start+1] == '!' {
		for start < len(text) && text[start] != '\n' {
			start++
		}
		if start < len(text) {
			start++ // skip the newline
		}
	}
	return &Scanner{
		source: source,
		pos:    start,
		errors: &diagnostic.Errors{Source: source},
	}
}

// Next returns the next token from the source.
func (s *Scanner) Next() syn.Token {
	return s.next()
}

// Errors returns the accumulated lexer diagnostics.
func (s *Scanner) Errors() *diagnostic.Errors {
	return s.errors
}

func (s *Scanner) next() syn.Token {
	s.skipWhitespaceAndComments()
	nlBefore := s.sawNewline
	s.sawNewline = false
	tok := s.scanToken()
	tok.NewlineBefore = nlBefore
	return tok
}

func (s *Scanner) scanToken() syn.Token {
	for {
		if s.pos >= len(s.source.Text) {
			return syn.Token{Kind: syn.TokEOF, S: span.Span{Start: span.Pos(s.pos), End: span.Pos(s.pos)}}
		}

		start := s.pos
		ch := s.peekRune()

		// Single-char delimiters
		switch ch {
		case '(':
			s.advanceRune()
			return s.tok(syn.TokLParen, start)
		case ')':
			s.advanceRune()
			return s.tok(syn.TokRParen, start)
		case '{':
			s.advanceRune()
			return s.tok(syn.TokLBrace, start)
		case '}':
			s.advanceRune()
			return s.tok(syn.TokRBrace, start)
		case '[':
			s.advanceRune()
			return s.tok(syn.TokLBracket, start)
		case ']':
			s.advanceRune()
			return s.tok(syn.TokRBracket, start)
		case ',':
			s.advanceRune()
			return s.tok(syn.TokComma, start)
		case ';':
			s.advanceRune()
			return s.tok(syn.TokSemicolon, start)
		}

		// Multi-char punctuation & operators
		if ch == '-' && s.peekRuneAt(1) == '>' && !isOperatorChar(s.peekRuneAt(2)) {
			s.pos += 2
			return s.tok(syn.TokArrow, start)
		}
		if ch == '-' && s.peekRuneAt(1) == '|' && !isOperatorChar(s.peekRuneAt(2)) {
			s.pos += 2
			return s.tok(syn.TokDashPipe, start)
		}
		if ch == '<' && s.peekRuneAt(1) == '-' && !isOperatorChar(s.peekRuneAt(2)) {
			s.pos += 2
			return s.tok(syn.TokLArrow, start)
		}
		if ch == ':' && s.peekRuneAt(1) == ':' {
			s.pos += 2
			return s.tok(syn.TokColonColon, start)
		}
		if ch == ':' && s.peekRuneAt(1) == '=' {
			if isOperatorChar(s.peekRuneAt(2)) {
				s.pos += 2
				for s.pos < len(s.source.Text) {
					r, size := utf8.DecodeRuneInString(s.source.Text[s.pos:])
					if !isOperatorChar(r) && r != ':' {
						break
					}
					s.pos += size
				}
				text := s.source.Text[start:s.pos]
				s.errors.Add(&diagnostic.Error{
					Code:    diagnostic.ErrReservedInOp,
					Phase:   diagnostic.PhaseLex,
					Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
					Message: fmt.Sprintf("operator %q contains reserved symbol :=", text),
				})
				return s.tok(syn.TokOp, start)
			}
			s.pos += 2
			return s.tok(syn.TokColonEq, start)
		}
		if ch == ':' {
			s.advanceRune()
			return s.tok(syn.TokColon, start)
		}
		if ch == '.' {
			if s.peekRuneAt(1) == '#' && !isOperatorChar(s.peekRuneAt(2)) {
				s.pos += 2
				return s.tok(syn.TokDotHash, start)
			}
			s.advanceRune()
			return s.tok(syn.TokDot, start)
		}
		if ch == '\\' {
			s.advanceRune()
			return s.tok(syn.TokBackslash, start)
		}
		if ch == '@' {
			s.advanceRune()
			return s.tok(syn.TokAt, start)
		}
		if ch == '=' && s.peekRuneAt(1) == '>' && !isOperatorChar(s.peekRuneAt(2)) {
			s.pos += 2
			return s.tok(syn.TokFatArrow, start)
		}
		if ch == '=' && !isOperatorChar(s.peekRuneAt(1)) {
			s.advanceRune()
			return s.tok(syn.TokEq, start)
		}
		if ch == '|' && !isOperatorChar(s.peekRuneAt(1)) {
			s.advanceRune()
			return s.tok(syn.TokPipe, start)
		}
		if ch == '~' && !isOperatorChar(s.peekRuneAt(1)) {
			s.advanceRune()
			return s.tok(syn.TokTilde, start)
		}
		if ch == '_' && !isIdentCont(s.peekRuneAt(1)) {
			s.advanceRune()
			return s.tok(syn.TokUnderscore, start)
		}

		// String literal
		if ch == '"' {
			return s.scanString()
		}

		// Rune literal
		if ch == '\'' {
			return s.scanRune(start)
		}

		// Numeric literal
		if ch >= '0' && ch <= '9' {
			return s.scanNumeric(start)
		}

		// Lower identifier or keyword
		if isLowerStart(ch) {
			s.pos += utf8.RuneLen(ch)
			for s.pos < len(s.source.Text) {
				r, size := utf8.DecodeRuneInString(s.source.Text[s.pos:])
				if !isIdentCont(r) {
					break
				}
				s.pos += size
			}
			text := s.source.Text[start:s.pos]
			if kind, ok := keywords[text]; ok {
				return syn.Token{Kind: kind, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(s.pos)}}
			}
			return syn.Token{Kind: syn.TokLower, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(s.pos)}}
		}

		// Upper identifier
		if isUpperStart(ch) {
			s.pos += utf8.RuneLen(ch)
			for s.pos < len(s.source.Text) {
				r, size := utf8.DecodeRuneInString(s.source.Text[s.pos:])
				if !isIdentCont(r) {
					break
				}
				s.pos += size
			}
			text := s.source.Text[start:s.pos]
			return syn.Token{Kind: syn.TokUpper, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(s.pos)}}
		}

		// Label literal: #name (type-level label reference).
		// Must precede operator scanning since # is an operator character.
		if ch == '#' {
			next := s.peekRuneAt(1)
			if isLowerStart(next) || next == '_' {
				s.pos++ // consume #
				labelStart := s.pos
				for s.pos < len(s.source.Text) {
					r, size := utf8.DecodeRuneInString(s.source.Text[s.pos:])
					if !isIdentCont(r) {
						break
					}
					s.pos += size
				}
				text := s.source.Text[labelStart:s.pos]
				return syn.Token{Kind: syn.TokLabelLit, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(s.pos)}}
			}
		}

		// Operator
		if isOperatorChar(ch) {
			for s.pos < len(s.source.Text) {
				r, size := utf8.DecodeRuneInString(s.source.Text[s.pos:])
				if !isOperatorChar(r) {
					break
				}
				s.pos += size
			}
			return s.tok(syn.TokOp, start)
		}

		// Unknown character — skip and retry (loop instead of recursion to prevent stack overflow).
		_, size := utf8.DecodeRuneInString(s.source.Text[s.pos:])
		s.pos += size
		s.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrUnexpectedChar,
			Phase:   diagnostic.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
			Message: "unexpected character",
		})
		s.skipWhitespaceAndComments()
	}
}

func (s *Scanner) skipWhitespaceAndComments() {
	for s.pos < len(s.source.Text) {
		ch := s.peekRune()
		if ch == ' ' || ch == '\t' || ch == '\r' {
			s.pos++
			continue
		}
		if ch == '\n' {
			s.pos++
			s.sawNewline = true
			continue
		}
		// Line comment
		if ch == '-' && s.peekRuneAt(1) == '-' {
			for s.pos < len(s.source.Text) && s.source.Text[s.pos] != '\n' {
				s.pos++
			}
			continue
		}
		// Block comment (nestable)
		if ch == '{' && s.peekRuneAt(1) == '-' {
			commentStart := s.pos
			s.pos += 2
			depth := 1
			for s.pos < len(s.source.Text) && depth > 0 {
				if s.source.Text[s.pos] == '{' && s.peekRuneAt(1) == '-' {
					depth++
					s.pos += 2
				} else if s.source.Text[s.pos] == '-' && s.peekRuneAt(1) == '}' {
					depth--
					s.pos += 2
				} else {
					_, size := utf8.DecodeRuneInString(s.source.Text[s.pos:])
					s.pos += size
				}
			}
			if depth > 0 {
				s.errors.Add(&diagnostic.Error{
					Code:    diagnostic.ErrUnterminatedLit,
					Phase:   diagnostic.PhaseLex,
					Span:    span.Span{Start: span.Pos(commentStart), End: span.Pos(s.pos)},
					Message: "unterminated block comment",
				})
			}
			continue
		}
		break
	}
}

func (s *Scanner) peekRune() rune {
	if s.pos >= len(s.source.Text) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s.source.Text[s.pos:])
	return r
}

// peekRuneAt decodes the rune at byte offset s.pos+offset.
// The offset is in bytes, not runes — callers pass small constants
// (1, 2) to disambiguate ASCII-only multi-char tokens.
func (s *Scanner) peekRuneAt(offset int) rune {
	p := s.pos + offset
	if p >= len(s.source.Text) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s.source.Text[p:])
	return r
}

func (s *Scanner) advanceRune() {
	if s.pos < len(s.source.Text) {
		_, size := utf8.DecodeRuneInString(s.source.Text[s.pos:])
		s.pos += size
	}
}

func (s *Scanner) tok(kind syn.TokenKind, start int) syn.Token {
	return syn.Token{
		Kind: kind,
		Text: s.source.Text[start:s.pos],
		S:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
	}
}

var keywords = map[string]syn.TokenKind{
	// "as" is contextual — only special after import, usable as variable name.
	"assumption": syn.TokAssumption,
	"case":       syn.TokCase,
	"do":         syn.TokDo,
	"form":       syn.TokForm,
	"lazy":       syn.TokLazy,
	"type":       syn.TokType,
	"infixl":     syn.TokInfixl,
	"infixr":     syn.TokInfixr,
	"infixn":     syn.TokInfixn,
	"impl":       syn.TokImpl,
	"import":     syn.TokImport,
	"if":         syn.TokIf,
	"then":       syn.TokThen,
	"else":       syn.TokElse,
}

func isLowerStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || r == '_'
}

func isUpperStart(r rune) bool {
	return r >= 'A' && r <= 'Z'
}

func isIdentCont(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '\''
}

func isOperatorChar(r rune) bool {
	switch r {
	case '!', '#', '$', '%', '&', '*', '+', '-', '/', '<', '=', '>', '?', '^', '~', '|':
		return true
	}
	return false
}
