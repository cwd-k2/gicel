# Phase 2: Core IR

## Objective

Define the 13 Core term formers that serve as the intermediate representation between type checking and evaluation. The checker elaborates surface AST into Core; the evaluator interprets Core.

## Dependencies

Phase 1 (`types/`).

## Package: `core/`

### 2.1 Core Terms (`core/core.go`)

Spec §13.1 — 13 formers (12 from spec + TyLam from elaboration requirements).

```go
package core

import (
    "github.com/cwd-k2/gomputation/types"
    "github.com/cwd-k2/gomputation/internal/span"
)

// Core is a term in the core intermediate representation.
type Core interface {
    coreNode()
    Span() span.Span
}

// Var — variable reference.
type Var struct {
    Name string
    S    span.Span
}

// Lam — lambda abstraction. ParamType is filled during elaboration.
type Lam struct {
    Param     string
    ParamType types.Type // may be nil before elaboration
    Body      Core
    S         span.Span
}

// App — function application.
type App struct {
    Fun Core
    Arg Core
    S   span.Span
}

// TyApp — type application (e @T).
type TyApp struct {
    Expr    Core
    TyArg   types.Type
    S       span.Span
}

// TyLam — type abstraction (elaboration of forall).
// Introduction form for polymorphism; TyApp is the elimination.
// Erased at runtime — evaluator skips to body.
type TyLam struct {
    TyParam string
    Kind    types.Kind
    Body    Core
    S       span.Span
}

// Con — constructor application (C e1 ... en).
type Con struct {
    Name string
    Args []Core
    S    span.Span
}

// Case — case analysis.
type Case struct {
    Scrutinee Core
    Alts      []Alt
    S         span.Span
}

// LetRec — mutually recursive bindings (reserved for future use).
type LetRec struct {
    Bindings []Binding
    Body     Core
    S        span.Span
}

// Pure — computation introduction (pure e).
type Pure struct {
    Expr Core
    S    span.Span
}

// Bind — computation sequencing (bind c (\x -> e)).
type Bind struct {
    Comp Core
    Var  string
    Body Core
    S    span.Span
}

// Thunk — suspend computation (thunk c).
type Thunk struct {
    Comp Core
    S    span.Span
}

// Force — resume suspended computation (force e).
type Force struct {
    Expr Core
    S    span.Span
}

// PrimOp — host-provided primitive operation.
type PrimOp struct {
    Name string
    Args []Core
    S    span.Span
}
```

### 2.2 Patterns and Alternatives (`core/pattern.go`)

```go
// Alt is a case alternative: pattern -> body.
type Alt struct {
    Pattern Pattern
    Body    Core
    S       span.Span
}

// Pattern in Core IR.
type Pattern interface {
    patternNode()
    Span() span.Span
    // Bindings returns variable names introduced by this pattern.
    Bindings() []string
}

// PVar — variable pattern (binds a value).
type PVar struct {
    Name string
    S    span.Span
}

// PWild — wildcard pattern.
type PWild struct {
    S span.Span
}

// PCon — constructor pattern (C p1 ... pn).
type PCon struct {
    Con    string
    Args   []Pattern
    S      span.Span
}
```

### 2.3 Bindings (`core/binding.go`)

```go
// Binding is a named definition in LetRec or top-level.
type Binding struct {
    Name string
    Type types.Type  // annotated type
    Expr Core
    S    span.Span
}

// Program is a complete Core program (top-level bindings).
type Program struct {
    DataDecls []DataDecl
    Bindings  []Binding
}

// DataDecl is a data type declaration in Core.
type DataDecl struct {
    Name     string
    TyParams []TyParam
    Cons     []ConDecl
    S        span.Span
}

// TyParam is a type parameter with its kind.
type TyParam struct {
    Name string
    Kind types.Kind
}

// ConDecl is a single constructor declaration.
type ConDecl struct {
    Name   string
    Fields []types.Type
    S      span.Span
}
```

Note: `TyParam` is defined in `core/` rather than `types/` because it binds a name to a kind — it is a syntactic form, not a type-level entity.

### 2.4 Traversal (`core/walk.go`)

```go
// Walk visits every Core node in depth-first order.
// The visitor returns false to stop traversal.
func Walk(c Core, visit func(Core) bool)

// Transform applies a function to every Core node bottom-up,
// replacing each node with the function's return value.
func Transform(c Core, f func(Core) Core) Core
```

### 2.5 Pretty Printing (`core/pretty.go`)

```go
// Pretty renders a Core term as readable pseudo-syntax.
func Pretty(c Core) string

// PrettyProgram renders a full program.
func PrettyProgram(p *Program) string
```

Output format (indented, one line per node for complex terms):

```
Bind(
  PrimOp("dbOpen"),
  _,
  Bind(
    App(PrimOp("dbQuery"), Var("sql")),
    rows,
    Bind(
      PrimOp("dbClose"),
      _,
      Pure(Var("rows")))))
```

### 2.6 Free Variables (`core/free.go`)

```go
// FreeVars returns term-level free variables in a Core expression.
func FreeVars(c Core) map[string]struct{}

// FreeTypeVars returns type-level free variables.
func FreeTypeVars(c Core) map[string]struct{}
```

## Design Notes

### Block expression representation

Block expressions `{ x := e; body }` are already desugared before reaching Core:

```
{ x := e; body }  →  App(Lam(x, body), e)
```

There is no dedicated Core node for block expressions.

### do block representation

do blocks are desugared before reaching Core:

```
do { x <- c; body }  →  Bind(c, x, body)
do { c; body }        →  Bind(c, _, body)
do { x := e; body }   →  App(Lam(x, body), e)
```

### Bind structure

`Bind` in Core has the continuation **inlined** (variable name + body), not a lambda argument. This differs from the surface `bind c (\x -> e)`. The checker transforms the lambda application form into the direct `Bind(c, x, e)` form during elaboration.

Rationale: the evaluator does not need to allocate a closure for the continuation; it binds the variable directly.

## Test Strategy

- **Construction**: build Core terms programmatically, verify structure.
- **Walk**: count nodes, collect variable names, verify depth-first order.
- **Transform**: identity transform (idempotent), variable renaming.
- **Free variables**: closed terms → empty, open terms → correct set, shadowing respected.
- **Pretty printing**: golden-string tests against expected output.
- **Pattern bindings**: `Bindings()` returns correct variable names.

## Completion Criteria

- [ ] All 13 Core formers defined and constructible (12 spec + TyLam)
- [ ] Pattern types defined with Bindings() method
- [ ] Program/DataDecl/Binding types defined
- [ ] Walk and Transform work correctly
- [ ] Pretty printing produces readable output
- [ ] No dependency beyond `types/` and `internal/span`
- [ ] All tests pass
