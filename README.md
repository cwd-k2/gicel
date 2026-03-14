# GICEL

**G**o's **I**ndexed **C**apability **E**ffect **L**ibrary /
**G**ICEL's **I**ndexed **C**apability **E**ffect **L**anguage

An embedded typed effect language for Go. Haskell-like surface syntax with an
Atkey parameterized monad (`Computation pre post a`) and row-typed capability
environments.

```
main := do {
  _ <- incCounter ();
  _ <- incCounter ();
  getCounter ()
}
```

## Install

```
go get github.com/cwd-k2/gicel@latest
```

CLI:

```
go install github.com/cwd-k2/gicel/cmd/gicel@latest
```

## CLI

```
gicel run     [flags] <file>   # compile and execute
gicel check   [flags] <file>   # type-check only
gicel docs    [topic]          # show language reference
gicel example [name]           # show example programs
```

Flags:

| Flag              | Default  | Description                                   |
| ----------------- | -------- | --------------------------------------------- |
| `--use <packs>`   | `all`    | Stdlib packs: Num, Str, List, Fail, State, IO |
| `--recursion`     |          | Enable recursive definitions (run, check)     |
| `--entry <name>`  | `main`   | Entry point binding (run only)                |
| `--timeout <dur>` | `5s`     | Execution timeout (run only)                  |
| `--max-steps <n>` | `100000` | Step limit (run only)                         |
| `--max-depth <n>` | `100`    | Depth limit (run only)                        |
| `--json`          |          | JSON output (run only)                        |
| `--explain`       |          | Semantic evaluation trace (run only)          |

```
gicel run hello.gicel
gicel run --use Num,Str --json program.gicel
gicel run --explain program.gicel
gicel check program.gicel
gicel docs stdlib
gicel example hello
```

## Library

### Three-tier lifecycle

```
Engine   (mutable, configurable)
  ↓ NewRuntime(source)
Runtime  (immutable, goroutine-safe)
  ↓ RunContext(ctx, ...)
result   (per-execution)
```

### Minimal example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/cwd-k2/gicel"
)

func main() {
    eng := gicel.NewEngine()

    rt, err := eng.NewRuntime(`
        not := \b -> case b { True -> False; False -> True }
        main := not False
    `)
    if err != nil {
        log.Fatal(err)
    }

    result, err := rt.RunContext(context.Background(), nil, nil, "main")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Value) // True
}
```

### Host bindings

Inject Go values into GICEL programs:

```go
eng := gicel.NewEngine()
eng.RegisterType("Int", gicel.KindType())
eng.DeclareBinding("n", gicel.ConType("Int"))

rt, err := eng.NewRuntime(`main := Just n`)
if err != nil { log.Fatal(err) }

result, err := rt.RunContext(context.Background(), nil,
    map[string]gicel.Value{"n": gicel.ToValue(42)}, "main")
```

The same `Runtime` can be executed concurrently with different binding values.

### Capabilities

Primitives interact with a capability environment (`CapEnv`) that threads
through evaluation with copy-on-write semantics:

```go
eng := gicel.NewEngine()
eng.RegisterType("Int", gicel.KindType())

eng.RegisterPrim("getCounter",
    func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
        v, _ := capEnv.Get("counter")
        return gicel.ToValue(v.(int)), capEnv, nil
    })

eng.RegisterPrim("incCounter",
    func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
        v, _ := capEnv.Get("counter")
        return gicel.ToValue(nil), capEnv.Set("counter", v.(int)+1), nil
    })

rt, _ := eng.NewRuntime(`
    getCounter :: () -> Computation {} {} Int
    getCounter := assumption

    incCounter :: () -> Computation {} {} ()
    incCounter := assumption

    main := do {
      _ <- incCounter ();
      _ <- incCounter ();
      getCounter ()
    }
`)

result, _ := rt.RunContextFull(context.Background(),
    map[string]any{"counter": 0}, nil, "main")

fmt.Println(gicel.MustHost[int](result.Value)) // 2
```

### Sandbox API

Single-call compile+execute with conservative resource limits. Designed for
AI agent integration:

```go
result, err := gicel.RunSandbox(source, &gicel.SandboxConfig{
    Packs:    []gicel.Pack{gicel.Num, gicel.Str},
    Timeout:  3 * time.Second,
    MaxSteps: 50_000,
    MaxAlloc: 10 * 1024 * 1024, // 10 MiB
})
```

### Stdlib packs

| Pack    | Contents                                                  |
| ------- | --------------------------------------------------------- |
| `Num`   | Integer arithmetic, `Eq`/`Ord` Int instances              |
| `Str`   | String and rune operations                                |
| `List`  | `fromSlice`, `toSlice`, `length`, `concat`, `foldl`, etc. |
| `Fail`  | Fail effect capability                                    |
| `State` | `get`/`put` state capabilities                            |
| `IO`    | `print`/`debug` via CapEnv buffer                         |

### Value conversion

```go
gicel.ToValue(42)                       // Go → GICEL
gicel.MustHost[int](v)                  // GICEL → Go (panics on type mismatch)
gicel.FromHost(v)                       // GICEL → Go (returns ok)
gicel.FromBool(v)                       // Bool constructor → bool
gicel.FromCon(v)                        // Constructor → (name, args, ok)
gicel.FromRecord(v)                     // Record → map[string]Value
gicel.ToList([]any{1, 2, 3})            // Go slice → GICEL List
gicel.FromList(v)                       // GICEL List → Go slice
```

### Error handling

Compilation errors are returned as `*gicel.CompileError` with structured
diagnostics:

```go
rt, err := eng.NewRuntime(source)
if err != nil {
    var ce *gicel.CompileError
    if errors.As(err, &ce) {
        for _, d := range ce.Diagnostics() {
            fmt.Printf("%s:%d:%d: %s\n", d.Phase, d.Line, d.Col, d.Message)
        }
    }
}
```

Runtime errors are returned as `*gicel.RuntimeError`.

### Resource limits

```go
eng.SetStepLimit(500_000)   // max evaluation steps
eng.SetDepthLimit(200)      // max call depth
eng.SetAllocLimit(10 << 20) // max allocation (bytes)
eng.EnableRecursion()       // enable rec/fix (off by default)
```

## Language

GICEL has 11 keywords: `case do data type forall infixl infixr infixn class instance import`.

See [docs/agent-guide/](docs/agent-guide/) for a complete language
reference and [spec/language.md](spec/language.md) for the formal specification.

## Examples

- `examples/gicel/` — GICEL programs covering ADTs, type classes, HKT, records, effects, etc.
- `examples/go/` — Go programs demonstrating the embedding API (lifecycle, capabilities, sandbox, etc.)

## Editor Support

- [tree-sitter-gicel](https://github.com/cwd-k2/tree-sitter-gicel) — Tree-sitter grammar for GICEL
- [vscode-gicel](https://github.com/cwd-k2/vscode-gicel) — VS Code extension (syntax highlighting, diagnostics)

## License

MIT
