# Phase 5: Type Checker

## Objective

Implement bidirectional type checking with row unification and elaboration from surface AST to Core IR. This is the largest and most complex phase.

## Dependencies

Phase 1 (`types/`), Phase 2 (`core/`), Phase 4 (`syntax/`).

## Package: `check/`

### 5.1 Architecture Overview

```
Surface AST (syntax.Expr)
    │
    ├─ kind checking (types well-formed?)
    ├─ bidirectional type checking (terms well-typed?)
    │    ├─ infer mode (Γ ⊢ e ⇒ A)
    │    └─ check mode (Γ ⊢ e ⇐ A)
    ├─ row unification (row constraints solvable?)
    ├─ exhaustiveness checking (all cases covered?)
    └─ elaboration (AST → Core IR)
        │
        Core IR (core.Core)
```

All four activities happen **simultaneously** during a single pass. The checker walks the AST, checks types, solves unification constraints, and produces Core IR nodes as it goes.

### 5.2 Checker State (`check/checker.go`)

```go
package check

// Checker holds mutable state during type checking.
type Checker struct {
    ctx       *Context      // typing context (ordered, DK-style)
    unifier   *Unifier      // unification engine
    elaborated core.Core    // result
    errors    *errs.Errors  // accumulated diagnostics
    source    *span.Source
    freshID   int           // counter for fresh metavariables/names
}

// Check type-checks a surface AST program and produces a Core IR program.
// The CheckConfig provides registered types, assumption types, gated built-ins,
// and an optional trace hook — all supplied by the Engine at compilation time.
func Check(prog *syntax.AstProgram, source *span.Source, config *CheckConfig) (*core.Program, *errs.Errors)
```

### 5.3 Typing Context (`check/context.go`)

DK-style ordered context. The context is a sequence of entries, where order matters for scoping.

```go
// Context is an ordered typing context.
type Context struct {
    entries []CtxEntry
}

// CtxEntry variants.
type CtxEntry interface {
    ctxEntry()
}

type CtxVar struct {          // x : A  (term variable)
    Name string
    Type types.Type
}

type CtxTyVar struct {        // a : K  (type variable)
    Name string
    Kind types.Kind
}

type CtxMeta struct {          // ?α : K  (unsolved metavariable)
    ID   int
    Kind types.Kind
}

type CtxSolved struct {        // ?α = T  (solved metavariable)
    ID   int
    Kind types.Kind
    Soln types.Type
}

type CtxMarker struct {        // ▶?α  (scope marker for generalization)
    ID int
}

// Operations on Context.
func (c *Context) LookupVar(name string) (types.Type, bool)
func (c *Context) LookupTyVar(name string) (types.Kind, bool)
func (c *Context) Push(entry CtxEntry)
func (c *Context) Pop() CtxEntry

// InsertBefore inserts entries before a target entry (by ID).
// Required by DK instantiation rules.
func (c *Context) InsertBefore(targetID int, entries ...CtxEntry)

// IsToLeftOf checks whether entry `left` precedes `right` in the context.
// Required for DK well-scopedness checks.
func (c *Context) IsToLeftOf(leftID, rightID int) bool

// DropAfterMarker removes all entries after (and including) the marker.
// Returns the dropped entries (for extracting solved metavars).
func (c *Context) DropAfterMarker(markerID int) []CtxEntry

// Apply walks the context and substitutes all solved metavariables in a type.
func (c *Context) Apply(t types.Type) types.Type
```

### 5.4 Unification Engine (`check/unify.go`)

Union-find with path compression for metavariables. Row unification follows spec §8.3.

```go
// Unifier manages type unification.
type Unifier struct {
    parent map[int]int         // union-find parent
    rank   map[int]int         // union-find rank
    soln   map[int]types.Type  // solved metavariables
    labels map[int]map[string]struct{} // label context per row metavar
}

func NewUnifier() *Unifier

// Unify solves the constraint A ~ B.
// Returns error on failure.
func (u *Unifier) Unify(a, b types.Type) error

// Solve returns the current solution for a metavariable, or nil.
func (u *Unifier) Solve(id int) types.Type

// Zonk walks a type and replaces all solved metavariables with their solutions.
func (u *Unifier) Zonk(t types.Type) types.Type
```

