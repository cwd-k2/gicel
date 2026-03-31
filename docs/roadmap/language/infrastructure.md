# Infrastructure Path

## GIMonad: Graded Indexed Monad

IxMonad の上位概念。Per-computation grade を IxMonad に統合し、FKM 直積圏上の category-graded monad を型クラスとして表現する。

```gicel
form GIMonad := \(g: Kind) (m: g -> Row -> Row -> Type -> Type).
    GradeAlgebra g => {
  gipure: \a (r: Row). a -> m GradeDrop r r a;
  gibind: \a b (e1: g) (e2: g) (r1: Row) (r2: Row) (r3: Row).
              m e1 r1 r2 a -> (a -> m e2 r2 r3 b) -> m (GradeCompose e1 e2) r1 r3 b
}
```

**モナド階層**:

```
            GIMonad (g, pre, post)
           ／              ＼
    GradedMonad (g)      IxMonad (pre, post)
           ＼              ／
            Monad
```

IxMonad = GIMonad where g = () (自明な grade)。後方互換。

### GradeAlgebra: Compose/Join 分離

GradeAlgebra は3つの associated type を持つ:

```gicel
form GradeAlgebra := \(g: Kind). {
  type GradeCompose :: g -> g -> g;  -- bind の逐次合成 (半環の +)
  type GradeJoin    :: g -> g -> g;  -- case の分岐結合 (束の ∨)
  type GradeDrop    :: g;            -- pure の単位元   (半環の 0)
  -- default: GradeJoin = GradeCompose (冪等 grade 用)
}
```

**代数的条件**:

- `(g, GradeDrop, GradeCompose)` は monoid — 結合律, 単位元
- `(g, GradeDrop, GradeJoin)` は有界結合半束 — 冪等, 結合律, 交換律, 単位元
- `GradeCompose x y ≤ GradeJoin x y` (上界条件)
- 冪等 grade では `GradeCompose = GradeJoin` に退化 (デフォルト実装で吸収)

**設計根拠** (2026-03-30 確定): bind の逐次合成 (半環の加算) と case の分岐結合 (束の join) は意味論的に異なる操作。冪等な grade (Mult) では偶然一致するが、カウンティング半環等で分離が必要。class 定義は一度出すと変更コストが高いため、最初から分離する。

### 実装方針

- GradeAlgebra の associated type を 2→3 に増加 (GradeCompose 追加)
- 既存の `impl GradeAlgebra Mult` に `GradeCompose := MultJoin` を追加
- TyCBPV に Grade フィールド追加 (nil = trivial grade で後方互換)
- Core IR (Bind/Pure/Thunk/Force) は変更なし — grade は型レベルで消去
- Optimizer/VM/bytecode は変更なし
- Checker の型レベル処理を拡張: inferPure に GradeDrop、inferBind に GradeCompose、case 分岐に GradeJoin
- core.gicel: IxMonad → GIMonad に置換
- do 記法の三パス構造 (built-in CBPV / GIMonad dispatch / Monad dispatch) は維持
- Monad class は別クラスとして存続

### supermonad との関係

Supermonad (Bracker-Nilsson) はモナド則下で単一の indexed type に対し GIMonad に退化する。GIMonad は supermonad の正規形であり、不要な自由度 (heterogeneous bind) を持たない分、推論が決定的。

### 理論的リスク

- ~~**Impredicativity**~~: **解決済** — Quick Look (Serrano et al. 2020) 実装。multi-arg constructor の impredicative instantiation が動作
- GradeJoin 半環分離: **確定済** (上記参照)

## Open Questions

- **`@` の将来**: 型演算子への降格 (`(@) :: Type -> Grade -> Type`)
- **Grade polymorphism**: `\(π: Mult). A @π -> B`
- **□ modality**: `□ g A` の導入で `@` が中置糖衣に降格
