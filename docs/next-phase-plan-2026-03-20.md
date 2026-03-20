# Next-Phase Architecture Plan

Engine 再編 (v0.13) の成果を踏まえた、次期アーキテクチャ改善の計画。

## Engine 再編の成果

Engine は `{host HostEnv, store ModuleStore, limits Limits}` の 3 fields に。23 public methods は全て 1–5 行の delegation。compilation pipeline は standalone 関数化 (`lexAndParse`, `makeCheckConfig`, `compileModule`, `compileMain`, `assembleRuntime`)。check への入口は `makeCheckConfig → check.Check/CheckModule` という明確な seam。

## 優先度判定

### v0.14 (row-level type families) の critical path

Row-level TF が触る箇所を severity 順に:

| Severity | 箇所 | 内容 |
|---|---|---|
| **High** | `check/unify/row_unify.go` | `resolveEvidenceTails` が `TyFamilyApp` tail を想定していない |
| **High** | `check/solver.go` | stuck row-family の reactivation に result-meta 登録が必要 |
| **Medium** | `check/resolve_type.go` | row tail に型式を許す (現在は変数のみ) |
| **Medium** | `syntax/parse` | row type syntax で tail に type expression を許す |
| **Low** | `check/family/reduce.go` | `MapType` の `TyEvidenceRow` 走査確認 |
| **Low** | `check/inert.go` | `CtFunEq` に `ResultKind` field 追加 |

**Critical path 結論**: row-level TF は check の `unify → solver → resolve_type` 軸に集中。parse は row tail syntax の小変更のみ。check の state ownership 整理は row-level TF と直交するが、solver 周辺のコード理解を容易にする前提条件として価値がある。

### check の密結合が次に何をブロックするか

Checker (14 fields, 161 methods, 31 files) の密結合問題:

1. **solver state が Checker 直轄** — worklist/inertSet/ambiguityCache が Checker の fields。solver 改善 (v0.13 performance items) と row-level TF (v0.14) が同じ fields を触る → 変更の衝突リスク
2. **scope/import が Checker に散在** — 15 methods が import resolution を担当。module boundary hardening (v0.13) が checker 内部を直接操作する必要がある
3. **family → solver → unifier の callback chain** — `OnSolve`, `FamilyReducer`, `registerStuckViaInert` が 3 パッケージを横断する callback で結合。row-level TF で新たな callback (result-meta reactivation) を追加する際に、この chain の理解が必要

**結論**: check の全面再編は v0.14 の前提条件ではない。しかし solver state の分離は row-level TF の作業を低リスクにする。

### parse の recovery 改善は独立して進められるか

**Yes — 完全に独立**。parse は `syntax`, `errs`, `span` のみに依存。独自の resource limits を持ち、budget とは無関係。改善項目 (error code 分化, expression-level recovery, separator 統合) は check/engine への影響ゼロ。

## 計画

4 フェーズを順に実施。各フェーズは独立してコンパイル・テスト通過する。

### Phase A: Parse Diagnostics Hardening

**目的**: v0.14 の型エラー診断改善の前提として、フロントエンド品質を底上げ。

**scope**: roadmap v0.13 の Parser & Diagnostics Hardening 項目。

| Item | 内容 | 工数 |
|---|---|---|
| A1 | E0100 → E0101–E0110 分化 | 小 |
| A2 | Expression-level recovery (`ErrorExpr` node) | 中 |
| A3 | Newline separator 統合 | 小 |
| A4 | `main :=` silent acceptance の修正 | 小 |

**独立性**: 完全独立。check/engine への変更なし。

**検証**: `go test ./internal/syntax/parse/...` + smoke test。

### Phase B: Solver State Extraction

**目的**: Checker から solver state を分離し、row-level TF 実装の準備を整える。

**方針**: `check` 内部で型による責務分離 (engine 再編と同じパターン)。パッケージ分割しない。

```
Checker struct {
    session  CheckSession     // source, diagnostics, budget, config
    scope    ScopeState       // imports, qualification, ownership
    registry SemanticRegistry // constructors, aliases, classes, instances, families
    solver   SolverState      // worklist, inertSet, ambiguityCache, depth, level
    unifier  *unify.Unifier   // unchanged — already well-owned
    ...elaboration fields...
}
```

| Step | 内容 | リスク |
|---|---|---|
| B1 | `CheckSession` — source, errors, budget, config, cancelled, freshID を集約 | 低 |
| B2 | `ScopeState` — `checkerScope` を型として明示化 (既存の nested struct をそのまま promote) | 低 |
| B3 | `SemanticRegistry` — `checkerRegistry` を型として明示化 (同上) | 低 |
| B4 | `SolverState` — worklist, inertSet, ambiguityCache, depth, resolveDepth, level を集約 | 中 |
| B5 | solver methods を `SolverState` のメソッドに移動 (solver.go, deferred.go) | 中 |

