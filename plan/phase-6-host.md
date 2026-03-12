# Phase 6: Host Boundary

## Objective

Implement the Go-facing API that ties together parsing, type checking, and evaluation. This is the public interface of the library — what Go application developers interact with.

## Dependencies

Phases 1–5.

## Package: `host/`

### 6.1 Boundary Architecture

Three tiers separate configuration, compilation, and execution:

```
┌───────────────────────────────────────────────────┐
│ Engine (host/)                                     │
│   Lifecycle: application                           │
│   Mutability: mutable during setup                 │
│   Concurrency: not goroutine-safe                  │
│                                                     │
│   configure → compile → Runtime                    │
└──────────────────────┬────────────────────────────┘
                       │ NewRuntime(source)
                       ▼
┌───────────────────────────────────────────────────┐
│ Runtime (host/)                                    │
│   Lifecycle: program (cache & reuse)               │
│   Mutability: immutable after creation             │
│   Concurrency: goroutine-safe                      │
│                                                     │
│   holds compiled Core IR + frozen config           │
└──────────────────────┬────────────────────────────┘
                       │ Run(ctx, caps, bindings, entry)
                       ▼
┌───────────────────────────────────────────────────┐
│ Evaluator (eval/)                                  │
│   Lifecycle: single execution                      │
│   Mutability: mutable (step counter)               │
│   Concurrency: NOT goroutine-safe                  │
│                                                     │
│   Eval(env, capEnv, expr) → (EvalResult, error)   │
└───────────────────────────────────────────────────┘
```

**Key invariants**:

- Engine → Runtime is a **freeze** transition: all mutable setup state is snapshotted into an immutable Runtime.
- Runtime → Evaluator is a **spawn** transition: each `Run` call creates fresh mutable state (`Limit`, `Env`, `CapEnv`).
- `PrimImpl` functions are stored by reference. They may be called concurrently from multiple `Run` invocations on the same Runtime. **Host-provided PrimImpl must be goroutine-safe** if the Runtime is shared across goroutines.

### 6.2 Engine (`host/engine.go`)

Configuration and compilation. The Engine is the builder; the Runtime is the compiled artifact.

