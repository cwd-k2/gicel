# Language Roadmap

GICEL の言語機能拡張の方向性。バージョン番号は付けない — 各項目は依存関係で順序付ける。

**性格**: library/tooling roadmap が需要駆動・展開観察型であるのに対し、言語機能は**追求型**。型理論・圏論的構造の内在的な帰結を追い、設計空間の論理的な到達点を目指す。

## 全体

```
  着手可能
  ────────────────────────────────────────────
  SMC Phase 2/3: >< (parallel), dag
  Named capabilities (ゲート → 名前付きリソース)
  Per-computation grade (double grading)
  Row TF: Without / Lookup (型レベル label 後)
  Independent: Type operators (:>), -<

  長期
  ────────────────────────────────────────────
  L4: Solver 統合 (OutsideIn(X))
```

## ドキュメント構成

| ファイル                               | 内容                                 |
| -------------------------------------- | ------------------------------------ |
| [smc.md](smc.md)                       | SMC Path — `><`/`dag` + Multiplicity |
| [infrastructure.md](infrastructure.md) | 応用層 + L4 + Open Questions         |
| [universe.md](universe.md)             | Impredicativity (計画外)             |
| [independent.md](independent.md)       | 構文拡張、設計判断、session types    |

## 次の着手候補

- **SMC Phase 2**: `><` — 型検査器内部の disjoint row merge。パーサ変更なし。
- **Named capabilities**: stdlib API 改修。型システムの変更は最小限。
- **Without/Lookup**: 型レベル label 表現の設計判断が前提。
