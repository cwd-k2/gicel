package eval

// TraceEvent describes one evaluation step.
// NodeKind is the Core IR node type (e.g. "Var", "App", "Lam", "Case").
// NodeDesc is a human-readable description of the node.
type TraceEvent struct {
	Depth    int
	NodeKind string
	NodeDesc string
	CapEnv   CapEnv
}

// TraceHook is called before each evaluation step.
// Returning a non-nil error aborts evaluation.
type TraceHook func(TraceEvent) error
