# Non-Linear Effect Composition: Branching, Exceptions, and Early Return in Indexed Effect Systems

One-line description: a formal analysis of where the linear composition law of `bind` breaks down, and what structures are needed to restore compositionality in the presence of branching, failure, and early return.

## Table of Contents

1. Formal Characterization of the Problem
2. The Branching Problem
3. The Exception/Failure Problem
4. The Early Return Problem
5. The Scope and Resource Safety Problem
6. Solutions in Existing Systems
7. Formal Analysis: Categorical and Algebraic Perspectives
8. Comparison Table of Solutions
9. Implications and Recommendations for Gomputation
10. Key References

---

## 1. Formal Characterization of the Problem

### 1.1 Why `bind` is inherently linear

The core sequencing law of Gomputation is:

```text
bind : Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

This law demands that the post-state of the first computation equals the pre-state of the second. Diagrammatically, each computation is an edge in a directed graph of capability states:

```text
c1 : r1 -----> r2
c2 :            r2 -----> r3
bind c1 c2 :  r1 ----------------> r3
```

The composition is a path. Paths are inherently linear structures: each edge has exactly one successor. The post-state `r2` appears as both the output of `c1` and the input of `c2`, and these must be unified. There is no room for ambiguity.

This linearity is a feature for sequential protocols. It is also the source of every problem discussed in this document.

### 1.2 Where linearity breaks

Linearity breaks whenever a computation has more than one possible continuation or more than one possible outcome state. The fundamental situations are:

1. **Branching**: `if c then comp1 else comp2` where `comp1` and `comp2` may produce different post-states.
2. **Failure**: an operation that may not reach its declared post-state.
3. **Early return**: exiting a sequence before completing all planned state transitions.
4. **Scope escape**: a capability opened within a block that the block exits before closing.

All four are manifestations of the same underlying tension: the graph of capability-state transitions is no longer a path but a tree, a diamond, or a partial path. The linear chain algebra of `bind` cannot represent these shapes without augmentation.

### 1.3 The categorical view in brief

An Atkey-style parameterized monad is a lax 2-functor from a category of indices (capability states and transitions) to the 2-category of types and functions. The Kleisli composition:

```text
(>=>:) : (a -> M p q b) -> (b -> M q r c) -> (a -> M p r c)
```

requires a definite intermediate object `q`. Branching produces a cospan:

```text
       q1
      /
p ---
      \
       q2
```

There is no canonical way to continue from a cospan without additional structure. The options are:

- Force `q1 = q2` (equalization).
- Provide a join `q1 ∨ q2` (lattice structure on the index category).
- Make the continuation depend on which branch was taken (dependent post-state).
- Intercept the branching with a handler that normalizes the post-state.

Each option corresponds to a distinct design strategy, analyzed below.

---

## 2. The Branching Problem

### 2.1 The problem stated

Consider the following Gomputation program:

```text
openAndMaybeUpgrade :: Bool -> Computation { db : DB[Closed] | r } ??? Unit

openAndMaybeUpgrade flag =
  bind dbOpen (\_ ->
    if flag
      then dbUpgrade    -- post: { db : DB[Upgraded] | r }
      else pure ()      -- post: { db : DB[Opened]   | r }
  )
```

What should `???` be? The two branches produce different post-states:

```text
then-branch : Computation { db : DB[Opened] | r } { db : DB[Upgraded] | r } Unit
else-branch : Computation { db : DB[Opened] | r } { db : DB[Opened]   | r } Unit
```

The type of `bind` requires a single, definite post-state. These cannot be unified.

### 2.2 Solution A: Require equal post-states (state-homogeneous branching)

The simplest solution: all branches of a conditional must produce the same capability post-state.

```text
if-comp :
  Bool ->
  Computation r1 r2 a ->
  Computation r1 r2 a ->
  Computation r1 r2 a
```

The `openAndMaybeUpgrade` example would be rejected. The programmer would need to restructure, for example by making both branches end in the same state:

```text
openAndMaybeUpgrade flag =
  bind dbOpen (\_ ->
    if flag
      then bind dbUpgrade (\_ -> dbDowngrade)  -- return to Opened
      else pure ()
  )
```

**Advantages**: Simple. No new type machinery. Compatible with the current `bind` type. Preserves the linear composition law unchanged.

**Disadvantages**: Restrictive. Forces the programmer to manually equalize states, which may be unnatural or impossible. Cannot express protocols where different branches genuinely lead to different capability configurations.

**Assessment for Gomputation**: This is the natural starting point. A language that does not need branch-divergent state transitions should start here.

### 2.3 Solution B: Join/meet on rows (lattice structure)

Equip capability states with a partial order and require that `if-then-else` produces the join (least upper bound) of the two branch post-states.

```text
if-comp :
  Bool ->
  Computation r1 r2 a ->
  Computation r1 r3 a ->
  Computation r1 (r2 ∨ r3) a
