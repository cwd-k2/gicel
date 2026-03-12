# Extended Catalog of Advanced Type-System Directions

One-line description: a broader catalog of advanced type-system features, organized by the previously identified extension lanes rather than by language or research tradition.

## Table of Contents

1. Purpose
2. Reading Guide
3. Abstraction Lane
4. Index Lane
5. Type-Equality Lane
6. Logic Lane
7. Resource-and-Effect Lane
8. Usage Discipline Lane
9. Packaging and Modularity Adjacent to the Lanes
10. Which Features Tend to Travel Together
11. Which Features Are Closest to Gomputation
12. Key References

## 1. Purpose

The earlier extension-directions note identified the major lanes along which advanced type systems tend to evolve.

This note broadens the catalog. Its goal is not to be exhaustive in the literature-survey sense, but to ensure that common and practically important advanced type-system features are not omitted from the lane map.

In particular, this note fills gaps around:

- rank-n and impredicative polymorphism
- qualified and quantified constraints
- singleton techniques
- union and intersection types
- occurrence typing
- session types
- ownership, borrowing, and regions
- graded effects and coeffects
- path-dependent and object-oriented type refinement

## 2. Reading Guide

Each section lists features by the lane they primarily extend.

This does not mean a feature belongs to only one lane. Many advanced features interact with multiple lanes. The placement here is based on what each feature most fundamentally strengthens.

## 3. Abstraction Lane

This lane concerns what can be abstracted over and what forms of polymorphism the language admits.

### 3.1 Rank-n Polymorphism

This is the direct continuation of rank-1 and rank-2 polymorphism:

```text
rank-1
-> rank-2
-> rank-n
```

Rank-n polymorphism allows `forall` under arrows at arbitrary depth. GHC treats this as `RankNTypes`.

This belongs squarely in the abstraction lane.

### 3.2 Impredicative Polymorphism

Impredicativity goes beyond rank-n polymorphism by allowing polymorphic types to instantiate type variables:

```text
id : forall a. a -> a
id @(forall x. x -> x)
```

This is still an abstraction-lane feature, but it pressures type inference much more than ordinary rank-n polymorphism. In practice it also leans on stronger instantiation and equality machinery.

### 3.3 Bounded Quantification

Classic `F<:`-style bounded quantification extends abstraction with constraints on quantified variables:

```text
forall a <: T. ...
```

This is abstraction plus subtyping. It is a distinct branch from ML-style polymorphism and often pushes a language toward OO-style generic reasoning.

### 3.4 Higher-Kinded and Kind Polymorphism

These were already discussed, but they belong here as the continuation of abstraction over larger syntactic categories:

```text
type variables
-> constructor variables
-> kind-polymorphic variables
```

### 3.5 Qualified Types and Type Classes

Qualified polymorphism extends abstraction with explicit constraints:

```text
forall a. C a => ...
```

This is one of the most practical abstraction enrichments in real languages. It often creates demand for:

- associated types
- higher-kinded abstraction
- law-oriented library design

### 3.6 Quantified Constraints

Quantified constraints lift constraints themselves into quantified positions:

```text
forall a. C a => D a
```

This is abstraction over evidence structure, not just ordinary types.

### 3.7 Polymorphic Function Values and Dependent Function Values

Languages like Scala 3 distinguish:

- polymorphic function types
- dependent function types

These continue the abstraction lane while also leaning toward dependent typing.

## 4. Index Lane

This lane concerns carrying more static information inside family parameters.

### 4.1 Phantom Types

This is the entry point:

```text
T s
```

where `s` is not represented dynamically.

### 4.2 DataKinds and Promoted Finite Indices

Already discussed. This remains the cleanest practical next step after phantom or indexed types.

### 4.3 Singleton Types and Singleton Techniques

Singleton-based programming bridges type-level and term-level information by creating a one-to-one correspondence between some values and types.

In practice, singleton techniques are often the pragmatic bridge between promoted indices and dependent-style programming.

