# Embedded Language Design in Go

One-line description: practical implementation patterns and comparison points for a deterministic, capability-controlled embedded language hosted by Go.

## Table of Contents

1. Host-Language Requirements
2. Architecture Options
3. Lessons from Existing Embedded Languages
4. Recommended Runtime Shape
5. Determinism Requirements
6. Implementation Plan Implications
7. Key References

## 1. Host-Language Requirements

The draft targets a language that runs inside a Go application as a library. That implies four practical constraints:

1. the host must construct and register capabilities
2. the interpreter must expose predictable APIs for parse, check, and run
3. untrusted programs must not escape the host authority model
4. evaluation cost and error reporting must be operationally clear

## 2. Architecture Options

### 2.1 Minimal typed interpreter

Pipeline:

```text
source -> parser -> typed AST -> evaluator
```

Pros:

- simplest to validate
- easiest to align with the written spec
- best for early-stage language design

Cons:

- runtime performance may be limited

### 2.2 Desugaring to a core calculus

Pipeline:

```text
surface syntax -> elaboration -> small core language -> evaluator
```

Pros:

- better spec structure
- easier to formalize typing and evaluation
- isolates surface syntax churn

Cons:

- more upfront design effort

For this project, this is likely the right medium-term architecture.

### 2.3 Bytecode or compiled backend

Not needed yet. This is premature before the type system and semantics freeze.

## 3. Lessons from Existing Embedded Languages

### 3.1 Starlark

Starlark is a strong reference point for deterministic embedding. Its spec states that execution is deterministic and excludes random numbers, clocks, and unspecified iteration order from the core language.

Relevant lessons:

- deterministic semantics should be explicit, not implied
- host-provided side effects should be narrow and controlled
- configuration and rule languages benefit from a deliberately limited core

### 3.2 CEL

CEL is a strong reference point for embeddable, safe evaluation in Go. Its public positioning emphasizes:

- embedded execution
- host-provided data and functions
- safety through a non-Turing complete core

Relevant lessons:

- parse, check, and evaluate should be separate APIs
- host extension points should be first-class
- clear cost boundaries improve adoptability

Your language is more ambitious than CEL because it includes general functions and indexed effects, but the embedding discipline is similar.

### 3.3 Koka

Koka is not a Go embedding library, but it is a strong reference for:

- effect typing
- effect polymorphism
- separating pure and effectful code

Relevant lesson:

- effect precision pays off only if the surface language and typechecker rules are explicit

## 4. Recommended Runtime Shape

A practical Go API likely wants these layers:

```go
type Module struct { ... }
type CheckedModule struct { ... }
type Value interface { ... }
type Capability interface { ... }
```

and this flow:

```text
Parse(source) -> AST
Check(ast, hostSignatureEnv) -> TypedAST
Run(typedAst, capabilityEnv) -> Value
```

Suggested separation:

- signature environment: static types of host primitives and capabilities
- capability environment: dynamic host values used during execution

This mirrors the distinction between static rows and runtime handles.

## 5. Determinism Requirements

If determinism is a hard goal, the implementation should rule out or specify:

- map iteration order leaks from Go
- time and randomness unless passed as explicit capabilities
- goroutine scheduling as part of user-visible semantics
- reflection-based host access

This should be stated as a host contract as well as a language property.

## 6. Implementation Plan Implications

### 6.1 Build the checker before optimizing the evaluator

The typechecker carries most of the novelty:

- kinding for `Type` and `Row`
- row unification
- bidirectional typing
- indexed computation sequencing

### 6.2 Represent rows canonically

In Go, use a canonical internal representation such as:

- sorted slice of labels plus optional tail
- or map plus normalized comparison form

Do not let source-order equality leak into the typechecker.

### 6.3 Keep host primitives opaque

Do not reify arbitrary Go values directly into the language unless a typed wrapper exists. Otherwise the capability model becomes reflection by another name.

### 6.4 Separate pure values from computation closures

The runtime should distinguish:

- pure values
- suspended computations
- host primitive invocations

That separation will make the evaluator and error messages clearer.

## 7. Key References

1. Starlark specification. https://github.com/google/starlark-go/blob/master/doc/spec.md
2. CEL overview. https://cel.dev/
3. CEL Go implementation. https://github.com/google/cel-go
4. The Koka language book. https://koka-lang.github.io/koka/doc/book.html

## Relevance to Gomputation

The most useful immediate implementation stance is:

1. parse to a surface AST
2. elaborate to a small core
3. typecheck the core with rows and indexed computations
4. evaluate against an explicit capability environment

That keeps the implementation aligned with the spec and avoids importing accidental complexity from general-purpose scripting runtimes.
