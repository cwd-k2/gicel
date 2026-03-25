# Language Roadmap

GICEL の言語機能拡張の方向性。バージョン番号は付けない — 各項目は依存関係で順序付ける。

**性格**: library/tooling roadmap が需要駆動・展開観察型であるのに対し、言語機能は**追求型**。型理論・圏論的構造の内在的な帰結を追い、設計空間の論理的な到達点を目指す。外部需要やユーザ friction ではなく、型システムの整合性と表現力の完成が駆動力。したがって direction.md の判断軸（§1 主目的直結、§2 収束速度）とは評価経路が異なり、ここでの優先順序は理論的依存関係で決まる。

## 全体の依存グラフ

```
  ┌────────────────────────────────────────────────────────┐
  │ Independent (並行可能)                                  │
  │   Type operators (:>)                                  │
  │   Type application operator (-<)                        │
  │   L0-a: Data.Set.map bug                               │
  │   L0-c: Optimizer asymmetry                            │
  │   L0-d: Tuple Eq/Ord                                   │
  └────────────────────────────────────────────────────────┘


  SMC Path                         Infrastructure Path
  ──────────                       ────────────────────

  ┌──────────────┐                 ┌──────────────────┐
  │ SMC Phase 1  │                 │ L0-b: Universe   │
  │ Row TF       │                 │   enforcement    │
  └──────┬───────┘                 └────────┬─────────┘
         │                                  │
  ┌──────┴───────┐                 ┌────────┴─────────┐
  │ SMC Phase 2  │                 │ L1: Solver       │
  │ ><            │                 │   foundation     │
  └──────┬───────┘                 └────────┬─────────┘
         │                                  │
  ┌──────┴───────┐                 ┌────────┴─────────┐
  │ SMC Phase 3  │                 │ L2: TF reduction │
  │ dag          │                 │   + Grade errors  │
  └──────┬───────┘                 └────────┬─────────┘
         │                                  │
         │     ┌────────────────────────────┘
         │     │
  ┌──────┴─────┴──┐
  │ SMC Phase 4   │   ← 両パスが合流
  │ Multiplicity  │
  │ = L3: Grade   │
  │   algebra     │
  └───────┬───────┘
          │
  ┌───────┴───────┐                 ┌──────────────────┐
  │ L4: Solver    │                 │ Universe         │
  │   coupling    │                 │   extensions     │
  │   (長期)      │                 │   (post-L0-b)    │
  └───────────────┘                 └──────────────────┘
```

## ドキュメント構成

| ファイル                               | 内容                                                                                      |
| -------------------------------------- | ----------------------------------------------------------------------------------------- |
| [smc.md](smc.md)                       | SMC Path — Phase 1→4、理論的到達点                                                        |
| [infrastructure.md](infrastructure.md) | Infrastructure Path — L0→L4 の実装計画 (universe enforcement, solver 修正, grade algebra) |
| [universe.md](universe.md)             | Universe extensions — polymorphism, cumulativity, non-nullary promotion, Sort₂+           |
| [independent.md](independent.md)       | 独立項目 — 構文拡張、設計判断、convention、session types、far future                      |

## クリティカルパス

**Infrastructure**: L1-b (inert set scope) → L2-a (TF reduction) → L3 (grade algebra)

L0-b (universe) は L3 の前提だが L1/L2 と並行できるため、クリティカルパス上にない。SMC Phase 1-3 は Infrastructure と独立に進行可能。両パスは SMC Phase 4 / L3 で合流する。
