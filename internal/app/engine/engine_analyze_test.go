// Analyze tests — AnalysisResult partial results, HoverIndex integration, backward compatibility.
// Does NOT cover: HoverIndex data structure (hoverindex_test.go).

package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
)

func TestAnalyze_Success(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	ar := eng.Analyze(context.Background(), `import Prelude; main := 1 + 2`)
	if !ar.Complete {
		t.Fatalf("expected Complete, got errors: %v", ar.Errors.Format())
	}
	if ar.Program == nil {
		t.Fatal("expected non-nil Program")
	}
	if ar.Source == nil {
		t.Fatal("expected non-nil Source")
	}
	if len(ar.Program.Bindings) == 0 {
		t.Fatal("expected at least one binding")
	}
}

func TestAnalyze_ParseError(t *testing.T) {
	eng := NewEngine()
	ar := eng.Analyze(context.Background(), `main := (`)
	if ar.Complete {
		t.Fatal("expected !Complete for parse error")
	}
	if ar.Errors == nil || !ar.Errors.HasErrors() {
		t.Fatal("expected parse errors")
	}
	if ar.Program == nil {
		t.Fatal("Program should be non-nil (empty) even on parse error")
	}
}

func TestAnalyze_CheckError(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	ar := eng.Analyze(context.Background(), `import Prelude; main := 1 + "hello"`)
	if ar.Complete {
		t.Fatal("expected !Complete for type error")
	}
	if ar.Errors == nil || !ar.Errors.HasErrors() {
		t.Fatal("expected type errors")
	}
	// Partial IR: Program should still be non-nil with bindings.
	if ar.Program == nil {
		t.Fatal("expected non-nil partial Program on check error")
	}
}

func TestAnalyze_BackwardCompat_Compile(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	// Compile should still work as before.
	cr, err := eng.Compile(context.Background(), `import Prelude; main := 42`)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if cr == nil {
		t.Fatal("expected non-nil CompileResult")
	}
	types := cr.PrettyBindingTypes()
	if _, ok := types["main"]; !ok {
		t.Fatal("expected 'main' in binding types")
	}
}

func TestAnalyze_BackwardCompat_NewRuntime(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `import Prelude; main := 1 + 2`)
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	res, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("RunWith failed: %v", err)
	}
	if res.Value == nil {
		t.Fatal("expected non-nil result value")
	}
}

func TestAnalyze_WithHoverIndex(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableHoverIndex()
	ar := eng.Analyze(context.Background(), `import Prelude; main := 42`)
	if !ar.Complete {
		t.Fatalf("expected Complete, got errors: %v", ar.Errors.Format())
	}
	if ar.HoverIndex == nil {
		t.Fatal("expected non-nil HoverIndex when EnableHoverIndex is set")
	}
	if ar.HoverIndex.Len() == 0 {
		t.Fatal("expected HoverIndex to have entries")
	}
}

func TestAnalyze_WithoutHoverIndex(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	// TypeRecorder not enabled: HoverIndex should be nil.
	ar := eng.Analyze(context.Background(), `import Prelude; main := 42`)
	if ar.HoverIndex != nil {
		t.Fatal("expected nil HoverIndex when EnableHoverIndex is not called")
	}
}
