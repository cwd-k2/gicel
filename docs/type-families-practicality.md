# Type Families: Practicality and Alternatives

One-line description: whether type-level functions are worth adding to Gomputation, and what problems they actually solve.

## Table of Contents

1. Short Answer
2. What Type Families Are
3. Where They Could Help
4. Why They Are Expensive Here
5. Alternatives
6. Recommendation for Gomputation
7. Key References

## 1. Short Answer

Type families are powerful, but they are not an early-stage feature for Gomputation.

They become useful when the language needs:

- non-trivial type-level computation
- associated result types for interfaces
- reusable relations between indices

The current draft does not yet need that power.

## 2. What Type Families Are

GHC describes type families as indexed type and data families, and as named functions on types. In practice, they are the standard way to express type-level computation in a non-dependent language.

Examples in the style of existing languages:

```text
type family Elem c
type family Next s
```

with equations like:

```text
Next Opened = Closed
Next Closed = Closed
```

There are at least two practically distinct forms:

1. open families, where equations can be added separately
2. closed families, where all equations are declared together and reduced in order

For Gomputation, only closed families are even remotely reasonable at first. Open families complicate coherence and reasoning too early.

## 3. Where They Could Help

### 3.1 Protocol transition functions

Instead of writing every state transition by hand, one could express a relation at the type level:

```text
Advance : State -> State
```

This is appealing if there are many related states.

### 3.2 Associated result types

If the language later gains traits or interfaces, type families can express associated output shapes:

```text
type Elem c
```

This is one of the clearest practical uses.

### 3.3 Type-level normalization

In richer indexed APIs, type families can simplify types that would otherwise stay syntactic. This is attractive, but it also means the checker must reason about reduction, compatibility, and termination.

## 4. Why They Are Expensive Here

### 4.1 They introduce type-level evaluation

Once type families exist, kinds and types are no longer enough. The checker must also know how type-level expressions reduce.

That means specifying:

- reduction strategy
- equation selection
- apartness or compatibility conditions
- termination restrictions

GHC's user guide spends a great deal of machinery on these points for a reason.

### 4.2 They complicate unification

Rows and indexed `Comp` already require non-trivial unification. Type family applications create stuck forms:

```text
F a
```

that may or may not reduce later. This makes type equality less syntax-directed.

### 4.3 They are most useful once other abstractions exist

The biggest wins appear with:

- traits or type classes
- generic libraries
- richer indexed datatypes

Those are not yet part of the draft.

## 5. Alternatives

### 5.1 Closed ADTs plus ordinary pattern matching

If the goal is to express protocol logic, most of it can stay at the term level while static transitions are written directly in primitive signatures.

### 5.2 GADTs

Some invariants that might tempt type families can instead be expressed by GADT constructor result types, often with better locality.

### 5.3 Associated types later, not general families now

If the real future need is interface-local associated output types, it may be better to design for associated types only when traits exist, rather than adding general top-level type families now.

## 6. Recommendation for Gomputation

Do not add type families to the first version.

If the design grows in this order:

1. ADTs
2. promoted protocol-state kinds
3. GADTs where needed
4. traits or interfaces

then the real need for associated types or closed families will become clearer. Until then, type families are speculative power.

## 7. Key References

1. GHC User's Guide, `TypeFamilies`. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/type_families.html
2. Schrijvers et al., "Type Checking with Open Type Functions", ICFP 2008. https://doi.org/10.1145/1411204.1411215
3. Chakravarty et al., "Associated Type Synonyms". https://doi.org/10.1145/1086365.1086397

## Relevance to Gomputation

Type families solve real problems, but not the problems currently blocking this project. For now they would mostly increase the complexity of type equality before there is enough library structure to justify them.
