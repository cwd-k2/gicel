# Higher-Kinded Types: Practicality and Alternatives

One-line description: whether higher-kinded types are worth adding to Gomputation now, what they buy, and what they cost.

## Table of Contents

1. Short Answer
2. What HKT Means Here
3. Where HKT Would Actually Help
4. What HKT Costs in This Design
5. Evidence from Existing Languages
6. Alternatives That Cover Most Real Use Cases
7. Recommendation for Gomputation
8. Key References

## 1. Short Answer

Higher-kinded types can be practical, but not as an early feature in the current design.

For Gomputation specifically:

- HKT is practical if the language intends to expose reusable abstractions over type constructors, such as `Functor`, `Traverse`, generic computation combinators, or effect-polymorphic libraries.
- HKT is not necessary for the core `Comp pre post a` design, capability rows, or host primitive registration.
- HKT will materially increase the complexity of kinds, type application, error messages, and the specification surface.

So the practical answer is:

1. do not make HKT part of v0.1 or the semantic core
2. keep the design HKT-compatible
3. add it later only if a concrete library use case justifies it

## 2. What HKT Means Here

Higher-kinded types let you abstract over type constructors rather than only over concrete types.

Examples:

```text
f  : Type -> Type
m  : Type -> Type -> Type
t  : Row -> Type -> Type
```

In your language, HKT would mean terms and type abstractions can quantify over constructors like:

```text
forall f. ...
```

where `f` is not a plain `Type`, but something like:

```text
f : Type -> Type
```

That requires a richer kind system than the current draft, which only names:

```text
Type
Row
```

To support HKT cleanly, you would need kinds such as:

```text
Type -> Type
Row -> Row -> Type -> Type
```

and therefore kind checking, kind inference, and type constructor application rules.

## 3. Where HKT Would Actually Help

HKT is useful when you want to write abstractions over families of types.

### 3.1 Standard structure abstractions

If you want traits or type classes later, HKT is the natural route for:

```text
Functor f
Applicative f
Monad m
```

For example:

```text
map : forall f a b. Functor f => (a -> b) -> f a -> f b
```

Without HKT, such interfaces are awkward or impossible to express directly.

### 3.2 Abstractions over computation constructors

You may eventually want combinators parameterized over computation-like constructors:

```text
forall c. ...
```

where `c` might have a kind resembling:

```text
Row -> Row -> Type -> Type
```

This would be relevant only if Gomputation wants to abstract not only over values and computations, but over whole effect encodings.

### 3.3 Library design, not core semantics

The important point is that HKT mostly helps library abstraction. It does not make the basic semantics of `Comp pre post a` work. The core indexed-effect story is already expressible without HKT.

## 4. What HKT Costs in This Design

### 4.1 Kinds stop being trivial

The current draft has a very small kind language. HKT changes that immediately.

You need:

- type constructor kinds
- kind-directed type application
- kind checking for declarations
- kind variables if you want polymorphism over kinds later

Even Scala 3 and GHC treat higher-kinded and kind-polymorphic features as foundational type-system machinery, not as a small extension.

### 4.2 Type inference gets harder to specify

The user-visible issue is not only algorithmic complexity. It is predictability.

Once you have:

- higher-rank polymorphism
- row polymorphism
- indexed computations
- HKT

the checker can still be implementable, but the number of annotation boundaries rises sharply.

This is especially relevant because the draft already needs bidirectional typing for higher-rank terms. HKT adds kind-level annotation and constructor application rules on top of that.

### 4.3 Error messages get worse fast

A simple mismatch becomes:

```text
expected constructor of kind Type -> Type
found constructor of kind Row -> Type
```

or:

```text
cannot apply type constructor of kind Type
```

That is manageable for expert users, but this project is currently aimed at embedded scripting, rule engines, and configuration logic. Those users rarely need higher-kinded abstraction early.

### 4.4 The gain is deferred until traits or modules exist

HKT is most compelling with:

- type classes / traits
- first-class modules
- generic effect combinator libraries

None of those are in the draft yet. So HKT would be arriving before its ecosystem.

## 5. Evidence from Existing Languages

### 5.1 HKT is practical in languages built around abstraction-heavy libraries

PureScript explicitly advertises higher-kinded types alongside row polymorphism and higher-rank polymorphism. That is evidence that the combination is viable in practice.

But the important contextual detail is that PureScript is a full-featured language ecosystem with type classes and large reusable libraries. HKT pays for itself there.

### 5.2 Flix shows a disciplined middle ground

Flix supports HKT, rows, effects, and associated types. Its documentation is instructive for this exact question because it explicitly says:

- kind annotations are typically needed for HKT
- associated types can express some of the same abstractions
- in some cases associated types are more flexible

