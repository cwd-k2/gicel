## 6. Prelude Reference

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
readDouble :: String -> Maybe Double
```

Arithmetic via `Num Double` (`+`, `-`, `*`, `negate`) and `Div Double` (`/`). Instances: `Eq`, `Ord`, `Num`, `Div`, `Semigroup`, `Monoid`, `Show`, `Read`.

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
