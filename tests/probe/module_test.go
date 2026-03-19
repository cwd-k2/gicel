//go:build probe

// Module interaction and registration edge-case tests.
package probe_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel"
)

// ===================================================================
// Probe C: Module interaction
// ===================================================================

func TestProbeC_Module_QualifiedConstructor(t *testing.T) {
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `
data Color := Red | Blue | Green
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Lib
main := Red
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, result.Value, "Red")
}

func TestProbeC_Module_QualifiedFunction(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	err := eng.RegisterModule("Lib", `
import Prelude
double :: Int -> Int
double := \x. x + x
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import Lib
main := double 21
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 42)
}

func TestProbeC_Module_PrivateNotExported(t *testing.T) {
	// _prefixed names should not be exported.
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `
data Bool := True | False
_internal := True
public := _internal
`)
	if err != nil {
		t.Fatal(err)
	}
	// Accessing public should work.
	rt, err := eng.NewRuntime(context.Background(), `
import Lib
main := public
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, result.Value, "True")
}

func TestProbeC_Module_PrivateDirectAccess(t *testing.T) {
	// Attempting to access _internal directly should fail at compile time.
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `
data Bool := True | False
_secret := True
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.NewRuntime(context.Background(), `
import Lib
main := _secret
`)
	if err == nil {
		t.Fatal("expected error accessing private binding from another module")
	}
}

func TestProbeC_Module_EmptyModule(t *testing.T) {
	// Register a module with only data declarations (no value bindings).
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `
data Unit := Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Lib
main := Unit
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertConVal(t, result.Value, "Unit")
}

func TestProbeC_Module_MultiModule_ChainedImport(t *testing.T) {
	eng := gicel.NewEngine()
	eng.Use(gicel.Prelude)
	err := eng.RegisterModule("A", `
import Prelude
valA :: Int
valA := 10
`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("B", `
import Prelude
import A
valB :: Int
valB := valA + 20
`)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
import B
main := valB
`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	probeAssertHostInt(t, result.Value, 30)
}

func TestProbeC_Module_RegisterModuleSyntaxError(t *testing.T) {
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Bad", `this is not valid @@@@`)
	if err == nil {
		t.Fatal("expected error for module with syntax errors")
	}
}

func TestProbeC_Module_DuplicateRegistration(t *testing.T) {
	eng := gicel.NewEngine()
	err := eng.RegisterModule("Lib", `data U := U`)
	if err != nil {
		t.Fatal(err)
	}
	err = eng.RegisterModule("Lib", `data V := V`)
	if err == nil {
		t.Fatal("expected error for duplicate module registration")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("expected 'already registered' in error, got: %v", err)
	}
}

func TestProbeC_Module_EmptyModuleName(t *testing.T) {
	eng := gicel.NewEngine()
	err := eng.RegisterModule("", `data U := U`)
	if err == nil {
		t.Fatal("expected error for empty module name")
	}
}
