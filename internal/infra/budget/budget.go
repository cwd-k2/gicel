// Package budget provides resource limiters for the GICEL pipeline.
//
// [Budget] tracks runtime dimensions: step count, call depth, structural
// nesting, and allocation bytes. [CheckBudget] tracks compiler dimensions:
// structural nesting, type family reduction, constraint solving, and
// instance resolution. The two types draw from independent resource pools.
//
// Both carry a [context.Context] for external cancellation (e.g. timeout).
package budget

import (
	"context"
	"errors"
	"fmt"
)

// Budget tracks runtime resource consumption. It is not goroutine-safe;
// each execution should use its own instance.
type Budget struct {
	ctx        context.Context
	steps      int
	max        int
	depth      int
	maxDepth   int
	nesting    int
	maxNesting int
	alloc      int64
	maxAlloc   int64
}

// New creates a Budget with the given step and depth limits.
// ctx is checked on each Step call for external cancellation.
// A zero limit disables that dimension. Negative values are
// clamped to zero (disabled).
func New(ctx context.Context, maxSteps, maxDepth int) *Budget {
	if maxSteps < 0 {
		maxSteps = 0
	}
	if maxDepth < 0 {
		maxDepth = 0
	}
	return &Budget{ctx: ctx, max: maxSteps, maxDepth: maxDepth}
}

// SetNestingLimit sets the structural nesting depth limit. This bounds the
// Go call stack depth when evaluating nested expressions or traversing
// nested Core/Value structures. Zero disables the check.
// Negative values are treated as zero (disabled).
func (b *Budget) SetNestingLimit(n int) {
	if n < 0 {
		n = 0
	}
	b.maxNesting = n
}

// SetAllocLimit sets the allocation byte limit. Zero disables the check.
// Negative values are treated as zero (disabled).
func (b *Budget) SetAllocLimit(bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	b.maxAlloc = bytes
}

// checkCtx returns the context error if cancelled (non-blocking).
// The raw context error is wrapped in a GICEL-specific error type
// so users see "execution timed out" instead of "context deadline exceeded".
func (b *Budget) checkCtx() error {
	select {
	case <-b.ctx.Done():
		return wrapCtxErr(b.ctx.Err())
	default:
		return nil
	}
}

// Step records one unit of work. Returns an error if the step limit is
// exceeded or the context has been cancelled.
func (b *Budget) Step() error {
	if err := b.checkCtx(); err != nil {
		return err
	}
	b.steps++
	if b.max > 0 && b.steps > b.max {
		return &StepLimitError{}
	}
	return nil
}

// Enter increments nesting depth. Returns an error if the depth limit
// is exceeded.
func (b *Budget) Enter() error {
	b.depth++
	if b.maxDepth > 0 && b.depth > b.maxDepth {
		return &DepthLimitError{}
	}
	return nil
}

// Leave decrements nesting depth.
func (b *Budget) Leave() {
	b.depth--
}

// Nest increments structural nesting depth. Returns an error if the
// nesting limit is exceeded. Unlike Enter/Leave which track logical call
// depth (Bind, Closure, Force), Nest/Unnest track structural expression
// nesting to bound Go call stack usage in non-tail recursive evaluation.
func (b *Budget) Nest() error {
	b.nesting++
	if b.maxNesting > 0 && b.nesting > b.maxNesting {
		return &NestingLimitError{}
	}
	return nil
}

// Unnest decrements structural nesting depth.
func (b *Budget) Unnest() {
	b.nesting--
}

// Alloc records a heap allocation. Returns an error if the cumulative
// allocation exceeds the limit.
func (b *Budget) Alloc(bytes int64) error {
	b.alloc += bytes
	if b.maxAlloc > 0 && b.alloc > b.maxAlloc {
		return &AllocLimitError{Used: b.alloc, Limit: b.maxAlloc}
	}
	return nil
}

// --- Read accessors ---

// Steps returns the number of steps consumed so far.
func (b *Budget) Steps() int { return b.steps }

// Depth returns the current nesting depth.
func (b *Budget) Depth() int { return b.depth }

