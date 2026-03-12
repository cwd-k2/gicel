# Theoretical Positioning of Gomputation

One-line description: where `Computation pre post a` sits in the formal landscape of effect systems, graded monads, coeffects, and usage disciplines — and what this positioning means for future extensions.

## Table of Contents

1. Purpose
2. Gomputation as an Atkey Parameterized Monad
3. The Graded Monad Connection
4. The Coeffect Structure Already Present
5. Usage Discipline: Current Position and Pressure Points
6. Lattice Structure and the Branching Question
7. Summary: Position on Each Theoretical Axis
8. What This Means for Extensions
9. Key References

---

## 1. Purpose

Gomputation's `Computation pre post a` is often described informally as "an indexed monad with row-typed capability environments." This document makes the theoretical positioning precise, identifies which formal structures are already present (perhaps implicitly), and clarifies what additional structure each planned extension would introduce.

This matters because the project sits at an intersection of several active research areas — parameterized monads, graded effects, coeffects, and usage disciplines — and understanding the exact position prevents accidental commitments to incompatible extensions.

---

## 2. Gomputation as an Atkey Parameterized Monad

### 2.1 The formal structure

The computation type:

```text
Computation : Row -> Row -> Type -> Type
```

with operations:

```text
pure : a -> Computation r r a
bind : Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

is an Atkey parameterized monad (Atkey 2009) where the index set is the collection of row types (capability environments).

The categorical formulation: let **S** be the category whose objects are row types and whose morphisms are state transitions (induced by capability operations). `Computation` is a lax 2-functor from **S**^op × **S** to the category of types, satisfying the parameterized monad laws:

- **Left identity**: `bind (pure a) f = f a`
- **Right identity**: `bind m pure = m`
- **Associativity**: `bind (bind m f) g = bind m (\a -> bind (f a) g)`

Index threading ensures that intermediate states match: the post-state of one computation must equal the pre-state of the next. This is path composition in **S**.

### 2.2 The Kleisli category

The parameterized Kleisli category of `Computation` has:
- Objects: row types (capability environments)
- Morphisms from `r1` to `r2`: values of type `a -> Computation r1 r2 b`
- Composition: Kleisli composition via `bind`
- Identity: `pure`

Each host-provided primitive (e.g., `dbOpen`) is a morphism in this category. A complete program is a composition of such morphisms — a path through the capability state graph.

---

## 3. The Graded Monad Connection

### 3.1 Katsumata's unification

Katsumata (POPL 2014) established that graded monads provide the semantic framework for effect systems. A graded monad is indexed by elements of a monoid:

```text
T : M -> Type -> Type
pure : a -> T e a                              -- e = monoid identity
bind : T m a -> (a -> T n b) -> T (m · n) b   -- · = monoid operation
```

### 3.2 Parameterized monads as category-graded monads

Orchard, Wadler & Yoshida (2020) showed that parameterized monads and graded monads are special cases of **category-graded monads**:

```text
T : C -> Type -> Type
```

where `C` is a (small) category. `pure` uses identity morphisms, `bind` uses composition.

- **Graded monad**: `C` is a monoid (one-object category). The grade is the single morphism.
- **Parameterized monad**: `C` is a category with multiple objects. The grade is a morphism from pre-state to post-state.

**Gomputation's position**: the grading category **C** has:
- Objects: row types (capability environments)
- Morphisms: state transitions (each capability operation defines a morphism)
- Composition: sequential execution via `bind`

This means **Gomputation is already a category-graded monad**. The `(pre, post)` pair is the grade, drawn from the morphism set of the capability-state category.

### 3.3 Comparison with traditional graded effects

Traditional graded effect systems (e.g., Koka's effect rows) use a **commutative monoid** as the grade — typically set union:

```text
effects: {Read, Write} ∪ {IO} = {Read, Write, IO}
```

Gomputation's grade is **non-commutative** — state transitions compose sequentially, not commutatively:

```text
(Closed → Opened) ; (Opened → Closed) ≠ (Opened → Closed) ; (Closed → Opened)
```

This non-commutativity is essential: it captures the fact that the order of operations matters for capability state.

| Property | Traditional graded effect | Gomputation |
|----------|-------------------------|-------------|
| Grade structure | Commutative monoid (set, lattice) | Category (non-commutative) |
| What grade represents | Which effects may occur | How state transforms |
| Composition | Union / join | Path composition |
| Commutativity | Yes (effects are unordered) | No (transitions are ordered) |
| Branching | Join of effect sets | Requires additional structure (§6) |

### 3.4 Additional grading dimensions

The current grade — `(pre, post)` — tracks **which** state transitions occur. Additional grading dimensions could track:

| Dimension | Grade structure | What it tracks | Status |
|-----------|----------------|---------------|--------|
| State transition | Category of row types | How capabilities change | **Implemented** |
| Quantity | Natural numbers / semiring | How many times operations are used | Not implemented |
| Security level | Lattice | What clearance is required | Not implemented |
| Cost | Ordered monoid | Computational resources consumed | Not implemented (step limit is runtime) |

These are orthogonal refinements. Adding any of them would make Gomputation a **multi-graded monad** — graded by a product of grade structures. The existing `(pre, post)` grade would remain; new grades would add precision.

---

## 4. The Coeffect Structure Already Present

### 4.1 Effects and coeffects

Petricek, Orchard & Mycroft (2014) identified the dual of effects:

- **Effect**: what a computation **produces** (side effects, state changes, raised exceptions)
- **Coeffect**: what a computation **requires** (context, resources, environment)

Formally, an effect system annotates the **output** of typing judgments:

```text
Γ ⊢ e : T ! ε          -- e has type T and produces effects ε
```

A coeffect system annotates the **input**:

```text
Γ @ c ⊢ e : T           -- e has type T and requires context satisfying c
```

### 4.2 Gomputation's dual structure

The `Computation pre post a` type encodes **both** simultaneously:

```text
Computation pre post a
            ^^^  ^^^^
            │    └── effect: what the computation does to the environment
            └─────── coeffect: what the computation requires from the environment
