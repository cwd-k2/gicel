# Indexed Effects, Typestate, and Capabilities

One-line description: the semantic core behind `Comp pre post a`, protocol-checked effects, and host-controlled authority.

## Table of Contents

1. Problem Framing
2. Indexed Effects
3. Typestate
4. Capability-Based Security
5. How These Pieces Fit Together
6. Design Choices for Gomputation
7. Common Pitfalls
8. Key References

## 1. Problem Framing

The draft language separates pure values from effectful computations:

```text
Value
Computation
```

and gives computations the type:

```text
Comp pre post a
```

This is not just a monad with an effect label. It is closer to an indexed monad, also called a parameterized monad, where the index records a state transition at the type level.

That design is a good fit when:

- operations require protocol states such as `DB[Closed] -> DB[Opened]`
- the host decides which effects exist
- the language must stay deterministic and tightly sandboxed

## 2. Indexed Effects

### 2.1 Core idea

Robert Atkey's parameterized monads generalize ordinary monads by threading pre and post indices through computations:

```text
T : S^op x S x C -> C
```

Intuitively:

- `S1` is the required pre-state
- `S2` is the guaranteed post-state
- `A` is the result value

The core laws correspond closely to your draft:

```text
pure : a -> Comp s s a

bind :
  Comp s1 s2 a ->
  (a -> Comp s2 s3 b) ->
  Comp s1 s3 b
```

This is the right abstraction when the type of the world changes across sequencing.

### 2.2 Why ordinary effect labels are not enough

A plain effect row like `IO + Exn` says that an effect may happen, but not that a resource changed protocol state. For example:

- ordinary effects can say "this function touches the database"
- indexed effects can say "this function requires a closed database and returns an opened one"

That extra precision matters for:

- open/close protocols
- session-style APIs
- resource initialization
- single-use or linear-style host capabilities

### 2.3 Operational intuition

You can think of evaluation as:

```text
run : Env pre -> Comp pre post a -> (Env post, a)
```

where `Env` is a capability environment supplied by the host. In an implementation, the runtime value may be a host-managed structure, while `pre` and `post` are static descriptions used only by the checker.

## 3. Typestate

### 3.1 Core definition

Typestate refines ordinary type with a current usage state. The classic IBM formulation says type determines all operations ever permitted, while typestate determines which subset is permitted in the current context.

This is exactly the interpretation of examples like:

```text
Comp { db : DB[Closed] } { db : DB[Opened] } Unit
```

### 3.2 What typestate buys you

It catches protocol misuse statically:

- cannot read before initialization
- cannot close an already closed resource
- cannot write to a handle that has transitioned to a terminal state

### 3.3 Why your setting is simpler than OO typestate

A large part of typestate research deals with aliasing in mutable object graphs. Your draft is easier because:

- the language is pure at the value level
- capabilities are explicit in `Row`
- state transitions are attached to computations, not arbitrary mutable references

That means you can get strong protocol guarantees without importing the full complexity of alias permissions or ownership types.

## 4. Capability-Based Security

### 4.1 Core model

In a capability system, authority is conveyed only by possessing an unforgeable reference and passing it explicitly. No ambient authority should exist.

For this language, that translates to:

- the Go host creates capabilities
- the language cannot forge capabilities
- the language can only use capabilities present in its static and dynamic environment

### 4.2 Why capability discipline matches row-typed environments

A row like:

```text
{ db : DB[Opened], log : Logger[Ready] }
```

is a static description of the authority available to a computation. This is much closer to object-capability discipline than to global IO.

### 4.3 Practical capability rules for the spec

The spec should make these rules explicit:

1. A program can access only capabilities supplied by the host.
2. Capabilities are named, typed, and may carry protocol state.
3. There is no global namespace of implicit effects.
4. Capability introduction happens only at host-defined entry points.
5. Capability elimination or attenuation should be representable by row transformation.

## 5. How These Pieces Fit Together

The three concepts line up cleanly:

| Concept | Role in Gomputation | Static artifact |
| --- | --- | --- |
| Indexed monad | sequencing with state transition | `Comp pre post a` |
| Typestate | protocol validity of each capability | types like `DB[Opened]` |
| Capability security | authority comes only from host-provided handles | row environment |

The important design insight is that `pre` and `post` should describe authority plus protocol state, not just effect names.

## 6. Design Choices for Gomputation

### 6.1 Keep the core small

A good minimal kernel is:

```text
pure
bind
primitive host operations
```

where each host primitive has an indexed type such as:

```text
dbOpen  : Comp {db : DB[Closed]} {db : DB[Opened]} Unit
dbClose : Comp {db : DB[Opened]} {db : DB[Closed]} Unit
dbQuery : Query -> Comp {db : DB[Opened]} {db : DB[Opened]} Rows
```

### 6.2 Separate static from dynamic capability objects

Use two levels:

- static row entries that describe what is required and produced
- dynamic host handles that the interpreter carries internally

This avoids exposing host internals in the surface language.

### 6.3 Define computation introduction clearly

The draft currently has `pure` and `bind`, but not host primitive introduction. Add a section like:

```text
primitive name : Type
```

with the restriction that primitive computation types must be declared by the host, not by user code.

### 6.4 Define determinism narrowly

Determinism should mean:

- evaluation order is specified
- no implicit time, randomness, threads, or external iteration nondeterminism
- all observable effects are mediated by explicit capabilities

Host capabilities may still wrap real-world systems, but the language-level semantics stays explicit about where nondeterminism enters.

## 7. Common Pitfalls

### 7.1 Confusing effect sets with state transitions

If rows mean only "available capabilities", but not "current protocol states", typestate guarantees collapse.

### 7.2 Letting user code invent capabilities

If user code can fabricate a value with type `DB[Opened]`, the capability story is broken. Capability-bearing values need controlled constructors.

### 7.3 Mixing ambient authority back in

A standard library function that silently reads the clock or environment variables would violate the model.

### 7.4 Under-specifying resource duplication

If a capability can be duplicated freely, some protocols become unsound. The spec must decide whether all capabilities are shareable or whether some require affine or linear treatment later.

## 8. Key References

1. Robert Atkey, "Parameterised Notions of Computation", 2006. https://bentnib.org/param-notions.pdf
2. Robert E. Strom and Shaula Yemini, "Typestate: A Programming Language Concept for Enhancing Software Reliability", IEEE TSE, 1986. https://research.ibm.com/publications/typestate-a-programming-language-concept-for-enhancing-software-reliability
3. Robert DeLine and Manuel Fahndrich, "Typestates for Objects", ECOOP 2004. https://www.microsoft.com/en-us/research/publication/typestates-for-objects/
4. Kevin Bierhoff and Jonathan Aldrich, "Modular Typestate Checking of Aliased Objects". https://www.cs.cmu.edu/~aldrich/papers/typestate-verification.pdf
5. Mark S. Miller, Ka-Ping Yee, Jonathan Shapiro, "Capability Myths Demolished". https://erights.org/talks/myths/index.html
6. E language notes on capability discipline. https://www.erights.org/elang/kernel/auditors/index.html

## Relevance to Gomputation

For the current draft, the clearest route is:

1. Treat `Comp pre post a` as an indexed monad, not as a vague effect annotation.
2. Treat rows as capability environments containing protocol-state-bearing entries.
3. Treat host registration as the only source of authority.
4. Keep the first version free of alias-sensitive or linear capability rules unless a concrete use case forces them in.