```go
package host

import (
    "context"

    "github.com/cwd-k2/gomputation/types"
    "github.com/cwd-k2/gomputation/eval"
)

// Engine is the main entry point for embedding Gomputation in a Go application.
// It holds configuration and produces Runtime instances via NewRuntime.
//
// Not goroutine-safe. Configure from a single goroutine, then call NewRuntime.
type Engine struct {
    registry     *Registry
    typeEnv      *TypeEnv
    bindings     map[string]types.Type  // declared host bindings (type info for checker)
    recEnabled   bool       // host opt-in for general recursion
    stepLimit    int        // max eval steps (default: 1_000_000)
    depthLimit   int        // max call depth (default: 1_000)
}

// New creates a new Engine with default settings.
func New() *Engine

// EnableRecursion enables the `rec` and `fix` built-in identifiers.
// Without this, programs that reference `rec`/`fix` fail with an unbound variable error.
// When enabled, programs may diverge — step limits are enforced.
func (e *Engine) EnableRecursion() *Engine

// SetStepLimit sets the maximum number of evaluation steps.
// Default: 1,000,000. Set to 0 for unlimited (not recommended).
func (e *Engine) SetStepLimit(n int) *Engine

// SetDepthLimit sets the maximum call stack depth.
// Default: 1,000.
func (e *Engine) SetDepthLimit(n int) *Engine

// Registry returns the engine's assumption registry for host registration.
func (e *Engine) Registry() *Registry

// TypeEnv returns the engine's type environment for opaque type registration.
func (e *Engine) TypeEnv() *TypeEnv

// DeclareBinding declares that a named value with the given type will be
// provided at Run time through the bindings map. The checker accepts this
// name as a valid variable with the declared type.
//
// This separates type declaration (compile time) from value provision (run time),
// enabling scenarios where the host configures the type-level contract once
// and provides concrete values per execution.
//
//   engine.DeclareBinding("query", host.ConType("Query"))
//   engine.DeclareBinding("limit", host.ConType("Int"))
//
func (e *Engine) DeclareBinding(name string, ty types.Type) *Engine

// NewRuntime compiles a Gomputation program and returns an immutable Runtime.
//
// The Runtime holds the compiled Core IR and a frozen snapshot of the Engine's
// configuration (PrimRegistry, step/depth limits, rec gating).
//
// Compilation: prelude prepend → lex → parse → check → elaborate → Core IR.
// Returns CompileError on failure.
//
// The returned Runtime is goroutine-safe and can be reused for multiple
// Run calls with different inputs.
func (e *Engine) NewRuntime(source string) (*Runtime, error)

// Run is a convenience method: NewRuntime(source) + Runtime.Run(...).
// For one-shot execution where compilation caching is not needed.
func (e *Engine) Run(source string, capEnv map[string]any, bindings map[string]any, entry string) (any, error)

// RunContext is a convenience method: NewRuntime(source) + Runtime.RunContext(...).
func (e *Engine) RunContext(ctx context.Context, source string, capEnv map[string]any, bindings map[string]any, entry string) (any, error)

// SetEvalTrace sets a trace hook for evaluation debugging.
// The hook is snapshotted into each Runtime and called during Run/RunContext.
// Set to nil to disable tracing.
func (e *Engine) SetEvalTrace(hook eval.TraceHook) *Engine

// SetCheckTrace sets a trace hook for type checking debugging.
// The hook is called during NewRuntime and Check.
// Set to nil to disable tracing.
func (e *Engine) SetCheckTrace(hook check.CheckTraceHook) *Engine

// Check parses and type-checks a program without evaluating.
// Returns structured errors if type checking fails.
// If SetCheckTrace was called, the hook receives trace events.
//
// Uses only types registered via TypeEnv and Registry — no runtime values needed.
func (e *Engine) Check(source string) error

// Parse parses a program and returns the AST.
// Useful for tooling (formatters, linters).
func (e *Engine) Parse(source string) (*syntax.AstProgram, error)
```

### 6.3 Runtime (`host/runtime.go`)

Compiled program + frozen execution configuration. Immutable and goroutine-safe after creation.

```go
// Runtime holds a compiled Gomputation program and its execution configuration.
// It is immutable after creation and safe for concurrent use from multiple goroutines.
//
// Create via Engine.NewRuntime(source). Reuse for multiple Run calls
// with different inputs (compile once, run many).
type Runtime struct {
    program          *core.Program      // compiled Core IR (immutable)
    prims            *eval.PrimRegistry // frozen (read-only after freeze)
    declaredBindings map[string]bool    // names that must be provided at Run time
    stepLimit        int
    depthLimit       int
    evalTrace        eval.TraceHook     // optional, snapshotted from Engine
}

// Program returns the compiled Core IR for inspection.
func (rt *Runtime) Program() *core.Program

// PrettyProgram returns a pretty-printed representation of the compiled Core IR.
// Useful for debugging elaboration ("what Core did the checker produce?").
func (rt *Runtime) PrettyProgram() string

// Run executes the compiled program with the given inputs.
// Equivalent to RunContext(context.Background(), ...).
func (rt *Runtime) Run(capEnv map[string]any, bindings map[string]any, entry string) (any, error)

// RunContext executes the compiled program with the given context and inputs.
//
// Each call creates a fresh Evaluator with its own Limit (step counter)
// and environments. Multiple goroutines can call RunContext concurrently
// on the same Runtime — no shared mutable state.
//
// The context is propagated to:
//   - The Evaluator (checked at each step alongside the step limit)
//   - PrimImpl calls (enables host-side timeout of blocking operations)
//
// Returns the final value and any error (RuntimeError, StepLimitError,
// DepthLimitError, context.Canceled, context.DeadlineExceeded).
func (rt *Runtime) RunContext(ctx context.Context, capEnv map[string]any, bindings map[string]any, entry string) (any, error)
```

