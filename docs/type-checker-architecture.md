# Type Checker Implementation Architecture

Bridging the theoretical bidirectional typing rules to a practical Go implementation for the Gomputation embedded typed effect language.

## Table of Contents

1. Overall Checker Architecture
2. Pipeline Phases
3. Go Type Definitions
4. Context Management
5. Unification and Constraint Solving
6. Bidirectional Type Checking Implementation
7. Kind Checking
8. Elaboration
9. Error Handling
10. Testing Strategy
11. Performance Considerations
12. Specific Recommendations for Gomputation

---

## 1. Overall Checker Architecture

### 1.1 Architecture Diagram

```text
                         Source Text
                             |
                             v
                    +------------------+
                    |      Lexer       |   tokens
                    +------------------+
                             |
                             v
                    +------------------+
                    |      Parser      |   surface AST (SurfExpr, SurfType, SurfDecl)
                    +------------------+
                             |
                             v
               +----------------------------+
               |     Name Resolution /      |   resolved AST (ResExpr, ResType, ResDecl)
               |     Desugaring             |   - unique names for all bindings
               +----------------------------+   - do-notation desugared to bind/pure
                             |                  - operator sections resolved
                             v
               +----------------------------+
               |     Kind Checking          |   kind-checked types
               +----------------------------+   - type constructors verified
                             |                  - Row vs Type kinds distinguished
                             v                  - ADT declarations validated
               +----------------------------+
               |     Type Checking          |   elaborated core (CoreExpr)
               |     (Bidirectional)        |   - all type applications explicit
               +----------------------------+   - all row applications explicit
                             |                  - all forall introductions explicit
                             v                  - intermediate bind annotations
               +----------------------------+
               |     Zonking               |   fully resolved core
               +----------------------------+   - no unsolved existentials remain
                             |                  - all metavariables substituted
                             v
               +----------------------------+
               |     Evaluator             |   runtime values
               +----------------------------+
```

### 1.2 Design Decision: Single-Pass Interleaved Architecture

Gomputation should use a **single-pass interleaved** design for type checking, not a separate constraint generation and constraint solving architecture.

**Rationale.** The OutsideIn(X) approach used by GHC separates constraint generation from solving. This separation is justified when the constraint domain is complex (type classes, type families, GADTs with implication constraints) and when modularity between the constraint domain and the inference engine is valuable. Gomputation has none of these: its constraint domain is type equality and row equality, with no type classes, no type families, and no implication constraints.

The DK-style interleaved approach -- where unification fires eagerly during bidirectional traversal -- is simpler, produces better error messages (because the error occurs at the point of mismatch rather than during a later solving phase), and is sufficient for the combination of features in the current spec: rank-1 inference, annotation-guided higher-rank polymorphism, and row unification.

The interleaved approach follows the ordered context discipline of Dunfield and Krishnaswami. The context accumulates knowledge (solved existentials) as checking progresses. Each rule can immediately apply known solutions, avoiding the need to defer constraint processing.

### 1.3 Comparison with Real Systems

**GHC** organizes its pipeline as: Parser -> Renamer -> Type Checker -> Zonker -> Desugarer -> Core Lint. The renamer (`GHC.Rename`) resolves names and performs scope analysis. The type checker (`GHC.Tc`) generates and solves constraints via the OutsideIn(X) framework. The zonker (`GHC.Tc.Zonk`) substitutes solved metavariables into the AST. Core Lint re-checks the elaborated Core for internal consistency. GHC's type checker runs within the `TcM` monad (now unified with `RnM` as `TcRn`), a state monad carrying the global and local type-checking environment.

**PureScript** uses bidirectional checking with skolemization (rigid type variables for universal quantification) rather than DK-style ordered contexts. When checking against `forall a. A`, PureScript replaces `a` with a fresh skolem constant; if the skolem escapes its scope, a type error is reported. PureScript's row unification follows Leijen's scoped labels pattern, with the modification that rows may have duplicate labels. PureScript separates desugaring, type checking, and code generation into distinct compiler passes, each operating on different AST representations.

**Elm** uses a simpler architecture: parsing, canonicalization (name resolution + desugaring), type inference via Algorithm W with union-find, and code generation. Elm does not support higher-rank polymorphism, so its inference engine is a straightforward HM implementation. The simplicity of Elm's approach is instructive as a lower bound on complexity.

**Koka** uses HM inference extended with row-polymorphic effect types. Row variables are inferred automatically in rank-1 positions. Koka's row unification uses Leijen's scoped labels algorithm with duplicate labels allowed. This is simpler than Gomputation's unique-label design but demonstrates that row inference integrates smoothly with HM.

Gomputation sits between PureScript (bidirectional + rows + higher-rank) and Elm (simple HM) in complexity. The recommended architecture draws on DK's ordered context for scoping, Leijen's flatten-then-diff for row unification, and PureScript's practical experience with the interaction between rows and higher-rank types.

---

## 2. Pipeline Phases

### 2.1 Phase 1: Name Resolution and Desugaring

**Input:** Surface AST from the parser.

**Output:** Resolved AST with unique identifiers and desugared syntax.

**Responsibilities:**

1. **Unique name generation.** Every binding site (lambda parameter, let binding, top-level definition, data constructor, type variable) receives a unique internal name. This eliminates shadowing ambiguity in later phases.

2. **Scope analysis.** Verify that every variable reference has a corresponding binding. Report "not in scope" errors with suggestions for typos.

3. **Do-notation desugaring.** Convert `do { x <- c1; c2 }` to `bind c1 (\x -> c2)` and `do { c1; c2 }` to `bind c1 (\_ -> c2)`. This must happen before type checking so the checker sees only `bind`, `pure`, lambdas, and applications.

4. **Operator desugaring.** Convert infix applications `a + b` to `(+) a b`. Resolve operator sections. Apply precedence and associativity from `infixl`/`infixr`/`infix` declarations (this is already handled by the Pratt parser, but sections and partially applied operators need desugaring).

5. **Source span preservation.** Attach source spans from the original surface syntax to desugared nodes. When `do { x <- c1; c2 }` becomes `bind c1 (\x -> c2)`, the `bind` node carries the span of the `<-` token, and the lambda node carries the span of `c2`. This enables error messages to refer to the surface syntax.

**Go representation:**

```go
package resolve

// Name is a unique identifier assigned during resolution.
type Name struct {
    Text     string       // original user-visible text
    Unique   int          // globally unique ID
    SrcSpan  Span         // where the binding was introduced
}

// Span locates a range in the source text.
type Span struct {
    File  string
    Start Position
    End   Position
}

type Position struct {
    Line   int
    Col    int
    Offset int
}

// ResExpr is the resolved expression AST.
type ResExpr interface {
    resExpr()
    GetSpan() Span
}

type ResVar struct {
    Span Span
    Name Name
}

type ResApp struct {
    Span Span
    Fun  ResExpr
    Arg  ResExpr
}

type ResLam struct {
    Span  Span
    Param Name
    Body  ResExpr
}

type ResLet struct {
    Span  Span
    Name  Name
    Annot *ResType // nil if no annotation
    Def   ResExpr
    Body  ResExpr
}

type ResAnn struct {
    Span Span
    Expr ResExpr
    Type ResType
}

type ResPure struct {
    Span Span
    Expr ResExpr
}

type ResBind struct {
    Span  Span
    Comp  ResExpr
    Cont  ResExpr // always a ResLam after desugaring
}

type ResCase struct {
    Span       Span
    Scrutinee  ResExpr
    Alts       []ResAlt
}

type ResAlt struct {
    Span    Span
    Pattern ResPattern
    Body    ResExpr
}

type ResCon struct {
    Span Span
    Name Name
    Args []ResExpr
}

type ResLit struct {
    Span Span
    Lit  Literal // IntLit, StringLit, etc.
}
```

### 2.2 Phase 2: Kind Checking

**Input:** Resolved type expressions and data declarations.

**Output:** Kind-annotated types and validated data declarations.

**Responsibilities:**

1. Verify that `Computation R1 R2 A` has arguments of kinds `Row`, `Row`, `Type`.
2. Verify that type variables have consistent kinds across their scope.
3. Verify that `forall a. T` and `forall r. T` bind variables of the correct kind.
4. Validate ADT declarations: each constructor's field types must be well-kinded.
5. Check that row tails have kind `Row` and row field types have kind `Type`.

**Design choice: integrated with type checking.** Kind checking runs as a preliminary pass over type-level syntax within the type checker, not as a fully separate compiler phase. When the type checker encounters a type annotation or a `forall`, it invokes the kind checker on the type. This avoids duplicating context management.

The kind language is small and closed:

```text
Kind ::= Type | Row | Kind -> Kind
```

`Kind -> Kind` is included prospectively for type constructors like `DB : Type -> Type`, but in the current spec, all type constructors are fully applied and kinding reduces to checking that arguments match expected arities.

### 2.3 Phase 3: Type Checking (Bidirectional)

**Input:** Resolved AST, kind-checked type annotations, host-registered primitives.

**Output:** Elaborated core AST with all type information explicit.

This is the central phase. It implements the DK-style bidirectional algorithm extended with row unification. The details are given in Sections 4--6 below.

### 2.4 Phase 4: Zonking

**Input:** Elaborated core AST with possibly-unsolved existential variables.

**Output:** Fully resolved core AST with no metavariables.

Zonking walks the elaborated core and replaces every existential variable with its solution from the context. If an existential has no solution, it is either:

- **Generalized** (at a `let` binding or top-level definition): replaced by a fresh universal variable under a `forall`.
- **Defaulted** (if a defaulting strategy exists): replaced by a default type.
- **An error**: reported as an ambiguous type.

Zonking is idempotent: zonking a fully zonked term has no effect. Zonking is applied lazily during type checking (when applying the context as a substitution) and eagerly as a final pass after checking completes.

```go
// Zonk replaces all solved existentials in a type with their solutions.
func (c *Checker) Zonk(t Type) Type {
    switch v := t.(type) {
    case *ExVar:
        if sol, ok := c.ctx.Solution(v.ID); ok {
            return c.Zonk(sol) // chase transitive solutions
        }
        return t // unsolved: leave as-is (will be generalized or reported)
    case *Arrow:
        return &Arrow{
            Param:  c.Zonk(v.Param),
            Result: c.Zonk(v.Result),
            Span:   v.Span,
        }
    case *ForAll:
        return &ForAll{
            Var:  v.Var,
            Kind: v.Kind,
            Body: c.Zonk(v.Body),
            Span: v.Span,
        }
    case *Computation:
        return &Computation{
            Pre:    c.ZonkRow(v.Pre),
            Post:   c.ZonkRow(v.Post),
            Result: c.Zonk(v.Result),
            Span:   v.Span,
        }
    case *TyCon:
        args := make([]Type, len(v.Args))
        for i, a := range v.Args {
            args[i] = c.Zonk(a)
        }
        return &TyCon{Name: v.Name, Args: args, Span: v.Span}
    default:
        return t
    }
}
```

---

## 3. Go Type Definitions

### 3.1 Types and Rows

The internal type representation is used by the checker and unifier. It is distinct from the surface type syntax and from the elaborated core types.

