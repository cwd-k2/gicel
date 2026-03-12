# Embedded Typed Effect Language

**Specification Draft v0.2**

---

# 1. Position

This document specifies a typed embedded language designed to run inside a Go application as a library.

The language is intended as a small core for:

* safe embedded scripting
* domain logic
* rule evaluation
* configuration evaluation
* protocol / typestate controlled execution

The host defines available capabilities.

The language cannot perform effects that are not explicitly provided by the host.

This draft is written from two design commitments:

1. the language should be explained from a **minimal vocabulary**
2. future growth should follow explicit **extension directions**

This draft therefore does **not** begin from a catalog of features.

It begins from a small core that can later support richer features without changing its basic shape.

---

# 2. Design Commitments

## 2.1 Semantic Commitments

The language is:

* purely functional at the value level
* effectful at the computation level
* statically typed
* capability-based
* indexed by capability state
* deterministic

## 2.2 Methodological Commitments

The core should stay small enough that:

* judgments are explicit
* the relationship between values and computations is clear
* capabilities are explicit in types
* future extensions can be read as refinements of the same core

This draft also makes one explicit architectural choice:

* `Term` and `Type` are treated as distinct layers in the core

This is a design decision, not a metaphysical claim about programming languages.

The purpose of this separation in the current draft is:

* to keep the core specification simple
* to keep type equality manageable
* to make the value/computation distinction explicit
* to leave room for later, deliberate bridges between terms and types

Its background comes from several established traditions:

* symbolic logic and proof theory, where judgments and layers of classification are explicit
* typed lambda calculus, where terms and their classifiers are usually distinguished
* type-theoretic design practice, where stricter separation is often chosen first to preserve tractability

This choice is therefore informed by Curry-Howard-adjacent traditions, but it is **not** justified by Curry-Howard alone.

The primary justification in this draft is pragmatic:

* simpler static semantics
* more controlled type equality
* clearer staging of future extensions

Future drafts may weaken this separation in controlled ways, for example through promoted protocol states or richer indexed forms.

## 2.3 Non-Goals of This Draft

This draft does **not** attempt to specify:

* higher-kinded polymorphism
* type families
* general refinement types
* dependent types
* algebraic effect handlers
* linear usage tracking
* module systems

These are possible future directions, but not part of the active core.

---

# 3. Minimal Vocabulary

This draft uses the following core vocabulary.

## 3.1 Sorts

```
Term
Type
Kind
```

`Term` classifies expressions.

`Type` classifies terms.

`Kind` classifies type constructors and indices.

The distinction between `Term` and `Type` is an explicit choice of this draft.

It should be read as:

* the current core keeps computation-level terms and their classifiers separate
* this separation is chosen for clarity and tractability
* it does not rule out future designs that admit controlled interaction between the two layers

## 3.2 Contexts

Judgments are relative to contexts.

Contexts may contain:

* term variables
* type variables
* row variables

## 3.3 Families

Type constructors are treated uniformly as families:

```
F : K1 -> ... -> Kn -> Type
```

This draft uses one distinguished family for computations:

```
Computation : Row -> Row -> Type -> Type
```

## 3.4 Formation, Introduction, Elimination

The specification is organized around:

* formation of well-formed types and rows
* introduction of values and computations
* elimination by application and pattern-like use

This is the proof-theoretic backbone of the draft.

## 3.5 Core Judgments

The active judgments of this draft are:

```
Gamma |- t : T
Gamma |- T wf
Gamma |- R wf
Gamma |- T1 == T2
```

This draft reserves conceptual space for richer judgments later:

* constraint / entailment judgment
* usage judgment

but does not activate them yet.

---

# 4. Main Extension Direction

This draft commits to one main extension path:

```
ADT
-> indexed types
-> row-indexed computations
-> promoted protocol states
-> maybe GADTs
```

This means the core is designed to explain:

* ordinary algebraic data
* state-indexed resources
* capability rows
* indexed computation sequencing

