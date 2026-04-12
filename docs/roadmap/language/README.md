# Language Roadmap

GICEL の言語機能拡張の方向性。

**性格**: library/tooling roadmap が需要駆動・展開観察型であるのに対し、言語機能は**追求型**。型理論・圏論的構造の内在的な帰結を追い、設計空間の論理的な到達点を目指す。

## 設計方針

- **依存型・リファイン型は導入しない** — 型レベル計算の複雑度を抑え、sandbox の予測可能性を維持
- **上位宇宙は開く** — Level N への拡張は行う。ただし実行系（evaluator, VM）には触れない
- **ゴール**: CBPV + Atkey + Row + Grade の4本柱が型レベルで完全に表現可能になること — **達成済み**

## 将来の拡張

独立して着手可能。

| 項目                | 分類 | 詳細                                               |
| ------------------- | ---- | -------------------------------------------------- |
| Type operators      | 構文 | infix type alias。[independent.md](independent.md) |
| `deriving Eq, Show` | 機能 | 自動導出                                           |

## 設計判断の記録

評価した上で採用しなかった項目。→ [independent.md](independent.md)

## ドキュメント構成

| ファイル                               | 内容                                     |
| -------------------------------------- | ---------------------------------------- |
| [infrastructure.md](infrastructure.md) | Open Questions (grade の将来)            |
| [independent.md](independent.md)       | 構文拡張、設計判断、sessions、不採用項目 |
