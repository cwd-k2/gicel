# Gomputation Grammar Reference

## Lexical Structure

### Keywords (12)

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
| `import`   | Module import                    |

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

Unsigned decimal integers: `[0-9]+`. Negative values via `negate`.

### String Literals

Double-quoted: `"hello world"`. Escape sequences: `\n`, `\t`, `\r`, `\\`, `\"`, `\'`, `\0`.

### Rune Literals

Single-quoted single character: `'a'`, `'\n'`. Same escape sequences as strings.

---

## Declarations

### Data Type (ADT)

```
data Name param* = Con field* (| Con field*)*
```

Parameters can be bare type variables or kinded: `(name : Kind)`.

Examples:
```
data Bool = True | False
data Maybe a = Just a | Nothing
data Result e a = Ok a | Err e
data List a = Cons a (List a) | Nil
data Dict (c : Constraint) = MkDict c    -- Constraint-kinded param
data Evidence (c : Constraint) a = MkEvidence c a
```

### Data Type (GADT)

```
data Name param* = {
  Con :: TypeExpr;
  Con :: TypeExpr
}
```

Distinguished from ADT by `= {`. Each constructor has a full type signature including return type.

Examples:
```
data Expr a = {
  LitBool :: Bool -> Expr Bool;
  LitInt  :: Int -> Expr Int;
  Not     :: Expr Bool -> Expr Bool;
  Add     :: Expr Int -> Expr Int -> Expr Int
}
```

GADT constructors enable type refinement in `case` branches: matching `LitBool` on `Expr a` refines `a ~ Bool`. Exhaustiveness checking filters constructors whose return type cannot unify with the scrutinee type.

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

### Operator Definition

```
(op) :: TypeExpr
(op) := Expr
```

Operators are defined by wrapping the operator symbol in parentheses. This allows defining type annotations and value definitions for infix operators.

Example:
```
infixl 6 +
(+) :: forall a. Num a => a -> a -> a
(+) := add
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

### Import Declaration

```
import ModuleName
```

Dotted module names are supported for stdlib packs:
```
import Std.Num
import Std.Str
```

Import declarations must appear before all other declarations. All exported types, constructors, type classes, instances, and values from the named module become available.

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
(Eq a, Ord a) => a -> Bool         -- constraint product
(Eq a, Ord a, Show a) => a -> Bool -- multiple constraints
(Eq a, Ord a) => Show a => a -> Bool  -- product + curried (mixed)
```

Constraint products `(C1, C2, ...)` and curried constraints `C1 => C2 => ...` are equivalent; both elaborate to a single `TyEvidence` with multiple constraint entries. `(C)` with a single constraint is treated as a parenthesized constraint, not a product.

### Quantified Constraints

```
(forall a. Eq a => Eq (f a)) => f Bool -> f Bool -> Bool
(forall a. Eq a => Show a => Eq (f a)) => ...    -- multiple premises
(Show Bool, forall a. Eq a => Eq (f a)) => ...   -- mixed with product
```

A quantified constraint `forall vars. context => head` asserts that, for any instantiation of `vars`, if the `context` constraints hold, then the `head` constraint holds. Evidence for a quantified constraint is a *function* from context dictionaries to the head dictionary:

```
-- Evidence type for (forall a. Eq a => Eq (f a)):
-- forall a. Eq$Dict a -> Eq$Dict (f a)
```

At use sites, the quantified constraint is resolved by finding a matching global instance. For example, `instance Eq a => Eq (F a)` satisfies `forall a. Eq a => Eq (F a)`.

Within a function body, quantified evidence can be applied to produce dictionaries for specific types. If `f` has constraint `(forall a. Eq a => Eq (g a))`, then `eq (x :: g Bool) y` resolves `Eq (g Bool)` by applying the quantified evidence to `Bool` and the `Eq Bool` dictionary.

### Dict Reification

Constraint-kinded type parameters in data declarations enable reification of class evidence as first-class values:

```
data Dict (c : Constraint) = MkDict c
```

The parameter `c` has kind `Constraint`. The constructor field `c` elaborates to an implicit evidence argument — the dictionary for the constraint. At construction, evidence is resolved automatically from the context:

```
mkDict :: Dict (Eq Bool)
mkDict := MkDict           -- resolves Eq Bool evidence implicitly
```

Pattern matching on `Dict` brings the evidence back into scope:

```
withDict :: forall a. Dict (Eq a) -> a -> a -> Bool
withDict := \d x y -> case d of { MkDict -> eq x y }
```

The user writes `MkDict` with zero explicit pattern arguments; the evidence field is implicit. Inside the branch body, the constraint `Eq a` is available for resolution.

Constraint-kinded parameters can coexist with regular parameters:

```
data Evidence (c : Constraint) a = MkEvidence c a
```

Here `c` is the implicit evidence field and `a` is a regular field.

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

### Parenthesized Type / Constraint Tuple

```
(a -> b)          -- grouping
(Maybe a)         -- grouping
(Eq a, Ord a)     -- constraint tuple (only valid before =>)
```

---

## Kind Expressions

```
Type                  -- kind of value types
Row                   -- kind of row types
Constraint            -- kind of class constraints
Type -> Type          -- higher-kinded (e.g. Maybe : Type -> Type)
Constraint -> Type    -- constraint-parameterized (higher-kinded constraint)
(Row -> Type)         -- parenthesized kind
Bool                  -- DataKinds: promoted data type as kind
DBState               -- DataKinds: user-defined promoted kind
```

