# Pattern Matching: Grammar, Compilation, and Exhaustiveness Checking

One-line description: formal treatment of pattern grammar, compilation to decision trees, exhaustiveness and redundancy checking, and their interaction with indexed types, the value/computation split, and Go as a host language.

## Table of Contents

1. Overview and Motivation
2. Pattern Grammar
3. Typing Rules for Case Expressions
4. Pattern Match Compilation
5. Exhaustiveness Checking
6. Redundancy Detection
7. Interaction with Indexed Types and GADTs
8. Interaction with Computations
9. Implementation in Go
10. Practical Systems Comparison
11. Recommendations for Gomputation
12. Key References

---

## 1. Overview and Motivation

### 1.1 The Role of Pattern Matching

Pattern matching is the elimination form for algebraic data types. In a language with ADTs, constructors are introduction forms --- they build values of a given type. Pattern matching is the corresponding elimination form --- it decomposes values by case analysis on their structure.

The spec (v0.2, Section 14.7) states:

- `case` is fundamentally a value-level eliminator
- the scrutinee has an algebraic data type
- each branch is checked under bindings introduced by its pattern
- all branches must produce the same result type

This document gives the formal grammar, typing rules, compilation strategy, and exhaustiveness algorithm needed to make these commitments precise.

### 1.2 What Pattern Matching Must Guarantee

A correct pattern matching system provides three guarantees:

1. **Completeness (exhaustiveness)**: every possible value of the scrutinee type is handled by at least one branch. If a pattern set is non-exhaustive, the checker must report the missing cases.

2. **Non-redundancy (usefulness)**: every branch in a match expression is reachable by some value. Redundant branches are dead code --- they indicate either a programmer error or a maintenance hazard.

3. **Type correctness**: pattern variables are bound with correct types, and any type refinement implied by matching a constructor (especially relevant for future GADTs) is reflected in the context of the branch body.

### 1.3 Design Scope

This document covers the design space appropriate for Gomputation's current trajectory:

- **Immediate**: simple constructor patterns, variable patterns, wildcards, nested patterns, literal patterns. Exhaustiveness and redundancy checking over ordinary ADTs.
- **Near-term**: as-patterns, tuple patterns (if tuples are added).
- **Deferred but designed-for**: GADT-refined pattern matching with type equality witnesses.

---

## 2. Pattern Grammar

### 2.1 Formal Grammar

The pattern language is defined by the following grammar. It is intentionally close to the Haskell tradition but adapted to Gomputation's syntactic conventions.

```
Pat ::= _                               -- wildcard
      | x                               -- variable
      | C Pat_1 ... Pat_n               -- constructor pattern (n >= 0)
      | lit                              -- literal pattern (Int, String, ...)
      | x @ Pat                          -- as-pattern (bind + match)
      | ( Pat )                          -- parenthesized pattern
```

Where:

- `_` matches any value without binding.
- `x` matches any value and binds it to the name `x`.
- `C Pat_1 ... Pat_n` matches a value built by constructor `C` with `n` fields, recursively matching each field against the corresponding sub-pattern. The arity `n` must match the declared arity of `C`.
- `lit` matches a literal value (integer, string, etc.) by equality.
- `x @ Pat` matches a value against `Pat` and simultaneously binds the entire matched value to `x`.

### 2.2 Design Decisions

**Variable vs. wildcard.** Both `_` and `x` match anything. The difference is binding: `x` introduces a name into the branch body's context, while `_` does not. This distinction is syntactically lightweight but semantically important. It also interacts with linearity --- the wildcard explicitly signals that the matched value is unused.

**Nested constructor patterns.** Allowing `C (D x y) z` directly in the surface syntax avoids forcing users to write nested case expressions. This is standard in Haskell, OCaml, Rust, Elm, and PureScript.

**Literal patterns.** Literal patterns are desirable for ergonomics but introduce a complication: the set of values for a literal type (e.g., `Int`) is unbounded, so a match on literals can never be exhaustive without a wildcard or variable catch-all. The exhaustiveness checker must treat literal types as having an infinite (or at least non-enumerable) constructor set.

**As-patterns.** As-patterns (`x @ Pat`) are a convenience for binding the whole matched value while also decomposing it. They are cheap to implement and useful in practice. They do not affect exhaustiveness --- `x @ Pat` covers exactly the values that `Pat` covers.

**What is not included.** The following are deliberately excluded from the initial pattern grammar:

- **Or-patterns** (`Pat1 | Pat2`): useful but complicate the exhaustiveness algorithm and require care about variable binding consistency across alternatives. Defer.
- **View patterns** (`f -> Pat`): require running a function during matching, which interacts with the value/computation boundary. Defer.
- **Pattern guards** (`Pat | guard`): guards interact with exhaustiveness checking in subtle ways (the checker cannot know whether guards are satisfiable in general). Defer, but design for future addition.
- **Record/row patterns**: the spec currently uses rows only for capability environments, not for general record values. If row-typed records are added later, record patterns should follow.

### 2.3 Linearity Constraint

Within a single pattern, each variable name must appear at most once. This is the standard linearity condition on patterns. Violating it would require an implicit equality check --- a different semantic mechanism that should be explicit if ever supported.

```
-- Legal:
C x y
x @ (C _ y)

-- Illegal:
C x x          -- x appears twice; would require x == x check
```

### 2.4 Interaction with Types

Patterns are not merely syntactic forms; they carry type information.

Given a scrutinee of type `D a_1 ... a_m` (an algebraic data type applied to type arguments), matching against a constructor pattern `C p_1 ... p_n` is well-formed when:

1. `C` is a constructor of type `D`.
2. `C` has arity `n`.
3. Each sub-pattern `p_i` has the type of the `i`-th field of `C`, instantiated with the type arguments `a_1, ..., a_m`.

This information is available from the data type declaration and the type of the scrutinee.

### 2.5 Surface Syntax for Case Expressions

Following the Haskell-like surface direction from the spec:

```
case scrutinee of
  Pat_1 -> Expr_1
  Pat_2 -> Expr_2
  ...
  Pat_n -> Expr_n
```

The arrow `->` separates each pattern from its branch body. Branches are separated by newlines (layout-sensitive) or semicolons (layout-insensitive). Indentation rules follow standard Haskell convention.

---

## 3. Typing Rules for Case Expressions

### 3.1 Checking Rule

