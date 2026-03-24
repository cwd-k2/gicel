package engine

// Limits holds execution and compilation resource limits.
type Limits struct {
	stepLimit  int
	depthLimit int

	nestingLimit int
	allocLimit   int64

	// Compiler limits (type checking).
	maxTFSteps      int
	maxSolverSteps  int
	maxResolveDepth int
}
