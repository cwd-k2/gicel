### Effect.IO

Provides log/dbg capabilities via the `io` capability. Load with `eng.Use(gicel.EffectIO)` and import with `import Effect.IO`.

**Functions:**

| Name  | Type                                  | Description                              |
| ----- | ------------------------------------- | ---------------------------------------- |
| `log` | `String -> Effect { io: () \| r } ()` | Append a string to the IO buffer         |
| `dbg` | `\a. a -> Effect { io: () \| r } ()`  | Append debug representation to IO buffer |

Host provides `"io"` capability. Output accumulates as `[]string` in the final CapEnv.

> **Note:** `log` and `dbg` do not write to stdout or stderr. They are pure operations
> that append strings to the `io` capability buffer. The host retrieves accumulated output
> from `result.CapEnv` after execution.

**Example:**

```
import Prelude
import Effect.IO

main := do {
  log "hello, ";
  log "world!";
  dbg 42
}
```

With `--json`, the `io` key in `capEnv` contains the accumulated strings.
