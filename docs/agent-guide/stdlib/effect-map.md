### Effect.Map

Provides mutable ordered maps backed by AVL trees, gated by the `{ mmap: () }` effect. Load with `eng.Use(gicel.EffectMap)` and import with `import Effect.Map`.

**Functions:**

| Name           | Type                                                                                                 | Description                    |
| -------------- | ---------------------------------------------------------------------------------------------------- | ------------------------------ |
| `new`          | `\k v r. Ord k => Computation { mmap: () \| r } { mmap: () \| r } (MMap k v)`                        | Create empty mutable map       |
| `insert`       | `\k v r. Ord k => k -> v -> MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } ()`          | Insert or overwrite (in-place) |
| `lookup`       | `\k v r. Ord k => k -> MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } (Maybe v)`        | Lookup by key                  |
| `delete`       | `\k v r. Ord k => k -> MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } ()`               | Remove a key (in-place)        |
| `size`         | `\k v. MMap k v -> Int`                                                                              | Number of entries (pure)       |
| `member`       | `\k v r. Ord k => k -> MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } Bool`             | Key membership test            |
| `toList`       | `\k v r. MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } (List (k, v))`                  | In-order key-value pairs       |
| `fromList`     | `\k v r. Ord k => List (k, v) -> Computation { mmap: () \| r } { mmap: () \| r } (MMap k v)`         | Build from pairs               |
| `foldlWithKey` | `\k v b r. (b -> k -> v -> b) -> b -> MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } b` | Left fold with key and value   |
| `keys`         | `\k v r. MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } (List k)`                       | All keys in sorted order       |
| `values`       | `\k v r. MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } (List v)`                       | All values in key order        |
| `adjust`       | `\k v r. Ord k => k -> (v -> v) -> MMap k v -> Computation { mmap: () \| r } { mmap: () \| r } ()`   | Apply function to value at key |

**Notes:**

- Mutation is in-place. The `MMap` value is a mutable reference shared by all aliases.
- `size` is pure. All other operations are effectful (result depends on mutable state).
- Uses the same AVL tree infrastructure as `Data.Map`; the difference is in-place root mutation.

> **Tip:** Use qualified imports: `import Effect.Map as MMap`.
