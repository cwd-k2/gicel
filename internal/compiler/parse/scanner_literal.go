package parse

import (
	"unicode/utf8"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

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

	if s.pos >= len(s.source.Text) {
		s.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrUnterminatedLit,
			Phase:   diagnostic.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
			Message: "unterminated rune literal",
		})
	} else if s.source.Text[s.pos] == '\'' {
		s.pos++ // skip closing '\''
	} else {
		// Extra characters before closing quote: scan forward to recover.
		for s.pos < len(s.source.Text) && s.source.Text[s.pos] != '\'' && s.source.Text[s.pos] != '\n' {
			if s.source.Text[s.pos] == '\\' && s.pos+1 < len(s.source.Text) {
				s.pos += 2 // skip escape pair
			} else {
				s.pos++
			}
		}
		if s.pos < len(s.source.Text) && s.source.Text[s.pos] == '\'' {
			s.pos++ // consume closing quote for recovery
			s.errors.Add(&diagnostic.Error{
				Code:    diagnostic.ErrMultiCharRune,
				Phase:   diagnostic.PhaseLex,
				Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
				Message: "rune literal must contain exactly one character",
			})
		} else {
			s.errors.Add(&diagnostic.Error{
				Code:    diagnostic.ErrUnterminatedLit,
				Phase:   diagnostic.PhaseLex,
				Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
				Message: "unterminated rune literal",
			})
		}
	}
	return syn.Token{
		Kind: syn.TokRuneLit,
		Text: string(r),
		S:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
	}
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
	// Detect malformed numeric like 3.14.15 (second dot after double).
	if isDouble && s.pos < len(s.source.Text) && s.source.Text[s.pos] == '.' &&
		s.pos+1 < len(s.source.Text) && s.source.Text[s.pos+1] >= '0' && s.source.Text[s.pos+1] <= '9' {
		s.errors.Add(&diagnostic.Error{
			Code:    diagnostic.ErrMalformedNumber,
			Phase:   diagnostic.PhaseLex,
			Span:    span.Span{Start: span.Pos(start), End: span.Pos(s.pos)},
			Message: "malformed numeric literal: extra '.' in floating-point number",
		})
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
