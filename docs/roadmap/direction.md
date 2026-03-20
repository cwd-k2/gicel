# Project Direction

GICEL の成熟方針と立ち位置。

## 方針

1. `more features` より `faster convergence`
2. `more theory` より `clearer product value`
3. `more surface` より `less divergence`

## 判断軸

変更の優先度はこの順で判定する。

### 1. 主目的に直接効くか

最優先は typed sandbox / proof-of-concept programming / Go embedding に直結するもの。面白くても主目的に直接つながらない変更は後回しにする。

### 2. 収束速度を上げるか

ボトルネックは「書けるか」より「詰まったときの収束速度」にある。高く評価するべき変更:

- diagnostics の強化
- docs/example/help の整合
- runtime failure の調査性
- JSON 契約の一貫性
- multi-module / host API の friction 削減

### 3. 中核の価値を外周が隠していないか

外周改善（docs / CLI / examples / diagnostics）は「付随作業」ではなく、product value の露出そのもの。

### 4. 高度機能は支えが揃っているか

前に出してよい条件:

- working example がある
- docs がある
- failure path が収束できる
- diagnostics / explain が追える

揃っていなければ、機能の存在を否定せず露出を下げる。

## 優先順位

| 順位 | 領域           | 対象                                                       |
| ---- | -------------- | ---------------------------------------------------------- |
| 1    | 収束性         | diagnostics, explain, JSON 契約, docs 整合                 |
| 2    | 埋め込み運用性 | RunSandbox 契約, host API, security boundary, multi-module |
| 3    | framing        | README, examples, docs 露出順, 比較対象との位置づけ        |
| 4    | 高度機能の成熟 | sessions, type families, evidence/checker 拡張             |

## 判断原則

1. **新機能は「収束経路込み」で評価する** — 学び方・壊れ方・直し方まで一単位
2. **docs / diagnostics / CLI は product core** — 外周は後回しの仕上げではない
3. **「理論的に正しい」より「主戦場で強い」** — typed sandbox / PoC / embedding に効かないなら優先度を上げない
4. **高度機能の露出は maturity に比例** — docs/examples/CLI が追いつくまで露出は抑える
5. **divergence を増やす変更を嫌う** — docs と実装の意味ズレ、CLI と Go API の分裂、support surface の放置を避ける

## 成熟した状態の定義

- 何のための言語かが一言で説明できる
- docs / CLI / examples / diagnostics が同じ worldview を返す
- 代表的な失敗から速く収束できる
- Go embedding と sandbox boundary の説明責任が果たせる
- 高度機能が、あるだけでなく使い切れる

## 競争環境とポジション

GICEL は「軽い埋め込み言語」の競争ではなく、**安全で強い PoC sandbox language** という少し空いたポジションで勝負する。

### 勝ちやすい場面

- AI agent が安全に小さな PoC プログラムを書く
- ホストアプリが typed scripting / capability-bounded logic を持ちたい
- stateful sandbox logic を Go に埋め込みたい
- 軽量式評価では足りず、汎用言語を開放するには危ない場面

### 苦しい場面

- 単純な式評価だけでよい（→ CEL, expr）
- 設定記述や schema 記述が主目的（→ Dhall, CUE）
- 超軽量な導入が最優先（→ Starlark, Tengo）
- 既存の成熟エコシステムが必須

### 主な比較対象

| 対象     | 主戦場             | GICEL の差別化                          |
| -------- | ------------------ | --------------------------------------- |
| Starlark | 埋め込みスクリプト | 型と capability で勝つ                  |
| CEL      | 制限付き式評価     | プログラム性で勝負                      |
| Dhall    | 安全な設定言語     | effect/state を持つ「実行する検証言語」 |
| Rego     | ポリシー言語       | 汎用 PoC sandbox として住み分け         |
| Koka     | effectful 実用言語 | Go 埋め込み / sandbox 文脈で独自        |

### 実用メトリクス

| メトリクス                | 観点                                         |
| ------------------------- | -------------------------------------------- |
| First success time        | 最初の有意味な PoC を動かすまでの時間        |
| Recovery time             | failure から修正完了までの時間               |
| Docs-only completion rate | docs/examples だけで課題を解ける割合         |
| JSON repairability        | 構造化出力だけで自動修正ループを回せる割合   |
| Capability clarity        | sandbox / trusted 境界を誤解なく説明できるか |
