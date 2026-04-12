## Prelude Reference

The Prelude is loaded via `eng.Use(gicel.Prelude)` and must be explicitly imported in source with `import Prelude`. It bundles Num, Str, and List alongside the core form types and type classes. Everything below becomes available with `import Prelude`.

### Data Types

```
form Bool := True | False
form Ordering := LT | EQ | GT
form Result := \e a. { Ok: a -> Result e a; Err: e -> Result e a }
form Maybe := \a. { Just: a -> Maybe a; Nothing: Maybe a }
form List := \a. { Cons: a -> List a -> List a; Nil: List a }
```

`()` is the unit type (empty record). `(a, b)` is the tuple type (sugar for `Record { _1: a, _2: b }`).

### Primitive Types

**Int** — 64-bit signed integer (Go `int64`). Literals: `42`, `100_000`. Negative values: `-5` (desugars to `negate 5`). Overflow wraps silently (two's-complement). Instances: `Eq`, `Ord`, `Num`, `Div`, `Semigroup`, `Monoid`, `Show`, `Read`.

**Double** — IEEE 754 64-bit floating point. Literals: `3.14`, `1.0e10`, `2.5e-3`. Operations:

```
toDouble  :: Int -> Double
round     :: Double -> Int
floor     :: Double -> Int
ceiling   :: Double -> Int
truncate  :: Double -> Int
```

Arithmetic via `Num Double` (`+`, `-`, `*`, `negate`) and `Div Double` (`/`). Instances: `Eq`, `Ord`, `Num`, `Div`, `Semigroup`, `Monoid`, `Show`, `Read`. Use `read :: String -> Maybe Double` (from the `Read` instance) to parse strings.

**Byte** — 8-bit unsigned integer (0--255). Conversions:

```
byteToInt :: Byte -> Int
intToByte :: Int -> Maybe Byte
```

Instances: `Eq`, `Ord`, `Show`.

**Rune** — Unicode code point. Literals: `'a'`, `'\n'`, `'\0'`. Escape sequences: `\n`, `\t`, `\r`, `\\`, `\'`, `\"`, `\0`. Conversions:

```
toRunes  :: String -> List Rune
fromRunes :: List Rune -> String
charAt   :: Int -> String -> Maybe Rune
```

Classification functions: `isAlpha`, `isDigit`, `isAlphaNum`, `isSpace`, `isUpper`, `isLower`. Conversions: `runeToInt`, `intToRune`, `digitToInt`. Instances: `Eq`, `Ord`, `Show`. String can be packed/unpacked as `Slice Rune` or `Slice Byte` via the `Packed` class.

### Type Aliases

```
type Effect := \r a. Computation Zero r r a
type Suspended := \r a. Thunk Zero r r a
type Lift := \(m: Type -> Type) (g: Kind) (r1: Row) (r2: Row) a. m a
```

`Effect` and `Suspended` fix the grade to `Zero` (the trivial grade). `Lift` wraps a plain `Type -> Type` monad into the graded indexed monad shape expected by `GIMonad`.

### Core Type Classes

These are defined in Core (auto-imported) and form the foundation of the effect system.

**GradeAlgebra** — type-level grade operations for graded indexed monads:

```
form GradeAlgebra := \(g: Kind). {
  type GradeJoin :: g -> g -> g;
  type GradeCompose :: g -> g -> g;
  type GradeDrop :: g;
  type GradeUnit :: g
}
```

`GradeCompose` combines grades when sequencing computations via `bind`. `GradeDrop` is the identity grade for `pure`. `GradeJoin` merges grades at branch joins. `GradeUnit` is the multiplicative identity.

**Trivial** — the single-element grade algebra (carries no information):

```
form Trivial := { Triv: Trivial }

impl GradeAlgebra Trivial := {
  type GradeJoin := \(a: Trivial) (b: Trivial). Triv;
  type GradeCompose := \(a: Trivial) (b: Trivial). Triv;
  type GradeDrop := Triv;
  type GradeUnit := Triv
}
```

Used implicitly when grade information is irrelevant (the `Zero` in `Effect r a := Computation Zero r r a` refers to `GradeDrop` of the inferred grade algebra).

**GIMonad** — the canonical graded indexed monad class:

```
form GIMonad := \(g: Kind) (m: g -> Row -> Row -> Type -> Type). GradeAlgebra g => {
  gipure: \a (r: Row). a -> m GradeDrop r r a;
  gibind: \a b (e1: g) (e2: g) (r1: Row) (r2: Row) (r3: Row).
              m e1 r1 r2 a -> (a -> m e2 r2 r3 b) -> m (GradeCompose e1 e2) r1 r3 b
}
```

`Computation` is the built-in `GIMonad` instance. `Lift m` wraps ordinary monads (`Maybe`, `List`, `Result e`) into the indexed shape.

**UsageSemiring** — value-level grade arithmetic (parallel to type-level `GradeAlgebra`):

```
form UsageSemiring := \(s: Type). {
  zero: s;
  one: s;
  plus: s -> s -> s;
  mult: s -> s -> s
}
```

Laws (not enforced): `(s, zero, plus)` is a commutative monoid, `(s, one, mult)` is a monoid, `mult` distributes over `plus`, `mult x zero = zero`.

### Type Classes

**Eq**

```
form Eq := \a. {
  eq: a -> a -> Bool
}
```

**Ord**

```
form Ord := \a. Eq a => {
  compare: a -> a -> Ordering
}
```

**Num**

```
form Num := \a. Eq a => {
  add:    a -> a -> a;
  sub:    a -> a -> a;
  mul:    a -> a -> a;
  negate: a -> a
}
```

**Div**

```
form Div := \a. Num a => {
  div: a -> a -> a
}
```

**Semigroup**

```
form Semigroup := \a. {
  append: a -> a -> a
}
```

**Monoid**

```
form Monoid := \a. Semigroup a => {
  empty: a
}
```

**Functor**

```
form Functor := \f. {
  fmap: \a b. (a -> b) -> f a -> f b
}
```

**Foldable**

```
form Foldable := \t. {
  foldr: \a b. (a -> b -> b) -> b -> t a -> b
}
```

**Applicative**

```
form Applicative := \f. Functor f => {
  wrap: \a. a -> f a;
  ap:   \a b. f (a -> b) -> f a -> f b
}
```

**Traversable**

```
form Traversable := \t. (Functor t, Foldable t) => {
  traverse: \f a b. Applicative f => (a -> f b) -> t a -> f (t b)
}
```

**Show**

```
form Show := \a. {
  show: a -> String
}
```

**Alternative**

```
form Alternative := \f. Applicative f => {
  none: \a. f a;
  alt:  \a. f a -> f a -> f a
}
```

**Monad**

```
form Monad := \(m: Type -> Type). {
  mpure: \a. a -> m a;
  mbind: \a b. m a -> (a -> m b) -> m b
}
```

**Packed**

```
form Packed := \c e. {
  pack:   Slice e -> c;
  unpack: c -> Slice e
}
```

**Read**

```
form Read := \a. {
  read: String -> Maybe a
}
```

**Bifunctor**

```
form Bifunctor := \(p: Type -> Type -> Type). {
  bimap: \a b c d. (a -> c) -> (b -> d) -> p a b -> p c d
}
```

Derived: `bfirst :: (a -> c) -> p a b -> p c b`, `bsecond :: (b -> d) -> p a b -> p a d`.
Tuple variants: `bimapPair`, `firstPair`, `secondPair`.

**FromList**

```
form FromList := \l. {
  type Elem l :: Type;
  fromList: List (Elem l) -> l
}
```

**ToList**

```
form ToList := \l. FromList l => {
  toList: l -> List (Elem l)
}
```

### Instances

| Class         | Instances                                                                                                       |
| ------------- | --------------------------------------------------------------------------------------------------------------- |
| `GIMonad`     | `Computation` (built-in), `Lift Maybe`, `Lift List`, `Lift (Result e)`                                          |
| `Monad`       | `Maybe`, `List`, `Result e`                                                                                     |
| `Eq`          | `Bool`, `()`, `Ordering`, `Maybe a`, `(a,b)`, `List a`, `Result e a`, `Int`, `Double`, `String`, `Rune`, `Byte` |
| `Ord`         | `Bool`, `()`, `Ordering`, `Maybe a`, `(a,b)`, `List a`, `Result e a`, `Int`, `Double`, `String`, `Rune`, `Byte` |
| `Num`         | `Int`, `Double`                                                                                                 |
| `Div`         | `Int`, `Double`                                                                                                 |
| `Semigroup`   | `()`, `Ordering`, `Maybe a`, `List a`, `Int`, `Double`, `String`                                                |
| `Monoid`      | `()`, `Ordering`, `Maybe a`, `List a`, `Int`, `Double`, `String`                                                |
| `Show`        | `Bool`, `()`, `Ordering`, `Int`, `Double`, `Byte`, `Rune`, `String`, `Maybe a`, `List a`, `Result e a`, `(a,b)` |
| `Functor`     | `Maybe`, `List`, `Result e`                                                                                     |
| `Foldable`    | `Maybe`, `List`, `Result e`                                                                                     |
| `Applicative` | `Maybe`, `List`                                                                                                 |
| `Traversable` | `Maybe`, `List`                                                                                                 |
| `Alternative` | `Maybe`, `List`                                                                                                 |
| `Packed`      | `Packed String Rune`, `Packed String Byte`                                                                      |
| `Read`        | `Int`, `Double`, `String`                                                                                       |
| `Bifunctor`   | `Result`                                                                                                        |
| `FromList`    | `List a`, `Maybe a`, `String`                                                                                   |
| `ToList`      | `List a`, `Maybe a`, `String`                                                                                   |

### Variant Construction

| Name     | Type                                                                                       | Description                                    |
| -------- | ------------------------------------------------------------------------------------------ | ---------------------------------------------- |
| `inject` | `\(tag: Label) (choices: Row). Lookup tag choices -> Variant choices (Lookup tag choices)` | Construct a `Variant` value at the given label |

`Variant :: Row -> Type -> Type` is the labeled coproduct dual of `Record`. It represents a value tagged with one label from a row:

```
v := inject @#ping 42   -- Variant { ping: Int } Int
```

Use with session types (`receiveAt`) and external choice dispatch:

```
tag <- receiveAt @#ch;
case tag {
  #ping x => handle x;
  #quit   => pure ()
}
```

### Function Utilities

| Name      | Type                                               | Description              |
| --------- | -------------------------------------------------- | ------------------------ |
| `id`      | `\a. a -> a`                                       | Identity function        |
| `const`   | `\a b. a -> b -> a`                                | Constant function        |
| `flip`    | `\a b c. (a -> b -> c) -> b -> a -> c`             | Flip first two arguments |
| `on`      | `\a b c. (b -> b -> c) -> (a -> b) -> a -> a -> c` | Apply then combine       |
| `curry`   | `\a b c. ((a, b) -> c) -> a -> b -> c`             | Curry a pair function    |
| `uncurry` | `\a b c. (a -> b -> c) -> (a, b) -> c`             | Uncurry to pair function |

Operators: `(.)` (composition, infixr 9), `($)` (apply, infixr 0), `(&)` (reverse apply, infixl 1).

### Boolean

| Name   | Type                      | Description             |
| ------ | ------------------------- | ----------------------- |
| `not`  | `Bool -> Bool`            | Logical negation        |
| `bool` | `\a. a -> a -> Bool -> a` | Church-style eliminator |

Operators: `(&&)` (infixr 3), `(||)` (infixr 2).

### Numeric

| Name        | Type                                            | Description             |
| ----------- | ----------------------------------------------- | ----------------------- |
| `mod`       | `Int -> Int -> Int`                             | Integer modulus         |
| `even`      | `Int -> Bool`                                   | Test if even            |
| `odd`       | `Int -> Bool`                                   | Test if odd             |
| `gcd`       | `Int -> Int -> Int`                             | Greatest common divisor |
| `lcm`       | `Int -> Int -> Int`                             | Least common multiple   |
| `min`       | `\a. Ord a => a -> a -> a`                      | Minimum of two values   |
| `max`       | `\a. Ord a => a -> a -> a`                      | Maximum of two values   |
| `clamp`     | `\a. Ord a => (a, a) -> a -> a`                 | Clamp to (lo, hi) range |
| `comparing` | `\a b. Ord b => (a -> b) -> a -> a -> Ordering` | Compare by projection   |
| `equating`  | `\a b. Eq b => (a -> b) -> a -> a -> Bool`      | Equality by projection  |

### Tuple

| Name         | Type                                                 | Description        |
| ------------ | ---------------------------------------------------- | ------------------ |
| `fst`        | `\a (r: Row). Record { _1: a \| r } -> a`            | First element      |
| `snd`        | `\a (r: Row). Record { _2: a \| r } -> a`            | Second element     |
| `swap`       | `\a b. (a, b) -> (b, a)`                             | Swap pair elements |
| `bimapPair`  | `\a b c d. (a -> c) -> (b -> d) -> (a, b) -> (c, d)` | Map both elements  |
| `firstPair`  | `\a b c. (a -> c) -> (a, b) -> (c, b)`               | Map first element  |
| `secondPair` | `\a b d. (b -> d) -> (a, b) -> (a, d)`               | Map second element |

### Maybe

| Name          | Type                                  | Description             |
| ------------- | ------------------------------------- | ----------------------- |
| `maybe`       | `\a b. b -> (a -> b) -> Maybe a -> b` | Eliminator with default |
| `isJust`      | `\a. Maybe a -> Bool`                 | True if `Just`          |
| `isNothing`   | `\a. Maybe a -> Bool`                 | True if `Nothing`       |
| `listToMaybe` | `\a. List a -> Maybe a`               | Head or `Nothing`       |
| `maybeToList` | `\a. Maybe a -> List a`               | Singleton or empty      |

### Result

| Name               | Type                                                 | Description                   |
| ------------------ | ---------------------------------------------------- | ----------------------------- |
| `result`           | `\e a b. (e -> b) -> (a -> b) -> Result e a -> b`    | Eliminator                    |
| `isOk`             | `\e a. Result e a -> Bool`                           | True if `Ok`                  |
| `isErr`            | `\e a. Result e a -> Bool`                           | True if `Err`                 |
| `fromOk`           | `\e a. a -> Result e a -> a`                         | Extract `Ok` with default     |
| `fromErr`          | `\e a. e -> Result e a -> e`                         | Extract `Err` with default    |
| `mapErr`           | `\e1 e2 a. (e1 -> e2) -> Result e1 a -> Result e2 a` | Map over error side           |
| `partitionResults` | `\e a. List (Result e a) -> (List e, List a)`        | Separate errors and successes |
| `rights`           | `\e a. List (Result e a) -> List a`                  | Collect successes             |
| `lefts`            | `\e a. List (Result e a) -> List e`                  | Collect errors                |

### List

| Name            | Type                                                                     | Description                           |
| --------------- | ------------------------------------------------------------------------ | ------------------------------------- |
| `head`          | `\a. List a -> Maybe a`                                                  | First element                         |
| `tail`          | `\a. List a -> Maybe (List a)`                                           | All but first                         |
| `uncons`        | `\a. List a -> Maybe (a, List a)`                                        | Split head and tail                   |
| `null`          | `\a. List a -> Bool`                                                     | True if empty                         |
| `singleton`     | `\a. a -> List a`                                                        | Wrap in single-element list           |
| `map`           | `\a b. (a -> b) -> List a -> List b`                                     | Map a function over elements          |
| `filter`        | `\a. (a -> Bool) -> List a -> List a`                                    | Keep elements satisfying predicate    |
| `find`          | `\a. (a -> Bool) -> List a -> Maybe a`                                   | First element satisfying predicate    |
| `elem`          | `\a. Eq a => a -> List a -> Bool`                                        | Membership test                       |
| `notElem`       | `\a. Eq a => a -> List a -> Bool`                                        | Non-membership test                   |
| `any`           | `\a. (a -> Bool) -> List a -> Bool`                                      | True if any element satisfies         |
| `all`           | `\a. (a -> Bool) -> List a -> Bool`                                      | True if all elements satisfy          |
| `and`           | `List Bool -> Bool`                                                      | Conjunction of all elements           |
| `or`            | `List Bool -> Bool`                                                      | Disjunction of all elements           |
| `lookup`        | `\a b. Eq a => a -> List (a, b) -> Maybe b`                              | Association list lookup               |
| `concatMap`     | `\a b. (a -> List b) -> List a -> List b`                                | Map then flatten                      |
| `flatten`       | `\a. List (List a) -> List a`                                            | Flatten nested lists                  |
| `catMaybes`     | `\a. List (Maybe a) -> List a`                                           | Collect `Just` values                 |
| `mapMaybe`      | `\a b. (a -> Maybe b) -> List a -> List b`                               | Filter-map                            |
| `partition`     | `\a. (a -> Bool) -> List a -> (List a, List a)`                          | Split by predicate                    |
| `takeWhile`     | `\a. (a -> Bool) -> List a -> List a`                                    | Take prefix while predicate holds     |
| `intersperse`   | `\a. a -> List a -> List a`                                              | Insert separator between elements     |
| `nub`           | `\a. Eq a => List a -> List a`                                           | Remove duplicates                     |
| `minimum`       | `\a. Ord a => List a -> Maybe a`                                         | Smallest element                      |
| `maximum`       | `\a. Ord a => List a -> Maybe a`                                         | Largest element                       |
| `minimumBy`     | `\a. (a -> a -> Ordering) -> List a -> Maybe a`                          | Smallest by comparator                |
| `maximumBy`     | `\a. (a -> a -> Ordering) -> List a -> Maybe a`                          | Largest by comparator                 |
| `sum`           | `List Int -> Int`                                                        | Sum of integers                       |
| `product`       | `List Int -> Int`                                                        | Product of integers                   |
| `sumDouble`     | `List Double -> Double`                                                  | Sum of doubles                        |
| `productDouble` | `List Double -> Double`                                                  | Product of doubles                    |
| `groupBy`       | `\a. (a -> a -> Bool) -> List a -> List (List a)`                        | Group consecutive equal elements      |
| `group`         | `\a. Eq a => List a -> List (List a)`                                    | Group consecutive equal elements (Eq) |
| `isPrefixOf`    | `\a. Eq a => List a -> List a -> Bool`                                   | Prefix check                          |
| `isSuffixOf`    | `\a. Eq a => List a -> List a -> Bool`                                   | Suffix check                          |
| `foldMap`       | `\(t: Type -> Type) a m. (Foldable t, Monoid m) => (a -> m) -> t a -> m` | Map then fold with Monoid             |
| `collectList`   | `\(t: Type -> Type) a. Foldable t => t a -> List a`                      | Convert any Foldable to List          |

Operators: `(<+)` (cons, infixr 5).

### String

| Name           | Type                                   | Description                       |
| -------------- | -------------------------------------- | --------------------------------- |
| `strlen`       | `String -> Int`                        | String length (UTF-8 byte count)  |
| `charAt`       | `Int -> String -> Maybe Rune`          | Character at byte index           |
| `substring`    | `Int -> Int -> String -> String`       | Byte-range substring              |
| `toUpper`      | `String -> String`                     | Convert to uppercase              |
| `toLower`      | `String -> String`                     | Convert to lowercase              |
| `trim`         | `String -> String`                     | Strip leading/trailing whitespace |
| `contains`     | `String -> String -> Bool`             | Substring containment test        |
| `split`        | `String -> String -> List String`      | Split by separator                |
| `join`         | `String -> List String -> String`      | Join with separator               |
| `words`        | `String -> List String`                | Split on whitespace               |
| `lines`        | `String -> List String`                | Split on newlines                 |
| `unlines`      | `List String -> String`                | Join with newlines                |
| `unwords`      | `List String -> String`                | Join with spaces                  |
| `startsWith`   | `String -> String -> Bool`             | Prefix test                       |
| `endsWith`     | `String -> String -> Bool`             | Suffix test                       |
| `indexOf`      | `String -> String -> Maybe Int`        | First occurrence index            |
| `lastIndexOf`  | `String -> String -> Maybe Int`        | Last occurrence index             |
| `count`        | `String -> String -> Int`              | Count occurrences                 |
| `replace`      | `String -> String -> String -> String` | Replace all occurrences           |
| `reverseStr`   | `String -> String`                     | Reverse string                    |
| `replicateStr` | `Int -> String -> String`              | Repeat n times                    |
| `stripPrefix`  | `String -> String -> Maybe String`     | Remove prefix if present          |
| `stripSuffix`  | `String -> String -> Maybe String`     | Remove suffix if present          |
| `toRunes`      | `String -> List Rune`                  | Decode to rune list               |
| `fromRunes`    | `List Rune -> String`                  | Encode from rune list             |

### Applicative / Functor Utilities

| Name     | Type                                                                                          | Description           |
| -------- | --------------------------------------------------------------------------------------------- | --------------------- |
| `liftA2` | `\(f: Type -> Type) a b c. Applicative f => (a -> b -> c) -> f a -> f b -> f c`               | Lift binary function  |
| `liftA3` | `\(f: Type -> Type) a b c d. Applicative f => (a -> b -> c -> d) -> f a -> f b -> f c -> f d` | Lift ternary function |
| `guard`  | `\(f: Type -> Type). Alternative f => Bool -> f ()`                                           | Fail if false         |
| `void`   | `\(f: Type -> Type) a. Functor f => f a -> f ()`                                              | Discard result        |
| `thenA`  | `\(f: Type -> Type) a b. Applicative f => f a -> f b -> f b`                                  | Sequence, keep second |

Operators: `(<$>)` (fmap, infixl 4), `(<&>)` (flipped fmap, infixl 1), `(<*>)` (ap, infixl 4), `(*>)` (sequence, infixl 4), `(<*)` (discard, infixl 4), `(<|>)` (alt, infixl 3).

### Monad Utilities

| Name     | Type                                                  | Description                   |
| -------- | ----------------------------------------------------- | ----------------------------- |
| `mjoin`  | `\(m: Type -> Type) a. Monad m => m (m a) -> m a`     | Flatten nested monad          |
| `when`   | `\(m: Type -> Type). Monad m => Bool -> m () -> m ()` | Conditional execution         |
| `unless` | `\(m: Type -> Type). Monad m => Bool -> m () -> m ()` | Negated conditional execution |

Operators: `(>>=)` (bind, infixl 1), `(>>)` (sequence, infixl 1), `(=<<)` (flipped bind, infixr 1), `(>=>)` (Kleisli L→R, infixr 1), `(<=<)` (Kleisli R→L, infixr 1).

### Effectful List Combinators

These operate on `List` within an `Effect r` computation:

| Name         | Type                                                                 | Description                       |
| ------------ | -------------------------------------------------------------------- | --------------------------------- |
| `mapM`       | `\a b (r: Row). (a -> Effect r b) -> List a -> Effect r (List b)`    | Map with effect                   |
| `mapM_`      | `\a b (r: Row). (a -> Effect r b) -> List a -> Effect r ()`          | Map with effect, discard results  |
| `forM`       | `\a b (r: Row). List a -> (a -> Effect r b) -> Effect r (List b)`    | Flipped mapM                      |
| `forM_`      | `\a b (r: Row). List a -> (a -> Effect r b) -> Effect r ()`          | Flipped mapM\_                    |
| `sequence`   | `\a (r: Row). List (Suspended r a) -> Effect r (List a)`             | Execute list of suspended effects |
| `sequence_`  | `\a (r: Row). List (Suspended r a) -> Effect r ()`                   | Execute, discard results          |
| `foldM`      | `\a b (r: Row). (b -> a -> Effect r b) -> b -> List a -> Effect r b` | Effectful left fold               |
| `filterM`    | `\a (r: Row). (a -> Effect r Bool) -> List a -> Effect r (List a)`   | Filter with effect                |
| `replicateM` | `\a (r: Row). Int -> Suspended r a -> Effect r (List a)`             | Execute n times, collect results  |

### Operator Quick Reference

All operators sorted by precedence (highest binds tightest):

| Prec | Op     | Assoc | Meaning              | Source      |
| ---- | ------ | ----- | -------------------- | ----------- |
| 9    | `.`    | right | function composition | Prelude     |
| 7    | `*`    | left  | multiplication       | Prelude     |
| 7    | `/`    | left  | division (Div)       | Prelude     |
| 6    | `+`    | left  | addition             | Prelude     |
| 6    | `-`    | left  | subtraction          | Prelude     |
| 6    | `<>`   | right | semigroup append     | Prelude     |
| 5    | `<+`   | right | list cons            | Prelude     |
| 5    | `+>`   | right | stream cons          | Data.Stream |
| 4    | `<$>`  | left  | functor map          | Prelude     |
| 4    | `<*>`  | left  | applicative apply    | Prelude     |
| 4    | `*>`   | left  | applicative then     | Prelude     |
| 4    | `<*`   | left  | applicative but      | Prelude     |
| 4    | `==`   | none  | equality             | Prelude     |
| 4    | `/=`   | none  | inequality           | Prelude     |
| 4    | `<`    | none  | less than            | Prelude     |
| 4    | `>`    | none  | greater than         | Prelude     |
| 4    | `<=`   | none  | less or equal        | Prelude     |
| 4    | `>=`   | none  | greater or equal     | Prelude     |
| 3    | `&&`   | right | boolean and          | Prelude     |
| 3    | `***`  | right | parallel composition | Core        |
| 3    | `<\|>` | left  | alternative choice   | Prelude     |
| 2    | `\|\|` | right | boolean or           | Prelude     |
| 1    | `<&>`  | left  | flipped fmap         | Prelude     |
| 1    | `>>=`  | left  | monad bind           | Prelude     |
| 1    | `>>`   | left  | monad sequence       | Prelude     |
| 1    | `&`    | left  | reverse application  | Prelude     |
| 1    | `=<<`  | right | flipped bind         | Prelude     |
| 1    | `>=>`  | right | Kleisli composition  | Prelude     |
| 1    | `<=<`  | right | flipped Kleisli      | Prelude     |
| 0    | `$`    | right | low-precedence apply | Prelude     |

Undeclared operators default to `infixl 9`.

Non-associative (`infixn`) operators cannot be chained: `a == b == c` is a parse error.
