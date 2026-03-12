# Indexed Monads, Parameterized Monads, and Graded Monads

Deep domain research for Gomputation's `Computation pre post a` core type.

## Table of Contents

1. Atkey's Parameterized Monads
2. McBride's Indexed Monads
3. Graded Monads (Katsumata)
4. Formal Properties and Laws
5. The Unifying View: Category-Graded Monads
6. Relationship to Typestate
7. Limitations and Pressure Points
8. Concrete Practical Examples
9. Mapping to Gomputation
10. Key References

---

## 1. Atkey's Parameterized Monads

### 1.1 Origin and motivation

Robert Atkey's "Parameterised Notions of Computation" (JFP, 2009) generalizes computational monads by allowing the type of the "world" to change across sequencing. Where an ordinary monad `M a` captures computations that produce a value of type `a`, a parameterized monad `M i j a` captures computations that produce a value of type `a` while transitioning from a state described by index `i` to a state described by index `j`.

This additional structure captures effects where the *type* of the state changes, not merely its value. Standard monadic state `State s a` fixes the state type `s` throughout the computation. The parameterized variant allows:

- Side-effects where the type of the state varies (e.g., a heterogeneous stack machine)
- Composable continuations where the answer type changes
- I/O channels where the range of inputs and outputs varies
- Protocol-indexed resources that transition between states

### 1.2 Formal definition

A parameterized monad over a set of indices `S` is a type constructor `T : S -> S -> Type -> Type` equipped with two operations:

```text
return : a -> T i i a
bind   : T i j a -> (a -> T j k b) -> T i k b
```

The categorical formulation: given a category **C** and a set S, a parameterized monad is a functor `T : S^op x S x C -> C` equipped with natural transformations:

```text
eta_{s,X}  : X -> T(s, s, X)           -- unit, natural in X, dinatural in s
mu_{s1,s2,s3,X} : T(s1, s2, T(s2, s3, X)) -> T(s1, s3, X)
                                        -- multiplication, natural in s1, s3, X
                                        -- dinatural in s2
```

The dinaturality in the intermediate index `s2` (for `eta`) and `s2` (for `mu`) reflects the "handoff" where one computation's post-state must match the next computation's pre-state.

### 1.3 The parameterized Kleisli category

Just as an ordinary monad `M` on **C** gives rise to a Kleisli category **C_M** whose morphisms are `a -> M b`, a parameterized monad `T` gives rise to a parameterized Kleisli category whose morphisms are:

```text
Hom(i, j, a, b) = a -> T i j b
```

Composition is:

```text
(g <=< f) x = bind (f x) g
-- where f : a -> T i j b, g : b -> T j k c
-- result: a -> T i k c
```

Identity is `return : a -> T i i a`.

The index composition `(i,j) ; (j,k) = (i,k)` traces the path of state transitions. This is formally a category enriched over the indiscrete category on `S` — morphisms compose only when the intermediate indices match.

### 1.4 Atkey's examples

**Variable-type state:** `IxState i j a = i -> (a, j)` — a computation that receives an initial state of type `i` and produces a result plus a final state of type `j`. This generalizes `State s a = s -> (a, s)` by allowing `i /= j`.

**Composable continuations:** `IxCont r1 r2 a = (a -> r2) -> r1` — a computation that, given a continuation expecting `a` and delivering `r2`, produces `r1`. Bind threads the intermediate answer type.

**Session-typed channels:** Protocol steps can be indexed by the session type before and after the operation. `Send Int s` transitions to `s`; `Recv Int s` transitions to `s`; `End` is terminal. Each channel operation is a parameterized monadic action.

---

## 2. McBride's Indexed Monads

### 2.1 The alternative formulation

Conor McBride's "Kleisli arrows of outrageous fortune" (2011) takes a different approach. Rather than adding two index parameters to the monad, McBride works in the category of *indexed types* and defines monads there.

An indexed type is `p : I -> Type`. Index-preserving functions are:

```text
type p :-> q = forall i. p i -> q i
```

McBride's indexed monad is a monad in the category of indexed types:

```text
class IMonad (m :: (I -> Type) -> (I -> Type)) where
    iskip   : p :-> m p
    iextend : (p :-> m q) -> (m p :-> m q)
```

This says: `m` transforms one indexed type into another, with `iskip` as unit and `iextend` as Kleisli extension.

### 2.2 Recovering parameterized structure

The connection to parameterized monads is mediated by a "singleton" indexed type:

```text
data (a := j) i where
    V :: a -> (a := j) j
```

The type `(a := j) i` is inhabited only when `i = j`, carrying a value of type `a`. Using this:

