# Known Vulnerabilities and Open Security Issues

> **Status**: Temporary tracking document. Do not commit — address issues
> first, then remove or convert to CHANGELOG entries.
>
> **Date**: 2026-03-21
> **Discovered by**: Adversarial CLI testing (115+ attack vectors)

---

## V1: String Literal Allocation Limit Bypass

**Severity**: HIGH
**Status**: Fixed (commit `7197306`)
**Reproduction**: `tests/stress/stress_adversarial_test.go` — `TestAdversarial_StringLiteral_BypassAllocLimit`

### Description

String literals of arbitrary size bypass the `--max-alloc` / `MaxAlloc`
budget entirely. A 500 MB string literal is accepted and evaluated even
when `MaxAlloc` is set to 1 KiB.

### Root Cause

The allocation budget (`budget.Alloc()`) is only initialized and checked
during **evaluation** (`internal/runtime/eval/eval.go`). String literals
are materialized during **lexing** (`internal/compiler/parse/lexer.go:369-433`)
as Go `string` values and carried unchanged through the AST, IR, and into
`HostVal` at evaluation time — without any allocation charge.

```
Lexer (scanString)          → string allocated on Go heap    ← NO BUDGET
  ↓
AST (ExprStrLit.Value)      → same string pointer            ← NO BUDGET
  ↓
IR  (ir.Lit.Value)          → same string pointer            ← NO BUDGET
  ↓
Eval (case *ir.Lit)         → wraps in HostVal{Inner: ...}   ← NO BUDGET
```

**Files involved**:

- `internal/compiler/parse/lexer.go:369-433` — string scanning
- `internal/runtime/eval/eval.go:312-313` — `ir.Lit` case (zero alloc charge)
- `internal/infra/budget/budget.go:112-118` — `Alloc()` (never called for literals)

### Impact

An attacker who can provide arbitrary source to `RunSandbox()` can force
unbounded Go heap allocation, leading to OOM of the host process. This is
the primary sandbox escape vector.

### Mitigation Options

1. **Input size guard** (simplest): Validate `len(source)` before lexing;
   reject sources exceeding a configurable maximum (e.g., 10 MiB).

2. **Lexer-time allocation accounting**: Thread a byte counter through the
   lexer; charge scanned string bytes against a compile-time budget.

3. **Literal size cap**: Reject string literals exceeding a fixed size
   (e.g., 1 MiB) during lexing. Simpler than full accounting.

4. **Budget charge at eval time**: In the `ir.Lit` case of `evalStep()`,
   charge `len(string)` bytes to the budget. Cheap to implement but does
   not prevent the memory from being allocated; it only detects the
   overrun after the fact.

**Recommendation**: Option 1 + 4. Input size guard prevents the worst
case; eval-time charge ensures the budget correctly reflects actual
resource usage.

---

## V2: `check` Command Has No Timeout

**Severity**: MEDIUM
**Status**: Fixed (commit `7197306`)
**Reproduction**: `bin/gicel check --help` — no `--timeout` flag listed

### Description

The `run` command includes `--timeout` (default 5s) that covers the entire
pipeline (parse → check → optimize → eval). The `check` command (type-check
only, no evaluation) does not expose a timeout flag. If a pathological
program causes the type checker or constraint solver to loop, the process
runs indefinitely.

### Current Mitigations

- Type family reduction has a fuel limit (~100 steps).
- Parser has recursion depth (256) and step limits.
- In practice, no current input has been found that hangs the type checker.
  (V6 was originally attributed to the constraint solver but turned out to
  be a parser-level hang; see V6.)

### Impact

A theoretical DoS vector for hosts that expose `check` as a service.
Less severe than V1 because the type checker is performant on all tested
adversarial inputs.

### Fix

Add `--timeout` to the `check` subcommand, mirroring `run`. Apply
`context.WithTimeout` to the compilation pipeline.

---

## V3: No Input Size Validation

**Severity**: LOW–MEDIUM
**Status**: Fixed (commit `7197306`)

### Description

`readSource()` in `cmd/gicel/main.go:257-268` calls `os.ReadFile()` or
`io.ReadAll()` with no size check. A multi-gigabyte source file is read
entirely into memory before any processing begins.

