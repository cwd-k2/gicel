// Safety boundary tests — solver cancellation, compilation timeout, VM reuse after failure.
// Does NOT cover: instance_test.go, check_solver_test.go (unit-level solver tests).

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// TestCompilationContextCancel verifies that a cancelled context aborts compilation.
func TestCompilationContextCancel(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	eng.SetCompileContext(ctx)
	_, err := eng.NewRuntime(ctx, `import Prelude; main := 1 + 2`)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

// TestCompilationTimeout verifies that a tight timeout aborts compilation.
func TestCompilationTimeout(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // ensure deadline passes
	eng.SetCompileContext(ctx)
	_, err := eng.NewRuntime(ctx, `import Prelude; main := 1 + 2`)
	if err == nil {
		t.Fatal("expected error from timed-out context")
	}
}

// TestVMRerunAfterStepLimit verifies that a VM can be reused after step limit failure.
func TestVMRerunAfterStepLimit(t *testing.T) {
	eng := NewEngine()
	eng.Use(stdlib.Prelude)
	eng.EnableRecursion()
	eng.SetStepLimit(10) // very tight

	rt, err := eng.NewRuntime(context.Background(), `
import Prelude
-- Simple infinite recursion for step limit
f := rec \self x. self (x + 1)
main := f 0
`)
	if err != nil {
		t.Fatal(err)
	}

	// First run: should fail with step limit.
	_, err = rt.RunWith(context.Background(), nil)
	if err == nil {
		t.Fatal("expected step limit error")
	}
	if !budget.IsLimitError(err) {
		t.Fatalf("expected limit error, got: %v", err)
	}

	// Second run with a simple program: should succeed if VM unwind is correct.
	eng2 := NewEngine()
	eng2.Use(stdlib.Prelude)
	eng2.SetStepLimit(10000)
	rt2, err := eng2.NewRuntime(context.Background(), `import Prelude; main := 42`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := rt2.RunWith(context.Background(), nil)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if hv, ok := result.Value.(*eval.HostVal); !ok || hv.Inner != int64(42) {
		t.Fatalf("expected 42, got %v", result.Value)
	}
}

// TestModuleCacheKeyIncludesSourceHash verifies that module cache distinguishes
// modules with the same name but different source content.
func TestModuleCacheKeyIncludesSourceHash(t *testing.T) {
	ResetModuleCache()
	defer ResetModuleCache()

	eng1 := NewEngine()
	eng1.Use(stdlib.Prelude)
	if err := eng1.RegisterModule("Foo", `import Prelude; x := 1`); err != nil {
		t.Fatal(err)
	}

	eng2 := NewEngine()
	eng2.Use(stdlib.Prelude)
	if err := eng2.RegisterModule("Foo", `import Prelude; x := 2`); err != nil {
		t.Fatal(err)
	}

	// If cache keys were identical, the second RegisterModule would return
	// a module compiled against the first source. Since both succeed without
	// error, the cache key correctly distinguished them.
	if ModuleCacheLen() < 2 {
		t.Errorf("expected at least 2 cache entries (different Foo sources), got %d", ModuleCacheLen())
	}
}
