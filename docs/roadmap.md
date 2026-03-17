# GICEL Roadmap

Current state: **v0.7 + Type System Extensions.** All core features implemented. Type system extended with type families, associated types, functional dependencies, graded evidence, divergent post-states, session types, and data families.

See `spec/language.md` for the complete language specification.

---

## Completed: Type System Extensions (2026-03-17)

9 extensions implemented on branch `feature/type-system-extensions`:

- **Type families**: Closed, recursive (fuel 100), constraint families, injectivity verification
- **Associated types**: In class declarations, equation collection from instances
- **Functional dependencies**: `| a -> b` on MPTCs, improvement in instance resolution
- **Graded Evidence**: `@Mult` syntax on row fields, `RowField.Mult` through full pipeline
- **Divergent post-states**: Case branches with different post-states joined via intersection
- **Session types**: Library feature on recursive TF + DataKinds
- **Data families**: Constructor mangling, exhaustiveness checking

---

## Graded Evidence — Remaining Work

Usage tracking (linear/affine/unrestricted) structural foundation complete. Remaining:

- `checkMultiplicity` enforcement at bind sites (stub ready)
- LUB type family integration for multiplicity join at branch points

---

## Module System Evolution

- Prelude becomes an ordinary module (currently built-in source)
- Stdlib packs become importable modules
- Selective exports
- Qualified imports

---

## Open Design Fork Points

| Fork Point                                         | Current State         | Decision Trigger                                                     |
| -------------------------------------------------- | --------------------- | -------------------------------------------------------------------- |
| `Row` as built-in kind vs general structured-index | Built-in kind         | Need for non-capability indexing                                     |
| Algebraic effects/handlers vs indexed monad        | Indexed monad (Atkey) | Evidence that handler-based approach better serves AI agent use case |

Resolved fork points:

- ~~Branching with divergent post-states~~ → **Resolved**: intersection join (2026-03-17)
- ~~Usage judgment (linear/affine)~~ → **Resolved**: `@Mult` annotation + `RowField.Mult` (2026-03-17)
- ~~Type families~~ → **Resolved**: full implementation (2026-03-17)

---

## Potential Extensions (assessed, not planned)

| Extension        | Classification   | Prerequisite      |
| ---------------- | ---------------- | ----------------- |
| Refinement Types | Phase transition | Separate analysis |
| Dependent Types  | Full restructure | Far future        |
