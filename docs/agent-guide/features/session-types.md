## Session Types

Session types encode communication protocols as types. GICEL expresses them via the Atkey indexed monad without special syntax -- protocol states are regular type constructors, and `@Linear` annotations enforce usage discipline.

### Protocol States

```
form Send := \s. { MkSend: Send s }   -- send a value, continue as s
form Recv := \s. { MkRecv: Recv s }   -- receive a value, continue as s
form End  := MkEnd                     -- session complete
```

### Duality

The `Dual` type family computes the complementary protocol:

```
type Dual :: Type := \(s: Type). case s {
  Send s => Recv (Dual s);
  Recv s => Send (Dual s);
  End    => End
}
```

`Dual (Dual S)` reduces to `S` for all closed protocol states.

### Branching (Offer / Choose)

Session protocols support branching. `Choose` (internal choice) lets this side select a branch; `Offer` (external choice) lets the peer select:

```
form Offer  := \(choices: Row). { MkOffer: Offer choices }
form Choose := \(choices: Row). { MkChoose: Choose choices }
```

Duality maps `Offer` ↔ `Choose` with `MapRow Dual` applied to each branch:

```
type DualRow :: Row := \(r: Row). MapRow Dual r
-- Dual (Offer { a: Send End }) = Choose { a: Recv End }
```

### Variant Type

`Variant :: Row -> Type -> Type` is the labeled coproduct dual of `Record`. It represents a value tagged with one label from a row. Used by `receiveAt` to dispatch on externally chosen branches:

```
-- receiveAt returns Variant choices s; case with #label patterns dispatches:
tag <- receiveAt @#ch;
case tag {
  #ping => do { ... };
  #quit => do { ... }
}
```

The `inject` function creates a `Variant` value: `inject @#tag value`.

### Operations

The `Effect.Session` stdlib pack provides session lifecycle operations. See the [stdlib reference](../stdlib/effect-session.md) for full type signatures.

| Operation      | Description                                           |
| -------------- | ----------------------------------------------------- |
| `closeAt`      | Close a session capability, removing it from the row  |
| `chooseAt`     | Select a branch (internal choice)                     |
| `receiveAt`    | Receive a choice (external choice), returns `Variant` |
| `inject`       | Create a `Variant` value from a label and payload     |
| `runSessionAt` | Introduce a session capability, run, close            |

The actual send/recv transport for message-passing must be implemented by the host via `RegisterPrim`.

### Multiplicity Annotations

Row fields accept `@Linear`, `@Affine`, or `@Unrestricted` (default):

```
open  :: Computation {} { h: Handle @Linear } ()
close :: Computation { h: Handle @Linear } {} ()
```

- `@Linear` -- must be consumed exactly once
- `@Affine` -- may be consumed at most once
- `@Unrestricted` -- no usage constraint (default)

### Branch Joins

When case branches produce different post-states, the `LUB` type family computes the joined post-state field-wise:

```
case cond {
  True  => consume handle;    -- post: {}
  False => pure ()            -- post: { h: Handle @Linear }
}
-- joined post-state: LUB applied field-wise
```

### Session Fidelity

The type system guarantees:

1. **Protocol compliance** -- each operation advances the state according to the type family rules.
2. **Communication safety** -- type parameters match the protocol specification.
3. **Session completion** -- if a channel is absent from the post-state, it reached `End` and was closed.