```

This requires a join semilattice on capability states. For example, if `DB[Opened]` and `DB[Upgraded]` both refine a common state `DB[Active]`:

```text
DB[Opened] ∨ DB[Upgraded] = DB[Active]
```

then after the branch, the capability would be known to be in state `DB[Active]` — less precise than either branch, but enough to continue.

This approach is standard in traditional (commutative) effect systems, where effects form a bounded join semilattice and the typing rule for conditionals is:

```text
Gamma |- e1 : A ! ε1
Gamma |- e2 : A ! ε2
--------------------------------------
Gamma |- if c then e1 else e2 : A ! (ε1 ∨ ε2)
```

For sequential/indexed effect systems, the structure needed is an effect quantale: a complete lattice with an associative sequential composition that distributes over the join. The typing rule for branching becomes:

```text
Gamma |- c1 : Computation r1 r2 a
Gamma |- c2 : Computation r1 r3 a
--------------------------------------
Gamma |- if b then c1 else c2 : Computation r1 (r2 ∨ r3) a
```

**Advantages**: Handles divergent branches automatically. Well-understood algebraically. Compatible with traditional effect system theory.

**Disadvantages**: Requires a lattice structure on capability states, which is a significant addition to the type system. The join may be too coarse — `DB[Active]` may not support the operations that `DB[Opened]` or `DB[Upgraded]` support individually. Requires deciding what the lattice structure is for every capability type, which is a burden on host authors. The join of two rows is not always obvious, especially with row polymorphism.

**Assessment for Gomputation**: Premature. The current design does not have lattice structure on capability states, and introducing one would require significant machinery. This approach is more natural for effect-set systems (like Koka's) than for indexed-typestate systems.

### 2.4 Solution C: Result-dependent post-states (Idris-style)

Allow the post-state to depend on the result value of the computation. The computation type becomes:

```text
Computation r1 (fun : a -> Row) a
```

where the post-state is a function from the result to a row. This is exactly what the Idris effects library does:

```text
-- Idris
open : String -> Mode -> Eff Bool [FILE_IO ()]
       (\res => [FILE_IO (case res of
                            True  => OpenFile m
                            False => ())])
```

In Gomputation syntax, this would look like:

```text
dbOpen : Computation { db : DB[Closed] | r }
                     (\res -> case res of
                                True  -> { db : DB[Opened] | r }
                                False -> { db : DB[Closed]  | r })
                     Bool
```

The continuation must then pattern-match on the result to determine the actual post-state:

```text
bind dbOpen (\res ->
  case res of
    True  -> ... -- here db : DB[Opened]
    False -> ... -- here db : DB[Closed]
)
```

The `bind` type generalizes to:

```text
bind : Computation r1 f a -> ((x : a) -> Computation (f x) r3 b) -> Computation r1 r3 b
```

This is a dependent type: `f x` in the second argument depends on the value `x` bound by the first.

**Advantages**: Maximally expressive. Handles every branching scenario. Each branch gets precise type information about the capability state. This is how Idris and F* handle the problem, and it works well.

**Disadvantages**: Requires dependent types — specifically, the post-state must be a function of the result value, and the continuation's pre-state must depend on the bound variable. This is a large addition to the type system. Gomputation's current draft explicitly separates Term and Type and defers dependent types as a non-goal. Type inference becomes significantly harder. The Go host boundary becomes more complex because the host must express result-dependent state transitions.

**Assessment for Gomputation**: This is the theoretically correct solution but is incompatible with the current design's explicit commitment to Term/Type separation. It should remain on the horizon as the long-term answer, but cannot be the v0 approach.

### 2.5 Solution D: Sum types in the post-state

Encode the ambiguity explicitly in the post-state using a sum at the type level:

```text
if-comp :
  Bool ->
  Computation r1 r2 a ->
  Computation r1 r3 a ->
  Computation r1 (r2 | r3) a
```

where `(r2 | r3)` is a row-level sum (disjoint union of possible capability states). Subsequent operations would need to case-split on which state they are in.

This is closely related to session types, where branching produces a choice type:

```text
S1 ⊕ S2    (internal choice: sender decides)
S1 & S2    (external choice: receiver decides)
```

**Advantages**: Does not require dependent types. Captures the ambiguity explicitly. Related to well-understood session type theory.

**Disadvantages**: Introduces type-level sums over rows, which is a significant addition. Every operation after a branch must handle both possibilities, creating cascading case analysis. The row-level sum interacts badly with row polymorphism and unification.

**Assessment for Gomputation**: Interesting but heavyweight. Better suited for a session-types-oriented language than for the current capability-focused design.

### 2.6 Solution E: Algebraic effect handlers normalize state

If computations are algebraic effects with handlers, branching is handled naturally because the handler decides how to interpret the effects of each branch:

```text
handle (if flag then dbUpgrade else skip) with
  | dbUpgrade k -> ... resume k with normalized state ...
```

The handler intercepts effect operations from either branch and can normalize the post-state at the handler boundary.

**Advantages**: Extremely flexible. Well-studied. Koka and Effekt demonstrate this approach works.

**Disadvantages**: Requires algebraic effect handlers, which the current spec explicitly defers as a non-goal. The handler-based approach is a different computational model from the indexed-monad model. Retrofitting handlers onto an indexed monad system is non-trivial.

**Assessment for Gomputation**: Not available in the current design. If Gomputation later adopts algebraic effects, this becomes the natural solution.

---

## 3. The Exception/Failure Problem

### 3.1 The problem stated

Consider:

```text
dbOpen : Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
```

What if the host's database connection fails? The type says the post-state will be `{ db : DB[Opened] | r }`, but if `dbOpen` fails, the database is still closed. The promise is broken.

This is the fundamental tension between capability-safety (the type says what state you are in) and real-world fallibility (operations can fail).

### 3.2 Solution A: Capabilities never fail

The host guarantees that every capability operation succeeds. If the underlying system fails, the host either retries, blocks, or terminates the computation entirely — but from the language's perspective, the operation always succeeds.

```text
dbOpen : Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
-- If this returns at all, the DB is open.
```

**Advantages**: Maximally simple. Preserves the linear composition law perfectly. The language-level safety story is clean: if `dbOpen` returns, the state has transitioned.

**Disadvantages**: The host must decide what "failure" means. For some operations, host-level retry is unreasonable. For non-recoverable failures, the computation must be aborted entirely, which means the language has an invisible total-failure mode.

**Assessment for Gomputation**: This is actually a strong starting position for a capability-based embedded language. The host controls the computation boundary. If a host operation fails, the host can abort the entire computation. The language-level type system need not represent this abort because it happens outside the language boundary. This is the simplest approach and arguably the most honest: the language's safety guarantees hold for all computations that complete.

### 3.3 Solution B: Failure in the value, not in the state

The operation always transitions the state but returns an error value:

```text
dbOpen : Computation { db : DB[Closed] | r }
                     { db : DB[Opened] | r }
                     (Either Error Unit)
