# Phase 1: Types

## Objective

Define the canonical representations of Kind, Type, and Row that every subsequent package consumes. This package is the lingua franca of the compiler.

## Dependencies

Phase 0 (`internal/span`).

## Package: `types/`

### 1.1 Kind (`types/kind.go`)

```go
package types

// Kind classifies types and rows.
//
//   Kind ::= KType | KRow | KArrow(K1, K2)
//
type Kind interface {
    kindNode()
    Equal(Kind) bool
    String() string
}

type KType struct{}           // Type
type KRow  struct{}           // Row
type KArrow struct {          // K1 -> K2
    From Kind
    To   Kind
}
```

Three constructors. `KArrow` enables higher-kinded types in future drafts but is already needed for `Computation : Row -> Row -> Type -> Type`.

**Arity helper**:

```go
// Arity returns the number of arguments a kind accepts.
//   KType → 0, KRow → 0, KArrow(K1, K2) → 1 + Arity(K2)
func Arity(k Kind) int

// ResultKind returns the kind after all arguments are applied.
func ResultKind(k Kind) Kind
```

### 1.2 Type (`types/type.go`)

```go
// Type is the unified representation for value types, computation types, and row types.
//
// Spec §13.2 — Core Types
//
type Type interface {
    typeNode()
    Span() span.Span
    // Children returns immediate sub-types for traversal.
    Children() []Type
}

// TyVar — type or row variable.
type TyVar struct {
    Name string
    S    span.Span
}

// TyCon — named type constructor (Int, String, Bool, user-defined).
type TyCon struct {
    Name string
    S    span.Span
}

// TyApp — general type application (F T).
// Includes partial application: Computation R1 = TyApp(TyCon("Computation"), R1).
type TyApp struct {
    Fun Type
    Arg Type
    S   span.Span
}

// TyArrow — function type (A -> B).
type TyArrow struct {
    From Type
    To   Type
    S    span.Span
}

// TyForall — universal quantification (forall a:K. T).
type TyForall struct {
    Var  string
    Kind Kind   // bound variable's kind
    Body Type
    S    span.Span
}

// TyComp — Computation pre post a (dedicated node for efficiency).
type TyComp struct {
    Pre    Type   // Row
    Post   Type   // Row
    Result Type
    S      span.Span
}

// TyThunk — Thunk pre post a.
type TyThunk struct {
    Pre    Type
    Post   Type
    Result Type
    S      span.Span
}

// TyRow — row type { l1:T1, ..., ln:Tn | tail? }.
type TyRow struct {
    Fields []RowField
    Tail   Type    // nil = closed row, TyVar = open row
    S      span.Span
}

// RowField is a single label:type pair in a row.
type RowField struct {
    Label string
    Type  Type
    S     span.Span
}

// TyMeta — unification metavariable (created by the checker, never in source).
type TyMeta struct {
    ID   int
    Kind Kind
    S    span.Span
}

// TyError — poison type for error recovery.
// Unifies with any type without producing further errors.
type TyError struct {
    S span.Span
}
```

**Design decisions**:

- `TyComp` and `TyThunk` are **dedicated nodes**, not desugared to `TyApp(TyApp(TyApp(TyCon("Computation"), pre), post), result)`. Rationale: the evaluator and checker pattern-match on these constantly; dedicated nodes avoid repeated deconstruction.
- `TyMeta` exists here (not in `check/`) because types flow between packages. The checker creates metavariables; the types package provides the representation.
- `TyRow` stores fields sorted by label. Normalization is enforced at construction.

### 1.3 Row Operations (`types/row.go`)

```go
// Normalize sorts fields by label and flattens nested extensions.
// Panics if duplicate labels are found (caller must check).
func Normalize(r *TyRow) *TyRow

// Labels returns the set of label names in a row (excluding tail).
func Labels(r *TyRow) map[string]struct{}

// HasLabel checks if a label exists in a row's fields.
func HasLabel(r *TyRow, label string) bool

// ExtendRow adds a field to a row, maintaining sorted order.
// Returns error if label already exists in fields.
func ExtendRow(r *TyRow, f RowField) (*TyRow, error)

// RemoveLabel removes a field by label. Returns the field and the remaining row.
func RemoveLabel(r *TyRow, label string) (RowField, *TyRow, bool)
```

