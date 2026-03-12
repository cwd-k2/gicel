# Type Error Reporting and Diagnostics for Row Types and Indexed Effects

One-line description: exhaustive design for error messages, diagnostic architecture, and user-facing reporting in a language with row-typed capability environments, indexed computation sequencing, bidirectional type checking, and row unification.

## Table of Contents

1. Why Error Reporting Is Structurally Difficult Here
2. Taxonomy of Error Categories
3. Error Messages by Category: Examples and Design
4. Row Type Error Reporting
5. Indexed Effect Error Reporting
6. Bidirectional Typing and Error Locality
7. Error Reporting Techniques from the Literature
8. Comparison of Error Reporting Across Languages
9. Error Message Design Principles
10. Implementation Architecture in Go
11. Capability-Specific Error Messages
12. LSP and Tooling Integration
13. Recommendations for Gomputation
14. Key References

---

## 1. Why Error Reporting Is Structurally Difficult Here

Gomputation's type system combines three features that each independently make error reporting hard:

### 1.1 Row Types and the Label Problem

Row-polymorphic systems unify structured types with labeled fields. When unification fails, the failure may involve:

- A single label with mismatched field type (e.g., `db : DB[Opened]` vs `db : DB[Closed]`).
- A label present in one row but absent in another.
- An open row tail that cannot accommodate the required labels.
- A closed row used where an open row is expected, or vice versa.

The difficulty is that rows may have dozens of labels, and the standard "could not match type X with type Y" message produces two enormous type expressions that differ in only one label. PureScript's issue tracker documents this problem extensively (issues #1896, #3392, #3726): users see walls of text and must visually diff two near-identical rows to find the one field that changed. The information-theoretic content of the error is one label; the noise is the entire environment.

### 1.2 Indexed Effects and the Sequencing Problem

The `Computation pre post a` type introduces pre/post state indices that must agree across `bind` chains. When sequencing fails, the error is typically:

> The post-state of computation C1 does not match the pre-state of computation C2.

But which computation is wrong? In a chain of five `bind`s, the error is detected at the join point between two adjacent computations, but the actual mistake might be three steps back -- the user called `dbClose` too early, and the mismatch only manifests later when `dbQuery` demands `DB[Opened]`. This is the "error distance" problem: the detection site diverges from the mistake site.

### 1.3 Higher-Rank Polymorphism and Annotation Gaps

Bidirectional typing requires annotations at rank boundaries. When an annotation is missing, the type checker switches from checking to inference mode, and the resulting error message may be an unresolved existential variable rather than a meaningful "you forgot an annotation" message. The user sees `Could not unify a^42 with Int -> Bool` instead of `This function requires a type annotation because it is used polymorphically`.

### 1.4 The Embedding Dimension

Unlike a standalone language, Gomputation runs inside Go. Error messages must bridge two worlds:

- Source locations in the embedded language's source text.
- Host-registered primitives whose types come from Go code, not from the embedded program.
- Runtime errors that cross the host boundary.

Users are Go programmers first. They expect errors to be structured, greppable, and compatible with standard tooling.

---

## 2. Taxonomy of Error Categories

This section defines every category of error that the Gomputation type checker can produce. The taxonomy is exhaustive with respect to the spec v0.2 type system.

### 2.1 Formation Errors

Errors detected during well-formedness checking of types and rows, before typing rules are applied.

| Code | Error | Description |
|---|---|---|
| F001 | Unbound type variable | A type expression references a variable not in scope. |
| F002 | Unbound row variable | A row tail references a row variable not in scope. |
| F003 | Kind mismatch | A type constructor is applied to an argument of the wrong kind (e.g., `Computation Int Int Int` where `Row` is expected). |
| F004 | Duplicate row label | A row contains the same label more than once. |
| F005 | Non-type in row | A row field payload is not a well-formed type. |
| F006 | Non-row tail | A row extension `{ l : T | e }` where `e` is not of kind `Row`. |
| F007 | Malformed computation type | `Computation` applied to the wrong number or kind of arguments. |

### 2.2 Typing Errors (Values)

Errors in the value-level type checking.

| Code | Error | Description |
|---|---|---|
| T001 | Unbound variable | A term variable not present in the context. |
| T002 | Type mismatch | General mismatch between expected and inferred types. |
| T003 | Not a function | Application `e1 e2` where `e1` does not have a function type. |
| T004 | Argument type mismatch | Application `f x` where `x` has the wrong type for `f`'s domain. |
| T005 | Missing annotation | A higher-rank or polymorphic term requires an annotation that was not provided. |
| T006 | Polymorphism instantiation failure | A `forall`-quantified type cannot be instantiated to satisfy the context. |
| T007 | Non-exhaustive patterns | A `case` expression does not cover all constructors. |
| T008 | Branch type mismatch | Different branches of a `case` produce different types. |
| T009 | Unknown constructor | A constructor name not defined in any `data` declaration. |
| T010 | Constructor arity mismatch | A constructor applied to the wrong number of arguments. |

### 2.3 Typing Errors (Computations)

Errors specific to computation-level type checking.

| Code | Error | Description |
|---|---|---|
| C001 | Pre-state mismatch | A computation's pre-state row does not match the available capability environment. |
| C002 | Post-state mismatch | The post-state of a computation does not match the pre-state expected by the next computation in a `bind` chain. |
| C003 | Capability not available | A computation requires a capability not present in the current row. |
| C004 | Capability state mismatch | A capability is present but in the wrong protocol state. |
| C005 | Result type mismatch | The result type `a` in `Computation pre post a` does not match the expected type in a `bind` continuation. |
| C006 | Bind chain state inconsistency | The composed row transitions in a `bind` chain are internally inconsistent. |
| C007 | Pure used as computation | A pure value used where a computation is expected, or vice versa. |

### 2.4 Row Unification Errors

Errors arising from the row unification algorithm (spec v0.2 Section 16).

| Code | Error | Description |
|---|---|---|
| R001 | Row label type conflict | Two rows share a label but assign it incompatible types. |
| R002 | Missing row label | A required label is absent from the row. |
| R003 | Extra row label | A row contains a label not present in the expected row (closed row context). |
| R004 | Closed vs open mismatch | A closed row is provided where an open row is expected, or vice versa. |
| R005 | Row occurs check | A row variable occurs in its own solution, producing an infinite row. |
| R006 | Row tail unification failure | Two row tails cannot be unified. |

### 2.5 Declaration Errors

Errors at the declaration level.

| Code | Error | Description |
|---|---|---|
| D001 | Type/definition mismatch | A value definition does not match its declared type annotation. |
| D002 | Duplicate definition | A name defined more than once. |
| D003 | Missing definition | A type annotation without a corresponding definition. |
| D004 | Invalid primitive type | A `primitive` declaration with a type that is not well-formed. |
| D005 | Fixity conflict | Conflicting fixity declarations for the same operator. |

---

## 3. Error Messages by Category: Examples and Design

