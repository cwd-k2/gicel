### Effect.Fail

Provides failure/error effects via the `fail` capability. Load with `eng.Use(gicel.EffectFail)` and import with `import Effect.Fail`.

**Functions:**

| Name         | Type                                              | Description                      |
| ------------ | ------------------------------------------------- | -------------------------------- |
| `failWith`   | `\e r a. e -> Effect { fail: e \| r } a`          | Fail with a typed error value    |
| `fail`       | `\r a. Effect { fail: () \| r } a`                | Fail with () (no error payload)  |
| `fromMaybe`  | `\a r. Maybe a -> Effect { fail: () \| r } a`     | Extract Just or fail on Nothing  |
| `fromResult` | `\e a r. Result e a -> Effect { fail: e \| r } a` | Extract Ok or failWith on Err    |
| `failWithAt` | `\(l: Label) e r a. e -> Effect { l: e \| r } a`  | Fail with named error capability |

`fail`/`failWith` abort the computation. Use `try`/`tryAt` to catch failures and convert to `Result`.

**Handlers:**

| Name    | Type                                                                    | Description                        |
| ------- | ----------------------------------------------------------------------- | ---------------------------------- |
| `try`   | `\e a r. Suspended { fail: e \| r } a -> Effect r (Result e a)`         | Catch anonymous fail → Result      |
| `tryAt` | `\(l: Label) e a r. Suspended { l: e \| r } a -> Effect r (Result e a)` | Catch named fail at label → Result |

`try` catches anonymous `failWith`; `tryAt @#label` catches only `failWithAt @#label`. Non-matching errors propagate to outer handlers, enabling nested error handling with independent channels.

**Example — basic try:**

```gicel
import Prelude
import Effect.Fail

safeDivide :: \(r: Row). Int -> Int -> Suspended { fail: String | r } Int
safeDivide := \a b.
  if b == 0
    then failWith "division by zero"
    else pure (a / b)

main := do {
  ok <- try (safeDivide 10 2);   -- Ok 5
  err <- try (safeDivide 10 0);  -- Err "division by zero"
  pure (ok, err)
}
-- (Ok 5, Err ("division by zero"))
```

**Example — nested error handling with `tryAt`:**

```gicel
import Prelude
import Effect.Fail

main := do {
  outer <- tryAt @#outer (do {
    inner <- tryAt @#inner (do {
      failWithAt @#inner "inner error"
    });
    pure (show inner)
  });
  pure outer
}
-- Ok "Err (\"inner error\")"
```

Named fail channels (`failWithAt @#label` + `tryAt @#label`) solve the nested `try` limitation: anonymous `try` cannot nest because both introduce a `fail` label in the row, but named channels use distinct labels and compose freely.
