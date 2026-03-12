# Bidirectional Type Checking with Rows, Indexed Types, and Higher-Rank Polymorphism

Research document for the Gomputation embedded typed effect language.

## Table of Contents

1. Bidirectional Type Checking Foundations
2. Dunfield-Krishnaswami: Complete and Easy Bidirectional Typing
3. Bidirectional Typing and Row Polymorphism
4. Bidirectional Typing and Indexed Computation Types
5. Practical Systems: GHC, PureScript, Koka, Idris
6. Inference Boundaries and Annotation Tax
7. Algorithm Design for Gomputation
8. Concrete Recommendations
9. Key References

---

## 1. Bidirectional Type Checking Foundations

### 1.1 Origins: Pierce and Turner's Local Type Inference

Pierce and Turner (POPL 1998, TOPLAS 2000) introduced bidirectional typing as "local type inference" for a language combining subtyping and impredicative polymorphism. The core insight: split the traditional single typing judgment into two mutually recursive judgments that differ in the direction of information flow.

The two judgment forms are:

```
Gamma |- e => A    (synthesis / inference)
Gamma |- e <= A    (checking)
```

In **synthesis mode**, the term is input and the type is output. The algorithm examines the term and computes its type. In **checking mode**, both the term and the type are inputs. The algorithm verifies that the term inhabits the given type.

The fundamental design principle, due to Pierce and Turner and later refined by Dunfield and Krishnaswami, is:

- **Introduction forms are checked.** Lambdas, constructors, and literals are naturally checked against an expected type.
- **Elimination forms synthesize.** Variables, applications, and projections naturally produce a type.

This principle follows directly from the structure of natural deduction: introduction rules build values of a known type; elimination rules decompose values and reveal their type.

### 1.2 The Core Rules (Simply Typed)

The basic bidirectional rules for the simply typed lambda calculus:

**Var (synthesis):**
```
x : A in Gamma
-----------------
Gamma |- x => A
```

**Anno (synthesis):**
```
Gamma |- e <= A
---------------------
Gamma |- (e : A) => A
```

**App (synthesis):**
```
Gamma |- e1 => A -> B
Gamma |- e2 <= A
---------------------
Gamma |- e1 e2 => B
```

**Lam (checking):**
```
Gamma, x : A |- e <= B
---------------------------
Gamma |- \x -> e <= A -> B
```

**Sub (checking):**
```
Gamma |- e => A
A <: B
-----------------
Gamma |- e <= B
```

The **Sub** rule (subsumption) bridges synthesis and checking: when checking a synthesizing form, synthesize its type and check subtyping. In a system without subtyping, this becomes an equality check.

### 1.3 Why Bidirectional Typing Scales

The key advantage of bidirectional typing over full inference (Damas-Milner style) is that it remains decidable even when the type system grows beyond what HM can handle. Adding higher-rank polymorphism, GADTs, indexed types, or effect annotations to HM breaks principal types or decidability. Bidirectional typing sidesteps this by requiring annotations at specific positions and propagating type information through checking mode.

The key advantage over fully explicit (Church-style) typing is that the annotation burden is small. Annotations are required only at the boundary between checking and synthesis -- typically at top-level definitions and higher-rank lambda parameters.

### 1.4 Mode-Correctness

A bidirectional rule is **mode-correct** if every "input" metavariable in each premise is determined by:
- the inputs to the conclusion (for checking rules: the context and the type), or
- the outputs of earlier premises.

Mode-correctness ensures the rules can be read as an algorithm: the premises are processed left-to-right, and each premise has enough information to proceed.

### 1.5 The Application Judgment

Many bidirectional systems introduce a third judgment form for applications:

```
Gamma |- A . e =>> C     (application)
```

Read: "A function of type A, applied to argument e, produces result type C." This judgment handles multi-argument applications and instantiation of polymorphic functions at application sites without requiring the programmer to write type applications.

---

## 2. Dunfield-Krishnaswami: Complete and Easy Bidirectional Typing

### 2.1 The Problem Solved

Dunfield and Krishnaswami (DK, ICFP 2013) give a bidirectional type system for higher-rank polymorphism that is:

- **Complete**: every well-typed term in the declarative system is accepted by the algorithm.
- **Easy**: the algorithm is simple enough to implement directly.
- **Decidable**: the algorithm always terminates.

The system handles `forall` types at arbitrary positions in types, not just at the outermost level.

### 2.2 The Declarative System

Types:
```
A, B ::= 1 | alpha | A -> B | forall alpha. A
```

The declarative system has three judgments:

1. **Subtyping**: `Gamma |- A <: B` (A is a subtype of B under context Gamma)
2. **Checking**: `Gamma |- e <= A` (e checks against A)
3. **Synthesis**: `Gamma |- e => A` (e synthesizes type A)

Key declarative rules for polymorphism:

**ForallI (checking):**
```
Gamma, alpha |- e <= A
----------------------------
Gamma |- e <= forall alpha. A
```

When checking a term against a universally quantified type, introduce a fresh type variable and check the body. This is forall-introduction in checking mode.

**ForallE (subtyping):**
```
Gamma |- [tau/alpha]A <: B
---------------------------  (for some monotype tau)
Gamma |- forall alpha. A <: B
```

A universally quantified type is a subtype of B if there exists some instantiation that makes it so.

**Sub (subsumption):**
```
Gamma |- e => A
Gamma |- A <: B
-----------------
Gamma |- e <= B
```

