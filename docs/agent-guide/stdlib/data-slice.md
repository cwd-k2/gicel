### Data.Slice

Provides contiguous array with O(1) length/index. Load with `eng.Use(gicel.DataSlice)` and import with `import Data.Slice`.

| Name        | Type                                       | Description     |
| ----------- | ------------------------------------------ | --------------- |
| `empty`     | `\a. Slice a`                              | Empty slice     |
| `singleton` | `\a. a -> Slice a`                         | Single-element  |
| `length`    | `\a. Slice a -> Int`                       | O(1) length     |
| `index`     | `\a. Int -> Slice a -> Maybe a`            | O(1) index      |
| `toList`    | `\a. Slice a -> List a`                    | Convert to list |
| `fromList`  | `\a. List a -> Slice a`                    | Build from list |
| `foldr`     | `\a b. (a -> b -> b) -> b -> Slice a -> b` | Right fold      |
| `foldl`     | `\a b. (b -> a -> b) -> b -> Slice a -> b` | Left fold       |
| `fmap`      | `\a b. (a -> b) -> Slice a -> Slice b`     | Map over slice  |

| `filter` | `\a. (a -> Bool) -> Slice a -> Slice a` | Keep matching |
| `reverse` | `\a. Slice a -> Slice a` | Reverse order |
| `zipWith` | `\a b c. (a -> b -> c) -> Slice a -> Slice b -> Slice c` | Zip with function |
| `concat` | `\a. Slice (Slice a) -> Slice a` | Flatten nested |
| `replicate` | `\a. Int -> a -> Slice a` | N copies of elem |
| `generate` | `\a. Int -> (Int -> a) -> Slice a` | Generate by index|
| `range` | `Int -> Int -> Slice Int` | Integer range [lo, hi) |
| `slice` | `\a. Int -> Int -> Slice a -> Slice a` | Sub-slice [start, end) |
| `sort` | `\a. Ord a => Slice a -> Slice a` | Sort (stable) |
| `sortBy` | `\a. (a -> a -> Ordering) -> Slice a -> Slice a` | Sort with comparator |
| `any` | `\a. (a -> Bool) -> Slice a -> Bool` | Any match |
| `all` | `\a. (a -> Bool) -> Slice a -> Bool` | All match |
| `find` | `\a. (a -> Bool) -> Slice a -> Maybe a` | First match |
| `findIndex` | `\a. (a -> Bool) -> Slice a -> Maybe Int` | Index of first match |
| `head` | `\a. Slice a -> Maybe a` | First element |
| `last` | `\a. Slice a -> Maybe a` | Last element |

Instances: `Functor Slice`, `Foldable Slice`, `FromList (Slice a)`, `ToList (Slice a)`, `Show a => Show (Slice a)`.
`toList`, `fromList`, `fmap`, and `foldr` are provided via these instances. They work unqualified but cannot be used with qualified syntax.

**Example:**

```
import Prelude
import Data.Slice as S

xs := fromList [10, 20, 30] :: Slice Int
main := (S.length xs, S.index 1 xs, toList (fmap (* 2) xs))
-- (3, Just 20, [20, 40, 60])
```
