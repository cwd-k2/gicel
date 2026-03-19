## 7. Stdlib Reference

Stdlib packs are loaded on the host side via `eng.Use(pack)` and imported in source. Core is auto-registered and auto-imported; the user cannot control it. Prelude requires explicit `eng.Use(gicel.Prelude)` on the engine and `import Prelude` in source. `NewEngine()` returns a bare engine with only Core.

### Prelude

Prelude bundles what was previously Num, Str, and List into a single pack. Load with `eng.Use(gicel.Prelude)` and import with `import Prelude`. All types, instances, functions, and operators below become available with a single import.

#### Num (arithmetic)

**Type classes:**

```
class Eq a => Num a {
  add    :: a -> a -> a;
  sub    :: a -> a -> a;
  mul    :: a -> a -> a;
  negate :: a -> a
}

class Num a => Div a {
  div :: a -> a -> a
}
```

**Instances:**

| Instance           |
| ------------------ |
| `Eq Int`           |
| `Ord Int`          |
| `Show Int`         |
| `Num Int`          |
| `Div Int`          |
| `Semigroup Int`    |
| `Monoid Int`       |
| `Eq Double`        |
| `Ord Double`       |
| `Show Double`      |
| `Num Double`       |
| `Div Double`       |
| `Semigroup Double` |
| `Monoid Double`    |

**Functions:** `add`, `sub`, `mul`, `negate` (Num methods), `div` (Div method), `mod`, `abs`, `sign`, `toDouble`, `round`, `floor`, `ceiling`, `truncate`, `absDouble`, `signDouble`.

**Operators:** `+` `-` (infixl 6), `*` `/` (infixl 7). `+`, `-`, `*` are `Num a => a -> a -> a`; `/` is `Div a => a -> a -> a`.

