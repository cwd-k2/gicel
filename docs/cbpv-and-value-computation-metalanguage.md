# Call-By-Push-Value and the Value/Computation Metalanguage

One-line description: the theoretical foundation for separating values from computations, and its precise relationship to Gomputation's `Computation pre post a` design.

## Table of Contents

1. Overview and Motivation
2. Levy's Call-By-Push-Value (CBPV)
3. Moggi's Computational Lambda Calculus
4. The Adjunction F -| U
5. The Enriched Effect Calculus and Related Systems
6. Fine-Grain Call-By-Value
7. Frank and Ability Types
8. Haskell's IO Monad
9. Graded and Indexed Extensions of CBPV
10. Mapping CBPV to Gomputation
11. Formal Typing Rules: CBPV Core
12. Formal Typing Rules: Gomputation Core
13. What the Value/Computation Split Buys and Costs
14. Key References

---

## 1. Overview and Motivation

The most fundamental design decision in Gomputation is the split between values and computations. This is not an ad hoc choice. It has a precise theoretical pedigree that runs through three lines of work:

- Moggi's computational lambda calculus (1989, 1991), which introduced monads as a uniform account of computational effects.
- Levy's call-by-push-value (CBPV, 1999--2004), which decomposed Moggi's monad into an adjunction and made the value/computation distinction syntactically and type-theoretically explicit.
- Atkey's parameterized monads (2006--2009), which generalized the monadic framework with pre/post indices -- the direct ancestor of `Computation pre post a`.

These three strands converge in Gomputation's design. Understanding each of them, and how they relate, is essential for making sound decisions about the language's type system, its operational semantics, and its future extensions.

---

## 2. Levy's Call-By-Push-Value (CBPV)

### 2.1 The Core Slogan

CBPV is organized around a single principle:

> A value *is*; a computation *does*.

Values are data at rest. Computations are processes that may perform effects, diverge, or interact with the environment before producing a result. The type system enforces this distinction syntactically: there are value types and computation types, and the two worlds are connected by explicit coercions.

### 2.2 Value Types and Computation Types

**Value types** (positive types, written A+, A, or with superscript +) classify data that can be stored, duplicated, and discarded freely:

```text
A ::= 1               -- unit
    | A x A            -- product
    | A + A            -- sum / coproduct
    | U B              -- thunk of a computation (the "forgetful" shift)
```

**Computation types** (negative types, written B-, B, or with superscript -) classify running processes:

```text
B ::= A -> B           -- function taking a value, producing a computation
    | F A              -- returner: a computation that produces a value (the "free" shift)
    | B & B            -- lazy product / record of computations
```

The critical insight is that **function types are computation types**, not value types. A function `A -> B` is a computation that, given a value of type A, runs as a computation of type B. This is a departure from the simply typed lambda calculus, where `A -> B` lives in the same universe as A and B.

### 2.3 The Shift Types: F and U

The two shift types mediate between the value and computation worlds:

**F A** (read "free A", "returner of A", or "produces A"):
- F A is a **computation type**.
- It classifies computations that may perform effects and then return a value of type A.
- Introduction: `return V` (or `produce V`) creates a computation of type F A from a value V : A.
- Elimination: `M to x. N` (or `let val x = M in N`) sequences: run M : F A, bind the result to x : A, then run N : B.

**U B** (read "thunk of B"):
- U B is a **value type**.
- It classifies suspended computations: a value of type U B is a frozen computation of type B, not yet executed.
- Introduction: `thunk M` creates a value of type U B from a computation M : B.
- Elimination: `force V` starts executing the suspended computation: given V : U B, it produces a computation of type B.

The key equations are:

```text
force (thunk M) = M          -- beta for U
thunk (force V) = V          -- eta for U
(return V) to x. N = N[V/x]  -- beta for F
M to x. return x = M         -- eta for F
```

### 2.4 Typing Judgments

CBPV has two typing judgments sharing a single context of value-typed variables:

```text
Gamma |- V : A      -- value judgment: V is a value of value type A
Gamma |- M : B      -- computation judgment: M is a computation of computation type B
```

The context Gamma contains only value-typed bindings `x : A`. This is because values are the things that can be stored in variables; computations cannot be stored -- they must be thunked first to become values.

### 2.5 Core Typing Rules

#### Values

```text
                x : A in Gamma
  VAR  ----------------------------------------
                Gamma |- x : A


  UNIT ----------------------------------------
                Gamma |- () : 1


         Gamma |- V1 : A1    Gamma |- V2 : A2
  PAIR ----------------------------------------
          Gamma |- (V1, V2) : A1 x A2


                Gamma |- M : B
  THUNK ----------------------------------------
             Gamma |- thunk M : U B
```

#### Computations

