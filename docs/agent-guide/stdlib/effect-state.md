### Effect.State

Provides get/put state capabilities via the `state` capability in CapEnv. Load with `eng.Use(gicel.EffectState)` and import with `import Effect.State`.

**Operations:**

| Name       | Type                                                                      | Description                     |
| ---------- | ------------------------------------------------------------------------- | ------------------------------- |
| `get`      | `\s r. Effect { state: s \| r } s`                                        | Read current state              |
| `put`      | `\s1 s2 r. s2 -> Computation { state: s1 \| r } { state: s2 \| r } ()`    | Replace current state           |
| `modify`   | `\s r. (s -> s) -> Effect { state: s \| r } ()`                           | Apply a function to state       |
| `getAt`    | `\(l: Label) s r. Effect { l: s \| r } s`                                 | Read named state                |
| `putAt`    | `\(l: Label) s1 s2 r. s2 -> Computation { l: s1 \| r } { l: s2 \| r } ()` | Replace named state             |
| `modifyAt` | `\(l: Label) s r. (s -> s) -> Effect { l: s \| r } ()`                    | Apply a function to named state |

**Handlers:**

Handlers introduce the state capability with an initial value, run a suspended computation, and eliminate the capability from the row. Same pattern as `try` in Effect.Fail.

| Name        | Type                                                            | Description                            |
| ----------- | --------------------------------------------------------------- | -------------------------------------- |
| `runState`  | `\s a r. s -> Suspended { state: s \| r } a -> Effect r (s, a)` | Run, return (finalState, result) pair  |
| `evalState` | `\s a r. s -> Suspended { state: s \| r } a -> Effect r a`      | Run, return result only                |
| `execState` | `\s a r. s -> Suspended { state: s \| r } a -> Effect r s`      | Run, return final state only           |
| `*StateAt`  | —                                                               | Pending (label erasure VM interaction) |

**Notes:**

- `put` and `putAt` use `Computation pre post` (not `Effect`) because they can change the state type.
- Handlers require the inner computation to preserve the state type (`Suspended { state: s | r }` — same `s` in pre and post row).
- Host can also provide `"state"` capability via `RunOptions.Caps`, but handlers are the idiomatic approach.

**Example:**

```
import Prelude
import Effect.State

-- evalState introduces state with initial value — no Caps needed
main := evalState 0 (thunk do {
  modify (+ 5);
  modify (* 2);
  get              -- 10
})
```

**Handler example — returning both state and result:**

```
import Prelude
import Effect.State

main := do {
  (finalState, result) <- runState 100 (thunk do {
    modify (+ 50);
    x <- get;
    pure (x * 2)
  });
  pure finalState  -- 150
}
```
