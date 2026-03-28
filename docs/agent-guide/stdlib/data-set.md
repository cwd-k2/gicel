### Data.Set

Provides an ordered immutable set backed by a Map. Load with `eng.Use(gicel.DataSet)` and import with `import Data.Set`.

**Functions:**

| Name           | Type                                                         | Description                        |
| -------------- | ------------------------------------------------------------ | ---------------------------------- |
| `empty`        | `\k. Ord k => Set k`                                         | Empty set                          |
| `insert`       | `\k. Ord k => k -> Set k -> Set k`                           | Insert an element                  |
| `member`       | `\k. Ord k => k -> Set k -> Bool`                            | Membership test                    |
| `delete`       | `\k. Ord k => k -> Set k -> Set k`                           | Remove an element                  |
| `size`         | `\k. Set k -> Int`                                           | Number of elements                 |
| `toList`       | `\k. Set k -> List k`                                        | Sorted element list                |
| `fromList`     | `\k. Ord k => List k -> Set k`                               | Build set from list                |
| `union`        | `\k. Ord k => Set k -> Set k -> Set k`                       | Set union                          |
| `intersection` | `\k. Ord k => Set k -> Set k -> Set k`                       | Set intersection                   |
| `difference`   | `\k. Ord k => Set k -> Set k -> Set k`                       | Set difference (A - B)             |
| `null`         | `\k. Set k -> Bool`                                          | Test if set is empty               |
| `singleton`    | `\k. Ord k => k -> Set k`                                    | Single-element set                 |
| `isSubsetOf`   | `\k. Ord k => Set k -> Set k -> Bool`                        | Test if first is subset of second  |
| `findMin`      | `\k. Set k -> Maybe k`                                       | Smallest element                   |
| `findMax`      | `\k. Set k -> Maybe k`                                       | Largest element                    |
| `partition`    | `\k. Ord k => (k -> Bool) -> Set k -> (Set k, Set k)`        | Split by predicate (pass, fail)    |
| `filter`       | `\k. Ord k => (k -> Bool) -> Set k -> Set k`                 | Keep elements satisfying predicate |
| `map`          | `\k1 k2. (Ord k1, Ord k2) => (k1 -> k2) -> Set k1 -> Set k2` | Map a function over elements       |
| `fold`         | `\k b. (b -> k -> b) -> b -> Set k -> b`                     | Left fold over elements            |

**Notes:**

- Sets are persistent (immutable). Insert/delete return new sets.
- `toList` returns elements in sorted order.
- `toList` and `fromList` are typeclass instance methods (`ToList`, `FromList`), not direct module exports. They work unqualified but cannot be used with qualified syntax.

> **Tip:** `Data.Set` exports `insert`, `member`, `delete`, `size` which overlap with `Data.Map`.
> Use qualified imports when both are needed: `import Data.Map as Map`, `import Data.Set as Set`.