before it attempts more ambitious directions such as type-level computation or proof-oriented typing.

---

# 5. Core Semantic Split

The language distinguishes two semantic layers:

```
Value
Computation
```

This split is also a design choice of the current draft.

It is adopted because the language is being developed as a small typed core with explicit effect tracking.

It should not be read as a claim that all future versions must preserve exactly this boundary.

## 5.1 Values

Values are pure terms.

Examples:

```
1
"x"
\x -> x
f x
```

## 5.2 Computations

Computations are typed transitions over capability environments.

Their type is:

```
Computation pre post a
```

Meaning:

```
pre   : capability state before execution
post  : capability state after execution
a     : result type
```

This draft uses `Computation` rather than `Comp` because the longer name is clearer at the specification level.

---

# 6. Kinds and Indices

## 6.1 Built-In Kinds

This draft uses:

```
Type
Row
```

`Row` is the kind of capability environments.

## 6.2 Indexed Families

The design treats indexed types uniformly.

Examples:

```
DB          : Type -> Type
Logger      : Type -> Type
Computation : Row -> Row -> Type -> Type
```

This is intentionally a small indexed core.

The draft does not yet introduce user-visible promoted kinds or term-dependent indices, but it is structured so that they can be added later.

---

# 7. Row-Kinded Capability Environments

Rows describe capability environments.

## 7.1 Grammar

```
R ::= {}
    | { l : T }
    | { l : T, ... }
    | { l : T | r }
```

## 7.2 Intended Reading

A row entry:

```
l : T
```

means that capability `l` is available in state `T`.

Examples:

```
{}
{ db : DB[Closed] }
{ db : DB[Opened], log : Logger[Ready] }
{ db : DB[Opened] | r }
```

## 7.3 Design Intent

Rows in this draft are intended to satisfy the following discipline:

* labels are unique
* order is not semantically relevant
* open rows preserve unknown surrounding capability context

Full row equality and unification rules are intentionally deferred, but future drafts should respect these intentions.

## 7.4 Exact and Open Reading

This draft adopts the following reading of rows.

### Closed Rows

A closed row is read as an exact capability environment description.

For example:

```text
{ db : DB[Closed] }
```

means exactly that capability environment, not merely a lower bound on a larger one.

### Open Rows

An open row is read as an exact environment schema with unknown remainder.

For example:

```text
{ db : DB[Closed] | r }
```

means:

* the environment contains `db : DB[Closed]`
* the remaining capability context is described by `r`
* the whole environment is still exact, but partially abstract

This is the intended source of extensibility in the current design.

### Consequence

This draft therefore does **not** interpret rows as implicit lower bounds.

If a specification needs preserved surrounding context, it should express it explicitly with an open row.

---

# 8. Core Type Language

## 8.1 Grammar

```
T ::= a
    | T -> T
    | forall a. T
    | Computation R R T
    | C
```

Where:

* `a` is a type variable
* `C` is a named type constructor or type constant

## 8.2 Included in This Draft

This draft actively includes:

* function types
* universal quantification
* indexed computation types
* named data types
* explicit row polymorphism

## 8.3 Deferred Beyond This Draft

This draft intentionally excludes:

* higher-kinded polymorphism
* type-level computation
* solver-backed predicates
* dependent typing

## 8.4 Polymorphism Strategy

This draft adopts a deliberately conservative polymorphism strategy.

### Active Polymorphism Forms

The draft actively permits:

* ordinary type-variable polymorphism
* row-variable polymorphism

This means types of the following shape are intended to be expressible:

```text
forall a. a -> a
forall r. Computation { db : DB[Closed] | r }
                     { db : DB[Opened] | r }
                     Unit
```

### Intended Role of Row Polymorphism

Row polymorphism is included because open rows are part of the active design of this draft.

Without explicit row polymorphism, the intended extensibility of open rows would remain only informal.

