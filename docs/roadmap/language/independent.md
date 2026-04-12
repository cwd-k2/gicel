# Independent Items

他のパスと依存関係を持たない項目。

## Type-Level Syntax Extensions

パーサのみの変更。Core IR・型検査への影響なし。

### Type Operators

infix type alias:

```gicel
type (:>) a b := a b
-- Send :> Recv :> End = Send (Recv (End))
```

Session type DSL の可読性と SMC 型レベル row 操作のために。

## Design Fork Points

| Fork Point                                  | Current State                            | Decision Trigger                            |
| ------------------------------------------- | ---------------------------------------- | ------------------------------------------- |
| `Row` as L1 TyCon vs structured index       | L1 TyCon (unified representation)        | Non-capability row-like indexing の需要     |
| Algebraic effects/handlers vs indexed monad | GIMonad (graded indexed monad, 設計確定) | Handlers が AI agent use case に優る場合    |
| Tensor product kind (`QType`)               | Not present (rows cover current needs)   | Quantum entanglement or non-separable state |

## Intentional Capability Bounds

- **Non-entry top-level bindings must be values** (CBPV discipline, E0291)
- **Compiler-generated names use `$` convention** — Lexer rejects user `$`
- **Tuples are records with `_N` labels** — `(a, b, c)` = `Record { _1: a, _2: b, _3: c }`
- **Exhaustiveness witness is best-effort** — error reporting 専用

## Assessed and Not Adopted

検討した上で採用しなかった設計判断。

| 項目                              | 判断   | 理由                                                                                                                                                 |
| --------------------------------- | ------ | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| 依存型                            | 不採用 | 型レベル計算の複雑度。sandbox の予測可能性を損なう                                                                                                   |
| リファイン型                      | 不採用 | SMT solver 依存を避ける                                                                                                                              |
| 実行系の変更                      | 不採用 | evaluator/VM は安定。型レベルの拡張に閉じる                                                                                                          |
| inferDo GIMonad dispatch          | 見送り | DK bidirectional の方向性制約として理論的に正当。annotation-required は正しい制限。Approach C (context-propagation) が最有力だがユースケースが限定的 |
| Polytype guard (impredicativity)  | 不採用 | 既存の `Just id :: Maybe (∀a. a→a)` が壊れる。Quick Look を選択                                                                                      |
| Semiring law 型レベル enforcement | 不採用 | 依存型の設計制約に抵触。具体値 reduce で現時点の実害なし。→ [infrastructure.md](infrastructure.md)                                                   |
| Tensor product kind (`QType`)     | 未着手 | Full SMC + quantum use case が必要                                                                                                                   |