```go
package checker

// ---------- Types ----------

// Type is the internal representation of types during checking.
type Type interface {
    typeNode()
    GetSpan() Span
}

// UniVar is a universal type variable (introduced by forall, immutable).
type UniVar struct {
    Span Span
    ID   int
    Name string
    Kind Kind
}

// ExVar is an existential type variable (introduced during inference, may be solved).
type ExVar struct {
    Span Span
    ID   int
    Kind Kind
}

// Arrow is a function type A -> B.
type Arrow struct {
    Span   Span
    Param  Type
    Result Type
}

// ForAll is a universally quantified type: forall a. T or forall r. T.
type ForAll struct {
    Span Span
    Var  int    // unique ID of the bound variable
    Name string // user-visible name
    Kind Kind   // Type or Row
    Body Type
}

// Computation is the indexed computation type: Computation pre post a.
type Computation struct {
    Span   Span
    Pre    Row
    Post   Row
    Result Type
}

// TyCon is a named type constructor applied to arguments: DB[Opened], Option a.
type TyCon struct {
    Span Span
    Name string
    Args []Type
}

// ---------- Rows ----------

// Row is the internal representation of row types.
type Row interface {
    rowNode()
    GetSpan() Span
}

// RowEmpty is the empty row {}.
type RowEmpty struct {
    Span Span
}

// RowExtend is a row extension { l : T | R }.
type RowExtend struct {
    Span  Span
    Label string
    Type  Type
    Tail  Row
}

// RowUniVar is a universal row variable (introduced by forall r).
type RowUniVar struct {
    Span Span
    ID   int
    Name string
}

// RowExVar is an existential row variable (introduced during inference).
type RowExVar struct {
    Span Span
    ID   int
}

// ---------- Kinds ----------

// Kind classifies types and rows.
type Kind int

const (
    KindType Kind = iota // classifies value types
    KindRow              // classifies capability rows
)
```

### 3.2 Flattened Row (Canonical Form)

```go
// FlatRow is the canonical, order-independent representation of a row.
// Used internally by unification; not part of the persistent type AST.
type FlatRow struct {
    Fields []LabeledType // sorted by label for determinism
    Tail   *RowExVar     // nil for closed rows
}

type LabeledType struct {
    Label string
    Type  Type
}

// Lookup finds a label in the sorted fields. Returns the type and true,
// or nil and false.
func (fr *FlatRow) Lookup(label string) (Type, bool) {
    i := sort.Search(len(fr.Fields), func(i int) bool {
        return fr.Fields[i].Label >= label
    })
    if i < len(fr.Fields) && fr.Fields[i].Label == label {
        return fr.Fields[i].Type, true
    }
    return nil, false
}
```

### 3.3 Elaborated Core

The output of type checking. Every type application, row application, and quantifier introduction is explicit. This core can be type-checked without inference as a sanity check.

```go
package core

// CoreExpr is the elaborated core expression.
type CoreExpr interface {
    coreExpr()
    GetSpan() Span
    GetType() Type
}

// CoreVar: variable reference with its type.
type CoreVar struct {
    Span Span
    Name Name
    Type Type
}

// CoreApp: term-level application f x.
type CoreApp struct {
    Span Span
    Fun  CoreExpr
    Arg  CoreExpr
    Type Type
}

// CoreLam: term-level lambda \x -> e with parameter type annotation.
type CoreLam struct {
    Span      Span
    Param     Name
    ParamType Type
    Body      CoreExpr
    Type      Type
}

// CoreTyApp: explicit type application e @T.
type CoreTyApp struct {
    Span    Span
    Expr    CoreExpr
    TyArg   Type
    Type    Type
}

// CoreRowApp: explicit row application e @R.
type CoreRowApp struct {
    Span    Span
    Expr    CoreExpr
    RowArg  Row
    Type    Type
}

// CoreTyLam: explicit type abstraction /\a -> e.
type CoreTyLam struct {
    Span     Span
    TyParam  int    // unique ID
    TyName   string // user-visible name
    Kind     Kind
    Body     CoreExpr
    Type     Type
}

// CorePure: pure e with explicit pre=post row.
type CorePure struct {
    Span   Span
    Row    Row
    Expr   CoreExpr
    Type   Type
}

// CoreBind: bind c1 (\x -> c2) with explicit intermediate row.
type CoreBind struct {
    Span     Span
    PreRow   Row
    MidRow   Row
    PostRow  Row
    ElemType Type
    ResType  Type
    Comp     CoreExpr
    Cont     CoreExpr // always a CoreLam
    Type     Type
}

// CoreCase: case e of alts with scrutinee type and result type.
type CoreCase struct {
    Span       Span
    Scrutinee  CoreExpr
    ScrutType  Type
    Alts       []CoreAlt
    Type       Type
}

type CoreAlt struct {
    Span    Span
    Pattern CorePattern
    Body    CoreExpr
}

// CoreLet: let x = e1 in e2 with type scheme for x.
type CoreLet struct {
    Span     Span
    Name     Name
    NameType Type
    Def      CoreExpr
    Body     CoreExpr
    Type     Type
}

// CoreLit: literal with its type.
type CoreLit struct {
    Span Span
    Lit  Literal
    Type Type
}

// CoreCon: data constructor application.
type CoreCon struct {
    Span   Span
    Name   Name
    TyArgs []Type
    Args   []CoreExpr
    Type   Type
}
```

### 3.4 Checker State

```go
package checker

// Checker is the main type checker struct. It holds mutable state
// for the checking session: the ordered context, the unifier,
// fresh variable counters, and accumulated errors.
type Checker struct {
    // Ordered context (DK-style)
    ctx *Context

    // Unification engine
    unifier *Unifier

    // Fresh variable counter (shared between types and rows)
    nextID int

    // Host-registered primitives: name -> type scheme
    primitives map[string]Type

    // ADT declarations: constructor name -> constructor info
    constructors map[string]*ConstructorInfo

    // Type constructor declarations: name -> kind
    typeConstructors map[string]Kind

    // Accumulated errors
    errors []CheckError

    // Configuration
    maxErrors int // stop after this many errors (0 = unlimited)
}

type ConstructorInfo struct {
    TypeName   string
    TyParams   []int    // type parameter IDs
    FieldTypes []Type   // types of constructor fields
    ResultType Type     // the ADT type with parameters applied
}

// NewChecker creates a checker with the given host primitives and ADT declarations.
func NewChecker(prims map[string]Type, adts []ADTDecl) *Checker {
    c := &Checker{
        ctx:              NewContext(),
        unifier:          NewUnifier(),
        nextID:           0,
        primitives:       prims,
        constructors:     make(map[string]*ConstructorInfo),
        typeConstructors: make(map[string]Kind),
        errors:           nil,
        maxErrors:        50,
    }
    for _, adt := range adts {
        c.registerADT(adt)
    }
    return c
}

// FreshExVar creates a fresh existential type variable.
func (c *Checker) FreshExVar(kind Kind, span Span) *ExVar {
    id := c.nextID
    c.nextID++
    ev := &ExVar{Span: span, ID: id, Kind: kind}
    c.ctx.InsertExVar(ev)
    return ev
}

// FreshRowExVar creates a fresh existential row variable.
func (c *Checker) FreshRowExVar(span Span) *RowExVar {
    id := c.nextID
    c.nextID++
    rv := &RowExVar{Span: span, ID: id}
    c.ctx.InsertExRow(rv)
    return rv
}
```

---

## 4. Context Management

### 4.1 The Ordered Context

The ordered context is the central data structure of the DK algorithm. It is a sequence of entries where position encodes scope: an entry can only refer to entries to its left.

```go
package checker

// CtxEntry is one entry in the ordered context.
type CtxEntry interface {
    ctxEntry()
}

// CtxTermVar: a term variable x : A.
type CtxTermVar struct {
    Name Name
    Type Type
}

// CtxUniVar: a universal type variable (introduced by forall).
type CtxUniVar struct {
    ID   int
    Name string
    Kind Kind
}

// CtxExVar: an unsolved existential type variable.
type CtxExVar struct {
    ID   int
    Kind Kind
}

// CtxExVarSolved: a solved existential type variable.
type CtxExVarSolved struct {
    ID       int
    Kind     Kind
    Solution Type
}

// CtxExRow: an unsolved existential row variable.
type CtxExRow struct {
    ID int
}

// CtxExRowSolved: a solved existential row variable.
type CtxExRowSolved struct {
    ID       int
    Solution Row
}

// CtxMarker: a scope marker for scoping out existentials.
type CtxMarker struct {
    ID int // same ID as the existential it was introduced with
}

// Context is the ordered context.
type Context struct {
    entries []CtxEntry
}

func NewContext() *Context {
    return &Context{entries: make([]CtxEntry, 0, 64)}
}
```

### 4.2 Context Operations

The ordered context supports five fundamental operations. Each must preserve the ordering invariant.

```go
// Extend appends an entry to the right end of the context.
func (ctx *Context) Extend(entry CtxEntry) {
    ctx.entries = append(ctx.entries, entry)
}

// LookupTermVar finds the type of a term variable by name.
func (ctx *Context) LookupTermVar(name Name) (Type, bool) {
    // Search right-to-left (most recent binding wins for shadowing).
    for i := len(ctx.entries) - 1; i >= 0; i-- {
        if tv, ok := ctx.entries[i].(*CtxTermVar); ok && tv.Name.Unique == name.Unique {
            return tv.Type, true
        }
    }
    return nil, false
}

// Solution returns the solution for an existential type variable, if solved.
func (ctx *Context) Solution(id int) (Type, bool) {
    for i := len(ctx.entries) - 1; i >= 0; i-- {
        if sv, ok := ctx.entries[i].(*CtxExVarSolved); ok && sv.ID == id {
            return sv.Solution, true
        }
    }
    return nil, false
}

// RowSolution returns the solution for an existential row variable, if solved.
func (ctx *Context) RowSolution(id int) (Row, bool) {
    for i := len(ctx.entries) - 1; i >= 0; i-- {
        if sv, ok := ctx.entries[i].(*CtxExRowSolved); ok && sv.ID == id {
            return sv.Solution, true
        }
    }
    return nil, false
}

// SolveExVar replaces an unsolved existential with a solved one.
// Precondition: the solution mentions only variables to the left of id.
func (ctx *Context) SolveExVar(id int, solution Type) error {
    for i, e := range ctx.entries {
        if ev, ok := e.(*CtxExVar); ok && ev.ID == id {
            ctx.entries[i] = &CtxExVarSolved{
                ID:       id,
                Kind:     ev.Kind,
                Solution: solution,
            }
            return nil
        }
    }
    return fmt.Errorf("existential %d not found in context", id)
}

// SolveExRow replaces an unsolved existential row variable with a solved one.
func (ctx *Context) SolveExRow(id int, solution Row) error {
    for i, e := range ctx.entries {
        if ev, ok := e.(*CtxExRow); ok && ev.ID == id {
            ctx.entries[i] = &CtxExRowSolved{
                ID:       id,
                Solution: solution,
            }
            return nil
        }
    }
    return fmt.Errorf("existential row %d not found in context", id)
}

// DropAfter removes all entries after (and including) the entry
// with the given marker ID. Used when scoping out of a forall.
func (ctx *Context) DropAfter(markerID int) {
    for i, e := range ctx.entries {
        if m, ok := e.(*CtxMarker); ok && m.ID == markerID {
            ctx.entries = ctx.entries[:i]
            return
        }
    }
}

// IsToLeftOf checks whether variable with ID 'left' appears before
// variable with ID 'right' in the context. Used during instantiation
// to enforce the scoping discipline.
func (ctx *Context) IsToLeftOf(left, right int) bool {
    leftIdx, rightIdx := -1, -1
    for i, e := range ctx.entries {
        switch v := e.(type) {
        case *CtxExVar:
            if v.ID == left { leftIdx = i }
            if v.ID == right { rightIdx = i }
        case *CtxExVarSolved:
            if v.ID == left { leftIdx = i }
            if v.ID == right { rightIdx = i }
        case *CtxExRow:
            if v.ID == left { leftIdx = i }
            if v.ID == right { rightIdx = i }
        case *CtxExRowSolved:
            if v.ID == left { leftIdx = i }
            if v.ID == right { rightIdx = i }
        }
    }
    return leftIdx >= 0 && rightIdx >= 0 && leftIdx < rightIdx
}

// InsertBefore inserts new entries immediately before the entry with
// the given existential ID. Used by InstLArr / InstRArr to split
// an existential into sub-existentials.
func (ctx *Context) InsertBefore(targetID int, newEntries ...CtxEntry) {
    for i, e := range ctx.entries {
        var entryID int
        switch v := e.(type) {
        case *CtxExVar:
            entryID = v.ID
        case *CtxExRow:
            entryID = v.ID
        default:
            continue
        }
        if entryID == targetID {
            // Insert newEntries before index i.
            result := make([]CtxEntry, 0, len(ctx.entries)+len(newEntries))
            result = append(result, ctx.entries[:i]...)
            result = append(result, newEntries...)
            result = append(result, ctx.entries[i:]...)
            ctx.entries = result
            return
        }
    }
}
```

