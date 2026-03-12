# Typed Embedded Language Host Boundary Design Patterns

One-line description: exhaustive analysis of Go implementation patterns at the boundary between an embedded typed effect language and its Go host, grounded in the real designs of Starlark-go, CEL-go, Tengo, and goja.

## Table of Contents

1. Comparison of Go Embedded Language Implementations
2. Type-Safe Host Function Registration
3. Value Representation at the Boundary
4. Error Handling Across the Boundary
5. Capability Environment Runtime Representation
6. Determinism and Host Interaction
7. API Design Patterns
8. Security Considerations
9. Specific Recommendations for Gomputation
10. Key References

---

## 1. Comparison of Go Embedded Language Implementations

### 1.1 Overview Table

| Dimension | Starlark-go | CEL-go | Tengo | goja |
|---|---|---|---|---|
| **Language model** | Python dialect, dynamically typed | Expression language, gradually typed | Custom scripting, dynamically typed | ECMAScript 5.1, dynamically typed |
| **Turing complete** | Yes | No (by design) | Yes | Yes |
| **Value type** | `Value` interface (7 methods) | `ref.Val` interface | `Object` interface (5 methods) | `Value` interface (opaque) |
| **Host function signature** | `func(*Thread, *Builtin, Tuple, []Tuple) (Value, error)` | `func(...ref.Val) ref.Val` via overload bindings | `func(...Object) (Object, error)` | `func(FunctionCall) Value` or arbitrary Go func |
| **Type checking** | None (dynamic) | Static, optional but recommended | None (dynamic) | None (dynamic) |
| **Determinism** | Designed for it: no time, no randomness, no I/O | Side-effect free evaluation | Not a design goal | `SetTimeSource` for testing only |
| **Parse/Check/Run pipeline** | Parse + Exec (no separate check) | Parse -> Check -> Program -> Eval | Compile -> Run | Compile -> RunProgram |
| **Thread safety** | Single-thread per `Thread` | `Program` is thread-safe and cacheable | `Compiled.Clone()` for goroutine use | Single-goroutine per `Runtime` |
| **Resource limits** | `SetMaxExecutionSteps`, `Cancel` | Bounded by non-Turing completeness | `SetMaxAllocs`, stack/frame limits | `Interrupt`, `SetMaxCallStackSize` |
| **Cancellation** | `Thread.Cancel(reason)` from any goroutine | N/A (expressions terminate) | `RunContext(ctx)` | `Runtime.Interrupt(v)` from any goroutine |
| **Immutability** | `Freeze()` on all values | Values are immutable by design | `ImmutableArray`, `ImmutableMap` | JavaScript semantics (mutable) |
| **Custom types** | Implement `Value` + optional interfaces | `ref.Type` + `TypeAdapter` + `TypeProvider` | Embed `ObjectImpl`, implement methods | Go structs auto-reflected |
| **Argument unpacking** | `UnpackArgs`, `UnpackPositionalArgs` | Overload dispatch by type | Variadic `...Object` | Automatic reflection or manual `FunctionCall` |

### 1.2 Host Function Registration Patterns

**Starlark-go** uses a uniform signature:

```go
fn := starlark.NewBuiltin("openDB", func(
    thread *starlark.Thread,
    fn *starlark.Builtin,
    args starlark.Tuple,
    kwargs []starlark.Tuple,
) (starlark.Value, error) {
    // Implementation
})
predeclared := starlark.StringDict{"openDB": fn}
```

All builtins share one function signature. Type safety is achieved by `UnpackArgs` at runtime, not at registration time. This is simple but moves all type errors to runtime.

**CEL-go** uses typed overload declarations:

```go
env, _ := cel.NewEnv(
    cel.Function("greet",
        cel.MemberOverload("string_greet_string",
            []*cel.Type{cel.StringType, cel.StringType},
            cel.StringType,
            cel.BinaryBinding(func(lhs, rhs ref.Val) ref.Val {
                return types.String(fmt.Sprintf("Hello %s, I'm %s", rhs, lhs))
            }),
        ),
    ),
)
```

Functions are declared with explicit input/output types. The type checker validates calls against these declarations before evaluation. This is the closest existing model to what Gomputation needs.

**Tengo** uses the simplest approach:

```go
script := tengo.NewScript(src)
script.Add("openDB", &tengo.UserFunction{
    Name: "openDB",
    Value: func(args ...tengo.Object) (tengo.Object, error) {
        // Implementation
    },
})
```

No type information is attached at registration. Entirely runtime-checked.

**goja** provides two modes:

```go
// Mode 1: Automatic reflection-based wrapping
vm.Set("add", func(a, b int) int { return a + b })

// Mode 2: Manual value handling (faster)
vm.Set("add", func(call goja.FunctionCall) goja.Value {
    a := call.Argument(0).ToInteger()
    b := call.Argument(1).ToInteger()
    return vm.ToValue(a + b)
})
```

Mode 1 is ergonomic but uses reflection. Mode 2 avoids reflection but loses type information at the API boundary.

### 1.3 Patterns and Anti-Patterns Observed

**Pattern: Uniform function signature.** All four implementations converge on a single canonical function signature for host functions. This enables a single dispatch mechanism without per-function code generation.

