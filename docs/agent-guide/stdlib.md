## 7. Stdlib Reference

Each stdlib module must be loaded on the host side (`eng.Use(gicel.Num)`) and imported in source (`import Std.Num`).

### Std.Num

Provides integer arithmetic.

**Type class:**

```
class Eq a => Num a {
  add    :: a -> a -> a;
  sub    :: a -> a -> a;
  mul    :: a -> a -> a;
  negate :: a -> a
}
```

**Instances:**

| Instance        |
| --------------- |
| `Eq Int`        |
| `Ord Int`       |
| `Num Int`       |
| `Semigroup Int` |
| `Monoid Int`    |

**Functions:**

| Name     | Type                             | Description                   |
| -------- | -------------------------------- | ----------------------------- |
| `add`    | `forall a. Num a => a -> a -> a` | Addition (class method)       |
| `sub`    | `forall a. Num a => a -> a -> a` | Subtraction (class method)    |
| `mul`    | `forall a. Num a => a -> a -> a` | Multiplication (class method) |
| `negate` | `forall a. Num a => a -> a`      | Negation (class method)       |
| `div`    | `Int -> Int -> Int`              | Integer division              |
| `mod`    | `Int -> Int -> Int`              | Modulo                        |
| `abs`    | `Int -> Int`                     | Absolute value                |
| `sign`   | `Int -> Int`                     | Sign (-1, 0, or 1)            |

**Operators:**

| Operator | Fixity     | Type                             |
| -------- | ---------- | -------------------------------- |
| `+`      | `infixl 6` | `forall a. Num a => a -> a -> a` |
| `-`      | `infixl 6` | `forall a. Num a => a -> a -> a` |
| `*`      | `infixl 7` | `forall a. Num a => a -> a -> a` |
| `/`      | `infixl 7` | `Int -> Int -> Int`              |

**Notes:**

- Integer literals (`42`) only parse when Std.Num is imported.
- Negative numbers: write `negate 5`, not `-5`.
- Division by zero is a runtime error.

### Std.Str

Provides string and rune operations.

**Instances:**

| Instance           |
| ------------------ |
| `Eq String`        |
| `Ord String`       |
| `Semigroup String` |
| `Monoid String`    |
| `Eq Rune`          |
| `Ord Rune`         |

**Functions:**

| Name        | Type                              | Description                                      |
| ----------- | --------------------------------- | ------------------------------------------------ |
| `strlen`    | `String -> Int`                   | Length in runes (Unicode code points)            |
| `toRunes`   | `String -> List Rune`             | Convert string to list of runes                  |
| `charAt`    | `Int -> String -> Maybe Rune`     | Rune at index (0-based), Nothing if out of range |
| `substring` | `Int -> Int -> String -> String`  | `substring start count s` extracts a substring   |
| `toUpper`   | `String -> String`                | Convert to uppercase                             |
| `toLower`   | `String -> String`                | Convert to lowercase                             |
| `trim`      | `String -> String`                | Trim leading/trailing whitespace                 |
| `contains`  | `String -> String -> Bool`        | `contains needle haystack`                       |
| `split`     | `String -> String -> List String` | `split separator string`                         |
| `join`      | `String -> List String -> String` | `join separator parts`                           |
| `showInt`   | `Int -> String`                   | Convert Int to its decimal string                |
| `showBool`  | `Bool -> String`                  | Convert Bool to "True" or "False"                |
| `readInt`   | `String -> Maybe Int`             | Parse decimal string to Int                      |

Instances: `Eq/Ord/Semigroup/Monoid String`, `Eq/Ord Rune`, `Packed String Rune`. `strlen` counts runes, not bytes. `charAt`/`substring` use 0-based rune indexing. Concatenation via `append`.

### Std.List

Provides list operations (native-speed implementations).

**Functions:**

| Name        | Type                                            | Description                                            |
| ----------- | ----------------------------------------------- | ------------------------------------------------------ |
| `fromSlice` | `forall a. List a -> List a`                    | Identity on Cons/Nil chains; converts HostVal slices   |
| `toSlice`   | `forall a. List a -> List a`                    | Identity on Cons/Nil chains; converts to HostVal slice |
| `length`    | `forall a. List a -> Int`                       | Count elements                                         |
| `concat`    | `forall a. List a -> List a -> List a`          | Concatenate two lists                                  |
| `foldl`     | `forall a b. (b -> a -> b) -> b -> List a -> b` | Strict left fold                                       |
| `take`      | `forall a. Int -> List a -> List a`             | First n elements                                       |
| `drop`      | `forall a. Int -> List a -> List a`             | Drop first n elements                                  |
| `index`     | `forall a. Int -> List a -> Maybe a`            | Element at index (0-based)                             |
| `replicate` | `forall a. Int -> a -> List a`                  | List of n copies of a value                            |
| `reverse`   | `forall a. List a -> List a`                    | Reverse a list                                         |
| `zip`       | `forall a b. List a -> List b -> List (a, b)`   | Zip two lists into pairs                               |
| `unzip`     | `forall a b. List (a, b) -> (List a, List b)`   | Unzip a list of pairs                                  |