That is directly relevant to Gomputation. If your real use case is only "abstract over an associated result shape", HKT may be unnecessary.

### 5.3 Mainstream compilers treat HKT as type-system infrastructure

Scala 3 models higher-kinded types through type lambdas and kind-aware bounds. GHC supports higher-rank kinds and kind polymorphism, but its documentation makes clear that higher-rank kinds have non-trivial constraints.

This is evidence that HKT is not a lightweight feature. It is feasible, but it changes the architecture of the typechecker.

### 5.4 Theory says "possible", engineering says "commit carefully"

At the theory level, richer systems like F-omega exist. But this should not be read as "free to add". Once you move toward constructor polymorphism, the checker and specification become substantially more sophisticated than the current draft.

## 6. Alternatives That Cover Most Real Use Cases

### 6.1 Plain polymorphic functions over `Comp`

Many useful abstractions do not need HKT at all:

```text
mapResult :
  forall r1 r2 a b.
  (a -> b) ->
  Comp r1 r2 a ->
  Comp r1 r2 b
```

```text
andThen :
  forall r1 r2 r3 a b.
  Comp r1 r2 a ->
  (a -> Comp r2 r3 b) ->
  Comp r1 r3 b
```

These already cover a surprising amount of practical API design.

### 6.2 Associated types or trait-local type members

Flix explicitly contrasts HKT with associated types and notes that associated types can be more flexible in some designs.

If Gomputation later adopts traits, then many interfaces can be expressed with:

```text
trait Collection t where
  type Elem t
```

instead of:

```text
trait Functor f where ...
```

You lose some standard FP encodings, but you avoid introducing full constructor polymorphism immediately.

### 6.3 Specialize around `Comp`

If the primary abstraction target is the computation type itself, a project-specific design may be better than general HKT.

For example, instead of abstracting over arbitrary `m : Type -> Type`, define a small fixed family of combinators for `Comp`.

That often gives:

- simpler syntax
- better errors
- more predictable performance

### 6.4 First-order kinded rows only

A very useful intermediate step is:

- keep row variables of kind `Row`
- do not yet quantify over arbitrary constructors of kind `Type -> Type`

This is enough for row-polymorphic capability tracking without committing to HKT.

## 7. Recommendation for Gomputation

### 7.1 Do not require HKT for the first usable language

The current pattern does not need it.

The semantic kernel can remain:

```text
Type
Row
Comp : Row -> Row -> Type -> Type
```

with user-level polymorphism only over `Type` and maybe `Row`.

### 7.2 Leave a compatibility seam

Do this now:

1. specify kinds explicitly enough that `Comp` is understood as a constructor
2. keep the internal checker representation capable of storing higher-order kinds later
3. avoid syntax choices that would block future type-parameter kind annotations

That keeps the door open without forcing the whole feature in.

### 7.3 Add HKT only when one of these becomes central

HKT becomes justified if the project commits to at least one of:

1. trait or type-class based standard libraries
2. generic abstractions over container-like types
3. generic abstractions over computation constructors beyond `Comp`
4. reusable effect-polymorphic libraries written inside the language

Absent those, HKT is mostly theoretical headroom.

### 7.4 Strong recommendation

For Gomputation, the practical choice is:

- no user-facing HKT in the first version
- yes to explicit kinds internally
- yes to revisiting HKT after traits, modules, or standard combinator libraries appear

That preserves momentum on the real hard parts: rows, indexed computations, host capabilities, and bidirectional typing.

## 8. Key References

1. Scala 3 reference, kind polymorphism. https://docs.scala-lang.org/scala3/reference/other-new-features/kind-polymorphism.html
2. GHC User's Guide, kind polymorphism and higher-rank kinds. https://downloads.haskell.org/ghc/9.6.1-alpha2/docs/users_guide/exts/poly_kinds.html
3. Flix documentation, higher-kinded types. https://doc.flix.dev/higher-kinded-types.html
4. Flix documentation, associated types. https://doc.flix.dev/associated-types.html
5. PureScript language site. https://www.purescript.org/
6. Simon Peyton Jones et al., "Practical Type Inference for Arbitrary-Rank Types". https://www.microsoft.com/en-us/research/publication/practical-type-inference-for-arbitrary-rank-types/
7. J. B. Wells, "Typability and type checking in System F are equivalent and undecidable". https://doi.org/10.1016/S0168-0072(98)00047-5

## Relevance to Gomputation

The right question is not "can HKT exist with this design?" The answer to that is yes.

The right question is "does HKT buy enough, soon enough, to justify the added kind and typechecking complexity?" For the current draft, the answer is probably no.
