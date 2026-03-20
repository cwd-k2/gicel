# Internal Coupling Review 2026-03-20

対象:

- `internal/*` 全体

目的:

- 各モジュールの内部依存関係とロジック密結合度をざっくり比較する
- 変更波及が大きそうな層を特定する

確認方法:

- パッケージごとの直下ファイル数
- レシーバーメソッド数の偏り
- 中央状態オブジェクトの有無
- 実装責務の凝集度と shared mutable state の強さ

## 生の計測

### パッケージ別ファイル数

- `check`: 95
- `engine`: 31
- `types`: 21
- `eval`: 20
- `stdlib`: 17
- `core`: 9
- `syntax`: 6
- `opt`: 3
- `budget`: 2
- `errs`: 2
- `span`: 2
- `reg`: 1

### レシーバーメソッド数 上位

- `Checker`: 161
- `Parser`: 88
- `Unifier`: 30
- `Engine`: 23
- `Budget`: 19
- `Lexer`: 11
- `InertSet`: 11
- `CheckEnv`: 11
- `ReduceEnv`: 10
- `ExplainObserver`: 9
- `Worklist`: 8
- `Evaluator`: 8
- `Errors`: 8
- `Runtime`: 7
- `Env`: 7
- `Context`: 7

## 総論

密結合の強い順に見ると、おおむねこうです。

1. `check`
2. `engine`
3. `eval`
4. `types` / `stdlib`
5. `core`
6. `budget` / `errs` / `span` / `reg`

`parse` については別メモで扱った通り、中程度の結合です。異常系 recovery はやや密ですが、全体としては parser の責務範囲に収まっています。

## 各モジュール

### `check`

判定:

- 結合度: 非常に高い

根拠:

- 95 ファイル
- `Checker` メソッド 161
- 中央状態 `Checker` が巨大: [`internal/check/checker.go:115`]( /Users/cwd-k2/Projects/gicel/internal/check/checker.go#L115 )

抱えているもの:

- typing context
- unifier
- diagnostics
- import/module scope
- constraint solver state
- family reduction state
- registry
- budget / cancellation

所見:

- ほぼ「型検査 OS」です。
- 機能追加の自由度は高いが、局所変更の影響範囲は読みにくい。
- import, export, solving, elaboration, kinding が同じ中心状態を共有しているのが本質的な密結合です。

### `engine`

判定:

- 結合度: 高

根拠:

- 31 ファイル
- `Engine` メソッド 23
- `Engine` が compile pipeline の司令塔になっている: [`internal/engine/engine.go:30`]( /Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L30 )

所見:

- `engine` は自前で複雑なアルゴリズムを持つというより、`parse` / `check` / `opt` / `eval` / `stdlib` を束ねる統合境界です。
- そのため coupling の質は「横断依存」です。
- `Engine` が持つ state は比較的理解しやすいですが、公開 API と内部 subsystems の接着がここに集中しています。
- 特に module registration, runtime assembly, source ownership, diagnostics で変更波及が起きやすいです。

### `eval`

判定:

- 結合度: 中高

根拠:

- 20 ファイル
- `Evaluator` 8 メソッド、`Env` 7、`ExplainObserver` 9
- `Evaluator` が実行、trace、budget、capability threading をまとめて扱う: [`internal/eval/eval.go:36`]( /Users/cwd-k2/Projects/gicel/internal/eval/eval.go#L36 )

所見:

- `eval` は状態が中央集約されていますが、`check` ほど責務が散っていません。
- 中心課題は一貫していて、「Core をどう走らせるか」です。
- ただし以下が同一 execution path に入っているため、変更時は注意が必要です。

- evaluation semantics
- CapEnv threading
- explain / trace instrumentation
- budget charging
- trampoline / bounce

つまり `eval` は「意味的凝集は高いが、横機能が同じ loop に乗っている」タイプの結合です。

### `types`

判定:

- 結合度: 中

根拠:

- 21 ファイル
- 中央抽象は `Type` hierarchy: [`internal/types/type.go:9`]( /Users/cwd-k2/Projects/gicel/internal/types/type.go#L9 )

所見:

- `types` は shared model layer です。
- 多数の他モジュールが依存しますが、内部は比較的素直です。
- 結合は強いというより「基盤依存が集中している」形です。
- ここを変えると `check`, `core`, `engine` に広く波及するので、変更コストは高いです。

### `stdlib`

判定:

- 結合度: 中

根拠:

- 17 ファイル
- pack 単位で分かれているが、共通コストモデルや helper を共有する: [`internal/stdlib/stdlib.go:1`]( /Users/cwd-k2/Projects/gicel/internal/stdlib/stdlib.go#L1 )

所見:

- `stdlib` は pack ごとにそこそこ分離されています。
- ただし `eval.Value`, `CapEnv`, allocation charging, embedded source module への依存が共通なので、完全独立ではありません。
- coupling の中心は「データ表現共有」です。algorithmic coupling は低めです。

### `core`

判定:

- 結合度: 低中

根拠:

- 9 ファイル
- 中核は AST/IR 定義: [`internal/core/core.go:8`]( /Users/cwd-k2/Projects/gicel/internal/core/core.go#L8 )

所見:

- `core` は IR 定義と補助処理に責務がまとまっています。
- 他モジュールからの依存は強いですが、内部は比較的フラットです。
- 注意点は panic ベースの totality 確保で、node 種別追加時の更新漏れが起こると壊れやすいことです。

### `budget`

判定:

- 結合度: 低

所見:

- 明確に独立した utility module です。
- API の意味も狭いです。
- `eval` / `check` から使われますが、内部凝集は高く、モジュール境界も明瞭です。

### `errs`

判定:

- 結合度: 低

所見:

- shared diagnostics formatting layer。
- `span.Source` への依存はありますが、責務はほぼ一つです。
- 内部結合は小さいです。

### `span`

判定:

- 結合度: 低

所見:

- 値オブジェクトと位置解決のみ。
- 非常に素直です。

### `reg`

判定:

- 結合度: 低

所見:

- interface / protocol layer です。
- 内部ロジックほぼなし。

## 密結合の質

### `check`

- shared mutable state coupling
- feature interaction coupling
- phase coupling

### `engine`

- orchestration coupling
- subsystem boundary coupling

### `eval`

- execution-path coupling
- instrumentation coupling

### `types`

- foundational model coupling

### `stdlib`

- representation coupling

## 実務上の見方

変更しやすさの観点では:

- `budget`, `errs`, `span`, `reg` は安全
- `core`, `stdlib`, `types` は中くらい
- `eval` は注意が必要
- `engine` は public contract の影響が大きい
- `check` は最も慎重に触るべき

## 一言で言うと

- `check` は強力だが、構造的には最も密結合
- `engine` は統合境界として結合が強い
- `eval` は責務は一貫しているが、1 本の実行経路に色々載せている
- それ以外は比較的健全
