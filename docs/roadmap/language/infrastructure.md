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

## Open Questions

- **`@` の将来**: 型演算子への降格 (`(@) :: Type -> Grade -> Type`)
- **Grade polymorphism**: `\(π: Mult). A @π -> B`
- **□ modality**: `□ g A` の導入で `@` が中置糖衣に降格
