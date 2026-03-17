# GICEL Roadmap

Current state: **v0.7.** All core features implemented, including type families, associated types, functional dependencies, data families, multiplicity annotations, divergent post-states, and session types.

See `spec/language.md` for the complete language specification.

---

## Multiplicity Enforcement

Usage tracking (linear/affine/unrestricted) has structural foundation in place (`@Mult` annotation, `RowField.Mult` through the full pipeline). Remaining:

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

| Fork Point                                         | Current State                                          | Decision Trigger                                                         |
| -------------------------------------------------- | ------------------------------------------------------ | ------------------------------------------------------------------------ |
| `Row` as built-in kind vs general structured-index | Built-in kind; reduced pressure via DataKinds          | Need for non-capability indexing (e.g., session types)                   |
| Algebraic effects/handlers vs indexed monad        | Indexed monad (Atkey); type families reduce motivation | Evidence that handler-based approach better serves the AI agent use case |

---

## Research Directions

Type families introduce type-level computation into GICEL's unique coordinate (Atkey indexed monad × row polymorphism × CBPV × Go embedding), opening research directions specific to this intersection:

- **Double grading**: Adding multiplicity grades to `Computation pre post a` creates a doubly-graded structure where state transition and usage discipline interact, mediated by row unification.
- **Evidence fiber interaction**: Type families cross fiber boundaries (`Type → Constraint`, `promoted kind → Row`). Where fiber independence ends and fiber interaction begins is specific to GICEL's evidence architecture.
- **Reduction and unification scheduling**: When a type family returns a `Row` used in a unification target, type family reduction and row unification become interdependent — a non-trivial scheduling problem.

---

## Potential Extensions (assessed, not planned)

| Extension        | Classification   | Prerequisite      |
| ---------------- | ---------------- | ----------------- |
| Refinement Types | Phase transition | Separate analysis |
| Dependent Types  | Full restructure | Far future        |
