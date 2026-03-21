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
	ctx        context.Context
	steps      int
	max        int
	depth      int
	maxDepth   int
	nesting    int
	maxNesting int
	alloc      int64
	maxAlloc   int64

	// Compiler dimensions — type checking resource limits.
	tfSteps         int
	maxTFSteps      int
	solverSteps     int
	maxSolverSteps  int
	resolveDepth    int
	maxResolveDepth int
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
// Go call stack depth when evaluating nested expressions, unifying nested
// types, or traversing nested Core/Value structures. Zero disables the check.
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

// SetTFStepLimit sets the type family reduction step limit.
// Zero disables the check. Negative values are treated as zero.
func (b *Budget) SetTFStepLimit(n int) {
	if n < 0 {
		n = 0
	}
	b.maxTFSteps = n
}

// SetSolverStepLimit sets the constraint solver step limit.
// Zero disables the check. Negative values are treated as zero.
func (b *Budget) SetSolverStepLimit(n int) {
	if n < 0 {
		n = 0
	}
	b.maxSolverSteps = n
}

// SetResolveDepthLimit sets the instance resolution depth limit.
// Zero disables the check. Negative values are treated as zero.
func (b *Budget) SetResolveDepthLimit(n int) {
	if n < 0 {
		n = 0
	}
	b.maxResolveDepth = n
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

// TFStep records one type family reduction step. Returns an error if the
// TF step limit is exceeded or the context has been cancelled.
func (b *Budget) TFStep() error {
	select {
	case <-b.ctx.Done():
		return b.ctx.Err()
	default:
	}
	b.tfSteps++
	if b.maxTFSteps > 0 && b.tfSteps > b.maxTFSteps {
		return &TFStepLimitError{}
	}
	return nil
}

// ResetTFSteps zeroes the TF step counter for a fresh reduction pass.
func (b *Budget) ResetTFSteps() {
	b.tfSteps = 0
}

// SolverStep records one constraint solver iteration. Returns an error
// if the solver step limit is exceeded or the context has been cancelled.
func (b *Budget) SolverStep() error {
	select {
	case <-b.ctx.Done():
		return b.ctx.Err()
	default:
	}
	b.solverSteps++
	if b.maxSolverSteps > 0 && b.solverSteps > b.maxSolverSteps {
		return &SolverStepLimitError{}
	}
	return nil
}

// ResetSolverSteps zeroes the solver step counter.
func (b *Budget) ResetSolverSteps() {
	b.solverSteps = 0
}

// EnterResolve increments instance resolution depth. Returns an error
// if the depth limit is exceeded.
func (b *Budget) EnterResolve() error {
	b.resolveDepth++
	if b.maxResolveDepth > 0 && b.resolveDepth > b.maxResolveDepth {
		return &ResolveDepthLimitError{}
	}
	return nil
}

// LeaveResolve decrements instance resolution depth.
func (b *Budget) LeaveResolve() {
	b.resolveDepth--
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

// TFSteps returns the number of TF reduction steps consumed so far.
func (b *Budget) TFSteps() int { return b.tfSteps }

// MaxTFSteps returns the configured TF step limit.
func (b *Budget) MaxTFSteps() int { return b.maxTFSteps }

// SolverSteps returns the number of solver steps consumed so far.
func (b *Budget) SolverSteps() int { return b.solverSteps }

// MaxSolverSteps returns the configured solver step limit.
func (b *Budget) MaxSolverSteps() int { return b.maxSolverSteps }

// ResolveDepth returns the current instance resolution depth.
func (b *Budget) ResolveDepth() int { return b.resolveDepth }

// MaxResolveDepth returns the configured resolve depth limit.
func (b *Budget) MaxResolveDepth() int { return b.maxResolveDepth }

// Context returns the associated context.
func (b *Budget) Context() context.Context { return b.ctx }

// --- Error types ---

// StepLimitError indicates the step limit was exceeded.
type StepLimitError struct{}

func (e *StepLimitError) Error() string { return "step limit exceeded" }

// DepthLimitError indicates the nesting depth limit was exceeded.
type DepthLimitError struct{}

func (e *DepthLimitError) Error() string { return "depth limit exceeded" }

// NestingLimitError indicates the structural nesting limit was exceeded.
type NestingLimitError struct{}

func (e *NestingLimitError) Error() string { return "nesting limit exceeded" }

// AllocLimitError indicates the allocation limit was exceeded.
type AllocLimitError struct {
	Used  int64
	Limit int64
}

func (e *AllocLimitError) Error() string {
	return fmt.Sprintf("allocation limit exceeded: %d bytes used, %d bytes allowed", e.Used, e.Limit)
}

// TFStepLimitError indicates the type family reduction step limit was exceeded.
type TFStepLimitError struct{}

func (e *TFStepLimitError) Error() string { return "type family reduction step limit exceeded" }

// SolverStepLimitError indicates the constraint solver step limit was exceeded.
type SolverStepLimitError struct{}

func (e *SolverStepLimitError) Error() string { return "constraint solver step limit exceeded" }

// ResolveDepthLimitError indicates the instance resolution depth limit was exceeded.
type ResolveDepthLimitError struct{}

func (e *ResolveDepthLimitError) Error() string { return "instance resolution depth limit exceeded" }

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
