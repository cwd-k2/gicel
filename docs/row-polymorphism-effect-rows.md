# Row Polymorphism and Effect Rows

One-line description: the type-theoretic machinery needed to represent extensible capability environments without losing type inference.

## Table of Contents

1. Why Rows Matter Here
2. Row Polymorphism Basics
3. Record Rows vs Effect Rows
4. Row Operations Needed by the Draft
5. Equality and Unification Choices
6. Open Questions for Gomputation
7. Recommended Minimal Rule Set
8. Key References

## 1. Why Rows Matter Here

The draft uses rows for capability states:

```text
R ::= {}
    | { l : T }
    | { l : T , ... }
    | { l : T | r }
```

This is the right direction, but rows are only useful if the language defines:

- whether label order matters
- whether duplicate labels are allowed
- how row variables unify
- whether rows denote exact environments or lower bounds

Without those choices, neither typechecking nor error reporting will stabilize.

## 2. Row Polymorphism Basics

### 2.1 Core idea

Row polymorphism represents extensible labeled structures while preserving static typing. Instead of saying a record has exactly fields `x` and `y`, you can say it has at least `x` plus some unknown remainder `r`:

```text
{ x : Int | r }
```

The same idea transfers naturally to effect and capability environments.

### 2.2 Why rows are preferable to subtyping here

For this language, rows are usually a better fit than width subtyping:

- row unification is syntax-directed and more predictable
- principal types are easier to preserve
- host capability composition is explicit

Subtyping tends to complicate inference and often obscures which capabilities are actually used.

## 3. Record Rows vs Effect Rows

Rows first became popular for extensible records and variants, but effect systems reuse the same machinery.

There are two common interpretations:

| Row kind | Meaning |
| --- | --- |
| Record row | available labeled fields in a value |
| Effect row | available or produced effects in a computation |

Your draft uses rows in a third but closely related way:

| Gomputation row | Meaning |
| --- | --- |
| Capability row | available authority together with protocol state |

That is more structured than a plain effect set. A capability entry is not just `db`; it is `db : DB[Opened]`.

## 4. Row Operations Needed by the Draft

### 4.1 Extension

You need a way to say a computation preserves unknown capabilities:

```text
dbOpen : Comp { db : DB[Closed] | r } { db : DB[Opened] | r } Unit
```

This is the key benefit of open rows. Otherwise every primitive must mention the entire environment.

### 4.2 Lookup

The checker must establish that a row contains a label with a compatible type. This is typically handled by unification against a row variable.

### 4.3 Replacement

Typestate transitions require replacing the type at an existing label:

```text
{ db : DB[Closed] | r } -> { db : DB[Opened] | r }
```

This is not just addition or removal. It is state update at a fixed label.

### 4.4 Composition

`bind` composes post-state and pre-state:

```text
Comp r1 r2 a ->
(a -> Comp r2 r3 b) ->
Comp r1 r3 b
```

So row equivalence and normalization matter directly to sequencing.

## 5. Equality and Unification Choices

### 5.1 Duplicate labels

You need to decide whether duplicate labels are legal.

For capability environments, the answer should almost certainly be no. Scoped duplicate labels are useful for extensible records, but they are a poor fit for authority tracking because:

- capability lookup becomes less obvious
- protocol updates become ambiguous
- error messages become harder to trust

### 5.2 Permutation

Rows should almost certainly be equal up to permutation:

```text
{ db : DB[Opened], log : Logger[Ready] }
==
{ log : Logger[Ready], db : DB[Opened] }
```

This is standard and avoids spurious mismatch errors.

### 5.3 Open vs closed rows

Use:

- closed rows for exact environments
- open rows for polymorphic capability preservation

That gives precise primitives and reusable abstractions without needing subtyping.

### 5.4 Row unification

A practical unifier usually needs:

1. normalization of row order
2. decomposition on matching labels
3. occurs check for row variables
4. a representation of row tails

Pseudo-constraint examples:

```text
{ db : DB[Closed] | r1 } ~ { db : DB[Closed], log : Logger[Ready] }
=> r1 ~ { log : Logger[Ready] }
```

```text
{ db : DB[Closed] | r1 } ~ { db : DB[Opened] | r2 }
=> fail
```

## 6. Open Questions for Gomputation

### 6.1 Are rows exact resources or lower bounds?

If `pre` means "at least these capabilities", then open rows fit naturally. If it means "exactly this environment", the language is stricter but harder to use compositionally.

Recommendation: interpret rows as exact when closed, lower-bounded when open.

### 6.2 Are capability labels globally unique?

This should be yes. If the host registers two `db` capabilities, user-level reasoning becomes unclear unless a module or namespace system exists.

### 6.3 Can a capability disappear?

If so, you need primitives like:

```text
closeAndDrop : Comp { tmp : File[Open] | r } { r } Unit
```

That requires row subtraction, not just replacement.

### 6.4 Will effect rows and capability rows coexist?

You may later want a separate effect layer such as `Exn` or `Div`. If so, keep that separate from capability rows. Do not overload one row structure with two unrelated meanings unless there is a concrete plan for their interaction.

## 7. Recommended Minimal Rule Set

For a first implementation, use this simplified design:

1. Rows are finite maps from unique labels to types plus an optional tail variable.
2. Rows are equal up to permutation.
3. Duplicate labels are rejected.
4. Open rows express preserved unknown capabilities.
5. No subtyping; use unification only.
6. Capability transitions are replacement at a fixed label.
7. Row subtraction is optional and can wait until there is a strong use case.

This gives enough structure for:

- typestate-aware primitives
- principal types in common cases
- predictable implementation cost

## 8. Key References

1. Mitchell Wand, "Type inference for record concatenation and multiple inheritance", 1987.
2. Daan Leijen, "Extensible records with scoped labels", 2005. https://www.microsoft.com/en-us/research/wp-content/uploads/2016/02/scopedlabels.pdf
3. Sam Lindley and Conor McBride, "Hasochism: The Pleasure and Pain of Dependently Typed Haskell Programming", for row-style indexed encodings.
4. The Koka language book, effect types and handlers. https://koka-lang.github.io/koka/doc/book.html

## Relevance to Gomputation

The main spec task here is not to import the entire literature. It is to freeze a small, explicit row discipline. For this project, the highest-leverage choice is:

- unique labels
- permutation-insensitive rows
- open tail variables
- unification instead of subtyping

That combination is strong enough to support capability-preserving primitives and still realistic to implement in Go.