### 4.4 Sized Types

Sized types index structures by size information to control recursion and productivity:

```text
Stream i a
```

This is a clear index-lane feature with strong links to termination and guarded recursion.

### 4.5 GADTs

GADTs sit later in the index lane because constructors can refine result indices.

### 4.6 Full Dependent Indices

This is the far end of the lane:

```text
Vect n a
```

where `n` is not merely a promoted finite state but an arbitrary value-level quantity admitted into types.

## 5. Type-Equality Lane

This lane concerns how the checker decides whether two types count as the same.

### 5.1 Row Normalization and Unification

Already central to Gomputation.

### 5.2 Constructor-Refined Equality from GADTs

Pattern matching can refine what equalities hold locally. This is a major strengthening of the equality lane.

### 5.3 Type Families and Match Types

Type families in Haskell and match types in Scala both strengthen equality by allowing type-level reduction.

This is one of the clearest signs that a language has moved from "indices as labels" to "indices as computations".

### 5.4 Equality Types and Coercion Systems

Some languages expose equality itself as an object:

- propositional equality
- coercion witnesses
- equality constraints

This strengthens equality from an implicit checker relation to an explicit program-relevant notion.

### 5.5 Definitional Equality by Evaluation

This is the dependent-type end of the lane. Type checking now needs normalization or evaluation to compare types.

## 6. Logic Lane

This lane concerns what propositions or invariants types can express and discharge.

### 6.1 Union and Intersection Types

These are often presented as ordinary type constructors, but operationally they also strengthen logical expressiveness:

- union says a value belongs to at least one type
- intersection says a value satisfies multiple type views simultaneously

They are especially useful in dynamically flavored or structurally typed settings.

### 6.2 Occurrence Typing

Occurrence typing refines types using predicate outcomes in control flow. Typed Racket is a canonical practical example.

This is a logic-lane feature because it adds branch-sensitive reasoning about propositions established by predicates.

### 6.3 Refinement Types

Already discussed. These are explicit first-order predicates attached to types.

### 6.4 Predicate Subtyping

Predicate subtyping generalizes the idea that logical implications induce subtype relations. This is a more logic-heavy cousin of refinement typing.

### 6.5 Session Types

Session types are protocol specifications for communication. They can be seen as:

- logic over structured interaction traces
- protocol-indexed types
- sometimes a specialization of linear or effectful typing

They sit between the logic lane, the index lane, and the usage lane.

### 6.6 Temporal and Protocol Logics in Types

Some systems move beyond state labels and express temporal or trace properties in the type layer. This is further along the logic lane than ordinary typestate.

## 7. Resource-and-Effect Lane

This lane concerns how computations are classified and composed.

### 7.1 Latent Effects

A function type may carry an effect annotation that is realized when it is called.

### 7.2 Indexed Effects

Already core to Gomputation.

### 7.3 Effect Polymorphism

Effect-polymorphic systems abstract over effect information in the same way type-polymorphic systems abstract over types.

### 7.4 Algebraic Effects

These represent effects by operations rather than baked-in primitives.

### 7.5 Effect Handlers

Handlers interpret or eliminate algebraic effects. They are a major continuation of this lane.

### 7.6 Graded Effects

Graded effect systems quantify not only which effects may happen, but with what grade, quantity, or modality.

This is a bridge toward usage-sensitive systems.

### 7.7 Coeffects

Coeffects track requirements on contextual resources rather than produced effects. They are the contextual dual of many effect systems.

This is an important advanced branch because it intersects with usage, context dependence, and environmental requirements.

## 8. Usage Discipline Lane

This lane concerns how values or capabilities may be consumed.

### 8.1 Affine and Linear Types

These enforce at-most-once or exactly-once use.

### 8.2 Uniqueness Types

Uniqueness typing tracks whether a value has a unique reference, often to allow safe in-place update.