**注意**: B4/B5 は solver methods が Checker の他の fields (unifier, registry, scope) を参照するため、receiver 変更だけでは済まない。method 引数として必要な context を渡すか、SolverState に必要な参照を持たせる。後者のほうが callback chain との整合性が良い。

**独立性**: engine API 変更なし。check の public API 変更なし。internal method signature のみ変更。

**検証**: `go test ./internal/check/...` + `go test ./internal/engine/...`。

### Phase C: Row-Level Type Families (v0.14 core)

**目的**: v0.14 の本体実装。Phase B で solver state が分離されていることを前提とする。

| Step | 内容 | 箇所 |
|---|---|---|
| C1 | Row tail に type expression を許す (parser + AST) | `syntax/`, `parse/parse_type.go` |
| C2 | `resolveTypeExpr` で row tail の `TyFamilyApp` を処理 | `check/resolve_type.go` |
| C3 | `resolveEvidenceTails` で stuck `TyFamilyApp` tail を処理 | `check/unify/row_unify.go` |
| C4 | Stuck row-family の result-meta 登録 | `check/solver.go`, `check/type_family.go` |
| C5 | `CtFunEq` に `ResultKind` field 追加 (optional, for diagnostics) | `check/constraint.go`, `check/inert.go` |
| C6 | `MapType` の `TyEvidenceRow` 走査確認 | `types/map.go` |

**前提条件**: Phase B (SolverState 分離) が完了していること。C4 で solver state を操作する際に、state ownership が明確であることが作業効率に直結する。

**検証**: 専用テストスイート + `go test ./...` + smoke test。

### Phase D: Budget 統一 (optional)

**目的**: parse の独自 resource limits を budget に寄せ、パイプライン全体の防御を統一。

**scope**: parse の `recurseDepth`/`maxRecurseDepth` と `steps`/`maxSteps` を `budget.Budget` 経由に変更。

| Step | 内容 |
|---|---|
| D1 | `budget.Budget` に `Recurse()`/`Unrecurse()` を追加 (Nest/Unnest の alias でも可) |
| D2 | `parse.NewParser` が `*budget.Budget` を受け取るように変更 |
| D3 | `enterRecurse`/`leaveRecurse` を `budget.Recurse()`/`Unrecurse()` に委譲 |
| D4 | `advance()` 内の step check を `budget.Step()` に委譲 |
| D5 | `lexAndParse` (engine/pipeline.go) で budget を構築して Parser に渡す |

**リスク**: 低。parse の internal defense mechanism の実装変更のみ。外部 API への影響はパラメータ追加のみ。

**独立性**: Phase A–C とは独立。どのタイミングでも実施可能。

**判断基準**: parse の limits を engine の `Limits` から設定可能にする実益があるかどうか。現状の hardcoded defaults (recurse=256, steps=tokens×4) で十分なら、この phase は defer してよい。

## 実施順序

```
Phase A (parse diagnostics) ──┐
                               ├── 並行可能
Phase B (solver extraction) ──┘
         │
         ▼
Phase C (row-level TF) ── v0.14 release
         │
         ▼
Phase D (budget unify) ── optional, any time
```

Phase A と B は並行可能。C は B 完了後。D は独立。

## 共有語彙層の命名について

現状の `types`/`core`/`syntax`/`span` は Layer 0–2 の shared vocabulary として適切に機能している。

```
Layer 0: span (62 lines) — 全ノードの位置情報
Layer 1: types (2290 lines) — 型代数, kind, evidence row
Layer 2: core (955 lines), syntax (860 lines) — IR と AST
```

**命名変更の必要性**: 低い。現在の名前は Go コミュニティの慣習に沿っており、外部から読みやすい。`docs/architecture-reorg-conclusion` が提案する `domain/types`, `domain/core` 等のネスト構造は、パッケージ分割の恩恵より import path の長大化のコストが大きい。v1.0 までは現状維持。

## リスク管理

- **Phase B の最大リスク**: solver methods が Checker の多くの fields を参照するため、SolverState への分離が「引数の爆発」を招く可能性。対策: SolverState に HostContext (unifier + registry への read-only reference) を持たせる。
- **Phase C の最大リスク**: row tail の `TyFamilyApp` が既存のユーザーコードで意図しない型推論結果を生む可能性。対策: 新機能は feature gate で段階的に有効化。
- **全フェーズ共通**: check の 161 methods × 31 files は refactoring error の温床。対策: engine テスト (57 smoke cases, 1066-line module tests, 23 host API tests) が regression guard として機能。