The primary typing rule for `case` is a checking rule in a bidirectional system. When an expected type is available (from an annotation or checking mode), all branches are checked against it.

```
Gamma |- s => D a_1 ... a_m                         -- synthesize scrutinee type
D has constructors C_1, ..., C_k
for each branch i:
  Pat_i is a well-formed pattern for D a_1 ... a_m
  bindings(Pat_i, D a_1 ... a_m) = Delta_i
  Gamma, Delta_i |- e_i <= A                         -- check body against expected type
patterns {Pat_1, ..., Pat_n} are exhaustive for D
------------------------------------------------------
Gamma |- (case s of Pat_1 -> e_1; ...; Pat_n -> e_n) <= A
```

### 3.2 Synthesis Rule

When no expected type is available, the case expression synthesizes its result type from the first branch.

```
Gamma |- s => D a_1 ... a_m
D has constructors C_1, ..., C_k
Pat_1 is well-formed for D a_1 ... a_m
bindings(Pat_1, D a_1 ... a_m) = Delta_1
Gamma, Delta_1 |- e_1 => A                           -- synthesize from first branch
for each subsequent branch i > 1:
  bindings(Pat_i, D a_1 ... a_m) = Delta_i
  Gamma, Delta_i |- e_i <= A                         -- check remaining branches
patterns {Pat_1, ..., Pat_n} are exhaustive for D
------------------------------------------------------
Gamma |- (case s of Pat_1 -> e_1; ...; Pat_n -> e_n) => A
```

### 3.3 Pattern Binding Extraction

The function `bindings(Pat, T)` extracts the variable bindings introduced by a pattern matched against a value of type `T`. It is defined inductively:

```
bindings(_, T)           = {}
bindings(x, T)           = {x : T}
bindings(C p_1...p_n, D a_1...a_m)
  where C : T_1 -> ... -> T_n -> D a_1...a_m
                         = bindings(p_1, T_1) ++ ... ++ bindings(p_n, T_n)
bindings(lit, T)         = {}
bindings(x @ p, T)       = {x : T} ++ bindings(p, T)
```

The types `T_1, ..., T_n` are the field types of constructor `C`, instantiated with the type arguments `a_1, ..., a_m` from the scrutinee's type.

### 3.4 Well-Formedness of Patterns

A pattern is well-formed relative to a type `T` when:

1. If `T = D a_1 ... a_m` and the pattern is `C p_1 ... p_n`, then `C` is a constructor of `D` with arity `n`, and each `p_i` is well-formed relative to the corresponding field type of `C` (instantiated at `a_1, ..., a_m`).
2. If the pattern is `_` or `x`, it is well-formed for any type.
3. If the pattern is `lit`, it is well-formed when `T` is the type of the literal.
4. If the pattern is `x @ p`, then `p` is well-formed relative to `T`.
5. All variable names in the pattern are distinct (linearity).

### 3.5 The Scrutinee Must Synthesize

An important design point: in a bidirectional system, the scrutinee should synthesize its type. The case expression needs to know the data type of the scrutinee to determine which constructors are valid and what their field types are. This means the scrutinee must be in synthesis position --- a variable, an application, or an annotated expression. If the scrutinee is a bare lambda or other checking-mode-only form, it must be annotated.

This is consistent with the bidirectional principle: elimination forms synthesize, and `case` is an elimination form.

---

## 4. Pattern Match Compilation

### 4.1 The Problem

The surface syntax allows arbitrarily nested patterns. The runtime or code generator must translate these into a sequence of primitive tests (constructor tag checks, literal equality checks) and variable bindings. The goal is to produce code that:

1. Tests each value component at most once.
2. Never duplicates a test.
3. Handles all cases (exhaustiveness has already been checked statically).
4. Produces no dead code (redundancy has already been checked statically).

### 4.2 Pattern Matrices

The standard framework for pattern match compilation, due to Augustsson (1985) and systematized by Maranget (2008), represents the set of clauses as a **pattern matrix**.

Given a case expression:

```
case (v_1, ..., v_n) of
  p_{1,1} ... p_{1,n} -> e_1
  p_{2,1} ... p_{2,n} -> e_2
  ...
  p_{m,1} ... p_{m,n} -> e_m
```

the pattern matrix is the m-by-n matrix P where `P[i][j] = p_{i,j}`.

For a single-scrutinee case expression `case v of ...`, the matrix has one column (`n = 1`). Nested patterns are handled by expanding the matrix: when a constructor `C` with arity `a` is tested, the column for `v` is replaced by `a` new columns for the fields of `C`.

### 4.3 Augustsson's Algorithm: Decision Trees

Augustsson (1985) gave the first systematic algorithm for compiling pattern matching. The algorithm produces a **decision tree** --- a tree of test nodes, where:

- Each internal node tests the constructor tag (or literal value) of a specific sub-value.
- Each edge from an internal node corresponds to one possible constructor (or the default case).
- Each leaf node corresponds to a right-hand-side expression, with the pattern variables bound.

**Algorithm sketch:**

```
compile(vars, clauses):
  -- vars: the list of values being matched [v_1, ..., v_n]
  -- clauses: list of (pattern-row, body) pairs

  if clauses is empty:
    error "non-exhaustive patterns"  -- should not happen after static check

  if all patterns in the first column are variables or wildcards:
    -- no constructor test needed on the first value
    -- bind the variable (or discard for wildcard) and recurse on remaining columns
    for each clause (p_1 p_2...p_n -> e):
      let bindings = if p_1 is variable x then [x := v_1] else []
      substitute bindings into remaining patterns and body
    return compile(vars[2..n], simplified_clauses)

  otherwise:
    -- the first column contains at least one constructor pattern
    -- pick a column to split on (heuristic: the first with a constructor)
    let col = index of a column with a constructor pattern
    let v = vars[col]
    let type_of_v = D a_1...a_m with constructors C_1, ..., C_k

    for each constructor C_i of arity a_i:
      -- specialize: keep only rows whose pattern in column col
      --   matches C_i, and expand C_i's fields into new columns
      let specialized = specialize(clauses, col, C_i)
      let new_vars = [v.field_1, ..., v.field_{a_i}] ++ vars without col
      branches[C_i] = compile(new_vars, specialized)

    -- default case: rows where column col has a variable/wildcard
    let default_clauses = default(clauses, col)
    if default_clauses is non-empty:
      default_branch = compile(vars without col, default_clauses)

    return Switch(v, branches, default_branch)
```

