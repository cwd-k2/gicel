package parse

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// Lexer tokenizes source text.
type Lexer struct {
	source     *span.Source
	pos        int
	errors     *diagnostic.Errors
	sawNewline bool
}

// NewLexer creates a lexer for the given source.
func NewLexer(source *span.Source) *Lexer {
	start := 0
	// Skip UTF-8 BOM if present.
	if len(source.Text) >= 3 && source.Text[0] == 0xEF && source.Text[1] == 0xBB && source.Text[2] == 0xBF {
		start = 3
	}
	return &Lexer{
		source: source,
		pos:    start,
		errors: &diagnostic.Errors{Source: source},
	}
}

// Tokenize scans the entire source and returns the token stream.
func (l *Lexer) Tokenize() ([]syn.Token, *diagnostic.Errors) {
	var tokens []syn.Token
	for {
		tok := l.next()
		tokens = append(tokens, tok)
		if tok.Kind == syn.TokEOF {
			break
		}
	}
	return tokens, l.errors
}

func (l *Lexer) next() syn.Token {
	l.skipWhitespaceAndComments()
	nlBefore := l.sawNewline
	l.sawNewline = false
	tok := l.scanToken()
	tok.NewlineBefore = nlBefore
	return tok
}

func (l *Lexer) scanToken() syn.Token {
	for {
		if l.pos >= len(l.source.Text) {
			return syn.Token{Kind: syn.TokEOF, S: span.Span{Start: span.Pos(l.pos), End: span.Pos(l.pos)}}
		}

		start := l.pos
		ch := l.peek()

		// Single-char delimiters
		switch ch {
		case '(':
			l.advance()
			return l.tok(syn.TokLParen, start)
		case ')':
			l.advance()
			return l.tok(syn.TokRParen, start)
		case '{':
			l.advance()
			return l.tok(syn.TokLBrace, start)
		case '}':
			l.advance()
			return l.tok(syn.TokRBrace, start)
		case '[':
			l.advance()
			return l.tok(syn.TokLBracket, start)
		case ']':
			l.advance()
			return l.tok(syn.TokRBracket, start)
		case ',':
			l.advance()
			return l.tok(syn.TokComma, start)
		case ';':
			l.advance()
			return l.tok(syn.TokSemicolon, start)
		}

		// Multi-char punctuation & operators
		if ch == '-' && l.peekAt(1) == '>' && !isOperatorChar(l.peekAt(2)) {
			l.pos += 2
			return l.tok(syn.TokArrow, start)
		}
		if ch == '<' && l.peekAt(1) == '-' && !isOperatorChar(l.peekAt(2)) {
			l.pos += 2
			return l.tok(syn.TokLArrow, start)
		}
		if ch == ':' && l.peekAt(1) == ':' {
			l.pos += 2
			return l.tok(syn.TokColonColon, start)
		}
		if ch == ':' && l.peekAt(1) == '=' {
			if isOperatorChar(l.peekAt(2)) {
				l.pos += 2
				for l.pos < len(l.source.Text) {
					r, size := utf8.DecodeRuneInString(l.source.Text[l.pos:])
					if !isOperatorChar(r) && r != ':' {
						break
					}
					l.pos += size
				}
				text := l.source.Text[start:l.pos]
				l.errors.Add(&diagnostic.Error{
					Code:    diagnostic.ErrReservedInOp,
					Phase:   diagnostic.PhaseLex,
					Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
					Message: fmt.Sprintf("operator %q contains reserved symbol :=", text),
				})
				return l.tok(syn.TokOp, start)
			}
			l.pos += 2
			return l.tok(syn.TokColonEq, start)
		}
		if ch == ':' {
			l.advance()
			return l.tok(syn.TokColon, start)
		}
		if ch == '.' {
			if l.peekAt(1) == '#' && !isOperatorChar(l.peekAt(2)) {
				l.pos += 2
				return l.tok(syn.TokDotHash, start)
			}
			l.advance()
			return l.tok(syn.TokDot, start)
		}
		if ch == '\\' {
			l.advance()
			return l.tok(syn.TokBackslash, start)
		}
		if ch == '@' {
			l.advance()
			return l.tok(syn.TokAt, start)
		}
		if ch == '=' && l.peekAt(1) == '>' && !isOperatorChar(l.peekAt(2)) {
			l.pos += 2
			return l.tok(syn.TokFatArrow, start)
		}
		if ch == '=' && !isOperatorChar(l.peekAt(1)) {
			l.advance()
			return l.tok(syn.TokEq, start)
		}
		if ch == '|' && !isOperatorChar(l.peekAt(1)) {
			l.advance()
			return l.tok(syn.TokPipe, start)
		}
		if ch == '~' && !isOperatorChar(l.peekAt(1)) {
			l.advance()
			return l.tok(syn.TokTilde, start)
		}
		if ch == '_' && !isIdentCont(l.peekAt(1)) {
			l.advance()
			return l.tok(syn.TokUnderscore, start)
		}

		// String literal
		if ch == '"' {
			return l.scanString()
		}

		// Rune literal
		if ch == '\'' {
			return l.scanRune(start)
		}

		// Numeric literal (Int or Double, supports _ separators)
		// Examples: 42, 100_000, 3.14, 1e10, 1.05e+10, 2_000.5e-3
		if ch >= '0' && ch <= '9' {
			l.scanDigits() // integer part
			isDouble := false
			// Decimal part: '.' followed by digit
			if l.pos < len(l.source.Text) && l.source.Text[l.pos] == '.' &&
				l.pos+1 < len(l.source.Text) && l.source.Text[l.pos+1] >= '0' && l.source.Text[l.pos+1] <= '9' {
				isDouble = true
				l.pos++ // skip '.'
				l.scanDigits()
			}
			// Exponent part: e/E followed by optional +/- and digits
			if l.pos < len(l.source.Text) && (l.source.Text[l.pos] == 'e' || l.source.Text[l.pos] == 'E') {
				next := l.pos + 1
				if next < len(l.source.Text) {
					nc := l.source.Text[next]
					hasExp := false
					if nc >= '0' && nc <= '9' {
						hasExp = true
						l.pos = next
					} else if (nc == '+' || nc == '-') && next+1 < len(l.source.Text) && l.source.Text[next+1] >= '0' && l.source.Text[next+1] <= '9' {
						hasExp = true
						l.pos = next + 1 // skip e and sign
					}
					if hasExp {
						isDouble = true
						l.scanDigits()
					}
				}
			}
			if isDouble {
				return l.tok(syn.TokDoubleLit, start)
			}
			return l.tok(syn.TokIntLit, start)
		}

		// Lower identifier or keyword
		if isLowerStart(ch) {
			l.pos += utf8.RuneLen(ch)
			for l.pos < len(l.source.Text) {
				r, size := utf8.DecodeRuneInString(l.source.Text[l.pos:])
				if !isIdentCont(r) {
					break
				}
				l.pos += size
			}
			text := l.source.Text[start:l.pos]
			if kind, ok := keywords[text]; ok {
				return syn.Token{Kind: kind, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(l.pos)}}
			}
			return syn.Token{Kind: syn.TokLower, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(l.pos)}}
		}

		// Upper identifier
		if isUpperStart(ch) {
			l.pos += utf8.RuneLen(ch)
			for l.pos < len(l.source.Text) {
				r, size := utf8.DecodeRuneInString(l.source.Text[l.pos:])
				if !isIdentCont(r) {
					break
				}
				l.pos += size
			}
			text := l.source.Text[start:l.pos]
			return syn.Token{Kind: syn.TokUpper, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(l.pos)}}
		}

		// Operator
		if isOperatorChar(ch) {
			for l.pos < len(l.source.Text) {
				r, size := utf8.DecodeRuneInString(l.source.Text[l.pos:])
				if !isOperatorChar(r) {
					break
				}
				l.pos += size
			}
			return l.tok(syn.TokOp, start)
		}

		// Unknown character — skip and retry (loop instead of recursion to prevent stack overflow).
		l.pos += utf8.RuneLen(ch)
		l.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrUnexpectedChar,
			Phase:   diagnostic.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
			Message: "unexpected character",
		})
		l.skipWhitespaceAndComments()
	}
}

