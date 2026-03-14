## 6. Prelude Reference

The Prelude is automatically loaded unless `NoPrelude` is set on the Engine. Everything below is available without any `import`.

### Data Types

```
data Bool = True | False
data Ordering = LT | EQ | GT
data Result e a = Ok a | Err e
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
```

`()` is the unit type (empty record). `(a, b)` is the tuple type (sugar for `Record { _1 : a, _2 : b }`).

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

| Instance              | Notes                                    |
| --------------------- | ---------------------------------------- |
| `IxMonad Computation` | Uses built-in `pure`/`bind`              |
| `IxMonad Maybe`       | `Nothing` propagates; `Just a` applies f |
| `IxMonad List`        | Concatmap (list monad)                   |

**Eq instances:**

| Instance                    |
| --------------------------- |
| `Eq Bool`                   |
| `Eq ()`                     |
| `Eq Ordering`               |
| `Eq a => Eq (Maybe a)`      |
| `Eq a => Eq b => Eq (a, b)` |
| `Eq a => Eq (List a)`       |

**Ord instances:**

| Instance                       |
| ------------------------------ |
| `Ord Bool`                     |
| `Ord ()`                       |
| `Ord Ordering`                 |
| `Ord a => Ord (Maybe a)`       |
| `Ord a => Ord b => Ord (a, b)` |

**Semigroup instances:**

| Instance                             |
| ------------------------------------ |
| `Semigroup ()`                       |
| `Semigroup Ordering`                 |
| `Semigroup a => Semigroup (Maybe a)` |
| `Semigroup (List a)`                 |

**Monoid instances:**

| Instance                          |
| --------------------------------- |
| `Monoid ()`                       |
| `Monoid Ordering`                 |
| `Semigroup a => Monoid (Maybe a)` |
| `Monoid (List a)`                 |

**Functor instances:**

| Instance        |
| --------------- |
| `Functor Maybe` |
| `Functor List`  |

**Foldable instances:**

| Instance         |
| ---------------- |
| `Foldable Maybe` |
| `Foldable List`  |

**Applicative instances:**

| Instance            |
| ------------------- |
| `Applicative Maybe` |

**Traversable instances:**

| Instance            |
| ------------------- |
| `Traversable Maybe` |
