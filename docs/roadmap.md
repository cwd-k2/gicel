# Gomputation Roadmap

Current state: **v0.5** (spec) / **Phase 5 complete** (implementation through type checker + evidence system Levels 1-8). Literals (Int, String, Rune) and the `Lit` Core former (14 total) are already implemented.

---

## v0.6 — Literals, Operators, Standard Library Packs

**Status**: Implemented.

### Scope

| Feature | Description |
|---------|-------------|
| Integer literals | `[0-9]+` → `Int` (monomorphic; `Num a => a` later) |
| String literals | `"..."` → `String` (with escape sequences) |
| Rune literals | `'.'` → `Rune` (with escape sequences) |
| Operator definition | `(+) :: ...` / `(+) := ...` — parenthesized operator names |
| Stdlib packs | `engine.Use(stdlib.Num)` — Go-side pack registration |
| Std.Num | `Num` class, `Eq`/`Ord` Int, arithmetic operators |
| Std.Str | `Eq`/`Ord`/`Semigroup`/`Monoid` String, `Eq`/`Ord` Rune |
| Std.Fail | `fail` capability, `fromMaybe`, `fromResult` |
| Std.State | `get`/`put` capabilities |

### Key Design Decisions

- Types (`Int`, `String`, `Rune`) are checker built-ins; operations come from stdlib packs.
- Runtime representation: `HostVal` wrapping Go values (`int64`, `string`, `rune`). No new Value variants.
- Pack = `func(*Engine) error` — bundles `RegisterType` + `RegisterModule` + `RegisterPrim`. Pack is not a module; it is a Go-side configuration action.
- Effects (Maybe/State/Reader/Writer) are encoded as capability row patterns, not monad transformers.

---

## v0.7+ — Row-Polymorphic Records

**Status**: Design spec complete, implementation deferred.

### Design

- `TyRecord : Row → Type` — record types parameterized by rows.
- Records and capabilities share `Row` kind — row variables, unification, and polymorphic functions apply uniformly.
- Surface syntax: `{ x = 1, y = True }` (literal), `r.x` (projection), `{ r | x = 42 }` (update).
- Elaboration to `PrimOp` nodes; no new Core IR formers.
- Runtime: `map[string]Value` (no copy-on-write needed — records are pure values).

### Open Questions

- Anonymous-only vs named records (recommendation: start anonymous).
- Record pattern matching (recommendation: support for ergonomics).
- Runtime field ordering for deterministic serialization.

---

## Evidence Sort (Level 9)

**Status**: Deferred — design document complete. Trigger conditions defined.

### Concept

Unify `TyRow` and `TyConstraintRow` into a single `TyEvidenceRow` parameterized by `EvidenceKind`. Both are instances of the same categorical construction: extensible records over a labeling scheme with a shared 5-case tail unification structure.

### Trigger Conditions

Level 9 becomes justified when:
1. **Level 10 (Graded Evidence)** is prioritized — requires unified evidence type for usage annotations.
2. **A third evidence fiber** is needed (e.g., linear capabilities, named effects).
3. **Maintenance burden** from parallel Row/Constraint implementations becomes measurable.

### Current Equilibrium

The Level 8 implementation achieves the isomorphism at the algorithmic level: `unifyRows` and `unifyConstraintRows` are visibly the same algorithm with different match functors. Unifying the implementations (~800 lines across 12 files) would be more abstract but not more capable.

---

## Graded Evidence (Level 10)

**Status**: Future — no design document yet.

Usage tracking (linear/affine/unrestricted) over all evidence fibers. Requires Level 9 as prerequisite.

Connects to open fork point: usage judgment for linear/affine capabilities.

---

## Open Design Fork Points

| Fork Point | Current State | Decision Trigger |
|------------|---------------|-----------------|
| Branching with divergent post-states | Deferred (equal post-states required) | User demand for `if`-like branching where branches modify capabilities differently |
| `Row` as built-in kind vs general structured-index | Built-in kind | Need for non-capability indexing (e.g., session types) |
| Usage judgment (linear/affine capabilities) | Not implemented | Graded Evidence (Level 10) design |
| Algebraic effects/handlers vs indexed monad | Indexed monad (Atkey) | Evidence that handler-based approach better serves the AI agent use case |

---

## Phase 6 — Host Boundary (not yet implemented)

Module loading from source, prelude as loadable module, full host API surface.

## Phase 7 — Integration (not yet implemented)

CLI (`cmd/gpc`), REPL, formatter, prelude packaging.

---

## Long-Term Vision

### Evidence Unification

Type, Row, and Constraint converge toward a single Evidence abstraction — an indexed fibration where each fiber (what a value *is*, what a computation *has*, what a type *has*) retains its structure while sharing resolution mechanisms.

### Module System

Prelude becomes an ordinary module. Stdlib packs become importable modules. Selective exports. Qualified imports.

### Potential Extensions (assessed, not planned)

| Extension | Verdict | Prerequisite |
|-----------|---------|-------------|
| Higher-Kinded Types | Practical with kind inference | Kind system maturity |
| Type Families | Phase transition — not v0 | Substantial checker changes |
| Refinement Types | Not v0 | Separate analysis |
| Dependent Types | Upper bound of the design space | Far future |
