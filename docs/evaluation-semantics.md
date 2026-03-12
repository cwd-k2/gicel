# Evaluation Semantics for a CBPV-like Indexed Effect Language

One-line description: complete formal operational semantics for Gomputation's value/computation split, covering evaluation strategy, big-step and small-step rules, environment-based evaluation, primitive operations, do-notation desugaring, metatheoretic properties, and Go-oriented implementation guidance.

## Table of Contents

1. Evaluation Strategy Choice
2. Syntax of the Core Calculus
3. Big-Step (Natural) Semantics
4. Small-Step (Structural Operational) Semantics
5. Environment-Based Evaluation for Go Implementation
6. Primitive Operation Evaluation
7. Do-Notation Desugaring and Elaboration
8. Metatheoretic Properties
9. Evaluation Traces: Worked Examples
10. Comparison with Related Systems
11. Recommendations for Gomputation
12. Key References

---

## 1. Evaluation Strategy Choice

### 1.1 The Three Classical Strategies

The evaluation strategy determines when arguments are reduced and when substitutions occur.

**Call-by-value (CBV).** Arguments are evaluated to values before being passed to functions. A function application `f e` first reduces `e` to a value `v`, then reduces `f v` by substitution. This is the strategy of ML, OCaml, Scheme, and most imperative languages.

**Call-by-name (CBN).** Arguments are substituted into the function body unevaluated. A function application `f e` immediately substitutes `e` for the parameter in the body of `f`. If the parameter is used multiple times, `e` is re-evaluated each time. This is the strategy of the pure lambda calculus and (classically) Algol 60 call-by-name parameters.

**Call-by-need (lazy).** Like call-by-name, but each argument is evaluated at most once. The result is memoized. This is the strategy of Haskell.

### 1.2 CBPV's Resolution

Levy's CBPV does not choose between CBV and CBN. It subsumes both by making the evaluation discipline syntactically explicit through the value/computation split. In CBPV:

- Values are always fully evaluated (they *are*, they do not *do*).
- Computations are processes that may perform effects.
- The `thunk`/`force` pair mediates between the two layers.

CBV embeds into CBPV by interpreting a CBV function type `A =>_v B` as `U(A -> F B)`: a thunked computation that takes a value argument and produces a result. CBN embeds by interpreting `A =>_n B` as `U A -> B`: a computation that receives a thunked argument.

CBPV is therefore not a third strategy but a substrate that explains when each strategy applies.

### 1.3 Why CBV Is the Right Default for Gomputation

Gomputation should adopt strict, call-by-value evaluation for the value layer. The reasons are structural:

**1. The host language is strict.** Go evaluates arguments before function calls. An embedded language that uses lazy evaluation would create a semantic mismatch at the host boundary. When a host function receives an argument, it expects a fully evaluated value, not a thunk that may trigger arbitrary computation when inspected.

**2. Determinism requires predictable evaluation order.** Lazy evaluation introduces sharing and memoization, which make it harder to predict when and whether a subexpression is evaluated. For a deterministic embedded language, strict left-to-right evaluation provides the simplest and most predictable semantics.

**3. Resource accounting is straightforward.** Step counting, fuel limits, and execution cost are easy to define for strict evaluation: each evaluation step is observable and countable. Under lazy evaluation, the cost of forcing a thunk depends on whether it has already been forced elsewhere, creating non-local cost dependencies.

**4. Error reporting is precise.** Under strict evaluation, an error occurs at the point where the failing expression is textually located. Under lazy evaluation, an error occurs at the point where the result is first demanded, which may be far from the source of the error.

**5. The value/computation split already provides the necessary control.** Gomputation distinguishes values (pure, strict) from computations (effectful, sequenced). Laziness in Haskell serves partly to defer effects; in Gomputation, effects are already deferred by the `Computation` type. There is no need for laziness to serve this role.

**6. Compatibility with CBPV's operational semantics.** Levy's big-step semantics for CBPV evaluates the value layer strictly. Values are in normal form; only computations reduce. This is exactly the evaluation discipline Gomputation should adopt.

### 1.4 How the Strategy Affects the Value/Computation Boundary

Under strict evaluation:

- Every value-layer expression is fully evaluated before it can participate in a computation.
- `pure v` receives a fully evaluated value `v`.
- `bind c (\x -> c')` evaluates `c` to a result value `v`, then substitutes `v` for `x` in `c'`.
- A function `\x -> e` is a value (a closure). The body `e` is not evaluated until the function is applied.
- Application `f u` evaluates `f` to a closure and `u` to a value, then substitutes.

Under lazy evaluation (for contrast):

- `pure e` would receive an unevaluated thunk.
- `bind c (\x -> c')` would pass a thunk for the result of `c` into the body of the continuation.
- Forcing could trigger effects at unpredictable points.

The strict discipline keeps the boundary clean: values are data at rest; computations are processes that run to completion.

### 1.5 Strict vs Lazy: Summary

| Property | Strict (CBV) | Lazy (CBN/need) |
|---|---|---|
| Arguments | Evaluated before passing | Evaluated on demand |
| Evaluation order | Predictable, left-to-right | Demand-driven, non-local |
| Resource accounting | Direct step counting | Non-trivial (sharing) |
| Error localization | At source | At demand site |
| Host boundary | Natural (Go is strict) | Impedance mismatch |
| Effect sequencing | Explicit via `bind` | Also explicit, but thunks can hide effects |
| Memory model | Stack-like, GC for closures | Graph reduction, heap thunks |
| Determinism | Straightforward | Requires care with sharing |

Gomputation adopts strict evaluation for the value layer and explicit sequencing (via `bind`) for the computation layer.

---

## 2. Syntax of the Core Calculus

The following grammar defines the core calculus after elaboration. Surface syntax (including do-notation) is desugared to this core before evaluation.

### 2.1 Value Expressions

```text
v ::= x                        -- variable
    | n                         -- integer literal
    | s                         -- string literal
    | ()                        -- unit
    | \x -> e                   -- lambda abstraction
    | v1 v2                     -- application
    | C v1 ... vn               -- constructor application
    | case v of { pi -> ei }    -- case analysis
    | (v : T)                   -- type annotation (erased at runtime)
```

### 2.2 Computation Expressions

```text
c ::= pure v                   -- lift a value into a computation
    | bind c (\x -> c')         -- sequencing
    | prim(op, v1, ..., vn)     -- primitive operation invocation
```

### 2.3 Values (Runtime)

```text
w ::= n                        -- integer
    | s                         -- string
    | ()                        -- unit
    | <\x -> e, rho>            -- closure: lambda body + captured environment
    | C w1 ... wn               -- constructed data
```

### 2.4 Computation Results (Runtime)

```text
cr ::= (sigma', w)              -- completed computation: new cap env + result value
```

### 2.5 Environments

```text
rho   ::= []                    -- empty variable environment
         | rho[x -> w]          -- variable binding extension

sigma ::= []                    -- empty capability environment
         | sigma[l -> h]        -- capability binding: label to handle
```

Where `h` is a host-managed capability handle carrying an opaque internal value and a protocol state tag.

---

## 3. Big-Step (Natural) Semantics

Big-step semantics defines the relation "expression `e` in environment `rho` evaluates to value `w`." For computations, it also threads the capability environment.

### 3.1 Value Evaluation Judgment

```text
rho |- v ==> w
```

Read: "Under variable environment rho, value expression v evaluates to runtime value w."