**Pattern: Separate parse and execution phases.** CEL-go's three-phase pipeline (parse, check, program) is the gold standard. Starlark-go combines parse and execution, which limits optimization and caching.

**Pattern: Value interface with optional capability interfaces.** Starlark-go's design where `Value` is minimal and capabilities like `HasAttrs`, `Iterable`, `Indexable` are opt-in interfaces is clean and extensible. CEL-go's `ref.Val` with traits follows the same principle.

**Anti-pattern: Automatic Go struct reflection.** goja's automatic struct mapping is convenient but breaks encapsulation. Fields become visible to scripts unless explicitly hidden via struct tags. This is incompatible with capability-based security.

**Anti-pattern: `interface{}` tunneling.** Tengo's `Add(name string, value interface{})` loses all type information and defers everything to runtime. For a statically typed embedded language, this is the wrong model.

**Anti-pattern: Mutable shared state.** goja's JavaScript-style mutability makes it difficult to reason about determinism or capability safety. Starlark's `Freeze()` mechanism is a better fit for deterministic languages.

---

## 2. Type-Safe Host Function Registration

### 2.1 The Problem

The spec shows:

```
dbOpen : Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
```

In Go, the host must register a Go function that:
1. Declares this type to the type checker
2. Implements the actual operation
3. Has its Go-level type align with the declared language-level type

The challenge is connecting these three concerns without losing safety or ergonomics.

### 2.2 Option A: String-Based Type Registration with Runtime Checking

```go
vm.RegisterOp("dbOpen",
    "forall r. Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit",
    func(env *CapEnv) (*CapEnv, Value, error) {
        handle := env.Get("db")
        db := handle.(*sqlDB)
        if err := db.conn.Open(); err != nil {
            return env, nil, err
        }
        env2 := env.Transition("db", DBOpened)
        return env2, UnitVal, nil
    },
)
```

**Advantages:**
- Most readable at the registration site
- Type string is the same language used in the spec
- The type checker parses and validates the string once at registration time
- No Go generics complexity

**Disadvantages:**
- Type strings can have typos undetected until the parser runs
- No compile-time connection between the declared type and the Go function's behavior
- The Go function signature is generic (`func(*CapEnv) (*CapEnv, Value, error)`) and could violate the declared type at runtime

**Mitigations:**
- Parse and validate the type string at `RegisterOp` call time (fail-fast at startup)
- Verify arity: if the declared type takes arguments, the Go function must accept that many `Value` parameters
- Run registration-time checks: the pre-row must mention labels the Go function accesses, and the post-row must be a valid transition from the pre-row

### 2.3 Option B: Go Generics-Based Registration

```go
type Op[Pre, Post RowConstraint, A any] struct {
    Name string
    Impl func(env *TypedEnv[Pre]) (*TypedEnv[Post], A, error)
}

// Registration
RegisterOp(Op[DBClosed, DBOpened, Unit]{
    Name: "dbOpen",
    Impl: func(env *TypedEnv[DBClosed]) (*TypedEnv[DBOpened], Unit, error) {
        // ...
    },
})
```

**Advantages:**
- Go compiler catches some type mismatches at compile time
- Self-documenting function signatures

**Disadvantages:**
- Go's type system cannot express row polymorphism, so `{ db : DB[Closed] | r }` has no Go-level representation
- Each capability state combination would need a distinct Go type, causing combinatorial explosion
- Go generics do not support higher-kinded types, so `Computation` as a type-level family cannot be represented
- The gap between Go's type system and the language's type system is too wide for useful compile-time checking

**Assessment:** This option is impractical for the full type system. It can be useful for narrow cases (e.g., ensuring a primitive returns `Unit` rather than `Int`), but it cannot represent row-polymorphic pre/post conditions.

### 2.4 Option C: Reflection-Based Registration with Type Descriptor Matching

```go
vm.RegisterOp("dbOpen", TypeDesc{
    Quantifiers: []RowVar{"r"},
    Pre:  Row{{"db", TypeApp("DB", Closed)}, Tail("r")},
    Post: Row{{"db", TypeApp("DB", Opened)}, Tail("r")},
    Result: UnitType,
}, openDBImpl)
```

where `TypeDesc` is a Go struct that mirrors the language's type AST.

**Advantages:**
- Structured rather than stringly-typed
- IDE autocompletion on type constructors
- Can be validated at registration time

**Disadvantages:**
- Verbose compared to the string form
- A parallel type representation in Go that must stay synchronized with the language's type grammar
- Still no compile-time connection to the Go implementation function

**Assessment:** This is a reasonable middle ground. It avoids string parsing errors while preserving structural validation. The verbosity can be reduced with builder helpers.

### 2.5 Option D: Code Generation for Host Primitive Stubs

A `go generate` tool reads declarations like:

```
primitive dbOpen : forall r. Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
```

and generates Go stub code:

```go
// GENERATED
func init() {
    vm.RegisterOp("dbOpen", parsedTypeForDbOpen, func(env *CapEnv) (*CapEnv, Value, error) {
        return dbOpenImpl(env)
    })
}

// User fills in:
func dbOpenImpl(env *CapEnv) (*CapEnv, Value, error) { ... }
```

