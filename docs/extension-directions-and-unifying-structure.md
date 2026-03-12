# Extension Directions and Unifying Structure

One-line description: a map of which advanced type and effect features belong to the same extension direction, which do not, and what the smallest common core looks like.

## Table of Contents

1. Purpose
2. Central Thesis
3. The Main Extension Lanes
4. Where Each Feature Sits
5. Overlooked but Important Adjacent Concepts
6. What Can Be Unified by a Small Core
7. What Cannot Be Unified Cheaply
8. Minimal Vocabulary Proposal
9. Recommended Roadmap for Gomputation
10. Key References

## 1. Purpose

The project is not trying to collect advanced type-system features as a checklist. The real question is:

```text
Which features are instances of the same underlying extension direction?
Which features require a fundamentally different checker architecture?
```

This note organizes the previously researched topics by extension direction rather than by language-feature name.

## 2. Central Thesis

Most of the features already discussed do not form a flat list. They cluster around a few extension lanes:

1. index-bearing types
2. richer abstraction over terms, types, and constructors
3. stronger notions of type equality
4. stronger notions of logical validity
5. richer control over resource usage and effects

The key design move for Gomputation is to identify the first lane as the main one:

```text
ADT
-> indexed types
-> row-indexed computations
-> promoted finite states
-> maybe GADTs
```

That lane matches the current draft far better than the alternatives.

## 3. The Main Extension Lanes

### 3.1 Index Lane

This lane strengthens the ability of a type constructor to carry static state or protocol information.

Progression:

```text
parametric types
-> phantom/indexed types
-> row-indexed types
-> promoted finite indices
-> constructor-refined indices
-> value-dependent indices
```

Examples:

- `List a`
- `DB s`
- `Comp pre post a`
- `DB Opened`
- GADT-indexed terms
- dependent vectors `Vect n a`

This is the cleanest lane for Gomputation because the current draft already lives in it.

### 3.2 Abstraction Lane

This lane strengthens what can be quantified over and abstracted over.

Progression:

```text
rank-1 polymorphism
-> row polymorphism
-> higher-rank polymorphism
-> higher-kinded polymorphism
-> kind polymorphism
-> dependent abstraction
```

This lane matters when the language wants reusable libraries and generic interfaces. It is not the same as the index lane, but it often interacts with it.

### 3.3 Type-Equality Lane

This lane strengthens the notion of when two types are considered equal.

Progression:

```text
syntactic equality
-> normalization
-> row normalization
-> local equality refinement by pattern match
-> type-level reduction
-> definitional equality with evaluation
```

The important threshold is that `Type Families` and dependent types both push the checker from "compare syntax with some normalization" toward "evaluate type expressions to decide equality".

### 3.4 Logic Lane

This lane strengthens what types are allowed to claim.

Progression:

```text
plain membership
-> finite state invariants
-> constructor-indexed invariants
-> first-order logical predicates
-> proof terms
-> propositions-as-types
```

This is where refinement types and dependent types live.

### 3.5 Resource-and-Effect Lane

This lane is easy to miss if one focuses only on types. It governs how computations interact with state, capabilities, and control flow.

Progression:

```text
plain effects
-> indexed effects
-> row-polymorphic effects
-> algebraic effects
-> handlers
-> effect instances / named handlers / control abstractions
```

Gomputation currently sits at:

```text
indexed effects + capability rows
```

That is an important fact. It means the language already has a strong effect story without needing handlers yet.

### 3.6 Usage Discipline Lane

Another easily overlooked lane is usage tracking.

Progression:

```text
unrestricted use
-> ad hoc protocol discipline
-> affine use
-> linear use
-> graded/modal usage
```

This lane matters if capability duplication becomes a soundness issue. It is adjacent to typestate, but not identical to it.

## 4. Where Each Feature Sits

This section places concrete features on the lanes.

### 4.1 ADT

- Index lane: foundation only
- Type-equality lane: minimal impact
- Logic lane: plain structural invariants

ADT is basic structure, not a major lane jump.

### 4.2 GADT

