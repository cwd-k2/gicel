# Phase 3: Evaluator

## Objective

Implement the runtime: value types, environments, and the `Evaluator` that directly interprets Core IR. No intermediate computation tree — Core nodes are the execution format.

## Dependencies

Phase 1 (`types/`), Phase 2 (`core/`).

## Package: `eval/`

### 3.1 Runtime Values (`eval/value.go`)

Spec §9.1 — four value forms.

```go
package eval

// Value is a runtime value.
type Value interface {
    valueNode()
    String() string
}

// HostVal wraps an opaque Go value injected from the host.
// The language treats it as a black box; operations require host assumptions.
type HostVal struct {
    Inner any
}

// Closure is a function value capturing its definition environment.
type Closure struct {
    Env   *Env
    Param string
    Body  core.Core
}

// ConVal is a fully-applied constructor value.
type ConVal struct {
    Con  string
    Args []Value
}

// ThunkVal is a suspended computation captured by `thunk`.
// Not memoized — forcing executes the computation each time.
type ThunkVal struct {
    Env  *Env
    Comp core.Core
}
```

**Design note**: `Closure` and `ThunkVal` capture `*Env` (pointer), not a copy. The `Env` itself is immutable-by-convention (extend returns a new `Env`), so sharing is safe.

### 3.2 Variable Environment (`eval/env.go`)

```go
// Env is a lexically-scoped variable environment.
// Immutable: Extend returns a new Env with the additional binding.
type Env struct {
    bindings map[string]Value
    parent   *Env
}

// EmptyEnv creates an empty environment.
func EmptyEnv() *Env

// Extend returns a new Env with an additional binding.
func (e *Env) Extend(name string, val Value) *Env

// ExtendMany returns a new Env with multiple bindings.
func (e *Env) ExtendMany(bindings map[string]Value) *Env

// Lookup searches for a variable, walking up the parent chain.
func (e *Env) Lookup(name string) (Value, bool)
```

Implementation: linked list of scopes. Each scope is a `map[string]Value`. `Extend` creates a new node pointing to the current env as parent.

### 3.3 Capability Environment (`eval/capenv.go`)

Spec §9.2 — threaded through computations.

```go
// CapEnv is a capability environment: label -> capability state.
// Copy-on-write: modifications return a new CapEnv.
type CapEnv struct {
    caps map[string]any
}

// EmptyCapEnv creates an empty capability environment.
func EmptyCapEnv() CapEnv

// NewCapEnv creates a CapEnv from a map.
func NewCapEnv(m map[string]any) CapEnv

// Get retrieves a capability by label.
func (c CapEnv) Get(label string) (any, bool)

// Set returns a new CapEnv with the label set to the given value.
// Does not modify the receiver (copy-on-write).
func (c CapEnv) Set(label string, val any) CapEnv

// Delete returns a new CapEnv with the label removed.
func (c CapEnv) Delete(label string) CapEnv

// Labels returns all capability labels.
func (c CapEnv) Labels() []string
```

**Copy-on-write implementation**: `Set` copies the underlying map only if modification is needed. Use a CoW wrapper:

```go
type CapEnv struct {
    caps map[string]any
    cow  bool // true if this map is shared and must be copied before write
}

func (c CapEnv) Set(label string, val any) CapEnv {
    if c.cow {
        newCaps := make(map[string]any, len(c.caps)+1)
        for k, v := range c.caps {
            newCaps[k] = v
        }
        newCaps[label] = val
        return CapEnv{caps: newCaps, cow: false}
    }
    c.caps[label] = val
    return c
}
```

**Linearity assumption**: The non-CoW fast path (`cow == false`) mutates the underlying map in-place and returns the struct. This is correct because the evaluator threads CapEnv linearly — each CapEnv value is consumed exactly once (passed to the next evaluation step, never retained). The Go type system does not enforce this linearity; it is maintained by the structure of `Eval`, where each call receives a CapEnv and produces a new one, and the caller never reuses the old value. The CoW flag serves as a safety net: when a CapEnv is shared (e.g., captured in a ThunkVal), it is marked `cow = true` so that subsequent `Set` calls copy instead of mutating.