### 3.2 Computation Evaluation Judgment

```text
rho; sigma |- c ==> sigma'; w
```

Read: "Under variable environment rho and capability environment sigma, computation c evaluates to result value w and produces new capability environment sigma'."

### 3.3 Value Evaluation Rules

**V-Lit (Integer):**

```text
  ─────────────────  [V-Lit-Int]
  rho |- n ==> n
```

**V-Lit (String):**

```text
  ─────────────────  [V-Lit-Str]
  rho |- s ==> s
```

**V-Unit:**

```text
  ─────────────────  [V-Unit]
  rho |- () ==> ()
```

**V-Var:**

```text
  rho(x) = w
  ─────────────────  [V-Var]
  rho |- x ==> w
```

**V-Lam:**

```text
  ──────────────────────────────────  [V-Lam]
  rho |- \x -> e ==> <\x -> e, rho>
```

A lambda evaluates to a closure that captures the current environment. No evaluation of the body occurs.

**V-App:**

```text
  rho |- v1 ==> <\x -> e, rho'>
  rho |- v2 ==> w2
  rho'[x -> w2] |- e ==> w
  ──────────────────────────────  [V-App]
  rho |- v1 v2 ==> w
```

Application evaluates the function to a closure, the argument to a value, then evaluates the body in the closure's environment extended with the argument binding.

**V-Ctor:**

```text
  rho |- v1 ==> w1   ...   rho |- vn ==> wn
  ─────────────────────────────────────────────  [V-Ctor]
  rho |- C v1 ... vn ==> C w1 ... wn
```

Constructor arguments are evaluated left to right.

**V-Case:**

```text
  rho |- v ==> C_i w1 ... wk
  rho[y1 -> w1, ..., yk -> wk] |- ei ==> w
  ───────────────────────────────────────────────────────────  [V-Case]
  rho |- case v of { ... | C_i y1 ... yk -> ei | ... } ==> w
```

The scrutinee is evaluated, matched against the patterns, and the matching branch is evaluated in the environment extended with the pattern bindings.

### 3.4 Computation Evaluation Rules

**C-Pure:**

```text
  rho |- v ==> w
  ──────────────────────────────────  [C-Pure]
  rho; sigma |- pure v ==> sigma; w
```

`pure` evaluates its argument and returns the value with the capability environment unchanged.

**C-Bind:**

```text
  rho; sigma |- c ==> sigma'; w
  rho[x -> w]; sigma' |- c' ==> sigma''; w'
  ─────────────────────────────────────────────────────  [C-Bind]
  rho; sigma |- bind c (\x -> c') ==> sigma''; w'
```

`bind` evaluates the first computation `c` under the current capability environment `sigma`, obtaining an intermediate value `w` and updated capability environment `sigma'`. It then evaluates the continuation `c'` with `x` bound to `w` and the capability environment set to `sigma'`.

This is the central rule. It makes explicit:

1. The left-to-right evaluation order of the two computations.
2. The threading of the capability environment through the computation chain.
3. The binding of the intermediate result into the continuation.

**C-PrimOp:**

```text
  rho |- v1 ==> w1   ...   rho |- vn ==> wn
  host(op, sigma, w1, ..., wn) = Ok(sigma', w)
  ───────────────────────────────────────────────────────  [C-PrimOp]
  rho; sigma |- prim(op, v1, ..., vn) ==> sigma'; w
```

Arguments are evaluated left to right. The host-provided implementation `host(op, ...)` is invoked with the current capability environment and the evaluated arguments. The host returns an updated capability environment and a result value.

**C-PrimOp-Err:**

```text
  rho |- v1 ==> w1   ...   rho |- vn ==> wn
  host(op, sigma, w1, ..., wn) = Err(msg)
  ───────────────────────────────────────────────────────────  [C-PrimOp-Err]
  rho; sigma |- prim(op, v1, ..., vn) ==> error(msg)
```

If the host operation fails, evaluation halts with an error. The capability environment does not transition.

### 3.5 Well-Formedness Conditions

The big-step rules assume:

1. The term is well-typed according to the static type system.
2. The capability environment `sigma` matches the `pre` row of the computation type.
3. Host operations are registered with the correct type signatures.
4. Patterns in `case` are exhaustive (verified statically).

### 3.6 The Run Function

The big-step semantics can be stated as a total function (for well-typed, non-failing programs):

```text
run : Env rho -> CapEnv sigma -> Computation pre post a -> Result
where Result = Ok (CapEnv sigma', Value w) | Err string

run rho sigma (pure v) =
    let w = eval rho v
    in Ok (sigma, w)

run rho sigma (bind c k) =
    match run rho sigma c with
    | Err msg -> Err msg
    | Ok (sigma', w) ->
        let <\x -> c', rho'> = eval rho k
        in run rho'[x -> w] sigma' c'

run rho sigma (prim op v1 ... vn) =
    let w1 = eval rho v1, ..., wn = eval rho vn
    in host(op, sigma, w1, ..., wn)
```

This is the operational reading the spec already provides in schematic form (Section 17.2). The formal rules above make it precise.

---

## 4. Small-Step (Structural Operational) Semantics

Small-step semantics defines a single-step reduction relation. It is useful for reasoning about intermediate states, divergence, and resource limits.

### 4.1 Reduction Configurations

A configuration is a triple:

```text
<rho; sigma; c>
```

where `rho` is the variable environment, `sigma` is the capability environment, and `c` is the computation expression being evaluated. A terminal configuration is:

```text
<rho; sigma; done(w)>
```

indicating that computation has completed with result `w` under capability environment `sigma`.

### 4.2 Value Reduction

Since the language is strict at the value layer, we also need a value reduction relation. However, in practice, value evaluation can be defined as a function (it is total for well-typed terms and always terminates in a finite-data language without general recursion). We write:

```text
eval(rho, v) = w
```

as a meta-level function. If general recursion were added, value reduction would need its own small-step relation; for the current core, the functional presentation suffices.

### 4.3 Computation Reduction Rules

**S-Pure:**

```text
  eval(rho, v) = w
  ─────────────────────────────────────────────  [S-Pure]
  <rho; sigma; pure v>  -->  <rho; sigma; done(w)>
```

**S-Bind-Step:**

```text
  <rho; sigma; c>  -->  <rho'; sigma'; c_next>
  ──────────────────────────────────────────────────────────  [S-Bind-Step]
  <rho; sigma; bind c (\x -> c')>  -->  <rho'; sigma'; bind c_next (\x -> c')>
```

If the first computation can take a step, the bind context takes a step with it. The continuation `\x -> c'` is held as part of the evaluation context.

**S-Bind-Done:**

```text
  ─────────────────────────────────────────────────────────────────  [S-Bind-Done]
  <rho; sigma; bind (done(w)) (\x -> c')>  -->  <rho[x -> w]; sigma; c'>
```

When the first computation is done, the result value is substituted into the continuation.

**S-PrimOp:**

```text
  eval(rho, v1) = w1   ...   eval(rho, vn) = wn
  host(op, sigma, w1, ..., wn) = Ok(sigma', w)
  ──────────────────────────────────────────────────────────────────  [S-PrimOp]
  <rho; sigma; prim(op, v1, ..., vn)>  -->  <rho; sigma'; done(w)>
```

### 4.4 Evaluation Contexts

Alternatively, the small-step semantics can be formulated using evaluation contexts (Felleisen-style):

```text
E ::= []
    | bind E (\x -> c)
```