### 4.3 The Scoping Discipline

The ordered context enforces a critical invariant: **an existential variable can only be solved to a type that mentions variables to its left in the context.** This prevents solutions from escaping their scope.

Concretely, when `forall alpha. A <: B` is checked:

1. A scope marker `|>` and a fresh existential `a^` are pushed.
2. `[a^/alpha]A <: B` is checked, possibly solving `a^`.
3. The marker, `a^`, and everything after them are dropped.

If `a^` was solved to a type mentioning variables introduced after it, the solution would reference variables that no longer exist after dropping -- but the scoping check at solve time prevents this.

Row existentials participate in the same ordered context. When `forall r. A` is instantiated, a row existential `r^` is pushed. It obeys the same left-of scoping rule. When `r^` is solved by row unification, the solution must mention only labels (which are global constants, always in scope) and type/row variables to the left of `r^`.

### 4.4 Context Application

Applying the context as a substitution replaces solved existentials with their solutions:

```go
// Apply replaces all solved existentials in a type with their solutions.
// This is the "apply context" operation from DK.
func (ctx *Context) Apply(t Type) Type {
    switch v := t.(type) {
    case *ExVar:
        if sol, ok := ctx.Solution(v.ID); ok {
            return ctx.Apply(sol) // chase transitive solutions
        }
        return t
    case *Arrow:
        return &Arrow{
            Span:   v.Span,
            Param:  ctx.Apply(v.Param),
            Result: ctx.Apply(v.Result),
        }
    case *ForAll:
        return &ForAll{
            Span: v.Span,
            Var:  v.Var,
            Name: v.Name,
            Kind: v.Kind,
            Body: ctx.Apply(v.Body),
        }
    case *Computation:
        return &Computation{
            Span:   v.Span,
            Pre:    ctx.ApplyRow(v.Pre),
            Post:   ctx.ApplyRow(v.Post),
            Result: ctx.Apply(v.Result),
        }
    case *TyCon:
        args := make([]Type, len(v.Args))
        for i, a := range v.Args {
            args[i] = ctx.Apply(a)
        }
        return &TyCon{Name: v.Name, Args: args, Span: v.Span}
    default:
        return t
    }
}

// ApplyRow replaces all solved existential row variables with their solutions.
func (ctx *Context) ApplyRow(r Row) Row {
    switch v := r.(type) {
    case *RowExVar:
        if sol, ok := ctx.RowSolution(v.ID); ok {
            return ctx.ApplyRow(sol)
        }
        return r
    case *RowExtend:
        return &RowExtend{
            Span:  v.Span,
            Label: v.Label,
            Type:  ctx.Apply(v.Type),
            Tail:  ctx.ApplyRow(v.Tail),
        }
    default:
        return r
    }
}
```

---

## 5. Unification and Constraint Solving

### 5.1 Union-Find for Type Variables

The classic Tarjan union-find with path compression and union by rank provides near-linear-time equivalence management for type variables.

For the DK-style ordered context, union-find is used primarily for row variables (where the flatten-then-diff algorithm requires finding representative variables). Type existentials in the ordered context are solved by direct replacement rather than union, because the ordered context needs to track exactly which existential was solved and what its solution is.

However, union-find is still useful for an optimization: when two unsolved existentials are unified (both have no solution), one can be pointed to the other via union-find rather than introducing a solved entry. This reduces context growth.

```go
package checker

// Unifier handles type and row unification.
type Unifier struct {
    // Union-find for row variables (path compression + union by rank)
    rowParent map[int]int
    rowRank   map[int]int

    // Reference to the checker's context for solving existentials
    ctx *Context

    // Fresh variable counter (shared with checker)
    freshID *int
}

func NewUnifier(ctx *Context, freshID *int) *Unifier {
    return &Unifier{
        rowParent: make(map[int]int),
        rowRank:   make(map[int]int),
        ctx:       ctx,
        freshID:   freshID,
    }
}

// FindRow returns the representative of a row variable's equivalence class.
func (u *Unifier) FindRow(id int) int {
    // Initialize if not yet seen.
    if _, ok := u.rowParent[id]; !ok {
        u.rowParent[id] = id
        u.rowRank[id] = 0
    }
    // Path compression.
    root := id
    for u.rowParent[root] != root {
        root = u.rowParent[root]
    }
    // Compress path.
    current := id
    for current != root {
        next := u.rowParent[current]
        u.rowParent[current] = root
        current = next
    }
    return root
}

// UnionRow merges two row variable equivalence classes.
func (u *Unifier) UnionRow(a, b int) {
    ra, rb := u.FindRow(a), u.FindRow(b)
    if ra == rb {
        return
    }
    // Union by rank.
    if u.rowRank[ra] < u.rowRank[rb] {
        u.rowParent[ra] = rb
    } else if u.rowRank[ra] > u.rowRank[rb] {
        u.rowParent[rb] = ra
    } else {
        u.rowParent[rb] = ra
        u.rowRank[ra]++
    }
}

// FreshRowVar creates a fresh row existential variable.
func (u *Unifier) FreshRowVar(span Span) *RowExVar {
    id := *u.freshID
    *u.freshID++
    rv := &RowExVar{Span: span, ID: id}
    u.ctx.InsertExRow(&CtxExRow{ID: id})
    return rv
}
```

### 5.2 Type Unification

```go
// Unify unifies two types, solving existential variables as needed.
// Returns an error if the types cannot be unified.
func (u *Unifier) Unify(t1, t2 Type) error {
    t1 = u.ctx.Apply(t1)
    t2 = u.ctx.Apply(t2)

    // Both existentials: solve one to the other.
    if e1, ok := t1.(*ExVar); ok {
        if e2, ok := t2.(*ExVar); ok {
            if e1.ID == e2.ID {
                return nil
            }
            // Solve the one that appears later in the context to the earlier one.
            if u.ctx.IsToLeftOf(e1.ID, e2.ID) {
                return u.ctx.SolveExVar(e2.ID, t1)
            }
            return u.ctx.SolveExVar(e1.ID, t2)
        }
        return u.solveExVarTo(e1, t2)
    }
    if e2, ok := t2.(*ExVar); ok {
        return u.solveExVarTo(e2, t1)
    }

    // Structural decomposition.
    switch a := t1.(type) {
    case *UniVar:
        if b, ok := t2.(*UniVar); ok && a.ID == b.ID {
            return nil
        }
        return &UnificationError{
            Expected: t1,
            Got:      t2,
            Message:  "rigid type variable mismatch",
        }

    case *Arrow:
        b, ok := t2.(*Arrow)
        if !ok {
            return &UnificationError{Expected: t1, Got: t2}
        }
        if err := u.Unify(a.Param, b.Param); err != nil {
            return fmt.Errorf("in function parameter: %w", err)
        }
        return u.Unify(u.ctx.Apply(a.Result), u.ctx.Apply(b.Result))

    case *Computation:
        b, ok := t2.(*Computation)
        if !ok {
            return &UnificationError{Expected: t1, Got: t2}
        }
        if err := u.UnifyRows(a.Pre, b.Pre); err != nil {
            return fmt.Errorf("in pre-row: %w", err)
        }
        if err := u.UnifyRows(
            u.ctx.ApplyRow(a.Post),
            u.ctx.ApplyRow(b.Post),
        ); err != nil {
            return fmt.Errorf("in post-row: %w", err)
        }
        return u.Unify(u.ctx.Apply(a.Result), u.ctx.Apply(b.Result))

    case *TyCon:
        b, ok := t2.(*TyCon)
        if !ok || a.Name != b.Name || len(a.Args) != len(b.Args) {
            return &UnificationError{Expected: t1, Got: t2}
        }
        for i := range a.Args {
            if err := u.Unify(
                u.ctx.Apply(a.Args[i]),
                u.ctx.Apply(b.Args[i]),
            ); err != nil {
                return fmt.Errorf("in type argument %d of %s: %w", i, a.Name, err)
            }
        }
        return nil

    default:
        return &UnificationError{Expected: t1, Got: t2}
    }
}

// solveExVarTo solves an existential to a given type after occurs check
// and scope check.
func (u *Unifier) solveExVarTo(ev *ExVar, t Type) error {
    if u.occursIn(ev.ID, t) {
        return &UnificationError{
            Expected: ev,
            Got:      t,
            Message:  "infinite type (occurs check)",
        }
    }
    // Scope check: all free variables in t must be to the left of ev in the context.
    if err := u.scopeCheck(ev.ID, t); err != nil {
        return err
    }
    return u.ctx.SolveExVar(ev.ID, t)
}

// occursIn checks whether an existential variable occurs in a type.
func (u *Unifier) occursIn(id int, t Type) bool {
    t = u.ctx.Apply(t)
    switch v := t.(type) {
    case *ExVar:
        return v.ID == id
    case *Arrow:
        return u.occursIn(id, v.Param) || u.occursIn(id, v.Result)
    case *Computation:
        return u.occursInRow(id, v.Pre) ||
            u.occursInRow(id, v.Post) ||
            u.occursIn(id, v.Result)
    case *TyCon:
        for _, arg := range v.Args {
            if u.occursIn(id, arg) {
                return true
            }
        }
        return false
    case *ForAll:
        return u.occursIn(id, v.Body)
    default:
        return false
    }
}
```

### 5.3 Row Unification (Flatten-Then-Diff)

This is the five-case algorithm from the row unification research document, expressed as Go methods on the unifier.

