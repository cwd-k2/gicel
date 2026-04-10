# GICEL

**G**o's **I**ndexed **C**apability **E**ffect **L**ibrary /
**G**ICEL's **I**ndexed **C**apability **E**ffect **L**anguage

**v0.28.1** — [Changelog](CHANGELOG.md)

> **Pre-1.0 notice:** GICEL is pre-1.0. Breaking changes to syntax or API may occur between minor versions.

Embed a type-safe, sandboxed language in your Go application.
GICEL compiles Haskell-like source into typed computations, runs them with
explicitly granted capabilities, and returns results — all in pure Go.

```gicel
import Prelude
import Effect.State

consume :: Int -> Effect { state: Int | r } ()
consume := \n. modify (\budget. budget - n)

main := evalState 1000 (thunk do {
  consume 200;
  consume 150;
  get
})
```

The source imports `Effect.State` — but the host decides whether that module exists:

```sh
$ gicel run --packs prelude program.gicel         # host grants only Prelude
error[E0230]: unknown module: Effect.State
 --> program.gicel:2:1
   |
 2 | import Effect.State
   | ^^^^^^^^^^^^^^^^^^^

$ gicel run --packs prelude,state program.gicel   # host grants State too
650
```

Same source. The difference is what the host allowed.
No blacklist. No restricted mode. Capabilities are absent until granted.

## Why GICEL?

When you run untrusted code inside Go — from an AI agent, a user script,
or a plugin — existing approaches try to **restrict a permissive environment**:
blacklisting APIs, blocking syscalls, filtering globals. The harder you lock
down, the more edge cases slip through.

**GICEL inverts the model.**

- **Nothing exists until you grant it.** Programs run in an empty capability
  environment. The host explicitly opts in to each capability via Packs.
- **Type-checked isolation.** Effects like State or IO only become available
  when the host provides them, and the type system enforces this boundary
  at compile time.
- **Resource limits with clean termination.** Step count, call depth,
  allocation ceiling, and timeout. `RunSandbox` applies timeout to the
  entire pipeline including compilation and evaluation.
- **Go-native.** No CGo, no FFI. Runtimes are immutable and goroutine-safe.

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
gicel run -e 'import Prelude; main := 1 + 2'  # inline eval
gicel run --explain program.gicel  # semantic evaluation trace
gicel run --json program.gicel     # structured output for tooling
gicel example                      # browse built-in examples
```

### Go Library

Three-tier lifecycle — compile once, run many:

```go
eng := gicel.NewEngine()
eng.Use(gicel.Prelude)      // grant Prelude
eng.Use(gicel.EffectState)  // grant Effect.State

rt, err := eng.NewRuntime(context.Background(), source)
// rt is immutable and goroutine-safe

result, err := rt.RunWith(context.Background(), nil)
fmt.Println(gicel.MustHost[int64](result.Value)) // 20
```

### AI Agent Sandbox

Single-call API with conservative defaults:

```go
result, err := gicel.RunSandbox(agentCode, &gicel.SandboxConfig{
    Packs:    []gicel.Pack{gicel.Prelude, gicel.EffectState},
    Timeout:  3 * time.Second,
    MaxSteps: 50_000,
    MaxAlloc: 10 * 1024 * 1024, // 10 MiB
})
```

Docs, examples, type checker, and evaluation trace are all in the binary —
an agent can learn, write, check, and debug without external resources:

```sh
gicel docs                        # browse reference topics
gicel example effects.state-effect  # read example source
gicel check --json program.gicel  # structured error diagnostics
gicel run --explain program.gicel # semantic step trace
```

## Extend with Go

Expose your own Go functions as host capabilities:

```go
eng.RegisterPrim("fetchPrice",
    func(ctx context.Context, ce gicel.CapEnv, args []gicel.Value, _ gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
        price, err := db.GetPrice(ctx, gicel.MustHost[string](args[0]))
        if err != nil { return nil, ce, err }
        return gicel.ToValue(price), ce, nil
    })
```

GICEL source declares the type; the host provides the implementation:

```gicel
fetchPrice :: String -> Effect { db: () | r } Int
fetchPrice := assumption