**Specialization.** Given a constructor `C` of arity `a`, the specialization of a clause set keeps only rows compatible with `C`:

- If `p_{i,col} = C q_1...q_a`, replace column `col` with columns `q_1, ..., q_a`.
- If `p_{i,col} = _` or `x`, replace column `col` with `a` wildcards `_, ..., _`.
- If `p_{i,col} = D q_1...q_b` where `D != C`, drop the row.

**Default matrix.** The default matrix keeps rows where column `col` has a variable or wildcard, and removes column `col`.

### 4.4 Maranget's Algorithm: Optimized Decision Trees

Maranget (2008) refined the decision-tree approach with two key improvements:

1. **Column selection heuristic.** Instead of always choosing the leftmost column with a constructor pattern, Maranget proposes heuristics that minimize the size of the decision tree. The most effective simple heuristic is to choose the column that maximizes the number of distinct constructors, thereby maximizing the branching factor and reducing tree depth.

2. **Necessity analysis.** A column is "necessary" if there exist values that can only be distinguished by testing that column. Maranget's algorithm prefers to test necessary columns first, avoiding redundant tests.

The resulting algorithm is:

```
compile(occurrence_vector, clause_matrix):
  if clause_matrix is empty:
    FAIL  -- unreachable after exhaustiveness check

  if the first row is all wildcards/variables:
    LEAF(first_row_action, first_row_bindings)

  let col = select_column(clause_matrix)  -- heuristic
  let v = occurrence_vector[col]
  let constructors = head_constructors(clause_matrix, col)

  for each c in constructors:
    let S_c = specialize(clause_matrix, col, c)
    let new_occs = expand(occurrence_vector, col, c)
    branches[c] = compile(new_occs, S_c)

  if constructors is a complete signature for the type:
    return SWITCH(v, branches)  -- no default needed
  else:
    let D = default_matrix(clause_matrix, col)
    let default_occs = occurrence_vector without col
    default_branch = compile(default_occs, D)
    return SWITCH(v, branches, default_branch)
```

### 4.5 Decision Trees vs. Backtracking Automata

There are two main compilation strategies:

**Decision trees** (Augustsson, Maranget):
- Each value component is tested at most once along any path.
- No backtracking: once a constructor test is performed, the result is committed.
- Code size can be exponential in the worst case (when patterns overlap significantly).
- Runtime is optimal: O(size of value) tests.

