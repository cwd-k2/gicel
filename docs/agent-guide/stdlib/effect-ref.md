### Effect.Ref

Provides mutable reference cells gated by the `{ ref: () }` effect. Load with `eng.Use(gicel.EffectRef)` and import with `import Effect.Ref`.

**Functions:**

| Name     | Type                                                           | Description                   |
| -------- | -------------------------------------------------------------- | ----------------------------- |
| `new`    | `\a (r: Row). a -> Effect { ref: () \| r } (Ref a)`            | Create a new reference cell   |
| `read`   | `\a (r: Row). Ref a -> Effect { ref: () \| r } a`              | Read the current value        |
| `write`  | `\a (r: Row). a -> Ref a -> Effect { ref: () \| r } ()`        | Replace the value             |
| `modify` | `\a (r: Row). (a -> a) -> Ref a -> Effect { ref: () \| r } ()` | Apply a function to the value |

**Named capability variants** (use `@#label` to parameterize the effect slot):

| Name       | Type                                                                    | Description                   |
| ---------- | ----------------------------------------------------------------------- | ----------------------------- |
| `newAt`    | `\(l: Label) a (r: Row). a -> Effect { l: () \| r } (Ref a)`            | Create ref in named slot      |
| `readAt`   | `\(l: Label) a (r: Row). Ref a -> Effect { l: () \| r } a`              | Read the current value        |
| `writeAt`  | `\(l: Label) a (r: Row). a -> Ref a -> Effect { l: () \| r } ()`        | Replace the value             |
| `modifyAt` | `\(l: Label) a (r: Row). (a -> a) -> Ref a -> Effect { l: () \| r } ()` | Apply a function to the value |

**Notes:**

- References are mutable cells backed by in-place Go mutation.
- Multiple independent references can coexist in the same computation (unlike `Effect.State` which provides a single cell).
- The `Ref a` type is opaque; the only operations are `new`, `read`, `write`, `modify` and their `*At` variants.
- Named variants require `@#label` (e.g., `newAt @#cell 0`).

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