```text
ireturn : IMonad m => a -> m (a := i) i
ireturn a = iskip (V a)

(=>>=) : IMonad m => m (a := j) i -> (a -> m q j) -> m q i
c =>>= f = iextend (\(V a) -> f a) c
```

This recovers the parameterized monad signature, but with additional expressive power.

### 2.3 Key advantage: indexed choice

McBride's formulation naturally handles *branching* through indexed coproducts:

```text
data (p :\/ q) i = L (p i) | R (q i)
```

An operation with two possible outcomes can return:

```text
open : FilePath -> FH (() := Closed :\/ () := Open) Closed
```

meaning: starting from `Closed`, the result is either `() := Closed` (failure) or `() := Open` (success). A recovery combinator can then branch on the outcome:

```text
orElse : m (() := j :\/ q) i -> m (p :\/ q) j -> m (p :\/ q) i
```

### 2.4 Equivalence of formulations

The two formulations are formally equivalent in expressive power:

**Parameterized to McBride:** Given a parameterized monad `T`, define:

```text
newtype IP T p i = IP (forall k z. (forall j. p j -> T j k z) -> T i k z)
```

This wraps the parameterized monad in a continuation-passing transform that defers the choice of final index.

**McBride to parameterized:** Given an indexed monad `m`, define:

```text
newtype PI m i j a = PI (m (a := j) i)
```

The practical difference is ergonomic: McBride's formulation handles dynamic branching more naturally, while Atkey's is more intuitive for sequential state threading.

---

## 3. Graded Monads (Katsumata)

### 3.1 Definition

A graded monad (also called a *parametric effect monad*) is indexed by a single grade drawn from a monoid, rather than by a pair of pre/post indices. Formally, given a monoid `(M, @, e)` and a category **C**, an M-graded monad consists of:

```text
T   : M -> C -> C        -- a family of endofunctors indexed by M
eta : Id -> T_e           -- unit, using the monoid identity
mu  : T_m . T_n -> T_{m @ n}  -- multiplication, combining grades
```

In programming terms:

```text
return : a -> T e a
bind   : T m a -> (a -> T n b) -> T (m @ n) b
```

where `e` is the monoid identity and `@` is the monoid operation.

### 3.2 Category-theoretic formulation

An M-graded monad on **C** is equivalently a lax monoidal functor:

```text
(M, @, e) -> ([C, C], ., Id)
```

from the monoid M (viewed as a one-object monoidal category) to the monoidal category of endofunctors on **C** (with composition as the monoidal product). This is the nLab definition: a lax monoidal functor from the grading monoid to the endofunctor category.

When M is the trivial monoid (a single element), this recovers an ordinary monad.

### 3.3 Katsumata's contribution

Shin-ya Katsumata's "Parametric effect monads and semantics of effect systems" (POPL 2014) established that graded monads provide a principled semantic framework for effect systems. The key insight: an effect system annotates computations with elements of a monoid (or pre-ordered monoid / "pomonoid"), and the rules of the effect system — sequential composition combines annotations via the monoid operation, pure computations carry the identity annotation — are exactly the laws of a graded monad.

Katsumata showed that graded monads admit analogues of standard monadic constructions:

- Graded Kleisli triples
- Graded state monads
- Graded continuation monads
- Algebraic operations in the sense of Plotkin and Power
- Categorical TT-lifting

He also provided a systematic method to generate both effect annotations and a graded monad from a monad morphism.

### 3.4 Difference from parameterized monads

The fundamental structural difference:

| Property | Graded monad | Parameterized monad |
| --- | --- | --- |
| Index structure | Single grade from monoid (M, @, e) | Pair of indices from arbitrary set S |
| Composition law | `bind : T m a -> (a -> T n b) -> T (m@n) b` | `bind : T i j a -> (a -> T j k b) -> T i k b` |
| What indices mean | Abstract effect description | Pre-condition / post-condition |
| Constraint on composition | Grades combine via monoid operation | Intermediate indices must match exactly |
| Models | Effect systems | Program logics, typestate |

Graded monads say *what effects happen* (and combine them). Parameterized monads say *what state transitions happen* (and chain them). Both generalize ordinary monads, but in orthogonal directions.

---

## 4. Formal Properties and Laws

### 4.1 Parameterized monad laws

The three monad laws for a parameterized monad `T`:

**Left identity:**

```text
bind (return a) f  =  f a

-- In types: bind : T i i a -> (a -> T i k b) -> T i k b
-- return a : T i i a, so bind (return a) f : T i k b
-- f a : T i k b
```

**Right identity:**

```text
bind m return  =  m

-- In types: bind : T i j a -> (a -> T j j a) -> T i j a
-- m : T i j a, return : a -> T j j a
```

