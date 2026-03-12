# Recursion, Fixed Points, and Totality in Embedded Languages

One-line description: the design space for recursion in Gomputation, covering totality, structural recursion, general recursion with controls, recursion as a capability, interaction with indexed effects, and a concrete recommendation.

## Table of Contents

1. The Unresolved Question
2. The Design Space
3. Total Languages and Configuration Languages
4. Structural Recursion and Termination Checking
5. General Recursion with Controls
6. Recursion as a Capability
7. Recursive Data Types without General Recursion
8. Interaction with Indexed Effects and Typestate
9. Decision Matrix
10. Formal Rules for `fix` and `letrec`
11. Implementation in Go
12. Recommendation for Gomputation
13. Key References

---

## 1. The Unresolved Question

The spec v0.2 does not specify whether general recursion is allowed. The evaluation semantics document notes:

> The current spec does not include general recursion. Without a fixed-point combinator or recursive let-bindings, all well-typed terms in the core calculus terminate.

And later:

> If general recursion is added in a future extension (e.g., recursive let-bindings or a fix-point combinator), termination is no longer guaranteed, and step limits become essential for the termination of the evaluator.

This leaves a fundamental question open: should Gomputation admit general recursion, and if so, under what discipline? The answer shapes the language's expressiveness, its safety guarantees, its implementation complexity, and its fitness for each stated use case. This document analyzes the design space exhaustively and makes a concrete recommendation.

---

## 2. The Design Space

There are five distinct positions on the recursion spectrum, each with different trade-offs.

### 2.1 No recursion at all

Programs are pure expressions over a finite vocabulary. There are no loops, no recursive function definitions, no `fix`, no `letrec`. Every program terminates and can be evaluated in time proportional to its size.

**Examples:** CEL (Google Common Expression Language).

CEL evaluates in linear time, is mutation-free, and is not Turing-complete. It has no loops or recursion, and expressions always complete. This design is appropriate when the language is a predicate or policy language: the programs are small, the inputs are structured data, and the evaluation must be fast and guaranteed to terminate.

**What you give up:** Any form of iteration. Programs cannot process lists of unknown length, traverse trees, or express algorithms that require repetition. Programs are limited to a fixed depth of nesting determined by the syntax.

### 2.2 No general recursion, but with folds over recursive data

Programs cannot define recursive functions, but the language provides recursive data types (lists, trees) with built-in fold/catamorphism eliminators. Every program terminates because the only form of iteration is structural: you can fold over a finite data structure, and the fold is guaranteed to consume the structure in finite steps.

**Examples:** Dhall, CUE.

Dhall is a total programming language. It does not permit general recursion but uses Church encodings (Boehm-Berarducci encodings) to represent recursive data types as their own fold functions. A list is not a data structure with a recursive spine; it is a function that is hard-coded to call its argument a fixed number of times. The general algorithm for translating recursive code to non-recursive code is based on the Boehm-Berarducci encoding.

CUE is intentionally Turing-incomplete and does not permit general recursion. It disallows cyclic references and restricts recursion to maintain its non-Turing-complete status.

**What you give up:** Programs that require iteration patterns not expressible as folds over existing data (e.g., iterating until convergence, general graph algorithms, interactive loops). The fold discipline can be awkward for programs that are conceptually iterative.

### 2.3 Structural recursion with totality checking

Programs may define recursive functions, but the language statically verifies that all recursion is structurally decreasing: every recursive call must be on a syntactically smaller argument, ensuring termination.

**Examples:** Agda, Idris, Coq/Rocq.

Agda's termination checker allows structural recursion, meaning recursive calls must be on a strict subexpression of the argument. Arguments may decrease in lexicographic order. Idris checks totality by verifying that pattern matching is exhaustive (covering) and that recursive calls decrease toward a base case.

**What you give up:** Programs with non-structural recursion patterns (e.g., recursion where the argument decreases by subtraction, mutual recursion through intermediate data structures, general while-loops). The termination checker is necessarily conservative: many terminating programs will be rejected. Implementation complexity is significant.

### 2.4 General recursion with runtime controls

Programs may define arbitrary recursive functions. The language does not attempt to prove termination statically. Instead, runtime mechanisms (step limits, fuel, gas metering, timeouts) bound execution.

**Examples:** Starlark, Lua, blockchain languages (Solidity/EVM, Move).

Starlark permits recursion (optionally, via a flag in the Go implementation) and provides `SetMaxExecutionSteps` to bound computation. The interpreter decrements a counter at each evaluation step and returns an error when the limit is reached.

Ethereum's gas model assigns a cost to each bytecode instruction. Execution halts when gas is exhausted. The accuracy of individual costs matters less than their relative magnitudes.

**What you give up:** Static termination guarantees. A program may diverge, and the only defense is the runtime bound. The bound is a blunt instrument: it does not distinguish between a productive computation that happens to be large and an infinite loop.

### 2.5 General recursion as an explicit effect or capability

General recursion is not built into the language core but is provided by the host as a capability. Programs written without the recursion capability are total. Programs that receive the recursion capability may diverge but are subject to runtime controls.

**Examples:** No mainstream language has taken this exact approach. It is a novel design that fits naturally with Gomputation's capability model.

**What you gain:** The capability model extends naturally. Totality becomes a property derivable from the absence of a capability, not a separate analysis. The host controls whether programs may diverge.

**What you give up:** The design is untested and may introduce ergonomic friction.

---

## 3. Total Languages and Configuration Languages

### 3.1 Dhall

Dhall is a programmable configuration language that guarantees termination. Its core is System F-omega (polymorphic lambda calculus with type operators), which is strongly normalizing.

**No general recursion.** Dhall has no `fix`, no `letrec`, no recursive `let`. Functions cannot refer to themselves.

**Recursive data via Church encoding.** Lists, optionals, and naturals are encoded as their own fold functions. A `List a` is morally `forall r. (a -> r -> r) -> r -> r`. To process a list, you provide a step function and an initial accumulator; the list applies the step function once per element. This encoding guarantees that all operations on lists terminate because the fold is bounded by the (finite) number of elements.

**Built-in fold.** Dhall provides `List/fold : forall (a : Type) -> List a -> forall (r : Type) -> (a -> r -> r) -> r -> r` as a primitive. This is the catamorphism for lists. There is no `List/unfold` or `List/iterate` that could produce unbounded output.

**Natural number fold.** `Natural/fold : Natural -> forall (r : Type) -> (r -> r) -> r -> r` iterates a function `n` times. This is bounded iteration, not general recursion.

