# Evidence Sort Design (Level 9)

## Status: Deferred — Design Document

Level 8 (Constraint-Row Isomorphism) is complete. This document records the design for Level 9 (Evidence Sort) and the analysis that informs the deferral decision.

---

## 1. Theoretical Foundation

### 1.1 The Observation

Row types and constraint rows are both **extensible records of evidence** resolved by structural matching:

| Dimension | Row (Capability) | Constraint (Class) |
|-----------|------------------|-------------------|
| Kind | `KRow` | `KConstraint` |
| Structure | `{ label : Type \| tail }` | `{ ClassName Args \| tail }` |
| Semantics | what a computation *has* | what a type *has* |
| Resolution | bidirectional unification | directed evidence search |
| Extension | `ExtendRow` | `ExtendConstraint` |
| Variable | `TyMeta{Kind: KRow}` | `TyMeta{Kind: KConstraint}` |

Both share the same **5-case tail unification** structure:

```
closed ─ closed  →  check exact match
open   ─ closed  →  solve tail to residual
closed ─ open    →  solve tail to residual
open   ─ open    →  fresh variable, split residuals
shared elements  →  recursive unification/resolution
```

This is not accidental. Formally, both are instances of the same categorical construction: **extensible records over a labeling scheme**, parameterized by the label type and the element-matching rule.

### 1.2 Indexed Fibration Perspective

The evidence-direction vision describes an **indexed fibration over Evidence**:

```
Evidence
  ├── Type     : what a value *is*
  ├── Row      : what a computation *has* (capabilities)
  └── Constraint : what a type *has* (properties)
```

Each fiber retains its own structure (label type, matching rule, uniqueness invariants) while sharing a common resolution mechanism. Level 9 identifies the **base** of this fibration: the shared algorithmic skeleton.

### 1.3 Category-Theoretic Framing

The unification algorithm for extensible rows is a **coend computation** over the label set:

```
∫^L  Match(L, entries₁, entries₂) × Tail(residual₁, residual₂)
```

Where `Match` is the entry-matching functor (parameterized by label equality) and `Tail` is the 5-case solver. Row unification uses `Match = label string equality`; constraint resolution uses `Match = className equality + arg unification`. The coend structure is identical; only the `Match` functor differs.

---

## 2. Concrete Design

### 2.1 Unified Type: TyEvidenceRow

```go
// EvidenceKind distinguishes the two fibers.
type EvidenceKind int
const (
    EvidenceCapability  EvidenceKind = iota  // was KRow
    EvidenceConstraint                        // was KConstraint
)

// TyEvidenceRow is the unified extensible record type.
type TyEvidenceRow struct {
    Kind    EvidenceKind
    Entries []EvidenceEntry
    Tail    Type
    S       span.Span
}

// EvidenceEntry is a single entry in an evidence row.
// For capabilities: Label is the capability name, Payload is the capability type.
// For constraints: Label is the class name, Payload encodes args.
type EvidenceEntry struct {
    Label   string        // capability label or class name
    Payload []Type        // [capabilityType] for rows, classArgs for constraints
    Extra   *EntryExtra   // nil for capabilities, non-nil for quantified/variable constraints
    S       span.Span
}

type EntryExtra struct {
    Quantified    *QuantifiedConstraint
    ConstraintVar Type
}
```

### 2.2 Kind Unification

```go
// KEvidence replaces both KRow and KConstraint.
type KEvidence struct {
    Fiber EvidenceKind
}

// Alternatively, keep KRow and KConstraint as aliases:
type KRow = KEvidence{Fiber: EvidenceCapability}
type KConstraint = KEvidence{Fiber: EvidenceConstraint}
```

**Design choice**: Keep `KRow` and `KConstraint` as distinct kinds (not unified) to preserve kind-level error messages and prevent capability/constraint confusion. The unification happens at the **algorithmic** level, not the **kind** level.

### 2.3 Unified Unification Algorithm

```go
func (u *Unifier) unifyEvidenceRows(r1, r2 *TyEvidenceRow) error {
    if r1.Kind != r2.Kind {
        return fmt.Errorf("evidence kind mismatch: %v vs %v", r1.Kind, r2.Kind)
    }

    normalize(r1)
    normalize(r2)

    shared, onlyLeft, onlyRight := classify(r1, r2, r1.Kind)

    for _, m := range shared {
        if err := u.unifyPayloads(m.A, m.B, r1.Kind); err != nil {
            return err
        }
    }

    return u.solveTail5(r1.Tail, r2.Tail, onlyLeft, onlyRight, r1.Kind)
}
```

The `classify` and `solveTail5` functions are parameterized by `EvidenceKind`, selecting:
- **Capability**: label string equality, label uniqueness enforcement
- **Constraint**: className equality + greedy ConstraintKey matching

### 2.4 Migration Path

Phase A: Introduce `TyEvidenceRow` alongside `TyRow` / `TyConstraintRow` (adapter pattern).
Phase B: Migrate traversals (subst, free, equal, pretty, zonk) one at a time.
Phase C: Migrate unification algorithm.
Phase D: Remove `TyRow` and `TyConstraintRow`.

