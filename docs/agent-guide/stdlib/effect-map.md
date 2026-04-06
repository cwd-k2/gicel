### Effect.Map

Provides mutable ordered maps backed by AVL trees, gated by the `{ mmap: () }` effect. Load with `eng.Use(gicel.EffectMap)` and import with `import Effect.Map`.

**Functions:**

| Name           | Type                                                                          | Description                    |
| -------------- | ----------------------------------------------------------------------------- | ------------------------------ |
| `new`          | `\k v r. Ord k => Effect { mmap: () \| r } (MMap k v)`                        | Create empty mutable map       |
| `insert`       | `\k v r. Ord k => k -> v -> MMap k v -> Effect { mmap: () \| r } ()`          | Insert or overwrite (in-place) |
| `lookup`       | `\k v r. Ord k => k -> MMap k v -> Effect { mmap: () \| r } (Maybe v)`        | Lookup by key                  |
| `delete`       | `\k v r. Ord k => k -> MMap k v -> Effect { mmap: () \| r } ()`               | Remove a key (in-place)        |
| `size`         | `\k v r. MMap k v -> Effect { mmap: () \| r } Int`                            | Number of entries              |
| `member`       | `\k v r. Ord k => k -> MMap k v -> Effect { mmap: () \| r } Bool`             | Key membership test            |
| `toList`       | `\k v r. MMap k v -> Effect { mmap: () \| r } (List (k, v))`                  | In-order key-value pairs       |
| `fromList`     | `\k v r. Ord k => List (k, v) -> Effect { mmap: () \| r } (MMap k v)`         | Build from pairs               |
| `foldlWithKey` | `\k v b r. (b -> k -> v -> b) -> b -> MMap k v -> Effect { mmap: () \| r } b` | Left fold with key and value   |
| `keys`         | `\k v r. MMap k v -> Effect { mmap: () \| r } (List k)`                       | All keys in sorted order       |
| `values`       | `\k v r. MMap k v -> Effect { mmap: () \| r } (List v)`                       | All values in key order        |
| `adjust`       | `\k v r. Ord k => k -> (v -> v) -> MMap k v -> Effect { mmap: () \| r } ()`   | Apply function to value at key |

**Named capability variants** (use `@#label` to parameterize the effect slot):

| Name             | Type                                                                                         | Description                         |
| ---------------- | -------------------------------------------------------------------------------------------- | ----------------------------------- |
| `newAt`          | `\(l: Label) k v r. (k -> k -> Ordering) -> Effect { l: () \| r } (MMap k v)`                | Create map (explicit compare)       |
| `insertAt`       | `\(l: Label) k v r. k -> v -> MMap k v -> Effect { l: () \| r } ()`                          | Insert or overwrite                 |
| `lookupAt`       | `\(l: Label) k v r. k -> MMap k v -> Effect { l: () \| r } (Maybe v)`                        | Lookup by key                       |
| `deleteAt`       | `\(l: Label) k v r. k -> MMap k v -> Effect { l: () \| r } ()`                               | Remove a key                        |
| `sizeAt`         | `\(l: Label) k v r. MMap k v -> Effect { l: () \| r } Int`                                   | Number of entries                   |
| `memberAt`       | `\(l: Label) k v r. k -> MMap k v -> Effect { l: () \| r } Bool`                             | Key membership test                 |
| `toListAt`       | `\(l: Label) k v r. MMap k v -> Effect { l: () \| r } (List (k, v))`                         | In-order key-value pairs            |
| `fromListAt`     | `\(l: Label) k v r. (k -> k -> Ordering) -> List (k, v) -> Effect { l: () \| r } (MMap k v)` | Build from pairs (explicit compare) |
| `foldlWithKeyAt` | `\(l: Label) k v b r. (b -> k -> v -> b) -> b -> MMap k v -> Effect { l: () \| r } b`        | Left fold with key and value        |
| `keysAt`         | `\(l: Label) k v r. MMap k v -> Effect { l: () \| r } (List k)`                              | All keys in sorted order            |
| `valuesAt`       | `\(l: Label) k v r. MMap k v -> Effect { l: () \| r } (List v)`                              | All values in key order             |
| `adjustAt`       | `\(l: Label) k v r. k -> (v -> v) -> MMap k v -> Effect { l: () \| r } ()`                   | Apply function to value at key      |

**Notes:**

- Mutation is in-place. The `MMap` value is a mutable reference shared by all aliases.
- All operations are effectful (result depends on mutable state).
- Uses the same AVL tree infrastructure as `Data.Map`; the difference is in-place root mutation.
- Named `newAt`/`fromListAt` take an explicit `(k -> k -> Ordering)` compare function instead of `Ord k =>` (the compare is stored in the handle at creation time).

**Example:**

```gicel
import Prelude
import Effect.Map as MMap

main := do {
  m <- MMap.new @Int @String;
  MMap.insert 1 "one" m;
  MMap.insert 2 "two" m;
  v <- MMap.lookup 1 m;
  pure v
}
-- Just "one"
```

> **Tip:** Use qualified imports: `import Effect.Map as MMap`.
