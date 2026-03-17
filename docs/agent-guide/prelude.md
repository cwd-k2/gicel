## 6. Prelude Reference

The Prelude is loaded via `eng.Use(gicel.Prelude)` and must be explicitly imported in source with `import Prelude`. It bundles Num, Str, and List alongside the core data types and type classes. Everything below becomes available with `import Prelude`.

### Data Types

```
data Bool := True | False
data Ordering := LT | EQ | GT
data Result e a := Ok a | Err e
data Maybe a := Just a | Nothing
data List a := Cons a (List a) | Nil
```

`()` is the unit type (empty record). `(a, b)` is the tuple type (sugar for `Record { _1: a, _2: b }`).

### Type Aliases

```
type Effect r a := Computation r r a
type Lift (m: Type -> Type) (r1: Row) (r2: Row) a := m a
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

**Num**

```
class Eq a => Num a {
  add    :: a -> a -> a;
  sub    :: a -> a -> a;
  mul    :: a -> a -> a;
  negate :: a -> a
}
```

**Div**

```
class Num a => Div a {
  div :: a -> a -> a
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
  fmap :: \a b. (a -> b) -> f a -> f b
}
```

**Foldable**

```
class Foldable t {
  foldr :: \a b. (a -> b -> b) -> b -> t a -> b
}
```

**Applicative**

```
class Functor f => Applicative f {
  wrap :: \a. a -> f a;
  ap   :: \a b. f (a -> b) -> f a -> f b
}
```

**Traversable**

```
class Functor t => Foldable t => Traversable t {
  traverse :: \f a b. Applicative f => (a -> f b) -> t a -> f (t b)
}
```

**IxMonad**

```
class IxMonad (m: Row -> Row -> Type -> Type) {
  ixpure :: \a (r: Row). a -> m r r a;
  ixbind :: \a b (r1: Row) (r2: Row) (r3: Row).
              m r1 r2 a -> (a -> m r2 r3 b) -> m r1 r3 b
}
```

**Show**

```
class Show a {
  show :: a -> String
}
```

**Alternative**

```
class Applicative f => Alternative f {
  none :: \a. f a;
  alt  :: \a. f a -> f a -> f a
}
```

**Monad**

```
class Monad (m: Type -> Type) {
  mpure :: \a. a -> m a;
  mbind :: \a b. m a -> (a -> m b) -> m b
}
```

**Packed**

```
class Packed c e {
  pack   :: List e -> c;
  unpack :: c -> List e
}
```

### Instances

| Class         | Instances                                                            |
| ------------- | -------------------------------------------------------------------- |
| `IxMonad`     | `Computation` (built-in), `Maybe`, `List`                            |
| `Monad`       | `Maybe`, `List`                                                      |
| `Eq`          | `Bool`, `()`, `Ordering`, `Maybe a`, `(a,b)`, `List a`, `Result e a` |
| `Ord`         | `Bool`, `()`, `Ordering`, `Maybe a`, `(a,b)`, `List a`, `Result e a` |
| `Num`         | `Int`, `Double`                                                      |
| `Div`         | `Int`, `Double`                                                      |
| `Semigroup`   | `()`, `Ordering`, `Maybe a`, `List a`, `Int`, `Double`               |
| `Monoid`      | `()`, `Ordering`, `Maybe a`, `List a`, `Int`, `Double`               |
| `Show`        | `Bool`, `()`, `Ordering`                                             |
| `Functor`     | `Maybe`, `List`, `Result e`                                          |
| `Foldable`    | `Maybe`, `List`, `Result e`                                          |
| `Applicative` | `Maybe`, `List`                                                      |
| `Traversable` | `Maybe`, `List`                                                      |
| `Alternative` | `Maybe`, `List`                                                      |
| `Packed`      | `Packed (List a) a` (identity)                                       |

`Show Int`, additional `Show` instances (`String`, `Maybe a`, `List a`, `Result e a`, `(a,b)`), and `Packed String Rune` are all provided by the Prelude.
