# Universe Path

宇宙階層の正当化と拡張。実行系（evaluator, VM）には触れない — 型レベルの拡張に閉じる。

**依存チェーン**: Phase A → Phase B → Phase C。PolyKinds は Phase A が前提。

## 現状

3層ヒエラルキー + cumulativity:

```
Level 0 (Term)  ──  値。Int, Bool, \x. x
Level 1 (Kind)  ──  Type, Row, Constraint, Label, promoted data kinds
Level 2 (Sort₀) ──  Kind (Level 1 の classifier)
```

- Russell-style: 明示的 lift 演算子なし
- Cumulativity: ground kinds (L1) ↔ Sort₀ (L2) の単一化が動作
- HKT kind 変数: `form Functor := \(f: k -> Type)` — lowercase → 暗黙 kind 変数
- 暗黙 kind 多相: unannotated param は `TypeOfTypes` skip で promoted kinds を受容

## Phase A: LevelMeta 活性化 ← DONE

**実装済み** (v0.24+)。`checkTypeAppKind` の `TypeOfTypes` skip を除去し、`Type` パラメータに対する統一的な `UnifyLevels` パスに統合。

**完了事項**:

1. ✅ `resolveKindExpr` で unannotated param に `LevelMeta`（推論メタ変数）を fresh で割り当て（既存）
2. ✅ `TypeOfTypes` skip を除去し、`LevelMeta` 単一化で kind を解決
3. ✅ `ZonkLevelDefault` で未解決 `LevelMeta` → L0 にデフォルト（既存）
4. ✅ cumulativity ルール (`levelAdjacentCumulativity`)（既存）

**効果**: kind 推論が理論的に正当化される。ヒューリスティックの排除。

## Phase B: 明示的レベル量化

構文で Level 変数を量化可能にする:

```gicel
type Id :: forall (l: Level). Type l -> Type l := \a. a;
```

- パーサに `Level` kind を追加、`resolveKindExpr` に `"Sort"` 等のケースを追加
- `LevelVar` をスコープ管理
- `Type l` 表記で任意のレベルの universe を参照 (`Type 0 = Type`, `Type 1 = Kind`, `Type 2 = Sort`)

**前提**: Phase A。

## Phase C: LevelMax / LevelSucc

型族・class の kind 計算で `max(ℓ₁, ℓ₂)` が必要になる場面で活性化:

```gicel
form Pair := \(a: Type l1) (b: Type l2). { fst: a; snd: b; };
-- kind: Type (max l1 l2)
```

- `LevelMax` 制約の解決: レベル変数のグラフで非循環性チェック
- `LevelSucc` による `Type_i : Type_{i+1}` の表現

**前提**: Phase B。

**理論的注意**: LevelMax 制約の decidability は Phase B 着手前に確認が必要。Agda/Lean の制約解決器が参考になる。

## PolyKinds

Phase A が前提。本格的な kind 多相:

- ~~現在の暗黙 kind 多相 (unannotated param → TypeOfTypes skip) を LevelMeta ベースの正当な推論に置換~~ **Done** (Phase A)
- kind 変数が任意の promoted data kind を受容できるようになる

## 到達後の姿

```
Level N の宇宙を型レベルで自由に扱える:
  Type 0 = Type    (値の型の型)
  Type 1 = Kind    (kind の型)
  Type 2 = Sort    (sort の型)
  Type N = ...     (任意のレベル)

  forall (l: Level). Type l -> Type l   -- レベル多相な関数
  max(l1, l2)                            -- レベルの上界
```

依存型は導入しない。上位宇宙は開くが、値レベルでの型操作 (large elimination 等) は制限を維持する。
