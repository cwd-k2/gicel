//go:build probe

// Parser recovery probe tests — statement-level recovery and delimiter hints.
// Does NOT cover: parser_crash_probe_test.go (crash, depth, step limits).
package parse

import (
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
)

// TestProbeD_RecoveryInDoBlock verifies that a broken statement inside a
// do-block does not swallow subsequent valid statements.
func TestProbeD_RecoveryInDoBlock(t *testing.T) {
	source := `main := do {
  x <- pure 1
  + + +
  y <- pure 2
  pure y
}`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected errors for broken statement in do-block")
	}
	// The key invariant: error count should be contained, not cascading.
	if es.Len() > 6 {
		t.Errorf("do-block recovery: %d errors, expected ≤6", es.Len())
	}
}

// TestProbeD_RecoveryInCaseAlt verifies that a broken case alternative
// does not cascade into subsequent alternatives.
func TestProbeD_RecoveryInCaseAlt(t *testing.T) {
	source := `main := case x {
  True + => 1
  False => 2
}`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected errors for broken case alt")
	}
	// Errors should be contained to the broken alt.
	if es.Len() > 5 {
		t.Errorf("case alt recovery: %d errors, expected ≤5", es.Len())
	}
}

// TestProbeD_UnclosedBraceHint verifies that an unclosed { error carries
// a Hint pointing to the opening delimiter.
func TestProbeD_UnclosedBraceHint(t *testing.T) {
	source := `main := do { pure 1`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected error for unclosed brace")
	}
	found := false
	for _, e := range es.Errs {
		if e.Code == diagnostic.ErrUnclosedDelim && len(e.Hints) > 0 {
			for _, h := range e.Hints {
				if h.Message == "opening delimiter here" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected ErrUnclosedDelim with opening-delimiter hint, got: %s", es.Format())
	}
}

// TestProbeD_UnclosedParenHint verifies that an unclosed ( error carries
// a Hint pointing to the opening delimiter.
func TestProbeD_UnclosedParenHint(t *testing.T) {
	source := `main := (1, 2, 3`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected error for unclosed paren")
	}
	found := false
	for _, e := range es.Errs {
		if e.Code == diagnostic.ErrUnclosedDelim && len(e.Hints) > 0 {
			for _, h := range e.Hints {
				if h.Message == "opening delimiter here" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected ErrUnclosedDelim with opening-delimiter hint, got: %s", es.Format())
	}
}

// TestProbeD_UnclosedBracketHint verifies that an unclosed [ error carries
// a Hint pointing to the opening delimiter.
func TestProbeD_UnclosedBracketHint(t *testing.T) {
	source := `main := [1, 2`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected error for unclosed bracket")
	}
	found := false
	for _, e := range es.Errs {
		if e.Code == diagnostic.ErrUnclosedDelim && len(e.Hints) > 0 {
			for _, h := range e.Hints {
				if h.Message == "opening delimiter here" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("expected ErrUnclosedDelim with opening-delimiter hint, got: %s", es.Format())
	}
}

// TestProbeD_CaseAltMissingArrowRecovery verifies that a case alt with
// a missing => recovers and parses the next alt.
func TestProbeD_CaseAltMissingArrowRecovery(t *testing.T) {
	source := `main := case x {
  True 1
  False => 2
}`
	_, es := parse(source)
	if !es.HasErrors() {
		t.Fatal("expected errors for missing => in case alt")
	}
	// Should report the missing arrow but not cascade excessively.
	if es.Len() > 4 {
		t.Errorf("missing arrow recovery: %d errors, expected ≤4", es.Len())
	}
}
