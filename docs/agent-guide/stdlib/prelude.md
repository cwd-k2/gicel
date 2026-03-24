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

### Type Aliases

```
type Effect := \r a. Computation r r a
type Suspended := \r a. Thunk r r a
type Lift := \(m: Type -> Type) (r1: Row) (r2: Row) a. m a
```

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

**IxMonad**

```
form IxMonad := \(m: Row -> Row -> Type -> Type). {
  ixpure: \a (r: Row). a -> m r r a;
  ixbind: \a b (r1: Row) (r2: Row) (r3: Row).
              m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
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
| `IxMonad`     | `Computation` (built-in), `Maybe`, `List`                                                                       |
| `Monad`       | `Maybe`, `List`                                                                                                 |
| `Eq`          | `Bool`, `()`, `Ordering`, `Maybe a`, `(a,b)`, `List a`, `Result e a`, `Int`, `Double`, `String`, `Rune`, `Byte` |
| `Ord`         | `Bool`, `()`, `Ordering`, `Maybe a`, `(a,b)`, `List a`, `Result e a`, `Int`, `Double`, `String`, `Rune`, `Byte` |
| `Num`         | `Int`, `Double`                                                                                                 |
| `Div`         | `Int`, `Double`                                                                                                 |
| `Semigroup`   | `()`, `Ordering`, `Maybe a`, `List a`, `Int`, `Double`, `String`                                                |
| `Monoid`      | `()`, `Ordering`, `Maybe a`, `List a`, `Int`, `Double`, `String`                                                |
| `Show`        | `Bool`, `()`, `Ordering`, `Int`, `Double`, `Byte`, `String`, `Maybe a`, `List a`, `Result e a`, `(a,b)`         |
| `Functor`     | `Maybe`, `List`, `Result e`                                                                                     |
| `Foldable`    | `Maybe`, `List`, `Result e`                                                                                     |
| `Applicative` | `Maybe`, `List`                                                                                                 |
| `Traversable` | `Maybe`, `List`                                                                                                 |
| `Alternative` | `Maybe`, `List`                                                                                                 |
| `Packed`      | `Packed String Rune`, `Packed String Byte`                                                                      |
| `Read`        | `Int`, `Double`, `String`                                                                                       |
| `FromList`    | `List a`, `Maybe a`, `String`                                                                                   |
| `ToList`      | `List a`, `Maybe a`, `String`                                                                                   |