### 1.4 Type Equality (`types/equal.go`)

Spec §8.2 — two types are equal if their normal forms are identical.

```go
// Equal checks structural equality of two types.
// For rows: compares normalized forms (label order irrelevant).
// For forall: alpha-equivalence (bound variable names irrelevant).
func Equal(a, b Type) bool
```

Implementation:

1. If both are `TyRow`, normalize both, compare sorted fields pairwise, compare tails.
2. If both are `TyForall`, rename bound vars to a canonical form, compare bodies.
3. Otherwise, structural comparison (recursively compare children).

### 1.5 Substitution (`types/subst.go`)

```go
// Subst applies a substitution [var := replacement] throughout a type.
// Respects binding: does not substitute under forall that shadows the variable.
func Subst(t Type, varName string, replacement Type) Type

// SubstMany applies multiple substitutions simultaneously.
func SubstMany(t Type, subs map[string]Type) Type
```

**Capture avoidance**: If substituting into `forall a. T` and `a` is free in `replacement`, rename `a` to a fresh variable before substituting into `T`. Use a global counter for fresh names.

### 1.6 Free Variables (`types/free.go`)

```go
// FreeVars returns the set of free type/row variables in a type.
func FreeVars(t Type) map[string]struct{}

// OccursIn checks if a variable name appears free in a type.
func OccursIn(name string, t Type) bool
```

### 1.7 Pretty Printing (`types/pretty.go`)

```go
// Pretty renders a type as human-readable text.
// Used for error messages and debugging.
func Pretty(t Type) string

// PrettyKind renders a kind.
func PrettyKind(k Kind) string
```

Formatting rules:
- `TyArrow`: `A -> B`, parenthesized when `A` is itself an arrow
- `TyForall`: `forall a. T`, coalescing adjacent foralls: `forall a b. T`
- `TyComp`: `Computation { ... } { ... } A`
- `TyThunk`: `Thunk { ... } { ... } A`
- `TyRow`: `{ l1 : T1, l2 : T2 | r }` or `{}`
- `TyMeta`: `?α₀`, `?α₁` etc. (debugging only)

### 1.8 Built-in Types (`types/builtin.go`)

```go
// Pre-defined type constructors and their kinds.
var (
    KindOfComputation = &KArrow{&KRow{}, &KArrow{&KRow{}, &KArrow{&KType{}, &KType{}}}}
    KindOfThunk       = &KArrow{&KRow{}, &KArrow{&KRow{}, &KArrow{&KType{}, &KType{}}}}
)

// Convenience constructors.
func MkComp(pre, post, result Type) *TyComp
func MkThunk(pre, post, result Type) *TyThunk
func MkArrow(from, to Type) *TyArrow
func MkForall(v string, k Kind, body Type) *TyForall
func EmptyRow() *TyRow
func ClosedRow(fields ...RowField) *TyRow
func OpenRow(fields []RowField, tail Type) *TyRow
```

## Test Strategy

- **Kind**: equality, arity, string representation.
- **Type construction**: build Computation/Thunk types, verify structure.
- **Row normalization**: unsorted input → sorted output; nested extensions → flat; duplicate detection.
- **Equality**: alpha-equivalence of forall, row permutation equality, negative cases.
- **Substitution**: simple substitution, capture avoidance, identity substitution.
- **Free variables**: closed types → empty set, open rows → row var in set.
- **Pretty printing**: golden-string tests for complex types.

## Completion Criteria

- [ ] All types constructible and pattern-matchable in Go
- [ ] Row normalization correct (sorted, flat, duplicate-free)
- [ ] Type equality handles alpha-equivalence and row permutation
- [ ] Substitution is capture-avoiding
- [ ] Pretty printing round-trips readable output
- [ ] No dependency on any other gomputation package (only `internal/span`)
- [ ] All tests pass
