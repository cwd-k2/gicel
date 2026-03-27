### Effect.Set

Provides mutable ordered sets backed by AVL trees (internally MMap k ()), gated by the `{ mset: () }` effect. Load with `eng.Use(gicel.EffectSet)` and import with `import Effect.Set`.

**Functions:**

| Name       | Type                                                                                        | Description               |
| ---------- | ------------------------------------------------------------------------------------------- | ------------------------- |
| `new`      | `\k r. Ord k => Computation { mset: () \| r } { mset: () \| r } (MSet k)`                   | Create empty mutable set  |
| `insert`   | `\k r. Ord k => k -> MSet k -> Computation { mset: () \| r } { mset: () \| r } ()`          | Insert element (in-place) |
| `member`   | `\k r. Ord k => k -> MSet k -> Computation { mset: () \| r } { mset: () \| r } Bool`        | Membership test           |
| `delete`   | `\k r. Ord k => k -> MSet k -> Computation { mset: () \| r } { mset: () \| r } ()`          | Remove element (in-place) |
| `size`     | `\k r. MSet k -> Computation { mset: () \| r } { mset: () \| r } Int`                       | Number of elements        |
| `toList`   | `\k r. MSet k -> Computation { mset: () \| r } { mset: () \| r } (List k)`                  | Sorted element list       |
| `fromList` | `\k r. Ord k => List k -> Computation { mset: () \| r } { mset: () \| r } (MSet k)`         | Build from list           |
| `fold`         | `\k b r. (b -> k -> b) -> b -> MSet k -> Computation { mset: () \| r } { mset: () \| r } b`         | Left fold over elements            |
| `union`        | `\k r. Ord k => MSet k -> MSet k -> Computation { mset: () \| r } { mset: () \| r } (MSet k)`       | Union (mutates first set in-place) |
| `intersection` | `\k r. Ord k => MSet k -> MSet k -> Computation { mset: () \| r } { mset: () \| r } (MSet k)`       | Intersection (returns new set)     |
| `difference`   | `\k r. Ord k => MSet k -> MSet k -> Computation { mset: () \| r } { mset: () \| r } (MSet k)`       | Difference a \\ b (returns new set) |

**Notes:**

- Backed by `MMap k ()` internally, mirroring how `Data.Set` backs on `Data.Map`.
- All operations are effectful.
- `union` mutates the first set in-place; `intersection` and `difference` return new sets.

> **Tip:** Use qualified imports: `import Effect.Set as MSet`.