**Consequences for expressiveness:**
- Programs can process lists, optionals, and naturals.
- Programs cannot express algorithms that require iteration until convergence.
- Programs cannot express parsers, interpreters, or anything requiring unbounded computation.
- The Boehm-Berarducci encoding is sometimes awkward: building a list requires knowing its length in advance or constructing it from a fixed-size input.

**Consequences for safety:**
- Every Dhall program terminates.
- Type checking terminates.
- Evaluation can be bounded by a polynomial in the input size (for the core language without imports).

### 3.2 Nickel

Nickel is a configuration language that combines gradual typing, contracts, and lazy evaluation. It uses lazy evaluation to enable recursive record definitions (fields can reference other fields in the same record), which is essential for configuration merging.

**Recursion status:** Nickel allows recursive let-bindings (`let rec`). This makes Nickel Turing-complete. However, Nickel's intended use case (configuration) means that programs are expected to produce finite output, and divergence is considered a bug rather than a feature.

**Contracts over totality checking.** Rather than statically checking termination, Nickel relies on runtime contracts for validation. A contract can assert that a value has a certain shape without requiring the language to prove that the computation producing the value terminates.

**Lazy evaluation.** Nickel's laziness means that recursive definitions do not diverge unless a specific field is demanded and its computation loops. This is a weaker guarantee than totality but a pragmatic one for configuration.

**Relevance to Gomputation:** Nickel shows that a configuration-oriented language can survive with general recursion if the evaluation model (laziness, contracts) provides enough practical safety. However, Gomputation's strict evaluation makes this approach less applicable: under strict evaluation, a divergent recursive definition diverges immediately, not only when demanded.

### 3.3 CEL (Common Expression Language)

CEL is a non-Turing-complete expression language designed for evaluating untrusted expressions in performance-critical applications (IAM policies, Kubernetes admission control, security rules).

**No recursion, no loops, no mutation.** CEL is a pure expression language. Evaluation is linear in the size of the expression and the input. There are no user-defined functions, no `let` bindings in the original spec (though extensions add them), and no way to iterate.

**Macros for bounded iteration.** CEL provides macros like `exists`, `all`, `map`, and `filter` that iterate over lists. These are not user-definable and are expanded at parse time. They provide bounded iteration over finite inputs.

**Relevance to Gomputation:** CEL is too restrictive for Gomputation's use cases. Gomputation needs user-defined functions (`\x -> e`), algebraic data types, and the ability to process recursive data. CEL's model is appropriate for predicate evaluation, not for embedded scripting or domain logic.

### 3.4 CUE

CUE is a constraint-based configuration language. It is intentionally Turing-incomplete: not disallowing recursion would make CUE Turing-complete, and the language deliberately restricts it.

**No general recursion, no user-defined functions.** CUE does not have functions in the traditional sense. Computation is expressed through constraint unification: you state properties and relationships, and the evaluator solves for values that satisfy all constraints simultaneously.

**Relevance to Gomputation:** CUE's constraint model is fundamentally different from Gomputation's functional computation model. CUE's approach to totality (no recursion because there are no functions) does not transfer to a lambda-calculus-based language.

### 3.5 Summary: what total configuration languages give up

| Language | Recursion | Iteration | User functions | Turing complete |
|---|---|---|---|---|
| CEL | None | Macros over lists | No | No |
| CUE | None | Comprehensions | No | No |
| Dhall | None | Folds over data | Yes (total) | No |
| Nickel | `let rec` | Lazy recursion | Yes | Yes |
| Starlark | Optional | Loops | Yes | Yes |

For Gomputation's stated use cases:
- **Configuration evaluation:** Dhall-style totality (folds, no general recursion) would be sufficient.
- **Rule evaluation:** CEL-style expression evaluation might suffice for simple rules; Dhall-style folds are needed for rules that process structured data.
- **Domain logic:** Folds are often sufficient, but some algorithms naturally require general recursion.
- **Embedded scripting:** General recursion is typically expected.
- **Protocol-controlled execution:** The recursion question interacts with typestate; see Section 8.

---

## 4. Structural Recursion and Termination Checking

### 4.1 What structural recursion means

A function is structurally recursive if every recursive call is on a syntactic subterm of the original argument. The argument gets "smaller" at each call in a well-founded sense determined by the structure of the data type.

```text
-- Structural recursion: xs is a subterm of (Cons x xs)
length : List a -> Int
length Nil = 0
length (Cons x xs) = 1 + length xs

-- NOT structural recursion: (n - 1) is not a syntactic subterm of n
collatz : Int -> Int
collatz 1 = 0
collatz n = if even n then collatz (n / 2) else collatz (3 * n + 1)
```

### 4.2 Agda's termination checker

Agda checks termination by analyzing the call graph of recursive functions. For each recursive call, it records which arguments decreased, stayed the same, or increased. The checker then looks for a lexicographic ordering on the arguments that decreases at every recursive call.

The algorithm is based on the "size-change principle" (Lee, Jones, and Ben-Amram, 2001): a program terminates for all inputs if every infinite call sequence would cause an infinite descent in a well-founded ordering. Agda's implementation approximates this principle by checking that some argument is structurally smaller in every recursive call, with lexicographic extensions for mutual recursion.

**Limitations:**
- The checker is necessarily conservative (the halting problem is undecidable).
- Functions like `merge sort` require careful structuring to pass the checker.
- Functions that recurse on computed values (not pattern-matched subterms) are rejected.
- The checker does not handle recursion through higher-order functions well without inlining.

### 4.3 Idris's totality checker

Idris checks two properties for a function to be total:

1. **Covering:** Pattern matching is exhaustive. Every possible input pattern is handled.
2. **Terminating:** All chains of recursive calls eventually reach a base case. The checker verifies that at least one argument decreases structurally at each recursive call.

Idris provides escape hatches:
- `assert_total` asserts that an expression terminates (trusted, unchecked).
- `assert_smaller` asserts that one value is structurally smaller than another (helps the checker when it cannot infer the relationship).
- Functions can be marked `partial` to opt out of totality checking entirely.

### 4.4 Sized types

Sized types are an alternative to syntactic structural recursion checking. Instead of analyzing call graphs, the type system tracks the "size" of data as a type-level index.

```text
data List : Size -> Type -> Type where
  Nil  : List 0 a
  Cons : a -> List n a -> List (n + 1) a

-- The type says: input has size (n+1), recursive call has size n
length : List (n + 1) a -> Nat
length (Cons x xs) = 1 + length xs  -- xs : List n a, so recursive call is smaller
```