`Constraint` can be used in kind annotations for type parameters:
```
forall (c : Constraint). Bool                    -- constraint-kinded param
forall a (c : Constraint). a -> Bool             -- mixed kinds
class Constrained (c : Constraint) { ... }       -- in class declarations
data Dict (c : Constraint) = MkDict c            -- in data declarations (Dict reification)
```

### DataKinds Promotion

When a data type is declared, it is automatically promoted to a kind of the same name. Nullary constructors (those with no fields) are promoted to types of that kind. Constructors with fields are not promoted.

```
data DBState = Opened | Closed
-- DBState is now a kind
-- Opened : DBState, Closed : DBState (type-level)

data DB (s : DBState) = MkDB
-- DB Opened : Type, DB Closed : Type
```

Resolution order in type positions: registered type constructor → type alias → promoted constructor.

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

`lowercase` | `uppercase` | `data` | `type` | `infixl` | `infixr` | `infixn` | `class` | `instance` | `import` | `(op)` (operator definition)

No explicit semicolons needed between top-level declarations.

---

## Built-in Types

| Type                        | Kind              | Description                |
|-----------------------------|-------------------|----------------------------|
| `Computation pre post a`    | `Row → Row → Type → Type` | Effectful computation |
| `Thunk pre post a`          | `Row → Row → Type → Type` | Suspended computation |
| `Int`                       | `Type`            | 64-bit integer             |
| `String`                    | `Type`            | Unicode string             |
| `Rune`                      | `Type`            | Unicode code point         |

---

## Stdlib Packs

Stdlib packs are loaded via `Engine.Use(pack)` on the host side and `import Std.X` in source.

### Std.Num

Provides `Num` class, `Eq`/`Ord` Int instances, and arithmetic operators.

```
class Eq a => Num a {
  add    :: a -> a -> a;
  sub    :: a -> a -> a;
  mul    :: a -> a -> a;
  negate :: a -> a
}

instance Eq Int    instance Ord Int    instance Num Int

div :: Int -> Int -> Int
mod :: Int -> Int -> Int

infixl 6 +   infixl 6 -
infixl 7 *   infixl 7 /
```

### Std.Str

Provides `Eq`/`Ord`/`Semigroup`/`Monoid` String instances, `Eq`/`Ord` Rune instances.

```
instance Eq String    instance Ord String
instance Semigroup String    instance Monoid String
instance Eq Rune    instance Ord Rune

length :: String -> Int
```

### Std.Fail

Provides fail effect capability.

```
failWith :: forall e r a. e -> Computation { fail : e | r } { fail : e | r } a
fail     :: forall r a. Computation { fail : Unit | r } { fail : Unit | r } a
fromMaybe  :: forall a r. Maybe a -> Computation { fail : Unit | r } ... a
fromResult :: forall e a r. Result e a -> Computation { fail : e | r } ... a
```

### Std.State

Provides get/put state capabilities.

```
get    :: forall s r. Computation { state : s | r } { state : s | r } s
put    :: forall s r. s -> Computation { state : s | r } { state : s | r } Unit
modify :: forall s r. (s -> s) -> Computation { state : s | r } { state : s | r } Unit
```

---

## Prelude (auto-included unless `NoPrelude`)

### Data Types

```
data Bool = True | False
data Unit = Unit
data Ordering = LT | EQ | GT
data Result e a = Ok a | Err e
data Pair a b = Pair a b
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
```

### Type Classes

```
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Ordering }
class Semigroup a { append :: a -> a -> a }
class Semigroup a => Monoid a { empty :: a }
class Functor f { fmap :: forall a b. (a -> b) -> f a -> f b }
class Foldable t { foldr :: forall a b. (a -> b -> b) -> b -> t a -> b }
class Functor f => Applicative f {
  wrap :: forall a. a -> f a;
  ap   :: forall a b. f (a -> b) -> f a -> f b
}
class Functor t => Foldable t => Traversable t {
  traverse :: forall f a b. Applicative f => (a -> f b) -> t a -> f (t b)
}
class IxMonad (m : Row -> Row -> Type -> Type) {
  ixpure :: forall a (r : Row). a -> m r r a;
  ixbind :: forall a b (r1 : Row) (r2 : Row) (r3 : Row).
              m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}
```

### Type Aliases

```
type Effect r a = Computation r r a
type Lift (m : Type -> Type) (r1 : Row) (r2 : Row) a = m a
```

### Functions

```
then :: forall a b (r1 : Row) (r2 : Row) (r3 : Row).
  Computation r1 r2 a -> Computation r2 r3 b -> Computation r1 r3 b
```

### Instances

```
-- IxMonad
instance IxMonad Computation    -- uses built-in pure/bind
instance IxMonad Maybe          instance IxMonad List

-- Eq
instance Eq Bool          instance Eq Unit
instance Eq Ordering      instance Eq a => Eq (Maybe a)
instance Eq a => Eq b => Eq (Pair a b)
instance Eq a => Eq (List a)

-- Ord
instance Ord Bool         instance Ord Unit
instance Ord Ordering     instance Ord a => Ord (Maybe a)
instance Ord a => Ord b => Ord (Pair a b)

-- Semigroup / Monoid
instance Semigroup Unit        instance Semigroup Ordering
instance Semigroup (List a)
instance Monoid Unit           instance Monoid Ordering
instance Monoid (List a)

-- Functor / Foldable / Applicative / Traversable
instance Functor Maybe         instance Functor (Pair a)
instance Functor List
instance Foldable Maybe        instance Foldable (Pair a)
instance Foldable List
instance Applicative Maybe
instance Traversable Maybe     instance Traversable (Pair a)
```