```go
// UnifyRows unifies two rows using the flatten-then-diff algorithm.
func (u *Unifier) UnifyRows(r1, r2 Row) error {
    flat1, err := u.Flatten(r1)
    if err != nil {
        return err
    }
    flat2, err := u.Flatten(r2)
    if err != nil {
        return err
    }

    // Compute label partitions.
    shared, onlyIn1, onlyIn2 := partitionLabels(flat1, flat2)

    // Unify types at shared labels.
    for _, l := range shared {
        t1, _ := flat1.Lookup(l)
        t2, _ := flat2.Lookup(l)
        if err := u.Unify(u.ctx.Apply(t1), u.ctx.Apply(t2)); err != nil {
            return fmt.Errorf("at label %q: %w", l, err)
        }
    }

    // Dispatch on tail shapes.
    switch {
    case flat1.Tail == nil && flat2.Tail == nil:
        // Case 1: both closed.
        if len(onlyIn1) > 0 || len(onlyIn2) > 0 {
            return &RowMismatchError{
                ExtraLeft:  onlyIn1,
                ExtraRight: onlyIn2,
            }
        }
        return nil

    case flat1.Tail != nil && flat2.Tail == nil:
        // Case 2: open ~ closed.
        if len(onlyIn1) > 0 {
            return &RowMismatchError{
                Message: "open row has labels not present in closed row",
                ExtraLeft: onlyIn1,
            }
        }
        return u.solveTailToClosed(flat1.Tail, flat2, onlyIn2)

    case flat1.Tail == nil && flat2.Tail != nil:
        // Case 3: closed ~ open (symmetric to Case 2).
        if len(onlyIn2) > 0 {
            return &RowMismatchError{
                Message: "open row has labels not present in closed row",
                ExtraRight: onlyIn2,
            }
        }
        return u.solveTailToClosed(flat2.Tail, flat1, onlyIn1)

    default:
        // Case 4: open ~ open.
        return u.unifyOpenOpen(flat1, flat2, onlyIn1, onlyIn2)
    }
}

// solveTailToClosed solves a tail variable to a closed row
// consisting of the given extra labels.
func (u *Unifier) solveTailToClosed(
    tail *RowExVar,
    source *FlatRow,
    extraLabels []string,
) error {
    var solution Row = &RowEmpty{}
    for _, l := range extraLabels {
        t, _ := source.Lookup(l)
        solution = &RowExtend{Label: l, Type: t, Tail: solution}
    }
    if u.occursInRow(tail.ID, solution) {
        return fmt.Errorf("infinite row type")
    }
    return u.ctx.SolveExRow(tail.ID, solution)
}

// unifyOpenOpen handles the case where both rows have tail variables.
func (u *Unifier) unifyOpenOpen(
    flat1, flat2 *FlatRow,
    onlyIn1, onlyIn2 []string,
) error {
    r1 := u.FindRow(flat1.Tail.ID)
    r2 := u.FindRow(flat2.Tail.ID)

    // Same variable, no extra fields: nothing to do.
    if r1 == r2 && len(onlyIn1) == 0 && len(onlyIn2) == 0 {
        return nil
    }

    // Create fresh row variable for the common unknown remainder.
    rFresh := u.FreshRowVar(flat1.Tail.Span)

    // Build solution for r1: fields only in row2 + fresh tail.
    var sol1 Row = rFresh
    for _, l := range onlyIn2 {
        t, _ := flat2.Lookup(l)
        sol1 = &RowExtend{Label: l, Type: t, Tail: sol1}
    }

    // Build solution for r2: fields only in row1 + fresh tail.
    var sol2 Row = rFresh
    for _, l := range onlyIn1 {
        t, _ := flat1.Lookup(l)
        sol2 = &RowExtend{Label: l, Type: t, Tail: sol2}
    }

    // Occurs checks.
    if u.occursInRow(r1, sol1) {
        return fmt.Errorf("infinite row type")
    }
    if u.occursInRow(r2, sol2) {
        return fmt.Errorf("infinite row type")
    }

    // Solve.
    if err := u.ctx.SolveExRow(r1, sol1); err != nil {
        return err
    }
    return u.ctx.SolveExRow(r2, sol2)
}

// Flatten resolves a row to its canonical flattened form.
func (u *Unifier) Flatten(r Row) (*FlatRow, error) {
    var fields []LabeledType
    current := r
    for {
        switch v := current.(type) {
        case *RowEmpty:
            sort.Slice(fields, func(i, j int) bool {
                return fields[i].Label < fields[j].Label
            })
            return &FlatRow{Fields: fields, Tail: nil}, nil

        case *RowExVar:
            // Follow solutions.
            if sol, ok := u.ctx.RowSolution(v.ID); ok {
                current = sol
                continue
            }
            sort.Slice(fields, func(i, j int) bool {
                return fields[i].Label < fields[j].Label
            })
            return &FlatRow{Fields: fields, Tail: v}, nil

        case *RowUniVar:
            // Universal row variable: treated as an opaque tail.
            sort.Slice(fields, func(i, j int) bool {
                return fields[i].Label < fields[j].Label
            })
            // Convert to RowExVar-like representation for uniformity.
            // Universal row vars cannot be solved, so this acts as a closed tail.
            return &FlatRow{Fields: fields, Tail: nil}, nil

        case *RowExtend:
            // Check for duplicate labels.
            for _, f := range fields {
                if f.Label == v.Label {
                    return nil, fmt.Errorf("duplicate label %q in row", v.Label)
                }
            }
            fields = append(fields, LabeledType{
                Label: v.Label,
                Type:  u.ctx.Apply(v.Type),
            })
            current = v.Tail

        default:
            return nil, fmt.Errorf("unexpected row node: %T", v)
        }
    }
}

// partitionLabels computes shared, only-in-left, only-in-right label sets.
func partitionLabels(flat1, flat2 *FlatRow) (shared, onlyIn1, onlyIn2 []string) {
    set2 := make(map[string]bool, len(flat2.Fields))
    for _, f := range flat2.Fields {
        set2[f.Label] = true
    }
    for _, f := range flat1.Fields {
        if set2[f.Label] {
            shared = append(shared, f.Label)
        } else {
            onlyIn1 = append(onlyIn1, f.Label)
        }
    }
    set1 := make(map[string]bool, len(flat1.Fields))
    for _, f := range flat1.Fields {
        set1[f.Label] = true
    }
    for _, f := range flat2.Fields {
        if !set1[f.Label] {
            onlyIn2 = append(onlyIn2, f.Label)
        }
    }
    sort.Strings(shared)
    sort.Strings(onlyIn1)
    sort.Strings(onlyIn2)
    return
}
```

### 5.4 Occurs Check for Rows

```go
// occursInRow checks whether an existential (type or row) occurs in a row.
func (u *Unifier) occursInRow(id int, r Row) bool {
    r = u.ctx.ApplyRow(r)
    switch v := r.(type) {
    case *RowExVar:
        return u.FindRow(v.ID) == id
    case *RowExtend:
        return u.occursIn(id, v.Type) || u.occursInRow(id, v.Tail)
    default:
        return false
    }
}
```

---

## 6. Bidirectional Type Checking Implementation

### 6.1 The Three Core Functions

The type checker implements three mutually recursive function families:

```
check(ctx, expr, expectedType) -> (ctx', coreExpr)
infer(ctx, expr)               -> (ctx', inferredType, coreExpr)
subtype(ctx, typeA, typeB)     -> ctx'
```

Plus auxiliary functions:

```
instantiateL(ctx, exVar, type) -> ctx'   (solve exVar to be a subtype of type)
instantiateR(ctx, type, exVar) -> ctx'   (solve exVar to be a supertype of type)
appInfer(ctx, funType, argExpr) -> (ctx', resultType, coreExpr)
```

### 6.2 Checking Mode

```go
// Check checks an expression against an expected type.
// Returns the elaborated core expression.
func (c *Checker) Check(expr ResExpr, expected Type) CoreExpr {
    expected = c.ctx.Apply(expected)

    switch e := expr.(type) {

    // --- ForallI: check against forall a. A ---
    // When checking against a universally quantified type, introduce the
    // universal variable and check the body.
    case forAllType:
        if fa, ok := expected.(*ForAll); ok {
            // Introduce universal variable.
            c.ctx.Extend(&CtxUniVar{ID: fa.Var, Name: fa.Name, Kind: fa.Kind})
            // Check body against the opened type.
            body := c.Check(expr, fa.Body)
            // Drop the universal variable.
            c.ctx.DropAfter(fa.Var)
            // Elaborate: type lambda.
            return &CoreTyLam{
                Span:    expr.GetSpan(),
                TyParam: fa.Var,
                TyName:  fa.Name,
                Kind:    fa.Kind,
                Body:    body,
                Type:    expected,
            }
        }

    default:
        // Fall through to term-specific checking.
    }

    // Dispatch on term form.
    switch e := expr.(type) {

    // --- Lam: check lambda against A -> B ---
    case *ResLam:
        arr, ok := expected.(*Arrow)
        if !ok {
            // Expected a function type but got something else.
            // Try synthesizing and using subtyping.
            return c.checkBySynthesis(expr, expected)
        }
        // Introduce the parameter.
        c.ctx.Extend(&CtxTermVar{Name: e.Param, Type: arr.Param})
        body := c.Check(e.Body, arr.Result)
        return &CoreLam{
            Span:      e.Span,
            Param:     e.Param,
            ParamType: arr.Param,
            Body:      body,
            Type:      expected,
        }

    // --- Pure: check pure e against Computation R R A ---
    case *ResPure:
        comp, ok := expected.(*Computation)
        if !ok {
            return c.checkBySynthesis(expr, expected)
        }
        // Pre and post rows must be equal.
        if err := c.unifier.UnifyRows(comp.Pre, comp.Post); err != nil {
            c.addError(e.Span, "pure requires equal pre and post rows: %v", err)
        }
        inner := c.Check(e.Expr, comp.Result)
        return &CorePure{
            Span: e.Span,
            Row:  c.ctx.ApplyRow(comp.Pre),
            Expr: inner,
            Type: expected,
        }

    // --- Bind: check bind c1 (\x -> c2) against Computation R1 R3 B ---
    case *ResBind:
        comp, ok := expected.(*Computation)
        if !ok {
            return c.checkBySynthesis(expr, expected)
        }

        // Introduce existentials for the intermediate row and the element type.
        r2 := c.FreshRowExVar(e.Span)
        a := c.FreshExVar(KindType, e.Span)

        // Check c1 against Computation R1 r2^ a^.
        c1Expected := &Computation{
            Span:   e.Span,
            Pre:    comp.Pre,
            Post:   r2,
            Result: a,
        }
        c1Core := c.Check(e.Comp, c1Expected)

        // Apply solutions learned from c1.
        r2Solved := c.ctx.ApplyRow(r2)
        aSolved := c.ctx.Apply(a)

        // Check continuation against aSolved -> Computation r2Solved R3 B.
        contExpected := &Arrow{
            Span:  e.Span,
            Param: aSolved,
            Result: &Computation{
                Span:   e.Span,
                Pre:    r2Solved,
                Post:   comp.Post,
                Result: comp.Result,
            },
        }
        contCore := c.Check(e.Cont, contExpected)

        return &CoreBind{
            Span:     e.Span,
            PreRow:   c.ctx.ApplyRow(comp.Pre),
            MidRow:   c.ctx.ApplyRow(r2Solved),
            PostRow:  c.ctx.ApplyRow(comp.Post),
            ElemType: c.ctx.Apply(aSolved),
            ResType:  c.ctx.Apply(comp.Result),
            Comp:     c1Core,
            Cont:     contCore,
            Type:     expected,
        }

    // --- Case: check case e of alts against expected type ---
    case *ResCase:
        // Infer the scrutinee type.
        scrutType, scrutCore := c.Infer(e.Scrutinee)
        scrutType = c.ctx.Apply(scrutType)

        // Check each alternative against the expected type.
        coreAlts := make([]CoreAlt, len(e.Alts))
        for i, alt := range e.Alts {
            coreAlts[i] = c.checkAlt(alt, scrutType, expected)
        }

        return &CoreCase{
            Span:      e.Span,
            Scrutinee: scrutCore,
            ScrutType: scrutType,
            Alts:      coreAlts,
            Type:      expected,
        }

    // --- Let: check let x = e1 in e2 ---
    case *ResLet:
        var defType Type
        var defCore CoreExpr
        if e.Annot != nil {
            // Annotated: check definition against annotation.
            annotType := c.resolveType(*e.Annot)
            defCore = c.Check(e.Def, annotType)
            defType = annotType
        } else {
            // Unannotated: infer definition type.
            defType, defCore = c.Infer(e.Def)
        }
        // Generalize at let-binding.
        defType = c.generalize(defType)
        // Extend context with the binding.
        c.ctx.Extend(&CtxTermVar{Name: e.Name, Type: defType})
        bodyCore := c.Check(e.Body, expected)
        return &CoreLet{
            Span:     e.Span,
            Name:     e.Name,
            NameType: defType,
            Def:      defCore,
            Body:     bodyCore,
            Type:     expected,
        }

    // --- Constructor: check constructor application ---
    case *ResCon:
        return c.checkConstructor(e, expected)

    default:
        // --- Sub rule: synthesize and check subtyping ---
        return c.checkBySynthesis(expr, expected)
    }
}

// checkBySynthesis implements the Sub rule: synthesize a type for the
// expression, then check that the synthesized type is a subtype of
// the expected type.
func (c *Checker) checkBySynthesis(expr ResExpr, expected Type) CoreExpr {
    inferredType, coreExpr := c.Infer(expr)
    c.Subtype(c.ctx.Apply(inferredType), c.ctx.Apply(expected))
    return coreExpr
}
```

