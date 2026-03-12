# Core Intermediate Representation and Surface-to-Core Elaboration

One-line description: design of Gomputation's core IR, its relationship to the surface language, the elaboration pass that translates between them, and concrete Go implementation guidance.

## Table of Contents

1. Why a Core Language
2. Comparison with Related Systems
3. Core Language Design Principles
4. Core Term Grammar
5. Core Type Grammar
6. Go Type Definitions for the Core AST
7. Elaboration: Surface to Core
8. Elaboration Rules for Each Surface Construct
9. Worked Examples
10. Type Annotations in the Core
11. The Evaluation Target
12. Error Messages and the Core
13. Go Implementation of the Elaboration Pass
14. Recommendations for Gomputation

---

## 1. Why a Core Language

The spec v0.1 (Section 21) and v0.2 (Section 15.4) both establish that surface syntax should compile into a minimal core language, and that computation notation should elaborate to this core rather than replace it. This section explains why the separation matters.

### 1.1 The Problem with Evaluating the Surface AST

A surface AST reflects the programmer's notation. It contains `do` blocks, operator expressions with precedence, `if-then-else`, pattern matching syntax, implicit type applications, and syntactic sugar of various kinds. Each of these forms has operational meaning, but the meaning is derivative -- each one can be expressed in terms of a smaller set of primitives.

If the evaluator works directly on the surface AST, it must handle every syntactic form. This creates several problems:

1. **The evaluator becomes large.** Every surface construct needs an evaluation case. The evaluator reimplements elaboration logic inline.

2. **The type checker must handle every surface form.** If the type checker works on the surface AST, it needs rules for `do` blocks, operator resolution, pattern matching, and every other sugar -- all of which are derivable from simpler rules.

3. **Correctness arguments fragment.** To show that evaluation preserves types, one must reason about every surface form individually. With a core language, the metatheory covers only the core; the correctness of elaboration is a separate, simpler argument.

4. **Adding surface syntax touches the evaluator.** Every new syntactic convenience requires changes to both the parser and the evaluator. With a core language, new surface forms require changes only to the parser and elaborator; the evaluator remains stable.

### 1.2 What a Core Language Provides

A core language is a small calculus with the following properties:

- **Every surface construct has a translation into core terms.** The core is expressively complete relative to the surface language.

- **The core has fewer term formers.** The evaluator handles only the core forms. The type checker (if applied to the core) handles only the core forms.

- **The core may be explicitly typed.** Where the surface language permits implicit type applications and omitted annotations, the core makes all type information explicit. This simplifies type checking of the core to type verification.

- **The core is stable.** Surface syntax changes (adding new sugar, changing operator rules) do not affect the core grammar or the evaluator.

The core is not the surface language stripped of sugar. It is a distinct calculus designed for the downstream passes (type checking, evaluation, possible future optimization). The elaboration pass is the bridge.

### 1.3 The Phase Structure

The overall pipeline that this design targets:

```text
Source Text
    |
    v
  Lexer  ------>  []Token
    |
    v
  Parser ------>  Surface AST
    |
    v
  Elaboration -->  Core IR        (this document)
    |
    v
  Type Checker -->  Typed Core IR  (bidirectional, outputs core)
    |
    v
  Evaluator ----->  Value
```

There is a design choice embedded here: does elaboration happen before or during type checking? Section 7 addresses this question in detail. The recommended answer for Gomputation is that elaboration and type checking are interleaved -- the bidirectional type checker consumes the surface AST and produces typed core terms as output. This is the "elaboration during checking" strategy, used by Idris, Agda, and (internally) GHC.

---

## 2. Comparison with Related Systems

### 2.1 GHC: Haskell to System FC

GHC's pipeline is the canonical example of the core language approach.

**Surface Haskell** is an enormous language: type classes, do-notation, list comprehensions, record syntax, pattern guards, view patterns, rebindable syntax, deriving, Template Haskell, and dozens of extensions.

**GHC Core** (System FC) is a small explicitly-typed lambda calculus with:
- Variables, lambdas, application
- Let bindings (recursive and non-recursive)
- Case expressions (with a single level of constructor matching)
- Type abstractions (big lambda) and type applications
- Coercions (explicit proof terms for type equalities)
- Literals and primitive operations

The elaboration from Haskell to Core is GHC's "desugarer," which runs after type checking and dictionary insertion. In practice, GHC's type checker (the "renamer" and "type checker" phases) produces an intermediate representation (HsSyn with type annotations), and the desugarer translates this to Core.

Key lessons for Gomputation:

- **Coercions are central to GHC Core.** They represent evidence of type equalities. Gomputation does not need coercions in v0 because it does not have type families, newtypes, or GADTs. If GADTs are added later, coercion-like evidence terms may become necessary.

- **Core is explicitly typed.** Every lambda binder has a type annotation. Every polymorphic function application includes type arguments. This makes Core independently type-checkable, which is invaluable for debugging compiler passes.

- **Core has no do-notation, no operator syntax, no pattern guards.** All of these are elaborated away. Case expressions in Core have flat constructor matching (no nested patterns).

- **Core retains let bindings.** This is a pragmatic choice: let bindings enable sharing (important for optimization), and GHC's optimizer works extensively on let/case expressions.

### 2.2 Idris: Surface to TT

Idris uses a dependently-typed core called TT (a variant of Martin-Lof type theory). The elaboration from surface Idris to TT is interleaved with type checking: the "elaborator" is a tactic-based system that builds TT terms as it type-checks the surface program.

Key lessons:

- **Elaboration during type checking** is natural for bidirectional systems. The type checker has the contextual information (expected types, solved metavariables) needed to produce the core term. Separating elaboration from type checking would require duplicating this information.

- **The elaborator resolves implicits.** In Idris, implicit arguments are represented as metavariables during elaboration. The type checker unifies them. The output core term has all implicits filled in.

- **The core is dependently typed.** TT has Pi types, Sigma types, and universes. Gomputation's core is much simpler (no dependent types), but the elaboration-during-checking strategy applies equally well.

### 2.3 PureScript: Surface to CoreFn

PureScript compiles surface syntax to CoreFn, a simplified functional core, which is then translated to JavaScript.

CoreFn contains:
- Variables, literals, applications, abstractions
- Let bindings
- Case expressions (with simple patterns)
- Constructors
- Type class dictionaries (passed as explicit arguments)

Key lessons:

- **CoreFn drops type information.** PureScript's CoreFn is essentially untyped -- type information is erased after type checking. This is appropriate for PureScript because the target is JavaScript (untyped). For Gomputation, where the evaluator is in Go and runtime type information is needed for capability checking, partial type retention is preferable.

- **CoreFn is simple.** It has approximately 10 term formers. This is a reasonable target for Gomputation's core.

### 2.4 Elm: Surface to Canonical AST

Elm has a canonical intermediate representation that is structurally similar to CoreFn. It is simple, explicitly typed enough for code generation, and serves as the boundary between front-end (parsing, type checking) and back-end (JavaScript generation).

### 2.5 Koka: Surface to Core

Koka compiles to a core language that retains effect types. Effect handlers are elaborated into explicit evidence-passing operations in the core. This is relevant because Gomputation also needs to retain capability/effect information in the core -- the evaluator must know which capability transitions to perform.

Key lesson:

- **Effect information must survive into the core if the evaluator uses it.** In Gomputation, the evaluator threads capability environments. Primitive operation names (which drive capability transitions) must be present in the core. Row types themselves can be erased (they are checked statically), but the identity of each primitive operation must be retained.