```

The problem: if `dbOpen` returns `Left err`, the type says the state is `{ db : DB[Opened] | r }`, but the database might not actually be open. The type has lied.

One variant: model failure as non-transition:

```text
dbOpen : Computation { db : DB[Closed] | r }
                     { db : DB[Closed] | r }
                     (Either Error Unit)
```

But this claims the state is unchanged regardless of outcome, which is also wrong when the operation succeeds.

The only honest version requires result-dependent post-states (Solution 2.4), which brings us back to dependent types:

```text
dbOpen : Computation { db : DB[Closed] | r }
                     (\res -> case res of
                                Right _ -> { db : DB[Opened] | r }
                                Left  _ -> { db : DB[Closed] | r })
                     (Either Error Unit)
```

**Assessment**: Without dependent types, this approach forces the programmer to lie in one direction or the other. With dependent types, this is the Idris solution.

### 3.4 Solution C: Separate error capability

Introduce an explicit error or exception capability that the computation can invoke:

```text
dbOpen : Computation { db : DB[Closed], err : Exn[Inactive] | r }
                     { db : DB[Opened], err : Exn[Inactive] | r }
                     Unit
```

When the operation fails, it activates the exception capability:

```text
-- on failure, internally:
raiseExn : Computation { err : Exn[Inactive] | r }
                       { err : Exn[Active]   | r }
                       Void
```

The difficulty here is that raising the exception changes the error capability's state but leaves the db capability in an intermediate position. The continuation after `raiseExn` is never reached (the return type is `Void`), but the db capability is now in an inconsistent state.

This approach works only if there is a handler that intercepts the exception and manages the cleanup:

```text
handleExn :
  Computation { err : Exn[Inactive] | r }
              { err : Exn[Inactive] | r }
              a ->
  (Error -> Computation r' r' a) ->
  Computation { err : Exn[Inactive] | r }
              { err : Exn[Inactive] | r }
              a
```

But now we are back to requiring handler-like machinery.

**Assessment for Gomputation**: This approach is essentially embedding algebraic effect handling into the capability system. It could work but adds significant complexity, and the interaction between the error capability and other capabilities is subtle.

### 3.5 Solution D: Total-at-boundary, partial-inside

The cleanest factoring for Gomputation's design:

1. **Host primitives are total from the language's perspective.** If `dbOpen` returns, it succeeded. The type is honest.
2. **If a host operation fails, the host aborts the computation.** The language never sees the failure. The host's Go code handles the error.
3. **Failable operations return `Either` without state change.** If a primitive needs to express potential failure without state transition, it returns a value-level indicator.

```text
-- Always succeeds; host guarantees it or aborts
dbOpen : Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit

-- Queries can fail gracefully; state doesn't change
dbQuery : Query -> Computation { db : DB[Opened] | r } { db : DB[Opened] | r } (Either Error Rows)
```

The key insight: state-transitioning operations are either total (the host guarantees them) or the host wraps them as fallible-but-state-preserving. The decision is made at the host boundary, not in the language.

**Assessment for Gomputation**: This is the recommended approach for v0. It is honest, simple, and leverages the host boundary — which is Gomputation's defining architectural feature.

---

## 4. The Early Return Problem

### 4.1 The problem stated

Consider a `do`-style computation:

```text
pipeline =
  bind dbOpen (\_ ->
  bind (dbQuery q1) (\rows1 ->
    if isEmpty rows1
      then ??? -- want to return early with an empty result
      else bind (dbQuery q2) (\rows2 ->
        bind dbClose (\_ ->
          pure (merge rows1 rows2)))))
```

If `rows1` is empty, the programmer wants to return early. But the db is still open. The early-return path has post-state `{ db : DB[Opened] | r }`, while the normal path ends at `{ db : DB[Closed] | r }`. The types do not match.

### 4.2 Why this is distinct from branching

The branching problem (Section 2) is about two branches of a conditional having different post-states. The early return problem is about leaving a sequence of operations before all state transitions are complete. The early return path has not only a different post-state but a strictly incomplete state transition sequence.

### 4.3 Solution A: No early return (explicit cleanup)

Force the programmer to complete the state transition in every path:

```text
pipeline =
  bind dbOpen (\_ ->
  bind (dbQuery q1) (\rows1 ->
    if isEmpty rows1
      then bind dbClose (\_ -> pure emptyResult)  -- must close before returning
      else bind (dbQuery q2) (\rows2 ->
        bind dbClose (\_ ->
          pure (merge rows1 rows2)))))
```

**Advantages**: No new machinery needed. The type system already enforces this.

**Disadvantages**: Verbose. Every early-return point requires manual cleanup. This scales poorly with deeply nested protocols.

### 4.4 Solution B: Bracket/scoped resource pattern

Introduce a `bracket` combinator that guarantees cleanup:

```text
bracket :
  Computation r1 r2 a ->           -- acquire
  (a -> Computation r2 r1 Unit) -> -- release (always runs)
  (a -> Computation r2 r2 b) ->    -- use (state-preserving body)
  Computation r1 r1 b
