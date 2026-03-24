### Data.Map

Provides an ordered immutable map backed by an AVL tree. All key-parameterized operations require `Ord k`. Load with `eng.Use(gicel.DataMap)` and import with `import Data.Map`.

**Functions:**

| Name            | Type                                                   | Description                          |
| --------------- | ------------------------------------------------------ | ------------------------------------ |
| `empty`         | `\k v. Ord k => Map k v`                               | Empty map                            |
| `insert`        | `\k v. Ord k => k -> v -> Map k v -> Map k v`          | Insert or overwrite a key-value pair |
| `lookup`        | `\k v. Ord k => k -> Map k v -> Maybe v`               | Lookup by key                        |
| `delete`        | `\k v. Ord k => k -> Map k v -> Map k v`               | Remove a key                         |
| `size`          | `\k v. Map k v -> Int`                                 | Number of entries                    |
| `toList`        | `\k v. Map k v -> List (k, v)`                         | In-order key-value pairs             |
| `fromList`      | `\k v. Ord k => List (k, v) -> Map k v`                | Build map from pairs                 |
| `member`        | `\k v. Ord k => k -> Map k v -> Bool`                  | Key membership test                  |
| `foldlWithKey`  | `\k v b. (b -> k -> v -> b) -> b -> Map k v -> b`      | Left fold with key and value         |
| `unionWith`     | `\k v. (v -> v -> v) -> Map k v -> Map k v -> Map k v` | Union, combining duplicates with f   |
| `keys`          | `\k v. Map k v -> List k`                              | All keys in sorted order             |
| `values`        | `\k v. Map k v -> List v`                              | All values in key order              |
| `mapValues`     | `\k v w. (v -> w) -> Map k v -> Map k w`               | Apply function to each value         |
| `filterWithKey` | `\k v. (k -> v -> Bool) -> Map k v -> Map k v`         | Keep entries where predicate is true |

**Notes:**

- Maps are persistent (immutable). Insert/delete return new maps.
- `toList` returns pairs sorted by key.

> **Tip:** `Data.Map` exports `insert`, `member`, `delete`, `size` which overlap with `Data.Set`.
> Use qualified imports when both are needed: `import Data.Map as Map`, `import Data.Set as Set`.