```text
               Gamma |- V : U B
  FORCE ----------------------------------------
              Gamma |- force V : B


               Gamma |- V : A
  RETURN ----------------------------------------
            Gamma |- return V : F A


      Gamma |- M : F A    Gamma, x : A |- N : B
  TO ----------------------------------------
          Gamma |- M to x. N : B


            Gamma, x : A |- M : B
  LAMBDA ----------------------------------------
           Gamma |- lambda x. M : A -> B


       Gamma |- M : A -> B    Gamma |- V : A
  APP ----------------------------------------
              Gamma |- M V : B
```

Note the asymmetry in the application rule: the function position M is a computation, but the argument V is a value. This is CBPV's "push-value" discipline: the caller evaluates the argument to a value and pushes it to the callee.

### 2.6 How CBPV Subsumes Call-By-Value and Call-By-Name

CBPV is not a third evaluation strategy alongside CBV and CBN. It is a substrate into which both embed as translations.

**Call-by-value embedding:** A CBV function type `A =>_v B` translates to `U(A -> F B)`. That is: a thunked computation that takes a value argument and returns (with effects) a value result. CBV application evaluates the argument first (to a value), then forces the thunk and applies.

```text
CBV:   A =>_v B    ~>    U(A -> F B)
```

**Call-by-name embedding:** A CBN function type `A =>_n B` translates to `U A -> B`. That is: a computation that takes a thunked argument (not yet evaluated) and computes. CBN application passes the argument unevaluated (as a thunk).

```text
CBN:   A =>_n B    ~>    U A -> B
```

The monad `T` that appears in Moggi's metalanguage corresponds to the composite `U . F`:

```text
T A  =  U(F A)
```

This is a value type: it is a thunked computation that, when forced, may perform effects and return a value of type A.

### 2.7 Operational Semantics

CBPV has a natural big-step operational semantics. The key insight is that **only computations evaluate** -- values are already in normal form.

Terminal computations are values under `return`:

```text
return V    -- terminal: computation that has produced value V
lambda x. M -- terminal: computation waiting for an argument
```

The evaluation rules:

```text
M => return V    N[V/x] => T
-------------------------------
    M to x. N => T

M => lambda x. N    N[V/x] => T
---------------------------------
        M V => T

force (thunk M) => T   when   M => T
```

There is also a stack machine presentation where computations are evaluated against stacks (continuations), giving an imperative reading:

- `return V` pops the current continuation from the stack and passes V to it.
- `M to x. N` pushes the continuation `x. N` onto the stack, then evaluates M.
- `lambda x. M` pops a value argument from the stack and substitutes.
- `force (thunk M)` unwraps and continues.

This is why Levy calls it "push-value": values are pushed onto the stack for consumption by computations.

---

## 3. Moggi's Computational Lambda Calculus

### 3.1 The Original Framework

Moggi's 1989/1991 papers introduced the idea that computational effects can be modeled uniformly using a monad `(T, eta, mu)` on the category of types and terms.

The key insight: in a pure lambda calculus, a term of type `A -> B` denotes a function from A-values to B-values. In an effectful language, it denotes a function from A-values to B-*computations*: processes that may diverge, raise exceptions, perform I/O, etc. Moggi captured this by interpreting types in a category C but morphisms (programs) in the Kleisli category C_T of a monad T on C.

### 3.2 The Metalanguage

Moggi's computational metalanguage is a typed lambda calculus extended with:

- A type constructor `T A` (the computation type for A).
- `val V : T A` when `V : A` (embedding a value as a trivial computation).
- `let x <= M in N : T B` when `M : T A` and `x : A |- N : T B` (sequencing).

The typing rules:

```text
         Gamma |- V : A
  VAL ---------------------
      Gamma |- val V : T A


   Gamma |- M : T A    Gamma, x : A |- N : T B
  LET -----------------------------------------------
        Gamma |- let x <= M in N : T B
```

The equational laws are exactly the monad laws:

```text
let x <= val V in N  =  N[V/x]              -- left unit
let x <= M in val x  =  M                   -- right unit
let y <= (let x <= L in M) in N              -- associativity
    =  let x <= L in (let y <= M in N)
```

### 3.3 Relationship to CBPV

Moggi's metalanguage does not distinguish between value types and computation types at the syntactic level. Every type A is a "value type," and T A is the type of computations returning A. This means:

- There is only one universe of types.
- The computation structure is carried entirely by the monad T.
- Thunking is implicit: a function `A -> T B` can be stored and passed around because `A -> T B` is an ordinary type.

CBPV refines this by decomposing T into F and U:

```text
T A  =  U(F A)
```

and making the value/computation boundary syntactically explicit. The advantage: CBPV can express distinctions that Moggi's metalanguage conflates. For instance, in CBPV, `A -> B` (computation function) and `U(A -> B)` (thunked function, a value) are different things. In Moggi's system, functions are always values.

