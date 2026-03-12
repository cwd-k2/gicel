# Phase 7: Integration

## Objective

Implement the prelude (standard library), end-to-end integration tests, example programs, and polish. This phase validates that all previous phases compose correctly.

## Dependencies

Phases 0–6.

## 7.1 Prelude (`prelude/`)

The prelude is **Gomputation source code**, not Go code. It defines standard types and aliases that every program implicitly imports.

### Prelude Source (`prelude/prelude.gmp`)

```text
-- Standard algebraic data types
data Bool = True | False
data Unit = Unit
data Pair a b = Pair a b
data Maybe a = Just a | Nothing
data Result e a = Ok a | Err e

-- Convenience type alias
type Effect r a = Computation r r a
```

### Prelude Loader (`prelude/prelude.go`)

```go
package prelude

import _ "embed"

//go:embed prelude.gmp
var Source string

// Declarations returns the prelude source text.
func Declarations() string {
    return Source
}
```

The engine prepends the prelude source to user source before parsing. Alternative: parse prelude separately and merge ASTs. The prepend approach is simpler and preserves source positions.

### Built-in vs Prelude Boundary

| Item | Where | Why |
|------|-------|-----|
| `pure`, `bind` | Built-in (checker + evaluator) | Elaborate to Core.Pure / Core.Bind |
| `thunk` | Built-in (checker + evaluator) | Term former, not function |
| `force` | Built-in (evaluator) | Could be function but needs ThunkVal dispatch |
| `assumption` | Built-in (checker) | Declaration-level construct |
| `Bool`, `Unit`, `Result`, `Maybe`, `Pair` | Prelude source | Ordinary ADTs |
| `Effect` | Prelude source | Ordinary type alias |
| `Computation`, `Thunk` | Built-in type constructors | Checker and evaluator know their kind/semantics |
| `Int`, `String`, etc. | Host-registered opaque types | No prelude definition |

## 7.2 End-to-End Test Suite

### Test infrastructure

```go
package integration_test

// newTestEngine creates a pre-configured Engine with standard opaque types.
func newTestEngine() *host.Engine {
    engine := host.New()
    engine.TypeEnv().RegisterType("Int", types.KType{})
    engine.TypeEnv().RegisterType("String", types.KType{})
    return engine
}

// runProgram is a test helper: source text → final value.
// Uses Engine.Run convenience method (compiles + runs in one shot).
func runProgram(t *testing.T, source string, caps map[string]any, bindings map[string]any, entry string) any {
    result, err := newTestEngine().Run(source, caps, bindings, entry)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    return result
}

// compileAndRun separates compilation from execution for tests
// that need to inspect the Runtime or run the same program multiple times.
func compileAndRun(t *testing.T, source string, caps map[string]any, bindings map[string]any, entry string) any {
    rt, err := newTestEngine().NewRuntime(source)
    if err != nil {
        t.Fatalf("compilation error: %v", err)
    }
    result, err := rt.Run(caps, bindings, entry)
    if err != nil {
        t.Fatalf("runtime error: %v", err)
    }
    return result
}

// runProgramWithContext uses RunContext for cancellation/timeout tests.
func runProgramWithContext(t *testing.T, ctx context.Context, source string, caps map[string]any, bindings map[string]any, entry string) (any, error) {
    rt, err := newTestEngine().NewRuntime(source)
    if err != nil {
        return nil, err
    }
    return rt.RunContext(ctx, caps, bindings, entry)
}

// checkFails is a test helper: source text → expect type error.
func checkFails(t *testing.T, source string, expectedCode errs.Code) {
    err := newTestEngine().Check(source)
    // Verify error code matches.
}
```

### Test categories

#### A. Pure computation (no effects)

1. **Identity**: `main := \x -> x` applied to host-injected value.
2. **Constructor + case**: `data Color = Red | Blue; main := case Red of { Red -> Unit; Blue -> Unit }`.
3. **Nested case**: case on `Result`, branch on `Ok`/`Err`.
4. **Block expression**: `{ x := f a; g x }` desugars correctly.
5. **Multi-param lambda**: `\x y -> Pair x y`.
6. **Type application**: explicit `@Int`.
7. **Polymorphic identity**: `id @Bool True`.

#### B. Computation (with effects)

8. **Single assumption**: register one prim, call it, verify CapEnv change.
9. **Bind chain**: `dbOpen; dbQuery; dbClose` — full protocol.
10. **do block sugar**: equivalent to explicit bind chain.
11. **Pure in do**: `do { x := helper arg; pure x }`.
12. **Thunk creation**: `thunk dbClose` creates ThunkVal without executing.
13. **Thunk force**: `force (thunk (pure Unit))` evaluates to Unit.
14. **Thunk not memoized**: force same thunk twice, verify two executions.

#### C. Row polymorphism

15. **Open row**: assumption with `| r`, compose with other capabilities.
16. **Multiple capabilities**: `{ db : ..., log : ... }` — two capabilities in one program.
17. **Row variable unification**: open-open unification in bind chain.

#### D. General recursion (rec)

