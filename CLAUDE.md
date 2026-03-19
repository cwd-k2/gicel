# GICEL

## Commands

```sh
go test ./...                          # test
go build ./...                         # compile check (no output)
go build -o bin/gicel ./cmd/gicel/     # build CLI binary to bin/
go run ./examples/go/<name>/           # run Go example (no binary)
goimports -w .                         # format Go
prettier --write docs/           # format docs
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

## Test Strategy

### Directory layout

```
internal/*/_test.go              # unit tests — パッケージ内部の関数・型を直接テスト
internal/engine/*_test.go        # integration tests — Engine/Runtime 経由の end-to-end
tests/probe/                     # adversarial probe tests (build tag: probe)
tests/stress/                    # stress tests — 大規模入力・リソース境界
```

パッケージ内 probe: `internal/check/*_probe_test.go`, `internal/syntax/parse/*_probe_test.go`, `internal/eval/*_probe_test.go`

### Build tags

- `probe`: `//go:build probe` 付き。`go test ./...` では実行されない。`go test -tags probe ./...` で明示実行。
- `stress`: タグなし。`go test ./tests/stress/` で実行。

### File naming

実装・テストとも同じ規則に従う:

```
<feature>[_<topic>]*.go      — 実装
<feature>[_<topic>]*[_NNNN][_probe]_test.go  — テスト
```

| Position      | Role                       | Notes                                             |
| ------------- | -------------------------- | ------------------------------------------------- |
| `<feature>`   | 主ドメイン (必須)          | `evidence`, `type_family`, `eval`, `lexer`, ...   |
| `[_<topic>]*` | スコープ絞り込み (0個以上) | `_resolve`, `_reduction`, `_unit`, `_stress`, ... |
| `[_NNNN]`     | 連番 (optional)            | 500行超で分割が必要なとき: `_0001`, `_0002`, ...  |
| `[_probe]`    | ビルド修飾子 (optional)    | `_test` と常に隣接。`//go:build probe` 付き       |

**テストファイル名は対応する実装ファイル名に近接させる。**
`evidence.go` → `evidence_test.go`, `evidence_resolve_test.go`, `evidence_probe_test.go`

**500行を超えるファイルは分割を検討する。** 連番 `_NNNN` は内容による分割名が見つからないときの最終手段。

例:

```
evidence_test.go                    — evidence.go の標準テスト
evidence_resolve_test.go            — resolve.go のテスト (evidence feature)
evidence_sort_stress_test.go        — evidence sort のストレステスト
evidence_probe_test.go              — evidence adversarial (probe tag)
type_family_reduction_unit_test.go  — type_family.go の reduction algorithm unit
```

**tests/probe/ ディレクトリ**: 全ファイルが probe なので `_probe` 不要。feature 名のみ。

### File header (必須)

```go
// <Feature> [probe] tests — <scope>.
// Does NOT cover: <related files>.
```

### テスト追加先の決定

1. `ls <feature>*_test.go` で関連ファイルを確認
2. 該当ファイルが無ければ `<feature>_test.go` を作成
3. Adversarial → `<feature>[_<topic>]*_probe_test.go` (probe tag 必須)
4. 500行超 → topic による分割、または `_NNNN` 連番

## Rules

- Build output goes to `bin/` (gitignored).
- Format Go with `goimports`, docs with `prettier`.
- Commit per logical group or phase completion.
- Do not run test agents in background (memory exhaustion incident, 2024-03-14).
- **一つのことには一つのやり方。** 同じ操作・同じパターンに複数の実装経路を作らない。共通ロジックが複数箇所に現れたら、代表となるヘルパーに統合する。分岐が必要なのは意味論的に異なる場合だけ。コードベースの divergence は設計意図を曖昧にし、変更コストを増幅する。
- **ハックもワークアラウンドも入れない。理論に従う。** 表示用 API（`Pretty`, `String`）を identity や cache key に流用しない。名前規約やテキスト形状から意味を推定するヒューリスティックを作らない。構造的なデータで意味を表現し、文字列エンコーディングが必要なら単射性を保証する canonical serializer を一つだけ用意する。妥協が必要な場合は、暗黙に劣化させるのではなく、制約と理由を文書化して明示的に境界を引く。
