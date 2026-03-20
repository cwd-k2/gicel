# Budget Review 2026-03-20

対象:

- `internal/budget`
- `parse` / `check` / `unify` / `family` / `eval` / `engine` / `stdlib`

目的:

- 各フェーズの防御機構としての budget 構造を確認する
- 防御が統一化されているかを見る
- 二重管理や未統一箇所を洗い出す

## 総評

`budget` パッケージ自体の設計はかなり良いです。責務も明確で、以下を一つにまとめています。

- step
- logical depth
- structural nesting
- allocation
- context cancellation

根拠: [`internal/budget/budget.go:1`]( /Users/cwd-k2/Projects/gicel/internal/budget/budget.go#L1 )

ただし、実際の適用は完全には統一されていません。

現状はこうです。

- `eval` / `stdlib` / `check` / `unify` / `family` は同じ budget 系に乗っている
- `parse` は独自の step / recurse 制限を持っていて budget 非採用
- `opt` は budget の外
- compile と eval で同一 budget instance を共有しているわけではない

つまり「共通 budget の核はあるが、パイプライン全体では半統一」です。

## 何が統一されているか

### 1. `budget.Budget` の抽象は一貫している

`Budget` は以下の API を持つ:

- `Step()`
- `Enter()` / `Leave()`
- `Nest()` / `Unnest()`
- `Alloc()`
- `Context()`
- `ContextWithBudget()` / `ChargeAlloc()`

根拠: [`internal/budget/budget.go:17`]( /Users/cwd-k2/Projects/gicel/internal/budget/budget.go#L17 )

このレイヤ自体は整理されています。

### 2. `eval` と `stdlib` の接続はきれい

`eval` は per-execution の `Budget` を作り: [`internal/engine/runtime.go:137`]( /Users/cwd-k2/Projects/gicel/internal/engine/runtime.go#L137 )

- step
- depth
- nesting
- alloc

を設定してから、`Evaluator` に渡しています。

さらに `Evaluator` はその budget を context に埋め込み: [`internal/eval/eval.go:52`]( /Users/cwd-k2/Projects/gicel/internal/eval/eval.go#L52 )

`stdlib` 側は `budget.ChargeAlloc(ctx, bytes)` で Go 側 allocation を課金します。

この流れはかなり素直です。

### 3. `check` / `unify` / `family` も同じ budget に乗っている

`Checker` は作成時に `budget.New(...)` を作り: [`internal/check/checker.go:217`]( /Users/cwd-k2/Projects/gicel/internal/check/checker.go#L217 )

- `Checker.budget`
- `Unifier.Budget`
- `ReduceEnv.Budget`

に共有させています。

これは良い設計です。少なくとも type-checking phase の内部では、防御機構が一箇所に揃っています。

## 何が統一されていないか

### 1. `parse` だけ独自防御

`Parser` は独自に:

- `maxRecurseDepth`
- `maxSteps`
- `halted`

を持っています: [`internal/syntax/parse/parser.go:19`]( /Users/cwd-k2/Projects/gicel/internal/syntax/parse/parser.go#L19 )

これは budget パッケージを使っていません。

影響:

- parse phase だけ limit の意味論が別物
- CLI / sandbox / engine の limit 設定と直接つながっていない
- 「pipeline 全体で何をどこまで制限しているか」が説明しづらい

評価:

- 防御としては有効
- ただし統一という観点では弱い

### 2. compile と eval は同じ budget instance を共有していない

`budget` パッケージのコメントは「同じ Budget instance を複数 phase で共有できる」と言っていますが: [`internal/budget/budget.go:4`]( /Users/cwd-k2/Projects/gicel/internal/budget/budget.go#L4 )

実際には:

- `check` で一つ作る
- `eval` で別の一つを作る

です。

特に runtime execution は毎回新しい budget を作る: [`internal/engine/runtime.go:137`]( /Users/cwd-k2/Projects/gicel/internal/engine/runtime.go#L137 )

影響:

- 「統一 API」はあるが「統一 resource pool」にはなっていない
- compile + eval の合算 budget ではない
- timeout だけが context 経由で compile / eval にまたがる

評価:

- これは必ずしも悪くない
- ただし package comment は少し理想寄りで、現実の使い方とはズレています

### 3. `check` の step budget は実質 type family 防御に偏っている

`Checker` は `budget.New(ctx, family.MaxReductionWork, 0)` で生成されます: [`internal/check/checker.go:219`]( /Users/cwd-k2/Projects/gicel/internal/check/checker.go#L219 )

つまり:

- step limit は `family.MaxReductionWork`
- depth limit は 0
- nesting limit は任意

です。

ここでの `maxSteps` は checker 全体の一般的な work budget というより、かなり type family reduction 起点の上限です。

加えて `ReduceEnv.ReduceAll()` は毎回 `ResetCounters()` します: [`internal/check/family/reduce.go:36`]( /Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L36 )

影響:

- checker phase の「step budget」という名前から想像するほど全域的ではない
- family reduction は守られているが、checker 全体の work accounting は均一ではない

評価:

- 防御としては pragmatic
- ただし semantics は少し uneven

### 4. `parse` と `check` で depth / nesting の意味が違う

`parse`:

- `recurseDepth`: parser 実装の再帰深さ
- `maxSteps`: `advance()` 回数

`budget`:

- `depth`: logical call depth (`Enter/Leave`)
- `nesting`: structural nesting (`Nest/Unnest`)

影響:

- どちらも「深さ」や「ステップ」と呼べるが、意味が揃っていない
- モニタリングや user-facing docs に落とすときに混乱しやすい

評価:

- これは統一 abstraction の未完成さとして見てよい

### 5. `opt` は budget の外

少なくとも現状、optimization phase は budget の共通構造に乗っていません。

影響:

- budget package のコメントが言う「parsing, type checking, optimization, evaluation」の統一防御には未到達

評価:

- 実害は小さいが、設計説明としては誇張気味

## 防御機構としての質

### 良い点

- `eval` と `stdlib` の allocation accounting は一貫している
- `check` / `unify` / `family` が同じ budget を共有している
- timeout は context 経由で compile / eval にまたがる
- エラー型も `StepLimitError`, `DepthLimitError`, `NestingLimitError`, `AllocLimitError` と明快

### 弱い点

- `parse` が独自制御
- checker の step budget が全域統一というより family 中心
- compile と eval が同一 budget pool を共有しない
- package comment と実運用のズレがある

## 判定

### 設計の核

- 良い

### パイプライン全体の統一度

- 中程度

### ドキュメントと実態の一致

- やや弱い

## 改善するとしたら

1. `parse` を `budget.Budget` に寄せるか、少なくとも概念名を揃える
2. `budget` package comment を「共有可能」へ弱め、現実の使用形態に合わせる
3. checker の step budget を family 専用と全体専用で分けて説明する
4. optimization phase に budget を入れるか、対象外と明記する

## 一言で言うと

防御機構は存在するし、かなり真面目です。

ただし「単一の統一 budget が全フェーズを貫いている」とまでは言えません。現状は:

- `eval/check` 側は共通 budget 文化
- `parse` は独自文化

という二層構造です。