`foldl` is strict. Prelude provides `foldr`, `map`, `filter`, `head`, `tail`, `null`, `singleton`, `append` for lists.

### Std.State

Provides get/put state capabilities via the `state` capability in CapEnv.

**Functions:**

| Name     | Type                                                                           | Description               |
| -------- | ------------------------------------------------------------------------------ | ------------------------- |
| `get`    | `forall s r. Computation { state : s \| r } { state : s \| r } s`              | Read current state        |
| `put`    | `forall s r. s -> Computation { state : s \| r } { state : s \| r } ()`        | Replace current state     |
| `modify` | `forall s r. (s -> s) -> Computation { state : s \| r } { state : s \| r } ()` | Apply a function to state |

Host provides `"state"` capability. Final state is in `result.CapEnv`.

### Std.Fail

Provides failure/error effects via the `fail` capability.

**Functions:**

| Name         | Type                                                                            | Description                     |
| ------------ | ------------------------------------------------------------------------------- | ------------------------------- |
| `failWith`   | `forall e r a. e -> Computation { fail : e \| r } { fail : e \| r } a`          | Fail with a typed error value   |
| `fail`       | `forall r a. Computation { fail : () \| r } { fail : () \| r } a`               | Fail with () (no error payload) |
| `fromMaybe`  | `forall a r. Maybe a -> Computation { fail : () \| r } { fail : () \| r } a`    | Extract Just or fail on Nothing |
| `fromResult` | `forall e a r. Result e a -> Computation { fail : e \| r } { fail : e \| r } a` | Extract Ok or failWith on Err   |

`fail`/`failWith` abort the computation. No catch/recover at language level; the host handles errors.

### Std.IO

Provides print/debug capabilities via the `io` capability.

**Functions:**

| Name    | Type                                                              | Description                              |
| ------- | ----------------------------------------------------------------- | ---------------------------------------- |
| `print` | `String -> Computation { io : () \| r } { io : () \| r } ()`      | Append a string to the IO buffer         |
| `debug` | `forall a. a -> Computation { io : () \| r } { io : () \| r } ()` | Append debug representation to IO buffer |

Host provides `"io"` capability. Output accumulates as `[]string` in the final CapEnv.

### Std.Stream

Provides lazy list (stream) operations. Requires recursion (`fix`), loaded via `RegisterModuleRec`.

```
data Stream a = LCons a (() -> Stream a) | LNil
```

| Name       | Type                                              | Description            |
| ---------- | ------------------------------------------------- | ---------------------- |
| `headS`    | `forall a. Stream a -> Maybe a`                   | First element          |
| `tailS`    | `forall a. Stream a -> Maybe (Stream a)`          | Tail (forces thunk)    |
| `toList`   | `forall a. Stream a -> List a`                    | Convert to strict list |
| `fromList` | `forall a. List a -> Stream a`                    | Convert to lazy stream |
| `mapS`     | `forall a b. (a -> b) -> Stream a -> Stream b`    | Map over stream        |
| `foldrS`   | `forall a b. (a -> b -> b) -> b -> Stream a -> b` | Right fold             |
| `takeS`    | `forall a. Int -> Stream a -> List a`             | Take first n as list   |
| `dropS`    | `forall a. Int -> Stream a -> Stream a`           | Drop first n           |

Instances: `Functor Stream`, `Foldable Stream`

### Std.Slice

Provides contiguous array with O(1) length/index.

| Name             | Type                                             | Description    |
| ---------------- | ------------------------------------------------ | -------------- |
| `sliceEmpty`     | `forall a. Slice a`                              | Empty slice    |
| `sliceSingleton` | `forall a. a -> Slice a`                         | Single-element |
| `sliceCons`      | `forall a. a -> Slice a -> Slice a`              | Prepend        |
| `sliceSnoc`      | `forall a. Slice a -> a -> Slice a`              | Append         |
| `sliceLength`    | `forall a. Slice a -> Int`                       | O(1) length    |
| `sliceIndex`     | `forall a. Int -> Slice a -> Maybe a`            | O(1) index     |
| `sliceFromList`  | `forall a. List a -> Slice a`                    | From list      |
| `sliceToList`    | `forall a. Slice a -> List a`                    | To list        |
| `sliceAppend`    | `forall a. Slice a -> Slice a -> Slice a`        | Concatenate    |
| `sliceFoldr`     | `forall a b. (a -> b -> b) -> b -> Slice a -> b` | Right fold     |
| `sliceFoldl`     | `forall a b. (b -> a -> b) -> b -> Slice a -> b` | Left fold      |

Instances: `Functor Slice`, `Foldable Slice`, `Semigroup (Slice a)`, `Monoid (Slice a)`, `Packed (Slice a) a`

---