**Advantages:**
- Single source of truth for primitive types
- Generated code can include runtime assertions
- The declaration file can be the same syntax as the language spec

**Disadvantages:**
- Build tooling dependency
- Generated code must be kept in sync
- Additional complexity in the development workflow

### 2.6 Recommendation

Use **Option A (string-based) as the primary API**, with the type string parsed and validated at registration time (fail-fast). Complement with **Option C (structured TypeDesc)** as an alternative for programmatic construction. Reserve Option D for a later phase when the primitive set stabilizes.

The critical invariant is: **type validation happens at registration time, not at first use.** The registered type becomes part of the static environment available to the type checker. If the type string is malformed or internally inconsistent, the program fails at startup before any user code is processed.

---

## 3. Value Representation at the Boundary

### 3.1 Options

**Option 1: `interface{}` (any)**

```go
type Value = any
```

Zero overhead, maximum flexibility, zero safety. This is what Tengo's `Script.Add` uses at its outermost layer. Unsuitable for a statically typed language because it erases the type information that the type checker established.

**Option 2: `Value` interface**

```go
type Value interface {
    Type() Type       // runtime type tag
    sealed()          // prevent external implementation
}
```

This is the approach used by Starlark-go, CEL-go, and (internally) goja. The sealed method (unexported) prevents code outside the package from creating fake values, which is essential for capability unforgeability.

**Option 3: Typed wrappers (sum type via interface)**

```go
type Value interface { sealed() }

type IntVal    struct{ V int64 }
type StringVal struct{ V string }
type UnitVal   struct{}
type FuncVal   struct{ /* closure */ }
type CompVal   struct{ /* suspended computation */ }
type HandleVal struct{ /* opaque capability handle */ }

func (IntVal)    sealed() {}
func (StringVal) sealed() {}
// ...
```

This is a refinement of Option 2 that makes the value variants explicit as Go types. Pattern matching (type switch) becomes exhaustive-checkable by linters.

### 3.2 Handle Types for Capabilities

Capability handles require special treatment. A `HandleVal` wraps a Go value that represents host-side state (e.g., a database connection), but that Go value must not be directly accessible to language-level code.

```go
type HandleVal struct {
    label    string       // capability label ("db")
    state    TypeIndex    // current protocol state (Opened, Closed, ...)
    internal any          // host-managed opaque value, never exposed to language
    sealed   sealToken    // unexported type prevents forgery
}
```

**Preventing capability forgery:**

1. **Sealed interface:** The `Value` interface has an unexported method. External packages cannot create a type that satisfies `Value`, so they cannot forge a `HandleVal`.

2. **Opaque internal field:** The `internal` field is unexported. Even if user code obtains a `HandleVal` through the language, it cannot extract the underlying Go value (e.g., `*sql.DB`) via the language's own operations.

3. **No reflection exposure:** The runtime must never expose `HandleVal` through operations that allow reflection-like inspection. The language has no `reflect` equivalent. Host functions that need the internal value receive it through the runtime, not through the language.

4. **State immutability:** `HandleVal` instances are conceptually immutable from the language's perspective. State transitions produce new `HandleVal` values (or mutate the capability environment, which is host-managed and not language-accessible).

### 3.3 Go-to-Language Value Conversion

The conversion boundary should be explicit and narrow:

```go
// Host-to-language conversion (used at registration and result injection)
func WrapInt(v int64) Value          { return IntVal{V: v} }
func WrapString(v string) Value      { return StringVal{V: v} }
func WrapBool(v bool) Value          { return BoolVal{V: v} }
func WrapHandle(label string, state TypeIndex, v any) Value {
    return HandleVal{label: label, state: state, internal: v}
}

// Language-to-host conversion (used in host function implementations)
func UnwrapInt(v Value) (int64, bool) {
    if iv, ok := v.(IntVal); ok { return iv.V, true }
    return 0, false
}
```

Contrast with goja's approach, which uses Go reflection to automatically convert between Go structs and JavaScript objects. Automatic conversion is convenient but violates the principle that the host boundary should be an explicit, auditable surface.

### 3.4 Recommendation

Use **typed wrappers (Option 3)** with a **sealed `Value` interface**. The wrapper types (`IntVal`, `HandleVal`, etc.) are concrete structs with exported value fields (for primitive types) and unexported fields (for handles). The sealed interface prevents forgery. Conversion functions are explicit and narrow.

This mirrors the pattern used by Starlark-go's `Value` interface with concrete types like `Int`, `String`, `*List`, but adapted for the specific needs of a capability-based language where handle opacity is a security requirement.

---

## 4. Error Handling Across the Boundary

### 4.1 The Tension

Go uses `(value, error)` returns. The language uses `Computation pre post a` with pure sequencing. Host functions are Go functions that implement language-level computations. The question is how errors in Go functions surface in the language.

### 4.2 Option A: Host Functions Return `(Value, error)` with Conversion

```go
func dbOpenImpl(env *CapEnv) (*CapEnv, Value, error) {
    db := env.GetInternal("db").(*sqlDB)
    if err := db.conn.Ping(); err != nil {
        return nil, nil, fmt.Errorf("dbOpen: connection failed: %w", err)
    }
    // ...
}
```

