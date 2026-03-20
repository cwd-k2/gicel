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

## V6: Parser Hang on Multiline Instance Method Body

**Severity**: MEDIUM
**Status**: Open — confirmed reproducible (2026-03-21)
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
unification was incorrect; it described what *would* happen if the
parser produced an AST, but the parser never completes.

These points described what *would* happen if the parser produced an
AST, but the parser never completes. Retained for reference only.

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
