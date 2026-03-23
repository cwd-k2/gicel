## Algebraic Data Types

### ADT Declaration

Simple enumerations (no fields, no type parameters) use the short form:

```
data Name := Con (| Con)*
```

Example:

```
data Ordering := LT | EQ | GT
data Bool := True | False
```

Parameterized types and constructors with fields use the unified syntax:

```
data Name := \param*. { Con: Type; ... }
```

Parameters can be bare type variables or kinded:

```
data Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a }
data List := \a. { Cons: a -> List a -> List a; Nil: List a }
data Shape := { Circle: Int -> Shape; Rect: Int -> Int -> Shape }
```

### GADTs

GADTs use braces with full constructor type signatures. Each constructor can refine the type parameter:

```
data Expr := \a. {
  LitBool: Bool -> Expr Bool;
  LitInt:  Int -> Expr Int;
  Add:     Expr Int -> Expr Int -> Expr Int
}
```

GADT pattern matching refines type variables in branches. Existential types are supported:

```
data SomeEq := {
  MkSomeEq: \a. Eq a => a -> SomeEq
}
```

### Pattern Matching

```
case scrutinee {
  Con x y => expr;
  _       => expr
}
```

Patterns: variable, wildcard (`_`), constructor, literal, nested constructor. Branches separated by `;`.

```
case m {
  Just True  => "yes";
  Just False => "no";
  Nothing    => "empty"
}
```

Literal patterns (integers, strings, runes) require a wildcard catch-all since literal types are open.

### DataKinds Promotion

Data types are automatically promoted to kinds. Nullary constructors become types of that kind:

```
data DBState := Opened | Closed
-- DBState is now also a kind
-- Opened: DBState, Closed: DBState at the type level

data DB := \(s: DBState). { MkDB: DB s }
-- DB Opened: Type, DB Closed: Type
```

### Type Aliases

```
type Name := \param*. TypeExpr
```

Example:

```
type Effect := \r a. Computation r r a
```

See the language specification (Chapter 7) for elaboration and exhaustiveness details.
