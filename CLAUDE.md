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

| Flag                 | Default   | Description                                                |
| -------------------- | --------- | ---------------------------------------------------------- |
| `--use <packs>`      | `all`     | Stdlib packs: `prelude,fail,state,io,stream,slice,map,set` |
| `--module Name=path` | —         | Register user module (repeatable, order matters)           |
| `--recursion`        | off       | Enable `fix`/`rec`                                         |
| `-e <source>`        | —         | Evaluate source string directly                            |
| `--entry <name>`     | `main`    | Entry point binding                                        |
| `--timeout <dur>`    | `5s`      | Execution timeout                                          |
| `--max-steps <n>`    | `100000`  | Step limit                                                 |
| `--max-depth <n>`    | `100`     | Depth limit                                                |
| `--max-alloc <n>`    | `100 MiB` | Allocation byte limit                                      |
| `--json`             | off       | Output result as JSON                                      |
| `--explain`          | off       | Show semantic evaluation trace                             |
| `--explain-all`      | off       | Trace stdlib internals (with `--explain`)                  |
| `--verbose`          | off       | Show source context in explain trace                       |
| `--no-color`         | off       | Disable color output                                       |

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

## Test Directory Strategy

```
internal/engine/*_test.go        # integration tests — Engine/Runtime 経由の end-to-end
internal/*/_test.go              # unit tests — パッケージ内部の関数・型を直接テスト
tests/probe/                     # adversarial probe tests (build tag: probe)
tests/stress/                    # stress tests — 大規模入力・リソース境界
internal/check/*_probe_test.go   # checker probe tests (build tag: probe)
internal/syntax/parse/*_probe_test.go  # parser probe tests (build tag: probe)
```

**Build tags:**
- `probe` テストは `//go:build probe` 付き。`go test ./...` では実行されない。`go test -tags probe ./...` で明示実行。
- `stress` テストはタグなし。`go test ./tests/stress/` で実行。

**配置ルール:**
- パッケージ内部のロジックをテストする場合 → `internal/*/` に配置
- Engine/Runtime 経由の統合テスト → `internal/engine/` に配置
- 敵対的入力・境界探索（probe） → `tests/probe/` または対象パッケージの `*_probe_test.go`（build tag 必須）
- 負荷・大規模テスト（stress） → `tests/stress/`

## Test Naming Convention

Pattern: `<feature>[_probe][_<topic>][_<tier>]_test.go`

左から右にスコープが狭まる:
- **feature** (必須): テスト対象 (`evidence`, `type_family`, `unify`, `hkt`, ...)
- **probe** (optional): adversarial boundary testing (`//go:build probe`)
- **topic** (optional): feature 内の焦点 (`_resolve`, `_reduction`, `_kind`, ...)
- **tier** (optional): テスト手法 (`_stress`, `_unit`, `_pathological`, `_interaction`)

例:
```
evidence_test.go               — evidence 標準テスト
evidence_probe_test.go         — evidence adversarial (probe tag)
evidence_resolve_test.go       — evidence 解決ロジック
evidence_sort_stress_test.go   — evidence sort stress
type_family_reduction_unit_test.go — TF reduction algorithm unit
```

**テストの追加先:**
1. `ls <feature>_*` で関連ファイルを確認
2. 該当ファイルが無ければ `<feature>_test.go` を作成 (header comment 必須)
3. Adversarial → `<feature>_probe[_<topic>]_test.go` (probe tag)
4. Unit (unexported) → `<feature>_<topic>_unit_test.go`
5. Stress → `<feature>[_<topic>]_stress_test.go`

**tests/probe/ ディレクトリ**: 全ファイルが probe なので `_probe` marker 不要。feature 名のみ。

**File header** (必須):
```go
// <Feature> [probe] tests — <scope>.
// Does NOT cover: <related files>.
```

## Rules

- Build output goes to `bin/` (gitignored).
- Format Go with `goimports`, docs with `prettier`.
- Commit per logical group or phase completion.
- Do not run test agents in background (memory exhaustion incident, 2024-03-14).