Size indices are parametric: functions cannot inspect or branch on sizes. Sizes serve only to prove termination and are erased at runtime.

**Advantages over structural checking:**
- More programs are accepted (the ordering need not be syntactic).
- Sized types compose better with higher-order functions.
- The check is local: each function's type contains its termination argument.

**Disadvantages:**
- Type annotations become more complex.
- Size inference is needed to avoid annotation burden.
- Size polymorphism interacts with other polymorphism mechanisms.
- Implementation complexity is considerable.

### 4.5 Cost for Gomputation

Implementing a structural termination checker for Gomputation would require:

1. A call-graph analysis pass after type checking.
2. A size-change analysis or foetus-style termination argument.
3. Handling of mutual recursion.
4. Error messages that explain why a function was rejected.
5. An escape hatch (`assert_total` or similar) for programs the checker cannot verify.

The implementation cost is significant. Agda's termination checker is one of the more complex parts of the implementation. For a language embedded in Go, targeting scripting and configuration use cases, this level of sophistication may not be justified.

Moreover, Gomputation's type system (rank-1 polymorphism, row-indexed computations, ADTs) is considerably simpler than Agda's or Idris's dependent types. The interaction between a structural termination checker and the existing type system would need careful design, particularly around:
- Higher-order functions (does `map f xs` terminate if `f` terminates and `xs` is finite?).
- Functions that return computations (does `\x -> bind (f x) g` terminate?).
- Row-polymorphic functions (the termination argument must be orthogonal to the row structure).

---

## 5. General Recursion with Controls

### 5.1 The `fix` combinator

The standard way to introduce general recursion into a lambda calculus is the fixed-point combinator:

```text
fix : forall a. ((a -> b) -> (a -> b)) -> (a -> b)
```

Operationally, `fix f` computes the fixed point of `f`: the value `x` such that `f x = x`. For functions, this means `fix f = f (fix f)`, which unfolds the recursive definition.

In a strict (call-by-value) language, the naive equation `fix f = f (fix f)` diverges because `fix f` is evaluated before being passed to `f`. The standard CBV fix-point uses an eta-expansion:

```text
fix f = f (\x -> fix f x)
```

This wraps the recursive call in a lambda, deferring it until application. The thunk `\x -> fix f x` is a value and can be passed to `f` without triggering infinite unfolding.

### 5.2 Recursive let-bindings (`letrec`)

An alternative to `fix` is `letrec`, which allows a binding to refer to itself:

```text
letrec f = \x -> ... f ... in body
```

This is equivalent to `let f = fix (\f -> \x -> ... f ...) in body` but is syntactically more natural. In an environment-based evaluator, `letrec` is implemented by creating a closure whose environment contains a back-pointer to the closure itself (a "tied knot").

### 5.3 Step counting

The simplest runtime control for general recursion. The evaluator maintains a counter that is decremented at each evaluation step. When the counter reaches zero, evaluation halts with an error.

```text
eval(expr, env, fuel):
    fuel--
    if fuel <= 0: error("step limit exceeded")
    case expr of ...
```

**Properties:**
- Deterministic: the same program with the same fuel always produces the same result or the same error.
- Composable with existing evaluation: a single integer counter threaded through the evaluator.
- No false positives: programs that terminate within the limit produce correct results.
- Blunt: cannot distinguish productive computation from infinite loops.
- Starlark-go implements exactly this via `Thread.SetMaxExecutionSteps`.

### 5.4 Fuel-based evaluation