#### Type unification cases

```go
func (u *Unifier) Unify(a, b types.Type) error {
    a = u.Zonk(a)
    b = u.Zonk(b)

    switch {
    case isMeta(a):
        return u.solveMeta(a.(*types.TyMeta), b)
    case isMeta(b):
        return u.solveMeta(b.(*types.TyMeta), a)

    case isTyVar(a) && isTyVar(b):
        if a.(*types.TyVar).Name == b.(*types.TyVar).Name { return nil }
        return mismatch(a, b)

    case isTyCon(a) && isTyCon(b):
        if a.(*types.TyCon).Name == b.(*types.TyCon).Name { return nil }
        return mismatch(a, b)

    case isArrow(a) && isArrow(b):
        // Unify domains and codomains.

    case isForall(a) && isForall(b):
        // Unify bodies under fresh variable.

    case isApp(a) && isApp(b):
        // Unify function parts and argument parts.

    case isComp(a) && isComp(b):
        // Unify pre, post, result.

    case isThunk(a) && isThunk(b):
        // Unify pre, post, result.

    case isRow(a) && isRow(b):
        return u.unifyRows(a.(*types.TyRow), b.(*types.TyRow))

    default:
        return mismatch(a, b)
    }
}
```

#### Row unification (spec §8.3)

```go
func (u *Unifier) unifyRows(r1, r2 *types.TyRow) error {
    // Normalize both rows (sort by label).
    r1 = types.Normalize(r1)
    r2 = types.Normalize(r2)

    // Categorize: shared labels, only-left, only-right.
    shared, onlyLeft, onlyRight := classifyFields(r1.Fields, r2.Fields)

    // Unify types for shared labels.
    for _, label := range shared {
        if err := u.Unify(fieldType(r1, label), fieldType(r2, label)); err != nil {
            return err
        }
    }

    // Case analysis based on tails.
    switch {
    case r1.Tail == nil && r2.Tail == nil:
        // Closed-Closed: require no only-left or only-right.
        if len(onlyLeft) > 0 || len(onlyRight) > 0 {
            return labelMismatch(onlyLeft, onlyRight)
        }

    case r1.Tail != nil && r2.Tail == nil:
        // Open-Closed: onlyLeft must be empty; solve tail.
        if len(onlyLeft) > 0 {
            return extraLabels(onlyLeft)
        }
        rightRemainder := collectFields(r2, onlyRight)
        return u.solveRowVar(r1.Tail, types.ClosedRow(rightRemainder...))

    case r1.Tail == nil && r2.Tail != nil:
        // Closed-Open: symmetric.
        if len(onlyRight) > 0 {
            return extraLabels(onlyRight)
        }
        leftRemainder := collectFields(r1, onlyLeft)
        return u.solveRowVar(r2.Tail, types.ClosedRow(leftRemainder...))

    case r1.Tail != nil && r2.Tail != nil:
        // Open-Open: introduce fresh row variable.
        fresh := u.freshRowMeta()
        leftRemainder := collectFields(r1, onlyLeft)
        rightRemainder := collectFields(r2, onlyRight)
        // r1.Tail := { rightRemainder | fresh }
        // r2.Tail := { leftRemainder | fresh }
        u.solveRowVar(r1.Tail, types.OpenRow(rightRemainder, fresh))
        u.solveRowVar(r2.Tail, types.OpenRow(leftRemainder, fresh))
    }

    return nil
}
```

#### Label uniqueness check

When solving `ρ := R`, verify no label in `R` duplicates a label already surrounding `ρ`:

```go
func (u *Unifier) solveRowVar(v types.Type, solution types.Type) error {
    meta := asMeta(v)
    if meta == nil {
        // v is a rigid variable — cannot solve.
        return rigidError(v, solution)
    }
    // Occurs check.
    if types.OccursIn(meta.ID, solution) {
        return occursError(meta, solution)
    }
    // Label uniqueness check.
    surrounding := u.labels[meta.ID]
    if row, ok := solution.(*types.TyRow); ok {
        for _, f := range row.Fields {
            if _, dup := surrounding[f.Label]; dup {
                return duplicateLabel(f.Label)
            }
        }
    }
    u.soln[meta.ID] = solution
    return nil
}
```

