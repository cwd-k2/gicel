# Gomputation Agent Guide

A self-contained reference for writing correct Gomputation programs.
Read this document and you have everything you need.

---

## 1. Quick Start

### Minimal Program

```
main := True
```

This defines a binding `main` whose value is `True` (a Bool constructor from the Prelude).

### With Arithmetic (requires Std.Num)

```
import Std.Num

main := 2 + 3
```

### Hello World (requires Std.Str and Std.IO)

```
import Std.Str
import Std.IO

main := print "Hello, world!"
```

`main` here is a `Computation { io : Unit | r } { io : Unit | r } Unit`. The host must provide the `io` capability.

### Running Programs

**CLI:**

```sh
# Run with all stdlib packs (default)
gomputation run program.gmp

# Type-check only
gomputation check program.gmp

# Select specific packs
gomputation run --use Num,Str program.gmp

# Custom entry point, limits, JSON output
gomputation run --entry myFunc --timeout 10s --max-steps 500000 --json program.gmp
```

CLI flags:

| Flag            | Default   | Description                                      |
|-----------------|-----------|--------------------------------------------------|
| `--use`         | `all`     | Comma-separated packs: Num, Str, List, Fail, State, IO |
| `--entry`       | `main`    | Entry point binding name                         |
| `--timeout`     | `5s`      | Execution timeout (run only)                     |
| `--max-steps`   | `100000`  | Step limit (run only)                            |
| `--max-depth`   | `100`     | Depth limit (run only)                           |
| `--json`        | `false`   | Output result as JSON (run only)                 |

**Go API (Sandbox):**

```go
import gmp "github.com/cwd-k2/gomputation"

result, err := gmp.RunSandbox(`
import Std.Num
main := 2 + 3
`, &gmp.SandboxConfig{
    Packs: []gmp.Pack{gmp.Num},
})
// result.Value is HostVal{Inner: int64(5)}
```

**Go API (Full lifecycle):**

```go
eng := gmp.NewEngine()
eng.Use(gmp.Num)
eng.Use(gmp.Str)

rt, err := eng.NewRuntime(source)
result, err := rt.RunContext(ctx, nil, nil, "main")
```

---

## 2. Language Overview

### Keywords (11)

| Keyword    | Purpose                          |
|------------|----------------------------------|
| `case`     | Pattern matching                 |
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

| Identifier   | Role                                          |
|--------------|-----------------------------------------------|
| `pure`       | Lift a value into a Computation (the F of CBPV) |
| `bind`       | Monadic sequencing                            |
| `thunk`      | Suspend a Computation into a value (U of CBPV) |
| `force`      | Eliminate a thunk, resuming the computation    |
| `assumption` | Marker for host-provided primitive bindings   |
| `rec`        | Recursive combinator (gated, must be enabled) |
| `fix`        | Value-level fixpoint (gated, must be enabled) |

`pure` and `bind` are always available without any import.

### Punctuation and Delimiters

| Token  | Meaning                              |
|--------|--------------------------------------|
| `->`   | Function type arrow / lambda body    |
| `<-`   | Monadic bind in do-block             |
| `=>`   | Constraint qualifier                 |
| `::`   | Type annotation                      |
| `:=`   | Value definition                     |
| `:`    | Kind annotation separator            |
| `.`    | Forall body separator (also compose operator) |
| `\`    | Lambda introducer                    |
| `_`    | Wildcard pattern                     |
| `=`    | Data constructor separator           |
| `@`    | Explicit type application            |
| `\|`   | Constructor alternative / row tail   |
| `;`    | Declaration / statement separator    |

### Comments

```
-- line comment
{- nestable block comment {- inner -} outer -}
```

### Literals

- **Integer:** Unsigned decimal digits `[0-9]+`. Requires `import Std.Num` to use. Negative values via `negate 5`, not `-5`.
- **String:** Double-quoted `"hello\nworld"`. Escape sequences: `\n`, `\t`, `\r`, `\\`, `\"`, `\'`, `\0`.
- **Rune:** Single-quoted single character `'a'`, `'\n'`. Same escapes as strings.

### Declaration Boundaries

Top-level declarations are separated by newlines or semicolons. Both are interchangeable at the top level; trailing and repeated semicolons are permitted. A new declaration starts when the next token (preceded by a newline or semicolon at depth 0) is one of: lowercase identifier, uppercase identifier, `data`, `type`, `infixl`, `infixr`, `infixn`, `class`, `instance`, `import`, or `(op)`. Inside braces (`do`, `case`, GADT), semicolons are required between statements/alternatives.

Import declarations must appear before all other declarations.

---

## 3. Type System

### Base Types

| Type       | Kind   | Description          | Source   |
|------------|--------|----------------------|----------|
| `Bool`     | `Type` | `True \| False`      | Prelude  |
| `Unit`     | `Type` | `Unit`               | Prelude  |
| `Ordering` | `Type` | `LT \| EQ \| GT`    | Prelude  |
| `Int`      | `Type` | 64-bit integer       | Built-in |
| `String`   | `Type` | Unicode string       | Built-in |
| `Rune`     | `Type` | Unicode code point   | Built-in |

