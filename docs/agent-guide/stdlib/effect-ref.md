### Effect.Ref

Provides mutable reference cells gated by the `{ ref: () }` effect. Load with `eng.Use(gicel.EffectRef)` and import with `import Effect.Ref`.

**Functions:**

| Name     | Type                                                                                 | Description                   |
| -------- | ------------------------------------------------------------------------------------ | ----------------------------- |
| `new`    | `\a (r: Row). a -> Computation { ref: () \| r } { ref: () \| r } (Ref a)`            | Create a new reference cell   |
| `read`   | `\a (r: Row). Ref a -> Computation { ref: () \| r } { ref: () \| r } a`              | Read the current value        |
| `write`  | `\a (r: Row). a -> Ref a -> Computation { ref: () \| r } { ref: () \| r } ()`        | Replace the value             |
| `modify` | `\a (r: Row). (a -> a) -> Ref a -> Computation { ref: () \| r } { ref: () \| r } ()` | Apply a function to the value |

**Notes:**

- References are mutable cells backed by in-place Go mutation.
- Multiple independent references can coexist in the same computation (unlike `Effect.State` which provides a single cell).
- The `Ref a` type is opaque; the only operations are `new`, `read`, `write`, and `modify`.

**Example:**

```
import Prelude
import Effect.Ref

main := do {
  counter <- new 0;
  modify (\n. n + 1) counter;
  modify (\n. n + 1) counter;
  read counter
}
-- Result: 2
```
