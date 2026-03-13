// Package gomputation provides an embedded typed effect language for Go.
//
// The compilation pipeline follows a three-tier lifecycle:
//
//	Engine   (mutable, configurable)
//	  ↓ NewRuntime(source)
//	Runtime  (immutable, goroutine-safe)
//	  ↓ RunContext(ctx, ...)
//	result   (per-execution)
package gomputation

import (
	"github.com/cwd-k2/gomputation/internal/check"
	"github.com/cwd-k2/gomputation/internal/eval"
	"github.com/cwd-k2/gomputation/internal/stdlib"
)

// Value is a runtime value produced by evaluation.
type Value = eval.Value

// HostVal wraps an opaque Go value injected from the host.
type HostVal = eval.HostVal

// ConVal is a fully-applied constructor value.
type ConVal = eval.ConVal

// CapEnv is a capability environment with copy-on-write semantics.
type CapEnv = eval.CapEnv

// PrimImpl is the signature for host-provided primitive operations.
type PrimImpl = eval.PrimImpl

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

// Stdlib re-exports — users import only the root package.

// Num provides integer arithmetic: Num class, Eq/Ord Int instances, and operators.
var Num Pack = func(e *Engine) error { return stdlib.Num(e) }

// Str provides string and rune operations.
var Str Pack = func(e *Engine) error { return stdlib.Str(e) }

// Fail provides the fail effect capability.
var Fail Pack = func(e *Engine) error { return stdlib.Fail(e) }

// State provides get/put state capabilities.
var State Pack = func(e *Engine) error { return stdlib.State(e) }