### 2.3 The Algorithmic System

The algorithmic system introduces **existential variables** (written with a hat, here rendered as `a^`) and **ordered contexts**. These two mechanisms are the key innovation.

**Algorithmic contexts:**
```
Gamma ::= .
         | Gamma, x : A       (term variable)
         | Gamma, alpha        (universal type variable)
         | Gamma, a^           (existential type variable, unsolved)
         | Gamma, a^ = tau     (existential type variable, solved)
         | Gamma, |>           (scope marker)
```

The context is an **ordered list**. The order encodes scoping: an existential variable can only be solved to a type that mentions only variables to its left in the context. This ensures that solutions respect scoping.

**Algorithmic judgments** all thread the context:

```
Gamma |- A <: B -| Delta       (subtyping, input context Gamma, output context Delta)
Gamma |- e <= A -| Delta       (checking)
Gamma |- e => A -| Delta       (synthesis)
Gamma |- a^ <=: A -| Delta     (instantiation, left)
Gamma |- A :=> a^ -| Delta     (instantiation, right)
```

The output context Delta records knowledge gained during type checking (solutions to existential variables).

### 2.4 Key Algorithmic Rules

**Subtyping rules for forall:**

```
ForallL:
  Gamma, |>, a^ |- [a^/alpha]A <: B -| Delta, |>, Theta
  -------------------------------------------------------
  Gamma |- forall alpha. A <: B -| Delta
```

To show `forall alpha. A <: B`, introduce a fresh existential `a^` (with a scope marker), substitute it for alpha, and check the resulting subtype relation. The scope marker ensures that `a^` is solved only to types visible at this point.

```
ForallR:
  Gamma, alpha |- A <: B -| Delta, alpha, Theta
  -----------------------------------------------
  Gamma |- A <: forall alpha. B -| Delta
```

To show `A <: forall alpha. B`, introduce a fresh universal variable alpha and check the subtype relation. This is the standard right rule for universal quantification.

**Instantiation rules:**

The instantiation judgment `a^ <=: A` solves `a^` to make it equal to (or a subtype of) A.

```
InstLSolve:
  Gamma[a^], a^ = tau |- ...
  (when tau is a monotype with all free variables to the left of a^ in Gamma)
```

When instantiating an existential against a monotype, simply solve it.

```
InstLArr:
  Gamma[a^2, a^1, a^ = a^1 -> a^2] |- A1 :=> a^1 -| Theta
  Theta |- a^2 <=: [Theta]A2 -| Delta
  --------------------------------------------------
  Gamma[a^] |- a^ <=: A1 -> A2 -| Delta
```

When instantiating an existential against a function type, split it into two fresh existentials for domain and codomain, solve the original to their arrow, then recursively instantiate. Note the **contravariance**: for the domain, the direction flips.

```
InstLAllR:
  Gamma[a^], alpha |- a^ <=: A -| Delta, alpha, Theta
  ---------------------------------------------------
  Gamma[a^] |- a^ <=: forall alpha. A -| Delta
```

When instantiating against a polymorphic type on the right, introduce the universal variable and continue.

### 2.5 Checking and Synthesis for Polymorphism

**ForallI (algorithmic checking):**
```
Gamma, alpha |- e <= A -| Delta, alpha, Theta
---------------------------------------------
Gamma |- e <= forall alpha. A -| Delta
```

When checking against `forall alpha. A`, introduce alpha as a universal variable, check the body, then discard alpha and everything after it from the output context.

**ForallApp (application):**
```
Gamma, a^ |- [a^/alpha]A . e =>> C -| Delta
----------------------------------------------
Gamma |- forall alpha. A . e =>> C -| Delta
```

When applying a function of universally quantified type, introduce a fresh existential for the quantified variable, substitute, and continue with the application judgment. The existential will be solved by matching against the argument.

### 2.6 Properties

The DK system satisfies:

- **Soundness**: If the algorithm accepts, the declarative system accepts.
- **Completeness**: If the declarative system accepts with a particular context extension, the algorithm accepts and produces at least as informative a context.
- **Decidability**: The algorithm terminates on all inputs.

### 2.7 Implementation Strategy

An implementation of DK proceeds as follows:

1. Represent the context as an ordered list of entries (variables, existentials, markers).
2. Implement `check`, `synth`, `subtype`, `instL`, `instR` as mutually recursive functions that take a context and return a (possibly modified) context.
3. When introducing an existential, append it to the context.
4. When solving an existential, replace its entry in the context with a solved entry.
5. When scoping out (ForallR, ForallL), drop everything after the scope marker.
6. Apply the context as a substitution when comparing types -- solved existentials are replaced by their solutions.

The ordered context avoids the need for a separate unification engine. Scoping is managed by position in the list. This is simpler than maintaining a separate constraint store, at the cost of requiring careful context manipulation.

---

## 3. Bidirectional Typing and Row Polymorphism

### 3.1 The Core Question

When row-polymorphic types appear in a bidirectional system, the question is: how does checking mode propagate row type information, and when can row variables be inferred?

Consider Gomputation's central use case:

```
dbOpen :: forall r. Computation { db : DB[Closed] | r }
                                { db : DB[Opened] | r }
                                Unit
```

When `dbOpen` appears in a `bind` chain, the row variable `r` must be determined. The question is whether this happens by inference (from the surrounding context) or requires annotation.

### 3.2 Row Variables as Existential Variables

