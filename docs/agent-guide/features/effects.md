## 5. Effect System

### Computation pre post a

The core abstraction is `Computation pre post a` -- an Atkey-style parameterized monad (indexed monad). It represents an effectful computation that:

- Requires capability environment `pre` (a row type) at the start
- Produces capability environment `post` (a row type) at the end
- Returns a value of type `a`

When `pre` and `post` are the same, the computation preserves its environment. The type alias `Effect r a := Computation r r a` is provided for this common case.

### pure and bind

```
pure :: \a (r: Row). a -> Computation r r a
bind :: \a b (r1: Row) (r2: Row) (r3: Row).
          Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

These are built-in -- always available without import. Note how `bind` composes pre/post indices: `r1->r2` then `r2->r3` yields `r1->r3`.

### Do-notation

```
do {
  x <- getState;               -- bind: extract value from Computation
  putState (x + 1);            -- bare expression: sequence, discard result
  y := x + 1;                  -- let: pure binding (no effect)
  pure y                       -- return result
}
```

Desugaring:

```
bind getState (\x.
  bind (putState (x + 1)) (\_.
    (\y. pure y) (x + 1)))
```

### Capability Environments

Capability environments are row types that describe what effects are available:

```
{}                                            -- no capabilities (pure)
{ state: Int }                                -- state holding an Int
{ state: Int, fail: String }                  -- state and failure
{ io: () | r }                                -- io plus whatever else r contains
```

A function requiring state:

```
increment := \(). do {
  n <- get;
  put (n + 1);
  pure n
}
```

A `Computation` is an action awaiting execution — it cannot be a bare top-level binding (only the entry point `main` is exempt). To name a standalone computation, suspend it with `thunk` and `force` it at the call site:

```
counter :: Suspended { state: Int } Int
counter := thunk do {
  n <- get;
  put (n + 1);
  pure n
}

main := do {
  result <- force counter;
  pure result
}
```

This restriction applies only to `Computation`. Value-typed monads (`List`, `Maybe`, custom `Monad` instances) also support `do`-notation when the binding has a type annotation:

```
pairs :: List (Int, Int)
pairs := do { x <- [1,2,3]; y <- [10,20]; pure (x, y) }
```

The type annotation is required because without it, `do` defaults to `Computation` inference. With the annotation, the checker dispatches to `mbind`/`mpure` from the `Monad` instance:

```
form Writer := \a. { MkWriter: (a, List String) -> Writer a }
-- (Monad instance omitted for brevity)

tell :: String -> Writer ()
tell := \s. MkWriter ((), [s])

program :: Writer String          -- annotation selects Monad dispatch
program := do {
  tell "hello";
  tell "world";
  pure "done"
}
```

Note: inside value-monad `do` blocks, both `pure` and `mpure` work. `pure` is the universal return (works in both `Computation` and value-monad contexts); `mpure` is an alias provided by `Monad` instances.

CapEnv is copy-on-write: effects thread through Computation indices. `put` does not mutate; it produces a new CapEnv.

### thunk and force

`thunk` suspends a computation into a first-class value (CBPV's U):

```
thunk :: Computation pre post a -> Thunk pre post a
```

`force` runs a suspended computation:

```
force :: Thunk pre post a -> Computation pre post a
```

The type alias `Suspended r a := Thunk r r a` mirrors `Effect` for suspended computations that preserve their capability state.

### Named Capabilities

Named capabilities allow multiple independent instances of the same effect type to coexist. Use `@#name` in type application position to specify a label:

```
import Effect.State

-- Named state capability (function form avoids bare Computation restriction)
program :: \(r: Row). () -> Effect { x: Int | r } Int
program := \(). do {
  putAt @#x 42;
  getAt @#x
}

-- Multiple named capabilities (same type)
counter :: \(r: Row). () -> Effect { a: Int, b: Int | r } Int
counter := \(). do {
  putAt @#a 10;
  putAt @#b 20;
  x <- getAt @#a;
  y <- getAt @#b;
  pure (x + y)
}
```

Available `*At` variants:

| Named variant              | Fixed equivalent | Pack           |
| -------------------------- | ---------------- | -------------- |
| `getAt @#label`            | `get`            | `Effect.State` |
| `putAt @#label value`      | `put value`      | `Effect.State` |
| `failWithAt @#label error` | `failWith error` | `Effect.Fail`  |

Row type families `Without` and `Lookup` operate on label-parameterized rows:

```
Without #a { a: Int, b: String }  -- reduces to { b: String }
Lookup #a { a: Int, b: String }   -- reduces to Int
```

### seq

Provided by Core (always available without import) for sequencing when you do not need the intermediate result:

```
seq :: \a b (r1: Row) (r2: Row) (r3: Row).
  Computation r1 r2 a -> Computation r2 r3 b -> Computation r1 r3 b
```

---
