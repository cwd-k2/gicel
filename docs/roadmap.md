# GICEL Roadmap

Current state: **v0.13** — foundation hardening complete. Branch `rearch/v0.14` is active development.
Specification: `docs/spec/language.md`. Changelog: `CHANGELOG.md`.

---

## OutsideIn(X) Extension Path

The checker architecture supports incremental migration toward OutsideIn(X). Current level: **L3** (worklist + inert set).

| Level  | Status           | Description                                  |
| ------ | ---------------- | -------------------------------------------- |
| L0–L2  | done             | Ad-hoc reduction → stuck index → rework loop |
| **L3** | done (v0.12)     | Worklist + inert set, constraint AST         |
| **L4** | **next (v0.14)** | Touchability, implication constraints        |

What L4 adds: meta level enforcement (touchability), local assumptions (implication constraints), GADT given simplification of stuck families. Required for row-level type family stuck constraint management.

---

## Release Plan

### v0.14 — Row-Level Type Families + OutsideIn(X) L4

Two extensions developed together. Row-level TF and L4 touchability/given simplification are co-dependent.

#### Type Family Reduction: Exponential Branching Fix

`reduceFamilyAppsN` branches exponentially for patterns like `Grow a = Pair (Grow a) (Grow a)`. `MaxReductionWork` bounds step count but not branching factor. The `Merge` type family hits this when recursively expanding over open row tails — prerequisite fix for row-level TF.

**Approach**: shared-basis reduction (reduce identical TyApp once and memoize), or explicit branching limit.

#### OutsideIn(X) L4

Touchability (meta level enforcement), implication constraints (local assumptions), GADT given simplification.

When `Merge` encounters an open row tail, a stuck `CtFunEq` is generated. L4 touchability tracks which scope can solve each meta, preventing unnecessary re-activation. GADT pattern matches supplying givens that simplify stuck `Merge` also require L4.

#### Row-Level Type Families (SMC Phase 1)

Expose row merging, splitting, and lookup as type-level operations via type family pattern matching.

```gicel
type Merge (r1: Row) (r2: Row) :: Row    -- merge two disjoint rows
type Without (l: Type) (r: Row) :: Row   -- remove a label
type Lookup (l: Type) (r: Row) :: Type   -- look up a label
```

`Merge` reduction reuses the existing `classifyFields` algorithm (shared/onlyA/onlyB classification) exposed as a type family. Duplicate labels are type errors. Open row tails produce stuck constraints (`CtFunEq` entering the worklist, resolved by L4 re-activation).

**Implementation**: add `TyEvidenceRow` pattern to `MatchTyPattern()` in `internal/compiler/check/family/reduce.go`, reusing the existing row unification algorithm.

**Resolves**: "Row operations not exposed at type level" — the root cause preventing `><` while `bind` works.

#### Type-Level Syntax Extensions

Parser-only changes. No impact on Core IR or type checker.

**Type Operators** — infix type aliases:

```gicel
type (:>) a b := a b
-- Send :> Recv :> End = Send (Recv (End))
```

Motivated by session type DSL readability and upcoming SMC type-level row operations.

**Type Application Operator (`-<`)** — built-in right-associative type application:

```gicel
Map String -< List -< Maybe -< Int
= Map String (List (Maybe Int))

-- juxtaposition > -< > ->
String -> Map String -< List -< Int
= String -> (Map String (List Int))
```

Visually pairs with `->` (precedent: Haskell arrow notation `-<` = arrow application).

---

### v0.15 — Parallel Composition + Dagger (SMC Phase 2-3)

Builds the remaining two Free SMC composition operations on top of v0.14's type-level row operations.

#### Parallel Composition (SMC Phase 2)

```gicel
infixr 3 ><
(><) :: Computation pre₁ post₁ a -> Computation pre₂ post₂ b
     -> Computation (Merge pre₁ pre₂) (Merge post₁ post₂) (a, b)
```

Host-provided primitive. Runtime: splits capability environments, runs both computations independently, merges result environments. Type checking uses the `Merge` type family to verify row composition.

**Implementation**: `PrimOp` registration + built-in `Merge` reduction. Type checker changes are included in v0.14.

#### Dagger (SMC Phase 3)

