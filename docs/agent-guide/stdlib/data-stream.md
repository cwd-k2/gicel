### Data.Stream

Provides lazy list (stream) operations. Internally uses recursion (`fix`) via `RegisterModuleRec`; no `--recursion` flag needed by the user. Load with `eng.Use(gicel.DataStream)` and import with `import Data.Stream`.

```
form Stream := \a. { LCons: a -> (() -> Stream a) -> Stream a; LNil: Stream a }
```

| Name        | Type                                                        | Description                           |
| ----------- | ----------------------------------------------------------- | ------------------------------------- |
| `head`      | `\a. Stream a -> Maybe a`                                   | First element                         |
| `tail`      | `\a. Stream a -> Maybe (Stream a)`                          | Tail (forces thunk)                   |
| `toList`    | `\a. Stream a -> List a`                                    | Convert to strict list                |
| `fromList`  | `\a. List a -> Stream a`                                    | Convert to lazy stream                |
| `fmap`      | `\a b. (a -> b) -> Stream a -> Stream b`                    | Map over stream                       |
| `foldr`     | `\a b. (a -> b -> b) -> b -> Stream a -> b`                 | Right fold                            |
| `take`      | `\a. Int -> Stream a -> List a`                             | Take first n as list                  |
| `drop`      | `\a. Int -> Stream a -> Stream a`                           | Drop first n                          |
| `filter`    | `\a. (a -> Bool) -> Stream a -> Stream a`                   | Keep elements matching pred           |
| `zipWith`   | `\a b c. (a -> b -> c) -> Stream a -> Stream b -> Stream c` | Combine two streams with f            |
| `zip`       | `\a b. Stream a -> Stream b -> Stream (a, b)`               | Pair elements of two streams          |
| `iterate`   | `\a. (a -> a) -> a -> Stream a`                             | Infinite stream: x, f x, f (f x), ... |
| `takeWhile` | `\a. (a -> Bool) -> Stream a -> List a`                     | Take while predicate holds            |
| `repeat`    | `\a. a -> Stream a`                                         | Infinite constant stream              |

Instances: `Functor Stream`, `Foldable Stream`, `FromList (Stream a)`, `ToList (Stream a)`

**Example:**

```
import Prelude
import Data.Stream

main := take 5 (iterate (* 2) 1)   -- [1, 2, 4, 8, 16]
```
