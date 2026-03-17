## 11. Go Integration

### Sandbox API

The simplest way to run GICEL from Go:

```go
import "github.com/cwd-k2/gicel"

result, err := gicel.RunSandbox(source, &gicel.SandboxConfig{
    Packs:    []gicel.Pack{gicel.Prelude, gicel.DataMap, gicel.DataSet, gicel.EffectFail, gicel.EffectState, gicel.EffectIO, gicel.DataStream, gicel.DataSlice},
    Entry:    "main",              // default: "main"
    Timeout:  5 * time.Second,     // default: 5s
    MaxSteps: 100_000,             // default: 100,000
    MaxDepth: 100,                 // default: 100
    Caps:     map[string]any{"state": gicel.ToValue(0), "io": gicel.ToValue(nil)},
    Bindings: map[string]gicel.Value{"input": gicel.ToValue("hello")},
})
```

`SandboxConfig` fields are all optional. `nil` uses conservative defaults.

`RunResult`: `Value` (result), `CapEnv` (final capabilities), `Stats` (EvalStats: `Steps int`, `MaxDepth int`, `Allocated int64`).

### Full Lifecycle API

```go
eng := gicel.NewEngine()
eng.Use(gicel.Prelude)
eng.Use(gicel.EffectState)
eng.SetStepLimit(500_000)

rt, err := eng.NewRuntime(source)
result, err := rt.RunWith(ctx, &gicel.RunOptions{Caps: caps, Bindings: bindings})
// result.Value, result.CapEnv, result.Stats
```

### Available Packs

| Pack               | Module         | Provides                                 |
| ------------------ | -------------- | ---------------------------------------- |
| `gicel.Prelude`    | `Prelude`      | Num, Str, List — arithmetic, strings, lists |
| `gicel.EffectFail` | `Effect.Fail`  | Failure effect                           |
| `gicel.EffectState`| `Effect.State` | Get/put state                            |
| `gicel.EffectIO`   | `Effect.IO`    | Print/debug output                       |
| `gicel.DataStream` | `Data.Stream`  | Lazy lists (requires recursion)          |
| `gicel.DataSlice`  | `Data.Slice`   | O(1) contiguous arrays                   |
| `gicel.DataMap`    | `Data.Map`     | Ordered immutable map (AVL)              |
| `gicel.DataSet`    | `Data.Set`     | Ordered immutable set                    |

### Custom Primitives (RegisterPrim)

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
| `gicel.FromRecord(v) (map, bool)`   | Extract record fields as `map[string]Value`             |

### Type Construction Helpers

| Function                                               | Description                   |
| ------------------------------------------------------ | ----------------------------- |
| `gicel.ConType(name)`                                  | Type constructor: `"Int"`     |
| `gicel.ArrowType(from, to)`                            | Function type: `from -> to`   |
| `gicel.AppType(f, arg)`                                | Type application: `f a`       |
| `gicel.CompType(pre, post, result)`                    | `Computation pre post result` |
| `gicel.ForallType(var, body)`                          | `\var. body`                  |
| `gicel.ForallRow(var, body)`                           | `\(var: Row). body`          |
| `gicel.EmptyRowType()`                                 | Empty row `{}`                |
| `gicel.RecordType(fields ...RowField)`                 | Closed record type            |
| `gicel.TupleType(elems ...Type)`                       | Tuple record type             |
| `gicel.KindType()`, `KindRow()`, `KindArrow(from, to)` | Kind constructors             |

**RowBuilder:** `gicel.NewRow().And("state", gicel.ConType("Int")).Closed()` or `.Open("r")`

### Engine Configuration

| Method                                     | Description                      |
| ------------------------------------------ | -------------------------------- |
| `eng.Use(pack)`                            | Apply a stdlib pack              |
| `eng.RegisterPrim(name, impl)`             | Register a primitive             |
| `eng.RegisterType(name, kind)`             | Register an opaque host type     |
| `eng.DeclareBinding(name, ty)`             | Declare a host-provided variable |
| `eng.EnableRecursion()`                    | Enable `rec` and `fix` built-ins |
| `eng.SetStepLimit(n)` / `SetDepthLimit(n)` | Resource limits                  |
| `eng.SetAllocLimit(bytes)`                 | Allocation limit (0 = disabled)  |
| `eng.Use(gicel.Prelude)`                   | Load Prelude (Num, Str, List)    |
| `eng.RegisterModule(name, src)`            | Register a custom module         |
| `eng.NewRuntime(source)`                   | Compile to Runtime               |
| `eng.Compile(source)` / `Parse(source)`    | Type-check or parse only         |

### Error Handling

```go
// Compile errors
var ce *gicel.CompileError
if errors.As(err, &ce) {
    for _, d := range ce.Diagnostics() {
        fmt.Printf("[%s] %d:%d: %s\n", d.Phase, d.Line, d.Col, d.Message)
    }
}

// Runtime errors
var re *gicel.RuntimeError
if errors.As(err, &re) { fmt.Println(re.Message) }
```

`Diagnostic`: `Code int`, `Phase string` ("lex"/"parse"/"check"), `Line int`, `Col int`, `Message string`.

`RuntimeError`: `Message string`, `Span`. Covers: unbound variable, non-exhaustive match, division by zero, `fail`/`failWith`. Step/depth/alloc limit exceeded return distinct error types (check with `err.Error()` or `errors.As`).

### Hooks (per-execution via RunOptions)

**TraceHook** fires on every eval step. Signature: `func(TraceEvent) error`. Fields: `Depth int`, `Node` (Core IR node, opaque), `Env` (lexical environment), `CapEnv`. Return non-nil error to abort.

**ExplainHook** fires at semantic boundaries. Signature: `func(ExplainStep)`. Fields: `Seq int`, `Depth int`, `Kind ExplainKind`, `Line int`, `Col int`, `Detail ExplainDetail`. Kinds: `ExplainBind`, `ExplainMatch`, `ExplainEffect`, `ExplainLabel`, `ExplainResult`.

```go
rt.RunWith(ctx, &gicel.RunOptions{
    Explain: func(s gicel.ExplainStep) { /* ... */ },
    Trace:   func(e gicel.TraceEvent) error { return nil },
})
```

**CheckTraceHook** fires during type checking. Set via `eng.SetCheckTraceHook(hook)`. Signature: `func(CheckTraceEvent)`. Fields: `Kind CheckTraceKind`, `Depth int`, `Message string`.