**Backtracking automata** (Lennart Augustsson's alternative, some OCaml versions):
- Compact representation: clauses are tried in order, with backtracking on failure.
- Linear code size.
- Runtime can be worse: the same test may be repeated after backtracking.

**Trade-off.** For Gomputation, decision trees are the recommended choice because:

- The language targets small embedded programs, so exponential code size is unlikely in practice.
- An interpreted language benefits from minimizing the number of runtime tests.
- Decision trees are simpler to reason about for correctness.
- The implementation target is Go, where switch statements map naturally to decision tree branches.

### 4.6 Compiling Nested Patterns

Nested patterns like `C (D x y) z` are handled naturally by the pattern matrix representation. When the algorithm specializes on constructor `C`, it expands the fields of `C` into new columns. If one of those fields is itself a constructor pattern `D x y`, it will be handled when the algorithm recurses and selects that column for splitting.

No special treatment is needed. The pattern matrix framework reduces nested matching to a sequence of flat constructor tests.

### 4.7 Compiling Literal Patterns

Literal patterns are compiled as equality tests rather than constructor tag checks. Since the set of literals is unbounded, literal pattern columns always require a default branch. The compilation algorithm treats each distinct literal as an analogue of a constructor with zero fields:

```
case n of
  0 -> e_0
  1 -> e_1
  _ -> e_default
```

compiles to:

```
if n == 0 then e_0
else if n == 1 then e_1
else e_default
```

### 4.8 Compiling As-Patterns

As-patterns `x @ p` are compiled by adding a binding `x := v` at the point where value `v` is being matched, then continuing to match `v` against `p`. In the pattern matrix representation, `x @ p` is treated as `p` with an additional binding recorded.

---

## 5. Exhaustiveness Checking

### 5.1 The Problem

Given a data type `D` with constructors `C_1, ..., C_k` and a list of patterns `Pat_1, ..., Pat_n`, determine whether every possible value of type `D` is matched by at least one of the patterns. If not, produce a concrete example of an unmatched value (for error reporting).

### 5.2 Maranget's Usefulness Algorithm

Maranget (2007) gives a clean, recursive algorithm based on the concept of **usefulness**. A pattern `q` is **useful** relative to a pattern matrix `P` if there exists a value matched by `q` that is not matched by any row of `P`.

Exhaustiveness is the dual: a pattern matrix `P` is exhaustive if and only if no wildcard pattern is useful relative to `P`. Equivalently, `P` is exhaustive if the wildcard pattern (or a vector of wildcards) is not useful.

The usefulness predicate `U(P, q)` ("is pattern vector `q` useful relative to matrix `P`?") is defined recursively:

```
U(P, q):
  -- P is an m-by-n pattern matrix
  -- q is a 1-by-n pattern vector

  if n = 0:
    -- base case: zero columns
    return (m == 0)  -- useful only if P has no rows

  let q_1 = first element of q
  let q_rest = remaining elements of q

  case q_1 of:
    C p_1 ... p_a:
      -- q starts with a constructor C of arity a
      let S_C(P) = specialize P by C
      let S_C(q) = (p_1, ..., p_a, q_rest)
      return U(S_C(P), S_C(q))

    _ or x:
      -- q starts with a wildcard/variable
      let Sigma = set of constructors appearing in column 1 of P
      if Sigma is a complete signature for the type:
        -- every constructor appears; test usefulness under each
        return any(U(S_C(P), (_, ..., _, q_rest)) for each C in Sigma)
          -- where _ is repeated arity(C) times
      else:
        -- not all constructors appear; use the default matrix
        let D(P) = default matrix of P (column 1 removed for wildcard rows)
        return U(D(P), q_rest)
```

**Specialization S_C(P):** For each row of P:
- If the first element is `C p_1...p_a`, replace it with `p_1, ..., p_a`.
- If the first element is `_` or `x`, replace it with `a` wildcards.
- If the first element is a different constructor, drop the row.

**Default matrix D(P):** For each row of P:
- If the first element is `_` or `x`, keep the row with the first column removed.
- If the first element is a constructor, drop the row.

**Complete signature.** A set of constructors `Sigma` is a "complete signature" for type `T` if `Sigma` contains all constructors of `T`. For literal types, the signature is never complete (there are infinitely many integers, strings, etc.), so the algorithm always falls through to the default matrix for literals.

### 5.3 Exhaustiveness Check

To check exhaustiveness of a pattern matrix P:

```
exhaustive(P):
  let n = number of columns in P
  let q = (_, _, ..., _)   -- n wildcards
  return not U(P, q)
```

If `U(P, q)` returns true, the wildcard vector is useful --- meaning there exist values not covered by P, and the patterns are non-exhaustive.

### 5.4 Producing Counterexamples

When the exhaustiveness check fails, the user needs to know which values are missing. The usefulness algorithm can be modified to produce a **witness** --- a concrete pattern describing the missing cases.

The modified algorithm, instead of returning a boolean, returns either "not useful" or a **set of value abstractions** (concrete patterns representing the missing cases):

```
missing(P, q):
  if n = 0:
    if m == 0: return {()}  -- the empty tuple is missing
    else: return {}          -- nothing missing

  let q_1 = first element of q

  case q_1 of:
    C p_1 ... p_a:
      let M = missing(S_C(P), (p_1, ..., p_a, q_rest))
      return { C(m_1, ..., m_a, m_rest) | (m_1, ..., m_a, m_rest) in M }

    _ or x:
      let Sigma = head constructors in column 1 of P
      if Sigma is a complete signature:
        return union over all C in Sigma of:
          { C(m_1, ..., m_a, m_rest)
            | (m_1, ..., m_a, m_rest) in missing(S_C(P), (_, ..., _, q_rest)) }
      else:
        let D_missing = missing(D(P), q_rest)
        let unmissing_constrs = Sigma
        let missing_constrs = all_constructors(T) - Sigma
        -- Values missing from default: pair each missing constructor with wildcards
        let from_missing_constrs =
          union over C in missing_constrs of:
            { C(_, ..., _, m_rest) | m_rest in D_missing }
        -- Values missing from specialized:
        let from_present_constrs =
          union over C in unmissing_constrs of:
            { C(m_1, ..., m_a, m_rest)
              | (m_1, ..., m_a, m_rest) in missing(S_C(P), (_, ..., _, q_rest)) }
        return from_missing_constrs ++ from_present_constrs
```

This produces counterexamples like:

```
-- Given:
data List a = Nil | Cons a (List a)

case xs of
  Nil -> ...
  Cons x Nil -> ...

-- Missing: Cons _ (Cons _ _)
```

### 5.5 Error Messages

Good error messages for non-exhaustive patterns should:

1. State which `case` expression is non-exhaustive.
2. List concrete example patterns that are not covered.
3. Limit the number of examples shown (e.g., at most 3--5).
4. Use `_` for positions that can be anything.

Example error message:

```
error: non-exhaustive patterns in case expression at line 42
  missing cases:
    Cons _ (Cons _ _)
```

For redundant patterns:

```
warning: redundant pattern at line 45
  pattern `Nil` is already covered by previous patterns
```

### 5.6 Complexity

The usefulness algorithm runs in time O(m * n * |constructors|^n) in the worst case, where m is the number of clauses, n is the pattern width, and |constructors| is the maximum branching factor. In practice, patterns are small and shallow, so performance is not a concern for the size of programs Gomputation targets.

---

## 6. Redundancy Detection

### 6.1 The Problem

A pattern `Pat_i` in a case expression is redundant if no value reaches it --- every value matched by `Pat_i` is already matched by some earlier pattern `Pat_j` (j < i).

### 6.2 Usefulness-Based Detection

Redundancy detection is the direct application of the usefulness predicate. Pattern `Pat_i` is redundant if and only if it is not useful relative to the matrix formed by patterns `Pat_1, ..., Pat_{i-1}`.

```
redundant(Pat_i, [Pat_1, ..., Pat_{i-1}]):
  let P = matrix formed by [Pat_1, ..., Pat_{i-1}]
  return not U(P, Pat_i)
```

This is checked incrementally: each pattern is tested for usefulness against the patterns above it.

### 6.3 Warning vs. Error

Redundant patterns are a code quality issue, not a soundness issue. The recommended treatment for Gomputation:

- **Warning** for redundant patterns in the default mode.
- **Error** if a strict mode flag is enabled (useful for CI/CD pipelines).
- The wildcard pattern `_` at the end of a complete match should trigger a warning --- it suggests the programmer may not have considered the specific constructors.

### 6.4 Ordering Sensitivity

Redundancy is sensitive to pattern order. Given:

```
case x of
  _     -> e1
  Nil   -> e2
  Cons  -> e3
```

Both `Nil` and `Cons` are redundant because `_` already catches everything. But if the order is:

```
case x of
  Nil   -> e1
  Cons  -> e2
  _     -> e3
```

then `_` is redundant (all constructors are already covered) but `Nil` and `Cons` are not.

### 6.5 Interaction with Exhaustiveness

Redundancy and exhaustiveness are independent properties:

- A pattern set can be exhaustive and contain redundant patterns.
- A pattern set can be non-exhaustive and contain no redundant patterns.
- A pattern set can be both non-exhaustive and contain redundant patterns.

The checker should report both independently: exhaustiveness failure is an error (or a warning that the user should handle), while redundancy is a warning.

---

## 7. Interaction with Indexed Types and GADTs

### 7.1 Why This Matters

The spec (Section 4) commits to the extension path:

```
ADT -> indexed types -> row-indexed computations -> promoted protocol states -> maybe GADTs
```

If GADTs are eventually added, pattern matching must support **type refinement**: matching on a GADT constructor reveals type equalities that refine the types of other variables in scope. This changes both the typing rules for case expressions and the exhaustiveness checker.

### 7.2 The GADT Pattern Matching Problem

Consider a hypothetical future GADT in Gomputation:

```
data Term a where
  LitInt  : Int -> Term Int
  LitBool : Bool -> Term Bool
  Add     : Term Int -> Term Int -> Term Int
  If      : Term Bool -> Term a -> Term a -> Term a
```

When matching on a value of type `Term a`:

```
case t of
  LitInt n  -> ...    -- here, a ~ Int is known
  LitBool b -> ...    -- here, a ~ Bool is known
  Add x y   -> ...    -- here, a ~ Int is known
  If c x y  -> ...    -- here, a is unconstrained (polymorphic)
```

Each branch brings a type equality into scope. The branch body is type-checked in a context extended with that equality.

### 7.3 Impact on Exhaustiveness

With GADTs, some branches may become **impossible** for a given type instantiation. For example, if we match on `Term Int`:

```
case (t : Term Int) of
  LitInt n -> ...
  Add x y  -> ...
  If c x y -> ...
```

The branch `LitBool b` is absent, but the match is still exhaustive, because `LitBool` can only construct `Term Bool`, not `Term Int`. The exhaustiveness checker must account for this: a constructor whose result type is incompatible with the scrutinee type should be excluded from the required coverage.

### 7.4 Karachalias et al.'s Approach (GHC)

Karachalias, Schrijvers, Vytiniotis, and Peyton Jones (ICFP 2015) introduced a pattern-match checking framework for GHC that handles GADTs, type classes, and boolean guards. The key ideas:

1. **Value abstractions.** Instead of working with pattern matrices directly, the algorithm works with **value set abstractions** --- symbolic representations of the set of values that remain to be matched. Each abstraction is a triple `(x, Gamma, Delta)` where `x` is a variable, `Gamma` is a type context, and `Delta` is a set of type constraints.

2. **Constraint-based refinement.** When a GADT constructor is matched, its type equalities are added to the constraint set. If the resulting constraint set is unsatisfiable (detected by the type checker's constraint solver), the constructor is considered **inaccessible** --- it cannot produce a value of the required type --- and is excluded from the exhaustiveness requirement.

3. **Oracle.** The algorithm uses a **satisfiability oracle** (typically the same constraint solver used by the type checker) to determine whether a given set of type constraints is satisfiable. If the constraints for a constructor are unsatisfiable, that constructor's branch is inaccessible.

### 7.5 Design-For Recommendation

For Gomputation's current draft:

- Implement exhaustiveness checking for ordinary ADTs using Maranget's algorithm (Section 5).
- When GADTs are added, extend the algorithm with constraint-based refinement following Karachalias et al.
- The key architectural requirement is that the exhaustiveness checker must be **parameterized by an oracle** that can answer satisfiability queries about type constraints. For ordinary ADTs, this oracle is trivial (all constraints are satisfiable). For GADTs, it delegates to the type checker's constraint solver.

This means the exhaustiveness checker should be designed from the start with a pluggable satisfiability interface, even though the initial implementation uses a trivial one.

---

## 8. Interaction with Computations

### 8.1 The Spec's Position

The spec (v0.2, Section 15.5) states:

> `case` is fundamentally a value-level eliminator. Computation-oriented case notation, if later added, should elaborate through the value/computation core rather than redefine it.

This means that `case` at the core level operates on values and produces values.

### 8.2 Case in Computation Position

Users will inevitably want to write `case` inside `do` blocks where the branches are computations:

```
main := do {
  state <- getState;
  case state of
    Opened -> doSomething
    Closed -> doSomethingElse
}
```

Here, `doSomething` and `doSomethingElse` are computations, not values. The question is how this elaborates.

### 8.3 Desugaring Strategies

**Strategy 1: Value-level case, branches return computations.**

The case expression is value-level. The branches produce values of type `Computation pre post A`, which is itself a value type (a thunk/description of a computation). The surrounding `do` block sequences the result.

```
-- Surface:
do {
  state <- getState;
  case state of
    Opened -> doSomething
    Closed -> doSomethingElse
}

-- Elaboration:
bind getState (\state ->
  case state of
    Opened -> doSomething
    Closed -> doSomethingElse
)
```

Here, `case state of ...` produces a value of type `Computation pre post A`, and this value is the result of the lambda passed to `bind`. Since `bind` expects `a -> Computation r2 r3 b`, and the lambda returns a `Computation`, the types align.

This works because in Gomputation (following the spec's value/computation architecture), `Computation pre post A` is a type --- a value that describes a computation. The case expression produces this value, and `bind` sequences it.

**Strategy 2: Explicit `pure` wrapping for value branches.**

If a branch body is a plain value (not a computation), it could be implicitly wrapped in `pure`:

```
-- Surface:
do {
  x <- getState;
  case x of
    Opened -> 42
    Closed -> 0
}

-- Elaboration:
bind getState (\x ->
  case x of
    Opened -> pure 42
    Closed -> pure 0
)
```

This should be handled by the type checker. If the `case` is checked against `Computation r r Int`, each branch is checked against `Computation r r Int`. A bare value `42` checked against `Computation r r Int` can be elaborated to `pure 42` by a standard bidirectional checking rule for `pure` insertion.

**Strategy 3: Computation-level case as a derived form.**

An explicit computation-level case could be added as syntactic sugar:

```
caseC s of
  Pat_1 -> c_1
  ...
  Pat_n -> c_n

-- desugars to:
bind (pure s) (\s' ->
  case s' of
    Pat_1 -> c_1
    ...
    Pat_n -> c_n
)
```

But this is equivalent to Strategy 1 when the scrutinee is already a value. The `bind (pure s)` is an identity and should be optimized away.

### 8.4 Recommended Approach

**Use Strategy 1 with bidirectional typing.** The case expression is always a value-level eliminator. When the expected type is `Computation pre post A`, the branches are checked against that type. Since `Computation pre post A` is a value type (it classifies descriptions of computations), no special computation-level case form is needed.

This is the simplest approach that:
- Preserves the spec's commitment that `case` is value-level.
- Requires no new syntax or typing rules.
- Relies on the existing bidirectional checking infrastructure.
- Works naturally inside `do` blocks via the `bind` desugaring.

### 8.5 What Haskell, Koka, and Idris Do

**Haskell:** `case` is a value-level expression. In `do` blocks, branches can be `IO` actions (or any monadic value), and the result is sequenced by `>>=`. This is exactly Strategy 1.

**Koka:** `match` is an expression. In effectful contexts, each branch can be an effectful computation. Koka's effect system infers the combined effect of all branches (the union of their effect rows). There is no separate computation-level match.

**Idris:** `case` is an expression. In `do` blocks, branches can be effectful. Idris's dependent types allow more refined patterns but the basic approach is the same as Haskell's.

**PureScript:** Same as Haskell. `case` branches in `do` blocks produce monadic values.

All major systems use Strategy 1. This validates the recommended approach for Gomputation.

### 8.6 Pre/Post Row Constraints

When `case` appears in computation position, there is a subtlety: all branches must agree on the pre/post row transition.

```
case state of
  Opened -> doA    -- Computation r1 r2 A
  Closed -> doB    -- Computation r1 r2 A
```

Both branches must have the same `Computation pre post A` type. This is already enforced by the standard typing rule (all branches must have the same result type). Since `pre` and `post` are part of the type, they must match across branches.

This is a consequence of the spec's requirement (Section 15.5): `case` does not change capability state, because it is a value-level eliminator. The computation described by each branch may change capability state, but all branches must describe the same pre-to-post transition. This is exactly the non-linear branching discipline described in the spec's non-linear effect composition story.

---

## 9. Implementation in Go

### 9.1 AST Representation of Patterns

```go
// Pattern represents a pattern in a case expression.
type Pattern interface {
    patternNode()
}

type WildcardPattern struct{}

type VarPattern struct {
    Name string
}

type ConstructorPattern struct {
    Constructor string
    Args        []Pattern
}

type LiteralPattern struct {
    Value Literal  // Int, String, etc.
}

type AsPattern struct {
    Name    string
    Pattern Pattern
}
```

### 9.2 AST Representation of Case Expressions

```go
type CaseExpr struct {
    Scrutinee Expr
    Branches  []CaseBranch
}

type CaseBranch struct {
    Pattern Pattern
    Body    Expr
}
```

### 9.3 Compiled Decision Trees

The output of pattern match compilation is a decision tree, represented as:

```go
// Decision represents a compiled pattern match.
type Decision interface {
    decisionNode()
}

// Leaf: we have matched; execute the body with the given bindings.
type Leaf struct {
    BodyIndex int               // index into the original branch list
    Bindings  map[string]Access // variable name -> how to access the value
}

// Switch: test the constructor tag of a value.
type Switch struct {
    Scrutinee Access           // what value to test
    Branches  map[string]*SwitchBranch // constructor name -> branch
    Default   Decision         // fallback (nil if exhaustive over constructors)
}

type SwitchBranch struct {
    Bindings []string  // names for the constructor's fields (empty strings for unused)
    Body     Decision
}

// Access describes how to reach a sub-value at runtime.
type Access struct {
    Root      string // the original scrutinee variable
    Path      []int  // sequence of field indices from the root
}
```

### 9.4 Runtime Pattern Matching

At runtime, the evaluator walks the decision tree:

```go
func evalDecision(d Decision, env *Env) (Value, error) {
    switch d := d.(type) {
    case *Leaf:
        // Extend the environment with pattern bindings
        localEnv := env.Extend()
        for name, access := range d.Bindings {
            val := followAccess(access, env)
            localEnv.Bind(name, val)
        }
        return eval(d.Body, localEnv)

    case *Switch:
        // Get the value to test
        val := followAccess(d.Scrutinee, env)
        tag := val.ConstructorTag()

        if branch, ok := d.Branches[tag]; ok {
            // Bind the constructor's fields
            localEnv := env.Extend()
            fields := val.Fields()
            for i, name := range branch.Bindings {
                if name != "" {
                    localEnv.Bind(name, fields[i])
                }
            }
            return evalDecision(branch.Body, localEnv)
        }
        if d.Default != nil {
            return evalDecision(d.Default, env)
        }
        return nil, fmt.Errorf("non-exhaustive match") // unreachable after static check
    }
}

func followAccess(a Access, env *Env) Value {
    val := env.Lookup(a.Root)
    for _, idx := range a.Path {
        val = val.Fields()[idx]
    }
    return val
}
```

### 9.5 Compilation to Go Switch Statements (if generating Go code)

If Gomputation ever compiles to Go code rather than interpreting, pattern matches compile to nested `switch` statements:

```go
// Source:
// case shape of
//   Circle r    -> pi * r * r
//   Rect w h    -> w * h

// Generated Go:
switch shape.Tag {
case "Circle":
    r := shape.Fields[0]
    return pi * r * r
case "Rect":
    w := shape.Fields[0]
    h := shape.Fields[1]
    return w * h
}
```

For nested patterns, the switch statements are nested:

```go
// Source:
// case tree of
//   Leaf x        -> x
//   Node (Leaf a) r -> a
//   Node l r      -> default_val

// Generated Go:
switch tree.Tag {
case "Leaf":
    x := tree.Fields[0]
    return x
case "Node":
    l := tree.Fields[0]
    r := tree.Fields[1]
    switch l.Tag {
    case "Leaf":
        a := l.Fields[0]
        return a
    default:
        return default_val
    }
}
```

### 9.6 Value Representation for Pattern Matching

The runtime value representation must support:

1. **Constructor tag access**: O(1) access to the constructor name or tag.
2. **Field access**: O(1) access to constructor fields by index.
3. **Literal equality**: comparison for literal pattern matching.

A suitable Go representation:

```go
type Value interface {
    // For constructed values
    Tag() string
    Fields() []Value
    // For literals
    LitEquals(other Value) bool
}

type ConstructedValue struct {
    tag    string
    fields []Value
}

func (v *ConstructedValue) Tag() string     { return v.tag }
func (v *ConstructedValue) Fields() []Value { return v.fields }
```

---

## 10. Practical Systems Comparison

### 10.1 GHC (Haskell)

**Pattern grammar:** Variables, constructors (nested), wildcards, as-patterns, literal patterns, view patterns, pattern synonyms, or-patterns (proposal stage), bang patterns (strictness), lazy patterns.

**Compilation:** Historically used Augustsson's algorithm. Modern GHC uses a mixture, with desugarer producing `case` trees. The recent pattern-match checker (Karachalias et al., ICFP 2015; Lower Your Guards, Karachalias and Graf et al., ICFP 2020) uses a constraint-based approach.

**Exhaustiveness:** The current GHC checker (since GHC 8.2, refined in 9.x) uses the "Lower Your Guards" framework. It decomposes each clause into a sequence of guard-like constraints, then checks coverage using a constraint solver. This handles GADTs, type classes, pattern synonyms, boolean guards, and view patterns uniformly.

**Error messages:** GHC reports missing patterns as concrete value suggestions. Error quality has improved significantly with the newer checker.

**Lessons for Gomputation:** The "Lower Your Guards" approach is more general than Maranget's algorithm but significantly more complex. For a language without GADTs, pattern guards, or pattern synonyms, Maranget's algorithm is sufficient and simpler.

### 10.2 OCaml

**Pattern grammar:** Variables, constructors, wildcards, as-patterns, or-patterns, literal patterns, record patterns, exception patterns.

**Compilation:** OCaml uses a highly optimized decision tree compiler based on Maranget's work (Le Fessant and Maranget, 2001). It produces particularly good code for real-world patterns.

**Exhaustiveness:** Uses Maranget's usefulness algorithm. OCaml does not have GADTs with the same type-equality power as GHC, so the exhaustiveness checker is simpler. (OCaml does have GADTs, but their exhaustiveness checking is more conservative.)

**Error messages:** OCaml produces concrete counterexample patterns for non-exhaustive matches. It also warns about redundant patterns.

**Lessons for Gomputation:** OCaml's approach is the gold standard for the ADT-only case. Its exhaustiveness checker and decision-tree compiler are directly applicable to Gomputation.

### 10.3 Rust

**Pattern grammar:** Variables, constructors (enum variants), wildcards, literal patterns, reference patterns, range patterns, struct patterns, tuple patterns, or-patterns, binding modes.

**Compilation:** Rust uses a decision-tree-based approach.

**Exhaustiveness:** Rust requires exhaustive matching for all `match` expressions. Its exhaustiveness checker handles enums, nested patterns, integer ranges, and reference patterns. It uses a variant of Maranget's algorithm extended with range analysis.

**Error messages:** Rust produces excellent error messages, listing concrete missing patterns.

**Lessons for Gomputation:** Rust demonstrates that strict exhaustiveness checking (match must be exhaustive, no implicit fall-through) is ergonomically viable and leads to better code quality. Gomputation should follow this discipline.

### 10.4 Elm

**Pattern grammar:** Variables, constructors, wildcards, literal patterns, as-patterns, tuple patterns. Deliberately minimal.

**Compilation:** Simple decision tree approach.

**Exhaustiveness:** Strict exhaustiveness checking. Elm is notable for its excellent error messages --- it shows the user exactly which patterns are missing, with concrete examples.

**Error messages:** Elm's error messages are widely regarded as the best in the industry for pattern matching. They are clear, specific, and actionable.

**Lessons for Gomputation:** Elm shows that a small pattern language with strict exhaustiveness checking produces a very good user experience. Gomputation should aim for Elm-quality error messages.

### 10.5 PureScript

**Pattern grammar:** Variables, constructors, wildcards, literal patterns, as-patterns, record patterns (row-typed). No or-patterns.

**Compilation:** Standard decision tree approach.

**Exhaustiveness:** Mandatory exhaustiveness checking. PureScript checks exhaustiveness over constructors and produces concrete counterexamples.

**Error messages:** Good quality, similar to Elm's.

**Lessons for Gomputation:** PureScript's row-typed record patterns are relevant if Gomputation adds record types. The rest of PureScript's approach aligns closely with what Gomputation should do.

### 10.6 Summary Table

| Feature | GHC | OCaml | Rust | Elm | PureScript | **Gomputation** |
|---|---|---|---|---|---|---|
| Constructor patterns | yes | yes | yes | yes | yes | **yes** |
| Nested patterns | yes | yes | yes | yes | yes | **yes** |
| Wildcard | yes | yes | yes | yes | yes | **yes** |
| Variable | yes | yes | yes | yes | yes | **yes** |
| As-patterns | yes | yes | yes | yes | yes | **yes (v1)** |
| Literal patterns | yes | yes | yes | yes | yes | **yes** |
| Or-patterns | proposal | yes | yes | no | no | **deferred** |
| Guards | yes | yes | yes | no | yes | **deferred** |
| View patterns | yes | no | no | no | no | **deferred** |
| Exhaustive required | warning | warning | error | error | error | **error** |
| Redundancy check | warning | warning | warning | error | warning | **warning** |
| GADT refinement | yes | partial | N/A | N/A | N/A | **deferred** |

---

## 11. Recommendations for Gomputation

### 11.1 Pattern Grammar: What to Include Now

**Immediate (v0.3):**

1. **Wildcard patterns** (`_`). Essential, trivial to implement.
2. **Variable patterns** (`x`). Essential, trivial to implement.
3. **Constructor patterns** (`C p_1 ... p_n`), including nested. Essential for ADT elimination.
4. **Literal patterns** (`42`, `"hello"`). Important for usability with built-in types.

**Near-term (v0.4):**

5. **As-patterns** (`x @ p`). Useful convenience, low implementation cost.

**Deferred:**

6. Or-patterns, guards, view patterns, record patterns. Wait until the need is clear and the implementation is stable.

### 11.2 Exhaustiveness: Mandatory

Gomputation should **require** exhaustive pattern matching. A non-exhaustive `case` expression is a type error, not a warning. This follows Rust, Elm, and PureScript rather than Haskell and OCaml.

Rationale: Gomputation is designed for safety-critical embedded scripting. Non-exhaustive matches are a source of runtime crashes. The type system should prevent them statically.

If the user wants a partial match, they must include a catch-all branch:

```
case x of
  Opened -> doSomething
  _      -> pure unit    -- explicit catch-all
```

### 11.3 Redundancy: Warning

Redundant patterns should produce a **warning**, not an error. This allows intermediate states during development (e.g., adding a catch-all while refactoring). A strict mode flag can promote this to an error.

### 11.4 Compilation Strategy: Decision Trees

Use Maranget's decision tree algorithm. It is well-understood, produces optimal test sequences, and maps directly to Go switch statements.

### 11.5 Exhaustiveness Algorithm: Maranget's Usefulness

Use Maranget's usefulness algorithm (Section 5.2) for both exhaustiveness and redundancy checking. Produce concrete counterexample patterns for non-exhaustive errors.

Design the checker with a pluggable satisfiability oracle from the start, to prepare for future GADT support. The initial oracle is trivial: all type constraints are satisfiable.

### 11.6 Case in Computation Position

Use the bidirectional typing approach described in Section 8.4. The `case` expression is always value-level. When checked against `Computation pre post A`, each branch is checked against that type. No special computation-level case form is needed.

### 11.7 Implementation Phases

**Phase 1: Core pattern matching.**
- AST representation of patterns (Section 9.1).
- Pattern binding extraction (Section 3.3).
- Well-formedness checking (Section 3.4).
- Flat constructor matching (no nesting) via simple switch.

**Phase 2: Full pattern matching.**
- Nested pattern support via pattern matrix compilation.
- Decision tree generation (Section 4.4).
- Decision tree evaluation (Section 9.4).

**Phase 3: Static analysis.**
- Exhaustiveness checking (Section 5.3).
- Redundancy detection (Section 6.2).
- Counterexample generation (Section 5.4).
- Error message formatting.

**Phase 4: Extensions.**
- As-patterns.
- Literal patterns with range analysis (if needed).
- Pluggable oracle for future GADT support.

### 11.8 Formal Grammar Summary

The complete pattern grammar for Gomputation, in the notation used by the spec:

```
Pat ::= _                               -- wildcard
      | x                               -- variable
      | C Pat*                           -- constructor (possibly nested, arity-checked)
      | lit                              -- literal (Int, String)
      | x @ Pat                          -- as-pattern

CaseExpr ::= case Expr of { Branch+ }

Branch ::= Pat -> Expr
```

Well-formedness conditions:

1. In `C Pat_1 ... Pat_n`, the arity `n` must equal the declared arity of constructor `C`.
2. Variable names within a single pattern are distinct (linearity).
3. The constructor `C` must belong to the algebraic data type of the scrutinee.
4. The set of branches must be exhaustive for the scrutinee's type (static error if not).

### 11.9 Typing Rule Summary

The typing rules for case expressions in bidirectional form:

**Checking mode:**

```
Gamma |- s => D a_1 ... a_m
for each branch (Pat_i -> e_i):
  Pat_i is well-formed for D a_1 ... a_m
  bindings(Pat_i, D a_1 ... a_m) = Delta_i
  Gamma, Delta_i |- e_i <= A
{Pat_1, ..., Pat_n} exhaustive for D a_1 ... a_m
-----------------------------------------------------
Gamma |- (case s of Pat_1 -> e_1 ... Pat_n -> e_n) <= A
```

**Synthesis mode:**

```
Gamma |- s => D a_1 ... a_m
bindings(Pat_1, D a_1 ... a_m) = Delta_1
Gamma, Delta_1 |- e_1 => A
for each subsequent branch (Pat_i -> e_i), i > 1:
  bindings(Pat_i, D a_1 ... a_m) = Delta_i
  Gamma, Delta_i |- e_i <= A
{Pat_1, ..., Pat_n} exhaustive for D a_1 ... a_m
-----------------------------------------------------
Gamma |- (case s of Pat_1 -> e_1 ... Pat_n -> e_n) => A
```

---

## 12. Key References

### Core Theory

1. Lennart Augustsson. "Compiling Pattern Matching." In *Conference on Functional Programming Languages and Computer Architecture*, LNCS 201, pp. 368--381. Springer, 1985. The original algorithm for compiling pattern matching to decision trees.

2. Luc Maranget. "Compiling Pattern Matching to Good Decision Trees." In *Proceedings of the ACM SIGPLAN Workshop on ML*, pp. 35--46. ACM, 2008. The refined decision tree algorithm with column selection heuristics and necessity analysis. The standard reference for practical pattern match compilation.

3. Luc Maranget. "Warnings for Pattern Matching." *Journal of Functional Programming*, 17(3):387--421, 2007. The usefulness algorithm for exhaustiveness and redundancy checking. The standard reference for static analysis of pattern matching.

### GADT Extensions

4. Georgios Karachalias, Tom Schrijvers, Dimitrios Vytiniotis, and Simon Peyton Jones. "GADTs Meet Their Match: Pattern-matching Warnings That Account for GADTs, Guards, and Laziness." In *Proceedings of ICFP*, pp. 424--436. ACM, 2015. The constraint-based pattern-match checker for GHC that handles GADTs and guards.

5. Sebastian Graf, Simon Peyton Jones, and Ryan G. Scott. "Lower Your Guards: A Compositional Pattern-Match Coverage Checker." In *Proceedings of ICFP*. ACM, 2020. The current GHC pattern-match checker, based on decomposing clauses into guards and using a constraint solver.

### Practical Implementations

6. Fabrice Le Fessant and Luc Maranget. "Optimizing Pattern Matching." In *Proceedings of ICFP*, pp. 26--37. ACM, 2001. OCaml's highly optimized pattern match compiler.

7. Peter Sestoft. "ML Pattern Match Compilation and Partial Evaluation." In *Partial Evaluation*, LNCS 1110, pp. 446--464. Springer, 1996. A clear treatment of pattern match compilation with good pedagogical presentation.

### Tutorials and Surveys

8. Simon Peyton Jones. *The Implementation of Functional Programming Languages*, Chapter 5: "Efficient Compilation of Pattern-Matching." Prentice-Hall, 1987. Available online. A classic and accessible treatment of pattern matching compilation.

9. Luc Maranget. "Pattern Matching and Exhaustivity." Course notes, MPRI, Paris, 2021. Recent pedagogical presentation of the algorithms.

### Related Language Designs

10. Rust Reference, Chapter "Patterns." https://doc.rust-lang.org/reference/patterns.html. Rust's pattern matching design, including exhaustiveness requirements.

11. Elm Guide, "Pattern Matching." https://guide.elm-lang.org/types/pattern_matching. Elm's deliberately simple and ergonomic pattern matching.

---

## Relevance to Gomputation

Pattern matching is the elimination form for ADTs --- the spec's most fundamental user-facing data structure. Without a well-specified pattern grammar and exhaustiveness checker, the language cannot safely eliminate algebraic values. This document provides the formal grammar (Section 2), typing rules (Section 3), compilation algorithm (Section 4), and exhaustiveness algorithm (Section 5) needed for the next draft of the spec.

The key design decisions are:

- **Mandatory exhaustiveness** (follow Rust/Elm, not Haskell's warning-only approach). This fits Gomputation's safety-first embedding context.
- **Value-level case with bidirectional typing** for computation-position use. No special computation-level case form is needed.
- **Pluggable satisfiability oracle** in the exhaustiveness checker, to prepare for GADT support without implementing it now.
- **Decision tree compilation** via Maranget's algorithm, mapping naturally to Go switch statements.
