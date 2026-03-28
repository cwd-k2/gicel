### Effect.Array

Provides mutable fixed-size arrays with O(1) read/write, gated by the `{ array: () }` effect. Load with `eng.Use(gicel.EffectArray)` and import with `import Effect.Array`.

**Functions:**

| Name        | Type                                                               | Description                       |
| ----------- | ------------------------------------------------------------------ | --------------------------------- |
| `new`       | `\a r. Int -> a -> Effect { array: () \| r } (Array a)`            | Create array of size n with fill  |
| `readAt`    | `\a r. Int -> Array a -> Effect { array: () \| r } (Maybe a)`      | Read element at index             |
| `writeAt`   | `\a r. Int -> a -> Array a -> Effect { array: () \| r } ()`        | Write element at index (in-place) |
| `size`      | `\a. Array a -> Int`                                               | Array length (pure)               |
| `resize`    | `\a r. Int -> a -> Array a -> Effect { array: () \| r } (Array a)` | Resize with fill value            |
| `toSlice`   | `\a r. Array a -> Effect { array: () \| r } (Slice a)`             | Snapshot as immutable Slice       |
| `fromSlice` | `\a r. Slice a -> Effect { array: () \| r } (Array a)`             | Create from Slice                 |

**Notes:**

- Mutation is in-place. The `Array` value is a mutable reference.
- `size` is pure (no effect annotation needed).
- Out-of-bounds reads return `Nothing`; out-of-bounds writes are no-ops.

**Example:**

```
import Prelude
import Effect.Array

main := do {
  arr <- new 3 0;            -- [0, 0, 0]
  writeAt 1 42 arr;          -- [0, 42, 0]
  v <- readAt 1 arr;         -- Just 42
  pure v
}
```