A refinement of step counting where the fuel is an explicit parameter in the semantics, often used in verified settings (e.g., Coq's `Fuel` idiom for modeling potentially non-terminating computations in a total language).

```text
data Fuel = More Fuel | Empty

eval : Fuel -> Expr -> Env -> Result (Either Timeout Value)
eval Empty _ _ = Left Timeout
eval (More f) expr env = case expr of
    ...  -- recursive calls use f
```

Fuel-based evaluation makes the termination argument explicit: every recursive call decreases the fuel, so the evaluation function is structurally recursive on the fuel parameter and is itself total.

### 5.5 Gas metering

Used in blockchain languages (Ethereum/Solidity, Sui/Move). Each operation has an associated cost (gas). Execution halts when the gas budget is exhausted.

Gas metering differs from step counting in that different operations may have different costs. An addition might cost 3 gas while a storage write costs 20,000 gas. The relative magnitudes of costs matter more than their absolute values.

**Relevance to Gomputation:** Gas metering could be relevant if Gomputation assigns different costs to different operations (e.g., host primitive calls are more expensive than pure computations). For the initial design, uniform step counting is simpler and sufficient.

### 5.6 Stack depth limits

A complementary mechanism to step counting. Even with step limits, deeply recursive programs can exhaust memory by building a large call stack.

```go
func eval(expr Expr, env *Env, depth int) (Value, error) {
    if depth > maxDepth {
        return nil, fmt.Errorf("stack depth exceeded")
    }
    // recursive calls pass depth+1
}
```

Stack depth limits catch a specific failure mode (stack overflow) that step limits do not directly address, since a program could make many shallow calls without triggering a depth limit, or few deep calls without triggering a step limit.

### 5.7 Timeout via `context.Context`

Go's `context.Context` provides cancellation and deadline support. The evaluator can check `ctx.Done()` periodically:

```go
func eval(ctx context.Context, expr Expr, env *Env) (Value, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    // ... continue evaluation
}
```

**Properties:**
- Non-deterministic: the result depends on wall-clock time, not computation steps.
- Not suitable as the primary termination mechanism for a deterministic language.
- Useful as a defense-in-depth layer alongside step counting.

### 5.8 Koka's divergence effect

Koka takes a hybrid approach. It has a simple termination checker that assigns the `div` (divergence) effect to potentially non-terminating functions. If the checker determines that each recursive call decreases on an inductive data type, the function is marked as total (no `div` effect). Otherwise, `div` is added to the effect type.

```text
fun length(xs : list<a>) : int    -- no div effect, proven terminating
fun collatz(n : int) : div int    -- div effect, may not terminate
```

This approach tracks divergence in the type system without preventing it. The caller knows whether a function may diverge and can handle that information (e.g., by imposing a timeout or declining to call divergent functions in contexts that require totality).

**Relevance to Gomputation:** Koka's approach is interesting but requires a termination checker (to distinguish total from potentially divergent functions) and an effect system that can track `div`. Gomputation's effect system tracks capability state, not effect labels. Adding a `div` label would require extending the effect tracking mechanism.

---

## 6. Recursion as a Capability

### 6.1 The idea

Gomputation's central design principle is that all authority comes from the host. The language cannot perform effects that are not explicitly provided. This principle can be extended to recursion: general recursion is not a built-in feature of the language but a capability that the host may or may not grant.

Without the recursion capability, all programs are total. The type system guarantees termination because there is no way to express an infinite computation. With the recursion capability, programs may express general recursion, subject to the runtime controls (step limits, fuel) that the host configures.

### 6.2 The primitive

The recursion capability can be provided as a host-registered primitive with the following type:

```text
rec : forall a b. ((a -> Computation r r b) -> a -> Computation r r b)
                -> a -> Computation r r b
```

This is a computation-level fixed-point combinator. The type says:

- You provide a "step function" `f` that takes a "self-reference" (a function `a -> Computation r r b`) and an argument `a`, and produces a computation.
- `rec f` returns a function `a -> Computation r r b` that is the fixed point of `f`.
- The pre-state and post-state must be the same (`r` and `r`), meaning recursive computations preserve capability state. See Section 8 for the rationale.

Usage:

```text
-- Factorial, using the recursion capability
factorial :: Int -> Computation r r Int
factorial = rec (\self n ->
    if n <= 0
        then pure 1
        else bind (self (n - 1)) (\m -> pure (n * m))
    )
```

### 6.3 Value-level recursion as a capability

The above `rec` operates at the computation level. A separate capability could provide value-level recursion:

```text
fix : forall a b. ((a -> b) -> a -> b) -> a -> b
```

This is a pure fixed-point combinator. It is more powerful (and more dangerous) because it operates outside the computation layer and is not subject to capability-state threading.

The two capabilities can be provided independently:

- `rec` alone: recursion is permitted only within computations, subject to step limits.
- `fix` alone: recursion is permitted at the value level, also subject to step limits.
- Both: full general recursion.
- Neither: the language is total.

### 6.4 Has any language done this?

No mainstream language treats recursion as a capability in exactly this way. However, the idea has precedents:

**Starlark's recursion flag.** The Go implementation of Starlark has a `-recursion` flag that enables or disables recursive function calls. When disabled, a function that calls itself (directly or indirectly) causes a runtime error. This is a coarse-grained version of "recursion as a capability": the host decides at interpreter creation time whether recursion is allowed.

**Dhall's design.** Dhall achieves totality by not providing `fix` or `letrec`. This is equivalent to "never granting the recursion capability." The built-in folds (`List/fold`, `Natural/fold`) are the only iteration primitives, and they are total.

**Dreyer, Harper, and Crary's type system for well-founded recursion.** This academic work tracks the usage of recursive variables statically, ensuring that recursion is well-founded. The recursive variable is a resource that must be used in a decreasing way. This is conceptually similar to treating recursion as a controlled resource, though the mechanism is static rather than dynamic.

**Koka's `div` effect.** Koka tracks divergence as an effect. A function with the `div` effect may not terminate; a function without it is guaranteed to terminate. This is a type-level version of "recursion as a capability": the type tells you whether a function uses the capability to diverge.

### 6.5 Design for Gomputation

The recursion-as-capability approach fits Gomputation's architecture naturally:

1. **Consistent with the capability model.** All other authority (database access, logging, network I/O) is host-provided. Recursion can follow the same pattern.

2. **Host controls termination.** A host that requires totality (e.g., for configuration evaluation) simply does not grant the recursion capability. A host that permits general computation (e.g., for scripting) grants it with step limits.

3. **Composable with existing features.** The `rec` primitive has a type expressible in the existing type system. No new type-level mechanisms are needed.

4. **Gradual introduction.** The initial language ships without general recursion (total by default). The recursion capability is added as a host-provided primitive when needed.

**Potential concerns:**

1. **Ergonomics.** Writing recursive functions via `rec (\self n -> ...)` is more verbose than `let f n = ... f ... in ...`. A surface-level `letrec` desugaring can mitigate this, but the underlying mechanism remains explicit.

2. **Unfamiliarity.** No mainstream language does this. Users may find it surprising that recursion requires explicit permission.

3. **Value-level recursion.** If `fix` is provided as a value-level capability, step counting must cover value evaluation, not just computation evaluation. The evaluation semantics document already recommends this as an option: "Optionally, each value-level evaluation step also decrements the counter."

4. **Mutual recursion.** The simple `rec` combinator handles self-recursion. Mutual recursion requires either a product-type encoding or a more general letrec-style primitive. For the initial design, self-recursion through `rec` is sufficient.

---

## 7. Recursive Data Types without General Recursion

### 7.1 The distinction

Recursive data types and general recursion are independent features. A language can have:

- Recursive data types **with** general recursion: Haskell, OCaml, most functional languages.
- Recursive data types **without** general recursion: Dhall, Agda, Idris.
- No recursive data types and no general recursion: CEL.

Gomputation's spec v0.2 assumes algebraic data types. The question is whether those ADTs can be recursive (lists, trees) and, if so, how they are eliminated.

### 7.2 Recursive data types with fold eliminators

If the language has recursive data types but no general recursion, the safe elimination form is the fold (catamorphism). For each recursive data type, the language provides a fold that replaces each constructor with a user-provided function:

```text
data List a = Nil | Cons a (List a)

-- The fold replaces Nil with z and Cons with f:
fold : forall a r. (a -> r -> r) -> r -> List a -> r
fold f z Nil         = z
fold f z (Cons x xs) = f x (fold f z xs)
```

The fold is total: it recurses structurally over the list, consuming one `Cons` at each step, and terminates when it reaches `Nil`.

### 7.3 Dhall's approach

Dhall does not have native recursive data types. Instead, it uses the Boehm-Berarducci encoding: a recursive data type is represented as its own fold.

```text
-- Dhall's List is (morally):
List a = forall (r : Type) -> (a -> r -> r) -> r -> r

-- A concrete list [1, 2, 3]:
\(r : Type) -> \(cons : Natural -> r -> r) -> \(nil : r) ->
    cons 1 (cons 2 (cons 3 nil))
```

This encoding guarantees totality because the "list" is a function that calls its argument a fixed number of times. There is no recursive spine to traverse; the iteration count is baked into the representation.

### 7.4 Native recursive types with restricted elimination

An alternative to Dhall's Church encoding is to provide native recursive data types but restrict how they can be eliminated. The language provides `case` analysis (pattern matching) but not general recursion. To iterate over recursive data, the language provides built-in fold combinators.

```text
data List a = Nil | Cons a (List a)

-- Built-in, not user-definable:
foldList : forall a r. (a -> r -> r) -> r -> List a -> r
```

The user can pattern-match on a list to inspect its head, but cannot write a recursive function that traverses the entire list. Only the built-in `foldList` provides iteration.

This approach is more ergonomic than Dhall's Church encoding (lists are actual data structures, not functions) while preserving totality.

### 7.5 Interaction with ADTs in the current spec

The spec v0.2 assumes algebraic data types with case analysis. If recursive data types are allowed, the spec must decide:

1. **Are recursive type definitions permitted?** E.g., `data List a = Nil | Cons a (List a)`.
2. **How are recursive types eliminated?** Case analysis alone does not provide iteration. Either built-in folds, structural recursion, or general recursion (with controls) is needed.
3. **Can the user define folds?** In Dhall, folds are built-in. In Agda, the user writes structurally recursive functions that the termination checker verifies. In a language with general recursion, the user writes whatever recursive functions they want.

The recommended approach for Gomputation (see Section 12) is: allow recursive data types, provide built-in `fold` combinators for common types, and use the recursion-as-capability mechanism for general recursive functions.

---

## 8. Interaction with Indexed Effects and Typestate

### 8.1 The core tension

Gomputation's computation type is `Computation pre post a`. The `bind` operation composes computations by threading the capability state:

```text
bind : Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

Recursion introduces a loop. In a loop, the computation body is executed repeatedly. For the types to compose, each iteration's post-state must be compatible with the next iteration's pre-state. This imposes a constraint: **the body of a recursive computation must preserve capability state**.

### 8.2 State-preserving recursion

A recursive computation that preserves capability state has the type:

```text
loop_body : a -> Computation r r b
```

Here `pre = post = r`. The body starts and ends in the same capability state. This is a natural requirement: a loop that opens a database connection on each iteration without closing it would accumulate unbounded state, which is both a type error and a resource leak.

The `rec` primitive from Section 6 enforces this:

```text
rec : forall a b. ((a -> Computation r r b) -> a -> Computation r r b)
                -> a -> Computation r r b
```

The requirement that `pre = post = r` is built into the type of `rec`. This means:

- A recursive function can use capabilities (read from a database, log messages) as long as it returns the capability environment to its original state at the end of each iteration.
- A recursive function **cannot** progressively change capability state across iterations (e.g., opening a sequence of connections without closing them).

### 8.3 Why state-changing recursion is problematic

Consider a hypothetical recursive computation that changes state at each iteration:

```text
-- Hypothetical: each iteration opens a new connection
openMany : Int -> Computation { connections : Vec n } { connections : Vec (n + k) } Unit
```

This type requires dependent types (the post-state depends on the runtime value `k`) and type-level arithmetic. It is far beyond the current spec's type system and is not a practical concern for the initial design.

More concretely, even without dependent types, a loop body with `pre /= post` creates a type that cannot compose with itself:

```text
body : Computation r1 r2 a
-- Second iteration: Computation r2 r3 a
-- Third iteration: Computation r3 r4 a
-- ...
```

The types diverge at each iteration. Without dependent types or type-level naturals, there is no way to express the type of `n` iterations.

### 8.4 Recursion and protocol compliance

The state-preserving constraint `pre = post` has a natural interpretation in the protocol/typestate model: a recursive computation is a "transaction" that leaves the system in the same protocol state it started in. This is analogous to a database transaction that commits or rolls back to the original state.

Protocols that require progressive state changes (e.g., a multi-step authentication handshake) should be expressed as a sequence of `bind` operations, not as a loop. The type system enforces this by construction: `bind` allows `pre /= post`, but `rec` requires `pre = post`.

### 8.5 Higher-order iteration over state-changing operations

The non-linear-effect-composition document (already in the docs) analyzes a related issue: `mapM` over state-changing operations. The conclusion there applies here:

> `mapM` only works for state-preserving operations. For operations that genuinely change state, the list length would need to be reflected in the type.

This is consistent with the `rec` design: iteration is state-preserving, and state-changing sequences are explicit compositions via `bind`.

### 8.6 Row polymorphism and recursion

The `rec` primitive is row-polymorphic:

```text
rec : forall r a b. ((a -> Computation r r b) -> a -> Computation r r b)
                  -> a -> Computation r r b
```

The row variable `r` means: the recursive function works in any capability environment, as long as it preserves that environment. This is the correct generalization: a recursive helper function should not be tied to a specific set of capabilities.

---

## 9. Decision Matrix

### 9.1 Design options evaluated against Gomputation's use cases

| Design | Config eval | Rule eval | Domain logic | Scripting | Protocol exec | Impl. cost |
|---|---|---|---|---|---|---|
| No recursion (CEL) | Partial | Partial | Insufficient | Insufficient | Compatible | Minimal |
| Folds only (Dhall) | Good | Good | Adequate | Limited | Compatible | Low |
| Structural recursion | Good | Good | Good | Good | Compatible | High |
| General + step limits | Good | Good | Good | Good | Compatible* | Low |
| Recursion as capability | Good | Good | Good | Good | Compatible | Low-Medium |

*Compatible if `rec` requires `pre = post`.

### 9.2 Properties of each design

| Design | Terminates? | Static guarantee? | Ergonomics | Familiarity |
|---|---|---|---|---|
| No recursion | Always | Yes | Poor (no iteration) | Unusual |
| Folds only | Always | Yes | Moderate (fold-only) | Unusual |
| Structural recursion | Always | Yes | Good (natural) | Familiar (Agda/Idris) |
| General + step limits | Up to limit | No | Good (natural) | Familiar (Starlark/Lua) |
| Recursion as capability | Depends on host | Conditional | Moderate | Novel |

### 9.3 Implementation complexity

| Design | Type checker changes | Evaluator changes | New analyses |
|---|---|---|---|
| No recursion | None | None | None |
| Folds only | Built-in fold types | Built-in fold eval | None |
| Structural recursion | None | None | Termination checker |
| General + step limits | `fix`/`letrec` typing | Recursive closures | Step counter |
| Recursion as capability | `rec` as host prim | Recursive closures | Step counter |

### 9.4 Compatibility with existing design

| Design | Consistent with capability model? | Consistent with indexed effects? | Consistent with determinism? |
|---|---|---|---|
| No recursion | Yes (no new authority) | Yes (trivially) | Yes |
| Folds only | Neutral (built-in, not host-provided) | Yes (folds are state-preserving) | Yes |
| Structural recursion | Neutral (compiler feature) | Yes (but checker needed) | Yes |
| General + step limits | Weak (recursion is ambient) | Yes if `pre = post` enforced | Yes (step limits are deterministic) |
| Recursion as capability | Strong (fits the model exactly) | Yes (`rec` requires `pre = post`) | Yes (step limits are deterministic) |

---

## 10. Formal Rules for `fix` and `letrec`

This section gives the formal typing and evaluation rules for both `fix` (value-level) and `rec` (computation-level) under the assumption that general recursion is introduced.

### 10.1 Value-level `fix`

**Typing rule:**

```text
Gamma |- f : (a -> b) -> (a -> b)
──────────────────────────────────  [Fix]
Gamma |- fix f : a -> b
```

More precisely, with explicit polymorphism:

```text
Gamma |- f : forall a b. ((a -> b) -> (a -> b))
────────────────────────────────────────────────  [Fix-Poly]
Gamma |- fix f : forall a b. (a -> b)
```

**Big-step evaluation rule (CBV):**

```text
rho |- f ==> <\g -> body_f, rho_f>
rho |- fix f ==> w
where w is defined by: w = eval(body_f, rho_f[g -> <\x -> body_fix, rho_fix>])
and rho_fix = rho[fix_f -> w]   -- circular environment
```

In practice, the environment-based evaluator creates a recursive closure:

```text
rho |- fix f ==> <\x -> e, rho'>
where:
  rho |- f ==> <\self -> \x -> e, rho_f>
  rho' = rho_f[self -> <\x -> e, rho'>]   -- self-referential
