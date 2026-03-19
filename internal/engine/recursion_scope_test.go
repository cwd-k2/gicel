package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/cwd-k2/gicel/internal/stdlib"
)

// TestEnableRecursionScopedToModule verifies that EnableRecursion
// called by a stdlib pack does NOT leak fix/rec into user code
// compiled later on the same engine.
func TestEnableRecursionScopedToModule(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)

	// Stream pack calls EnableRecursion internally.
	if err := eng.Use(stdlib.Stream); err != nil {
		t.Fatal(err)
	}

	// User code should NOT have access to fix.
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
main := fix (\self x. x) True
`)
	if err == nil {
		t.Fatal("expected compile error: fix should not be in scope for user code")
	}
	if !strings.Contains(err.Error(), "fix") {
		t.Fatalf("expected error about 'fix', got: %v", err)
	}
}

// TestEnableRecursionWorksInModule verifies that the module
// registered with EnableRecursion CAN use fix.
func TestEnableRecursionWorksInModule(t *testing.T) {
	eng := NewEngine()
	if err := eng.Use(stdlib.Prelude); err != nil {
		t.Fatal(err)
	}
	// Stream's toList uses fix internally — verify it works.
	if err := eng.Use(stdlib.Stream); err != nil {
		t.Fatal(err)
	}
	_, err := eng.NewRuntime(context.Background(), `
import Prelude
import Data.Stream
main := toList (fromList (Cons True Nil) :: Stream Bool)
`)
	if err != nil {
		t.Fatalf("Stream module should compile and work: %v", err)
	}
}

// TestExplicitEnableRecursionStillWorks verifies that a user who
// explicitly calls EnableRecursion gets fix/rec in their code.
func TestExplicitEnableRecursionStillWorks(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
main := fix (\self x. x) True
`)
	if err != nil {
		t.Fatalf("explicit EnableRecursion should work: %v", err)
	}
	_ = rt
}
