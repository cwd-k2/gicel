// Package gicel provides an embedded typed effect language for Go.
//
// The compilation pipeline follows a three-tier lifecycle:
//
//	Engine   (mutable, configurable)
//	  ↓ NewRuntime(source)
//	Runtime  (immutable, goroutine-safe)
//	  ↓ RunWith(ctx, opts)
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

// RecordVal is a record value { l1: v1, ..., ln: vn }.
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

// ExplainStep is a single semantic event during evaluation.
type ExplainStep = eval.ExplainStep

// ExplainHook receives semantic evaluation events.
type ExplainHook = eval.ExplainHook

// ExplainKind classifies semantic evaluation events.
type ExplainKind = eval.ExplainKind

// ExplainKind constants.
const (
	ExplainBind   = eval.ExplainBind
	ExplainMatch  = eval.ExplainMatch
	ExplainEffect = eval.ExplainEffect
	ExplainLabel  = eval.ExplainLabel
	ExplainResult = eval.ExplainResult
)

// ExplainDetail carries kind-specific structured data for explain events.
type ExplainDetail = eval.ExplainDetail

// PrettyValue formats a runtime value in source-level terms.
func PrettyValue(v Value) string { return eval.PrettyValue(v) }

// ExplainDepth controls how deeply the explain trace instruments evaluation.
type ExplainDepth int

const (
	// ExplainUser traces user code only; stdlib internals are suppressed.
	ExplainUser ExplainDepth = iota
	// ExplainAll traces all code including stdlib internals.
	ExplainAll
)

// RunOptions configures a single execution. Per-execution concerns
// (explain, trace, entry point) live here, not on the Runtime.
type RunOptions struct {
	// Entry is the top-level binding to evaluate (default: "main").
	Entry string
	// Caps provides initial capability values.
	Caps map[string]any
	// Bindings provides host-injected value bindings.
	Bindings map[string]Value
	// Explain receives semantic evaluation events. Nil disables explain.
	Explain ExplainHook
	// ExplainDepth controls stdlib suppression (default: ExplainUser).
	ExplainDepth ExplainDepth
	// Trace receives low-level evaluation step events. Nil disables trace.
	Trace TraceHook
}

// CheckTraceKind classifies type checking trace events.
type CheckTraceKind = check.CheckTraceKind

// CheckTraceKind constants for filtering trace events.
const (
	TraceUnify       = check.TraceUnify
	TraceSolveMeta   = check.TraceSolveMeta
	TraceInfer       = check.TraceInfer
	TraceCheck       = check.TraceCheck
	TraceInstantiate = check.TraceInstantiate
	TraceRowUnify    = check.TraceRowUnify
)

// CheckTraceEvent describes one type checking decision.
type CheckTraceEvent = check.CheckTraceEvent

// CheckTraceHook receives trace events during type checking.
type CheckTraceHook = check.CheckTraceHook

// RuntimeError represents an error during evaluation.
// Use errors.As to match this type from RunWith errors.
type RuntimeError = eval.RuntimeError

// StepLimitError indicates the evaluation step limit was exceeded.
// Use errors.As to distinguish step-limit exhaustion from other errors.
type StepLimitError = eval.StepLimitError

// DepthLimitError indicates the call depth limit was exceeded.
type DepthLimitError = eval.DepthLimitError

// AllocLimitError indicates the allocation byte limit was exceeded.
// Used and Limit fields report the actual and allowed byte counts.
type AllocLimitError = eval.AllocLimitError

// NewCapEnv creates a new capability environment from a map.
// The map is not copied; the caller must not modify it after this call.
func NewCapEnv(caps map[string]any) CapEnv {
	return eval.NewCapEnv(caps)
}

// Stdlib re-exports — users import only the root package.

// Prelude provides the combined standard prelude: core computation types,
// data types, type classes, arithmetic, list operations, and string operations.
var Prelude Pack = stdlib.Prelude

// EffectFail provides the fail effect capability.
var EffectFail Pack = stdlib.Fail

// EffectState provides get/put state capabilities.
var EffectState Pack = stdlib.State

// EffectIO provides print/debug capabilities using CapEnv buffer.
var EffectIO Pack = stdlib.IO

// DataStream provides lazy list operations: LCons/LNil, headS, tailS, takeS, dropS.
var DataStream Pack = stdlib.Stream

// DataSlice provides contiguous array operations: O(1) length/index, Functor/Foldable.
var DataSlice Pack = stdlib.Slice

// DataMap provides immutable ordered map backed by AVL tree, keyed by Ord.
var DataMap Pack = stdlib.Map

// DataSet provides immutable ordered set backed by Map k ().
var DataSet Pack = stdlib.Set