**Associativity:**

```text
bind (bind m f) g  =  bind m (\a -> bind (f a) g)

-- In types:
-- m : T i j a
-- f : a -> T j k b
-- g : b -> T k l c
-- Both sides: T i l c
```

The index threading makes these laws *more constrained* than ordinary monad laws: the intermediate indices must align at every step. This is not a bug; it is the mechanism that enforces protocol compliance.

### 4.2 Category-theoretic structure

The index composition `(i,j) ; (j,k) = (i,k)` has the structure of composition in a *free category* over the complete graph on S. More precisely:

- The indices form the objects of a thin category
- Each parameterized monadic computation `T i j a` is a morphism from `i` to `j` (carrying data `a`)
- `return` provides identity morphisms
- `bind` provides composition

This is the **parameterized Kleisli category**: it is a category whose objects are indices and whose hom-sets are `{a -> T i j b}`.

The crucial observation: the indices need *no algebraic structure* beyond equality. Unlike graded monads (which require a monoid), parameterized monads work over an arbitrary set of indices. The structure comes from the *path composition*, not from the indices themselves.

### 4.3 Graded monad laws

For a graded monad `T` over monoid `(M, @, e)`:

**Left identity:**

```text
bind (return a) f  =  f a
-- return a : T e a, f : a -> T m b
-- bind (return a) f : T (e @ m) b = T m b  (by monoid left identity)
```

**Right identity:**

```text
bind m return  =  m
-- m : T m a, return : a -> T e a
-- bind m return : T (m @ e) a = T m a  (by monoid right identity)
```

**Associativity:**

```text
bind (bind m f) g  =  bind m (\a -> bind (f a) g)
-- Left: T ((m1 @ m2) @ m3)
-- Right: T (m1 @ (m2 @ m3))
-- Equal by monoid associativity
```

The monad laws hold *up to* the monoid laws. If the monoid is only a pre-ordered monoid (pomonoid), the laws hold up to the ordering, which models subeffecting.

### 4.4 Index composition in Gomputation

For Gomputation's `Computation pre post a`, the indices are capability rows. The composition structure is:

```text
bind :
  Computation r1 r2 a ->
  (a -> Computation r2 r3 b) ->
  Computation r1 r3 b
```

The algebraic structure here is **state threading**: `r2` must match exactly between the post-state of the first computation and the pre-state of the second. This is neither a monoid (no single combining operation on rows) nor a group — it is exactly the path-composition structure of the parameterized Kleisli category.

The rows form the *objects* of a category. Each primitive operation is a *generating morphism*. `bind` composes morphisms. `pure` provides identity morphisms (where `pre = post`). The "algebra" of rows is the free category generated by the host-declared primitive transitions.

This means:

1. Rows need not have a monoidal structure. They compose by *handoff*, not by *product*.
2. The type system's job is to verify that the intermediate row `r2` is equal (under row normalization) in both occurrences.
3. The resulting paths through row-space are exactly the valid protocol traces.

---

## 5. The Unifying View: Category-Graded Monads

### 5.1 Orchard, Wadler, and Eades (2020)

The paper "Unifying graded and parameterised monads" (MSFP 2020) introduces *category-graded monads* that subsume both graded and parameterized monads as special cases. The key insight: both generalizations can be seen as special cases of viewing a monad as a lax functor.

A category-graded monad provides a family of functors `T_f` indexed by *morphisms* `f` of some category **D** (the "grading category"):

```text
T : D -> [C, C]
```

where this is a lax functor from D to the endofunctor category of C.

### 5.2 Recovering both variants

**Graded monads** arise when D is a one-object category (i.e., a monoid M). The morphisms of D are the elements of M, and `T_m` is the endofunctor graded by `m`.

**Parameterized monads** arise when D is the *indiscrete category* on a set S (all hom-sets are singletons). Each pair `(i, j)` gives exactly one morphism, and `T_{(i,j)}` is the parameterized computation from state `i` to state `j`.

**Category-graded monads** in general use an arbitrary category D. The objects of D play the role of pre/post states (like parameterized monads), while the morphisms of D play the role of effect grades (like graded monads). This enables both "ruling out certain effect compositions" (program-logic style) and "abstractly describing effects" (effect-system style) simultaneously.

### 5.3 Relevance to Gomputation

Gomputation's design sits firmly in the *parameterized* specialization: the grading category is the indiscrete category on rows, meaning any row can follow any row, but the transition is governed by the specific primitive operations the host provides. There is no separate "effect grade" — the effect is fully determined by the pre/post row pair.

