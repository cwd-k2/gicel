# Symbolic Logic: Foundations and Branches

One-line description: a compact research map of symbolic logic as it matters for programming languages, type systems, proof theory, and computation.

## Table of Contents

1. Scope
2. Why Symbolic Logic Matters Here
3. Core Syntactic and Semantic Layers
4. Main Proof Systems
5. Classical vs Intuitionistic Logic
6. First-Order, Higher-Order, and Type-Theoretic Logic
7. Structural Proof Theory
8. Major Modern Branches Relevant to Type Systems
9. Semantics and Meta-Theory
10. Logic and Computation
11. Relevance to Gomputation
12. Key References

## 1. Scope

This note is not a complete history of symbolic logic. It focuses on the parts most relevant to the design of typed languages and effect systems:

- syntax and inference
- semantics and validity
- proof systems and normalization
- structural rules
- constructive vs classical reasoning
- logical systems that later influenced type theory and programming languages

## 2. Why Symbolic Logic Matters Here

The project is designing a typed embedded language. That immediately places it inside a long line of ideas from symbolic logic:

- propositions as formal expressions
- inference as rule-governed derivation
- proof normalization
- semantics as interpretation in structures
- constructive reasoning as computation

Once the language grows types, judgments, effects, and capability invariants, it is no longer enough to think only in terms of parser and evaluator design. The specification begins to inherit ideas from proof theory and model theory, whether stated explicitly or not.

## 3. Core Syntactic and Semantic Layers

### 3.1 Syntax

A formal logic begins with:

- symbols
- formation rules
- formulas or judgments

Typical layers include:

```text
terms
formulas
contexts
derivations
```

This pattern later reappears in type systems as:

```text
terms
types
contexts
typing derivations
```

### 3.2 Semantics

Classically, semantics asks what formulas mean and when they are true in a structure.

Key notions:

- interpretation
- model
- satisfaction
- validity
- consequence

This gives the semantic counterpart to derivability. One studies whether:

```text
Gamma |- phi
```

matches:

```text
Gamma |= phi
```

### 3.3 Meta-Theory

Logic quickly becomes a study of systems about systems.

Standard meta-properties include:

- soundness
- completeness
- consistency
- decidability
- compactness
- cut elimination
- normalization

Programming-language type systems inherit many of the same questions.

## 4. Main Proof Systems

### 4.1 Hilbert Systems

Hilbert systems use small sets of axioms plus a few inference rules.

They are elegant for meta-theory, but often poor for computational intuition.

### 4.2 Natural Deduction

Gentzen's natural deduction organizes proofs around introduction and elimination rules for each connective.

This is one of the most important systems for programming-language theory because it aligns closely with typed lambda terms:

- introduction resembles constructor formation
- elimination resembles use or pattern-directed consumption

This is one root of the Curry-Howard correspondence.

### 4.3 Sequent Calculus

Gentzen's sequent calculus makes structural rules explicit and studies derivations of sequents:

```text
Gamma => Delta
```

This is crucial for:

- cut elimination
- structural proof theory
- substructural logics such as linear logic

### 4.4 Proof Nets and Related Graphical Systems

In linear logic and related systems, graphical proof formalisms expose proof structure more directly and often reduce bureaucratic redundancy in sequent-style derivations.

## 5. Classical vs Intuitionistic Logic

### 5.1 Classical Logic

Classical logic validates principles such as excluded middle and double-negation elimination.

It is the dominant metalanguage of ordinary mathematics.

### 5.2 Intuitionistic Logic

Intuitionistic logic treats proofs more constructively. A statement is justified by evidence, not merely by non-refutation.

This shift matters enormously for programming languages because constructive proofs behave like programs.

### 5.3 Why the Distinction Matters for Type Systems

Many typed calculi and proof assistants follow intuitionistic rather than classical logic because:

- introduction and elimination rules become computationally meaningful
- proof normalization aligns with evaluation
- existence proofs require witnesses

This is one of the foundational reasons type theory became a natural home for computation.

## 6. First-Order, Higher-Order, and Type-Theoretic Logic

### 6.1 Propositional Logic

The simplest layer studies formulas built from atoms with connectives such as:

- conjunction
- disjunction
- implication
- negation

### 6.2 First-Order Logic

First-order logic adds:

- variables
- function symbols
- predicate symbols
- quantifiers over individuals

This is the classical foundational system of much of twentieth-century logic.

### 6.3 Higher-Order Logic

Higher-order logic allows quantification over predicates, functions, or sets, depending on formulation.

It is more expressive, but proof theory and semantics become correspondingly richer.

### 6.4 Type-Theoretic Logic

