# SMC Path — Complete

Atkey indexed monad (monad in Prof) から Free †-SMC への段階的拡張。全フェーズ完了。

## Phase 2: Parallel Composition — DONE

`Merge`/`***` — disjoint row merge による parallel composition。

## Phase 3: Dagger — DONE

`dag` — pre/post swap。型レベルで保証、runtime は identity。

## Phase 4: Multiplicity Generalization — DONE

- Step 1: `joinGrades` を `GradeJoin` algebra-aware に切替
- Step 2: `UsageSemiring` class 定義 + `Trivial`/`Mult` instances
- Semiring laws は文書化のみ（型レベル enforcement は設計制約により不採用）

## Full SMC 到達

| Concept            | Target                   |
| ------------------ | ------------------------ |
| Foundation         | Free †-SMC               |
| Sequential compose | `;` (do blocks)          |
| Parallel compose   | `Merge` / `***`          |
| Inversion          | `dag` (pre/post swap)    |
| Wire bundles       | Row types                |
| Morphism type      | `Computation pre post a` |
| User row ops       | Merge / Without / Lookup |
| Grade algebra      | GradeAlgebra (type-level) + UsageSemiring (value-level) |
