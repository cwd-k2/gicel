package eval

import "fmt"

// Limit tracks evaluation budget: steps, call depth, and heap allocation.
type Limit struct {
	remaining  int
	maxDepth   int
	depth      int
	allocated  int64
	allocLimit int64
}

// NewLimit creates a Limit with the given step and depth budgets.
func NewLimit(steps, maxDepth int) *Limit {
	return &Limit{remaining: steps, maxDepth: maxDepth}
}

// DefaultLimit returns a Limit with default budgets.
func DefaultLimit() *Limit {
	l := NewLimit(1_000_000, 1_000)
	l.allocLimit = 10 * 1024 * 1024 // 10 MiB
	return l
}

// SetAllocLimit sets the allocation byte limit. Zero disables the check.
func (l *Limit) SetAllocLimit(bytes int64) {
	l.allocLimit = bytes
}

// Step decrements the counter. Returns error at zero.
func (l *Limit) Step() error {
	l.remaining--
	if l.remaining < 0 {
		return &StepLimitError{}
	}
	return nil
}

// Enter increments call depth. Returns error if max exceeded.
func (l *Limit) Enter() error {
	l.depth++
	if l.depth > l.maxDepth {
		return &DepthLimitError{}
	}
	return nil
}

// Leave decrements call depth.
func (l *Limit) Leave() {
	l.depth--
}

// Depth returns the current call depth.
func (l *Limit) Depth() int {
	return l.depth
}

// Alloc records a heap allocation of the given size.
// Returns error if the cumulative allocation exceeds the limit.
func (l *Limit) Alloc(bytes int64) error {
	l.allocated += bytes
	if l.allocLimit > 0 && l.allocated > l.allocLimit {
		return &AllocLimitError{Used: l.allocated, Limit: l.allocLimit}
	}
	return nil
}

// Allocated returns the cumulative allocation in bytes.
func (l *Limit) Allocated() int64 {
	return l.allocated
}

// StepLimitError indicates the step limit was exceeded.
type StepLimitError struct{}

func (e *StepLimitError) Error() string { return "step limit exceeded" }

// DepthLimitError indicates the call depth limit was exceeded.
type DepthLimitError struct{}

func (e *DepthLimitError) Error() string { return "call depth limit exceeded" }

// AllocLimitError indicates the allocation limit was exceeded.
type AllocLimitError struct {
	Used  int64
	Limit int64
}

func (e *AllocLimitError) Error() string {
	return fmt.Sprintf("allocation limit exceeded: %d bytes used, %d bytes allowed", e.Used, e.Limit)
}