- Index lane: constructor-refined indices
- Type-equality lane: local equality refinement under pattern matching
- Logic lane: stronger finite invariants

GADTs are an extension of the same general direction as indexed types, not a completely different paradigm.

### 4.3 DataKinds

- Index lane: promoted finite indices
- Abstraction lane: kind structure becomes more explicit

DataKinds mostly strengthens the index lane in a disciplined way.

### 4.4 Higher-Kinded Types

- Abstraction lane: constructor-level abstraction

HKT is mostly about abstraction power, not stronger invariants.

### 4.5 Type Families

- Type-equality lane: type-level reduction
- Abstraction lane: often paired with interface design

Type families are less about "having indices" and more about "computing with indices".

### 4.6 Refinement Types

- Logic lane: first-order predicates
- Type-equality lane: often solver-mediated implication or checking

Refinement types are not just more indexing. They add a logic.

### 4.7 Dependent Types

- Index lane: value-dependent indices
- Abstraction lane: dependent abstraction
- Type-equality lane: definitional equality by evaluation
- Logic lane: proofs and programs align

Dependent types sit at the far end of multiple lanes simultaneously.

### 4.8 Row Polymorphism

- Index lane: extensible capability/index environments
- Abstraction lane: row-level quantification
- Type-equality lane: row normalization and unification

This is central to Gomputation, not an optional side topic.

### 4.9 Indexed Effects

- Resource-and-effect lane: indexed sequencing
- Index lane: state-bearing computation type

This is already the main semantic abstraction of the draft.

## 5. Overlooked but Important Adjacent Concepts

Several concepts are easy to omit if one only lists the headline features.

### 5.1 Existential Types

Existentials often appear together with GADTs and module-like packaging.

They matter when:

- a constructor hides an internal type
- a capability packages state or implementation details
- elaboration wants to erase a more precise internal representation

They do not define a new lane, but they strongly affect pattern-match typing and encapsulation.

### 5.2 Constraints, Traits, and Type Classes

Constraint systems form another abstraction layer:

```text
plain polymorphism
-> qualified polymorphism
-> quantified constraints
```

These features often create the demand for HKT and associated types. They are not necessary for the core Gomputation design, but they change the value proposition of HKT and type families dramatically.

### 5.3 Subtyping

Subtyping is a separate design axis.

It is often confused with rows because both can express extensibility. But for Gomputation:

- rows provide explicit capability structure
- subtyping would add implicit coercive structure

This tends to make inference and error reporting worse. It is better treated as a separate, probably unwanted, lane.

### 5.4 Linearity and Affinity

If capability duplication threatens soundness, typestate alone may be insufficient. Linear or affine usage is the principled extension.

This is especially relevant if a capability should be:

- single-consumer
- non-duplicable
- guaranteed to be closed exactly once

This concept was only touched indirectly in earlier notes and should remain visible as a future pressure point.

### 5.5 Algebraic Effects and Handlers

Handlers are a real extension direction beyond indexed effects.

They matter if the language wants:

- user-defined control effects
- resumable operations
- effect reinterpretation
- library-defined control abstractions

They are not required for the current host-capability model. In fact, adding handlers too early may blur the clean host-defined authority model.

### 5.6 Modules and Existential Packaging

If the language ever grows a module or signature system, many features move:

- HKT becomes more valuable
- associated types become more valuable
- existential packaging becomes more important

That is why some features feel premature now: their natural ecosystem is not present yet.

## 6. What Can Be Unified by a Small Core

A surprisingly large cluster of features can be unified by the idea of indexed algebraic types plus explicit kinds.

This family includes:

1. ADTs
2. indexed types
3. `Comp pre post a`
4. capability rows
5. promoted finite state indices
6. a significant subset of GADT use cases

The common structure is:

```text
F : K1 -> ... -> Kn -> Type
```

where indices may come from:

- ordinary type parameters
- row parameters
- finite promoted state kinds

Examples:

```text
Option : Type -> Type
DB     : State -> Type
Comp   : Row -> Row -> Type -> Type
Vect   : Nat -> Type -> Type
```

The design insight is that `Comp`, `DB`, and later promoted-state datatypes are all instances of the same indexed-constructor shape.