In a DK-style system, row variables can be treated exactly like type variables in the ordered context. When `dbOpen` is used:

1. Its `forall r` is instantiated with a fresh existential `r^`.
2. The type becomes `Computation { db : DB[Closed] | r^ } { db : DB[Opened] | r^ } Unit`.
3. The `r^` existential is solved when the pre-row or post-row is unified with a known row from the surrounding context.

This works naturally when checking mode provides the expected `Computation pre post a` type: the `pre` row from the expected type unifies with `{ db : DB[Closed] | r^ }`, solving `r^`.

### 3.3 Row Unification in the Ordered Context

Row unification fits into the DK framework with the following extension:

**Row-kinded existentials** are existential variables of kind `Row` rather than kind `Type`. They participate in the same ordered context and obey the same scoping rules.

**Row unification algorithm** (adapted from Leijen's scoped labels and the standard extensible records literature):

To unify two rows `{ l1 : A1, ..., ln : An | tail1 }` and `{ m1 : B1, ..., mk : Bk | tail2 }`:

1. **Normalize** both rows to canonical form (sorted by label).
2. **Decompose** on shared labels: for each label `l` present in both rows, unify `Ai` with `Bj`.
3. **Compute remainders**: labels present in one row but not the other form the "remainder" rows.
4. **Unify tails with remainders**:
   - If both tails are closed (empty): remainders must also be empty, or fail.
   - If one tail is a variable `r^` and the remainder from the other side is `R_extra`: solve `r^ = R_extra`.
   - If both tails are variables: introduce a fresh row variable for the shared remainder and solve both.

**Interaction with DK scoping**: The key requirement is that when solving a row existential `r^`, the solution must mention only labels and types that are in scope (to the left of `r^` in the ordered context). Since row labels are typically constants (not bound variables), this constraint is usually satisfied. The types within the row entries must respect scoping.

### 3.4 Row Inference in Practice

Row variables can typically be inferred in these situations:

1. **Application against a known context**: When the overall computation type is known (from an annotation or an enclosing `bind`), checking mode provides the expected rows, and row unification solves the row variable.

2. **Sequential composition**: In `bind c1 (\x -> c2)`, the intermediate row `r2` must match between `c1`'s post-row and `c2`'s pre-row. If either is known, unification determines the other.

3. **Closed-row primitives**: When a primitive has a closed pre-row (no row variable), no row inference is needed.

Row variables **must be annotated** in these situations:

1. **Top-level definitions without annotations**: If neither the pre-row nor the post-row is determined by context, the row variable remains unsolved.

2. **Higher-rank row polymorphism**: If a function takes a row-polymorphic argument (a function whose type mentions `forall r`), the `forall r` requires annotation, just as `forall a` does for types.

3. **Ambiguous decomposition**: When two open rows are unified and neither constrains the other sufficiently.

### 3.5 Example: Row Inference Through a Bind Chain

Consider:

```
program :: Computation { db : DB[Closed], log : Logger[Ready] }
                       { db : DB[Closed], log : Logger[Ready] }
                       Rows

program = do
  dbOpen          -- requires { db : DB[Closed] | r }, produces { db : DB[Opened] | r }
  rows <- dbQuery q
  dbClose
  pure rows
```

With a top-level annotation providing the overall pre and post rows, checking proceeds:

1. The overall type is known: `Computation { db : DB[Closed], log : Logger[Ready] } { db : DB[Closed], log : Logger[Ready] } Rows`.
2. Desugaring `do` produces nested `bind` calls.
3. For `dbOpen` (first in the chain), the pre-row is `{ db : DB[Closed], log : Logger[Ready] }`. The primitive's pre-row is `{ db : DB[Closed] | r^ }`. Unification solves `r^ = { log : Logger[Ready] }`.
4. The post-row of `dbOpen` becomes `{ db : DB[Opened], log : Logger[Ready] }`, which is the pre-row for `dbQuery`.
5. This continues through the chain. Each intermediate row is determined by the previous step's post-row.

The row variable `r` in `dbOpen` is fully inferred -- no annotation beyond the top-level type signature is needed.

---

## 4. Bidirectional Typing and Indexed Computation Types

### 4.1 The Central Challenge

`Computation pre post a` is an indexed type: its first two arguments are row indices, not ordinary type parameters. The challenge is that sequencing via `bind` imposes an **equational constraint** on the intermediate index:

```
bind : Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

The `r2` in the first argument's post-position must equal the `r2` in the second argument's pre-position. This is a unification problem on row indices.

### 4.2 Checking Mode for Computation Types

When checking a computation expression against a known `Computation pre post a`:

**Pure (checking):**
```
Gamma |- e <= A -| Delta
-------------------------------------------------
Gamma |- pure e <= Computation R R A -| Delta
```

When `pure e` is checked against `Computation pre post a`, we require `pre = post` (row unification) and check `e <= a`.

**Bind (checking):**
```
Gamma, r2^ |- c1 <= Computation R1 r2^ A -| Theta1
Theta1 |- (\x -> c2) <= A -> Computation [Theta1]r2^ R3 B -| Delta
-------------------------------------------------------------------
Gamma |- bind c1 (\x -> c2) <= Computation R1 R3 B -| Delta
```

When `bind c1 (\x -> c2)` is checked against `Computation R1 R3 B`:

1. Introduce a fresh existential `r2^` for the intermediate row.
2. Check `c1` against `Computation R1 r2^ A` (where `A` may itself involve a fresh existential for the result type).
3. Apply the knowledge gained (solving `r2^`) to the continuation.
4. Check the continuation against the appropriate type using the solved intermediate row.

The intermediate row `r2^` is determined by **inference within checking mode**: it is never annotated by the programmer but is solved by unification during checking of `c1`.

### 4.3 Synthesis Mode for Computation Types

When synthesizing the type of a computation expression:

**Bind (synthesis):**
```
Gamma |- c1 => Computation R1 R2 A -| Theta1
Theta1 |- (\x -> c2) <= A -> Computation R2 R3 B -| Delta
----------------------------------------------------------
Gamma |- bind c1 (\x -> c2) => Computation R1 R3 B -| Delta
```

Synthesize the type of `c1`, extract `R2` and `A`, then check the continuation. Here `R2` comes from synthesis, not from an expected type.

### 4.4 Do-Notation Desugaring

Do-notation desugars to nested `bind` calls before type checking. The critical question is: does desugaring lose type information?

Standard Haskell-style desugaring:
```
do { x <- c1; c2 }    ==>    bind c1 (\x -> c2)
do { c1; c2 }         ==>    bind c1 (\_ -> c2)
```

This desugaring preserves all the type information needed for bidirectional checking because:

1. The outer type `Computation R1 R3 B` provides `R1` and `R3` in checking mode.
2. The `bind` function's type provides the structural skeleton for propagating the intermediate row.
3. Each primitive in the chain contributes row constraints that determine intermediate rows.

**Desugaring should happen before type checking** (during parsing or renaming), because:
- The type checker sees only `bind` and lambdas, which it already knows how to handle.
- Error reporting can map locations back to the surface syntax.
- No special typing rules for `do` syntax are needed in the core.

### 4.5 Propagation Through Chains

In a chain of n computations:

```
do
  x1 <- c1    -- Computation R0 R1 A1
  x2 <- c2    -- Computation R1 R2 A2
  ...
  xn <- cn    -- Computation R(n-1) Rn An
  pure result
```

If the overall type `Computation R0 Rn T` is known:

- `R0` is the pre-row (known from annotation).
- `c1`'s type determines `R1` (from its post-row, constrained by its primitive type and `R0`).
- `c2`'s type determines `R2` (from its post-row, constrained by `R1`).
- This cascades forward.
- `Rn` is the post-row (known from annotation, providing a check against the cascade).

This is a **forward-chaining inference** within checking mode. Each step determines the next intermediate row. The top-level annotation provides the boundary conditions; the intermediate rows are fully inferred.

If the overall type is not known (synthesis mode for the whole block), then:
- `c1` must synthesize `R0` and `R1`.
- Each subsequent step synthesizes the next row.
- The final `Rn` and result type are synthesized.
- This works as long as each primitive's type is fully known (which it is, since primitives are declared by the host).

### 4.6 When Annotation Is Required

For indexed computations, annotation is required when:

1. **The overall computation type is ambiguous**: A `let`-bound computation without a type signature, where neither the pre-row nor post-row is determined by usage context.

2. **Row-polymorphic abstraction**: When writing a function that is itself polymorphic over rows, the `forall r` must be annotated.

3. **Higher-rank computation arguments**: When passing a computation as an argument to a function that expects a polymorphic computation type.

Annotation is **not** required for:
- Individual steps within a `do` block, when the overall type is annotated.
- Intermediate rows between `bind` steps.
- Row variables in primitives, when context determines them.

---

## 5. Practical Systems

### 5.1 GHC (Haskell)

**Architecture**: GHC uses bidirectional typing for its core type inference, built on the OutsideIn(X) framework (Vytiniotis, Peyton Jones, Schrijvers, Sulzmann, JFP 2011). The system is parameterized over a constraint domain X, which currently includes type classes, GADTs, and type families.

**Higher-rank types**: GHC requires annotations for lambda-bound variables with polymorphic types. The programmer provides either a direct pattern type signature or a type signature on the enclosing function. GHC does not infer polymorphic types for lambda-bound variables.

**Subsumption**: GHC applies instantiation and re-generalization only at the outermost level of function types. Deeply nested foralls require manual eta-expansion or the `DeepSubsumption` extension.

**Impredicativity**: GHC's "Quick Look" (Serrano, Hage, Peyton Jones, Vytiniotis, ICFP 2020) adds limited impredicative inference at application sites. Quick Look performs a fast pre-pass over arguments to find "obvious" type instantiations, affecting only 1% of the inference engine.

**Implicit quantification**: GHC adds implicit `forall` only at the outermost level of user-written types. `(a -> a) -> Int` means `forall a. (a -> a) -> Int`, not `(forall a. a -> a) -> Int`.

**Constraint generation**: OutsideIn separates type checking into constraint generation and constraint solving. Constraints include equality constraints, type class constraints, and implication constraints (from local assumptions in GADT branches). The solver operates on "wanted" constraints, discharging them against "given" constraints from the context.

**Relevance to Gomputation**: GHC demonstrates that bidirectional typing with higher-rank polymorphism is practical at scale. The annotation burden is modest: top-level type signatures plus annotations on polymorphic lambda parameters. The OutsideIn architecture of separating constraint generation from solving is a proven strategy for managing complexity.

### 5.2 PureScript

**Architecture**: PureScript combines bidirectional type checking with HM-style inference, supporting type classes, row polymorphism, and higher-kinded polymorphism. The type checker draws on DK for higher-rank types and on Leijen's scoped labels for row types, though the implementation diverges from the papers in important ways.

**Row polymorphism**: PureScript models rows as unordered collections of labeled types with optional duplicate labels. Row polymorphism is the primary mechanism for record types and effect tracking. Rows are first-class in the kind system: the kind `Row Type` classifies rows of types.

**Inference for rows**: PureScript infers row types through standard unification. When a function pattern-matches on `{ x: _, y: _ | ... }`, the inferred type includes those fields plus an open row variable. Row variables are generalized at `let` bindings just like type variables.

**Implementation approach**: PureScript uses **skolemization** rather than ordered contexts for handling universal quantification. When checking against `forall a. A`, the type variable `a` is replaced by a rigid skolem constant. If the skolem escapes its scope, a type error is reported. This is operationally different from DK's ordered context approach but achieves the same effect.

**AST rewriting**: PureScript heavily uses AST-level rewriting transformations, preferring stateless transformations over context-threaded state. This is a pragmatic engineering choice that simplifies implementation at the cost of some theoretical elegance.

**Relevance to Gomputation**: PureScript is the closest existing system to Gomputation in terms of feature combination: row polymorphism + higher-rank types + bidirectional checking. Key lessons:
- Row inference works well for record operations and can be expected to work similarly for capability rows.
- Skolemization is a simpler alternative to ordered contexts for implementation.
- Combining row polymorphism with rank-n types took "several years to iron out corner cases" (paf31).

### 5.3 Koka

**Architecture**: Koka uses HM-style inference extended with row-polymorphic effect types. Effect rows are inferred automatically in most cases. The system does not use bidirectional typing in the DK sense; it relies on Hindley-Milner with row extensions.

**Effect rows**: Effect types are rows of effect labels. A row is either empty, a variable, or an extension `<l|e>`. Duplicate labels are permitted (following Leijen's scoped labels design), which simplifies unification by eliminating the need for "lacks" constraints.

**Row unification**: Koka adapts Leijen's scoped labels unification algorithm. The key rules:

1. Standard Robinson unification for non-row types.
2. For row types, when unifying `<l :: tau | r>` with a row `s`, rewrite `s` to the form `<l :: tau' | s'>` and recursively unify `tau` with `tau'` and `r` with `s'`.
3. If the label is not found in `s` and `s` has a tail variable, the tail variable is solved to `<l :: tau | fresh>` and unification continues.

This algorithm is deterministic and terminating. It does not require additional constraint forms (lacks, absence), which keeps the inference engine simple.

**Effect inference**: Function application composes effects: if `f : A -<e1>-> B` and `x : A` with effect `e2`, then `f x` has effect `<e1 | e2>`. Effect polymorphism is handled by generalization at `let` bindings.

**No higher-rank effects**: Koka does not support higher-rank polymorphism over effect rows. Effect variables are always rank-1. This keeps inference fully automatic at the cost of expressiveness.

**Relevance to Gomputation**: Koka demonstrates that row-polymorphic effect inference can be fully automatic when restricted to rank-1. The scoped labels unification algorithm is directly applicable to Gomputation's capability rows, with the modification that Gomputation rejects duplicate labels (which is actually simpler than Koka's design). The limitation is that Koka's approach does not extend to higher-rank row polymorphism without adding annotations.

### 5.4 Idris

**Architecture**: Idris 2 uses bidirectional elaboration from a high-level intermediate language (TTImp) to a core dependent type theory (TT). Elaboration relies heavily on unification and metavariable solving, similar to Agda's approach (Norell's thesis).

**Indexed types**: Idris handles indexed types natively because its core is a full dependent type theory. Indexed families like `Vect : Nat -> Type -> Type` are first-class. Pattern matching on indexed constructors refines type indices through unification.

**Metavariable solving**: When elaboration encounters an implicit argument or a `_` placeholder, it creates a metavariable (a fresh name applied to the current environment). Unification attempts to solve metavariables. Unsolved metavariables are retried when blocking metavariables are solved, in a worklist-style loop.

**Bidirectional elaboration**: Idris uses checking mode when an expected type is available and synthesis mode otherwise. The `Implicit` constructor in TTImp has a boolean flag: `True` means "bind as a new variable if unsolved" (for `_` in patterns), `False` means "leave as a hole" (for `?` in terms).

**Pattern matching and indexed types**: Patterns are checked left-to-right, and each pattern refines the type context for subsequent patterns. For indexed constructors, this introduces equalities between indices that are solved by unification. Coverage checking ensures all cases are handled.

**Relevance to Gomputation**: Idris demonstrates that bidirectional typing with indexed types is practical. The key insight is that indexed types introduce equational constraints (between indices), and these constraints are solved by the same unification engine that handles ordinary type inference. For Gomputation, the `Computation pre post a` type introduces row equalities at each `bind`, which can be handled by the same unification-based approach.

---

## 6. Inference Boundaries and Annotation Tax

### 6.1 What Must Be Annotated

Across all practical systems, the following consistently require annotations:

| Feature | Why annotation is needed |
|---------|------------------------|
| Top-level definitions | To establish the principal type and serve as documentation |
| Higher-rank lambda parameters | The type of a lambda parameter with a `forall` cannot be inferred |
| Polymorphic arguments to higher-order functions | When a function expects `forall a. a -> a`, the argument must be annotated |
| Ambiguous type class instances | When multiple instances could apply |
| GADT constructor scrutinees (sometimes) | To determine index refinements |

### 6.2 What Can Be Inferred

| Feature | How it is inferred |
|---------|-------------------|
| Types of local variables | HM-style unification or checking mode propagation |
| Type arguments at application sites | Instantiation of `forall` with fresh existentials, solved by unification |
| Row variables at application sites | Row unification against known context |
| Intermediate rows in bind chains | Forward-chaining from known pre-row |
| Result types in bind chains | Backward-chaining from known overall result type |
| Effect rows (in Koka-style systems) | Fully automatic via row unification |

### 6.3 The Annotation Tax for Different Designs

**Design A: HM + Rank-1 Rows (Koka-style)**

Annotation tax: minimal. Top-level signatures are optional. Row variables are inferred everywhere.

Limitation: No higher-rank polymorphism. Cannot write `forall r. Computation { db : DB[Closed] | r } ...` as a first-class type passed to a function.

**Design B: Bidirectional + Higher-Rank Types (GHC-style)**

Annotation tax: moderate. Top-level signatures are strongly recommended. Polymorphic lambda parameters must be annotated. Type applications are sometimes needed.

Limitation: No native row types. Effect tracking requires type class encoding or extension.

**Design C: Bidirectional + Higher-Rank + Rows (PureScript-style)**

Annotation tax: moderate. Top-level signatures required for higher-rank definitions. Row variables in rank-1 positions are inferred. Higher-rank row polymorphism requires annotation.

Limitation: Complexity of interaction between features. Corner cases took years to resolve in PureScript.

**Design D: Full Dependent Types (Idris-style)**

Annotation tax: high for novel code, low for pattern-following code. Metavariable solving handles many cases, but type-level computation can require explicit proofs.

Limitation: Far beyond Gomputation's scope.

### 6.4 Annotation Requirements for Gomputation-Style Programs

For a typical Gomputation program with capability transitions:

```
-- Requires annotation: top-level type signature
program :: Computation { db : DB[Closed], log : Logger[Ready] }
                       { db : DB[Closed], log : Logger[Ready] }
                       Rows

-- No annotation needed inside the do block
program = do
  dbOpen                    -- r inferred as { log : Logger[Ready] }
  rows <- dbQuery someQuery -- no row change, inferred
  dbClose                   -- r inferred again
  pure rows
```

The annotation burden is **one type signature per top-level computation definition**. Inside the body, all row variables and intermediate types are inferred. This is a good trade-off: the signature documents the capability protocol, and the body is free of type noise.

For utility functions with row polymorphism:

```
-- Requires annotation: forall r in the signature
withDB :: forall r. (forall s. Computation { db : DB[s] | r } { db : DB[s] | r } a)
       -> Computation { db : DB[Closed] | r } { db : DB[Closed] | r } a
```

This is a higher-rank type (the argument is polymorphic in `s`), so it requires full annotation. This is unavoidable and consistent with all practical systems.

---

## 7. Algorithm Design for Gomputation

### 7.1 Recommended Architecture

Gomputation should use a **two-phase** approach:

**Phase 1: Constraint Generation (Bidirectional)**

Walk the term structure bidirectionally, generating constraints. The constraints are:

- Type equality constraints: `A ~ B`
- Row equality constraints: `R1 ~ R2`
- Row unification constraints (decomposition): `{ l : A | R1 } ~ { l : B | R2 }` yields `A ~ B` and `R1 ~ R2`
- Instantiation constraints: `forall a. A <=: B` yields a fresh existential and appropriate sub-constraints

The bidirectional walk determines which constraints to generate:

- In checking mode, the expected type provides inputs that constrain the term.
- In synthesis mode, fresh existentials are introduced and constrained by the term's structure.

**Phase 2: Constraint Solving (Unification)**

Solve the collected constraints using a unification engine:

- Standard first-order unification for type equalities.
- Row unification (adapted from Leijen) for row equalities.
- Occurs check for both type and row variables.

The two phases can be **interleaved** (as in DK's ordered context) or **separated** (as in OutsideIn). For Gomputation's scale, interleaving is simpler and sufficient:

- Use DK-style ordered contexts for type and row variables.
- Solve eagerly when possible during constraint generation.
- Defer only when necessary (when a metavariable is not yet constrained).

### 7.2 Judgments

The type checker implements these judgments:

```
Gamma |- e => A -| Delta           (synthesis)
Gamma |- e <= A -| Delta           (checking)
Gamma |- A <: B -| Delta           (subtyping / type compatibility)
Gamma |- a^ <=: A -| Delta         (instantiation, left)
Gamma |- A :=> a^ -| Delta         (instantiation, right)
Gamma |- A . e =>> C -| Delta      (application)
```

Plus row-specific judgments:

```
Gamma |- R1 ~ R2 -| Delta          (row unification)
Gamma |- r^ ~ { l : A | R } -| Delta   (row instantiation)
```

### 7.3 Context Structure

```
ContextEntry ::=
  | TermVar    Name Type
  | TypeVar    Name Kind          -- universal type variable
  | RowVar     Name               -- universal row variable
  | ExType     Name (Maybe Type)  -- existential type variable (possibly solved)
  | ExRow      Name (Maybe Row)   -- existential row variable (possibly solved)
  | Marker     Name               -- scope marker

Context = [ContextEntry]          -- ordered list, left = older
```

Row existentials are separate from type existentials because they inhabit a different kind. This prevents kind errors during instantiation.

### 7.4 Elaboration

Type checking should produce an **elaborated term** where:

1. All type applications are explicit: `e @T` replaces implicit instantiation.
2. All row applications are explicit: `e @R` replaces implicit row instantiation.
3. All `forall` introductions are explicit: `\@a -> e` replaces implicit generalization.
4. `bind` has explicit intermediate type annotations: `bind @R1 @R2 @R3 @A @B c1 k`.

This elaborated form is unambiguous and can be type-checked without inference (a useful sanity check for the implementation). It also serves as the input to the interpreter or code generator.

### 7.5 Handling forall in Both Modes

**Checking against forall alpha. A:**

```
check(Gamma, e, forall alpha. A) =
  let Gamma' = Gamma, alpha
  let Delta = check(Gamma', e, A)
  return dropAfter(alpha, Delta)
```

Introduce a universal variable, check the body, discard the variable. The elaborated term gains a type lambda: `\@alpha -> [[e]]`.

**Checking against forall r. A (row quantification):**

```
check(Gamma, e, forall r. A) =
  let Gamma' = Gamma, r : Row
  let Delta = check(Gamma', e, A)
  return dropAfter(r, Delta)
```

Identical to type quantification, but the variable has kind `Row`. The elaborated term gains a row lambda: `\@r -> [[e]]`.

**Synthesizing application of forall alpha. A:**

```
appSynth(Gamma, forall alpha. A, e) =
  let a^ = fresh existential
  let Gamma' = Gamma, a^
  appSynth(Gamma', [a^/alpha]A, e)
```

Introduce an existential, substitute, continue. The elaborated term gains a type application: `[[f]] @(solution of a^) [[e]]`.

### 7.6 The Bind Rule in Detail

The most important rule for Gomputation is the typing of `bind` in checking mode:

```
check(Gamma, bind c1 (\x -> c2), Computation R1 R3 B) =
  -- Introduce existentials for the intermediate row and type
  let r2^ = fresh row existential
  let a^  = fresh type existential
  let Gamma1 = Gamma, r2^, a^

  -- Check c1 against Computation R1 r2^ a^
  let Delta1 = check(Gamma1, c1, Computation R1 r2^ a^)

  -- Apply solutions from Delta1
  let R2 = apply(Delta1, r2^)
  let A  = apply(Delta1, a^)

  -- Check the continuation
  let Delta2 = check(Delta1, \x -> c2, A -> Computation R2 R3 B)

  return Delta2
```

This rule:
- Uses checking mode to push `R1` into `c1`'s pre-row (solving any row variables in `c1`'s type).
- Discovers `R2` (the intermediate row) by checking `c1`.
- Pushes `R2` into `c2`'s pre-row.
- Pushes `R3` and `B` from the overall expected type into `c2`.

The programmer annotates only the outer type; all intermediate rows flow automatically.

### 7.7 Row Variable Scoping and Type Variable Scoping

Row variables and type variables participate in the same ordered context, so their scoping is unified. A `forall` can quantify over both:

```
forall a r. a -> Computation { db : DB[a] | r } { db : DB[a] | r } a
```

The existentials introduced for `a` and `r` when this type is instantiated are adjacent in the context and can refer to each other (an existential can be solved to a type/row mentioning earlier existentials but not later ones).

No special interaction between type variable scoping and row variable scoping is needed beyond ensuring they share the same ordered context.

---

## 8. Concrete Recommendations

### 8.1 Adopt Bidirectional Typing from the Start

The spec should commit to bidirectional typing as the type checking strategy. The alternative (pure HM) does not scale to higher-rank polymorphism, and the alternative (fully explicit types) imposes too heavy an annotation burden.

### 8.2 Use DK-Style Ordered Contexts

The DK algorithm (possibly with the skolemization variant used by PureScript) provides the right foundation. The ordered context elegantly handles both type and row existentials with correct scoping.

An alternative is OutsideIn-style constraint separation. This is more modular and better for error reporting at scale, but more complex to implement. For Gomputation's current scope, DK is sufficient.

### 8.3 Treat Row Variables as Existentials of Kind Row

Row variables participate in the same context as type variables. Row unification is an extension of the instantiation judgment, not a separate mechanism.

### 8.4 Use Leijen-Style Row Unification (Without Duplicate Labels)

Adapt Leijen's scoped labels unification algorithm, simplified by the prohibition on duplicate labels. The algorithm:

1. Normalize row order.
2. Decompose on shared labels.
3. Solve tail variables against remainder rows.
4. Check for label uniqueness.

Without duplicate labels, the algorithm is strictly simpler than Leijen's original (no need for label scoping). Without lacks constraints, no additional constraint forms are needed.

### 8.5 Require Top-Level Annotations for All Computation Definitions

This is both good practice and algorithmically necessary:

- The annotation provides the boundary conditions for row inference through `bind` chains.
- It documents the capability protocol.
- It eliminates ambiguity in the presence of row polymorphism.

Individual computations inside a `do` block need no annotations.

### 8.6 Require Annotations for Higher-Rank Quantification

When a function parameter must be polymorphic (rank > 1), require an annotation on the enclosing definition. This is consistent with GHC, PureScript, and the DK system.

### 8.7 Desugar Do-Notation Before Type Checking

Desugar to nested `bind` calls during parsing or renaming, before the bidirectional type checker runs. The desugared form has sufficient structure for the type checker to propagate row information through checking mode.

Map error locations back to the surface syntax for user-facing messages.

### 8.8 Elaborate to an Explicitly Typed Core

The output of type checking should be an elaborated term with:
- Explicit type and row applications at every instantiation point.
- Explicit type and row lambdas at every generalization point.
- Explicit intermediate type annotations on `bind`.

This core is independently checkable and serves as the interface to the interpreter.

### 8.9 Inference Boundary Summary

| Construct | Mode | Annotation needed? |
|-----------|------|-------------------|
| Top-level value definition | Checking (against signature) | Yes: type signature |
| Top-level computation definition | Checking (against signature) | Yes: type signature with rows |
| Lambda parameter (rank-1) | Checking (from expected arrow type) | No |
| Lambda parameter (higher-rank) | Checking (from annotated type) | Yes: on enclosing signature |
| Application | Synthesis | No |
| Primitive use in do-block | Checking (from bind chain) | No |
| Row variable in primitive | Instantiation (solved by unification) | No |
| Intermediate row in bind chain | Existential (solved by forward-chaining) | No |
| forall r in definition type | Explicit quantification | Yes: in signature |
| let-binding (local) | Synthesis + generalization | Optional but recommended |
| Type annotation expression `(e : T)` | Mode switch: check e against T, synthesize T | Explicit by definition |

### 8.10 Example: Minimal Annotations for a Full Program

```
-- Host-registered primitives (annotations provided by host):
primitive dbOpen  :: forall r. Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
primitive dbClose :: forall r. Computation { db : DB[Opened] | r } { db : DB[Closed] | r } Unit
primitive dbQuery :: forall r. Query -> Computation { db : DB[Opened] | r } { db : DB[Opened] | r } Rows
primitive logMsg  :: forall r. String -> Computation { log : Logger[Ready] | r } { log : Logger[Ready] | r } Unit

-- User code: one annotation per top-level definition
runReport :: Query -> Computation { db : DB[Closed], log : Logger[Ready] }
                                  { db : DB[Closed], log : Logger[Ready] }
                                  Rows
runReport q = do
  logMsg "Opening database"     -- r inferred: { db : DB[Closed] }... wait, log is consumed
  dbOpen                        -- r inferred: { log : Logger[Ready] }
  logMsg "Running query"        -- r inferred: { db : DB[Opened] }
  rows <- dbQuery q             -- r inferred: { log : Logger[Ready] }
  logMsg "Closing database"     -- r inferred: { db : DB[Opened] }
  dbClose                       -- r inferred: { log : Logger[Ready] }
  pure rows                     -- no transition
```

In this program, the programmer writes exactly one type signature. All row variables, intermediate rows, and result types inside the `do` block are fully inferred by the bidirectional checker propagating the top-level annotation through the `bind` chain.

---

## 9. Key References

### Foundational Papers

1. Benjamin C. Pierce and David N. Turner. "Local Type Inference." ACM TOPLAS 22(1), 2000.
   https://www.cis.upenn.edu/~bcpierce/papers/lti-toplas.pdf

2. Jana Dunfield and Neelakantan R. Krishnaswami. "Complete and Easy Bidirectional Typechecking for Higher-Rank Polymorphism." ICFP 2013.
   https://arxiv.org/abs/1306.6032

3. Jana Dunfield and Neelakantan R. Krishnaswami. "Sound and Complete Bidirectional Typechecking for Higher-Rank Polymorphism with Existentials and Indexed Types." POPL 2019.
   https://arxiv.org/abs/1601.05106

4. Jana Dunfield and Neelakantan R. Krishnaswami. "Bidirectional Typing." ACM Computing Surveys 54(5), 2021.
   https://arxiv.org/abs/1908.05839

### GHC and Haskell Type Inference

5. Dimitrios Vytiniotis, Simon Peyton Jones, Tom Schrijvers, Martin Sulzmann. "OutsideIn(X): Modular type inference with local assumptions." JFP 21(4-5), 2011.
   https://www.microsoft.com/en-us/research/publication/outsideinx-modular-type-inference-with-local-assumptions/

6. Simon Peyton Jones, Dimitrios Vytiniotis, Stephanie Weirich, Mark Shields. "Practical type inference for arbitrary-rank types." JFP 17(1), 2007.
   https://www.microsoft.com/en-us/research/publication/practical-type-inference-for-arbitrary-rank-types/

7. Alejandro Serrano, Jurriaan Hage, Simon Peyton Jones, Dimitrios Vytiniotis. "A Quick Look at Impredicativity." ICFP 2020.
   https://dl.acm.org/doi/10.1145/3408971

### Row Types and Effect Systems

8. Daan Leijen. "Extensible records with scoped labels." 2005.
   https://www.microsoft.com/en-us/research/publication/extensible-records-with-scoped-labels/

9. Daan Leijen. "Koka: Programming with Row Polymorphic Effect Types." 2014.
   https://arxiv.org/abs/1406.2061

### Indexed Types and Indexed Effects

10. Robert Atkey. "Parameterised Notions of Computation." JFP 19(3-4), 2009.
    https://bentnib.org/param-notions.pdf

### Idris and Dependent Types

11. Edwin Brady. "Idris 2: Quantitative Type Theory in Practice." ECOOP 2021.
    https://arxiv.org/abs/2104.00480

### Implementations

12. Olle Fredriksson. Haskell implementation of DK bidirectional type checking.
    https://github.com/ollef/Bidirectional

13. PureScript type system discussion.
    https://discourse.purescript.org/t/academic-theoretical-basis-of-the-purescript-type-system/748
