// Boundary tests — type errors, limits, context cancel/timeout, error paths.
// Does NOT cover: host API config (host_api_test.go), value conversion (value_conversion_test.go).

package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/cwd-k2/gicel/internal/stdlib"
)

func TestTypeErrorUnboundVar(t *testing.T) {
	eng := NewEngine()
	_, err := eng.NewRuntime(context.Background(), `main := totally_undefined`)
	if err == nil {
		t.Fatal("expected compile error for unbound variable")
	}
	if _, ok := err.(*CompileError); !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

func TestTypeErrorNonExhaustive(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x { True -> () }
main := f True
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive pattern")
	}
	ce, ok := err.(*CompileError)
	if !ok {
		t.Fatalf("expected CompileError, got %T", err)
	}
	if !strings.Contains(ce.Error(), "non-exhaustive") {
		t.Errorf("expected non-exhaustive in error message, got: %s", ce.Error())
	}
}

func TestTypeErrorMismatch(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	// Apply True (a Bool, not a function) to an argument.
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := True ()
`)
	if err == nil {
		t.Fatal("expected compile error for type mismatch")
	}
	if _, ok := err.(*CompileError); !ok {
		t.Errorf("expected CompileError, got %T", err)
	}
}

func TestContextCancellation(t *testing.T) {
	eng := NewEngine()
	rt, err := eng.NewRuntime(context.Background(), `main := pure ()`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	_, err = rt.RunWith(ctx, nil)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestContextTimeout(t *testing.T) {
	eng := NewEngine()
	eng.SetStepLimit(1) // extremely low step limit
	eng.EnableRecursion()
	rt, err := eng.NewRuntime(context.Background(), `
data Unit := Unit
data Bool := True | False
not := \b. case b { True -> False; False -> True }
main := not (not (not True))
`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err = rt.RunWith(ctx, nil)
	if err == nil {
		t.Error("expected error from step limit or timeout")
	}
}

func TestDepthLimitError(t *testing.T) {
	eng := NewEngine()
	eng.EnableRecursion()
	eng.SetDepthLimit(5)
	rt, err := eng.NewRuntime(context.Background(), `
deep := fix (\self x. self x)
main := deep ()
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for deep recursion")
	}
}

func TestPrimImplError(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := div 1 0
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for division by zero")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected division by zero error, got: %v", err)
	}
}

func TestMissingBinding(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.RegisterType("Int", KindType())
	eng.DeclareBinding("x", ConType("Int"))
	rt, err := eng.NewRuntime(context.Background(), `main := x`)
	if err != nil {
		t.Fatal(err)
	}
	// Run without providing the binding.
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing binding")
	}
	if !strings.Contains(err.Error(), "missing binding") {
		t.Fatalf("expected missing binding error, got: %v", err)
	}
}

func TestMissingEntryPoint(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
foo := True
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for missing entry point")
	}
	if !strings.Contains(err.Error(), "entry point") {
		t.Fatalf("expected entry point error, got: %v", err)
	}
}

// Non-exhaustive case on Maybe must name the missing constructor.
func TestNonExhaustiveMaybe(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
f := \x. case x { Just y -> y }
main := f (Just True)
`)
	if err == nil {
		t.Fatal("expected compile error for non-exhaustive case on Maybe")
	}
	ce := err.(*CompileError)
	if !strings.Contains(ce.Error(), "Nothing") {
		t.Errorf("expected missing constructor 'Nothing', got: %s", ce.Error())
	}
}

func TestParseChecksSyntax(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	if err := eng.Parse(`main := True`); err != nil {
		t.Fatal(err)
	}
}

func TestCheckReturnsOpaque(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	core, err := eng.Compile(context.Background(), `
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	if core == nil {
		t.Fatal("expected non-nil CoreProgram")
	}
	if core.Pretty() == "" {
		t.Error("expected non-empty Pretty output")
	}
}

func TestProgramOpaque(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	prog := rt.Program()
	if prog == nil {
		t.Fatal("expected non-nil CoreProgram from Runtime.Program()")
	}
	if prog.Pretty() == "" {
		t.Error("expected non-empty Pretty output from Runtime.Program()")
	}
}