If Gomputation later needs to track additional effect information beyond capability state transitions (e.g., "this computation may diverge" or "this computation performs at most n database queries"), category-graded monads provide the theoretical framework for combining both dimensions without abandoning the current structure.

---

## 6. Relationship to Typestate

### 6.1 Natural encoding of state machines

The type `M pre post a` directly encodes finite state machine transitions:

- States are types (or type-level indices)
- Transitions are computations: `M S1 S2 Unit`
- Sequencing of transitions is `bind`
- The initial state is the `pre` of the first computation
- The final state is the `post` of the last computation

For Gomputation, the states are *rows of capability states*, and each row entry can independently transition. A computation `Computation {db: DB[Closed]} {db: DB[Opened]} Unit` is a single transition in the state machine for the `db` capability.

### 6.2 Plaid and typestate-oriented programming

The Plaid language (Aldrich et al., CMU) is the most developed typestate-oriented language. Key features:

- Objects have *typestates*: a combination of interface, representation, and behavior that can change over time.
- The state change operator transitions an object from one typestate to another, potentially changing its available methods, fields, and runtime representation.
- A *permission-based type system* controls aliasing: each reference carries an access permission that governs how it interacts with other aliases.
- Gradual typing allows mixing statically and dynamically checked typestate.

Plaid's approach differs from Gomputation's in important ways:

| Aspect | Plaid | Gomputation |
| --- | --- | --- |
| Paradigm | OO with mutable objects | Functional with indexed computations |
| State bearer | Objects with mutable identity | Row entries in a capability environment |
| Alias control | Permissions (unique, immutable, shared, ...) | No aliasing: capability rows are structural descriptions |
| Transition syntax | State change operator on object | `bind` threading pre/post rows |
| Verification target | Method call sequences on objects | Capability protocol compliance |

Gomputation's pure functional core sidesteps the aliasing problem that dominates OO typestate research. In Plaid, the hard problem is: "object `x` is in state `Open`, but who else holds a reference to `x`?" In Gomputation, capability states live in rows that are structurally threaded, so there is no aliasing — the row *is* the single description of the current state.

### 6.3 Session types as a special case

Session types describe communication protocols between parties. A session type like:

```text
!Int . ?Bool . End
```

means: send an `Int`, then receive a `Bool`, then terminate. The *dual* protocol is `?Int . !Bool . End`.

Session types are a special case of parameterized monadic protocols where:

- The pre-index is the remaining session type before an operation
- The post-index is the remaining session type after an operation

```text
send : a -> Session (!a . s) s ()
recv : Session (?a . s) s a
close : Session End Done ()
```

Each operation peels off the head of the session type, transitioning to the tail. `bind` chains these transitions, and the type system ensures the full protocol is followed.

Haskell implementations include the `full-sessions` package (Keigo Imai et al.) and the `sessiontypes` package, as well as work by Sam Lindley and Garrett Morris on embedding GV-style session types. Squeal uses a similar indexed monad for tracking PostgreSQL schema migrations at the type level.

The correspondence to Gomputation is structural: capability protocols in Gomputation are to session types as open-ended state machines are to two-party communication protocols. Both use the `M pre post a` shape; they differ in what the indices denote.

---

## 7. Limitations and Pressure Points

### 7.1 Linear composition only

The bind signature requires exact index matching:

```text
bind : T i j a -> (a -> T j k b) -> T i k b
```

The intermediate index `j` must be *the same* in both occurrences. This means computations compose strictly linearly — the post-state of one computation must be exactly the pre-state of the next. There is no built-in mechanism for merging two computations that end in different states, or for running two computations "in parallel" with independent state changes.

This is by design for protocol enforcement, but it creates friction when the desired composition pattern is not purely sequential.

### 7.2 The branching problem

Consider a conditional expression:

```text
if condition
  then comp1 : Computation r1 r2 a
  else comp2 : Computation r1 r3 a
```

When `r2 /= r3`, this expression has no valid type. The two branches promise different post-states, and there is no `Computation r1 ? a` that encompasses both.

This is the *index divergence* problem, and it is fundamental: the type system enforces that the post-state is statically known, but a conditional makes it dynamically determined.

**Approaches to the branching problem:**

**(a) Require branches to agree.** The simplest solution: both branches must produce the same post-state. This is what Gomputation's current design implies. It means you cannot write `if cond then openDB else skip` unless both branches result in the same row.

```text
-- Legal: both branches end in r2
bind (if cond then comp1 else comp2) k
  where comp1 : Computation r1 r2 a
        comp2 : Computation r1 r2 a

-- Illegal: branches diverge
bind (if cond then openDB else skip) k
  where openDB : Computation {db:Closed} {db:Opened} ()
        skip   : Computation {db:Closed} {db:Closed} ()
```