### Inference Boundary

This draft does **not** commit to full inference for all polymorphic forms.

The intended direction is:

* rank-1 type polymorphism may be inferred in common cases
* row polymorphism should be supported explicitly in annotations
* higher-rank polymorphism, when added later, should rely on explicit annotations and bidirectional checking

### Practical Consequence

The current draft should be read as favoring:

* explicit polymorphic annotations at declaration sites
* a simple inference story for ordinary terms
* a conservative checking story for richer polymorphic structure

This keeps the core specification compatible with future higher-rank work without forcing that complexity into the current draft.

---

# 9. Computation Core

The computation core is intentionally minimal.

## 9.1 Pure Computation

```
pure : a -> Computation r r a
```

`pure` lifts a value into a computation that preserves capability state.

## 9.2 Sequencing

```
bind :
  Computation r1 r2 a ->
  (a -> Computation r2 r3 b) ->
  Computation r1 r3 b
```

`bind` sequences computations and composes typestate transitions.

## 9.3 Operational Reading

The intended operational intuition is:

```
run : Env pre -> Computation pre post a -> (Env post, a)
```

`Env` is runtime host state.

`pre` and `post` are static descriptions of required and resulting capability environments.

More precisely:

* a closed `pre` or `post` row is an exact environment description
* an open `pre` or `post` row is an exact environment schema parameterized by an unknown remainder

This means that the usability benefits of lower-bound style capability typing are intended to come from open-row abstraction, not from an implicit weakening of row meaning.

---

# 10. Host Boundary

The host is the only source of authority.

## 10.1 Primitive Operations

The host may register primitive terms with types expressed in the same core language.

Examples:

```
dbOpen  : Computation { db : DB[Closed] } { db : DB[Opened] } Unit
dbClose : Computation { db : DB[Opened] } { db : DB[Closed] } Unit
dbQuery : Query -> Computation { db : DB[Opened] } { db : DB[Opened] } Rows
```

Open-row variants are expected to be useful:

```
dbOpen : Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
```

## 10.2 Capability Discipline

This draft assumes:

* capabilities are supplied by the host
* capabilities are not forgeable by user code
* there is no ambient authority

This is the core capability-security commitment of the language.

---

# 11. Equality and Well-Formedness

## 11.1 Well-Formed Types

Types must be well-kinded relative to the current context.

## 11.2 Well-Formed Rows

Rows must contain well-formed types at each label.

## 11.3 Equality

This draft assumes a minimal equality relation based on:

* alpha-equivalence
* syntactic equality
* future row normalization

This draft does not yet include:

* equality by type-level reduction
* equality by solver discharge
* full definitional equality over term computation

## 11.4 Formation Strategy

This draft intends well-formedness to play a foundational role.

The design strategy is:

* formation is defined before typing
* family application is checked at formation time
* row structure is validated at formation time
* typing rules may assume that referenced types and rows are already well-formed

The purpose of this layer is not only to reject malformed syntax.

Its purpose is to define the universe of meaningful type-level objects before typing judgments are introduced.

## 11.5 Formation Judgments

The intended formation judgments are:

```text
Gamma |- T wf
Gamma |- R wf
Gamma |- T : Type
Gamma |- R : Row
```

The `wf` and kinding-style readings are equivalent in spirit in this draft.

The important point is that:

* types must be checked as types
* rows must be checked as rows

before they are used inside typing rules.

## 11.6 Intended Formation Rules

The following are intended as the basis of future formal rules.

### Type Variable

If a type variable is declared in the context, it is a well-formed type.

```text
a : Type in Gamma
-----------------
Gamma |- a : Type
```

### Function Type

```text
Gamma |- A : Type
Gamma |- B : Type
-----------------
Gamma |- A -> B : Type
```

### Universal Quantification

```text
Gamma, a : Type |- T : Type
---------------------------
Gamma |- forall a. T : Type
```

This is the current draft's formation rule for type polymorphism.

