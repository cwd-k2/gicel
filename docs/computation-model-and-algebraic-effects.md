# Computation Model and Algebraic Effects

One-line description: why Gomputation's `Computation pre post a` is not algebraic effects, where it sits in the design space, and what the handler-as-host separation means for the language's architecture.

## Table of Contents

1. The Question
2. Algebraic Effects: Structure
3. Gomputation's Indexed Capability Model: Structure
4. Structural Comparison
5. Where Gomputation Sits in the Design Space
6. Handler-as-Host: The Separation Principle
7. Implications for Extension Design
8. Key References

---

## 1. The Question

Both algebraic effect systems and Gomputation use row types to track what a computation may do. Both compose operations monadically. The surface syntax can look similar:

```text
-- Algebraic effect style (Koka, Eff, Effekt)
fun open-db() : <state<DB>> Unit

-- Gomputation style
dbOpen :: Computation { db : DB Closed | r } { db : DB Opened | r } Unit
```

Despite this surface resemblance, the two systems are structurally different. The distinction is not incidental — it follows from Gomputation's design requirements.

---

## 2. Algebraic Effects: Structure

Algebraic effects (Plotkin & Power 2003; handlers: Plotkin & Pretnar 2009) decompose effectful computation into two layers:

### 2.1 Operations (free layer)

An effect is a signature of operations. Operations have no intrinsic meaning — they are syntactic symbols:

```text
effect State s {
  get : Unit -> s
  put : s -> Unit
}
```

A computation that uses `get` and `put` constructs a free algebra over these operations. The computation is a syntax tree of operation calls interleaved with pure code.

### 2.2 Handlers (interpretation layer)

A handler gives meaning to operations by pattern-matching on them and having access to the delimited continuation:

```text
handle comp with {
  get () k   -> \s -> k s s         -- resume with current state
  put s' k   -> \_ -> k () s'       -- resume with new state
  return x   -> \s -> (x, s)        -- final answer
}
```

The handler is a fold (catamorphism) over the free algebra. The same operation can be interpreted differently by different handlers.

### 2.3 Key properties

| Property | Description |
|----------|-------------|
| Operations are free | No intrinsic meaning until handled |
| Handlers are user-defined | Any user can write a handler that reinterprets operations |
| Continuation access | Handlers can resume, abort, fork, or store the continuation |
| Effect row = operation set | `{State s, IO \| r}` tracks which operations are available |
| State transitions not tracked | The row describes what effects are present, not how state evolves |

### 2.4 Theoretical identity

An algebraic effect system with handlers is equivalent to the free monad over a functor signature, with handlers as algebras (folds). This is the "algebraic" in "algebraic effects" — the operations form a theory, and handlers are models of that theory.

---

## 3. Gomputation's Indexed Capability Model: Structure

Gomputation's computation type is:

```text
Computation : Row -> Row -> Type -> Type
```

where `pre` is the capability environment before the computation and `post` is the capability environment after.

### 3.1 Primitives (fixed layer)

Operations are host-provided primitives with fixed semantics:

```text
-- Gomputation: meaning is fixed at host registration time
dbOpen := assumption
-- Go side: eng.DeclareBinding("dbOpen", ...)
-- Runtime: PrimImpl that actually opens the database
```

The host (Go program) defines what `dbOpen` does. The agent (Gomputation program) cannot alter, intercept, or reinterpret the operation.

### 3.2 No handlers

There is no handler construct. The meaning of every operation is determined by the host before the agent's code runs. This is by design: the host defines the sandbox; the agent operates within it.

### 3.3 State transition tracking

The type system tracks how each operation transforms the capability environment:

```text
dbOpen  :: Comp { db : DB Closed | r } { db : DB Opened | r } Unit
dbClose :: Comp { db : DB Opened | r } { db : DB Closed | r } Unit
```

`bind` chains these transitions:

```text
bind : Comp r1 r2 a -> (a -> Comp r2 r3 b) -> Comp r1 r3 b
```

The pre-state of each continuation must match the post-state of its predecessor. This is Atkey's parameterized monad (2009): a lax 2-functor from a category of states to the category of types.

### 3.4 Key properties