### 3.4 Primitive Implementation Registry (`eval/prim.go`)

```go
// PrimImpl is the signature for host-provided primitive operations.
//
// Receives: context (for cancellation/timeout) + current capability environment + evaluated arguments.
// Returns:  result value + updated capability environment + error.
//
// The context.Context parameter allows the host to enforce timeouts and
// cancellation on blocking operations (network calls, DB queries, etc.).
// Implementations MUST check ctx.Done() for long-running operations.
//
// Error semantics:
//   - If the declared return type is Result e a: error should not be used;
//     use Result constructors instead (recoverable).
//   - Otherwise: error = abort (non-recoverable runtime error).
//   - context.Canceled / context.DeadlineExceeded: abort (non-recoverable).
type PrimImpl func(ctx context.Context, capEnv CapEnv, args []Value) (Value, CapEnv, error)

// PrimRegistry maps assumption names to their implementations.
type PrimRegistry struct {
    impls map[string]PrimImpl
}

func NewPrimRegistry() *PrimRegistry

// Register adds a primitive implementation.
func (r *PrimRegistry) Register(name string, impl PrimImpl)

// Lookup retrieves a primitive by name.
func (r *PrimRegistry) Lookup(name string) (PrimImpl, bool)
```

### 3.5 The Evaluator (`eval/eval.go`)

The `Evaluator` holds per-execution immutable context (`ctx`, `prims`) and per-execution mutable state (`limit`). Its single method `Eval` takes only the truly variable arguments: `env`, `capEnv`, `expr`.

```go
// EvalResult is the result of evaluation.
type EvalResult struct {
    Value  Value
    CapEnv CapEnv
}

// Evaluator is the per-execution evaluation engine.
//
// Created fresh for each Runtime.Run call. NOT goroutine-safe — a single
// Evaluator drives a single, single-threaded evaluation.
//
// Holds:
//   - ctx:   context.Context for cancellation/timeout (per-execution, immutable)
//   - prims: PrimRegistry for assumption lookup (shared, read-only)
//   - limit: step/depth budget (per-execution, mutable)
type Evaluator struct {
    ctx   context.Context
    prims *PrimRegistry
    limit *Limit
}

// NewEvaluator creates an Evaluator for a single execution.
// trace may be nil (no tracing overhead).
func NewEvaluator(ctx context.Context, prims *PrimRegistry, limit *Limit, trace TraceHook) *Evaluator {
    return &Evaluator{ctx: ctx, prims: prims, limit: limit, trace: trace}
}

// Eval evaluates a Core expression.
//
// The single entry point for all evaluation. CapEnv is threaded through
// every former — pure formers return it unchanged, computation formers
// (Pure, Bind, Force, PrimOp) may modify it.
//
// Spec §9.3 (value evaluation) and §9.4 (computation evaluation) are
// unified into this method.
func (ev *Evaluator) Eval(env *Env, capEnv CapEnv, expr core.Core) (EvalResult, error)
```

#### Case analysis within Eval