**Design note: CapEnv return value**. The current API returns `(any, error)`, discarding the final CapEnv. For typestate verification (e.g., asserting the DB is closed after execution), the host may want to observe the final capability state. Option:

```go
// RunResult holds the complete result of an execution.
type RunResult struct {
    Value  any
    CapEnv map[string]any
    Stats  eval.EvalStats
}

// RunContextFull is like RunContext but also returns the final CapEnv and evaluation stats.
func (rt *Runtime) RunContextFull(ctx context.Context, capEnv map[string]any, bindings map[string]any, entry string) (*RunResult, error)
```

Deferred to implementation: choose the appropriate surface based on ergonomics.

### 6.4 Type Builder (`host/typebuilder.go`)

Go-side type construction API. The host registers types via Go objects, not strings.

```go
// TypeEnv holds registered opaque types and their kinds.
type TypeEnv struct {
    types map[string]types.Kind
}

func NewTypeEnv() *TypeEnv

// RegisterType registers an opaque type visible to Gomputation programs.
//
//   env.RegisterType("Int", types.KType{})
//   env.RegisterType("DB", types.KArrow{From: types.KType{}, To: types.KType{}})
//
func (te *TypeEnv) RegisterType(name string, kind types.Kind)
```

### 6.5 Registration API (`host/registry.go`)

Host-provided assumption registration.

```go
// Registry manages assumption implementations.
type Registry struct {
    prims   *eval.PrimRegistry
    typeEnv *TypeEnv
}

func NewRegistry() *Registry

// Register registers a host assumption with its type and implementation.
//
// Example:
//
//   reg.Register("dbOpen",
//       host.CompType(
//           host.Row("db", host.AppType("DB", host.ConType("Closed"))).Open("r"),
//           host.Row("db", host.AppType("DB", host.ConType("Opened"))).Open("r"),
//           host.ConType("Unit"),
//       ),
//       func(ctx context.Context, capEnv eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
//           db := capEnv.Get("db").(*sql.DB)
//           if err := db.PingContext(ctx); err != nil {
//               return nil, capEnv, err
//           }
//           newCap := capEnv.Set("db", db)
//           return &eval.ConVal{Con: "Unit"}, newCap, nil
//       },
//   )
func (r *Registry) Register(name string, ty types.Type, impl eval.PrimImpl)

// RegisterValue registers a value binding (non-computation).
//
//   reg.RegisterValue("maxRetries", host.ConType("Int"), eval.HostVal{Inner: 3})
//
func (r *Registry) RegisterValue(name string, ty types.Type, val eval.Value)
```

### 6.6 Type Construction Helpers (`host/typehelpers.go`)

Convenience functions for building types from Go code without importing `types/` directly.

```go
// ConType creates a named type constructor.
func ConType(name string) types.Type

// AppType creates a type application.
func AppType(con string, args ...types.Type) types.Type

// ArrowType creates a function type.
func ArrowType(from, to types.Type) types.Type

// CompType creates a Computation type.
func CompType(pre, post, result types.Type) types.Type

// ThunkType creates a Thunk type.
func ThunkType(pre, post, result types.Type) types.Type

// ForallType creates a universally quantified type.
func ForallType(varName string, kind types.Kind, body types.Type) types.Type

// ---- Row construction ----

// RowBuilder helps construct row types.
type RowBuilder struct {
    fields []types.RowField
}

// Row starts a row with one field.
func Row(label string, ty types.Type) *RowBuilder

// And adds a field to the row.
func (rb *RowBuilder) And(label string, ty types.Type) *RowBuilder

// Closed returns a closed row.
func (rb *RowBuilder) Closed() types.Type

// Open returns an open row with a tail variable.
func (rb *RowBuilder) Open(tailVar string) types.Type

// EmptyRow returns the empty row {}.
func EmptyRow() types.Type
```

### 6.7 Value Conversion (`host/convert.go`)

