# SMC Path

Atkey indexed monad (monad in Prof) から Free †-SMC への段階的拡張。

**依存チェーン**: Phase 2 → Phase 3 → Phase 4。Phase 1 は `><` の前提ではなく独立して実施可能。

Phase 4 は [infrastructure.md](infrastructure.md) の L3 (grade algebra) と合流する。

## Phase 2: Parallel Composition

```gicel
infixr 3 ><
(><) :: Computation pre₁ post₁ a -> Computation pre₂ post₂ b
     -> Computation pre₃ post₃ (a, b)
     -- pre₃, post₃ は型検査器が内部的に disjoint merge で計算
```

Host-provided primitive。Runtime: capability 環境を分割し、両 computation を独立実行、結果環境をマージ。

**実装**: 型検査器内部の disjoint row merge。既存の `classifyFields` アルゴリズムを拡張し、重複ラベルをエラーにする。`Merge` 型族をユーザに公開する必要はない — `(+)` の型に `Add Int Int` 型族が不要なのと同じ。

**前提**: なし（Infrastructure Path と独立に着手可能）

## Phase 3: Dagger

```gicel
type Gate pre post := Computation pre post ()
dag :: Gate pre post -> Gate post pre
```

pre/post スワップ。`dag (dag f) = f` は構造的に保証。`dag (f ; g) = dag g ; dag f` は host 実装が保証。

**前提**: なし（型レベル pre/post swap のみ）。Phase 2 と独立だが、概念的には Phase 2 後。

## Phase 4: Multiplicity Generalization

ハードコードの `@Linear`/`@Affine`/`@Unrestricted` を型クラスベースの半環に一般化:

```gicel
form UsageSemiring := \(s: Type). {
  zero: s; one: s; plus: s -> s -> s; mult: s -> s -> s
}
```

既存の `{0, 1, ω}` 半環はデフォルトインスタンスとして保存。量子リソース追跡 (probability semiring) や QTT 接続を可能にする。

**解決する問題**:

- "Double grading" — 半環形式化で State × Usage の積圏を明示
- "Evidence fiber crossing" — `@Mult` が型レベルパラメータになり fiber 間相互作用を形式化

**前提**: Phase 2 + Phase 3 + [infrastructure.md](infrastructure.md) L3 (grade algebra user-definable)

**実装基盤**: [infrastructure.md](infrastructure.md) の L0-b (universe enforcement) + L1-L3 (solver 修正 + grade algebra) が前提基盤を整備する。本 Phase 4 の残り作業は半環形式化 (`plus`/`mult` 追加)、積圏表現、QTT 接続。

## Phase 1: Row-Level Type Families

Row merging, splitting, lookup をユーザ向け型レベル操作として公開。

```gicel
type Merge :: Row := \(r1: Row) (r2: Row). ...    -- merge two disjoint rows
type Without :: Row := \(l: Type) (r: Row). ...    -- remove a label
type Lookup :: Type := \(l: Type) (r: Row). ...    -- look up a label
```

**位置づけ**: Phase 2 (`><`) は型検査器内部の row merge で実装でき、Phase 1 は `><` の前提ではない。Phase 1 はユーザが `Merge` 等を型シグネチャで使えるようにするために実施する:

```gicel
split :: Computation (Merge r1 r2) post a -> ...
withoutKey :: Computation (Without "key" r) post a -> ...
```

**前提**: L2-a (TF on-demand reduction) が整備されていること。`><` / `dag` とは独立に着手可能。

### Type Family Reduction: Exponential Branching Fix

`reduceFamilyAppsN` が `Grow a = Pair (Grow a) (Grow a)` のようなパターンで指数的に分岐する。Phase 1 を実装する場合、`Merge` が再帰的に open row tail を展開する際の前提修正が必要。

**アプローチ**: shared-basis reduction (同一 TyApp を一度だけ reduce して memoize) または明示的分岐制限。

## Theoretical Status After Full SMC

| Concept            | Current                             | Target                         |
| ------------------ | ----------------------------------- | ------------------------------ |
| Foundation         | Atkey indexed monad (monad in Prof) | Free †-SMC                     |
| Sequential compose | `bind` (do blocks)                  | `;` — unchanged                |
| Parallel compose   | none                                | `><` (internal disjoint merge) |
| Inversion          | none                                | `dag` (pre/post swap)          |
| Wire bundles       | Row types                           | Row types — unchanged          |
| Morphism type      | `Computation pre post a`            | same                           |
| User row ops       | none                                | Phase 1: Merge/Without/Lookup  |

**ゼロ構文変更。** `do` blocks = sequential, `><` = parallel, `dag` = inversion。パーサ変更なし。意味論拡張のみ。
