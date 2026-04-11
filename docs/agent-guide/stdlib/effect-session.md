### Effect.Session

Provides session type primitives for protocol-safe communication via capability tracking. Load with `eng.Use(gicel.EffectSession)` and import with `import Effect.Session`. Requires `Prelude` and `Effect.State`.

**Session State Constructors** (defined in the module):

| Type       | Kind          | Description                         |
| ---------- | ------------- | ----------------------------------- |
| `Send s`   | `Type → Type` | Peer expects to receive, then `s`   |
| `Recv s`   | `Type → Type` | Peer expects to send, then `s`      |
| `End`      | `Type`        | Protocol complete                   |
| `Offer r`  | `Row → Type`  | External choice (peer selects)      |
| `Choose r` | `Row → Type`  | Internal choice (this side selects) |

**Type Families:**

| Name      | Kind          | Description                                |
| --------- | ------------- | ------------------------------------------ |
| `Dual`    | `Type → Type` | Compute peer's view of a protocol          |
| `DualRow` | `Row → Row`   | Map `Dual` over each field (`MapRow Dual`) |

`Dual` satisfies the involution property: `Dual (Dual S) ≡ S` for all closed protocol states.

**Operations:**

| Name        | Type                                                                                                                  | Description                         |
| ----------- | --------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| `closeAt`   | `\(l: Label) s r. Computation { l: s \| r } r ()`                                                                     | Close a session capability          |
| `chooseAt`  | `\(l: Label) (tag: Label) (choices: Row) r. Computation { l: Choose choices \| r } { l: Lookup tag choices \| r } ()` | Select a branch (internal choice)   |
| `receiveAt` | `\(l: Label) (choices: Row) (s: Type) r. Computation { l: Offer choices \| r } { l: s \| r } (Variant choices s)`     | Receive a choice (external choice)  |
| `inject`    | `\(tag: Label) (choices: Row). Lookup tag choices -> Variant choices (Lookup tag choices)`                            | Create a Variant value from a label |

**Handler:**

| Name           | Type                                                                 | Description                                |
| -------------- | -------------------------------------------------------------------- | ------------------------------------------ |
| `runSessionAt` | `\(l: Label) s a r. s -> Thunk Zero { l: s \| r } r a -> Effect r a` | Introduce a session capability, run, close |

**Example — ATM Protocol:**

```
import Prelude
import Effect.State
import Effect.Session

type ATM := Choose { balance: Recv End, deposit: Send (Recv End), quit: End }

-- Client chooses "balance", receives result, closes.
client :: Computation { ch: ATM } {} Int
client := do {
  chooseAt @#ch @#balance;
  n <- getAt @#ch;
  closeAt @#ch;
  pure n
}
```

**Example — External Choice (Offer):**

```
type Server := Offer { ping: Send End, quit: End }

handler :: Computation { srv: Server } {} Int
handler := do {
  tag <- receiveAt @#srv;
  case tag {
    #ping => do { putAt @#srv (MkSend (MkEnd)); closeAt @#srv; pure 42 };
    #quit => do { closeAt @#srv; pure 0 }
  }
}
```

**Notes:**

- `receiveAt` returns `Variant choices s` where `s` is a shared type index. The `case` expression with `#label` patterns refines `s` per branch.
- `chooseAt` uses `Lookup` to resolve the selected branch's continuation type.
- Session operations have nil grades in their type signatures, making them compatible with `@Linear` capabilities via the nil-as-identity rule.
- `runSessionAt` introduces the capability with an initial state value and removes it after the body completes.
