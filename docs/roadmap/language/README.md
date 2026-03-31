# Language Roadmap

GICEL の言語機能拡張の方向性。バージョン番号は付けない — 各項目は依存関係で順序付ける。

**性格**: library/tooling roadmap が需要駆動・展開観察型であるのに対し、言語機能は**追求型**。型理論・圏論的構造の内在的な帰結を追い、設計空間の論理的な到達点を目指す。

## 設計方針

- **依存型・リファイン型は導入しない** — 型レベル計算の複雑度を抑え、sandbox の予測可能性を維持
- **上位宇宙は開く** — Level N への拡張は行う。ただし実行系（evaluator, VM）には触れない
- **ゴール**: CBPV + Atkey + Row + Grade の4本柱が型レベルで完全に表現可能になること

## GOAL LINE — REACHED

```
╔═══════════════════════════════════════════════════════╗
║  GICEL = Graded Indexed Capability Effect Language    ║
║                                                       ║
║  4本柱が型システムに全て存在する:                     ║
║    CBPV   value/computation    ✅                     ║
║    Atkey  pre/post capability  ✅                     ║
║    Row    capability 環境      ✅                     ║
║    Grade  per-computation 量   ✅ GIMonad class       ║
║                                                       ║
║  + 宇宙の正当性が保証されている                       ║
║    ✅ 宇宙多相 Phase A (LevelMeta)                    ║
║    ✅ PolyKinds Phase D                               ║
║    ✅ Quick Look impredicativity                      ║
║    ✅ Universe Phase B-C (Level quantification + Max) ║
╚═══════════════════════════════════════════════════════╝
```

## 残存ロードマップ

```
  将来の拡張 (独立して着手可能):

    SMC Phase 4 → UsageSemiring (GradeJoin 切替済み、UsageSemiring class 残)
    -| 演算子 (型レベル構文)
    deriving Eq, Show
    Session types runtime host primitives
```

## ドキュメント構成

| ファイル                               | 内容                                  |
| -------------------------------------- | ------------------------------------- |
| [infrastructure.md](infrastructure.md) | GIMonad + Open Questions              |
| [universe.md](universe.md)             | 宇宙多相 — Phase A-C 完了             |
| [smc.md](smc.md)                       | SMC Path — `Merge`/`***`/`dag` + Mult |
| [independent.md](independent.md)       | 構文拡張、設計判断、sessions          |