This is related to linearity, but operationally motivated by optimization and controlled mutation.

### 8.3 Ownership and Borrowing

Rust's ownership system and borrow checking are a practically important branch of usage discipline. They are not merely linear types, but they are closely related in spirit.

### 8.4 Region Types

Region systems track where storage lives and when it may be deallocated. This is another practical form of usage/resource discipline.

### 8.5 Fractional Permissions and Separation-Inspired Typing

These systems refine usage discipline by tracking how access rights may be split and recombined.

### 8.6 Session-Typed Channels

Session typing often depends on linear or affine treatment of communication endpoints. This is why session types reappear here as well.

## 9. Packaging and Modularity Adjacent to the Lanes

Some features do not fit neatly as a lane step, but they materially change how other lane features pay off.

### 9.1 Existential Types

Existentials support hiding implementation details and packaging values with hidden internal types.

They often travel with:

- GADTs
- modules
- first-class packaging

### 9.2 First-Class Modules and Functors

Module systems are not just syntax. They can express abstraction, existential hiding, and structured parameterization in ways that interact strongly with HKT, associated types, and constraints.

### 9.3 Path-Dependent Types

Path-dependent types, as seen in Scala-family designs, refine types by the identity of a term path.

They sit somewhere between:

- dependency
- modules
- object-oriented abstraction

They are not a natural early extension for Gomputation, but they are an important advanced type-system direction in the wild.

### 9.4 Gradual Typing

Gradual typing is not primarily an "advanced static typing" feature, but it is a significant typing direction. It inserts a typed-untyped boundary and consistency relation instead of ordinary equality alone.

This matters because union types, occurrence typing, and refinement-like narrowing often show up near gradually typed systems.

## 10. Which Features Tend to Travel Together

Certain feature clusters recur in language design.

### 10.1 ML/FP Precision Cluster

- ADTs
- rank-n polymorphism
- GADTs
- type classes / constraints
- type families or associated types
- DataKinds
- singletons

### 10.2 Lightweight Verification Cluster

- refinement types
- occurrence typing
- union/intersection types
- predicate subtyping

### 10.3 Resource Safety Cluster

- typestate
- affine/linear typing
- ownership/borrowing
- session types
- graded effects or coeffects

### 10.4 Dependent Programming Cluster

- DataKinds
- singletons
- GADTs
- equality types
- dependent functions
- definitional equality

These clusters are useful because they show why some features feel premature in isolation: their surrounding ecosystem is absent.

## 11. Which Features Are Closest to Gomputation

For the current project, the nearest neighboring features are:

1. rank-n polymorphism
2. qualified constraints, if a trait layer ever appears
3. DataKinds
4. GADTs
5. existential packaging
6. linear or affine capability tracking, if duplication becomes unsound
7. graded effects or coeffects, if capability requirements become more quantitative
8. session-typed protocol abstractions, if the protocol story grows beyond simple host primitives

More distant features include:

1. impredicative polymorphism
2. path-dependent types
3. full dependent types
4. gradual typing

These are real and important, but they are not natural next steps for the current design.

## 12. Key References

1. GHC User's Guide, arbitrary-rank polymorphism. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/rank_polymorphism.html
2. GHC User's Guide, impredicative polymorphism. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/impredicative_types.html
3. GHC User's Guide, quantified constraints. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/quantified_constraints.html
4. Typed Racket Guide, occurrence typing. https://docs.racket-lang.org/ts-guide/occurrence-typing.html
5. `singletons` package overview. https://hackage.haskell.org/package/singletons
6. Links language site. https://links-lang.org/
7. GHC User's Guide, linear types. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/linear_types.html
8. Rust book, ownership. https://doc.rust-lang.org/book/ch04-01-what-is-ownership.html
9. Coeffects paper repository page. https://kar.kent.ac.uk/57493/
10. Scala 3 reference, new types and polymorphic/dependent function types. https://docs.scala-lang.org/scala3/reference/