### 5.5 Bidirectional Checking (`check/bidir.go`)

Spec §7.2 (inference) and §7.3 (checking).

```go
// infer produces a type for an expression (Γ ⊢ e ⇒ A) and a Core IR node.
func (ch *Checker) infer(expr syntax.Expr) (types.Type, core.Core)

// check verifies that an expression has a given type (Γ ⊢ e ⇐ A) and produces Core IR.
func (ch *Checker) check(expr syntax.Expr, expected types.Type) core.Core
```

#### Infer cases

```go
func (ch *Checker) infer(expr syntax.Expr) (types.Type, core.Core) {
    switch e := expr.(type) {
    case *syntax.ExprVar:
        if e.Name == "assumption" {
            // Special: assumption as RHS of :=
            // Should have been handled at declaration level.
            ch.error("assumption used outside of definition")
        }
        ty, ok := ch.ctx.LookupVar(e.Name)
        if !ok {
            ch.error("unbound variable: %s", e.Name)
        }
        // Implicit instantiation: if ty is forall, generate fresh metas.
        ty, coreExpr := ch.instantiate(ty, &core.Var{Name: e.Name, S: e.S})
        return ty, coreExpr

    case *syntax.ExprCon:
        // Look up constructor type, instantiate.

    case *syntax.ExprApp:
        funTy, funCore := ch.infer(e.Fun)
        // funTy should be A -> B. Unify and check arg.
        argTy, retTy := ch.matchArrow(funTy)
        argCore := ch.check(e.Arg, argTy)
        return retTy, &core.App{Fun: funCore, Arg: argCore, S: e.S}

    case *syntax.ExprTyApp:
        // Infer expr, expect forall, substitute type argument.

    case *syntax.ExprAnn:
        // Parse annotation type, check expr against it.
        ty := ch.resolveType(e.AnnType)
        coreExpr := ch.check(e.Expr, ty)
        return ty, coreExpr

    case *syntax.ExprInfix:
        // Treat as App(App(Var(op), left), right).

    case *syntax.ExprBlock:
        // Desugar: { x := e; body } → App(Lam(x, body), e)
        return ch.inferBlock(e)

    case *syntax.ExprDo:
        // Desugar do block to pure/bind applications, then infer.
        return ch.inferDo(e)
    }
}
```

#### Check cases

```go
func (ch *Checker) check(expr syntax.Expr, expected types.Type) core.Core {
    switch e := expr.(type) {
    case *syntax.ExprLam:
        // Γ, x : A ⊢ body ⇐ B  ⊢  \x -> body ⇐ A -> B
        argTy, retTy := ch.matchArrow(expected)
        // Bind parameter in context.
        ch.ctx.Push(&CtxVar{Name: paramName(e.Params[0]), Type: argTy})
        bodyCore := ch.check(e.Body, retTy)  // or multi-param lambda
        ch.ctx.Pop()
        return &core.Lam{Param: paramName(e.Params[0]), Body: bodyCore, S: e.S}

    case *syntax.ExprCase:
        // Check scrutinee, check each branch against expected type.
        // Exhaustiveness check.
        return ch.checkCase(e, expected)

    default:
        // Subsumption: infer type, then check A ≤ expected.
        inferredTy, coreExpr := ch.infer(expr)
        ch.subsumes(inferredTy, expected, expr.Span())
        return coreExpr
    }
}
```

### 5.6 Built-in Identifiers (`check/builtin.go`)

`pure`, `bind`, `thunk`, `force`, `assumption` have special treatment.

```go
// builtinTypes are the type signatures of built-in identifiers.
var builtinTypes = map[string]types.Type{
    "pure": /* forall a r. a -> Computation r r a */,
    "bind": /* forall a b r1 r2 r3. Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b */,
    "thunk": /* forall a r1 r2. Computation r1 r2 a -> Thunk r1 r2 a */,
    "force": /* forall a r1 r2. Thunk r1 r2 a -> Computation r1 r2 a */,
    "rec": /* forall r a b. ((a -> Computation r r b) -> a -> Computation r r b) -> a -> Computation r r b */,
    "fix": /* forall a b. ((a -> b) -> a -> b) -> a -> b */,
}

// gatedBuiltins are built-ins that require host opt-in.
// The checker only exposes these if the Engine has enabled them.
var gatedBuiltins = map[string]bool{
    "rec": true,
    "fix": true,
}
```

