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

**Notes:**

- `strlen` counts Unicode code points, not bytes. Named `strlen` (not `length`) to avoid collision with `Std.List.length`.
- `toRunes` decomposes a string into a `List Rune` for character-level processing.
- `charAt` and `substring` use 0-based rune indexing.
- String concatenation uses the `append` method from `Semigroup String`. The empty string is `empty` from `Monoid String`.
- No string interpolation. Build strings with `append` and conversion functions.

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

**Notes:**

- When both Std.List and Std.Str are imported, `length` may be ambiguous. Qualify if needed, or use only one.
- `foldl` is strict (evaluates the accumulator at each step).
- The Prelude already provides `foldr`, `map`, `filter`, `head`, `tail`, `null`, `singleton`, and `append` for lists.

### Std.State

Provides get/put state capabilities via the `state` capability in CapEnv.

**Functions:**

| Name     | Type                                                                           | Description               |
| -------- | ------------------------------------------------------------------------------ | ------------------------- |
| `get`    | `forall s r. Computation { state : s \| r } { state : s \| r } s`              | Read current state        |
| `put`    | `forall s r. s -> Computation { state : s \| r } { state : s \| r } ()`        | Replace current state     |
| `modify` | `forall s r. (s -> s) -> Computation { state : s \| r } { state : s \| r } ()` | Apply a function to state |

**Host setup:** Provide the initial state as the `"state"` capability:

```go
caps := map[string]any{"state": gicel.ToValue(0)}
result, err := rt.RunContextFull(ctx, caps, nil, "main")
// result.CapEnv contains the final state
```

### Std.Fail

Provides failure/error effects via the `fail` capability.

**Functions:**

| Name         | Type                                                                            | Description                     |
| ------------ | ------------------------------------------------------------------------------- | ------------------------------- |
| `failWith`   | `forall e r a. e -> Computation { fail : e \| r } { fail : e \| r } a`          | Fail with a typed error value   |
| `fail`       | `forall r a. Computation { fail : () \| r } { fail : () \| r } a`               | Fail with () (no error payload) |
| `fromMaybe`  | `forall a r. Maybe a -> Computation { fail : () \| r } { fail : () \| r } a`    | Extract Just or fail on Nothing |
| `fromResult` | `forall e a r. Result e a -> Computation { fail : e \| r } { fail : e \| r } a` | Extract Ok or failWith on Err   |

**Notes:**

- `fail` and `failWith` abort the computation. There is no catch/recover at the language level; the host handles the error.
- The return type is `a` (universally quantified), meaning failure can appear in any position.

### Std.IO

Provides print/debug capabilities via the `io` capability.

**Functions:**

| Name    | Type                                                              | Description                              |
| ------- | ----------------------------------------------------------------- | ---------------------------------------- |
| `print` | `String -> Computation { io : () \| r } { io : () \| r } ()`      | Append a string to the IO buffer         |
| `debug` | `forall a. a -> Computation { io : () \| r } { io : () \| r } ()` | Append debug representation to IO buffer |

**Host setup:** Provide the `"io"` capability. Output accumulates as `[]string` in the final CapEnv:

```go
caps := map[string]any{"io": gicel.ToValue(nil)}
result, err := rt.RunContextFull(ctx, caps, nil, "main")
// Read output:
ioVal, _ := result.CapEnv.Get("io")
lines := ioVal.([]string)
```

---
