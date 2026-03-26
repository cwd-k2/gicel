# Infrastructure Path

型システム基盤の整備計画。L3 (grade algebra) + universe extensions まで完了。

## 完了済み

| Layer            | 項目                         | 概要                                                      |
| ---------------- | ---------------------------- | --------------------------------------------------------- |
| L0-b             | Universe enforcement         | `KSort{Level int}`、`form :: Kind` 拒否                   |
| L1-a             | SolverLevel protocol         | 3 timing pattern の不変条件コメント                       |
| L1-b             | Inert set scope              | `scopeDepth`、scope-aware `Reset`                         |
| L1-c             | Equality constraints         | `TyExprEq`、`CtEq`、given eq via `InstallGivenEq`         |
| L2-a partial     | TF on-demand (joinGrades)    | joinGrades の CtFunEq 化                                  |
| L2-b             | Grade CtFunEq error          | `OnFailure` callback、`ErrMultiplicity`                   |
| L3               | Grade algebra user-definable | `GradeAlgebra` class、dynamic resolution、e2e テスト      |
| Non-nullary prom | Constructor promotion        | `Just :: KType -> KData{Maybe}` — kind arrow 付き promote |
| Row TF           | Merge                        | `Merge :: Row -> Row -> Row` builtin type family          |
| Cumulativity     | Kind cumulativity            | ground kinds (Type, Row, KData) ≤ Sort₀                   |

## 残存

### 応用層（着手可能）

- **Named capabilities**: ゲートモデル → 名前付きリソース。stdlib API 大規模改修。詳細は下記。
- **Per-computation grade**: Computation 型自体に grade。`bind` での grade 合成 (Katsumata)。
- **Row TF: Without/Lookup**: 型レベル label 表現（型レベル文字列 or promoted symbol）の設計が前提。

### L4: Solver 統合（長期）

GADT given eq の solver 統合 + normalize FamilyReducer 除去 + OutsideIn(X)。相互依存。現状は三層（`InstallGivenEq` + eager normalize + deferred CtFunEq）で正しく動作。

### Universe extensions（残存）

- **Level metavariable**: Sort₂+ が必要になったときに `KSort{Level: LevelExpr}` に拡張。現時点で需要なし。
- **Sort₂+**: `KSort{Level: int}` で構造的に対応済み。コードパス未生成。

### 独立タスク

| 項目               | 状態   |
| ------------------ | ------ |
| L0-d: Tuple Eq/Ord | 未着手 |

## Named Capabilities

現行のゲートモデル (`{ array: () | r }` = permission bit) から、名前付きリソースモデルへ移行する。

```gicel
-- ゲートモデル (現行)
main := do {
  a <- Arr.new 5 0;    -- { array: () | r } — 全配列が同一ゲート
  b <- Arr.new 3 0;
}

-- 名前付きリソースモデル (目標)
main := do {
  counts <- allocArray 5 0;
  -- post: { counts: Array Int @Linear | r }
  cache  <- allocMap;
  -- post: { counts: Array Int @Linear, cache: Map K V @Affine | r }
}
```

**理論的根拠**: Atkey indexed monad + row types + grades が本来持つ表現力の復元。ゲートモデルはその退化形。

**変更範囲**: stdlib の Effect.Array/Map/State/Set の API 改修が主。型検査器の変更は限定的（row infrastructure は既に named typed fields をサポート）。runtime の CapEnv にも handle 管理を追加。

## Per-Computation Grade (Double Grading)

```gicel
-- per-resource grade (現行): { x: T @Linear | r }
-- per-computation grade (追加): Computation pre post @cost a
```

設計未決: Computation の grade パラメータ位置、grade 合成規則、`bind` での grade 合成。

## Open Questions

- **`@` の将来**: 型演算子への降格 (`(@) :: Type -> Grade -> Type`)
- **Grade polymorphism**: `\(π: Mult). A @π -> B`
- **□ modality**: `□ g A` の導入で `@` が中置糖衣に降格
- **Impredicativity**: 計画外