```gicel
type Gate pre post := Computation pre post ()
dag :: Gate pre post -> Gate post pre
```

Swaps pre/post. Involution `dag (dag f) = f` holds structurally (double swap). Contravariance `dag (f ; g) = dag g ; dag f` is guaranteed by the host implementation.

**Implementation**: `PrimOp` registration. Type-level pre/post swap only — no type checker changes needed.

#### Theoretical Status after v0.15

| Concept            | v0.13 (current)                     | v0.15 (target)                                |
| ------------------ | ----------------------------------- | --------------------------------------------- |
| Foundation         | Atkey indexed monad (monad in Prof) | Free †-SMC                                    |
| Sequential compose | `bind` (do blocks)                  | `;` — unchanged                               |
| Parallel compose   | none                                | `><` (Merge type family)                      |
| Inversion          | none                                | `dag` (pre/post swap)                         |
| Wire bundles       | Row types                           | Row types — unchanged                         |
| Morphism type      | `Computation pre post a`            | same (= sugar for `pre ⊸ {_r: Cl a \| post}`) |

**Zero syntax changes.** `do` blocks = sequential, user-defined operator `><` = parallel, function `dag` = inversion. Parser unchanged. Semantic extension only.

---

### v0.16 — Multiplicity Generalization (SMC Phase 4 + Evidence Phase 5)

Unifies Evidence Phase 5 (multiplicity polymorphism) with SMC Phase 4 (semiring generalization). Effectively the same work.

Generalizes the hard-coded `@Linear`/`@Affine`/`@Unrestricted` to a type-class-based semiring:

```gicel
class UsageSemiring (s: Type) {
  zero :: s; one :: s; plus :: s -> s -> s; mult :: s -> s -> s
}
```

The existing `{0, 1, ω}` semiring is preserved as the default instance. Enables quantum resource tracking (probability semiring) and QTT connections.

**Resolves**:

- "Double grading" — semiring formalization makes the product category State × Usage explicit. Opens the path to triple grading (State × Usage × Probability).
- "Evidence fiber crossing" — `@Mult` becoming a type-level parameter lets fiber interactions be handled formally.

---

## Design Fork Points

| Fork Point                                  | Current State                              | Decision Trigger                            |
| ------------------------------------------- | ------------------------------------------ | ------------------------------------------- |
| `Row` as built-in kind vs structured index  | Built-in kind (DataKinds reduces pressure) | Need for non-capability row-like indexing   |
| Algebraic effects/handlers vs indexed monad | Indexed monad (type families compensate)   | Handlers prove better for AI agent use case |
| Tensor product kind (`QType`)               | Not present (rows cover current needs)     | Quantum entanglement or non-separable state |

### Tensor Product Kind

v0.15 provides row merging (separable composition), but quantum entanglement (inseparable composition) requires a true tensor product `A ⊗ B`. Row labels are addressable (projectable); tensor products are inseparable (non-projectable). Classical capability management uses rows, quantum entanglement uses tensors — separated at the kind level. v0.14–15 complete without tensor products; the decision can be deferred.

---

## Known Theoretical Boundaries

Consequences of GICEL's design coordinate (Atkey indexed monad × row polymorphism × CBPV × Go embedding) not addressed by existing literature. Not bugs or missing features — design consequences with practical workarounds.

### Double Grading

`Computation pre post a` is indexed by state transition (pre → post). Adding `@Mult` grading creates a second axis: how many times a capability can be used. Row unification must account for both label presence and usage count when computing pre/post diffs.

**Current state**: multiplicity enforcement counts same-type preservations at bind sites. Row unification uses LUB for heterogeneous joins.
**Trigger**: multiplicity _polymorphism_ (quantifying over `@Mult`). Row unification then solves state-transition and usage constraints simultaneously — not covered by existing graded monad literature (Orchard, Petricek et al.), which treats single-axis grading.
**Resolved by**: v0.16 (semiring generalization formalizes the product category State × Usage).

### Type Family / Row Unification Scheduling

Type families can return `Row` values used as `Computation pre post a` indices. Row unification needs the reduced result, but reduction may need unification to resolve meta-variables first.

**Current state**: L2 re-activation index handles this — stuck families are re-reduced when blocking metas are solved, with cascading via `ProcessRework`.
**Trigger**: programs requiring L4+ (GADT givens simplifying stuck families, touchability for Merge on open rows).
**Resolved by**: v0.14 (L4 touchability + row-level type families, co-developed).

