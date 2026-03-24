### Effect.Array

Provides mutable fixed-size arrays with O(1) read/write, gated by the `{ array: () }` effect. Load with `eng.Use(gicel.EffectArray)` and import with `import Effect.Array`.

**Functions:**

| Name        | Type                                                                                       | Description                       |
| ----------- | ------------------------------------------------------------------------------------------ | --------------------------------- |
| `new`       | `\a r. Int -> a -> Computation { array: () \| r } { array: () \| r } (Array a)`            | Create array of size n with fill  |
| `readAt`    | `\a r. Int -> Array a -> Computation { array: () \| r } { array: () \| r } (Maybe a)`      | Read element at index             |
| `writeAt`   | `\a r. Int -> a -> Array a -> Computation { array: () \| r } { array: () \| r } ()`        | Write element at index (in-place) |
| `size`      | `\a. Array a -> Int`                                                                       | Array length (pure)               |
| `resize`    | `\a r. Int -> a -> Array a -> Computation { array: () \| r } { array: () \| r } (Array a)` | Resize with fill value            |
| `toSlice`   | `\a r. Array a -> Computation { array: () \| r } { array: () \| r } (Slice a)`             | Snapshot as immutable Slice       |
| `fromSlice` | `\a r. Slice a -> Computation { array: () \| r } { array: () \| r } (Array a)`             | Create from Slice                 |

**Notes:**

- Mutation is in-place. The `Array` value is a mutable reference.
- `size` is pure (no effect annotation needed).
- Out-of-bounds reads return `Nothing`; out-of-bounds writes are no-ops.
