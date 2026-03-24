### Data.Stream

Provides lazy list (stream) operations. Requires recursion (`fix`), loaded via `RegisterModuleRec`. Load with `eng.Use(gicel.DataStream)` and import with `import Data.Stream`.

```
form Stream := \a. { LCons: a -> (() -> Stream a) -> Stream a; LNil: Stream a }
```

| Name       | Type                                        | Description            |
| ---------- | ------------------------------------------- | ---------------------- |
| `head`     | `\a. Stream a -> Maybe a`                   | First element          |
| `tail`     | `\a. Stream a -> Maybe (Stream a)`          | Tail (forces thunk)    |
| `toList`   | `\a. Stream a -> List a`                    | Convert to strict list |
| `fromList` | `\a. List a -> Stream a`                    | Convert to lazy stream |
| `fmap`     | `\a b. (a -> b) -> Stream a -> Stream b`    | Map over stream        |
| `foldr`    | `\a b. (a -> b -> b) -> b -> Stream a -> b` | Right fold             |
| `take`     | `\a. Int -> Stream a -> List a`             | Take first n as list   |
| `drop`     | `\a. Int -> Stream a -> Stream a`           | Drop first n           |

Instances: `Functor Stream`, `Foldable Stream`, `FromList (Stream a)`, `ToList (Stream a)`
