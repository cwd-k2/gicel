package engine

import "github.com/cwd-k2/gicel/internal/check"

// Limits holds execution and compilation configuration.
type Limits struct {
	stepLimit      int
	depthLimit     int
	nestingLimit   int
	allocLimit     int64
	entryPoint     string
	checkTraceHook check.CheckTraceHook
}