```

Note the type: the body must be state-preserving (`r2 -> r2`), and the overall computation restores the original state (`r1 -> r1`). The release action always runs.

Usage:

```text
pipeline =
  bracket dbOpen (\_ -> dbClose) (\_ ->
    bind (dbQuery q1) (\rows1 ->
      if isEmpty rows1
        then pure emptyResult
        else bind (dbQuery q2) (\rows2 ->
          pure (merge rows1 rows2))))
```

The body is state-preserving (`DB[Opened] -> DB[Opened]`), so early return within the body is safe — the `bracket` guarantees `dbClose` runs.

**Advantages**: Clean. Well-understood (Haskell's `bracket`, Rust's `Drop`, Go's `defer`). Does not require dependent types or handlers.

**Disadvantages**: The body must be state-preserving with respect to the bracketed resource. Nested brackets for multiple resources become verbose. The `bracket` combinator itself has a subtle type — it requires the release to undo the acquire, which may not be representable in all cases.

**Assessment for Gomputation**: This is highly compatible with the current design. It does not require dependent types, algebraic effects, or lattice structure. It leverages the host boundary naturally (the host provides both `dbOpen` and `dbClose`). It can be implemented as a derived combinator over `bind` with a disciplined control flow contract.

### 4.5 Solution C: Continuation-based early return

Use a continuation to represent the "rest of the computation" and allow discarding it:

```text
callCC :
  ((a -> Computation r2 r3 b) -> Computation r1 r2 a) ->
  Computation r1 r2 a
```

Early return invokes the continuation, skipping the remaining operations. But this has the same state-consistency problem: the continuation expects a specific pre-state, and the early-return point may leave capabilities in a different state.

**Assessment**: `callCC` for indexed monads is fraught. The continuation's pre-state must match the early-return point's current state, but the whole point of early return is that we have not completed the planned transitions. This approach does not work cleanly without additional state-management machinery.

### 4.6 Solution D: Either-based sequencing

Use the standard monadic error pattern, but at the computation level:

```text
bindEither :
  Computation r1 r2 (Either e a) ->
  (a -> Computation r2 r3 (Either e b)) ->
  Computation r1 r3 (Either e b)
```

But this has the same post-state problem: if the first computation returns `Left`, we skip the second computation and return `Left` — but the post-state should be `r2`, not `r3`. The typing rule is:

```text
-- If first returns Left: post-state is r2
-- If first returns Right and second returns: post-state is r3
-- These differ => type error
```

This works only if `r2 = r3` (state-preserving body) or with dependent types.

**Assessment**: Either-based sequencing over indexed monads inherits all the problems of branching (Section 2). It is not a solution to early return; it is a reformulation of the same problem.

---

## 5. The Scope and Resource Safety Problem

### 5.1 The problem stated

If a capability is acquired within a `do` block and the block is exited early (by exception, early return, or handler unwinding), the capability may be leaked:

```text
leaky =
  bind dbOpen (\_ ->
  bind riskyOperation (\_ ->  -- what if this raises/fails?
  bind dbClose (\_ ->
    pure result)))
```

If `riskyOperation` fails (by whatever mechanism), `dbClose` is never reached. The database handle is leaked.

### 5.2 The relationship to linearity

The core issue is that the capability `db : DB[Opened]` must be consumed exactly once (by `dbClose`). This is a linearity constraint. Non-linear control flow (branching, exceptions, early return) threatens linearity by introducing paths where the capability is consumed zero times (leaked) or more than once (double-close).

### 5.3 How existing systems handle this

**Rust**: Ownership ensures cleanup via `Drop`. When a value goes out of scope, its destructor runs automatically, even with early returns, panics, or `?` operator short-circuiting. The typestate pattern works because state transitions consume the old state by move, and the compiler ensures the new state is either used or dropped. This is the gold standard for resource safety with non-linear control flow.

**Haskell**: `bracket` provides runtime resource safety. The `ResourceT` transformer provides scoped resource management. Linear Haskell (via `LinearTypes` extension) provides static guarantees similar to Rust's, but with more complexity.

**Session types / Linear logic**: The absence of weakening (you cannot discard a resource) and contraction (you cannot duplicate a resource) in linear logic ensures that every capability is used exactly once. The typing rule for branching (additive conjunction `&`) requires both branches to use disjoint copies of the linear context, and the typing rule for choice (additive disjunction `⊕`) requires each branch to have access to the full linear context. The unit `1` requires all linear resources to be consumed before a session can terminate.

**Idris effects**: Resource safety is enforced by the type system. The dependent post-state mechanism ensures that every path through the computation ends in a well-typed state. Because Idris has full dependent types, it can express and check that every resource opened is eventually closed.

**Effekt / Koka**: Effect handlers provide a mechanism for guaranteed cleanup. A handler can intercept any control-flow operation and perform cleanup before propagating. In Effekt, control-flow linearity (whether a continuation is invoked zero, one, or many times) is tracked at the type level to prevent resource leaks with multi-shot handlers (the "Soundly Handling Linearity" result, POPL 2024).

### 5.4 The bracket pattern in indexed effect systems

The `bracket` combinator (Section 4.4) is the primary tool for resource safety without linearity:

```text
bracket :
  Computation r1 r2 a ->           -- acquire
  (a -> Computation r2 r1 Unit) -> -- release
  (a -> Computation r2 r2 b) ->    -- body (state-preserving)
  Computation r1 r1 b
