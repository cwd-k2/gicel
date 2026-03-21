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

| ID  | Description                                                | Fix                        |
| --- | ---------------------------------------------------------- | -------------------------- |
| V1  | String literal allocation limit bypass                     | commit `7197306`           |
| V2  | `check` command has no timeout                             | commit `7197306`           |
| V3  | No input size validation                                   | commit `7197306`           |
| V5  | Evidence dictionary scope loss on long operator chains     | commit `7197306`           |
| V6  | Parser hang + multiline instance body parsing              | commits `56427c2`, current |
| V7  | GADT type refinement lost in polymorphic recursion         | commit `8f9306a`           |
| V8  | `do` notation: Monad dispatch + IxMonad-only compile error | commit `8f9306a`, current  |
| V10 | Nested `case` brace ambiguity                              | commit `8f9306a`           |
| V11 | Stale "no recursion in CLI" comments in examples           | commit `8f9306a`           |

---

## Defense Summary

The following defenses were confirmed as fully operational across 115+
adversarial test cases:

| Defense Layer               | Mechanism                    | Tests       |
| --------------------------- | ---------------------------- | ----------- |
| Parser recursion limit      | `maxRecurseDepth = 256`      | 500-deep () |
| Parser step limit           | `tokens × 4`                 | 10K ops     |
| Parser body iteration limit | `parseBody` max iterations   | V6 defense  |
| Step limit                  | `budget.Step()`              | fix loops   |
| Depth limit                 | `budget.Enter()/Leave()`     | do-blocks   |
| Nesting limit               | `budget.Nest()/Unnest()`     | deep App    |
| Alloc limit (runtime)       | `budget.Alloc()`             | Cons, <>    |
| Timeout                     | `context.WithTimeout`        | last resort |
| Panic recovery              | `defer recover()` in sandbox | always      |
| Type family fuel            | reduction counter            | Loop TF     |
| Pattern exhaustiveness      | compile-time checker         | missing C   |
| Overlap detection           | instance resolver            | C a vs Int  |
| Integer literal validation  | lexer                        | huge int    |
| Duplicate import detection  | scope checker                | 20× import  |
| Error truncation            | diagnostic formatter         | 5K errors   |
| Value sharing               | DAG evaluation (not tree)    | 2^20 tree   |
| Solver step limit           | `Budget.SolverStep()`        | constraint  |
| TF reduction limit          | `Budget.TFStep()`            | Loop TF     |
| Resolve depth limit         | `Budget.EnterResolve()`      | deep inst.  |
| Input size limit            | `MaxSourceSize`              | V1/V3       |
