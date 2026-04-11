# GICEL

## Rules

- Build output goes to `bin/` (gitignored).
- Temporary files go to `tmp/` (gitignored). Test artifacts, scratch files, generated data — anything ephemeral.
- Format Go with `goimports`, docs with `prettier`.
- Commit per logical group or phase completion.
- Do not run test agents in background (memory exhaustion incident, 2024-03-14).
- **One way for one thing.** Do not create multiple implementation paths for the same operation or pattern. When common logic appears in multiple places, consolidate into a single representative helper. Branching is justified only when the semantics genuinely differ. Codebase divergence obscures design intent and amplifies the cost of change.
- **No hacks, no workarounds. Follow theory.** Do not repurpose display APIs (`Pretty`, `String`) for identity or cache keys. Do not build heuristics that infer meaning from naming conventions or text shapes. Express meaning through structural data; when string encoding is needed, provide exactly one canonical serializer with guaranteed injectivity. When compromise is necessary, do not silently degrade — document the constraints and reasons, and draw the boundary explicitly.

## Commands

```sh
go test ./...                          # run tests
go build ./...                         # compile check (no output)
go build -o bin/gicel ./cmd/gicel/     # build CLI binary to bin/
go run ./examples/go/<name>/           # run Go example (no binary)
goimports -w .                         # format Go
prettier --write docs/                 # format docs
./scripts/smoke-test.sh                # CLI smoke test
./scripts/run-examples.sh              # run all Go + GICEL examples
./scripts/full-check.sh                # full suite (tests + examples + smoke)

# Performance (see docs/perf-overview.md for tier definitions)
./scripts/perf-snapshot.sh [label]     # take a full bench snapshot → tmp/perf/<label>/
./scripts/perf-compare.sh A B          # benchstat-diff two snapshots
./scripts/perf-profile.sh BenchName    # CPU + alloc profile for one bench
```

**Build output goes to `bin/` only.** Never `go build ./some/pkg` without `-o bin/...` — it dumps a binary in the working directory.

## CLI Reference

Build first: `go build -o bin/gicel ./cmd/gicel/`

### run — compile and execute

```sh
bin/gicel run [flags] <file>.gicel
```

| Flag                 | Default    | Description                                                |
| -------------------- | ---------- | ---------------------------------------------------------- |
| `--packs <packs>`    | header/all | Stdlib packs (see table below); header directive overrides |
| `--module Name=path` | —          | Register user module (repeatable, order matters)           |
| `--recursion`        | off        | Enable `fix`/`rec`                                         |
| `-e <source>`        | —          | Evaluate source string directly                            |
| `--entry <name>`     | `main`     | Entry point binding                                        |
| `--timeout <dur>`    | `5s`       | Execution timeout                                          |
| `--max-steps <n>`    | `100000`   | Step limit                                                 |
| `--max-depth <n>`    | `10000`    | Depth limit                                                |
| `--max-nesting <n>`  | `512`      | Structural nesting depth limit                             |
| `--max-alloc <n>`    | `100 MiB`  | Allocation byte limit                                      |
| `--json`             | off        | Output result as JSON                                      |
| `--explain`          | off        | Show semantic evaluation trace                             |
| `--explain-all`      | off        | Trace stdlib internals (with `--explain`)                  |
| `--verbose`          | off        | Show source context in explain trace                       |
| `--no-color`         | off        | Disable color output                                       |

#### Stdlib packs

| CLI name  | Module name      | Notes    |
| --------- | ---------------- | -------- |
| `prelude` | `Prelude`        |          |
| `fail`    | `Effect.Fail`    |          |
| `state`   | `Effect.State`   |          |
| `io`      | `Effect.IO`      |          |
| `stream`  | `Data.Stream`    |          |
| `slice`   | `Data.Slice`     |          |
| `array`   | `Effect.Array`   |          |
| `ref`     | `Effect.Ref`     |          |
| `map`     | `Data.Map`       |          |
| `set`     | `Data.Set`       |          |
| `json`    | `Data.JSON`      |          |
| `mmap`    | `Effect.Map`     |          |
| `mset`    | `Effect.Set`     |          |
| `session` | `Effect.Session` |          |
| `math`    | `Data.Math`      |          |
| `seq`     | `Data.Sequence`  |          |
| `console` | `Console`        | CLI-only |

### check — type-check only

```sh
bin/gicel check [flags] <file>.gicel
```

Shares `--packs`, `--module`, `--recursion`, `-e`, `--json`, `--timeout`, `--max-nesting` with `run`.

### lsp — language server

```sh
bin/gicel lsp [flags]
```

Starts the LSP server over stdio. Supports diagnostics and hover.

| Flag              | Default | Description        |
| ----------------- | ------- | ------------------ |
| `--packs <packs>` | `all`   | Stdlib packs       |
| `--recursion`     | off     | Enable `fix`/`rec` |

### File header directives

Source files can declare `--packs`, `--module`, and `--recursion` in leading comments:

```gicel
-- gicel: --packs prelude,console
-- gicel: --module Lib=./lib/Lib.gicel
-- gicel: --recursion
import Prelude
import Lib
main := ...
```

Paths resolve relative to the declaring file. Transitive dependencies are discovered recursively. CLI flags override header directives. When `--packs` is not specified on the CLI, the header value is used; if neither is specified, all packs are loaded. Recognized by `run`, `check`, and `lsp`.

### docs / example — reference & examples

```sh
bin/gicel docs                # list topics
bin/gicel docs [topic]        # show topic (e.g., docs about, docs features.effects)
bin/gicel example             # list examples
bin/gicel example [name]      # show example source
```

### Workflow examples

