# GICEL

**G**o's **I**ndexed **C**apability **E**ffect **L**ibrary /
**G**ICEL's **I**ndexed **C**apability **E**ffect **L**anguage

Embed a type-safe, sandboxed language in your Go application.
GICEL compiles Haskell-like source into typed computations, runs them with
explicitly granted capabilities, and returns results — all in pure Go.

```gicel
import Std.Num
import Std.State

main := do { _ <- put 42; get }
```

The source imports `Std.State` — but the host decides whether that module exists:

```sh
$ gicel run --use Num program.gicel        # host grants only Num
error[E0230]: unknown module: Std.State
 --> program.gicel:2:1
   |
 2 | import Std.State
   | ^^^^^^^^^^^^^^^^

$ gicel run --use Num,State program.gicel  # host grants State too
42
```

Same source. The difference is what the host allowed.
No blacklist. No restricted mode. Capabilities are absent until granted.

## Why GICEL?

When you run untrusted code inside Go — from an AI agent, a user script,
or a plugin — existing approaches force trade-offs:

| Approach            | Trade-off                                             |
| ------------------- | ----------------------------------------------------- |
| Lua / JS embed      | Dynamically typed; capabilities leak via global state |
| Wasm sandbox        | Heavy runtime; complex FFI boundary with Go           |
| Template engine     | Safe, but limited expressiveness                      |
| Go subset interpret | Powerful, but attack surface equals Go itself         |

All of these share a fundamental problem: they try to **restrict a permissive
environment** — blacklisting dangerous APIs, blocking syscalls, filtering
globals. The harder you lock down, the more edge cases slip through.

**GICEL inverts the model.**

- **Nothing exists until you grant it.** Programs run in an empty capability
  environment. No IO, no state, no network — not restricted, simply absent.
  The host explicitly opts in to each capability via Packs. This is a
  whitelist, not a blacklist.
- **No side effects by default.** A GICEL computation is pure. It cannot
  touch the host's filesystem, memory, or goroutines. Effects like State
  or IO only become available when the host provides them, and the type
  system enforces this boundary at compile time.
- **Resource limits with clean termination.** Step count, memory ceiling,
  and timeout. Execution halts cleanly — no killed goroutines, no leaked state.
- **Go-native.** No CGo, no FFI. Runtimes are immutable and goroutine-safe.
  Embed it like any other Go library.

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
```

### Go Library

Three-tier lifecycle for production use:

```
Engine   (mutable)     — register types, capabilities, packs
  ↓ NewRuntime(source)
Runtime  (immutable)   — goroutine-safe, reuse across requests
  ↓ RunWith(ctx, opts)
result   (per-call)    — value + execution stats
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
    eng.Use(gicel.Num)   // grant Std.Num
    eng.Use(gicel.State) // grant Std.State

    rt, err := eng.NewRuntime(`
        import Std.Num
        import Std.State

        main := do {
            _ <- put 10;
            _ <- modify (\n. n * 2);
            get
        }
    `)
    if err != nil {
        log.Fatal(err)
    }

    // Safe to call from multiple goroutines
    result, err := rt.RunWith(context.Background(), nil)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(gicel.MustHost[int64](result.Value)) // 20
}
```

## AI Agent Sandbox

LLM generates code → GICEL compiles and type-checks → sandbox executes →
only the result comes back. The agent never touches the host environment.

Single-call API — no Engine/Runtime lifecycle management needed:

```go
result, err := gicel.RunSandbox(agentCode, &gicel.SandboxConfig{
    Packs:    []gicel.Pack{gicel.Num, gicel.Str},
    Timeout:  3 * time.Second,
    MaxSteps: 50_000,
    MaxAlloc: 10 * 1024 * 1024, // 10 MiB
})
// result.Value, result.Stats.Steps, result.Stats.Allocated
```

### Self-contained feedback loop

Docs, examples, type checker, and execution trace are all in the binary.
No external resources, no API calls — an agent can learn, write, check,
and debug without leaving the CLI:

```sh
# 1. Learn — browse available examples
$ gicel example
Basics:
  hello           Hello World
  lists           List Operations
  ...
Effects & Applications:
  state-machine   State Machine
  data-pipeline   Data Pipeline
  ...

# 2. Study — read an example
$ gicel example state-effect
import Std.Num
import Std.State
...

# 3. Write and check — catch errors before execution
$ cat program.gicel
import Std.Num
main := do { _ <- put 42; get }

$ gicel check program.gicel
error[E0201]: unbound variable: put
 --> program.gicel:2:19
   |
 2 | main := do { _ <- put 42; get }
   |                   ^^^

# 4. Fix — grant the State capability, run with trace
$ cat program.gicel
import Std.Num
import Std.State
main := do { _ <- put 42; get }

$ gicel run --explain program.gicel
── main ────────────────────────────────────────────────
  0  :  3  effect │ put 42  [state: _ → 42]
  0  :  3  effect │ get ⇒ 42
42
```

Reference docs are also queryable by topic:

```sh
gicel docs stdlib       # available types and functions
gicel docs patterns     # idiomatic patterns and pitfalls
gicel docs syntax       # language syntax reference
```

## Extend with Go

The whitelist model means nothing exists by default — and it means you can
grant anything. Expose your own Go functions as capabilities:

```go
eng.RegisterPrim("fetchPrice",
    func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
        itemID := gicel.MustHost[string](args[0])
        price, err := db.GetPrice(ctx, itemID) // your Go code
        if err != nil {
            return gicel.Nil, capEnv, err
        }
        return gicel.ToValue(price), capEnv, nil
    })
```

GICEL source declares the type with `assumption` — a placeholder that says
"this function exists, with this type, but the implementation lives on the
host side":

```gicel
fetchPrice :: \(r: Row). String -> Effect { db: () | r } Int
fetchPrice := assumption

main := fetchPrice "item-42"
```

`Effect { db: () | r } Int` means: this function requires a `db` capability.
If you compose it with other effectful functions, the type system ensures all
required capabilities are present — your custom effects get the same
compile-time guarantees as the built-in ones.

Register types, bindings, and primitives — GICEL programs can only use what
you explicitly provide. See [`examples/go/`](examples/go/) for full patterns:
host bindings, custom capabilities, custom prelude, and more.

## Features

- **Small, learnable syntax** — 10 keywords. ADTs, pattern matching, type classes, do-notation
- **Errors caught before execution** — full type inference with bidirectional checking. Missing capabilities are compile-time errors, not runtime surprises
- **Expressive when you need it** — higher-rank polymorphism, higher-kinded types, kind inference
- **Records & tuples** — structured data with row polymorphism
- **8 stdlib packs** — Num, Str, List, Fail, State, IO, Stream, Slice — opt in to what you need

## Stdlib Packs

| Pack     | Contents                                                  |
| -------- | --------------------------------------------------------- |
| `Num`    | Integer arithmetic, `Eq`/`Ord` Int instances              |
| `Str`    | String and rune operations                                |
| `List`   | `fromSlice`, `toSlice`, `length`, `concat`, `foldl`, etc. |
| `Fail`   | Fail effect capability                                    |
| `State`  | `get`/`put` state capabilities                            |
| `IO`     | `print`/`debug` via CapEnv buffer                         |
| `Stream` | Lazy list: `LCons`/`LNil`, `headS`, `tailS`, `takeS`      |
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