```

The closure `<\x -> e, rho'>` contains an environment `rho'` that points back to the closure itself. This is the standard "knot-tying" implementation of `fix` in environment-based evaluators.

### 10.2 Computation-level `rec`

**Typing rule:**

```text
Gamma |- f : (a -> Computation r r b) -> (a -> Computation r r b)
────────────────────────────────────────────────────────────────────  [Rec]
Gamma |- rec f : a -> Computation r r b
```

**Big-step evaluation rule:**

```text
rho |- f ==> <\self -> \x -> body, rho_f>
rho |- v ==> w
rho'; sigma |- body[self -> rec_closure, x -> w] ==> sigma'; w'
where rec_closure = <\x -> body, rho_f[self -> rec_closure]>
────────────────────────────────────────────────────────────────
rho; sigma |- rec f v ==> sigma'; w'
```

Note that the capability environment `sigma` is threaded through the body. The type constraint `pre = post = r` ensures that `sigma'` has the same structure as `sigma` (up to row equality).

### 10.3 `letrec` as syntactic sugar

If the language provides a surface `letrec`, it desugars to `fix` or `rec`:

```text
-- Surface:
letrec f x = body in expr

-- Desugared (value level):
let f = fix (\f -> \x -> body) in expr

-- Desugared (computation level):
let f = rec (\f -> \x -> body) in expr
```

