package engine

import "github.com/cwd-k2/gicel/internal/compiler/check"

// Limits holds execution and compilation configuration.
type Limits struct {
	stepLimit      int
	depthLimit     int
	nestingLimit   int
	allocLimit     int64
	entryPoint     string
	checkTraceHook check.CheckTraceHook

	// Compiler limits (type checking).
	maxTFSteps      int
	maxSolverSteps  int
	maxResolveDepth int
}