### 6.3 Synthesis Mode

```go
// Infer synthesizes a type for an expression.
// Returns the inferred type and the elaborated core expression.
func (c *Checker) Infer(expr ResExpr) (Type, CoreExpr) {

    switch e := expr.(type) {

    // --- Var: look up in context ---
    case *ResVar:
        t, ok := c.ctx.LookupTermVar(e.Name)
        if !ok {
            // Check primitives.
            t, ok = c.primitives[e.Name.Text]
            if !ok {
                c.addError(e.Span, "variable %q not in scope", e.Name.Text)
                return c.errorType(), &CoreVar{Span: e.Span, Name: e.Name}
            }
        }
        return t, &CoreVar{Span: e.Span, Name: e.Name, Type: t}

    // --- Ann: e :: T ---
    case *ResAnn:
        annType := c.resolveType(e.Type)
        core := c.Check(e.Expr, annType)
        return annType, core

    // --- App: f x ---
    case *ResApp:
        funType, funCore := c.Infer(e.Fun)
        funType = c.ctx.Apply(funType)
        resType, argCore := c.AppInfer(funType, e.Arg)
        return resType, &CoreApp{
            Span: e.Span,
            Fun:  funCore,
            Arg:  argCore,
            Type: resType,
        }

    // --- Lit: integer, string, etc. ---
    case *ResLit:
        t := c.literalType(e.Lit)
        return t, &CoreLit{Span: e.Span, Lit: e.Lit, Type: t}

    // --- Constructor used in synthesis mode ---
    case *ResCon:
        return c.inferConstructor(e)

    // --- Lam without expected type ---
    case *ResLam:
        // Cannot infer the parameter type of an unannotated lambda.
        // Introduce a fresh existential for the parameter type.
        paramType := c.FreshExVar(KindType, e.Span)
        c.ctx.Extend(&CtxTermVar{Name: e.Param, Type: paramType})
        bodyType, bodyCore := c.Infer(e.Body)
        return &Arrow{
            Span:   e.Span,
            Param:  c.ctx.Apply(paramType),
            Result: bodyType,
        }, &CoreLam{
            Span:      e.Span,
            Param:     e.Param,
            ParamType: c.ctx.Apply(paramType),
            Body:      bodyCore,
            Type: &Arrow{
                Span:   e.Span,
                Param:  c.ctx.Apply(paramType),
                Result: bodyType,
            },
        }

    // --- Bind in synthesis mode ---
    case *ResBind:
        // Infer c1's type.
        c1Type, c1Core := c.Infer(e.Comp)
        c1Type = c.ctx.Apply(c1Type)
        comp, ok := c1Type.(*Computation)
        if !ok {
            c.addError(e.Span, "expected computation type in bind, got %s", c1Type)
            return c.errorType(), c1Core
        }
        // Check continuation against a -> Computation post post2 b.
        r3 := c.FreshRowExVar(e.Span)
        b := c.FreshExVar(KindType, e.Span)
        contExpected := &Arrow{
            Span:  e.Span,
            Param: comp.Result,
            Result: &Computation{
                Span:   e.Span,
                Pre:    comp.Post,
                Post:   r3,
                Result: b,
            },
        }
        contCore := c.Check(e.Cont, contExpected)

        resType := &Computation{
            Span:   e.Span,
            Pre:    c.ctx.ApplyRow(comp.Pre),
            Post:   c.ctx.ApplyRow(r3),
            Result: c.ctx.Apply(b),
        }
        return resType, &CoreBind{
            Span:     e.Span,
            PreRow:   c.ctx.ApplyRow(comp.Pre),
            MidRow:   c.ctx.ApplyRow(comp.Post),
            PostRow:  c.ctx.ApplyRow(r3),
            ElemType: c.ctx.Apply(comp.Result),
            ResType:  c.ctx.Apply(b),
            Comp:     c1Core,
            Cont:     contCore,
            Type:     resType,
        }

    // --- Pure in synthesis mode ---
    case *ResPure:
        innerType, innerCore := c.Infer(e.Expr)
        r := c.FreshRowExVar(e.Span)
        resType := &Computation{
            Span:   e.Span,
            Pre:    r,
            Post:   r,
            Result: innerType,
        }
        return resType, &CorePure{
            Span: e.Span,
            Row:  r,
            Expr: innerCore,
            Type: resType,
        }

    default:
        c.addError(expr.GetSpan(), "cannot infer type of %T", expr)
        return c.errorType(), nil
    }
}
```

### 6.4 Application Inference

The application judgment handles instantiation of polymorphic functions at call sites.

```go
// AppInfer implements the application judgment:
// given a function type and an argument expression,
// determine the result type and elaborate the argument.
func (c *Checker) AppInfer(funType Type, arg ResExpr) (Type, CoreExpr) {
    funType = c.ctx.Apply(funType)

    switch ft := funType.(type) {

    // --- ForallApp: forall a. A applied to argument ---
    case *ForAll:
        // Instantiate the forall with a fresh existential.
        ex := c.FreshExVar(ft.Kind, arg.GetSpan())
        // Substitute the existential for the bound variable.
        instantiated := c.substitute(ft.Body, ft.Var, ex)
        // Continue with the instantiated type.
        resType, argCore := c.AppInfer(instantiated, arg)
        return resType, argCore

    // --- Arrow: A -> B applied to argument ---
    case *Arrow:
        argCore := c.Check(arg, ft.Param)
        return c.ctx.Apply(ft.Result), argCore

    // --- Existential: a^ applied to argument ---
    case *ExVar:
        // Split: a^ = a1^ -> a2^, then check argument against a1^.
        a1 := c.FreshExVar(KindType, arg.GetSpan())
        a2 := c.FreshExVar(KindType, arg.GetSpan())
        // Insert a1^ and a2^ before a^ in the context.
        c.ctx.InsertBefore(ft.ID, &CtxExVar{ID: a1.ID, Kind: KindType},
            &CtxExVar{ID: a2.ID, Kind: KindType})
        // Solve a^ = a1^ -> a2^.
        c.ctx.SolveExVar(ft.ID, &Arrow{Param: a1, Result: a2})
        // Check argument against a1^.
        argCore := c.Check(arg, a1)
        return c.ctx.Apply(a2), argCore

    default:
        c.addError(arg.GetSpan(), "cannot apply non-function type %s", funType)
        return c.errorType(), nil
    }
}
```

### 6.5 Subtyping

In Gomputation, subtyping is equality plus handling of `forall`. There is no structural subtyping.

```go
// Subtype checks that typeA is a subtype of typeB (A <: B).
// In Gomputation, this is equality except for forall handling.
func (c *Checker) Subtype(typeA, typeB Type) {
    typeA = c.ctx.Apply(typeA)
    typeB = c.ctx.Apply(typeB)

    switch {

    // --- ForallL: forall a. A <: B ---
    case isForAll(typeA):
        fa := typeA.(*ForAll)
        // Introduce marker and fresh existential.
        marker := c.freshMarkerID()
        c.ctx.Extend(&CtxMarker{ID: marker})
        ex := c.FreshExVar(fa.Kind, fa.Body.GetSpan())
        // Substitute existential for the bound variable.
        opened := c.substitute(fa.Body, fa.Var, ex)
        // Continue checking.
        c.Subtype(opened, typeB)
        // Drop marker and everything after.
        c.ctx.DropAfter(marker)

    // --- ForallR: A <: forall a. B ---
    case isForAll(typeB):
        fb := typeB.(*ForAll)
        // Introduce universal variable.
        c.ctx.Extend(&CtxUniVar{ID: fb.Var, Name: fb.Name, Kind: fb.Kind})
        // Check A <: B[a := alpha].
        c.Subtype(typeA, fb.Body)
        // Drop the universal variable and everything after.
        c.ctx.DropAfter(fb.Var)

    // --- Existential on the left: a^ <: A ---
    case isExVar(typeA):
        ev := typeA.(*ExVar)
        c.InstantiateL(ev, typeB)

    // --- Existential on the right: A <: a^ ---
    case isExVar(typeB):
        ev := typeB.(*ExVar)
        c.InstantiateR(typeA, ev)

    // --- Structural: delegate to unification ---
    default:
        if err := c.unifier.Unify(typeA, typeB); err != nil {
            c.addError(typeA.GetSpan(),
                "type mismatch: %s is not compatible with %s: %v",
                c.prettyType(typeA), c.prettyType(typeB), err)
        }
    }
}
```

### 6.6 Instantiation