| Property | Description |
|----------|-------------|
| Primitives are fixed | Meaning determined by host, not by user code |
| No handlers | No user-defined reinterpretation of operations |
| No continuation access | Primitives receive arguments and return values |
| Capability row = state environment | `{ db : DB Opened \| r }` tracks the current state of each capability |
| State transitions tracked | `pre -> post` records how capabilities change |

### 3.5 Theoretical identity

Gomputation's computation model is an Atkey parameterized monad indexed over a category whose objects are row-typed capability environments and whose morphisms are state transitions. This is not a free monad — there is no interpretation layer.

---

## 4. Structural Comparison

### 4.1 The fundamental axis: who defines meaning?

```text
Algebraic Effects:
  operation ─── free (no meaning) ──→ handler ─── fold (user gives meaning)
                                         ↑
                                    user-defined

Gomputation:
  primitive ─── fixed (meaning sealed) ──→ host impl ─── PrimImpl (host gives meaning)
                                              ↑
                                         host-defined
```

This is the primary structural difference. Everything else follows from it.

### 4.2 What the row tracks

| System | Row represents | Example |
|--------|---------------|---------|
| Algebraic effects | Available operations (what you can do) | `{State Int, IO \| r}` |
| Gomputation | Capability state (what state things are in) | `{db: DB Opened, log: Log \| r}` |

Algebraic effect rows answer "which effects are in scope?" Gomputation's rows answer "what is the current state of each capability?"

### 4.3 Composition model

| Aspect | Algebraic Effects | Gomputation |
|--------|------------------|-------------|
| Sequencing | `bind` (effect row propagated) | `bind` (pre/post chained) |
| Effect discharge | Handler eliminates one effect layer | Not applicable (no discharge) |
| Nesting | Handlers nest; inner handles first | Primitives don't nest |
| Branching | Both branches have same effect set | Both branches must have same post-state (v0.4) |

### 4.4 Control flow

| Aspect | Algebraic Effects | Gomputation |
|--------|------------------|-------------|
| Delimited continuations | Yes (handler captures `k`) | No |
| Resumption | Handler decides whether to resume | Not applicable |
| Multi-shot continuations | Some systems allow | Not applicable |
| Exception-like abort | Handler can discard `k` | Host can return error |

---

## 5. Where Gomputation Sits in the Design Space

The two axes that distinguish effect management strategies are:

1. **Freedom of interpretation**: Are operation meanings free (user-definable) or fixed (host/built-in)?
2. **State transition tracking**: Does the type system track how state changes, or only which effects are available?

```text
                     State transition tracking
                  None ←──────────────→ Tracked
                   │                      │
  Free         Koka, Eff,             (theoretically possible;
  (handler)    Effekt, OCaml 5,        few practical systems)
               Frank, Links
                   │                      │
  Fixed        IO monad,              ★ Gomputation ★
  (host/       ST monad,              Atkey parameterized monad
   built-in)   simple effect           + row-typed capabilities
               tracking
                   │                      │
```

Gomputation occupies the **fixed interpretation × state tracking** quadrant. This is a deliberate design choice, not a limitation to be corrected.

---

## 6. Handler-as-Host: The Separation Principle

The absence of user-defined handlers in Gomputation is not an omission — it is the central architectural decision. The principle:

```text
The host is the handler.
```

In algebraic effect systems, a handler is a piece of user code that interprets operations. In Gomputation, the Go host program fulfills this role:

| Algebraic effect concept | Gomputation equivalent |
|--------------------------|----------------------|
| Effect signature | `DeclareBinding` type declarations |
| Handler | `PrimImpl` functions registered by the host |
| Handler installation | `Engine` configuration at setup time |
| Continuation resumption | Return value from `PrimImpl` (always resumes exactly once) |
| Handler scope | Runtime scope (fixed for entire execution) |

### 6.1 Why this separation matters

The primary use case for Gomputation is an AI agent wrapper: agents construct and execute pure computations in a sandboxed environment. The requirements:

1. **The host defines available capabilities.** The agent should not be able to invent new effects or reinterpret existing ones.
2. **The host controls execution.** Step limits, depth limits, `context.Context` cancellation — all enforced by the host.
3. **Capability semantics are trustworthy.** When `dbOpen` appears in a type, it means what the host says it means. No interception, no redefinition.

