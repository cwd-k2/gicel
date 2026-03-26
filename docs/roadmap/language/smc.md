# SMC Path

Atkey indexed monad (monad in Prof) から Free †-SMC への段階的拡張。

**依存チェーン**: Phase 2 → Phase 3 → Phase 4。

## Phase 2: Parallel Composition

```gicel
infixr 3 ><
(><) :: Computation pre₁ post₁ a -> Computation pre₂ post₂ b
     -> Computation pre₃ post₃ (a, b)
     -- pre₃, post₃ は型検査器が内部的に disjoint merge で計算
```

Host-provided primitive。Runtime: capability 環境を分割し、両 computation を独立実行、結果環境をマージ。

**実装**: 型検査器内部の disjoint row merge。`classifyFields` + `Merge` builtin type family で基盤あり。

## Phase 3: Dagger

```gicel
type Gate pre post := Computation pre post ()
dag :: Gate pre post -> Gate post pre
```

pre/post スワップ。`dag (dag f) = f` は構造的に保証。`dag (f ; g) = dag g ; dag f` は host 実装が保証。

## Phase 4: Multiplicity Generalization

`GradeAlgebra` を半環に拡張:

```gicel
form UsageSemiring := \(s: Type). {
  zero: s; one: s; plus: s -> s -> s; mult: s -> s -> s
}
```

量子リソース追跡 (probability semiring) や QTT 接続を可能にする。

**残り作業**: 半環形式化 (`plus`/`mult` 追加)、積圏表現、QTT 接続。

## Row TF: Without / Lookup

`Merge :: Row -> Row -> Row` は builtin type family として実装済み。`Without` と `Lookup` は型レベル label 表現の設計が前提。

```gicel
type Without :: Row -> Type -> Row   -- remove a label
type Lookup :: Type -> Row -> Type   -- look up a label
```

## Full SMC 到達後

| Concept            | Target                         |
| ------------------ | ------------------------------ |
| Foundation         | Free †-SMC                     |
| Sequential compose | `;` (do blocks)                |
| Parallel compose   | `><` (internal disjoint merge) |
| Inversion          | `dag` (pre/post swap)          |
| Wire bundles       | Row types                      |
| Morphism type      | `Computation pre post a`       |
| User row ops       | Merge / Without / Lookup       |

**ゼロ構文変更。** `do` blocks = sequential, `><` = parallel, `dag` = inversion。パーサ変更なし。意味論拡張のみ。
