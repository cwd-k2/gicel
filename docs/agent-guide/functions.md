### Functions

**Identity and combinators:**

```
id    :: forall a. a -> a
const :: forall a b. a -> b -> a
flip  :: forall a b c. (a -> b -> c) -> b -> a -> c
```

**Composition:**

```
infixr 9 .
(.) :: forall b c a. (b -> c) -> (a -> b) -> a -> c
```

**Boolean logic:**

```
not  :: Bool -> Bool
(&&) :: Bool -> Bool -> Bool   -- infixr 3
(||) :: Bool -> Bool -> Bool   -- infixr 2
```

**Eliminators:**

```
maybe  :: forall a b. b -> (a -> b) -> Maybe a -> b
result :: forall e a b. (e -> b) -> (a -> b) -> Result e a -> b
bool   :: forall a. a -> a -> Bool -> a
```

**Tuple / List basics:**

```
fst :: forall a b. (a, b) -> a          snd :: forall a b. (a, b) -> b
swap :: forall a b. (a, b) -> (b, a)    curry :: forall a b c. ((a, b) -> c) -> a -> b -> c
uncurry :: forall a b c. (a -> b -> c) -> (a, b) -> c

head :: forall a. List a -> Maybe a     tail :: forall a. List a -> Maybe (List a)
null :: forall a. List a -> Bool        singleton :: forall a. a -> List a
map  :: forall a b. (a -> b) -> List a -> List b
filter :: forall a. (a -> Bool) -> List a -> List a
```

**Comparison:**

```
(==) (/=) :: Eq a => a -> a -> Bool       -- infixn 4
(<) (>) (<=) (>=) :: Ord a => a -> a -> Bool  -- infixn 4
min  max :: Ord a => a -> a -> a
on :: forall a b c. (b -> b -> c) -> (a -> b) -> a -> a -> c
comparing :: Ord b => (a -> b) -> a -> a -> Ordering
equating  :: Eq b => (a -> b) -> a -> a -> Bool
```

**Effect sequencing:**

```
then :: Computation r1 r2 a -> Computation r2 r3 b -> Computation r1 r3 b
```

---

### Prelude Utility Functions

| Category | Functions                                                    |
| -------- | ------------------------------------------------------------ |
| Maybe    | `isJust`, `isNothing`                                        |
| Result   | `isOk`, `isErr`, `fromOk`, `fromErr`                         |
| Foldable | `foldMap`, `toList`, `find`, `elem`, `notElem`, `any`, `all` |
| List     | `lookup`, `concatMap`, `flatten`, `catMaybes`, `mapMaybe`    |
| List     | `partition`, `takeWhile`, `intersperse`, `nub`, `and`, `or`  |
| Monadic  | `guard`, `when`, `unless`, `mjoin`, `liftA2`, `void`         |

---

### Operator Quick Reference

All operators sorted by precedence (highest binds tightest):

| Prec | Op    | Assoc | Meaning              | Source  |
| ---- | ----- | ----- | -------------------- | ------- |
| 9    | `.`   | right | function composition | Prelude |
| 7    | `*`   | left  | multiplication       | Std.Num |
| 7    | `/`   | left  | integer division     | Std.Num |
| 6    | `+`   | left  | addition             | Std.Num |
| 6    | `-`   | left  | subtraction          | Std.Num |
| 6    | `<>`  | right | semigroup append     | Prelude |
| 4    | `<$>` | left  | functor map          | Prelude |
| 4    | `<*>` | left  | applicative apply    | Prelude |
| 4    | `*>`  | left  | applicative then     | Prelude |
| 4    | `<*`  | left  | applicative but      | Prelude |
| 4    | `==`  | none  | equality             | Prelude |
| 4    | `/=`  | none  | inequality           | Prelude |
| 4    | `<`   | none  | less than            | Prelude |
| 4    | `>`   | none  | greater than         | Prelude |
| 4    | `<=`  | none  | less or equal        | Prelude |
| 4    | `>=`  | none  | greater or equal     | Prelude |
| 3    | `&&`  | right | boolean and          | Prelude |
| 3    | `<¦>` | left  | alternative choice   | Prelude |
| 2    | `¦¦`  | right | boolean or           | Prelude |
| 1    | `<&>` | left  | flipped fmap         | Prelude |
| 1    | `>>=` | left  | monad bind           | Prelude |
| 1    | `>>`  | left  | monad sequence       | Prelude |
| 1    | `&`   | left  | reverse application  | Prelude |
| 1    | `=<<` | right | flipped bind         | Prelude |
| 1    | `>=>` | right | Kleisli composition  | Prelude |
| 1    | `<=<` | right | flipped Kleisli      | Prelude |
| 0    | `$`   | right | low-precedence apply | Prelude |

Undeclared operators default to `infixl 9`.

Non-associative (`infixn`) operators cannot be chained: `a == b == c` is a parse error.
