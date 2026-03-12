# Domain Research for Gomputation

Domain knowledge collected to support the draft specification in [../spec.draft.v0_1.md](../spec.draft.v0_1.md).

## Table of Contents

1. [Indexed Effects, Typestate, and Capabilities](./indexed-effects-typestate-capabilities.md)
2. [Row Polymorphism and Effect Rows](./row-polymorphism-effect-rows.md)
3. [Higher-Rank Polymorphism and Typechecking Strategy](./higher-rank-polymorphism-and-typechecking.md)
4. [Higher-Kinded Types: Practicality and Alternatives](./higher-kinded-types-practicality.md)
5. [DataKinds and Type-Level Promotion](./datakinds-and-promotion.md)
6. [Type Families: Practicality and Alternatives](./type-families-practicality.md)
7. [Refinement Types: Practicality and Constraints](./refinement-types-practicality.md)
8. [ADT and GADT Design Tradeoffs](./adt-gadt-design-tradeoffs.md)
9. [Dependent Types: Practicality and Boundaries](./dependent-types-practicality.md)
10. [Extension Directions and Unifying Structure](./extension-directions-and-unifying-structure.md)
11. [Minimal Vocabulary Under Current Consideration](./minimal-vocabulary-under-current-consideration.md)
12. [Extended Catalog of Advanced Type-System Directions](./extended-catalog-of-advanced-type-system-directions.md)
13. [Symbolic Logic: Foundations and Branches](./symbolic-logic-foundations-and-branches.md)
14. [Lambda Calculus: Foundations and Extensions](./lambda-calculus-foundations-and-extensions.md)
15. [Embedded Language Design in Go](./embedded-language-design-go.md)
16. [Call-By-Push-Value and the Value/Computation Metalanguage](./cbpv-and-value-computation-metalanguage.md)
17. [Typed Embedded Language Host Boundary Design Patterns](./host-boundary-design-patterns.md)
18. [Indexed, Parameterized, and Graded Monads](./indexed-parameterized-graded-monads.md)
19. [Row Unification Algorithms](./row-unification-algorithms.md)
20. [Bidirectional Typing with Rows and Indexed Types](./bidirectional-typing-with-rows-and-indexed-types.md)
21. [Non-Linear Effect Composition](./non-linear-effect-composition.md)
22. [Pattern Matching: Grammar, Compilation, and Exhaustiveness](./pattern-matching-and-exhaustiveness.md)
23. [Expression Grammar, Parsing Strategy, and Concrete Syntax Design](./expression-grammar-and-parsing.md)
24. [Evaluation Semantics](./evaluation-semantics.md)
25. [Type Checker Implementation Architecture](./type-checker-architecture.md)
26. [Type Error Reporting and Diagnostics](./type-error-reporting.md)
27. [Recursion, Fixed Points, and Totality](./recursion-and-totality.md)
28. [Core IR Design and Surface-to-Core Elaboration](./core-ir-and-elaboration.md)

## Scope

These notes focus on the parts of the draft that most affect soundness and implementation cost:

- `Comp pre post a` as the core computation type
- row-typed capability environments
- higher-rank polymorphism and feasible typechecking
- deterministic, capability-controlled embedding in a Go host

## Recommended Reading Order

1. Start with indexed effects and typestate, because that defines the semantic core.
2. Read row polymorphism next, because it determines how capabilities compose.
3. Read higher-rank typechecking before expanding the surface syntax.
4. Read the HKT note before adding kind-level abstraction to the language.
5. Read DataKinds before considering promoted protocol states.
6. Read Type Families before considering type-level computation.
7. Read GADT and dependent-type notes before strengthening invariants beyond ordinary ADTs.
8. Read refinement types before committing to SMT-backed checking.
9. Read the extension-directions note when deciding which features belong to the same abstraction family.
10. Read the minimal-vocabulary note when deciding what the core language should name explicitly.
11. Read the extended-catalog note when checking whether a known type-system feature fits an existing lane or a new one.
12. Read the symbolic-logic and lambda-calculus notes when grounding the spec in proof theory and computation theory.
13. Use the Go embedding notes when translating the theory into an implementation plan.
14. Read the CBPV note when grounding the value/computation split in its formal foundations and understanding how `Computation pre post a` relates to the adjunction F -| U.
15. Read the host-boundary patterns note when designing the Go API for host function registration, value conversion, capability environments, error handling, and security.
16. Read the indexed/parameterized/graded monads note to understand `Computation pre post a` as an Atkey parameterized monad and its formal limitations.
17. Read the row unification algorithms note for the concrete unification algorithm, pseudocode, and Go implementation strategy.
18. Read the bidirectional typing note for the type inference strategy, annotation requirements, and how checking mode propagates through `bind` chains.
19. Read the non-linear effect composition note for the branching/failure/early-return pressure points and the recommended v0 solution (equal post-states + total-at-boundary + bracket).
20. Read the pattern matching note for the formal pattern grammar, exhaustiveness and redundancy algorithms, pattern match compilation to decision trees, and how case expressions interact with the value/computation boundary.
21. Read the expression grammar and parsing note for the complete EBNF grammar, Pratt parsing algorithm, token design, precedence table, and Go implementation architecture for the lexer and parser.
22. Read the type checker architecture note for the concrete Go implementation plan: checker state, ordered context, unification engine, bidirectional checking functions, kind checking, elaboration, error handling, testing strategy, and performance guidance.

## Suggested Next Spec Sections

1. Capability declarations and host primitive registration
2. Row equality, row extension, and row unification
3. Type inference boundaries and annotation rules
4. Operational semantics for deterministic evaluation
5. Concrete syntax for computation expressions