main := fetchPrice "item-42"
```

The type `Effect { db: () | r } Int` means: requires a `db` capability.
Compose it with other effectful functions — the type system ensures all
required capabilities are present. Custom effects get the same compile-time
guarantees as built-in ones.

See [`examples/go/`](examples/go/) for full patterns.

## Features

### Sandbox

- **Capability isolation** — nothing exists until granted; enforced at compile time
- **Resource limits** — step count, call depth, allocation ceiling, timeout
- **Determinism** — same source + same capabilities = same result
- **Structured diagnostics** — error codes, source locations, hints in JSON

### Type System

- Full type inference with higher-rank polymorphism
- Type classes (via unified `form`/`impl`) with superclasses, associated types
- Type families — closed, associated, form families with recursive reduction
- Row polymorphism — extensible records and capability environments
- GADTs with refined return types and existential types
- Grade annotations (`@Linear`, `@Affine`) for resource tracking
- DataKinds — all constructors promoted to type level
- Scoped evidence injection (`value => expr`) for local instance override

### Language

- 15 keywords: `case do form lazy type impl import infixl infixr infixn if then else as assumption`
- ADTs with exhaustive pattern matching (pipe shorthand and GADT styles)
- Do-notation for monadic sequencing
- Records, tuples, module system (open, selective, qualified imports)
- Scoped evidence injection (`value => expr`) with private instances

### Runtime

- Bytecode VM with tail-call optimization
- Allocation tracking against `MaxAlloc`
- Core IR optimizer with registered fusion rules
- `--explain` semantic trace; `--explain-all` with stdlib dim distinction

## Stdlib Packs

| Pack          | Module         | Highlights                                  |
| ------------- | -------------- | ------------------------------------------- |
| `Prelude`     | `Prelude`      | Num, Str, List, 15 type classes             |
| `EffectFail`  | `Effect.Fail`  | `fail`, `failWith`, `fromMaybe`             |
| `EffectState` | `Effect.State` | `get`, `put`, `modify`                      |
| `EffectIO`    | `Effect.IO`    | `log`, `dbg` (CapEnv buffer, not stdout)    |
| `EffectArray` | `Effect.Array` | Mutable fixed-size arrays                   |
| `EffectMap`   | `Effect.Map`   | Mutable ordered maps (AVL)                  |
| `EffectSet`   | `Effect.Set`   | Mutable ordered sets (AVL)                  |
| `DataStream`  | `Data.Stream`  | Lazy streams with `Foldable`                |
| `DataSlice`   | `Data.Slice`   | O(1) index/length immutable arrays          |
| `DataMap`     | `Data.Map`     | Immutable ordered map                       |
| `DataSet`     | `Data.Set`     | Immutable ordered set                       |
| `EffectRef`   | `Effect.Ref`   | Mutable reference cells                     |
| `DataJSON`    | `Data.JSON`    | `ToJSON`/`FromJSON` type classes            |
| `Console`     | `Console`      | `putLine`, `getLine` (CLI-only, real stdio) |

## Documentation

- [Language Specification](docs/spec/language.md) — formal spec (18 chapters)
- [Agent Guide](docs/agent-guide/) — complete reference with Go API details
- [Grammar Reference](docs/grammar-reference.md) — syntax at a glance
- [Changelog](CHANGELOG.md)

## Examples

- [`examples/gicel/`](examples/gicel/) — GICEL programs: ADTs, effects, type classes, records, ...
- [`examples/go/`](examples/go/) — Go embedding: lifecycle, capabilities, sandbox, ...
- [`examples/cli/multi-module/`](examples/cli/multi-module/) — CLI multi-file project

## Editor Support

- [tree-sitter-gicel](https://github.com/cwd-k2/tree-sitter-gicel) — Tree-sitter grammar
- [vscode-gicel](https://github.com/cwd-k2/vscode-gicel) — VS Code extension
- [nvim-gicel](https://github.com/cwd-k2/nvim-gicel) — Neovim plugin
- [zed-gicel](https://github.com/cwd-k2/zed-gicel) — Zed extension

## License

MIT
