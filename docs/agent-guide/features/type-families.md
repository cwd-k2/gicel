## Type Families

Type families define type-level functions evaluated during type checking and fully erased before runtime. No runtime cost.

### Standalone Closed Type Family

Declared with `type Name :: Kind := \params. case scrutinee { equations }`. Equations checked top-to-bottom; first match wins.

```
type Elem :: Type := \(c: Type). case c {
  List a  => a;
  Slice a => a;
  String  => Rune
}
```

Distinguished from a type alias by `::` after the name.

### Associated Types

Classes can declare associated types (kind signature only). Instances provide definitions.

```
data Container c {
  type Elem c :: Type;
  cfold: \b. (Elem c -> b -> b) -> b -> c -> b
}

impl Container (List a) := {
  type Elem := a;
  cfold := foldr
}
```

`Elem (List Int)` reduces to `Int` during type checking.

### Data Families

Data families are generative -- each instance creates a distinct data type with its own constructors:

```
data Collection c {
  data Key c :: Type;
  lookup: Key c -> c -> Maybe (Elem c)
}

impl Collection (List a) := {
  data Key := ListIndex Int;
  lookup := \k xs. case k { ListIndex i => index xs i }
}
```

### Injectivity

Injectivity is verified at declaration time by pairwise comparison: if two alternatives' right-hand sides unify, their left-hand sides must also unify.

```
type Effects :: Row := \(mode: AppMode). case mode {
  ReadOnly  => { get: () -> String };
  ReadWrite => { get: () -> String, put: String -> () }
}
```

### Reduction

- **Non-recursive** type families reduce in one step per application.
- **Recursive** type families use a fuel limit (default: 100) to prevent divergence.
- **Indeterminate** matches (unsolved metavariables) cause reduction to stick, preventing premature commitment.
- Type families cannot be partially applied.

See the language specification (Chapter 17) for pattern matching rules and interaction with row types, GADTs, and the evidence system.
