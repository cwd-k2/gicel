// Budget tests — unified check amortization, compiler dimensions (CheckBudget).
// Does NOT cover: step, depth, alloc limits (tested via eval integration tests).
package budget

import (
	"context"
	"testing"
)

func TestCheckBudgetCompilerDimensions(t *testing.T) {
	b := NewCheckBudget(context.Background())

	// --- TF step limit ---
	b.SetTFStepLimit(3)
	for i := range 3 {
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
	for i := range 2 {
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
	for i := range 2 {
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
	b := NewCheckBudget(context.Background())
	b.SetNestingLimit(3)

	for i := range 3 {
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

func TestCheckBudgetContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	b := NewCheckBudget(ctx)
	b.SetTFStepLimit(1000)
	b.SetSolverStepLimit(1000)

	if err := b.TFStep(); err == nil {
		t.Fatal("expected context error from TFStep after cancel")
	}
	if err := b.SolverStep(); err == nil {
		t.Fatal("expected context error from SolverStep after cancel")
	}
}

// TestBudgetUnifiedCheck verifies that the amortized check fires within
// the expected window and catches limit violations across all dimensions.
func TestBudgetUnifiedCheck(t *testing.T) {
	t.Run("StepLimit", func(t *testing.T) {
		b := New(context.Background(), 10, 0)
		// Exceed step limit, then pump ops until check fires.
		var hitErr bool
		for range 200 {
			if err := b.Step(); err != nil {
				if _, ok := err.(*StepLimitError); !ok {
					t.Fatalf("expected *StepLimitError, got %T: %v", err, err)
				}
				hitErr = true
				break
			}
		}
		if !hitErr {
			t.Fatal("expected StepLimitError within amortization window")
		}
	})

	t.Run("DepthLimit", func(t *testing.T) {
		b := New(context.Background(), 0, 5)
		var hitErr bool
		for range 200 {
			if err := b.Enter(); err != nil {
				if _, ok := err.(*DepthLimitError); !ok {
					t.Fatalf("expected *DepthLimitError, got %T: %v", err, err)
				}
				hitErr = true
				break
			}
		}
		if !hitErr {
			t.Fatal("expected DepthLimitError within amortization window")
		}
	})

	t.Run("AllocLimit", func(t *testing.T) {
		b := New(context.Background(), 0, 0)
		b.SetAllocLimit(100)
		var hitErr bool
		for range 200 {
			if err := b.Alloc(10); err != nil {
				if _, ok := err.(*AllocLimitError); !ok {
					t.Fatalf("expected *AllocLimitError, got %T: %v", err, err)
				}
				hitErr = true
				break
			}
		}
		if !hitErr {
			t.Fatal("expected AllocLimitError within amortization window")
		}
	})

	t.Run("PeakDepthTracked", func(t *testing.T) {
		b := New(context.Background(), 0, 0)
		// Enter 128 times to guarantee at least one check fires at
		// depth >= 64 (check fires every 64 ops).
		for range 128 {
			_ = b.Enter()
		}
		// PeakDepth is updated when check fires during Enter; the
		// exact value depends on which ops hit the 64-boundary.
		if b.PeakDepth() == 0 {
			t.Fatal("expected PeakDepth > 0 after 128 Enter calls")
		}
		if b.PeakDepth() > 128 {
			t.Fatalf("PeakDepth %d exceeds 128 Enter calls", b.PeakDepth())
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		b := New(ctx, 0, 0)
		var hitErr bool
		for range 200 {
			if err := b.Step(); err != nil {
				hitErr = true
				break
			}
		}
		if !hitErr {
			t.Fatal("expected context error within amortization window")
		}
	})
}
