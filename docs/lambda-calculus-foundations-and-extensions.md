# Lambda Calculus: Foundations and Extensions

One-line description: a research map of lambda calculus as the core formal model of functions, substitution, evaluation, types, and many later type-system extensions.

## Table of Contents

1. Scope
2. Why Lambda Calculus Matters Here
3. Untyped Lambda Calculus
4. Operational Foundations
5. Simply Typed Lambda Calculus
6. Polymorphic and Higher-Order Extensions
7. Data, Recursion, and Richer Type Disciplines
8. Dependent and Proof-Relevant Extensions
9. Substructural and Effect-Sensitive Variants
10. Semantic and Meta-Theoretic Themes
11. Relevance to Gomputation
12. Key References

## 1. Scope

This note focuses on lambda calculus as it matters for language design:

- function abstraction and application
- substitution and reduction
- normalization
- typed vs untyped calculi
- major extensions that later became programming-language features

## 2. Why Lambda Calculus Matters Here

The current Gomputation draft already assumes a language with:

- functions
- application
- binders
- pure expressions
- typed computations

Lambda calculus is the canonical formal core for exactly this setting. It provides the smallest durable account of:

- variable binding
- substitution
- evaluation
- normalization
- interaction between syntax and typing

Many later systems, including type theory, proof assistants, and effect calculi, can be understood as elaborations of typed lambda calculi.

## 3. Untyped Lambda Calculus

### 3.1 Core Syntax

The classic syntax is minimal:

```text
t ::= x | \x. t | t t
```

That is:

- variable
- abstraction
- application

This minimality is one reason lambda calculus remains foundational.

### 3.2 Beta Reduction

The central computational rule is beta reduction:

```text
(\x. t) u -> t[x := u]
```

This internalizes computation as substitution.

### 3.3 Alpha Equivalence

Bound variable names are not semantically important. Renaming bound variables gives alpha-equivalent terms.

### 3.4 Eta Principles

Eta conversion captures extensionality of functions in suitable settings:

```text
\x. f x  =  f
```

when `x` is not free in `f`.

### 3.5 Expressive Power

Even untyped lambda calculus can encode:

- booleans
- pairs
- naturals
- lists
- recursion via fixed-point combinators

This is important because it separates "computability power" from "static discipline".

## 4. Operational Foundations

### 4.1 Substitution

Substitution is not a minor implementation detail. It is the central semantic mechanism. Any serious specification that uses binders must decide how substitution behaves and how variable capture is avoided.

### 4.2 Confluence

Untyped lambda calculus enjoys the Church-Rosser property: if a term reduces in different ways, reductions can be joined.

This means normal forms, when they exist, are unique up to equivalence.

### 4.3 Normalization vs Divergence

Untyped lambda calculus is computationally universal and therefore admits divergence. Some terms do not normalize.

This distinction becomes important when comparing:

- untyped calculi
- simply typed calculi
- strongly normalizing dependent or proof-relevant systems

### 4.4 Evaluation Strategy

Different strategies matter operationally:

- call by name
- call by value
- call by need

Lambda calculus is neutral enough to study these systematically.

## 5. Simply Typed Lambda Calculus

### 5.1 Core Idea

The simply typed lambda calculus adds types to abstraction and application:

```text
Gamma, x : A |- t : B
---------------------
Gamma |- \x. t : A -> B
```

and:

```text
Gamma |- f : A -> B
Gamma |- u : A
----------------
Gamma |- f u : B
```

This gives one of the cleanest formal models of typed functional programming.

### 5.2 Main Properties

The classic properties are:

- subject reduction
- progress in suitable formulations
- strong normalization

This is one reason STLC is such a central reference point in type-system design.

### 5.3 Limits

STLC is elegant but limited:

- no polymorphism
- no general recursion if strong normalization is preserved
- no direct expression of many real program invariants

## 6. Polymorphic and Higher-Order Extensions

### 6.1 System F

System F extends lambda calculus with universal quantification over types.

This is a key foundation for parametric polymorphism:

```text
Lambda a. \x : a. x
```

at the type level.

System F is a major stepping stone from STLC toward modern typed languages.

### 6.2 Higher-Rank and Impredicative Polymorphism

Allowing polymorphic types in richer positions leads to:

- higher-rank polymorphism
- impredicative polymorphism

These are highly expressive but substantially harder to infer or elaborate.

### 6.3 F-omega