Each example follows a consistent format inspired by Rust's RFC 1644 and Elm's error message guidelines:

1. A brief header line with error code, severity, and one-sentence summary.
2. Source location with relevant code highlighted.
3. Primary annotation at the point of error.
4. Secondary annotations for context (where constraints originated).
5. An explanation clause with domain-specific language.
6. A suggestion clause when a likely fix is available.

### 3.1 Formation Errors

**F001 -- Unbound type variable**

```
error[F001]: unbound type variable `s`

  --> script.gc:5:22
   |
 5 | query :: Computation { db : DB[s] } { db : DB[s] } Rows
   |                              ^
   |                              type variable `s` is not in scope
   |
   = help: did you mean to quantify over `s`?
           forall s. Computation { db : DB[s] } { db : DB[s] } Rows
```

**F004 -- Duplicate row label**

```
error[F004]: duplicate label `db` in row type

  --> script.gc:3:28
   |
 3 | f :: Computation { db : DB[Opened], db : DB[Closed] } {} Unit
   |                    ---------------  ---------------
   |                    first occurrence  duplicate
   |
   = note: capability labels must be unique within a row
```

**F003 -- Kind mismatch**

```
error[F003]: kind mismatch in computation type

  --> script.gc:7:18
   |
 7 | g :: Computation Int { db : DB[Opened] } Unit
   |                  ^^^
   |                  expected kind `Row`, found kind `Type`
   |
   = note: the first argument to `Computation` must be a capability
           environment (a row), not a value type
   = help: did you mean:
           Computation { ... } { db : DB[Opened] } Unit
```

### 3.2 Value Typing Errors

**T003 -- Not a function**

```
error[T003]: value of type `Int` is not a function

  --> script.gc:10:5
   |
 9 | x := 42
10 | y := x 3
   |      ^ `x` has type `Int`, which cannot be applied to arguments
   |
   = note: function application requires the left-hand side to have
           a function type `A -> B`
```

**T005 -- Missing annotation**

```
error[T005]: type annotation required for higher-rank argument

  --> script.gc:14:10
   |
14 | apply := \f -> f 1
   |           ^
   |           cannot infer the type of `f` without an annotation
   |
   = note: `f` is applied to `1 : Int`, but may need to be
           polymorphic
   = help: add a type annotation:
           apply :: (forall a. a -> a) -> Int
           apply := \f -> f 1
```

**T007 -- Non-exhaustive patterns**

```
error[T007]: non-exhaustive pattern match

  --> script.gc:20:1
   |
20 | describe := \x -> case x of
21 |   Just y  -> "found"
   |
   = note: missing case for constructor `Nothing`
   = help: add the missing case:
           Nothing -> ...
```

### 3.3 Computation Typing Errors

**C003 -- Capability not available**

```
error[C003]: capability `cache` is not available

  --> script.gc:25:3
   |
24 | process :: Computation { db : DB[Opened] } { db : DB[Opened] } Rows
25 |   process := do
26 |     data <- cacheGet "key"
   |             ^^^^^^^^
   |             `cacheGet` requires capability `cache`, which is
   |             not present in the environment
   |
   = note: available capabilities: db
   = help: add `cache : Cache[Ready]` to the pre-state:
           process :: Computation { db : DB[Opened], cache : Cache[Ready] }
                                  { db : DB[Opened], cache : Cache[Ready] }
                                  Rows
```

**C004 -- Capability state mismatch**

```
error[C004]: capability `db` is in state `Closed`, but `Opened` is required

  --> script.gc:30:5
   |
29 | badQuery :: Computation { db : DB[Closed] } { db : DB[Closed] } Rows
30 |   badQuery := do
31 |     rows <- dbQuery (Select "users")
   |             ^^^^^^^
   |             `dbQuery` requires `db : DB[Opened]`
   |             but the current state is `db : DB[Closed]`
   |
   = help: open the database before querying:
           do
             _ <- dbOpen
             rows <- dbQuery (Select "users")
             ...
```

**C002 -- Post-state mismatch in bind chain**

```
error[C002]: state mismatch in computation sequencing

  --> script.gc:38:5
   |
35 | pipeline :: Computation { db : DB[Closed] } { db : DB[Closed] } Rows
36 |   pipeline := do
37 |     _ <- dbOpen           -- post: { db : DB[Opened] }
38 |     _ <- dbClose          -- post: { db : DB[Closed] }
39 |     rows <- dbQuery ...   -- requires: { db : DB[Opened] }
   |             ^^^^^^^
   |             expected pre-state:  { db : DB[Opened] }
   |             actual environment:  { db : DB[Closed] }
   |
   = note: after `dbClose` on line 38, the database is in state
           `Closed`, so `dbQuery` on line 39 cannot proceed
   = help: remove `dbClose` on line 38, or move `dbQuery` before it
```

### 3.4 Row Unification Errors

**R001 -- Row label type conflict**

```
error[R001]: conflicting types for label `db`

  --> script.gc:45:10
   |
44 | open :: Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
45 | f := open >>= \_ -> open
   |                      ^^^^
   |      expected `db : DB[Closed]` in the pre-state
   |      but found `db : DB[Opened]` (from the post-state of the
   |      first `open`)
   |
   = note: `open` transitions `db` from `Closed` to `Opened`;
           calling it again requires `db` to be `Closed`
```

**R002 -- Missing row label**

```
error[R002]: missing label `log` in capability environment

  --> script.gc:50:3
   |
49 | run :: Computation { db : DB[Opened] } { db : DB[Opened] } Unit
50 |   run := do
51 |     _ <- logInfo "starting"
   |          ^^^^^^^
   |          `logInfo` requires capability `log : Logger[Ready]`
   |
   = note: the pre-state { db : DB[Opened] } does not contain `log`
   = help: extend the pre-state to include the `log` capability:
           Computation { db : DB[Opened], log : Logger[Ready] }
                       { db : DB[Opened], log : Logger[Ready] }
                       Unit
```

**R004 -- Closed vs open row mismatch**

```
error[R004]: expected an open row, found a closed row

  --> script.gc:55:12
   |
54 | helper :: forall r. Computation { db : DB[Closed] | r }
55 |                                 { db : DB[Opened] | r } Unit
56 | main := helper @{ db : DB[Closed] }
   |                  ^^^^^^^^^^^^^^^^^^
   |                  this is a closed row; `helper` expects an
   |                  open row with tail variable `r`
   |
   = note: `helper` is polymorphic over additional capabilities via
           row variable `r`
   = help: instantiate with a specific tail:
           helper -- the row variable `r` will be inferred as `{}`
```

---

## 4. Row Type Error Reporting

### 4.1 The Core Problem: Row Diffs

When two large rows fail to unify, showing both complete rows produces noise proportional to the environment size. A database-and-logging application might have six capabilities, but the error involves one label. The user must visually diff two six-entry rows.

**Solution: compute and display the row diff.**

Given two rows R1 and R2 that fail to unify, the error reporter should compute:

1. **Shared labels with matching types.** These are correct and should be elided (or shown minimally as `...`).
2. **Shared labels with conflicting types.** These are the primary error site.
3. **Labels in R1 but not R2.** These are extra labels (in a closed-row context) or needed labels.
4. **Labels in R2 but not R1.** These are missing labels or unexpected labels.

The error message should show only categories 2-4, with an ellipsis indicating that other labels were matched successfully.

**Example of diff-based reporting:**

```
error[R001]: row mismatch

  --> script.gc:12:5
   |
   expected: { db : DB[Opened], ... }
      found: { db : DB[Closed], ... }
             ~~~~~~~~~~~~~~~~~~~~~~~~~~
             label `db` has type `DB[Closed]` but `DB[Opened]`
             is required
   |
   = note: 3 other labels matched successfully (log, cache, metrics)
```

This approach is inspired by PureScript issue #3392, which proposed showing only unique/differing labels rather than complete row structures. The technique reduces a 30-line error to 5 lines while preserving all actionable information.

### 4.2 Open Row vs Closed Row Errors

These errors are conceptually distinct from label-level mismatches. The user needs to understand whether the problem is:

- "You have extra capabilities that this function does not accept" (closed row rejection).
- "This function is designed to work with additional unknown capabilities, but you provided a fixed set" (open row expectation).

**Design principle:** name the structural mismatch, then explain the consequence.

```
error[R004]: closed row cannot satisfy open row constraint

  The function `helper` is polymorphic over additional capabilities:
    helper :: forall r. Computation { db : DB[Closed] | r } ...

  But it was called with a fixed capability set:
    { db : DB[Closed] }

  This means `helper` cannot preserve unknown additional capabilities
  because none were provided.
```

### 4.3 Row Variable Occurs Check

The occurs check error is rare but deeply confusing when it arises. The standard message "infinite type" is meaningless to most users.

```
error[R005]: capability environment would be infinitely recursive

  --> script.gc:22:5
   |
   While unifying row variable `r` with `{ db : DB[Opened] | r }`,
   the variable `r` appears in its own definition.

   This would create an infinite environment:
     { db : DB[Opened], db : DB[Opened], db : DB[Opened], ... }

   = help: this usually indicates a row variable is being used on
           both sides of a computation type in an incompatible way
```

### 4.4 The Permutation Non-Issue

Rows are equal up to label permutation (spec v0.2 Section 16.1). The unification algorithm normalizes label order before comparison. This means the error reporter should never produce:

> Could not match `{ db : DB[Opened], log : Logger[Ready] }` with `{ log : Logger[Ready], db : DB[Opened] }`.

If this error appears, it is a bug in the unification algorithm, not a user error. The implementation must canonicalize row label order before comparison.

---

## 5. Indexed Effect Error Reporting

### 5.1 The Bind Chain Problem

Consider a do-block with five sequential computations:

```
pipeline := do
  _    <- step1    -- Comp r0 r1 Unit
  _    <- step2    -- Comp r1 r2 Unit
  _    <- step3    -- Comp r2 r3 Unit
  _    <- step4    -- Comp r3 r4 Unit
  result <- step5  -- Comp r4 r5 Result
```

The type checker verifies that each `r_{i}` (post-state of step `i`) equals `r_{i+1}` (pre-state of step `i+1`). When `r3 != r4`, the error is detected between step3 and step4. But the actual mistake might be anywhere in the chain:

- step3 produces the wrong post-state.
- step4 requires the wrong pre-state.
- step2 was supposed to transition something that step3 depends on.

**Design principle: show the state flow.**

The error message should reconstruct the capability state at each step and show the trajectory:

```
error[C002]: state mismatch in computation sequencing

  --> script.gc:42:5
   |
39 |   pipeline := do
40 |     _ <- dbOpen           -- { db : Closed } -> { db : Opened }
41 |     _ <- dbClose          -- { db : Opened } -> { db : Closed }
42 |     _ <- logInit          -- { db : Closed, log : _ } -> ...
43 |     rows <- dbQuery ...   -- requires { db : Opened }
   |             ^^^^^^^
   |
   state flow:
     line 40: db: Closed -> Opened    (ok)
     line 41: db: Opened -> Closed    (ok)
     line 42: log: _ -> Ready         (ok)
     line 43: db: expected Opened, have Closed   <-- mismatch
   |
   = note: `dbClose` on line 41 transitioned `db` to `Closed`,
           but `dbQuery` on line 43 requires `db : DB[Opened]`
   = help: remove `dbClose` on line 41, or re-open the database
           before querying
```

The "state flow" section is the key innovation. It turns a point error into a trajectory, letting the user see where the state went wrong.

### 5.2 Protocol Violation Errors

When a capability has a well-known protocol (like `Closed -> Opened -> Closed`), the error message should reference the protocol:

```
error[C004]: protocol violation for capability `db`

  The `db` capability follows the protocol:
    Closed --(dbOpen)--> Opened --(dbClose)--> Closed

  At line 43, the computation `dbQuery` requires state `Opened`,
  but `db` is in state `Closed`.

  State history:
    line 40: dbOpen    Closed -> Opened
    line 41: dbClose   Opened -> Closed
    line 43: dbQuery   requires Opened   <-- violation
```

This requires the host to register protocol metadata alongside primitive types, which is a small addition to the host boundary API that yields a large improvement in error quality.

### 5.3 Capability Lifecycle Errors

A distinct class of errors involves capabilities that should have been introduced or consumed:

```
error[C003]: capability `log` appears in pre-state but not in
             post-state

  --> script.gc:50:1
   |
50 | cleanup :: Computation { db : DB[Opened], log : Logger[Ready] }
51 |                        { db : DB[Closed] }
52 |                        Unit
   |
   = note: capability `log` is required in the pre-state but absent
           from the post-state
   = help: if `log` should be preserved, add it to the post-state:
           { db : DB[Closed], log : Logger[Ready] }
           if `log` should be consumed, use an explicit close
           operation
```

### 5.4 Intermediate State Reconstruction

When a bind chain error is detected, the checker can reconstruct the intermediate state at each step. This is possible because each primitive operation has a known type `Computation pre post a`, and the bind rule composes these transitions sequentially.

**Algorithm for state reconstruction:**

1. Start with the declared (or inferred) pre-state of the do-block.
2. For each statement in the do-block, apply the post-state of the current computation to get the new environment.
3. Record the state at each step.
4. When unification fails between adjacent steps, report the accumulated state history.

This is a diagnostic-time computation, not part of the core type checker. It runs only when an error is detected, using the information already available from the type checking pass.

---

## 6. Bidirectional Typing and Error Locality

### 6.1 How Checking Mode Improves Errors

The bidirectional design (Dunfield-Krishnaswami style) naturally produces better-localized errors than pure inference. The mechanism is direct:

- **Checking mode** carries an expected type downward. When a term fails to check against the expected type, the error is at the term itself, not at some distant unification point.
- **Inference mode** propagates types upward. When inference fails, the error may be at the use site rather than the definition site.

