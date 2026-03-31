## 5. Effect System

### Computation pre post a

The core abstraction is `Computation @g pre post a` -- a graded Atkey-style parameterized monad (graded indexed monad). It represents an effectful computation that:

- Is graded by `g` (a type-level grade from a `GradeAlgebra` instance)
- Requires capability environment `pre` (a row type) at the start
- Produces capability environment `post` (a row type) at the end
- Returns a value of type `a`

When `pre` and `post` are the same and the grade is `Zero`, the computation preserves its environment with no grade information. The type alias `Effect r a := Computation Zero r r a` is provided for this common case.

### pure and bind

```
pure :: \a (r: Row) g. a -> Computation @g r r a
bind :: \a b g1 g2 g3 (r1: Row) (r2: Row) (r3: Row).
          Computation @g1 r1 r2 a -> (a -> Computation @g2 r2 r3 b) -> Computation @g3 r1 r3 b
```

These are built-in -- always available without import. Note how `bind` composes pre/post indices: `r1->r2` then `r2->r3` yields `r1->r3`. Grade parameters are inferred automatically; `g3` is resolved to `GradeCompose g1 g2` when used through `GIMonad`.

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
thunk :: Computation @g pre post a -> Thunk @g pre post a
```

`force` runs a suspended computation:

```
force :: Thunk @g pre post a -> Computation @g pre post a
```

The type alias `Suspended r a := Thunk Zero r r a` mirrors `Effect` for suspended computations that preserve their capability state with zero grade.

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

Available `*At` variants (all effect packs provide named variants for every effectful operation):

| Pack           | Named variants                                                                                                                                      |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Effect.State` | `getAt`, `putAt`, `modifyAt`                                                                                                                        |
| `Effect.Fail`  | `failWithAt`                                                                                                                                        |
| `Effect.Array` | `newAt`, `readAt`, `writeAt`, `resizeAt`, `toSliceAt`, `fromSliceAt`                                                                                |
| `Effect.Ref`   | `newAt`, `readAt`, `writeAt`, `modifyAt`                                                                                                            |
| `Effect.Map`   | `newAt`\*, `insertAt`, `lookupAt`, `deleteAt`, `sizeAt`, `memberAt`, `toListAt`, `fromListAt`\*, `foldlWithKeyAt`, `keysAt`, `valuesAt`, `adjustAt` |
| `Effect.Set`   | `newAt`\*, `insertAt`, `memberAt`, `deleteAt`, `sizeAt`, `toListAt`, `fromListAt`\*, `foldAt`, `unionAt`, `intersectionAt`, `differenceAt`          |

\* `newAt`/`fromListAt` for Effect.Map and Effect.Set take an explicit `(k -> k -> Ordering)` compare function instead of `Ord k =>` (compare is stored in the handle at creation).

Row type families `Without` and `Lookup` operate on label-parameterized rows:

```
Without #a { a: Int, b: String }  -- reduces to { b: String }
Lookup #a { a: Int, b: String }   -- reduces to Int
```

### seq

Provided by Core (always available without import) for sequencing when you do not need the intermediate result:

```
seq :: \a b (g: Kind) (e1: g) (e2: g) (r1: Row) (r2: Row) (r3: Row).
  Computation @e1 r1 r2 a -> Computation @e2 r2 r3 b -> Computation @e2 r1 r3 b
```

---
