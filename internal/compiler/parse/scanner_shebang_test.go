// Shebang tests — verifies #! line skipping in the scanner.
// Does NOT cover: BOM handling, token scanning.

package parse

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/infra/diagnostic"
	"github.com/cwd-k2/gicel/internal/infra/span"
)

func TestShebang_Skipped(t *testing.T) {
	src := span.NewSource("test", "#!/usr/bin/env gicel run\nmain := 42\n")
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		t.Fatalf("lex errors: %s", p.LexErrors().Format())
	}
	if es.HasErrors() {
		t.Fatalf("parse errors: %s", es.Format())
	}
	if len(ast.Decls) == 0 {
		t.Fatal("expected at least one declaration")
	}
}

func TestShebang_WithBOM(t *testing.T) {
	src := span.NewSource("test", "\xEF\xBB\xBF#!/usr/bin/env gicel\nmain := 42\n")
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		t.Fatalf("lex errors: %s", p.LexErrors().Format())
	}
	if es.HasErrors() {
		t.Fatalf("parse errors: %s", es.Format())
	}
	if len(ast.Decls) == 0 {
		t.Fatal("expected at least one declaration")
	}
}

func TestShebang_NoShebang(t *testing.T) {
	src := span.NewSource("test", "main := 42\n")
	es := &diagnostic.Errors{Source: src}
	p := NewParser(context.Background(), src, es)
	ast := p.ParseProgram()
	if p.LexErrors().HasErrors() {
		t.Fatalf("lex errors: %s", p.LexErrors().Format())
	}
	if es.HasErrors() {
		t.Fatalf("parse errors: %s", es.Format())
	}
	if len(ast.Decls) == 0 {
		t.Fatal("expected at least one declaration")
	}
}
