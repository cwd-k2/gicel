package parse

import (
	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// Scanner produces tokens on demand from source text.
// It wraps the internal lexer, exposing a streaming interface
// for one-token-at-a-time consumption.
type Scanner struct {
	lexer *lexer
}

// NewScanner creates a streaming token scanner for the given source.
func NewScanner(source *span.Source) *Scanner {
	return &Scanner{lexer: newLexer(source)}
}

// Next returns the next token from the source.
// After EOF, subsequent calls return TokEOF repeatedly.
func (s *Scanner) Next() syn.Token {
	return s.lexer.next()
}

// Errors returns the accumulated lexer diagnostics.
func (s *Scanner) Errors() *diagnostic.Errors {
	return s.lexer.errors
}

// ScannerState captures the scanner's position for save/restore.
type ScannerState struct {
	pos        int
	sawNewline bool
}

// SaveState captures the current scanner state.
func (s *Scanner) SaveState() ScannerState {
	return ScannerState{pos: s.lexer.pos, sawNewline: s.lexer.sawNewline}
}

// RestoreState resets the scanner to a previously saved state.
func (s *Scanner) RestoreState(st ScannerState) {
	s.lexer.pos = st.pos
	s.lexer.sawNewline = st.sawNewline
}