User-defined handlers would violate all three requirements. An agent with handler access could:

- Reinterpret a `dbOpen` to be a no-op
- Capture continuations and replay them (violating single-execution assumptions)
- Define handlers that circumvent capability protocols

### 6.2 The handler is not missing — it is on the other side of the boundary

```text
┌─────────────────────────────────────────────┐
│  Go Host (the handler)                      │
│                                             │
│  eng.DeclareBinding("dbOpen", dbOpenType)   │
│  eng.RegisterPrim("dbOpen", func(...) {     │
│      // This IS the handler body            │
│      db := openDatabase(...)                │
│      return db, newCapEnv, nil              │
│  })                                         │
│                                             │
├─────────────────────────────────────────────┤
│  Gomputation Agent (the computation)        │
│                                             │
│  dbOpen :: Comp {db:DB Closed|r}            │
│                 {db:DB Opened|r} Unit       │
│                                             │
│  program := do {                            │
│    _ <- dbOpen;                             │
│    ...                                      │
│  }                                          │
└─────────────────────────────────────────────┘
```

The boundary between host and agent is the boundary between handler and computation. This separation is enforced by the language boundary itself (Go vs. Gomputation), not by a type-level mechanism within the language.

---

## 7. Implications for Extension Design

### 7.1 Extensions that remain compatible

The handler-as-host model is compatible with extensions along these lanes:

- **Index Lane** (DataKinds, GADTs, existentials): These strengthen what indices can express, not who interprets effects.
- **Abstraction Lane** (rank-N, HKT, qualified constraints): These strengthen what can be abstracted over, not how effects are handled.
- **Type-Equality Lane** (local refinement, type families): These strengthen when two types are considered equal.
- **Usage Discipline Lane** (affine/linear): These constrain how capabilities are used, which reinforces the host authority model.

### 7.2 Extensions that create tension

- **Algebraic effects + handlers**: Directly conflicts with host authority. If added, handlers would need to be restricted to a safe subset (e.g., host-defined handlers only, no user-defined handlers).
- **Effect reinterpretation / effect instances**: Similar tension. The host, not the agent, must control interpretation.
- **Multi-shot continuations**: Incompatible with the single-execution, single-resume model of `PrimImpl`.

### 7.3 The branching problem through this lens

The branching problem (§2 of `non-linear-effect-composition.md`) is interesting in light of this separation:

- **Solution A (equal post-states)**: Works within the current model.
- **Solution B (lattice join)**: Works within the current model if the lattice is host-defined.
- **Solution C (dependent post)**: Requires dependent types — a phase transition.
- **Solution D (row sums)**: Potentially compatible but heavyweight.
- **Solution E (handlers)**: Conflicts with the host authority model unless handlers are host-only.

This suggests that Solutions A and B are the most natural continuations for Gomputation.

---

## 8. Key References

1. Plotkin, G. D. & Power, J. (2003). Algebraic operations and generic effects.
2. Plotkin, G. D. & Pretnar, M. (2009). Handlers of algebraic effects.
3. Atkey, R. (2009). Parameterised notions of computation.
4. Bauer, A. & Pretnar, M. (2015). Programming with algebraic effects and handlers.
5. Lindley, S., McBride, C., & McLaughlin, C. (2017). Do be do be do (Frank language).
6. Leijen, D. (2017). Type directed compilation of row-typed algebraic effects (Koka).
7. [Extension Directions and Unifying Structure](./extension-directions-and-unifying-structure.md) — §5.5 on algebraic effects and handlers.
8. [Non-Linear Effect Composition](./non-linear-effect-composition.md) — branching problem analysis.
9. [Indexed, Parameterized, and Graded Monads](./indexed-parameterized-graded-monads.md) — theoretical foundations.

## Relevance to Gomputation

This document establishes that Gomputation's `Computation pre post a` is an **Atkey parameterized monad with row-typed capability environments**, not an algebraic effect system. The handler role is filled by the Go host, and this separation is architecturally intentional. Extension design must preserve this separation: the host defines what capabilities mean; the agent uses them under typed protocols.