If later drafts add row polymorphism or richer binders, they should extend this pattern rather than replace it.

### Row Quantification

```text
Gamma, r : Row |- T : Type
--------------------------
Gamma |- forall r. T : Type
```

This is the intended formation rule for explicit row polymorphism.

It is included because open-row capability typing is part of the active design of this draft.

### Computation Type

```text
Gamma |- R1 : Row
Gamma |- R2 : Row
Gamma |- A  : Type
-----------------------------
Gamma |- Computation R1 R2 A : Type
```

This rule is especially important because it fixes `Computation` as a family whose first two arguments are row indices and whose last argument is a value type.

### Empty Row

```text
-------------
Gamma |- {} : Row
```

### Row Extension

```text
Gamma |- A : Type
Gamma |- R : Row
l fresh in R
-------------------------
Gamma |- { l : A | R } : Row
```

This is the intended abstract rule behind open rows.

The side condition `l fresh in R` expresses the design intent that row labels are unique.

### Closed Multi-Field Rows

Closed rows with several fields are understood as iterated row extension over `{}`.

For example:

```text
{ db : DB[Opened], log : Logger[Ready] }
```

is read abstractly as a finite row built from the empty row by repeated extension.

## 11.7 Design Consequences

These intended rules imply the following specification stance.

### Formation Before Typing

Typing rules should not be responsible for deciding whether:

* a family was applied to arguments of the right kinds
* a row tail is actually a row
* a field type is actually a type

Those responsibilities belong to formation.

### Row Discipline at Formation Time

The following should be rejected before ordinary typing:

* duplicate row labels
* non-type row payloads
* non-row row tails

### Future Extensions

If later drafts add:

* promoted protocol states
* row polymorphism
* GADTs
* richer constraints

then the preferred strategy is to strengthen formation and equality in a controlled way rather than to overload ordinary typing with those responsibilities.

---

# 12. Haskell-Like Surface Direction

The part of `v0.1` intentionally retained in spirit is the surface style.

This draft assumes a Haskell-like concrete syntax direction:

* `Name :: Type` for type annotation
* `Name := Expr` or nearby Haskell-like definition syntax
* `data Name = ...` for algebraic data
* lambda syntax in the style `\x -> e`
* infix declarations in a Haskell-like style

This is a surface-language preference, not the organizing principle of the specification.

The organizing principle remains the minimal core vocabulary described above.

---

# 13. Minimal Declaration Forms

Programs consist of declarations.

```
Program ::= Decl*
```

This draft assumes at least the following declaration forms.

## 13.1 Type Annotation

```
Name :: Type
```

## 13.2 Definition

```
Name := Expr
```

## 13.3 Data Declaration

```
data Name = Constructor*
```

## 13.4 Host Primitive Declaration

The exact concrete syntax is left open, but the abstract form is:

```
primitive Name :: Type
```

These declarations are introduced by the host, not ordinary user code.

## 13.5 Operator Declarations

```
infixl n op
infixr n op
infix  n op
```

---

# 14. Typing Rules for Values

This section gives the intended shape of value-level typing.

These rules are schematic rather than exhaustive, but they define the direction of the core type system.

## 14.1 Variable

```text
x : A in Gamma
--------------
Gamma |- x : A
```

## 14.2 Lambda

```text
Gamma, x : A |- e : B
---------------------
Gamma |- \x -> e : A -> B
```

## 14.3 Application

```text
Gamma |- f : A -> B
Gamma |- u : A
------------------
Gamma |- f u : B
```

## 14.4 Type Annotation

```text
Gamma |- e : A
Gamma |- A == B
------------------
Gamma |- (e : B) : B
```

## 14.5 Type Instantiation

```text
Gamma |- e : forall a. T
Gamma |- A : Type
--------------------------
Gamma |- e : T[a := A]
```

This draft presents instantiation abstractly.

The intended design direction is:

* top-level annotations may quantify explicitly
* use sites may rely on implicit instantiation in simple rank-1 cases
* richer polymorphic use should remain specification-visible

Later drafts should distinguish explicit and implicit instantiation more carefully, but this draft does not require explicit type application syntax.

## 14.6 Row Instantiation

```text
Gamma |- e : forall r. T
Gamma |- R : Row
-------------------------
Gamma |- e : T[r := R]
```

This rule is the value-level counterpart of explicit row polymorphism.

The intended design direction is:

* row-polymorphic declarations should normally be written with explicit annotations
* use sites may instantiate row variables implicitly when unification is straightforward
* row polymorphism should not depend on unrestricted higher-rank inference

## 14.7 Case Analysis over Algebraic Data

This draft assumes that algebraic data is eliminated by case analysis.

Abstractly:

```text
Gamma |- s : D
Gamma, ... |- b1 : A
...
Gamma, ... |- bn : A
----------------------------
Gamma |- case s of ... : A
```

The exact pattern grammar is deferred, but the intended rule shape is:

* the scrutinee has an algebraic data type
* each branch is checked under bindings introduced by its pattern
* all branches must produce the same result type

This is the core elimination form for ADTs in the current design.

---

# 15. Typing Rules for Computations

This section gives the intended shape of computation-level typing.

The language keeps computations explicit at the type level even if future surface syntax introduces lighter notation.

## 15.1 Pure Introduction

```text
Gamma |- e : A
-----------------------------
Gamma |- pure e : Computation r r A
```

## 15.2 Sequencing

```text
Gamma |- c1 : Computation r1 r2 A
Gamma, x : A |- c2 : Computation r2 r3 B
-----------------------------------------
Gamma |- bind c1 (\x -> c2) : Computation r1 r3 B
```

This rule is the core typing principle for indexed sequencing.

## 15.3 Primitive Computation Terms

If the host registers a primitive term with a type `T`, then that primitive may be used wherever a term of type `T` is allowed.

For example, if:

```text
dbOpen :
  forall r.
  Computation { db : DB[Closed] | r }
              { db : DB[Opened] | r }
              Unit
```

then `dbOpen` may be typed by ordinary variable or primitive lookup together with row instantiation.

## 15.4 Value/Computation Boundary

This draft assumes the following boundary:

* function abstraction and application are value-level constructs
* `pure` and `bind` are the primitive computation-level constructs

Future surface syntax may add computation notation, but it should elaborate to this core rather than replace it.

## 15.5 Case Analysis and Computations

This draft does not yet fix whether `case` is:

* purely a value-level eliminator
* or also allowed directly in computation-level surface forms

The intended core direction is conservative:

* `case` is fundamentally a value-level eliminator
* computation-oriented case notation, if later added, should elaborate through the value/computation core rather than redefine it

---

# 16. Row Equality and Unification Direction

This draft does not yet provide a full algorithm, but it does fix the intended direction of row equality and unification.

## 16.1 Intended Equality Discipline

Rows are intended to be equal up to:

* alpha-equivalence of row variables
* permutation of labels

Rows are **not** intended to admit duplicate labels.

So, for example:

```text
{ db : DB[Opened], log : Logger[Ready] }
==
{ log : Logger[Ready], db : DB[Opened] }
```

is intended to hold.

## 16.2 Intended Unification Discipline

Row unification is intended to support:

* matching on shared labels
* solving for row tails
* checking tail variables have kind `Row`
* rejecting duplicate-label solutions

Examples of intended behavior:

```text
{ db : DB[Closed] | r1 }
~
{ db : DB[Closed], log : Logger[Ready] }
```

should reduce to:

```text
r1 ~ { log : Logger[Ready] }
```

while:

```text
{ db : DB[Closed] | r1 }
~
{ db : DB[Opened] | r2 }
```

should fail.

## 16.3 Why Unification Matters Here

Open rows are not a surface convenience only.

They are the mechanism by which:

