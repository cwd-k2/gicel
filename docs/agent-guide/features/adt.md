## Algebraic Data Types

### ADT Declaration

Simple enumerations (no fields, no type parameters) use the short form:

```
form Name := Con (| Con)*
```

Example:

```
form Ordering := LT | EQ | GT
form Bool := True | False
```

Parameterized types and constructors with fields use the unified syntax:

```
form Name := \param*. { Con: Type; ... }
```

Parameters can be bare type variables or kinded:

```
form Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a }
form List := \a. { Cons: a -> List a -> List a; Nil: List a }
form Shape := { Circle: Int -> Shape; Rect: Int -> Int -> Shape }
```

### GADTs

GADTs use braces with full constructor type signatures. Each constructor can refine the type parameter:

```
form Expr := \a. {
  LitBool: Bool -> Expr Bool;
  LitInt:  Int -> Expr Int;
  Add:     Expr Int -> Expr Int -> Expr Int
}
```

GADT pattern matching refines type variables in branches. Existential types are supported:

```
form SomeEq := {
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

### Lazy Co-Data Declaration

The `lazy` keyword introduces co-data types where constructor arguments are implicitly wrapped in `Thunk`. This enables lazy evaluation without manual thunking:

```
lazy Name := \param*. { Con: Type -> ... -> Name param; ... }
```

Example:

```
lazy Stream := \a. { LCons: a -> Stream a -> Stream a; LNil: Stream a }
```

In a `lazy` form, the `Stream a` argument to `LCons` is automatically wrapped in `Thunk` at the representation level. Users write `LCons x rest` — the runtime handles suspension transparently. Pattern matching on lazy constructors auto-forces thunked fields.

### DataKinds Promotion

Data types are automatically promoted to kinds. Nullary constructors become types of that kind:

```
form DBState := Opened | Closed
-- DBState is now also a kind
-- Opened: DBState, Closed: DBState at the type level

form DB := \(s: DBState). { MkDB: DB s }
-- DB Opened: Type, DB Closed: Type
```

### Type Aliases

```
type Name := \param*. TypeExpr
```

Example:

```
type Effect := \r a. Computation Zero r r a
```

---

See also: `features.type-classes` for class declaration with `form`, `features.type-families` for associated types, `stdlib.prelude` for built-in data types (Bool, Maybe, Result, List).