### 2.6 Summary Table

| System | Core Language | Typed Core? | Elaboration Timing | Core Complexity |
|---|---|---|---|---|
| GHC | System FC | Fully explicit | After type checking | ~10 term formers + coercions |
| Idris | TT | Fully explicit | During type checking | ~8 term formers (dependent) |
| PureScript | CoreFn | Type-erased | After type checking | ~10 term formers |
| Elm | Canonical AST | Partial | After type checking | ~10 term formers |
| Koka | Core | Effect-annotated | During type checking | ~12 term formers |
| **Gomputation** | **Proposed** | **Annotated binders** | **During type checking** | **~12 term formers** |

---

## 3. Core Language Design Principles

### 3.1 Minimality

The core should have the smallest number of term formers that are expressively complete. Every surface construct must be translatable to the core, but the core should not have redundant forms.

The practical test: if a term former can be expressed as a composition of other term formers without loss of information needed by downstream passes, it should not be in the core.

### 3.2 The Value/Computation Split

The spec v0.2 (Section 5) commits to the value/computation distinction. The core should preserve this distinction syntactically: there should be value terms and computation terms, with different type structures.

This is a design choice, not a logical necessity. One could define a core without the split (making `pure` and `bind` ordinary functions). But preserving the split in the core has concrete benefits:

- The evaluator can use different evaluation functions for values and computations (as shown in the evaluation semantics document, Section 5.5).
- The capability environment is threaded only through computation evaluation.
- The type checker can enforce the split structurally.

### 3.3 Explicit Type Information

The core should carry enough type information for independent type verification. The recommended level for Gomputation v0:

- **Lambda binders are annotated.** Every `\x -> e` in the core becomes `\(x : A) -> e`.
- **Type applications are explicit.** Every use of a polymorphic value includes type arguments: `f @Int` rather than bare `f`.
- **Row applications are explicit.** `dbOpen @{log : Logger[Ready]}` rather than bare `dbOpen`.
- **Computation types are not attached to every subterm.** The pre/post rows of each computation step are inferrable from the types of the subterms. The core does not annotate every `bind` with intermediate row types. The type checker can re-derive these if needed.

This level of annotation -- binders and applications, but not every subexpression -- is a pragmatic middle ground. It is sufficient for type verification without being as verbose as GHC Core.

### 3.4 Flat Pattern Matching

Surface pattern matching may have deeply nested patterns, guards, and multiple equations. The core should have only flat, single-level case analysis on constructors. The elaboration pass compiles nested patterns to decision trees (as described in the pattern matching document) and emits nested flat `case` expressions.

### 3.5 No Operators, No Do-Notation

The core has no infix operators and no do-notation. Operators are elaborated to function applications. Do blocks are elaborated to `pure`/`bind` chains. This eliminates precedence, associativity, and statement-level semantics from the core.

### 3.6 No If-Then-Else

`if-then-else` is sugar for `case` on a boolean. The core has `case`; it does not need `if`.

---

## 4. Core Term Grammar

### 4.1 Value Expressions

```text
val ::= x                                -- variable
      | n                                -- integer literal
      | s                                -- string literal
      | ()                               -- unit literal
      | \(x : T) -> val                  -- annotated lambda abstraction
      | val val                           -- application
      | val @T                            -- type application
      | C val ... val                     -- constructor application
      | case val of { alt* }              -- case analysis (flat patterns)
      | let x : T = val in val            -- non-recursive let binding
      | letrec x : T = val in val         -- recursive let binding
```

### 4.2 Computation Expressions

```text
comp ::= pure val                         -- lift value to computation
       | bind comp (\(x : T) -> comp)     -- sequencing
       | prim(op, val, ..., val)           -- primitive host operation
       | val_comp                          -- a value expression that has
                                           --   computation type (e.g., an
                                           --   application whose result is
                                           --   Computation pre post a)
```

### 4.3 Case Alternatives

```text
alt ::= C x1 ... xn -> body              -- constructor pattern
      | _ -> body                          -- default (wildcard)
```

Patterns are flat: a single constructor applied to variable binders. No nested patterns, no literal patterns (literal matching is elaborated to equality checks), no guards.

The `body` in an alternative is a value expression. If the case is used in a computation context, the body is a value expression that produces a computation (i.e., a `val_comp` term).

### 4.4 Term Formers: Summary

| # | Form | Category | Purpose |
|---|---|---|---|
| 1 | `Var x` | Value | Variable reference |
| 2 | `Lit n / s / ()` | Value | Literal constants |
| 3 | `Lam (x : T) body` | Value | Lambda abstraction |
| 4 | `App f a` | Value | Function application |
| 5 | `TyApp e T` | Value | Type/row application |
| 6 | `Con C args` | Value | Constructor application |
| 7 | `Case scrut alts` | Value | Case analysis |
| 8 | `Let x T rhs body` | Value | Non-recursive binding |
| 9 | `LetRec x T rhs body` | Value | Recursive binding |
| 10 | `Pure v` | Computation | Value-to-computation lift |
| 11 | `Bind c (x : T) c'` | Computation | Computation sequencing |
| 12 | `PrimOp op args` | Computation | Host primitive invocation |

This gives 12 term formers (counting literal variants as one), which is in line with GHC Core and PureScript CoreFn.

### 4.5 Design Decisions

**Why `Let` and `LetRec` in the core?** The surface language uses top-level definitions (`Name := Expr`). A program is a sequence of definitions, which elaborates to nested let bindings in the core. The `let` form is also needed for desugaring pure bindings in `do` blocks (`x := e; rest` becomes `let x = e in rest`). Recursive let is needed because top-level definitions may be mutually recursive.

**Why `TyApp` as a separate form?** The surface language has implicit instantiation in many cases (rank-1 polymorphism). The core makes all instantiations explicit. The type checker resolves implicit instantiation during elaboration and emits explicit `TyApp` nodes. This means the core is closer to System F than to Damas-Milner.

**Why `val_comp`?** Some value expressions produce computation types. For example, `f x` where `f : Int -> Computation r1 r2 String` produces a computation. At the core level, this is still a value-layer application (the function is applied to the argument), but the result type is `Computation r1 r2 String`. The evaluator handles this by switching from value evaluation to computation evaluation when the result of a value expression is a computation. In practice, this means `Bind` takes either a `PrimOp`, a `Pure`, another `Bind`, or a value expression whose type is a computation type.

A cleaner alternative: make the computation language a little richer so that `Bind` takes only computation expressions, and function application of effectful functions is explicitly wrapped. The recommended approach is to keep the grammar as stated but implement the evaluator with a check: when evaluating the first argument of `Bind`, determine at runtime whether it is a `Pure`, `Bind`, `PrimOp`, or a value expression that needs value evaluation followed by computation interpretation.

---

## 5. Core Type Grammar

The core type language mirrors the surface type language but without ambiguity or implicit structure.

```text
T ::= a                                   -- type variable
    | T -> T                               -- function type
    | forall (a : K). T                    -- universal quantification (explicit kind)
    | Computation R R T                    -- indexed computation type
    | C T ... T                            -- type constructor application
    | (T, T, ..., T)                       -- tuple type (future)

R ::= {}                                   -- empty row
    | { l : T | R }                        -- row extension
    | r                                    -- row variable

K ::= Type                                -- kind of value types
    | Row                                  -- kind of rows
    | K -> K                               -- kind of type constructors
```

### 5.1 Differences from the Surface Type Language

