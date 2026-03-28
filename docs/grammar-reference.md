# GICEL Grammar Reference

## Lexical Structure

### Keywords (12 + 1 contextual)

| Keyword  | Purpose                                                                                              |
| -------- | ---------------------------------------------------------------------------------------------------- |
| `case`   | Pattern matching                                                                                     |
| `do`     | Monadic do-block                                                                                     |
| `form`   | Algebraic form type / type class declaration                                                         |
| `type`   | Type alias / type family declaration                                                                 |
| `impl`   | Type class instance declaration                                                                      |
| `infixl` | Left-associative operator fixity                                                                     |
| `infixr` | Right-associative operator fixity                                                                    |
| `infixn` | Non-associative operator fixity                                                                      |
| `import` | Module import                                                                                        |
| `if`     | Conditional expression (if-then-else)                                                                |
| `then`   | Conditional expression (if-then-else)                                                                |
| `else`   | Conditional expression (if-then-else)                                                                |
| `as`     | Qualified import alias (contextual — only special after `import`, usable as variable name elsewhere) |

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

| Token | Meaning                                                                         |
| ----- | ------------------------------------------------------------------------------- |
| `->`  | Function type                                                                   |
| `<-`  | Monadic bind in do-block                                                        |
| `=>`  | Constraint qualifier / case alternative / grade annotation / evidence injection |
| `::`  | Type annotation                                                                 |
| `:=`  | Value definition / let-bind                                                     |
| `:`   | Field / method type separator                                                   |
| `.`   | Lambda / quantifier body separator                                              |
| `\`   | Lambda / universal quantification                                               |
| `_`   | Wildcard pattern                                                                |
| `~`   | Type equality constraint                                                        |
| `@`   | Explicit type application                                                       |
| `\|`  | Constructor / row tail separator                                                |
| `;`   | Declaration / statement separator                                               |

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

Unsigned decimal integers: `[0-9]+`. Underscore separators allowed: `100_000`. Negative values via `negate`.

### Double Literals

Decimal point or exponent makes a numeric literal a Double: `3.14`, `1e10`, `1.05e+10`, `2_000.5e-3`. Underscore separators allowed in the integer part.

### String Literals

Double-quoted: `"hello world"`. Escape sequences: `\n`, `\t`, `\r`, `\\`, `\"`, `\'`, `\0`.

### Rune Literals

Single-quoted single character: `'a'`, `'\n'`. Same escape sequences as strings.

---

## Declarations

### Data Type (ADT shorthand)

```
form Name := Con field* (| Con field*)*
```

Parameters can be bare type variables or kinded: `(name: Kind)`.

Examples:

```
form Bool := True | False
form Maybe a := Just a | Nothing
form Result e a := Ok a | Err e
form List a := Cons a (List a) | Nil
form Dict (c: Constraint) := MkDict c    -- Constraint-kinded param
form Evidence (c: Constraint) a := MkEvidence c a
```

### Data Type (GADT-style full constructor types)

```
form Name := [\param*.] {
  Con: TypeExpr;
  Con: TypeExpr
}
```

Each constructor declares its full type including the return type. Distinguished from ADT shorthand by `:= {` or `:= \params. {`. The parameter lambda and brace body form are used for both type classes and GADTs.

Examples:

```
form Expr := \a. {
  LitBool: Bool -> Expr Bool;
  LitInt:  Int -> Expr Int;
  Not:     Expr Bool -> Expr Bool;
  Add:     Expr Int -> Expr Int -> Expr Int
}

form List := \a. {
  Nil:  List a;
  Cons: a -> List a -> List a
}
```

GADT constructors enable type refinement in `case` branches: matching `LitBool` on `Expr a` refines `a ~ Bool`. Exhaustiveness checking filters constructors whose return type cannot unify with the scrutinee type.

### Type Class (via `form`)

Type classes are declared using `form` with a brace body containing method signatures. Method types use `:` (not `::`).

```
form ClassName := [\param*.] [Constraint =>] {
  method1: TypeExpr;
  method2: TypeExpr;
  [AssocTypeDecl]*;
  [AssocFormDecl]*
}
```

Associated type and form declarations within the class body:

```
AssocTypeDecl (in class body)
  = 'type' UpperName TyBinder* '::' ResultKind

AssocFormDecl (in class body)
  = 'form' UpperName TyBinder* '::' KindExpr
```

