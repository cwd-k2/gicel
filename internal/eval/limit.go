package eval

// Limit tracks evaluation budget.
type Limit struct {
	remaining int
	maxDepth  int
	depth     int
}

// NewLimit creates a Limit with the given step and depth budgets.
func NewLimit(steps, maxDepth int) *Limit {
	return &Limit{remaining: steps, maxDepth: maxDepth}
}

// DefaultLimit returns a Limit with default budgets.
func DefaultLimit() *Limit {
	return NewLimit(1_000_000, 1_000)
}

// Step decrements the counter. Returns error at zero.
func (l *Limit) Step() error {
	l.remaining--
	if l.remaining <= 0 {
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

// StepLimitError indicates the step limit was exceeded.
type StepLimitError struct{}

func (e *StepLimitError) Error() string { return "step limit exceeded" }

// DepthLimitError indicates the call depth limit was exceeded.
type DepthLimitError struct{}

func (e *DepthLimitError) Error() string { return "call depth limit exceeded" }