### 3.4 The Kleisli Category

Given a monad `(T, eta, mu)` on a category C, the Kleisli category C_T has:

- Objects: the same as C (i.e., value types).
- Morphisms A -> B in C_T: morphisms A -> T B in C (i.e., effectful functions).
- Identity: eta_A : A -> T A.
- Composition: Kleisli composition using mu (the monad multiplication).

Programs in Moggi's metalanguage are interpreted as morphisms in C_T. This is the categorical semantics of "functions that may have effects."

CBPV's refinement: instead of working with a monad on a single category, CBPV works with an adjunction `F -| U` between two categories -- the category of value types (V) and the category of computation types (C). The monad `T = U . F` is then derived, and the Kleisli category of T is equivalent to a certain construction from this adjunction.

---

## 4. The Adjunction F -| U

### 4.1 The Categorical Structure

The categorical semantics of CBPV is based on an adjunction between two categories:

- **V**: the category of value types (with products, coproducts -- cartesian structure).
- **C**: the category of computation types (with function spaces and products -- cartesian closed structure for the negative connectives).

The adjunction consists of two functors:

```text
F : V -> C      (the "free" functor, left adjoint)
U : C -> V      (the "forgetful"/"thunk" functor, right adjoint)
F -| U
```

The adjunction `F -| U` means:

```text
Hom_C(F A, B)  ~=  Hom_V(A, U B)
```

In programming terms: a computation that takes a produced value of type A and runs as B is in natural bijection with a value-level function from A to thunked B. This is exactly the thunk/force correspondence.

### 4.2 The Derived Monad and Comonad

From any adjunction `F -| U`, we get:

- A monad `T = U . F` on V (the value category). This is Moggi's T.
- A comonad `D = F . U` on C (the computation category).

The unit of the monad `eta : Id_V -> U . F` corresponds to `return` followed by `thunk`.
The multiplication `mu : U . F . U . F -> U . F` corresponds to `force` followed by sequencing followed by `thunk`.

### 4.3 Kleisli and Co-Kleisli

From the adjunction, two canonical category constructions arise:

- The **Kleisli category** of T = UF: objects are value types, morphisms A -> B are value-level maps A -> U(F B). This is the CBV embedding: effectful functions from values to computations.

- The **co-Kleisli category** of D = FU: objects are computation types, morphisms B -> B' are computation-level maps F(U B) -> B'. This is the CBN embedding: computations that consume thunked arguments.

This is Levy's central decomposition theorem: CBV and CBN are not ad hoc strategies but arise canonically from the same adjunction, viewed from different sides.

### 4.4 The Mode Theory