Conversion between Go values and Gomputation runtime values.

```go
// ToValue converts a Go value to a Gomputation Value.
// Supported types: int, string, bool, []any, map[string]any.
// Unknown types are wrapped in HostVal.
func ToValue(v any) eval.Value

// FromValue converts a Gomputation Value back to a Go value.
// HostVal → unwrap inner value.
// ConVal → structured representation (map with "constructor" and "fields" keys).
func FromValue(v eval.Value) any
```

### 6.8 Pipeline Implementation

The pipeline is split between `Engine.NewRuntime` (compilation) and `Runtime.RunContext` (execution).

#### Compilation (`Engine.NewRuntime`)

```go
func (e *Engine) NewRuntime(source string) (*Runtime, error) {
    // 1. Prepend prelude source.
    fullSource := prelude.Declarations() + "\n" + source

    // 2. Create source object.
    src := span.NewSource("<input>", fullSource)

    // 3. Lex.
    lexer := syntax.NewLexer(src)
    tokens, lexErrs := lexer.Tokenize()
    if lexErrs.HasErrors() {
        return nil, &CompileError{Errors: lexErrs}
    }

    // 4. Parse.
    errs := errs.New(src)
    parser := syntax.NewParser(tokens, errs)
    ast := parser.ParseProgram()
    if errs.HasErrors() {
        return nil, &CompileError{Errors: errs}
    }

    // 5. Type check + elaborate.
    coreProgram, checkErrs := check.Check(ast, src, e.checkConfig())
    if checkErrs.HasErrors() {
        return nil, &CompileError{Errors: checkErrs}
    }

    // 6. Freeze: snapshot engine config into immutable Runtime.
    declared := make(map[string]bool, len(e.bindings))
    for name := range e.bindings {
        declared[name] = true
    }
    return &Runtime{
        program:          coreProgram,
        prims:            e.registry.prims,  // read-only after this point
        declaredBindings: declared,
        stepLimit:        e.stepLimit,
        depthLimit:       e.depthLimit,
        evalTrace:        e.evalTrace,       // snapshotted (may be nil)
    }, nil
}
```

#### Execution (`Runtime.RunContext`)

```go
func (rt *Runtime) Run(capEnv map[string]any, bindings map[string]any, entry string) (any, error) {
    return rt.RunContext(context.Background(), capEnv, bindings, entry)
}

func (rt *Runtime) RunContext(ctx context.Context, capEnv map[string]any, bindings map[string]any, entry string) (any, error) {
    // 1. Validate: all declared bindings must be provided.
    for name := range rt.declaredBindings {
        if _, ok := bindings[name]; !ok {
            return nil, &RuntimeError{Message: fmt.Sprintf("missing declared binding: %s", name)}
        }
    }

    // 2. Create per-execution Evaluator (fresh Limit, shared prims, optional trace).
    ev := eval.NewEvaluator(ctx, rt.prims, eval.NewLimit(rt.stepLimit, rt.depthLimit), rt.evalTrace)

    // 3. Build initial variable environment from host bindings.
    env := eval.EmptyEnv()
    for name, val := range bindings {
        env = env.Extend(name, ToValue(val))
    }

    // 4. Build initial capability environment.
    cap := eval.NewCapEnv(capEnv)

    // 5. Look up entry point and evaluate.
    result, err := ev.Eval(env, cap, lookupEntry(rt.program, entry))
    if err != nil {
        return nil, err
    }

    // 6. Convert result. (Stats available via RunContextFull.)
    return FromValue(result.Value), nil
}
```

#### Convenience methods on Engine

```go
func (e *Engine) Run(source string, capEnv map[string]any, bindings map[string]any, entry string) (any, error) {
    return e.RunContext(context.Background(), source, capEnv, bindings, entry)
}

func (e *Engine) RunContext(ctx context.Context, source string, capEnv map[string]any, bindings map[string]any, entry string) (any, error) {
    rt, err := e.NewRuntime(source)
    if err != nil {
        return nil, err
    }
    return rt.RunContext(ctx, capEnv, bindings, entry)
}
```

