# Universe Extensions

## 完了済み

| 項目                       | 概要                                               | Commit    |
| -------------------------- | -------------------------------------------------- | --------- |
| L0-b: Universe enforcement | `KSort{Level int}`、`form :: Kind` 拒否            | `92e9428` |
| Non-nullary promotion      | `Just :: KType -> KData{Maybe}` kind arrow         | `a8bf44e` |
| Cumulativity (Sort₀)       | ground kinds (Type, Row, Constraint, KData) ≤ Kind | `b6cc515` |

## 残存

### Level Metavariable / Universe Polymorphism

単一の定義を複数の universe level で使う。

```gicel
-- 現状: kind poly は推論で解決（cumulativity により）
type Id := \(k: Kind) (a: k). a  -- k は Type, Row, KData 等に推論

-- 将来: 明示的 level quantification（Sort₂+ が必要になったとき）
id :: \(l: Level). \(a: Type l). a -> a
```

**必要なインフラ**: `KSort{Level: int}` → `KSort{Level: LevelExpr}`、Level metavar、Level constraint solver。

**現状**: cumulativity により `\(k: Kind)` で実用的な kind polymorphism が動作。Level metavariable は Sort₂+ の需要発生時に追加。

### Sort₂+

`Kind` をペイロードに持つ data 型の promotion で必要。`KSort{Level: int}` で構造的に対応済み。コードパスは需要発生時に追加。

### Impredicativity

計画外。
