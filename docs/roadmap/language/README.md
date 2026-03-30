# Language Roadmap

GICEL の言語機能拡張の方向性。バージョン番号は付けない — 各項目は依存関係で順序付ける。

**性格**: library/tooling roadmap が需要駆動・展開観察型であるのに対し、言語機能は**追求型**。型理論・圏論的構造の内在的な帰結を追い、設計空間の論理的な到達点を目指す。

## 設計方針

- **依存型・リファイン型は導入しない** — 型レベル計算の複雑度を抑え、sandbox の予測可能性を維持
- **上位宇宙は開く** — Level N への拡張は行う。ただし実行系（evaluator, VM）には触れない
- **ゴール**: CBPV + Atkey + Row + Grade の4本柱が型レベルで完全に表現可能になること

## GOAL LINE

```
╔═══════════════════════════════════════════════════════╗
║  GICEL = Graded Indexed Capability Effect Language    ║
║                                                       ║
║  4本柱が型システムに全て存在する:                     ║
║    CBPV   value/computation    ✅                     ║
║    Atkey  pre/post capability  ✅                     ║
║    Row    capability 環境      ✅                     ║
║    Grade  per-computation 量   ◻ → GIMonad class      ║
║                                                       ║
║  + 宇宙の正当性が保証されている                       ║
║    (ヒューリスティックの排除) → 宇宙多相 Phase A      ║
╚═══════════════════════════════════════════════════════╝
```

GOAL LINE を構成する2項目は互いに独立で並行着手可能:

| 項目               | スコープ                                                                      | 詳細                                   |
| ------------------ | ----------------------------------------------------------------------------- | -------------------------------------- |
| GIMonad class 定義 | GradeAlgebra 拡張 (Compose/Join 分離), TyCBPV.Grade 追加, inferPure/Bind 拡張 | [infrastructure.md](infrastructure.md) |
| 宇宙多相 Phase A   | LevelMeta 活性化, TypeOfTypes skip 解消, kind 推論の理論的正当化              | [universe.md](universe.md)             |

## 全体構造

```
             ┌────────────────────────────────────────┐
             │            GOAL LINE                   │
             │                                        │
             │  GIMonad class     宇宙多相 Phase A    │
             │  (独立)            (独立)              │
             └──────┬─────────────────┬───────────────┘
                    │                 │
  ══════════════════╪═════════════════╪════════════════════
                    │                 │
                    │                 ├──→ 宇宙多相 B-C → Level N
                    │                 └──→ PolyKinds
                    │
                    └···⚠··· impredicativity

  POST-GOAL (独立して着手可能):
    -| 演算子, lazy co-data, SMC Phase 2/3,
    Optimizer 2-3, nested let-gen, deriving
```

## ドキュメント構成

| ファイル                               | 内容                                  |
| -------------------------------------- | ------------------------------------- |
| [infrastructure.md](infrastructure.md) | GIMonad + Open Questions              |
| [universe.md](universe.md)             | 宇宙多相 Phase A-C + PolyKinds        |
| [smc.md](smc.md)                       | SMC Path — `Merge`/`***`/`dag` + Mult |
| [independent.md](independent.md)       | 構文拡張、設計判断、sessions          |
