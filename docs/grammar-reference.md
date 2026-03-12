# Gomputation Grammar Reference

## Lexical Structure

### Keywords (11)

| Keyword    | Purpose                          |
|------------|----------------------------------|
| `case`     | Pattern matching                 |
| `of`       | Case alternative separator       |
| `do`       | Monadic do-block                 |
| `data`     | Algebraic data type declaration  |
| `type`     | Type alias declaration           |
| `forall`   | Universal quantification         |
| `infixl`   | Left-associative operator fixity |
| `infixr`   | Right-associative operator fixity|
| `infixn`   | Non-associative operator fixity  |
| `class`    | Type class declaration           |
| `instance` | Type class instance declaration  |

### Built-in Identifiers

| Identifier   | Role                              |
|--------------|-----------------------------------|
| `pure`       | Value → Computation (F)           |
| `bind`       | Monadic sequencing                |
| `thunk`      | Computation → suspended value (U) |
| `force`      | Elimination of U                  |
| `assumption` | Host-provided primitive marker    |
| `rec`        | Recursive combinator (gated)      |
| `fix`        | Value-level fixpoint (gated)      |

### Punctuation & Operators

| Token  | Meaning                       |
|--------|-------------------------------|
| `->`   | Function type / lambda body   |
| `<-`   | Monadic bind in do-block      |
| `=>`   | Constraint qualifier          |
| `::`   | Type annotation               |
| `:=`   | Value definition / let-bind   |
| `:`    | Kind annotation separator     |
| `.`    | Forall body separator         |
| `\`    | Lambda introducer             |
| `_`    | Wildcard pattern              |
| `=`    | Data constructor separator    |
| `@`    | Explicit type application     |
| `\|`   | Constructor / row tail separator |

### Identifiers

- **Lowercase** `[a-z_][a-zA-Z0-9_']*` — variables, type variables, field labels
- **Uppercase** `[A-Z][a-zA-Z0-9_']*` — constructors, type constructors, class names
- **Operators** sequences of `! # $ % & * + - / < = > ? ^ ~ | : .` (excluding reserved)

### Comments

```
-- line comment
{- nestable block comment {- inner -} outer -}
```

### Integer Literals

Unsigned decimal integers: `[0-9]+`. Negative values via prefix operator.

---

## Declarations

### Data Type

```
data Name param* = Con field* (| Con field*)*
```

Examples:
```
data Bool = True | False
data Maybe a = Just a | Nothing
data Result e a = Ok a | Err e
data List a = Cons a (List a) | Nil
```

### Type Alias

```
type Name param* = TypeExpr
```

Example:
```
type Effect r a = Computation r r a
```

### Type Annotation

```
name :: TypeExpr
```

Example:
```
f :: forall a. Eq a => a -> a -> Bool
```

### Value Definition

```
name := Expr
```

Example:
```
not := \b -> case b of { True -> False; False -> True }
```

### Operator Fixity

```
infixl Prec Op    -- left-associative
infixr Prec Op    -- right-associative
infixn Prec Op    -- non-associative
```

Precedence: 0–9 (default: left-associative, 9). Example:
```
infixl 6 plus
infixr 5 cons
```

### Type Class

```
class [Constraint =>] ClassName param* {
  method1 :: TypeExpr;
  method2 :: TypeExpr
}
```

Examples:
```
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Ordering }
class Functor f { fmap :: forall a b. (a -> b) -> f a -> f b }
class Coercible a b { coerce :: a -> b }
```

### Type Class Instance

```
instance [Constraint =>] ClassName TypeArg* {
  method1 := Expr;
  method2 := Expr
}
```

Examples:
```
instance Eq Bool { eq := \x -> \y -> True }
instance Eq a => Eq (Maybe a) {
  eq := \x -> \y -> case x of {
    Nothing -> case y of { Nothing -> True; Just _ -> False };
    Just a  -> case y of { Nothing -> False; Just b -> eq a b }
  }
}
```

---

## Expressions

### Variables and Constructors

```
x           -- variable
Nothing     -- nullary constructor
Just x      -- applied constructor (via App)
```

### Lambda

```
\param -> body
\x -> \y -> expr      -- curried
\x y -> expr           -- multi-parameter (sugar)
\(Con x y) -> expr     -- pattern parameter
```

### Application

```
f x           -- function application (left-associative)
f x y         -- = (f x) y
f @Int        -- explicit type application
```

### Case Expression

```
case scrutinee of {
  Con x y -> expr;
  _       -> expr
}
```

### Block Expression

```
{ x := e1; y := e2; body }
```

Desugars to `(\x -> (\y -> body) e2) e1`.

### Do Block

```
do {
  x <- computation;      -- monadic bind
  y := pure_expr;        -- pure let-bind
  _ <- side_effect;      -- discard result
  pure result             -- final expression
}
```

### Infix Operators

```
x `plus` y          -- backtick syntax
x + y               -- operator syntax (if declared)
```

### Type Annotation (in expression)

```
(expr :: Type)
```

### Special Forms

```
pure expr                    -- F: value → computation
bind comp (\x -> body)       -- monadic bind (explicit)
thunk computation            -- U: computation → value
force thunked_value          -- elimination of U
```

---

## Type Expressions

### Type Variables and Constructors

```
a               -- type variable
Int             -- type constructor
```

### Type Application

```
Maybe a         -- = TyApp(Maybe, a)
List Int        -- = TyApp(List, Int)
Map k v         -- = TyApp(TyApp(Map, k), v)
```

### Function Type

```
a -> b          -- right-associative
a -> b -> c     -- = a -> (b -> c)
```

### Qualified Type (Constraint)

```
Eq a => a -> a -> Bool
Eq a => Ord b => a -> b -> Bool    -- curried constraints
```

### Universal Quantification

```
forall a. a -> a
forall a b. a -> b -> a
forall (r : Row). Computation r r a
forall (f : Type -> Type). f a -> f b
```

### Row Type

```
{}                              -- empty row (closed)
{ x : Int, y : Bool }          -- closed row
{ x : Int | r }                -- open row (tail variable)
{ get : Unit -> Int | r }      -- capability row
```

### Parenthesized Type

```
(a -> b)        -- grouping
(Maybe a)       -- grouping
```

---

## Kind Expressions

```
Type                -- kind of value types
Row                 -- kind of row types
Constraint          -- kind of class constraints
Type -> Type        -- higher-kinded (e.g. Maybe : Type -> Type)
(Row -> Type)       -- parenthesized kind
```

---

## Patterns

```
x               -- variable binding
_               -- wildcard
Con             -- nullary constructor
Con x y         -- constructor with arguments
(Con x y)       -- parenthesized pattern
```

---

## Parser Precedence (Expressions)

| Level | Form              | Associativity |
|-------|-------------------|---------------|
| 0     | `:: Type`         | right         |
| 1–9   | Infix operators   | per `infixl/r/n` |
| 10    | Application `f x` | left          |
| —     | Atoms             | —             |

### Type Expression Precedence

| Level | Form         | Associativity |
|-------|--------------|---------------|
| 0     | `forall`     | —             |
| 1     | `=>`         | right         |
| 2     | `->`         | right         |
| 3     | Application  | left          |
| —     | Atoms        | —             |

---

## Declaration Boundaries

Declarations are separated by newlines at nesting depth 0. A new declaration begins when a newline-preceded token at depth 0 is one of:

`lowercase` | `uppercase` | `data` | `type` | `infixl` | `infixr` | `infixn` | `class` | `instance`

No explicit semicolons needed between top-level declarations.

---

## Built-in Types

| Type                        | Kind              | Description                |
|-----------------------------|-------------------|----------------------------|
| `Computation pre post a`    | `Row → Row → Type → Type` | Effectful computation |
| `Thunk pre post a`          | `Row → Row → Type → Type` | Suspended computation |

---

## Prelude (auto-included unless `NoPrelude`)

```
data Bool = True | False
data Unit = Unit
data Result e a = Ok a | Err e
data Pair a b = Pair a b
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
type Effect r a = Computation r r a
```
