## About GICEL

GICEL is a typed, pure functional language embedded in Go. It exists so that AI agents can **write and execute programs in a sandboxed environment** -- the host defines what capabilities are available, the agent writes GICEL source, and the host compiles and runs it safely with resource limits (step count, call depth, allocation, timeout).

Key properties for agents:

- **Safe by construction.** No file I/O, no network, no system calls unless the host explicitly provides them as capabilities.
- **Typed.** The type checker catches errors before execution. Use `gicel check` to validate without running.
- **Resource-bounded.** Step limits, depth limits, and allocation limits prevent runaway programs.
- **Deterministic.** Same source + same bindings = same result. No implicit state.

The type system supports advanced features -- type classes, type families, session types -- but none of them are required. Simple programs use only basic types and functions. When you need stronger safety guarantees (e.g., ensuring a protocol is followed or resources are used linearly), the type system and the sandbox capability model work together to enforce them.

## Quick Start

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

### Hello World (requires Prelude and Console)

```
import Prelude
import Console

main := putLine "Hello, world!"
```

`main` here is a `Computation { console: () | r } { console: () | r } ()`. The CLI provides the `console` capability and `putLine` writes directly to stdout.

> **Note:** `Effect.IO`'s `print` does _not_ write to stdout -- it appends to an internal buffer
> retrieved from `result.CapEnv` after execution. Use `Console` when you want visible terminal output.

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

| Flag            | Default   | Description                                                                                          |
| --------------- | --------- | ---------------------------------------------------------------------------------------------------- |
| `--packs`       | `all`     | Comma-separated packs: prelude, fail, state, io, stream, slice, map, set, array, mmap, mset, console |
| `--module`      | --        | Register user module: `Name=path` (repeatable, run & check)                                          |
| `--recursion`   |           | Enable recursive definitions (run, check)                                                            |
| `--entry`       | `main`    | Entry point binding name                                                                             |
| `--timeout`     | `5s`      | Execution/compilation timeout (run, check)                                                           |
| `--max-steps`   | `100000`  | Step limit (run only)                                                                                |
| `--max-depth`   | `10000`   | Depth limit (run only)                                                                               |
| `--max-nesting` | `512`     | Structural nesting depth limit (run only)                                                            |
| `--max-alloc`   | `100 MiB` | Allocation byte limit (run only)                                                                     |
| `--json`        | `false`   | Output result as JSON (run, check)                                                                   |
| `--explain`     | `false`   | Show semantic evaluation trace (run only)                                                            |
| `--explain-all` | `false`   | Trace stdlib internals too (with --explain)                                                          |
| `--verbose`     | `false`   | Show source context in explain trace (run only)                                                      |
| `--show`        | `true`    | Display result value on stdout (default on for non-JSON mode)                                        |
| `--no-color`    | `false`   | Disable color output; also respects `NO_COLOR` env var                                               |
| `--use`         | `all`     | Alias for `--packs`                                                                                  |
| `-e <source>`   | --        | Evaluate source string directly (run, check)                                                         |

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
// CLI: use --json to see the result, or --show to print the value
```

**Go API (Full lifecycle):**

```go
eng := gicel.NewEngine()
eng.Use(gicel.Prelude)
eng.Use(gicel.EffectState)

rt, err := eng.NewRuntime(ctx, source)
result, err := rt.RunWith(ctx, nil)
```