Instances: `Eq/Ord/Show/Num/Div/Semigroup/Monoid` for both `Int` and `Double`. Integer literals require `import Prelude`. Negative numbers: `negate 5`. Division by zero is a runtime error. Integer overflow wraps silently (Go `int64` two's-complement semantics).

#### Str (string and rune operations)

**Instances:**

| Instance             |
| -------------------- |
| `Eq String`          |
| `Ord String`         |
| `Semigroup String`   |
| `Monoid String`      |
| `Eq Rune`            |
| `Ord Rune`           |
| `Packed String Rune` |
| `Show String`        |
| `Show (Maybe a)`     |
| `Show (List a)`      |
| `Show (Result e a)`  |
| `Show (a, b)`        |

**Functions:** `strlen`, `toRunes`, `fromRunes`, `charAt`, `substring`, `toUpper`, `toLower`, `trim`, `contains`, `split`, `join`, `showInt`, `showBool`, `readInt`.

Rune-based indexing. Concatenation via `append`.

#### List (native-speed list operations)

**Functions:**

| Name          | Type                                                  | Description                                            |
| ------------- | ----------------------------------------------------- | ------------------------------------------------------ |
| `fromSlice`   | `\a. List a -> List a`                                | Identity on Cons/Nil chains; converts HostVal slices   |
| `toSlice`     | `\a. List a -> List a`                                | Identity on Cons/Nil chains; converts to HostVal slice |
| `length`      | `\a. List a -> Int`                                   | Count elements                                         |
| `concat`      | `\a. List a -> List a -> List a`                      | Concatenate two lists                                  |
| `foldl`       | `\a b. (b -> a -> b) -> b -> List a -> b`             | Strict left fold                                       |
| `take`        | `\a. Int -> List a -> List a`                         | First n elements                                       |
| `drop`        | `\a. Int -> List a -> List a`                         | Drop first n elements                                  |
| `index`       | `\a. Int -> List a -> Maybe a`                        | Element at index (0-based)                             |
| `replicate`   | `\a. Int -> a -> List a`                              | List of n copies of a value                            |
| `reverse`     | `\a. List a -> List a`                                | Reverse a list                                         |
| `zip`         | `\a b. List a -> List b -> List (a, b)`               | Zip two lists into pairs                               |
| `unzip`       | `\a b. List (a, b) -> (List a, List b)`               | Unzip a list of pairs                                  |
| `dropWhile`   | `\a. (a -> Bool) -> List a -> List a`                 | Drop leading elements while predicate holds            |
| `span`        | `\a. (a -> Bool) -> List a -> (List a, List a)`       | Split at first element failing predicate               |
| `break`       | `\a. (a -> Bool) -> List a -> (List a, List a)`       | Split at first element satisfying predicate            |
| `sortBy`      | `\a. (a -> a -> Ordering) -> List a -> List a`        | Merge sort with custom comparator                      |
| `sort`        | `\a. Ord a => List a -> List a`                       | Merge sort using `compare`                             |
| `scanl`       | `\a b. (b -> a -> b) -> b -> List a -> List b`        | Left scan collecting accumulator values                |
| `unfoldr`     | `\a b. (b -> Maybe (a, b)) -> b -> List a`            | Build list from seed                                   |
| `iterateN`    | `\a. Int -> (a -> a) -> a -> List a`                  | Generate n elements by repeated application            |
| `zipWith`     | `\a b c. (a -> b -> c) -> List a -> List b -> List c` | Zip with combining function                            |
| `intercalate` | `\a. List a -> List (List a) -> List a`               | Insert separator between sublists and flatten          |
| `nubBy`       | `\a. (a -> a -> Bool) -> List a -> List a`            | Remove duplicates with custom equality                 |

`foldl` is strict. Prelude also provides `foldr`, `map`, `filter`, `head`, `tail`, `null`, `singleton`, `append` for lists.

### Data.Map

Provides an ordered immutable map backed by an AVL tree. All key-parameterized operations require `Ord k`. Load with `eng.Use(gicel.DataMap)` and import with `import Data.Map`.

**Functions:**

| Name           | Type                                                   | Description                          |
| -------------- | ------------------------------------------------------ | ------------------------------------ |
| `empty`        | `\k v. Ord k => Map k v`                               | Empty map                            |
| `insert`       | `\k v. Ord k => k -> v -> Map k v -> Map k v`          | Insert or overwrite a key-value pair |
| `lookup`       | `\k v. Ord k => k -> Map k v -> Maybe v`               | Lookup by key                        |
| `delete`       | `\k v. Ord k => k -> Map k v -> Map k v`               | Remove a key                         |
| `size`         | `\k v. Map k v -> Int`                                 | Number of entries                    |
| `toList`       | `\k v. Map k v -> List (k, v)`                         | In-order key-value pairs             |
| `fromList`     | `\k v. Ord k => List (k, v) -> Map k v`                | Build map from pairs                 |
| `member`       | `\k v. Ord k => k -> Map k v -> Bool`                  | Key membership test                  |
| `foldlWithKey` | `\k v b. (b -> k -> v -> b) -> b -> Map k v -> b`      | Left fold with key and value         |
| `unionWith`    | `\k v. (v -> v -> v) -> Map k v -> Map k v -> Map k v` | Union, combining duplicates with f   |

**Notes:**

- Maps are persistent (immutable). Insert/delete return new maps.
- `toList` returns pairs sorted by key.

### Data.Set

Provides an ordered immutable set backed by a Map. Load with `eng.Use(gicel.DataSet)` and import with `import Data.Set`.

**Functions:**

| Name       | Type                               | Description         |
| ---------- | ---------------------------------- | ------------------- |
| `empty`    | `\k. Ord k => Set k`               | Empty set           |
| `insert`   | `\k. Ord k => k -> Set k -> Set k` | Insert an element   |
| `member`   | `\k. Ord k => k -> Set k -> Bool`  | Membership test     |
| `delete`   | `\k. Ord k => k -> Set k -> Set k` | Remove an element   |
| `size`     | `\k. Set k -> Int`                 | Number of elements  |
| `toList`   | `\k. Set k -> List k`              | Sorted element list |
| `fromList` | `\k. Ord k => List k -> Set k`     | Build set from list |

**Notes:**

- Sets are persistent (immutable). Insert/delete return new sets.
- `toList` returns elements in sorted order.

### Effect.State

Provides get/put state capabilities via the `state` capability in CapEnv. Load with `eng.Use(gicel.EffectState)` and import with `import Effect.State`.

**Functions:**

| Name     | Type                                                                   | Description               |
| -------- | ---------------------------------------------------------------------- | ------------------------- |
| `get`    | `\s r. Computation { state: s \| r } { state: s \| r } s`              | Read current state        |
| `put`    | `\s r. s -> Computation { state: s \| r } { state: s \| r } ()`        | Replace current state     |
| `modify` | `\s r. (s -> s) -> Computation { state: s \| r } { state: s \| r } ()` | Apply a function to state |

Host provides `"state"` capability. Final state is in `result.CapEnv`.

### Effect.Fail

Provides failure/error effects via the `fail` capability. Load with `eng.Use(gicel.EffectFail)` and import with `import Effect.Fail`.

**Functions:**

| Name         | Type                                                                    | Description                     |
| ------------ | ----------------------------------------------------------------------- | ------------------------------- |
| `failWith`   | `\e r a. e -> Computation { fail: e \| r } { fail: e \| r } a`          | Fail with a typed error value   |
| `fail`       | `\r a. Computation { fail: () \| r } { fail: () \| r } a`               | Fail with () (no error payload) |
| `fromMaybe`  | `\a r. Maybe a -> Computation { fail: () \| r } { fail: () \| r } a`    | Extract Just or fail on Nothing |
| `fromResult` | `\e a r. Result e a -> Computation { fail: e \| r } { fail: e \| r } a` | Extract Ok or failWith on Err   |

`fail`/`failWith` abort the computation. No catch/recover at language level; the host handles errors.

### Effect.IO

Provides print/debug capabilities via the `io` capability. Load with `eng.Use(gicel.EffectIO)` and import with `import Effect.IO`.

**Functions:**

| Name    | Type                                                       | Description                              |
| ------- | ---------------------------------------------------------- | ---------------------------------------- |
| `print` | `String -> Computation { io: () \| r } { io: () \| r } ()` | Append a string to the IO buffer         |
| `debug` | `\a. a -> Computation { io: () \| r } { io: () \| r } ()`  | Append debug representation to IO buffer |

Host provides `"io"` capability. Output accumulates as `[]string` in the final CapEnv.

### Data.Stream

Provides lazy list (stream) operations. Requires recursion (`fix`), loaded via `RegisterModuleRec`. Load with `eng.Use(gicel.DataStream)` and import with `import Data.Stream`.

```
data Stream a := LCons a (() -> Stream a) | LNil
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

### Data.Slice

Provides contiguous array with O(1) length/index. Load with `eng.Use(gicel.DataSlice)` and import with `import Data.Slice`.

| Name        | Type                                       | Description    |
| ----------- | ------------------------------------------ | -------------- |
| `empty`     | `\a. Slice a`                              | Empty slice    |
| `singleton` | `\a. a -> Slice a`                         | Single-element |
| `cons`      | `\a. a -> Slice a -> Slice a`              | Prepend        |
| `snoc`      | `\a. Slice a -> a -> Slice a`              | Append one     |
| `length`    | `\a. Slice a -> Int`                       | O(1) length    |
| `index`     | `\a. Int -> Slice a -> Maybe a`            | O(1) index     |
| `append`    | `\a. Slice a -> Slice a -> Slice a`        | Concatenate    |
| `foldl`     | `\a b. (b -> a -> b) -> b -> Slice a -> b` | Left fold      |

Instances: `Functor Slice`, `Foldable Slice`, `Semigroup (Slice a)`, `Monoid (Slice a)`, `Packed (Slice a) a`, `FromList (Slice a)`, `ToList (Slice a)`

---
