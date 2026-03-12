# Dependent Types: Practicality and Boundaries

One-line description: when dependent types are genuinely useful, and why they are probably beyond the current scope of Gomputation.

## Table of Contents

1. Short Answer
2. What Dependent Types Add
3. Practical Strengths
4. Costs and Architectural Consequences
5. Middle Grounds
6. Recommendation for Gomputation
7. Key References

## 1. Short Answer

Dependent types are practical in mature ecosystems like Idris and F*, but they would fundamentally change the nature of this project.

They are not the next incremental step after the current draft.

## 2. What Dependent Types Add

A dependent type is a type that can depend on a value. Idris examples make this concrete with vectors:

```text
Vect : Nat -> Type -> Type
```

and functions such as:

```text
app : Vect n a -> Vect m a -> Vect (n + m) a
```

This is much more than promoted finite states. It allows types to compute from values and functions, subject to the language's totality and normalization rules.

## 3. Practical Strengths

Dependent types shine when you want:

- exact protocol or data invariants in types
- proofs alongside programs
- verified elaboration or compilation pipelines
- theorem-prover-grade guarantees for critical components

For example, a dependently typed core can encode that an expression is well typed by construction.

## 4. Costs and Architectural Consequences

### 4.1 Type checking becomes evaluation-aware

Because types contain computations, checking types requires normalization or at least definitional equality machinery.

### 4.2 Totality and termination become central

Languages like Idris emphasize totality because unchecked nontermination at the type level makes reasoning collapse.

### 4.3 Implementation and UX costs are high

You need:

- normalization or elaboration machinery
- definitional equality
- explicit handling of proofs or erased terms
- much stronger tooling and error reporting

This is not compatible with a "small, embeddable, deterministic typed scripting language" unless the project intentionally pivots toward proof-oriented programming.

## 5. Middle Grounds

There are several useful stopping points before full dependent types:

1. phantom types
2. DataKinds
3. GADTs
4. refinement types
5. restricted associated types or closed families

These often cover the practical invariants people actually need in embedded languages.

## 6. Recommendation for Gomputation

Do not target full dependent types.

If you want stronger invariants, the better progression is:

1. ADTs
2. promoted protocol-state kinds
3. GADTs for witness-rich APIs
4. maybe refinement types for numeric or collection predicates

Full dependent types should enter the conversation only if the project decides that proof-carrying programs or verified compilation are core goals.

## 7. Key References

1. Idris 2 tutorial, dependent types. https://idris2.readthedocs.io/en/latest/tutorial/typesfuns.html
2. Idris language examples. https://www.idris-lang.org/pages/example.html
3. F* overview. https://fstarlang.github.io/
4. F* tutorial. https://fstar-lang.org/tutorial/old/tutorial.html

## Relevance to Gomputation

Dependent types are real and practical, but they optimize for a very different product shape. For this project they are better viewed as an upper bound on expressiveness, not as a roadmap item.
