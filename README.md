# GICEL

**G**o's **I**ndexed **C**apability **E**ffect **L**ibrary /
**G**ICEL's **I**ndexed **C**apability **E**ffect **L**anguage

Write pure, type-checked computations in Haskell-like syntax —
compile and run them safely inside your Go program.

```gicel
import Std.Num
import Std.State

processOrder := do {
  _ <- put 50;                           -- base price
  _ <- modify (\p -> p * 3);             -- quantity: 3
  _ <- modify (\t -> t - t * 10 / 100);  -- 10% discount
  get
}

main := processOrder  -- 135
```

## Features

- **Haskell-like surface syntax** — 11 keywords, ADTs, pattern matching, type classes, do-notation
- **Atkey parameterized monad** — `Computation pre post a` with row-typed capability environments
- **Bidirectional type checking** — higher-rank polymorphism, HKT with kind inference
- **Three-tier lifecycle** — Engine (configure) → Runtime (immutable, goroutine-safe) → result (per-call)
- **AI agent sandboxing** — single-call `RunSandbox` API with resource limits, built-in documentation and examples queryable from CLI and Go API, semantic evaluation trace (`--explain`)
- **8 stdlib packs** — Num, Str, List, Fail, State, IO, Stream, Slice

## Install

```
go get github.com/cwd-k2/gicel@latest
```

CLI:

```
go install github.com/cwd-k2/gicel/cmd/gicel@latest
```

## Quick Start

### CLI

```sh
gicel run hello.gicel              # compile and execute
gicel check program.gicel          # type-check only
gicel run --explain program.gicel  # semantic evaluation trace
gicel run --json program.gicel     # machine-readable output
gicel docs stdlib                  # query language reference
gicel example hello                # browse example programs
```

### Go Library

```
Engine   (mutable, configurable)
  ↓ NewRuntime(source)
Runtime  (immutable, goroutine-safe)
  ↓ RunWith(ctx, opts)
result   (per-execution)
```

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

    result, err := rt.RunWith(context.Background(), nil)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Value) // True
}
```

## Sandbox

Single-call compile+execute with resource limits.
Designed for AI agent integration — no Engine/Runtime lifecycle management needed:

```go
result, err := gicel.RunSandbox(source, &gicel.SandboxConfig{
    Packs:    []gicel.Pack{gicel.Num, gicel.Str},
    Timeout:  3 * time.Second,
    MaxSteps: 50_000,
    MaxAlloc: 10 * 1024 * 1024, // 10 MiB
})
```

The CLI also supports sandboxed execution with the same resource controls:

```sh
gicel run --timeout 3s --max-steps 50000 program.gicel
```

### Built-in documentation

Language reference and example programs are embedded in the CLI and queryable
from Go, so AI agents can learn the language without external resources:

```sh
gicel docs stdlib       # query a topic
gicel docs patterns     # idiomatic patterns and pitfalls
gicel example hello     # show a named example
```

### Evaluation trace

`--explain` produces a semantic trace of evaluation — useful for agents to
debug programs or understand execution flow:

```sh
gicel run --explain program.gicel          # high-level trace
gicel run --explain --verbose program.gicel # with source context
```

## Stdlib Packs

| Pack     | Contents                                                  |
| -------- | --------------------------------------------------------- |
| `Num`    | Integer arithmetic, `Eq`/`Ord` Int instances              |
| `Str`    | String and rune operations                                |
| `List`   | `fromSlice`, `toSlice`, `length`, `concat`, `foldl`, etc. |
| `Fail`   | Fail effect capability                                    |
| `State`  | `get`/`put` state capabilities                            |
| `IO`     | `print`/`debug` via CapEnv buffer                         |
| `Stream` | Lazy list: `LCons`/`LNil`, `headS`, `tailS`, `takeS`     |
| `Slice`  | Contiguous array: O(1) length/index, `Functor`/`Foldable` |

## Documentation

- [Language Specification](spec/language.md) — formal spec (17 chapters)
- [Agent Guide](docs/agent-guide/) — complete language reference with Go API details
- [Grammar Reference](docs/grammar-reference.md) — syntax at a glance

## Examples

- [`examples/gicel/`](examples/gicel/) — GICEL programs: ADTs, type classes, HKT, records, effects, ...
- [`examples/go/`](examples/go/) — Go embedding: lifecycle, capabilities, sandbox, ...

## Editor Support

- [tree-sitter-gicel](https://github.com/cwd-k2/tree-sitter-gicel) — Tree-sitter grammar
- [vscode-gicel](https://github.com/cwd-k2/vscode-gicel) — VS Code extension
- [nvim-gicel](https://github.com/cwd-k2/nvim-gicel) — Neovim plugin
- [zed-gicel](https://github.com/cwd-k2/zed-gicel) — Zed extension

## License

MIT