- **Kind annotations are explicit on all quantified variables.** The surface `forall a. T` becomes `forall (a : Type). T` in the core. The surface `forall r. T` (where `r` is a row variable) becomes `forall (r : Row). T`.

- **Type constructor application is explicit.** The surface `Maybe a` is `Maybe a` in the core as well, but the distinction between type variable and type constructor is always unambiguous (constructors begin with uppercase, variables with lowercase -- this is already true in the surface language).

- **Rows are in canonical form.** Labels are sorted. This eliminates row equality up to permutation as a concern for the core.

---

## 6. Go Type Definitions for the Core AST

### 6.1 Source Span

```go
// Span records the source location of a core term.
// Elaboration preserves source spans from the surface AST
// so that errors referencing core terms can point to source locations.
type Span struct {
    Start Position
    End   Position
}

type Position struct {
    Line   int
    Col    int
    Offset int
}
```

### 6.2 Value Expressions

```go
// ValueExpr is the interface for all core value-level expressions.
type ValueExpr interface {
    valueExpr()
    ExprSpan() Span
}

type VarExpr struct {
    Name string
    S    Span
}

type LitInt struct {
    Value int64
    S     Span
}

type LitString struct {
    Value string
    S     Span
}

type LitUnit struct {
    S Span
}

type LamExpr struct {
    Param    string
    ParamTy  Type     // always present in core
    Body     ValueExpr
    S        Span
}

type AppExpr struct {
    Func ValueExpr
    Arg  ValueExpr
    S    Span
}

type TyAppExpr struct {
    Expr  ValueExpr
    TyArg Type
    S     Span
}

type ConExpr struct {
    Tag  string      // constructor name
    Args []ValueExpr
    S    Span
}

type CaseExpr struct {
    Scrutinee ValueExpr
    Alts      []CaseAlt
    S         Span
}

type CaseAlt struct {
    ConTag   string   // constructor name, or "_" for default
    Bindings []string // bound variable names
    Body     ValueExpr
}

type LetExpr struct {
    Name string
    Ty   Type
    Rhs  ValueExpr
    Body ValueExpr
    S    Span
}

type LetRecExpr struct {
    Name string
    Ty   Type
    Rhs  ValueExpr
    Body ValueExpr
    S    Span
}
```

### 6.3 Computation Expressions

```go
// CompExpr is the interface for all core computation-level expressions.
type CompExpr interface {
    compExpr()
    ExprSpan() Span
}

type PureExpr struct {
    Value ValueExpr
    S     Span
}

type BindExpr struct {
    Comp CompExpr
    Var  string
    VarTy Type      // type of the intermediate result
    Cont CompExpr   // continuation (body after binding)
    S    Span
}

type PrimOpExpr struct {
    OpName string
    Args   []ValueExpr
    S      Span
}

// CompFromValue wraps a value expression whose type is
// Computation pre post a. This occurs when a function application
// or variable reference produces a computation.
type CompFromValue struct {
    Value ValueExpr
    S     Span
}
```

### 6.4 Type Representations

```go
// Type is the core type representation.
type Type interface {
    typeNode()
}

type TyVar struct {
    Name string
}

type TyArrow struct {
    Domain   Type
    Codomain Type
}

type TyForall struct {
    Var  string
    Kind Kind
    Body Type
}

type TyComp struct {
    Pre    Row
    Post   Row
    Result Type
}

type TyApp struct {
    Con  string  // type constructor name
    Args []Type
}

type TyTuple struct {
    Elems []Type
}
```

### 6.5 Row Representations

```go
// Row is the core row representation. Rows are always canonical
// (labels sorted, no duplicates).
type Row interface {
    rowNode()
}

type RowEmpty struct{}

type RowExtend struct {
    Label   string
    FieldTy Type
    Tail    Row
}

type RowVar struct {
    Name string
}
```

### 6.6 Kind Representations

```go
type Kind interface {
    kindNode()
}

type KindType struct{}

type KindRow struct{}

type KindArrow struct {
    Domain   Kind
    Codomain Kind
}
```

### 6.7 Program and Declarations

At the core level, a program is a single expression (a nested `LetRec` chain) rather than a list of declarations. Top-level declarations are elaborated into core let bindings. However, it is practical to retain a declaration-level structure in the core for better correspondence with the surface program:

```go
// CoreProgram is the top-level core representation.
type CoreProgram struct {
    Decls []CoreDecl
}

// CoreDecl is a top-level core declaration.
type CoreDecl interface {
    coreDecl()
}

type CoreValueDecl struct {
    Name string
    Ty   Type
    Body ValueExpr
    S    Span
}

type CoreDataDecl struct {
    Name   string
    Params []TypeParam
    Cons   []CoreConDecl
    S      Span
}

type CoreConDecl struct {
    Name   string
    Fields []Type
}

type TypeParam struct {
    Name string
    Kind Kind
}

type CorePrimDecl struct {
    Name string
    Ty   Type
    S    Span
}
```

---

## 7. Elaboration: Surface to Core

### 7.1 When Does Elaboration Happen?

There are three possible timings for elaboration relative to type checking:

**Option A: Elaborate first, then type check the core.** The surface AST is translated to core without type information. The type checker then works on the core. This is conceptually clean but problematic: the elaboration pass needs type information to resolve implicit type applications, to insert type annotations on lambda binders, and to desugar pattern matching correctly (exhaustiveness depends on types).

**Option B: Type check the surface, then elaborate.** The type checker works on the surface AST and annotates it with types. A subsequent elaboration pass translates the annotated surface AST to core. This is GHC's approach (the desugarer runs on type-annotated HsSyn). It works, but the type checker must handle all surface forms.

**Option C: Elaborate during type checking.** The bidirectional type checker consumes the surface AST and produces core terms as output. When the type checker processes a `do` block, it simultaneously type-checks the statements and emits the corresponding `Bind`/`Pure` chain. When it processes a lambda without annotation, it obtains the parameter type from checking mode and emits an annotated `LamExpr`. This is Idris's approach and the recommended approach for Gomputation.

### 7.2 Why "Elaborate During Checking" Is Right for Gomputation

1. **Bidirectional typing naturally produces annotated terms.** When checking `\x -> e` against `A -> B`, the checker knows that `x : A`. It can immediately emit `LamExpr{Param: "x", ParamTy: A, Body: ...}`. No separate annotation pass is needed.

2. **Implicit instantiation resolution is part of type checking.** When a polymorphic variable `f : forall a. a -> a` is used at type `Int -> Int`, the type checker solves `a = Int`. It can immediately emit `TyAppExpr{Expr: VarExpr{"f"}, TyArg: TyApp{"Int", nil}}`. A pre-checking elaboration pass cannot do this because it does not know `a = Int` yet.

3. **Do-notation desugaring is compatible with either timing.** Desugaring `do` blocks to `bind`/`pure` chains can happen before type checking (it is a purely syntactic transformation). However, if desugaring and checking are interleaved, the checker can propagate the expected computation type into each statement, which improves error messages: the error says "in the third statement of the do block" rather than "in the second argument of the third bind call."

4. **Pattern compilation needs type information.** Exhaustiveness checking depends on knowing which constructors a type has. This information is available during type checking. Compiling patterns to flat case expressions is therefore best done during or after type checking.

### 7.3 The Recommended Architecture

```text
Surface AST  --->  Bidirectional Type Checker / Elaborator  --->  Core IR
                          |                                          |
                   uses: type environment,                    result: explicitly typed
                         operator table,                             core terms
                         constructor table,
                         primitive signatures
```

The type checker / elaborator is a pair of mutually recursive functions:

```text
check(ctx, surfaceExpr, expectedType) -> (coreExpr, updatedCtx)
synth(ctx, surfaceExpr) -> (coreExpr, inferredType, updatedCtx)
```

Each function returns a core expression as part of its output. The core expression is the elaborated form of the surface expression, with all type information filled in.

### 7.4 Pre-Elaboration Simplifications

Some surface-to-surface transformations can happen before type checking, as a syntactic preprocessing step:

1. **Operator resolution.** Convert infix expressions to function application using the fixity table. `a + b` becomes `(+) a b`. This is purely syntactic and does not require type information.

2. **If-then-else to case.** `if e then a else b` becomes `case e of { True -> a; False -> b }`. This is purely syntactic.

3. **Multi-parameter lambdas to nested lambdas.** `\x y -> e` becomes `\x -> \y -> e`. Purely syntactic.

4. **Negation to function application.** `-e` becomes `negate e`. Purely syntactic.

These transformations produce a "desugared surface AST" that is still in terms of surface AST types but has fewer forms. The type checker / elaborator then operates on this simplified surface AST.

---

## 8. Elaboration Rules for Each Surface Construct

This section specifies how each surface construct maps to core terms. The rules are given as translation schemas: `[[e]]` denotes the core translation of surface expression `e`.

### 8.1 Variables

```text
Surface:  x
Core:     VarExpr{Name: "x"}
```

No transformation. Variables map directly.

### 8.2 Literals

```text
Surface:  42
Core:     LitInt{Value: 42}

Surface:  "hello"
Core:     LitString{Value: "hello"}

Surface:  ()
Core:     LitUnit{}
```

Direct mapping.

### 8.3 Lambda Abstraction

**Unannotated lambda (type comes from checking mode):**

```text
Surface:  \x -> e
Context:  checking against A -> B

Core:     LamExpr{Param: "x", ParamTy: A, Body: [[e]] checked against B}
```

The parameter type `A` is obtained from the expected function type in checking mode.

**Annotated lambda:**

```text
Surface:  \(x :: A) -> e
Core:     LamExpr{Param: "x", ParamTy: [[A]], Body: [[e]]}
```

The annotation is preserved verbatim.

**Multi-parameter lambda (pre-desugared):**

```text
Surface:  \x y z -> e
Desugar:  \x -> \y -> \z -> e
Core:     LamExpr{..., Body: LamExpr{..., Body: LamExpr{..., Body: [[e]]}}}
```

### 8.4 Function Application

```text
Surface:  f x
Core:     AppExpr{Func: [[f]], Arg: [[x]]}
```

Left-associative application is already handled by the parser: `f x y` parses as `App(App(f, x), y)`.

### 8.5 Type Application

**Explicit type application:**

```text
Surface:  f @Int
Core:     TyAppExpr{Expr: [[f]], TyArg: TyApp{"Int", nil}}
```

**Implicit instantiation (resolved by checker):**

When a polymorphic variable `f : forall a. T` is used in a context that determines `a = S`, the checker emits:

```text
Core:     TyAppExpr{Expr: VarExpr{"f"}, TyArg: S}
```

The programmer writes `f`; the elaborator outputs `f @S`. This is the fundamental service that elaboration-during-checking provides for polymorphism.

### 8.6 Operator Expressions

**Binary operator (pre-desugared):**

```text
Surface:  a + b
Desugar:  (+) a b
Core:     AppExpr{Func: AppExpr{Func: VarExpr{"+"}, Arg: [[a]]}, Arg: [[b]]}
```

Operators are resolved to variables and applied as curried functions. The fixity table determines the tree structure (which operand is `a` and which is `b`), but the core representation is uniform function application.

**Operator sections (pre-desugared):**

```text
Surface:  (+ 1)
Desugar:  \x -> (+) x 1
Core:     LamExpr{Param: "x", ParamTy: ..., Body: AppExpr{...}}

Surface:  (1 +)
Desugar:  \x -> (+) 1 x
Core:     LamExpr{Param: "x", ParamTy: ..., Body: AppExpr{...}}

Surface:  (+)
Core:     VarExpr{"+"}
```

### 8.7 If-Then-Else

```text
Surface:  if e then a else b
Desugar:  case e of { True -> a; False -> b }
Core:     CaseExpr{
            Scrutinee: [[e]],
            Alts: [
              CaseAlt{ConTag: "True",  Bindings: [], Body: [[a]]},
              CaseAlt{ConTag: "False", Bindings: [], Body: [[b]]},
            ],
          }
```

### 8.8 Case Expressions

**Simple (already flat) patterns:**

```text
Surface:  case x of { Nothing -> 0; Just y -> y }
Core:     CaseExpr{
            Scrutinee: [[x]],
            Alts: [
              CaseAlt{ConTag: "Nothing", Bindings: [],    Body: LitInt{0}},
              CaseAlt{ConTag: "Just",    Bindings: ["y"], Body: VarExpr{"y"}},
            ],
          }
```

**Nested patterns (compiled to flat case):**

```text
Surface:  case x of { Just (Just y) -> y; _ -> 0 }

Core:     CaseExpr{
            Scrutinee: [[x]],
            Alts: [
              CaseAlt{ConTag: "Just", Bindings: ["_tmp0"],
                Body: CaseExpr{
                  Scrutinee: VarExpr{"_tmp0"},
                  Alts: [
                    CaseAlt{ConTag: "Just", Bindings: ["y"],
                      Body: VarExpr{"y"}},
                    CaseAlt{ConTag: "_", Bindings: [],
                      Body: LitInt{0}},
                  ],
                },
              },
              CaseAlt{ConTag: "_", Bindings: [],
                Body: LitInt{0}},
            ],
          }
```

The pattern compilation algorithm (described in the pattern matching document) produces this nested flat-case structure. Each level matches a single constructor.

**Literal patterns:**

```text
Surface:  case n of { 0 -> "zero"; 1 -> "one"; _ -> "other" }

Core:     -- elaborated using equality checks:
          CaseExpr on (n == 0) with True/False branches,
          nested CaseExpr on (n == 1), etc.
```

Literal patterns are compiled to chains of equality tests.

### 8.9 Do Blocks

Do blocks are the most complex elaboration. Each statement form maps differently.

**Computation binding (`x <- e`):**

```text
Surface:  do { x <- c1; rest }
Core:     BindExpr{
            Comp: [[c1]],
            Var:  "x",
            VarTy: A,         -- type of x, from type checking
            Cont: [[rest]],
          }
```

**Pure binding (`x := e`):**

```text
Surface:  do { x := e; rest }
Core:     -- within computation context, this is a let binding:
          -- the value e is bound, and rest continues as a computation
          CompFromValue{
            Value: LetExpr{
              Name: "x",
              Ty:   A,
              Rhs:  [[e]],
              Body: ... -- rest needs to be in value position
            },
          }
```

In practice, the cleanest elaboration of pure bindings in do blocks depends on the surrounding computation context. There are two approaches:

**Approach A (bind with pure):** Treat `x := e` as `x <- pure e`:

```text
Surface:  do { x := e; rest }
Core:     BindExpr{
            Comp: PureExpr{Value: [[e]]},
            Var:  "x",
            VarTy: A,
            Cont: [[rest]],
          }
```

This is semantically clean and uniform: every do-statement becomes a `Bind`. The cost is one unnecessary `Bind`/`Pure` pair, which can be optimized away.

**Approach B (let binding):** The pure binding becomes a `let` in the continuation:

