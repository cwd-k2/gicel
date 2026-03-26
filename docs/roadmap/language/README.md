# Language Roadmap

GICEL の言語機能拡張の方向性。バージョン番号は付けない — 各項目は依存関係で順序付ける。

**性格**: library/tooling roadmap が需要駆動・展開観察型であるのに対し、言語機能は**追求型**。型理論・圏論的構造の内在的な帰結を追い、設計空間の論理的な到達点を目指す。外部需要やユーザ friction ではなく、型システムの整合性と表現力の完成が駆動力。したがって direction.md の判断軸（§1 主目的直結、§2 収束速度）とは評価経路が異なり、ここでの優先順序は理論的依存関係で決まる。

## 全体の依存グラフ

```
  ┌────────────────────────────────────────────────────────┐
  │ Independent (並行可能)                                 │
  │   Type operators (:>)                                  │
  │   Type application operator (-<)                       │
  │   L0-a: Data.Set.map bug                               │
  │   L0-c: Optimizer asymmetry                            │
  │   L0-d: Tuple Eq/Ord                                   │
  └────────────────────────────────────────────────────────┘


  SMC Path (改訂)                  Infrastructure Path
  ────────────────                 ────────────────────
                                   ✓ L0-b: Universe
  ┌──────────────┐                 ✓ L1: Solver + Equality
  │ SMC Phase 2  │                 ✓ L2-b: Grade errors
  │ >< (内部     │
  │   row merge) │                 ┌─────────────────┐
  └──────┬───────┘                 │ L2-a: TF reduce │
         │                         │   on-demand     │
  ┌──────┴───────┐                 └────────┬────────┘
  │ SMC Phase 3  │                          │
  │ dag          │                          │
  └──────┬───────┘                          │
         │                                  │
         │     ┌────────────────────────────┘
         │     │
  ┌──────┴─────┴──┐
  │ SMC Phase 4   │   ← SMC + Infrastructure 合流
  │ Multiplicity  │
  │ = L3: Grade   │
  │   algebra     │
  └───────┬───────┘
          │
  ┌───────┴──────────────────────────────────────────────┐
  │ L3 後の応用層 (並行可能)                             │
  │                                                      │
  │   Named capabilities (ゲート → 名前付きリソース)     │
  │   Per-computation grade (double grading)             │
  │   Row TF (Phase 1: Merge/Without/Lookup)             │
  └───────┬──────────────────────────────────────────────┘
          │
  ┌───────┴───────┐                 ┌──────────────────┐
  │ L4: Solver    │                 │ Universe         │
  │   coupling    │                 │   extensions     │
  │   (長期)      │                 │   (post-L0-b     │
  └───────────────┘                 │   + L1/L2)       │
                                    │                  │
                                    │ Non-nullary prom │
                                    │ Univ polymorphism│
                                    │ Cumulativity     │
                                    └──────────────────┘
```

## ドキュメント構成

| ファイル                               | 内容                                                                                                   |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| [smc.md](smc.md)                       | SMC Path — Phase 1-4。`><`/`dag` は Phase 1 なしで着手可能、Phase 4 で L3 と合流                       |
| [infrastructure.md](infrastructure.md) | Infrastructure Path — L0→L4 + L3 後の応用層 (named capabilities, per-computation grade, grade algebra) |
| [universe.md](universe.md)             | Universe extensions — non-nullary promotion, polymorphism, cumulativity, Sort₂+                        |
| [independent.md](independent.md)       | 独立項目 — 構文拡張、設計判断、convention、session types、far future                                   |

## クリティカルパス

**Infrastructure**: ~~L1-b~~ → ~~L1-c~~ → L2-a (TF reduction) → L3 (grade algebra)

L0-b, L1-a/b/c, L2-b は完了。残りは L2-a と L3。

## SMC Path 改訂 (2025-03)

`><` (parallel composition) と `dag` は **Phase 1 (Row TF) なしで実装可能**。Row merging は既存の `classifyFields` アルゴリズムを型検査器内部で実行すればよく、`Merge` を型族としてユーザに公開する必要はない。これにより:

- Phase 2/3 は Infrastructure Path と独立に着手可能
- Phase 1 (Row TF) は `><` の前提ではないが、ユーザが `Merge` 等を型シグネチャで使うために実施する
- 両パスは SMC Phase 4 / L3 で合流

## L3 後の応用層 (2025-03 追記)

L3 (grade algebra) 完了後に以下が並行着手可能になる:

- **Named capabilities**: ゲートモデル (`{ array: () | r }`) から名前付きリソースモデル (`{ counts: Array Int @Linear | r }`) への移行。分離論理・QTT・†-SMC の自然な対象。詳細は [infrastructure.md](infrastructure.md)。
- **Per-computation grade**: row field 上の per-resource grade に加え、Computation 型自体に grade を付与する double grading。計算量追跡・N+1 検出等のユースケース。詳細は [infrastructure.md](infrastructure.md)。
- **Row TF (Phase 1)**: `Merge`/`Without`/`Lookup` 型族。`><` の前提ではないが、ユーザが `Merge` を型シグネチャで使えるようにする。詳細は [smc.md](smc.md)。