`RunSandbox()` also accepts arbitrarily large `source string` arguments
without validation.

### Impact

Combined with V1, this enables multi-gigabyte memory consumption. Even
without V1, the AST and token list for a large input scale linearly with
input size.

### Fix

Add a `MaxSourceSize` field to `SandboxConfig` (default 10 MiB).
In `readSource()`, use `io.LimitReader` or check file size before reading.

---

## V4: Output Amplification via Sharing Expansion

**Severity**: LOW
**Status**: Documented / by design

### Description

The Pretty printer fully expands shared (DAG) values into their tree
representation. A small program with nested shared tuples produces output
exponentially larger than the source:

```
a := Just 42            -- 10 chars in output
b := (a, a, ..., a)     -- 10× amplification per level
```

10 levels of 10-tuples → ~2.8 × 10^10 chars of output (theoretical).

### Current Mitigations

- Timeout (5s) limits total wall-clock time, bounding actual output.
- Allocation limit bounds the in-memory representation.

### Impact

Minor. The timeout prevents unbounded output in practice. The output is
streamed, so memory usage remains proportional to evaluation, not output.

### Possible Improvements

- Add `--max-output` flag to bound output size.
- Implement sharing-aware Pretty printing (detected cycles → `...`).

---

## V5: Evidence Dictionary Scope Loss on Long Operator Chains

**Severity**: MEDIUM
**Status**: Fixed (commit `7197306`)
**Reproduction**: `tests/stress/stress_adversarial_test.go` — `TestAdversarial_LongOperatorChain_EvidenceLimit`

### Description

When a single expression contains ~500+ binary operator applications
(e.g., `0 + 1 + 1 + ... + 1`), the runtime fails with:

```
runtime error: unbound variable: $dict_512
```

The evidence-passing system generates dictionary variables (`$dict_N`)
during elaboration, but at high counts the variable falls out of scope
before it is referenced at runtime.

### Impact

Programs with very long infix chains in a single expression fail at
runtime. Workaround: break expressions into intermediate bindings.

### Notes

200 operators works reliably. The threshold appears to be around 500.
The root cause likely involves evidence variable numbering interacting
with scope trimming or environment management.

---

## V7: GADT Type Refinement Lost in Polymorphic Recursive Functions

**Severity**: MEDIUM (functional limitation)
**Status**: Fixed
**Commit**: `e8b2cd7` (v0.14)

### Description

GADT pattern matching does not refine the return type variable inside
a polymorphic recursive function. The type checker rejects the canonical
GADT use case — a type-safe expression evaluator:

```gicel
data Expr a := {
  LitI :: Int -> Expr Int;
  LitB :: Bool -> Expr Bool;
  Add  :: Expr Int -> Expr Int -> Expr Int;
  If_  :: Expr Bool -> Expr a -> Expr a -> Expr a
}

-- This is rejected:
eval :: \a. Expr a -> a
eval := \e. case e {
  LitI n -> n;           -- Error: expected #a, got Int
  LitB b -> b;           -- Error: expected #a, got Bool
  Add x y -> eval x + eval y;   -- Error: expected Expr #a, got Expr Int
  If_ c t f -> case eval c { True -> eval t; False -> eval f }
}
```

Errors: `cannot unify Int with rigid type variable #a` (and similar for
`Bool`). The checker does not refine `#a ~ Int` when matching on `LitI`,
nor `#a ~ Bool` when matching on `LitB`.

### Shallow Pattern Matching Works

Non-recursive functions with a concrete return type do work:

```gicel
getInt :: Expr Int -> Maybe Int
getInt := \e. case e { LitInt n -> Just n; _ -> Nothing }
-- OK: #a is already Int from the argument type
```

The existing `examples/gicel/types/gadts.gicel` demonstrates this
shallow-match pattern.

### Impact

GADT type refinement is the primary mechanism for embedding type-safe
DSLs. Without recursive eval, GADTs are limited to shallow destructors
— a significant loss of expressive power. This blocks the canonical
use cases:

- Type-safe expression evaluator
- Typed printf / format strings
- Type-safe protocol state machines with recursive processing
- Singleton-style proofs

### Root Cause (Hypothesis)