```

- `pre` is the **coeffect**: the capability environment that must be available before the computation runs.
- `post` is the **effect**: the capability environment that results after the computation runs.
- The difference `pre → post` is the net state change — the combined effect/coeffect of the computation.

This is not a coincidence. The Atkey parameterized monad, when viewed through the coeffect lens, naturally captures both input requirements and output effects in its index pair. Traditional effect systems capture only the output half; traditional coeffect systems capture only the input half. Gomputation captures both, because capability-state protocols require knowing both what you need and what you leave behind.

### 4.3 The coeffect monoid

In Petricek et al.'s framework, coeffects form a **semiring** (or lattice) that combines via:
- **Sequential composition** (`·`): passing through one coeffect context, then another
- **Parallel composition** (`+`): requiring both coeffect contexts simultaneously

For Gomputation:
- Sequential composition of coeffects = path composition of `(pre, post)` pairs
- Parallel composition = row extension (`{ a : T, b : U | r }` requires both `a` and `b`)

Row extension already serves as the parallel coeffect composition operator, without needing additional formalism.

---

## 5. Usage Discipline: Current Position and Pressure Points

### 5.1 The spectrum

Usage discipline tracks how values (particularly capability handles) may be consumed:

```text
Unrestricted → Relevant (≥1) → Affine (≤1) → Linear (=1) → Graded (= n)
```

| Discipline | Guarantee | Prevents |
|-----------|-----------|----------|
| Unrestricted | None | Nothing |
| Relevant | Used at least once | Unused capabilities |
| Affine | Used at most once | Double-use (double-close, replay) |
| Linear | Used exactly once | Both unused and double-use |
| Graded | Used exactly n times | Any deviation from declared count |

### 5.2 Gomputation's current position: unrestricted

Capability **values** (variables bound in `do`-blocks) are currently unrestricted. They can be used zero or more times:

```text
program := do {
  token <- getToken;
  _ <- useToken token;     -- first use
  _ <- useToken token;     -- second use: currently allowed
  pure Unit
}
```

The **capability row** tracks state transitions but not individual value usage. The row knows that `token` was obtained (post-state changed), but does not prevent `token` from being used again.

### 5.3 Why this works for now

In the current model, host primitives control their own safety:
- A `PrimImpl` can detect double-use at runtime and return an error.
- The host can design primitives so that double-use is harmless (idempotent operations).
- Step/depth limits prevent unbounded replay.

The type system defers to the host for usage safety.

### 5.4 Pressure points: when static usage tracking becomes necessary

| Scenario | Problem | Required discipline |
|----------|---------|-------------------|
| One-time tokens / nonces | Runtime detection only; type system silent | Affine (≤1) |
| File handles after close | Use-after-close is undefined behavior | Affine (≤1) |
| Exactly-once delivery | Must consume without dropping | Linear (=1) |
| Resource budgets | N API calls allowed | Graded (= n) |
| Session protocol channels | Each message sent/received exactly once in order | Linear (=1) |

### 5.5 Where affine tracking would fit

Two levels of tracking are possible:

**Level A: Capability-level affinity** — annotate row fields with usage modes:

```text
{ db : DB Opened (Once) | r }
```

This extends the row type with a usage annotation per capability. The checker enforces that capabilities marked `Once` appear at most once in the computation's free variables. This is a **refinement of the existing row structure** — it adds information to existing row fields without changing the fundamental architecture.

**Level B: Value-level affinity** — track usage of all variables in the typing judgment:

```text
Γ; x :¹ Token ⊢ e : T
```

The superscript `¹` marks that `x` has exactly one permitted use. This is a **full linear type system** (as in Linear Haskell or Granule). It requires modifying every typing rule to split and track usage contexts.

Level A is a natural extension for Gomputation. Level B is a phase transition.

---

## 6. Lattice Structure and the Branching Question

### 6.1 The missing structure

The Atkey parameterized monad requires a definite intermediate state for composition:

```text
bind : Comp r1 r2 a -> (a -> Comp r2 r3 b) -> Comp r1 r3 b
                              ^^
                              must be determined