func (l *Lexer) skipWhitespaceAndComments() {
	for l.pos < len(l.source.Text) {
		ch := l.peek()
		if ch == ' ' || ch == '\t' || ch == '\r' {
			l.pos++
			continue
		}
		if ch == '\n' {
			l.pos++
			l.sawNewline = true
			continue
		}
		// Line comment
		if ch == '-' && l.peekAt(1) == '-' {
			for l.pos < len(l.source.Text) && l.source.Text[l.pos] != '\n' {
				l.pos++
			}
			continue
		}
		// Block comment (nestable)
		if ch == '{' && l.peekAt(1) == '-' {
			commentStart := l.pos
			l.pos += 2
			depth := 1
			for l.pos < len(l.source.Text) && depth > 0 {
				if l.source.Text[l.pos] == '{' && l.peekAt(1) == '-' {
					depth++
					l.pos += 2
				} else if l.source.Text[l.pos] == '-' && l.peekAt(1) == '}' {
					depth--
					l.pos += 2
				} else {
					_, size := utf8.DecodeRuneInString(l.source.Text[l.pos:])
					l.pos += size
				}
			}
			if depth > 0 {
				l.errors.Add(&diagnostic.Error{
					Code:    diagnostic.ErrUnterminatedLit,
					Phase:   diagnostic.PhaseLex,
					Span:    span.Span{Start: span.Pos(commentStart), End: span.Pos(l.pos)},
					Message: "unterminated block comment",
				})
			}
			continue
		}
		break
	}
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.source.Text) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.source.Text[l.pos:])
	return r
}