Type theory reorganizes logic around typed terms and judgments rather than starting from untyped formulas alone.

This is where the connection to programming languages becomes especially strong.

Important milestones include:

- Church's simple theory of types
- Martin-Lof's intuitionistic type theory
- modern proof assistants built on richer dependent type theories

## 7. Structural Proof Theory

Structural proof theory studies not only which formulas are derivable, but how proofs are shaped.

### 7.1 Structural Rules

The classic structural rules are:

- weakening
- contraction
- exchange
- cut

Different logics modify or restrict these rules.

### 7.2 Cut Elimination

Cut elimination is one of the central results of proof theory. It shows that proofs can often be normalized into a form that uses only subformula-relevant reasoning.

For programming languages, this matters because cut elimination is closely related to:

- normalization
- substitution
- compositional evaluation

### 7.3 Substructural Logics

By varying structural rules, one gets systems such as:

- linear logic
- affine logic
- relevant logic

These are directly relevant to resource-sensitive typing disciplines.

## 8. Major Modern Branches Relevant to Type Systems

### 8.1 Intuitionistic Type Theory

Martin-Lof's intuitionistic type theory is a major turning point. It treats propositions as types and proofs as inhabitants, while also providing a constructive foundation for mathematics.

This is one of the clearest bridges from proof theory to programming language theory.

### 8.2 Linear Logic

Girard's linear logic refines structural proof theory by making resource use explicit.

This has had enormous downstream influence on:

- linear and affine type systems
- session types
- capability and resource reasoning
- effect-sensitive semantics

### 8.3 Modal and Temporal Logics

Modal logics enrich reasoning with notions such as:

- necessity
- possibility
- time
- staged or contextual access

These influence staged computation, contextual typing, and temporal protocol reasoning.

### 8.4 Categorical and Semantic Logic

Category-theoretic semantics provides models for many logical systems and typed calculi. This is important when moving from syntax to compositional semantics, though it is likely beyond the first implementation needs of Gomputation.

## 9. Semantics and Meta-Theory

### 9.1 Soundness and Completeness

Every proof-oriented language design eventually faces versions of:

- does the proof system derive only semantically valid judgments?
- does it derive all semantically intended judgments?

Type systems often focus more on soundness than completeness, but the analogy remains useful.

### 9.2 Normalization

Normalization says derivations or terms reduce to normal form. In logic this is a proof-theoretic property; in programming languages it often corresponds to evaluation behavior or canonicity.

### 9.3 Decidability

A formal system may be expressive enough to become undecidable. This is the pressure point when moving from simple polymorphism toward type-level computation, refinement reasoning, or full dependency.

## 10. Logic and Computation

### 10.1 Curry-Howard Perspective

The Curry-Howard correspondence links:

- propositions with types
- proofs with programs
- normalization with evaluation

This is not merely a metaphor. It is one of the core explanatory bridges between logic and typed programming languages.

### 10.2 Judgments as the Real Interface

One of the most important lessons from logic for language design is that judgments matter as much as syntax.

Typical logical judgments:

```text
Gamma |- phi
Gamma |= phi
```

Typical programming-language judgments:

```text
Gamma |- e : A
Gamma |- A == B
Gamma |- c : Comp r1 r2 a
```

This is why the earlier "minimal vocabulary" note emphasized equality, constraint, effect, and usage judgments as core notions.

## 11. Relevance to Gomputation

For Gomputation, symbolic logic contributes at least four foundational ideas:

1. judgments as the primary semantic interface
2. natural-deduction style thinking for introduction and elimination rules
3. structural proof theory for understanding rows, effects, and future usage restrictions
4. constructive logic as the conceptual bridge from proofs to typed computations

The project does not need to become a proof assistant. But if the specification wants to stay coherent as it grows richer types and effects, it should continue to borrow its conceptual scaffolding from proof theory rather than from ad hoc feature accumulation.

## 12. Key References

1. Jean H. Gallier, "Constructive Logics Part I: A Tutorial on Proof Systems and Typed Lambda-Calculi". https://repository.upenn.edu/handle/20.500.14332/7337
2. Per Martin-Lof, *Intuitionistic Type Theory* and later expository material summarized in the SEP entry. https://plato.stanford.edu/entries/type-theory-intuitionistic/
3. Alonzo Church, *A Formulation of the Simple Theory of Types*. https://doi.org/10.2307/2266170
4. Jean-Yves Girard, "Linear Logic". https://doi.org/10.1016/0304-3975(87)90045-4
5. Jean H. Gallier, "Constructive Logics Part II: Linear Logic and Proof Nets". https://repository.upenn.edu/handle/20.500.14332/7377