The runtime catches the error and converts it to a language-level evaluation error, halting computation.

**How Starlark-go does this:** Host functions return `(Value, error)`. A non-nil error becomes an `EvalError` with a backtrace. The script halts. This is the simplest model and the one most consistent with a deterministic language: an error is an abort, not a recoverable condition.

**How goja does this:** JavaScript exceptions can be caught with try/catch. Go host functions can `panic(vm.NewTypeError(...))`, which becomes a catchable JavaScript exception. This is more flexible but breaks determinism if catch blocks have uncontrolled behavior.

**Fit for Gomputation:** This is the right default. If a host operation fails, the computation aborts with a diagnostic. The type system's guarantees remain sound because the post-condition was never reached. The result is either `(Env post, a)` or an error -- never a partial transition.

### 4.3 Option B: Errors as Capability State Transitions

```go
// Type: Computation { db : DB[Closed] | r }
//                   { db : DB[Opened] | db : DB[Error] | r }
//                   Unit
```

Here the host function always succeeds at the computation level, but the capability may transition to an error state that subsequent operations must handle.

**Advantages:**
- Error handling is tracked in the type system
- No implicit abort
- The program must acknowledge the error before proceeding

**Disadvantages:**
- Type complexity: every fallible operation needs a sum-typed post-condition
- Without algebraic data types in the row itself, expressing `DB[Opened] | DB[Error]` requires either a sum type in the state index or protocol branching
- Increases the burden on both host implementors and language users

**Assessment:** This is theoretically elegant but premature for v0.2. It requires sum-typed indices or a branching computation form that the spec does not yet include. Better to start with Option A and add protocol branching as a future extension.

### 4.4 Option C: Dedicated Error Capability

```go
// A dedicated "error" capability
// error : Error[None]  ->  error : Error[Some(msg)]
```

This treats errors as a built-in capability rather than as Go-level errors.

**Assessment:** This conflates error reporting with capability state. An error is not a resource. It is better modeled as a computation-level outcome.

### 4.5 Option D: Panic Recovery

Host functions that panic are caught by the runtime via `recover()`, and the panic value is converted to an evaluation error.

**How goja does this:** goja explicitly uses panics for JavaScript exceptions. `vm.Try(func() { ... })` wraps panic recovery. This works because JavaScript has throw/catch semantics.

**Fit for Gomputation:** Panics should be caught as a safety net but not used as a control flow mechanism. A host function that panics indicates a bug in the host implementation, not a normal error condition. The runtime should recover, log, and produce a clear error message, but panic-based error handling should not be the documented pattern.

### 4.6 Determinism Under Host Failure

When a host function fails, the language runtime must guarantee:

1. **No partial state transition.** The capability environment either fully transitions to the post-state or remains at the pre-state. This is the transactional invariant.
2. **No side effects on failure.** If the host function returns an error, the runtime must not have already committed observable side effects. This is a host contract, not something the runtime can enforce in general.
3. **Deterministic error identity.** Given the same inputs and the same host behavior, the same error should be produced. The error message itself need not be identical (it may contain timestamps from the underlying system), but the fact that an error occurred and at which point must be deterministic.

### 4.7 Recommendation

Use **Option A** as the primary pattern: host functions return `(Value, error)`, and a non-nil error aborts the computation with a diagnostic. Add **Option D** (panic recovery) as a safety net. Document the host contract: host functions must not perform irreversible side effects before returning an error.

Reserve Option B (state-transition errors) as a future extension when the language gains sum types in indices or branching computation forms.

---

## 5. Capability Environment Runtime Representation

### 5.1 The Static Picture

The type system describes:

```
{ db : DB[Opened], log : Logger[Ready] }
```

This is a row -- a finite labeled map from capability names to capability types (which include protocol states).

### 5.2 The Runtime Picture

At runtime, the evaluator must carry a structure that:

1. Maps capability labels to host-managed handles
2. Tracks the current protocol state of each handle
3. Enforces that operations receive handles in the correct state
4. Transitions states atomically when operations succeed

### 5.3 Option A: `map[string]Capability`

```go
type CapEnv struct {
    caps map[string]*CapEntry
}

type CapEntry struct {
    Label    string
    State    TypeIndex    // e.g., DBOpened, DBClosed
    Internal any          // host-managed value
}
```

**Advantages:**
- Simple, flexible
- Easy to extend with new capabilities
- O(1) lookup by label

**Disadvantages:**
- Go maps have non-deterministic iteration order (must use sorted keys for deterministic behavior)
- No compile-time structure; all label names are strings

**Mitigations:**
- Never iterate over the map in evaluation-order-sensitive code
- Use a sorted slice representation internally for any operation that requires deterministic ordering (e.g., row serialization for debugging)

### 5.4 Option B: Sorted Slice

```go
type CapEnv struct {
    entries []CapEntry  // sorted by label
}

func (e *CapEnv) Get(label string) *CapEntry {
    i := sort.Search(len(e.entries), func(i int) bool {
        return e.entries[i].Label >= label
    })
    if i < len(e.entries) && e.entries[i].Label == label {
        return &e.entries[i]
    }
    return nil
}
```

