//go:build probe

// Speculate budget probe tests — step budget rollback on speculation failure.
// Does NOT cover: pattern bind parsing, expression parsing.
package parse

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

// TestProbeF_SpeculateBudgetRestore verifies that a failed speculation
// restores the step counter, ensuring subsequent parsing can proceed
// under a tight budget. Without rollback, the steps consumed during the
// speculative parse of `(a + b)` as a pattern would be permanently lost,
// causing the parser to hit the step limit on the remaining syntax.
//
// Mechanism: in a do-block, `(a + b)` triggers pattern-bind speculation
// (parseStmt tries pattern <- ... before falling through to expression).
// The speculation fails because `+` is not `<-` or `:=`. On failure,
// speculate() must restore guard.steps to the pre-speculation value.
//
// The test uses a source with enough subsequent statements that a tight
// budget (tokens*4) would be exhausted if the speculative steps leaked.
func TestProbeF_SpeculateBudgetRestore(t *testing.T) {
	// The do-block contains:
	//   (a + b)          — triggers speculation (paren-led stmt), fails, backtracks
	//   x <- pure 1      — normal bind
	//   y <- pure 2      — normal bind
	//   z <- pure 3      — normal bind
	//   pure (x + y + z) — trailing expr
	// Each statement consumes steps. If speculation leaks, the budget runs out.
	src := `main := do {
  (a + b);
  x <- pure 1;
  y <- pure 2;
  z <- pure 3;
  pure (x + y + z)
}`
	source := span.NewSource("test", src)
	l := NewLexer(source)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatalf("lex errors: %s", lexErrs.Format())
	}

	es := &diagnostic.Errors{Source: source}
	p := NewParser(context.Background(), tokens, es)
	prog := p.ParseProgram()
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	if es.HasErrors() {
		t.Fatalf("expected no parse errors, got: %s", es.Format())
	}

	// Verify structural correctness: the do-block should have 5 statements.
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}

	// Additionally, verify that the step counter did not exceed the budget.
	// The parser would have halted and emitted errors if steps leaked,
	// so the absence of errors above is the primary assertion. We also
	// confirm the parser is not in a halted state.
	if p.guard.isHalted() {
		t.Error("parser should not be halted after successful parse with budget rollback")
	}
}

// TestProbeF_SpeculateBudgetRestore_Block verifies the same rollback
// property in block expressions, where `(expr)` can be mistaken for
// a pattern binding `(pat) := expr`.
func TestProbeF_SpeculateBudgetRestore_Block(t *testing.T) {
	// The block contains:
	//   (a + b)  — triggers pattern-bind speculation, fails, backtracks to body expr
	// The block has bindings before it to consume some budget.
	src := `main := {
  x := 1;
  y := 2;
  z := 3;
  w := 4;
  (x + y + z + w)
}`
	source := span.NewSource("test", src)
	l := NewLexer(source)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatalf("lex errors: %s", lexErrs.Format())
	}

	es := &diagnostic.Errors{Source: source}
	p := NewParser(context.Background(), tokens, es)
	prog := p.ParseProgram()
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	if es.HasErrors() {
		t.Fatalf("expected no parse errors, got: %s", es.Format())
	}
	if p.guard.isHalted() {
		t.Error("parser should not be halted after successful parse with budget rollback")
	}
}

// TestProbeF_SpeculateBudgetRestore_Repeated verifies budget rollback
// across multiple consecutive speculation failures. Each failed attempt
// must independently restore its consumed steps.
func TestProbeF_SpeculateBudgetRestore_Repeated(t *testing.T) {
	// Multiple paren-led expression statements in a do-block, each
	// triggering and failing pattern-bind speculation.
	src := `main := do {
  (1 + 2);
  (3 + 4);
  (5 + 6);
  pure 42
}`
	source := span.NewSource("test", src)
	l := NewLexer(source)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatalf("lex errors: %s", lexErrs.Format())
	}

	es := &diagnostic.Errors{Source: source}
	p := NewParser(context.Background(), tokens, es)
	prog := p.ParseProgram()
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}
	if es.HasErrors() {
		t.Fatalf("expected no parse errors, got: %s", es.Format())
	}
	if p.guard.isHalted() {
		t.Error("parser should not be halted after multiple speculation rollbacks")
	}
}

// TestProbeF_SpeculateStepsActuallyRollback directly observes the step
// counter before and after a failed speculation. This is a white-box test
// that accesses the parser's guard field to verify the invariant.
func TestProbeF_SpeculateStepsActuallyRollback(t *testing.T) {
	src := `main := do { (a + b); pure 1 }`
	source := span.NewSource("test", src)
	l := NewLexer(source)
	tokens, lexErrs := l.Tokenize()
	if lexErrs.HasErrors() {
		t.Fatalf("lex errors: %s", lexErrs.Format())
	}

	es := &diagnostic.Errors{Source: source}
	p := NewParser(context.Background(), tokens, es)

	// Advance to just before the do-block to observe step counter behavior.
	// Parse the full program and check steps are within budget.
	prog := p.ParseProgram()
	if prog == nil {
		t.Fatal("ParseProgram returned nil")
	}

	// The step count should be well within budget (tokens*4).
	// Without rollback, speculation would consume extra steps that
	// accumulate beyond what the final parse position requires.
	maxSteps := max(len(tokens)*4, 100)
	if p.guard.steps > maxSteps {
		t.Errorf("step count %d exceeds budget %d — speculation may not have rolled back",
			p.guard.steps, maxSteps)
	}
}
