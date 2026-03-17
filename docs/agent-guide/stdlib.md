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
| `Show Int`      |
| `Num Int`       |
| `Semigroup Int` |
| `Monoid Int`    |

**Functions:** `add`, `sub`, `mul`, `negate` (class methods), `div`, `mod`, `abs`, `sign`.

**Operators:** `+` `-` (infixl 6), `*` `/` (infixl 7).

Instances: `Eq/Ord/Show/Num/Semigroup/Monoid Int`. Integer literals require `import Std.Num`. Negative numbers: `negate 5`. Division by zero is a runtime error.

### Std.Str

Provides string and rune operations.

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

### Std.List

Provides list operations (native-speed implementations).

**Functions:**

| Name          | Type                                                        | Description                                            |
| ------------- | ----------------------------------------------------------- | ------------------------------------------------------ |
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

`foldl` is strict. Prelude provides `foldr`, `map`, `filter`, `head`, `tail`, `null`, `singleton`, `append` for lists.

### Std.Map

Provides an ordered immutable map backed by an AVL tree. All key-parameterized operations require `Ord k`.

**Functions:**

| Name           | Type                                                         | Description                          |
| -------------- | ------------------------------------------------------------ | ------------------------------------ |
| `mapEmpty`     | `\k v. Ord k => Map k v`                               | Empty map                            |
| `insert`       | `\k v. Ord k => k -> v -> Map k v -> Map k v`          | Insert or overwrite a key-value pair |
| `mapLookup`    | `\k v. Ord k => k -> Map k v -> Maybe v`               | Lookup by key                        |
| `delete`       | `\k v. Ord k => k -> Map k v -> Map k v`               | Remove a key                         |
| `mapSize`      | `\k v. Map k v -> Int`                                 | Number of entries                    |
| `toList`       | `\k v. Map k v -> List (k, v)`                         | In-order key-value pairs             |
| `fromList`     | `\k v. Ord k => List (k, v) -> Map k v`                | Build map from pairs                 |
| `member`       | `\k v. Ord k => k -> Map k v -> Bool`                  | Key membership test                  |
| `foldlWithKey` | `\k v b. (b -> k -> v -> b) -> b -> Map k v -> b`      | Left fold with key and value         |
| `unionWith`    | `\k v. (v -> v -> v) -> Map k v -> Map k v -> Map k v` | Union, combining duplicates with f   |

**Notes:**

- Maps are persistent (immutable). Insert/delete return new maps.
- `toList` returns pairs sorted by key.

### Std.Set

Provides an ordered immutable set backed by a Map.

**Functions:**

| Name          | Type                                     | Description         |
| ------------- | ---------------------------------------- | ------------------- |
| `setEmpty`    | `\k. Ord k => Set k`               | Empty set           |
| `setInsert`   | `\k. Ord k => k -> Set k -> Set k` | Insert an element   |
| `setMember`   | `\k. Ord k => k -> Set k -> Bool`  | Membership test     |
| `setDelete`   | `\k. Ord k => k -> Set k -> Set k` | Remove an element   |
| `setSize`     | `\k. Set k -> Int`                 | Number of elements  |
| `setToList`   | `\k. Set k -> List k`              | Sorted element list |
| `setFromList` | `\k. Ord k => List k -> Set k`     | Build set from list |

**Notes:**

- Sets are persistent (immutable). Insert/delete return new sets.
- `setToList` returns elements in sorted order.

### Std.State

Provides get/put state capabilities via the `state` capability in CapEnv.

**Functions:**

| Name     | Type                                                                           | Description               |
| -------- | ------------------------------------------------------------------------------ | ------------------------- |
| `get`    | `\s r. Computation { state : s \| r } { state : s \| r } s`              | Read current state        |
| `put`    | `\s r. s -> Computation { state : s \| r } { state : s \| r } ()`        | Replace current state     |
| `modify` | `\s r. (s -> s) -> Computation { state : s \| r } { state : s \| r } ()` | Apply a function to state |