Examples:

```
form Eq := \a. { eq: a -> a -> Bool }
form Ord := \a. Eq a => { compare: a -> a -> Ordering }
form Functor := \f. { fmap: \a b. (a -> b) -> f a -> f b }

-- Associated type in class
form Container := \c. {
  type Elem c :: Type;
  cfold: \b. (Elem c -> b -> b) -> b -> c -> b
}

-- Associated form family in class
form Collection := \c. {
  form Key c :: Type;
  lookup: Key c -> c -> Maybe (Elem c)
}
```

### Type Class Instance (`impl`)

```
impl [Constraint =>] ClassName TypeArg* := {
  method1 := Expr;
  method2 := Expr;
  [AssocTypeDef]*;
  [AssocFormDef]*
}

impl _name :: [Constraint =>] ClassName TypeArg* := Expr

AssocTypeDef (in impl body)
  = 'type' UpperName ':=' TypeExpr

AssocFormDef (in impl body)
  = 'form' UpperName ':=' ConDecl ('|' ConDecl)*
```

**Private instances.** An `impl` declaration with a name starting with `_` is _private_ — it is invisible to the constraint solver and will not be selected during automatic instance resolution. Private instances are accessible only through explicit evidence injection (`value => expr`):

```
impl _myEq :: Eq Int := { eq := \x y. eqInt x y }

-- _myEq is NOT available to the solver.
-- Must inject explicitly:
result := _myEq => eq 1 2
```

Examples:

```
impl Eq Bool := {
  eq := \x y. case x {
    True  => case y { True => True; False => False };
    False => case y { True => False; False => True }
  }
}

impl Eq a => Eq (Maybe a) := {
  eq := \x y. case x {
    Nothing => case y { Nothing => True; Just _ => False };
    Just a  => case y { Nothing => False; Just b => eq a b }
  }
}

-- Associated type definition in impl
impl Container (List a) := {
  type Elem := a;
  cfold := foldr
}

-- Associated form family definition in impl
impl Collection (List a) := {
  form Key := ListIndex Int;
  lookup := \k xs. case k {
    ListIndex i => index xs i
  }
}
```

### Type Alias

```
type Name param* := TypeExpr
```

Example:

```
type Effect r a := Computation r r a
```

### Type Family (Closed)

Type families are type-level functions declared with `type`, distinguished from type aliases by `::` after the parameters. Equations use `case` with `=>`.

```
TypeFamilyDecl
  = 'type' UpperName '::' ResultKind ':=' '\' TyBinder+ '.' 'case' TyBinder '{' Equation (';' Equation)* '}'

ResultKind
  = KindExpr

Equation
  = TypePattern '=>' TypeCaseBody

TypeCaseBody
  = TypeApp ('->' TypeCaseBody)?
```

The case alternative body (`TypeCaseBody`) permits `->` (function types) but not `=>` (constraint qualification). This avoids ambiguity with the case separator `=>`.

Equations are checked top-to-bottom; first match wins. Reduction is stuck (not skipped) when a match is indeterminate due to unsolved metavariables.

Examples:

```
type Elem :: Type := \(c: Type). case c {
  List a => a;
  String => Rune
}

type NextSeason :: Season := \(s: Season). case s {
  Spring => Summer;
  Summer => Autumn;
  Autumn => Winter;
  Winter => Spring
}

type IsWeekend :: Bool := \(d: Season). case d {
  Summer => True;
  Winter => True;
  _      => False
}

-- Case body with -> (function type result)
type Handler :: Type := \(e: Type). case e {
  IOError     => String -> Result String ();
  ParseError  => Int -> String
}
```

### Builtin Type Families

`Merge :: Row -> Row -> Row` performs disjoint merge of two capability rows. Overlapping labels produce a compile error.

```
type Combined :: Row := Merge { a: Int } { b: Bool }
-- Combined reduces to { a: Int, b: Bool }
```

### DataKinds: Non-Nullary Constructor Promotion

All constructors are promoted to the type level, including those with fields. Non-nullary constructors receive kind arrows:

```
form Maybe := \a. { Nothing: Maybe a; Just: a -> Maybe a }
-- Nothing :: Maybe (kind arity 0)
-- Just :: Type -> Maybe (kind arrow from field type)

type IsJust :: Bool := \(m: Maybe). case m {
  Just _ => True;
  Nothing => False
}
```

