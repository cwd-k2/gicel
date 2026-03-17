## 5. Effect System

### Computation pre post a

The core abstraction is `Computation pre post a` -- an Atkey-style parameterized monad (indexed monad). It represents an effectful computation that:

- Requires capability environment `pre` (a row type) at the start
- Produces capability environment `post` (a row type) at the end
- Returns a value of type `a`

When `pre` and `post` are the same, the computation preserves its environment. The type alias `Effect r a = Computation r r a` is provided for this common case.

### pure and bind

```
pure :: \a (r : Row). a -> Computation r r a
bind :: \a b (r1 : Row) (r2 : Row) (r3 : Row).
          Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

These are built-in -- always available without import. Note how `bind` composes pre/post indices: `r1->r2` then `r2->r3` yields `r1->r3`.

### Do-notation

```
do {
  x <- getState;               -- bind: extract value from Computation
  _ <- putState (x + 1);       -- bind: sequence, discard result
  y := x + 1;                  -- let: pure binding (no effect)
  pure y                       -- return result
}
```

Desugaring:

```
bind getState (\x ->
  bind (putState (x + 1)) (\_ ->
    (\y -> pure y) (x + 1)))
```

### Capability Environments

Capability environments are row types that describe what effects are available:

```
{}                                            -- no capabilities (pure)
{ state : Int }                               -- state holding an Int
{ state : Int, fail : String }                -- state and failure
{ io : () | r }                               -- io plus whatever else r contains
```

A function requiring state:

```
counter :: Computation { state : Int } { state : Int } Int
counter := do {
  n <- get;
  put (n + 1);
  pure n
}
```

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

### then

Provided by the Prelude for sequencing when you do not need the intermediate result:

```
then :: \a b (r1 : Row) (r2 : Row) (r3 : Row).
  Computation r1 r2 a -> Computation r2 r3 b -> Computation r1 r3 b
```

---
