# Universe Extensions

## Level Metavariable / Universe Polymorphism

単一の定義を複数の universe level で使う。

```gicel
-- 現状: cumulativity により \(k: Kind) で実用的な kind polymorphism が動作
type Id := \(k: Kind) (a: k). a

-- 将来: 明示的 level quantification（Sort₂+ が必要になったとき）
id :: \(l: Level). \(a: Type l). a -> a
```

**必要なインフラ**: `KSort{Level: int}` → `KSort{Level: LevelExpr}`、Level metavar、Level constraint solver。

Sort₂+ の需要発生時に着手。

## Sort₂+

`Kind` をペイロードに持つ data 型の promotion で必要。`KSort{Level: int}` で構造的に対応済み。コードパスは需要発生時に追加。

## Impredicativity

計画外。
