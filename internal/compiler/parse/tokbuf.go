package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// tokenBuffer is a lazy-fill token buffer backed by a Scanner.
// Tokens are fetched on demand and retained only while needed for
// lookahead and speculation. Compaction discards consumed tokens
// that are no longer reachable.
type tokenBuffer struct {
	scanner *Scanner      // nil when fully seeded (backward compat)
	buf     []syn.Token   // buffered tokens
	base    int           // logical index of buf[0]
	pos     int           // current logical read position
	marks   []int         // speculation save points (stack)
	eof     bool          // true once TokEOF has been buffered
}

func newTokenBuffer(scanner *Scanner, seed []syn.Token) *tokenBuffer {
	tb := &tokenBuffer{
		scanner: scanner,
		buf:     make([]syn.Token, 0, max(len(seed), 64)),
	}
	tb.buf = append(tb.buf, seed...)
	for _, tok := range seed {
		if tok.Kind == syn.TokEOF {
			tb.eof = true
			break
		}
	}
	return tb
}

// fill ensures the buffer contains up to logical index idx.
func (tb *tokenBuffer) fill(idx int) {
	for !tb.eof && idx >= tb.base+len(tb.buf) {
		if tb.scanner == nil {
			// Fully seeded — synthesize EOF
			tb.buf = append(tb.buf, syn.Token{Kind: syn.TokEOF})
			tb.eof = true
			return
		}
		tok := tb.scanner.Next()
		tb.buf = append(tb.buf, tok)
		if tok.Kind == syn.TokEOF {
			tb.eof = true
		}
	}
}

// at returns the token at logical index idx.
func (tb *tokenBuffer) at(idx int) syn.Token {
	tb.fill(idx)
	physical := idx - tb.base
	if physical < 0 || physical >= len(tb.buf) {
		return syn.Token{Kind: syn.TokEOF}
	}
	return tb.buf[physical]
}

// peek returns the token at the current position.
func (tb *tokenBuffer) peek() syn.Token {
	return tb.at(tb.pos)
}

// peekAt returns the token at pos+offset. Negative offsets are allowed
// as long as the token is still in the buffer (not yet compacted).
func (tb *tokenBuffer) peekAt(offset int) syn.Token {
	return tb.at(tb.pos + offset)
}

// advance moves pos forward by 1 and returns the consumed token.
func (tb *tokenBuffer) advance() syn.Token {
	tok := tb.peek()
	if tok.Kind != syn.TokEOF {
		tb.pos++
	}
	tb.maybeCompact()
	return tok
}

// prevEnd returns the End position of the token at pos-1.
func (tb *tokenBuffer) prevEnd() span.Pos {
	if tb.pos > 0 {
		prev := tb.at(tb.pos - 1)
		return prev.S.End
	}
	return span.Pos(0)
}

// save pushes the current position onto the speculation stack.
func (tb *tokenBuffer) save() {
	tb.marks = append(tb.marks, tb.pos)
}

// restore pops and restores the last saved position.
func (tb *tokenBuffer) restore() {
	n := len(tb.marks) - 1
	tb.pos = tb.marks[n]
	tb.marks = tb.marks[:n]
}

// commit pops the last saved position without restoring.
func (tb *tokenBuffer) commit() {
	tb.marks = tb.marks[:len(tb.marks)-1]
	tb.maybeCompact()
}

// scanForward scans tokens starting from the current position until
// pred returns stop=true. Returns the result from the stopping predicate.
// Does not advance the read position.
func (tb *tokenBuffer) scanForward(pred func(tok syn.Token, offset int) (stop bool, result bool)) bool {
	for i := 0; ; i++ {
		tok := tb.at(tb.pos + i)
		if stop, result := pred(tok, i); stop {
			return result
		}
		if tok.Kind == syn.TokEOF {
			return false
		}
	}
}

// maybeCompact discards tokens that are no longer reachable.
// Retains pos-1 for prevEnd() access.
const compactThreshold = 64

func (tb *tokenBuffer) maybeCompact() {
	watermark := tb.pos - 1 // retain previous token for prevEnd
	if watermark < tb.base {
		return
	}
	for _, m := range tb.marks {
		if m < watermark {
			watermark = m
		}
	}
	drop := watermark - tb.base
	if drop >= compactThreshold {
		// Slide buffer: reuse underlying array when possible
		copy(tb.buf, tb.buf[drop:])
		tb.buf = tb.buf[:len(tb.buf)-drop]
		tb.base += drop
	}
}
