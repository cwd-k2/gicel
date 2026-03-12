# DataKinds and Type-Level Promotion

One-line description: whether promoting value-level data constructors to the type level is useful for Gomputation, and what it would cost.

## Table of Contents

1. Short Answer
2. What DataKinds Adds
3. Where Promotion Helps in This Design
4. Costs and Constraints
5. Alternatives
6. Recommendation for Gomputation
7. Key References

## 1. Short Answer

DataKinds is one of the more practical advanced type-system features for this project.

It gives a disciplined way to move finite protocol states from terms into types:

```text
data DBState = Opened | Closed
```

can support types such as:

```text
DB Opened
DB Closed
```

That is directly aligned with the existing draft examples.

The important limitation is that DataKinds alone does not add type-level computation. It mainly adds promoted constants and kind distinctions.

## 2. What DataKinds Adds

GHC's `DataKinds` feature promotes datatype declarations to the kind level and promotes constructors to type constructors. The classic motivation is preventing nonsense types like `Vec Int Char` by distinguishing a dedicated index kind such as `Nat`.

For Gomputation, the analogous move would be:

```text
data DBState = Opened | Closed
```

and then:

```text
DB : DBState -> Type
```

This is conceptually lighter than dependent types because:

- the promoted data is still finite and syntactic
- type-level inhabitants come from declared constructors
- there is no general computation over runtime values

## 3. Where Promotion Helps in This Design

### 3.1 Protocol states become better behaved

Your draft already writes:

```text
DB[Closed]
DB[Opened]
```

Promotion gives this pattern a clearer foundation. Instead of informal bracket syntax, the spec could describe:

```text
data DBState = Opened | Closed
DB : DBState -> Type
```

That prevents accidental mixing with ordinary types and makes protocol states first-class at the kind level.

### 3.2 Capability rows become more precise

Rows like:

```text
{ db : DB Opened, log : Logger Ready }
```

read more coherently if `Opened` and `Ready` are promoted constructors, not ad hoc type names.

### 3.3 GADTs become more meaningful later

If you eventually add GADTs, promoted state indices make constructor result types much clearer. Promotion is often the first useful step before GADTs, not after.

## 4. Costs and Constraints

### 4.1 You need richer kinds

The current kind grammar:

```text
Type
Row
```

would need to acknowledge user-declared promoted kinds such as `DBState`.

This is still manageable. It is much cheaper than adding HKT or full dependent types.

### 4.2 Syntax must distinguish term and type constructors

Languages handle this differently. GHC uses ticks in some contexts and also has unique syntax for type-level lists and tuples. Gomputation should keep the syntax simpler, but it still needs a clear rule about when `Opened` denotes a term constructor and when it denotes a promoted type-level constructor.

### 4.3 Promotion is not enough for type-level functions

If you later want:

```text
NextState Opened = Closed
```

you have crossed into type-family or dependent-type territory. Promotion does not provide this on its own.

## 5. Alternatives

### 5.1 Opaque phantom types

You can model protocol states with plain type constructors only:

```text
type Opened
type Closed
DB Opened
DB Closed
```

This is simpler to implement than general promotion, but less uniform and less expressive if you later want promoted lists, promoted sums, or shared state-index kinds.

### 5.2 Keep state names as ordinary type constructors

This is the lightest path. It works if the language only ever needs a few hand-written phantom states. It becomes awkward once users want families of indexed types.

## 6. Recommendation for Gomputation

DataKinds is a plausible near-term feature.

Recommended stance:

1. allow user-defined algebraic datatypes as ordinary ADTs first
2. leave room for promotion in the kind system
3. add DataKinds when protocol-state syntax starts feeling ad hoc

If you want a sharper recommendation: DataKinds is more justified for Gomputation than HKT, Type Families, or dependent types.

## 7. Key References

1. GHC User's Guide, `DataKinds`. https://ghc.gitlab.haskell.org/ghc/doc/users_guide/exts/data_kinds.html
2. Yorgey et al., "Giving Haskell a Promotion", TLDI 2012. https://dreixel.net/research/pdf/ghp.pdf

## Relevance to Gomputation

If the project wants typed protocol states to feel principled rather than ad hoc, DataKinds is a strong candidate. It sharpens the meaning of `DB[Opened]`-style examples without forcing the language into full type-level computation.
