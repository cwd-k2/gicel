# GICEL Roadmap

Current state: **First release ready.** All planned features implemented — type system (bidirectional DK, higher-rank, row polymorphism), type classes (10 classes, dictionary passing), GADTs with existentials, DataKinds, HKT (kind variables, kind unification, poly-kinded classes), records and tuples (row-polymorphic, `!#` projection), evidence sort (unified fiber architecture), module system, host boundary (Engine → Runtime → Evaluator), 8 stdlib packs, Core IR optimizer (algebraic simplifications + pack-registered fusion rules), CLI tool.

See `spec/language.md` for the complete language specification.

---

## Graded Evidence

Usage tracking (linear/affine/unrestricted) over all evidence fibers. The unified evidence architecture (`TyEvidenceRow` with `EvidenceFiber` interface) provides the foundation.

Connects to the open fork point: usage judgment for linear/affine capabilities.

---

## Module System Evolution

- Prelude becomes an ordinary module (currently built-in source)
- Stdlib packs become importable modules
- Selective exports
- Qualified imports

---

## Open Design Fork Points

| Fork Point                                         | Current State              | Decision Trigger                                                                   |
| -------------------------------------------------- | -------------------------- | ---------------------------------------------------------------------------------- |
| Branching with divergent post-states               | Equal post-states required | User demand for `if`-like branching where branches modify capabilities differently |
| `Row` as built-in kind vs general structured-index | Built-in kind              | Need for non-capability indexing (e.g., session types)                             |
| Usage judgment (linear/affine capabilities)        | Not implemented            | Graded Evidence design                                                             |
| Algebraic effects/handlers vs indexed monad        | Indexed monad (Atkey)      | Evidence that handler-based approach better serves the AI agent use case           |

---

## Potential Extensions (assessed, not planned)

| Extension        | Classification   | Prerequisite                |
| ---------------- | ---------------- | --------------------------- |
| Type Families    | Phase transition | Substantial checker changes |
| Refinement Types | Phase transition | Separate analysis           |
| Dependent Types  | Full restructure | Far future                  |