### 6.9 Error API (`host/error.go`)

Errors are separated by tier:

- **CompileError**: returned by `Engine.NewRuntime`, `Engine.Check`. Contains structured diagnostics with source locations.
- **RuntimeError**: returned by `Runtime.Run` / `Runtime.RunContext`. Wraps evaluation errors.
- **StepLimitError** / **DepthLimitError**: specific runtime errors from resource limits.
- **context.Canceled** / **context.DeadlineExceeded**: standard Go errors from context cancellation.

```go
// CompileError represents a compilation error (lex, parse, or check).
type CompileError struct {
    Errors *errs.Errors
}

func (e *CompileError) Error() string
func (e *CompileError) Diagnostics() []Diagnostic

// Diagnostic is a single error suitable for display.
type Diagnostic struct {
    Line    int
    Column  int
    Message string
    Hints   []string
}

// RuntimeError wraps an evaluation error.
type RuntimeError struct {
    Inner error
}
```

## Test Strategy

### Unit tests

- **TypeEnv**: register types, verify kinds.
- **Registry**: register assumptions, verify lookup.
- **Type helpers**: build complex types, verify structure.
- **Value conversion**: round-trip Go → Value → Go.
- **Row builder**: build rows, verify structure.

### Integration tests (full pipeline)

1. **Minimal program**: `data Unit = Unit; main :: Unit; main := Unit` → runs, returns Unit.
2. **Identity**: `id :: forall a. a -> a; id := \x -> x; main := id Unit` → Unit.
3. **Host values**: inject integer from Go, pass through identity, retrieve.
4. **Assumption**: register dbOpen, dbClose, run open→close program, verify CapEnv transitions.
5. **do block**: multi-step computation with assumption calls.
6. **Type error**: ill-typed program produces CompileError with diagnostics.
7. **Missing assumption**: program references unregistered assumption → runtime error.
8. **Block expression**: local bindings work end-to-end.
9. **Type alias**: `Effect` alias works in annotations.
10. **Thunk/force**: suspended computation executes on force.

### DeclareBinding tests

13. **Declared binding used in source**: `DeclareBinding("x", Int)`, source uses `x` → type checks, runs with binding provided.
14. **Declared binding type mismatch**: `DeclareBinding("x", Int)`, source uses `x` where `String` is expected → type error at check time.
15. **Missing declared binding at run time**: `DeclareBinding("x", Int)`, `Run` called without `"x"` in bindings → RuntimeError.
16. **Undeclared variable in source**: source uses `y` without `DeclareBinding("y", ...)` → unbound variable error at check time.

### Runtime reuse tests

11. **Compile once, run twice**: same Runtime, different bindings → different results.
12. **Concurrent runs**: same Runtime, two goroutines → both succeed, no data races.

### Example-based tests

Reproduce spec §15 example program as a test case.

## Completion Criteria

- [ ] Engine.NewRuntime compiles source to immutable Runtime
- [ ] Runtime.Run / RunContext executes compiled program
- [ ] Runtime is goroutine-safe (verified by concurrent test)
- [ ] Engine.Run / RunContext convenience methods work
- [ ] Engine.Check validates without evaluating
- [ ] Type registration API works
- [ ] Assumption registration API works
- [ ] DeclareBinding registers type info for checker
- [ ] Declared bindings are type-checked (source using wrong type → error)
- [ ] Missing declared binding at Run time → RuntimeError
- [ ] Value binding injection works
- [ ] Value conversion round-trips correctly
- [ ] Type construction helpers build correct types
- [ ] Error API provides structured diagnostics (CompileError vs RuntimeError)
- [ ] Runtime.Program() / PrettyProgram() expose compiled IR
- [ ] RunContextFull returns final CapEnv and EvalStats
- [ ] SetEvalTrace / SetCheckTrace propagate hooks correctly
- [ ] Spec §15 example program runs correctly
- [ ] All tests pass
