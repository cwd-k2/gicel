// Scanner tests — tokenBuffer equivalence with raw Scanner.
// Does NOT cover: lexer edge cases (lexer_probe_test.go).
package parse

import (
	"testing"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// TestScannerTokenBufferEquivalence verifies that tokenBuffer (peek/advance)
// produces the exact same token stream as raw Scanner.Next().
func TestScannerTokenBufferEquivalence(t *testing.T) {
	sources := []string{
		"",
		"main := 42",
		"import Prelude; main := 1 + 2",
		"f x y := case x { True -> y; False -> 0 }",
		`name := "hello\nworld"`,
		"infixl 6 +; infixr 5 ++",
		"import Effect.State; import Data.Map as M",
		"form Bool := True | False",
		"-- comment\nmain := 1",
		"{- block {- nested -} -} main := 1",
	}

	for _, src := range sources {
		t.Run(src[:min(len(src), 30)], func(t *testing.T) {
			source := span.NewSource("<test>", src)

			// Raw Scanner path
			s := NewScanner(source)
			var raw []syn.Token
			for {
				tok := s.Next()
				raw = append(raw, tok)
				if tok.Kind == syn.TokEOF {
					break
				}
			}

			// tokenBuffer path
			source2 := span.NewSource("<test>", src)
			tb := newTokenBuffer(NewScanner(source2))
			var buffered []syn.Token
			for {
				tok := tb.advance()
				buffered = append(buffered, tok)
				if tok.Kind == syn.TokEOF {
					break
				}
			}

			if len(raw) != len(buffered) {
				t.Fatalf("length mismatch: raw=%d, buffered=%d", len(raw), len(buffered))
			}
			for i := range raw {
				if raw[i] != buffered[i] {
					t.Errorf("token %d: raw=%v, buffered=%v", i, raw[i], buffered[i])
				}
			}
		})
	}
}