```

Branching produces two possible post-states:

```text
       r2a
      /
r1 --
      \
       r2b
```

There is no canonical way to continue without additional structure on the state space.

### 6.2 What structures resolve branching?

| Structure | Operation | What it enables |
|-----------|-----------|----------------|
| **Equality** (current) | `r2a = r2b` required | State-homogeneous branching only |
| **Join semilattice** | `r2a ∨ r2b` | Automatic coarsening at branch points |
| **Coproduct (sums)** | `r2a + r2b` | Explicit case-split after branch |
| **Dependent indexing** | `post : a → Row` | Result-dependent post-state (Idris) |

### 6.3 Gomputation's current position: equality only

The current type system requires `r2a = r2b` (via unification). Both branches must unify to the same post-state. This is restrictive but sound, and requires no additional structure.

### 6.4 The lattice option as a refinement

If a join semilattice were added to capability states, the branching rule would become:

```text
Γ ⊢ c1 : Comp r1 r2a a
Γ ⊢ c2 : Comp r1 r2b a
──────────────────────────────
Γ ⊢ case ... : Comp r1 (r2a ∨ r2b) a
```

This is a **refinement** of the existing structure — it weakens the equality requirement to a lattice join. The question is whether the loss of precision (the join may be coarser than either branch) is acceptable.

For Gomputation's capability model, the lattice would need to be **host-defined**: the host declares what states join to. This is a non-trivial API extension but stays within the handler-as-host architecture.

---

## 7. Summary: Position on Each Theoretical Axis

```text
Axis                          Current position              Next refinement
─────────────────────────────────────────────────────────────────────────────
Monad structure               Atkey parameterized           (stable)
Grading                       Category-graded               Multi-graded (+ quantity)
Effect tracking               State transitions (pre→post)  (stable)
Coeffect tracking             Pre-state as requirement      (stable; implicit)
Equality                      Syntactic + row normal.       + GADT local eq (done)
Usage discipline              Unrestricted                  Affine (capability-level)
Branching                     Equal post-states             Join semilattice (?)
Interpretation                Fixed (host-as-handler)       (invariant)
```

---

## 8. What This Means for Extensions

### 8.1 Extensions that refine existing structure

These extensions add precision to the structures already present, without introducing new axes:

| Extension | Refines | Grade change |
|-----------|---------|-------------|
| Existential types | Abstraction (hiding internal types) | None |
| Rank-N polymorphism | Abstraction (deeper quantification) | None |
| Stdlib expansion | Vocabulary (more instances/classes) | None |
| `bracket` combinator | Resource safety pattern | None |

### 8.2 Extensions that introduce new grade dimensions

| Extension | New dimension | Grade structure |
|-----------|---------------|----------------|
| Affine capabilities | Usage count per capability | Nat (0 or 1) |
| Graded effects (quantity) | Operation count | Nat or interval |
| Security levels | Clearance requirement | Lattice |

### 8.3 Extensions that change fundamental structure

| Extension | Structural change | Impact |
|-----------|-------------------|--------|
| Join semilattice on states | Lattice on index category | Branching rule changes |
| Dependent post-states | Index becomes function of value | Term/Type boundary dissolves |
| Algebraic effect handlers | Free monad replaces direct model | Interpretation model changes |
| Full linear types | Usage context in all judgments | Every typing rule modified |

### 8.4 The Katsumata test

A useful heuristic for evaluating extensions: **does the extension change the grading category, add a new grade dimension, or leave the grading structure unchanged?**

- Unchanged → safe refinement (Existentials, Rank-N, Stdlib)
- New dimension → orthogonal enrichment (Affine, Graded quantity)
- Category change → phase transition (Dependent indexing, Handlers)

---

## 9. Key References

1. Atkey, R. (2009). Parameterised notions of computation. JFP.
2. McBride, C. (2011). Kleisli arrows of outrageous fortune.
3. Katsumata, S. (2014). Parametric effect monads and semantics of effect systems. POPL.
4. Orchard, D., Wadler, P. & Yoshida, N. (2020). Unifying graded and parameterised monads. MSFP.
5. Petricek, T., Orchard, D. & Mycroft, A. (2014). Coeffects: a calculus of context-dependent computation. ICFP.
6. Gaboardi, M. et al. (2016). Combining effects and coeffects via grading. ICFP.
7. Brunel, A. et al. (2014). Coeffect core calculus.
8. Orchard, D. & Liepelt, V. (2019). Quantitative program reasoning with graded modal types. ICFP (Granule).
9. [Computation Model and Algebraic Effects](./computation-model-and-algebraic-effects.md) — companion document on the handler-as-host separation.
10. [Non-Linear Effect Composition](./non-linear-effect-composition.md) — detailed analysis of the branching problem.
11. [Indexed, Parameterized, and Graded Monads](./indexed-parameterized-graded-monads.md) — formal foundations.

## Relevance to Gomputation

Gomputation already occupies a precise position in the formal landscape: it is a **category-graded monad** with **row-typed grades**, capturing both **effects** (post-state) and **coeffects** (pre-state) in a single indexed structure. The host-as-handler principle fixes the interpretation, making it a non-free, non-algebraic effect system. Future extensions should be evaluated against this positioning: refinements that add grade dimensions (affine tracking, quantity grading) are natural, while changes that alter the grading category (dependent indexing) or the interpretation model (algebraic handlers) represent phase transitions.