**Advantages:**
- Deterministic iteration order
- Canonical representation simplifies equality checks
- Mirrors the row representation in the type checker (sorted labels)

**Disadvantages:**
- O(log n) lookup, O(n) insertion
- For typical capability environments (5-20 entries), this is negligible

### 5.5 Option C: Struct-Based (One Type per Environment Shape)

```go
type DBLogEnv struct {
    DB  *CapEntry
    Log *CapEntry
}
```

**Assessment:** Impractical. The number of distinct environment shapes is unbounded due to row polymorphism. This would require code generation for every instantiation. Rejected.

### 5.6 Enforcing State Transitions at Runtime

Even though the type checker validates transitions statically, the runtime should enforce them as a defense-in-depth measure:

```go
func (e *CapEnv) Transition(label string, fromState, toState TypeIndex) error {
    entry := e.Get(label)
    if entry == nil {
        return fmt.Errorf("capability %q not found", label)
    }
    if entry.State != fromState {
        return fmt.Errorf("capability %q in state %v, expected %v", label, entry.State, fromState)
    }
    entry.State = toState
    return nil
}
```

This is a redundant check -- the type checker should have already verified this -- but it catches bugs in host function implementations and in the type checker itself.

### 5.7 Preventing Runtime Violations

The runtime must prevent operations that the type checker rejected:

1. **Host functions receive the capability environment through the runtime, not by constructing it.** A host function should not create a `CapEnv` from scratch.
2. **The evaluator validates state before calling host functions.** Even though the type checker says the state is correct, the runtime confirms.
3. **State transitions are performed by the runtime, not by host functions directly.** The host function returns a result and the runtime performs the state transition. This prevents host functions from putting the environment into an inconsistent state.

Alternative design: host functions receive a `CapAccessor` interface that provides only the operations they are typed to perform:

```go
type CapAccessor interface {
    // GetInternal returns the host value for the given capability.
    // Panics if the capability is not in the expected state.
    GetInternal(label string) any

    // Transition moves a capability to a new state.
    // Panics if the capability is not in the expected pre-state.
    Transition(label string, toState TypeIndex) any
}
```

This is more restrictive and harder for host functions to misuse.

### 5.8 Recommendation

Use **Option B (sorted slice)** for the capability environment. It provides deterministic behavior, canonical form, and is efficient enough for the expected environment sizes. Use runtime state checking as defense-in-depth. Expose a `CapAccessor` interface to host functions rather than the raw `CapEnv`.

---

## 6. Determinism and Host Interaction

### 6.1 How Starlark Achieves Determinism

Starlark's design principles are instructive:

1. **No randomness.** The core language has no `random()` function. If randomness is needed, it must be provided as a host-injected capability.
2. **No time.** No `time.now()` or `Date` in the core language. Time, if needed, is a host-provided value.
3. **No I/O.** No file system access, no network access, no environment variable reading. All I/O is mediated by host-provided functions.
4. **No goroutine leaking.** Starlark evaluation is single-threaded. The `Thread` type is not safe for concurrent use. Each `Thread` has its own execution context.
5. **Deterministic dict iteration.** Unlike Python 3.6+ (insertion order) or earlier Python (arbitrary), Starlark specifies a deterministic iteration order.
6. **Step counting.** `Thread.SetMaxExecutionSteps(n)` provides a deterministic resource bound. Steps are counted in the interpreter loop, not by wall-clock time.

### 6.2 The Host Contract

For Gomputation, the host must document and guarantee:

1. **Functional purity contract.** A host function that declares a state-preserving type (pre == post) must have no observable side effects beyond its return value. This is not enforceable by the runtime, but it is a documented contract.

2. **Idempotent failure contract.** If a host function fails (returns error), it must not have performed irreversible side effects. If this cannot be guaranteed (e.g., for a real database write), the host documentation must state that the operation has "at-most-once" semantics.

3. **Determinism classification.** Each host function should be classified as:
   - **Pure:** same inputs always produce the same output (e.g., string formatting)
   - **Deterministic given capability state:** same inputs and same capability state produce the same output (e.g., reading from a deterministic data source)
   - **Non-deterministic:** output depends on external state (e.g., reading the current time, querying a live database)

   The type system does not distinguish these classifications, but the host documentation should.

4. **No ambient authority.** Host functions must not access global state, environment variables, or the file system unless those resources are explicitly threaded through the capability environment. A host function that reads `os.Getenv()` inside its implementation violates this contract even if its type signature looks correct.

### 6.3 Handling Go's Inherent Non-Determinism

Go has several sources of non-determinism that the runtime must manage:

| Source | Risk | Mitigation |
|---|---|---|
| Map iteration order | Leaks into language-visible behavior if maps are used for rows | Use sorted slice for CapEnv; never iterate Go maps in evaluation-order-sensitive code |
| Goroutine scheduling | Relevant if host functions spawn goroutines | Document that host functions must not spawn goroutines visible to the evaluator |
| `select` statement | Non-deterministic channel selection | Not applicable if evaluation is single-threaded |
| Garbage collection | Finalizer ordering | Do not use finalizers for capability state transitions |
| Floating point | x87 vs SSE, etc. | Use IEEE 754 semantics; document any platform-specific behavior |

### 6.4 What "Deterministic" Means with I/O Capabilities

