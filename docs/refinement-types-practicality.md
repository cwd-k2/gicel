# Refinement Types: Practicality and Constraints

One-line description: how SMT-backed refinement types compare to Gomputation's existing indexed-effect direction.

## Table of Contents

1. Short Answer
2. What Refinement Types Add
3. Where They Could Help
4. Practical Costs
5. Lessons from Existing Systems
6. Recommendation for Gomputation
7. Key References

## 1. Short Answer

Refinement types are practical in industry-facing tools, but they are a different project shape from Gomputation's current draft.

They are best when you want:

- lightweight verification of numeric, collection, or protocol predicates
- SMT-automated checking
- richer invariants than ordinary ML-style types can express

They are not a small extension to the current design. They add a logic, a verification condition generator, and usually an SMT solver dependency.

## 2. What Refinement Types Add

A refinement type is a base type plus a logical predicate, for example:

```text
{v : Int | v >= 0}
```

or:

```text
{xs : List a | len xs > 0}
```

Systems like Liquid Haskell check such properties by generating SMT obligations. F7 and F* show that refinement-style checking can scale beyond toy examples, including security-oriented APIs and verified protocols.

## 3. Where They Could Help

### 3.1 Numeric and collection safety

Refinements are excellent for:

- non-zero divisors
- array bounds
- list non-emptiness
- resource counters

These are areas where ADTs and GADTs are often awkward.

### 3.2 Protocol preconditions stronger than finite typestate

Your current design expresses finite protocol state changes well:

```text
Closed -> Opened
```

Refinements help when the condition is not just a finite state, but a predicate:

- retry count is less than max retries
- transaction amount is positive
- query parameter set is validated

### 3.3 Host capability contracts

In principle, refinement types could express facts about capabilities:

```text
{db : DB Opened | db.poolSize > 0}
```

In practice, that is only attractive if the host model and logic are tightly integrated.

## 4. Practical Costs

### 4.1 You need a logic fragment

The language must decide:

- what predicates are allowed
- what theories are supported
- what is decidable automatically

This is a major design commitment.

### 4.2 You need solver integration

Practical refinement systems usually rely on SMT solvers such as Z3. That affects:

- implementation architecture
- portability
- performance
- error reporting

This is a much larger dependency surface than the current draft implies.

### 4.3 Soundness and usability trade off

Recent refinement-type work emphasizes both practicality and soundness, precisely because industrial refinement tools often make pragmatic compromises. This is not a criticism; it is a consequence of trying to automate richer reasoning at scale.

### 4.4 The mental model changes

The current Gomputation draft is close to "typed functional language with indexed computations". Refinement types would shift it toward "lightweight verification language". That may or may not fit the product goal.

## 5. Lessons from Existing Systems

### 5.1 Liquid Haskell

Liquid Haskell is a strong example of practical refinement typing retrofitted onto an existing language. It demonstrates that refinement types are useful when you want automated checking of rich, mostly first-order properties without requiring full theorem proving.

### 5.2 F7 and F*

F7 and F* show that refinement-like and dependent reasoning can verify security properties and protocol implementations, but they also show how much infrastructure is involved once you go beyond lightweight checks.

## 6. Recommendation for Gomputation

Do not add general refinement types early.

A reasonable future path is:

1. build the core indexed-effect language first
2. identify concrete predicates that ordinary types cannot express
3. if those are mostly arithmetic and collection invariants, consider a restricted refinement layer later

For the current project, indexed rows plus typestate will likely deliver more value per unit of complexity.

## 7. Key References

1. Liquid Haskell specifications. https://ucsd-progsys.github.io/liquidhaskell/specifications/
2. F7 overview. https://www.microsoft.com/en-us/research/project/f7-refinement-types-for-f/
3. F* tutorial. https://fstar-lang.org/tutorial/old/tutorial.html
4. Vazou, "Practicality and Soundness of Refinement Types". https://memento.epfl.ch/event/practicality-and-soundness-of-refinement-types/

## Relevance to Gomputation

Refinement types are practical, but they solve a different class of problems than the ones your draft already isolates well. They are best treated as a possible later verification layer, not as the next core feature.
