### Data.Set

Provides an ordered immutable set backed by a Map. Load with `eng.Use(gicel.DataSet)` and import with `import Data.Set`.

**Functions:**

| Name           | Type                                   | Description            |
| -------------- | -------------------------------------- | ---------------------- |
| `empty`        | `\k. Ord k => Set k`                   | Empty set              |
| `insert`       | `\k. Ord k => k -> Set k -> Set k`     | Insert an element      |
| `member`       | `\k. Ord k => k -> Set k -> Bool`      | Membership test        |
| `delete`       | `\k. Ord k => k -> Set k -> Set k`     | Remove an element      |
| `size`         | `\k. Set k -> Int`                     | Number of elements     |
| `toList`       | `\k. Set k -> List k`                  | Sorted element list    |
| `fromList`     | `\k. Ord k => List k -> Set k`         | Build set from list    |
| `union`        | `\k. Ord k => Set k -> Set k -> Set k` | Set union              |
| `intersection` | `\k. Ord k => Set k -> Set k -> Set k` | Set intersection       |
| `difference`   | `\k. Ord k => Set k -> Set k -> Set k` | Set difference (A - B) |

**Notes:**

- Sets are persistent (immutable). Insert/delete return new sets.
- `toList` returns elements in sorted order.

> **Tip:** `Data.Set` exports `insert`, `member`, `delete`, `size` which overlap with `Data.Map`.
> Use qualified imports when both are needed: `import Data.Map as Map`, `import Data.Set as Set`.
