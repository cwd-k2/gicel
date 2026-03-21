# Known Vulnerabilities and Open Issues

> **Date**: 2026-03-21
> **Discovered by**: Adversarial CLI testing (115+ attack vectors) + pattern exploration

---

## V4: Output Amplification via Sharing Expansion

**Severity**: LOW
**Status**: Documented / by design

### Description

The Pretty printer fully expands shared (DAG) values into their tree
representation. A small program with nested shared tuples produces output
exponentially larger than the source.

### Current Mitigations

- Timeout (5s) limits total wall-clock time, bounding actual output.
- Allocation limit bounds the in-memory representation.

### Possible Improvements

- Add `--max-output` flag to bound output size.
- Implement sharing-aware Pretty printing (detected cycles → `...`).

---

## V6: Multiline Expressions Not Supported in Instance Method Bodies

**Severity**: LOW–MEDIUM
**Status**: Partially fixed — hang prevented (commit `56427c2`), but multiline still produces parse errors

### Description

Instance method bodies do not support multiline function application.
Continuation lines after a newline are treated as new statements rather
than arguments to the previous expression. Originally caused an infinite
parser loop (fixed); now produces parse errors.

### Example

```gicel
-- Parse error: continuation lines treated as separate statements
instance Comonad Zipper {
  extend := \f z. MkZipper
    (map f (allLefts z))     -- ← parse error here
    (f z)
    (map f (allRights z))
}

-- Works: same expression on one line
instance Comonad Zipper {
  extend := \f z. MkZipper (map f (allLefts z)) (f z) (map f (allRights z))
}
```

Note: top-level bindings handle multiline expressions correctly. Only
instance (and class) method bodies are affected.

### Workaround

Write instance method bodies on a single line, or extract to a top-level
helper function.

---

## V8: `do` Notation Runtime Error with User-Defined IxMonad (a ≠ b)

**Severity**: MEDIUM
**Status**: Open — marked Fixed but still reproducible (2026-03-21)

### Description

When a user-defined monad uses `do` notation and the bind produces type
`m a` but the continuation returns `m b` where `a ≠ b`, a runtime
non-exhaustive pattern match error occurs. Same code works with explicit
`mbind`.

### Reproduction

```gicel
import Prelude

data Reader e a := MkReader (e -> a)

runReader :: \e a. Reader e a -> e -> a
runReader := \r env. case r { MkReader f -> f env }

ask :: \e. Reader e e
ask := MkReader (\e. e)

instance IxMonad (Reader e) {
  ixpure := \a. MkReader (\_. a);
  ixbind := \ma f. MkReader (\env. runReader (f (runReader ma env)) env)
}

-- OK:  a = b = Int
prog_ok :: Reader Int Int
prog_ok := do { x <- ask; pure (x + 1) }

-- FAIL: a = Int, b = String
prog_bad :: Reader Int String
prog_bad := do { x <- ask; pure (show x) }

main := runReader prog_bad 42
-- runtime error: non-exhaustive pattern match on HostVal(42)
```

### Root Cause (Hypothesis)

`do` notation dispatches through `IxMonad (Lift (Reader e))`. The `Lift`
wrapper's elaboration may incorrectly reuse the bind result type for
the continuation's parameter, causing the evaluator to encounter a raw
`HostVal` where it expects a `ConVal (MkReader ...)`.

### Workaround

Use explicit `mbind`/`mpure` instead of `do` notation when `a ≠ b`.

---

## V9: `fix` Cannot Produce Data Constructor Values

**Severity**: LOW–MEDIUM
**Status**: Documented (CBV structural limitation; error message improved)

### Description

`fix` works when the body is a lambda but fails at runtime when the body
is a data constructor application. This prevents mutual recursion via
product-of-functions fixpoint.

### Reproduction

```gicel
data EvenOdd := MkEO (Int -> Bool) (Int -> Bool)

getEven := \eo. case eo { MkEO e _ -> e }

eo := fix (\self. MkEO
  (\n. case eq n 0 { True -> True;  False -> getEven self (n - 1) })
  (\n. case eq n 0 { True -> False; False -> getEven self (n - 1) }))
-- runtime error: non-exhaustive pattern match on Closure(...)
```

### Root Cause

In CBV evaluation, `fix` creates a recursive closure. When the body is
a constructor application rather than a lambda, `self` is passed as an
unevaluated closure. Pattern matching on `self` sees `Closure` instead
of `ConVal`.

### Workaround

Encode mutual recursion as a single recursive function, or use `fix`
on a function returning a tuple.

---

## Resolved Issues

The following issues were fixed and are retained as historical reference:

| ID  | Description                                          | Fix                         |
| --- | ---------------------------------------------------- | --------------------------- |
| V1  | String literal allocation limit bypass               | commit `7197306`            |
| V2  | `check` command has no timeout                       | commit `7197306`            |
| V3  | No input size validation                             | commit `7197306`            |
| V5  | Evidence dictionary scope loss on long operator chains | commit `7197306`          |
| V6  | Parser hang on multiline instance body (infinite loop) | commit `56427c2`          |
| V7  | GADT type refinement lost in polymorphic recursion   | commit `56427c2`            |
| V10 | Nested `case` brace ambiguity                        | commit `56427c2`            |
| V11 | Stale "no recursion in CLI" comments in examples     | commit `56427c2`            |

---

## Defense Summary

The following defenses were confirmed as fully operational across 115+
adversarial test cases:

| Defense Layer              | Mechanism                    | Tests       |
| -------------------------- | ---------------------------- | ----------- |
| Parser recursion limit     | `maxRecurseDepth = 256`      | 500-deep () |
| Parser step limit          | `tokens × 4`                 | 10K ops     |
| Parser body iteration limit | `parseBody` max iterations  | V6 defense  |
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
| Solver step limit          | `maxSolverSteps = 100_000`   | constraint  |
| Input size limit           | `MaxSourceSize`              | V1/V3       |
