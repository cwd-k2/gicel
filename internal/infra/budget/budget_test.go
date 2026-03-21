// Budget tests — nesting limit (runtime) and compiler dimensions (CheckBudget).
// Does NOT cover: step, depth, alloc limits (tested via eval integration tests).
package budget

import (
	"context"
	"testing"
)

func TestNestUnest(t *testing.T) {
	b := New(context.Background(), 0, 0)
	b.SetNestingLimit(3)

	// Should succeed up to limit.
	for i := 0; i < 3; i++ {
		if err := b.Nest(); err != nil {
			t.Fatalf("Nest() #%d: unexpected error: %v", i+1, err)
		}
	}
	if b.Nesting() != 3 {
		t.Fatalf("expected nesting=3, got %d", b.Nesting())
	}

	// Exceeding limit should error.
	err := b.Nest()
	if err == nil {
		t.Fatal("expected NestingLimitError")
	}
	if _, ok := err.(*NestingLimitError); !ok {
		t.Fatalf("expected *NestingLimitError, got %T: %v", err, err)
	}

	// Exceeding Nest still incremented counter (consistent with Enter).
	// Unnest twice to get back within limit.
	b.Unnest()
	b.Unnest()
	if b.Nesting() != 2 {
		t.Fatalf("expected nesting=2 after two Unnest, got %d", b.Nesting())
	}

	// After unnest, Nest should succeed again.
	if err := b.Nest(); err != nil {
		t.Fatalf("Nest() after Unnest: unexpected error: %v", err)
	}
}

func TestNestingLimitZeroDisabled(t *testing.T) {
	b := New(context.Background(), 0, 0)
	// maxNesting=0 means disabled.
	for i := 0; i < 10000; i++ {
		if err := b.Nest(); err != nil {
			t.Fatalf("Nest() #%d: unexpected error with disabled limit: %v", i+1, err)
		}
	}
}

func TestNestingLimitNegativeClamped(t *testing.T) {
	b := New(context.Background(), 0, 0)
	b.SetNestingLimit(-5)
	if b.MaxNesting() != 0 {
		t.Fatalf("expected maxNesting=0 after negative input, got %d", b.MaxNesting())
	}
}

func TestCheckBudgetCompilerDimensions(t *testing.T) {
	b := NewCheck(context.Background())

	// --- TF step limit ---
	b.SetTFStepLimit(3)
	for i := 0; i < 3; i++ {
		if err := b.TFStep(); err != nil {
			t.Fatalf("TFStep() #%d: unexpected error: %v", i+1, err)
		}
	}
	err := b.TFStep()
	if err == nil {
		t.Fatal("expected TFStepLimitError after exceeding limit")
	}
	if _, ok := err.(*TFStepLimitError); !ok {
		t.Fatalf("expected *TFStepLimitError, got %T: %v", err, err)
	}

	// ResetTFSteps should allow new steps.
	b.ResetTFSteps()
	if err := b.TFStep(); err != nil {
		t.Fatalf("TFStep() after reset: unexpected error: %v", err)
	}

	// --- Solver step limit ---
	b.SetSolverStepLimit(2)
	for i := 0; i < 2; i++ {
		if err := b.SolverStep(); err != nil {
			t.Fatalf("SolverStep() #%d: unexpected error: %v", i+1, err)
		}
	}
	err = b.SolverStep()
	if err == nil {
		t.Fatal("expected SolverStepLimitError after exceeding limit")
	}
	if _, ok := err.(*SolverStepLimitError); !ok {
		t.Fatalf("expected *SolverStepLimitError, got %T: %v", err, err)
	}

	// ResetSolverSteps should allow new steps.
	b.ResetSolverSteps()
	if err := b.SolverStep(); err != nil {
		t.Fatalf("SolverStep() after reset: unexpected error: %v", err)
	}

	// --- Resolve depth limit ---
	b.SetResolveDepthLimit(2)
	for i := 0; i < 2; i++ {
		if err := b.EnterResolve(); err != nil {
			t.Fatalf("EnterResolve() #%d: unexpected error: %v", i+1, err)
		}
	}
	err = b.EnterResolve()
	if err == nil {
		t.Fatal("expected ResolveDepthLimitError after exceeding limit")
	}
	if _, ok := err.(*ResolveDepthLimitError); !ok {
		t.Fatalf("expected *ResolveDepthLimitError, got %T: %v", err, err)
	}

	// LeaveResolve + re-enter should succeed.
	b.LeaveResolve()
	b.LeaveResolve()
	if err := b.EnterResolve(); err != nil {
		t.Fatalf("EnterResolve() after leave: unexpected error: %v", err)
	}
}

func TestCheckBudgetNesting(t *testing.T) {
	b := NewCheck(context.Background())
	b.SetNestingLimit(3)

	for i := 0; i < 3; i++ {
		if err := b.Nest(); err != nil {
			t.Fatalf("Nest() #%d: unexpected error: %v", i+1, err)
		}
	}
	err := b.Nest()
	if err == nil {
		t.Fatal("expected NestingLimitError")
	}
	if _, ok := err.(*NestingLimitError); !ok {
		t.Fatalf("expected *NestingLimitError, got %T: %v", err, err)
	}
	b.Unnest()
	b.Unnest()
	if b.Nesting() != 2 {
		t.Fatalf("expected nesting=2 after two Unnest, got %d", b.Nesting())
	}
}
