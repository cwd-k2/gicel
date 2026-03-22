// Prelude module tests — prelude modes, core availability, custom prelude.
// Does NOT cover: general module system (module_test.go), prelude type class instances (prelude_test.go).

package engine

import (
	"context"
	"testing"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

func TestPreludeAsModule(t *testing.T) {
	// Prelude is now loaded as an implicit module — Bool, (), etc. should be available.
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestNoPreludeWithModules(t *testing.T) {
	eng := NewEngine()
	// Without prelude, Bool is not defined — need to define it ourselves.
	rt, err := eng.NewRuntime(context.Background(), `
data MyBool := Yes | No
main := Yes
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Yes" {
		t.Errorf("expected Yes, got %s", result.Value)
	}
}

func TestSetPreludeCustom(t *testing.T) {
	// Custom prelude replaces default: only defines MyBool, no standard Bool.
	eng := NewEngine()
	eng.RegisterModule("Prelude", `
data MyBool := Yes | No
`)
	rt, err := eng.NewRuntime(context.Background(), "import Prelude\nmain := Yes")
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "Yes" {
		t.Errorf("expected Yes, got %s", result.Value)
	}
}

func TestSetPreludeCoreStillAvailable(t *testing.T) {
	// Core definitions (IxMonad, Effect, then) available even with custom Prelude.
	eng := NewEngine()
	eng.RegisterModule("Prelude", `
data Bool := { True: Bool; False: Bool; }
`)
	// Effect and then come from CoreSource, Bool from custom prelude.
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main :: Effect {} Bool
main := pure True
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	con, ok := result.Value.(*eval.ConVal)
	if !ok || con.Con != "True" {
		t.Errorf("expected True, got %s", result.Value)
	}
}

func TestSetPreludeNoDefaultBool(t *testing.T) {
	// Custom prelude that doesn't define Bool — standard Bool should not be available.
	eng := NewEngine()
	eng.RegisterModule("Prelude", `data Color := Red | Blue`)
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := True
`)
	if err == nil {
		t.Error("expected compile error: True not defined with custom prelude")
	}
}
