# GICEL Agent Guide

## What is GICEL?

GICEL is a typed, pure functional language embedded in Go. It exists so that AI agents can **write and execute programs in a sandboxed environment** -- the host defines what capabilities are available, the agent writes GICEL source, and the host compiles and runs it safely with resource limits (step count, call depth, allocation, timeout).

Key properties for agents:

- **Safe by construction.** No file I/O, no network, no system calls unless the host explicitly provides them as capabilities.
- **Typed.** The type checker catches errors before execution. Use `gicel check` to validate without running.
- **Resource-bounded.** Step limits, depth limits, and allocation limits prevent runaway programs.
- **Deterministic.** Same source + same bindings = same result. No implicit state.

## How to use this guide

Use `gicel docs` to list available topics, then `gicel docs <topic>` to read one. Topics use dot-separated names for subcategories (e.g., `gicel docs features.records`). Use `gicel example` to browse example programs.

Typical agent workflow:

1. `gicel example` -- browse example programs to learn patterns
2. `gicel docs stdlib.packs` -- check what operations are available
3. Write a `.gicel` file with a `main` binding
4. `gicel check program.gicel` -- validate types
5. `gicel run program.gicel` -- execute and read the result
6. `gicel run --json program.gicel` -- machine-readable output

## Topics

### Language Basics

| Topic         | Content                                    |
| ------------- | ------------------------------------------ |
| `syntax`      | Keywords, punctuation, literals, comments  |
| `expressions` | Lambda, case, do, operators, special forms |
| `patterns`    | Common patterns and pitfalls               |

### Features

| Topic                    | Content                                                 |
| ------------------------ | ------------------------------------------------------- |
| `features.records`       | Record literals, projection, update, tuples, rows       |
| `features.adt`           | Data types, constructors, GADTs, pattern matching       |
| `features.type-classes`  | Classes, instances, superclasses, class hierarchy       |
| `features.type-families` | Closed TF, associated types, data families, injectivity |
| `features.effects`       | Computation, pure/bind, CapEnv, thunk/force             |
| `features.modules`       | Import forms, qualified names, private `_` prefix       |
| `features.session-types` | Session types, multiplicity, @Linear/@Affine, Dual      |

### Standard Library

| Topic              | Content                              |
| ------------------ | ------------------------------------ |
| `stdlib.prelude`   | Prelude types, classes, instances    |
| `stdlib.functions` | Prelude functions + operator table   |
| `stdlib.packs`     | Prelude, Effect._, Data._ pack guide |

### Host Integration

| Topic    | Content                                    |
| -------- | ------------------------------------------ |
| `go-api` | Go integration: sandbox, lifecycle, errors |

---

## 1. Quick Start

### Minimal Program

```
main := ()
```

This defines a binding `main` whose value is `()` (the unit value). No imports required.

### With Arithmetic (requires Prelude)

```
import Prelude

main := 2 + 3
```

### Hello World (requires Prelude and Effect.IO)

```
import Prelude
import Effect.IO

main := print "Hello, world!"
```

`main` here is a `Computation { io: () | r } { io: () | r } ()`. The host must provide the `io` capability.

### Running Programs

**CLI:**

```sh
# Run with all stdlib packs (default)
gicel run program.gicel

# Type-check only
gicel check program.gicel

# Select specific packs
gicel run --packs prelude,state program.gicel

# Custom entry point, limits, JSON output
gicel run --entry myFunc --timeout 10s --max-steps 500000 --json program.gicel

# Semantic evaluation trace -- shows effects, binds, and pattern matches
gicel run --explain program.gicel

# Verbose trace with source context
gicel run --explain --verbose program.gicel
```

CLI flags:

| Flag            | Default  | Description                                                              |
| --------------- | -------- | ------------------------------------------------------------------------ |
| `--packs`       | `all`    | Comma-separated packs: prelude, fail, state, io, stream, slice, map, set |
| `--module`      | --       | Register user module: `Name=path` (repeatable, run & check)              |
| `--recursion`   |          | Enable recursive definitions (run, check)                                |
| `--entry`       | `main`   | Entry point binding name                                                 |
| `--timeout`     | `5s`     | Execution timeout (run only)                                             |
| `--max-steps`   | `100000` | Step limit (run only)                                                    |
| `--max-depth`   | `100`    | Depth limit (run only)                                                   |
| `--max-nesting` | `512`    | Structural nesting depth limit                                           |
| `--max-alloc`   | `100MiB` | Allocation byte limit (run only)                                         |
| `--json`        | `false`  | Output result as JSON (run, check)                                       |
| `--explain`     | `false`  | Show semantic evaluation trace (run only)                                |
| `--explain-all` | `false`  | Trace stdlib internals too (with --explain)                              |
| `--verbose`     | `false`  | Show source context in explain trace (run only)                          |
| `--no-color`    | `false`  | Disable color output; also respects `NO_COLOR` env var                   |
| `-e <source>`   | --       | Evaluate source string directly (run, check)                             |

**Inline source (`-e`):** Semicolons and newlines are interchangeable separators.
Use `;` when writing inline: `gicel run -e 'import Prelude; main := 1 + 2'`.

**`--explain-all`** is only effective when `--explain` is also set.

**Go API (Sandbox):**

```go
import "github.com/cwd-k2/gicel"

result, err := gicel.RunSandbox(`
import Prelude
main := 2 + 3
`, &gicel.SandboxConfig{
    Packs: []gicel.Pack{gicel.Prelude},
})
// result.Value is HostVal{Inner: int64(5)}
// CLI prints: 5  (PrettyValue formats source-level terms)
```

**Go API (Full lifecycle):**

```go
eng := gicel.NewEngine()
eng.Use(gicel.Prelude)
eng.Use(gicel.EffectState)

rt, err := eng.NewRuntime(ctx, source)
result, err := rt.RunWith(ctx, nil)
```

---
