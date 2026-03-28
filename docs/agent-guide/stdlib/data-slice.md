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

Instances: `Functor Slice`, `Foldable Slice`, `FromList (Slice a)`, `ToList (Slice a)`.
`toList`, `fromList`, `fmap`, and `foldr` are provided via these instances. They work unqualified but cannot be used with qualified syntax.

**Example:**

```
import Prelude
import Data.Slice as S

xs := fromList [10, 20, 30] :: Slice Int
main := (S.length xs, S.index 1 xs, toList (fmap (* 2) xs))
-- (3, Just 20, [20, 40, 60])
```
