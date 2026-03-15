# GICEL Agent Guide

## What is GICEL?

GICEL is a typed, pure functional language embedded in Go. It exists so that AI agents can **write and execute programs in a sandboxed environment** — the host defines what capabilities are available, the agent writes GICEL source, and the host compiles and runs it safely with resource limits (step count, call depth, allocation, timeout).

Key properties for agents:

- **Safe by construction.** No file I/O, no network, no system calls unless the host explicitly provides them as capabilities.
- **Typed.** The type checker catches errors before execution. Use `gicel check` to validate without running.
- **Resource-bounded.** Step limits, depth limits, and allocation limits prevent runaway programs.
- **Deterministic.** Same source + same bindings = same result. No implicit state.

## How to use this guide

Use `gicel docs <topic>` to query a specific chapter. Start with `syntax` for the language, `stdlib` for available operations, `patterns` for idiomatic code, or `go-api` for embedding in Go. Use `gicel example` to browse example programs.

Typical agent workflow:

1. `gicel example` — browse example programs to learn patterns
2. `gicel docs stdlib` — check what operations are available
3. Write a `.gicel` file with a `main` binding
4. `gicel check program.gicel` — validate types
5. `gicel run program.gicel` — execute and read the result
6. `gicel run --json program.gicel` — machine-readable output

## Chapters

| File             | Content                                            |
| ---------------- | -------------------------------------------------- |
| [syntax.md]      | Keywords, punctuation, literals, comments          |
| [types.md]       | Type system: ADT, GADT, forall, rows, kinds        |
| [expressions.md] | Lambda, case, do, operators, special forms         |
| [effects.md]     | Computation, pure/bind, CapEnv, thunk/force        |
| [prelude.md]     | Prelude types, classes, instances                  |
| [functions.md]   | Prelude functions + operator reference             |
| [stdlib.md]      | Std.Num, Str, List, State, Fail, IO, Stream, Slice |
| [patterns.md]    | Common patterns + pitfalls                         |
| [go-api.md]      | Go integration: sandbox, lifecycle, errors         |

---

## 1. Quick Start

### Minimal Program

```
main := True
```

This defines a binding `main` whose value is `True` (a Bool constructor from the Prelude).

### With Arithmetic (requires Std.Num)

```
import Std.Num

main := 2 + 3
```

### Hello World (requires Std.Str and Std.IO)

```
import Std.Str
import Std.IO

main := print "Hello, world!"
```

`main` here is a `Computation { io : () | r } { io : () | r } ()`. The host must provide the `io` capability.

### Running Programs

**CLI:**

```sh
# Run with all stdlib packs (default)
gicel run program.gicel

# Type-check only
gicel check program.gicel

# Select specific packs
gicel run --use Num,Str program.gicel

# Custom entry point, limits, JSON output
gicel run --entry myFunc --timeout 10s --max-steps 500000 --json program.gicel

# Semantic evaluation trace — shows effects, binds, and pattern matches
gicel run --explain program.gicel

# Verbose trace with source context
gicel run --explain --verbose program.gicel
```

CLI flags:

| Flag          | Default  | Description                                                           |
| ------------- | -------- | --------------------------------------------------------------------- |
| `--use`       | `all`    | Comma-separated packs: Num, Str, List, Fail, State, IO, Stream, Slice |
| `--recursion` |          | Enable recursive definitions (run, check)                             |
| `--entry`     | `main`   | Entry point binding name                                              |
| `--timeout`   | `5s`     | Execution timeout (run only)                                          |
| `--max-steps` | `100000` | Step limit (run only)                                                 |
| `--max-depth` | `100`    | Depth limit (run only)                                                |
| `--json`      | `false`  | Output result as JSON (run only)                                      |
| `--explain`   | `false`  | Show semantic evaluation trace (run only)                             |
| `--verbose`   | `false`  | Show source context in explain trace (run only)                       |
| `--no-color`  | `false`  | Disable color output; also respects `NO_COLOR` env var                |

**Go API (Sandbox):**

```go
import "github.com/cwd-k2/gicel"

result, err := gicel.RunSandbox(`
import Std.Num
main := 2 + 3
`, &gicel.SandboxConfig{
    Packs: []gicel.Pack{gicel.Num, gicel.Str},
})
// result.Value is HostVal{Inner: int64(5)}
// CLI prints: 5  (PrettyValue formats source-level terms)
```

**Go API (Full lifecycle):**

```go
eng := gicel.NewEngine()
eng.Use(gicel.Num)
eng.Use(gicel.Str)

rt, err := eng.NewRuntime(source)
result, err := rt.RunContext(ctx, nil, nil, "main")
```

---
