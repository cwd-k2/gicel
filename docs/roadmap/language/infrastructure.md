# Infrastructure Path

## 応用層

### GIMonad (仮名): Graded Indexed Monad

IxMonad の上位概念。Per-computation grade を IxMonad に統合し、FKM 直積圏上の category-graded monad を型クラスとして表現する。

```gicel
form GIMonad := \(g: Kind) (m: g -> Row -> Row -> Type -> Type).
    GradeAlgebra g => {
  gipure: \a (r: Row). a -> m GradeDrop r r a;
  gibind: \a b (e1: g) (e2: g) (r1: Row) (r2: Row) (r3: Row).
              m e1 r1 r2 a -> (a -> m e2 r2 r3 b) -> m (GradeJoin e1 e2) r1 r3 b
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

**代数的条件**: GIMonad grade に使える条件は `(g, GradeDrop, GradeJoin)` が有界結合半束であること。GradeDrop が真の底元 — `GradeJoin(GradeDrop, g) ≡ g` が全ての g で成立。

- Mult: 底元なし (Zero ⊥ Linear) → per-field grade のみ (GIMonad grade に不適格)
- Clearance 等の全順序 lattice: 底元あり → GIMonad grade として適格

**GradeJoin の二重用途**: 有界結合半束では bind (逐次合成) と case (分岐合流) の両方に GradeJoin を共用可能。冪等性が崩れたら半環拡張 (GradeCompose/GradeJoin 分離) が必要 — SMC Phase 4 の UsageSemiring と合流点。

**supermonad との関係**: Supermonad (Bracker-Nilsson) はモナド則下で単一の indexed type に対し GIMonad に退化する。GIMonad は supermonad の正規形であり、不要な自由度 (heterogeneous bind) を持たない分、推論が決定的。

**実装方針**:

- TyCBPV に Grade フィールド追加 (nil = trivial grade で後方互換)
- Core IR (Bind/Pure/Thunk/Force) は変更なし — grade は型レベルで消去
- Optimizer/VM/bytecode は変更なし
- Checker の型レベル処理を拡張: inferPure に GradeDrop、inferBind に GradeJoin 合成
- core.gicel: IxMonad → GIMonad に置換
- do 記法の三パス構造 (built-in CBPV / GIMonad dispatch / Monad dispatch) は維持
- Monad class は別クラスとして存続

**命名**: GIMonad は仮名。最終命名は別途検討。

## Known Divergence

**GradeJoin arity**: GIMonad の `gibind` では `GradeJoin e1 e2` (binary, kind `g -> g -> g`) を前提とする。一方 Prelude の `GradeAlgebra` 宣言は `type GradeJoin :: g -> g` (unary) である。現行の `MultJoin :: Mult -> Mult -> Mult` は実装上動作するが、class 宣言の kind とは不整合。GIMonad 実装時に class 宣言の kind を `g -> g -> g` に修正する必要がある。

## Open Questions

- **`@` の将来**: 型演算子への降格 (`(@) :: Type -> Grade -> Type`)
- **Grade polymorphism**: `\(π: Mult). A @π -> B`
- **□ modality**: `□ g A` の導入で `@` が中置糖衣に降格
