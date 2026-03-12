# Gomputation Implementation Plan

A typed effect language embedded in Go. AI agents write pure, capability-indexed computations; the host provides effects through a controlled boundary; the type system and runtime enforce safety, determinism, and bounded execution.

Spec: `spec.draft.v0_3.md`

## Dependency Graph

```
phase-0-foundation
    └─→ phase-1-types
            ├─→ phase-2-core ─────┐
            │       └─→ phase-3-eval
            └─→ phase-4-syntax    │
                    └─→ phase-5-check ─→ phase-6-host ─→ phase-7-integration
```

## Package Layout

Go standard layout: `pkg/` = public sub-packages, `internal/` = private implementation.

| Layer | Package | Import path | Visibility |
|-------|---------|-------------|------------|
| Root API | `/` | `gomputation` | Public — Engine, Runtime, Value, CapEnv, PrimImpl |
| Type builders | `pkg/types/` | `gomputation/pkg/types` | Public — Type, Kind, Row, builders for DeclareBinding |
| Source positions | `internal/span/` | — | Internal |
| Diagnostics | `internal/errs/` | — | Internal |
| Core IR | `internal/core/` | — | Internal |
| Evaluator | `internal/eval/` | — | Internal |
| Lexer/Parser | `internal/syntax/` | — | Internal |
| Type checker | `internal/check/` | — | Internal |
| Prelude | `internal/prelude/` | — | Internal (v0); future: module system 経由で提供 |

## Phase Index

| Phase | Name | Package(s) | Depends On | Summary |
|-------|------|------------|------------|---------|
| 0 | [Foundation](./phase-0-foundation.md) | `internal/span/`, `internal/errs/` | — | Go module, directory scaffold, shared primitives |
| 1 | [Types](./phase-1-types.md) | `pkg/types/` | 0 | Kind, Type, Row representations; equality; substitution |
| 2 | [Core IR](./phase-2-core.md) | `internal/core/` | 1 | 13 Core term formers; Core types; pretty-printing |
| 3 | [Evaluator](./phase-3-eval.md) | `internal/eval/` | 1, 2 | Value types; Evaluator; CapEnv threading; pattern match |
| 4 | [Syntax](./phase-4-syntax.md) | `internal/syntax/` | 1 | Token, Lexer, surface AST, Pratt parser, fixity |
| 5 | [Type Checker](./phase-5-check.md) | `internal/check/` | 1, 2, 4 | Bidirectional checking; row unification; elaboration to Core |
| 6 | [Host Boundary](./phase-6-host.md) | `/` (root) | 1–5 | Engine, Runtime, registration API, type builder |
| 7 | [Integration](./phase-7-integration.md) | `internal/prelude/`, `examples/` | 0–6 | Prelude, end-to-end pipeline, example programs |

## Confirmed Design Decisions

These are settled and must not be revisited during implementation.

- **Computation representation**: direct Core IR interpretation, no CompVal
- **Three-tier architecture**: Engine (config) → Runtime (compiled, immutable) → Evaluator (per-execution, mutable)
- **CapEnv**: `map[string]any`, copy-on-write
- **Evaluator**: `(ev *Evaluator) Eval(env, capEnv, expr) → (EvalResult, error)` — holds `ctx`, `prims`, `limit` on receiver
- **PrimImpl**: `func(ctx, CapEnv, []Value) (Value, CapEnv, error)` — receives `context.Context` for timeout/cancellation of blocking host operations
- **Error recovery**: `Result` return type = recoverable, otherwise = abort
- **Prelude**: Gomputation source (`Bool`, `Unit`, `Result`, `Effect`), separate from core
- **Built-in boundary**: `pure`/`bind`/`thunk`/`force`/`assumption`/`rec`/`fix` in checker+evaluator; prelude = source
- **`rec`/`fix`**: host opt-in via `EnableRecursion()`; elaborate to `LetRec`; `rec` requires `pre = post`
- **`fold`**: assumption (Go-side `PrimImpl`); host provides per data type; always total
- **No literals, no if, no let**: block expressions desugar to lambda+app; concrete values enter through host bindings
- **DeclareBinding**: `Engine.DeclareBinding(name, ty)` declares host-injected values at type level; checker validates, runtime provides values
- **9 keywords**: `case of do data type forall infixl infixr infixn`
- **13 Core formers**: Var, Lam, App, TyApp, TyLam, Con, Case, LetRec, Pure, Bind, Thunk, Force, PrimOp
- **stdlib deferred**: standard operations (Num, Semigroup, etc.) require type classes; planned after Constraint vocabulary activation

