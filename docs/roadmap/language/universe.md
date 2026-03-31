# Universe Path

宇宙階層の正当化と拡張。実行系（evaluator, VM）には触れない — 型レベルの拡張に閉じる。

## 現状 — Phase A-C + PolyKinds 完了

- Phase A: LevelMeta ベースの kind 推論
- Phase B: 明示的レベル量化 `\(l: Level) (a: Type l). a -> a`、kind application `Type l` パーサ対応
- Phase C: `LevelMax` result kind 推論 (`form Pair := \(a: Type l1) (b: Type l2). {...}` → `Type (max l1 l2)`)、dual level/type substitution、`ZonkLevelDefault` 正規化
- PolyKinds Phase D: LevelMeta と具体 Type kind パスの統合

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
