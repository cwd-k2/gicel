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

**Tuple:**

```
fst :: forall a b. (a, b) -> a
fst := \p -> p!#_1

snd :: forall a b. (a, b) -> b
snd := \p -> p!#_2
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

## 8. Operator Quick Reference

All operators sorted by precedence (highest binds tightest):

| Precedence | Operator | Associativity   | Type                                           | Source  |
| ---------- | -------- | --------------- | ---------------------------------------------- | ------- |
| 9          | `.`      | right           | `forall b c a. (b -> c) -> (a -> b) -> a -> c` | Prelude |
| 7          | `*`      | left            | `forall a. Num a => a -> a -> a`               | Std.Num |
| 7          | `/`      | left            | `Int -> Int -> Int`                            | Std.Num |
| 6          | `+`      | left            | `forall a. Num a => a -> a -> a`               | Std.Num |
| 6          | `-`      | left            | `forall a. Num a => a -> a -> a`               | Std.Num |
| 4          | `==`     | non-associative | `forall a. Eq a => a -> a -> Bool`             | Prelude |
| 4          | `/=`     | non-associative | `forall a. Eq a => a -> a -> Bool`             | Prelude |
| 4          | `<`      | non-associative | `forall a. Ord a => a -> a -> Bool`            | Prelude |
| 4          | `>`      | non-associative | `forall a. Ord a => a -> a -> Bool`            | Prelude |
| 4          | `<=`     | non-associative | `forall a. Ord a => a -> a -> Bool`            | Prelude |
| 4          | `>=`     | non-associative | `forall a. Ord a => a -> a -> Bool`            | Prelude |
| 3          | `&&`     | right           | `Bool -> Bool -> Bool`                         | Prelude |
| 2          | `\|\|`   | right           | `Bool -> Bool -> Bool`                         | Prelude |

Undeclared operators default to `infixl 9`.

Non-associative (`infixn`) operators cannot be chained: `a == b == c` is a parse error. Write `(a == b) && (b == c)`.
