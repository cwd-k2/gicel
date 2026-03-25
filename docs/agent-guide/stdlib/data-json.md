### Data.JSON

Provides `ToJSON`/`FromJSON` type classes for JSON encoding and decoding. Load with `eng.Use(gicel.DataJSON)` and import with `import Data.JSON`.

**Type classes:**

```
form ToJSON   := \a. { toJSON:   a -> String }
form FromJSON := \a. { fromJSON: String -> Maybe a }
```

**Instances:**

| Type         | ToJSON | FromJSON | JSON representation                              |
| ------------ | ------ | -------- | ------------------------------------------------ |
| `Int`        | yes    | yes      | number (integer)                                 |
| `Double`     | yes    | yes      | number (NaN/Inf → `null`)                        |
| `String`     | yes    | yes      | quoted string with JSON escaping                 |
| `Bool`       | yes    | yes      | `true` / `false`                                 |
| `()`         | yes    | --       | `null`                                           |
| `List a`     | yes    | yes      | JSON array                                       |
| `Maybe a`    | yes    | yes      | `null` for Nothing, value for Just               |
| `(a, b)`     | yes    | yes      | two-element JSON array                           |
| `Result e a` | yes    | yes      | `{"tag":"Ok","value":...}` / `{"tag":"Err",...}` |

**Notes:**

- Compound instances compose automatically via dictionary passing. `toJSON [Just 1, Nothing]` produces `[1,null]`.
- `FromJSON (Maybe a)`: `"null"` decodes to `Just Nothing`, a valid value to `Just (Just x)`, invalid JSON to `Nothing`.
- `FromJSON` returns `Nothing` on parse failure; it never throws.
- For record types or custom ADTs, provide instances via host-side `assumption` primitives.

**Example:**

```
import Prelude
import Data.JSON

main := {
  encoded := toJSON [1, 2, 3];
  decoded := fromJSON encoded :: Maybe (List Int);
  roundtrip := toJSON (Ok "hello" :: Result String String);
  (encoded, decoded, roundtrip)
}
-- Result: ("[1,2,3]", Just [1, 2, 3], "{\"tag\":\"Ok\",\"value\":\"hello\"}")
```