The choice between `fix` and `rec` depends on whether `body` is a value expression or a computation expression.

### 10.4 Step-counted evaluation rules

With step counting, the evaluation rules are augmented with a fuel parameter:

```text
rho; sigma; n |- c ==> sigma'; w; n'
```

where `n` is the remaining fuel and `n'` is the fuel after evaluation.

```text
n > 0
rho |- f ==> <\self -> \x -> body, rho_f>
rho |- v ==> w
rho_f[self -> rec_closure, x -> w]; sigma; (n-1) |- body ==> sigma'; w'; n'
────────────────────────────────────────────────────────────────────────────
rho; sigma; n |- rec f v ==> sigma'; w'; n'

n <= 0
────────────────────────────────────────────────
rho; sigma; n |- rec f v ==> ERROR("step limit")
```

Each entry into `rec` consumes one step. The body may consume additional steps through nested computations.

---

## 11. Implementation in Go

### 11.1 Step counter

The step counter is the simplest and most important runtime control. It is a single integer threaded through the evaluator.

```go
type EvalContext struct {
    VarEnv  *VarEnv
    CapEnv  *CapEnv
    Fuel    int
    MaxDepth int
    Depth   int
}

func (ctx *EvalContext) consumeStep() error {
    ctx.Fuel--
    if ctx.Fuel <= 0 {
        return fmt.Errorf("step limit exceeded after %d steps", ctx.MaxFuel)
    }
    return nil
}
```

Every entry into `evalComp` and (optionally) `evalValue` calls `consumeStep`. This is the approach used by Starlark-go.

### 11.2 Stack depth limit

A separate counter tracks the depth of recursive calls:

```go
func (ctx *EvalContext) pushFrame() error {
    ctx.Depth++
    if ctx.Depth > ctx.MaxDepth {
        return fmt.Errorf("stack depth exceeded: %d frames", ctx.Depth)
    }
    return nil
}

func (ctx *EvalContext) popFrame() {
    ctx.Depth--
}
```

The depth counter is incremented at function application and decremented on return. This catches deep recursion even if the step counter has not been exhausted.

### 11.3 Implementing `fix` in an environment-based evaluator

The key implementation challenge for `fix` is creating a self-referential closure. In Go, this is done by mutation: allocate the closure, then update its environment to point back to itself.

```go
func evalFix(ctx *EvalContext, f Value) (Value, error) {
    // f should be a closure: \self -> \x -> body
    fClosure, ok := f.(*Closure)
    if !ok {
        return nil, fmt.Errorf("fix: argument must be a function")
    }

    // Create a placeholder for the recursive closure
    recClosure := &Closure{}

    // Apply f to the recursive closure to get the "unwrapped" function
    innerEnv := fClosure.Env.Extend(fClosure.Param, recClosure)
    inner, err := evalValue(ctx, fClosure.Body, innerEnv)
    if err != nil {
        return nil, err
    }

    innerClosure, ok := inner.(*Closure)
    if !ok {
        return nil, fmt.Errorf("fix: result of applying f must be a function")
    }

    // Tie the knot: the recursive closure IS the inner closure,
    // but its environment contains a reference to itself
    *recClosure = *innerClosure
    recClosure.Env = innerClosure.Env.Extend(fClosure.Param, recClosure)

    return recClosure, nil
}
```

The mutation is confined to initialization and does not violate the language's value-level purity: the recursive closure is a value that, when applied, behaves identically to its mathematical definition.

### 11.4 Implementing `rec` as a host-provided primitive

If recursion is a capability, `rec` is registered as a host primitive:

```go
type RecPrimitive struct{}

func (p *RecPrimitive) Name() string { return "rec" }

func (p *RecPrimitive) Type() TypeExpr {
    // forall r a b. ((a -> Computation r r b) -> a -> Computation r r b)
    //            -> a -> Computation r r b
    return /* the type expression */
}

func (p *RecPrimitive) Execute(capEnv *CapEnv, args []Value) (*CapEnv, Value, error) {
    // args[0]: the step function f
    // args[1]: the initial argument
    f := args[0].(*Closure)
    arg := args[1]

    // Create a self-referential computation function
    recFunc := &RecursiveClosure{StepFn: f}
    recFunc.Self = recFunc  // tie the knot

    // Apply the recursive function to the argument
    return evalComp(ctx, Apply(recFunc, arg), capEnv)
}
```

The exact implementation depends on the evaluator architecture, but the principle is the same: create a closure that refers to itself, and evaluate the body with this closure in scope.

### 11.5 Implementing structural recursion checking

If Gomputation were to adopt structural recursion checking (not recommended for the initial design), the implementation would require:

1. **Call graph construction.** After type checking, build a graph of which functions call which other functions.
2. **Size analysis.** For each recursive call, determine which arguments decreased. This requires pattern-matching analysis: if a function pattern-matches on `Cons x xs` and recurses on `xs`, then `xs` is structurally smaller.
3. **Lexicographic ordering.** Check that there exists a lexicographic ordering on the arguments that decreases at every recursive call.
4. **Mutual recursion.** Extend the analysis to strongly connected components of the call graph.

The implementation cost is estimated at 1,000-3,000 lines of Go for a basic structural checker, with significant additional complexity for good error messages and edge cases.

### 11.6 Performance implications

- **Step counting:** Negligible overhead. A single integer decrement and comparison per evaluation step.
- **Stack depth tracking:** Negligible overhead. A single integer increment/decrement per function call.
- **Recursive closure creation:** One-time cost per `fix`/`rec` application. The knot-tying mutation is a pointer write.
- **Structural recursion checking:** Compile-time cost only. No runtime overhead once the check passes.

---

## 12. Recommendation for Gomputation

### 12.1 The recommended design: layered totality with recursion as a capability

The recommendation is a three-layer design that leverages Gomputation's existing capability architecture:

**Layer 1: Total core (default).** The base language has no general recursion. All programs in the base language terminate. This is suitable for configuration evaluation, rule evaluation, and contexts where termination must be guaranteed.

**Layer 2: Built-in folds for recursive data.** The language provides recursive data types (lists, trees, user-defined recursive ADTs) with built-in fold/catamorphism combinators. These folds are total: they iterate structurally over finite data. This gives the base language sufficient expressiveness for most configuration and rule-evaluation tasks without sacrificing totality.

**Layer 3: General recursion as a host-provided capability.** The host may register a `rec` primitive that provides general recursion at the computation level. Programs that use `rec` may diverge and are subject to step limits configured by the host. This is suitable for embedded scripting, domain logic, and any context where the host is willing to accept the risk of non-termination in exchange for expressiveness.

### 12.2 Rationale

1. **Default safety.** Most of Gomputation's use cases (configuration, rules, protocol execution) benefit from termination guarantees. Making the language total by default serves the majority case.

2. **Opt-in power.** Scripting and domain logic sometimes require general recursion. Making it available as a capability gives the host explicit control.

3. **No termination checker.** A structural termination checker is complex to implement and maintain. The layered design achieves a similar effect (totality for programs that do not use `rec`) without the implementation burden.

4. **Consistency with the capability model.** Recursion-as-capability is a natural extension of Gomputation's existing design. The host already controls what effects a program can perform; controlling whether a program can recurse is the same kind of decision.

5. **Step limits as defense-in-depth.** Even in the total core (Layers 1 and 2), step limits should be active. They protect against bugs in the type checker, pathological inputs to folds, and other unexpected scenarios. When `rec` is available (Layer 3), step limits are essential.

### 12.3 Concrete specification additions

To implement this recommendation, the spec should add:

1. **Recursive data type declarations.** Allow `data List a = Nil | Cons a (List a)`. This is a straightforward extension of the existing ADT support.

2. **Built-in fold combinators.** For each recursive data type `D`, the language provides a fold combinator `foldD` with a type derived from the constructors of `D`. For example:
   ```text
   foldList : forall a r. r -> (a -> r -> r) -> List a -> r
   ```
   These are either automatically generated or provided as primitives.

3. **The `rec` primitive.** A host-registerable primitive with the type:
   ```text
   rec : forall r a b. ((a -> Computation r r b) -> a -> Computation r r b)
                     -> a -> Computation r r b
   ```
   The host decides whether to register this primitive. If registered, it is available to user programs. If not, general recursion is not expressible.

4. **Step limit specification.** The host sets a step limit. The evaluator decrements a counter at each evaluation step (computation-level and, optionally, value-level). When the counter reaches zero, evaluation halts with a deterministic error.

5. **Stack depth limit specification.** The host sets a maximum call stack depth. The evaluator tracks the current depth and halts with a deterministic error if the limit is exceeded.

### 12.4 What this design rules out

- **Totality proofs.** The design does not statically prove termination. Totality of programs that do not use `rec` is ensured by the absence of general recursion in the core, not by a termination checker. This is Dhall's strategy: totality by construction, not by analysis.

- **Structural recursion.** Users cannot write structurally recursive functions directly. They must use the built-in fold combinators or the `rec` capability. This is a deliberate simplification that avoids the implementation cost of a termination checker.

