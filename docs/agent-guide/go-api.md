## 11. Go Integration

### Sandbox API

The simplest way to run GICEL from Go:

```go
import "github.com/cwd-k2/gicel"

result, err := gicel.RunSandbox(source, &gicel.SandboxConfig{
    Packs:    []gicel.Pack{gicel.Num, gicel.Str, gicel.List, gicel.Fail, gicel.State, gicel.IO},
    Entry:    "main",              // default: "main"
    Timeout:  5 * time.Second,     // default: 5s
    MaxSteps: 100_000,             // default: 100,000
    MaxDepth: 100,                 // default: 100
    Caps:     map[string]any{      // initial capabilities
        "state": gicel.ToValue(0),
        "io":    gicel.ToValue(nil),
    },
    Bindings: map[string]gicel.Value{  // host-provided variable values
        "input": gicel.ToValue("hello"),
    },
})
```

`SandboxConfig` fields are all optional. Passing `nil` uses conservative defaults (no packs, entry "main", 5s timeout, 100k steps, depth 100).

`SandboxResult` contains:

| Field    | Type        | Description                       |
| -------- | ----------- | --------------------------------- |
| `Value`  | `Value`     | The result of evaluating `entry`  |
| `CapEnv` | `CapEnv`    | Final capability environment      |
| `Stats`  | `EvalStats` | Steps taken and max depth reached |

**EvalStats fields:**

| Field       | Type    | Description                            |
| ----------- | ------- | -------------------------------------- |
| `Steps`     | `int`   | Total evaluation steps taken           |
| `MaxDepth`  | `int`   | Maximum call depth reached             |
| `Allocated` | `int64` | Bytes allocated (0 if tracking is off) |

Note: the field is `Allocated`, not `Allocs` or `AllocBytes`.

### Full Lifecycle API

```go
// 1. Create and configure the engine
eng := gicel.NewEngine()
eng.Use(gicel.Num)
eng.Use(gicel.Str)
eng.SetStepLimit(500_000)
eng.SetDepthLimit(200)

// 2. Compile to immutable Runtime (goroutine-safe, reusable)
rt, err := eng.NewRuntime(source)

// 3. Execute (can call many times with different inputs)
result, err := rt.RunContext(ctx, caps, bindings, "main")
// or with full CapEnv in result:
result, err := rt.RunContextFull(ctx, caps, bindings, "main")
```

### Available Packs

| Pack          | Variable | Module Name | Provides                   |
| ------------- | -------- | ----------- | -------------------------- |
| `gicel.Num`   | `Num`    | `Std.Num`   | Arithmetic, Int instances  |
| `gicel.Str`   | `Str`    | `Std.Str`   | String/Rune ops, instances |
| `gicel.List`  | `List`   | `Std.List`  | Native list operations     |
| `gicel.Fail`  | `Fail`   | `Std.Fail`  | Failure effect             |
| `gicel.State` | `State`  | `Std.State` | Get/put state              |
| `gicel.IO`    | `IO`     | `Std.IO`    | Print/debug output         |

### Custom Primitives (RegisterPrim)

Register a Go function as a primitive callable from GICEL:

```go
// In source: declare type and mark as assumption
// greet :: String -> ()
// greet := assumption

eng.RegisterPrim("greet", func(
    ctx context.Context,
    capEnv gicel.CapEnv,
    args []gicel.Value,
    apply gicel.Applier,
) (gicel.Value, gicel.CapEnv, error) {
    s := gicel.MustHost[string](args[0])
    fmt.Println("Hello,", s)
    return gicel.ToValue(nil), capEnv, nil  // nil -> ()
})
```

**PrimImpl signature:** `func(ctx, capEnv, args, apply) -> (Value, CapEnv, error)`

- `ctx`: context for cancellation/timeout
- `capEnv`: current capability environment (copy-on-write)
- `args`: fully-applied argument values (the runtime curries automatically)
- `apply`: callback to apply a GICEL closure to an argument (for higher-order primitives like `foldl`)

The primitive must return the new CapEnv (or the same one if unchanged).

### Host Bindings (DeclareBinding)

Inject a typed variable from Go that the source can reference:

```go
eng.RegisterType("Int", gicel.KindType())
eng.DeclareBinding("myInput", gicel.ConType("Int"))

// At runtime, provide the value:
bindings := map[string]gicel.Value{
    "myInput": gicel.ToValue(42),
}
result, err := rt.RunContext(ctx, nil, bindings, "main")
```

In source, `myInput` is available as a variable of type `Int`.

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

For use with `DeclareBinding` and `DeclareAssumption`:

| Function                                    | Description                                |
| ------------------------------------------- | ------------------------------------------ |
| `gicel.ConType(name) Type`                  | Simple type constructor: `"Int"`, `"Bool"` |
| `gicel.VarType(name) Type`                  | Type variable reference                    |
| `gicel.ArrowType(from, to) Type`            | Function type: `from -> to`                |
| `gicel.AppType(f, arg) Type`                | Type application: `f a`                    |
| `gicel.CompType(pre, post, result) Type`    | `Computation pre post result`              |
| `gicel.ThunkType(pre, post, result) Type`   | `Thunk pre post result`                    |
| `gicel.ForallType(var, body) Type`          | `forall var. body` (kind Type)             |
| `gicel.ForallRow(var, body) Type`           | `forall (var : Row). body`                 |
| `gicel.ForallKind(var, k, body) Type`       | `forall (var : k). body`                   |
| `gicel.EmptyRowType() Type`                 | Empty closed row `{}`                      |
| `gicel.KindType() Kind`                     | The `Type` kind                            |
| `gicel.KindRow() Kind`                      | The `Row` kind                             |
| `gicel.KindArrow(from, to) Kind`            | Kind arrow: `from -> to`                   |
| `gicel.RecordType(fields ...RowField) Type` | Shorthand for closed record type           |
| `gicel.TupleType(elems ...Type) Type`       | Shorthand for tuple record type            |
| `gicel.TypePretty(t Type) string`           | Pretty-print a type for debugging          |
| `gicel.TypeEqual(a, b Type) bool`           | Structural type equality check             |

