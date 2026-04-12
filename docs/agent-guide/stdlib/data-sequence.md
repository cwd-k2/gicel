### Data.Sequence

Double-ended sequence backed by a 2-3 finger tree.

CLI: `--packs seq`

```gicel
import Prelude
import Data.Sequence
```

## Complexity

| Operation                     | Time             |
| ----------------------------- | ---------------- |
| `cons`, `snoc`                | O(1) amortized   |
| `uncons`, `unsnoc`            | O(1) amortized   |
| `append`                      | O(log(min(m,n))) |
| `index`                       | O(log n)         |
| `length`                      | O(1)             |
| `toList`, `fromList`, `foldl` | O(n)             |

## Construction

| Function    | Type                      | Description               |
| ----------- | ------------------------- | ------------------------- |
| `empty`     | `Seq a`                   | Empty sequence            |
| `singleton` | `a -> Seq a`              | Single-element sequence   |
| `cons`      | `a -> Seq a -> Seq a`     | Prepend element           |
| `snoc`      | `Seq a -> a -> Seq a`     | Append element            |
| `append`    | `Seq a -> Seq a -> Seq a` | Concatenate two sequences |

## Deconstruction

| Function | Type                        | Description                 |
| -------- | --------------------------- | --------------------------- |
| `uncons` | `Seq a -> Maybe (a, Seq a)` | Split off the first element |
| `unsnoc` | `Seq a -> Maybe (Seq a, a)` | Split off the last element  |
| `head`   | `Seq a -> Maybe a`          | First element               |
| `last`   | `Seq a -> Maybe a`          | Last element                |

## Query

| Function | Type                      | Description              |
| -------- | ------------------------- | ------------------------ |
| `length` | `Seq a -> Int`            | Number of elements       |
| `null`   | `Seq a -> Bool`           | True if empty            |
| `index`  | `Int -> Seq a -> Maybe a` | Element at 0-based index |

## Fold and Transform

| Function  | Type                               | Description                        |
| --------- | ---------------------------------- | ---------------------------------- |
| `foldl`   | `(b -> a -> b) -> b -> Seq a -> b` | Left fold                          |
| `foldr`   | `(a -> b -> b) -> b -> Seq a -> b` | Right fold                         |
| `map`     | `(a -> b) -> Seq a -> Seq b`       | Apply function to each element     |
| `filter`  | `(a -> Bool) -> Seq a -> Seq a`    | Keep elements satisfying predicate |
| `reverse` | `Seq a -> Seq a`                   | Reverse element order              |

## Instances

- `FromList (Seq a)` — `fromList :: List a -> Seq a`
- `ToList (Seq a)` — `toList :: Seq a -> List a`
- `Show a => Show (Seq a)` — displays as `Seq.fromList [...]`
