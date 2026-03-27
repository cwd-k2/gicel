### Effect.State

Provides get/put state capabilities via the `state` capability in CapEnv. Load with `eng.Use(gicel.EffectState)` and import with `import Effect.State`.

**Functions:**

| Name     | Type                                                                   | Description               |
| -------- | ---------------------------------------------------------------------- | ------------------------- |
| `get`    | `\s r. Computation { state: s \| r } { state: s \| r } s`              | Read current state        |
| `put`    | `\s r. s -> Computation { state: s \| r } { state: s \| r } ()`        | Replace current state     |
| `modify` | `\s r. (s -> s) -> Computation { state: s \| r } { state: s \| r } ()` | Apply a function to state |

Host provides `"state"` capability. Final state is in `result.CapEnv`.

**Example:**

```
import Prelude
import Effect.State

main := do {
  put 0;
  modify (+ 5);
  modify (* 2);
  get              -- 10
}
```