```text
Surface:  do { x := e; rest }
Core:     -- rest is a computation; we substitute e for x in rest
          -- In the core, this is represented by wrapping the continuation:
          (the continuation [[rest]] with [[e]] substituted for x)
```

This avoids the extra `Bind`/`Pure` but requires the elaborator to handle `let` bindings within computation contexts.

**Recommendation:** Use Approach A for simplicity. `x := e` elaborates to `bind (pure e) (\x -> rest)`. The evaluator's `Bind` + `Pure` case reduces to a simple variable binding (as specified in the evaluation semantics: `bind (pure v) k --> k v`). An optional optimization pass can inline this pattern.

**Expression statement (`e;`):**

```text
Surface:  do { e; rest }
Core:     BindExpr{
            Comp: [[e]],    -- e must have type Computation r1 r2 Unit
            Var:  "_",
            VarTy: Unit,
            Cont: [[rest]],
          }
```

The result is discarded (bound to `_`).

**Tail expression:**

```text
Surface:  do { e }
Core:     [[e]]             -- the expression itself, which must be a computation
```

The tail expression is the final expression in the do block. It is not wrapped in anything; it is the computation returned by the block.

**Complete do block example:**

```text
Surface:
  do {
    x := 1;
    openDB;
    rows <- query x;
    closeDB;
    pure rows
  }

Core:
  BindExpr{
    Comp: PureExpr{Value: LitInt{1}},
    Var: "x", VarTy: Int,
    Cont: BindExpr{
      Comp: PrimOpExpr{OpName: "openDB", Args: []},
      Var: "_", VarTy: Unit,
      Cont: BindExpr{
        Comp: CompFromValue{
          Value: AppExpr{
            Func: VarExpr{"query"},
            Arg: VarExpr{"x"},
          },
        },
        Var: "rows", VarTy: Rows,
        Cont: BindExpr{
          Comp: PrimOpExpr{OpName: "closeDB", Args: []},
          Var: "_", VarTy: Unit,
          Cont: PureExpr{Value: VarExpr{"rows"}},
        },
      },
    },
  }
```

### 8.10 Type Annotations

**Expression-level annotation:**

```text
Surface:  (e :: T)
Core:     [[e]]   -- the annotation is consumed by the type checker;
                   -- it directs checking mode but does not appear in the core
```

Type annotations in the surface language switch the checker from synthesis to checking mode. They do not produce core nodes because the core already has type information on binders and type applications.

### 8.11 Constructor Application

```text
Surface:  Just x
Core:     ConExpr{Tag: "Just", Args: [VarExpr{"x"}]}
```

### 8.12 Negation

```text
Surface:  -x
Desugar:  negate x
Core:     AppExpr{Func: VarExpr{"negate"}, Arg: [[x]]}
```

### 8.13 Top-Level Declarations

**Type annotation + value definition pair:**

```text
Surface:
  f :: forall a. a -> a
  f := \x -> x

Core:
  CoreValueDecl{
    Name: "f",
    Ty:   TyForall{"a", KindType{}, TyArrow{TyVar{"a"}, TyVar{"a"}}},
    Body: TyAppExpr is not needed here -- the definition is polymorphic;
          the core body is:
          LamExpr{Param: "x", ParamTy: TyVar{"a"},
                  Body: VarExpr{"x"}},
  }
```

Polymorphic definitions in the core retain their `forall` in the type. The body is checked under the assumption that `a` is a type variable. Instantiation happens at use sites, not at the definition.

**Data declarations:**

```text
Surface:
  data Maybe a = Nothing | Just a

Core:
  CoreDataDecl{
    Name:   "Maybe",
    Params: [{Name: "a", Kind: KindType{}}],
    Cons: [
      CoreConDecl{Name: "Nothing", Fields: []},
      CoreConDecl{Name: "Just",    Fields: [TyVar{"a"}]},
    ],
  }
```

**Primitive declarations:**

```text
Surface:
  primitive dbOpen :: forall r. Computation {db : DB[Closed] | r}
                                            {db : DB[Opened] | r}
                                            Unit

Core:
  CorePrimDecl{
    Name: "dbOpen",
    Ty:   TyForall{"r", KindRow{},
            TyComp{
              Pre:  RowExtend{"db", TyApp{"DB", [TyApp{"Closed", nil}]},
                      RowVar{"r"}},
              Post: RowExtend{"db", TyApp{"DB", [TyApp{"Opened", nil}]},
                      RowVar{"r"}},
              Result: TyApp{"Unit", nil},
            },
          },
  }
```

### 8.14 Fixity Declarations

Fixity declarations do not produce core terms. They are consumed during the pre-elaboration operator resolution phase and do not appear in the core program.

---

## 9. Worked Examples

### 9.1 Identity Function

```text
Surface:
  id :: forall a. a -> a
  id := \x -> x

Core:
  CoreValueDecl{
    Name: "id",
    Ty:   forall (a : Type). a -> a,
    Body: Lam (x : a) -> Var x,
  }
```

Use site:

```text
Surface:  id 42
Core:     App (TyApp (Var "id") Int) (LitInt 42)
```

The type checker determines that `a = Int` and inserts the type application.

### 9.2 Function Composition

```text
Surface:
  compose :: forall a b c. (b -> c) -> (a -> b) -> (a -> c)
  compose := \f -> \g -> \x -> f (g x)

Core:
  CoreValueDecl{
    Name: "compose",
    Ty:   forall (a : Type). forall (b : Type). forall (c : Type).
            (b -> c) -> (a -> b) -> (a -> c),
    Body: Lam (f : b -> c) ->
            Lam (g : a -> b) ->
              Lam (x : a) ->
                App (Var "f") (App (Var "g") (Var "x")),
  }
```

### 9.3 Database Transaction

```text
Surface:
  transaction :: Computation {db : DB[Closed]} {db : DB[Closed]} String
  transaction := do {
    openDB;
    rows <- query "select 1";
    text := render rows;
    closeDB;
    pure text
  }

Core:
  CoreValueDecl{
    Name: "transaction",
    Ty:   Computation {db : DB[Closed]} {db : DB[Closed]} String,
    Body: Bind
            (PrimOp "openDB" [])           -- Comp {db:Closed} {db:Opened} Unit
            (_ : Unit)
            (Bind
              (CompFromValue
                (App (TyApp (Var "query") ...) (LitString "select 1")))
                                            -- Comp {db:Opened} {db:Opened} Rows
              (rows : Rows)
              (Bind
                (Pure (App (Var "render") (Var "rows")))
                                            -- Comp {db:Opened} {db:Opened} String
                (text : String)
                (Bind
                  (PrimOp "closeDB" [])     -- Comp {db:Opened} {db:Closed} Unit
                  (_ : Unit)
                  (Pure (Var "text"))        -- Comp {db:Closed} {db:Closed} String
                )
              )
            ),
  }
```

The row variable in `openDB : forall r. Computation {db:Closed|r} {db:Opened|r} Unit` is instantiated to `{}` (the empty row) because the top-level type has a closed row `{db : DB[Closed]}`. The core representation includes the type application `TyApp (Var "openDB") RowEmpty{}`.

### 9.4 Pattern Matching with Nested Patterns

```text
Surface:
  extract :: Maybe (Maybe Int) -> Int
  extract := \x -> case x of {
    Just (Just n) -> n;
    _ -> 0
  }

Core:
  CoreValueDecl{
    Name: "extract",
    Ty:   Maybe (Maybe Int) -> Int,
    Body: Lam (x : Maybe (Maybe Int)) ->
            Case (Var "x") [
              Alt "Just" ["_tmp0"]
                (Case (Var "_tmp0") [
                  Alt "Just" ["n"] (Var "n"),
                  Alt "_"   []     (LitInt 0),
                ]),
              Alt "_" [] (LitInt 0),
            ],
  }
```