```

The key insight: the body is constrained to be state-preserving (`r2 -> r2`) with respect to the bracketed resource. This means:

1. The body cannot transition the capability to a different state.
2. The body can use the capability (e.g., query a database) but cannot close or fundamentally alter it.
3. The release action is guaranteed to run.

This constraint is restrictive but safe. It prevents the body from half-completing a protocol and then failing.

### 5.5 Bracket limitations

The `bracket` pattern assumes a clean acquire/release pair. It does not handle:

1. **Multi-step protocols**: If a capability must go through states `A -> B -> C -> D` and the computation fails at state `C`, bracket can only restore to `A` (via release). The intermediate states `B` and `C` are not individually recoverable.

2. **Interleaved resources**: If two resources are opened in sequence and the second open fails, the first must be cleaned up. Nested brackets handle this but at the cost of deep nesting.

3. **State-dependent cleanup**: If the cleanup action depends on what state the resource is currently in (which may vary depending on where in the protocol failure occurred), the bracket type is too rigid.

For multi-step protocols, a richer abstraction is needed — essentially, a state machine with rollback edges. This is the province of compensating transactions and saga patterns, which are beyond the scope of a type system but could inform the design of host-level primitives.

---

## 6. Solutions in Existing Systems

### 6.1 Haskell indexed monads

Haskell's indexed monad libraries (Atkey's original, `indexed`, `freer-indexed`) typically solve the branching problem by avoidance:

- The `XMonad` typeclass provides `(>>=:) :: m p q a -> (a -> m q r b) -> m p r b`, which requires definite intermediate state `q`.
- Branching within an indexed monad is only type-correct when both branches produce the same post-state.
- There is no built-in mechanism for branch-divergent states.
- Exception handling is typically handled at the ordinary (non-indexed) monad level, not at the indexed level.

The `freer-indexed` package notes that computations represent "edges in the graph of resource state transitions" and "chained type parameters in bind operation require that associated resource changes are continuous." This is an explicit acknowledgment of the linearity requirement.

In practice, Haskell programmers using indexed monads either (a) structure their programs to ensure homogeneous branching, or (b) use runtime checks with `Either` and accept the loss of static state tracking at branch points.

### 6.2 Idris effects library

Idris provides the most complete solution through dependent types:

- The `Eff` type has a result-dependent resource signature: `Eff a xs (\result => xs')`.
- The post-resource-list is a function from the result value to a list of effects.
- Branching is handled via `case` (not `if-then-else`, because `case` introduces the necessary dependent pattern matching).
- The `pureM` combinator (instead of `pure`) returns a value and computes the post-state from it.
- File I/O demonstrates this: `open` returns `Bool` and the post-state is `OpenFile m` on `True`, `()` on `False`. The continuation must case-split on the result, and in each branch, the type system knows the exact resource state.

The key limitation: this requires full dependent types. The `case` construct is fundamentally different from `if-then-else` because it introduces a dependent elimination that lets the type checker learn different facts in different branches.

### 6.3 Koka

Koka avoids the indexed-monad branching problem entirely by using a different computational model:

- Effects are tracked as rows (sets of effect labels), not as pre/post state transitions.
- A function type is `() -> <exn, console> int`, meaning the function may throw exceptions and use the console.
- Branching is trivially handled because the effect of `if c then e1 else e2` is the union of the effects of `e1` and `e2`.
- There are no pre/post capability states — effects are capabilities, not state machines.
- Algebraic effect handlers provide exception-like control flow, resource management, and backtracking.
- The `with` construct handles effects with guaranteed handler execution.

Koka's approach works because it does not try to track protocol state transitions. It tracks which effects may occur, not what state resources are in. This is a fundamentally different design point from Gomputation's indexed typestate approach.

### 6.4 Eff and Effekt

Eff (by Bauer and Pretnar) and Effekt (by Brachthäuser, Schuster, and others) use algebraic effect handlers:

- Effects are operations that can be called by the computation and handled by a surrounding handler.
- Handlers can resume the computation (after handling an operation) or discard it (for exceptions).
- Multiple resumptions enable backtracking, generators, and non-determinism.
- Effekt specifically tracks control-flow linearity: whether a handler uses its continuation zero, one, or many times. This enables sound interaction between linear resources and effect handlers.

The Effekt approach from "Soundly Handling Linearity" (Tang, Hillerström, Morris, POPL 2024) is particularly relevant:

- A type-and-effect system tracks control-flow linearity alongside the usual effect types.
- If a handler may discard its continuation (zero uses), linear resources in the handled computation may be leaked.
- If a handler may duplicate its continuation (many uses), linear resources may be used multiple times.
- By restricting handlers based on control-flow linearity, the system prevents these violations statically.
- The approach requires no programmer annotations — control-flow linearity is inferred.

### 6.5 Session types

Session types handle branching via explicit choice constructs from linear logic:

- **Internal choice** (`⊕`): the process selects which branch to take and sends the choice label. `S = l1 : S1 ⊕ l2 : S2` means "I will choose either protocol S1 or protocol S2."
- **External choice** (`&`): the process offers multiple branches and the peer selects. `S = l1 : S1 & l2 : S2` means "I will handle either protocol S1 or protocol S2."

The key insight: branching is not implicit. It is a first-class protocol operation with its own typing rule. Both peers know that branching is occurring, and the types enforce that all offered branches are handled.

Duality ensures consistency: the dual of `⊕` is `&`. If one process selects, the other must offer.

Resource cleanup is enforced by the linear discipline: every channel must be eventually closed (reaching the `end` type). The type `1` (unit of tensor) requires that no linear channels remain in scope.

### 6.6 Rust

Rust handles the branching and resource safety problem through ownership and the `Drop` trait:

- Typestate transitions are modeled by functions that consume the old state by move and return a new state.
- The compiler ensures that every value is either moved, consumed, or dropped.
- `Drop::drop()` runs automatically when a value goes out of scope, regardless of how scope is exited (normal return, early return via `?`, panic unwinding).
- Branching is handled by the borrow checker: if two branches produce values of different types, the result must be unified (usually the same type) or handled separately.

For typestate specifically, Rust uses the "consume and return" pattern:

```rust
fn open(db: DB<Closed>) -> DB<Opened> { ... }
fn close(db: DB<Opened>) -> DB<Closed> { ... }
```

If `open` fails (returns `Result::Err`), the old `DB<Closed>` value may be returned in the error, preserving the original state. The ownership system ensures no aliasing, so the state is always consistent.

---

## 7. Formal Analysis: Categorical and Algebraic Perspectives

### 7.1 Parameterized monads and linear composition

An Atkey parameterized monad is a functor `T : S^op × S × C → C` (where `S` is the category of states and `C` is the category of types) equipped with:

```text
η_s : Id → T(s, s, -)        (unit/pure, for each state s)
μ_{s,t,u} : T(s, t, T(t, u, -)) → T(s, u, -)   (multiplication/join)
```

satisfying the monad laws indexed by states. Kleisli composition is:

```text
(a → T(s, t, b)) ; (b → T(t, u, c)) = (a → T(s, u, c))
```

The intermediate state `t` must be fixed. This is a path composition in the category `S`: the composite of morphisms `s → t` and `t → u` is `s → u`. There is no way to compose a cospan `s → t1` and `s → t2` without additional structure.

### 7.2 What structure handles branching?

**Lattice structure** (Solution B): If `S` has binary coproducts (joins), then a cospan `t1 ← s → t2` can be completed by `t1 → t1 ∨ t2 ← t2`. The composition becomes:

```text
(a → T(s, t1, b)) + (a → T(s, t2, b))  →  (a → T(s, t1 ∨ t2, b))
```

This requires `T` to support a "weakening" operation `T(s, t, a) → T(s, t ∨ t', a)` for any `t'`.

**Dependent indexing** (Solution C): Replace the fixed post-state with a family indexed by the result:

```text
T(s, f, a) where f : a → S
```

This makes `T` a dependent type. Composition becomes:

```text
T(s, f, a) × ((x : a) → T(f(x), g, b)) → T(s, g ∘ f, b)
```

This is a fibered construction over the category of states.

**Choice/sum in the index** (Solution D): Replace `S` with a category that has explicit sum objects. A computation `T(s, t1 + t2, a)` produces one of two possible post-states. The continuation must handle both cases.

### 7.3 Arrows and profunctors

Hughes' Arrows abstract over computations with an input and output type:

```text
arr : (a → b) → Arrow a b
(>>>) : Arrow a b → Arrow b c → Arrow a c
first : Arrow a b → Arrow (a, c) (b, c)
```

Arrows naturally model computations with different input and output types, and the `ArrowChoice` subclass handles branching:

```text
left : Arrow a b → Arrow (Either a c) (Either b c)
```

This embeds branching into the type structure: the computation operates on `Either` values, and different branches may transform different components of the sum.

Categorically, an arrow is a monoid in the category of profunctors `C^op × C → Set` (the Heunen-Jacobs characterization). The relationship to indexed monads: every indexed monad gives rise to a "Kleisli arrow" via `a → T(s, t, b)`, and every arrow with certain properties corresponds to an indexed Freyd category (Atkey's result).

The advantage of arrows for branching: `ArrowChoice` is a standard, well-understood abstraction that handles branching explicitly. The disadvantage: arrows are less ergonomic than monads and require the `proc`/arrow notation that most programmers find cumbersome.

### 7.4 Graded monads and the unification

The paper "Unifying Graded and Parameterised Monads" (Orchard, Wadler, Yoshida, 2020) identifies a common generalization:

- **Graded monads** index by a monoid (capturing effect accumulation): `T_m : C → C` with `return : T_1` and `bind : T_m(a) → (a → T_n(b)) → T_{m·n}(b)`.
- **Parameterized monads** index by pre/post-state pairs: `T(s, t, -)`.
- **Category-graded monads** generalize both: `T_f : C → C` where `f` is a morphism in some grading category. Return uses identity morphisms, and bind uses composition.

For branching, the relevant structure on the grading category is a join (coproduct). If the grading category has coproducts, then branching can be modeled by taking the coproduct of the two branch morphisms. This corresponds to the lattice/join approach (Solution B) but is stated categorically.

The key insight from this unification: the choice of grading category determines what compositional structure is available. A discrete category (just objects, no morphisms other than identities) recovers ordinary monads. A preorder recovers graded monads. A category with non-trivial morphisms recovers parameterized monads. A category with coproducts enables branching.

### 7.5 Relative monads

A relative monad `T : J → C` is a monad-like structure where the underlying functor `J : A → C` need not be an endofunctor. This generalization is relevant because Gomputation's computation type can be viewed as a relative monad over the embedding of capability states into the type universe.

The compositional structure of relative monads inherits the same linearity constraint as parameterized monads, because composition still requires matching intermediate objects. However, the richer categorical context of relative monads provides more room for defining branching operations via universal properties of the ambient category `C`.

---

## 8. Comparison Table of Solutions

### 8.1 Branching solutions

| Solution | Requires | Expressiveness | Complexity | Current Compatibility |
| --- | --- | --- | --- | --- |
| A. Equal post-states | Nothing new | Low — must equalize | Minimal | Fully compatible |
| B. Join on rows | Lattice on states | Medium — loses precision | Moderate | Requires new structure |
| C. Dependent post-states | Dependent types | High — fully precise | High | Incompatible (deferred) |
| D. Sum in post-state | Type-level sums | Medium — explicit splits | High | Major addition |
| E. Handler normalization | Algebraic effects | High — handler decides | High | Incompatible (deferred) |

### 8.2 Failure solutions

| Solution | Requires | Honesty | Complexity | Current Compatibility |
| --- | --- | --- | --- | --- |
| A. Capabilities never fail | Host guarantee | Host handles failure | Minimal | Fully compatible |
| B. Failure in value | Either return type | Dishonest without dep types | Low | Partially compatible |
| C. Separate error capability | Handler-like machinery | Honest but complex | High | Significant addition |
| D. Total-at-boundary | Host abort on failure | Honest | Minimal | Fully compatible |

### 8.3 Early return solutions

| Solution | Requires | Ergonomics | Complexity | Current Compatibility |
| --- | --- | --- | --- | --- |
| A. No early return | Manual cleanup | Poor for deep nesting | Minimal | Fully compatible |
| B. Bracket pattern | Bracket combinator | Good for acquire/release | Low-moderate | Compatible |
| C. callCC | Continuation primitives | Familiar to FP users | Moderate | Problematic typing |
| D. Either sequencing | Monad transformer style | Familiar | Low | Same problem as branching |

### 8.4 Resource safety solutions

| Solution | Requires | Guarantee | Complexity | Current Compatibility |
| --- | --- | --- | --- | --- |
| Bracket | Scoped combinator | Dynamic (handler-level) | Low | Compatible |
| Linear types | Usage judgment | Static (type-level) | High | Reserved but inactive |
| Ownership/Drop | Move semantics, destructors | Static (compiler) | High | Not available |
| CFL tracking | Control-flow linearity inference | Static (inferred) | High | Requires effect handlers |

---

## 9. Implications and Recommendations for Gomputation

### 9.1 Which solution is most compatible with the current design?

The current design has:

- `Computation r1 r2 a` with `pure` and `bind`
- No dependent types (Term/Type separation)
- No algebraic effect handlers
- No lattice structure on capability states
- No usage/linearity judgment (reserved but inactive)
- Host-provided primitives with exact pre/post capability states

Given these constraints, the compatible solutions are:

1. **Equal post-states for branching** (Solution 2.2/A)
2. **Total-at-boundary for failure** (Solution 3.5/D)
3. **Bracket pattern for resource safety** (Solution 4.4/B)
4. **Manual cleanup for early return** (Solution 4.3/A), softened by bracket

### 9.2 Recommended approach for v0

The recommendation is a three-part strategy:

#### Part 1: State-homogeneous branching as the base rule

All branches of a conditional (value-level `case` or any future computation-level branching) must produce the same post-state. This is the direct consequence of the current `bind` type and requires no new machinery.

The typing rule for computation-level case analysis:

```text
Gamma |- s : D
Gamma, ... |- b1 : Computation r1 r2 A
...
Gamma, ... |- bn : Computation r1 r2 A
-------------------------------------------
Gamma |- case s of ... : Computation r1 r2 A
```

Note: all branches share the same `r1 -> r2` transition.

#### Part 2: Host primitives are total; failure is host-managed

State-transitioning primitives are honest in their types. If a host operation can fail:

- The host either retries/blocks until success, or aborts the computation.
- Alternatively, the host exposes a fallible-but-state-preserving variant that returns `Either`.

```text
-- Total: always succeeds or computation aborts
dbOpen : forall r. Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit

-- Fallible but state-preserving: returns error as value
dbTryConnect : forall r. Computation { db : DB[Closed] | r } { db : DB[Closed] | r } (Either Error Connection)
```

The spec should state explicitly:

> A host primitive of type `Computation r1 r2 a` is a total operation from the language's perspective. If the underlying host operation fails, the host is responsible for either recovering or aborting the computation. The language-level type guarantee `r1 -> r2` holds for all computations that return.

#### Part 3: Bracket as a derived combinator

Add `bracket` as a standard combinator (not a primitive — it can be defined over `bind` if the language has a mechanism for guaranteed cleanup):

```text
withDB : forall r a.
  (forall r'. Computation { db : DB[Opened] | r' } { db : DB[Opened] | r' } a) ->
  Computation { db : DB[Closed] | r } { db : DB[Closed] | r } a
```

This is a scoped capability pattern: the body has access to an open database, and the combinator guarantees the database is closed afterward.

The full `bracket` type:

```text
bracket : forall r1 r2 a b.
  Computation r1 r2 a ->           -- acquire
  (a -> Computation r2 r1 Unit) -> -- release
  (a -> Computation r2 r2 b) ->    -- body (state-preserving)
  Computation r1 r1 b
```

**Implementation note**: In the current pure-`bind` model, `bracket` is just sequencing with a particular structure. The guarantee that release "always runs" is trivially true if the body is total (no exceptions). If the language later adds exceptions, `bracket` would need runtime support (similar to Go's `defer` or Haskell's `bracket`). For v0, defining `bracket` as:

```text
bracket acquire release body =
  bind acquire (\a ->
    bind (body a) (\b ->
      bind (release a) (\_ ->
        pure b)))
```

is sufficient. The "always runs" guarantee holds because there are no exceptions in v0.

### 9.3 What changes to the minimal vocabulary would each solution require?

| Solution | Vocabulary additions |
| --- | --- |
| Equal post-states | None. Already implied by current `bind`. |
| Join on rows | New: partial order on capability states, join operator on rows, weakening/coercion rules. |
| Dependent post-states | New: dependent function types (Pi types), term-level computation in type indices, dependent case elimination. This would require relaxing the Term/Type separation. |
| Sum in post-state | New: row-level sum kind, branching elimination over row sums, propagation through `bind`. |
| Handler normalization | New: effect operations, handler blocks, resume/discard continuations, handler typing rules. This is essentially a new computational model. |
| Bracket combinator | New: one derived combinator with a specific type. No vocabulary changes if the body is total. |

### 9.4 How does each solution affect the Go host boundary?

| Solution | Host boundary impact |
| --- | --- |
| Equal post-states | None. Host primitives are unchanged. |
| Join on rows | Host must declare the lattice structure on its capability states. |
| Dependent post-states | Host must express result-dependent state transitions in primitive types. The Go-side type representation becomes significantly more complex. |
| Sum in post-state | Host must handle row-sum types in its runtime capability management. |
| Handler normalization | Host primitives become effect operations. The handler model changes how host operations are invoked and resumed. |
| Bracket | None directly, but the host should expose acquire/release pairs for its capabilities. |

### 9.5 Growth path

The recommended growth path, in order of increasing commitment:

1. **v0**: Equal post-states + total-at-boundary + bracket. No new type machinery.

2. **v0.x**: Activate the usage judgment (currently reserved). Introduce affine or linear discipline for capabilities. This makes the bracket guarantee static rather than relying on totality. This is the Rust-like direction.

3. **v1**: Evaluate whether the usage of branching-with-state-change is common enough to warrant dependent post-states or lattice structure. If it is, the most principled extension is result-dependent post-states (the Idris model), which would require controlled relaxation of the Term/Type separation along the extension direction already anticipated in the spec ("promoted protocol states," "controlled interaction between layers").

4. **v1.x (alternative)**: If the language later adopts algebraic effect handlers (currently a deliberately deferred direction), the branching problem resolves naturally via handler normalization. This is a larger architectural decision.

### 9.6 Summary of the recommendation

The branching, exception, early return, and resource safety problems are real pressure points in any indexed effect system. For Gomputation, they are manageable in v0 because:

1. The language is embedded — the host controls the failure boundary.
2. The language is total at the computation level in v0 — no exceptions means `bracket` is just sequencing.
3. State-homogeneous branching is restrictive but honest, and it requires zero new machinery.
4. The `bracket` pattern provides the ergonomic escape from manual cleanup without requiring linear types.
5. The growth path toward dependent post-states or effect handlers is already anticipated in the spec's extension directions.

The critical design discipline is to avoid lying about capability states. Every solution that compromises the honesty of `Computation r1 r2 a` (e.g., returning `Either` with a pre-committed post-state) introduces unsoundness at the foundation. The recommended approach preserves honesty by making failure a host-boundary concern rather than a language-level concern.

---

## 10. Key References

1. Robert Atkey, "Parameterised Notions of Computation", JFP 2009. https://bentnib.org/paramnotions-jfp.pdf
2. Conor McBride, "Kleisli Arrows of Outrageous Fortune", 2011. https://personal.cis.strath.ac.uk/conor.mcbride/Kleisli.pdf
3. Edwin Brady, "Programming and Reasoning with Algebraic Effects and Dependent Types", ICFP 2013 (Idris effects library).
4. Edwin Brady, "Resource-Dependent Algebraic Effects", TFP 2014.
5. Idris Effects Documentation: Dependent Effects. https://docs.idris-lang.org/en/latest/effects/depeff.html
6. Daan Leijen, "Koka: Programming with Row Polymorphic Effect Types", MSFP 2014. https://arxiv.org/abs/1406.2061
7. Daan Leijen, "Algebraic Effects for Functional Programming", MSR-TR-2016. https://www.microsoft.com/en-us/research/wp-content/uploads/2016/08/algeff-tr-2016-v2.pdf
8. Dominic Orchard, Philip Wadler, Nobuko Yoshida, "Unifying Graded and Parameterised Monads", MSFP 2020. https://arxiv.org/abs/2001.10274
9. Colin S. Gordon, "Polymorphic Iterable Sequential Effect Systems", TOPLAS 2021. https://arxiv.org/abs/1808.02010
10. Wenhao Tang, Daniel Hillerström, J. Garrett Morris, "Soundly Handling Linearity", POPL 2024. https://arxiv.org/abs/2307.09383
11. Chris Heunen, Bart Jacobs, "Arrows, like Monads, are Monoids", MFPS 2006. https://homepages.inf.ed.ac.uk/cheunen/publications/2006/arrows/arrows.pdf
12. Caires, Pfenning, "Session Types as Intuitionistic Linear Propositions", CONCUR 2010.
13. Philip Wadler, "Propositions as Sessions", ICFP 2012.
14. Kenji Maillard et al., "Dijkstra Monads for All", POPL 2019. https://arxiv.org/abs/1903.01237
15. Nikhil Swamy et al., "Programming and Proving with Indexed Effects" (F*). https://fstar-lang.org/papers/indexedeffects/indexedeffects.pdf
16. Jonathan Brachthäuser, Philipp Schuster, Klaus Ostermann, Effekt language. https://effekt-lang.org/
17. freer-indexed Haskell package. https://hackage.haskell.org/package/freer-indexed
18. Rust Typestate Pattern. https://cliffle.com/blog/rust-typestate/

---

## Relevance to Gomputation

This document identifies the critical pressure point in Gomputation's current design: the linear composition law of `bind` is fundamentally incompatible with branching, failure, and early return when different paths produce different capability states.

The core finding is that the problem is manageable for v0 without departing from the current design commitments:

- State-homogeneous branching requires no new machinery and is the honest default.
- Total-at-boundary failure handling leverages Gomputation's defining feature: the host boundary.
- The bracket pattern provides resource safety ergonomics without linear types.
- The growth path toward dependent post-states is compatible with the spec's anticipated extension directions.

The deeper finding is that any eventual solution to branch-divergent capability states requires either dependent types (Idris), lattice structure (traditional effect systems), or algebraic effect handlers (Koka/Effekt). These are mutually compatible in theory but represent different growth directions. The spec should explicitly name this fork point so that future drafts can make the choice deliberately rather than by accident.