// Allocated returns the cumulative allocation in bytes.
func (b *Budget) Allocated() int64 { return b.alloc }

// Remaining returns the number of steps left before the limit.
// Returns -1 if the step limit is disabled (max == 0).
func (b *Budget) Remaining() int {
	if b.max <= 0 {
		return -1
	}
	r := b.max - b.steps
	if r < 0 {
		return 0
	}
	return r
}

// Max returns the configured step limit.
func (b *Budget) Max() int { return b.max }

// MaxDepth returns the configured depth limit.
func (b *Budget) MaxDepth() int { return b.maxDepth }

// Nesting returns the current structural nesting depth.
func (b *Budget) Nesting() int { return b.nesting }

// MaxNesting returns the configured nesting limit.
func (b *Budget) MaxNesting() int { return b.maxNesting }

// MaxAlloc returns the configured allocation limit.
func (b *Budget) MaxAlloc() int64 { return b.maxAlloc }

// Context returns the associated context.
func (b *Budget) Context() context.Context { return b.ctx }

// --- Error types ---

// LimitError is implemented by all resource limit errors.
// It distinguishes limit exhaustion (a global condition) from
// evaluation errors local to a specific binding.
type LimitError interface {
	error
	limitError() // marker method
}

// IsLimitError reports whether err is a resource limit error.
func IsLimitError(err error) bool {
	var le LimitError
	return errors.As(err, &le)
}

// StepLimitError indicates the step limit was exceeded.
type StepLimitError struct{}

func (e *StepLimitError) Error() string { return "step limit exceeded" }
func (e *StepLimitError) limitError()   {}

// DepthLimitError indicates the nesting depth limit was exceeded.
type DepthLimitError struct{}

func (e *DepthLimitError) Error() string { return "depth limit exceeded" }
func (e *DepthLimitError) limitError()   {}

// NestingLimitError indicates the structural nesting limit was exceeded.
type NestingLimitError struct{}

func (e *NestingLimitError) Error() string { return "nesting limit exceeded" }
func (e *NestingLimitError) limitError()   {}

// AllocLimitError indicates the allocation limit was exceeded.
type AllocLimitError struct {
	Used  int64
	Limit int64
}

func (e *AllocLimitError) Error() string {
	return fmt.Sprintf("allocation limit exceeded: %d bytes used, %d bytes allowed", e.Used, e.Limit)
}
func (e *AllocLimitError) limitError() {}

// TimeoutError indicates the execution timed out via context deadline.
// It wraps the underlying context error so errors.Is(err, context.DeadlineExceeded)
// continues to work.
type TimeoutError struct {
	Cause error
}

func (e *TimeoutError) Error() string { return "execution timed out" }
func (e *TimeoutError) Unwrap() error { return e.Cause }
func (e *TimeoutError) limitError()   {}

// CancelledError indicates the execution was cancelled externally.
// It wraps the underlying context error so errors.Is(err, context.Canceled)
// continues to work.
type CancelledError struct {
	Cause error
}

func (e *CancelledError) Error() string { return "execution cancelled" }
func (e *CancelledError) Unwrap() error { return e.Cause }
func (e *CancelledError) limitError()   {}

// wrapCtxErr wraps a raw context error in a user-friendly GICEL error type.
func wrapCtxErr(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &TimeoutError{Cause: err}
	}
	return &CancelledError{Cause: err}
}

// --- Context helpers ---

// budgetKey is the context key for embedding *Budget.
type budgetKey struct{}

// ContextWithBudget returns a context carrying the given Budget.
func ContextWithBudget(ctx context.Context, b *Budget) context.Context {
	return context.WithValue(ctx, budgetKey{}, b)
}

// ChargeAlloc charges bytes against the Budget embedded in ctx.
// Returns nil if ctx carries no Budget or the limit has not been set.
// Stdlib primitives call this for Go-level allocations invisible to the evaluator.
func ChargeAlloc(ctx context.Context, bytes int64) error {
	if b, ok := ctx.Value(budgetKey{}).(*Budget); ok {
		return b.Alloc(bytes)
	}
	return nil
}
