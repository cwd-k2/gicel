## 7. Stdlib Reference

Stdlib packs are loaded on the host side via `eng.Use(pack)` and imported in source. Core is auto-registered and auto-imported; the user cannot control it. Prelude requires explicit `eng.Use(gicel.Prelude)` on the engine and `import Prelude` in source. `NewEngine()` returns a bare engine with only Core.

| CLI name  | Module name    | Doc                                | Notes                  |
| --------- | -------------- | ---------------------------------- | ---------------------- |
| `prelude` | `Prelude`      | [prelude.md](prelude.md)           |                        |
| `fail`    | `Effect.Fail`  | [effect-fail.md](effect-fail.md)   |                        |
| `state`   | `Effect.State` | [effect-state.md](effect-state.md) |                        |
| `io`      | `Effect.IO`    | [effect-io.md](effect-io.md)       |                        |
| `array`   | `Effect.Array` | [effect-array.md](effect-array.md) |                        |
| `mmap`    | `Effect.Map`   | [effect-map.md](effect-map.md)     |                        |
| `mset`    | `Effect.Set`   | [effect-set.md](effect-set.md)     |                        |
| `stream`  | `Data.Stream`  | [data-stream.md](data-stream.md)   | requires `--recursion` |
| `slice`   | `Data.Slice`   | [data-slice.md](data-slice.md)     |                        |
| `map`     | `Data.Map`     | [data-map.md](data-map.md)         |                        |
| `set`     | `Data.Set`     | [data-set.md](data-set.md)         |                        |

See also: [functions.md](functions.md) for Prelude combinators and operators.
