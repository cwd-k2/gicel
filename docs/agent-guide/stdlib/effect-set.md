### Effect.Set

Provides mutable ordered sets backed by AVL trees (internally MMap k ()), gated by the `{ mset: () }` effect. Load with `eng.Use(gicel.EffectSet)` and import with `import Effect.Set`.

**Functions:**

| Name           | Type                                                                   | Description                         |
| -------------- | ---------------------------------------------------------------------- | ----------------------------------- |
| `new`          | `\k r. Ord k => Effect { mset: () \| r } (MSet k)`                     | Create empty mutable set            |
| `insert`       | `\k r. Ord k => k -> MSet k -> Effect { mset: () \| r } ()`            | Insert element (in-place)           |
| `member`       | `\k r. Ord k => k -> MSet k -> Effect { mset: () \| r } Bool`          | Membership test                     |
| `delete`       | `\k r. Ord k => k -> MSet k -> Effect { mset: () \| r } ()`            | Remove element (in-place)           |
| `size`         | `\k r. MSet k -> Effect { mset: () \| r } Int`                         | Number of elements                  |
| `toList`       | `\k r. MSet k -> Effect { mset: () \| r } (List k)`                    | Sorted element list                 |
| `fromList`     | `\k r. Ord k => List k -> Effect { mset: () \| r } (MSet k)`           | Build from list                     |
| `fold`         | `\k b r. (b -> k -> b) -> b -> MSet k -> Effect { mset: () \| r } b`   | Left fold over elements             |
| `union`        | `\k r. Ord k => MSet k -> MSet k -> Effect { mset: () \| r } (MSet k)` | Union (returns new set)             |
| `intersection` | `\k r. Ord k => MSet k -> MSet k -> Effect { mset: () \| r } (MSet k)` | Intersection (returns new set)      |
| `difference`   | `\k r. Ord k => MSet k -> MSet k -> Effect { mset: () \| r } (MSet k)` | Difference a \\ b (returns new set) |

**Named capability variants** (use `@#label` to parameterize the effect slot):

| Name             | Type                                                                                | Description                         |
| ---------------- | ----------------------------------------------------------------------------------- | ----------------------------------- |
| `newAt`          | `\(l: Label) k r. (k -> k -> Ordering) -> Effect { l: () \| r } (MSet k)`           | Create set (explicit compare)       |
| `insertAt`       | `\(l: Label) k r. k -> MSet k -> Effect { l: () \| r } ()`                          | Insert element                      |
| `memberAt`       | `\(l: Label) k r. k -> MSet k -> Effect { l: () \| r } Bool`                        | Membership test                     |
| `deleteAt`       | `\(l: Label) k r. k -> MSet k -> Effect { l: () \| r } ()`                          | Remove element                      |
| `sizeAt`         | `\(l: Label) k r. MSet k -> Effect { l: () \| r } Int`                              | Number of elements                  |
| `toListAt`       | `\(l: Label) k r. MSet k -> Effect { l: () \| r } (List k)`                         | Sorted element list                 |
| `fromListAt`     | `\(l: Label) k r. (k -> k -> Ordering) -> List k -> Effect { l: () \| r } (MSet k)` | Build from list (explicit compare)  |
| `foldAt`         | `\(l: Label) k b r. (b -> k -> b) -> b -> MSet k -> Effect { l: () \| r } b`        | Left fold over elements             |
| `unionAt`        | `\(l: Label) k r. MSet k -> MSet k -> Effect { l: () \| r } (MSet k)`               | Union (returns new set)             |
| `intersectionAt` | `\(l: Label) k r. MSet k -> MSet k -> Effect { l: () \| r } (MSet k)`               | Intersection (returns new set)      |
| `differenceAt`   | `\(l: Label) k r. MSet k -> MSet k -> Effect { l: () \| r } (MSet k)`               | Difference a \\ b (returns new set) |

**Notes:**

- Backed by `MMap k ()` internally, mirroring how `Data.Set` backs on `Data.Map`.
- All operations are effectful.
- `union`, `intersection`, and `difference` return new sets; operands are unchanged.
- Named `newAt`/`fromListAt` take an explicit `(k -> k -> Ordering)` compare function instead of `Ord k =>`.

> **Tip:** Use qualified imports: `import Effect.Set as MSet`.
