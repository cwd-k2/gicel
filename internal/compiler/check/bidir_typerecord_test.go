// HoverRecorder tests — verifies the infer()/check() hooks record span→type pairs.
// Does NOT cover: HoverIndex data structure (engine/hoverindex_test.go).

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

// testRecorder is a minimal HoverRecorder for testing.
type testRecorder struct{ typeCount int }

func (r *testRecorder) RecordType(_ span.Span, _ types.Type) { r.typeCount++ }
func (r *testRecorder) RecordOperator(_ span.Span, _, _ string, _ types.Type) {}
func (r *testRecorder) RecordVarDoc(_ span.Span, _ string)                    {}
func (r *testRecorder) RecordDecl(_ span.Span, _, _ string, _ types.Type)     {}
func (r *testRecorder) Rezonk(_ func(types.Type) types.Type)                  {}

func parseAndCheck(t *testing.T, source string, cfg *CheckConfig) (*diagnostic.Errors, int) {
	t.Helper()
	rec := &testRecorder{}
	if cfg == nil {
		cfg = &CheckConfig{}
	}
	cfg.HoverRecorder = rec
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
	return checkErrs, rec.typeCount
}

func TestHoverRecorder_Called(t *testing.T) {
	errs, count := parseAndCheck(t, `main := 42`, nil)
	if errs.HasErrors() {
		t.Fatalf("unexpected errors: %s", errs.Format())
	}
	if count == 0 {
		t.Fatal("HoverRecorder.RecordType was never called")
	}
}

func TestHoverRecorder_NilSafe(t *testing.T) {
	// Verify no panic when HoverRecorder is nil.
	checkSource(t, `main := 42`, nil)
}

func TestHoverRecorder_MultipleExpressions(t *testing.T) {
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