The nested pattern `Just (Just n)` is compiled to two levels of flat case matching. A fresh variable `_tmp0` is introduced for the intermediate scrutinee.

### 9.5 Row-Polymorphic Function

```text
Surface:
  withDB :: forall r a.
    (Computation {db : DB[Opened] | r} {db : DB[Opened] | r} a) ->
    Computation {db : DB[Closed] | r} {db : DB[Closed] | r} a
  withDB := \action -> do {
    openDB;
    result <- action;
    closeDB;
    pure result
  }

Core:
  CoreValueDecl{
    Name: "withDB",
    Ty:   forall (r : Row). forall (a : Type).
            (Computation {db:Opened|r} {db:Opened|r} a) ->
            Computation {db:Closed|r} {db:Closed|r} a,
    Body: Lam (action : Computation {db:Opened|r} {db:Opened|r} a) ->
            Bind
              (PrimOp "openDB" [])
                  -- openDB instantiated with row var r:
                  -- TyApp (Var "openDB") (RowVar "r")
              (_ : Unit)
              (Bind
                (CompFromValue (Var "action"))
                (result : a)
                (Bind
                  (PrimOp "closeDB" [])
                  (_ : Unit)
                  (Pure (Var "result"))
                )
              ),
  }
```

The row variable `r` remains free in the core body. It is bound by the `forall (r : Row)` in the type. The type applications on `openDB` and `closeDB` instantiate them with `RowVar "r"`.

---

## 10. Type Annotations in the Core

### 10.1 What Is Annotated

The following positions carry type information in the core:

1. **Lambda binders.** Every `LamExpr` has a `ParamTy` field.
2. **Bind result variables.** Every `BindExpr` has a `VarTy` field for the type of the intermediate result.
3. **Type applications.** Every `TyAppExpr` has a `TyArg` field.
4. **Let bindings.** Every `LetExpr` and `LetRecExpr` has a `Ty` field.
5. **Top-level declarations.** Every `CoreValueDecl` has a `Ty` field.

### 10.2 What Is Not Annotated

The following are not annotated in the core (they can be derived from the annotations above):

1. **Every subexpression.** The core does not attach a type to every AST node. A separate type-reconstruction pass could compute types for all nodes, but this is not necessary for evaluation.

2. **Constructor fields.** The types of constructor arguments are determined by the data declaration and the constructor name.

3. **Case scrutinees.** The type of the scrutinee is the return type of the previous expression.

4. **Intermediate rows in bind chains.** The pre and post rows of each `Bind` step are determined by the types of the subexpressions.

### 10.3 Fully Explicit vs. Annotated Binders

Two ends of the spectrum:

**Fully explicit (GHC Core style):** Every binder is annotated. Every polymorphic use includes type arguments. Every case expression is annotated with its result type. Coercions carry proof terms for type equalities. This allows the core to be type-checked independently without any inference. The cost is verbosity.

**Annotated binders only (recommended for Gomputation):** Lambda binders and let bindings carry type annotations. Type applications are explicit. But not every subexpression is annotated, and there are no coercion terms. This is sufficient because:

- The core can be type-verified by a simple traversal that propagates types through applications and case expressions.
- Gomputation does not have type families or newtypes, so there are no coercions to represent.
- The evaluator does not use type information (types can be erased for evaluation).

### 10.4 Type Erasure

For evaluation, all type information can be erased:

- `TyAppExpr` nodes are dropped (they have no runtime effect).
- `LamExpr.ParamTy` is ignored by the evaluator.
- `BindExpr.VarTy` is ignored by the evaluator.
- `forall` quantifiers disappear.

The evaluator operates on the term structure only. This is correct because Gomputation's type system has no runtime type dispatch (no typecase, no dynamic casts, no type-passing polymorphism).

The exception: **primitive operations** must retain their identity at runtime (the evaluator needs to know which host function to call). But this is the operation name, not the type.

---

## 11. The Evaluation Target

### 11.1 Tree-Walking on the Core

For Gomputation v0, the evaluator should be a tree-walking interpreter over the core AST. This is the simplest architecture and aligns with the big-step semantics defined in the evaluation semantics document.

The core AST is the direct input to the evaluator. There is no further lowering to ANF, CPS, bytecode, or SSA.

The evaluator consists of the two functions defined in the evaluation semantics document (Section 5.5):

```text
evalValue(env, valueExpr) -> Value
evalComp(env, capEnv, compExpr) -> (capEnv', Value)
```

These functions pattern-match on the core AST nodes.

### 11.2 Why Not ANF or CPS?

**A-Normal Form (ANF)** is a representation where every subexpression is either a value or a let-bound intermediate result. It makes evaluation order syntactically explicit and simplifies certain optimizations (common subexpression elimination, dead code elimination). However, for a tree-walking interpreter, ANF provides no benefit: the interpreter already evaluates in the order dictated by the AST structure, and the additional let bindings add overhead.

**Continuation-Passing Style (CPS)** transforms every function to take an explicit continuation argument. It makes control flow explicit and is useful for implementing coroutines, delimited continuations, or tail-call optimization. Gomputation does not need these features in v0.

**Recommendation:** Use the core AST directly as the evaluation target. Consider ANF or bytecode compilation as a future optimization if performance becomes a concern.

### 11.3 Future: Bytecode or Slot-Indexed Evaluation

If performance matters later, two approaches are available:

1. **Slot-indexed variables.** A compilation pass assigns each variable a numeric index (as Starlark-go does). Variable lookup becomes array indexing instead of map lookup or linked-list traversal.

2. **Bytecode compilation.** The core AST is compiled to a sequence of bytecodes for a stack machine or register machine. This eliminates AST traversal overhead and is suitable for high-performance interpreters.

Both of these operate on the core AST as input. The core language is stable; the bytecode compiler is an optional backend.

---

## 12. Error Messages and the Core

### 12.1 The Problem

If the type checker works on the core, error messages reference core terms, not surface terms. A message like "type mismatch in the second argument of bind" is incomprehensible to a user who wrote a `do` block.

### 12.2 The Solution: Source Spans

Every core AST node carries a `Span` field that records the source position of the original surface construct. When the elaborator produces a `BindExpr` from a do-block statement, the `Span` points to the `<-` operator in the surface source.

Error messages are generated using the `Span`, not the core term structure. The error renderer looks up the surface source location from the span and formats the message in terms of the surface syntax:

```text
error at line 5, col 3:
  rows <- query x
  ^^^^^^^^^^^^^^^^
  type mismatch: expected Computation {db:Opened} {db:Opened} Rows
                      got Computation {db:Closed} {db:Opened} Rows
```

### 12.3 Elaboration-During-Checking and Error Quality

The "elaborate during checking" approach has an inherent advantage for error quality: the type checker has the surface AST in hand at the point where it detects the error. It does not need to reverse-map from core to surface. The error message can reference the surface form directly, using the surface AST node's span.

This is one of the strongest practical arguments for elaboration during checking.

### 12.4 Error Mapping Table