```go
// InstantiateL solves existential ev so that ev <: typeB.
// This is the <=: judgment from DK.
func (c *Checker) InstantiateL(ev *ExVar, typeB Type) {
    typeB = c.ctx.Apply(typeB)

    switch b := typeB.(type) {

    // --- InstLSolve: a^ <=: tau (monotype) ---
    case monoType:
        if err := c.unifier.Unify(ev, typeB); err != nil {
            c.addError(ev.Span, "instantiation failed: %v", err)
        }

    // --- InstLArr: a^ <=: A1 -> A2 ---
    case *Arrow:
        a1 := c.FreshExVar(KindType, b.Span)
        a2 := c.FreshExVar(KindType, b.Span)
        c.ctx.InsertBefore(ev.ID,
            &CtxExVar{ID: a2.ID, Kind: KindType},
            &CtxExVar{ID: a1.ID, Kind: KindType})
        c.ctx.SolveExVar(ev.ID, &Arrow{Param: a1, Result: a2})
        // Contravariant: direction flips for the parameter.
        c.InstantiateR(b.Param, a1)
        c.InstantiateL(a2, c.ctx.Apply(b.Result))

    // --- InstLAllR: a^ <=: forall b. B ---
    case *ForAll:
        c.ctx.Extend(&CtxUniVar{ID: b.Var, Name: b.Name, Kind: b.Kind})
        c.InstantiateL(ev, b.Body)
        c.ctx.DropAfter(b.Var)

    // --- InstLComp: a^ <=: Computation pre post ret ---
    case *Computation:
        pre := c.FreshRowExVar(b.Span)
        post := c.FreshRowExVar(b.Span)
        ret := c.FreshExVar(KindType, b.Span)
        c.ctx.SolveExVar(ev.ID, &Computation{Pre: pre, Post: post, Result: ret})
        if err := c.unifier.UnifyRows(pre, b.Pre); err != nil {
            c.addError(b.Span, "in pre-row instantiation: %v", err)
        }
        if err := c.unifier.UnifyRows(
            c.ctx.ApplyRow(post), c.ctx.ApplyRow(b.Post)); err != nil {
            c.addError(b.Span, "in post-row instantiation: %v", err)
        }
        if err := c.unifier.Unify(c.ctx.Apply(ret), b.Result); err != nil {
            c.addError(b.Span, "in result type instantiation: %v", err)
        }

    // --- ExVar ~ ExVar ---
    case *ExVar:
        if err := c.unifier.Unify(ev, typeB); err != nil {
            c.addError(ev.Span, "instantiation failed: %v", err)
        }

    default:
        if err := c.unifier.Unify(ev, typeB); err != nil {
            c.addError(ev.Span, "instantiation failed: %v", err)
        }
    }
}

// InstantiateR is symmetric to InstantiateL with flipped variance.
// typeA :=> ev
func (c *Checker) InstantiateR(typeA Type, ev *ExVar) {
    typeA = c.ctx.Apply(typeA)

    switch a := typeA.(type) {

    case monoType:
        if err := c.unifier.Unify(typeA, ev); err != nil {
            c.addError(ev.Span, "instantiation failed: %v", err)
        }

    case *Arrow:
        a1 := c.FreshExVar(KindType, a.Span)
        a2 := c.FreshExVar(KindType, a.Span)
        c.ctx.InsertBefore(ev.ID,
            &CtxExVar{ID: a2.ID, Kind: KindType},
            &CtxExVar{ID: a1.ID, Kind: KindType})
        c.ctx.SolveExVar(ev.ID, &Arrow{Param: a1, Result: a2})
        // Contravariant: direction flips.
        c.InstantiateL(a1, a.Param)
        c.InstantiateR(c.ctx.Apply(a.Result), a2)

    case *ForAll:
        marker := c.freshMarkerID()
        c.ctx.Extend(&CtxMarker{ID: marker})
        ex := c.FreshExVar(a.Kind, a.Body.GetSpan())
        opened := c.substitute(a.Body, a.Var, ex)
        c.InstantiateR(opened, ev)
        c.ctx.DropAfter(marker)

    case *Computation:
        pre := c.FreshRowExVar(a.Span)
        post := c.FreshRowExVar(a.Span)
        ret := c.FreshExVar(KindType, a.Span)
        c.ctx.SolveExVar(ev.ID, &Computation{Pre: pre, Post: post, Result: ret})
        if err := c.unifier.UnifyRows(a.Pre, pre); err != nil {
            c.addError(a.Span, "in pre-row instantiation: %v", err)
        }
        if err := c.unifier.UnifyRows(
            c.ctx.ApplyRow(a.Post), c.ctx.ApplyRow(post)); err != nil {
            c.addError(a.Span, "in post-row instantiation: %v", err)
        }
        if err := c.unifier.Unify(a.Result, c.ctx.Apply(ret)); err != nil {
            c.addError(a.Span, "in result type instantiation: %v", err)
        }

    default:
        if err := c.unifier.Unify(typeA, ev); err != nil {
            c.addError(ev.Span, "instantiation failed: %v", err)
        }
    }
}
```

### 6.7 Generalization

Generalization at `let` bindings converts unsolved existentials in the inferred type to universally quantified variables.

```go
// generalize universalizes free existentials in a type that are not
// free in the current environment. This implements let-generalization.
func (c *Checker) generalize(t Type) Type {
    t = c.ctx.Apply(t)

    // Collect free existentials in t.
    freeInType := c.freeExVars(t)

    // Collect free existentials in the environment.
    freeInEnv := c.freeExVarsInEnv()

    // Generalizable = free in type but not in environment.
    var toGeneralize []int
    for id := range freeInType {
        if !freeInEnv[id] {
            toGeneralize = append(toGeneralize, id)
        }
    }

    // Sort for deterministic output.
    sort.Ints(toGeneralize)

    // Wrap in foralls.
    result := t
    for i := len(toGeneralize) - 1; i >= 0; i-- {
        id := toGeneralize[i]
        kind := c.exVarKind(id)
        name := c.freshTyVarName()
        uv := &UniVar{ID: id, Name: name, Kind: kind}
        // Replace the existential with the universal variable in the type.
        result = c.substituteExVar(result, id, uv)
        result = &ForAll{Var: id, Name: name, Kind: kind, Body: result}
    }

    return result
}
```

---

## 7. Kind Checking

### 7.1 Implementation: Integrated Lightweight Pass

Kind checking in Gomputation is simple enough to be a set of validation functions called within the type checker, rather than a separate compiler phase.

```go
// checkKind validates that a type expression has the expected kind.
func (c *Checker) checkKind(ty ResType, expected Kind) {
    actual := c.inferKind(ty)
    if actual != expected {
        c.addError(ty.GetSpan(),
            "kind mismatch: expected %s, got %s", expected, actual)
    }
}

// inferKind computes the kind of a type expression.
func (c *Checker) inferKind(ty ResType) Kind {
    switch t := ty.(type) {

    case *ResTypeVar:
        // Look up the variable's kind in the context.
        if kind, ok := c.lookupTypeVarKind(t.Name); ok {
            return kind
        }
        c.addError(t.Span, "type variable %q not in scope", t.Name.Text)
        return KindType

    case *ResArrow:
        c.checkKind(t.Param, KindType)
        c.checkKind(t.Result, KindType)
        return KindType

    case *ResForAll:
        // The bound variable has its declared kind.
        c.ctx.Extend(&CtxUniVar{ID: t.Var.Unique, Name: t.Var.Text, Kind: t.VarKind})
        bodyKind := c.inferKind(t.Body)
        c.ctx.DropAfter(t.Var.Unique)
        // forall a. T has kind Type if T has kind Type.
        if bodyKind != KindType {
            c.addError(t.Span, "body of forall must have kind Type, got %s", bodyKind)
        }
        return KindType

    case *ResComputation:
        c.checkKind(t.Pre, KindRow)
        c.checkKind(t.Post, KindRow)
        c.checkKind(t.Result, KindType)
        return KindType

    case *ResTyCon:
        // Look up expected argument kinds from the type constructor declaration.
        expectedKinds, ok := c.typeConstructorKinds(t.Name)
        if !ok {
            c.addError(t.Span, "unknown type constructor %q", t.Name)
            return KindType
        }
        if len(t.Args) != len(expectedKinds) {
            c.addError(t.Span,
                "type constructor %q expects %d arguments, got %d",
                t.Name, len(expectedKinds), len(t.Args))
            return KindType
        }
        for i, arg := range t.Args {
            c.checkKind(arg, expectedKinds[i])
        }
        return KindType

    case *ResRowEmpty:
        return KindRow

    case *ResRowExtend:
        c.checkKind(t.FieldType, KindType)
        c.checkKind(t.Tail, KindRow)
        return KindRow

    case *ResRowVar:
        if kind, ok := c.lookupTypeVarKind(t.Name); ok {
            if kind != KindRow {
                c.addError(t.Span,
                    "variable %q has kind %s, expected Row", t.Name.Text, kind)
            }
            return KindRow
        }
        c.addError(t.Span, "row variable %q not in scope", t.Name.Text)
        return KindRow

    default:
        c.addError(ty.GetSpan(), "unexpected type form")
        return KindType
    }
}
```

### 7.2 Kind Checking for ADT Declarations

```go
// checkADTDecl validates an algebraic data type declaration.
func (c *Checker) checkADTDecl(decl ADTDecl) {
    // Introduce type parameters.
    for _, param := range decl.TyParams {
        c.ctx.Extend(&CtxUniVar{
            ID:   param.Unique,
            Name: param.Text,
            Kind: KindType, // ADT parameters are always of kind Type
        })
    }

    // Check each constructor.
    for _, con := range decl.Constructors {
        for _, fieldType := range con.FieldTypes {
            c.checkKind(fieldType, KindType)
        }
    }

    // Drop type parameters.
    if len(decl.TyParams) > 0 {
        c.ctx.DropAfter(decl.TyParams[0].Unique)
    }
}
```

---

## 8. Elaboration

### 8.1 What Elaboration Produces

The output of type checking is an elaborated core where:

1. **All type applications are explicit.** Where the surface program writes `f x` and `f` has type `forall a. a -> a`, the core writes `f @Int x` (with the solved type argument).

2. **All row applications are explicit.** Where the surface program uses `dbOpen` with type `forall r. Computation { db : DB[Closed] | r } ...`, the core writes `dbOpen @{ log : Logger[Ready] }`.

3. **All `forall` introductions are explicit.** A definition `id := \x -> x` with inferred type `forall a. a -> a` elaborates to `/\a -> \(x : a) -> x`.

4. **`bind` carries explicit intermediate annotations.** `bind c1 (\x -> c2)` elaborates to `bind @R1 @R2 @R3 @A @B c1 (\(x : A) -> c2)`.

### 8.2 Why Elaboration Matters

The elaborated core serves three purposes:

1. **Sanity check.** The core can be type-checked without inference. If the core type-checks, the elaboration is correct. This is the "Core Lint" principle from GHC.

2. **Evaluator input.** The evaluator operates on the core, which has no ambiguity. No inference is needed at runtime.

3. **Error investigation.** When a type error is mysterious, dumping the elaborated core shows exactly what the type checker decided, including all instantiations.

### 8.3 Elaboration During Checking

Elaboration is not a separate pass. Each checking/inference function returns a `CoreExpr` alongside its type-checking work. The `CoreExpr` is constructed incrementally as checking proceeds.

This means the `Check` and `Infer` functions have the signatures shown in Section 6: they return `CoreExpr` values that are the elaborated forms of their input expressions.

---

## 9. Error Handling

### 9.1 Error Representation

```go
package checker

// CheckError represents a type checking error with full context.
type CheckError struct {
    // Where in the source the error occurred.
    Span Span

    // The error category.
    Category ErrorCategory

    // Human-readable message.
    Message string

    // The expected type (if applicable), zonked for display.
    Expected Type

    // The actual type (if applicable), zonked for display.
    Got Type

    // Additional notes (e.g., "the row variable was introduced here").
    Notes []ErrorNote
}

type ErrorNote struct {
    Span    Span
    Message string
}

type ErrorCategory int

const (
    ErrTypeMismatch ErrorCategory = iota
    ErrRowMismatch
    ErrRowLabelConflict
    ErrInfiniteType
    ErrNotInScope
    ErrKindMismatch
    ErrAmbiguousType
    ErrMissingAnnotation
    ErrDuplicateLabel
    ErrNonExhaustivePatterns
    ErrRedundantPattern
)
```

