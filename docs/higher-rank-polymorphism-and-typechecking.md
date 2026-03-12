# Higher-Rank Polymorphism and Typechecking Strategy

One-line description: what "high-rank polymorphism" should mean in practice, and how to support it without making the checker intractable.

## Table of Contents

1. Why This Matters
2. What Higher-Rank Means
3. Why Full Inference Is Hard
4. Practical Typechecking Strategies
5. Recommended Strategy for Gomputation
6. Interaction with Rows and `Comp`
7. Common Failure Modes
8. Key References

## 1. Why This Matters

The draft lists high-rank polymorphism as a design goal. That is a powerful feature, but it is also one of the fastest ways to make a language checker complex.

Before expanding the surface syntax, the spec should answer one question very clearly:

Should higher-rank types be inferred, partially inferred, or only checked against annotations?

## 2. What Higher-Rank Means

Rank-1 polymorphism places `forall` only at the outermost level:

```text
forall a. a -> a
```

Higher-rank polymorphism allows polymorphic values to appear under arrows:

```text
(forall a. a -> a) -> Int
```

or deeper:

```text
((forall a. a -> a) -> Int) -> Bool
```

This matters for APIs that accept polymorphic callbacks, module-like bundles of operations, or abstract effect combinators.

## 3. Why Full Inference Is Hard

The key fact from the literature is simple:

- complete inference for higher-rank impredicative polymorphism is undecidable

That does not mean the feature is impractical. It means the language needs a disciplined annotation policy.

Two stable lessons from the literature are:

1. require annotations at strategic points
2. use bidirectional typing or local type inference to reduce burden

## 4. Practical Typechecking Strategies

### 4.1 Full HM-style inference

Pros:

- pleasant for rank-1 code

Cons:

- does not scale cleanly to higher-rank polymorphism
- interacts badly with rows and indexed computations

### 4.2 Annotation-heavy checking

Pros:

- easiest to specify
- easiest to implement soundly

Cons:

- poor ergonomics if overused

Typical rule:

- lambda-bound parameters with polymorphic types need explicit annotations

### 4.3 Local type inference

Peyton Jones, Vytiniotis, Weirich, and Shields show that arbitrary-rank polymorphism can be made practical if annotations are required in carefully chosen places and the checker propagates information locally.

This is a strong reference point for a compact language implementation.

### 4.4 Bidirectional typing

Bidirectional typing splits judgments into:

```text
Gamma |- e => A   ; infer
Gamma |- e <= A   ; check
```

This is usually the best engineering choice for languages with:

- higher-rank polymorphism
- GADT-like features
- effect annotations
- richer introduction and elimination forms

The rule of thumb is:

- infer for variables and eliminations
- check for lambdas and annotated terms

## 5. Recommended Strategy for Gomputation

### 5.1 Use bidirectional typing from the beginning

Even if the initial surface language is small, this will scale better than retrofitting later.

### 5.2 Keep inference strong at rank 1

You can still support convenient ML-style inference for ordinary code:

- literals
- variables
- applications
- let-bound monomorphic code

### 5.3 Require annotations for higher-rank binders

A practical first rule:

1. top-level definitions may have optional annotations
2. lambda parameters that must be polymorphic require annotations
3. explicit type annotations are required at rank-raising introduction points

Example:

```text
useId : (forall a. a -> a) -> Int
useId f := ...
```

### 5.4 Keep generalization conservative

If `Comp` interacts with host effects, unrestricted let-generalization can become confusing. A conservative value restriction may be unnecessary in a pure core, but the spec should still define where generalization occurs.

Recommendation:

- generalize at top-level and pure `let`
- do not rely on implicit generalization inside computation syntax until the design settles

## 6. Interaction with Rows and `Comp`

Higher-rank polymorphism combines with your other features in subtle ways.

### 6.1 Polymorphism over rows

You may eventually want:

```text
forall r. Comp { log : Logger[Ready] | r } { log : Logger[Ready] | r } Unit
```

This is useful, but it raises the complexity of kinds and instantiation.

Recommendation:

- support type variable polymorphism first
- add row polymorphism deliberately, with explicit kinding rules

### 6.2 Polymorphic computation combinators

You will likely want library functions like:

```text
mapComp :
  forall a b r1 r2 r3.
  (a -> b) ->
  Comp r1 r2 a ->
  Comp r2 r3 b
```

These are easier to check than to infer from scratch.

### 6.3 Error quality matters more than completeness

For an embedded language, users will tolerate explicit annotations more readily than poor or mysterious type errors.

## 7. Common Failure Modes

### 7.1 Promising "high-rank polymorphism" without annotation policy

That leaves the checker underspecified.

### 7.2 Letting implicit instantiation happen everywhere

That often creates surprising ambiguity around `forall`, rows, and computation types.

### 7.3 Mixing inference and checking informally

Write bidirectional judgments explicitly in the spec. Otherwise implementation choices drift.

## 8. Key References

1. Simon Peyton Jones, Dimitrios Vytiniotis, Stephanie Weirich, Mark Shields, "Practical Type Inference for Arbitrary-Rank Types". https://www.microsoft.com/en-us/research/publication/practical-type-inference-for-arbitrary-rank-types/
2. Jana Dunfield and Neel Krishnaswami, "Complete and Easy Bidirectional Typechecking for Higher-Rank Polymorphism", ICFP 2013. https://doi.org/10.1145/2500365.2500582
3. Benjamin Pierce and David Turner, "Local Type Inference", TOPLAS 2000.

## Relevance to Gomputation

The draft should not commit to "full inference". A stronger and more realistic statement would be:

"The language supports rank-1 inference by default and higher-rank polymorphism via bidirectional typechecking with required annotations at rank-raising binders."

That is precise enough to implement and still expressive enough for advanced libraries.