#### Elaboration of built-ins

When the checker encounters these identifiers applied to arguments:

- `pure e` → `core.Pure(elab(e))`
- `bind c (\x -> e)` → `core.Bind(elab(c), x, elab(e))`
- `thunk c` → `core.Thunk(elab(c))` — special: `c` is NOT evaluated, captured as-is
- `force e` → `core.Force(elab(e))`
- `assumption` (as RHS of `:=`) → `core.PrimOp(name, [])` — elaborated at declaration level
- `rec f a` → `core.LetRec([Binding("self", Lam("x", App(App(f, Var("self")), Var("x"))))], App(Var("self"), a))`
- `fix f a` → `core.LetRec([Binding("self", Lam("x", App(App(f, Var("self")), Var("x"))))], App(Var("self"), a))`

The checker pattern-matches on these specific application forms and emits dedicated Core nodes rather than generic `App` nodes.

For `bind`, recognize the desugared do-block pattern: `bind c (\x -> body)` appears as `App(App(Var("bind"), c), Lam(x, body))`. The checker restructures this into `core.Bind(c, x, body)`.

For `rec`, the checker:
1. Verifies `rec` is enabled (host opt-in via `gatedBuiltins`)
2. Checks `f` against `(a -> Computation r r b) -> a -> Computation r r b`
3. Enforces `pre = post = r` (state-preserving recursion)
4. Emits `LetRec` with a self-referential binding

### 5.7 Kind Checking (`check/kind.go`)

```go
// checkKind verifies that a surface type expression is well-kinded
// and translates it to the canonical types.Type representation.
func (ch *Checker) checkKind(texpr syntax.TypeExpr) (types.Type, types.Kind)

// checkKindExpect checks a type expression against an expected kind.
func (ch *Checker) checkKindExpect(texpr syntax.TypeExpr, expected types.Kind) types.Type
```

Kind checking follows the formation rules in spec §6:

1. Type variable: look up in context.
2. Type constructor: look up registered kind.
3. Type application: infer kind of function, check argument kind.
4. Arrow type: check both sides have kind `Type`.
5. Forall: bind type variable, check body has kind `Type`.
6. Row: check each field has kind `Type`, result has kind `Row`.

### 5.8 Implicit Instantiation (`check/instantiate.go`)

```go
// instantiate replaces leading forall-quantified variables with fresh metavariables.
// Returns the instantiated type and a Core expression wrapped with TyApp nodes.
func (ch *Checker) instantiate(ty types.Type, expr core.Core) (types.Type, core.Core)

// instantiateL solves ?α ≤ B (existential on the left).
// DK left-instantiation judgment.
func (ch *Checker) instantiateL(meta *types.TyMeta, b types.Type)

// instantiateR solves A ≤ ?α (existential on the right).
// DK right-instantiation judgment.
func (ch *Checker) instantiateR(a types.Type, meta *types.TyMeta)

// appInfer implements Γ ⊢ A • e ⇒ C.
// Given function type A and argument e, produce result type C.
// Handles instantiation of forall types at application sites.
func (ch *Checker) appInfer(funType types.Type, arg syntax.Expr) (types.Type, core.Core)
```

Example: `id : forall a. a -> a` at a use site becomes `id @?α : ?α -> ?α`, where `?α` is fresh.

#### Label context registration for row metavariables

When instantiating a forall-quantified row variable that appears inside a row, the surrounding labels must be recorded in the unifier's label context. This enables the label uniqueness check during row variable solving (§5.4 `solveRowVar`).

Example: instantiating `forall r. Computation { db : DB[Closed] | r } ...` creates a fresh row metavar `?ρ` replacing `r`. The unifier must record `{"db"}` as the label context for `?ρ`, so that any future solution for `?ρ` cannot contain a `db` label (which would violate label uniqueness).

