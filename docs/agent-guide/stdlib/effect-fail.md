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

`fail`/`failWith` abort the computation. No catch/recover at language level; the host handles errors.
