# Record Design Spec: Row-Polymorphic Records

## 1. Motivation

AI agents construct structured data in the host language (Go maps, JSON objects) and need
a type-safe way to pass structured values into Gomputation computations. Records provide
the natural surface syntax. Row polymorphism, already present for capability environments,
unifies record and effect typing under a single Row kind.

## 2. Type-Level Design

### Record Type

```
TyRecord : Row → Type
```

A record type is parameterized by a row. `{ x : Int, y : Bool }` is sugar for
`Record { x : Int, y : Bool }`.

### Unified Row Kind

Rows serve dual duty:

| Usage | Example |
|-------|---------|
| Capability environments | `Computation { state : s \| r } { state : s \| r } a` |
| Record types | `Record { x : Int, y : Bool \| r }` |

Both share `Kind = Row`. This means row variables, row unification, and row-polymorphic
functions apply uniformly to both records and effects.

### Kind Disambiguation

`{ x : Int | r }` in isolation is ambiguous — it could be a row literal (Kind = Row) or
a record type (Kind = Type). Resolution:

- **In type position**: The parser produces a `TyRow`. The kind checker determines
  whether it stands alone (Row) or is wrapped in `Record` (Type) based on context.
- **In expression position**: `{ x = 1, y = True }` is always a record literal.
- **In pattern position**: `{ x, y }` destructures a record.

No new keyword is needed; the parser uses `{` as the leading token, and the `:` vs `=`
distinction separates type-level from expression-level.

## 3. Surface Syntax

### Record Literal

```
{ x = 1, y = True, z = "hello" }
```

Comma-separated `label = expr` pairs. Trailing comma optional.

### Field Projection

```
r.x
```

Dot-syntax desugars to a projection primitive. `r.x` has type
`forall r'. { x : a | r' } -> a` (row-polymorphic).

### Record Update

```
{ r | x = 42 }
```

Creates a new record identical to `r` except for the specified fields.
Type: `forall r'. { x : a | r' } -> { x : b | r' }` when updating `x : a` to `x : b`.

### Record Pattern

```
case rec of { { x, y } -> x + y }
```

Destructuring bind in case alternatives. Variables in `{ x, y }` bind to the
corresponding field values.

## 4. Elaboration

### Record Literal → PrimOp

```
{ x = e1, y = e2 }
  ↓ elaborate
PrimOp("record_mk", [("x", e1'), ("y", e2')])
```

The elaborator collects labels and elaborated sub-expressions, producing a `PrimOp` that
the evaluator handles specially.

### Projection → PrimOp

```
r.x
  ↓ elaborate
PrimOp("record_get", [r', "x"])
```

### Update → PrimOp

```
{ r | x = e }
  ↓ elaborate
PrimOp("record_set", [r', "x", e'])
```

### Type Inference

Record literals infer a closed row: `{ x = 1, y = True } : Record { x : Int, y : Bool }`.

Projection infers an open row: `\r -> r.x` gets type `forall a r'. Record { x : a | r' } -> a`.

Update infers: `\r -> { r | x = 42 }` gets type
`forall r'. Record { x : Int | r' } -> Record { x : Int | r' }`.

## 5. Runtime Representation

```go
// RecordVal is a runtime record value.
type RecordVal struct {
    Fields map[string]Value
}
```

- **Construction**: allocate map, insert fields.
- **Projection**: map lookup (O(1) amortized).
- **Update**: shallow-copy map, overwrite field(s).
- **Pattern match**: extract fields by name.

Copy-on-write is not needed for records (unlike CapEnv) because records are
pure values, not threaded through computations.

## 6. Row-Polymorphic Projection

The key power of row polymorphism for records: functions can require specific fields
without constraining the rest.

```
getName :: forall r. Record { name : String | r } -> String
getName := \rec -> rec.name
```

This works on any record with at least a `name : String` field. The row variable `r`
absorbs all other fields.

### Label Uniqueness

Following the existing row design, label uniqueness is enforced at instantiation time
via the row unification algorithm. No lacks constraints needed.

## 7. Interaction with Computation Rows

Records and capabilities share Row kind, but they inhabit different type constructors:

```
Record      : Row → Type
Computation : Row → Row → Type → Type
```

A record field and a capability label can share the same name without conflict — they
are in different namespaces (record vs capability).

However, it may be valuable in the future to thread records through computations as
capabilities: `{ state : Record { x : Int, y : Bool } | r }`. This composes naturally
since `Record { ... }` is just a `Type`.

## 8. Open Questions

### 8.1 Anonymous vs Named Records

Should all records be anonymous (structural), or should named record types be supported?

- **Anonymous only** (simpler): all records are `{ ... }` with structural typing.
- **Named** (traditional): `data Point = Point { x : Int, y : Int }` — records as
  single-constructor ADTs with named fields.
- **Both**: anonymous for ad-hoc use, named for domain modeling.

Recommendation: start with anonymous records; named records can be added later as
syntactic sugar over ADTs.

### 8.2 Pattern Matching on Records

Should records be matchable in `case`?

```
case point of { { x, y } -> x + y }
```

This requires extending the pattern grammar. Alternatively, projection suffices:
`let x = point.x; let y = point.y; x + y`.

Recommendation: support record patterns for ergonomics.

### 8.3 Record Subtyping

Row-polymorphic projection already subsumes most subtyping needs. Explicit width subtyping
(`{ x : Int, y : Bool }` is a subtype of `{ x : Int }`) is not needed when row variables
handle the extra fields.

### 8.4 Extensible Records vs Fixed Records

Row polymorphism gives us extensible records naturally. Should we also support fixed
(closed-row) records as a distinct concept? Closed-row records arise naturally when
you write `{ x = 1, y = True }` without a row variable. The type system handles both
cases uniformly.

### 8.5 Runtime Field Order

Maps are unordered. If deterministic iteration order is needed (e.g., for serialization
or display), consider a `[]FieldEntry` slice alongside the map, or an ordered map.
For v0 this is not critical.