### Kind Polymorphism and Cumulativity

Kind-polymorphic binders accept any ground kind (Type, Row, Constraint, promoted kinds) via cumulativity: ground kinds are sub-kinds of Kind.

```
type Id := \(k: Kind) (a: k). a
type T1 := Id Type Int         -- k = Type
type T2 := Id Row { x: Int }   -- k = Row
type T3 := Id Bool True         -- k = Bool (promoted)
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
not := \b. case b { True => False; False => True }
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

Three forms:

```
Import     ::= 'import' ModuleName                         -- open: all names
             | 'import' ModuleName '(' ImportList ')'       -- selective
             | 'import' ModuleName 'as' UpperName           -- qualified

ImportList ::= ImportItem (',' ImportItem)*
ImportItem ::= lower                -- value
             | '(' op ')'           -- operator
             | Upper                -- type/class bare
             | Upper '(' '..' ')'   -- type/class with all subs
             | Upper '(' Upper (',' Upper)* ')'  -- specific subs
```

Examples:

```
import Prelude                          -- open
import Prelude (map, Maybe(..), (+))    -- selective
import Effect.State                     -- dotted module name
import Data.Map as M                    -- qualified: M.lookup, M.insert
```

Import declarations must appear before all other declarations. Duplicate imports of the same module are an error. Two modules cannot share the same alias. Instances are always imported regardless of import form (coherence). `as` is contextual — it is only special immediately after the module name in an import declaration.

Value bindings whose name starts with `_` are module-private and excluded from exports.

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
  Con x y => expr;
  _       => expr
}
```

Case alternatives use `=>` to separate the pattern from the body.

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
  side_effect;           -- bare expression (result discarded)
  pure result             -- final expression
}
```

### Infix Operators

```
x + y               -- operator syntax (if declared)
```

### Type Annotation (in expression)

```
(expr :: Type)
```

### Evidence Injection

```
value => expr
```

Scoped evidence injection: the left operand provides a dictionary or evidence value, which is injected into the local scope for constraint resolution within `expr`. Right-associative, binds below annotation (`::`) at the same level as constraint `=>` in types.

This allows explicitly passing evidence to expressions that require it, bypassing the automatic solver:

```
myEqInstance => eq x y        -- use myEqInstance for Eq resolution in eq x y
```

Private instances (see `impl` declarations) are accessible only through this mechanism.

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

At use sites, the quantified constraint is resolved by finding a matching global instance. For example, `impl Eq a => Eq (F a) := { ... }` satisfies `\a. Eq a => Eq (F a)`.

Within a function body, quantified evidence can be applied to produce dictionaries for specific types. If `f` has constraint `(\a. Eq a => Eq (g a))`, then `eq (x :: g Bool) y` resolves `Eq (g Bool)` by applying the quantified evidence to `Bool` and the `Eq Bool` dictionary.

### Type Equality Constraint

The `~` operator asserts that two types are equal:

```
a ~ Bool => a -> Int
(a ~ Int, b ~ String) => a -> b
```

Type equality constraints can be written explicitly in type signatures. At the definition site, they are installed as given equalities (the skolem is locally equal to the RHS). At the call site, they become wanted constraints that must be discharged. GADT constructor matching also introduces equality constraints: matching `LitBool` on `Expr a` brings `a ~ Bool` into scope.

### Dict Reification

Constraint-kinded type parameters in form declarations enable reification of class evidence as first-class values:

```
form Dict (c: Constraint) := MkDict c
```

The parameter `c` has kind `Constraint`. The constructor field `c` elaborates to an implicit evidence argument — the dictionary for the constraint. At construction, evidence is resolved automatically from the context:

```
mkDict :: Dict (Eq Bool)
mkDict := MkDict           -- resolves Eq Bool evidence implicitly
```

Pattern matching on `Dict` brings the evidence back into scope:

```
withDict :: \a. Dict (Eq a) -> a -> a -> Bool
withDict := \d x y. case d { MkDict => eq x y }
```

The user writes `MkDict` with zero explicit pattern arguments; the evidence field is implicit. Inside the branch body, the constraint `Eq a` is available for resolution.

Constraint-kinded parameters can coexist with regular parameters:

```
form Evidence (c: Constraint) a := MkEvidence c a
```

Here `c` is the implicit evidence field and `a` is a regular field.

### Universal Quantification

```
\a. a -> a
\a b. a -> b -> a
\(r: Row). Computation r r a
\(f: Type -> Type). f a -> f b
```

`\` serves dual purpose: lambda in expression context (`\x. e`) and universal quantification in type context (`\a. T`). Both use `.` as the body separator. The parser disambiguates by context. Multi-parameter lambdas are supported: `\x y z. e` desugars to `\x. \y. \z. e`.

### Row Type

```
{}                              -- empty row (closed)
{ x: Int, y: Bool }            -- closed row
{ x: Int | r }                 -- open row (tail variable)
{ get: () -> Int | r }         -- capability row
{ x: Int @Linear }            -- grade-annotated field
```

Row field grammar:

```
RowField
  = LowerName ':' TypeExpr ['@' GradeExpr]