func (l *Lexer) peekAt(offset int) rune {
	p := l.pos + offset
	if p >= len(l.source.Text) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.source.Text[p:])
	return r
}

func (l *Lexer) advance() {
	if l.pos < len(l.source.Text) {
		_, size := utf8.DecodeRuneInString(l.source.Text[l.pos:])
		l.pos += size
	}
}

func (l *Lexer) tok(kind syn.TokenKind, start int) syn.Token {
	return syn.Token{
		Kind: kind,
		Text: l.source.Text[start:l.pos],
		S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
	}
}

func (l *Lexer) scanString() syn.Token {
	start := l.pos
	l.pos++ // skip opening '"'
	var buf []byte
	for l.pos < len(l.source.Text) {
		ch := l.source.Text[l.pos]
		if ch == '"' {
			l.pos++
			return syn.Token{
				Kind: syn.TokStrLit,
				Text: string(buf),
				S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
			}
		}
		if ch == '\\' {
			l.pos++
			if l.pos >= len(l.source.Text) {
				break
			}
			esc := l.source.Text[l.pos]
			l.pos++
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
				l.errors.Add(&diagnostic.Error{
					Code:    diagnostic.ErrBadEscape,
					Phase:   diagnostic.PhaseLex,
					Span:    span.Span{Start: span.Pos(l.pos - 2), End: span.Pos(l.pos)},
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
		l.pos++
	}
	l.errors.Add(&diagnostic.Error{
		Code:    diagnostic.ErrUnterminatedLit,
		Phase:   diagnostic.PhaseLex,
		Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
		Message: "unterminated string literal",
	})
	return syn.Token{
		Kind: syn.TokStrLit,
		Text: string(buf),
		S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
	}
}

func (l *Lexer) scanRune(start int) syn.Token {
	l.pos++ // skip opening '\''
	if l.pos >= len(l.source.Text) || l.source.Text[l.pos] == '\'' {
		l.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrEmptyRuneLit,
			Phase:   diagnostic.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
			Message: "empty rune literal",
		})
		if l.pos < len(l.source.Text) && l.source.Text[l.pos] == '\'' {
			l.pos++
		}
		return syn.Token{
			Kind: syn.TokRuneLit,
			Text: "\x00",
			S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
		}
	}

	var r rune
	if l.source.Text[l.pos] == '\\' {
		l.pos++
		if l.pos >= len(l.source.Text) {
			l.errors.Add(&diagnostic.Error{
				Code:    diagnostic.ErrUnterminatedLit,
				Phase:   diagnostic.PhaseLex,
				Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
				Message: "unterminated rune literal",
			})
			return syn.Token{Kind: syn.TokRuneLit, Text: "\x00", S: span.Span{Start: span.Pos(start), End: span.Pos(l.pos)}}
		}
		esc := l.source.Text[l.pos]
		l.pos++
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
			l.errors.Add(&diagnostic.Error{
				Code:    diagnostic.ErrBadEscape,
				Phase:   diagnostic.PhaseLex,
				Span:    span.Span{Start: span.Pos(l.pos - 2), End: span.Pos(l.pos)},
				Message: "unknown escape sequence",
			})
			r = rune(esc)
		}
	} else {
		r, _ = utf8.DecodeRuneInString(l.source.Text[l.pos:])
		l.pos += utf8.RuneLen(r)
	}

	if l.pos >= len(l.source.Text) || l.source.Text[l.pos] != '\'' {
		l.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrUnterminatedLit,
			Phase:   diagnostic.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
			Message: "unterminated rune literal",
		})
	} else {
		l.pos++ // skip closing '\''
	}
	return syn.Token{
		Kind: syn.TokRuneLit,
		Text: string(r),
		S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
	}
}

var keywords = map[string]syn.TokenKind{
	"case":   syn.TokCase,
	"do":     syn.TokDo,
	"form":   syn.TokForm,
	"type":   syn.TokType,
	"infixl": syn.TokInfixl,
	"infixr": syn.TokInfixr,
	"infixn": syn.TokInfixn,
	"impl":   syn.TokImpl,
	"import": syn.TokImport,
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

// scanDigits consumes a run of digits with optional underscore separators.
func (l *Lexer) scanDigits() {
	for l.pos < len(l.source.Text) {
		c := l.source.Text[l.pos]
		if c >= '0' && c <= '9' {
			l.pos++
		} else if c == '_' && l.pos+1 < len(l.source.Text) && l.source.Text[l.pos+1] >= '0' && l.source.Text[l.pos+1] <= '9' {
			l.pos++ // skip underscore between digits
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
