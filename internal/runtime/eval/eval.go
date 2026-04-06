// Package eval defines runtime value types, capability environments, and
// primitive registries for GICEL evaluation. Execution is performed by the
// bytecode VM (internal/runtime/vm); this package provides the shared types.
package eval

// Allocation cost estimates (bytes per value type).
// Used by the bytecode VM for budget.Alloc charges.
const (
	CostClosure     = 48               // VMClosure struct (incl. Source pointer)
	CostConBase     = 32               // ConVal struct
	CostConArg      = 16               // per arg in []Value
	CostThunk       = 32               // VMThunkVal struct (incl. Source pointer)
	CostRecord      = 32               // RecordVal struct + slice header
	CostRecordField = 24               // per RecordField (label string + Value interface)
	CostFix         = CostClosure + 40 // VMClosure + Env node for fix binding
	CostPAP         = 32               // PAPVal struct
	CostPAPArg      = 16               // per arg in PAPVal.Args
)

// EvalResult is the result of evaluation.
type EvalResult struct {
	Value  Value
	CapEnv CapEnv
}

// EvalStats holds post-evaluation statistics.
type EvalStats struct {
	Steps     int   `json:"steps"`
	MaxDepth  int   `json:"maxDepth"`
	Allocated int64 `json:"allocated"`
}
