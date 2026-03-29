### Effect.State

Provides get/put state capabilities via the `state` capability in CapEnv. Load with `eng.Use(gicel.EffectState)` and import with `import Effect.State`.

**Functions:**

| Name       | Type                                                                      | Description                     |
| ---------- | ------------------------------------------------------------------------- | ------------------------------- |
| `get`      | `\s r. Effect { state: s \| r } s`                                        | Read current state              |
| `put`      | `\s1 s2 r. s2 -> Computation { state: s1 \| r } { state: s2 \| r } ()`    | Replace current state           |
| `modify`   | `\s r. (s -> s) -> Effect { state: s \| r } ()`                           | Apply a function to state       |
| `getAt`    | `\(l: Label) s r. Effect { l: s \| r } s`                                 | Read named state                |
| `putAt`    | `\(l: Label) s1 s2 r. s2 -> Computation { l: s1 \| r } { l: s2 \| r } ()` | Replace named state             |
| `modifyAt` | `\(l: Label) s r. (s -> s) -> Effect { l: s \| r } ()`                    | Apply a function to named state |

**Notes:**

- `put` and `putAt` use `Computation pre post` (not `Effect`) because they can change the state type.
- Host provides `"state"` capability (or named label via `*At` variants). Final state is in `result.CapEnv`.

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
