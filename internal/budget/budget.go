// Package budget provides a unified resource limiter for all phases of the
// GICEL pipeline: parsing, type checking, optimization, and evaluation.
//
// A single Budget tracks step count, nesting depth, and allocation bytes.
// It also carries a [context.Context] for external cancellation (e.g. timeout).
// The same Budget instance can be shared across phases so that compilation
// and execution draw from one resource pool.
package budget

import (
	"context"
	"fmt"
)

// Budget tracks resource consumption. It is not goroutine-safe;
// each pipeline execution should use its own instance.
type Budget struct {
	ctx      context.Context
	steps    int
	max      int
	depth    int
	maxDepth int
	alloc    int64
	maxAlloc int64
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

// SetAllocLimit sets the allocation byte limit. Zero disables the check.
// Negative values are treated as zero (disabled).
func (b *Budget) SetAllocLimit(bytes int64) {
	if bytes < 0 {
		bytes = 0
	}
	b.maxAlloc = bytes
}

// Step records one unit of work. Returns an error if the step limit is
// exceeded or the context has been cancelled.
func (b *Budget) Step() error {
	select {
	case <-b.ctx.Done():
		return b.ctx.Err()
	default:
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

// Alloc records a heap allocation. Returns an error if the cumulative
// allocation exceeds the limit.
func (b *Budget) Alloc(bytes int64) error {
	b.alloc += bytes
	if b.maxAlloc > 0 && b.alloc > b.maxAlloc {
		return &AllocLimitError{Used: b.alloc, Limit: b.maxAlloc}
	}
	return nil
}

// ResetCounters zeroes the step and depth counters, keeping limits and context
// unchanged. Used to start a fresh phase (e.g. a new type family reduction
// pass) within the same budget instance.
func (b *Budget) ResetCounters() {
	b.steps = 0
	b.depth = 0
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

// MaxAlloc returns the configured allocation limit.
func (b *Budget) MaxAlloc() int64 { return b.maxAlloc }

// Context returns the associated context.
func (b *Budget) Context() context.Context { return b.ctx }

// --- Error types ---

// StepLimitError indicates the step limit was exceeded.
type StepLimitError struct{}

func (e *StepLimitError) Error() string { return "step limit exceeded" }

// DepthLimitError indicates the nesting depth limit was exceeded.
type DepthLimitError struct{}

func (e *DepthLimitError) Error() string { return "depth limit exceeded" }

// AllocLimitError indicates the allocation limit was exceeded.
type AllocLimitError struct {
	Used  int64
	Limit int64
}

func (e *AllocLimitError) Error() string {
	return fmt.Sprintf("allocation limit exceeded: %d bytes used, %d bytes allowed", e.Used, e.Limit)
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