**(b) Sum types in the index.** McBride's approach: allow the post-state to be a *sum* of possible states, using indexed coproducts. The computation returns `Left` or `Right` at the type level, and the continuation must handle both cases.

```text
-- The computation produces one of two possible post-states
ifOpen : Computation {db:Closed}
                     ({db:Opened} + {db:Closed})
                     ()

-- The continuation must pattern-match on the post-state
handleBoth : (a -> Computation {db:Opened} r3 b)
           -> (a -> Computation {db:Closed} r3 b)
           -> ...
```

This is expressive but significantly complicates the type system. It requires type-level sums in the row kind, plus an elimination form that dispatches on the actual post-state.

**(c) Existential post-state.** Hide the post-state behind an existential:

```text
data SomePost a = forall post. SomePost (Computation pre post a)
```

This throws away the static information about the final state, which defeats the purpose of indexed typing. It is essentially an escape hatch that says "I do not know the final state."

**(d) Explicit join points.** Require diverging branches to reconverge before proceeding:

```text
if cond
  then openDB >>= \_ -> closeDB   -- ends at {db:Closed}
  else skip                        -- ends at {db:Closed}
```

This is the most pragmatic approach for many real programs. The discipline is: every conditional must leave the capability environment in a known state before continuing. This is a reasonable design constraint for a language focused on protocol safety.

### 7.3 Early return and exceptions

Early return creates a similar problem: if a computation can "exit early," what is its post-state?

```text
earlyReturn : Computation {db:Opened} ??? a
```

The computation might either continue (ending in some expected post-state) or return early (possibly leaving the DB opened). The post-state depends on whether the early exit occurred.

**Approaches:**

**(a) No early return.** The simplest design: all computations run to completion. Errors are represented as values (`Either e a`), and the post-state is always the same regardless of success or failure.

```text
tryQuery : Query -> Computation {db:Opened} {db:Opened} (Either Error Rows)
```

This is the most compatible with parameterized monadic structure and is the natural choice for Gomputation.

**(b) Indexed exception monad.** Define an exception-like construct that carries the post-state in both the success and failure case:

```text
data IxExcept e i j a
  = IxSuccess j a      -- success: reached post-state j
  | IxFailure i e      -- failure: remained in pre-state i

bind : IxExcept e i j a -> (a -> IxExcept e j k b) -> IxExcept e i k b
bind (IxSuccess j a) f = f a     -- continue from j
bind (IxFailure i e) _ = IxFailure i e  -- propagate from i
```

This requires the failure case to specify a concrete post-state (here, remaining in the pre-state `i`). The result type is `IxExcept e i k b`, which means either success at `k` or failure at `i` — but the *outer type* still has a definite pair of indices.

**(c) Effect handler approach.** Use algebraic effect handlers to define exception-like control flow separately from the indexed structure. The handler decides what to do with the post-state upon exception. This is the most powerful approach but requires effect handlers, which Gomputation explicitly defers.

### 7.4 Composition with non-indexed code

A practical friction point: if part of the program does not change capability state (a pure computation, a helper function), it must still be given an indexed type:

```text
pureHelper : a -> b  -- pure function
-- To use in a computation:
pure (pureHelper x) : Computation r r b
```

The `pure` lifting solves this, but it adds syntactic noise. Every interaction between the indexed and non-indexed worlds requires explicit wrapping. In Haskell, `RebindableSyntax` and custom `do`-notation mitigate this; Gomputation's surface syntax should similarly provide ergonomic lifting.

### 7.5 Higher-order indexed operations

Standard monadic combinators do not generalize trivially. Consider:

```text
mapM : (a -> M b) -> [a] -> M [b]
```

For an indexed monad, the type would need to be:

```text
mapM : (a -> T i j b) -> [a] -> T i ??? [b]
```

After processing `n` elements, the index has transitioned `n` times: `i -> j -> j -> ... -> j`. But the intermediate steps are `(j, j)` — they must be uniform. So `mapM` only works for state-preserving operations. For operations that genuinely change state, the list length would need to be reflected in the type:

```text
mapM_n : (a -> T s s' b) -> Vec n a -> T (Iter n s s') (Vec n b)
```

This requires dependent types or type-level naturals, which Gomputation does not currently support. In practice, this means higher-order iteration over state-changing operations is limited to fixed, known sequences — which is, again, arguably the correct constraint for protocol-sensitive computations.

### 7.6 Inference difficulty

Type inference for parameterized monads is harder than for ordinary monads. The type checker must unify row expressions at every `bind` point, solving for intermediate row variables. For Gomputation's row-indexed computations, this means:

- Every `bind` generates a unification constraint between the post-row of the first computation and the pre-row of the second
- Row unification must handle permutation, variable solving, and the freshness side condition
- Polymorphic computations (with row variables) introduce quantifier instantiation at each use site

This is tractable for Gomputation's design because:

1. Host-declared primitives have explicit, known types
2. User programs are linear sequences of `bind` over those primitives
3. Row variables are rank-1 in the current design
4. The row structure is finite and label-unique

But it becomes significantly harder if the language later introduces higher-rank row polymorphism, type-level computation over rows, or implicit subeffecting.

---

## 8. Concrete Practical Examples

### 8.1 IxState in Haskell

The indexed state monad, where the state type can change:

```haskell
newtype IxState i j a = IxState { runIxState :: i -> (a, j) }

instance IxMonad IxState where
    ireturn a = IxState $ \s -> (a, s)
    ibind (IxState m) f = IxState $ \i ->
        let (a, mid) = m i
        in runIxState (f a) mid
```

Primitive operations:

```haskell
iget :: IxState s s s
iget = IxState $ \s -> (s, s)

iput :: j -> IxState i j ()
iput j = IxState $ \_ -> ((), j)

imodify :: (i -> j) -> IxState i j ()
imodify f = IxState $ \i -> ((), f i)
```

Usage (with `RebindableSyntax`):

```haskell
example :: IxState Int String ()
example = do
    n <- iget           -- n :: Int, state is Int
    iput (show n)       -- state transitions from Int to String
```

### 8.2 Motor: finite state machines in Haskell

The Motor library (Oskar Wickstrom) uses indexed monads with *row-typed* indices to track multiple named resources simultaneously — structurally similar to Gomputation's design:

```haskell
class IxMonad m => MonadFSM (m :: Row * -> Row * -> * -> *)

-- Actions describe state changes on named resources:
type Actions =
    '[ "door"   ':-> 'To Closed Open    -- transition door state
     , "light"  ':-> 'Add Off           -- introduce new resource
     ]
```

The row indices track a *map* from resource names to their states, and each operation modifies specific entries. This is precisely the structure of Gomputation's capability rows.

Motor's limitation: it describes only sequential composition and does not address branching with divergent states. The author noted that row polymorphism in this context is "obviously quite clumsy."

### 8.3 Squeal: schema-indexed database operations

Squeal (morphismtech) uses an Atkey-style indexed monad to track PostgreSQL schema changes at the type level:

```haskell
type PQ
  (db0 :: SchemasType)    -- schema before
  (db1 :: SchemasType)    -- schema after
  (m :: Type -> Type)     -- underlying monad
  (a :: Type)             -- result

createTable :: ... -> Definition db0 db1
  -- A Definition is a schema migration from db0 to db1

-- Migrations compose:
migrate :: Migration db0 db1 -> Migration db1 db2 -> Migration db0 db2
```

Each `Definition` (table creation, alteration, dropping) carries type-level pre/post schema information. Migrations are composed using an indexed `Category` instance, and the type system ensures that a migration chain is consistent — you cannot reference a table that has not been created yet.

### 8.4 Session-typed channels

From Pucella and Tov's "Haskell session types with (almost) no class":

```haskell
type Session s s' a  -- indexed by pre/post session type

send :: a -> Session (Send a s) s ()
recv :: Session (Recv a s) s a
close :: Session End Done ()

-- Protocol: send Int, receive Bool, close
protocol :: Session (Send Int (Recv Bool End)) Done Bool
protocol = do
    send 42
    b <- recv
    close
    return b
```

The session type is consumed left-to-right by each operation, and `bind` ensures the operations follow the protocol in order. The type system rejects any out-of-order operation.

---

## 9. Mapping to Gomputation

### 9.1 Gomputation is a parameterized monad

Gomputation's core type:

```text
Computation : Row -> Row -> Type -> Type
pure : a -> Computation r r a
bind : Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

is exactly Atkey's parameterized monad instantiated with:

- Index set S = the set of well-formed capability rows
- `return` = `pure`
- `bind` = `bind`

The operational semantics:

```text
run : Env pre -> Computation pre post a -> (Env post, a)
```

is exactly `IxState pre post a = pre -> (a, post)`, specialized to `Env` as the carrier.

### 9.2 Row indices vs scalar indices

Most theoretical examples of parameterized monads use simple scalar indices (a single state tag like `Open` or `Closed`). Gomputation's indices are *rows* — labeled, typed, and polymorphic. This is a significant enrichment:

- Rows compose by *label-wise state threading*, not by a single global transition
- Multiple capabilities transition independently within a single computation
- Row polymorphism (`| r`) allows computations to abstract over unknown capabilities

This enrichment is why Gomputation needs a row unification algorithm, not just index equality. The theoretical framework of parameterized monads says the indices must match; the practical challenge is defining what "match" means for rows with variables, permutation equivalence, and extension.

### 9.3 Gomputation is not a graded monad

Gomputation does *not* fit the graded monad pattern. In a graded monad, the grades combine via a monoid operation: `bind : T m a -> (a -> T n b) -> T (m@n) b`. The two grades `m` and `n` combine into `m@n`.

In Gomputation, the post-state of the first computation must *equal* the pre-state of the second. There is no combining operation on rows — there is a *matching constraint*. This is fundamentally the parameterized (not graded) pattern.

However, if Gomputation later adds effect annotations orthogonal to capability state (e.g., "this computation may diverge," "this computation is pure"), those annotations would naturally form a graded layer. The category-graded monad framework of Orchard et al. provides the theoretical basis for combining both dimensions: parameterized capability state + graded effect annotations.

### 9.4 Implications for the branching problem

The branching problem (Section 7.2) is the most consequential limitation for Gomputation's design evolution. The current approach — requiring all branches to agree on post-state — is sound and implementable. But it constrains what programs can express.

**Concrete scenario:**

```text
maybeOpenDB :: Bool -> Computation {db: DB[Closed] | r} ??? Unit
maybeOpenDB cond =
    if cond
        then dbOpen    -- post: {db: DB[Opened] | r}
        else pure ()   -- post: {db: DB[Closed] | r}
```

This is ill-typed under the current rules. Practical workarounds:

1. **Restructure the program:** Move the conditional *outside* the computation boundary, or ensure both branches reach the same state.

2. **Use a sum in the result type, not the index:**

    ```text
    tryOpenDB :: Computation {db: DB[Closed] | r} {db: DB[Closed] | r} (Either Error Unit)
    ```

    The DB remains closed in the type; the caller must handle the failure case and explicitly open the DB upon retry. This preserves the invariant that the post-state is statically known.

3. **Future direction: indexed sums.** If Gomputation later supports type-level sums in the row kind, McBride-style indexed choice becomes possible. This is a significant extension but aligns with the existing design trajectory.

The recommended stance for the current draft: embrace the constraint. Require branches to agree on post-state. Document this as a deliberate design choice that enforces protocol determinism. Defer indexed choice to a future extension that can be designed carefully.

### 9.5 The "no aliasing" advantage

Most of the theoretical difficulty with typestate in the literature (Plaid, DeLine-Fahndrich, Bierhoff-Aldrich) comes from aliasing: when multiple references point to the same object, a state transition through one reference invalidates assumptions made through another.

Gomputation avoids this entirely:

- Capability states are structural descriptions in rows, not references to mutable objects
- There is no aliasing of capability entries — each label appears at most once
- State transitions are enacted by `bind`, which threads the row linearly
- The runtime `Env` is managed by the interpreter, not by user code

This means Gomputation gets the safety guarantees of typestate-oriented programming without the complexity of permission-based alias control. The tradeoff is that capabilities cannot be first-class values that are passed around and aliased — they are environmental, present-or-absent, and structurally managed.

### 9.6 Future pressure toward linearity

While Gomputation's current design avoids aliasing, there is a related pressure: can a capability row entry be *duplicated*? Consider:

```text
dup : Computation {db: DB[Opened] | r}
                  {db1: DB[Opened], db2: DB[Opened] | r}
                  Unit
