# GICEL

## Commands

```sh
go test ./...                          # test
go build ./...                         # compile check (no output)
go build -o bin/gicel ./cmd/gicel/     # build CLI binary to bin/
go run ./examples/go/<name>/           # run Go example (no binary)
goimports -w .                         # format Go
prettier --write docs/ spec/           # format docs
```

**Build output goes to `bin/` only.** Never `go build ./some/pkg` without `-o bin/...` — it dumps a binary in the working directory.

## CLI Reference

Build first: `go build -o bin/gicel ./cmd/gicel/`

### run — compile and execute

```sh
bin/gicel run [flags] <file>.gicel
```

| Flag                 | Default  | Description                                                |
| -------------------- | -------- | ---------------------------------------------------------- |
| `--use <packs>`      | `all`    | Stdlib packs: `prelude,fail,state,io,stream,slice,map,set` |
| `--module Name=path` | —        | Register user module (repeatable, order matters)           |
| `--recursion`        | off      | Enable `fix`/`rec`                                         |
| `-e <source>`        | —        | Evaluate source string directly                            |
| `--entry <name>`     | `main`   | Entry point binding                                        |
| `--timeout <dur>`    | `5s`     | Execution timeout                                          |
| `--max-steps <n>`    | `100000` | Step limit                                                 |
| `--max-depth <n>`    | `100`    | Depth limit                                                |
| `--max-alloc <n>`    | `100MiB` | Allocation byte limit                                      |
| `--json`             | off      | Output result as JSON                                      |
| `--explain`          | off      | Show semantic evaluation trace                             |
| `--explain-all`      | off      | Trace stdlib internals (with `--explain`)                  |
| `--verbose`          | off      | Show source context in explain trace                       |
| `--no-color`         | off      | Disable color output                                       |

### check — type-check only

```sh
bin/gicel check [flags] <file>.gicel
```

Shares `--use`, `--module`, `--recursion`, `-e`, `--json` with `run`.

### docs / example — reference & examples

```sh
bin/gicel docs [topic]        # show language reference
bin/gicel example [name]      # show example programs
```

### Workflow examples

```sh
# Basic single-file run
bin/gicel run hello.gicel

# Type-check only
bin/gicel check hello.gicel

# With specific stdlib packs (skip unused packs for faster compile)
bin/gicel run --use prelude,state program.gicel

# Recursive definitions (fix/rec)
bin/gicel run --recursion recursive.gicel

# Multi-file project with user modules
bin/gicel run \
  --module Geometry=lib/Geometry.gicel \
  --module Color=lib/Color.gicel \
  main.gicel

# Check multi-file project
bin/gicel check \
  --module Geometry=lib/Geometry.gicel \
  main.gicel

# Custom entry point and limits
bin/gicel run --entry myMain --max-steps 500000 --timeout 10s program.gicel

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

### Working multi-module example

```sh
cd examples/cli/multi-module
../../../bin/gicel run \
  --module Geometry=Geometry.gicel \
  --module Color=Color.gicel \
  --module MathLib=MathLib.gicel \
  main.gicel
# → (3, "red", 6)
```

## Rules

- Build output goes to `bin/` (gitignored).
- Format Go with `goimports`, docs with `prettier`.
- Commit per logical group or phase completion.
- Do not run test agents in background (memory exhaustion incident, 2024-03-14).
