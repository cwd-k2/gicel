## 11. Go Integration

### Sandbox API

The simplest way to run GICEL from Go:

```go
import "github.com/cwd-k2/gicel"

result, err := gicel.RunSandbox(source, &gicel.SandboxConfig{
    Packs:    []gicel.Pack{gicel.Prelude, gicel.EffectFail, gicel.EffectState, gicel.EffectIO, gicel.DataStream, gicel.DataSlice, gicel.EffectArray, gicel.DataMap, gicel.DataSet, gicel.EffectMap, gicel.EffectSet},
    Entry:    "main",              // default: "main"
    Timeout:  5 * time.Second,     // default: 5s
    MaxSteps: 100_000,             // default: 100,000
    MaxDepth: 100,                 // default: 100
    MaxNesting: 256,               // default: 256
    MaxAlloc: 10 * 1024 * 1024,    // default: 10 MiB
    Caps:     map[string]any{"state": gicel.ToValue(0), "io": gicel.ToValue(nil)},
    Bindings: map[string]gicel.Value{"input": gicel.ToValue("hello")},
})
```

`SandboxConfig` fields are all optional. `nil` uses conservative defaults.
RunSandbox applies timeout to the entire pipeline (pack application, compilation, evaluation). For the Engine API, the context passed to NewRuntime bounds main compilation and evaluation; module compilation uses the Engine's compile context (set via SetCompileContext).

`RunResult`: `Value` (result), `CapEnv` (final capabilities), `Stats` (EvalStats: `Steps int`, `MaxDepth int`, `Allocated int64`).

### Full Lifecycle API

```go
eng := gicel.NewEngine()
_ = eng.Use(gicel.Prelude)
_ = eng.Use(gicel.EffectState)
eng.SetStepLimit(500_000)

rt, err := eng.NewRuntime(ctx, source)
result, err := rt.RunWith(ctx, &gicel.RunOptions{Caps: caps, Bindings: bindings})
// result.Value, result.CapEnv, result.Stats
```

### Available Packs

| Pack                | Module         | Provides                                         |
| ------------------- | -------------- | ------------------------------------------------ |
| `gicel.Prelude`     | `Prelude`      | Int, String, List — arithmetic, strings, lists   |
| `gicel.EffectFail`  | `Effect.Fail`  | Failure effect                                   |
| `gicel.EffectState` | `Effect.State` | Get/put state + runState/evalState/execState     |
| `gicel.EffectIO`    | `Effect.IO`    | Log/dbg output                                   |
| `gicel.DataStream`  | `Data.Stream`  | Lazy lists                                       |
| `gicel.DataSlice`   | `Data.Slice`   | O(1) contiguous arrays                           |
| `gicel.EffectArray` | `Effect.Array` | Mutable fixed-size arrays ({ array: () } effect) |
| `gicel.DataMap`     | `Data.Map`     | Ordered immutable map (AVL)                      |
| `gicel.DataSet`     | `Data.Set`     | Ordered immutable set                            |
| `gicel.EffectMap`   | `Effect.Map`   | Mutable ordered maps ({ mmap: () } effect)       |
| `gicel.EffectSet`   | `Effect.Set`   | Mutable ordered sets ({ mset: () } effect)       |
| `gicel.EffectRef`   | `Effect.Ref`   | Mutable reference cells ({ ref: () } effect)     |
| `gicel.DataJSON`    | `Data.JSON`    | ToJSON/FromJSON type classes                     |

### Custom Primitives (RegisterPrim)

> **Trust boundary**: Custom primitives are part of the trusted computing
> base. They run synchronously within the evaluator and are not subject
> to forced cancellation. The evaluator checks `context.Context` only at
> eval step boundaries — a primitive that blocks or ignores `ctx` cannot
> be interrupted by timeout. Only register primitives from trusted code.

```go
eng.RegisterPrim("greet", func(
    ctx context.Context, capEnv gicel.CapEnv,
    args []gicel.Value, apply gicel.Applier,
) (gicel.Value, gicel.CapEnv, error) {
    s := gicel.MustHost[string](args[0])
    fmt.Println("Hello,", s)
    return gicel.ToValue(nil), capEnv, nil
})
```

**PrimImpl**: `func(ctx, capEnv, args, apply) -> (Value, CapEnv, error)`. `apply` is for calling GICEL closures in higher-order primitives. Must return the (possibly updated) CapEnv.

### Host Bindings

```go
eng.DeclareBinding("myInput", gicel.ConType("Int"))
// At runtime: bindings := map[string]gicel.Value{"myInput": gicel.ToValue(42)}
```

### Value Conversion Helpers

