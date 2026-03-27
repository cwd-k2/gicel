# Library & Tooling Roadmap

GICEL のライブラリ、CLI、ツーリング、ホスト API の方向性。

## Stdlib

### 現状

12 packs: Prelude, Effect.Fail, Effect.State, Effect.IO, Data.Stream, Data.Slice, Effect.Array, Data.Map, Data.Set, Effect.Map, Effect.Set, Console

### 方向性

- **Prelude**: コア機能は十分。型クラスインスタンスの拡充（Tuple Eq/Ord runtime support 等）はユーザ需要に応じて
- **Data.Map / Data.Set**: 基本 CRUD + fold + 集合演算 (union/intersection/difference) は整備済み。更なる ergonomic expansion は需要次第
- **Effect.IO**: print/debug は capEnv.io バッファへの純粋操作。stdout/stderr 直接出力は意図的に提供しない（副作用は host 側の責務）

### 拡充候補

| 候補                                | 分類     | 前提                           |
| ----------------------------------- | -------- | ------------------------------ |
| Tuple Eq/Ord runtime support        | Prelude  | ユーザ需要                     |
| Data.Map: mapKeys, intersectionWith | Data.Map | ユーザ需要                     |
| Data.Text (UTF-8 aware operations)  | 新 pack  | 文字列処理ユースケースの拡大時 |
| Shell (外部プロセス実行)            | 新 pack  | Label kind 言語拡張。[設計書](shell.md) |

## CLI

### 現状

4 commands: run, check, docs, example。`--packs`, `--module`, `--explain`, `--json` 等の主要フラグは整備済み。

### 方向性

| 項目                         | 説明                                                    | 優先度                                                                |
| ---------------------------- | ------------------------------------------------------- | --------------------------------------------------------------------- |
| multi-module workflow        | manifest/directory discovery で `--module` 列挙を不要に | 低（主用途がワンショット sandbox のため `--module` 列挙で十分足りる） |
| spec/grammar in `gicel docs` | 仕様書・文法参照を CLI から到達可能に                   | 中（バイナリサイズとのトレードオフ）                                  |
| `--packs` default 見直し     | `all` → `prelude` で least-privilege 寄りに             | 低（UX trade-off）                                                    |

## Diagnostics

### 現状

compile diagnostics は code + phase + line/col + message + hints。runtime error は line/col 付き（RuntimeError 型の場合）。JSON 契約は preflight/compile/runtime で一貫。explain は failure path でも flush される。

### 方向性

| 項目                              | 説明                                                            | 優先度                          |
| --------------------------------- | --------------------------------------------------------------- | ------------------------------- |
| Diagnostics Phase B–E             | suggestion, enhanced span display, terminology normalization    | 増分対応可能                    |
| compile-error JSON source snippet | JSON に該当行テキストを含める                                   | 中                              |
| runtime error structured codes    | RuntimeError に error code 体系を追加                           | 中（現状は message 文字列のみ） |
| resource-limit tuning guidance    | limit 調整の導線強化（flag table 追加済み、dry-run は別アーキ） | 低                              |

## Host API

### 現状

三層ライフサイクル: Engine (mutable) → Runtime (immutable, goroutine-safe) → Evaluator (per-execution)。RunSandbox は parent context + explain hooks を持つ convenience API。

### 方向性

| 項目                       | 説明                                      | 優先度                                     |
| -------------------------- | ----------------------------------------- | ------------------------------------------ |
| RunSandbox with Trace hook | 低レベル trace を SandboxConfig に追加    | 低（Explain で大半のユースケースをカバー） |
| compile cache guidance     | Engine/Runtime 再利用パターンの docs 強化 | 対応済み（go-api.md Migration Path）       |
| host embedding examples    | 実サービス統合の参考例                    | 中                                         |

## Explain / Trace

### 現状

`--explain`: semantic trace (binds, effects, matches, sections)。`--explain-all`: stdlib internals 含む（dim 表示で区別）。`--verbose`: module transition 表示。failure path でも flush。JSON mode では error output に explain steps を含む。

### 方向性

| 項目                           | 説明                                             | 優先度 |
| ------------------------------ | ------------------------------------------------ | ------ |
| `--verbose` 前後行コンテキスト | 該当行だけでなく前後N行を表示                    | 中     |
| explain step filtering         | kind/source でフィルタリング（例: effects のみ） | 低     |

## Security Boundary

### 現状

capability model + `RegisterPrim` は TCB。docs に trust boundary 説明あり。

### 方向性

- pack naming 三層化（CLI `state` / Go `EffectState` / source `Effect.State`）の統一は大きな変更。docs の対照表で対応中
- `--packs all` default → 学習には便利だが least-privilege とは緊張。リネーム済み (`--use` → `--packs`) で「制限」の意味を強化

## Performance Tracking

既存ベンチマークで主要ホットスポットを追跡中。最適化は需要駆動。

| ホットスポット                   | ベンチマーク                | 特性                                            |
| -------------------------------- | --------------------------- | ----------------------------------------------- |
| check instance resolution        | `check_bench_scale_test.go` | compile 時最大コスト (7.6ms/op @ 100 instances) |
| E2E compile cost                 | `engine_bench_test.go`      | module 数に比例 (61.6ms/op)                     |
| runtime value/closure allocation | `eval_bench_test.go`        | lookup でなく構築が支配的                       |
| optimizer traversal              | `optimize_bench_test.go`    | no-op でも固定費あり                            |
| parse throughput                 | `parse_bench_test.go`       | program scale に対する線形性を確認              |
| budget overhead                  | `budget_bench_test.go`      | per-call ~5ns (Step), ~0.4ns (Enter/Leave)      |
