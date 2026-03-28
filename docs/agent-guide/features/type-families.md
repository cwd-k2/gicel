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
form Container := \c. {
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

Data families are generative -- each instance creates a distinct form type with its own constructors:

```
form Collection := \c. {
  type Key c :: Type;
  lookup: Key c -> c -> Maybe (Elem c)
}

impl Collection (List a) := {
  type Key := Int;
  lookup := \i xs. index i xs
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

### Builtin Row Type Families

Three builtin type families operate on capability rows:

| Family    | Signature              | Description                           |
| --------- | ---------------------- | ------------------------------------- |
| `Merge`   | `Row -> Row -> Row`    | Disjoint merge of two capability rows |
| `Without` | `Label -> Row -> Row`  | Remove a label from a capability row  |
| `Lookup`  | `Label -> Row -> Type` | Extract the type at a label in a row  |

```
Merge { a: Int } { b: String }         -- { a: Int, b: String }
Without #a { a: Int, b: String }       -- { b: String }
Lookup #a { a: Int, b: String }        -- Int
```

`Without` and `Lookup` require their first argument to have `Label` kind. In type application context (`@#name`), `#name` is a label literal.

### Reduction

- **Non-recursive** type families reduce in one step per application.
- **Recursive** type families use a fuel limit (default: 50,000) to prevent divergence.
- **Indeterminate** matches (unsolved metavariables) cause reduction to stick, preventing premature commitment.
- Type families cannot be partially applied.