| Error situation | Surface context | Core context | Error message references |
|---|---|---|---|
| Type mismatch in do binding | `x <- e` | `Bind(e, x, cont)` | The `<-` statement |
| Wrong arity for constructor | `Just x y` | `ConExpr{...}` | The constructor application |
| Non-exhaustive case | `case e of {...}` | `CaseExpr{...}` | The `case` expression |
| Unbound variable | `foo` | `VarExpr{"foo"}` | The variable name |
| Row mismatch in bind chain | `do { openDB; query ... }` | `Bind(PrimOp "openDB", ..., Bind(PrimOp "query", ...))` | The second statement |

---

## 13. Go Implementation of the Elaboration Pass

### 13.1 The Elaborator as Part of the Type Checker

In the recommended architecture, the elaborator is not a separate pass. It is woven into the bidirectional type checker. The type checker functions return core AST nodes alongside type information.

```go
// check checks a surface expression against an expected type
// and returns the elaborated core expression.
func (tc *TypeChecker) check(
    ctx *Context,
    expr surface.Expr,
    expected Type,
) (ValueExpr, *Context, error) {
    switch e := expr.(type) {
    case *surface.Lambda:
        return tc.checkLambda(ctx, e, expected)
    case *surface.DoExpr:
        return tc.checkDo(ctx, e, expected)
    // ...
    default:
        // subsumption: synthesize, then check against expected
        core, inferred, ctx2, err := tc.synth(ctx, expr)
        if err != nil {
            return nil, nil, err
        }
        ctx3, err := tc.unify(ctx2, inferred, expected)
        if err != nil {
            return nil, nil, err
        }
        return core, ctx3, nil
    }
}

// synth synthesizes a type for a surface expression
// and returns the elaborated core expression.
func (tc *TypeChecker) synth(
    ctx *Context,
    expr surface.Expr,
) (ValueExpr, Type, *Context, error) {
    switch e := expr.(type) {
    case *surface.Var:
        return tc.synthVar(ctx, e)
    case *surface.App:
        return tc.synthApp(ctx, e)
    case *surface.Annotation:
        return tc.synthAnno(ctx, e)
    // ...
    }
}
```

### 13.2 Elaborating Lambdas

```go
func (tc *TypeChecker) checkLambda(
    ctx *Context,
    lam *surface.Lambda,
    expected Type,
) (ValueExpr, *Context, error) {
    // expected must be a function type A -> B
    arrow, ok := expected.(*TyArrow)
    if !ok {
        return nil, nil, tc.errorf(lam.S, "expected function type, got %s", expected)
    }

    // extend context with parameter binding
    ctx2 := ctx.Extend(lam.Param, arrow.Domain)

    // check body against codomain
    body, ctx3, err := tc.check(ctx2, lam.Body, arrow.Codomain)
    if err != nil {
        return nil, nil, err
    }

    // emit annotated core lambda
    core := &LamExpr{
        Param:   lam.Param,
        ParamTy: arrow.Domain,
        Body:    body,
        S:       lam.S,
    }

    return core, ctx3.DropAfter(lam.Param), nil
}
```

### 13.3 Elaborating Do Blocks

```go
func (tc *TypeChecker) checkDo(
    ctx *Context,
    doExpr *surface.DoExpr,
    expected Type,
) (CompExpr, *Context, error) {
    // expected must be Computation pre post a
    comp, ok := expected.(*TyComp)
    if !ok {
        return nil, nil, tc.errorf(doExpr.S, "do block used in non-computation context")
    }

    return tc.elaborateStmts(ctx, doExpr.Stmts, comp.Pre, comp.Post, comp.Result, doExpr.S)
}

func (tc *TypeChecker) elaborateStmts(
    ctx *Context,
    stmts []surface.Stmt,
    currentPre Row,
    overallPost Row,
    resultTy Type,
    span Span,
) (CompExpr, *Context, error) {
    if len(stmts) == 0 {
        return nil, nil, tc.errorf(span, "empty do block")
    }

    // last statement: must be a tail expression
    if len(stmts) == 1 {
        stmt := stmts[0]
        if stmt.Kind != surface.StmtExpr {
            return nil, nil, tc.errorf(stmt.S,
                "last statement in do block must be an expression")
        }
        // check the expression against Computation currentPre overallPost resultTy
        tailTy := &TyComp{Pre: currentPre, Post: overallPost, Result: resultTy}
        core, ctx2, err := tc.checkComp(ctx, stmt.Expr, tailTy)
        return core, ctx2, err
    }

    stmt := stmts[0]
    rest := stmts[1:]

    switch stmt.Kind {
    case surface.StmtBind:
        // x <- e; rest
        // introduce fresh existential for intermediate row and result type
        midRow := tc.freshRowExistential(ctx)
        elemTy := tc.freshTypeExistential(ctx)

        stmtTy := &TyComp{Pre: currentPre, Post: midRow, Result: elemTy}
        compCore, ctx2, err := tc.checkComp(ctx, stmt.Expr, stmtTy)
        if err != nil {
            return nil, nil, err
        }

        // apply solutions to midRow
        solvedMidRow := tc.applySubst(ctx2, midRow)
        solvedElemTy := tc.applySubst(ctx2, elemTy)

        // extend context with binding
        ctx3 := ctx2.Extend(stmt.Name, solvedElemTy)

        // elaborate rest of statements
        contCore, ctx4, err := tc.elaborateStmts(
            ctx3, rest, solvedMidRow, overallPost, resultTy, span)
        if err != nil {
            return nil, nil, err
        }

        return &BindExpr{
            Comp:  compCore,
            Var:   stmt.Name,
            VarTy: solvedElemTy,
            Cont:  contCore,
            S:     stmt.S,
        }, ctx4, nil

    case surface.StmtPureBind:
        // x := e; rest  -->  bind (pure e) (\x -> rest)
        valCore, valTy, ctx2, err := tc.synthValue(ctx, stmt.Expr)
        if err != nil {
            return nil, nil, err
        }

        ctx3 := ctx2.Extend(stmt.Name, valTy)

        contCore, ctx4, err := tc.elaborateStmts(
            ctx3, rest, currentPre, overallPost, resultTy, span)
        if err != nil {
            return nil, nil, err
        }

        return &BindExpr{
            Comp:  &PureExpr{Value: valCore, S: stmt.S},
            Var:   stmt.Name,
            VarTy: valTy,
            Cont:  contCore,
            S:     stmt.S,
        }, ctx4, nil

    case surface.StmtExpr:
        // e; rest  -->  bind e (\_ -> rest)
        midRow := tc.freshRowExistential(ctx)
        stmtTy := &TyComp{
            Pre: currentPre, Post: midRow,
            Result: &TyApp{Con: "Unit"},
        }

        compCore, ctx2, err := tc.checkComp(ctx, stmt.Expr, stmtTy)
        if err != nil {
            return nil, nil, err
        }

        solvedMidRow := tc.applySubst(ctx2, midRow)

        contCore, ctx3, err := tc.elaborateStmts(
            ctx2, rest, solvedMidRow, overallPost, resultTy, span)
        if err != nil {
            return nil, nil, err
        }

        return &BindExpr{
            Comp:  compCore,
            Var:   "_",
            VarTy: &TyApp{Con: "Unit"},
            Cont:  contCore,
            S:     stmt.S,
        }, ctx3, nil
    }

    panic("unreachable")
}
```

### 13.4 Elaborating Variables with Implicit Instantiation

