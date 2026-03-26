# Universe Extensions

## Type/Kind 統合 (Phase Transition)

Type と Kind を統一表現に統合する。`types.Kind` interface を廃止し、`types.Type` に universe level を付与。

**動機**: 現在 Type と Kind が別の Go type hierarchy であることが以下の制限の根源:

- `->` が Type-level (`TyArrow`) と Kind-level (`KArrow`) で別表現
- kind annotation 内で type family application が使えない
- type family の結果を kind position で利用できない
- Kind unification と Type unification のロジック重複

**設計**:

```
Term   ← runtime。唯一の特別扱い。型に出現しない（= 依存型なし）
Type   ← compile-time の統一表現。universe level で階層化
           Level 0: Int, Bool, List a, ...  (値の型)
           Level 1: Type, Row, Constraint, Bool (promoted), ...  (型の型 = kind)
           Level 2: Kind (= Sort₀)  (kind の型 = sort)
           ...
```

| 現状                   | 統一後                         |
| ---------------------- | ------------------------------ |
| `KType{}`              | `TyCon("Type")` at level 1     |
| `KRow{}`               | `TyCon("Row")` at level 1      |
| `KArrow{From, To}`     | `TyArrow{From, To}` (同一表現) |
| `KData{Name}`          | `TyCon(Name)` at level 1       |
| `KSort{Level: n}`      | `TyCon("Sort")` at level n+2   |
| `KMeta{ID}`            | `TyMeta{ID}` (統合)            |
| kind annotation `(a:)` | type expression として parse   |
| kind unification       | type unification と同一        |

**得られるもの**:

- `->` が全 level で一つの表現
- type family の結果を kind position で使用可能
- kind annotation に任意の type expression を許可
- promotion が自明（level shift のみ）
- unification ロジックの統合

**守られるもの**:

- Term は Type に出現しない（依存型なし）
- ユーザは universe level を意識しない（推論）

**理論的位置づけ**: PTS (Pure Type System) から Term-Type 間の dependent rule を除いた構成。System Fω + stratified universes。

**規模**: `types.Kind` interface を使う全コードが影響を受ける phase transition 級の変更。

**前提**: 現在の cumulativity (ground kinds ≤ Sort₀) は暫定措置。Type/Kind 統合後は自明に解消。

## Level Metavariable

Type/Kind 統合後、universe level の推論に必要。`TySort{Level: LevelExpr}` で表現。

- `LevelExpr ::= LevelLit(n) | LevelVar(name) | LevelMax(a, b) | LevelSucc(e) | LevelMeta(id)`
- Level constraint solver: Harper-Pollack (1991) — constraint graph の acyclicity + shortest-path

## Impredicativity

計画外。