## 7. What Cannot Be Unified Cheaply

Some features are not just "more of the same". They require a different checker responsibility.

### 7.1 Type Families

These require type-level reduction and therefore a richer theory of type equality.

### 7.2 Refinement Types

These require logical predicates, implication, and usually solver support.

### 7.3 Full Dependent Types

These require evaluation in types and a much stronger normalization story.

### 7.4 Linear Types

These require usage tracking in typing judgments, not just richer type constructors.

### 7.5 Effect Handlers

These require a richer operational and typing model of effects than simple host-provided indexed primitives.

These features are all valid research directions, but they should not be mistaken for cheap extensions of the same core.

## 8. Minimal Vocabulary Proposal

If the project wants a core that can absorb the "same-family" features later, the vocabulary should be kept very small and explicit.

Suggested core:

```text
Kinds   ::= Type | Row | UserKind
Types   ::= a
         | T -> T
         | forall a. T
         | F T1 ... Tn
         | Comp R R T
Rows    ::= {} | { l : T | r } | ...
Terms   ::= x | \x -> e | e1 e2 | ...
Decls   ::= value | type annotation | data declaration | primitive declaration
```

Principles:

1. `Comp` is just a distinguished indexed type constructor in the core.
2. Rows are a dedicated kind because capability environments are central.
3. User-defined ADTs provide the source of future promoted finite indices.
4. Type equality should remain syntax-directed plus row normalization only.
5. Type-level reduction, solver-backed reasoning, and usage tracking are not in the initial core.

## 9. Recommended Roadmap for Gomputation

If the project prioritizes a coherent minimal language, the progression should stay on the main lane as long as possible.

Recommended order:

1. ADTs
2. pattern matching
3. row-polymorphic indexed `Comp`
4. bidirectional typing
5. explicit kinding for `Type`, `Row`, and user-declared datatype kinds
6. maybe DataKinds
7. maybe GADTs

Only after that should the project reconsider:

1. HKT, if generic constructor-level abstractions become central
2. associated types or closed families, if traits/modules appear
3. refinement types, if specific predicate checking becomes essential
4. linearity, if capability duplication becomes unsound
5. handlers, if user-defined control effects become a requirement

## 10. Key References

1. [Indexed Effects, Typestate, and Capabilities](./indexed-effects-typestate-capabilities.md)
2. [Row Polymorphism and Effect Rows](./row-polymorphism-effect-rows.md)
3. [Higher-Rank Polymorphism and Typechecking Strategy](./higher-rank-polymorphism-and-typechecking.md)
4. [Higher-Kinded Types: Practicality and Alternatives](./higher-kinded-types-practicality.md)
5. [DataKinds and Type-Level Promotion](./datakinds-and-promotion.md)
6. [Type Families: Practicality and Alternatives](./type-families-practicality.md)
7. [Refinement Types: Practicality and Constraints](./refinement-types-practicality.md)
8. [ADT and GADT Design Tradeoffs](./adt-gadt-design-tradeoffs.md)
9. [Dependent Types: Practicality and Boundaries](./dependent-types-practicality.md)
10. GHC User's Guide, existential quantification. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/existential_quantification.html
11. GHC User's Guide, quantified constraints. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/quantified_constraints.html
12. GHC User's Guide, linear types overview. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/linear_types.html
13. Eff language. https://www.eff-lang.org/
14. Effekt language. https://effekt-lang.org/
15. Flix effects and handlers. https://doc.flix.dev/effects-and-handlers.html

## Relevance to Gomputation

The main unifying abstraction for this project is not "advanced types" in general. It is:

```text
indexed algebraic structure + row-indexed computations + restrained equality
```

That is the smallest direction that explains the current draft and still leaves room for the right later extensions.

## Addendum: Broader Catalog

For a broader lane-by-lane catalog including rank-n polymorphism, impredicativity, singleton techniques, union and intersection types, occurrence typing, session types, ownership, regions, graded effects, and coeffects, see [Extended Catalog of Advanced Type-System Directions](./extended-catalog-of-advanced-type-system-directions.md).