| Function                            | Description                                             |
| ----------------------------------- | ------------------------------------------------------- |
| `gicel.ToValue(v any) Value`        | Wrap Go value: nil->(), bool->True/False, else->HostVal |
| `gicel.FromBool(v) (bool, bool)`    | Extract Bool constructor                                |
| `gicel.FromHost(v) (any, bool)`     | Extract inner value from HostVal                        |
| `gicel.FromCon(v) (name, args, ok)` | Extract constructor name and arguments                  |
| `gicel.MustHost[T](v) T`            | Extract typed HostVal, panics on mismatch               |
| `gicel.ToList(items []any) Value`   | Build a Cons/Nil chain from Go slice                    |
| `gicel.FromList(v) ([]any, bool)`   | Destructure Cons/Nil chain to Go slice                  |
| `gicel.FromRecord(v) (map, bool)`   | Extract record fields as `map[string]Value` (via AsMap) |

### Type Correspondence

| GICEL Type   | Go Native            | Wrapper     | Extract                          |
| ------------ | -------------------- | ----------- | -------------------------------- |
| `Int`        | `int64`              | `HostVal`   | `MustHost[int64]`, `FromHost`    |
| `Double`     | `float64`            | `HostVal`   | `MustHost[float64]`, `FromHost`  |
| `String`     | `string`             | `HostVal`   | `MustHost[string]`, `FromHost`   |
| `Rune`       | `rune`               | `HostVal`   | `MustHost[rune]`, `FromHost`     |
| `Bool`       | `True` / `False`     | `ConVal`    | `FromBool`                       |
| `()`         | `nil` unit           | `RecordVal` | `FromRecord` (empty map)         |
| `(a, b)`     | pair                 | `RecordVal` | `FromRecord` (fields `_1`, `_2`) |
| `List a`     | `Cons`/`Nil` chain   | `ConVal`    | `FromList`, `ToList`             |
| `Maybe a`    | `Just a` / `Nothing` | `ConVal`    | `FromCon`                        |
| `{l: T ...}` | record               | `RecordVal` | `FromRecord`                     |
| `Con a b`    | ADT constructor      | `ConVal`    | `FromCon`                        |

### Type Construction Helpers

| Function                                  | Description                           |
| ----------------------------------------- | ------------------------------------- |
| `gicel.ConType(name)`                     | Type constructor: `"Int"`             |
| `gicel.ArrowType(from, to)`               | Function type: `from -> to`           |
| `gicel.AppType(f, arg)`                   | Type application: `f a`               |
| `gicel.CompType(pre, post, result)`       | `Computation pre post result`         |
| `gicel.VarType(name)`                     | Type variable reference               |
| `gicel.ForallType(var, body)`             | `\var. body`                          |
| `gicel.ForallRow(var, body)`              | `\(var: Row). body`                   |
| `gicel.ForallKind(name, kind, body)`      | `\(name: kind). body` (explicit kind) |
| `gicel.EmptyRowType()`                    | Empty row `{}`                        |
| `gicel.RecordType(fields ...RowField)`    | Closed record type                    |
| `gicel.TupleType(elems ...Type)`          | Tuple record type                     |
| `gicel.KindType()`, `KindArrow(from, to)` | Kind constructors                     |
| `gicel.KindRow()`                         | Row kind                              |
| `gicel.TypePretty(t)`                     | Human-readable type string            |

**Preferred usage:**

- `RecordType(fields...)` for closed record types (most common)
- `NewRow().And(...).Closed()` / `.Open("r")` for incremental or open-row construction
- `EmptyRowType()` is a lower-level row helper — prefer `RecordType` or `NewRow()` builder when constructing record types

### Engine Configuration

| Method                                     | Description                       |
| ------------------------------------------ | --------------------------------- |
| `eng.Use(pack) error`                      | Apply a stdlib pack               |
| `eng.RegisterPrim(name, impl)`             | Register a primitive              |
| `eng.RegisterType(name, kind)`             | Register an opaque host type      |
| `eng.DeclareBinding(name, ty)`             | Declare a host-provided variable  |
| `eng.EnableRecursion()`                    | Enable `rec` and `fix` built-ins  |
| `eng.SetStepLimit(n)` / `SetDepthLimit(n)` | Resource limits                   |
| `eng.SetNestingLimit(n)`                   | Structural nesting depth limit    |
| `eng.SetAllocLimit(bytes)`                 | Allocation limit (0 = disabled)   |
| `eng.SetEntryPoint(name)`                  | Entry point name (default: main)  |
| `eng.SetCompileContext(ctx)`               | Context for module compilation    |
| `eng.RegisterModule(name, src)`            | Register a custom module          |
| `eng.RegisterModuleFile(path)`             | Register module from .gicel file  |
| `eng.RegisterModuleRec(name, src)`         | Register module with fix/rec      |
| `eng.DenyAssumptions()`                    | Block user `assumption` decls     |
| `eng.DisableInlining()`                    | Disable optimizer inlining pass   |
| `eng.NewRuntime(ctx, source)`              | Compile to Runtime                |
| `eng.Compile(ctx, source)`                 | Type-check; returns CompileResult |
| `eng.Parse(source)`                        | Parse-only (syntax errors)        |

### Error Handling

