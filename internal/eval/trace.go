package eval

import "github.com/cwd-k2/gomputation/internal/core"

// TraceEvent describes one evaluation step.
type TraceEvent struct {
	Depth  int
	Node   core.Core
	Env    *Env
	CapEnv CapEnv
}

// TraceHook is called before each evaluation step.
// Returning a non-nil error aborts evaluation.
type TraceHook func(TraceEvent) error

// EvalStats holds post-evaluation statistics.
type EvalStats struct {
	Steps    int
	MaxDepth int
}