```go
func (ev *Evaluator) Eval(env *Env, capEnv CapEnv, expr core.Core) (EvalResult, error) {
    switch e := expr.(type) {

    case *core.Var:
        // ρ(x) = v  →  return (v, σ)
        v, ok := env.Lookup(e.Name)
        if !ok {
            return ..., unboundVarError(e)
        }
        return EvalResult{v, capEnv}, nil

    case *core.Lam:
        // \x -> body  →  Closure(env, x, body)
        return EvalResult{&Closure{env, e.Param, e.Body}, capEnv}, nil

    case *core.App:
        // Eval fun, eval arg, apply
        funR, err := ev.Eval(env, capEnv, e.Fun)
        // ... eval arg with funR.CapEnv ...
        // ... apply closure ...

    case *core.TyApp:
        // Type application is erased at runtime. Evaluate the expression.
        return ev.Eval(env, capEnv, e.Expr)

    case *core.Con:
        // Evaluate all args left-to-right, build ConVal.
        // Thread capEnv through args (though args should be pure,
        // capEnv passes through unchanged).

    case *core.Case:
        // Eval scrutinee, match against alts, eval matching branch.

    case *core.Pure:
        // Eval the inner expression, return (v, σ) unchanged.
        return ev.Eval(env, capEnv, e.Expr)

    case *core.Bind:
        // Eval c1 → (v1, σ')
        // Extend env with x=v1
        // Eval c2 with σ'  → (v2, σ'')
        // Return (v2, σ'')

    case *core.Thunk:
        // Capture env + computation, return ThunkVal.
        return EvalResult{&ThunkVal{env, e.Comp}, capEnv}, nil

    case *core.Force:
        // Eval argument to ThunkVal, then eval captured computation
        // with captured env and current capEnv.

    case *core.PrimOp:
        // Eval all args, look up PrimImpl via ev.prims, call it with ev.ctx.

    case *core.LetRec:
        // Knot-tying: create closures with placeholder environments,
        // then backpatch to establish self-reference.
        //
        // Bindings must be lambdas (enforced by checker).
        recEnv := env
        closures := make([]*Closure, len(e.Bindings))
        for i, b := range e.Bindings {
            lam := b.Expr.(*core.Lam)
            clo := &Closure{Env: nil, Param: lam.Param, Body: lam.Body}
            closures[i] = clo
            recEnv = recEnv.Extend(b.Name, clo)
        }
        // Backpatch: each closure's Env now points to recEnv,
        // which contains all sibling closures — establishing the cycle.
        for _, clo := range closures {
            clo.Env = recEnv
        }
        return ev.Eval(recEnv, capEnv, e.Body)
    }
}
```

#### CapEnv threading details

Pure formers (`Var`, `Lam`, `Con`, `Thunk`) do not modify CapEnv. However, they still receive and return it to maintain the single-function signature.

Argument evaluation in `App` and `Con`: arguments are evaluated left-to-right, threading CapEnv. In practice, arguments to constructors and functions are pure values, so CapEnv passes through unchanged. But the uniform threading ensures correctness if future extensions allow computation-valued arguments.

### 3.6 Pattern Matching (`eval/match.go`)

Spec §9.5.

```go
// Match attempts to match a value against a pattern.
// Returns the bindings on success, or nil on failure.
func Match(val Value, pat core.Pattern) map[string]Value
```

Implementation:

```go
func Match(val Value, pat core.Pattern) map[string]Value {
    switch p := pat.(type) {
    case *core.PVar:
        return map[string]Value{p.Name: val}
    case *core.PWild:
        return map[string]Value{}
    case *core.PCon:
        cv, ok := val.(*ConVal)
        if !ok || cv.Con != p.Con || len(cv.Args) != len(p.Args) {
            return nil
        }
        bindings := map[string]Value{}
        for i, arg := range p.Args {
            sub := Match(cv.Args[i], arg)
            if sub == nil {
                return nil // match failed in sub-pattern
            }
            for k, v := range sub {
                bindings[k] = v
            }
        }
        return bindings
    }
    return nil
}
```

### 3.7 Step Limit (`eval/limit.go`)

Defense-in-depth: even without general recursion, the evaluator enforces a step limit. Protects against type checker bugs, pathological fold inputs (future), and ensures the host always retains control.

```go
// Limit tracks evaluation budget.
type Limit struct {
    remaining int
    maxDepth  int
    depth     int
}

// NewLimit creates a Limit with the given step and depth budgets.
// Called by Runtime.Run for each execution.
func NewLimit(steps, maxDepth int) *Limit {
    return &Limit{remaining: steps, maxDepth: maxDepth}
}

// DefaultLimit returns a Limit with default budgets.
func DefaultLimit() *Limit {
    return NewLimit(1_000_000, 1_000)
}

// Step decrements the counter. Returns error at zero.
func (l *Limit) Step() error {
    l.remaining--
    if l.remaining <= 0 {
        return &StepLimitError{}
    }
    return nil
}

// Enter increments call depth. Returns error if max exceeded.
func (l *Limit) Enter() error {
    l.depth++
    if l.depth > l.maxDepth {
        return &DepthLimitError{}
    }
    return nil
}

// Leave decrements call depth.
func (l *Limit) Leave() {
    l.depth--
}

type StepLimitError struct{}
func (e *StepLimitError) Error() string { return "step limit exceeded" }

type DepthLimitError struct{}
func (e *DepthLimitError) Error() string { return "call depth limit exceeded" }
```