### 9.2 Error Accumulation Strategy

Gomputation should accumulate errors rather than stopping at the first one. This provides a better user experience: the programmer sees all problems at once rather than fixing them one at a time.

```go
// addError records a type checking error without stopping.
func (c *Checker) addError(span Span, format string, args ...interface{}) {
    err := CheckError{
        Span:    span,
        Message: fmt.Sprintf(format, args...),
    }
    c.errors = append(c.errors, err)

    // Stop accumulating after maxErrors to avoid cascading noise.
    if c.maxErrors > 0 && len(c.errors) >= c.maxErrors {
        panic(tooManyErrors{})
    }
}

// addTypeMismatch records a type mismatch with full context.
func (c *Checker) addTypeMismatch(span Span, expected, got Type, context string) {
    // Zonk types for display: show solved variables, not metavariable IDs.
    displayExpected := c.Zonk(expected)
    displayGot := c.Zonk(got)
    c.errors = append(c.errors, CheckError{
        Span:     span,
        Category: ErrTypeMismatch,
        Message: fmt.Sprintf("%s: expected %s, got %s",
            context, c.prettyType(displayExpected), c.prettyType(displayGot)),
        Expected: displayExpected,
        Got:      displayGot,
    })
}

type tooManyErrors struct{}
```

### 9.3 Error Recovery

When an error is detected, the checker should continue with a "poison" type to avoid cascading errors:

```go
// errorType returns a special error type that unifies with anything.
// This prevents cascading errors after the first real error.
func (c *Checker) errorType() Type {
    return &ErrorType{}
}

// During unification, ErrorType matches anything:
// unify(ErrorType, T) = ok
// unify(T, ErrorType) = ok
```

This follows the pattern used by GHC, Elm, and Rust: after an error, insert a "hole" type that is compatible with everything. Subsequent unification against the hole succeeds silently, preventing false cascading errors.

### 9.4 De-Zonking for Display

Error messages should display types as the user wrote them, not as internal metavariable identifiers. This requires zonking (substituting solved existentials) and then pretty-printing.

```go
// prettyType converts an internal type to a user-readable string.
func (c *Checker) prettyType(t Type) string {
    t = c.Zonk(t)
    switch v := t.(type) {
    case *UniVar:
        return v.Name
    case *ExVar:
        return fmt.Sprintf("?%d", v.ID) // unsolved: show as ?N
    case *Arrow:
        param := c.prettyType(v.Param)
        result := c.prettyType(v.Result)
        // Parenthesize the parameter if it is itself an arrow.
        if _, ok := v.Param.(*Arrow); ok {
            param = "(" + param + ")"
        }
        return param + " -> " + result
    case *ForAll:
        return "forall " + v.Name + ". " + c.prettyType(v.Body)
    case *Computation:
        return fmt.Sprintf("Computation %s %s %s",
            c.prettyRow(v.Pre), c.prettyRow(v.Post), c.prettyType(v.Result))
    case *TyCon:
        if len(v.Args) == 0 {
            return v.Name
        }
        args := make([]string, len(v.Args))
        for i, a := range v.Args {
            args[i] = c.prettyType(a)
        }
        return v.Name + "[" + strings.Join(args, ", ") + "]"
    default:
        return fmt.Sprintf("%T", t)
    }
}
```

### 9.5 Source Span Tracking

Every AST node, type, and core expression carries a `Span`. The span is inherited from the surface syntax during parsing and preserved through desugaring and type checking. When desugaring introduces synthetic nodes (e.g., `bind` from `do`-notation), the synthetic node carries the span of the corresponding surface syntax element (the `<-` or the semicolon).

---

## 10. Testing Strategy

### 10.1 Golden Tests (Primary Strategy)

Golden tests are the backbone of type checker testing. Each test case is a source file paired with an expected output file.

**Directory structure:**

```text
testdata/
    check/
        pass/
            identity.gmp           -- source: id := \x -> x
            identity.golden        -- expected: forall a. a -> a
            bind_chain.gmp         -- source: do { dbOpen; dbClose; pure () }
            bind_chain.golden      -- expected elaborated core + type
        fail/
            scope_escape.gmp       -- source with a scope error
            scope_escape.golden    -- expected error message
            row_conflict.gmp       -- db: DB[Closed] vs db: DB[Opened]
            row_conflict.golden    -- expected error
    rows/
        pass/
            open_closed.gmp        -- row unification: open ~ closed
            open_closed.golden
            open_open.gmp          -- row unification: open ~ open
            open_open.golden
        fail/
            duplicate_label.gmp
            duplicate_label.golden
    kinds/
        pass/
            computation_well_kinded.gmp
            computation_well_kinded.golden
        fail/
            wrong_kind_arg.gmp
            wrong_kind_arg.golden
```

**Test runner:**

```go
func TestGolden(t *testing.T) {
    files, _ := filepath.Glob("testdata/check/**/*.gmp")
    for _, f := range files {
        t.Run(filepath.Base(f), func(t *testing.T) {
            src, _ := os.ReadFile(f)
            goldenFile := strings.TrimSuffix(f, ".gmp") + ".golden"
            expected, _ := os.ReadFile(goldenFile)

            result := runChecker(string(src))
            actual := formatResult(result)

            if string(expected) != actual {
                t.Errorf("mismatch:\n--- expected ---\n%s\n--- actual ---\n%s",
                    expected, actual)
                // Optionally: update golden file with -update flag.
            }
        })
    }
}
```

### 10.2 Property-Based Testing

Property-based tests verify invariants that hold for all well-typed programs.

**Properties to test:**

1. **Soundness of elaboration:** If `Check(expr, ty)` succeeds, the resulting `CoreExpr` type-checks in the core checker (round-trip).

2. **Idempotence of zonking:** `Zonk(Zonk(t)) == Zonk(t)` for all types `t`.

3. **Soundness of unification:** If `Unify(t1, t2)` succeeds, then `Apply(t1) == Apply(t2)` after applying the resulting substitution.

4. **Occurs check prevents infinite types:** If `Unify(a^, T)` succeeds and `a^` appears in `T`, then the occurs check has failed (contradiction; this should not happen).

5. **Row permutation invariance:** `UnifyRows(R1, R2)` produces the same result regardless of the order of labels in `R1` and `R2`.

6. **Label uniqueness preservation:** If both input rows have unique labels, the solution (after solving tail variables) also has unique labels.

```go
func TestRowUnificationPermutationInvariant(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        // Generate a random set of labels and types.
        n := rapid.IntRange(1, 5).Draw(t, "n")
        labels := make([]string, n)
        for i := range labels {
            labels[i] = fmt.Sprintf("cap%d", i)
        }

        // Create two rows with the same labels in different orders.
        perm1 := rapid.Permutation(n).Draw(t, "perm1")
        perm2 := rapid.Permutation(n).Draw(t, "perm2")

        row1 := buildRow(labels, perm1)
        row2 := buildRow(labels, perm2)

        u1 := NewUnifier(NewContext(), new(int))
        u2 := NewUnifier(NewContext(), new(int))

        err1 := u1.UnifyRows(row1, closedRow(labels))
        err2 := u2.UnifyRows(row2, closedRow(labels))

        // Both should succeed or both should fail.
        if (err1 == nil) != (err2 == nil) {
            t.Fatalf("permutation changed result: err1=%v, err2=%v", err1, err2)
        }
    })
}
```

### 10.3 Targeted Unit Tests

Unit tests for specific algorithmic edge cases:

```go
// Test the five cases of row unification.
func TestRowUnification_ClosedClosed(t *testing.T) { ... }
func TestRowUnification_OpenClosed(t *testing.T) { ... }
func TestRowUnification_ClosedOpen(t *testing.T) { ... }
func TestRowUnification_OpenOpen(t *testing.T) { ... }
func TestRowUnification_BareVars(t *testing.T) { ... }

// Test occurs check.
func TestOccursCheck_Simple(t *testing.T) { ... }
func TestOccursCheck_InRow(t *testing.T) { ... }

// Test context operations.
func TestContext_DropAfter(t *testing.T) { ... }
func TestContext_SolveAndLookup(t *testing.T) { ... }
func TestContext_InsertBefore(t *testing.T) { ... }

// Test the bind typing rule.
func TestBind_CheckMode_InferIntermediateRow(t *testing.T) { ... }
func TestBind_SynthMode(t *testing.T) { ... }

// Test higher-rank instantiation.
func TestInstantiateL_Arrow(t *testing.T) { ... }
func TestInstantiateR_Arrow(t *testing.T) { ... }
func TestInstantiateL_ForAll(t *testing.T) { ... }
```

### 10.4 Error Message Regression Tests

Golden tests specifically for error messages, to prevent regressions in error quality:

```go
func TestErrorMessages(t *testing.T) {
    cases := []struct {
        name   string
        source string
        expect string // substring of the error message
    }{
        {
            name:   "row label conflict",
            source: `f := bind dbOpen dbClose`,
            expect: `at label "db", expected DB[Closed] but got DB[Opened]`,
        },
        {
            name:   "missing capability",
            source: `f := dbOpen`, // without db in environment
            expect: `closed row lacks labels: ["db"]`,
        },
        {
            name:   "infinite type",
            source: `f := \x -> f x x`,
            expect: `infinite type`,
        },
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            result := runChecker(tc.source)
            if !strings.Contains(result.Error(), tc.expect) {
                t.Errorf("expected error containing %q, got: %s", tc.expect, result.Error())
            }
        })
    }
}
```

### 10.5 Round-Trip Testing

Parse, check, elaborate, then re-check the core:

```go
func TestRoundTrip(t *testing.T) {
    sources := loadAllGoldenSources("testdata/check/pass/")
    for _, src := range sources {
        t.Run(src.name, func(t *testing.T) {
            // Phase 1: parse and check.
            core, ty, err := fullPipeline(src.text)
            if err != nil {
                t.Fatalf("initial check failed: %v", err)
            }

            // Phase 2: re-check the elaborated core (no inference needed).
            if err := coreCheck(core, ty); err != nil {
                t.Fatalf("core re-check failed: %v", err)
            }
        })
    }
}
```

---

## 11. Performance Considerations

### 11.1 Union-Find Path Compression

Path compression is essential for near-linear-time unification. Without it, chains of unification variables degrade to linear lookup. The two-pass path compression (find with pointer rewriting) shown in Section 5.1 is the standard technique.

For row variables, path compression during `FindRow` ensures that chains of row variable solutions are collapsed. This is particularly important when many `bind` steps thread a row variable through a long chain.

### 11.2 Context Operations

The ordered context is implemented as a slice (`[]CtxEntry`). This has the following performance characteristics:

| Operation | Complexity | Notes |
|-----------|-----------|-------|
| Extend (append) | O(1) amortized | Go slice append |
| Lookup by ID | O(n) | Linear scan, right-to-left |
| DropAfter | O(1) | Re-slice |
| SolveExVar | O(n) | Linear scan to find the entry |
| InsertBefore | O(n) | Requires slice copy |