### Built-in Computation Types

| Type                        | Kind                       | Description            |
|-----------------------------|----------------------------|------------------------|
| `Computation pre post a`    | `Row -> Row -> Type -> Type` | Effectful computation |
| `Thunk pre post a`          | `Row -> Row -> Type -> Type` | Suspended computation |

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

| Kind                    | Description                          |
|-------------------------|--------------------------------------|
| `Type`                  | Kind of value types                  |
| `Row`                   | Kind of row types                    |
| `Constraint`            | Kind of class constraints            |
| `Type -> Type`          | Higher-kinded (e.g., `Maybe`)        |
| `Row -> Row -> Type -> Type` | Kind of `Computation`, `Thunk` |
| `Constraint -> Type`    | Constraint-parameterized             |

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
{ get : Unit -> Int | r }      -- capability row
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

## 4. Expressions

### Lambda

Single-parameter only. Curry for multiple parameters:

```
\x -> expr
\x -> \y -> \z -> expr
\(Just x) -> expr              -- constructor pattern
\(a, b) -> expr                -- tuple pattern
\{ x, y } -> expr              -- record pattern
```

Pattern parameters are desugared to `case`: `\(a, b) -> body` becomes `\$p -> case $p { (a, b) -> body }`.

### Application

Left-associative:

```
f x                            -- function application
f x y                          -- = (f x) y
f @Int                         -- explicit type application
```

### Case Expression

```
case scrutinee {
  Con x y -> expr;
  _       -> expr
}
```

No `of` keyword. Branches separated by `;`. Patterns:

```
x                              -- variable binding
_                              -- wildcard
Con                            -- nullary constructor
Con x y                        -- constructor with arguments
(Con x y)                      -- parenthesized pattern
```

### Block Expression (let-bindings)

```
{ x := e1; y := e2; body }
```

Desugars to `(\x -> (\y -> body) e2) e1`.

### Do Block

```
do {
  x <- computation;            -- monadic bind
  y := pure_expr;              -- pure let-bind
  _ <- side_effect;            -- discard result
  pure result                  -- final expression (must be a Computation)
}
```

`x <- expr` desugars to `bind expr (\x -> ...)`. The entire do-block produces a `Computation`.

### Infix Operators

```
x + y                          -- operator syntax (if fixity is declared)
x `plus` y                     -- backtick syntax for any binary function
```

### Special Forms

```
pure expr                      -- lift a value into Computation
bind comp (\x -> body)         -- explicit monadic bind
thunk computation              -- suspend a computation into a value
force thunked_value            -- resume a suspended computation
```

### Expression Precedence

| Level | Form              | Associativity |
|-------|-------------------|---------------|
| 0     | `:: Type`         | right         |
| 1-9   | Infix operators   | per `infixl/r/n` |
| 10    | Application `f x` | left          |
| --    | Atoms             | --            |

### Type Expression Precedence

| Level | Form         | Associativity |
|-------|--------------|---------------|
| 0     | `forall`     | --            |
| 1     | `=>`         | right         |
| 2     | `->`         | right         |
| 3     | Application  | left          |
| --    | Atoms        | --            |

---

## 5. Effect System

### Computation pre post a

The core abstraction is `Computation pre post a` -- an Atkey-style parameterized monad (indexed monad). It represents an effectful computation that:

- Requires capability environment `pre` (a row type) at the start
- Produces capability environment `post` (a row type) at the end
- Returns a value of type `a`

When `pre` and `post` are the same, the computation preserves its environment. The type alias `Effect r a = Computation r r a` is provided for this common case.

### pure and bind

```
pure :: forall a (r : Row). a -> Computation r r a
bind :: forall a b (r1 : Row) (r2 : Row) (r3 : Row).
          Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

These are built-in -- always available without import. Note how `bind` composes pre/post indices: `r1->r2` then `r2->r3` yields `r1->r3`.

### Do-notation

```
do {
  x <- getState;               -- bind: extract value from Computation
  _ <- putState (x + 1);       -- bind: sequence, discard result
  y := x + 1;                  -- let: pure binding (no effect)
  pure y                       -- return result
}
```

Desugaring:

```
bind getState (\x ->
  bind (putState (x + 1)) (\_ ->
    (\y -> pure y) (x + 1)))
```

### Capability Environments

Capability environments are row types that describe what effects are available:

```
{}                                            -- no capabilities (pure)
{ state : Int }                               -- state holding an Int
{ state : Int, fail : String }                -- state and failure
{ io : Unit | r }                             -- io plus whatever else r contains
```

A function requiring state:

```
counter :: Computation { state : Int } { state : Int } Int
counter := do {
  n <- get;
  put (n + 1);
  pure n
}
```

CapEnv is copy-on-write: effects thread through Computation indices. `put` does not mutate; it produces a new CapEnv.

### thunk and force

`thunk` suspends a computation into a first-class value (CBPV's U):

```
thunk :: Computation pre post a -> Thunk pre post a
```

`force` runs a suspended computation:

```
force :: Thunk pre post a -> Computation pre post a
```

### then

Provided by the Prelude for sequencing when you do not need the intermediate result:

```
then :: forall a b (r1 : Row) (r2 : Row) (r3 : Row).
  Computation r1 r2 a -> Computation r2 r3 b -> Computation r1 r3 b
