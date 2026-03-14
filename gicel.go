// Package gicel provides an embedded typed effect language for Go.
//
// The compilation pipeline follows a three-tier lifecycle:
//
//	Engine   (mutable, configurable)
//	  ↓ NewRuntime(source)
//	Runtime  (immutable, goroutine-safe)
//	  ↓ RunContext(ctx, ...)
//	result   (per-execution)
package gicel

import (
	"github.com/cwd-k2/gicel/internal/check"
	"github.com/cwd-k2/gicel/internal/eval"
	"github.com/cwd-k2/gicel/internal/stdlib"
)

// Value is a runtime value produced by evaluation.
type Value = eval.Value

// HostVal wraps an opaque Go value injected from the host.
type HostVal = eval.HostVal

// ConVal is a fully-applied constructor value.
type ConVal = eval.ConVal

// RecordVal is a record value { l1 = v1, ..., ln = vn }.
type RecordVal = eval.RecordVal

// CapEnv is a capability environment with copy-on-write semantics.
type CapEnv = eval.CapEnv

// PrimImpl is the signature for host-provided primitive operations.
type PrimImpl = eval.PrimImpl

// Applier is a callback that applies a function value to an argument.
type Applier = eval.Applier

// EvalStats holds post-evaluation statistics.
type EvalStats = eval.EvalStats

// TraceEvent describes one evaluation step.
type TraceEvent = eval.TraceEvent

// TraceHook is called before each evaluation step.
type TraceHook = eval.TraceHook

// CheckTraceKind classifies type checking trace events.
type CheckTraceKind = check.CheckTraceKind

// CheckTraceEvent describes one type checking decision.
type CheckTraceEvent = check.CheckTraceEvent

// CheckTraceHook receives trace events during type checking.
type CheckTraceHook = check.CheckTraceHook

// RuntimeError represents an error during evaluation.
// Use errors.As to match this type from RunContext errors.
type RuntimeError = eval.RuntimeError

// NewCapEnv creates a new capability environment from a map.
func NewCapEnv(caps map[string]any) CapEnv {
	return eval.NewCapEnv(caps)
}

// Stdlib re-exports — users import only the root package.

// Num provides integer arithmetic: Num class, Eq/Ord Int instances, and operators.
var Num Pack = stdlib.Num

// Str provides string and rune operations.
var Str Pack = stdlib.Str

// Fail provides the fail effect capability.
var Fail Pack = stdlib.Fail

// State provides get/put state capabilities.
var State Pack = stdlib.State

// List provides list operations: fromSlice, toSlice, length, concat, foldl, etc.
var List Pack = stdlib.List

// IO provides print/debug capabilities using CapEnv buffer.
var IO Pack = stdlib.IO
