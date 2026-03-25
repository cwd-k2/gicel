// CheckBudget tracks resource consumption during type checking.
// It is separate from [Budget] (which serves the runtime) because
// the compiler and evaluator draw from independent resource pools
// with non-overlapping dimension sets.
//
// Not goroutine-safe; each compilation should use its own instance.
package budget

import "context"

// CheckBudget tracks compiler-phase resource limits: structural nesting,
// type family reduction, constraint solving, and instance resolution.
type CheckBudget struct {
	ctx             context.Context
	nesting         int
	maxNesting      int
	tfSteps         int
	maxTFSteps      int
	solverSteps     int
	maxSolverSteps  int
	resolveDepth    int
	maxResolveDepth int
}

// NewCheck creates a CheckBudget with the given context.
// ctx is checked on each step call for external cancellation.
func NewCheck(ctx context.Context) *CheckBudget {
	return &CheckBudget{ctx: ctx}
}

// Context returns the associated context.
func (b *CheckBudget) Context() context.Context { return b.ctx }

// --- Nesting ---

// SetNestingLimit sets the structural nesting depth limit.
// Zero disables the check. Negative values are treated as zero (disabled).
func (b *CheckBudget) SetNestingLimit(n int) {
	if n < 0 {
		n = 0
	}
	b.maxNesting = n
}

// Nest increments structural nesting depth. Returns an error if the
// nesting limit is exceeded.
func (b *CheckBudget) Nest() error {
	b.nesting++
	if b.maxNesting > 0 && b.nesting > b.maxNesting {
		return &NestingLimitError{}
	}
	return nil
}

// Unnest decrements structural nesting depth.
func (b *CheckBudget) Unnest() {
	b.nesting--
}

// Nesting returns the current structural nesting depth.
func (b *CheckBudget) Nesting() int { return b.nesting }

// MaxNesting returns the configured nesting limit.
func (b *CheckBudget) MaxNesting() int { return b.maxNesting }

// --- Type family reduction ---

// SetTFStepLimit sets the type family reduction step limit.
// Zero disables the check. Negative values are treated as zero.
func (b *CheckBudget) SetTFStepLimit(n int) {
	if n < 0 {
		n = 0
	}
	b.maxTFSteps = n
}

// TFStep records one type family reduction step. Returns an error if the
// TF step limit is exceeded or the context has been cancelled.
func (b *CheckBudget) TFStep() error {
	if err := b.checkCtx(); err != nil {
		return err
	}
	b.tfSteps++
	if b.maxTFSteps > 0 && b.tfSteps > b.maxTFSteps {
		return &TFStepLimitError{}
	}
	return nil
}

// ResetTFSteps zeroes the TF step counter for a fresh reduction pass.
func (b *CheckBudget) ResetTFSteps() {
	b.tfSteps = 0
}

// TFSteps returns the number of TF reduction steps consumed so far.
func (b *CheckBudget) TFSteps() int { return b.tfSteps }

// MaxTFSteps returns the configured TF step limit.
func (b *CheckBudget) MaxTFSteps() int { return b.maxTFSteps }

// --- Constraint solver ---

// SetSolverStepLimit sets the constraint solver step limit.
// Zero disables the check. Negative values are treated as zero.
func (b *CheckBudget) SetSolverStepLimit(n int) {
	if n < 0 {
		n = 0
	}
	b.maxSolverSteps = n
}

// SolverStep records one constraint solver iteration. Returns an error
// if the solver step limit is exceeded or the context has been cancelled.
func (b *CheckBudget) SolverStep() error {
	if err := b.checkCtx(); err != nil {
		return err
	}
	b.solverSteps++
	if b.maxSolverSteps > 0 && b.solverSteps > b.maxSolverSteps {
		return &SolverStepLimitError{}
	}
	return nil
}

// ResetSolverSteps zeroes the solver step counter.
func (b *CheckBudget) ResetSolverSteps() {
	b.solverSteps = 0
}

// SolverSteps returns the number of solver steps consumed so far.
func (b *CheckBudget) SolverSteps() int { return b.solverSteps }

// MaxSolverSteps returns the configured solver step limit.
func (b *CheckBudget) MaxSolverSteps() int { return b.maxSolverSteps }

// --- Instance resolution ---

// SetResolveDepthLimit sets the instance resolution depth limit.
// Zero disables the check. Negative values are treated as zero.
func (b *CheckBudget) SetResolveDepthLimit(n int) {
	if n < 0 {
		n = 0
	}
	b.maxResolveDepth = n
}

// EnterResolve increments instance resolution depth. Returns an error
// if the depth limit is exceeded.
func (b *CheckBudget) EnterResolve() error {
	b.resolveDepth++
	if b.maxResolveDepth > 0 && b.resolveDepth > b.maxResolveDepth {
		return &ResolveDepthLimitError{}
	}
	return nil
}

// LeaveResolve decrements instance resolution depth.
func (b *CheckBudget) LeaveResolve() {
	b.resolveDepth--
}

// ResolveDepth returns the current instance resolution depth.
func (b *CheckBudget) ResolveDepth() int { return b.resolveDepth }

// MaxResolveDepth returns the configured resolve depth limit.
func (b *CheckBudget) MaxResolveDepth() int { return b.maxResolveDepth }

// --- Internal ---

// checkCtx returns the context error if cancelled (non-blocking).
// Uses wrapCtxErr (defined in budget.go) for user-friendly messages.
func (b *CheckBudget) checkCtx() error {
	select {
	case <-b.ctx.Done():
		return wrapCtxErr(b.ctx.Err())
	default:
		return nil
	}
}

// --- Compiler-phase error types ---

// TFStepLimitError indicates the type family reduction step limit was exceeded.
type TFStepLimitError struct{}

func (e *TFStepLimitError) Error() string { return "type family reduction step limit exceeded" }

// SolverStepLimitError indicates the constraint solver step limit was exceeded.
type SolverStepLimitError struct{}

func (e *SolverStepLimitError) Error() string { return "constraint solver step limit exceeded" }

// ResolveDepthLimitError indicates the instance resolution depth limit was exceeded.
type ResolveDepthLimitError struct{}

func (e *ResolveDepthLimitError) Error() string { return "instance resolution depth limit exceeded" }