```

---

## 6. Prelude Reference

The Prelude is automatically loaded unless `NoPrelude` is set on the Engine. Everything below is available without any `import`.

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

### Type Aliases

```
type Effect r a = Computation r r a
type Lift (m : Type -> Type) (r1 : Row) (r2 : Row) a = m a
```

### Type Classes

**Eq**

```
class Eq a {
  eq :: a -> a -> Bool
}
```

**Ord**

```
class Eq a => Ord a {
  compare :: a -> a -> Ordering
}
```

**Semigroup**

```
class Semigroup a {
  append :: a -> a -> a
}
```

**Monoid**

```
class Semigroup a => Monoid a {
  empty :: a
}
```

**Functor**

```
class Functor f {
  fmap :: forall a b. (a -> b) -> f a -> f b
}
```

**Foldable**

```
class Foldable t {
  foldr :: forall a b. (a -> b -> b) -> b -> t a -> b
}
```

**Applicative**

```
class Functor f => Applicative f {
  wrap :: forall a. a -> f a;
  ap   :: forall a b. f (a -> b) -> f a -> f b
}
```

**Traversable**

```
class Functor t => Foldable t => Traversable t {
  traverse :: forall f a b. Applicative f => (a -> f b) -> t a -> f (t b)
}
```

**IxMonad**

```
class IxMonad (m : Row -> Row -> Type -> Type) {
  ixpure :: forall a (r : Row). a -> m r r a;
  ixbind :: forall a b (r1 : Row) (r2 : Row) (r3 : Row).
              m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}
```

### Instances

**IxMonad instances:**

| Instance                | Notes                                    |
|-------------------------|------------------------------------------|
| `IxMonad Computation`   | Uses built-in `pure`/`bind`              |
| `IxMonad Maybe`         | `Nothing` propagates; `Just a` applies f |
| `IxMonad List`          | Concatmap (list monad)                   |

**Eq instances:**

| Instance                          |
|-----------------------------------|
| `Eq Bool`                         |
| `Eq Unit`                         |
| `Eq Ordering`                     |
| `Eq a => Eq (Maybe a)`           |
| `Eq a => Eq b => Eq (Pair a b)` |
| `Eq a => Eq (List a)`           |

**Ord instances:**

| Instance                            |
|-------------------------------------|
| `Ord Bool`                          |
| `Ord Unit`                          |
| `Ord Ordering`                      |
| `Ord a => Ord (Maybe a)`           |
| `Ord a => Ord b => Ord (Pair a b)` |

**Semigroup instances:**

| Instance                                    |
|---------------------------------------------|
| `Semigroup Unit`                            |
| `Semigroup Ordering`                        |
| `Semigroup a => Semigroup (Maybe a)`       |
| `Semigroup (List a)`                        |

**Monoid instances:**

| Instance                                |
|-----------------------------------------|
| `Monoid Unit`                           |
| `Monoid Ordering`                       |
| `Semigroup a => Monoid (Maybe a)`      |
| `Monoid (List a)`                       |

**Functor instances:**

| Instance              |
|-----------------------|
| `Functor Maybe`       |
| `Functor (Pair a)`   |
| `Functor List`        |

**Foldable instances:**

| Instance               |
|------------------------|
| `Foldable Maybe`       |
| `Foldable (Pair a)`   |
| `Foldable List`        |

**Applicative instances:**

| Instance              |
|-----------------------|
| `Applicative Maybe`   |

**Traversable instances:**

| Instance                  |
|---------------------------|
| `Traversable Maybe`       |
| `Traversable (Pair a)`   |

### Functions

**Identity and combinators:**

```
id :: forall a. a -> a
id := \x -> x

const :: forall a b. a -> b -> a
const := \x -> \_ -> x

flip :: forall a b c. (a -> b -> c) -> b -> a -> c
flip := \f -> \b -> \a -> f a b
```

**Composition operator:**

```
infixr 9 .
(.) :: forall b c a. (b -> c) -> (a -> b) -> a -> c
(.) := \f -> \g -> \x -> f (g x)
```

**Boolean logic:**

```
not :: Bool -> Bool
not := \b -> case b { True -> False; False -> True }

infixr 3 &&
(&&) :: Bool -> Bool -> Bool
(&&) := \x -> \y -> case x { False -> False; True -> y }

infixr 2 ||
(||) :: Bool -> Bool -> Bool
(||) := \x -> \y -> case x { True -> True; False -> y }
```

**Maybe:**

```
maybe :: forall a b. b -> (a -> b) -> Maybe a -> b
maybe := \def -> \f -> \m -> case m { Nothing -> def; Just a -> f a }
```

**Result:**

```
result :: forall e a b. (e -> b) -> (a -> b) -> Result e a -> b
result := \onErr -> \onOk -> \r -> case r { Err e -> onErr e; Ok a -> onOk a }
```

**Pair:**

```
fst :: forall a b. Pair a b -> a
fst := \p -> case p { Pair a _ -> a }