For Gomputation, this means:

1. **Do-block checking.** When a do-block has a declared type `Computation pre post a`, the checker can propagate `pre` into the first statement, propagate intermediate states forward, and check the last statement's result against `a`. Errors are localized to the specific statement that violates the type.

2. **Lambda checking.** When `\x -> e` is checked against `A -> B`, the error for `e` is localized with `B` as the expected type. Without the checking mode, the error would be "could not unify a^1 with B" at the call site, which is less informative.

3. **Annotation checking.** When a definition has a type annotation `f :: T`, the body is checked against `T`. The error message can say "the body of `f` has type S, but the annotation declares type T" with both the annotation and the body highlighted.

### 6.2 The Expected vs Found Pattern

The bidirectional design produces a natural "expected vs found" structure in every error:

- **Expected type**: the type propagated downward by checking mode (from annotations, function argument types, bind chain state).
- **Found type**: the type synthesized upward by inference mode (from the term itself).

This pattern should be the backbone of every type mismatch error:

```
error[T002]: type mismatch

  --> script.gc:15:10
   |
14 | f :: Int -> Bool
15 | f x := x + 1
   |        ^^^^^
   |        expected: Bool
   |          found: Int
   |
   = note: the return type of `f` is declared as `Bool` on line 14,
           but the body `x + 1` has type `Int`
```

### 6.3 Propagating Expected Types Through Bind Chains

In a do-block with type `Computation pre post a`, the checker can propagate the expected types at each step:

1. The first computation is checked against pre-state `pre`.
2. Each subsequent computation inherits the post-state of its predecessor as its pre-state.
3. The final computation's result type is checked against `a`.

When a mismatch occurs at step `i`, the expected pre-state for step `i` is the post-state of step `i-1`, which is a concrete row (not a unification variable). This means the error message can name both the expected and actual states without ambiguity.

### 6.4 When to Report "Missing Annotation"

The transition from checking to inference mode is a natural boundary where annotations may be needed. The checker should detect this transition and, when inference fails or produces an ambiguous result, suggest an annotation rather than reporting a unification failure:

```
-- Instead of:
error: could not unify a^17 with forall r. Computation { db : DB[Closed] | r } ...

-- Report:
error[T005]: type annotation required

  --> script.gc:20:1
   |
20 | handler := \f -> bind (f ()) (\x -> pure x)
   |             ^
   |             the type of `f` cannot be inferred; it may need
   |             to be polymorphic
   |
   = help: add a type annotation for `handler`:
           handler :: (forall r. Unit -> Computation { ... | r }
                                                    { ... | r } a)
                   -> Computation { ... } { ... } a
```

---

## 7. Error Reporting Techniques from the Literature

### 7.1 Hazel: Marking Errors and Continuing

Hazel's marked lambda calculus (Zhao, Mao, Cimini, and Omar; POPL 2024) introduces a total type error localization and recovery system. The key ideas:

**Non-empty holes.** When the type checker detects an error, it wraps the problematic expression in a "non-empty hole" -- a marker that records the error but allows type checking to continue. The hole has a gradual type that can interact with surrounding code, enabling the checker to find additional errors downstream.

**Bidirectional marking.** The system is bidirectionally typed, so localization decisions are systematically predictable based on local information flow. Errors arising from inconsistent unification constraints are localized to holes using traced provenances, rather than attributed to arbitrary expressions.

**Relevance to Gomputation.** This approach is directly applicable. When a row unification fails at step 3 of a bind chain, the checker can mark the error, assume a "best guess" for the post-state, and continue checking steps 4 and 5. This finds multiple errors in a single pass rather than stopping at the first.

### 7.2 Error Slicing

Type error slicing (Haack and Wells, 2004; Rahli et al. with Skalpel for SML) identifies a minimal set of program locations that contribute to a type error. Rather than blaming one point, it shows all points and only the points that are necessary for the error to exist.

**The slice definition.** A type error slice is a subset S of program locations such that (a) removing any location from S eliminates the type error, and (b) S is minimal -- no proper subset of S is also a slice.

**Why it matters for rows.** In a row-polymorphic system, a label mismatch might depend on:
- The type annotation that declares the pre-state.
- The primitive operation that produces a conflicting post-state.
- The variable binding that introduces the label into scope.

An error slice would highlight all three locations, not just the unification failure point.

**Practical tradeoff.** Full error slicing is expensive (potentially exponential). For Gomputation, a pragmatic approximation is to track constraint provenance (where each unification constraint originated) and report the two or three most relevant source locations.

### 7.3 Helium: Specialized Heuristics

Helium (Heeren, Leijen, and van IJzendoorn; Haskell Workshop 2003) is a Haskell compiler designed for teaching. Its approach:

1. **Type graph representation.** Rather than a sequence of unification steps, Helium builds a graph where nodes are type variables and edges are constraints. When the graph is inconsistent, multiple possible error locations exist.

2. **Heuristics for blame assignment.** Helium applies heuristics to decide which edge (constraint) is most likely the user's mistake. Examples:
   - If a student writes `"c"` (string) where `'c'` (character) is expected, a heuristic recognizes the pattern and suggests the fix.
   - If a function is applied to too many arguments, a heuristic counts the arguments and suggests the likely missing parentheses.

3. **Relevance to Gomputation.** Capability-aware heuristics could be powerful:
   - If `db : DB[Closed]` where `db : DB[Opened]` is expected, suggest `dbOpen`.
   - If a capability is missing, suggest adding it to the pre-state annotation.
   - If capabilities appear in the wrong order, suggest reordering operations.

### 7.4 Constraint Provenance Tracking

Bhanuka et al. (OOPSLA 2023, "Getting into the Flow") propose explaining type errors as faulty data flows. Each constraint records where it originated and how types flow through the program.

**The approach:**
1. During type inference, annotate every constraint with its source location and the typing rule that generated it.
2. When constraints conflict, trace back through the provenance chain to find the program locations responsible.
3. Present the error as a data flow: "type `Int` flows from expression `x + 1` on line 5, but type `Bool` is required by the return type declared on line 3."

**For Gomputation, this translates to capability state flow tracking:** "state `DB[Closed]` flows from `dbClose` on line 10, but state `DB[Opened]` is required by `dbQuery` on line 12." The state flow is a special case of data flow where the "data" is the capability state index.

---

## 8. Comparison of Error Reporting Across Languages

### 8.1 Comparative Table

