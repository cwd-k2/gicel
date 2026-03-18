# GICEL Roadmap

Current state: **v0.8.** All core features implemented. See `spec/language.md` for the complete specification.

---

## Planned Work

### Multiplicity Enforcement

`@Mult` annotations propagate through the full pipeline (parser → checker → unifier → row fields), but usage is not enforced. `@Linear` compiles but permits multiple uses.

Remaining:

- `checkMultiplicity` enforcement at bind sites (stub ready)
- LUB type family for multiplicity join at branch points

**Known difficulty**: enforcement requires counting usage within row unification simultaneously with pre/post state transition resolution. See [Double grading](#double-grading) below.

### Module System

Selective imports, qualified imports, `_` private names, CLI `--module`, module-qualified Core IR, GHC-style ambiguity detection, and clean stdlib names are implemented. Remaining:

- Selective exports (`module M (x, T(..)) where ...`) — type-level privacy
- Qualified patterns (`case x { Q.Con a -> ... }`)
- Prelude as ordinary module (currently built-in source)
- Stdlib packs as importable modules

---

## Design Fork Points

| Fork Point                                  | Current State                                   | Decision Trigger                                          |
| ------------------------------------------- | ----------------------------------------------- | --------------------------------------------------------- |
| `Row` as built-in kind vs structured-index  | Built-in kind; DataKinds reduces pressure       | Need for non-capability indexing                          |
| Algebraic effects/handlers vs indexed monad | Indexed monad (Atkey); type families compensate | Evidence that handlers better serve the AI agent use case |

---

## Known Theoretical Boundaries

These are not bugs or missing features. They are consequences of GICEL's design coordinate (Atkey indexed monad × row polymorphism × CBPV × Go embedding) that existing literature does not address. Each is currently handled by a practical workaround; the notes below record when the workaround would break.

### Double grading

`Computation pre post a` is indexed by state transition (pre → post). Adding `@Mult` grading creates a second axis: how many times a capability can be used. The two axes interact inside row unification — the pre/post diff must account for both label presence and usage count.

**Current state**: `@Mult` is structural only. No enforcement, so no interaction.
**Triggers**: implementing `checkMultiplicity`. At that point, row unification must solve state-transition and usage constraints simultaneously — a problem not covered by existing graded monad literature (Orchard, Petricek et al.), which treats grading on a single axis.

### Type family / row unification scheduling

Type families can return `Row` values used in `Computation pre post a` indices. This creates a dependency: row unification needs the reduced result, but reduction may need unification to resolve meta-variables first.

```
f :: Computation (SessionDual r) r a
--              ^^^^^^^^^^^^^^^^
-- SessionDual r must reduce before unification can proceed,
-- but r is determined by unification.
```

**Current state**: reduce with fuel=100, leave stuck applications as `TyFamilyApp`, re-reduce after unification solves metas. This works for all current programs.
**Triggers**: multi-stage type family nesting where reduction and unification form a longer cycle. Manifests as fuel exhaustion on a program that should type-check. No reports to date.

### Evidence fiber crossing

The evidence system separates fibers (`Type`, `Constraint`, `Row`). Type families can cross fibers (`Row → Row`, `Type → Constraint`). When a family result feeds into a different fiber's unification, the "fibers are independent" assumption breaks.

**Current state**: the single-pass reduce → unify pipeline handles current cases because family results are fully reduced before entering unification.
**Triggers**: a family whose result is another family application in a different fiber (e.g., a `Row → Constraint` family whose result enters evidence resolution). Would require interleaved reduction across fibers.

---

## Potential Extensions (assessed, not planned)

| Extension        | Classification   | Prerequisite      |
| ---------------- | ---------------- | ----------------- |
| Refinement Types | Phase transition | Separate analysis |
| Dependent Types  | Full restructure | Far future        |