snd :: forall a b. Pair a b -> b
snd := \p -> case p { Pair _ b -> b }
```

**List:**

```
head :: forall a. List a -> Maybe a
head := \xs -> case xs { Nil -> Nothing; Cons x _ -> Just x }

tail :: forall a. List a -> Maybe (List a)
tail := \xs -> case xs { Nil -> Nothing; Cons _ rest -> Just rest }

null :: forall a. List a -> Bool
null := \xs -> case xs { Nil -> True; Cons _ _ -> False }

map :: forall a b. (a -> b) -> List a -> List b
map := fmap

filter :: forall a. (a -> Bool) -> List a -> List a
filter := \p -> foldr (\x -> \acc -> case p x { True -> Cons x acc; False -> acc }) Nil

singleton :: forall a. a -> List a
singleton := \x -> Cons x Nil
```

**Comparison operators:**

```
infixn 4 ==
(==) :: forall a. Eq a => a -> a -> Bool
(==) := eq

infixn 4 /=
(/=) :: forall a. Eq a => a -> a -> Bool
(/=) := \x -> \y -> not (eq x y)

infixn 4 <
(<) :: forall a. Ord a => a -> a -> Bool
(<) := \x -> \y -> case compare x y { LT -> True; _ -> False }

infixn 4 >
(>) :: forall a. Ord a => a -> a -> Bool
(>) := \x -> \y -> case compare x y { GT -> True; _ -> False }

infixn 4 <=
(<=) :: forall a. Ord a => a -> a -> Bool
(<=) := \x -> \y -> case compare x y { GT -> False; _ -> True }

infixn 4 >=
(>=) :: forall a. Ord a => a -> a -> Bool
(>=) := \x -> \y -> case compare x y { LT -> False; _ -> True }
```

**Min / Max:**

```
min :: forall a. Ord a => a -> a -> a
min := \x -> \y -> case compare x y { GT -> y; _ -> x }

max :: forall a. Ord a => a -> a -> a
max := \x -> \y -> case compare x y { LT -> y; _ -> x }
```

**Monadic sequencing:**

```
then :: forall a b (r1 : Row) (r2 : Row) (r3 : Row).
  Computation r1 r2 a -> Computation r2 r3 b -> Computation r1 r3 b