**RowBuilder** for constructing row types:

```go
row := gicel.NewRow().And("state", gicel.ConType("Int")).And("fail", gicel.ConType("String")).Closed()
// produces { state : Int, fail : String }

openRow := gicel.NewRow().And("state", gicel.ConType("Int")).Open("r")
// produces { state : Int | r }
```

### Engine Configuration Methods

| Method                                      | Description                                                             |
| ------------------------------------------- | ----------------------------------------------------------------------- |
| `eng.Use(pack Pack) error`                  | Apply a stdlib pack                                                     |
| `eng.RegisterPrim(name, impl)`              | Register a primitive implementation                                     |
| `eng.RegisterType(name, kind)`              | Register an opaque host type                                            |
| `eng.DeclareBinding(name, ty)`              | Declare a host-provided variable                                        |
| `eng.DeclareAssumption(name, ty)`           | Declare a primitive type (usually not needed if `::` is used in source) |
| `eng.EnableRecursion()`                     | Enable `rec` and `fix` built-ins                                        |
| `eng.SetStepLimit(n int)`                   | Set maximum evaluation steps                                            |
| `eng.SetDepthLimit(n int)`                  | Set maximum call depth                                                  |
| `eng.SetAllocLimit(bytes int64)`            | Set maximum allocation bytes (0 = disabled)                             |
| `eng.NoPrelude()`                           | Disable automatic Prelude loading                                       |
| `eng.SetPrelude(source string)`             | Replace default Prelude with custom source (CoreSource still prepended) |
| `eng.SetTraceHook(hook)`                    | Set evaluation trace callback (hook returns error to abort)             |
| `eng.SetCheckTraceHook(hook)`               | Set type checking trace callback                                        |
| `eng.RegisterModule(name, src)`             | Compile and register a custom module                                    |
| `eng.NewRuntime(source) (*Runtime, error)`  | Compile source to Runtime                                               |
| `eng.Check(source) (*CoreProgram, error)`   | Type-check without creating Runtime                                     |
| `eng.Parse(source) (*ParsedProgram, error)` | Parse without type-checking                                             |

### Error Handling

**Compile errors:**

```go
rt, err := eng.NewRuntime(source)
if err != nil {
    var ce *gicel.CompileError
    if errors.As(err, &ce) {
        for _, d := range ce.Diagnostics() {
            fmt.Printf("[%s] line %d col %d: %s\n", d.Phase, d.Line, d.Col, d.Message)
        }
    }
}
```

`Diagnostic` fields:

| Field     | Type     | Description                      |
| --------- | -------- | -------------------------------- |
| `Code`    | `int`    | Error code                       |
| `Phase`   | `string` | `"lex"`, `"parse"`, or `"check"` |
| `Line`    | `int`    | 1-based line number              |
| `Col`     | `int`    | 1-based column number            |
| `Message` | `string` | Human-readable error message     |

Note: `Phase` is a `string`, not an `int`.

**Runtime errors:**

```go
_, err := rt.RunContext(ctx, caps, bindings, "main")
if err != nil {
    var re *gicel.RuntimeError
    if errors.As(err, &re) {
        fmt.Println(re.Message)  // e.g. "step limit exceeded"
    }
}
```

`RuntimeError` fields: `Message string`, `Span span.Span`. Runtime errors include: step limit exceeded, depth limit exceeded, alloc limit exceeded, missing capability, division by zero, explicit `fail`/`failWith`.

### Trace Hooks

**Evaluation trace** (`SetTraceHook`):

```go
eng.SetTraceHook(func(event gicel.TraceEvent) error {
    fmt.Printf("depth=%d node=%T\n", event.Depth, event.Node)
    return nil  // return non-nil error to abort evaluation
})
```

`TraceEvent` fields:

| Field    | Type        | Description                                                    |
| -------- | ----------- | -------------------------------------------------------------- |
| `Depth`  | `int`       | Current call depth                                             |
| `Node`   | `core.Core` | Core IR node being evaluated (internal type; use `%T` or `%v`) |
| `Env`    | `*eval.Env` | Current environment (internal type; opaque to external users)  |
| `CapEnv` | `CapEnv`    | Current capability environment                                 |

Important: `TraceHook` has signature `func(TraceEvent) error` — returning a non-nil error aborts evaluation immediately. `Node` and `Env` are internal types; external users can inspect them via `fmt.Sprintf("%T", event.Node)` to see the Core IR node type (e.g., `*core.App`, `*core.Case`).

**Type checking trace** (`SetCheckTraceHook`):

```go
eng.SetCheckTraceHook(func(event gicel.CheckTraceEvent) {
    fmt.Printf("[%d] depth=%d %s\n", event.Kind, event.Depth, event.Message)
})
```

`CheckTraceEvent` fields: `Kind CheckTraceKind`, `Depth int`, `Message string`, `Span span.Span`. Note: `CheckTraceHook` does NOT return an error (unlike `TraceHook`). `CheckTraceKind` constants are internal; compare numerically if needed (0=Unify, 1=SolveMeta, 2=Infer, 3=Check, 4=Instantiate, 5=RowUnify).
