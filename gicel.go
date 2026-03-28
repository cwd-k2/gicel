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
	"github.com/cwd-k2/gicel/internal/app/engine"
	"github.com/cwd-k2/gicel/internal/host/registry"
	"github.com/cwd-k2/gicel/internal/host/stdlib"
	"github.com/cwd-k2/gicel/internal/infra/budget"
	"github.com/cwd-k2/gicel/internal/lang/ir"
	"github.com/cwd-k2/gicel/internal/lang/types"
	"github.com/cwd-k2/gicel/internal/runtime/eval"
)

// ---- Engine / Runtime / Compile ----

// Engine configures and compiles GICEL programs.
type Engine = engine.Engine

// Runtime is an immutable, compiled GICEL program.
type Runtime = engine.Runtime

// RunResult holds the result of an execution.
type RunResult = engine.RunResult

// RunOptions configures a single execution.
type RunOptions = engine.RunOptions

// ExplainDepth controls how deeply the explain trace instruments evaluation.
type ExplainDepth = engine.ExplainDepth

// DefaultEntryPoint is the default entry point binding name.
const DefaultEntryPoint = engine.DefaultEntryPoint

// ExplainDepth constants.
const (
	ExplainUser = engine.ExplainUser
	ExplainAll  = engine.ExplainAll
)

// SandboxConfig configures a sandboxed execution.
type SandboxConfig = engine.SandboxConfig

// CompileError wraps compilation errors.
type CompileError = engine.CompileError

// CompileResult holds all static information produced by compilation.
type CompileResult = engine.CompileResult

// Diagnostic is a single structured error from compilation.
type Diagnostic = engine.Diagnostic

// DiagnosticHint is a secondary annotation on a compilation diagnostic.
type DiagnosticHint = engine.DiagnosticHint

// InternalPanicError wraps a recovered panic from RunSandbox.
type InternalPanicError = engine.InternalPanicError

// RowBuilder constructs row types incrementally.
type RowBuilder = engine.RowBuilder

// CoreProgram is an opaque compiled Core IR for inspection.
type CoreProgram = engine.CoreProgram

// NewEngine creates a new Engine with default limits.
var NewEngine = engine.NewEngine

// RunSandbox compiles and executes a GICEL program in a single call.
var RunSandbox = engine.RunSandbox

// ---- Registration ----

// Registrar is the interface for registering primitives and modules.
type Registrar = registry.Registrar

// Pack configures a Registrar with a coherent set of types, primitives, and modules.
type Pack = registry.Pack

// ---- Runtime values ----

// Value is a runtime value produced by evaluation.
type Value = eval.Value

// HostVal wraps an opaque Go value injected from the host.
type HostVal = eval.HostVal

// ConVal is a fully-applied constructor value.
type ConVal = eval.ConVal

// RecordVal is a record value { l1: v1, ..., ln: vn }.
type RecordVal = eval.RecordVal

// RecordField is a single field in a record value.
type RecordField = eval.RecordField

// NewRecord creates a RecordVal from fields (sorts by label).
var NewRecord = eval.NewRecord

// NewRecordFromMap creates a RecordVal from a map (sorts labels).
var NewRecordFromMap = eval.NewRecordFromMap

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

// ExplainDetail carries kind-specific structured data within an ExplainStep.
type ExplainDetail = eval.ExplainDetail

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

// RuntimeError represents an error during evaluation.
type RuntimeError = eval.RuntimeError

// ---- Resource limit errors ----

// StepLimitError indicates the evaluation step limit was exceeded.
type StepLimitError = budget.StepLimitError

// DepthLimitError indicates the call depth limit was exceeded.
type DepthLimitError = budget.DepthLimitError

// AllocLimitError indicates the allocation byte limit was exceeded.
type AllocLimitError = budget.AllocLimitError

// NestingLimitError indicates the structural nesting depth limit was exceeded.
type NestingLimitError = budget.NestingLimitError

