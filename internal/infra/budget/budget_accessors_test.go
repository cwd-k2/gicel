// Budget accessor and limit-error tests — getter coverage and IsLimitError
// dispatch over every error type declared in budget.go and check.go.
// Does NOT cover: actual step/depth/alloc enforcement (budget_test.go).

package budget

import (
	"context"
	"errors"
	"testing"
)

func TestBudgetAccessors(t *testing.T) {
	b := New(context.Background(), 1000, 50)
	b.SetAllocLimit(1 << 20) // 1 MiB

	if got := b.Max(); got != 1000 {
		t.Errorf("Max() = %d, want 1000", got)
	}
	if got := b.MaxDepth(); got != 50 {
		t.Errorf("MaxDepth() = %d, want 50", got)
	}
	if got := b.MaxAlloc(); got != 1<<20 {
		t.Errorf("MaxAlloc() = %d, want %d", got, 1<<20)
	}
	if got := b.Steps(); got != 0 {
		t.Errorf("Steps() = %d, want 0", got)
	}
	if got := b.Depth(); got != 0 {
		t.Errorf("Depth() = %d, want 0", got)
	}
	if got := b.Allocated(); got != 0 {
		t.Errorf("Allocated() = %d, want 0", got)
	}
	if got := b.Remaining(); got != 1000 {
		t.Errorf("Remaining() = %d, want 1000", got)
	}
	if got := b.PeakDepth(); got != 0 {
		t.Errorf("PeakDepth() = %d, want 0", got)
	}
	if b.Context() == nil {
		t.Error("Context() returned nil")
	}

	// Step/Alloc increment and are observable via accessors.
	if err := b.Step(); err != nil {
		t.Fatalf("Step(): %v", err)
	}
	if got := b.Steps(); got != 1 {
		t.Errorf("Steps() after one Step = %d, want 1", got)
	}
	if got := b.Remaining(); got != 999 {
		t.Errorf("Remaining() after one Step = %d, want 999", got)
	}
	if err := b.Alloc(256); err != nil {
		t.Fatalf("Alloc(256): %v", err)
	}
	if got := b.Allocated(); got != 256 {
		t.Errorf("Allocated() after Alloc(256) = %d, want 256", got)
	}
}

func TestBudgetRemainingDisabled(t *testing.T) {
	b := New(context.Background(), 0, 0) // step limit disabled
	if got := b.Remaining(); got != -1 {
		t.Errorf("Remaining() with max=0: %d, want -1 (disabled)", got)
	}
}

func TestBudgetRemainingExhausted(t *testing.T) {
	b := New(context.Background(), 5, 0)
	for range 5 {
		_ = b.Step()
	}
	_ = b.Step() // overshoot
	if got := b.Remaining(); got != 0 {
		t.Errorf("Remaining() after overshoot = %d, want 0 (clamped)", got)
	}
}

func TestCheckBudgetAccessors(t *testing.T) {
	b := NewCheck(context.Background())
	b.SetNestingLimit(10)
	b.SetTFStepLimit(100)
	b.SetSolverStepLimit(200)
	b.SetResolveDepthLimit(30)

	if got := b.MaxNesting(); got != 10 {
		t.Errorf("MaxNesting() = %d, want 10", got)
	}
	if got := b.MaxTFSteps(); got != 100 {
		t.Errorf("MaxTFSteps() = %d, want 100", got)
	}
	if got := b.MaxSolverSteps(); got != 200 {
		t.Errorf("MaxSolverSteps() = %d, want 200", got)
	}
	if got := b.MaxResolveDepth(); got != 30 {
		t.Errorf("MaxResolveDepth() = %d, want 30", got)
	}
	if got := b.Nesting(); got != 0 {
		t.Errorf("Nesting() = %d, want 0", got)
	}
	if got := b.TFSteps(); got != 0 {
		t.Errorf("TFSteps() = %d, want 0", got)
	}
	if got := b.SolverSteps(); got != 0 {
		t.Errorf("SolverSteps() = %d, want 0", got)
	}
	if got := b.ResolveDepth(); got != 0 {
		t.Errorf("ResolveDepth() = %d, want 0", got)
	}
}

// TestIsLimitErrorDispatch verifies that every declared limit error type
// satisfies the LimitError interface and is recognized by IsLimitError.
// This is a guard against adding a new error type in budget/ without the
// limitError() marker — a latent bug class we want to catch at compile time.
func TestIsLimitErrorDispatch(t *testing.T) {
	cases := []struct {
		name string
		err  error
		msg  string
	}{
		{"StepLimit", &StepLimitError{}, "step limit exceeded"},
		{"DepthLimit", &DepthLimitError{}, "depth limit exceeded"},
		{"NestingLimit", &NestingLimitError{}, "nesting limit exceeded"},
		{"AllocLimit", &AllocLimitError{Used: 200, Limit: 100}, "allocation limit exceeded: 200 bytes used, 100 bytes allowed"},
		{"Timeout", &TimeoutError{Cause: context.DeadlineExceeded}, "execution timed out"},
		{"Cancelled", &CancelledError{Cause: context.Canceled}, "execution cancelled"},
		{"TFStepLimit", &TFStepLimitError{}, "type family reduction step limit exceeded"},
		{"SolverStepLimit", &SolverStepLimitError{}, "constraint solver step limit exceeded"},
		{"ResolveDepthLimit", &ResolveDepthLimitError{}, "instance resolution depth limit exceeded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err.Error() != tc.msg {
				t.Errorf("Error() = %q, want %q", tc.err.Error(), tc.msg)
			}
			if !IsLimitError(tc.err) {
				t.Errorf("IsLimitError(%T) = false, want true", tc.err)
			}
		})
	}

	// Negative: a non-limit error must not match.
	if IsLimitError(errors.New("user code error")) {
		t.Error("IsLimitError(plain error) = true, want false")
	}
}

// TestTimeoutAndCancelledUnwrap verifies Unwrap behaviour for errors.Is
// compatibility with context.DeadlineExceeded / context.Canceled.
func TestTimeoutAndCancelledUnwrap(t *testing.T) {
	timeout := &TimeoutError{Cause: context.DeadlineExceeded}
	if !errors.Is(timeout, context.DeadlineExceeded) {
		t.Error("TimeoutError should unwrap to context.DeadlineExceeded")
	}
	cancelled := &CancelledError{Cause: context.Canceled}
	if !errors.Is(cancelled, context.Canceled) {
		t.Error("CancelledError should unwrap to context.Canceled")
	}
}

// TestWrapCtxErr checks the public wrapper for ctx-derived errors.
// WrapCtxErr is documented to wrap context errors; any non-deadline input
// falls into the CancelledError branch. Callers should only pass ctx errors.
func TestWrapCtxErr(t *testing.T) {
	timeout, ok := WrapCtxErr(context.DeadlineExceeded).(*TimeoutError)
	if !ok {
		t.Errorf("WrapCtxErr(DeadlineExceeded): got %T, want *TimeoutError", timeout)
	} else if !errors.Is(timeout, context.DeadlineExceeded) {
		t.Error("wrapped TimeoutError should unwrap to DeadlineExceeded")
	}
	cancelled, ok := WrapCtxErr(context.Canceled).(*CancelledError)
	if !ok {
		t.Errorf("WrapCtxErr(Canceled): got %T, want *CancelledError", cancelled)
	} else if !errors.Is(cancelled, context.Canceled) {
		t.Error("wrapped CancelledError should unwrap to Canceled")
	}
}