The only evaluation context is the left position of `bind`. This reflects the fact that computations are evaluated sequentially: the first computation in a `bind` chain runs to completion before the continuation begins.

The context rule:

```text
  <rho; sigma; c>  -->  <rho'; sigma'; c'>
  ──────────────────────────────────────────────  [S-Context]
  <rho; sigma; E[c]>  -->  <rho'; sigma'; E[c']>
```

And the context-filling reduction for completed computations:

```text
  ───────────────────────────────────────────────────────────  [S-Bind-Fill]
  <rho; sigma; bind (done(w)) (\x -> c')>  -->  <rho[x -> w]; sigma; c'>
```

### 4.5 Big-Step vs Small-Step: Which Is More Appropriate?

Both formulations define the same evaluation relation for terminating programs. The choice affects the presentation and implementation:

**Big-step advantages:**

- More natural for an interpreter implementation (the rules directly translate to a recursive evaluator).
- Corresponds directly to the `run` function in the spec.
- Simpler for reasoning about total programs.
- Environment-based evaluation is natural (closures carry environments).

**Small-step advantages:**

- Can express divergence (non-termination) by an infinite reduction sequence.
- More natural for step counting and fuel-based resource limits.
- Required for reasoning about observable intermediate states.
- Better suited for proving progress and type safety theorems.

**Recommendation for Gomputation:** Use big-step semantics as the primary specification and implementation guide. Use small-step semantics as a secondary tool for reasoning about resource limits and for the metatheory (progress and preservation proofs).

The Go implementation should be a tree-walking interpreter structured as a recursive function, directly mirroring the big-step rules. Step counting can be layered on top by incrementing a counter at each recursive call to the evaluation function.

---

## 5. Environment-Based Evaluation for Go Implementation

### 5.1 Why Environment-Based, Not Substitution-Based

The big-step rules in Section 3 use environments and closures rather than syntactic substitution. This choice is standard for implementations and has concrete advantages:

**Substitution-based evaluation** replaces variables with their values in the syntax tree. This requires:

- Capture-avoiding substitution (alpha-renaming to avoid variable shadowing).
- Copying of syntax tree nodes.
- O(n) cost per substitution, where n is the size of the expression.

**Environment-based evaluation** stores variable bindings in a separate data structure (the environment). A closure is a pair of a lambda body and the environment in which it was created. This requires:

- Environment lookup instead of substitution.
- Closure construction at lambda evaluation.
- No modification of the syntax tree.

For Gomputation's Go implementation, environment-based evaluation is clearly preferable:

- The AST is immutable after parsing and type-checking.
- Environment lookup is O(1) with a hash map or O(log n) with a persistent map.
- No risk of capture-avoidance bugs.
- The AST can be shared across multiple evaluations.

### 5.2 Closure Representation

A closure pairs a function body with the environment that was active when the closure was created:

```go
type Closure struct {
    Param string     // parameter name
    Body  Expr       // body expression (AST node)
    Env   *VarEnv    // captured variable environment
}
```

When a closure is applied to an argument, the body is evaluated in the captured environment extended with the parameter binding:

```go
func applyClosure(clo *Closure, arg Value) (Value, error) {
    extEnv := clo.Env.Extend(clo.Param, arg)
    return eval(extEnv, clo.Body)
}
```

### 5.3 Variable Environment: Linked List vs Flat Map

Two standard representations for the variable environment:

**Linked list (persistent, functional):**

```go
type VarEnv struct {
    name   string
    value  Value
    parent *VarEnv    // nil for empty environment
}

func (env *VarEnv) Lookup(name string) (Value, bool) {
    for e := env; e != nil; e = e.parent {
        if e.name == name {
            return e.value, true
        }
    }
    return nil, false
}

func (env *VarEnv) Extend(name string, val Value) *VarEnv {
    return &VarEnv{name: name, value: val, parent: env}
}
```

**Flat map (imperative, copied-on-extend):**

```go
type VarEnv struct {
    bindings map[string]Value
}

func (env *VarEnv) Lookup(name string) (Value, bool) {
    v, ok := env.bindings[name]
    return v, ok
}

func (env *VarEnv) Extend(name string, val Value) *VarEnv {
    newBindings := make(map[string]Value, len(env.bindings)+1)
    for k, v := range env.bindings {
        newBindings[k] = v
    }
    newBindings[name] = val
    return &VarEnv{bindings: newBindings}
}
```

**Comparison:**

| Property | Linked list | Flat map |
|---|---|---|
| Extend cost | O(1) allocation | O(n) copy |
| Lookup cost | O(d) where d = depth | O(1) amortized |
| Sharing | Parent shared across children | Each extension is independent |
| Memory | Efficient for deep scopes | Efficient for wide, shallow scopes |
| Go idiom | Less conventional | More conventional |

**Recommendation:** Use the linked list for the initial implementation. It is simpler, allocation-efficient for the recursive evaluator pattern, and naturally persistent (closures that share environments do not interfere with each other). If profiling reveals that lookup cost is significant (unlikely for programs with typical scope depths of 5-20 bindings), switch to a hybrid scheme (e.g., linked list with periodic flattening).

**How Starlark-go does it:** Starlark-go uses a flat array indexed by a compile-time-assigned slot number. Each function's local variables are assigned consecutive integer indices during compilation. The environment is a `[]Value` slice. This is the fastest approach but requires a compilation pass that assigns indices; it is appropriate for a mature implementation but not necessary for the initial interpreter.

**How CEL-go does it:** CEL-go uses an `Activation` interface with a `ResolveName(name string) (any, bool)` method. Activations are linked: a child activation can delegate to a parent. This is essentially the linked-list approach with an interface abstraction.

### 5.4 Two Environments: Variables and Capabilities

The evaluator maintains two distinct environment structures:

1. **Variable environment (`VarEnv`):** Maps variable names to values. Extended when a `bind` result is named or when entering a function body. Used for value-layer evaluation.

2. **Capability environment (`CapEnv`):** Maps capability labels to host-managed handles with protocol state tags. Modified only by primitive operations. Threaded through computation evaluation.

These two environments serve fundamentally different purposes and must be kept separate:

- The variable environment is **extended** by `bind` and function application (new bindings are added; old bindings are shadowed but not removed).
- The capability environment is **transformed** by primitive operations (an existing entry's state changes; the label set remains the same except for operations that add or remove capabilities).

- The variable environment is **lexically scoped** (a closure captures its creation-time variable environment).
- The capability environment is **linearly threaded** (it flows from left to right through the `bind` chain, and is not captured by closures).

This distinction is critical for correctness. If a closure captured the capability environment, its later invocation would use a stale capability state, violating the typestate discipline. The formal rules enforce this: closures capture `rho` (the variable environment) but not `sigma` (the capability environment). The capability environment is always the "current" one at the point where the computation executes.

```go
type EvalState struct {
    VarEnv *VarEnv    // variable bindings (lexically scoped)
    CapEnv *CapEnv    // capability state (linearly threaded)
}
```

### 5.5 The Evaluation Loop in Go

The evaluator is a pair of mutually recursive functions:

```go
// evalValue evaluates a value expression to a runtime value.
// It uses only the variable environment; the capability environment is not needed.
func evalValue(env *VarEnv, expr ValueExpr) (Value, error) {
    switch e := expr.(type) {
    case *VarExpr:
        v, ok := env.Lookup(e.Name)
        if !ok {
            return nil, fmt.Errorf("unbound variable: %s", e.Name)
        }
        return v, nil

    case *LitIntExpr:
        return IntVal(e.Value), nil

    case *LitStrExpr:
        return StrVal(e.Value), nil

    case *UnitExpr:
        return UnitVal{}, nil

    case *LamExpr:
        return &Closure{Param: e.Param, Body: e.Body, Env: env}, nil

    case *AppExpr:
        fv, err := evalValue(env, e.Func)
        if err != nil {
            return nil, err
        }
        clo, ok := fv.(*Closure)
        if !ok {
            return nil, fmt.Errorf("application of non-function")
        }
        av, err := evalValue(env, e.Arg)
        if err != nil {
            return nil, err
        }
        return evalValue(clo.Env.Extend(clo.Param, av), clo.Body)

    case *CtorExpr:
        args := make([]Value, len(e.Args))
        for i, a := range e.Args {
            v, err := evalValue(env, a)
            if err != nil {
                return nil, err
            }
            args[i] = v
        }
        return &CtorVal{Tag: e.Tag, Args: args}, nil

    case *CaseExpr:
        sv, err := evalValue(env, e.Scrutinee)
        if err != nil {
            return nil, err
        }
        cv := sv.(*CtorVal)
        for _, branch := range e.Branches {
            if branch.Tag == cv.Tag {
                extEnv := env
                for i, name := range branch.Bindings {
                    extEnv = extEnv.Extend(name, cv.Args[i])
                }
                return evalValue(extEnv, branch.Body)
            }
        }
        return nil, fmt.Errorf("non-exhaustive case")
    }
    panic("unreachable")
}

// evalComp evaluates a computation expression.
// It uses both the variable environment and the capability environment.
// It returns the updated capability environment and the result value.
func evalComp(
    env *VarEnv,
    capEnv *CapEnv,
    comp CompExpr,
) (*CapEnv, Value, error) {
    switch c := comp.(type) {
    case *PureExpr:
        v, err := evalValue(env, c.Value)
        if err != nil {
            return nil, nil, err
        }
        return capEnv, v, nil

    case *BindExpr:
        capEnv1, v, err := evalComp(env, capEnv, c.Comp)
        if err != nil {
            return nil, nil, err
        }
        extEnv := env.Extend(c.Var, v)
        return evalComp(extEnv, capEnv1, c.Cont)

    case *PrimOpExpr:
        args := make([]Value, len(c.Args))
        for i, a := range c.Args {
            v, err := evalValue(env, a)
            if err != nil {
                return nil, nil, err
            }
            args[i] = v
        }
        return c.Op.Execute(capEnv, args)
    }
    panic("unreachable")
}
```

Several things to note about this structure:

1. `evalValue` does not take or return a `CapEnv`. Value evaluation is pure.
2. `evalComp` threads `CapEnv` through: it receives a `CapEnv` and returns a (possibly different) `CapEnv`.
3. The `BindExpr` case directly mirrors the [C-Bind] rule.
4. The `PrimOpExpr` case delegates to the host-provided `Execute` method.

### 5.6 Step Counting

Resource limits are implemented by threading a step counter through the evaluation functions:

```go
func evalCompWithFuel(
    env *VarEnv,
    capEnv *CapEnv,
    comp CompExpr,
    fuel *int,
) (*CapEnv, Value, error) {
    *fuel--
    if *fuel <= 0 {
        return nil, nil, fmt.Errorf("step limit exceeded")
    }
    // ... same switch as above, but passing fuel to recursive calls
}
```

Each entry into `evalComp` (and optionally `evalValue`) decrements the counter. When the counter reaches zero, evaluation halts with a deterministic error.

This is precisely how Starlark-go implements `Thread.SetMaxExecutionSteps`: a counter on the `Thread` object is decremented at each statement execution, and the interpreter returns an error when it reaches zero.

---

## 6. Primitive Operation Evaluation

### 6.1 The Host Boundary

Primitive operations are the only source of capability state transitions. They are registered by the host and invoked by the evaluator when a `prim(op, ...)` expression is encountered.

A primitive operation has:

- A **name** (e.g., `dbOpen`).
- A **declared type** (e.g., `forall r. Computation { db : DB[Closed] | r } { db : DB[Opened] | r } Unit`).
- A **Go implementation** (a function that operates on the capability environment and argument values).

### 6.2 The PrimOp Interface

```go
type PrimOp interface {
    Name() string
    Type() TypeExpr     // the declared type, used by the type checker
    Execute(capEnv *CapEnv, args []Value) (*CapEnv, Value, error)
}
```

The `Execute` method receives:

1. The current capability environment (corresponding to the `pre` row).
2. The evaluated arguments (values, already reduced by the evaluator).

It returns:

1. The new capability environment (corresponding to the `post` row).
2. The result value.
3. An error, if the operation fails.

### 6.3 How Capability State Transitions Happen

A primitive operation's Go implementation is responsible for:

1. Extracting the relevant capability handle from the environment.
2. Performing the host-side operation (e.g., opening a database connection).
3. Constructing the new capability environment with updated state.

However, as recommended in the host-boundary design patterns document, the runtime should mediate this:

```go
func (op *DBOpenOp) Execute(capEnv *CapEnv, args []Value) (*CapEnv, Value, error) {
    // 1. Get the capability handle (runtime-verified)
    handle, err := capEnv.GetInState("db", DBClosed)
    if err != nil {
        return nil, nil, err  // should not happen if type checker is correct
    }

    // 2. Perform the host operation
    db := handle.Internal().(*sqlx.DB)
    if err := db.Ping(); err != nil {
        return nil, nil, fmt.Errorf("dbOpen: %w", err)
    }

    // 3. Transition the capability state
    newCapEnv, err := capEnv.Transition("db", DBOpened)
    if err != nil {
        return nil, nil, err  // should not happen if type checker is correct
    }

    return newCapEnv, UnitVal{}, nil
}
```

The runtime checks in steps 1 and 3 are defense-in-depth: the type checker has already verified that the capability is in the correct state. But the runtime check catches bugs in the host implementation and in the type checker itself.

### 6.4 Error Handling During Primitive Evaluation

When a primitive operation fails:

1. The capability environment is **not** transitioned. The `pre` state is preserved.
2. The error propagates upward through the `bind` chain.
3. Evaluation halts with an error diagnostic that includes the operation name and the error message.
4. No further computations in the `bind` chain are executed.

This is the "abort on error" model described in the host-boundary design patterns document. It is the simplest model consistent with the current spec's lack of exception handling or error recovery constructs.

### 6.5 Primitive Operations with Arguments

Some primitive operations take arguments from the embedded language:

```text
dbQuery : Query -> Computation { db : DB[Opened] | r } { db : DB[Opened] | r } Rows
```

In the core calculus, this is modeled as a value-level function that returns a computation:

```text
dbQuery = \q -> prim(dbQueryOp, q)
```

The evaluator first evaluates the application `dbQuery queryExpr` as a value-level application, producing a computation `prim(dbQueryOp, queryVal)`. Then, when this computation is executed (via `bind`), the primitive operation receives the query value as an argument.

Alternatively, the elaborator can produce a `prim` node with pre-evaluated argument slots. Both approaches are equivalent; the choice is an implementation concern.

### 6.6 Non-Effectful Host Functions

Some host functions are pure (they do not modify capability state):

```text
parseInt : String -> Int
```

These are registered as value-level primitives, not computation-level primitives. They are invoked during value evaluation:

```go
case *PrimValExpr:
    args := make([]Value, len(e.Args))
    for i, a := range e.Args {
        v, err := evalValue(env, a)
        if err != nil {
            return nil, err
        }
        args[i] = v
    }
    return e.Impl(args)
```

Pure host functions do not interact with the capability environment. They are ordinary functions that happen to be implemented in Go rather than in the embedded language.

---

## 7. Do-Notation Desugaring and Elaboration

### 7.1 The Surface Syntax

The spec (Section 12) indicates a Haskell-like surface syntax. Do-notation provides syntactic sugar for computation sequencing:

```text
do {
    x <- c1
    y <- c2
    c3
}
```

### 7.2 Desugaring Rules

The elaborator translates do-notation to the core calculus before evaluation. The rules are mechanical:

**Rule D-Bind (named bind):**

```text
do { x <- c; rest }   ==>   bind c (\x -> do { rest })
```

A monadic bind with a named result. The variable `x` is bound in the remaining computations.

**Rule D-Then (unnamed bind):**

```text
do { c; rest }   ==>   bind c (\_ -> do { rest })
```

A monadic bind where the result is discarded. The wildcard `_` indicates that the result value is not used.

**Rule D-Let (local value binding):**

```text
do { x := e; rest }   ==>   (\x -> do { rest }) e
```

A local value binding. The expression `e` is evaluated as a value, and the result is bound to `x` in the remaining computations. This is a value-level operation (application of a lambda), not a computation-level operation.

Note: the application `(\x -> ...) e` is a value-level beta-redex. It evaluates `e` to a value, then substitutes into the body. This is equivalent to `let x = e in ...` at the value level.

**Rule D-Tail (final expression):**

```text
do { c }   ==>   c
```

The final expression in a do-block is the computation itself. If the final expression is a value expression `v` rather than a computation, it must be wrapped:

```text
do { v }   ==>   pure v
```

Whether this wrapping is explicit or implicit is a surface-syntax design choice. The spec's current direction suggests that the final position should be a computation; wrapping with `pure` is the user's responsibility unless inference determines it is needed.

### 7.3 Complete Desugaring Example

Surface:

```text
workflow :: Computation { db : DB[Closed] }
                        { db : DB[Closed] }
                        Rows
workflow = do {
    dbOpen
    rows <- dbQuery (SELECT * FROM users)
    dbClose
    pure rows
}
```

After desugaring:

```text
workflow =
    bind dbOpen (\_ ->
        bind (dbQuery (SELECT * FROM users)) (\rows ->
            bind dbClose (\_ ->
                pure rows)))
```

Each line of the do-block becomes a `bind` with a continuation. The final `pure rows` is already a computation and remains unchanged.

### 7.4 Elaboration Timing: Before Evaluation

Desugaring occurs during elaboration, which happens after parsing and before type checking. The evaluation phase operates only on the core calculus (no do-notation, no surface sugar).

The pipeline:

```text
source text
  -> parse -> surface AST (with do-notation, operators, etc.)
  -> elaborate -> core AST (pure/bind/prim only)
  -> type check -> typed core AST
  -> evaluate -> result
```

Elaboration before type checking ensures that the type checker only needs rules for the core constructs (`pure`, `bind`, `prim`, lambda, application, case). This keeps the type checker simple and aligned with the formal rules.

### 7.5 Let-Bindings in Computation Context

A do-block may contain value-level let-bindings:

```text
do {
    x := computeKey(input)    -- value-level: x = computeKey(input)
    rows <- dbQuery x          -- computation-level: rows <- dbQuery x
    pure rows
}
```

This desugars to:

```text
(\x ->
    bind (dbQuery x) (\rows ->
        pure rows)
) (computeKey input)
```

The value-level binding `x := computeKey(input)` becomes a lambda application. The key point: `computeKey(input)` is evaluated as a value (strictly, before the lambda body), and the result is bound to `x`. The computation `dbQuery x` then runs with `x` already evaluated.

---

## 8. Metatheoretic Properties

### 8.1 Determinism

**Theorem (Determinism of Evaluation).** For well-typed terms, if `rho; sigma |- c ==> sigma1; w1` and `rho; sigma |- c ==> sigma2; w2`, then `sigma1 = sigma2` and `w1 = w2`.

*Sketch.* By induction on the derivation of the big-step judgment. Each rule is deterministic:

- **V-Var:** environment lookup is a function.
- **V-Lam:** closure construction is deterministic.
- **V-App:** by induction, the function evaluates to a unique closure and the argument to a unique value; the body then evaluates deterministically.
- **C-Pure:** value evaluation is deterministic (by induction on value rules).
- **C-Bind:** both sub-computations are deterministic by induction.
- **C-PrimOp:** host operations are assumed to be deterministic functions. This is a **host contract**: the host must not introduce non-determinism. If a host operation's behavior depends on external state (e.g., current time), it must receive that state explicitly through the capability environment, not by reading ambient state.

The determinism property depends on three assumptions:

1. The term is well-typed.
2. Host operations are deterministic functions of their arguments and the capability environment.
3. Pattern matching is exhaustive (guaranteed by static analysis).

### 8.2 Type Safety (Schematic)

Type safety is traditionally decomposed into two theorems:

**Progress.** If `Gamma |- c : Computation r1 r2 A` and the runtime environments match `Gamma` and `r1`, then either `c` is a terminal computation (`pure v` where `v` is a value) or `c` can take a step.

**Preservation.** If `Gamma |- c : Computation r1 r2 A` and `<rho; sigma; c> --> <rho'; sigma'; c'>`, then there exist `Gamma'` and `r1'` such that `Gamma' |- c' : Computation r1' r2 A`, and `rho'` matches `Gamma'`, and `sigma'` matches `r1'`.

For the big-step formulation, the combined statement is:

**Type safety (big-step).** If `Gamma |- c : Computation r1 r2 A`, and `rho` matches `Gamma`, and `sigma` matches `r1`, and host operations satisfy their declared types, then `rho; sigma |- c ==> sigma'; w` where `sigma'` matches `r2` and `w` has type `A`.

*Sketch.* By induction on the typing derivation, showing that each evaluation rule produces a value of the correct type and a capability environment matching the declared post-row.

The critical case is [C-Bind]:

```text
  Gamma |- c : Computation r1 r2 A
  Gamma, x : A |- c' : Computation r2 r3 B
  ──────────────────────────────────────────
  Gamma |- bind c (\x -> c') : Computation r1 r3 B
```

By induction, evaluating `c` under `sigma` (matching `r1`) produces `sigma'` (matching `r2`) and `w : A`. The continuation `c'` is then evaluated under `sigma'` (matching `r2`) with `x` bound to `w : A`, producing `sigma''` (matching `r3`) and `w' : B`. The composition gives `sigma''` matching `r3` and `w' : B`, as required.

The case [C-PrimOp] requires the **host correctness assumption**: if a primitive is declared with type `Computation pre post A`, then its Go implementation, given a capability environment matching `pre` and arguments of the declared types, returns a capability environment matching `post` and a value of type `A` (or an error). This is a contract that the host must satisfy; the language runtime cannot verify it.

### 8.3 Termination

**Does the language guarantee termination?**

The current spec does not include general recursion. Without a fixed-point combinator or recursive let-bindings, all well-typed terms in the core calculus terminate.

- Value-level expressions terminate because: lambda bodies are only evaluated when applied, application reduces the size of the redex (no self-application in the simply-typed lambda calculus with ADTs), and case analysis is structural.
- Computation-level expressions terminate because: `pure` returns immediately, `bind` sequences two computations that terminate by induction, and `prim` is assumed to terminate (host contract).

**Step limits as a practical safeguard.** Even with termination guarantees from the type system, step limits provide defense-in-depth:

1. They protect against bugs in the type checker (an ill-typed term that somehow passes checking could diverge).
2. They bound execution time for DoS prevention in multi-tenant scenarios.
3. They enable deterministic resource accounting.

If general recursion is added in a future extension (e.g., recursive let-bindings or a fix-point combinator), termination is no longer guaranteed, and step limits become essential for the termination of the evaluator.

### 8.4 The Monad Laws at Runtime

The parameterized monad laws (left identity, right identity, associativity) should hold as runtime equalities:

**Left identity:**

```text
run rho sigma (bind (pure v) (\x -> c))
  = run rho sigma (c[x := v])
```

By [C-Bind] and [C-Pure]: `pure v` evaluates to `(sigma, v)`, then `c` is evaluated with `x` bound to `v` under `sigma`. This is the same as evaluating `c[x := v]` under `sigma`.

**Right identity:**

```text
run rho sigma (bind c (\x -> pure x))
  = run rho sigma c
```

By [C-Bind]: `c` evaluates to `(sigma', w)`, then `pure x` with `x = w` evaluates to `(sigma', w)`. Same result.

**Associativity:**

```text
run rho sigma (bind (bind c1 (\x -> c2)) (\y -> c3))
  = run rho sigma (bind c1 (\x -> bind c2 (\y -> c3)))
```

By expanding [C-Bind] on both sides: `c1` evaluates to `(sigma1, w1)`, then `c2` (with `x = w1`) evaluates to `(sigma2, w2)`, then `c3` (with `y = w2`) evaluates to `(sigma3, w3)`. Both sides produce `(sigma3, w3)`.

These equalities hold by the structure of the evaluation rules. They do not need to be separately verified at runtime; they are consequences of the rules.

---

## 9. Evaluation Traces: Worked Examples

### 9.1 Example: Simple Pure Computation

**Program:**

```text
run {} (pure 42)
```

**Trace (big-step):**

```text
rho = []; sigma = {}

rho; sigma |- pure 42 ==> ?

  By [C-Pure]:
    rho |- 42 ==> 42        (by [V-Lit-Int])
    sigma' = sigma = {}
    w = 42

Result: ({}, 42)
```

### 9.2 Example: Bind with Pure Computations

**Program:**

```text
run {} (bind (pure 1) (\x -> pure (x + 1)))
```

Assuming `+` is a built-in value-level operation or a host-provided pure function.

**Trace:**

```text
rho = []; sigma = {}

rho; sigma |- bind (pure 1) (\x -> pure (x + 1)) ==> ?

  By [C-Bind]:
    First, evaluate: rho; sigma |- pure 1 ==> ?
      By [C-Pure]:
        rho |- 1 ==> 1
        Result: (sigma={}, w=1)

    Then, evaluate continuation with x=1:
    rho[x->1]; {} |- pure (x + 1) ==> ?
      By [C-Pure]:
        rho[x->1] |- x + 1 ==> ?
          rho[x->1] |- x ==> 1       (by [V-Var])
          rho[x->1] |- 1 ==> 1       (by [V-Lit-Int])
          1 + 1 = 2                   (by built-in addition)
        rho[x->1] |- x + 1 ==> 2
        Result: ({}, 2)

Result: ({}, 2)
```

### 9.3 Example: Database Open/Close with Capability State Transition

**Program:**

```text
run { db: handle(DB, Closed) }
    (bind dbOpen (\_ ->
        bind dbClose (\_ ->
            pure ())))
```

**Trace:**

```text
rho = []; sigma = { db: <DB, Closed, conn> }

rho; sigma |- bind dbOpen (\_ -> bind dbClose (\_ -> pure ())) ==> ?

  By [C-Bind]:
    First, evaluate: rho; sigma |- dbOpen ==> ?
      dbOpen = prim(dbOpenOp)
      By [C-PrimOp]:
        host(dbOpenOp, { db: <DB, Closed, conn> })
          = Ok({ db: <DB, Opened, conn> }, ())
        Result: (sigma1 = { db: <DB, Opened, conn> }, w1 = ())

    Then, evaluate continuation with _=():
    rho[_->()]; sigma1 |- bind dbClose (\_ -> pure ()) ==> ?

      By [C-Bind]:
        First, evaluate: rho'; sigma1 |- dbClose ==> ?
          dbClose = prim(dbCloseOp)
          By [C-PrimOp]:
            host(dbCloseOp, { db: <DB, Opened, conn> })
              = Ok({ db: <DB, Closed, conn> }, ())
            Result: (sigma2 = { db: <DB, Closed, conn> }, w2 = ())

        Then, evaluate continuation with _=():
        rho''; sigma2 |- pure () ==> ?
          By [C-Pure]:
            Result: (sigma2 = { db: <DB, Closed, conn> }, ())

Result: ({ db: <DB, Closed, conn> }, ())
```

The capability environment transitions: `{ db: Closed }` to `{ db: Opened }` to `{ db: Closed }`. The type system verifies that the final state matches the declared `post` row.

### 9.4 Example: Database Query with Value Threading

**Program (surface):**

```text
workflow = do {
    dbOpen
    rows <- dbQuery (SELECT * FROM users)
    dbClose
    pure rows
}
```

**After desugaring:**

```text
bind dbOpen (\_ ->
    bind (dbQuery (SELECT * FROM users)) (\rows ->
        bind dbClose (\_ ->
            pure rows)))
```

**Trace:**

```text
rho = []; sigma = { db: <DB, Closed, conn> }

Step 1: evaluate dbOpen
  host(dbOpenOp, sigma) = Ok({ db: <DB, Opened, conn> }, ())
  sigma1 = { db: <DB, Opened, conn> }

Step 2: evaluate dbQuery (SELECT * FROM users)
  First, evaluate the argument: SELECT * FROM users ==> queryVal
  host(dbQueryOp, sigma1, queryVal) = Ok({ db: <DB, Opened, conn> }, rowsVal)
  sigma2 = { db: <DB, Opened, conn> }   (state-preserving operation)

Step 3: evaluate dbClose
  host(dbCloseOp, sigma2) = Ok({ db: <DB, Closed, conn> }, ())
  sigma3 = { db: <DB, Closed, conn> }

Step 4: evaluate pure rows
  rows is bound to rowsVal from Step 2
  Result: (sigma3, rowsVal)

Final result: ({ db: <DB, Closed, conn> }, rowsVal)
```

### 9.5 Example: Value-Level Function Application inside a Computation

**Program:**

```text
let double = \x -> x + x
in
bind (pure 21) (\n ->
    pure (double n))
```

**Trace:**

```text
rho = []; sigma = {}

Step 1: evaluate \x -> x + x
  rho |- \x -> x + x ==> <\x -> x + x, []>
  rho1 = [double -> <\x -> x + x, []>]

Step 2: evaluate bind (pure 21) (\n -> pure (double n))
  under rho1, sigma

  By [C-Bind]:
    rho1; sigma |- pure 21 ==> ({}, 21)

    rho1[n->21]; {} |- pure (double n) ==> ?
      rho2 = [double -> <\x -> x + x, []>, n -> 21]
      rho2 |- double n ==> ?
        rho2 |- double ==> <\x -> x + x, []>    (by [V-Var])
        rho2 |- n ==> 21                         (by [V-Var])
        By [V-App]:
          [][x -> 21] |- x + x ==> ?
            [x->21] |- x ==> 21
            21 + 21 = 42
          Result: 42
      rho2 |- double n ==> 42
      Result: ({}, 42)

Final result: ({}, 42)
```

Note how the closure `double` captured the empty environment `[]`. When applied, it evaluates its body in `[][x -> 21]`, not in the caller's environment. This is lexical scoping.

### 9.6 Example: Small-Step Trace

Using the small-step rules for the same bind example:

**Program:**

```text
bind (pure 1) (\x -> pure (x + 1))
```

**Small-step trace:**

```text
<[]; {}; bind (pure 1) (\x -> pure (x + 1))>
  --> by [S-Bind-Step] + [S-Pure]:
    eval([], 1) = 1
    <[]; {}; pure 1> --> <[]; {}; done(1)>
  so:
<[]; {}; bind (done(1)) (\x -> pure (x + 1))>
  --> by [S-Bind-Done]:
<[x->1]; {}; pure (x + 1)>
  --> by [S-Pure]:
    eval([x->1], x + 1) = 2
<[x->1]; {}; done(2)>

Terminal. Result: ({}, 2).
```

Three steps. Each step is a single rule application. Step counting in this regime counts computation-level reductions.

---

## 10. Comparison with Related Systems

### 10.1 Starlark

**Evaluation model:** Tree-walking interpreter with deterministic evaluation. Strict, left-to-right, no lazy evaluation.

**Relevance:** Starlark is the closest practical reference for Gomputation's implementation. Its evaluator walks the AST recursively, maintains an environment of bindings per scope, and provides step counting via `Thread.SetMaxExecutionSteps`.

**Key difference:** Starlark has no value/computation split and no capability state tracking. All functions can have side effects (through host-provided builtins), and the type system is dynamic. Gomputation adds the indexed computation layer and static typing, but the core evaluation loop has the same shape: a recursive function that dispatches on the AST node type, maintains an environment, and calls host functions for primitive operations.

**Starlark's environment model:** Flat array indexed by compile-time-assigned slot numbers. This is more efficient than a linked-list or map-based environment but requires a compilation pass. For Gomputation's initial implementation, the simpler linked-list approach is preferable; Starlark's array-based approach is a good optimization target.

### 10.2 CEL

**Evaluation model:** Expression evaluation with host-provided data and functions. No side effects, no mutation, no loops. Guaranteed termination.

**Relevance:** CEL demonstrates that a non-Turing-complete embedded language with host-provided functions can be practical and widely adopted. Its evaluation model is strictly simpler than Gomputation's: expressions evaluate to values, and there is no computation layer. The CEL evaluator is essentially a function `eval : Activation * Expr -> Val`.

**Key difference:** CEL has no sequencing, no `bind`, no capability state. It evaluates a single expression to a single value. Gomputation extends this model with computation sequencing and capability state threading.

**CEL's type system:** CEL has a static type checker that validates expressions before evaluation. This is the same pipeline structure Gomputation should adopt: parse, check, evaluate as separate phases.

### 10.3 Koka

**Evaluation model:** Koka uses an algebraic effect system with handlers. Evaluation is strict (call-by-value) with effect typing. Effect handlers provide scoped control flow.

**Relevance:** Koka demonstrates that a strict, effect-typed language can be practical. Its evaluation semantics involves reducing effect operations to handler frames, which is more complex than Gomputation's model (where effects are simple host callbacks, not handler-dispatched operations).

**Key difference:** Koka's effects are algebraic: they have a well-defined interaction with handlers that can resume, abort, or transform computations. Gomputation's effects are simpler: a primitive operation calls the host, gets a result, and continues. There is no handler mechanism, no resumption, and no effect polymorphism beyond row polymorphism.

**Evaluation strategy:** Koka is strict, which aligns with Gomputation's choice. Koka compiles to a backend (C, JavaScript), while Gomputation is interpreted. But the surface-level evaluation discipline is the same: arguments are evaluated before passing, and effects are sequenced explicitly.

### 10.4 Haskell

**Evaluation model:** Lazy (call-by-need) with monadic sequencing of effects. The IO monad provides deterministic sequencing within monadic code, but the evaluation of pure expressions is demand-driven.

**Relevance:** Haskell's `do`-notation is the syntactic inspiration for Gomputation's surface computation syntax. The monad laws that Haskell programmers rely on are the same laws that govern Gomputation's `pure` and `bind`. Haskell's separation of pure and effectful code via the IO monad is the conceptual ancestor of Gomputation's value/computation split.

**Key difference:** Laziness. Haskell's evaluation of pure expressions is lazy, which means thunks are created for unevaluated expressions and forced on demand. This is incompatible with Gomputation's strict evaluation. In Haskell, `let x = error "boom" in 42` evaluates to `42` (the error is never forced). In Gomputation, the equivalent would evaluate `error "boom"` immediately, producing an error before `42` is ever reached.

Haskell's parameterized monad libraries (`indexed`, `indexed-extras`, `Control.Monad.Indexed`) provide Atkey-style indexed monads with the same `ireturn`/`ibind` signature as Gomputation's `pure`/`bind`. The evaluation semantics of these libraries is the same as Gomputation's: `ibind` evaluates the first computation, passes the result to the continuation, and evaluates the continuation. Laziness affects when pure subexpressions within the monadic code are evaluated, but the sequencing of monadic actions is strict (left-to-right) regardless of the evaluation strategy.

### 10.5 Idris

**Evaluation model:** Strict by default (Idris 2), with optional laziness via `Lazy` and `Delay`/`Force`. Dependent types mean that the evaluator is also used during type checking (to normalize type-level expressions).

**Relevance:** Idris demonstrates an elaboration-driven architecture where surface syntax is translated to a small core language (TT) before evaluation. This is the architecture Gomputation should adopt: elaborate surface syntax (including do-notation) to a core calculus, then evaluate the core.

**Key difference:** Idris's evaluator must handle type-level computation (reducing types during type checking), which Gomputation does not need. Gomputation's type-level language is simpler: types are structural (alpha-equivalence plus row normalization), with no type-level reduction.

**Idris's elaboration:** Idris uses a tactic-based elaborator that translates high-level syntax to a core calculus through a sequence of type-directed steps. Gomputation's elaboration is simpler (mechanical desugaring of do-notation and syntactic sugar), but the architectural principle is the same: separate surface from core, and evaluate only the core.

---

## 11. Recommendations for Gomputation

### 11.1 Evaluation Strategy

Adopt strict, call-by-value evaluation for the value layer. This is non-negotiable given the Go host, the determinism commitment, and the CBPV-inspired design.

### 11.2 Primary Semantics

Use big-step (natural) semantics as the primary specification of evaluation behavior. The rules in Section 3 should be considered the normative definition. They directly translate to a Go implementation as a pair of recursive functions (`evalValue`, `evalComp`).

### 11.3 Implementation Architecture

```text
source -> parse -> surface AST -> elaborate -> core AST -> type check -> typed core -> evaluate -> result
```

The evaluator operates on the typed core AST. It is a tree-walking interpreter with:

- Recursive descent over the AST.
- Linked-list variable environments with closures.
- Sorted-slice capability environments.
- Host-provided primitive operations invoked via an interface.
- Step counting for resource limits.

### 11.4 Environment Separation

Maintain two distinct environments:

1. **Variable environment** (lexically scoped, captured by closures, extended by `bind` and lambda).
2. **Capability environment** (linearly threaded, not captured by closures, transformed only by primitive operations).

### 11.5 Error Model

Use the "abort on error" model: host operation failures halt computation with a diagnostic. No exception handling or error recovery in the current spec. Errors from host operations do not transition capability state.

### 11.6 Step Limits

Implement step counting as a decrementing counter threaded through the evaluation functions. Each computation-level evaluation step decrements the counter. Optionally, each value-level evaluation step also decrements the counter (for defense against computationally expensive pure expressions).

### 11.7 Elaboration

Desugar do-notation and surface sugar during elaboration, before type checking. The type checker and evaluator should only know about the core constructs: `pure`, `bind`, `prim`, lambda, application, case, literals.

### 11.8 Future Considerations

If general recursion is added:

- Termination is no longer guaranteed.
- Step limits become essential.
- The small-step semantics becomes important for reasoning about divergence.
- Consider adding a `fix` construct to the core calculus with an explicit evaluation rule.

If algebraic effects and handlers are added:

- The evaluation model changes significantly: effect operations search for a handler frame on the evaluation stack.
- The small-step semantics becomes essential.
- The big-step semantics must be reformulated in terms of evaluation stacks or continuations.
- This is explicitly deferred in the current spec.

---

## 12. Key References

### Operational Semantics: Textbooks

1. Gilles Kahn. "Natural semantics." *Proceedings of STACS*, 1987. The original paper on big-step (natural) semantics.

2. Gordon Plotkin. "A structural approach to operational semantics." *DAIMI Report FN-19*, Aarhus University, 1981. Reprinted in *Journal of Logic and Algebraic Programming*, 60--61:17--139, 2004. The foundational paper on structural operational semantics (small-step).

3. Matthias Felleisen and Robert Hieb. "The revised report on the syntactic theories of sequential control and state." *Theoretical Computer Science*, 103(2):235--271, 1992. Evaluation contexts and reduction semantics.

4. Benjamin C. Pierce. *Types and Programming Languages*. MIT Press, 2002. Chapters 3--5 (untyped arithmetic and lambda calculus), Chapters 8--9 (type safety for simply typed lambda calculus). The standard textbook treatment of operational semantics and type safety.

5. Robert Harper. *Practical Foundations for Programming Languages*. Cambridge University Press, 2nd edition, 2016. Part I (Judgments and Rules), Part II (Statics and Dynamics). Harper's treatment of dynamics (evaluation semantics) is particularly relevant.

### CBPV Operational Semantics

6. Paul Blain Levy. *Call-By-Push-Value: A Functional/Imperative Synthesis*. Springer, 2004. Chapter 4 (operational semantics) gives big-step rules for CBPV with the value/computation split.

7. Robert Harper. "Polarization: Call-by-Push-Value." Course notes for 15-819, CMU, 2025. Operational rules for CBPV in the same notation used in *Practical Foundations*.

### Parameterized Monads

8. Robert Atkey. "Parameterised Notions of Computation." *Journal of Functional Programming*, 19(3-4):335--376, 2009. Section 3 gives the operational meaning of parameterized monadic sequencing.

### Embedded Language Implementations

9. Alan Donovan et al. Starlark-go implementation. https://github.com/google/starlark-go. The `eval.go` file contains a tree-walking interpreter with environment-based evaluation and step counting.

10. Google. CEL-go implementation. https://github.com/google/cel-go. The `interpreter/` directory contains the expression evaluator with activation-based environments.

### Effect Systems and Handlers

11. Daan Leijen. "Type directed compilation of row-typed algebraic effects." *Proceedings of POPL*, 2017. Operational semantics for Koka's effect system.

12. Daniel Hillerstr\"{o}m and Sam Lindley. "Shallow effect handlers." *Proceedings of APLAS*, 2018. Small-step operational semantics for algebraic effect handlers.

### Deterministic Evaluation

13. Starlark language specification. https://github.com/bazelbuild/starlark/blob/master/spec.md. Section on "Execution model" describes deterministic evaluation guarantees.

---

## Appendix A: Summary of Judgments

```text
Judgment                              Reading
────────────────────────────────────  ────────────────────────────────
rho |- v ==> w                        Value v evaluates to w under rho
rho; sigma |- c ==> sigma'; w         Computation c evaluates to w,
                                        transforming sigma to sigma'
eval(rho, v) = w                      Meta-level value evaluation function
host(op, sigma, w1..wn) = r           Host primitive evaluation
<rho; sigma; c> --> <rho'; sigma'; c'> Small-step computation reduction
```

## Appendix B: Summary of Evaluation Rules

### Value Layer (Big-Step)

```text
[V-Lit-Int]   rho |- n ==> n
[V-Lit-Str]   rho |- s ==> s
[V-Unit]      rho |- () ==> ()
[V-Var]       rho(x) = w  =>  rho |- x ==> w
[V-Lam]       rho |- \x -> e ==> <\x -> e, rho>
[V-App]       rho |- v1 ==> <\x -> e, rho'>
              rho |- v2 ==> w2
              rho'[x -> w2] |- e ==> w
              =>  rho |- v1 v2 ==> w
[V-Ctor]      rho |- vi ==> wi (for all i)
              =>  rho |- C v1..vn ==> C w1..wn
[V-Case]      rho |- v ==> Ci w1..wk
              rho[y1->w1,..,yk->wk] |- ei ==> w
              =>  rho |- case v of {..|Ci y1..yk -> ei|..} ==> w
```

### Computation Layer (Big-Step)

```text
[C-Pure]      rho |- v ==> w
              =>  rho; sigma |- pure v ==> sigma; w
[C-Bind]      rho; sigma |- c ==> sigma'; w
              rho[x->w]; sigma' |- c' ==> sigma''; w'
              =>  rho; sigma |- bind c (\x -> c') ==> sigma''; w'
[C-PrimOp]    rho |- vi ==> wi (for all i)
              host(op, sigma, w1..wn) = Ok(sigma', w)
              =>  rho; sigma |- prim(op, v1..vn) ==> sigma'; w
```

### Computation Layer (Small-Step)

```text
[S-Pure]      eval(rho, v) = w
              =>  <rho; sigma; pure v> --> <rho; sigma; done(w)>
[S-Bind-Step] <rho; sigma; c> --> <rho'; sigma'; c'>
              =>  <rho; sigma; bind c (\x -> c'')>
                  --> <rho'; sigma'; bind c' (\x -> c'')>
[S-Bind-Done] <rho; sigma; bind (done(w)) (\x -> c')>
              --> <rho[x->w]; sigma; c'>
[S-PrimOp]    eval(rho, vi) = wi, host(op, sigma, w1..wn) = Ok(sigma', w)
              =>  <rho; sigma; prim(op, v1..vn)> --> <rho; sigma'; done(w)>
```

## Appendix C: Desugaring Rules

```text
[D-Bind]      do { x <- c; rest }    ==>  bind c (\x -> do { rest })
[D-Then]      do { c; rest }         ==>  bind c (\_ -> do { rest })
[D-Let]       do { x := e; rest }    ==>  (\x -> do { rest }) e
[D-Tail]      do { c }               ==>  c
[D-Tail-Val]  do { v }               ==>  pure v    (if v is a value expression)
```