// TimeoutError indicates the execution timed out via context deadline.
type TimeoutError = budget.TimeoutError

// CancelledError indicates the execution was cancelled externally.
type CancelledError = budget.CancelledError

// ---- Type construction helpers ----

// RowField is a single label:type pair in a row.
// Used with RecordType to construct closed record types.
type RowField = types.RowField

var (
	ConType      = engine.ConType
	ArrowType    = engine.ArrowType
	CompType     = engine.CompType
	ForallType   = engine.ForallType
	ForallRow    = engine.ForallRow
	VarType      = engine.VarType
	AppType      = engine.AppType
	NewRow       = engine.NewRow
	KindType     = engine.KindType
	KindArrow    = engine.KindArrow
	KindRow      = engine.KindRow
	ForallKind   = engine.ForallKind
	EmptyRowType = engine.EmptyRowType
	RecordType   = engine.RecordType
	TupleType    = engine.TupleType
	TypePretty   = engine.TypePretty
)

// ---- Value conversion helpers ----

var (
	ToValue    = engine.ToValue
	FromBool   = engine.FromBool
	FromHost   = engine.FromHost
	FromCon    = engine.FromCon
	ToList     = engine.ToList
	FromList   = engine.FromList
	FromRecord = engine.FromRecord
)

// MustHost extracts the inner Go value from a HostVal, panicking if it is not one.
func MustHost[T any](v Value) T { return engine.MustHost[T](v) }

// ---- Utility functions ----

// PrettyValue formats a runtime value in source-level terms.
func PrettyValue(v Value) string { return eval.PrettyValue(v) }

// CollectList extracts a Cons/Nil chain into a slice of element values.
// Returns (nil, false) if v is not a well-formed list.
func CollectList(v *ConVal) ([]Value, bool) { return eval.CollectList(v) }

// IsTuple reports whether a RecordVal encodes a tuple (fields _1, _2, ..., _n).
func IsTuple(r *RecordVal) bool { return eval.IsTuple(r) }

// IsBool checks if a ConVal is a Prelude Bool (True or False, nullary).
func IsBool(v *ConVal) (val bool, ok bool) { return eval.IsBool(v) }

// TupleLabel returns the canonical field label for a 1-based tuple position.
func TupleLabel(pos int) string { return ir.TupleLabel(pos) }

// ValidateModuleName checks that name is a valid module identifier.
func ValidateModuleName(name string) error { return engine.ValidateModuleName(name) }

// ---- Stdlib re-exports ----

// Prelude provides the combined standard prelude.
var Prelude Pack = stdlib.Prelude

// EffectFail provides the fail effect capability.
var EffectFail Pack = stdlib.Fail

// EffectState provides get/put state capabilities.
var EffectState Pack = stdlib.State

// EffectIO provides print/debug capabilities using CapEnv buffer.
var EffectIO Pack = stdlib.IO

// DataStream provides lazy list operations.
var DataStream Pack = stdlib.Stream

// DataSlice provides immutable indexed snapshots.
var DataSlice Pack = stdlib.Slice

// EffectArray provides mutable fixed-size arrays gated by the { array: () } effect.
var EffectArray Pack = stdlib.Array

// DataMap provides immutable ordered map backed by AVL tree.
var DataMap Pack = stdlib.Map

// DataSet provides immutable ordered set backed by Map k ().
var DataSet Pack = stdlib.Set

// EffectMap provides mutable ordered maps gated by the { mmap: () } effect.
var EffectMap Pack = stdlib.EffectMap

// EffectSet provides mutable ordered sets gated by the { mset: () } effect.
var EffectSet Pack = stdlib.EffectSet

// EffectRef provides mutable reference cells gated by the { ref: () } effect.
var EffectRef Pack = stdlib.Ref

// DataJSON provides ToJSON/FromJSON type classes for JSON encoding/decoding.
var DataJSON Pack = stdlib.JSON
