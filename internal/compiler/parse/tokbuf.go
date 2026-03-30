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
	scanner      *Scanner    // token source
	buf          []syn.Token // buffered tokens
	base         int         // logical index of buf[0]
	pos          int         // current logical read position
	prev         syn.Token   // last consumed token (for prevEnd/prevToken)
	marks        []int       // speculation save points (stack)
	eof          bool        // true once TokEOF has been buffered
	compactCount int         // advance count since last compact check
}

// initialBufCap covers the typical lookahead window (~100 tokens for
// looksLikePipeADT line scans) plus compact threshold (64) plus margin,
// avoiding slice growth during normal parsing.
const initialBufCap = 256

func newTokenBuffer(scanner *Scanner) *tokenBuffer {
	return &tokenBuffer{
		scanner: scanner,
		buf:     make([]syn.Token, 0, initialBufCap),
	}
}

// fill ensures the buffer contains up to logical index idx.
// Called only from the slow path of at().
//
//go:noinline
func (tb *tokenBuffer) fill(idx int) {
	for !tb.eof && idx >= tb.base+len(tb.buf) {
		tok := tb.scanner.Next()
		tb.buf = append(tb.buf, tok)
		if tok.Kind == syn.TokEOF {
			tb.eof = true
		}
	}
}

// at returns the token at logical index idx.
// Fast path: direct array access when the token is already buffered.
func (tb *tokenBuffer) at(idx int) syn.Token {
	physical := idx - tb.base
	if physical >= 0 && physical < len(tb.buf) {
		return tb.buf[physical]
	}
	// Slow path: need to fill from scanner or out of range.
	tb.fill(idx)
	physical = idx - tb.base
	if physical < 0 || physical >= len(tb.buf) {
		return syn.Token{Kind: syn.TokEOF}
	}
	return tb.buf[physical]
}

// peek returns the token at the current position.
func (tb *tokenBuffer) peek() syn.Token {
	return tb.at(tb.pos)
}

// peekAt returns the token at pos+offset.
func (tb *tokenBuffer) peekAt(offset int) syn.Token {
	return tb.at(tb.pos + offset)
}

// advance moves pos forward by 1 and returns the consumed token.
func (tb *tokenBuffer) advance() syn.Token {
	tok := tb.peek()
	if tok.Kind != syn.TokEOF {
		tb.prev = tok
		tb.pos++
		tb.compactCount++
		if tb.compactCount >= compactThreshold {
			tb.compactCount = 0
			tb.maybeCompact()
		}
	}
	return tok
}

// position returns the current logical read position.
func (tb *tokenBuffer) position() int {
	return tb.pos
}

// prevToken returns the last consumed token.
func (tb *tokenBuffer) prevToken() syn.Token {
	return tb.prev
}

// prevEnd returns the End position of the previously consumed token.
func (tb *tokenBuffer) prevEnd() span.Pos {
	return tb.prev.S.End
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
const compactThreshold = 64

func (tb *tokenBuffer) maybeCompact() {
	watermark := tb.pos - 1 // retain previous token
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
		copy(tb.buf, tb.buf[drop:])
		tb.buf = tb.buf[:len(tb.buf)-drop]
		tb.base += drop
	}
}