When the checker introduces a skolem for the `\a` in `eval :: \a. Expr a -> a`
and then checks the `case` branches, the GADT constructor's type equality
(e.g., `LitI :: Int -> Expr Int` implies `a ~ Int`) must be recorded as a
local type refinement for the branch body. Either:

1. The case-branch checker does not extract type equalities from GADT
   constructor return types, or
2. The equalities are extracted but not propagated into the skolem's
   unification scope.

OutsideIn(X) handles this via "given" equalities local to each branch.
The DK bidirectional system may need explicit local-equality support to
implement this correctly.

### Fix Direction

Implement local given equalities in GADT case branches. When a GADT
constructor `C :: ... -> T τ₁ ... τₙ` is matched against a scrutinee of
type `T α₁ ... αₙ`, emit local equalities `αᵢ ~ τᵢ` that are available
only within that branch's checking scope.

---

## V6: Parser Hang on Multiline Instance Method Body

**Severity**: MEDIUM
**Status**: Fixed (commit `56427c2`, defense strengthened with parseBody iteration limit + context propagation)
**Commit**: `e8b2cd7` (v0.14)

### Description

The parser hangs when an instance method body contains a multiline
function application (constructor or function applied to arguments on
continuation lines). The type checker is never reached.

### Symptoms

- CPU usage: ~98% (single core, busy loop)
- Memory usage: stable at ~7 MB (no leak; computation is stack-bound)
- Process does not terminate — no output, no error, no timeout
- Affects both `check` and `run` (parse is the first pipeline stage)
- `SIGQUIT` goroutine dump confirms hang in `parseInstBody.func1` →
  `parseBody` (parser.go:310), not in the type checker

### Full Reproduction

Prerequisites: Go toolchain, GICEL built at commit `e8b2cd7`.

```sh
go build -o bin/gicel ./cmd/gicel/
```

**Step 1** — Save the following as `v6_repro.gicel`:

```gicel
import Prelude

class Functor w => Comonad w {
  extract :: \a. w a -> a;
  extend  :: \a b. (w a -> b) -> w a -> w b
}

data Zipper a := MkZipper (List a) a (List a)

instance Functor Zipper {
  fmap := \f z. case z {
    MkZipper ls c rs -> MkZipper (map f ls) (f c) (map f rs)
  }
}

goLeft :: \a. Zipper a -> Maybe (Zipper a)
goLeft := \z. case z { MkZipper ls c rs -> case ls {
  Nil -> Nothing;
  Cons l rest -> Just (MkZipper rest l (Cons c rs))
}}

goRight :: \a. Zipper a -> Maybe (Zipper a)
goRight := \z. case z { MkZipper ls c rs -> case rs {
  Nil -> Nothing;
  Cons r rest -> Just (MkZipper (Cons c ls) r rest)
}}

allLefts :: \a. Zipper a -> List (Zipper a)
allLefts := fix (\self z. case goLeft z {
  Nothing -> Nil;
  Just z' -> Cons z' (self z')
})

allRights :: \a. Zipper a -> List (Zipper a)
allRights := fix (\self z. case goRight z {
  Nothing -> Nil;
  Just z' -> Cons z' (self z')
})

-- ★ THIS INSTANCE HANGS THE PARSER ★
instance Comonad Zipper {
  extract := \z. case z { MkZipper _ c _ -> c };
  extend := \f z. MkZipper
    (map f (allLefts z))
    (f z)
    (map f (allRights z))
}

main := 0
```

**Step 2** — Run either command (both hang):

```sh
bin/gicel check --recursion v6_repro.gicel    # hangs
bin/gicel run --recursion v6_repro.gicel      # hangs
```

**Step 3** — Observe: process consumes ~98% CPU, ~7 MB RAM, produces no
output. Must be killed with `kill` or Ctrl-C.

**Step 4** — Verify it is not a type-checker issue. Putting the same body
on one line passes:

```gicel
instance Comonad Zipper {
  extract := \z. case z { MkZipper _ c _ -> c };
  extend := \f z. MkZipper (map f (allLefts z)) (f z) (map f (allRights z))
}
```

```sh
bin/gicel check --recursion v6_oneline.gicel   # ok (instant)
bin/gicel run --recursion v6_oneline.gicel     # ok (runs correctly)
```