```go
// Compile errors
var ce *gicel.CompileError
if errors.As(err, &ce) {
    for _, d := range ce.Diagnostics() {
        fmt.Printf("[%s] %d:%d: %s\n", d.Phase, d.Line, d.Col, d.Message)
    }
}

// Runtime errors (with source location)
var re *gicel.RuntimeError
if errors.As(err, &re) {
    fmt.Printf("%d:%d: %s\n", re.Line, re.Col, re.Message)
}

// Limit errors
var stepErr *gicel.StepLimitError
var depthErr *gicel.DepthLimitError
var allocErr *gicel.AllocLimitError
var nestErr *gicel.NestingLimitError
var timeoutErr *gicel.TimeoutError
var cancelErr *gicel.CancelledError
if errors.As(err, &stepErr) { /* step limit exceeded */ }
if errors.As(err, &depthErr) { /* depth limit exceeded */ }
if errors.As(err, &allocErr) { /* allocErr.Used, allocErr.Limit */ }
if errors.As(err, &nestErr) { /* structural nesting limit exceeded */ }
if errors.As(err, &timeoutErr) { /* execution timed out */ }
if errors.As(err, &cancelErr) { /* context cancelled */ }
```

`Diagnostic`: `Code int`, `Phase string` ("lex"/"parse"/"check"), `Line int`, `Col int`, `Message string`, `Hints []DiagnosticHint` (secondary annotations, may be nil). `DiagnosticHint`: `Line int`, `Col int`, `Message string`.

`RuntimeError`: `Message string`, `Line int`, `Col int` (1-based, populated by Runtime). Covers: unbound variable, non-exhaustive match, division by zero, `fail`/`failWith`. Limit/timeout errors return distinct error types: `StepLimitError`, `DepthLimitError`, `AllocLimitError`, `NestingLimitError`, `TimeoutError`, `CancelledError` (match with `errors.As`).

### Hooks (per-execution via RunOptions)

**TraceHook** fires on every eval step. Signature: `func(TraceEvent) error`. Fields: `Depth int`, `NodeKind string` (e.g. "Var", "App", "Lam"), `NodeDesc string` (human-readable), `CapEnv`. Return non-nil error to abort.

**ExplainHook** fires at semantic boundaries. Signature: `func(ExplainStep)`. Fields: `Seq int`, `Depth int`, `Kind ExplainKind`, `SourceName string`, `Line int`, `Col int`, `Detail ExplainDetail`. Kinds: `ExplainBind`, `ExplainMatch`, `ExplainEffect`, `ExplainLabel`, `ExplainResult`.

```go
rt.RunWith(ctx, &gicel.RunOptions{
    Explain: func(s gicel.ExplainStep) { /* ... */ },
    Trace:   func(e gicel.TraceEvent) error { return nil },
})
```

### Migration Path: RunSandbox → Engine/Runtime

`RunSandbox` is ideal for one-shot evaluation with default limits. Move to the `Engine`/`Runtime` API when you need:

- **Compile-once, run-many**: `NewRuntime` returns an immutable `Runtime` that can be executed concurrently with `RunWith`.
- **Custom observability**: `RunOptions.Explain` and `RunOptions.Trace` provide semantic and low-level hooks.
- **Fine-grained configuration**: register custom types, primitives, and rewrite rules via the `Engine` API.

### Observability Hooks

| Hook                 | Scope      | Use case                                          |
| -------------------- | ---------- | ------------------------------------------------- |
| `RunOptions.Explain` | Evaluation | Semantic trace: binds, effects, matches, sections |
| `RunOptions.Trace`   | Evaluation | Low-level step-by-step evaluation events          |

### Trust Boundary

GICEL programs run in a restricted sandbox. The **trusted computing base (TCB)** includes:

- Go host code (your application)
- Registered primitives (`RegisterPrim`) -- these execute as Go functions with full access
- Pack implementations (stdlib)

The sandbox guarantees do **not** extend to `RegisterPrim` implementations. A blocking or panicking primitive can violate timeout guarantees. Review custom primitives for:

- Bounded execution time
- No uncontrolled side effects
- Correct `CapEnv` handling
- **No panics**: primitive panics propagate to the embedder as Go runtime panics rather than being wrapped into a `RuntimeError`. This surfaces stdlib and host-prim bugs at their source instead of masking them. Wrap `Engine.Run` / `Runtime.RunWith` in `recover()` if you need to contain prim-impl bugs at the embedding boundary.

**CLI module paths**: The `--module Name=path` flag reads any file accessible to the process. If an AI agent constructs CLI arguments, validate module paths against an allowed directory to prevent arbitrary file reads via parser error messages.

**Assumption declarations**: User-written GICEL code cannot use `assumption` declarations (blocked at compile time). The Go API allows assumptions by default for host-controlled bindings; call `eng.DenyAssumptions()` to enforce the restriction in Go-embedded contexts where source code is untrusted. Note: `RunSandbox` automatically calls `DenyAssumptions()` — only the `Engine` API requires explicit invocation.
