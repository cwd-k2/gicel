# Coupling Review 2026-03-20

対象:

- `internal/check`
- `internal/syntax/parse`

目的:

- `check` / `parse` の内部依存関係とロジック結合度を、構造面からざっくり測る
- 変更波及が大きい箇所を特定する

確認方法:

- ファイル数
- `Checker` / `Parser` レシーバーメソッド数
- 中央状態オブジェクトの大きさ
- recovery / import / solver まわりの責務集中

## 生の計測値

### `internal/check`

- 直下ファイル数: 95
- `func (ch *Checker)` メソッド数: 161

### `internal/syntax/parse`

- 直下ファイル数: 20
- `func (p *Parser)` メソッド数: 88

## 結論

`parse` もそれなりに集中していますが、`check` のほうが明確に密結合です。

理由は単純で、`check` は:

- 状態を巨大な `Checker` に集約している
- その `Checker` に 161 個のメソッドがぶら下がっている
- 型解決、import、constraint solving、module export、family reduction、diagnostics が同じ中心オブジェクトを共有している

一方 `parse` は:

- `Parser` に状態は集中しているが、責務は比較的一貫している
- 主な複雑さは recovery と speculative parse に寄っている

## 詳細

### 1. `check` は「巨大な状態機械」に近い

`Checker` 自体がかなり多くの責務を抱えています: [`internal/check/checker.go:115`]( /Users/cwd-k2/Projects/gicel/internal/check/checker.go#L115 )

同じ struct の中に:

- typing context
- unifier
- budget / cancellation
- diagnostics
- semantic registry
- import / module scope
- worklist / inert set
- recursion / implication depth
- phase flags
- cached family reducer

が同居しています。

これは局所最適としては便利ですが、変更波及の観点ではかなり強い結合です。たとえば import 周りを変えるだけでも:

- `scope`
- `reg`
- `config.RegisteredTypes`
- ambiguity cache / ownership cache

に触れます。

### 2. `check` の結合は「shared mutable state 経由」で起きている

`check` の危なさは、ファイル数よりも共有状態の濃さです。

例:

- import は `RegisteredTypes`, `conInfo`, `aliases`, `classes`, `families`, `ctx` を直接更新する: [`internal/check/import.go:274`]( /Users/cwd-k2/Projects/gicel/internal/check/import.go#L274 )
- declaration pipeline も同じ `Checker` を通じて phase を進める: [`internal/check/decl.go:22`]( /Users/cwd-k2/Projects/gicel/internal/check/decl.go )
- solver は `worklist`, `inertSet`, `ambiguityCache`, `unifier`, `errors` にまたがる: [`internal/check/solver.go:14`]( /Users/cwd-k2/Projects/gicel/internal/check/solver.go#L14 )

つまり、責務ごとにオブジェクト境界が薄く、ほぼ全部 `Checker` に吸い込まれています。

これは:

- 実装速度は出る
- cross-feature interaction に強い

反面:

- 局所変更で副作用範囲が読みにくい
- テストが通っても設計負債が見えにくい
- 新機能追加時に「どこまで触ればよいか」が直感的でない

という形で効きます。

### 3. `parse` は集中しているが、`check` ほど悪くない

`Parser` も状態は中央集約です: [`internal/syntax/parse/parser.go:18`]( /Users/cwd-k2/Projects/gicel/internal/syntax/parse/parser.go#L18 )

ただし保持している状態は主に:

- token stream
- position
- fixity table
- error sink
- nesting / recursion / step limit
- statement boundary control

で、責務はまだ parser らしい範囲に収まっています。

`Parser` の 88 メソッドは少なくありませんが、`check` の 161 メソッドよりは意味的一貫性があります。

### 4. `parse` の本当の結合点は recovery と speculative parse

`parse` で結合が強いのは recovery 系です。

特に:

- speculative parse の後始末で `errors.Truncate(...)` に依存している: [`internal/syntax/parse/parse_expr.go:208`]( /Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_expr.go#L208 )
- 失敗後の top-level recovery は `syncToNextDecl()` という heuristic に依存している: [`internal/syntax/parse/parser.go:274`]( /Users/cwd-k2/Projects/gicel/internal/syntax/parse/parser.go#L274 )
- 実際に probe test 側でも valid decl を recovery が飲み込む既知問題を認めている: [`internal/syntax/parse/parser_crash_probe_test.go:432`]( /Users/cwd-k2/Projects/gicel/internal/syntax/parse/parser_crash_probe_test.go#L432 )

つまり `parse` は:

- 正常系はかなり安定
- 異常系 recovery の結合が高い

という構図です。

### 5. 結合の質が違う

`parse` の結合:

- 制御フロー結合が中心
- backtrack, stmt boundary, recovery が絡む
- 壊れ方は「診断品質低下」「有効コードの取りこぼし」

`check` の結合:

- データ結合が中心
- registry, context, unifier, import scope, solver が同じ状態を共有
- 壊れ方は「意味論のにじみ」「変更波及」「境界不明瞭」

この違いは大きいです。`parse` は局所的に直せる可能性が高いですが、`check` は構造改善なしだと大きくなるほど辛くなります。

## 判定

### `parse`

- 結合度: 中
- 主なリスク: error recovery の局所的密結合

### `check`

- 結合度: 高
- 主なリスク: `Checker` への状態集中による変更波及

## 改善するとしたら

優先度順にやるなら:

1. `check` の import/export/scope を `Checker` 本体から少し切り離す
2. `check` の solver 周辺を state object ごとに薄く分ける
3. `parse` の speculative parse を helper 化して「state rollback」を一段明示化する
4. `parse` の recovery を token heuristic から declaration-aware に寄せる

## 一言で言うと

`parse` は「複雑だがまだ parser の範囲にいる」。

`check` は「機能的には強いが、内部構造はかなり密結合」。
