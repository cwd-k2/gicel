# Known Vulnerabilities and Open Security Issues

> **Status**: Temporary tracking document. Do not commit — address issues
> first, then remove or convert to CHANGELOG entries.
>
> **Date**: 2026-03-21
> **Discovered by**: Adversarial CLI testing (115+ attack vectors)

---

## V1: String Literal Allocation Limit Bypass

**Severity**: HIGH
**Status**: Open
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
**Status**: Open
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
- ~~In practice, no current input has been found that hangs the type checker.~~
  **Invalidated**: See V6 — instance method bodies with polymorphic functions
  can trigger exponential blowup in the constraint solver. Confirmed
  reproducible on commit `e8b2cd7` (2026-03-21).

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
**Status**: Open

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
**Status**: Open
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

## V6: Type Checker Exponential Blowup in Instance Method Bodies

**Severity**: MEDIUM
**Status**: Open — confirmed reproducible (2026-03-21)
**Commit**: `e8b2cd7` (v0.14)

### Description

When a type class instance method body combines (1) a polymorphic function
from another class (e.g., `map`/`fmap`), (2) a function whose argument type
involves the instance's type constructor, and (3) the instance head's type
variable, the constraint solver enters exponential blowup and the type
checker does not terminate.

### Symptoms

- CPU usage: ~98% (single core, busy loop)
- Memory usage: stable at ~7 MB (no leak; computation is stack-bound)
- Process does not terminate — no output, no error, no timeout
- Affects both `check` and `run` (type checking occurs before evaluation)

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

-- ★ THIS INSTANCE HANGS THE TYPE CHECKER ★
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
bin/gicel run --recursion v6_repro.gicel      # hangs (timeout does NOT cover type checking)
```

**Step 3** — Observe: process consumes ~98% CPU, ~7 MB RAM, produces no
output. Must be killed with `kill` or Ctrl-C.

### Trigger Conditions

All three conditions must hold simultaneously:

1. **Instance method body** — the expression is inside an `instance` block
   (standalone bindings with the same body and an explicit type annotation
   are checked successfully)
2. **Polymorphic function call** — the body invokes a polymorphic function
   from another class (e.g., `map :: \a b. (a -> b) -> List a -> List b`)
3. **Instance type constructor in the argument** — the argument to the
   polymorphic function involves the type being instanced (e.g.,
   `allLefts z :: List (Zipper a)`, where `Zipper` is the instance head)

Removing any one of these conditions eliminates the hang:

| Variant                                 | Hangs? | Why                                    |
| --------------------------------------- | ------ | -------------------------------------- |
| Inline `extend` in instance (original)  | **Yes**| All three conditions met               |
| `extend := _extendZ` (external helper)  | No     | Condition 1 removed (body is trivial)  |
| Dummy `extend` without `map`            | No     | Condition 2 removed                    |
| `Env`/`Store` instances with `map`      | No     | Condition 3 removed (no recursive type)|

### Root Cause (Hypothesis)

Inside the instance body, the solver must unify:

1. `w ~ Zipper` (instance head)
2. `map`'s polymorphism: `\a b. (a -> b) -> List a -> List b`
3. `f :: Zipper a -> b` (from `extend`'s method signature)
4. `allLefts z :: List (Zipper a)` (return type involves the instance constructor)

The combination of instance context unification (`w ~ Zipper`) with
the polymorphic `map` invocation forces the DK ordered context to
explore constraint combinations that grow exponentially. The solver
lacks a mechanism to cut off or memoize these intermediate unification
states.

The blowup is CPU-bound (not memory-bound) because the solver
backtracks through constraint alternatives on the stack without
materializing intermediate structures on the heap.

### Workaround

Extract the method body into a standalone helper with an explicit type
annotation. The annotation precludes the combinatorial explosion by
providing the solver with a closed signature:

```gicel
_extendZ :: \a b. (Zipper a -> b) -> Zipper a -> Zipper b
_extendZ := \f z. MkZipper (map f (allLefts z)) (f z) (map f (allRights z))

instance Comonad Zipper {
  extract := \z. case z { MkZipper _ c _ -> c };
  extend := _extendZ   -- OK: no inline inference needed
}
```

This workaround is applied in `examples/gicel/patterns/comonad.gicel`.

### Impact

- **Sandbox**: A malicious source can hang the type checker indefinitely.
  Combined with V2 (no `check` timeout), this is a DoS vector for hosts
  that expose compilation as a service. Note: `run --timeout` does NOT
  mitigate this because the timeout covers evaluation, not type checking.
- **Usability**: Users writing non-trivial type class instances may hit
  unexplained hangs. The workaround (extract to helper) is not obvious.

### Fix Options

1. **Timeout for the solver** (immediate): Add a step/fuel limit to the
   constraint solver's unification loop, analogous to the type family
   reduction fuel (~100). Emit a diagnostic when the limit is reached.

2. **Memoize unification subproblems** (medium-term): Cache intermediate
   unification results in the DK context to prevent re-exploration of
   equivalent constraint states.

3. **Instance method signature propagation** (targeted): When checking
   an instance method body, push the method's class-declared signature
   (with the instance head substituted) into the checking context before
   descending into the body. This gives the solver the same guidance that
   an explicit type annotation provides, eliminating the need for the
   workaround.

**Recommendation**: Option 1 + 3. The timeout prevents DoS; signature
propagation eliminates the root cause for this specific pattern.

---

## Defense Summary

The following defenses were confirmed as fully operational across 115+
adversarial test cases:

| Defense Layer              | Mechanism                    | Tests      |
| -------------------------- | ---------------------------- | ---------- |
| Parser recursion limit     | `maxRecurseDepth = 256`      | 500-deep () |
| Parser step limit          | `tokens × 4`                 | 10K ops    |
| Step limit                 | `budget.Step()`              | fix loops  |
| Depth limit                | `budget.Enter()/Leave()`     | do-blocks  |
| Nesting limit              | `budget.Nest()/Unnest()`     | deep App   |
| Alloc limit (runtime)      | `budget.Alloc()`             | Cons, <>   |
| Timeout                    | `context.WithTimeout`        | last resort|
| Panic recovery             | `defer recover()` in sandbox | always     |
| Type family fuel           | reduction counter             | Loop TF    |
| Pattern exhaustiveness     | compile-time checker          | missing C  |
| Overlap detection          | instance resolver             | C a vs Int |
| Integer literal validation | lexer                         | huge int   |
| Duplicate import detection | scope checker                 | 20× import |
| Error truncation           | diagnostic formatter          | 5K errors  |
| Value sharing              | DAG evaluation (not tree)     | 2^20 tree  |