In recent formulations (following Licata, Shulman, and Riley's framework), CBPV can be presented as an adjoint logic with two modes:

- **v** (the value mode): forms a cartesian monoid (v, x, 1).
- **c** (the computation mode): computations live here.

The mode morphisms capture the adjunction:

- F : v -> c (left adjoint)
- U : c -> v (right adjoint)

The value mode has contraction and weakening (values can be duplicated and discarded). The computation mode is more constrained: computations are used linearly in sequencing, though the context of value variables is still structural.

---

## 5. The Enriched Effect Calculus and Related Systems

### 5.1 The Enriched Effect Calculus (EEC)

The Enriched Effect Calculus, developed by Egger, Mogelberg, and Simpson, extends Moggi's computational metalanguage with primitives from linear logic. Its purpose: express linear aspects of computational effects -- for instance, the linear usage of continuations or state.

The EEC is closely related to both Moggi's metalanguage and Levy's CBPV. It extends a "basic effect calculus" (which is essentially a cleaned-up version of Moggi/CBPV) with:

- A linear function type (from linear logic).
- Controlled duplication and discarding of computations.

Semantic results for the EEC include soundness, completeness, initiality of the syntactic model, and an embedding theorem: every model of the basic effect calculus fully and faithfully embeds in a model of the enriched calculus.

### 5.2 Effect Information on Computation Types

In the basic CBPV framework, computation types carry no explicit effect annotations. A computation of type F A may perform any effect allowed by the ambient semantics. To track which effects a computation actually uses, one needs an additional layer:

- **Effect rows** (as in Koka, Eff, Links): attach a row of effect labels to the computation type, e.g., `F_{IO, Exn} A`.
- **Graded monads** (Katsumata, Fujii, Gaboardi et al.): parameterize the monad by an element of an effect algebra, e.g., `T_e A` where e is drawn from a semiring or effect monoid.
- **Indexed monads** (Atkey): parameterize by pre/post indices, e.g., `T_{s1,s2} A`.

Gomputation's `Computation pre post a` falls in the indexed-monad family, where the indices are row-kinded capability environments. This is a richer annotation than effect labels alone, because it tracks not just *which* effects are used but *how the capability state changes*.

### 5.3 Extended Call-by-Push-Value (Kammar, Pretnar)

Extended CBPV adds effect-dependent reasoning to the CBPV framework. It allows reasoning about effectful programs and evaluation order within a single calculus, and has been used to establish contextual equivalences for effectful programs.

---

## 6. Fine-Grain Call-By-Value

### 6.1 The Calculus

Fine-grain call-by-value (FGCBV) was introduced by Levy, Power, and Thielecke (2003) as a refinement of Moggi's metalanguage. It addresses a specific problem: in Moggi's system, the `let` construct conflates two operations:

1. Sequencing: running a computation and naming its result.
2. Evaluation: reducing a term to a value.

FGCBV separates these by restricting substitution so that only values (not arbitrary terms) can be substituted for variables. This gives a calculus that is:

- Still monadic (it has `return` and `bind`).
- But with a finer-grained operational semantics that matches CBV evaluation exactly.
- Intermediate between Moggi's metalanguage and full CBPV.

### 6.2 Relationship to CBPV

FGCBV can be seen as a fragment of CBPV that retains the monadic interface but makes the value restriction explicit. CBPV goes further by splitting the type universe in two, introducing U and F as first-class type constructors, and allowing computation types that are not of the form F A (e.g., function types A -> B).

In practice, FGCBV is useful as an intermediate step for understanding the CBV embedding of CBPV, or as a target language for compilers that want monadic structure without full polarization.

---

## 7. Frank and Ability Types

### 7.1 Frank's Design

Frank is a programming language designed by Conor McBride (with Lindley and McLaughlin) that combines CBPV's value/computation distinction with algebraic effects and handlers.

Frank's key innovations:

- **Ability types**: effect types in Frank are called "abilities." An ability denotes permission to invoke a set of commands (effect operations).
- **Ambient ability**: rather than accumulating effect rows outward (as in row-typed effect systems), Frank propagates an ambient ability inward. Functions implicitly have access to the ambient ability of their call site.
- **No effect variables in source code**: Frank's effect polymorphism avoids explicit effect variables. Instead, the ambient ability is threaded implicitly, and handlers delimit its scope.

### 7.2 Relationship to CBPV

Frank explicitly builds on CBPV's value/computation distinction:

- Value types and computation types are separate.
- Function types are computation types (as in CBPV).
- Thunking is explicit (as in CBPV).

The difference: Frank integrates algebraic effect handlers as a first-class feature, while CBPV itself is agnostic about the source of effects.

### 7.3 Relevance to Gomputation

Gomputation does *not* adopt algebraic effect handlers (this is an explicit non-goal of the current draft). But Frank's design validates a key architectural choice: the value/computation distinction from CBPV is compatible with rich effect typing. If Gomputation ever adds handlers, the CBPV foundation will support it.

Frank's ambient-ability approach is also worth noting as an alternative to Gomputation's explicit row threading. Gomputation chose explicit rows because they support typestate (pre/post transitions), which ambient abilities do not naturally capture.

---

## 8. Haskell's IO Monad

### 8.1 The Design

Haskell's approach to effects is Moggi-style, not CBPV-style. The `IO` type constructor plays the role of Moggi's `T`:

```haskell
return :: a -> IO a
(>>=)  :: IO a -> (a -> IO b) -> IO b
```

All effectful operations live inside `IO`. Pure functions live outside. The `do` notation is syntactic sugar for `>>=` (bind).

### 8.2 Differences from CBPV

1. **No value/computation type split**: Haskell has a single universe of types. `IO a` is an ordinary type, not a computation type in a separate judgment. A variable `x : IO Int` is perfectly legal -- you can store, duplicate, and pass around IO actions freely.

2. **Thunking is implicit**: In Haskell, `IO a` already represents a suspended computation (thanks to laziness). There is no need for an explicit `thunk`/`force`. This conflates `U(F A)` into a single `IO A`.

3. **No decomposition of the monad**: Haskell does not decompose `IO` into `F` and `U`. The monad is primitive and opaque.

4. **Effect polymorphism via type classes**: Haskell achieves effect abstraction through `MonadIO`, `MonadReader`, etc. -- not through an adjunction or polarized type system.

### 8.3 Consequences

Haskell's approach is simpler but less informative. Because `IO a` is an ordinary type, the type system cannot distinguish between "a computation that is running" and "a description of a computation that is stored as data." In CBPV, this distinction is syntactic and type-enforced. In Haskell, it is a matter of convention (and laziness).

For Gomputation, the CBPV-style split is more appropriate because:

- The language is embedded (the host needs to control what runs and when).
- Capability environments change across computations (requiring indexed types, not just a monolithic `IO`).
- Determinism requires explicit sequencing, which CBPV naturally provides.

---

## 9. Graded and Indexed Extensions of CBPV

### 9.1 Graded CBPV (McDermott, FSCD 2025)

Recent work by Dylan McDermott introduces call-by-push-value with effects (CBPVE), a refinement of CBPV where computation types carry explicit behavioral information about effects.

In CBPVE:

- The `F` type constructor is parameterized by a grade: `F_e A`, where `e` is drawn from an effect algebra (e.g., a semiring of effect labels).
- The `return` rule produces grade `1` (the unit of the effect algebra): `return V : F_1 A`.
- The `to` rule composes grades: if `M : F_e A` and `N : F_f B` (with x : A in context for N), then `M to x. N : F_{e . f} B`.

This is a systematic way to annotate CBPV computations with effect information. The paper proves coherence: under mild conditions on the grade algebra, the graded typing is consistent.

### 9.2 CBPV with Effects and Coeffects (PLClub, UPenn)

The PLClub formalization extends CBPV with both effect tracking (what a computation does) and coeffect tracking (what a computation requires from its context). This is particularly relevant for Gomputation, where the `pre` row is a coeffect (required capabilities) and the `post` row is an effect (produced capability state).

### 9.3 Atkey's Parameterized Monads

Atkey's parameterized (indexed) monads generalize ordinary monads by indexing the type constructor with pre and post states:

```text
T : S^op x S x C -> C
```

The operations:

```text
eta   : A -> T(s, s, A)                        -- return preserves state
bind  : T(s1, s2, A) -> (A -> T(s2, s3, B)) -> T(s1, s3, B)  -- bind composes states
```

The laws correspond to the indexed monad laws:

```text
bind (eta a) f     =  f a                    -- left unit
bind m eta         =  m                      -- right unit
bind (bind m f) g  =  bind m (\x -> bind (f x) g)  -- associativity
```

This is the direct theoretical basis for Gomputation's `Computation pre post a`.

### 9.4 Dependent Types and Fibred Effects (Ahman, Ghani)

Ahman and Ghani's work on dependent types and fibred computational effects provides a framework for CBPV where computation types can depend on values. This is relevant if Gomputation ever moves toward dependent indexing of capability states (e.g., a database connection indexed by the number of open transactions).

---

## 10. Mapping CBPV to Gomputation

### 10.1 The Correspondence

Gomputation's design maps onto CBPV concepts as follows:

| CBPV Concept | Gomputation Concept |
|---|---|
| Value type A | Pure types: `Int`, `String`, `A -> B`, `forall a. T` |
| Computation type B | `Computation R1 R2 A` |
| F A (returner) | `Computation r r A` (effect-free return) |
| U B (thunk) | Not explicit; functions `A -> Computation R1 R2 B` serve as thunks |
| `return V` | `pure v` |
| `M to x. N` | `bind c (\x -> c')` |
| `thunk M` | Lambda abstraction: `\() -> c` or implicit in function definitions |
| `force V` | Application: `f ()` or implicit in `bind` |

### 10.2 Where Gomputation Diverges from Vanilla CBPV

**1. Indexed computation types.** Vanilla CBPV has `F A` as the base computation type -- it carries no state index. Gomputation uses `Computation pre post A`, which is an indexed (parameterized) version of F:

```text
CBPV:         F A                -- computation returning A
Gomputation:  Computation r r A  -- computation returning A, preserving state r
Gomputation:  Computation r1 r2 A -- computation returning A, transitioning r1 to r2
```

This is the Atkey parameterized monad applied to CBPV's F type constructor.

**2. No explicit U type constructor.** Gomputation does not have a first-class `thunk` type. Instead, the role of U is played by function types: a thunked computation is represented as `Unit -> Computation pre post A` or, more idiomatically, as a function that closes over its environment. This is a deliberate simplification that trades the full generality of CBPV's polarized type system for the familiarity of the Haskell-like surface.

**3. Row-kinded indices.** CBPV's F and U are parameterized only by types. Gomputation's `Computation` is parameterized by rows (capability environments). This is an orthogonal enrichment: the row system provides the fine-grained capability tracking, while the value/computation split provides the sequencing discipline.

**4. Function types are value types.** In Gomputation (following the spec draft v0.2), `A -> B` is a value type, and function abstraction is a value-level construct. This differs from vanilla CBPV, where `A -> B` is a computation type. The practical consequence: in Gomputation, you can pass functions as values without explicit thunking. This is closer to the CBV embedding of CBPV (where functions are `U(A -> F B)`, a value type) than to raw CBPV.

### 10.3 The Subtle Question: Function Types and Computation

In CBPV, `A -> B` is a computation type. A function is a computation waiting for an argument. To store a function as a value, you must thunk it: `U(A -> B)`.

In Gomputation, `A -> B` is a value type. Functions can be stored, passed, and returned freely. But `A -> Computation r1 r2 B` is also a value type -- it is a function that, when applied, yields a computation.

This means Gomputation implicitly identifies:

```text
CBPV:          U(A -> F B)     -- a value: thunked effectful function
Gomputation:   A -> Computation r r B  -- a value: function returning a computation
```

The identification is safe as long as the language is strict at the value level (which the spec intends). In a lazy language, this identification would conflate thunking with laziness, but Gomputation's determinism commitment requires strict value evaluation.

### 10.4 The `pure`/`bind` Interface

Gomputation's `pure` and `bind` correspond exactly to CBPV's `return` and `to`, enriched with row indices:

```text
CBPV:
  return V  : F A                 -- given V : A
  M to x. N : B                   -- given M : F A, x : A |- N : B

Gomputation:
  pure v    : Computation r r A   -- given v : A
  bind c k  : Computation r1 r3 B -- given c : Computation r1 r2 A,
                                   --       k : A -> Computation r2 r3 B
```

The row index composition in `bind` is the key additional structure: the post-state of the first computation must equal the pre-state of the continuation. This is exactly the composition law of Atkey's parameterized monad, applied to row-kinded indices.

---

## 11. Formal Typing Rules: CBPV Core

The following is a self-contained presentation of the core CBPV typing rules in judgment notation.

### 11.1 Syntax

```text
Value types    A ::= 1 | A x A | A + A | U B
Comp. types    B ::= A -> B | F A | B & B

Values         V ::= x | () | (V, V) | inl V | inr V | thunk M
Computations   M ::= force V | return V | M to x. N
                    | lambda x. M | M V
                    | (M, M) | fst M | snd M
                    | case V of inl x => M | inr y => N
```

### 11.2 Value Typing: Gamma |- V : A

```text
  x : A in Gamma
  ──────────────────  [Var]
  Gamma |- x : A


  ──────────────────  [Unit]
  Gamma |- () : 1


  Gamma |- V1 : A1    Gamma |- V2 : A2
  ──────────────────────────────────────  [Pair]
  Gamma |- (V1, V2) : A1 x A2


  Gamma |- V : A1
  ──────────────────────────  [Inl]
  Gamma |- inl V : A1 + A2


  Gamma |- V : A2
  ──────────────────────────  [Inr]
  Gamma |- inr V : A1 + A2


  Gamma |- M : B
  ──────────────────────────  [Thunk]
  Gamma |- thunk M : U B
```

### 11.3 Computation Typing: Gamma |- M : B

```text
  Gamma |- V : U B
  ──────────────────────  [Force]
  Gamma |- force V : B


  Gamma |- V : A
  ────────────────────────────  [Return]
  Gamma |- return V : F A


  Gamma |- M : F A    Gamma, x : A |- N : B
  ─────────────────────────────────────────────  [To/Bind]
  Gamma |- M to x. N : B


  Gamma, x : A |- M : B
  ──────────────────────────────────  [Lambda]
  Gamma |- lambda x. M : A -> B


  Gamma |- M : A -> B    Gamma |- V : A
  ──────────────────────────────────────────  [App]
  Gamma |- M V : B


  Gamma |- V : A1 + A2
  Gamma, x : A1 |- M1 : B
  Gamma, y : A2 |- M2 : B
  ──────────────────────────────────────────────────────  [Case]
  Gamma |- case V of inl x => M1 | inr y => M2 : B
```

### 11.4 Beta and Eta Principles

```text
  force (thunk M)  =  M                           -- U-beta
  thunk (force V)  =  V                           -- U-eta
  (return V) to x. N  =  N[V/x]                   -- F-beta
  M to x. return x  =  M                          -- F-eta
  (lambda x. M) V  =  M[V/x]                      -- arrow-beta
  lambda x. M x  =  M    (x not free in M)        -- arrow-eta
```

### 11.5 The Monad Laws (derived)

Defining `T A = U(F A)` and the monadic operations:

```text
  unit : A -> T A
  unit V = thunk (return V)

  bind : T A -> (A -> T B) -> T B
  bind t f = thunk (force t to x. force (f x))
```

The monad laws follow from the beta/eta principles above.

---

## 12. Formal Typing Rules: Gomputation Core

For comparison, here are the corresponding rules from the Gomputation spec, presented in the same notation and with the CBPV correspondence annotated.

### 12.1 Syntax

```text
Types    T ::= a | T -> T | forall a. T | Computation R R T | C
Rows     R ::= {} | { l : T | R } | r

Values   v ::= x | \x -> e | f v | (e : T) | ...
Comps    c ::= pure e | bind c (\x -> c) | prim
```

### 12.2 Value Typing: Gamma |- v : T

```text
  x : A in Gamma
  ──────────────────  [Var]
  Gamma |- x : A


  Gamma, x : A |- e : B
  ──────────────────────────────  [Lam]      (cf. CBPV: values, not computations)
  Gamma |- \x -> e : A -> B


  Gamma |- f : A -> B    Gamma |- u : A
  ──────────────────────────────────────  [App]
  Gamma |- f u : B


  Gamma |- e : forall a. T    Gamma |- A : Type
  ────────────────────────────────────────────────  [TyInst]
  Gamma |- e : T[a := A]
```

### 12.3 Computation Typing: Gamma |- c : Computation R1 R2 A

```text
  Gamma |- e : A
  ────────────────────────────────────────  [Pure]    (cf. CBPV Return)
  Gamma |- pure e : Computation r r A


  Gamma |- c1 : Computation r1 r2 A
  Gamma, x : A |- c2 : Computation r2 r3 B
  ────────────────────────────────────────────────────────  [Bind]  (cf. CBPV To)
  Gamma |- bind c1 (\x -> c2) : Computation r1 r3 B
```

### 12.4 Correspondence Table

```text
  CBPV return V           <->   Gomputation pure v
  CBPV M to x. N          <->   Gomputation bind c (\x -> c')
  CBPV thunk M            <->   Gomputation \() -> c  (or implicit)
  CBPV force V             <->   Gomputation f ()  (or implicit in bind)
  CBPV F A                <->   Gomputation Computation r r A  (degenerate case)
  CBPV U B                <->   Gomputation not explicit; embedded in function types
  CBPV A -> B  (comp)     <->   Gomputation A -> Computation R1 R2 B  (value-level fn)
```

---

## 13. What the Value/Computation Split Buys and Costs

### 13.1 What It Buys

**Clear effect boundaries.** Every effectful operation is typed as a `Computation`. Pure code cannot accidentally trigger effects. The boundary is not a convention (as in Haskell, where purity is enforced by the type of `unsafePerformIO` being "don't use this") but a structural property of the type system.

**Type-directed compilation.** A compiler or interpreter can treat values and computations differently. Values can be stored, duplicated, and inlined freely. Computations must be sequenced, and their effects must be tracked. This enables optimizations that would be unsound without the distinction (e.g., eliminating redundant value computations, but not redundant effectful computations).

**Explicit sequencing.** The `bind` combinator makes the order of effects explicit in the syntax. There is no question about evaluation order: effects happen in the order dictated by bind chains. This is essential for Gomputation's determinism commitment.

**Typestate compositionality.** The pre/post row indices compose through `bind`. Each step in a computation transforms the capability environment, and the type system tracks the cumulative transformation. This would be impossible without the explicit sequencing that the value/computation split provides.

**Host boundary clarity.** The host registers primitives with computation types. User code cannot forge computations -- it can only sequence them. The value/computation split makes this authority boundary precise.

### 13.2 What It Costs

**Explicit lifting.** Every pure value that participates in a computation must be lifted with `pure`. This is syntactic overhead. A `do`-notation or computation expression syntax can mitigate it, but the underlying structure remains.

**No implicit effects.** Unlike languages where any function can silently perform effects, Gomputation requires every effectful operation to appear inside a `Computation` type. This is a feature for safety but a cost for convenience.

**Function types alone don't carry effect information.** The type `A -> B` says nothing about effects. To express an effectful function, one must write `A -> Computation r1 r2 B`. This is more verbose than systems where function types are annotated with effect rows (e.g., `A ->{IO} B` in Koka).

**Thunking is not first-class.** Without an explicit `U` type constructor, Gomputation cannot express "a suspended computation as a value" as cleanly as CBPV. The workaround (wrapping in a lambda) is adequate but less principled.

### 13.3 The Balance

For Gomputation's intended use cases -- embedded scripting, domain logic, protocol-controlled execution -- the benefits clearly outweigh the costs. The verbosity cost is manageable for small programs in controlled domains. The safety benefits are essential for a capability-secure, deterministic embedded language.

If the language grows toward general-purpose programming, the cost/benefit analysis may shift, and an explicit `U` type or effect-annotated function types might become worth adding. But for the current design scope, the simplified CBPV-via-indexed-monad approach is well-fitted.

---

## 14. Key References

### Foundational

1. Paul Blain Levy. *Call-By-Push-Value: A Functional/Imperative Synthesis*. Springer, 2004. The definitive treatment. Levy's PhD thesis (QMUL, 2001) contains the same material in longer form.

2. Paul Blain Levy. "Call-by-push-value: Decomposing call-by-value and call-by-name." *Higher-Order and Symbolic Computation*, 19(4):377--414, 2006. The journal paper most commonly cited for the core CBPV calculus.

3. Eugenio Moggi. "Computational lambda-calculus and monads." *Proceedings of LICS*, 1989. The original paper introducing monads for computational effects.

4. Eugenio Moggi. "Notions of computation and monads." *Information and Computation*, 93(1):55--92, 1991. The expanded journal version with the full categorical semantics.

5. Robert Atkey. "Parameterised notions of computation." *Journal of Functional Programming*, 19(3-4):335--376, 2009. (Earlier workshop version 2006.) The indexed/parameterized monad framework that directly underlies `Computation pre post a`.

### Extensions and Variants

6. Paul Blain Levy, John Power, and Hayo Thielecke. "Modelling environments in call-by-value programming languages." *Information and Computation*, 185(2):182--210, 2003. Fine-grain call-by-value.

7. Jeff Egger, Rasmus Ejlers Mogelberg, and Alex Simpson. "The enriched effect calculus: syntax and semantics." *Journal of Logic and Computation*, 24(3):615--654, 2014. The EEC extending Moggi/CBPV with linear primitives.

8. Sam Lindley, Conor McBride, and Craig McLaughlin. "Do be do be do." *Proceedings of POPL*, 2017. The Frank language: CBPV + algebraic effects + ambient abilities.

9. Dylan McDermott. "Grading Call-By-Push-Value, Explicitly and Implicitly." *Proceedings of FSCD*, LIPIcs vol. 337, 2025. Graded effect annotations on CBPV computation types.

10. Danel Ahman and Neil Ghani. "Dependent types and fibred computational effects." *Proceedings of FoSSaCS*, 2016. CBPV with dependent types.

### Course Notes and Tutorials

11. Robert Harper. "Polarization: Call-by-Push-Value." Course notes for 15-819 (Advanced Topics in Programming Languages), CMU, Spring 2025. Available at: https://www.cs.cmu.edu/~rwh/courses/atpl/pdfs/cbpv.pdf

12. Frank Pfenning. "Lecture Notes on Call-by-Push-Value." Course 15-816 (Substructural Logics), CMU, Fall 2016. Available at: https://www.cs.cmu.edu/~fp/courses/15816-f16/lectures/21-cbpv.pdf

13. Paul Blain Levy. "Call-By-Push-Value (introductory article)." Available at: https://www.khoury.northeastern.edu/home/cmartens/Courses/7400-f24/readings/cbpv-intro.pdf

14. Burak Emir. "CBPV and Natural Deduction" (blog series). Available at: https://burakemir.ch/post/cbpv-pt1-small-steps/

15. nLab. "call-by-push-value." Available at: https://ncatlab.org/nlab/show/call-by-push-value

### Levy's Own Resources

16. Paul Blain Levy. "Call-By-Push-Value FAQ." Available at: https://pblevy.github.io/cbpv.html

---

## Appendix A: Categorical Summary

For readers comfortable with category theory, the entire CBPV story can be stated in one paragraph:

A CBPV model consists of a category **V** (values) with finite products and coproducts, a category **C** (computations), and an adjunction F -| U : **C** -> **V**. The left adjoint F : **V** -> **C** is the "free computation" functor; the right adjoint U : **C** -> **V** is the "thunk" functor. The composite T = UF : **V** -> **V** is a monad on values, and its Kleisli category is the category of effectful value-to-value transformations (the CBV world). The composite D = FU : **C** -> **C** is a comonad on computations, and its co-Kleisli category captures the CBN world. The adjunction decomposes the semantics of any effectful language that can be modeled by a strong monad on **V** into a finer-grained structure where the distinction between "data at rest" and "process in motion" is made explicit.

For Gomputation, the enrichment is: the functor F is parameterized by pairs of objects from a row category **R**, giving a family of functors F_{r1,r2} : **V** -> **C** for each pair (r1, r2) of capability environments. The composition law of `bind` corresponds to the composition of these parameterized functors via the matching condition r2 = r2.

## Appendix B: Glossary

| Term | Definition |
|---|---|
| **Value type** | A type classifying data that can be stored, duplicated, and discarded. |
| **Computation type** | A type classifying running processes that may perform effects. |
| **F A** | The computation type "produces a value of type A" (possibly with effects). |
| **U B** | The value type "suspended computation of type B" (a thunk). |
| **return / produce** | Embeds a value into a trivial computation: `return V : F A`. |
| **to / bind / sequence** | Sequences two computations: run the first, name the result, run the second. |
| **thunk** | Suspends a computation into a value: `thunk M : U B`. |
| **force** | Resumes a suspended computation: `force V : B`. |
| **Polarization** | The classification of types into positive (value) and negative (computation). |
| **Adjunction** | F -| U: the categorical structure underlying the value/computation split. |
| **T = UF** | The monad derived from the adjunction; Moggi's T. |
| **Parameterized monad** | A monad indexed by pre/post states; Atkey's generalization. |
| **Capability environment** | A row describing which capabilities are available and in what state. |
