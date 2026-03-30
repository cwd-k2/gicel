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
	if len(source.Text) >= 3 && source.Text[0] == 0xEF && source.Text[1] == 0xBB && source.Text[2] == 0xBF {
		start = 3
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

func (s *Scanner) scanString() syn.Token {
	start := s.pos
	s.pos++ // skip opening '"'
	var buf []byte
	for s.pos < len(s.source.Text) {
		ch := s.source.Text[s.pos]
		if ch == '"' {
			s.pos++
			return syn.Token{
				Kind: syn.TokStrLit,
				Text: string(buf),
				S:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
			}
		}
		if ch == '\\' {
			s.pos++
			if s.pos >= len(s.source.Text) {
				break
			}
			esc := s.source.Text[s.pos]
			s.pos++
			switch esc {
			case 'n':
				buf = append(buf, '\n')
			case 't':
				buf = append(buf, '\t')
			case 'r':
				buf = append(buf, '\r')
			case '\\':
				buf = append(buf, '\\')
			case '"':
				buf = append(buf, '"')
			case '\'':
				buf = append(buf, '\'')
			case '0':
				buf = append(buf, 0)
			default:
				s.errors.Add(&diagnostic.Error{
					Code:    diagnostic.ErrBadEscape,
					Phase:   diagnostic.PhaseLex,
					Span:    span.Span{Start: span.Pos(s.pos - 2), End: span.Pos(s.pos)},
					Message: "unknown escape sequence",
				})
				buf = append(buf, esc)
			}
			continue
		}
		if ch == '\n' {
			break // unterminated (newline before closing quote)
		}
		buf = append(buf, ch)
		s.pos++
	}
	s.errors.Add(&diagnostic.Error{
		Code:    diagnostic.ErrUnterminatedLit,
		Phase:   diagnostic.PhaseLex,
		Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
		Message: "unterminated string literal",
	})
	return syn.Token{
		Kind: syn.TokStrLit,
		Text: string(buf),
		S:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
	}
}

func (s *Scanner) scanRune(start int) syn.Token {
	s.pos++ // skip opening '\''
	if s.pos >= len(s.source.Text) || s.source.Text[s.pos] == '\'' {
		s.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrEmptyRuneLit,
			Phase:   diagnostic.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
			Message: "empty rune literal",
		})
		if s.pos < len(s.source.Text) && s.source.Text[s.pos] == '\'' {
			s.pos++
		}
		return syn.Token{
			Kind: syn.TokRuneLit,
			Text: "\x00",
			S:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
		}
	}

	var r rune
	if s.source.Text[s.pos] == '\\' {
		s.pos++
		if s.pos >= len(s.source.Text) {
			s.errors.Add(&diagnostic.Error{
				Code:    diagnostic.ErrUnterminatedLit,
				Phase:   diagnostic.PhaseLex,
				Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
				Message: "unterminated rune literal",
			})
			return syn.Token{Kind: syn.TokRuneLit, Text: "\x00", S: span.Span{Start: span.Pos(start), End: span.Pos(s.pos)}}
		}
		esc := s.source.Text[s.pos]
		s.pos++
		switch esc {
		case 'n':
			r = '\n'
		case 't':
			r = '\t'
		case 'r':
			r = '\r'
		case '\\':
			r = '\\'
		case '\'':
			r = '\''
		case '"':
			r = '"'
		case '0':
			r = 0
		default:
			s.errors.Add(&diagnostic.Error{
				Code:    diagnostic.ErrBadEscape,
				Phase:   diagnostic.PhaseLex,
				Span:    span.Span{Start: span.Pos(s.pos - 2), End: span.Pos(s.pos)},
				Message: "unknown escape sequence",
			})
			r = rune(esc)
		}
	} else {
		var size int
		r, size = utf8.DecodeRuneInString(s.source.Text[s.pos:])
		s.pos += size
	}

	if s.pos >= len(s.source.Text) || s.source.Text[s.pos] != '\'' {
		s.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrUnterminatedLit,
			Phase:   diagnostic.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
			Message: "unterminated rune literal",
		})
	} else {
		s.pos++ // skip closing '\''
	}
	return syn.Token{
		Kind: syn.TokRuneLit,
		Text: string(r),
		S:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
	}
}

var keywords = map[string]syn.TokenKind{
	"as":         syn.TokAs,
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

// scanNumeric scans an integer or double literal (supports _ separators).
// Examples: 42, 100_000, 3.14, 1e10, 1.05e+10, 2_000.5e-3
func (s *Scanner) scanNumeric(start int) syn.Token {
	s.scanDigits() // integer part
	isDouble := false
	// Decimal part: '.' followed by digit
	if s.pos < len(s.source.Text) && s.source.Text[s.pos] == '.' &&
		s.pos+1 < len(s.source.Text) && s.source.Text[s.pos+1] >= '0' && s.source.Text[s.pos+1] <= '9' {
		isDouble = true
		s.pos++ // skip '.'
		s.scanDigits()
	}
	// Exponent part: e/E followed by optional +/- and digits
	if s.pos < len(s.source.Text) && (s.source.Text[s.pos] == 'e' || s.source.Text[s.pos] == 'E') {
		next := s.pos + 1
		if next < len(s.source.Text) {
			nc := s.source.Text[next]
			hasExp := false
			if nc >= '0' && nc <= '9' {
				hasExp = true
				s.pos = next
			} else if (nc == '+' || nc == '-') && next+1 < len(s.source.Text) && s.source.Text[next+1] >= '0' && s.source.Text[next+1] <= '9' {
				hasExp = true
				s.pos = next + 1 // skip e and sign
			}
			if hasExp {
				isDouble = true
				s.scanDigits()
			}
		}
	}
	if isDouble {
		return s.tok(syn.TokDoubleLit, start)
	}
	return s.tok(syn.TokIntLit, start)
}

// scanDigits consumes a run of digits with optional underscore separators.
func (s *Scanner) scanDigits() {
	for s.pos < len(s.source.Text) {
		c := s.source.Text[s.pos]
		if c >= '0' && c <= '9' {
			s.pos++
		} else if c == '_' && s.pos+1 < len(s.source.Text) && s.source.Text[s.pos+1] >= '0' && s.source.Text[s.pos+1] <= '9' {
			s.pos++ // skip underscore between digits
		} else {
			break
		}
	}
}

func isOperatorChar(r rune) bool {
	switch r {
	case '!', '#', '$', '%', '&', '*', '+', '-', '/', '<', '=', '>', '?', '^', '~', '|':
		return true
	}
	return false
}