### Row Operations Not Exposed at Type Level

Row merging, splitting, and label lookup are internal to the unifier (`unifyEvCapRows`) but not available as type-level expressions. This blocks parallel composition (`><`) — its type requires `Merge r1 r2`, a type-level _construction_, not unification. Sequential composition (`bind`) succeeds because it only requires _unification_ of shared indices (post₁ = pre₂).

**Resolved by**: v0.14 (row-level type families expose Merge/Without/Lookup).

### Evidence Fiber Crossing

The evidence system separates fibers (`Type`, `Constraint`, `Row`). Type families can cross fibers (`Row → Row`, `Type → Constraint`). When a family result feeds into a different fiber's unification, the "fibers are independent" assumption breaks.

**Current state**: the single-pass reduce → unify pipeline handles current cases because family results are fully reduced before entering unification.
**Trigger**: a family whose result is another family application in a different fiber (e.g., `Row → Constraint` whose result enters evidence resolution). Requires interleaved cross-fiber reduction.
**Resolved by**: v0.16 (`@Mult` generalization crosses the Type/Row fiber boundary).

---

## Intentional Capability Bounds

### Non-entry top-level bindings must be values (CBPV discipline)

Non-entry top-level bindings with bare `Computation` type are rejected (E0291). Wrap with `thunk` to produce a `Thunk` type (a value). Only the entry point (default `main`) is exempt.

```gicel
helper := thunk (do { x <- get; pure x })  -- Thunk = value
main := do { h <- force helper; pure h }    -- entry point: bare Computation OK
```

**Thunk + numeric literal caveat**: a `thunk`-wrapped computation containing `Num` literals gets let-generalized with polymorphic state types. On `force`, the `Num` dictionary parameter propagates, turning `main` into a function. Fix with explicit type annotation:

```gicel
counter :: Thunk { state: Int } { state: Int } Int
counter := thunk (do { put 0; modify (+ 1); get })
```

**Scope**: `NewRuntime` (compilation for execution) only. Disabled for `Compile` (check-only) and `RegisterModule` (modules). Controlled by `CheckConfig.EntryPoint` / CLI `--entry`.

### Fundep improvement is best-effort

Functional dependency improvement (`| a =: b`) is best-effort: when the `from` position matches an instance, the checker attempts to unify the `to` position with the instance type. If unification fails (e.g., the type is already constrained), the improvement is silently skipped.

**Rationale**: a hard error would reject valid programs where the fundep simply provides no additional information.

### Compiler-generated names use `$` convention

Compiler-generated identifiers (dictionary constructors, instance bindings, internal binders) contain `$`. The lexer rejects `$` in user identifiers, preventing collisions at the grammar level.

**Invariant**: `$` must remain prohibited in user identifiers. If this changes, explain-mode filtering must switch to explicit metadata.

### Tuples are encoded as records with `_1`, `_2`, ... labels

Tuples `(a, b, c)` are desugared to `Record { _1: a, _2: b, _3: c }` by the parser. Canonical definition: `types.TupleLabel(pos)`.

**Invariant**: all tuple construction and detection must use the `_N` convention. If the encoding changes, all consumers must be updated together.

### Exhaustiveness witness reconstruction is best-effort

The exhaustiveness checker's witness formatting (`exhaust/matrix.go`) uses best-effort shape recovery and sorts record fields for stable rendering. Witnesses are used only for error reporting, not for semantic decisions.

---

## Far Future (assessed, not planned)

| Extension                                                   | Category         | Prerequisite             |
| ----------------------------------------------------------- | ---------------- | ------------------------ |
| Tensor product kind (`QType`)                               | Type system      | v0.15 + quantum use case |
| Optimizer Phase 2–3 (selective inline + case-of-case)       | Optimization     | Benchmark-driven demand  |
| Tuple `Eq`/`Ord` runtime support                            | Stdlib           | User demand              |
| Diagnostics Phase B–E (suggestion, span, JSON, terminology) | DX               | Incremental, any time    |
| Refinement types                                            | Phase transition | Separate analysis        |
| Dependent types                                             | Full restructure | Far future               |
