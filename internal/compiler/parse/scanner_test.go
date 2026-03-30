// Scanner tests — streaming interface equivalence with Tokenize.
// Does NOT cover: lexer edge cases (lexer_test.go, lexer_probe_test.go).
package parse

import (
	"testing"

	syn "github.com/cwd-k2/gicel/internal/lang/syntax"

	"github.com/cwd-k2/gicel/internal/infra/span"
)

// TestScannerTokenizeEquivalence verifies that Scanner.Next() produces
// the exact same token stream as Lexer.Tokenize().
func TestScannerTokenizeEquivalence(t *testing.T) {
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

			// Tokenize path (via NewScanner, collecting all tokens)
			lexScanner := NewScanner(source)
			var tokens []syn.Token
			for {
				tok := lexScanner.Next()
				tokens = append(tokens, tok)
				if tok.Kind == syn.TokEOF {
					break
				}
			}

			// Scanner path
			s := NewScanner(source)
			var scanned []syn.Token
			for {
				tok := s.Next()
				scanned = append(scanned, tok)
				if tok.Kind == syn.TokEOF {
					break
				}
			}

			if len(tokens) != len(scanned) {
				t.Fatalf("length mismatch: Tokenize=%d, Scanner=%d", len(tokens), len(scanned))
			}
			for i := range tokens {
				if tokens[i] != scanned[i] {
					t.Errorf("token %d: Tokenize=%v, Scanner=%v", i, tokens[i], scanned[i])
				}
			}
		})
	}
}

// TestScannerSaveRestore verifies that SaveState/RestoreState
// allows re-scanning from a saved position.
func TestScannerSaveRestore(t *testing.T) {
	source := span.NewSource("<test>", "a + b * c")
	s := NewScanner(source)

	tok1 := s.Next() // a
	if tok1.Text != "a" {
		t.Fatalf("expected 'a', got %q", tok1.Text)
	}

	state := s.SaveState()

	tok2 := s.Next() // +
	tok3 := s.Next() // b
	if tok2.Text != "+" || tok3.Text != "b" {
		t.Fatalf("expected '+' 'b', got %q %q", tok2.Text, tok3.Text)
	}

	s.RestoreState(state)

	// Re-scan from saved position
	tok2b := s.Next() // + again
	tok3b := s.Next() // b again
	if tok2b != tok2 || tok3b != tok3 {
		t.Errorf("restored tokens differ: got %v %v, want %v %v", tok2b, tok3b, tok2, tok3)
	}
}
