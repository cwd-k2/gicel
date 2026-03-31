# SMC Path

Atkey indexed monad (monad in Prof) から Free †-SMC への段階的拡張。

## Phase 2: Parallel Composition — DONE

`Merge`/`***` — disjoint row merge による parallel composition。型検査器 + VM opcode 実装済み。

## Phase 3: Dagger — DONE

`dag` — pre/post swap。型レベルで保証、runtime は identity。

## Phase 4: Multiplicity Generalization

`GradeAlgebra` を半環に拡張:

```gicel
form UsageSemiring := \(s: Type). {
  zero: s; one: s; plus: s -> s -> s; mult: s -> s -> s
}
```

量子リソース追跡 (probability semiring) や QTT 接続を可能にする。

## Full SMC 到達後

| Concept            | Target                   |
| ------------------ | ------------------------ |
| Foundation         | Free †-SMC               |
| Sequential compose | `;` (do blocks)          |
| Parallel compose   | `Merge` / `***`          |
| Inversion          | `dag` (pre/post swap)    |
| Wire bundles       | Row types                |
| Morphism type      | `Computation pre post a` |
| User row ops       | Merge / Without / Lookup |

**ゼロ構文変更。** `do` blocks = sequential, `Merge`/`***` = parallel, `dag` = inversion。パーサ変更なし。意味論拡張のみ。
