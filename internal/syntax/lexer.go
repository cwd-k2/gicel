package syntax

import (
	"unicode"
	"unicode/utf8"

	"github.com/cwd-k2/gomputation/internal/errs"
	"github.com/cwd-k2/gomputation/internal/span"
)

// Lexer tokenizes source text.
type Lexer struct {
	source     *span.Source
	pos        int
	errors     *errs.Errors
	sawNewline bool
}

// NewLexer creates a lexer for the given source.
func NewLexer(source *span.Source) *Lexer {
	return &Lexer{
		source: source,
		pos:    0,
		errors: &errs.Errors{Source: source},
	}
}

// Tokenize scans the entire source and returns the token stream.
func (l *Lexer) Tokenize() ([]Token, *errs.Errors) {
	var tokens []Token
	for {
		tok := l.next()
		tokens = append(tokens, tok)
		if tok.Kind == TokEOF {
			break
		}
	}
	return tokens, l.errors
}

func (l *Lexer) next() Token {
	l.skipWhitespaceAndComments()
	nlBefore := l.sawNewline
	l.sawNewline = false
	tok := l.scanToken()
	tok.NewlineBefore = nlBefore
	return tok
}

func (l *Lexer) scanToken() Token {
	for {
		if l.pos >= len(l.source.Text) {
			return Token{Kind: TokEOF, S: span.Span{Start: span.Pos(l.pos), End: span.Pos(l.pos)}}
		}

		start := l.pos
		ch := l.peek()

		// Single-char delimiters
		switch ch {
		case '(':
			l.advance()
			return l.tok(TokLParen, start)
		case ')':
			l.advance()
			return l.tok(TokRParen, start)
		case '{':
			l.advance()
			return l.tok(TokLBrace, start)
		case '}':
			l.advance()
			return l.tok(TokRBrace, start)
		case '[':
			l.advance()
			return l.tok(TokLBracket, start)
		case ']':
			l.advance()
			return l.tok(TokRBracket, start)
		case ',':
			l.advance()
			return l.tok(TokComma, start)
		case ';':
			l.advance()
			return l.tok(TokSemicolon, start)
		}

		// Multi-char punctuation & operators
		if ch == '-' && l.peekAt(1) == '>' {
			l.pos += 2
			return l.tok(TokArrow, start)
		}
		if ch == '<' && l.peekAt(1) == '-' {
			l.pos += 2
			return l.tok(TokLArrow, start)
		}
		if ch == ':' && l.peekAt(1) == ':' {
			l.pos += 2
			return l.tok(TokColonColon, start)
		}
		if ch == ':' && l.peekAt(1) == '=' {
			l.pos += 2
			return l.tok(TokColonEq, start)
		}
		if ch == ':' {
			l.advance()
			return l.tok(TokColon, start)
		}
		if ch == '.' {
			l.advance()
			return l.tok(TokDot, start)
		}
		if ch == '\\' {
			l.advance()
			return l.tok(TokBackslash, start)
		}
		if ch == '@' {
			l.advance()
			return l.tok(TokAt, start)
		}
		if ch == '=' && l.peekAt(1) == '>' {
			l.pos += 2
			return l.tok(TokFatArrow, start)
		}
		if ch == '=' && !isOperatorChar(l.peekAt(1)) {
			l.advance()
			return l.tok(TokEq, start)
		}
		if ch == '|' && !isOperatorChar(l.peekAt(1)) {
			l.advance()
			return l.tok(TokPipe, start)
		}
		if ch == '_' && !isIdentCont(l.peekAt(1)) {
			l.advance()
			return l.tok(TokUnderscore, start)
		}

		// String literal
		if ch == '"' {
			return l.scanString()
		}

		// Rune literal
		if ch == '\'' {
			return l.scanRune(start)
		}

		// Integer literal
		if ch >= '0' && ch <= '9' {
			for l.pos < len(l.source.Text) && l.source.Text[l.pos] >= '0' && l.source.Text[l.pos] <= '9' {
				l.pos++
			}
			return l.tok(TokIntLit, start)
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
				return Token{Kind: kind, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(l.pos)}}
			}
			return Token{Kind: TokLower, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(l.pos)}}
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
			return Token{Kind: TokUpper, Text: text, S: span.Span{Start: span.Pos(start), End: span.Pos(l.pos)}}
		}

		// Record projection: !# (exactly these two chars, not part of longer operator)
		if ch == '!' && l.peekAt(1) == '#' && !isOperatorChar(l.peekAt(2)) {
			l.pos += 2
			return l.tok(TokBangHash, start)
		}

		// Operator
		if isOperatorChar(ch) {
			for l.pos < len(l.source.Text) && isOperatorChar(rune(l.source.Text[l.pos])) {
				l.pos++
			}
			return l.tok(TokOp, start)
		}

		// Unknown character — skip and retry (loop instead of recursion to prevent stack overflow).
		l.pos += utf8.RuneLen(ch)
		l.errors.Add(&errs.Error{
			Code:    errs.ErrUnexpectedChar,
			Phase:   errs.PhaseLex,
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
					l.pos++
				}
			}
			if depth > 0 {
				l.errors.Add(&errs.Error{
					Code:    errs.ErrUnterminatedStr,
					Phase:   errs.PhaseLex,
					Span:    span.Span{Start: span.Pos(l.pos), End: span.Pos(l.pos)},
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
	return rune(l.source.Text[p])
}

func (l *Lexer) advance() {
	l.pos++
}

func (l *Lexer) tok(kind TokenKind, start int) Token {
	return Token{
		Kind: kind,
		Text: l.source.Text[start:l.pos],
		S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
	}
}

func (l *Lexer) scanString() Token {
	start := l.pos
	l.pos++ // skip opening '"'
	var buf []byte
	for l.pos < len(l.source.Text) {
		ch := l.source.Text[l.pos]
		if ch == '"' {
			l.pos++
			return Token{
				Kind: TokStrLit,
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
				l.errors.Add(&errs.Error{
					Code:    errs.ErrBadEscape,
					Phase:   errs.PhaseLex,
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
	l.errors.Add(&errs.Error{
		Code:    errs.ErrUnterminatedLit,
		Phase:   errs.PhaseLex,
		Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
		Message: "unterminated string literal",
	})
	return Token{
		Kind: TokStrLit,
		Text: string(buf),
		S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
	}
}

func (l *Lexer) scanRune(start int) Token {
	l.pos++ // skip opening '\''
	if l.pos >= len(l.source.Text) || l.source.Text[l.pos] == '\'' {
		l.errors.Add(&errs.Error{
			Code:    errs.ErrEmptyRuneLit,
			Phase:   errs.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
			Message: "empty rune literal",
		})
		if l.pos < len(l.source.Text) && l.source.Text[l.pos] == '\'' {
			l.pos++
		}
		return Token{
			Kind: TokRuneLit,
			Text: "\x00",
			S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
		}
	}

	var r rune
	if l.source.Text[l.pos] == '\\' {
		l.pos++
		if l.pos >= len(l.source.Text) {
			l.errors.Add(&errs.Error{
				Code:    errs.ErrUnterminatedLit,
				Phase:   errs.PhaseLex,
				Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
				Message: "unterminated rune literal",
			})
			return Token{Kind: TokRuneLit, Text: "\x00", S: span.Span{Start: span.Pos(start), End: span.Pos(l.pos)}}
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
			l.errors.Add(&errs.Error{
				Code:    errs.ErrBadEscape,
				Phase:   errs.PhaseLex,
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
		l.errors.Add(&errs.Error{
			Code:    errs.ErrUnterminatedLit,
			Phase:   errs.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
			Message: "unterminated rune literal",
		})
	} else {
		l.pos++ // skip closing '\''
	}
	return Token{
		Kind: TokRuneLit,
		Text: string(r),
		S:    span.Span{Start: span.Pos(start), End: span.Pos(l.pos)},
	}
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
