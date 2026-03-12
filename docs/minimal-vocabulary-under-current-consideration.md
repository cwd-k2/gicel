# Minimal Vocabulary Under Current Consideration

One-line description: the smallest core vocabulary that currently appears sufficient to explain the full space of advanced type, effect, logic, and usage extensions surveyed for Gomputation.

## Table of Contents

1. Status
2. The Question
3. Method
4. Final Working Answer
5. The Irreducible Core
6. The Expanded Specification Core
7. Why This Is the Minimum
8. How Major Feature Families Reconstruct from It
9. What Is Deliberately Not in the Core Vocabulary
10. Current Recommendation for Gomputation
11. Open Questions

## 1. Status

This is a design-stage synthesis document.

It is not a final formal calculus. It is the current best attempt to compress all of the investigated material:

- symbolic logic
- lambda calculus
- typed lambda calculi
- indexed and effectful computation
- rows and capabilities
- advanced type-system extensions
- substructural and resource-sensitive typing

into the smallest vocabulary that still explains how these directions fit together.

## 2. The Question

The real question is not:

```text
Which advanced features should the language list?
```

The real question is:

```text
What notions must the specification be able to talk about
so that advanced features become refinements of those notions
rather than unrelated additions?
```

This is a question about explanatory compression, not only about syntax.

## 3. Method

The synthesis here uses three filters.

### 3.1 Proof-Theoretic Filter

From symbolic logic and type theory:

- judgments matter as much as syntax
- introduction and elimination are foundational
- equality and consequence are not optional conveniences

### 3.2 Computational Filter

From lambda calculus:

- binding and application are irreducible
- substitution and reduction sit behind all binder-based calculi
- typed extensions are usually refinements of a small lambda-like core

### 3.3 Extension-Lane Filter

From the broader type-system survey:

- indexing
- abstraction
- equality
- logic
- effects
- usage

are the recurring axes along which systems become more expressive.

Any candidate minimal vocabulary that cannot express all six axes is too small.

## 4. Final Working Answer

There are two useful notions of "minimal vocabulary".

### 4.1 Irreducible Core

At the most compressed level, the current best answer is:

1. sort
2. context
3. family
4. formation
5. introduction
6. elimination
7. equality judgment
8. entailment judgment
9. computation judgment
10. usage judgment

This is the smallest vocabulary that still seems able to explain the full surveyed design space.

### 4.2 Expanded Specification Core

For an actual language specification, the core should usually be written in a slightly expanded form:

1. sort
2. context
3. family
4. index
5. binder
6. application
7. constructor
8. elimination
9. equality judgment
10. constraint judgment
11. effect judgment
12. usage judgment

This second form is not conceptually smaller, but it is much easier to write a real specification from.

The rest of this document explains why the first list is the true conceptual minimum, while the second is the practical drafting minimum.

## 5. The Irreducible Core

### 5.1 Sort

The specification needs a notion of stratified syntactic or semantic levels.

At minimum:

```text
Term
Type
Kind
```

Possibly later:

```text
Constraint
Usage
```

Why this is irreducible:

- without levels, higher-rank, higher-kinded, promotion, and dependency cannot be described coherently
- the distinction between "program", "classifier", and "classifier of classifier" is one of the main organizational facts of advanced type systems

### 5.2 Context

The specification needs a notion of context.

Contexts carry assumptions such as:

- variable bindings
- type variables
- row variables
- capability assumptions
- constraints
- usage assumptions

Why this is irreducible:

- logic needs assumptions
- lambda calculus needs environments for binding
- typing, equality, constraint solving, and usage all depend on contexts

This is one place where the previous draft of the document was too compressed. Context is not optional. It is one of the basic nouns of the whole enterprise.

### 5.3 Family

The specification needs a notion of family:

```text
F : K1 -> ... -> Kn -> Sort
```

This is the unifying shape behind:

- type constructors
- indexed types
- `Comp pre post a`
- promoted-state families
- future dependent or proof-indexed structures

Why this is irreducible:

- without family, every indexed construction becomes a special case
- family is the smallest noun that covers both ordinary and indexed type structure

### 5.4 Formation

The specification needs a general notion of well-formedness or formation.

Examples:

- well-formed sorts
- well-formed families
- well-formed types
- well-formed rows
- well-formed contexts

Why this is irreducible:

- logic, type theory, and typed lambda calculi all distinguish between raw syntax and well-formed objects
- every later judgment presupposes formation

### 5.5 Introduction

The specification needs introduction forms.

These are the ways objects are built:

- lambda abstraction
- data constructors
- type abstraction
- proof introduction

Why this is irreducible:

- natural deduction, typed lambda calculus, and algebraic data all depend on introduction structure
- many advanced features are not new object kinds, but refined introduction rules

### 5.6 Elimination

The specification needs elimination forms.

These are the ways introduced structure is used:

- application
- case analysis
- projection
- evidence use

Why this is irreducible:

- GADTs, existentials, constraints, and dependent refinements all become meaningful through elimination
- introduction without elimination is not enough to explain computation or reasoning

### 5.7 Equality Judgment

The specification needs an equality judgment.

Initially this may cover only:

- alpha-equivalence
- syntactic equality
- row normalization

Later it may strengthen to include:

- local equalities from pattern matching
- type-level reduction
- definitional equality by evaluation

Why this is irreducible:

- advanced type systems are distinguished less by syntax than by what equalities the checker recognizes

### 5.8 Entailment Judgment

The specification needs a general notion of derived obligation or consequence.

This is broader than ordinary typing equality.

It includes future occupants such as:

- class or trait entailment
- row side conditions
- logical predicates
- solver-mediated obligations

Why this is irreducible:

- once the system grows beyond plain typing, there must be a place for "what follows from assumptions"
- this is the logical ancestor of constraints

`Entailment` is the more fundamental word; `constraint` is the implementation-facing word.

### 5.9 Computation Judgment

The specification needs a notion that classifies computations, not just values.

For Gomputation this already appears as:

```text
Comp pre post a
```

Why this is irreducible:

- the language distinguishes value and computation semantically
- effectful behavior is not only another type constructor; it is another mode of classification
- indexed effects, latent effects, handlers, and coeffects all grow from this axis

### 5.10 Usage Judgment

The specification needs a notion of usage discipline, even if dormant initially.

Possible future forms:

- unrestricted use
- affine use
- linear use
- graded use

Why this is irreducible:

- capability systems naturally create pressure toward usage tracking
- linearity, ownership, uniqueness, and session discipline all strengthen usage rather than ordinary type formation

## 6. The Expanded Specification Core

The irreducible core is conceptually clean, but too compressed for a working spec. For drafting purposes, it should be unpacked into the following explicit vocabulary.

### 6.1 Sort

Keep:

```text
Term
Type
Kind
```

Optionally reserve:

```text
Constraint
Usage
```

### 6.2 Context

Use contexts as explicit assumptions:

```text
Gamma, Delta, ...
```

These should be able to hold:

- term variables
- type variables
- row variables
- capability assumptions
- possibly constraints and usage assumptions later

### 6.3 Family

Use a general family notion for all constructor-like classifiers:

```text
F : K1 -> ... -> Kn -> Sort
```

Examples:

```text
Option : Type -> Type
DB     : State -> Type
Comp   : Row -> Row -> Type -> Type
```

### 6.4 Index

Introduce `index` as an explicit drafting notion, even though conceptually it is just a family argument with static meaning.

This is important because "indexed type" is a major organizing concept for the project.

### 6.5 Binder

Use explicit binders for:

- term abstraction
- type abstraction
- row abstraction
- possibly later kind or dependent abstraction

### 6.6 Application

Keep application explicit because it exposes where abstraction ranges:

- term application
- type application
- possibly index-level application later

### 6.7 Constructor

Use constructor as the formation-facing noun for algebraic inhabitants and primitive introductions.

### 6.8 Elimination

Use elimination as the consumption-facing noun for:

- application
- case analysis
- unpacking

### 6.9 Equality Judgment

Use equality as a first-class judgment of the specification:

```text
Gamma |- A == B
```

### 6.10 Constraint Judgment

Use a constraint judgment as the specification-facing unpacking of entailment:

```text
Gamma |- C
```

where `C` may later range over:

- row membership
- trait obligations
- logical predicates

### 6.11 Effect Judgment