The language is deterministic in the following sense: **given the same program, the same initial capability environment, and the same host function behavior, the same result is produced.** The host functions themselves may interact with the real world (databases, networks, files), making the overall system non-deterministic, but the language's evaluation semantics are deterministic.

This is analogous to Starlark's position: the core language is deterministic, but `print()` has a side effect. The determinism guarantee applies to the language semantics, not to the host's I/O behavior.

For testing and replay, a host can provide deterministic mock implementations of capabilities. The language runtime does not need to change; only the host functions do.

### 6.5 Recommendation

Follow Starlark's model: no ambient authority in the language, all non-determinism mediated by explicit capabilities, single-threaded evaluation, step-based resource limits. Document the host contract clearly and classify each host function's determinism level.

---

## 7. API Design Patterns

### 7.1 Parse -> Check -> Run Pipeline

CEL-go's pipeline is the right model:

```go
// Phase 1: Parse (source -> untyped AST)
ast, issues := env.Parse(source)
if issues.Err() != nil {
    return issues.Err()
}

// Phase 2: Check (untyped AST -> typed AST with row checking)
checked, issues := env.Check(ast)
if issues.Err() != nil {
    return issues.Err()
}

// Phase 3: Program (typed AST -> executable)
program, err := env.Program(checked)
if err != nil {
    return err
}

// Phase 4: Run (executable + capability environment -> result)
result, err := program.Eval(capEnv)
```

For Gomputation, the phases are:

```go
// 1. Create environment with host primitive types
env := gomputation.NewEnv(
    gomputation.Primitive("dbOpen",
        "forall r. Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit",
        dbOpenImpl,
    ),
    gomputation.Primitive("dbClose",
        "forall r. Computation { db : DB[Opened] | r } { db : DB[Closed] | r } Unit",
        dbCloseImpl,
    ),
    gomputation.Primitive("dbQuery",
        "forall r. Query -> Computation { db : DB[Opened] | r } { db : DB[Opened] | r } Rows",
        dbQueryImpl,
    ),
    gomputation.DataType("Query", /* constructors */),
    gomputation.DataType("Rows",  /* constructors */),
    gomputation.StateType("DB", "Closed", "Opened"),
)

// 2. Parse source
ast, err := env.Parse(source)

// 3. Type check (kinding, row unification, indexed computation checking)
checked, err := env.Check(ast)

// 4. Create evaluable program
program, err := env.Program(checked)

// 5. Set up runtime capability environment
caps := gomputation.NewCapEnv(
    gomputation.Cap("db", gomputation.DBClosed, myDBConn),
)

// 6. Evaluate
result, finalCaps, err := program.Eval(caps)
```

### 7.2 Module and Compilation Caching

Following CEL-go's model, `Program` objects should be:

- **Stateless:** no mutable state after creation
- **Thread-safe:** safe for concurrent `Eval` calls from multiple goroutines
- **Cacheable:** can be stored and reused across multiple evaluations

The key insight from CEL-go: parsing and type checking are expensive, evaluation is cheap. Cache at the `Program` level.

```go
type ProgramCache struct {
    mu       sync.RWMutex
    programs map[string]*Program  // keyed by source hash
}

func (c *ProgramCache) GetOrCompile(env *Env, source string) (*Program, error) {
    hash := sha256sum(source)
    c.mu.RLock()
    if p, ok := c.programs[hash]; ok {
        c.mu.RUnlock()
        return p, nil
    }
    c.mu.RUnlock()

    ast, err := env.Parse(source)
    if err != nil { return nil, err }

    checked, err := env.Check(ast)
    if err != nil { return nil, err }

    program, err := env.Program(checked)
    if err != nil { return nil, err }

    c.mu.Lock()
    c.programs[hash] = program
    c.mu.Unlock()
    return program, nil
}
```

### 7.3 Incremental Type Checking

CEL-go supports `env.Extend(opts...)` to create extended environments without re-checking the base. Gomputation should support a similar pattern:

```go
baseEnv := gomputation.NewEnv(/* core capabilities */)
extendedEnv := baseEnv.Extend(
    gomputation.Primitive("logWrite", /* ... */),
)
```

Extended environments share the base environment's type information and add new declarations. This enables modular host configuration without redundant type checking.

### 7.4 Thread Safety of the Evaluator

The evaluator should be single-threaded per evaluation, but `Program` objects should be shareable:

```go
// Safe: concurrent evaluation of the same program with different capability environments
go func() { program.Eval(caps1) }()
go func() { program.Eval(caps2) }()

// Unsafe: concurrent mutation of a capability environment
// (CapEnv is owned by a single evaluation and must not be shared)
```

This matches the model used by Starlark-go (`Thread` is single-use) and goja (`Runtime` is single-goroutine).

### 7.5 Context and Cancellation

Support `context.Context` for cancellation and deadlines:

```go
result, finalCaps, err := program.EvalContext(ctx, caps)
```

Internally, the evaluator checks `ctx.Done()` at regular intervals (e.g., every N reduction steps). This is cheaper than checking on every step and sufficient for practical cancellation latency.

Additionally, support step counting following Starlark-go's model:

```go
opts := gomputation.EvalOptions{
    MaxSteps:   100000,
    OnMaxSteps: func() error { return fmt.Errorf("step limit exceeded") },
}
result, finalCaps, err := program.Eval(caps, opts)
```

### 7.6 Resource Limits

| Limit | Mechanism | Precedent |
|---|---|---|
| Computation steps | Counter in the evaluator loop | Starlark `SetMaxExecutionSteps` |
| Memory allocation | Object allocation counter | Tengo `SetMaxAllocs` |
| Stack depth | Frame counter | goja `SetMaxCallStackSize`, Tengo `MaxFrames` |
| Wall-clock time | `context.Context` with timeout | Tengo `RunContext` |
| Value size | Limits on string/bytes length | Tengo `MaxStringLen`, `MaxBytesLen` |

Gomputation should support at minimum: step limits, stack depth limits, and context-based cancellation. Memory limits can be deferred to a later version.

---

## 8. Security Considerations

### 8.1 Preventing Escape from the Capability Sandbox

The capability sandbox depends on three invariants:

1. **No ambient authority.** The language provides no built-in functions that access the outside world.
2. **No capability forgery.** Language-level code cannot create a `HandleVal` or fake a capability state.
3. **No capability leaking.** Host functions do not expose more authority than their type declares.

Each invariant must be maintained across the host boundary.

### 8.2 Preventing Go Reflection from Bypassing Type Safety

Go's `reflect` package can:
- Inspect unexported struct fields
- Set unexported fields via `reflect.Value.Set()` (if the value is addressable)
- Call unexported methods
- Bypass interface type assertions

However, these capabilities are only available to Go code, not to the embedded language. The risk is that a host function implementation uses reflection to bypass the language's type safety.

**Mitigations:**
1. Document that host function implementations must not use `reflect` to access `HandleVal` internals.
2. Use the sealed interface pattern (unexported method) so that external packages cannot create fake values.
3. Store capability handles in a side table indexed by opaque tokens rather than directly in `HandleVal`:

```go
type HandleVal struct {
    token handleToken  // opaque, unexported type
}

type handleToken struct{ id uint64 }

// The runtime maintains:
type handleTable struct {
    mu      sync.Mutex
    entries map[handleToken]any
    nextID  uint64
}
```

This way, even if a `HandleVal` is inspected via reflection, the `token` field reveals nothing. The actual host value is stored in a side table that is only accessible through the runtime.

### 8.3 Preventing `unsafe` Package Leaking Through Capabilities

The `unsafe` package is a Go-level concern, not a language-level concern. Host function implementations that import `unsafe` can bypass any Go-level protection.

**Mitigations:**
1. Code review discipline: host function implementations should not import `unsafe`.
2. The language runtime itself should not import `unsafe`.
3. Static analysis tooling (e.g., `go vet`, custom linters) can flag `unsafe` imports in host function packages.

### 8.4 Comparison with Other Sandboxed Execution Models

| Model | Isolation mechanism | Capability model | Gomputation relevance |
|---|---|---|---|
| **WASM/WASI** | Memory isolation, deny-by-default imports | Host injects capabilities as imports | Very relevant: same deny-by-default principle. Gomputation's rows are the static counterpart of WASI's capability grants. |
| **gVisor** | Kernel syscall interception | Restrict syscalls to a subset | Less relevant: OS-level isolation, not language-level. |
| **Starlark** | No I/O in language, host builtins are the only authority | Implicit: host decides what to expose | Directly relevant: same model, but without static types. |
| **CEL** | Non-Turing complete, no side effects | Host provides variables and functions | Relevant for the expression-level, but CEL has no state transitions. |
| **Lua/LuaJIT** | Sandboxed environments, table-based | Less formal: globals can be restricted | Somewhat relevant for the embedding pattern, less for security. |

The closest model to Gomputation is **WASM/WASI's capability-based approach** combined with **Starlark's language-level determinism**. The key difference is that Gomputation adds static types and state indices, which provide compile-time assurance that WASM can only provide at runtime.

### 8.5 The Trust Boundary

The trust boundary in Gomputation is:

```
Untrusted: language-level code (user programs)
Trusted:   host functions, runtime, type checker
```

The type checker ensures that untrusted code cannot:
- Use a capability it does not have (row checking)
- Use a capability in the wrong state (state index checking)
- Forge a capability (sealed types, no constructors)
- Bypass sequencing (indexed computation checking)

Host functions are trusted. A buggy host function can violate any invariant. This is the same trust model as WASM: the host is trusted, the guest is sandboxed.

---

## 9. Specific Recommendations for Gomputation

### 9.1 Registration API Design

```go
// Primary API: string-based type with parsed validation at registration time
env := gomputation.NewEnv(
    gomputation.Primitive("dbOpen",
        "forall r. Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit",
        func(acc CapAccessor) (Value, error) {
            db := acc.GetInternal("db").(*sql.DB)
            if err := db.Ping(); err != nil {
                return nil, err
            }
            acc.Transition("db", DBOpened)
            return UnitVal{}, nil
        },
    ),
)
```

The type string is parsed and validated when `NewEnv` is called. If it is malformed, `NewEnv` returns an error (or panics, following Go convention for initialization-time failures).

### 9.2 Value Types

