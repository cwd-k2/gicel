## 3. Type System

### Base Types

| Type       | Kind                   | Description           | Source   |
| ---------- | ---------------------- | --------------------- | -------- |
| `Bool`     | `Type`                 | `True \| False`       | Prelude  |
| `()`       | `Type`                 | Empty record / unit   | Built-in |
| `Ordering` | `Type`                 | `LT \| EQ \| GT`      | Prelude  |
| `Int`      | `Type`                 | 64-bit integer        | Built-in |
| `String`   | `Type`                 | Unicode string        | Built-in |
| `Rune`     | `Type`                 | Unicode code point    | Built-in |
| `Map`      | `Type -> Type -> Type` | Ordered immutable map | Std.Map  |
| `Set`      | `Type -> Type`         | Ordered immutable set | Std.Set  |

### Built-in Computation Types

| Type                     | Kind                         | Description           |
| ------------------------ | ---------------------------- | --------------------- |
| `Computation pre post a` | `Row -> Row -> Type -> Type` | Effectful computation |
| `Thunk pre post a`       | `Row -> Row -> Type -> Type` | Suspended computation |

### Algebraic Data Types (ADT)

```
data Name param* = Con field* (| Con field*)*
```

Parameters can be bare type variables or kinded:

```
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
data Dict (c : Constraint) = MkDict c
```

### GADTs

```
data Name param* = {
  Con :: TypeExpr;
  Con :: TypeExpr
}
```

Distinguished from ADT by `= {`. Each constructor has a full type signature:

```
data Expr a = {
  LitBool :: Bool -> Expr Bool;
  LitInt  :: Int -> Expr Int;
  Add     :: Expr Int -> Expr Int -> Expr Int
}
```

GADT pattern matching refines type variables in branches.

### Type Aliases

```
type Name param* = TypeExpr
```

Example:

```
type Effect r a = Computation r r a
```

### Polymorphism (forall)

```
forall a. a -> a
forall a b. a -> b -> a
forall (r : Row). Computation r r a
forall (f : Type -> Type). f a -> f b
```

The `.` after `forall` separates the quantified variables from the body type. This is the same character as the compose operator; context disambiguates.

### Constraints (=>)

Constraints are curried. Each `C =>` introduces one constraint. Multiple constraints are chained:

```
Eq a => a -> a -> Bool
Eq a => Ord a => a -> Bool
```

### Quantified Constraints

```
(forall a. Eq a => Eq (f a)) => f Bool -> f Bool -> Bool
```

### Kinds

| Kind                         | Description                    |
| ---------------------------- | ------------------------------ |
| `Type`                       | Kind of value types            |
| `Row`                        | Kind of row types              |
| `Constraint`                 | Kind of class constraints      |
| `Type -> Type`               | Higher-kinded (e.g., `Maybe`)  |
| `Row -> Row -> Type -> Type` | Kind of `Computation`, `Thunk` |
| `Constraint -> Type`         | Constraint-parameterized       |

### DataKinds Promotion

When a data type is declared, it is automatically promoted to a kind. Nullary constructors become types of that kind:

```
data DBState = Opened | Closed
-- DBState is now also a kind
-- Opened : DBState, Closed : DBState at the type level

data DB (s : DBState) = MkDB
-- DB Opened : Type, DB Closed : Type
```

### Row Types

```
{}                              -- empty row (closed)
{ x : Int, y : Bool }          -- closed row
{ x : Int | r }                -- open row (tail variable r)
{ get : () -> Int | r }        -- capability row
```

### Type Annotations

Type annotations are written on a separate line from the definition:

```
f :: forall a. Eq a => a -> a -> Bool
f := \x -> \y -> eq x y
```

Free type variables in annotations are implicitly universally quantified (implicit `forall`):

```
myLength :: List a -> Int       -- equivalent to: forall a. List a -> Int
```

### Type Annotation in Expression

```
(expr :: Type)
```

---