15.5. **rec without EnableRecursion**: program uses `rec` → unbound variable error.
16. **rec enabled, simple recursion**: factorial via `rec`, verify result.
17. **rec step limit**: divergent program hits step limit → `StepLimitError`.
18. **rec depth limit**: deeply recursive program hits depth limit → `DepthLimitError`.
19. **rec state-preserving**: `rec` body has `pre = post`, verify capability state restored each iteration.
20. **rec with fold**: use `fold` inside `rec` body, verify composition.

#### E. Type errors (negative tests)

18. **Mismatched capability**: `dbQuery` without `dbOpen` first → type error.
19. **Non-exhaustive case**: missing constructor → error.
20. **Unbound variable**: reference to undefined name → error.
21. **Kind error**: `Computation Int Int Int` (Int where Row expected) → error.
22. **Missing annotation on assumption**: `dbOpen := assumption` without `::` → error.
23. **Duplicate label**: row with `{ db : A, db : B }` → error.
24. **Infinite type**: occurs check → error.

#### F. Context cancellation

26. **RunContext with timeout**: set short deadline, run program with slow assumption → `context.DeadlineExceeded`.
27. **RunContext cancellation**: cancel context during evaluation → `context.Canceled`.
28. **Run (no context)**: default `context.Background()` used, program completes normally.

#### G. Type alias cycles (negative tests)

29. **Direct cycle**: `type A = A` → cyclic alias error.
30. **Mutual cycle**: `type A = B; type B = A` → cyclic alias error with path.
31. **Transitive cycle**: `type A = B; type B = C; type C = A` → cyclic alias error.
32. **No cycle (DAG)**: `type A = B; type B = Int` → succeeds.

#### H. Runtime reuse

33. **Compile once, run twice**: same Runtime, different bindings → different results.
34. **Concurrent runs**: same Runtime, two goroutines calling `RunContext` → both succeed, no data races (`go test -race`).

#### E. Spec §15 reproduction

25. **Full example**: the spec §15 database program, with mock db host implementations.

```go
func TestSpecExample(t *testing.T) {
    engine := host.New()

    // Register opaque types.
    engine.TypeEnv().RegisterType("Query", types.KType{})
    engine.TypeEnv().RegisterType("Rows", types.KType{})
    engine.TypeEnv().RegisterType("DB", types.KArrow{From: types.KType{}, To: types.KType{}})
    engine.TypeEnv().RegisterType("DBState", types.KType{})  // user-defined, but host needs to know

    // Register assumptions.
    engine.Registry().Register("dbOpen", dbOpenType, func(ctx context.Context, cap eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
        return unitVal, cap.Set("db", &mockDB{state: "opened"}), nil
    })
    engine.Registry().Register("dbClose", dbCloseType, func(ctx context.Context, cap eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
        return unitVal, cap.Set("db", &mockDB{state: "closed"}), nil
    })
    engine.Registry().Register("dbQuery", dbQueryType, func(ctx context.Context, cap eval.CapEnv, args []eval.Value) (eval.Value, eval.CapEnv, error) {
        return okVal(mockRows), cap, nil
    })

    source := `
        data DBState = Opened | Closed

        dbOpen :: forall r. Computation { db : DB Closed | r } { db : DB Opened | r } Unit
        dbOpen := assumption

        dbClose :: forall r. Computation { db : DB Opened | r } { db : DB Closed | r } Unit
        dbClose := assumption

        dbQuery :: forall r. Query -> Computation { db : DB Opened | r } { db : DB Opened | r } (Result String Rows)
        dbQuery := assumption

        main :: Query -> Computation { db : DB Closed } { db : DB Closed } (Result String Rows)
        main := \query -> do {
            dbOpen;
            result <- dbQuery query;
            dbClose;
            pure result
        }
    `

    // Compile once — could reuse rt for multiple queries.
    rt, err := engine.NewRuntime(source)
    if err != nil {
        t.Fatalf("compilation error: %v", err)
    }

    result, err := rt.Run(map[string]any{"db": &mockDB{state: "closed"}}, map[string]any{"query": mockQuery}, "main")
    // Assert result is Ok(mockRows).
    // Assert final CapEnv has db in "closed" state.
}
```

## 7.3 Error Message Quality

Verify that error messages are clear and actionable:

- Type mismatch shows expected vs actual with pretty-printed types.
- Row mismatch shows which labels differ.
- Non-exhaustive case shows missing constructors.
- Source location (line:col) is accurate.

Golden-file tests: store expected error output in `testdata/errors/` and compare.

## 7.4 Performance Baseline

Not a hard requirement, but establish baselines:

- Parse 1000-line program: < 10ms.
- Type-check 1000-line program: < 50ms.
- Evaluate 10,000 bind steps: < 100ms.
- Memory: evaluator should not leak (CapEnv CoW verified).

Use `testing.B` benchmarks.

## Completion Criteria

- [ ] Prelude source parses and type-checks correctly
- [ ] Prelude types (Bool, Unit, Result, etc.) available in user programs
- [ ] Effect type alias works
- [ ] All 34+ integration tests pass
- [ ] Runtime reuse verified (compile once, run many)
- [ ] Concurrent Runtime.RunContext passes race detector
- [ ] Spec §15 example runs correctly
- [ ] Error messages are clear and include source locations
- [ ] No panics in any test (all errors returned as values)
- [ ] Performance baselines established
- [ ] `go vet ./...` clean
- [ ] `go test ./...` all pass