For Gomputation's expected context sizes (tens to hundreds of entries, not thousands), linear scans are acceptable. If profiling reveals context lookup as a bottleneck, a secondary index (map from ID to slice index) can be added.

**Optimization: avoid unnecessary Apply calls.** The `ctx.Apply(t)` operation walks the entire type, which can be expensive if called repeatedly on the same type. A practical optimization is to apply lazily -- only when the type is inspected -- and to cache the result. This is the "zonk on demand" pattern used by GHC.

### 11.3 Lazy vs Eager Zonking

**Lazy zonking (recommended for Gomputation).** Apply the context substitution only when a type is inspected (during unification, during error reporting, at the end of checking). This avoids redundant work when a type is constructed and then immediately unified without being inspected.

**Eager zonking.** Apply the context substitution immediately after every solve operation. This keeps all types fully resolved at all times, which simplifies debugging but increases work.

The recommended approach is lazy zonking during checking, with a single eager zonk pass at the end (Phase 4: Zonking) to produce the final elaborated core.

### 11.4 Memory Allocation in Go

Go's garbage collector handles allocation automatically, but allocation patterns affect performance:

1. **Pre-allocate slices.** The context slice, error list, and label lists should be pre-allocated with reasonable capacities: `make([]CtxEntry, 0, 64)`, `make([]CheckError, 0, 16)`.

2. **Avoid pointer chasing.** The `FlatRow` uses a slice of `LabeledType` structs (not pointers) for cache-friendly traversal. For small capability rows (2--10 labels), this is significantly faster than a map.

3. **Reuse FlatRow allocations.** During row unification, `FlatRow` values are transient. A sync.Pool or arena allocator can reduce GC pressure if profiling identifies row flattening as a hotspot.

4. **String interning for labels.** Capability labels are compared frequently during row unification. Interning label strings (mapping each unique string to a single instance) enables comparison by pointer equality rather than string comparison.

### 11.5 Benchmarking Strategy

```go
func BenchmarkUnifyRows_ClosedClosed_5Labels(b *testing.B) {
    row1 := closedRow([]string{"a", "b", "c", "d", "e"})
    row2 := closedRow([]string{"e", "d", "c", "b", "a"}) // reversed
    for i := 0; i < b.N; i++ {
        u := NewUnifier(NewContext(), new(int))
        u.UnifyRows(row1, row2)
    }
}

func BenchmarkCheckBindChain_10Steps(b *testing.B) {
    src := generateBindChain(10)
    for i := 0; i < b.N; i++ {
        runChecker(src)
    }
}

func BenchmarkContextLookup_100Entries(b *testing.B) {
    ctx := NewContext()
    for i := 0; i < 100; i++ {
        ctx.Extend(&CtxTermVar{Name: Name{Unique: i}, Type: intType})
    }
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ctx.LookupTermVar(Name{Unique: 50})
    }
}
```

---

## 12. Specific Recommendations for Gomputation

### 12.1 Implementation Order

The type checker should be built incrementally, with each stage adding one feature:

**Stage 1: Simply-typed lambda calculus.** Variables, lambdas, application, let-bindings, literals. Bidirectional check/infer with unification for type variables. No polymorphism, no rows, no computations. This validates the core architecture: context management, unification, elaboration, error reporting.

**Stage 2: Rank-1 polymorphism.** Add `forall`, let-generalization, and instantiation at application sites. This exercises the DK-style ordered context, the `ForallL`/`ForallR` subtyping rules, and the instantiation judgment. Test with `id : forall a. a -> a` and `const : forall a b. a -> b -> a`.

**Stage 3: Algebraic data types.** Add `data` declarations, constructors, and `case` expressions. This requires constructor typing, pattern checking, and exhaustiveness analysis. Test with `data Bool = True | False` and `data Option a = None | Some a`.

**Stage 4: Row types and unification.** Add rows, row variables, row unification (flatten-then-diff), and the `Computation` type constructor. Test with closed rows first, then open rows, then row variables solved by unification.

**Stage 5: Computation checking.** Add `pure`, `bind`, and the checking rules for `Computation pre post a`. Test with simple bind chains against known types. This is the point where the system becomes capable of checking Gomputation programs.

**Stage 6: Do-notation and surface syntax.** Integrate with the parser's do-notation desugaring. Test that do-blocks elaborate to correct bind chains with inferred intermediate rows.

**Stage 7: Higher-rank polymorphism.** Extend instantiation to handle `forall` under arrows. This exercises `InstLArr`, `InstRArr`, `InstLAllR`, and their symmetric counterparts. Test with `runST : (forall s. ST s a) -> a` style types.

**Stage 8: Kind checking hardening.** Strengthen kind checking to reject all malformed type expressions. Test with intentionally wrong kinds.

### 12.2 Architecture Principles

1. **The checker is a struct with methods, not free functions.** The `Checker` struct holds the context, unifier, error list, and fresh variable counter. All checking functions are methods on `Checker`. This is idiomatic Go and avoids threading state through function parameters.

2. **Mutable state is explicit.** The ordered context and unifier are mutable. There is no attempt to simulate immutability or functional state-threading in Go. The DK algorithm is inherently imperative (the ordered context is modified in place), and Go is a natural fit for this.

3. **Errors accumulate; checking continues.** The checker does not panic or return early on the first error. It records the error, inserts a poison type, and continues. This is important for user experience. The `maxErrors` limit prevents runaway error cascading.

4. **Elaboration is simultaneous with checking.** Every `Check` and `Infer` call returns both a type-level result and a `CoreExpr`. There is no separate elaboration pass.

5. **The core is independently checkable.** The elaborated core can be re-checked by a simple, non-inferring type checker. This "Core Lint" property is the strongest internal consistency guarantee.

### 12.3 Go-Specific Patterns

**Interface vs concrete types for AST nodes.** Both `Type` and `Row` are Go interfaces. This enables `switch v := t.(type)` dispatch, which is idiomatic for tree-structured data in Go. The alternative -- a single struct with a tag field -- is less ergonomic and less type-safe.

**No Go generics for the checker.** Go generics (type parameters) do not add significant value to the checker infrastructure. The checker's polymorphism is at the object level (types and rows), not at the meta level (Go types). Using Go interfaces for the AST and concrete structs for the checker state is the right approach.

**Error handling: errors as values, not panics.** Unification errors are returned as `error` values. Checking errors are accumulated in the `Checker.errors` slice. The only use of `panic` is the `tooManyErrors` escape hatch, which is caught by a `recover` at the top-level entry point.

**Testing: use `testing` and `testify` or `go-cmp`.** Standard Go testing with table-driven tests for unit tests, golden files for integration tests, and `rapid` or `gopter` for property-based tests.

### 12.4 Interaction Between Rows and the Ordered Context

Row existential variables participate in the same ordered context as type existential variables. They share the same fresh ID counter (ensuring globally unique IDs) and obey the same left-of scoping rule.

The only distinction is their kind: a row existential has kind `Row` and can only be solved to a row expression, while a type existential has kind `Type` and can only be solved to a type expression. The kind check prevents a row variable from being solved to a non-row type (and vice versa), which would be a kind error.

When `forall r. T` is instantiated, the process is identical to `forall a. T`:

1. A fresh row existential `r^` is created and inserted into the context.
2. The bound variable `r` is replaced by `r^` in the body `T`.
3. When `r^` is later unified with a concrete row, the solution is recorded in the context.
4. When the scope of `r^` ends (at the corresponding marker), `r^` and its solution are dropped.

### 12.5 The Bind Rule: The Most Important Rule

The typing of `bind` in checking mode is the central rule that makes Gomputation's capability tracking work. It deserves special attention in implementation and testing.

```text
check(ctx, bind c1 (\x -> c2), Computation R1 R3 B):
    r2^ := fresh row existential
    a^  := fresh type existential
    ctx' := check(ctx ++ [r2^, a^], c1, Computation R1 r2^ a^)
    R2  := apply(ctx', r2^)
    A   := apply(ctx', a^)
    ctx'' := check(ctx', \x -> c2, A -> Computation R2 R3 B)
    return ctx''
```

The key insight is that the intermediate row `R2` is **never annotated by the programmer**. It is discovered during checking of `c1`: when `c1` is a primitive like `dbOpen`, its type `Computation { db : DB[Closed] | r^ } { db : DB[Opened] | r^ } Unit` unifies with `Computation R1 r2^ a^`, which solves `r2^` to `R1` with `db` updated. This solution then flows into the pre-row of `c2`, enabling the chain to continue.

Testing strategy for `bind`:

1. Test with a single bind step and a known overall type.
2. Test with a chain of 3-5 bind steps, verifying that intermediate rows are correctly inferred.
3. Test that mismatched intermediate rows produce clear errors pointing to the step where the mismatch occurs.
4. Test with open rows (row-polymorphic primitives) to verify that row variable threading works.
5. Test that an unannotated bind chain in synthesis mode correctly infers the overall type.

### 12.6 Public API

The checker exposes a minimal public API to the host application:

```go
package checker

// CheckResult contains the result of type checking a program.
type CheckResult struct {
    // The elaborated core declarations.
    Declarations []CoreDecl

    // Type checking errors (may be non-empty even if some declarations succeed).
    Errors []CheckError

    // Whether the program is well-typed (no errors).
    OK bool
}

// CheckProgram type-checks a resolved program against the given
// host primitives and ADT declarations.
func CheckProgram(
    program []ResDecl,
    primitives map[string]Type,
    adts []ADTDecl,
) CheckResult {
    c := NewChecker(primitives, adts)

    var decls []CoreDecl
    for _, d := range program {
        coreDecl := c.checkDecl(d)
        if coreDecl != nil {
            decls = append(decls, coreDecl)
        }
    }

    // Final zonk pass.
    for i := range decls {
        decls[i] = c.zonkDecl(decls[i])
    }

    return CheckResult{
        Declarations: decls,
        Errors:       c.errors,
        OK:           len(c.errors) == 0,
    }
}
```

This integrates with the overall pipeline described in the embedded language design document: the host calls `Parse`, then `Resolve`, then `CheckProgram`, then evaluates the resulting `CoreDecl` values.

---

## Key References

1. Dunfield, J. and Krishnaswami, N. R. "Complete and Easy Bidirectional Typechecking for Higher-Rank Polymorphism." ICFP 2013.
2. Dunfield, J. and Krishnaswami, N. R. "Bidirectional Typing." ACM Computing Surveys, 2021.
3. Vytiniotis, D., Peyton Jones, S., Schrijvers, T., and Sulzmann, M. "OutsideIn(X): Modular type inference with local assumptions." JFP 2011.
4. Leijen, D. "Extensible records with scoped labels." Trends in Functional Programming, 2005.
5. Remy, D. "Type Inference for Records in a Natural Extension of ML." Research Report 1431, INRIA, 1991.
6. Tarjan, R. E. "Efficiency of a Good But Not Linear Set Union Algorithm." JACM, 1975.
7. Pierce, B. C. and Turner, D. N. "Local Type Inference." ACM TOPLAS, 2000.
8. Peyton Jones, S., Vytiniotis, D., Weirich, S., and Shields, M. "Practical Type Inference for Arbitrary-Rank Types." JFP, 2007.
9. Christiansen, D. R. "Bidirectional Typing Rules: A Tutorial." 2013.
10. Bernstein, M. "Row Polymorphism from Scratch." (Implementation reference for flatten-then-diff.)
