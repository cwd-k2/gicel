# Embedded Typed Effect Language

**Specification Draft v0.1**

---

# 1. Overview

This document specifies a **typed embedded programming language** designed to run inside a Go application as a library.

The language is:

* purely functional at the value level
* effectful at the computation level
* statically typed
* capability-based
* indexed-effect driven
* deterministic

It is intended for:

* safe embedded scripting
* domain logic
* rule engines
* configuration evaluation
* protocol / typestate controlled execution

The host (Go) defines available capabilities.

The language cannot perform operations not explicitly provided by the host.

---

# 2. Design Goals

## Goals

* Embeddable interpreter implemented as a Go library
* Strong static typing
* High-order functions
* High-rank polymorphism
* Indexed effect system
* Typestate protocols
* Deterministic execution
* Explicit host capability control

## Non-Goals

The language is **not intended to be**

* a general scripting language
* dynamically typed
* reflection based
* IO capable by default
* runtime extensible

All external effects must be explicitly registered by the host application.

---

# 3. Language Model

The language distinguishes two semantic layers:

```
Value
Computation
```

### Value

Pure expressions.

Examples:

```
1
"x"
\x -> x
f x
```

### Computation

Effectful operations tracked by the type system.

```
Comp pre post a
```

Meaning:

```
pre   : capability state before execution
post  : capability state after execution
a     : result type
```

---

# 4. Type System

## 4.1 Kinds

```
Type
Row
```

`Row` represents capability environments.

---

## 4.2 Types

```
T ::= a
    | T -> T
    | forall a. T
    | Comp R R T
    | TypeConstructor
```

Examples:

```
Int
String
a -> b
forall a. a -> a
Comp {} {} String
Comp {db : DB[Closed]} {db : DB[Opened]} Unit
```

---

# 5. Row Types

Rows describe capability states.

```
R ::= {}
    | { l : T }
    | { l : T , ... }
    | { l : T | r }
```

Examples

```
{}
{ db : DB[Closed] }
{ db : DB[Opened], log : Logger[Ready] }
{ db : DB[Opened] | r }
```

---

# 6. Computation Type

```
Comp pre post a
```

Meaning:

```
pre  : required capabilities
post : resulting capabilities
a    : result type
```

---

# 7. Primitive Computation Operations

## pure

```
pure : a -> Comp r r a
```

Pure computations do not modify capability state.

---

## bind

```
bind :
  Comp r1 r2 a ->
  (a -> Comp r2 r3 b) ->
  Comp r1 r3 b
```

This sequences computations while enforcing typestate transitions.

---

# 8. Program Structure

Programs consist of declarations.

```
Program ::= Decl*
```

---

# 9. Declarations

## Type annotation

```
Name :: Type
```

Example

```
id :: forall a. a -> a
```

---

## Value definition

```
Name := Expr
```

Example

```
id := \x -> x
```

---

## Data declaration

```
data Name = Constructor*
```

Example

```
data DBState = Opened | Closed
```

---

## Operator precedence

```
infixl n op
infixr n op
infix  n op
```

Example

```
infixr 9 .
```

---

# 10. Expressions

```
Expr ::= variable
       | literal
       | lambda
       | application
       | do
       | case
       | if
```

---

# 11. Lambda

```
\x -> expr
```

Example

```
\x -> x + 1
```

Function definitions are written using lambdas:

```
f := \x -> x + 1
```

---

# 12. Function Application

```
f x
f x y
```

Application associates to the left.

---

# 13. Type Application

```
f @Type
```

Example

```
id @Int
```

---

# 14. do Blocks

Sequential computation is written using `do`.

```
do { Stmt* }
```

---

# 15. Statements

```
Stmt ::=
      x := Expr ;
    | x <- Expr ;
    | Expr ;
    | Expr
```

---

## 15.1 Pure Binding

```
x := expr
```

Requirements

* `expr` must be pure
* not `Comp`

Semantics

```
(\x -> rest) expr
```

---

## 15.2 Computation Binding

```
x <- expr
```

Requirements

```
expr : Comp r1 r2 a
```

Semantics

```
bind expr (\x -> rest)
```

---

## 15.3 Effect Statement

```
expr;
```

Requires

```
expr : Comp r1 r2 Unit
```

---

## 15.4 Tail Expression

The final expression of the block is the return value.

---

# 16. Example

```
main := do {
  openDB;
  rows <- query "select * from users";
  text := render rows;
  closeDB;
  pure text
}
```

---

# 17. Variable Scope

Variables are **lexically scoped**.

Example:

```
do {
  x := 1;
  y := x + 2;
  pure y
}
```

Shadowing is allowed in nested scopes.

---

# 18. Mutable Capability

Mutable state is expressed using explicit capability operations.

Example interface:

```
declare : forall a.
  Comp r {var : Slot[Uninit,a] | r} Var

assign :
  Var -> a ->
  Comp {var : Slot[Uninit,a] | r}
       {var : Slot[Init,a] | r}
       Unit

read :
  Var ->
  Comp {var : Slot[Init,a] | r}
       {var : Slot[Init,a] | r}
       a
```

This enables typestate-safe mutation.

---

# 19. Type Checking Strategy

The type system uses **bidirectional typing**.

Characteristics:

* supports high-rank polymorphism
* requires explicit annotations where necessary
* effect rows unify during computation sequencing

Top-level type signatures are recommended.

---

# 20. Parser Design (Implementation Guidance)

Recommended architecture:

```
Lexer
↓
Declaration parser
↓
Expression parser
↓
AST
↓
Type checker
↓
Core IR
↓
Evaluator
```

### Expression parsing

Use:

* Pratt parser
* precedence climbing

Operators are resolved using declared fixity rules.

---

# 21. Core Intermediate Representation

Surface syntax should compile into a minimal core language.

Example core terms:

```
Var
Lam
App
Pure
Bind
Perform
```

This simplifies evaluation and optimization.

---

# 22. Go Integration Model

The language is embedded inside Go.

Host code registers capabilities.

Example:

```go
vm.RegisterOp(
  "openDB",
  `Comp {db : DB[Closed] | r} {db : DB[Opened] | r} Unit`,
  openDBImpl,
)
```

Execution:

```go
result, err := vm.Run("main")
```

---

# 23. Entry Point

Typical program entry:

```
main :: Comp {} {} String
```

The returned value is converted to a Go value.

---

# 24. Example Program

```
data DBState = Opened | Closed

compose := \f -> \g -> \x -> f (g x)

main :: Comp {db : DB[Closed]} {db : DB[Closed]} String
main := do {
  sql := "select * from users";
  openDB;
  rows <- query sql;
  text := render rows;
  closeDB;
  pure text
}
```

---

# 25. Implementation Roadmap

Suggested implementation phases.

## Phase 1

* lexer
* parser
* lambda calculus core
* basic type system

## Phase 2

* Comp type
* indexed effect system
* row polymorphism

## Phase 3

* typestate capabilities
* capability solver

## Phase 4

* high-rank polymorphism
* type application

## Phase 5

* operators
* syntactic sugar

---

# 26. Language Feature Summary

| Feature                | Supported |
| ---------------------- | --------- |
| High-order functions   | yes       |
| High-rank polymorphism | yes       |
| CPS encoding           | yes       |
| Codensity              | yes       |
| Indexed effects        | yes       |
| Typestate protocols    | yes       |
| Capability safety      | yes       |
| Operator definition    | yes       |

---

# End of Specification