```go
// instantiate records label context when a row variable appears inside a row.
// For each TyRow containing a forall-bound row variable:
//   labels surrounding the variable → u.labels[freshMeta.ID]
func (ch *Checker) instantiate(ty types.Type, expr core.Core) (types.Type, core.Core) {
    // ... create fresh metas for forall-bound variables ...
    // For each fresh row meta ?ρ, walk the instantiated type:
    //   if ?ρ appears as tail of { l1:T1, ..., ln:Tn | ?ρ },
    //   register {l1, ..., ln} as label context for ?ρ.
    ch.unifier.RegisterLabelContext(freshMeta.ID, surroundingLabels)
    // ...
}
```

### 5.9 Subsumption (`check/subsume.go`)

```go
// subsumes checks that inferredTy ≤ expectedTy.
//   - forall a. A ≤ A[a := T] (instantiation)
//   - A ≤ A (reflexivity)
//   - Row permutation
func (ch *Checker) subsumes(inferred, expected types.Type, s span.Span)
```

### 5.10 Exhaustiveness Checking (`check/exhaust.go`)

Maranget's algorithm (spec §7.6).

```go
// CheckExhaustive verifies that patterns cover all constructors of the scrutinee type.
// Returns missing patterns on failure.
func CheckExhaustive(patterns []core.Pattern, dataType *DataTypeInfo) []core.Pattern

// CheckRedundant warns on patterns that can never match.
func CheckRedundant(patterns []core.Pattern, dataType *DataTypeInfo) []int

// DataTypeInfo carries constructor information for exhaustiveness.
type DataTypeInfo struct {
    Name         string
    Constructors []ConInfo
}

type ConInfo struct {
    Name  string
    Arity int
}
```

#### Algorithm outline

1. Build a pattern matrix from all alternatives.
2. For each column, decompose by constructor.
3. Recursively check sub-matrices for completeness.
4. Missing = constructors not covered after full decomposition.

### 5.11 Declaration Checking (`check/decl.go`)

```go
// checkDecls processes all declarations in dependency order.
func (ch *Checker) checkDecls(decls []syntax.Decl) *core.Program
```

Processing order:

1. **First pass**: collect fixity declarations, build fixity table.
2. **Second pass**: collect type annotations (name → type).
3. **Third pass**: collect data declarations, register constructors and type constructor kinds.
4. **Fourth pass**: collect type aliases, validate alias graph is acyclic (`validateAliasGraph`), register alias expansions.
5. **Fifth pass**: check value definitions.
   - For each `(name :: type, name := expr)` pair:
     - Kind-check the annotation.
     - If expr is `assumption`: record as PrimOp, no body to check.
     - Otherwise: check expr against the annotated type, producing Core.

### 5.12 Type Alias Expansion (`check/alias.go`)

```go
// expandAlias replaces type alias applications with their definitions.
// type Effect r a = Computation r r a
// Effect R Int → Computation R R Int
func (ch *Checker) expandAlias(t types.Type) types.Type
```

Alias expansion happens during kind checking. Aliases are non-recursive (no mutual recursion).

#### Alias Cycle Detection

Before alias expansion, the checker validates that the alias dependency graph is acyclic. This prevents infinite expansion from definitions like `type A = B; type B = A`.

```go
// validateAliasGraph checks for cycles in type alias definitions.
// Called once during declaration processing (§5.11, fourth pass),
// before any alias expansion occurs.
//
// Algorithm: build a directed graph (alias → referenced aliases),
// then detect cycles via DFS with three-color marking.
func (ch *Checker) validateAliasGraph(aliases []syntax.DeclTypeAlias) error {
    // 1. Build adjacency list: alias name → set of alias names referenced in body.
    graph := make(map[string][]string)
    for _, a := range aliases {
        refs := ch.collectAliasRefs(a.Body, aliases)
        graph[a.Name] = refs
    }

    // 2. DFS with coloring: white (unvisited), gray (in progress), black (done).
    const (
        white = 0
        gray  = 1
        black = 2
    )
    color := make(map[string]int)
    parent := make(map[string]string) // for reconstructing cycle path

    var dfs func(name string) []string // returns cycle path or nil
    dfs = func(name string) []string {
        color[name] = gray
        for _, dep := range graph[name] {
            switch color[dep] {
            case gray:
                // Cycle found — reconstruct path.
                return reconstructCycle(parent, name, dep)
            case white:
                parent[dep] = name
                if cycle := dfs(dep); cycle != nil {
                    return cycle
                }
            }
        }
        color[name] = black
        return nil
    }

    for name := range graph {
        if color[name] == white {
            if cycle := dfs(name); cycle != nil {
                return &CyclicAliasError{Cycle: cycle}
            }
        }
    }
    return nil
}

// CyclicAliasError reports a cycle in type alias definitions.
type CyclicAliasError struct {
    Cycle []string // e.g., ["A", "B", "A"]
}

func (e *CyclicAliasError) Error() string {
    return fmt.Sprintf("cyclic type alias: %s", strings.Join(e.Cycle, " → "))
}
```