- **Divergence tracking in the type system.** The type of a computation does not indicate whether it may diverge. If `rec` is in scope, any function could potentially use it. If this distinction becomes important, a future extension could add a `div` effect (following Koka's approach) or track `rec` usage at the type level.

### 12.5 Examples under the recommended design

**Configuration evaluation (Layer 1 + 2, no `rec`):**

```text
-- Total: uses built-in fold, no general recursion
sumPrices :: List Item -> Computation r r Int
sumPrices items =
    pure (foldList 0 (\item acc -> price item + acc) items)
```

**Rule evaluation (Layer 1 + 2, no `rec`):**

```text
-- Total: structural fold over a decision tree
evaluate :: DecisionTree -> Input -> Computation r r Decision
evaluate tree input =
    pure (foldTree
        (\leaf -> leafDecision leaf)
        (\node left right ->
            if predicate node input
                then left
                else right)
        tree)
```

**Embedded scripting (Layer 3, with `rec`):**

```text
-- Requires rec capability; may diverge; subject to step limits
fibonacci :: Int -> Computation r r Int
fibonacci = rec (\self n ->
    if n <= 1
        then pure n
        else bind (self (n - 1)) (\a ->
             bind (self (n - 2)) (\b ->
             pure (a + b))))
```

**Protocol execution with recursion (Layer 3, with `rec`):**

```text
-- Recursive polling: check a condition, retry if not met
-- Preserves capability state (pre = post = r)
pollUntilReady :: Computation { sensor : Sensor[Active] | r }
                              { sensor : Sensor[Active] | r }
                              Reading
pollUntilReady = rec (\self () ->
    bind sensorRead (\reading ->
        if isReady reading
            then pure reading
            else self ())) ()
```

### 12.6 Migration path

The recommended design supports a natural migration path:

1. **v0.3 or similar:** Ship the total core with built-in folds. No general recursion. This covers configuration and rule evaluation.

2. **v0.4 or later:** Add the `rec` capability. This unlocks scripting and domain logic use cases. Step limits are already in place from v0.3.

3. **Future (if needed):** Add a `div` effect to track divergence in the type system. Add structural recursion as an ergonomic alternative to folds for programs that can be proven total. These are optional refinements, not prerequisites.

---

## 13. Key References

### Total languages and configuration languages

1. Gabriel Gonzalez. "Why Dhall advertises the absence of Turing-completeness." Blog post, 2020. https://www.haskellforall.com/2020/01/why-dhall-advertises-absence-of-turing.html

2. Dhall documentation. "How to translate recursive code to Dhall." https://docs.dhall-lang.org/howtos/How-to-translate-recursive-code-to-Dhall.html

3. Google. "Common Expression Language (CEL) Overview." https://cel.dev/overview/cel-overview

4. Google. CEL specification. https://github.com/google/cel-spec

5. CUE language. https://cuelang.org/

6. Nickel language. "Rationale." https://github.com/tweag/nickel/blob/master/RATIONALE.md

### Termination checking and structural recursion

7. Agda documentation. "Termination Checking." https://agda.readthedocs.io/en/latest/language/termination-checking.html

8. Idris documentation. "Theorem Proving" (totality). https://idris2.readthedocs.io/en/latest/tutorial/theorems.html

9. Chin Soon Lee, Neil D. Jones, and Amir M. Ben-Amram. "The size-change principle for program termination." *POPL*, 2001.

10. David Wahlstedt. "Comparing structural recursion and sized types." McGill University. https://www.cs.mcgill.ca/~dthibo1/papers/termination.pdf

11. Andreas Abel. "MiniAgda: Integrating Sized and Dependent Types." 2010. https://arxiv.org/pdf/1012.4896

12. Corrado Bohm and Alessandro Berarducci. "Automatic synthesis of typed Lambda-programs on term algebras." *Theoretical Computer Science*, 39:135-154, 1985.

### Recursion, fixed points, and type systems

13. Derek Dreyer, Robert Harper, and Karl Crary. "A type system for well-founded recursion." *POPL*, 2004. https://people.mpi-sws.org/~dreyer/papers/recursion/popl.pdf

14. Ravi Chugh. "ISOLATE: A Type System for Self-Recursion." *ESOP*, 2015.

### Effect systems and divergence

15. Daan Leijen. "Koka: Programming with Row-Polymorphic Effect Types." *MSFP*, 2014. https://arxiv.org/pdf/1406.2061

16. Sam Lindley, Conor McBride, and Craig McLaughlin. "Do Be Do Be Do." *POPL*, 2017. (The Frank language.)

### Parameterized monads (for Section 8)

17. Robert Atkey. "Parameterised Notions of Computation." *Journal of Functional Programming*, 19(3-4):335-376, 2009.

18. Conor McBride. "Kleisli Arrows of Outrageous Fortune." 2011.

### Embedded language implementations

19. Starlark specification. https://github.com/bazelbuild/starlark/blob/master/spec.md

20. Starlark-go. "Consider making recursion optional." Issue #97. https://github.com/bazelbuild/starlark/issues/97

21. Starlark-go implementation. https://pkg.go.dev/go.starlark.net/starlark (see `SetMaxExecutionSteps`).

### Blockchain gas metering

22. "Delivering Fair Gas Fees Through Resource Usage Metering." Sui blog. https://blog.sui.io/computation-costs-gas-fee-model/

---

## Appendix A: The Trade-Off Triangle

The design of recursion in an embedded language is governed by a three-way trade-off:

```text
         Expressiveness
            /\
           /  \
          /    \
         /      \
        /________\
Termination    Simplicity
guarantee
```

**Expressiveness vs. termination guarantee.** General recursion is maximally expressive but provides no termination guarantee. Total languages (no recursion, folds only) guarantee termination but restrict what programs can express.

**Expressiveness vs. simplicity.** Structural recursion with totality checking is expressive and guarantees termination, but the implementation is complex. General recursion with step limits is expressive and simple to implement, but does not guarantee termination.

**Termination guarantee vs. simplicity.** The simplest path to termination is "no recursion" (CEL's approach), but this is too restrictive. The next simplest is "folds only" (Dhall's approach), which requires built-in fold combinators. Structural recursion checking provides more flexibility but at significant implementation cost.

Gomputation's recommended design navigates this triangle by:
- Choosing **termination + simplicity** as the default (folds, no general recursion).
- Offering **expressiveness + simplicity** as an opt-in (general recursion via `rec`, with step limits).
- Deferring **expressiveness + termination** (structural recursion checking) to a future extension if the need arises.

## Appendix B: Comparison with Existing Gomputation Documents

This document connects to several existing documents in the Gomputation docs:

- **evaluation-semantics.md** Section 5.6 (step counting) and Section 8.3 (termination) directly anticipate the decisions made here. The step counter described there is the runtime mechanism that supports Layer 3 (general recursion with controls).

- **indexed-effects-typestate-capabilities.md** The capability-security model described there extends naturally to the recursion-as-capability design in Section 6.

- **indexed-parameterized-graded-monads.md** Section 7.5 (higher-order indexed operations) identifies the constraint that iteration over state-changing operations requires `pre = post`, which is the same constraint that Section 8 derives for recursive computations.

- **non-linear-effect-composition.md** The branching problem analyzed there is related to but distinct from the recursion problem: branching produces a cospan of post-states, while recursion requires a self-loop of matching pre/post states.

- **cbpv-and-value-computation-metalanguage.md** The value/computation split described there determines where `fix` (value-level) and `rec` (computation-level) live in the type system.

- **host-boundary-design-patterns.md** The host primitive registration pattern described there is the mechanism by which `rec` would be registered.
