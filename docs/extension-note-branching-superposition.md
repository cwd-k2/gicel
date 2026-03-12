# Extension Note: Branching Superposition (Solution A+B)

| Field       | Value                                          |
|-------------|------------------------------------------------|
| Status      | Extension note — not scheduled for implementation |
| Relates to  | Open fork point #1 (branching with divergent post-states) |
| Depends on  | [Non-Linear Effect Composition](./non-linear-effect-composition.md) (Solutions A, B) |
| Context     | [Theoretical Positioning](./theoretical-positioning.md) (Section 6) |
| Date        | 2026-03-12                                     |

---

## 1. Motivation

The branching problem in Gomputation is well-characterized: when two branches of a `case` produce different post-states, the linear composition law of `bind` has no canonical way to continue. The current design adopts Solution A (equal post-states via unification), which is sound and requires no additional type machinery. Solution B (join semilattice on states) is more expressive but introduces a lattice structure over the entire capability-state space — a commitment that is premature for v0.

This note describes a **superposition** of the two: Solution A remains the default; Solution B activates only where the host has explicitly declared a join operation on capability states. The system never requires the lattice; it exploits it when offered.

---

## 2. Design

### 2.1 Default: Solution A unchanged

The typing rule for computation-level `case` remains:

```text
Gamma |- b_i : Computation r1 r2 a     for all branches i
----------------------------------------------------------
Gamma |- case s of { ... } : Computation r1 r2 a
```

All branches must unify to a single `r2`. This is the base rule and is never weakened.

### 2.2 Opt-in: host-declared join semilattice

The host may declare, per capability state type, a join semilattice via the Engine API:

```go
eng.DeclareStateJoin("DBState", func(a, b string) (string, bool) {
    // Returns (join, ok).
    // ok = false means no join exists for this pair.
    switch {
    case a == b:
        return a, true
    case (a == "Opened" && b == "Upgraded") || (b == "Opened" && a == "Upgraded"):
        return "Active", true
    default:
        return "", false
    }
})
```

The host is responsible for ensuring the function satisfies the semilattice laws (idempotency, commutativity, associativity). The engine does not verify these — it trusts the host, consistent with the handler-as-host principle.

### 2.3 Checker behavior: unification first, join second

When checking a `case` expression with branches producing post-states `r2_1, ..., r2_n`:

1. **Attempt unification.** If all `r2_i` unify, succeed with the unified row. This is Solution A. No lattice consulted.
2. **If unification fails and a lattice exists**, compute the per-field join. If every field that differs has a declared join, succeed with the joined row. This is Solution B, scoped to fields with lattice structure.
3. **If any differing field lacks a join**, report a type error. The error message should indicate which fields diverged and suggest either equalizing or declaring a join.

The checker never silently loses precision. Step 2 only fires on unification failure, so programs that already work under Solution A are entirely unaffected.

### 2.4 Per-field join on rows

The join operates field-by-field on the capability row. Fields whose state types have a declared lattice are joined; all other fields must unify exactly.

Example. Suppose `DBState` has the lattice above, and `LogState` has none:

```text
Branch 1 post: { db : DB Opened,   log : Log Ready | r }
Branch 2 post: { db : DB Upgraded, log : Log Ready | r }
```

- `db`: `Opened` and `Upgraded` differ. Lattice exists. Join = `Active`.
- `log`: `Ready` and `Ready` unify. No lattice needed.
- `r`: row tail unifies.

Result:

```text
Joined post: { db : DB Active, log : Log Ready | r }
```

If branch 2 had `log : Log Flushed` instead, the checker would reject: `LogState` has no declared join, and `Ready != Flushed`.

---

## 3. Semantic Consequences

### 3.1 Precision loss is explicit

The join `Active` is strictly coarser than either `Opened` or `Upgraded`. After the branch, only operations whose pre-state accepts `Active` are available. The host defines what those are — if no primitives accept `DB Active` as a pre-state, the joined post-state is a dead end. This is intentional: the host controls the protocol graph.

### 3.2 Categorical reading

In the language of the theoretical positioning document (Section 6), this extension equips the grading category with **optional coproduct structure** on selected hom-sets. Where coproducts exist, the category supports cospan resolution; where they do not, strict equality remains the only path.

This is a refinement, not a phase transition. The grading category gains structure without changing identity. The Katsumata test (theoretical-positioning.md, Section 8.4) classifies this as a category enrichment — the same tier as adding coproducts to a category that already has composition and identity.

### 3.3 Interaction with `bracket`

The `bracket` combinator requires the body to be state-preserving (`r2 = r2`). Join does not interfere: if the body branches internally and the branches join to `r2'`, the bracket constraint becomes `r2' = r2`, which must hold by unification. The lattice is irrelevant inside bracket bodies unless the host has declared `r2` itself as a join. No special interaction.

---

## 4. What This Does Not Do

- **Does not introduce dependent post-states.** The join is computed at type-checking time from static state labels, not from runtime values.
- **Does not require the host to declare a lattice.** Absence of a lattice means Solution A applies. The superposition is strictly backward-compatible.
- **Does not change `bind`.** The type of `bind` remains `Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b`. The join affects only the rule that determines `r2` at branch points.
- **Does not validate lattice laws.** The host is trusted. An incorrect join function produces an unsound protocol — the same class of risk as an incorrect `PrimImpl`.

---

## 5. Open Questions

1. **Should the join function operate on state labels (strings) or on types?** The sketch above uses strings, which matches the current `CapEnv` representation. A type-level join would be more principled but requires richer type-level computation.

2. **Nested joins.** If a branch contains a sub-branch, both of which require joins, the checker must compute the join iteratively. The semilattice laws guarantee that the order of pairwise joins does not matter, but the implementation should verify convergence.

3. **Error quality.** When a join fails (no declared lattice for a differing field), the error should clearly distinguish "unification failed and no lattice exists" from "unification failed and the lattice does not cover this pair." These are different remediation paths.

4. **Discovery.** Should the engine expose a query for "what joins are declared"? This would help tooling and debugging but is an API surface decision, not a type system question.

---

## 6. Relation to the Growth Path

The non-linear effect composition document (Section 9.5) outlines a growth path:

- **v0**: Solution A (equal post-states) — current position
- **v0.x**: Usage discipline (affine/linear) — orthogonal
- **v1**: Evaluate dependent post-states or lattice structure

This extension note sits between v0 and v1. It is a minimal lattice introduction that does not require dependent types, does not change the `bind` signature, and does not alter the host boundary beyond one new `Engine` method. If the branching problem becomes a practical pressure point before v1, this superposition is the lowest-energy transition available.

If the project later moves to dependent post-states (the Idris path), the lattice becomes redundant — dependent indexing subsumes joins. The lattice would remain as a convenience (the host declares a join rather than writing dependent types), but it would no longer be the primary mechanism.

---

## References

- [Non-Linear Effect Composition](./non-linear-effect-composition.md) — Sections 2.2 (Solution A), 2.3 (Solution B), 7.4 (graded monads and coproducts), 9.5 (growth path)
- [Theoretical Positioning](./theoretical-positioning.md) — Section 6 (lattice structure and branching), Section 8.4 (Katsumata test)
- Katsumata, S. (2014). Parametric effect monads and semantics of effect systems. POPL.
- Orchard, D., Wadler, P. & Yoshida, N. (2020). Unifying graded and parameterised monads. MSFP.
