## Session Types

Session types encode communication protocols as types. GICEL expresses them via the Atkey indexed monad without special syntax -- protocol states are regular type constructors, and `@Linear` annotations enforce usage discipline.

### Protocol States

```
data Send := \s. { MkSend: Send s }   -- send a value, continue as s
data Recv := \s. { MkRecv: Recv s }   -- receive a value, continue as s
data End  := MkEnd                     -- session complete
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

### Operations

```
send  :: \s. Computation { ch: Send s @Linear } { ch: s @Linear } T
recv  :: \s. Computation { ch: Recv s @Linear } { ch: s @Linear } T
close :: Computation { ch: End @Linear } {} ()
```

Each operation advances the protocol state. The pre/post indices enforce the correct sequence.

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

See the language specification (Chapters 17-18) for the formal session fidelity theorem and proof structure.