### Trigger Condition

A multiline function application **inside an instance method body**
where continuation lines start with `(` after a newline:

```gicel
  extend := \f z. MkZipper      -- ← expression continues below
    (map f (allLefts z))         -- ← newline + `(` = hang trigger
    (f z)
    (map f (allRights z))
```

The parser treats the newline as a statement separator (`;` and newline
are interchangeable in GICEL). This causes the expression to be truncated
at `MkZipper`, leaving `(map f (allLefts z))` as an orphaned token
sequence. The `parseBody` recovery loop in `parser.go:308` then fails
to make progress, producing an infinite loop.

### Root Cause

`parseBody` (parser.go:305) loops while the next token is not `}` or
EOF. For each iteration, the `parseInstBody` callback handles:

- `TokData` → associated data
- `TokType` → associated type
- `TokLower` → method (name `:=` expr)

When the callback encounters `(` (from the continuation line), none of
these branches match. The callback returns without advancing. `parseBody`
then checks for stagnation (`p.pos == before`), calls
`syncToStmtBoundary()`, but `syncToStmtBoundary` apparently fails to
advance past the parenthesized expression, creating the infinite loop.

**Key files**:

- `internal/compiler/parse/parse_class.go:316-341` — `parseInstBody`
- `internal/compiler/parse/parser.go:305-326` — `parseBody`
- Stagnation recovery logic in `syncToStmtBoundary`

### Why Single-Line Works

When the entire expression is on one line (`MkZipper (map f ...) (f z) ...`),
no newline occurs between the constructor and its arguments. The expression
parser consumes all arguments as part of function application, and the
result is a single method definition. No orphaned tokens are created.

### Impact

- **Sandbox**: A malicious multiline source can hang the parser
  indefinitely. The `run --timeout` flag does NOT cover parsing.
  Combined with V2 (no `check` timeout), this is a DoS vector.
- **Usability**: Users writing multiline instance methods see unexplained
  hangs with no error message. The workaround (single-line or helper
  function) is not obvious.

### Workaround

Either put the expression on a single line:

```gicel
instance Comonad Zipper {
  extract := \z. case z { MkZipper _ c _ -> c };
  extend := \f z. MkZipper (map f (allLefts z)) (f z) (map f (allRights z))
}
```

Or extract to a helper with a type annotation:

```gicel
_extendZ :: \a b. (Zipper a -> b) -> Zipper a -> Zipper b
_extendZ := \f z. MkZipper
  (map f (allLefts z))
  (f z)
  (map f (allRights z))

instance Comonad Zipper {
  extract := \z. case z { MkZipper _ c _ -> c };
  extend := _extendZ
}
```

Note: the helper works because top-level bindings use different newline
handling than instance method bodies.

### Fix Options

1. **Fix stagnation recovery** (immediate): Ensure `syncToStmtBoundary`
   always advances at least one token when the current token is `(`. The
   `parseBody` stagnation check already has a fallback `p.advance()` at
   line 320, but it is not being reached.

2. **Allow continuation lines in instance bodies** (proper fix): Modify
   the instance body parser to recognize that `(` after an expression on
   the previous line is a continuation (application argument), not a new
   statement. This likely requires adjusting `stmtBoundaryDepth` or the
   `atStmtBoundary()` logic for instance bodies.

3. **Parser timeout** (defense-in-depth): Add a step limit to `parseBody`
   to prevent infinite loops from any cause.

**Recommendation**: Option 2 + 3. Fix the continuation handling to match
user expectations; add the step limit as defense against future regressions.

### Investigation History

Originally diagnosed as a type-checker constraint solver blowup. SIGQUIT
goroutine stack dump (2026-03-21) revealed the actual hang is in the
parser. Confirmed by showing that a single-line equivalent of the same
expression compiles and runs correctly. Debug counters injected into
`check`, `infer`, `subsCheck`, `instantiate`, and `resolveInstance`
showed all counters frozen at Prelude-processing levels (~1320 check
calls), proving the user code's type checking was never reached.

### Earlier Misdiagnosis

The solver step limit added to `solveWanteds` (100K steps) does not
help because the solver is never invoked — the hang precedes it. The
initial hypothesis about DK ordered context + polymorphic function
unification was incorrect; it described what _would_ happen if the
parser produced an AST, but the parser never completes.

These points described what _would_ happen if the parser produced an
AST, but the parser never completes. Retained for reference only.

---

## V8: `do` Notation Runtime Error with User-Defined IxMonad

**Severity**: MEDIUM
**Status**: Fixed (Monad dispatch fallback in do-notation elaboration)
**Commit**: `e8b2cd7` (v0.14)

### Description

When a user-defined monad has both a `Monad` and `IxMonad` instance, using
`do` notation (which dispatches via `IxMonad` + `Lift`) produces a runtime
non-exhaustive pattern match error, even though the same computation works
correctly with explicit `mbind` calls.

### Reproduction

```gicel
import Prelude

data Reader e a := MkReader (e -> a)

runReader :: \e a. Reader e a -> e -> a
runReader := \r env. case r { MkReader f -> f env }

ask :: \e. Reader e e
ask := MkReader (\e. e)

instance Monad (Reader e) {
  mpure := \a. MkReader (\_. a);
  mbind := \ma f. MkReader (\env. runReader (f (runReader ma env)) env)
}

instance IxMonad (Reader e) {
  ixpure := \a. MkReader (\_. a);
  ixbind := \ma f. MkReader (\env. runReader (f (runReader ma env)) env)
}

-- This WORKS:
prog_ok := mbind ask (\x. mpure (append "port=" (show x)))

-- This FAILS at runtime:
prog_bad :: Reader Int String
prog_bad := do { x <- ask; pure (append "port=" (show x)) }

main := runReader prog_ok 8080   -- "port=8080" ✓
-- main := runReader prog_bad 8080  -- runtime error: non-exhaustive pattern match
```

Error: `runtime error: non-exhaustive pattern match on HostVal(8080)`

### Trigger Condition

The do-block's bind produces a value of type `m a`, then the continuation
produces `m b` where **`a ≠ b`**. When `a = b`, everything works:

```gicel
-- OK:  Reader Int Int  (bind returns Int, continuation returns Int)
prog_ok :: Reader Int Int
prog_ok := do { x <- ask; pure (x + 1) }

-- FAIL: Reader Int String  (bind returns Int, continuation returns String)
prog_bad :: Reader Int String
prog_bad := do { x <- ask; pure (show x) }
```

### Root Cause (Hypothesis)

`do` notation dispatches through `IxMonad (Lift (Reader e))`. The `Lift`
wrapper's elaboration may incorrectly reuse the bind result type for
the continuation's parameter, causing the evaluator to encounter a
raw `HostVal` (the environment `Int`) where it expects a `ConVal
(MkReader ...)` after the type changes from `a` to `b` in the bind.

### Impact

Users who define custom monads and want to use `do` notation must fall back
to explicit `mbind`/`mpure` chains, negating the ergonomic benefit of
do-notation.

---

## V9: `fix` Cannot Produce Data Constructor Values

**Severity**: LOW–MEDIUM
**Status**: Documented (CBV structural limitation; error message improved)
**Commit**: `e8b2cd7` (v0.14)

### Description

`fix` works correctly when the fixed-point body is a function (lambda),
but fails at runtime when the body returns a data constructor application.

### Reproduction

```gicel
import Prelude

data EvenOdd := MkEO (Int -> Bool) (Int -> Bool)

getEven :: EvenOdd -> Int -> Bool
getEven := \eo. case eo { MkEO e _ -> e }

getOdd :: EvenOdd -> Int -> Bool
getOdd := \eo. case eo { MkEO _ o -> o }

-- Mutual recursion via product-of-functions fixpoint — FAILS:
eo := fix (\self. MkEO
  (\n. case eq n 0 { True -> True;  False -> getOdd self (n - 1) })
  (\n. case eq n 0 { True -> False; False -> getEven self (n - 1) }))

main := getEven eo 4
```

Error: `runtime error: non-exhaustive pattern match on Closure(_arg, ...)`

The `case self { MkEO e _ -> e }` inside `getEven` receives a `Closure`
(the unevaluated fixpoint thunk) instead of a `ConVal (MkEO ...)`.

### Workaround

Encode mutual recursion as a single recursive function with a flag or
use `fix` on a function that returns a tuple:

```gicel
-- Works: fix on a function
isEven := fix (\self n. case eq n 0 { True -> True; False -> not (self (n - 1)) })
```

### Root Cause

In strict (CBV) evaluation, `fix f = f (fix f)` diverges when `f`'s
result is not a function. GICEL's `fix` uses a recursive closure
mechanism that expects the body to be a lambda — when the body is a
constructor application, the closure is passed as `self` without being
forced, so pattern matching encounters a `Closure` instead of a `ConVal`.

---

## V10: Nested `case` Brace Ambiguity

**Severity**: LOW
**Status**: Fixed (noBraceAtom reset in parseParen)
**Commit**: `e8b2cd7` (v0.14)

### Description

When a `case` expression appears inside parentheses followed by another
`case`, the parser misinterprets the closing `}` of the inner case:

```gicel
-- This FAILS to parse:
test := case (case Just True { Just b -> b; Nothing -> False }) { True -> "yes"; False -> "no" }
```

Error: `expected }` at the `->` after `True` in the outer case.

### Workaround

Extract the inner case to a separate binding:

```gicel
inner := case Just True { Just b -> b; Nothing -> False }
test := case inner { True -> "yes"; False -> "no" }
```

### Root Cause

The parser's brace-matching does not correctly track that `}` closes
the inner `case` when the `case` is inside parentheses. The `) {` token
sequence (closing paren of the scrutinee, opening brace of the outer case)
likely confuses the parser's statement-boundary or brace-depth tracking.

### Impact

Low severity — easily worked around and relatively uncommon in practice.
Does not cause hangs or incorrect behavior, only a parse error.

---

## V11: Stale "No Recursion in CLI" Comments in Examples

**Severity**: LOW (documentation / usability)
**Status**: Fixed (examples updated with --recursion examples and V7 note)
**Commit**: `e8b2cd7` (v0.14)

### Description

Two example files contain comments stating recursive functions are "not
available in standalone CLI mode", but `--recursion` has been available
in the CLI since at least v0.14:

- `examples/gicel/types/gadts.gicel` line 7–9: claims recursive functions
  require EnableRecursion "not available in standalone CLI mode", then
  avoids defining a recursive `eval`. The real reason `eval` can't be
  written is V7 (GADT type refinement), not CLI unavailability.

- `examples/gicel/types/recursive-data.gicel` line 5–6: same claim,
  resulting in an example that only demonstrates shallow pattern matching
  on `Nat` and `Tree`, without `toInt`, `depth`, or any recursive function.

### Impact

Users reading these examples are misled about CLI capabilities and miss
the opportunity to learn recursive patterns. The comments mask V7 (the
real limitation for GADTs) and make recursion seem less accessible than
it is.

### Fix

Update both files: remove the stale CLI claims, add `--recursion`
examples where possible (e.g., `toInt` for `Nat`, `depth` for `Tree`),
and explicitly note V7 as the reason GADT `eval` cannot be written.

---

## Defense Summary

The following defenses were confirmed as fully operational across 115+
adversarial test cases:

| Defense Layer              | Mechanism                    | Tests       |
| -------------------------- | ---------------------------- | ----------- |
| Parser recursion limit     | `maxRecurseDepth = 256`      | 500-deep () |
| Parser step limit          | `tokens × 4`                 | 10K ops     |
| Step limit                 | `budget.Step()`              | fix loops   |
| Depth limit                | `budget.Enter()/Leave()`     | do-blocks   |
| Nesting limit              | `budget.Nest()/Unnest()`     | deep App    |
| Alloc limit (runtime)      | `budget.Alloc()`             | Cons, <>    |
| Timeout                    | `context.WithTimeout`        | last resort |
| Panic recovery             | `defer recover()` in sandbox | always      |
| Type family fuel           | reduction counter            | Loop TF     |
| Pattern exhaustiveness     | compile-time checker         | missing C   |
| Overlap detection          | instance resolver            | C a vs Int  |
| Integer literal validation | lexer                        | huge int    |
| Duplicate import detection | scope checker                | 20× import  |
| Error truncation           | diagnostic formatter         | 5K errors   |
| Value sharing              | DAG evaluation (not tree)    | 2^20 tree   |
