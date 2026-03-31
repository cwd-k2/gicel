# Independent Items

他のパスと依存関係を持たない項目。並行して着手可能。

## Type-Level Syntax Extensions

パーサのみの変更。Core IR・型検査への影響なし。

### Type Operators

infix type alias:

```gicel
type (:>) a b := a b
-- Send :> Recv :> End = Send (Recv (End))
```

Session type DSL の可読性と SMC 型レベル row 操作のために。

### Type Application Operator (`-|`)

組み込み右結合型適用:

```gicel
Map String -| List -| Maybe -| Int
= Map String (List (Maybe Int))
```

`->` とは **対ではなく対比**。`->` は関数型構築（矢印）、`-|` は適用の区切り（壁）。異なる操作であることが記号自体から伝わる。turnstile `⊢` の連想 — 「ここから先が引数」という境界の意味論。row の `|` と意味が通底する。

## Design Fork Points

| Fork Point                                  | Current State                            | Decision Trigger                            |
| ------------------------------------------- | ---------------------------------------- | ------------------------------------------- |
| `Row` as L1 TyCon vs structured index       | L1 TyCon (unified representation)        | Non-capability row-like indexing の需要     |
| Algebraic effects/handlers vs indexed monad | GIMonad (graded indexed monad, 設計確定) | Handlers が AI agent use case に優る場合    |
| Tensor product kind (`QType`)               | Not present (rows cover current needs)   | Quantum entanglement or non-separable state |

### Tensor Product Kind

Row merging (separable composition) は SMC で提供されるが、quantum entanglement (inseparable composition) には真のテンソル積 `A ⊗ B` が必要。Row label は addressable (projectable)、tensor product は inseparable (non-projectable)。Classical capability = rows、quantum entanglement = tensors — kind レベルで分離。SMC 完成まではテンソル積なしで完結する。

## Intentional Capability Bounds

### Non-entry top-level bindings must be values (CBPV discipline)

非 entry の top-level binding に bare `Computation` 型は不可 (E0291)。`thunk` で `Thunk` 型に変換する。entry point (default `main`) のみ免除。

### Compiler-generated names use `$` convention

辞書コンストラクタ等は `$` を含む。Lexer はユーザ識別子の `$` を拒否し衝突を防止。

### Tuples are records with `_N` labels

`(a, b, c)` は `Record { _1: a, _2: b, _3: c }` に desugar。

### Exhaustiveness witness reconstruction is best-effort

witness formatting は best-effort shape recovery。error reporting 専用、semantic 判断には不使用。

## Session Types Maturity

Session types は check-only で正しく動作する。Runtime 実行には host primitive (send/recv/close) が必要。

課題:

- session の CLI example は削除済み（check-only で実行不可）。Go example (examples/go/session) のみ
- structuring rule (bare Computation prohibition) が session 文脈で十分説明されていない
- runtime 対応は host primitive 設計を伴う

対応方針: check-only としての完成度を先に上げ、runtime 対応は [smc.md](smc.md) Phase 2 (parallel composition) と合わせて検討。

## Intentionally Not Planned

| Extension    | Reason                                             |
| ------------ | -------------------------------------------------- |
| 依存型       | 型レベル計算の複雑度。sandbox の予測可能性を損なう |
| リファイン型 | 同上。SMT solver 依存を避ける                      |
| 実行系の変更 | evaluator/VM は安定。型レベルの拡張に閉じる        |

## Far Future (assessed, not planned)

| Extension                     | Category    | Status                                          |
| ----------------------------- | ----------- | ----------------------------------------------- |
| Tensor product kind (`QType`) | Type system | Not planned. Full SMC + quantum use case needed |