```go
func (tc *TypeChecker) synthVar(
    ctx *Context,
    v *surface.Var,
) (ValueExpr, Type, *Context, error) {
    ty, ok := ctx.Lookup(v.Name)
    if !ok {
        return nil, nil, nil, tc.errorf(v.S, "unbound variable: %s", v.Name)
    }

    // instantiate outermost foralls with fresh existentials
    coreExpr := ValueExpr(&VarExpr{Name: v.Name, S: v.S})
    instantiatedTy := ty
    ctx2 := ctx

    for {
        forallTy, ok := instantiatedTy.(*TyForall)
        if !ok {
            break
        }
        // introduce fresh existential
        fresh := tc.freshExistential(ctx2, forallTy.Kind)
        ctx2 = ctx2.ExtendExistential(fresh)

        // wrap core expr with type application
        coreExpr = &TyAppExpr{
            Expr:  coreExpr,
            TyArg: &TyVar{Name: fresh},
            S:     v.S,
        }

        // substitute into the body
        instantiatedTy = tc.substitute(forallTy.Body, forallTy.Var, &TyVar{Name: fresh})
    }

    return coreExpr, instantiatedTy, ctx2, nil
}
```

### 13.5 Pre-Elaboration Operator Resolution

```go
// resolveOperators transforms infix expressions into explicit applications.
// This runs as a surface-to-surface pass before type checking.
func resolveOperators(expr surface.Expr, fixities map[string]Fixity) surface.Expr {
    switch e := expr.(type) {
    case *surface.BinOp:
        left := resolveOperators(e.Left, fixities)
        right := resolveOperators(e.Right, fixities)
        // (+) applied to left and right
        return &surface.App{
            Func: &surface.App{
                Func: &surface.Var{Name: e.Op, S: e.S},
                Arg:  left,
                S:    e.S,
            },
            Arg: right,
            S:   e.S,
        }
    case *surface.Negate:
        inner := resolveOperators(e.Operand, fixities)
        return &surface.App{
            Func: &surface.Var{Name: "negate", S: e.S},
            Arg:  inner,
            S:    e.S,
        }
    case *surface.IfExpr:
        cond := resolveOperators(e.Cond, fixities)
        thenE := resolveOperators(e.Then, fixities)
        elseE := resolveOperators(e.Else, fixities)
        return &surface.CaseExpr{
            Scrutinee: cond,
            Alts: []surface.Alt{
                {Pattern: surface.ConPat{Con: "True"},  Body: thenE},
                {Pattern: surface.ConPat{Con: "False"}, Body: elseE},
            },
            S: e.S,
        }
    // ... recurse into other forms
    default:
        return expr
    }
}
```

---

## 14. Recommendations for Gomputation

### 14.1 Core Language Summary

The recommended core language has:

- **12 term formers:** Var, Lit (Int/String/Unit), Lam, App, TyApp, Con, Case, Let, LetRec, Pure, Bind, PrimOp (+ CompFromValue as a bridge).
- **Annotated binders:** Lambda parameters, bind variables, and let bindings carry type annotations.
- **Explicit type applications:** All polymorphic instantiations are explicit.
- **Flat case expressions:** No nested patterns; patterns are compiled during elaboration.
- **No operators, no do-notation, no if-then-else:** All surface sugar is elaborated away.

### 14.2 Elaboration Strategy

Use **elaboration during type checking** (Option C from Section 7.1). The bidirectional type checker consumes the surface AST and produces core terms as output. This is the right choice because:

1. It eliminates a separate elaboration pass.
2. It provides the type information needed for implicit instantiation and lambda annotation.
3. It gives the best error messages (errors reference surface syntax directly).
4. It aligns with the bidirectional typing strategy already chosen for Gomputation.

### 14.3 Pre-Elaboration

A surface-to-surface preprocessing pass should handle:

1. Operator resolution (infix to prefix).
2. If-then-else to case desugaring.
3. Multi-parameter lambda to nested single-parameter lambdas.
4. Prefix negation to function application.

These are purely syntactic and require no type information.

### 14.4 Evaluation

The evaluator should be a tree-walking interpreter over the core AST, as defined in the evaluation semantics document. The core AST is the direct evaluation target. No further lowering is needed for v0.

Types can be erased for evaluation. The evaluator uses only the term structure, variable names, constructor tags, and primitive operation names.

### 14.5 Implementation Order

1. **Define the core AST types in Go.** This is the `core` package.
2. **Implement the pre-elaboration pass.** Operator resolution, if-to-case, multi-lambda desugaring.
3. **Implement the bidirectional type checker / elaborator.** This is the main intellectual effort. It consumes the surface AST and produces core terms.
4. **Implement the evaluator on the core AST.** Adapt the evaluator from the evaluation semantics document to work on the core AST types.
5. **Wire the pipeline.** Lexer -> Parser -> Pre-elaboration -> Type Checker / Elaborator -> Evaluator.

### 14.6 Future Extensions

When adding features to the surface language:

- **New surface sugar** (let-in expressions, where clauses, list comprehensions): add elaboration logic to the type checker. The core language does not change.
- **GADTs**: the core language may need coercion terms or refined case expressions. This is a core-level change.
- **Type classes or traits**: the core language needs dictionary-passing or evidence terms. This is a core-level change.
- **Algebraic effect handlers**: the core language needs handler terms and a richer computation structure. This is a core-level change.

The stability of the core is a function of the stability of the type system. Surface syntax changes do not affect the core; type system changes may.

### 14.7 Invariants the Core Must Maintain

1. **Every lambda binder is annotated with its type.** The elaborator guarantees this.
2. **Every polymorphic use is explicitly instantiated with type arguments.** The elaborator inserts `TyApp` nodes.
3. **Case expressions have flat, single-constructor patterns.** The elaborator compiles nested patterns.
4. **Row labels in core row types are sorted.** The elaborator normalizes rows.
5. **No surface-only forms appear in the core.** No BinOp, no DoExpr, no IfExpr, no operator sections.
6. **Source spans are preserved on all core nodes.** Every core node traces back to a surface source location.

---

## Key References

1. Simon Peyton Jones. "An implementation of a non-strict functional language on stock hardware: the Spineless Tagless G-machine." *Journal of Functional Programming*, 2(2):127--202, 1992. (GHC's backend architecture.)

2. Simon Peyton Jones and Simon Marlow. "Secrets of the Glasgow Haskell Compiler inliner." *Journal of Functional Programming*, 12(4+5):393--434, 2002. (How GHC Core enables optimization.)

3. Sulzmann, Chakravarty, Peyton Jones, Donnelly. "System F with Type Equality Coercions." *TLDI*, 2007. (System FC, the formal basis of GHC Core.)

4. Edwin Brady. "Idris 2: Quantitative Type Theory in Practice." *ECOOP*, 2021. (Idris 2 elaboration architecture.)

5. Edwin Brady. "Elaboration in Idris." Chapter in *Type-Driven Development with Idris*, Manning, 2017. (Accessible treatment of elaboration-during-checking.)

6. Phil Freeman. "PureScript by Example." (PureScript's CoreFn design.)

7. Daan Leijen. "Type Directed Compilation of Row-Typed Algebraic Effects." *POPL*, 2017. (Koka's evidence-passing core.)

8. Neel Krishnaswami and Jan Hoffmann. "Bidirectional Type Checking for Higher-Rank Polymorphism." Course notes, CMU 15-312, 2012. (Accessible bidirectional typing tutorial.)

9. Joshua Dunfield and Neel Krishnaswami. "Complete and Easy Bidirectional Typechecking for Higher-Rank Polymorphism." *ICFP*, 2013. (The DK system that Gomputation's type checker should build on.)

10. Luc Maranget. "Compiling Pattern Matching to Good Decision Trees." *ML Workshop*, 2008. (Pattern compilation algorithm.)
