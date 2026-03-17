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

### Polymorphism (\)

```
\a. a -> a
\a b. a -> b -> a
\(r : Row). Computation r r a
\(f : Type -> Type). f a -> f b
```

`\` serves dual purpose: lambda in expression context (`\x -> e`), and universal quantification in type context (`\a. T`). The separator distinguishes: `->` for lambda, `.` for quantification. The parser disambiguates by context. The `.` is the same character as the compose operator; context disambiguates there as well.

### Constraints (=>)

Constraints are curried. Each `C =>` introduces one constraint. Multiple constraints are chained:

```
Eq a => a -> a -> Bool
Eq a => Ord a => a -> Bool
```

### Quantified Constraints

```
(\a. Eq a => Eq (f a)) => f Bool -> f Bool -> Bool
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
f :: \a. Eq a => a -> a -> Bool
f := \x -> \y -> eq x y
```

Free type variables in annotations are implicitly universally quantified (implicit `\`):

```
myLength :: List a -> Int       -- equivalent to: \a. List a -> Int
```

### Type Annotation in Expression

```
(expr :: Type)
```

### Type Families

Type families define type-level functions — computed types that reduce during type checking and are fully erased before evaluation. No runtime cost.

#### Standalone Type Family

A closed type family is declared with `type Name params :: Kind = { equations }`. Equations are checked top-to-bottom; first match wins.

```
-- Standalone type family
type Elem (c : Type) :: Type = {
  Elem (List a) = a;
  Elem String = Rune
}
```

Distinguished from a type alias by `::` after the parameters. Each equation repeats the family name on the left-hand side.

#### Associated Type in Class

Classes can declare associated types. The class body provides the kind signature; instances provide the definition.

```
-- Associated type in class
class Container c {
  type Elem c :: Type;
  cfold :: \b. (Elem c -> b -> b) -> b -> c -> b
}

instance Container (List a) {
  type Elem (List a) = a;
  cfold := foldr
}
```

`Elem (List Int)` reduces to `Int` during type checking — no instance search heuristic, just structural reduction.

#### Data Families

Data families are like associated types but **generative** — each instance creates a distinct data type with its own constructors. This enables type-indexed data representations.

```
-- Data family in class: declares kind signature only
class Collection c {
  data Key c :: Type;
  lookup :: Key c -> c -> Maybe (Elem c)
}

-- Instance provides constructors
instance Collection (List a) {
  data Key (List a) = ListIndex Int;
  lookup := \k -> \xs -> case k {
    ListIndex i -> index xs i
  }
}
```

Each instance's `Key` is a distinct type: `Key (List a)` has constructor `ListIndex`, while another instance (e.g., `Key (Map k v)`) could have entirely different constructors. Data family constructors are visible wherever the instance is in scope.

#### Functional Dependencies

Classes can declare functional dependencies that constrain instance resolution. The `| a -> b` notation after class parameters means: knowing `a` determines `b`.

```
-- Functional dependency
class Convert a b | a -> b {
  convert :: a -> b
}
```

#### Multiplicity Annotations

Row fields can carry an optional multiplicity annotation using `@Mult`:

```
open :: Computation {} { h : Handle @Linear } ()
close :: Computation { h : Handle @Linear } {} ()
```

Without annotation, fields are `@Unrestricted`. The `@Linear` annotation means the capability must be consumed exactly once.

#### Injectivity

A type family can declare its result injective with a named result binder and functional dependency:

```
type Effects (mode : AppMode) :: (r : Row) | r -> mode = {
  Effects ReadOnly  = { get : () -> String };
  Effects ReadWrite = { get : () -> String, put : String -> () }
}
```

Here `| r -> mode` means: the result row uniquely determines the `mode` parameter. This enables the checker to infer `mode` from the result type.

---
