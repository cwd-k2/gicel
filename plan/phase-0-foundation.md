# Phase 0: Foundation

## Objective

Go module initialization, directory scaffold, and shared primitives that every subsequent phase depends on.

## Dependencies

None.

## Scope

### 0.1 Go Module

```
go mod init github.com/cwd-k2/gomputation
```

Go 1.23+. No external dependencies at this stage.

### 0.2 Directory Structure

Go standard layout を採用。`pkg/` = 公開サブパッケージ、`internal/` = 非公開実装。

```
gomputation/
├── go.mod
├── gomputation.go              ← root package: Engine, Runtime, Value, CapEnv, PrimImpl
│
├── pkg/                         ── public sub-packages ──
│   └── types/                  ← Phase 1: Type, Kind, Row, type builders
│
├── cmd/                         ── executables ──
│   └── gpc/                    ← future: REPL, formatter, linter
│
├── internal/                    ── private implementation ──
│   ├── span/                   ← Phase 0: source positions (Pos, Span, Source)
│   ├── errs/                   ← Phase 0: structured diagnostics (Error, Errors)
│   ├── core/                   ← Phase 2: Core IR (13 formers)
│   ├── eval/                   ← Phase 3: Evaluator
│   ├── syntax/                 ← Phase 4: Lexer, Parser (Pratt)
│   ├── check/                  ← Phase 5: Type checker + elaboration
│   └── prelude/                ← Phase 7: Bool, Unit, Result, Effect
│
├── testdata/                   ← test fixtures (.gpc source files)
├── examples/                   ← host-side usage examples (Go programs)
├── docs/                       ← research documents
├── plan/                       ← implementation plan
└── spec.draft.v0_3.md
```

**Rationale**:

- **root package** に Engine, Runtime, Value, CapEnv, PrimImpl を集約。ホスト開発者は `import "github.com/cwd-k2/gomputation"` だけで基本操作が完結する。Value 等の型は `internal/eval` から type alias で公開（`type Value = eval.Value`）。
- **`pkg/types/`** は `DeclareBinding` で型を構築するための独立した関心。Engine 設定フェーズ固有の語彙であり、実行フェーズとはライフサイクルが異なるため分離。
- **`internal/`** にコンパイラパイプライン全体を格納。外部からインポート不可。内部パッケージ間は自由にインポートできる。
- **`internal/prelude/`** は v0 では固定定義。将来モジュールシステム導入時に prelude 自体をモジュールとして扱う設計に移行予定（→ `pkg/` に昇格、またはモジュールローダー経由で提供）。

### 0.3 Source Position (`internal/span/`)

Every AST node, Core IR node, type, and error message carries source location. This must exist before any other package.

#### `internal/span/span.go`

```go
package span

// Pos is a byte offset into the source.
type Pos int

// Span is a half-open range [Start, End) in the source.
type Span struct {
    Start Pos
    End   Pos
}

// Source maps byte offsets to line/column for diagnostics.
type Source struct {
    Name  string // file name or "<input>"
    Text  string // full source text
    Lines []Pos  // byte offset of each line start
}

// NewSource builds line offset table.
func NewSource(name, text string) *Source

// Location returns human-readable line:col for a Pos.
func (s *Source) Location(p Pos) (line, col int)

// Excerpt returns the source text for a Span.
func (s *Source) Excerpt(sp Span) string
```

**Design note**: `Span` is a value type (no pointer). It embeds cheaply in AST/Core/Type nodes. `Source` is shared (one per parse unit), referenced by pointer.

### 0.4 Error Infrastructure (`internal/errs/`)

Structured error type shared across all phases. Follows Rust-style diagnostic model.

#### `internal/errs/error.go`

```go
package errs

import "github.com/cwd-k2/gomputation/internal/span"

// Code is a numeric error identifier (E0001–E9999).
type Code int

// Phase indicates which compiler stage produced the error.
type Phase int

const (
    PhaseLex   Phase = iota
    PhaseParse
    PhaseCheck
    PhaseEval
)

// Error is a structured diagnostic.
type Error struct {
    Code    Code
    Phase   Phase
    Span    span.Span
    Message string
    Hints   []Hint
}

func (e *Error) Error() string

// Hint is a secondary annotation on a span.
type Hint struct {
    Span    span.Span
    Message string
}

// Errors collects multiple diagnostics.
type Errors struct {
    Source *span.Source
    Errs   []*Error
}

func (es *Errors) Add(e *Error)
func (es *Errors) HasErrors() bool

// Format renders all errors for human consumption.
func (es *Errors) Format() string
```

**Design note**: Errors are collected, not panicked. Each phase appends to `*Errors`. The top-level engine decides when to stop (first error, or continue for more diagnostics).

## Test Strategy

- `span`: table-driven tests for `Location` and `Excerpt` with multi-line inputs, Unicode, empty input.
- `errs`: test `Format` output against golden strings.

## Completion Criteria

- [ ] `go mod init` succeeds
- [ ] All directories exist (empty `package.go` placeholder in each)
- [ ] `internal/span` compiles with tests passing
- [ ] `internal/errs` compiles with tests passing
- [ ] `go vet ./...` clean