then := \m1 -> \m2 -> bind m1 (\_ -> m2)
```

---

## 7. Stdlib Reference

Each stdlib module must be loaded on the host side (`eng.Use(gmp.Num)`) and imported in source (`import Std.Num`).

### Std.Num

Provides integer arithmetic.

**Type class:**

```
class Eq a => Num a {
  add    :: a -> a -> a;
  sub    :: a -> a -> a;
  mul    :: a -> a -> a;
  negate :: a -> a
}
```

**Instances:**

| Instance   |
|------------|
| `Eq Int`   |
| `Ord Int`  |
| `Num Int`  |

**Functions:**

| Name     | Type                   | Description           |
|----------|------------------------|-----------------------|
| `add`    | `forall a. Num a => a -> a -> a` | Addition (class method)  |
| `sub`    | `forall a. Num a => a -> a -> a` | Subtraction (class method) |
| `mul`    | `forall a. Num a => a -> a -> a` | Multiplication (class method) |
| `negate` | `forall a. Num a => a -> a`      | Negation (class method) |
| `div`    | `Int -> Int -> Int`    | Integer division      |
| `mod`    | `Int -> Int -> Int`    | Modulo                |
| `abs`    | `Int -> Int`           | Absolute value        |
| `sign`   | `Int -> Int`           | Sign (-1, 0, or 1)   |

**Operators:**

| Operator | Fixity     | Type                                   |
|----------|------------|----------------------------------------|
| `+`      | `infixl 6` | `forall a. Num a => a -> a -> a`       |
| `-`      | `infixl 6` | `forall a. Num a => a -> a -> a`       |
| `*`      | `infixl 7` | `forall a. Num a => a -> a -> a`       |
| `/`      | `infixl 7` | `Int -> Int -> Int`                    |

**Notes:**

- Integer literals (`42`) only parse when Std.Num is imported.
- Negative numbers: write `negate 5`, not `-5`.
- Division by zero is a runtime error.

### Std.Str

Provides string and rune operations.

**Instances:**

| Instance            |
|---------------------|
| `Eq String`         |
| `Ord String`        |
| `Semigroup String`  |
| `Monoid String`     |
| `Eq Rune`           |
| `Ord Rune`          |

**Functions:**

| Name       | Type                                  | Description                              |
|------------|---------------------------------------|------------------------------------------|
| `length`   | `String -> Int`                       | Length in runes (Unicode code points)     |
| `charAt`   | `Int -> String -> Maybe Rune`        | Rune at index (0-based), Nothing if out of range |
| `substring`| `Int -> Int -> String -> String`     | `substring start count s` extracts a substring |
| `toUpper`  | `String -> String`                    | Convert to uppercase                     |
| `toLower`  | `String -> String`                    | Convert to lowercase                     |
| `trim`     | `String -> String`                    | Trim leading/trailing whitespace         |
| `contains` | `String -> String -> Bool`           | `contains needle haystack`               |
| `split`    | `String -> String -> List String`    | `split separator string`                 |
| `join`     | `String -> List String -> String`    | `join separator parts`                   |
| `showInt`  | `Int -> String`                       | Convert Int to its decimal string        |
| `showBool` | `Bool -> String`                      | Convert Bool to "True" or "False"        |
| `readInt`  | `String -> Maybe Int`                | Parse decimal string to Int              |

**Notes:**

- `length` counts Unicode code points, not bytes.
- `charAt` and `substring` use 0-based rune indexing.
- String concatenation uses the `append` method from `Semigroup String`. The empty string is `empty` from `Monoid String`.
- No string interpolation. Build strings with `append` and conversion functions.

### Std.List

Provides list operations (native-speed implementations).

**Functions:**

| Name        | Type                                                   | Description                             |
|-------------|--------------------------------------------------------|-----------------------------------------|
| `fromSlice` | `forall a. List a -> List a`                          | Identity on Cons/Nil chains; converts HostVal slices |
| `toSlice`   | `forall a. List a -> List a`                          | Identity on Cons/Nil chains; converts to HostVal slice |
| `length`    | `forall a. List a -> Int`                             | Count elements                          |
| `concat`    | `forall a. List a -> List a -> List a`                | Concatenate two lists                   |
| `foldl`     | `forall a b. (b -> a -> b) -> b -> List a -> b`      | Strict left fold                        |
| `take`      | `forall a. Int -> List a -> List a`                   | First n elements                        |
| `drop`      | `forall a. Int -> List a -> List a`                   | Drop first n elements                   |
| `index`     | `forall a. Int -> List a -> Maybe a`                  | Element at index (0-based)              |
| `replicate` | `forall a. Int -> a -> List a`                        | List of n copies of a value             |
| `reverse`   | `forall a. List a -> List a`                          | Reverse a list                          |
| `zip`       | `forall a b. List a -> List b -> List (Pair a b)`     | Zip two lists into pairs                |
| `unzip`     | `forall a b. List (Pair a b) -> Pair (List a) (List b)` | Unzip a list of pairs                 |

**Notes:**

- When both Std.List and Std.Str are imported, `length` may be ambiguous. Qualify if needed, or use only one.
- `foldl` is strict (evaluates the accumulator at each step).
- The Prelude already provides `foldr`, `map`, `filter`, `head`, `tail`, `null`, `singleton`, and `append` for lists.

### Std.State

Provides get/put state capabilities via the `state` capability in CapEnv.

**Functions:**

| Name     | Type                                                                                   | Description                    |
|----------|----------------------------------------------------------------------------------------|--------------------------------|
| `get`    | `forall s r. Computation { state : s \| r } { state : s \| r } s`                     | Read current state             |
| `put`    | `forall s r. s -> Computation { state : s \| r } { state : s \| r } Unit`             | Replace current state          |
| `modify` | `forall s r. (s -> s) -> Computation { state : s \| r } { state : s \| r } Unit`      | Apply a function to state      |

**Host setup:** Provide the initial state as the `"state"` capability:

```go
caps := map[string]any{"state": gmp.ToValue(0)}
result, err := rt.RunContextFull(ctx, caps, nil, "main")
// result.CapEnv contains the final state
```

### Std.Fail

Provides failure/error effects via the `fail` capability.

**Functions:**

| Name         | Type                                                                                              | Description                           |
|--------------|---------------------------------------------------------------------------------------------------|---------------------------------------|
| `failWith`   | `forall e r a. e -> Computation { fail : e \| r } { fail : e \| r } a`                           | Fail with a typed error value         |
| `fail`       | `forall r a. Computation { fail : Unit \| r } { fail : Unit \| r } a`                            | Fail with Unit (no error payload)     |
| `fromMaybe`  | `forall a r. Maybe a -> Computation { fail : Unit \| r } { fail : Unit \| r } a`                 | Extract Just or fail on Nothing       |
| `fromResult` | `forall e a r. Result e a -> Computation { fail : e \| r } { fail : e \| r } a`                  | Extract Ok or failWith on Err         |

**Notes:**

- `fail` and `failWith` abort the computation. There is no catch/recover at the language level; the host handles the error.
- The return type is `a` (universally quantified), meaning failure can appear in any position.

### Std.IO

Provides print/debug capabilities via the `io` capability.

**Functions:**

| Name    | Type                                                                          | Description                         |
|---------|-------------------------------------------------------------------------------|-------------------------------------|
| `print` | `String -> Computation { io : Unit \| r } { io : Unit \| r } Unit`           | Append a string to the IO buffer    |
| `debug` | `forall a. a -> Computation { io : Unit \| r } { io : Unit \| r } Unit`      | Append debug representation to IO buffer |

**Host setup:** Provide the `"io"` capability. Output accumulates as `[]string` in the final CapEnv:

```go
caps := map[string]any{"io": gmp.ToValue(nil)}
result, err := rt.RunContextFull(ctx, caps, nil, "main")
// Read output:
ioVal, _ := result.CapEnv.Get("io")
lines := ioVal.([]string)
```

---

## 8. Operator Quick Reference

All operators sorted by precedence (highest binds tightest):

| Precedence | Operator | Associativity  | Type                                     | Source   |
|------------|----------|----------------|------------------------------------------|----------|
| 9          | `.`      | right          | `forall b c a. (b -> c) -> (a -> b) -> a -> c` | Prelude  |
| 7          | `*`      | left           | `forall a. Num a => a -> a -> a`         | Std.Num  |
| 7          | `/`      | left           | `Int -> Int -> Int`                      | Std.Num  |
| 6          | `+`      | left           | `forall a. Num a => a -> a -> a`         | Std.Num  |
| 6          | `-`      | left           | `forall a. Num a => a -> a -> a`         | Std.Num  |
| 4          | `==`     | non-associative | `forall a. Eq a => a -> a -> Bool`       | Prelude  |
| 4          | `/=`     | non-associative | `forall a. Eq a => a -> a -> Bool`       | Prelude  |
| 4          | `<`      | non-associative | `forall a. Ord a => a -> a -> Bool`      | Prelude  |
| 4          | `>`      | non-associative | `forall a. Ord a => a -> a -> Bool`      | Prelude  |
| 4          | `<=`     | non-associative | `forall a. Ord a => a -> a -> Bool`      | Prelude  |
| 4          | `>=`     | non-associative | `forall a. Ord a => a -> a -> Bool`      | Prelude  |
| 3          | `&&`     | right          | `Bool -> Bool -> Bool`                   | Prelude  |
| 2          | `\|\|`   | right          | `Bool -> Bool -> Bool`                   | Prelude  |

Undeclared operators default to `infixl 9`.

Non-associative (`infixn`) operators cannot be chained: `a == b == c` is a parse error. Write `(a == b) && (b == c)`.

---

## 9. Common Patterns

### Pattern Matching

```
-- Destructure Maybe
describe :: Maybe Bool -> String
describe := \m -> case m {
  Nothing -> "empty";
  Just b  -> case b { True -> "yes"; False -> "no" }
}

