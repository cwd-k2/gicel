# Supplement: Research Document Findings

Domain research documents (`docs/`) contain implementation details not fully reflected in the phase plans. This supplement records significant items, classified by whether they should be adopted, deferred, or noted.

## Adopt — Incorporate into implementation

### S.1 DK-Specific Context Operations (→ Phase 5)

The DK bidirectional algorithm requires ordered-context operations beyond Push/Pop:

```go
// InsertBefore inserts entries before a target entry.
// Needed for DK instantiation rules.
func (c *Context) InsertBefore(targetID int, entries ...CtxEntry)

// IsToLeftOf checks whether entry `left` precedes `right` in the context.
// Needed for well-scopedness checks during instantiation.
func (c *Context) IsToLeftOf(leftID, rightID int) bool
```

Source: `type-checker-architecture.md`

### S.2 InstantiateL / InstantiateR (→ Phase 5)

The DK algorithm distinguishes left-instantiation (existential ≤ type) from right-instantiation (type ≤ existential). These are not the same operation.

```go
// InstantiateL solves ?α ≤ B (existential on the left).
func (ch *Checker) instantiateL(meta *types.TyMeta, b types.Type)

// InstantiateR solves A ≤ ?α (existential on the right).
func (ch *Checker) instantiateR(a types.Type, meta *types.TyMeta)
```

Source: `type-checker-architecture.md`, `bidirectional-typing-with-rows-and-indexed-types.md`

### S.3 AppInfer Judgment (→ Phase 5)

Application typing in DK is its own judgment, not just "matchArrow + check." It handles instantiation of forall types at application sites:

```go
// appInfer implements Γ ⊢ A • e ⇒ C — given function type A and argument e, produce result type C.
func (ch *Checker) appInfer(funType types.Type, arg syntax.Expr) (types.Type, core.Core)
```

Source: `type-checker-architecture.md`

### S.4 ErrorType Poison Value (→ Phase 5)

For error recovery without cascading failures, introduce a poison type that unifies with anything:

```go
// TyError is a poison type inserted after a type error.
// It unifies with any other type without producing further errors.
type TyError struct {
    S span.Span
}
```

When the checker encounters an error, it inserts `TyError` and continues. The unifier must special-case `TyError`: `Unify(TyError, T) = success` for any `T`.

Source: `type-checker-architecture.md`

### ~~S.5 TyLam in Core IR~~ → ADOPTED in Spec §13.1, Phase 2

TyLam added as 13th Core former in both the specification and the plan. `TyLam` / `TyApp` form the introduction/elimination pair for polymorphism. `TyLam` is erased at runtime.

### S.6 Specific Error Categories (→ Phase 0, Phase 5)

Replace the generic `Code int` with named categories:

```go
const (
    ErrTypeMismatch          Code = iota + 1  // E0001
    ErrRowMismatch                             // E0002
    ErrRowLabelConflict                        // E0003
    ErrInfiniteType                            // E0004
    ErrNotInScope                              // E0005
    ErrKindMismatch                            // E0006
    ErrAmbiguousType                           // E0007
    ErrMissingAnnotation                       // E0008
    ErrDuplicateLabel                          // E0009
    ErrNonExhaustivePatterns                   // E0010
    ErrRedundantPattern                        // E0011
    ErrAssumptionNoAnnotation                  // E0012
    ErrMissingPrimImpl                         // E0013
)
```

Source: `type-error-reporting.md`

### S.7 Max Errors Limit (→ Phase 5)

Stop collecting diagnostics after N errors to prevent cascade noise:

```go
type Checker struct {
    // ...
    maxErrors int  // default: 20
}
```

Source: `type-checker-architecture.md`

### S.8 Golden-File Test Infrastructure (→ Phase 7)

Use `.gmp` / `.golden` file pairs in `testdata/`:

```
testdata/
  check/
    identity.gmp        — source
    identity.golden      — expected Core IR pretty-print
  errors/
    row-mismatch.gmp    — source with error
    row-mismatch.golden  — expected error output
  eval/
    bind-chain.gmp
    bind-chain.golden    — expected final value
```

Source: `type-checker-architecture.md`, `evaluation-semantics.md`

### S.9 FlatRow as Unification-Internal Type (→ Phase 5)

During unification, normalize rows into a flat canonical form distinct from `TyRow`:

```go
// FlatRow is the canonical form for row unification.
// Not part of the public Type interface — internal to the unifier.
type FlatRow struct {
    Fields []types.RowField  // sorted by label
    Tail   *types.TyMeta     // nil = closed
}

func flatten(r types.Type) (*FlatRow, error)
```

This avoids modifying `TyRow` during unification and provides a clean separation between the type representation and the unification algorithm.

Source: `row-unification-algorithms.md`

## Defer — Note for future, not v0

### ~~S.10 Fuel / Step Counting~~ → ADOPTED in Phase 3

Moved to Phase 3 eval as `Limit` type with step count + call depth. No longer deferred — active as defense-in-depth from v0.

### ~~S.14 context.Context for PrimImpl~~ → ADOPTED in Phase 3, 6

`PrimImpl` receives `context.Context` as first parameter. `Eval` also receives `context.Context` and checks cancellation at each step. Engine provides `RunContext` method. This guards against PrimImpl blocking (network, DB) and enables host-side timeout enforcement.

### ~~S.15 Type Alias Cycle Detection~~ → ADOPTED in Phase 5

`validateAliasGraph` detects cycles in type alias definitions via DFS with three-color marking. Called during declaration processing (fourth pass) before any alias expansion. Prevents infinite alias expansion from `type A = B; type B = A`.

### S.11 PrimOp as Go Interface (→ future)

The research docs suggest `PrimOp` as an interface with `Name()`, `Type()`, `Execute()`. Our confirmed design uses `PrimImpl` as a function type. The interface form could be adopted later if the registration API needs richer introspection.

### S.12 VarEnv as Cons-Cell Linked List (→ implementation choice)

The research docs suggest a cons-cell `struct { parent *VarEnv; name string; value Value }` instead of `map[string]Value`. Trade-off:

- Linked list: zero allocation per Extend, O(n) lookup.
- Map-per-scope: allocation per scope, O(1) lookup.

For typical program depth (< 50 scopes), linked list is likely faster. Decide during Phase 3 implementation based on profiling.

### S.13 Annotated Core Bind (→ future)

The research docs suggest `CoreBind` carrying explicit row annotations (`PreRow`, `MidRow`, `PostRow`). Our `core.Bind` stores only the Core terms. The annotated form enables independent re-checking of Core IR without re-running inference. Useful for a future Core-level optimizer or verifier, but not needed for v0.

## Confirmed Design Conflicts

The following research doc suggestions were explicitly overridden by confirmed design decisions. They are recorded here to prevent revisiting.

| Research suggestion | Confirmed decision | Rationale |
|---|---|---|
| Separate `evalValue` / `evalComp` | Single `eval` function | Unified function with CapEnv threading; pure formers pass CapEnv through unchanged |
| `PrimOp` as Go interface | `PrimImpl` as `func` type | Simpler registration; type provided separately |
| `if`/`then`/`else` in grammar | Removed (use `case` on `Bool`) | Minimal surface; Bool is ordinary ADT |
| 14 keywords | 9 keywords | Reduced surface after removing if/let/primitive |
| `primitive` keyword | `assumption` built-in identifier | Better semantics (program's assumption about host) |