Each phase maintains full backward compatibility. Estimated ~800 lines changed across 12 files.

---

## 3. Value Assessment

### 3.1 Theoretical Value

| Aspect | Benefit |
|--------|---------|
| **Algorithmic clarity** | The shared 5-case structure becomes a single implementation, making the isomorphism explicit |
| **Extension surface** | New evidence fibers (e.g., linear capabilities, graded effects) slot in by adding an `EvidenceKind` variant |
| **Graded Evidence (Level 10)** | Usage tracking requires a unified evidence representation; Level 9 is the prerequisite |
| **Formal correspondence** | Makes the indexed fibration structure machine-readable |

### 3.2 Practical Value

| Aspect | Assessment |
|--------|-----------|
| **Code deduplication** | ~200 lines of parallel traversal code would be unified |
| **Bug surface** | Future changes to row/constraint handling need only one code path |
| **API simplicity** | Internal API shrinks (one row type instead of two) |

### 3.3 Costs

| Aspect | Assessment |
|--------|-----------|
| **Blast radius** | 12 files, ~800 lines changed |
| **Test rewriting** | All row and constraint tests need type construction updates |
| **Performance risk** | Extra `EvidenceKind` dispatch at runtime (negligible) |
| **Asymmetry encoding** | `EntryExtra` field is nil for 100% of capability entries — wasted struct field |
| **Error messages** | Must preserve distinct error language for capabilities vs constraints |

---

## 4. Asymmetry Analysis

The key question for Level 9 is whether the asymmetries between Row and Constraint are **incidental** (and thus unifiable) or **essential** (requiring permanent separation).

### 4.1 Incidental Asymmetries (unifiable)

- **Different field names**: `Fields` vs `Entries`, `Label` vs `ClassName` — purely naming
- **Different constructors**: `RowField` vs `ConstraintEntry` — structural
- **Different kind tags**: `KRow` vs `KConstraint` — can be parameterized
- **Different normalization**: sort by label vs sort by ConstraintKey — same algorithm, different comparator

### 4.2 Essential Asymmetries (require parameterization)

| Asymmetry | Row | Constraint | Impact |
|-----------|-----|-----------|--------|
| **Entry shape** | `(label, Type)` — one payload | `(className, []Type)` — N payloads | `Payload []Type` accommodates both |
| **Quantification** | Never | `forall vars. context => head` | `EntryExtra` required |
| **ConstraintVar** | Never | `ConstraintVar Type` for Dict reification | `EntryExtra` required |
| **Label uniqueness** | Enforced (spec §8) | Not enforced (same class, different args allowed) | Kind-indexed behavior |
| **Matching rule** | Exact label equality | ClassName equality + arg unification | Kind-indexed classifier |
| **Resolution** | Bidirectional unification | Directed evidence search (context → superclass → global) | Different consumer, same structure |

### 4.3 Verdict

The asymmetries are **parameterizable but not eliminable**. The `EntryExtra` field would be nil for all capability entries, and the matching/resolution logic must branch on `EvidenceKind`. This makes the unification a **parameterized abstraction** rather than a **true identification**.

---

## 5. Decision

### Current State: Metastable Equilibrium

The Level 8 implementation achieves the plan's primary goal: constraint resolution and row unification **share the same algorithmic structure**. The isomorphism is manifest in the code — anyone reading `unifyRows` and `unifyConstraintRows` can see they are the same algorithm with different `Match` functors.

### Transition Energy

Level 9 would unify the implementations but at significant cost (12 files, ~800 lines, all tests). The resulting code would be more abstract but not more capable — no new surface features are enabled.

### Trigger Condition

Level 9 becomes justified when:
1. **Level 10 (Graded Evidence)** is prioritized — it requires a unified evidence type for usage annotations
2. **A third evidence fiber** is needed (e.g., linear capabilities, named effects) — the parameterized structure pays off with N>2 fibers
3. **Maintenance burden** becomes measurable — if bugs are introduced by forgetting to update both row and constraint paths in parallel

### Recommendation

**Defer.** The current parallel implementation is the right metastable equilibrium. The design in this document provides the blueprint for when the transition becomes justified.

---

## 6. Commit History (Phases 1–5)

```
Phase 1:  feat(types): add TyConstraintRow and TyEvidence type nodes
Phase 2:  refactor(check): extract evidence classification, add CtxEvidence and grouped deferral
Phase 3:  refactor(check): migrate TyQual to TyEvidence in bidirectional checker
Phase 4:  feat(check): unified evidence resolution algorithm (Level 8)
Phase 5A: feat(check): constraint aliases (Level 1)
Phase 5B: feat(syntax): constraint product syntax (Level 2)
Phase 5C: feat(check): constraint-kinded type parameters (Level 5)
Phase 5D: feat(check): quantified constraints (Level 3)
Phase 5E: feat(check): Dict reification (Level 4)
```

All levels 1–8 are implemented and tested. Level 9 is deferred with this design document as the specification.