## Boundary Architecture

### Lifecycle Tiers

| Tier | Type | Lifecycle | Mutability | Goroutine-safe |
|------|------|-----------|------------|----------------|
| Config | `Engine` | Application | Mutable during setup | No |
| Compiled | `Runtime` | Program (cache & reuse) | Immutable after creation | Yes |
| Execution | `Evaluator` | Single `Run` call | Mutable (`Limit` counter) | No |

**Engine → Runtime** (freeze): `NewRuntime(source)` compiles the program and snapshots all Engine configuration into an immutable Runtime. After freeze, Engine and Runtime share no mutable state.

**Runtime → Evaluator** (spawn): each `Run`/`RunContext` call creates a fresh `Evaluator` with its own `Limit`, `Env`, `CapEnv`. The `Evaluator` reads from Runtime's frozen `PrimRegistry` and `core.Program` but owns its mutable state exclusively.

### Cross-Cutting Boundaries

| Boundary | Where | What crosses | Invariant |
|----------|-------|-------------|-----------|
| Host ↔ Language | `PrimImpl` | `ctx`, `CapEnv`, `[]Value` ↔ `Value`, `CapEnv`, `error` | PrimImpl must be goroutine-safe if Runtime is shared |
| Surface → Core | `check.Check` | `syntax.Expr` → `core.Core` | Output has no `TyMeta`, no `TyError`, all cases exhaustive |
| Type erasure | Core → Eval | Types exist at check time, erased at eval time | Evaluator never inspects types |
| Compile → Runtime errors | `CompileError` / `RuntimeError` | Error types separated by tier | No eval if compilation fails |

## Termination

**Without `rec`**: all well-typed programs terminate (no recursion in core calculus).

**With `rec` (host opt-in)**: termination is not guaranteed. Step limit + call depth limit enforce bounded execution. Host-configurable via `SetStepLimit()` / `SetDepthLimit()`.

**Defense-in-depth**: limits are enforced regardless of `rec` availability (Phase 3).

**`context.Context`**: propagated through `Eval` and `PrimImpl`. Enables host-side cancellation and timeout of both evaluation steps and blocking host operations (Phase 3, 6).

**fold**: implemented as assumption (Go-side `PrimImpl`), always total. Host provides per data type.

**Type alias cycles**: detected statically via DFS cycle detection before alias expansion. Prevents infinite expansion at compile time (Phase 5).

See `docs/recursion-and-totality.md` for the full design space analysis.

## Debug Tools

Four layers of observability, all optional (nil hook = zero overhead).

| Layer | Mechanism | What it observes |
|-------|-----------|-----------------|
| Compilation | `CheckTraceHook` | Unification, meta solving, infer/check, instantiation, row unification |
| Core IR | `Runtime.Program()` / `PrettyProgram()` | Elaboration output (what Core was generated) |
| Evaluation | `TraceHook` | Per-step Core node, Env, CapEnv, call depth |
| Post-execution | `EvalStats` / `RunContextFull` | Total steps, peak depth, final CapEnv |

See Phase 3 (§3.10–3.11), Phase 5 (§5.15), Phase 6 (§6.2–6.3).

## Supplement

- [Research Findings](./supplement-research-findings.md) — additional details from domain research docs (DK operations, error recovery, deferred items)