Integration with `Eval`: every entry into `Eval` calls `ev.limit.Step()` and checks `ev.ctx`. Every `App` (function application) and `Force` call `ev.limit.Enter()` / `ev.limit.Leave()`.

```go
func (ev *Evaluator) Eval(env *Env, capEnv CapEnv, expr core.Core) (EvalResult, error) {
    // Check context cancellation (non-blocking).
    select {
    case <-ev.ctx.Done():
        return EvalResult{}, ev.ctx.Err()
    default:
    }
    // Check step limit.
    if err := ev.limit.Step(); err != nil {
        return EvalResult{}, err
    }
    // ... switch on expr ...
}
```

### 3.8 Termination Guarantee

**v0.3 property**: without `LetRec` or `rec` capability, all well-typed programs terminate.

Proof sketch: the Core IR has no self-referential construction. Each `Eval` call processes a strict sub-expression of the input. The call graph is a finite tree with no cycles. Therefore evaluation terminates in O(|program|) steps.

The step limit is **not** the source of this guarantee — it is defense-in-depth. Termination follows from the absence of recursion in the core calculus.

When `LetRec` or `rec`-as-capability is added (future), termination is no longer guaranteed and the step limit becomes essential.

### 3.9 Runtime Errors (`eval/error.go`)

```go
// RuntimeError represents an error during evaluation.
type RuntimeError struct {
    Message string
    Span    span.Span
}

func (e *RuntimeError) Error() string
```

Error conditions:
- Unbound variable (should not happen after type checking)
- Non-exhaustive pattern match (should not happen after exhaustiveness check)
- Missing primitive implementation (host registration error)
- Primitive returned an error (abort)
- Force applied to non-ThunkVal (should not happen after type checking)
- Application of non-function (should not happen after type checking)
- Context cancellation (`context.Canceled` / `context.DeadlineExceeded`)

Post-type-checking, most of these are internal consistency errors. The evaluator can panic for "impossible" states or return descriptive errors — implementation choice.

### 3.10 Evaluation Tracing (`eval/trace.go`)

Optional per-step callback for debugging evaluation. If nil, no overhead (single branch per step).

```go
// TraceEvent describes one evaluation step.
type TraceEvent struct {
    Depth  int       // current call depth
    Node   core.Core // Core node about to be evaluated
    Env    *Env      // current variable environment
    CapEnv CapEnv    // current capability environment
}

// TraceHook is called before each evaluation step.
// Returning a non-nil error aborts evaluation (useful for conditional breakpoints).
type TraceHook func(TraceEvent) error
```

The Evaluator accepts an optional TraceHook:

```go
type Evaluator struct {
    ctx   context.Context
    prims *PrimRegistry
    limit *Limit
    trace TraceHook  // optional, nil = no tracing
    stats EvalStats  // accumulated during evaluation
}

func NewEvaluator(ctx context.Context, prims *PrimRegistry, limit *Limit, trace TraceHook) *Evaluator
```

Integration with Eval:

```go
func (ev *Evaluator) Eval(env *Env, capEnv CapEnv, expr core.Core) (EvalResult, error) {
    // ... context + step checks ...
    ev.stats.Steps++
    if d := ev.limit.Depth(); d > ev.stats.MaxDepth {
        ev.stats.MaxDepth = d
    }
    if ev.trace != nil {
        if err := ev.trace(TraceEvent{
            Depth: ev.limit.Depth(), Node: expr, Env: env, CapEnv: capEnv,
        }); err != nil {
            return EvalResult{}, err
        }
    }
    // ... switch on expr ...
}
```