-- Nested patterns are not supported directly; nest case expressions.

-- Wildcard for catch-all
isZeroOrd :: Ordering -> Bool
isZeroOrd := \o -> case o { EQ -> True; _ -> False }
```

### List Processing

```
import Std.Num

-- Sum a list of Ints (uses Prelude foldr)
sum :: List Int -> Int
sum := foldr (\x -> \acc -> x + acc) 0

-- Build a list literal
myList :: List Int
myList := Cons 1 (Cons 2 (Cons 3 Nil))

-- Map and filter
evens :: List Int -> List Int
evens := filter (\x -> x `mod` 2 == 0)
```

Note: `mod` here is the function from Std.Num used with backtick syntax.

### Stateful Computation

```
import Std.Num
import Std.State

-- Increment a counter three times and return the final value
counter :: Computation { state : Int } { state : Int } Int
counter := do {
  modify (\n -> n + 1);
  modify (\n -> n + 1);
  modify (\n -> n + 1);
  get
}
```

### Error Handling

```
import Std.Num
import Std.Str
import Std.Fail

-- Parse an Int or fail
parseOrFail :: String -> Computation { fail : Unit | r } { fail : Unit | r } Int
parseOrFail := \s -> fromMaybe (readInt s)

-- Typed error
safeDivide :: Int -> Int -> Computation { fail : String | r } { fail : String | r } Int
safeDivide := \x -> \y -> case y == 0 {
  True  -> failWith "division by zero";
  False -> pure (div x y)
}
```

### Function Composition

```
-- The . operator (infixr 9) composes functions right-to-left
-- (f . g) x  =  f (g x)

import Std.Num

doubleNegate :: Int -> Int
doubleNegate := negate . negate

-- Pointfree pipeline
transform :: List Int -> List Int
transform := filter (\x -> x > 0) . map (\x -> x * 2)
```

### Combining Effects

```
import Std.Num
import Std.State
import Std.Fail

-- A computation that uses both state and fail
process :: Computation { state : Int, fail : Unit } { state : Int, fail : Unit } Int
process := do {
  n <- get;
  case n > 0 {
    True -> do { put (n - 1); pure n };
    False -> fail
  }
}
```

### Thunk and Force

```
-- Suspend a computation for later
suspended :: Thunk {} {} Bool
suspended := thunk (pure True)

