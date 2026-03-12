# ADT and GADT Design Tradeoffs

One-line description: when ordinary algebraic data types are enough, when GADTs are worth it, and how they interact with the current draft.

## Table of Contents

1. Short Answer
2. ADTs
3. GADTs
4. What GADTs Would Buy Here
5. What GADTs Cost
6. Recommendation for Gomputation
7. Key References

## 1. Short Answer

Ordinary ADTs are mandatory and low-risk. GADTs are practical, but should be added only after the language's typing judgments are more mature.

For Gomputation:

- ADTs should be in the first usable language
- GADTs are a plausible later feature
- GADTs are more justified than type families or full dependent types if the goal is stronger typed syntax trees, protocol witnesses, or intrinsically typed representations

## 2. ADTs

ADTs are the baseline:

```text
data DBState = Opened | Closed
data Option a = None | Some a
```

They are easy to specify, easy to evaluate, and indispensable for user programs.

The current draft already assumes them informally.

## 3. GADTs

GADTs let constructors return more specific result types than ordinary ADTs allow.

In GHC's formulation, a datatype is ordinary when every constructor result has the form:

```text
T a1 ... an
```

with distinct type variables. It is a GADT when constructors can refine the result type.

Classic example:

```text
data Term a where
  LitInt  : Int -> Term Int
  LitBool : Bool -> Term Bool
  If      : Term Bool -> Term a -> Term a -> Term a
```

Pattern matching on `Term a` reveals information about `a`.

## 4. What GADTs Would Buy Here

### 4.1 Typed syntax trees

If Gomputation eventually wants an intrinsically typed core AST, GADTs are one of the most direct encodings.

### 4.2 Proof-carrying witnesses

GADTs can encode finite evidence such as:

- a capability is in a given state
- a parser produced a value of a specific form
- a surface form elaborates to a well-typed core node

### 4.3 Better API precision without full dependent types

Many use cases that initially sound "dependent" are actually handled well by GADTs plus promoted indices.

## 5. What GADTs Cost

### 5.1 Pattern matching must refine types

This means the typechecker needs local reasoning during case analysis. Bidirectional typing becomes more valuable once GADTs enter the picture.

### 5.2 Type inference becomes less ML-like

GADTs are practical, but they usually require more annotations, especially around functions that pattern match on GADT values.

### 5.3 Existentials and equality evidence appear

Even if the surface syntax hides these details, the specification must account for them eventually.

## 6. Recommendation for Gomputation

Use this order:

1. ADTs first
2. pattern matching and exhaustiveness
3. bidirectional typing stabilized
4. then GADTs if typed ASTs or witness-based APIs become important

If the question is "which stronger type feature should come earliest after ordinary ADTs?", GADTs are one of the better answers.

## 7. Key References

1. GHC User's Guide, GADTs. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/gadt.html
2. GHC User's Guide, GADT syntax. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/gadt_syntax.html
3. Xi and Pfenning, "Dependent Types in Practical Programming". https://doi.org/10.1145/604131.604149

## Relevance to Gomputation

ADTs are part of the minimal language. GADTs are a strong candidate for the first "serious" expressiveness upgrade after the core typechecker settles, especially if the implementation adopts a typed core representation.
