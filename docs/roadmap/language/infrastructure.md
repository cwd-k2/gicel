# Infrastructure Path

## 応用層

### Per-Computation Grade (Double Grading)

Computation 型自体に grade を付与する。

```gicel
-- per-resource grade: { x: T @Linear | r }
-- per-computation grade: Computation pre post @#cost a
```

**設計未決**:

- Computation の grade パラメータの位置 (`Computation pre post a` → `Computation pre post g a` or `Computation g pre post a`)
- Per-computation grade と per-resource grade の相互作用規則
- `bind` での grade 合成: `bind : T_g A → (A → T_h B) → T_{g·h} B` (Katsumata graded monad)

## Open Questions

- **`@` の将来**: 型演算子への降格 (`(@) :: Type -> Grade -> Type`)
- **Grade polymorphism**: `\(π: Mult). A @π -> B`
- **□ modality**: `□ g A` の導入で `@` が中置糖衣に降格
