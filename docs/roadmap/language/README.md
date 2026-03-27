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
  Label kind (ラベルの型レベル昇格)
  Row TF: Without / Lookup (Label kind 後)
  Independent: Type operators (:>), -<
```

## ドキュメント構成

| ファイル                               | 内容                         |
| -------------------------------------- | ---------------------------- |
| [smc.md](smc.md)                       | SMC Path — `><`/`dag` + Mult |
| [infrastructure.md](infrastructure.md) | 応用層 + Open Questions      |
| [independent.md](independent.md)       | 構文拡張、設計判断、sessions |

## 次の着手候補

- **SMC Phase 2**: `><` — 型検査器内部の disjoint row merge。パーサ変更なし。
- **Named capabilities**: stdlib API 改修。型システムの変更は最小限。
- **Label kind**: ラベルを `Label` kind に昇格。Shell pack の前提。
- **Without/Lookup**: Label kind が前提。
