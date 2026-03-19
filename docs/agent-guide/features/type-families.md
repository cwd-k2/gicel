## Type Families

Type families define type-level functions evaluated during type checking and fully erased before runtime. No runtime cost.

### Standalone Closed Type Family

Declared with `type Name params :: Kind := { equations }`. Equations checked top-to-bottom; first match wins.

```
type Elem (c: Type) :: Type := {
  Elem (List a) =: a;
  Elem (Slice a) =: a;
  Elem String =: Rune
}
```

Distinguished from a type alias by `::` after parameters.

### Associated Types

Classes can declare associated types (kind signature only). Instances provide definitions.

```
class Container c {
  type Elem c :: Type;
  cfold :: \b. (Elem c -> b -> b) -> b -> c -> b
}

instance Container (List a) {
  type Elem (List a) =: a;
  cfold := foldr
}
```

`Elem (List Int)` reduces to `Int` during type checking.

### Data Families

Data families are generative -- each instance creates a distinct data type with its own constructors:

```
class Collection c {
  data Key c :: Type;
  lookup :: Key c -> c -> Maybe (Elem c)
}

instance Collection (List a) {
  data Key (List a) =: ListIndex Int;
  lookup := \k xs. case k { ListIndex i -> index xs i }
}
```

### Functional Dependencies

Constrain instance resolution. `| a =: b` means knowing `a` determines `b`:

```
class Convert a b | a =: b {
  convert :: a -> b
}
```

### Injectivity

Declare a type family result as injective with a named result binder:

```
type Effects (mode: AppMode) :: (r: Row) | r =: mode := {
  Effects ReadOnly  =: { get: () -> String };
  Effects ReadWrite =: { get: () -> String, put: String -> () }
}
```

`| r =: mode` means the result uniquely determines the argument.

### Reduction

- **Non-recursive** type families reduce in one step per application.
- **Recursive** type families use a fuel limit (default: 100) to prevent divergence.
- **Indeterminate** matches (unsolved metavariables) cause reduction to stick, preventing premature commitment.
- Type families cannot be partially applied.

See the language specification (Chapter 17) for pattern matching rules and interaction with row types, GADTs, and the evidence system.
