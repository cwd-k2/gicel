### Console

Provides real stdin/stdout operations via the `console` capability. This is a **CLI-only** pack -- it is defined in the `gicel` CLI binary and is not available through the Go library API (`gicel.NewEngine()`).

Load with `--packs console` (or the default `all`) and import with `import Console`.

**Functions:**

| Name      | Type                                                                 | Description                 |
| --------- | -------------------------------------------------------------------- | --------------------------- |
| `putLine` | `String -> Computation { console: () \| r } { console: () \| r } ()` | Write a line to real stdout |
| `getLine` | `Computation { console: () \| r } { console: () \| r } String`       | Read a line from real stdin |

**Behavior:**

- `putLine` writes the string followed by a newline to the process's stdout.
- `getLine` reads one line from the process's stdin (blocks until input is available).

Unlike `Effect.IO`, which accumulates output in an internal buffer retrievable from `result.CapEnv`, Console performs actual I/O against the host process's file descriptors.

**`--json` mode:** When the CLI is invoked with `--json`, `putLine` output is captured into the `console` key of the capability environment (as `[]string`) instead of being written to stdout. `getLine` returns an error in this mode since interactive stdin is not available.