Use an effect or computation judgment explicitly, even if the surface calculus encodes it via `Comp`.

Examples:

```text
Gamma |- c : Comp r1 r2 a
Gamma |- e : A
```

### 6.12 Usage Judgment

Reserve a usage judgment:

```text
Gamma |- u
```

or equivalent usage annotations in contexts.

This can remain inactive in the first version, but the specification should leave room for it conceptually.

## 7. Why This Is the Minimum

This section is the compression argument.

### 7.1 Why Not Smaller Than `sort + context + family`?

Without `sort`, the system loses stratification.

Without `context`, judgments lose assumptions.

Without `family`, indexed structure fragments into special cases.

Those three are non-negotiable.

### 7.2 Why Not Collapse Introduction and Elimination into Syntax Alone?

Because the whole logic-and-type-theory lineage says they are not accidental syntax classes. They are the organizing principles behind:

- natural deduction
- lambda abstraction/application
- algebraic data and pattern matching
- GADT refinement

### 7.3 Why Not Treat Equality, Entailment, Computation, and Usage as Mere Annotations?

Because these are precisely the four extension axes that change checker responsibility:

- equality strengthens type comparison
- entailment strengthens validity checking
- computation strengthens classification of programs
- usage strengthens resource discipline

If they are not named at the core, future growth becomes a pile of exceptions.

## 8. How Major Feature Families Reconstruct from It

### 8.1 ADT

Needs:

- family
- introduction
- elimination

### 8.2 DataKinds

Needs:

- sort
- family
- constructor-originating indices

### 8.3 GADT

Needs:

- family
- constructor
- elimination
- stronger equality

### 8.4 Rank-n and HKT

Needs:

- sort
- binder
- application
- family

### 8.5 Type Families

Needs:

- family
- index
- stronger equality

### 8.6 Refinement Types

Needs:

- entailment / constraint
- stronger logical validity

### 8.7 Indexed Effects

Needs:

- family
- index
- computation judgment

### 8.8 Linear and Ownership-Flavored Systems

Needs:

- usage judgment
- often richer contexts

### 8.9 Dependent Types

Needs:

- all of the above, with reduced separation between term and type strata and stronger equality

## 9. What Is Deliberately Not in the Core Vocabulary

The following are important, but they currently look like policy, specialization, or derived elaborations rather than core vocabulary items.

### 9.1 Subtyping

Important, but not part of the minimum. It is one possible relation over types, not a prerequisite for the surveyed extension space.

### 9.2 Modules

Important for packaging and abstraction boundaries, but one layer above the calculus core.

### 9.3 Specific Solvers or Semantic Models

SMT integration, categorical semantics, denotational models, and implementation strategies matter, but they are not part of the minimal vocabulary itself.

### 9.4 Particular Surface Syntax

`let`, `do`, handler syntax, match syntax, and so on are important design questions, but not part of the core explanatory minimum.

## 10. Current Recommendation for Gomputation

The strongest current recommendation is to distinguish two levels.

### 10.1 Conceptual Core

Think of the language as:

```text
a stratified, indexed, algebraic core
with explicit judgments for equality, entailment,
computation, and usage
```

### 10.2 Immediate Working Core

For the first concrete specification, use this active subset:

1. `Term`, `Type`, `Kind`
2. contexts
3. families
4. indices
5. binders and application
6. constructors and elimination
7. equality judgment
8. computation/effect judgment

Keep these reserved but not necessarily active in v0:

1. constraint/entailment judgment
2. usage judgment

That is small enough to stay coherent and large enough to explain where the project could grow.

## 11. Open Questions

The current answer is still provisional. The main remaining uncertainties are:

1. Should `entailment` stay a meta-judgment, or should `Constraint` become a first-class sort?
2. Should `computation judgment` remain tied to a distinguished `Comp` family, or become fully generic?
3. Should `usage` remain a separate judgment, or eventually be internalized into contexts or types?
4. Should `Row` remain a distinguished built-in kind, or eventually become one instance of a more general structured-index story?
5. At what point does equality become computational enough that the specification must separate normalization from full definitional equality explicitly?

These questions do not undermine the current synthesis. They are exactly what becomes visible once the core has been compressed far enough.