-- Resume it
resumed :: Computation {} {} Bool
resumed := force suspended
```

---

## 10. Pitfalls

### No multi-parameter lambdas

Wrong: `\x y -> x + y`
Correct: `\x -> \y -> x + y`

### Int literals require Std.Num

Without `import Std.Num`, writing `42` is a parse error. The Prelude alone has no numeric literals.

### No negative literals

Wrong: `-5`
Correct: `negate 5` (requires Std.Num)

### Type annotation is a separate declaration

Wrong:
```
f := \x -> x :: forall a. a -> a
```
Correct:
```
f :: forall a. a -> a
f := \x -> x
```

The `:: Type` inside an expression is a type ascription (annotation on the expression), not a declaration-level signature. Declaration-level signatures must be on their own line before the definition.

### case uses braces, not "of"

Wrong: `case x of { True -> 1 }`
Correct: `case x { True -> True }`

### No string interpolation

Wrong: `"count is ${n}"`
Correct: `append "count is " (showInt n)` (requires Std.Str)

### Non-associative operators cannot chain

Wrong: `a == b == c`
Correct: `(a == b) && (b == c)`

### No general recursion without rec/fix

By default, `rec` and `fix` are gated (disabled). Without them, you cannot define recursive functions at the value level. The Prelude's `foldr` and Std.List's `foldl` provide recursion for list processing. The host must call `eng.EnableRecursion()` to unlock `rec`/`fix`.

### The dot is overloaded

`.` is both the `forall` body separator and the compose operator (`infixr 9`). Context disambiguates: after `forall a`, the `.` separates quantifier from body. Everywhere else, it is function composition.

### import must come first

All `import` declarations must appear before any other declarations. This is enforced by the parser.

### Operator definitions require parentheses

To define an operator, wrap it in parentheses:

```
infixl 6 +
(+) :: forall a. Num a => a -> a -> a
(+) := add
```

### Prelude names shadow freely

The Prelude provides `length` for lists (`List a -> Bool` via `null` -- actually `head`/`tail`). Std.List and Std.Str also export `length` with different types. Importing multiple modules that export the same name can cause ambiguity.

### CapEnv capabilities must be provided by the host

If your program uses `get`/`put` (Std.State), the Go host must supply the initial `"state"` capability. If it uses `print` (Std.IO), the host must supply the `"io"` capability. Forgetting this causes a runtime error, not a compile error.

### Computation {} {} a is pure

A `Computation` with empty row indices `{}` requires no capabilities. It is essentially pure but still lives in the Computation type. You can always `pure x` to create one.

### Semicolons and newlines

`;` and newlines are both valid declaration separators at the **top level**. Trailing and repeated semicolons are harmless. **Inside braces** (`do { }`, `case { }`, GADT declarations), semicolons are **required** separators — newlines alone do not separate statements or alternatives within braces.

---

## 11. Go Integration

### Sandbox API

The simplest way to run Gomputation from Go:

```go
import gmp "github.com/cwd-k2/gomputation"

result, err := gmp.RunSandbox(source, &gmp.SandboxConfig{
    Packs:    []gmp.Pack{gmp.Num, gmp.Str, gmp.List, gmp.Fail, gmp.State, gmp.IO},
    Entry:    "main",              // default: "main"
    Timeout:  5 * time.Second,     // default: 5s
    MaxSteps: 100_000,             // default: 100,000
    MaxDepth: 100,                 // default: 100
    Caps:     map[string]any{      // initial capabilities
        "state": gmp.ToValue(0),
        "io":    gmp.ToValue(nil),
    },
    Bindings: map[string]gmp.Value{  // host-provided variable values
        "input": gmp.ToValue("hello"),
    },
})
```

`SandboxConfig` fields are all optional. Passing `nil` uses conservative defaults (no packs, entry "main", 5s timeout, 100k steps, depth 100).

`SandboxResult` contains:

| Field    | Type        | Description                       |
|----------|-------------|-----------------------------------|
| `Value`  | `Value`     | The result of evaluating `entry`  |
| `CapEnv` | `CapEnv`    | Final capability environment      |
| `Stats`  | `EvalStats` | Steps taken and max depth reached |

### Full Lifecycle API

```go
// 1. Create and configure the engine
eng := gmp.NewEngine()
eng.Use(gmp.Num)
eng.Use(gmp.Str)
eng.SetStepLimit(500_000)
eng.SetDepthLimit(200)

// 2. Compile to immutable Runtime (goroutine-safe, reusable)
rt, err := eng.NewRuntime(source)

// 3. Execute (can call many times with different inputs)
result, err := rt.RunContext(ctx, caps, bindings, "main")
// or with full CapEnv in result:
result, err := rt.RunContextFull(ctx, caps, bindings, "main")
```

### Available Packs

| Pack        | Variable    | Module Name  | Provides                                |
|-------------|-------------|--------------|-----------------------------------------|
| `gmp.Num`   | `Num`       | `Std.Num`    | Arithmetic, Int instances               |
| `gmp.Str`   | `Str`       | `Std.Str`    | String/Rune ops, instances              |
| `gmp.List`  | `List`      | `Std.List`   | Native list operations                  |
| `gmp.Fail`  | `Fail`      | `Std.Fail`   | Failure effect                          |
| `gmp.State` | `State`     | `Std.State`  | Get/put state                           |
| `gmp.IO`    | `IO`        | `Std.IO`     | Print/debug output                      |

### Custom Primitives (RegisterPrim)

Register a Go function as a primitive callable from Gomputation:

```go
// In source: declare type and mark as assumption
// greet :: String -> Unit
// greet := assumption