Host provides `"state"` capability. Final state is in `result.CapEnv`.

### Std.Fail

Provides failure/error effects via the `fail` capability.

**Functions:**

| Name         | Type                                                                            | Description                     |
| ------------ | ------------------------------------------------------------------------------- | ------------------------------- |
| `failWith`   | `\e r a. e -> Computation { fail : e \| r } { fail : e \| r } a`          | Fail with a typed error value   |
| `fail`       | `\r a. Computation { fail : () \| r } { fail : () \| r } a`               | Fail with () (no error payload) |
| `fromMaybe`  | `\a r. Maybe a -> Computation { fail : () \| r } { fail : () \| r } a`    | Extract Just or fail on Nothing |
| `fromResult` | `\e a r. Result e a -> Computation { fail : e \| r } { fail : e \| r } a` | Extract Ok or failWith on Err   |

`fail`/`failWith` abort the computation. No catch/recover at language level; the host handles errors.

### Std.IO

Provides print/debug capabilities via the `io` capability.

**Functions:**

| Name    | Type                                                              | Description                              |
| ------- | ----------------------------------------------------------------- | ---------------------------------------- |
| `print` | `String -> Computation { io : () \| r } { io : () \| r } ()`      | Append a string to the IO buffer         |
| `debug` | `\a. a -> Computation { io : () \| r } { io : () \| r } ()` | Append debug representation to IO buffer |

Host provides `"io"` capability. Output accumulates as `[]string` in the final CapEnv.

### Std.Stream

Provides lazy list (stream) operations. Requires recursion (`fix`), loaded via `RegisterModuleRec`.

```
data Stream a = LCons a (() -> Stream a) | LNil
```

| Name       | Type                                              | Description            |
| ---------- | ------------------------------------------------- | ---------------------- |
| `headS`    | `\a. Stream a -> Maybe a`                   | First element          |
| `tailS`    | `\a. Stream a -> Maybe (Stream a)`          | Tail (forces thunk)    |
| `toList`   | `\a. Stream a -> List a`                    | Convert to strict list |
| `fromList` | `\a. List a -> Stream a`                    | Convert to lazy stream |
| `mapS`     | `\a b. (a -> b) -> Stream a -> Stream b`    | Map over stream        |
| `foldrS`   | `\a b. (a -> b -> b) -> b -> Stream a -> b` | Right fold             |
| `takeS`    | `\a. Int -> Stream a -> List a`             | Take first n as list   |
| `dropS`    | `\a. Int -> Stream a -> Stream a`           | Drop first n           |

Instances: `Functor Stream`, `Foldable Stream`

### Std.Slice

Provides contiguous array with O(1) length/index.

| Name             | Type                                             | Description    |
| ---------------- | ------------------------------------------------ | -------------- |
| `sliceEmpty`     | `\a. Slice a`                              | Empty slice    |
| `sliceSingleton` | `\a. a -> Slice a`                         | Single-element |
| `sliceCons`      | `\a. a -> Slice a -> Slice a`              | Prepend        |
| `sliceSnoc`      | `\a. Slice a -> a -> Slice a`              | Append         |
| `sliceLength`    | `\a. Slice a -> Int`                       | O(1) length    |
| `sliceIndex`     | `\a. Int -> Slice a -> Maybe a`            | O(1) index     |
| `sliceFromList`  | `\a. List a -> Slice a`                    | From list      |
| `sliceToList`    | `\a. Slice a -> List a`                    | To list        |
| `sliceAppend`    | `\a. Slice a -> Slice a -> Slice a`        | Concatenate    |
| `sliceFoldr`     | `\a b. (a -> b -> b) -> b -> Slice a -> b` | Right fold     |
| `sliceFoldl`     | `\a b. (b -> a -> b) -> b -> Slice a -> b` | Left fold      |

Instances: `Functor Slice`, `Foldable Slice`, `Semigroup (Slice a)`, `Monoid (Slice a)`, `Packed (Slice a) a`

---
