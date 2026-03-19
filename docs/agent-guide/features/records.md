## Records and Tuples

Records are structural types parameterized by row types. Field order is semantically irrelevant.

### Record Literals

```
{ name: "Alice", age: 30 }
{ x: 1, y: True }
{}                                   -- empty record / unit
```

A record literal `{ l1: e1, l2: e2 }` has type `Record { l1: T1, l2: T2 }`.

### Record Projection

The `.#` operator projects a field from a record:

```
r.#name                              -- project field "name"
r.#x.#y                              -- chained (left-associative, atom precedence)
f r.#x                               -- f (r.#x) -- projection binds tighter than application
```

### Record Update

Update one or more fields in an existing record. The field must exist in the original:

```
{ r | age: 31 }
{ r | x: 42, y: True }
```

### Record Patterns

Record patterns are open -- unlisted fields are ignored:

```
\{ x: a, y: b }. a                  -- lambda with record pattern
\{ x: a }. a                        -- partial match, other fields ignored
case r { { x: a, y: b } -> a }
{ x: n } := r                       -- block binding destructuring
```

### Tuples

Tuples are syntactic sugar for records with positional labels `_1`, `_2`, `_3`, ...

| Surface            | Desugars to                    |
| ------------------ | ------------------------------ |
| `(1, True)`        | `{ _1: 1, _2: True }`          |
| `(Int, Bool)`      | `Record { _1: Int, _2: Bool }` |
| `t.#_1`            | projection on `_1`             |
| `(a, b)` (pattern) | `{ _1: a, _2: b }` (pattern)   |

`()` is the 0-tuple, equivalent to the empty record `{}`. `(expr)` with no comma is grouping, not a 1-tuple.

### Row Types

Records and capability environments share the `Row` kind:

```
{}                                   -- empty row (closed)
{ x: Int, y: Bool }                 -- closed row
{ x: Int | r }                      -- open row (tail variable r)
```

Row polymorphism lets functions accept any record with the required fields:

```
getName :: \(r: Row). Record { name: String | r } -> String
getName := \rec. rec.#name
```

### Type Annotations

In type position, always write `Record { ... }` explicitly. A bare `{ ... }` is a capability row (used in `Computation` indices), not a record type:

```
f :: Record { x: Int } -> Int        -- correct: Record type
g :: Computation { db: DB } {} ()    -- correct: capability rows
```

Higher-rank fields are supported:

```
r :: Record { apply: \a. a -> a }
r := { apply: \x. x }
```

See the language specification (Chapter 8) for elaboration details.
