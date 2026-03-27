# Infrastructure Path

## 応用層

### Named Capabilities

ゲートモデル (`{ array: () | r }` = permission bit) から、名前付きリソースモデルへ移行する。

```gicel
main := do {
  counts <- allocArray 5 0;
  -- post: { counts: Array Int @Linear | r }
  cache  <- allocMap;
  -- post: { counts: Array Int @Linear, cache: Map K V @Affine | r }
}
```

**理論的根拠**: Atkey indexed monad + row types + grades が本来持つ表現力の復元。ゲートモデルはその退化形。

**変更範囲**: stdlib の Effect.Array/Map/State/Set の API 改修が主。型検査器の変更は限定的（row infrastructure は既に named typed fields をサポート）。runtime の CapEnv に handle 管理を追加。

### Per-Computation Grade (Double Grading)

Computation 型自体に grade を付与する。

```gicel
-- per-resource grade: { x: T @Linear | r }
-- per-computation grade: Computation pre post @cost a
```

**設計未決**:

- Computation の grade パラメータの位置 (`Computation pre post a` → `Computation pre post g a` or `Computation g pre post a`)
- Per-computation grade と per-resource grade の相互作用規則
- `bind` での grade 合成: `bind : T_g A → (A → T_h B) → T_{g·h} B` (Katsumata graded monad)

### Row TF: Without / Lookup

型レベル label 表現（型レベル文字列 or promoted symbol）の設計が前提。

## L4: Solver 統合（長期）

GADT given eq の solver 統合、normalize FamilyReducer 除去、OutsideIn(X) 本格導入。相互依存。

### 完了

- CtFlavor: given / wanted の区別（CtEq.Flavor）
- Given equality の solver 処理（processGivenEq: skolemSoln install + inert set + kick-out）
- KickOutMentioningSkolem: given eq 到着時の stuck CtFunEq / CtEq 再活性化
- 矛盾検出: concrete ~ concrete の given eq で inaccessible flag 設定
- emitEq / CtOrigin: 14 箇所の checker Unify を constraint emission に変換
- GenerationScope: DK body checking 中の constraint 収集 infrastructure

### 残課題

- FamilyReducer → solver 統合（normalize 内の FamilyReducer は correct optimization として残置、solver の CtFunEq パスと併存）
- CtImplication 本格活用（EnterGenerationScope → body check → ExitGenerationScope → wrap as CtImplication）
- Full phase separation（DK interleaving との共存設計）
- skolemSoln の solver 完全管理（現在は hybrid: unifier.skolemSoln + solver given set）

## Open Questions

- **`@` の将来**: 型演算子への降格 (`(@) :: Type -> Grade -> Type`)
- **Grade polymorphism**: `\(π: Mult). A @π -> B`
- **□ modality**: `□ g A` の導入で `@` が中置糖衣に降格