* capability-preserving primitives remain reusable
* sequencing composes exact environment schemas
* row-polymorphic declarations become meaningful

## 16.4 Deferred Algorithmic Detail

Future drafts should give:

* a canonical row representation
* an occurs check for row variables
* a precise normalization procedure
* a unification algorithm or equivalent constraint formulation

This draft fixes the intended semantics but not yet the concrete algorithm.

---

# 17. Evaluation Semantics Direction

This draft does not yet give a full formal operational semantics, but it fixes the intended shape.

## 17.1 Value Evaluation

The value layer is intended to follow a lambda-calculus-like evaluation story with:

* variables
* lambda abstraction
* application
* algebraic constructors
* case analysis

The exact evaluation strategy is not fully fixed in this draft, but future drafts should make it explicit.

## 17.2 Computation Evaluation

The computation layer is intended to evaluate by sequencing explicit computation terms.

At minimum, the following equations should guide the future semantics:

```text
bind (pure v) k   ->   k v
```

and, operationally:

```text
run env (pure v) = (env, v)
```

```text
run env (bind c k) =
  let (env', v) = run env c
  in run env' (k v)
```

These equations are schematic, but they capture the intended role of `pure` and `bind`.

## 17.3 Primitive Operations

Primitive operations are evaluated by host-provided implementations.

The static type of a primitive constrains:

* which capability environment must be present
* how that environment may change
* what result is returned

## 17.4 Determinism Constraint

Whatever concrete evaluation strategy is chosen later, it must preserve the determinism commitments of this draft.

In particular:

* evaluation order must not be left unspecified
* host interaction must remain explicit
* capability transitions must remain observable in the static model

---

# 18. Determinism

Determinism means:

* evaluation order must eventually be specified
* no implicit access to time, randomness, threads, or ambient IO exists
* observable external behavior occurs only through explicit host-provided capabilities

The host may connect capabilities to real systems, but that authority must remain explicit in both runtime and static structure.

---

# 19. Boundary of This Draft

This draft is intentionally narrow.

It defines:

* a stratified core
* a row-indexed computation family
* a capability-centered host boundary
* a Haskell-like surface direction

## 15.1 Intended Growth

The current design is intentionally friendly to the following extension directions:

* row polymorphism over capability environments
* promoted protocol states
* GADT-like refinement over indexed data
* richer higher-rank polymorphism
* explicit constraint systems
* explicit usage discipline
* stronger capability protocol descriptions

These are considered natural continuations of the current core rather than departures from it.

## 15.2 Deliberate Commitments

The current draft also makes several deliberate commitments.

These commitments are not claimed to be universally necessary, but they are active design choices of this specification:

* `Term`, `Type`, and `Kind` are currently separated
* `Row` is currently a distinguished built-in kind
* `Computation` is currently the distinguished computation family
* rows are currently specialized to capability environments rather than general records
* equality is currently intended to stay syntax-directed plus row normalization

These choices make the current draft simpler, but they also favor some future extensions over others.

## 15.3 Deliberately Deferred Directions

The following directions are not ruled out forever, but they are not natural next steps from the current draft without further redesign:

* full dependent typing
* general type-level computation
* algebraic effects with handlers
* fully general structured indices beyond the current row story
* general module calculi
* subtyping-centered designs

If the language later moves in one of these directions, future drafts should say explicitly which current commitments are being relaxed or replaced.

It does not yet define:

* concrete expression grammar in full
* typing rules in rule notation
* row unification
* polymorphism checking strategy
* evaluation semantics in small-step or big-step form

Those should be the focus of the next draft.

---

# 20. Next Draft Priorities

The next draft should prioritize:

1. explicit expression grammar
2. a precise account of explicit vs implicit polymorphic instantiation syntax
3. a concrete row unification algorithm
4. evaluation semantics in rule form
5. pattern grammar and exhaustiveness
6. host primitive registration and runtime interface