```

The `@Grade` suffix annotates a field with a grade. Without annotation, fields are unrestricted.

```
{ x: Int @Linear }           -- grade annotation
{ h: Handle @Linear | r }    -- graded field in open row
```

The standard grade algebra uses `Mult` (Zero, Linear, Affine, Unrestricted), defined in Prelude via the `GradeAlgebra` class. User-defined grades are supported by implementing `GradeAlgebra` for a custom promoted kind:

```
form Level := { Public: Level; Secret: Level }
type LevelJoin :: Level -> Level -> Level := \(a: Level) (b: Level). case (a, b) {
  (Secret, _) => Secret;
  (_, Secret) => Secret;
  (x, _)      => x
}
impl GradeAlgebra Level := {
  type GradeJoin := LevelJoin;
  type GradeDrop := Public
}
```

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
Type -> Type          -- higher-kinded (e.g. Maybe: Type -> Type)
Constraint -> Type    -- constraint-parameterized (higher-kinded constraint)
(Row -> Type)         -- parenthesized kind
Bool                  -- DataKinds: promoted form type as kind
DBState               -- DataKinds: user-defined promoted kind
```

`Constraint` can be used in kind annotations for type parameters:

```
\(c: Constraint). Bool                    -- constraint-kinded param
\a (c: Constraint). a -> Bool             -- mixed kinds
form Constrained := \(c: Constraint). { ... }   -- in class declarations
form Dict (c: Constraint) := MkDict c            -- in form declarations (Dict reification)
```

### DataKinds Promotion

When a form type is declared, it is automatically promoted to a kind of the same name. All constructors — both nullary and those with fields — are promoted to type-level constructors of that kind.

```
form DBState := Opened | Closed
-- DBState is now a kind
-- Opened: DBState, Closed: DBState (type-level)

form DB (s: DBState) := MkDB
-- DB Opened: Type, DB Closed: Type

form HList := \(a: Type). HCons a (HList a) | HNil
-- HList is now a kind
-- HCons: Type -> HList a -> HList a (type-level)
-- HNil: HList a (type-level)
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
case n { 0 => "zero"; 1 => "one"; _ => "other" }
case name { "Alice" => "hello"; _ => "hi" }
case ch { 'x' => True; _ => False }
```

Since literal types cannot be exhaustively enumerated, a wildcard or variable catch-all is always required.

### Nested Patterns

Constructor patterns can be nested. Nullary constructors need no parentheses; multi-argument constructors must be parenthesized:

```
case m { Just True => "yes"; Just False => "no"; Nothing => "none" }
case xs { Cons Nothing rest => rest; Cons (Just x) rest => rest; Nil => Nil }
case m { Just (Just (Just True)) => "deep"; _ => "other" }
```

---

## Parser Precedence (Expressions)

| Level | Form              | Associativity    |
| ----- | ----------------- | ---------------- |
| 0     | `:: Type`         | right            |
| 0.5   | `=> expr`         | right            |
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

`lowercase` | `uppercase` | `form` | `type` | `infixl` | `infixr` | `infixn` | `impl` | `import` | `(op)` (operator definition)

Inside braces (`do`, `case`, block expressions, GADT declarations), semicolons are **required** separators between statements, branches, or constructors. Newlines alone do not act as separators within braces.

---

## Built-in Types