eng.RegisterPrim("greet", func(
    ctx context.Context,
    capEnv gmp.CapEnv,
    args []gmp.Value,
    apply gmp.Applier,
) (gmp.Value, gmp.CapEnv, error) {
    s := gmp.MustHost[string](args[0])
    fmt.Println("Hello,", s)
    return gmp.ToValue(nil), capEnv, nil  // nil -> Unit
})
```

**PrimImpl signature:** `func(ctx, capEnv, args, apply) -> (Value, CapEnv, error)`

- `ctx`: context for cancellation/timeout
- `capEnv`: current capability environment (copy-on-write)
- `args`: fully-applied argument values (the runtime curries automatically)
- `apply`: callback to apply a Gomputation closure to an argument (for higher-order primitives like `foldl`)

The primitive must return the new CapEnv (or the same one if unchanged).

### Host Bindings (DeclareBinding)

Inject a typed variable from Go that the source can reference:

```go
eng.RegisterType("Int", gmp.KindType())
eng.DeclareBinding("myInput", gmp.ConType("Int"))

// At runtime, provide the value:
bindings := map[string]gmp.Value{
    "myInput": gmp.ToValue(42),
}
result, err := rt.RunContext(ctx, nil, bindings, "main")
```

In source, `myInput` is available as a variable of type `Int`.

### Value Conversion Helpers

| Function                       | Description                                    |
|--------------------------------|------------------------------------------------|
| `gmp.ToValue(v any) Value`    | Wrap Go value: nil->Unit, bool->True/False, else->HostVal |
| `gmp.FromBool(v) (bool, bool)` | Extract Bool constructor                      |
| `gmp.FromHost(v) (any, bool)` | Extract inner value from HostVal               |
| `gmp.FromCon(v) (name, args, ok)` | Extract constructor name and arguments     |
| `gmp.MustHost[T](v) T`       | Extract typed HostVal, panics on mismatch      |
| `gmp.ToList(items []any) Value` | Build a Cons/Nil chain from Go slice         |
| `gmp.FromList(v) ([]any, bool)` | Destructure Cons/Nil chain to Go slice       |

### Type Construction Helpers

For use with `DeclareBinding` and `DeclareAssumption`:

| Function                             | Description                           |
|--------------------------------------|---------------------------------------|
| `gmp.ConType(name) Type`           | Simple type constructor: `"Int"`, `"Bool"` |
| `gmp.VarType(name) Type`           | Type variable reference               |
| `gmp.ArrowType(from, to) Type`     | Function type: `from -> to`           |
| `gmp.AppType(f, arg) Type`         | Type application: `f a`              |
| `gmp.CompType(pre, post, result) Type` | `Computation pre post result`     |
| `gmp.ThunkType(pre, post, result) Type` | `Thunk pre post result`          |
| `gmp.ForallType(var, body) Type`   | `forall var. body` (kind Type)        |
| `gmp.ForallRow(var, body) Type`    | `forall (var : Row). body`            |
| `gmp.ForallKind(var, k, body) Type` | `forall (var : k). body`            |
| `gmp.EmptyRowType() Type`          | Empty closed row `{}`                |
| `gmp.KindType() Kind`              | The `Type` kind                       |
| `gmp.KindRow() Kind`               | The `Row` kind                        |
| `gmp.KindArrow(from, to) Kind`     | Kind arrow: `from -> to`             |

**RowBuilder** for constructing row types:

```go
row := gmp.NewRow().And("state", gmp.ConType("Int")).And("fail", gmp.ConType("String")).Closed()
// produces { state : Int, fail : String }

openRow := gmp.NewRow().And("state", gmp.ConType("Int")).Open("r")
// produces { state : Int | r }
```

### Engine Configuration Methods

| Method                            | Description                                 |
|-----------------------------------|---------------------------------------------|
| `eng.Use(pack Pack) error`       | Apply a stdlib pack                         |
| `eng.RegisterPrim(name, impl)`   | Register a primitive implementation         |
| `eng.RegisterType(name, kind)`   | Register an opaque host type                |
| `eng.DeclareBinding(name, ty)`   | Declare a host-provided variable            |
| `eng.DeclareAssumption(name, ty)` | Declare a primitive type (usually not needed if `::` is used in source) |
| `eng.EnableRecursion()`          | Enable `rec` and `fix` built-ins            |
| `eng.SetStepLimit(n int)`        | Set maximum evaluation steps                |
| `eng.SetDepthLimit(n int)`       | Set maximum call depth                      |
| `eng.NoPrelude()`               | Disable automatic Prelude loading           |
| `eng.SetTraceHook(hook)`        | Set evaluation trace callback               |
| `eng.SetCheckTraceHook(hook)`   | Set type checking trace callback            |
| `eng.RegisterModule(name, src)`  | Compile and register a custom module        |
| `eng.NewRuntime(source) (*Runtime, error)` | Compile source to Runtime        |
| `eng.Check(source) (*CoreProgram, error)`  | Type-check without creating Runtime |
| `eng.Parse(source) (*ParsedProgram, error)` | Parse without type-checking      |

### Error Handling

Compilation errors are returned as `*gmp.CompileError`, which wraps structured error information with source locations. Runtime errors (step limit exceeded, depth limit exceeded, missing capability, division by zero, explicit `fail`) are returned as plain `error` values.
