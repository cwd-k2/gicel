package eval

import (
	"fmt"
	"strings"

	"github.com/cwd-k2/gicel/internal/core"
)

// TraceEvent describes one evaluation step.
// NodeKind is the Core IR node type (e.g. "Var", "App", "Lam", "Case").
// NodeDesc is a human-readable description of the node.
type TraceEvent struct {
	Depth    int
	NodeKind string
	NodeDesc string
	CapEnv   CapEnv
}

// newTraceEvent constructs a TraceEvent from a Core IR node.
func newTraceEvent(depth int, node core.Core, capEnv CapEnv) TraceEvent {
	kind := fmt.Sprintf("%T", node)
	if i := strings.LastIndex(kind, "."); i >= 0 {
		kind = kind[i+1:]
	}
	return TraceEvent{
		Depth:    depth,
		NodeKind: kind,
		NodeDesc: core.Pretty(node),
		CapEnv:   capEnv,
	}
}

// TraceHook is called before each evaluation step.
// Returning a non-nil error aborts evaluation.
type TraceHook func(TraceEvent) error

// EvalStats holds post-evaluation statistics.
type EvalStats struct {
	Steps     int
	MaxDepth  int
	Allocated int64
}
