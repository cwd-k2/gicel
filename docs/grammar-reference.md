# GICEL Grammar Reference

## Lexical Structure

### Keywords (10)

| Keyword    | Purpose                           |
| ---------- | --------------------------------- |
| `case`     | Pattern matching                  |
| `do`       | Monadic do-block                  |
| `data`     | Algebraic data type declaration   |
| `type`     | Type alias declaration            |
| `infixl`   | Left-associative operator fixity  |
| `infixr`   | Right-associative operator fixity |
| `infixn`   | Non-associative operator fixity   |
| `class`    | Type class declaration            |
| `instance` | Type class instance declaration   |
| `import`   | Module import                     |

### Built-in Identifiers

| Identifier   | Role                              | Status           |
| ------------ | --------------------------------- | ---------------- |
| `pure`       | Value → Computation (F)           | first-class fn   |
| `bind`       | Monadic sequencing                | first-class fn   |
| `thunk`      | Computation → suspended value (U) | term former      |
| `force`      | Elimination of U                  | term former      |
| `assumption` | Host-provided primitive marker    | declaration form |
| `rec`        | Recursive combinator (gated)      | first-class fn   |
| `fix`        | Value-level fixpoint (gated)      | first-class fn   |

### Punctuation & Operators

| Token | Meaning                            |
| ----- | ---------------------------------- |
| `->`  | Function type / case alternative   |
| `<-`  | Monadic bind in do-block           |
| `=>`  | Constraint qualifier               |
| `::`  | Type annotation                    |
| `:=`  | Value definition / let-bind        |
| `:`   | Kind annotation separator          |
| `.`   | Lambda / quantifier body separator |
| `\`   | Lambda / universal quantification  |
| `_`   | Wildcard pattern                   |
| `=`   | Data constructor separator         |
| `@`   | Explicit type application          |
| `\|`  | Constructor / row tail separator   |
| `;`   | Declaration / statement separator  |

### Identifiers

- **Lowercase** `[a-z_][a-zA-Z0-9_']*` — variables, type variables, field labels
- **Uppercase** `[A-Z][a-zA-Z0-9_']*` — constructors, type constructors, class names
- **Operators** sequences of `! # $ % & * + - / < = > ? ^ ~ |` (excluding reserved; `:` and `.` are handled specially by the lexer)

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

Free type variables are implicitly universally quantified:

```
f :: \a. Eq a => a -> a -> Bool
myLength :: List a -> Int              -- same as: \a. List a -> Int
```

### Value Definition

```
name := Expr
```

Example:

```
not := \b. case b { True -> False; False -> True }
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
(+) :: \a. Num a => a -> a -> a
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

### Type Family (Closed)

```
TypeFamilyDecl
  = 'type' UpperName TyBinder* '::' ResultKind '=' '{' Equation (';' Equation)* '}'

ResultKind
  = KindExpr
  | '(' LowerName ':' KindExpr ')' '|' DepList

DepList
  = LowerName '->' LowerName+

Equation
  = UpperName TypePattern* '=' TypeExpr
```

Distinguished from a type alias by `::` after the parameters. Equations are checked top-to-bottom; first match wins. Reduction is stuck (not skipped) when a match is indeterminate due to unsolved metavariables.

Examples:

```
type Elem (c : Type) :: Type = {
  Elem (List a) = a;
  Elem String = Rune
}

-- Injective (named result with functional dependency)
type Effects (mode : AppMode) :: (r : Row) | r -> mode = {
  Effects ReadOnly  = { get: () -> String };
  Effects ReadWrite = { get: () -> String, put: String -> () }
}
```

### Type Class

```
class [Constraint =>] ClassName param* [ClassFunDep] {
  method1 :: TypeExpr;
  method2 :: TypeExpr;
  [AssocTypeDecl]*;
  [AssocDataDecl]*
}

ClassFunDep (after class params, before '{')
  = '|' LowerName '->' LowerName+ (',' LowerName '->' LowerName+)*

AssocTypeDecl (in class body)
  = 'type' UpperName TyBinder* '::' ResultKind

AssocDataDecl (in class body)
  = 'data' UpperName TyBinder* '::' KindExpr
```

Examples:

```
class Eq a { eq :: a -> a -> Bool }
class Eq a => Ord a { compare :: a -> a -> Ordering }
class Functor f { fmap :: \a b. (a -> b) -> f a -> f b }

-- Associated type in class
class Container c {
  type Elem c :: Type;
  cfold :: \b. (Elem c -> b -> b) -> b -> c -> b
}

-- Functional dependency
class Convert a b | a -> b {
  convert :: a -> b
}

-- Associated data family in class
class Collection c {
  data Key c :: Type;
  lookup :: Key c -> c -> Maybe (Elem c)
}
```

### Type Class Instance

```
instance [Constraint =>] ClassName TypeArg* {
  method1 := Expr;
  method2 := Expr;
  [AssocTypeDef]*;
  [AssocDataDef]*
}

AssocTypeDef (in instance body)
  = 'type' UpperName TypePattern* '=' TypeExpr

AssocDataDef (in instance body)
  = 'data' UpperName TypePattern* '=' ConDecl ('|' ConDecl)*
```

Examples:

```
instance Eq Bool { eq := \x y. True }
instance Eq a => Eq (Maybe a) {
  eq := \x y. case x {
    Nothing -> case y { Nothing -> True; Just _ -> False };
    Just a  -> case y { Nothing -> False; Just b -> eq a b }
  }
}

-- Associated type definition in instance
instance Container (List a) {
  type Elem (List a) = a;
  cfold := foldr
}

-- Associated data family definition in instance
instance Collection (List a) {
  data Key (List a) = ListIndex Int;
  lookup := \k xs. case k {
    ListIndex i -> index xs i
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
\param. body
\x y z. expr           -- multi-parameter (desugars to \x. \y. \z. expr)
\(Con x y). expr       -- constructor pattern
\(a, b). expr          -- tuple pattern
\{ x, y }. expr        -- record pattern
```

### Application

```
f x           -- function application (left-associative)
f x y         -- = (f x) y
f @Int        -- explicit type application
```

### Case Expression

```
case scrutinee {
  Con x y -> expr;
  _       -> expr
}
```

### Record Literal

```
{ x: 1, y: True }              -- record construction
{ r | x: 42 }                  -- record update (functional)
```

### Record Projection

```
r.#x                            -- project field x from record r
r.#_1                           -- project first element of tuple
```

`.#` binds at atom level (tighter than function application).

### Operator Section

Three forms:

```
(+)                             -- prefix: operator as first-class value
(+ 1)                           -- right section: \x. x + 1
(1 +)                           -- left section:  \x. 1 + x
```

`(op)` wraps an operator in parentheses to produce a regular value. `(op expr)` binds the right argument, `(expr op)` binds the left argument. Both sections desugar to single-argument lambdas.

```
foldr (+) 0 xs                  -- pass (+) to higher-order function
map (+ 1) xs                    -- right section: increment each element
filter (0 <) xs                 -- left section: keep positives
```

This is the expression-level counterpart of the declaration syntax `(op) := ...`.

### Tuple

```
(1, True)                       -- 2-tuple, desugars to { _1: 1, _2: True }
(1, True, "hello")              -- 3-tuple
(1)                             -- grouping (not a 1-tuple)
```

### List Literal

```
[1, 2, 3]                      -- desugars to Cons 1 (Cons 2 (Cons 3 Nil))
[]                              -- Nil
```

### Block Expression

```
{ x := e1; y := e2; body }
```

Desugars to `(\x. (\y. body) e2) e1`.

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
thunk computation            -- U: computation → value (term former)
force thunked_value          -- elimination of U (term former)
```

### Built-in Functions

```
pure expr                    -- F: value → computation (first-class function)
bind comp (\x. body)         -- monadic bind (first-class function)
```

`pure` and `bind` are first-class functions: they can be partially applied and passed to higher-order functions (e.g. `map pure xs`). When fully applied, the checker optimizes them to direct Core nodes for capability environment threading.

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
(Eq a, Ord b) => a -> b -> Bool           -- constraint tuple
(Eq a, Show a, Ord a) => a -> Bool        -- multiple constraints
```

Multiple constraints use tuple syntax: `(C1, C2, ...) => T`. Single constraints remain bare: `C => T`.

### Quantified Constraints

```
(\a. Eq a => Eq (f a)) => f Bool -> f Bool -> Bool
(\a. (Eq a, Show a) => Eq (f a)) => ...                  -- multiple premises
(Show Bool, (\a. Eq a => Eq (f a))) => ...                -- mixed with tuple
```

A quantified constraint `\vars. context => head` asserts that, for any instantiation of `vars`, if the `context` constraints hold, then the `head` constraint holds. Evidence for a quantified constraint is a _function_ from context dictionaries to the head dictionary:

```
-- Evidence type for (\a. Eq a => Eq (f a)):
-- \a. Eq$Dict a -> Eq$Dict (f a)
```

At use sites, the quantified constraint is resolved by finding a matching global instance. For example, `instance Eq a => Eq (F a)` satisfies `\a. Eq a => Eq (F a)`.

Within a function body, quantified evidence can be applied to produce dictionaries for specific types. If `f` has constraint `(\a. Eq a => Eq (g a))`, then `eq (x :: g Bool) y` resolves `Eq (g Bool)` by applying the quantified evidence to `Bool` and the `Eq Bool` dictionary.

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
withDict :: \a. Dict (Eq a) -> a -> a -> Bool
withDict := \d x y. case d { MkDict -> eq x y }
```

The user writes `MkDict` with zero explicit pattern arguments; the evidence field is implicit. Inside the branch body, the constraint `Eq a` is available for resolution.

Constraint-kinded parameters can coexist with regular parameters:

```
data Evidence (c : Constraint) a = MkEvidence c a
```

Here `c` is the implicit evidence field and `a` is a regular field.

### Universal Quantification

```
\a. a -> a
\a b. a -> b -> a
\(r : Row). Computation r r a
\(f : Type -> Type). f a -> f b
```

`\` serves dual purpose: lambda in expression context (`\x. e`) and universal quantification in type context (`\a. T`). Both use `.` as the body separator. The parser disambiguates by context. Multi-parameter lambdas are supported: `\x y z. e` desugars to `\x. \y. \z. e`.

### Row Type

```
{}                              -- empty row (closed)
{ x: Int, y: Bool }            -- closed row
{ x: Int | r }                 -- open row (tail variable)
{ get: () -> Int | r }         -- capability row
{ h: Handle @Linear | r }     -- multiplicity-annotated field
```

Row field grammar (updated):

```
RowField
  = LowerName ':' TypeExpr ('@' TypeAtom)?
```

The optional `@Mult` suffix annotates a field with a multiplicity (e.g., `@Linear`, `@Affine`). Without annotation, fields are `@Unrestricted`.

### Record / Tuple Type

```
Record { x: Int, y: Bool }     -- record type
(Int, Bool)                     -- tuple type, desugars to Record { _1: Int, _2: Bool }
(Int, Bool, String)             -- 3-tuple type
```

### Parenthesized Type

```
(a -> b)          -- grouping
(Maybe a)         -- grouping
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
\(c : Constraint). Bool                    -- constraint-kinded param
\a (c : Constraint). a -> Bool             -- mixed kinds
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
x                -- variable binding
_                -- wildcard
42               -- integer literal pattern
"hello"          -- string literal pattern
'a'              -- rune literal pattern
Con              -- nullary constructor
Con x y          -- constructor with arguments
(Con x y)        -- parenthesized pattern
(a, b)           -- tuple pattern, desugars to { _1: a, _2: b }
{ x: a, y: b }   -- record pattern (open by default)
```

### Literal Patterns

Integer, string, and rune literals can appear as case patterns. They match by equality:

```
case n { 0 -> "zero"; 1 -> "one"; _ -> "other" }
case name { "Alice" -> "hello"; _ -> "hi" }
case ch { 'x' -> True; _ -> False }
```

Since literal types cannot be exhaustively enumerated, a wildcard or variable catch-all is always required.

### Nested Patterns

Constructor patterns can be nested. Nullary constructors need no parentheses; multi-argument constructors must be parenthesized:

```
case m { Just True -> "yes"; Just False -> "no"; Nothing -> "none" }
case xs { Cons Nothing rest -> rest; Cons (Just x) rest -> rest; Nil -> Nil }
case m { Just (Just (Just True)) -> "deep"; _ -> "other" }
```

---

## Parser Precedence (Expressions)

| Level | Form              | Associativity    |
| ----- | ----------------- | ---------------- |
| 0     | `:: Type`         | right            |
| 1–9   | Infix operators   | per `infixl/r/n` |
| 10    | Application `f x` | left             |
| —     | Atoms             | —                |

### Type Expression Precedence

| Level | Form        | Associativity |
| ----- | ----------- | ------------- |
| 0     | `\ ... .`   | —             |
| 1     | `=>`        | right         |
| 2     | `->`        | right         |
| 3     | Application | left          |
| —     | Atoms       | —             |

---

## Declaration Boundaries

Declarations are separated by newlines or semicolons at the top level. Both separators are interchangeable at the top-level declaration scope; trailing and repeated semicolons are permitted.

At nesting depth 0, a new declaration begins when the next token (preceded by a newline or semicolon) is one of:

`lowercase` | `uppercase` | `data` | `type` | `infixl` | `infixr` | `infixn` | `class` | `instance` | `import` | `(op)` (operator definition)

Inside braces (`do`, `case`, block expressions, GADT declarations), semicolons are **required** separators between statements, branches, or constructors. Newlines alone do not act as separators within braces.

---

## Built-in Types

| Type                     | Kind                      | Description           |
| ------------------------ | ------------------------- | --------------------- |
| `Computation pre post a` | `Row → Row → Type → Type` | Effectful computation |
| `Thunk pre post a`       | `Row → Row → Type → Type` | Suspended computation |
| `Int`                    | `Type`                    | 64-bit integer        |
| `String`                 | `Type`                    | Unicode string        |
| `Rune`                   | `Type`                    | Unicode code point    |
| `Slice a`                | `Type → Type`             | Contiguous array      |
| `Map k v`                | `Type → Type → Type`      | Ordered immutable map |
| `Set a`                  | `Type → Type`             | Ordered immutable set |

---

## Prelude

The Prelude is auto-included unless `NoPrelude` is set. Full reference: [agent-guide/prelude.md](agent-guide/prelude.md).

### Data Types and Constructors

```
data Bool = True | False
data Ordering = LT | EQ | GT
data Result e a = Ok a | Err e
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
```

`()` is the unit type (empty record). `(a, b)` is the tuple type (sugar for `Record { _1: a, _2: b }`).

### Operators

```
infixr 9 .         -- function composition
infixr 6 <>        -- Semigroup append
infixl 4 <$>       -- Functor map
infixl 4 <*>       -- Applicative apply
infixl 4 *>        -- Applicative sequence
infixl 4 <*        -- Applicative discard
infixn 4 ==  /=  <  >  <=  >=
infixr 3 &&        -- logical AND
infixl 3 <|>       -- Alternative choice
infixr 2 ||        -- logical OR
infixl 1 >>=       -- Monad bind
infixl 1 >>        -- Monad sequence
infixl 1 <&>       -- flipped Functor map
infixl 1 &         -- reverse application
infixr 1 =<<       -- flipped Monad bind
infixr 1 >=>       -- Kleisli left-to-right
infixr 1 <=<       -- Kleisli right-to-left
infixr 0 $         -- low-precedence apply
```

### Type Classes (13 in Prelude, 14 with Std.Num)

```
Eq ──→ Ord
Show
Semigroup ──→ Monoid
Functor ──→ Applicative ──→ Alternative
                        ──→ Monad
Functor ─┐
         ├──→ Traversable
Foldable ┘
IxMonad
Packed

Eq ──→ Num   (in Std.Num)
```

| Class         | Key Methods                                                 |
| ------------- | ----------------------------------------------------------- |
| `Eq`          | `eq :: a -> a -> Bool`                                      |
| `Ord`         | `compare :: a -> a -> Ordering`                             |
| `Show`        | `show :: a -> String`                                       |
| `Semigroup`   | `append :: a -> a -> a`                                     |
| `Monoid`      | `empty :: a`                                                |
| `Functor`     | `fmap :: (a -> b) -> f a -> f b`                            |
| `Foldable`    | `foldr :: (a -> b -> b) -> b -> t a -> b`                   |
| `Applicative` | `wrap :: a -> f a`, `ap :: f (a -> b) -> f a -> f b`        |
| `Alternative` | `none :: f a`, `alt :: f a -> f a -> f a`                   |
| `Monad`       | `mpure :: a -> m a`, `mbind :: m a -> (a -> m b) -> m b`    |
| `Traversable` | `traverse :: Applicative f => (a -> f b) -> t a -> f (t b)` |
| `IxMonad`     | `ixpure`, `ixbind`                                          |
| `Packed`      | `pack :: List e -> c`, `unpack :: c -> List e`              |

---

## Stdlib Packs

Stdlib packs are loaded via `Engine.Use(pack)` on the host side and `import Std.X` in source. Full reference: [agent-guide/stdlib.md](agent-guide/stdlib.md).

| Pack         | Provides                                                      |
| ------------ | ------------------------------------------------------------- |
| `Std.Num`    | `Num` class, `Int` instances, arithmetic operators (`+−*/`)   |
| `Std.Str`    | String/Rune instances, string operations, `showInt`/`readInt` |
| `Std.List`   | Native-speed list operations (`length`, `foldl`, `zip`, etc.) |
| `Std.Fail`   | Fail effect (`failWith`, `fromMaybe`, `fromResult`)           |
| `Std.State`  | State effect (`get`, `put`, `modify`)                         |
| `Std.IO`     | IO effect (`print`, `debug`)                                  |
| `Std.Stream` | Lazy streams (`Stream a`), requires recursion                 |
| `Std.Slice`  | Contiguous arrays (`Slice a`), O(1) length/index              |
| `Std.Map`    | Ordered immutable map (AVL), requires `Ord k`                 |
| `Std.Set`    | Ordered immutable set (backed by Map), requires `Ord k`       |
