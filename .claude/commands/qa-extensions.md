# Domain Extensions: Gomputation

Go / Gomputation 固有の QA チェック。グローバル `quality-assurance` コマンドから自動的に読み込まれる。

## Go-Specific Signals

各メタパターンの Go 固有シグナルを追加で検出すること:

### Pattern A (Detection–Halt Gap)

- `if err != nil { ... }` block that doesn't `return`, `break`, or skip
- `addError(...)` / `log.Warn(...)` followed by continued execution, not `return`
- Error appended to a list but the invalid entity is still registered/stored

### Pattern B (Semantic Layer Confusion)

- `.String()` / `.Pretty()` / `.Error()` in `==` or `!=` comparisons
- Error message string matching to determine error kind
- Serialized form used as map key where structural identity is needed

### Pattern C (Optimistic Continuation)

- Default values returned on error (zero value, empty struct, sentinel -1)
- `recover()` without re-raise or error conversion
- `_ = err` or error return value ignored
- `_ = someFunc()` or `result, _ := someFunc()` — error discarded

## Go-Specific Checklist Additions

### Error Path Integrity

- Does the `(result, error)` contract hold? When `err != nil`, is `result` safe to use?

### Semantic vs Representational Operations

Search for `.String()`, `.Pretty()`, `.Error()`, `.Format()` used in:
- `==` / `!=` comparisons
- Map keys
- Switch/case dispatch

Flag any use where structural/semantic comparison should be used instead.

### Silent Failure Detection

- `_ = someFunc()` or `result, _ := someFunc()` — error discarded
- `default:` / `else` branches returning zero values without error
- `recover()` without logging or error propagation
- Functions that return `(value, error)` where some paths return `(zeroValue, nil)` on failure

## Type System Domain Checks

When reviewing type checker code (`internal/check/`), additionally apply:

- **Unifier safety:** Classify each `Unify` call as committed (result always wanted), trial (needs save/restore), or semantic (needs `addSemanticUnifyError`). Report trial calls without rollback.
- **Instance coherence:** Overlap/self-cycle detection must return nil (prevent registration), not merely report.
- **Evidence integrity:** Dictionary elaboration must use structural type comparison (`types.Equal`), never `types.Pretty`.
- **Error code coverage:** Cross-reference `internal/errs/error.go` codes against test files. List untested codes.