```

If the host allows splitting a capability into two independent copies, the type system must track both. If capabilities are inherently non-duplicable (e.g., a file handle), the type system should enforce that — but the current design has no mechanism for it.

This is where linear/affine typing becomes relevant. The current draft's design of unique labels in rows provides a weak form of uniqueness (each label appears once), but it does not prevent a host from registering a primitive that "clones" a capability. If this becomes a soundness concern, the correct extension is usage tracking in the judgment, not a change to the monad structure.

---

## 10. Key References

### Foundational papers

1. Robert Atkey, "Parameterised Notions of Computation," *Journal of Functional Programming* 19(3-4):335-376, 2009. [Author page](https://bentnib.org/paramnotions-jfp.html) | [PDF](https://bentnib.org/paramnotions-jfp.pdf)

2. Conor McBride, "Kleisli Arrows of Outrageous Fortune," 2011. [PDF](https://personal.cis.strath.ac.uk/conor.mcbride/Kleisli.pdf) | [Lambda the Ultimate discussion](http://lambda-the-ultimate.org/node/4273)

3. Shin-ya Katsumata, "Parametric Effect Monads and Semantics of Effect Systems," *POPL 2014*, pp. 633-646. [ACM DL](https://dl.acm.org/doi/10.1145/2535838.2535846) | [Author page](https://www.kurims.kyoto-u.ac.jp/~sinya/research.html)

4. Dominic Orchard, Philip Wadler, Harley Eades III, "Unifying Graded and Parameterised Monads," *MSFP 2020*, EPTCS 317, pp. 18-38. [arXiv](https://arxiv.org/abs/2001.10274)

5. Soichiro Fujii, Shin-ya Katsumata, Paul-Andre Mellies, "Towards a Formal Theory of Graded Monads," *FoSSaCS 2016*. [PDF](https://www.irif.fr/~mellies/papers/fossacs2016-final-paper.pdf) | [Springer](https://link.springer.com/chapter/10.1007/978-3-662-49630-5_30)

### nLab and formal references

6. nLab, "Graded monad." [nLab](https://ncatlab.org/nlab/show/graded+monad)

7. Gavin Karvonen, "A 2-Categorical Study of Graded and Indexed Monads," 2019. [arXiv](https://arxiv.org/pdf/1904.08083)

### Typestate and session types

8. Jonathan Aldrich et al., "The Plaid Programming Language." [CMU page](http://www.cs.cmu.edu/~aldrich/plaid/) | [Introduction](https://www.cs.cmu.edu/~aldrich/plaid/plaid-intro.pdf)

9. Jonathan Aldrich et al., "Foundations of Typestate-Oriented Programming," *ACM TOPLAS* 36(4), 2014. [PDF](http://www.cs.cmu.edu/~aldrich/papers/toplas14-typestate.pdf) | [ACM DL](https://dl.acm.org/doi/10.1145/2629609)

10. Sam Lindley, J. Garrett Morris, "Embedding Session Types in Haskell." [PDF](https://homepages.inf.ed.ac.uk/slindley/papers/gvhs.pdf)

11. Riccardo Pucella, Jesse Tov, "Haskell Session Types with (Almost) No Class," *Haskell Symposium 2008*. [ACM DL](https://dl.acm.org/doi/10.1145/1411286.1411290)

### Haskell implementations and tutorials

12. Kwang Yul Seo, "Indexed Monads" (blog). [Blog post](https://kseo.github.io/posts/2017-01-12-indexed-monads.html)

13. Adam Wespiser, "Indexed Monads" (blog). [Blog post](https://wespiser.com/posts/2020-05-06-IxMonad.html)

14. ZedneWeb, "Parameterized monads vs monads over indexed types." [Blog post](https://www.eyrie.org/~zednenem/2012/07/29/paramonads)

15. Alexander Thiemann, "An introduction to the indexed state monad in Haskell, Scala, and C#." [Gist](https://gist.github.com/pthariensflame/5054294)

16. Oskar Wickstrom, "Motor: Finite-State Machines in Haskell." [Blog post](https://wickstrom.tech/2017-10-27-motor-finite-state-machines-haskell.html)

### Libraries

17. `indexed-extras` (Hackage): IxMonad, IxState. [Hackage](https://hackage.haskell.org/package/indexed-extras-0.2)

18. `freer-indexed` (Hackage): Freer indexed monad. [Hackage](https://hackage.haskell.org/package/freer-indexed)

19. `sessiontypes` (Hackage): Session types library. [Hackage](https://hackage.haskell.org/package/sessiontypes)

20. Squeal (morphismtech): PostgreSQL with indexed schema tracking. [GitHub](https://github.com/morphismtech/squeal)

---

## Relevance to Gomputation

Gomputation's `Computation pre post a` is an Atkey parameterized monad over row-kinded indices. This is the correct theoretical framing, not merely "a monad with extra parameters."

The key consequences of this identification:

1. **The three parameterized monad laws** (Section 4.1) must hold for the language to be well-behaved. These laws should be verified in the operational semantics.

2. **Row unification is the core algorithmic challenge**, not the monad structure itself. The monad structure is simple; the difficulty is in deciding when two rows are "the same."

3. **The branching limitation is structural, not accidental.** It follows from the parameterized monad definition. Any solution (indexed sums, existentials, explicit join) is an extension of the theory, not a bug fix.

4. **Gomputation is not a graded monad** and should not be confused with effect systems that use graded monads. The capability rows are pre/post state descriptions, not effect annotations.

5. **The "no aliasing" property** gives Gomputation an unusual advantage over OO typestate systems. This should be understood as a direct consequence of the functional, row-structural design.

6. **The extension path toward richer indexed structure** (indexed sums, linear capabilities, effect grades) is well-mapped by the theoretical literature. Category-graded monads provide the unifying framework if both parameterized and graded features are eventually needed.