F-omega extends polymorphism to higher-kinded abstraction and type operators.

This is the canonical typed setting behind many HKT-style discussions.

## 7. Data, Recursion, and Richer Type Disciplines

### 7.1 Algebraic Data via Encodings and Primitive Extensions

Data can be represented by encodings in pure lambda calculus, but practical languages usually add direct algebraic data and pattern matching instead.

### 7.2 Recursive Types

Recursive types extend typed lambda calculus with self-referential type structure.

These matter for practical data and general recursion, but they complicate normalization and semantic reasoning.

### 7.3 Intersection Types

Intersection type disciplines classify terms more finely and can type terms that simple systems cannot.

They are important in the theory of normalization and expressive typing.

### 7.4 GADTs and Indexed Datatypes

Later typed lambda calculi add more precise data constructors and pattern-match refinement. In practice this often appears as GADTs, dependent pattern matching, or indexed families in type theory.

## 8. Dependent and Proof-Relevant Extensions

### 8.1 Dependent Type Theory

The dependent-type end of lambda calculus allows types to depend on terms. This is the basis of modern proof assistants and proof-relevant programming.

### 8.2 Equality Types and Identity Types

Once dependency appears, equality itself often becomes internalized as a type.

### 8.3 Martin-Lof Style Type Theory

Martin-Lof type theory can be read as a family of richly typed lambda calculi with dependent products, dependent sums, universes, and identity types.

### 8.4 Calculus of Constructions and Descendants

Systems such as the Calculus of Constructions and its descendants push the lambda-calculus line further toward unified proof-and-program calculi.

## 9. Substructural and Effect-Sensitive Variants

### 9.1 Linear Lambda Calculi

Linear logic led to linear lambda calculi in which variable use is controlled.

This matters for:

- resource tracking
- uniqueness and ownership
- protocol use

### 9.2 Monadic and Effect Calculi

Many effect systems can be understood as typed lambda calculi with a distinguished effect-bearing type constructor or judgment.

For Gomputation, `Comp pre post a` fits naturally in this tradition.

### 9.3 Algebraic Effect Calculi

Later variants add explicit effect operations and handlers.

### 9.4 Contextual and Modal Calculi

Some systems enrich lambda calculus with modal or contextual structure to capture staging, environment requirements, or contextual effects.

## 10. Semantic and Meta-Theoretic Themes

### 10.1 Normalization

Typed lambda calculi often recover strong normalization where the untyped calculus diverges freely.

### 10.2 Subject Reduction

Typing should be preserved by reduction.

### 10.3 Canonicity and Decidability

Once the calculus gets richer, one studies:

- what normal forms look like
- whether type checking remains decidable
- whether equality becomes computationally hard

### 10.4 Encodings vs Primitive Structure

A recurring language-design question is whether a feature should be:

- encoded in a minimal calculus
- or made primitive for clarity, efficiency, and error quality

This is directly relevant to Gomputation's current "minimal vocabulary" discussion.

## 11. Relevance to Gomputation

Lambda calculus contributes the following core insights to Gomputation:

1. binders, application, substitution, and evaluation should be specified cleanly before feature growth
2. typed computation should be seen as an extension of typed lambda-calculus structure, not as an unrelated add-on
3. `Comp pre post a` can be understood as a typed effect family layered on an otherwise lambda-calculus-like value core
4. later features such as higher-rank polymorphism, HKT, DataKinds, GADTs, and dependency all fit into well-known lambda-calculus extension paths

In short, if symbolic logic provides the proof-theoretic scaffold, lambda calculus provides the computational skeleton.

## 12. Key References

1. Henk Barendregt, Wil Dekkers, Richard Statman, *Lambda Calculus with Types*. https://aslonline.org/books/perspectives-in-logic/available-volumes/lambda-calculus-with-types/
2. Jean H. Gallier, "Constructive Logics Part I: A Tutorial on Proof Systems and Typed Lambda-Calculi". https://repository.upenn.edu/handle/20.500.14332/7337
3. Alonzo Church, "A Set of Postulates for the Foundation of Logic". https://doi.org/10.2307/2269031
4. Alonzo Church, "A Formulation of the Simple Theory of Types". https://doi.org/10.2307/2266170
5. Per Martin-Lof, intuitionistic type theory tradition summarized in SEP. https://plato.stanford.edu/entries/type-theory-intuitionistic/
