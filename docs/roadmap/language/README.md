# Language Roadmap

GICEL の言語機能拡張の方向性。バージョン番号は付けない — 各項目は依存関係で順序付ける。

**性格**: library/tooling roadmap が需要駆動・展開観察型であるのに対し、言語機能は**追求型**。型理論・圏論的構造の内在的な帰結を追い、設計空間の論理的な到達点を目指す。外部需要やユーザ friction ではなく、型システムの整合性と表現力の完成が駆動力。したがって direction.md の判断軸（§1 主目的直結、§2 収束速度）とは評価経路が異なり、ここでの優先順序は理論的依存関係で決まる。

## 現状

```
  ✓ 完了
  ──────
  Infrastructure: L0-b → L1 → L2-b → L3 (grade algebra)
  Universe: Non-nullary promotion, Cumulativity (Sort₀)
  Row TF: Merge :: Row -> Row -> Row
  Equality constraints: (a ~ Int) =>

  着手可能
  ────────
  ┌───────────────────────────────────────────────────┐
  │ SMC Phase 2/3: >< (parallel compose), dag         │
  │ Named capabilities (ゲート → 名前付きリソース)     │
  │ Per-computation grade (double grading)             │
  │ Row TF: Without / Lookup (型レベル label 表現後)   │
  │ Independent: Type operators (:>), -<, Tuple Eq/Ord │
  └───────────────────────────────────────────────────┘

  長期
  ────
  L4: Solver 統合 (OutsideIn(X))
  Level metavar / Sort₂+ (需要発生時)
```

## ドキュメント構成

| ファイル                               | 内容                                                     |
| -------------------------------------- | -------------------------------------------------------- |
| [smc.md](smc.md)                       | SMC Path — Phase 2-4。`><`/`dag` + Multiplicity          |
| [infrastructure.md](infrastructure.md) | Infrastructure Path — 完了 + 応用層 + L4                 |
| [universe.md](universe.md)             | Universe extensions — 完了 + Level metavar (残存)        |
| [independent.md](independent.md)       | 独立項目 — 構文拡張、設計判断、convention、session types |

## SMC Path

`><` (parallel composition) と `dag` は Infrastructure Path と独立に着手可能。Row merging は既存の `classifyFields` + `Merge` type family で実装基盤がある。両パスは SMC Phase 4 / Grade algebra で合流（L3 完了済み）。

## 次の着手候補

- **SMC Phase 2**: `><` — 型検査器内部の disjoint row merge。パーサ変更なし。
- **Named capabilities**: stdlib API 大規模改修。型システムの変更は最小限。
- **Without/Lookup**: 型レベル label 表現の設計判断が前提。