```sh
# Basic single-file run
bin/gicel run hello.gicel

# Type-check only
bin/gicel check hello.gicel

# With specific stdlib packs (skip unused packs for faster compile)
bin/gicel run --packs prelude,state program.gicel

# Recursive definitions (fix/rec)
bin/gicel run --recursion recursive.gicel

# Multi-file project with user modules
bin/gicel run \
  --module Geometry=lib/Geometry.gicel \
  --module Color=lib/Color.gicel \
  main.gicel

# JSON output (for tooling / AI agent integration)
bin/gicel run --json program.gicel
bin/gicel check --json program.gicel

# Evaluation trace (debugging)
bin/gicel run --explain --verbose program.gicel
bin/gicel run --explain --explain-all program.gicel  # include stdlib internals

# Inline eval
bin/gicel run -e 'import Prelude; main := 1 + 2'

# Read from stdin (- as filename)
echo 'import Prelude; main := 1 + 2' | bin/gicel run -
```

## Test Strategy

### Directory layout

```
internal/lang/                         # language definition (syntax, types, ir)
internal/infra/                        # compiler infrastructure (span, budget, diagnostic)
internal/compiler/                     # source → Core IR (parse, check, optimize)
internal/runtime/                      # Core IR execution (eval)
internal/host/                         # Go integration (registry, stdlib)
internal/app/                          # orchestration (engine)
tests/probe/                           # adversarial probe tests (build tag: probe)
tests/stress/                          # stress tests — large inputs, resource boundaries
```

In-package probes: `internal/compiler/check/*_probe_test.go`, `internal/compiler/parse/*_probe_test.go`, `internal/runtime/eval/*_probe_test.go`

### Build tags

- `probe`: requires `//go:build probe`. Not run by `go test ./...`. Run explicitly with `go test -tags probe ./...`.
- `scale`: requires `//go:build scale`. Not run by `go test ./...`. Run explicitly with `go test -tags scale ./...`. O(N) scaling verification.
- `stress`: no tag. Run with `go test ./tests/stress/`.

### Performance benchmarks

See [docs/perf-overview.md](docs/perf-overview.md) for the full bench
map and tier definitions. Quick reference:

- **Tier 1** — `BenchmarkEngineEndToEnd*` / `BenchmarkEndToEnd*`: cold
  end-to-end (compile + run per iter). Reflects CLI wall time. Compile
  share is 97-99% on Map/Set workloads.
- **Tier 2** — `BenchmarkExec*`: pre-compiled, RunWith only. The right
  signal for runtime exec changes — Tier 1 conflates compile noise.
- **Tier 3** — `BenchmarkEngineCompile*` / `BenchmarkEngineNewRuntime*`:
  pure compile / runtime construction. Use for compile-side work.
- **Tier 4** — Per-package micro: parse, check, unify, solve, optimize,
  budget, types. Use for isolated subsystem regression checks.

Workflow for any perf-relevant change:

```sh
./scripts/perf-snapshot.sh main         # baseline (use git rev or label)
# … make changes …
./scripts/perf-snapshot.sh HEAD
./scripts/perf-compare.sh main HEAD     # benchstat-diff all categories
./scripts/perf-profile.sh BenchExecMapInsert50  # deep dive on a hot bench
```

Default snapshot uses `count=5 benchtime=2s` (≈5 min wall time).
Override with `PERF_COUNT=...` and `PERF_BENCHTIME=...`. Snapshots
land in `tmp/perf/<label>/` (gitignored).

### Probe test execution policy

Probe tests (`//go:build probe`) exercise adversarial corner cases and crash reproduction. Intentionally excluded from `go test ./...` to keep the default test suite fast.

```sh
go test -tags probe ./...                        # all probes
go test -tags probe ./internal/compiler/check    # check probes only
go test -tags probe ./internal/compiler/parse    # parse probes only
go test -tags probe ./tests/probe                # integration probes
```

Probes **must** be run before release and after changes to:

- parser recovery logic
- type checker unification/subsumption/evidence
- instance resolution or type family reduction
- evaluator limits and error paths
- sandbox/runtime boundary

### File naming

Both implementation and test files follow the same pattern:

```
<feature>[_<topic>]*.go      — implementation
<feature>[_<topic>]*[_NNNN][_probe]_test.go  — test
```

| Position      | Role                        | Notes                                                   |
| ------------- | --------------------------- | ------------------------------------------------------- |
| `<feature>`   | Primary domain (required)   | `evidence`, `type_family`, `eval`, `lexer`, ...         |
| `[_<topic>]*` | Scope narrowing (0 or more) | `_resolve`, `_reduction`, `_unit`, `_stress`, ...       |
| `[_NNNN]`     | Sequence number (optional)  | When splitting files over 500 lines                     |
| `[_probe]`    | Build qualifier (optional)  | Always adjacent to `_test`. Requires `//go:build probe` |

**Test file names must be adjacent to corresponding implementation files.**
`evidence.go` → `evidence_test.go`, `evidence_resolve_test.go`, `evidence_probe_test.go`

**Test-only files with no corresponding implementation (bench, helpers, fuzz, etc.) use the package name as the feature.**
`internal/compiler/check/bench_test.go` → `internal/compiler/check/check_bench_test.go`

**Consider splitting files over 500 lines.** Sequence numbers `_NNNN` are a last resort when no content-based split name exists.

### File header

New test files should include the following header:

```go
// <Feature> [probe] tests — <scope>.
// Does NOT cover: <related files>.
```

### Deciding where to add tests

1. Check for existing files: `ls <feature>*_test.go`
2. If none found, create `<feature>_test.go`
3. Adversarial tests → `<feature>[_<topic>]*_probe_test.go` (probe tag required)
4. Over 500 lines → split by topic, or use `_NNNN` sequence numbers