```go
// Sealed interface
type Value interface {
    valueType() Type
    valueSealed()       // unexported: prevents external implementation
}

// Concrete value types
type IntVal    struct{ V int64 }
type FloatVal  struct{ V float64 }
type StringVal struct{ V string }
type BoolVal   struct{ V bool }
type UnitVal   struct{}
type FuncVal   struct{ /* closure representation */ }
type ConsVal   struct{ Tag string; Fields []Value }  // ADT constructor application
type CompVal   struct{ /* suspended computation */ }
type HandleVal struct{ token handleToken }            // opaque capability handle
```

### 9.3 Error Strategy

```go
// Host functions return (Value, error)
// Non-nil error aborts the computation
type PrimitiveFunc func(acc CapAccessor, args ...Value) (Value, error)

// The runtime catches panics as a safety net
func (rt *Runtime) callPrimitive(f PrimitiveFunc, acc CapAccessor, args ...Value) (val Value, err error) {
    defer func() {
        if r := recover(); r != nil {
            err = fmt.Errorf("host function panicked: %v", r)
        }
    }()
    return f(acc, args...)
}
```

### 9.4 Capability Environment

```go
type CapEnv struct {
    entries []capEntry  // sorted by label, canonical form
}

type capEntry struct {
    label    string
    stateIdx TypeIndex
    handle   handleToken
}

// Separate handle storage
type handleStore struct {
    values map[handleToken]any
    nextID uint64
}
```

### 9.5 Pipeline Implementation Sketch

```go
// Env holds static type information and host primitive registrations
type Env struct {
    primitives map[string]primRegistration
    types      map[string]typeDecl
    // type checker state
}

// AST is the untyped parse tree
type AST struct { /* ... */ }

// CheckedAST is the typed AST after kinding and row checking
type CheckedAST struct { /* ... */ }

// Program is the evaluable form, stateless and thread-safe
type Program struct {
    checkedAST *CheckedAST
    primImpls  map[string]PrimitiveFunc
}

// Result of evaluation
type EvalResult struct {
    Value    Value
    FinalEnv *CapEnv
    Steps    uint64
}
```

### 9.6 Phased Implementation Plan

| Phase | Focus | Deliverable |
|---|---|---|
| 1 | Parser + AST | `env.Parse(source) -> AST` |
| 2 | Kind checker + row well-formedness | Validates `Type` and `Row` formation |
| 3 | Type checker + row unification | `env.Check(ast) -> CheckedAST` with row-indexed computation checking |
| 4 | Evaluator + capability environment | `program.Eval(caps) -> EvalResult` with step counting |
| 5 | Host primitive registration | String-based type registration with validation |
| 6 | Resource limits + cancellation | Step limits, context support |
| 7 | Program caching + thread safety | Stateless `Program`, concurrent `Eval` |

Phases 1-3 are where the type-theoretic novelty lives. Phases 4-7 follow established patterns from Starlark-go and CEL-go.

---

## 10. Key References

### Implementations
1. Starlark-go. https://pkg.go.dev/go.starlark.net/starlark
2. Starlark specification. https://github.com/google/starlark-go/blob/master/doc/spec.md
3. Starlark design document. https://github.com/bazelbuild/starlark/blob/master/design.md
4. CEL-go. https://pkg.go.dev/github.com/google/cel-go/cel
5. CEL-go codelab. https://codelabs.developers.google.com/codelabs/cel-go
6. CEL-go thread safety discussion. https://github.com/google/cel-go/discussions/878
7. Tengo. https://pkg.go.dev/github.com/d5/tengo/v2
8. Tengo source. https://github.com/d5/tengo
9. goja. https://pkg.go.dev/github.com/dop251/goja
10. goja source. https://github.com/dop251/goja

### Security and Capability Models
11. Wasmtime security model. https://docs.wasmtime.dev/security.html
12. Go capability-based security proposal. https://github.com/golang/go/issues/23267
13. Go unsafe package and type safety. https://arxiv.org/pdf/2006.09973

### Type Theory
14. Robert Atkey, "Parameterised Notions of Computation", 2006.
15. Dorchard/effect-monad (graded and parameterised monads in Haskell). https://github.com/dorchard/effect-monad
16. Koka: Programming with Row-polymorphic Effect Types. https://arxiv.org/pdf/1406.2061

## Relevance to Gomputation

This research establishes the practical foundation for implementing the host boundary. The key findings are:

1. **CEL-go's parse-check-program pipeline** is the right architectural model. It separates concerns cleanly and enables program caching.

2. **Starlark-go's determinism model** is the right security and determinism model. No ambient authority, explicit capabilities, step-based resource limits.

3. **String-based type registration** is the most pragmatic API for host primitives, provided the type string is parsed and validated at registration time.

4. **Sealed interfaces with opaque handles** prevent capability forgery without requiring linear types or other heavyweight machinery.

5. **Error-as-abort** is the right default error model. State-transition errors can be added later when the type system supports sum-typed indices.

6. **Sorted slices** for capability environments give deterministic behavior and canonical form, matching the row representation in the type checker.

7. **The trust boundary is between host and guest.** The type checker protects against untrusted guest code. Host functions are trusted and must conform to a documented contract.