| Dimension | PureScript | Elm | Koka | Rust | Idris 2 | GHC Haskell |
|---|---|---|---|---|---|---|
| **Row types** | Yes (records + effects) | No | Yes (effects only) | No | No (indexed types instead) | Limited (via extensions) |
| **Row diff in errors** | Proposed, partially implemented | N/A | Not implemented | N/A | N/A | N/A |
| **Error tone** | Technical, "Could not match..." | Conversational, "I see..." | Technical, compact | Technical, structured | Technical, terse | Technical, verbose |
| **Source annotations** | Single span | Multi-line context | Single span | Multi-span with labels | Single span | Multi-span (recent versions) |
| **Suggestions** | Rare | Frequent, actionable | Rare | Frequent, with code | Rare | Occasional (GHC 9+) |
| **Error codes** | No | No | No | Yes (E0308 etc.) | No | No |
| **Color/formatting** | Terminal color | Custom ANSI formatting | Terminal color | Rich ANSI with labels | Terminal color | Terminal color |
| **Indexed type errors** | N/A | N/A | N/A | Trait-based, good | Direct, sometimes cryptic | Type family errors, often poor |

### 8.2 PureScript: Row Types Done, Errors Lagging

PureScript has the most mature row type system among mainstream functional languages. Its row errors show the full "Could not match type (...) with type (...)" pattern, printing complete row structures. The community has long recognized this is inadequate for large rows (issue #3392). The proposed fix -- computing row diffs and showing only differing labels -- has been partially explored but not fully implemented as of early 2026.

**Lessons for Gomputation:**
- Implement row diffing from day one. It is cheaper to build it into the error reporter than to retrofit it later.
- PureScript's "Could not match" framing is technically precise but unhelpful. Prefer domain-specific language: "capability `db` is in state `Closed` but `Opened` is required."

### 8.3 Elm: Error Tone Without Row Complexity

Elm is famous for error message quality but has no row types. Its contribution is the design philosophy:

1. **First person.** "I see a problem" rather than "Error:". This is a stylistic choice that reduces intimidation for beginners.
2. **Contextual help.** Errors show the relevant source code and explain what the compiler expected.
3. **Progressive disclosure.** Brief message first, optional `--explain` for depth.
4. **Suggestions.** Whenever possible, suggest a concrete fix.

**Lessons for Gomputation:**
- Adopt the progressive disclosure pattern: brief terminal output, detailed JSON/LSP output.
- Adopt the suggestion pattern wherever the fix is unambiguous.
- Decline the first-person tone. Gomputation targets Go developers who expect terse, structured output, not conversational prose.

### 8.4 Koka: Effect Rows with Minimal Error Investment

Koka's error messages follow a consistent `inferred effect: X / expected effect: Y` pattern. The messages are compact and technically accurate but provide little contextual help. When effect rows grow large, the same readability problems as PureScript apply.

Koka's unique challenge is that it permits duplicate labels in effect rows (unlike Gomputation, which forbids duplicates). This makes Koka's error messages harder to interpret because the user must reason about shadowing.

**Lessons for Gomputation:**
- The `inferred / expected` pattern is a good structural template. Adopt it.
- Forbidding duplicate labels (as the spec commits) eliminates an entire class of confusing errors.

### 8.5 Rust: The Gold Standard for Structured Errors

Rust's error reporting (RFC 1644) is the most sophisticated among production languages:

1. **Error codes.** Every error has a stable code (E0308 for type mismatch, etc.) that can be looked up.
2. **Primary and secondary labels.** The primary label identifies the error site; secondary labels explain why (e.g., showing the annotation that declares the expected type).
3. **Multi-span flow.** Labels create a visual "flow" from cause to effect.
4. **Notes and suggestions.** Structured `= note:` and `= help:` lines.
5. **Color without dependency.** Errors are readable without color (using `^^^` and `---` underlines).

**Lessons for Gomputation:**
- Adopt error codes. They cost nothing to implement and provide a stable reference for documentation and search.
- Adopt the primary/secondary label pattern. It maps directly to bidirectional typing's "expected vs found" structure.
- Adopt `= note:` and `= help:` as structured suffixes.

### 8.6 Idris 2: Indexed Types and the Unification Dump

Idris 2 has the most relevant type system (dependent types with indexed families), but its error messages are often opaque. A common complaint is error messages of the form:

> Can't unify `Vect (S (S n))` with `Vect (plus n 2)`.

The user must know that `S (S n)` and `plus n 2` are definitionally equal only after normalization, and that the type checker did not normalize far enough. This is the "error distance" problem in its purest form: the mistake is the non-obvious relationship between two index expressions, not a simple wrong type.

**Lessons for Gomputation:**
- Gomputation's indices are simpler than Idris's (row states, not arbitrary terms), so the error messages can be more concrete.
- Normalize capability state indices before comparison. Never show raw unification variables in user-facing messages.
- When comparing states, use the capability's name and state label, not the internal type representation.

---

## 9. Error Message Design Principles

### 9.1 The Newspaper Test

Can a Go developer who has never seen the Gomputation spec understand the error after reading it once? This does not mean dumbing down the message. It means:

- Use domain vocabulary ("capability", "state", "opened", "closed") rather than type-theoretic vocabulary ("row unification", "existential variable", "kind mismatch").
- Show the concrete labels and states, not the abstract types.
- Show what the user wrote, not what the type checker computed.

### 9.2 The Actionability Principle

Every error message should answer three questions:

1. **What happened?** (One sentence.)
2. **Where?** (Source location with code context.)
3. **What can the user do?** (Suggestion or explanation.)

If the third question has no answer, the error message is incomplete.

### 9.3 Progressive Disclosure

Error information has three tiers:

**Tier 1: Summary.** The error code, one-line description, and primary source location. This is what appears in a terminal or IDE problem list.

```
error[C004]: capability `db` is in state `Closed`, expected `Opened` (script.gc:31)
```

**Tier 2: Context.** The source code snippet, primary and secondary labels, and the "expected vs found" comparison. This is the default terminal output.

**Tier 3: Explanation.** The full diagnostic with notes, suggestions, protocol history, and state flow. This is available via `--explain C004` or in hover tooltips in an LSP-aware editor.

### 9.4 Terminology Consistency

Every error message should use the same vocabulary:

| Concept | Standard term | Avoid |
|---|---|---|
| Row | capability environment | row type, record type, env |
| Label | capability name | field, key, label |
| State index | state | index, parameter, argument |
| Pre-state | required environment | input row, pre-condition |
| Post-state | resulting environment | output row, post-condition |
| Bind chain | computation sequence | monadic chain, do-block pipeline |
| Row variable tail | additional capabilities | row tail, rest |

### 9.5 Formatting Rules

1. **Error codes are always present.** Format: `error[XNNN]` where `X` is the category letter and `NNN` is the number.
2. **Source locations use the `file:line:col` format.** Same as Go compiler errors.
3. **Underlines use `^` for primary errors and `-` for secondary context.** ASCII-safe; no Unicode dependency.
4. **Notes use `= note:` prefix.** Help uses `= help:` prefix.
5. **Color is additive, not required.** Every error must be readable in plain monochrome text.
6. **Type expressions in errors are normalized.** Row labels are sorted alphabetically. Unification variables are replaced with descriptive names or elided. Forall-bound variables use short names.

---

## 10. Implementation Architecture in Go

### 10.1 Diagnostic Data Type

The core diagnostic type is a structured value, not a string. Formatting is a separate concern from construction.

```go
// Severity classifies the diagnostic level.
type Severity int

const (
    Error   Severity = iota
    Warning
    Info
    Hint
)

// Span identifies a contiguous region of source text.
type Span struct {
    File   string
    Start  Position
    End    Position
}

// Position is a line:column pair (1-indexed).
type Position struct {
    Line   int
    Column int
}

// Label annotates a source span with a message.
type Label struct {
    Span    Span
    Message string
    Primary bool // true for the main error site, false for context
}

// Diagnostic is a single compiler diagnostic.
type Diagnostic struct {
    Severity Severity
    Code     string   // e.g. "C004"
    Summary  string   // one-line summary
    Labels   []Label  // source annotations (primary first)
    Notes    []string // additional context (= note: lines)
    Helps    []string // suggested fixes (= help: lines)
}
```

### 10.2 Error Accumulation

The type checker should accumulate diagnostics rather than failing on the first error.

```go
// Diagnostics collects errors during a type checking pass.
type Diagnostics struct {
    items []Diagnostic
}

// Report adds a diagnostic.
func (d *Diagnostics) Report(diag Diagnostic) {
    d.items = append(d.items, diag)
}

// HasErrors returns true if any error-level diagnostic exists.
func (d *Diagnostics) HasErrors() bool {
    for _, item := range d.items {
        if item.Severity == Error {
            return true
        }
    }
    return false
}
```

### 10.3 Error Recovery Strategy

When a type error is detected mid-checking, the checker must decide how to proceed. The strategy depends on the error category:

**Recoverable errors (continue checking with a sentinel):**
- Row label type conflict (R001): use the expected type and continue.
- Missing row label (R002): assume the label is present with the expected type.
- Capability state mismatch (C004): use the expected state and continue.
- Type mismatch (T002): use the expected type and continue.

**Non-recoverable errors (stop checking this branch):**
- Kind mismatch (F003): the type is malformed; cannot assign any meaningful type.
- Row occurs check (R005): the row is infinite; no reasonable recovery.
- Not a function (T003): cannot determine argument types.

The recoverable strategy inserts "error holes" (in the Hazel sense) that allow downstream checking to proceed:

```go
// checkComputation checks a computation term against an expected type.
// On error, it records the diagnostic and returns the expected type
// as a recovery value, enabling continued checking.
func (tc *TypeChecker) checkComputation(
    term ast.Expr,
    expectedPre Row,
    expectedPost Row,
    expectedResult Type,
    diags *Diagnostics,
) Type {
    // ... checking logic ...
    if !unifyRows(actualPre, expectedPre) {
        diags.Report(Diagnostic{
            Code:    "C001",
            Summary: "pre-state mismatch",
            // ... labels, notes, helps ...
        })
        // Recovery: proceed as if the pre-state matched.
        // This may produce secondary errors, which are marked
        // as potentially cascading.
        return expectedResult
    }
    // ...
}
```

### 10.4 Cascading Error Suppression

When an error recovery produces downstream errors, those downstream errors may be artifacts of the recovery rather than genuine mistakes. The checker should track this:

```go
// Diagnostic may be marked as potentially cascading.
type Diagnostic struct {
    // ... fields from above ...
    MaybeCascading bool
}
```

In the default output mode, cascading errors are suppressed. In verbose mode (`--all-errors`), they are shown with a note:

```
note: this error may be a consequence of the error on line 31
```

### 10.5 Row Diff Computation

The row diff is a pure function on two rows:

```go
// RowDiff computes the structural difference between two rows.
type RowDiff struct {
    // Labels present in both rows with matching types.
    Matching []Label

    // Labels present in both rows with conflicting types.
    Conflicting []LabelConflict

    // Labels present in the first row but not the second.
    OnlyInFirst []LabelEntry

    // Labels present in the second row but not the first.
    OnlyInSecond []LabelEntry

    // Tail comparison.
    TailMismatch *TailMismatch // nil if tails match
}

type LabelConflict struct {
    Name     string
    TypeInA  Type
    TypeInB  Type
}

type LabelEntry struct {
    Name string
    Type Type
}

type TailMismatch struct {
    TailA RowTail // Closed, Open(var), or None
    TailB RowTail
}

// DiffRows computes the difference between two rows.
func DiffRows(a, b Row) RowDiff {
    // 1. Flatten both rows to sorted label lists + tails.
    // 2. Walk both lists in sorted order.
    // 3. Classify each label as matching, conflicting, or one-sided.
    // 4. Compare tails.
    // ...
}
```

This diff is then consumed by the error formatter to produce concise messages.

### 10.6 State Flow Reconstruction

For bind chain errors, the checker can reconstruct the state at each step:

```go
// StateStep records the capability state at one point in a bind chain.
type StateStep struct {
    Span     Span
    ExprDesc string // e.g. "dbOpen", "dbQuery (Select ...)"
    PreState Row
    PostState Row
}

// ReconstructStateFlow builds the state history for a do-block.
func ReconstructStateFlow(
    doBlock ast.DoBlock,
    initialPre Row,
    stepTypes []ComputationType,
) []StateStep {
    steps := make([]StateStep, len(doBlock.Stmts))
    current := initialPre
    for i, stmt := range doBlock.Stmts {
        steps[i] = StateStep{
            Span:      stmt.Span(),
            ExprDesc:  describeExpr(stmt.Expr),
            PreState:  current,
            PostState: stepTypes[i].Post,
        }
        current = stepTypes[i].Post
    }
    return steps
}
```

### 10.7 Output Formats

The diagnostic system should support multiple output formats:

```go
// Formatter renders diagnostics in a specific format.
type Formatter interface {
    Format(diag Diagnostic) string
}

// TerminalFormatter produces human-readable terminal output with
// optional ANSI color codes.
type TerminalFormatter struct {
    Color   bool
    Verbose bool
}

// JSONFormatter produces machine-readable JSON output suitable for
// integration with editors and CI systems.
type JSONFormatter struct{}

// LSPFormatter produces LSP-compatible diagnostics.
type LSPFormatter struct{}
```

**Terminal format** (default):
```
error[C004]: capability `db` is in state `Closed`, expected `Opened`

  --> script.gc:31:5
   |
31 |     rows <- dbQuery (Select "users")
   |             ^^^^^^^
   |             requires `db : DB[Opened]`
   |
   = note: `db` is in state `Closed` after `dbClose` on line 30
   = help: open the database before querying
```

**JSON format** (for tooling):
```json
{
  "severity": "error",
  "code": "C004",
  "summary": "capability `db` is in state `Closed`, expected `Opened`",
  "labels": [
    {
      "file": "script.gc",
      "start": {"line": 31, "column": 5},
      "end": {"line": 31, "column": 12},
      "message": "requires `db : DB[Opened]`",
      "primary": true
    }
  ],
  "notes": [
    "`db` is in state `Closed` after `dbClose` on line 30"
  ],
  "helps": [
    "open the database before querying"
  ]
}
```

**LSP format:** The JSON output maps directly to `textDocument/publishDiagnostics` notifications. The `Diagnostic` struct maps to LSP's `Diagnostic` type, with `relatedInformation` populated from secondary labels.

### 10.8 Constraint Provenance

Every unification constraint should carry provenance:

```go
// Provenance records where a type constraint originated.
type Provenance struct {
    Span    Span
    Reason  string // human-readable description
    Rule    string // e.g. "bind-post-pre", "app-arg", "annotation"
}

// Constraint is a type equality or row equality to be solved.
type Constraint struct {
    LHS        Type
    RHS        Type
    Provenance Provenance
}
```

When unification fails, the provenance of the conflicting constraint is used to produce the secondary labels in the error message. This is the mechanism that connects "where the error is detected" to "where the constraint originated."

---

## 11. Capability-Specific Error Messages

### 11.1 Domain-Specific Vocabulary

Gomputation is a capability-oriented language. Error messages should use capability vocabulary, not generic type-theoretic vocabulary:

| Generic (avoid) | Capability-specific (prefer) |
|---|---|
| "Row R1 does not unify with row R2" | "The required capabilities do not match the available capabilities" |
| "Label `db` has type `DB[Closed]` in R1 but `DB[Opened]` in R2" | "Capability `db` is in state `Closed`, but `Opened` is required" |
| "Row variable `r` cannot be unified with `{}`" | "No additional capabilities are available to satisfy the open environment" |
| "Kind mismatch: expected Row, got Type" | "Expected a capability environment, found a value type" |
| "Unification failure at constraint C47" | "State mismatch in computation sequencing at line 38" |

### 11.2 Contextual Capability Information

Error messages should include what is available, not just what is missing:

```
error[C003]: capability `cache` is not available

  available capabilities: db (Opened), log (Ready)
  required by `cacheGet`: cache (Ready)
```

This pattern -- listing the available capabilities -- is analogous to "did you mean?" suggestions but applied to the capability environment.

### 11.3 Capability Discovery Suggestions

When a capability is missing, and the host has registered a primitive that introduces it, suggest the primitive:

```
error[C003]: capability `db` is not available

  = help: use `dbOpen` to obtain the `db` capability:
          dbOpen :: Computation { db : DB[Closed] | r }
                                { db : DB[Opened] | r } Unit
```

This requires the error reporter to have access to the host primitive registry, which is a reasonable coupling for an embedded language.

### 11.4 Fuzzy Matching for Unknown Capabilities

When a user references a capability that does not exist at all (not in any registered primitive), apply edit-distance matching:

```
error[C003]: unknown capability `databse`

  = help: did you mean `database`?
          Available capabilities: database, logger, cache
```

### 11.5 Protocol State Suggestions

When the capability state is wrong, suggest the operation that performs the required transition:

```
error[C004]: capability `db` is in state `Closed`, expected `Opened`

  = help: transition `db` to `Opened` using `dbOpen`:
          _ <- dbOpen
```

This is the most valuable class of suggestions. It directly tells the user what code to write. Implementation requires a lookup table from `(capability, currentState, requiredState)` to `primitiveName`, which the host boundary API can provide.

---

## 12. LSP and Tooling Integration

### 12.1 LSP Diagnostic Mapping

The Gomputation diagnostic type maps to LSP's `Diagnostic` as follows:

| Gomputation field | LSP field |
|---|---|
| `Severity` | `severity` (1=Error, 2=Warning, 3=Info, 4=Hint) |
| `Code` | `code` (string) |
| `Summary` | `message` |
| `Labels[0]` (primary) | `range` |
| `Labels[1:]` (secondary) | `relatedInformation` |
| `Notes` | Appended to `message` |
| `Helps` | `codeAction` triggers |

### 12.2 Code Actions

Suggestions in `= help:` lines can be implemented as LSP code actions:

- **Add capability to pre-state**: inserts the missing label into the type annotation.
- **Add missing operation**: inserts a `bind` step with the suggested primitive.
- **Fix capability state**: inserts the required state transition operation.
- **Add type annotation**: inserts a type annotation for an unannotated binding.

### 12.3 Hover Information

On hover over a do-block statement, the LSP server should show the capability state at that point:

```
-- Hovering over `dbQuery ...` on line 43:
Capability state at this point:
  db:    DB[Opened]
  log:   Logger[Ready]
  cache: Cache[Ready]

Type: Computation { db : DB[Opened], log : Logger[Ready], cache : Cache[Ready] }
                  { db : DB[Opened], log : Logger[Ready], cache : Cache[Ready] }
                  Rows
```

This is a direct application of the state flow reconstruction algorithm from Section 10.6.

### 12.4 Inlay Hints

The LSP server can provide inlay hints showing the state transitions:

```
_ <- dbOpen     -- { db: Closed -> Opened }
_ <- logInit    -- { log: _ -> Ready }
rows <- dbQuery -- { db: Opened (preserved) }
```

These are not error messages but preventive diagnostics -- they help the user understand the state flow before errors arise.

---

## 13. Recommendations for Gomputation

### 13.1 Build the Diagnostic Infrastructure First

Before implementing the type checker, define the `Diagnostic`, `Span`, `Label`, and `Formatter` types. Every subsequent component (parser, kind checker, type checker, row unifier) should produce `Diagnostic` values, not string error messages. This is a one-time investment that pays off in every error message.

### 13.2 Implement Row Diffs from Day One

Do not ship a type checker that prints full rows in error messages. Implement `DiffRows` as part of the initial row unification code. The diff is simple (flatten-sort-walk), and the payoff in error readability is immediate.

### 13.3 Track Constraint Provenance

Every unification constraint should record its source span and a human-readable reason. This is a small overhead per constraint (one pointer and one string) that enables multi-span errors and data-flow explanations.

### 13.4 Exploit Bidirectional Checking for Error Placement

The bidirectional design naturally places errors at the right location. Lean into this:
- Check do-blocks against their declared type, propagating expected states forward.
- Check function bodies against their annotated return type.
- Report "expected vs found" at the point where checking fails.

### 13.5 Reconstruct State Flows for Bind Chain Errors

When a bind chain error is detected, reconstruct the capability state at each step and include the state flow in the error message. This turns a point error into a trajectory that the user can reason about.

### 13.6 Register Protocol Metadata at the Host Boundary

Extend the host primitive registration API to include:
- Protocol descriptions (which state transitions each primitive performs).
- Inverse lookup (given a `(capability, fromState, toState)` triple, which primitive performs it).

This metadata powers the most valuable error suggestions: "use `dbOpen` to transition `db` from `Closed` to `Opened`."

### 13.7 Implement Error Recovery with Hazel-Style Holes

When a type error is detected, insert an error hole with the expected type and continue checking. This finds multiple errors per pass and avoids the frustrating edit-check-fix-repeat cycle where each run reveals only one error.

Mark potentially cascading errors and suppress them in default output.

### 13.8 Adopt Error Codes

Assign a stable code to every diagnostic. The cost is negligible (an enum and a format string). The benefits are:
- Users can search for the code online.
- Documentation can provide detailed explanations per code.
- CI systems can filter or ignore specific codes.
- The `--explain XNNN` flag provides detailed documentation.

### 13.9 Support Three Output Formats

1. **Terminal** (default): human-readable with optional ANSI color.
2. **JSON**: machine-readable for editor integration and CI.
3. **LSP**: direct diagnostic push for language server protocol.

The three formats share the same `Diagnostic` data; only the formatter differs.

### 13.10 Use Go Developer Conventions

Gomputation's users are Go developers. Align with their expectations:
- Source locations in `file:line:col` format (same as `go vet`).
- One error per logical issue (not one per constraint failure).
- Structured output for machine consumption (same as `go test -json`).
- No chatty preamble or footer -- just the errors.

### 13.11 Avoid Common Anti-Patterns

1. **Never show raw unification variables.** Replace `a^42` with a descriptive name or elide it. If the variable is an unsolved existential, the user needs an annotation suggestion, not a variable name.

2. **Never show internal type representations.** The user writes `DB[Opened]`; the error should show `DB[Opened]`, not `TApp (TCon "DB") (TCon "Opened")`.

3. **Never blame the compiler.** Instead of "internal error: unexpected unification failure", say "type error: ..." and provide as much context as possible. If the error is genuinely an internal bug, say so clearly: "internal error (please report): ..." with the error code.

4. **Never produce empty suggestions.** If `= help:` has nothing useful to say, omit it entirely. An empty suggestion is worse than no suggestion.

5. **Never truncate without indication.** If a type expression is too long to display, show a prefix with `...` and note the truncation. Never silently drop information.

---

## 14. Key References

### Type Error Reporting

1. Haack, C. and Wells, J.B. "Type Error Slicing in Implicitly Typed Higher-Order Languages." *Science of Computer Programming*, 2004. The foundational work on error slicing: identifying minimal program fragments responsible for type errors.

2. Heeren, B., Leijen, D., and van IJzendoorn, A. "Helium, for Learning Haskell." *Haskell Workshop*, 2003. Heuristic-based error diagnosis using type graphs. Demonstrated that specialized heuristics dramatically improve error message quality for learners.

3. Hage, J. and Heeren, B. "Heuristics for Type Error Discovery and Recovery." *Utrecht University Technical Report*, 2006. Extended the Helium approach with additional heuristics and a formal framework for heuristic selection.

4. Zhao, E., Mao, R., Cimini, M., and Omar, C. "Total Type Error Localization and Recovery with Holes." *POPL*, 2024. The marked lambda calculus: bidirectionally typed error localization that continues past errors using non-empty holes with gradual types.

5. Bhanuka, I. et al. "Getting into the Flow: Towards Better Type Error Messages for Constraint-Based Type Inference." *OOPSLA*, 2023. Explains type errors as faulty data flows using constraint provenance tracking.

6. Loncaric, C. et al. "A Practical Framework for Type Inference Error Explanation." *University of Washington Technical Report*. Mycroft: a framework for explaining type errors using counter-factual reasoning.

### Bidirectional Typing

7. Pierce, B. and Turner, D. "Local Type Inference." *POPL*, 1998; *TOPLAS*, 2000. The origin of bidirectional type checking as "local type inference."

8. Dunfield, J. and Krishnaswami, N. "Complete and Easy Bidirectional Typechecking for Higher-Rank Polymorphism." *ICFP*, 2013. The DK algorithm: complete, easy, and decidable bidirectional typing for higher-rank polymorphism.

9. Dunfield, J. and Krishnaswami, N. "Bidirectional Typing." *ACM Computing Surveys*, 2021. Comprehensive survey of bidirectional typing systems, including mode-correctness, subsumption, and integration with advanced type features.

### Row Types and Unification

10. Remy, D. "Type Inference for Records in a Natural Extension of ML." *MIT Press*, 1993. The foundational treatment of row polymorphism with presence/absence flags.

11. Leijen, D. "Extensible Records with Scoped Labels." *Trends in Functional Programming*, 2005. Row polymorphism with scoped (duplicate) labels.

12. Leijen, D. "Koka: Programming with Row-Polymorphic Effect Types." *MSFP*, 2014. Row-polymorphic effect types with duplicate labels and evidence translation.

### Compiler Error Design

13. Rust RFC 1644. "Default and Expanded Rustc Errors." https://rust-lang.github.io/rfcs/1644-default-and-expanded-rustc-errors.html. The design principles for Rust's error message format: primary/secondary labels, error codes, progressive disclosure.

14. Czaplicki, E. Elm compiler error message design. https://elm-lang.org/. Conversational tone, actionable suggestions, and progressive disclosure.

15. Mernik, M., Heering, J., and Sloane, A.M. "When and How to Develop Domain-Specific Languages." *ACM Computing Surveys*, 2005. General principles for DSL design, including error reporting as a critical usability factor.

### Indexed Types and Effects

16. Atkey, R. "Parameterised Notions of Computation." *Journal of Functional Programming*, 2009. The indexed monad: pre/post state threading through monadic sequencing.

17. Brady, E. "Idris 2: Quantitative Type Theory in Practice." *ECOOP*, 2021. Dependent type implementation with indexed families; error reporting challenges for indexed types.

### Capability Systems

18. Miller, M.S., Yee, K.-P., and Shapiro, J. "Capability Myths Demolished." 2003. Foundational capability-security principles relevant to error message vocabulary.

---

## Relevance to Gomputation

Error reporting is not a peripheral concern. For an embedded language, the quality of error messages directly determines whether users will adopt the language or abandon it in frustration. Row type errors and indexed effect errors are structurally harder to report than ordinary type mismatches, because the relevant information (which label, which state, which step in the bind chain) is buried in structured types that the standard "could not match X with Y" pattern obscures.

The recommendations above are ordered by implementation priority:

1. **Diagnostic infrastructure** (Section 10.1-10.2) -- build this before the type checker.
2. **Row diffs** (Section 10.5) -- build this with the row unifier.
3. **Constraint provenance** (Section 10.8) -- thread this through unification.
4. **State flow reconstruction** (Section 10.6) -- build this for do-block checking.
5. **Error recovery** (Section 10.3-10.4) -- add after the basic checker works.
6. **Protocol metadata** (Section 11.5) -- extend the host boundary API.
7. **LSP integration** (Section 12) -- add when the language server is built.

The central design insight is this: Gomputation's type system is richer than most (rows, indices, bidirectional checking, higher-rank polymorphism), but its error domain is narrower than most (capability environments with protocol states). That narrowness is an advantage. Every error can be phrased in terms of capabilities, states, and transitions. The type-theoretic machinery is the implementation; the capability vocabulary is the interface.