### 5.13 Error Recovery (`check/recovery.go`)

When a type error is encountered, insert a poison type that unifies with anything:

```go
// TyError is defined in types/ — a poison type for error recovery.
// Unifying TyError with any type succeeds silently.
// This prevents cascading errors after the first mismatch.
```

The unifier must special-case `TyError`: `Unify(TyError, T) = success` for any `T`. The checker also has a max-errors limit:

```go
type Checker struct {
    // ...
    maxErrors int  // default: 20; stop collecting diagnostics after this count
}
```

### 5.14 Error Reporting (`check/errors.go`)

```go
// Structured type errors.
func (ch *Checker) typeMismatch(expected, actual types.Type, s span.Span)
func (ch *Checker) unboundVar(name string, s span.Span)
func (ch *Checker) unboundTyVar(name string, s span.Span)
func (ch *Checker) kindMismatch(expected, actual types.Kind, s span.Span)
func (ch *Checker) rowLabelMismatch(labels []string, s span.Span)
func (ch *Checker) nonExhaustive(missing []core.Pattern, s span.Span)
func (ch *Checker) duplicateLabel(label string, s span.Span)
func (ch *Checker) occursCheck(meta int, ty types.Type, s span.Span)
func (ch *Checker) missingAnnotation(name string, s span.Span)
func (ch *Checker) assumptionWithoutAnnotation(name string, s span.Span)
```

Each error includes:
- Span of the problematic expression.
- Expected vs actual types (pretty-printed).
- Hints pointing to related locations (e.g., where the expected type came from).

### 5.15 Type Check Tracing (`check/trace.go`)

Optional hook for debugging type checking decisions. Passed via `CheckConfig`.

```go
// CheckTraceKind classifies trace events.
type CheckTraceKind int

const (
    TraceUnify       CheckTraceKind = iota // unification attempt
    TraceSolveMeta                         // metavariable solved
    TraceInfer                             // type inferred for expression
    TraceCheck                             // expression checked against type
    TraceInstantiate                       // forall instantiated with fresh metas
    TraceRowUnify                          // row unification step
)

// CheckTraceEvent describes one type checking decision.
type CheckTraceEvent struct {
    Kind    CheckTraceKind
    Depth   int        // nesting depth (for indentation)
    Message string     // human-readable summary
    Span    span.Span  // source location (zero if internal)
}

// CheckTraceHook receives trace events during type checking.
type CheckTraceHook func(CheckTraceEvent)
```

Example trace output for `id True`:

```
infer: id ⇒ forall a. a -> a                        [1:5]
  instantiate: forall a. a -> a  →  ?1 -> ?1
  check: True ⇐ ?1                                  [1:8]
    infer: True ⇒ Bool
    unify: Bool ~ ?1 → ok
    solve: ?1 := Bool
  result: Bool
```

The hook is passed through `CheckConfig`:

```go
type CheckConfig struct {
    RegisteredTypes map[string]types.Kind
    Assumptions     map[string]types.Type
    Bindings        map[string]types.Type  // host-injected value bindings (from Engine.DeclareBinding)
    GatedBuiltins   map[string]bool
    Trace           CheckTraceHook // optional, nil = no tracing
}
```

The checker injects `Bindings` entries into the initial typing context as `CtxVar` entries before processing declarations. This makes host-declared names available for type checking without requiring them to be defined in the source:

```go
func (ch *Checker) initContext(config *CheckConfig) {
    // Host-declared bindings: available as typed variables.
    for name, ty := range config.Bindings {
        ch.ctx.Push(&CtxVar{Name: name, Type: ty})
    }
    // Built-in identifiers (pure, bind, thunk, force, etc.)
    for name, ty := range builtinTypes {
        if config.GatedBuiltins[name] || !gatedBuiltins[name] {
            ch.ctx.Push(&CtxVar{Name: name, Type: ty})
        }
    }
    // Assumptions.
    for name, ty := range config.Assumptions {
        ch.ctx.Push(&CtxVar{Name: name, Type: ty})
    }
}
```

Trace points in the checker:

| Point | Kind | Message format |
|-------|------|---------------|
| `Unify(A, B)` | `TraceUnify` | `"unify: A ~ B → ok/FAIL"` |
| `solveMeta(?α, T)` | `TraceSolveMeta` | `"solve: ?3 := T"` |
| `infer(expr)` entry | `TraceInfer` | `"infer: expr ⇒ ..."` |
| `check(expr, T)` entry | `TraceCheck` | `"check: expr ⇐ T"` |
| `instantiate(∀a.T)` | `TraceInstantiate` | `"instantiate: ∀a.T → T[a:=?n]"` |
| `unifyRows(r1, r2)` | `TraceRowUnify` | `"row-unify: {labels|tail} ~ {labels|tail} → case"` |

## Test Strategy

### Unit tests per module

- **Context**: push/pop, lookup, marker drop, apply.
- **Unifier**: type unification (simple, arrow, forall), row unification (all 5 cases), occurs check, label uniqueness.
- **Kind checking**: well-kinded types succeed, ill-kinded types fail.
- **Instantiation**: forall types get fresh metas, non-forall types unchanged.
- **Exhaustiveness**: complete patterns pass, missing patterns detected, redundant patterns warned.
- **Alias expansion**: simple alias, parameterized alias, nested alias.
- **Alias cycle detection**: direct cycle (`A = A`), mutual cycle (`A = B, B = A`), transitive cycle (`A = B, B = C, C = A`), no cycle (DAG) passes.

### Integration tests (AST → Core)

Build surface AST nodes programmatically (bypassing the parser) and type-check them:

1. **Identity function**: `id :: forall a. a -> a; id := \x -> x` → infer, elaborate to `Lam`.
2. **Application**: `id True` → infer, instantiation produces `TyApp`.
3. **Constructor**: `Ok 42` → `Con("Ok", [HostVal])`.
4. **do block**: multi-statement do with bind sequencing → correct Bind chain.
5. **Row unification**: open-open, open-closed, closed-closed.
6. **Capability threading**: `dbOpen; dbClose` → rows unify correctly.
7. **case exhaustiveness**: complete coverage passes, missing constructor fails.
8. **Block expression**: `{ x := e; body }` → elaborates to `App(Lam(x, body), e)`.
9. **thunk/force**: correct Core nodes emitted.
10. **assumption**: produces PrimOp Core node, requires annotation.
11. **Type alias**: `Effect r a` expands to `Computation r r a`.

### Full pipeline tests (source text → Core)

Combined with Phase 4 parser:

1. Parse source text → AST.
2. Type-check AST → Core.
3. Verify Core structure matches expected.

### Error message tests

Verify specific error messages for common mistakes:
- Missing type annotation on assumption.
- Mismatched row capability.
- Non-exhaustive case.
- Type mismatch in bind chain.

## Completion Criteria

- [ ] Bidirectional checking (infer + check) handles all expression forms
- [ ] Row unification correct for all 5 cases
- [ ] Label uniqueness enforced
- [ ] Occurs check prevents infinite types
- [ ] Built-in identifiers elaborate to correct Core nodes
- [ ] Kind checking validates all type expressions
- [ ] Exhaustiveness checking rejects incomplete case expressions
- [ ] Type aliases expand correctly
- [ ] Cyclic type aliases detected and reported with cycle path
- [ ] Implicit instantiation works
- [ ] Declaration processing handles all 5 declaration forms
- [ ] assumption declarations produce PrimOp
- [ ] Error messages include spans and hints
- [ ] CheckTraceHook receives events in correct order
- [ ] Trace output is human-readable and includes source locations
- [ ] All tests pass
