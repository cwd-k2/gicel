// TypeRecorder tests — verifies the infer() hook records span→type pairs.
// Does NOT cover: TypeIndex data structure (engine/typeindex_test.go).

package check

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/compiler/desugar"
	"github.com/cwd-k2/gicel/internal/compiler/parse"
	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
	"github.com/cwd-k2/gicel/internal/lang/types"
)

func parseAndCheck(t *testing.T, source string, cfg *CheckConfig) (*diagnostic.Errors, int) {
	t.Helper()
	var count int
	if cfg == nil {
		cfg = &CheckConfig{}
	}
	cfg.TypeRecorder = func(_ span.Span, _ types.Type) {
		count++
	}
	src := span.NewSource("test", source)
	es := &diagnostic.Errors{Source: src}
	p := parse.NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		t.Fatal("lex errors:", p.LexErrors().Format())
	}
	if es.HasErrors() {
		t.Fatal("parse errors:", es.Format())
	}
	desugar.Program(ast)
	_, checkErrs := Check(ast, src, cfg)
	return checkErrs, count
}

func TestTypeRecorder_Called(t *testing.T) {
	errs, count := parseAndCheck(t, `main := 42`, nil)
	if errs.HasErrors() {
		t.Fatalf("unexpected errors: %s", errs.Format())
	}
	if count == 0 {
		t.Fatal("TypeRecorder was never called")
	}
}

func TestTypeRecorder_NilSafe(t *testing.T) {
	// Verify no panic when TypeRecorder is nil.
	checkSource(t, `main := 42`, nil)
}

func TestTypeRecorder_MultipleExpressions(t *testing.T) {
	errs, count := parseAndCheck(t, `
x := 1
y := 2
main := x
`, nil)
	if errs.HasErrors() {
		t.Fatalf("unexpected errors: %s", errs.Format())
	}
	if count < 3 {
		t.Fatalf("expected at least 3 recordings, got %d", count)
	}
}