### 3.11 Evaluation Statistics (`eval/stats.go`)

```go
// EvalStats holds post-evaluation statistics.
type EvalStats struct {
    Steps    int // total evaluation steps consumed
    MaxDepth int // peak call depth reached
}

// Stats returns the accumulated statistics.
// Call after Eval completes.
func (ev *Evaluator) Stats() EvalStats
```

Step counting and depth tracking are always active (no overhead beyond two integer increments per step). The host can use `Stats()` to diagnose performance without enabling the full trace hook.

## Test Strategy

### Unit tests

- **Value construction**: verify String() representations.
- **Env**: extend, lookup, shadowing, parent chain lookup.
- **CapEnv**: get, set (copy-on-write verified — original unchanged), delete.
- **Match**: variable binding, wildcard, constructor matching, nested patterns, mismatch.

### Integration tests (Core → Value)

Build Core terms programmatically and evaluate them:

1. **Pure value**: `Pure(Con("Unit", []))` → `ConVal("Unit", [])`
2. **Lambda + App**: `App(Lam("x", Var("x")), Con("Unit", []))` → `ConVal("Unit", [])`
3. **Bind sequencing**: `Bind(Pure(hostVal(42)), "x", Pure(Var("x")))` → `HostVal(42)`, CapEnv unchanged
4. **CapEnv threading**: register a prim that modifies CapEnv, verify final CapEnv state
5. **Thunk/Force**: `Force(Thunk(Pure(hostVal(1))))` → `HostVal(1)`
6. **Thunk not memoized**: force a thunk that increments a counter twice, verify counter = 2
7. **Case analysis**: build ConVal, match with PCon, verify correct branch taken
8. **Type erasure**: TyApp returns same value as inner expression
9. **PrimOp**: register identity prim, call it, verify result

### Step limit tests

10. **Step limit**: construct a deep nested App chain, set low limit, verify `StepLimitError`.
11. **Depth limit**: construct deeply nested function calls, verify `DepthLimitError`.
12. **Normal execution within limits**: typical program completes without hitting limits.

### Context cancellation tests

13. **Cancelled context**: pass an already-cancelled context, verify `context.Canceled` returned immediately.
14. **Timeout during PrimImpl**: register a slow prim, set short deadline, verify `context.DeadlineExceeded`.
15. **Normal completion with context**: pass a context with ample deadline, verify normal evaluation completes.

### Trace and stats tests

16. **TraceHook receives events**: register a hook that collects events, verify Core nodes appear in order.
17. **TraceHook abort**: hook returns error → evaluation stops, error propagated.
18. **Nil trace no overhead**: nil hook → no trace calls (benchmark).
19. **Stats step count**: run a known program, verify `Stats().Steps` matches expected.
20. **Stats max depth**: nested function calls, verify `Stats().MaxDepth` matches nesting.

### Property tests

- `ev.Eval(env, cap, Pure(e))` never modifies CapEnv for any `e`.
- `ev.Eval(env, cap, Thunk(c))` always returns ThunkVal without evaluating `c`.
- CapEnv copy-on-write: original CapEnv is never mutated.

## Completion Criteria

- [ ] All four value types constructible
- [ ] Env and CapEnv operations correct with copy-on-write
- [ ] PrimRegistry works
- [ ] Evaluator type encapsulates ctx, prims, limit
- [ ] Eval method handles all 13 Core formers
- [ ] Pattern matching correct for all pattern types
- [ ] CapEnv threading verified (pure = pass-through, bind = thread)
- [ ] Type erasure (TyApp/TyLam) works
- [ ] Step limit and depth limit enforced
- [ ] Context cancellation terminates evaluation promptly
- [ ] PrimImpl receives context and can respect cancellation/timeout
- [ ] TraceHook receives correct events in order
- [ ] TraceHook abort stops evaluation
- [ ] EvalStats tracks steps and max depth accurately
- [ ] Runtime errors descriptive
- [ ] All tests pass