| Type                     | Kind                      | Description           |
| ------------------------ | ------------------------- | --------------------- |
| `Computation pre post a` | `Row → Row → Type → Type` | Effectful computation |
| `Thunk pre post a`       | `Row → Row → Type → Type` | Suspended computation |
| `Int`                    | `Type`                    | 64-bit integer        |
| `Double`                 | `Type`                    | 64-bit floating point |
| `String`                 | `Type`                    | Unicode string        |
| `Rune`                   | `Type`                    | Unicode code point    |
| `Slice a`                | `Type → Type`             | Contiguous array      |
| `Map k v`                | `Type → Type → Type`      | Ordered immutable map |
| `Set a`                  | `Type → Type`             | Ordered immutable set |

---

## Prelude

The Prelude is loaded via `eng.Use(gicel.Prelude)` and imported in source with `import Prelude`. Core is auto-registered and auto-imported. Full reference: [agent-guide/prelude.md](agent-guide/prelude.md).

### Data Types and Constructors

```
form Bool := True | False
form Ordering := LT | EQ | GT
form Result e a := Ok a | Err e
form Maybe a := Just a | Nothing
form List a := Cons a (List a) | Nil
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
infixr 5 <+        -- list cons (Prelude)
infixl 5 +>        -- slice snoc (Data.Slice)
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

### Type Classes (17: 1 in Core, 16 in Prelude)

```
IxMonad                           (Core)

Eq ──→ Ord
Eq ──→ Num ──→ Div
Show
Semigroup ──→ Monoid
Functor ──→ Applicative ──→ Alternative
                        ──→ Monad
Functor ─┐
         ├──→ Traversable
Foldable ┘
Packed
FromList ──→ ToList
```

| Class         | Key Methods                                               |
| ------------- | --------------------------------------------------------- |
| `IxMonad`     | `ixpure`, `ixbind` (Core)                                 |
| `Eq`          | `eq: a -> a -> Bool`                                      |
| `Ord`         | `compare: a -> a -> Ordering`                             |
| `Num`         | `add`, `sub`, `mul`, `negate`                             |
| `Div`         | `div: a -> a -> a`                                        |
| `Show`        | `show: a -> String`                                       |
| `Semigroup`   | `append: a -> a -> a`                                     |
| `Monoid`      | `empty: a`                                                |
| `Functor`     | `fmap: (a -> b) -> f a -> f b`                            |
| `Foldable`    | `foldr: (a -> b -> b) -> b -> t a -> b`                   |
| `Applicative` | `wrap: a -> f a`, `ap: f (a -> b) -> f a -> f b`          |
| `Alternative` | `none: f a`, `alt: f a -> f a -> f a`                     |
| `Monad`       | `mpure: a -> m a`, `mbind: m a -> (a -> m b) -> m b`      |
| `Traversable` | `traverse: Applicative f => (a -> f b) -> t a -> f (t b)` |
| `Packed`      | `pack: Slice e -> c`, `unpack: c -> Slice e`              |
| `FromList`    | `fromList: List (Elem l) -> l` (assoc type: `Elem`)       |
| `ToList`      | `toList: l -> List (Elem l)`                              |

---

## Stdlib Packs

Stdlib packs are loaded via `Engine.Use(pack)` on the host side and imported in source. `NewEngine()` returns a bare engine with only Core. Full reference: [agent-guide/stdlib.md](agent-guide/stdlib.md).

| Pack          | Module         | Provides                                                 |
| ------------- | -------------- | -------------------------------------------------------- |
| `Prelude`     | `Prelude`      | Num, Str, List — arithmetic, string ops, list operations |
| `EffectFail`  | `Effect.Fail`  | Fail effect (`failWith`, `fromMaybe`, `fromResult`)      |
| `EffectState` | `Effect.State` | State effect (`get`, `put`, `modify`)                    |
| `EffectIO`    | `Effect.IO`    | IO effect (`print`, `debug`)                             |
| `DataStream`  | `Data.Stream`  | Lazy streams (`Stream a`)                                |
| `DataSlice`   | `Data.Slice`   | Contiguous arrays (`Slice a`), O(1) length/index         |
| `DataMap`     | `Data.Map`     | Ordered immutable map (AVL), requires `Ord k`            |
| `DataSet`     | `Data.Set`     | Ordered immutable set (backed by Map), requires `Ord k`  |
