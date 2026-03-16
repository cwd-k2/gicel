# GICEL — Language Specification

GICEL is a typed embedded language designed to run inside a Go application as a library. Its primary purpose is to let **AI agents safely construct and execute pure computations** within a host-controlled sandbox.

The host Go application defines the available capabilities — database access, network calls, file operations, or any domain-specific effect. The agent writes GICEL source code that composes these capabilities under static type checking. The language guarantees that:

- the agent **cannot access resources** not explicitly provided by the host
- the agent **cannot diverge** without the host's explicit opt-in (`EnableRecursion`)
- the agent's code is **deterministic** and **reproducible**
- all **type errors are caught before execution** — no ill-typed program reaches the evaluator
- execution is **bounded** by step limits, depth limits, and context cancellation

These guarantees are enforced by the type system (capability-indexed computation types with row polymorphism), the evaluation model (strict CBV with defense-in-depth limits), and the architecture (three-tier Engine → Runtime → Evaluator separation).

Beyond the agent sandbox use case, the language serves as a general-purpose core for safe embedded scripting, domain logic and rule evaluation, configuration evaluation, and protocol/typestate controlled execution.

---

# 1. Design Commitments

## 1.1 Semantic Commitments

The language is:

- purely functional at the value level
- effectful at the computation level
- statically typed
- capability-based
- indexed by capability state
- deterministic

## 1.2 Methodological Commitments

The specification is organized by **constitutive vocabulary** — concepts whose removal would produce a fundamentally different language.

Additionally:

- judgments are explicit
- the relationship between values and computations is clear
- capabilities are explicit in types
- formation is defined before typing
- typing rules follow introduction/elimination structure
- `Term` and `Type` are treated as distinct layers

## 1.3 Extension Framework

Extensions are classified by their impact on the vocabulary:

1. **Refinement** — enriches an existing vocabulary item. The specification's basic shape is preserved.
2. **Addition** — introduces a new vocabulary item. New judgment forms or classification layers appear.
3. **Restructure** (phase transition) — changes the relationship between existing items.

Phase transitions cannot be reached by incremental refinement. They require explicit design decisions about which vocabulary commitments are relaxed.

---

# 2. Constitutive Vocabulary

The vocabulary names concepts that make this language _this_ language. Removing any item produces a fundamentally different language. Items that are universal to all typed languages (contexts, introduction/elimination structure) are treated as specification methodology, not vocabulary.

The vocabulary is organized into four strata reflecting conceptual dependency.

## 2.1 Stratum 0 — Principles

These define what computation _means_. Changing Stratum 0 changes the language's identity.

### 2.1.1 Value / Computation

The language has two modes of expression.

**Values** are pure, inert data: integers, strings, functions, data constructors, records. They are classified by types and evaluated without side effects.

**Computations** are typed transitions over capability environments. They may read, modify, or consume capabilities provided by the host. They are classified by the `Computation` type, which tracks the required and resulting capability state.

This split follows Call-By-Push-Value (Levy 1999). The two directions of the adjunction are:

- **F** (`pure`): lifts a value into a computation — introduction of computations from values
- **U** (`thunk`): suspends a computation into a value — introduction of suspended computations
- **force**: resumes a suspended computation — elimination of thunks

`bind` provides computation sequencing.

### 2.1.2 pure / bind

The two primitive operations that define the algebra of computation.

```
pure : a -> Computation r r a
bind : Computation r1 r2 a -> (a -> Computation r2 r3 b) -> Computation r1 r3 b
```

`pure` lifts a value into a computation that preserves capability state (identity transition).

`bind` sequences two computations, composing their state transitions. The post-state of the first must match the pre-state of the second.

Together, these operations form an **Atkey parameterized monad** (Atkey, JFP 2009) and must satisfy three laws:

```
bind (pure a) f       =  f a                                  -- left identity
bind m pure           =  m                                     -- right identity
bind (bind m f) g     =  bind m (\a -> bind (f a) g)           -- associativity
```

These laws are verified by construction in the evaluator, not by the type checker.

`pure` and `bind` are not reserved keywords. They are built-in definitions with known types. They elaborate to dedicated Core nodes (`Pure`, `Bind`). The `IxMonad` type class provides a generalized interface; the `Computation` instance uses these Core nodes as its implementation.

#### Relationship to monad variants

| Variant             | Index structure     | Composition                         | What it tracks      |
| ------------------- | ------------------- | ----------------------------------- | ------------------- |
| Standard monad      | None                | `M a → (a → M b) → M b`             | Effects             |
| Atkey parameterized | Pre/post pair (i,j) | `M i j a → (a → M j k b) → M i k b` | State transitions   |
| Graded (Katsumata)  | Monoid element g    | `M g a → (a → M h b) → M (g⊕h) b`   | Effect accumulation |
| Category-graded     | Category morphism   | Subsumes both above                 | Both simultaneously |

GICEL uses the Atkey specialization because capability environments are _state transitions_ (pre → post), not _accumulated effect descriptions_. The rows compose by index matching (handoff), not by a monoidal operation.

### 2.1.3 thunk / force

The two operations that mediate between computations and values in the opposite direction to `pure`.

```
thunk : Computation pre post a -> Thunk pre post a
force : Thunk pre post a -> Computation pre post a
```

`thunk` suspends a computation without executing it, producing a first-class value. This is the CBPV `U` (thunk) operator.

`force` resumes a suspended computation, executing it in the current capability environment. This is the CBPV elimination of `U`.

Together with `pure` (= F), `thunk`/`force` (= U) complete the CBPV adjunction F ⊣ U:

```
pure  : Value → Computation        -- F: value to computation
thunk : Computation → Value         -- U: computation to value
force : U(Computation) → Computation  -- elimination of U
```

Laws:

```
force (thunk c) = c                 -- thunk/force cancellation
```

Semantics: `thunk` does not evaluate its argument — it captures the computation as a value. `force` triggers evaluation. Thunks are not memoized: forcing the same thunk multiple times executes the computation each time.

`thunk` is a **term former** (like `\` or `case`), not a function. It cannot be partially applied. It elaborates to `Core.Thunk`. `force` is an ordinary function. It elaborates to `Core.Force`.

`thunk`/`force` are part of the evaluation model (the CBPV adjunction), not the computation algebra. They remain built-in term formers regardless of type class design.

## 2.2 Stratum 1 — Classification

These define how expressions are statically classified.

### 2.2.1 Type

Classifies values. Types are formed from type variables, function types, universal quantification, named type constructors, the `Computation` type constructor, record types, and tuple types.

### 2.2.2 Row

Describes capability environments and record field sets. A row is a finite mapping from labels to types, with optional open tail. Labels are unique and order is not semantically relevant.

Row is the specific **index domain** that instantiates the abstract pre/post indices of Stratum 0 and parameterizes record types. Capability rows and record rows share the same kind and unification mechanism.

### 2.2.3 Kind

Classifies types and rows. The kind vocabulary is:

```
Kind ::= 'Type'               -- kind of value types
       | 'Row'                -- kind of capability/record row descriptors
       | 'Constraint'         -- kind of type class predicates
       | 'Kind'               -- sort of kinds (for kind-polymorphic forall)
       | Kind '->' Kind       -- kind arrow
       | UserKind             -- promoted DataKinds (e.g., DBState)
       | KindVar              -- kind variable (explicit, in forall binders)
```

Kind variables are introduced with explicit annotation: `forall (k : Kind). ...`. Kind inference uses kind metavariables and unification (occurs check, substitution). Kind variables are never inferred — they must be explicitly bound.

### 2.2.4 Computation Type

```
Computation : Row -> Row -> Type -> Type
```

The sole computation classifier. First argument: pre-state (required capability environment). Second argument: post-state (resulting capability environment). Third argument: result type.

The type alias `Effect r a = Computation r r a` denotes computations that preserve their capability state.

### 2.2.5 Thunk Type

```
Thunk : Row -> Row -> Type -> Type
```

The type of suspended computations. `Thunk pre post a` is a value that, when forced, behaves as `Computation pre post a`.

### 2.2.6 Record Type

```
Record : Row -> Type
```

Record types are parameterized by rows. Records and capabilities share the `Row` kind — row variables, unification, and polymorphic functions apply uniformly.

## 2.3 Stratum 2 — Formation

These define how expressions are structured.

### 2.3.1 Algebraic Data Types

Named variants with **construction** (data constructors build values) and **case analysis** (pattern matching consumes values). Includes ADTs and GADTs.

GADTs extend ADTs with refined return types, local type equalities in case branches, and existential quantification in constructors.

### 2.3.2 Quantification

Universal quantification over type variables, row variables, and kind variables:

```
forall a. T              -- type polymorphism
forall (r : Row). T      -- row polymorphism
forall (k : Kind). T     -- kind polymorphism
```

Higher-rank polymorphism: `forall` may appear under arrows, enabling rank-N types. Higher-rank types require explicit annotations. The checker uses subsumption (DK bidirectional approach): skolemization for checking, instantiation for inference.

### 2.3.3 Host Assumption

The sole source of effects. Host-provided operations are declared with `assumption`:

```
dbOpen :: forall r. Computation { db : DB Closed | r }
                                { db : DB Opened | r }
                                ()
dbOpen := assumption
```

No ambient authority exists. Every effect requires an explicit capability in the pre-state.

### 2.3.4 Constraint

Type class predicates of kind `Constraint`. Values of kind `Constraint` are type class predicates (e.g., `Eq Bool : Constraint`). Constraints enable qualified polymorphism via dictionary passing — they elaborate to implicit function arguments.

## 2.4 Stratum 3 — Judgment

### 2.4.1 Equality

Type, row, and kind equivalence. The equality theory includes:

- alpha-equivalence of bound variables
- row permutation (label order is irrelevant)
- row normalization
- local equality refinement (GADTs: pattern matching introduces type equalities)
- kind unification (with kind metavariables)

---

# 3. Syntax

## 3.1 Keywords

```
case  do  data  type  forall  infixl  infixr  infixn  class  instance  import
```

11 keywords. Note that `pure`, `bind`, `thunk`, `force`, `assumption`, `rec`, and `fix` are **not** keywords — they are ordinary identifiers with built-in meaning.

`;` and newline are interchangeable as declaration/statement separators at the top level. Inside braces (`do`, `case`, GADT bodies), semicolons are required — newlines alone do not act as separators.

## 3.2 Identifiers

```
Var    ::= lower alpha-start identifier     -- x, foo, dbOpen
Con    ::= upper alpha-start identifier     -- True, DB, Opened
TyVar  ::= lower alpha-start identifier     -- a, r, pre, post
TyCon  ::= upper alpha-start identifier     -- Bool, Computation, Thunk
Label  ::= lower alpha-start identifier     -- db, log, auth
Op     ::= operator characters              -- +, -, *, /, ==, >>=, .
```

## 3.3 Tokens

| Token | Meaning                                                 |
| ----- | ------------------------------------------------------- |
| `::`  | Type annotation                                         |
| `:=`  | Definition binding                                      |
| `->`  | Function type arrow                                     |
| `=>`  | Constraint arrow                                        |
| `\`   | Lambda                                                  |
| `\|`  | Row extension / record update / case separator          |
| `.`   | forall body separator / composition operator (infixr 9) |
| `!#`  | Record projection                                       |
| `@`   | Explicit type application                               |

## 3.4 Type Syntax

```
Type      ::= 'forall' TyBinder+ '.' Type
            | Constraint '=>' Type
            | Type '->' Type
            | TypeApp
            | '(' Type ',' Type (',' Type)* ')'     -- tuple type

TypeApp   ::= TypeApp TypeAtom
            | TypeAtom

TypeAtom  ::= TyVar | TyCon
            | '(' Type ')'
            | RowExpr

TyBinder  ::= TyVar                          -- kind inferred
            | '(' TyVar ':' Kind ')'          -- kinded

Constraint ::= TypeApp                        -- e.g., Eq a, Ord b

Kind      ::= 'Type' | 'Row' | 'Constraint' | 'Kind'
            | Kind '->' Kind
            | ConName                          -- promoted DataKinds
            | KindVar
```

Precedence of type operators (loosest to tightest):

1. `forall ... .`
2. `=>` (right-associative)
3. `->` (right-associative)
4. Type application (left-associative)

## 3.5 Expression Syntax

```
Expr      ::= 'do' '{' Stmt+ '}'                           -- do block
            | '\' Pattern '->' Expr                         -- lambda (single parameter)
            | 'case' Expr '{' Branch (';' Branch)* '}'     -- case analysis
            | 'thunk' Expr                                  -- suspend computation
            | ExprInfix

ExprInfix ::= ExprInfix Op ExprApp                          -- operator application
            | ExprApp

ExprApp   ::= ExprApp ExprProj                              -- function application
            | ExprProj

ExprProj  ::= ExprProj '!#' LowerName                      -- record projection
            | ExprAtom

ExprAtom  ::= Var | Con | Lit
            | '(' Op ')'                                     -- operator section (prefix)
            | '(' Op Expr ')'                                -- right section
            | '(' Expr Op ')'                                -- left section
            | '(' Expr ')'
            | '(' Expr ',' Expr (',' Expr)* ')'             -- tuple literal
            | '(' ')'                                        -- unit literal
            | '{' FieldBind (',' FieldBind)* '}'             -- record literal
            | '{' Expr '|' FieldBind (',' FieldBind)* '}'   -- record update
            | '{' Bind (';' Bind)* ';' Expr '}'              -- block expression
            | Expr '@' TypeAtom                               -- type application

FieldBind ::= LowerName '=' Expr

Stmt      ::= Var '<-' Expr                                  -- bind
            | Var ':=' Expr                                   -- pure let-bind
            | Expr                                            -- execute

Branch    ::= Pattern '->' Expr

Lit       ::= IntLit | StringLit | RuneLit
```

`!#` binds at atom level (tighter than function application).

Three operator section forms exist:

- `(op)` wraps an operator in parentheses to use it as a first-class value (e.g. `foldr (+) 0 xs`). This mirrors the declaration-level `(op) := ...` syntax.
- `(op expr)` is a right section: `(+ 1)` desugars to `\x -> x + 1`. The operator binds the right argument.
- `(expr op)` is a left section: `(1 +)` desugars to `\x -> 1 + x`. The operator binds the left argument.

All three forms produce ordinary values of function type and can be passed to higher-order functions.

Disambiguation of `{`: `ident :=` → block expression, `ident =` → record literal, `expr |` → record update.

## 3.6 Pattern Syntax

```
Pattern   ::= Con Pattern*                                   -- constructor
            | Var                                            -- variable binding
            | '_'                                            -- wildcard
            | '(' Pattern ')'                                -- parenthesized
            | '(' Pattern ',' Pattern (',' Pattern)* ')'     -- tuple pattern
            | '(' ')'                                        -- unit pattern
            | '{' FieldPat (',' FieldPat)* '}'               -- record pattern (open)

FieldPat  ::= LowerName '=' Pattern
```

Record patterns are **open** — partial match is permitted. Unmentioned fields are ignored.

## 3.7 Declaration Syntax

```
Program   ::= Import* Decl*

Import    ::= 'import' ModuleName

Decl      ::= DeclBind | DeclData | DeclType | DeclFixity | DeclClass | DeclInstance

DeclBind  ::= Var '::' Type ';' Var ':=' Expr               -- annotated binding
            | Var ':=' Expr                                   -- unannotated binding

DeclData  ::= 'data' ConName TyBinder* '=' ConDecl ('|' ConDecl)*       -- ADT
            | 'data' ConName TyBinder* '=' '{' GADTCon (';' GADTCon)* '}'  -- GADT

ConDecl   ::= ConName TypeAtom*
GADTCon   ::= ConName '::' Type

DeclType  ::= 'type' ConName TyBinder* '=' Type              -- type alias

DeclFixity ::= ('infixl' | 'infixr' | 'infixn') Int Var

DeclClass ::= 'class' Constraint* ConName TyVar+ '{' ClassMethod (';' ClassMethod)* '}'
ClassMethod ::= VarName '::' Type

DeclInstance ::= 'instance' Constraint* ConName Type+ '{' InstMethod (';' InstMethod)* '}'
InstMethod ::= VarName ':=' Expr
```

## 3.8 Row Syntax

```
RowExpr  ::= '{' '}'                                          -- empty row
           | '{' RowField (',' RowField)* ('|' TyVar)? '}'   -- row

RowField ::= Label ':' Type
```

## 3.9 Operator Fixity

Built-in operators:

| Operator | Fixity | Precedence | Meaning               |
| -------- | ------ | ---------- | --------------------- |
| `.`      | infixr | 9          | Function composition  |
| `*`      | infixl | 7          | Multiplication        |
| `/`      | infixl | 7          | Division              |
| `%`      | infixl | 7          | Modulo                |
| `+`      | infixl | 6          | Addition              |
| `-`      | infixl | 6          | Subtraction           |
| `<>`     | infixr | 6          | Append (Semigroup)    |
| `<$>`    | infixl | 4          | Functor map           |
| `<*>`    | infixl | 4          | Applicative apply     |
| `*>`     | infixl | 4          | Applicative sequence  |
| `<*`     | infixl | 4          | Applicative discard   |
| `==`     | infixn | 4          | Equality              |
| `/=`     | infixn | 4          | Inequality            |
| `<`      | infixn | 4          | Less than             |
| `>`      | infixn | 4          | Greater than          |
| `<=`     | infixn | 4          | Less or equal         |
| `>=`     | infixn | 4          | Greater or equal      |
| `<\|>`   | infixl | 3          | Alternative choice    |
| `>>=`    | infixl | 1          | Monad bind            |
| `>>`     | infixl | 1          | Monad sequence        |
| `<&>`    | infixl | 1          | Flipped Functor map   |
| `=<<`    | infixr | 1          | Flipped Monad bind    |
| `>=>`    | infixr | 1          | Kleisli left-to-right |
| `<=<`    | infixr | 1          | Kleisli right-to-left |

---

# 4. Kind System

## 4.1 Base Kinds

| Kind         | Purpose                                               |
| ------------ | ----------------------------------------------------- |
| `Type`       | Kind of value types                                   |
| `Row`        | Kind of row descriptors (capabilities, record fields) |
| `Constraint` | Kind of type class predicates                         |

## 4.2 Kind Arrows

`Kind -> Kind` classifies type constructors. Examples:

```
Maybe      : Type -> Type
Computation : Row -> Row -> Type -> Type
Record     : Row -> Type
Eq         : Type -> Constraint
```

## 4.3 DataKinds

Every `data` declaration automatically promotes its nullary constructors to the type level:

```
data DBState = Opened | Closed
```

Produces:

| Level | Name               | Classification              |
| ----- | ------------------ | --------------------------- |
| Term  | `Opened`, `Closed` | Constructor, type `DBState` |
| Type  | `Opened`, `Closed` | Type, kind `DBState`        |
| Kind  | `DBState`          | Kind                        |

This enables precise capability tracking:

```
dbOpen :: forall r. Computation { db : DB Closed | r }
                                { db : DB Opened | r }
                                ()
```

Promotion applies only to nullary constructors. Constructors with fields are not promoted (promoting them would require type families — a phase transition).

In type position, names are resolved by: (1) check type constructors, (2) check promoted data constructors, (3) ambiguity error if both match.

## 4.4 Kind Polymorphism (HKT)

Kind variables are introduced with explicit annotation in `forall` binders:

```
forall (k : Kind). forall (f : k -> Type). f a -> f a
```

`Kind` is a distinguished sort — the kind of kinds. Kind variables range over all kinds.

Kind inference uses kind metavariables and unification:

- `KindMeta` metavariables in the ordered context
- Kind occurs check
- Kind substitution (`applyKindSubst`)

Kind variables are explicit and never inferred. Programs without kind variables have unchanged kind inference behavior.

---

# 5. Type System

## 5.1 Bidirectional Checking (DK)

The type checker uses two modes following Dunfield-Krishnaswami:

- **Check mode** (Γ ⊢ e ⇐ A): verify that expression `e` has type `A`
- **Infer mode** (Γ ⊢ e ⇒ A): synthesize the type of expression `e`

The checker maintains an ordered context with union-find for metavariable solving.

### 5.1.1 Subsumption

Higher-rank polymorphism is handled via subsumption:

```
Γ ⊢ e ⇒ ∀a. A
──────────────────
Γ ⊢ e ⇐ A[a := τ]    (instantiation)

Γ ⊢ e ⇐ ∀a. A
──────────────────
Γ, a fresh skolem ⊢ e ⇐ A    (skolemization)
```

Higher-rank types require explicit annotations. The checker never infers a higher-rank type. No impredicativity: type variables are instantiated only with monotypes.

## 5.2 Row Unification

Rows unify structurally with label-set matching.

### 5.2.1 Algorithm

Given rows `{ l₁ : T₁, ... | r₁ }` and `{ m₁ : S₁, ... | r₂ }`:

1. **Normalize** both rows by collecting labels and tail.
2. **Classify** labels into three groups: shared (in both), left-only, right-only.
3. For shared labels, unify corresponding types: `Tᵢ ~ Sⱼ`.
4. **Tail resolution**:
   - Both closed, no excess → done.
   - One side has excess, other has open tail → solve tail = excess fields.
   - Both have open tails → introduce fresh tail variable, solve both tails.
5. Apply resulting substitution.

### 5.2.2 Occurs Check

Row unification includes an occurs check to prevent infinite types:

```
r ~ { l : T | r }    -- rejected: r occurs in its own definition
```

## 5.3 Qualified Types

The `=>` token introduces constraints in type expressions:

```
f :: forall a. Eq a => a -> a -> Bool
```

Multiple constraints use curried form:

```
g :: forall a b. Eq a => Ord b => a -> b -> Bool
```

Constraints elaborate to implicit dictionary arguments via dictionary passing.

## 5.4 Implicit forall

Top-level type annotations with free type variables get an implicit outer `forall`:

```
id :: a -> a
-- equivalent to: id :: forall a. a -> a
```

## 5.5 Pattern Matching Exhaustiveness

The exhaustiveness checker uses the Maranget algorithm. For GADTs, a constructor is **relevant** for a scrutinee type `T` if its return type can unify with `T`. Irrelevant constructors need not be covered:

```
eval :: Expr Bool -> Bool
eval := \e -> case e {
  BoolLit b -> b;
  If c t f  -> ...
}
-- IntLit is irrelevant: Expr Int does not unify with Expr Bool
```

---

# 6. Type Classes

## 6.1 Declaration

```
class <Constraint>* <Name> <tyvar>+ {
  <method> :: <Type> ;
  ...
}
```

Superclass constraints precede the class name:

```
class Eq a => Ord a {
  compare :: a -> a -> Ordering
}
```

Type class parameters may be kind-polymorphic:

```
class Functor (f : k -> Type) {
  fmap :: forall a b. (a -> b) -> f a -> f b
}
```

A type class `C` with `n` parameters has kind `T₁ -> ... -> Tₙ -> Constraint`.

## 6.2 Instance Declaration

```
instance <Constraint>* <Name> <Type>+ {
  <method> := <Expr> ;
  ...
}
```

No default methods — every method must be defined in every instance. Orphan instances are allowed (controlled namespace). No overlapping instances.

## 6.3 Elaboration (Dictionary Passing)

Type classes elaborate entirely to existing Core IR constructs. No new Core nodes are needed.

**Class → Data Type + Selectors:**

```
class Eq a { eq :: a -> a -> Bool }
-- elaborates to:
data Eq$Dict a = Eq$MkDict (a -> a -> Bool)
eq :: forall a. Eq$Dict a -> a -> a -> Bool
```

A class with `n` methods and `m` superclasses produces a data type with one constructor of arity `m + n`. The first `m` fields are superclass dictionaries.

**Instance → Dictionary Value:**

```
instance Eq Bool { eq := ... }
-- elaborates to:
eq$Bool :: Eq$Dict Bool
eq$Bool := Eq$MkDict (...)
```

**Constrained Instance → Dictionary Function:**

```
instance Eq a => Eq (Maybe a) { ... }
-- elaborates to:
eq$Maybe :: forall a. Eq$Dict a -> Eq$Dict (Maybe a)
```

**Call Site → Dictionary Insertion:**

```
eq True False
-- elaborates to:
(eq @Bool) eq$Bool True False
```

## 6.4 Instance Resolution

For a goal `C T₁ ... Tₙ`:

1. Search all in-scope instances for `C`.
2. For each instance `instance Ctx => C S₁ ... Sₙ`, attempt to unify `Tᵢ ~ Sᵢ`.
3. Exactly one match → use it, recursively resolve context constraints.
4. Zero matches → error. Multiple matches → error (no overlapping instances).

No backtracking — resolution is greedy. Instance resolution matches kind arguments structurally for poly-kinded classes.

## 6.5 Class Hierarchy

```
Eq ──→ Ord

Show   (independent)

Semigroup ──→ Monoid

Functor ──→ Applicative ──→ Alternative
                         ──→ Monad
Functor ─┐
          ├──→ Traversable
Foldable ┘

IxMonad   (independent — indexed monadic interface)
```

12 type classes total:

| Class         | Parameters                       | Key Methods                                                 |
| ------------- | -------------------------------- | ----------------------------------------------------------- |
| `Eq`          | `a`                              | `eq :: a -> a -> Bool`                                      |
| `Ord`         | `a` (requires Eq)                | `compare :: a -> a -> Ordering`                             |
| `Show`        | `a`                              | `show :: a -> String`                                       |
| `Semigroup`   | `a`                              | `append :: a -> a -> a`                                     |
| `Monoid`      | `a` (requires Semigroup)         | `empty :: a`                                                |
| `Functor`     | `f : k -> Type`                  | `fmap :: (a -> b) -> f a -> f b`                            |
| `Foldable`    | `t`                              | `foldr :: (a -> b -> b) -> b -> t a -> b`                   |
| `Applicative` | `f` (requires Functor)           | `wrap :: a -> f a`, `ap :: f (a -> b) -> f a -> f b`        |
| `Alternative` | `f` (requires Applicative)       | `none :: f a`, `alt :: f a -> f a -> f a`                   |
| `Monad`       | `m : Type -> Type`               | `mpure :: a -> m a`, `mbind :: m a -> (a -> m b) -> m b`    |
| `Traversable` | `t` (requires Functor, Foldable) | `traverse :: Applicative f => (a -> f b) -> t a -> f (t b)` |
| `IxMonad`     | `m : Row -> Row -> Type -> Type` | `ixpure`, `ixbind`                                          |

`Applicative.wrap` corresponds to Haskell's `pure` but uses a different name to avoid collision with the language built-in `pure`. `Monad.mpure` and `Monad.mbind` similarly avoid collision with the built-in `pure` and `bind`.

## 6.6 Interaction with Computation Types

Constraints are value-level functions (dictionary arguments). They compose freely with `Computation`:

```
f :: forall a. Eq a => a -> Computation {} {} Bool
-- elaborates to:
f :: forall a. Eq$Dict a -> a -> Computation {} {} Bool
```

Constraints do not affect the `pre`/`post` row structure.

---

# 7. Algebraic Data Types

## 7.1 ADT Syntax

```
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
data Ordering = LT | EQ | GT
```

## 7.2 GADT Syntax

GADTs use `= {` to distinguish from regular ADTs:

```
data Expr a = {
  BoolLit :: Bool -> Expr Bool;
  IntLit  :: Int -> Expr Int;
  If      :: Expr Bool -> Expr a -> Expr a -> Expr a
}
```

### 7.2.1 Type Equality Refinement

When pattern matching on a GADT constructor, the checker introduces local type equalities derived from unifying the scrutinee type with the constructor's return type.

### 7.2.2 Existential Types

GADT constructors may introduce type variables not appearing in the return type — these are existentially quantified:

```
data SomeEq = {
  MkSomeEq :: forall a. Eq a => a -> SomeEq
}
```

When pattern matching on an existential constructor, the hidden type variable is introduced as a fresh skolem. Packed constraints become available in the branch body. The existential must not escape the branch scope.

Existential variables must be explicitly quantified with `forall`. No first-class existential types outside of constructors.

## 7.3 Elaboration

GADTs elaborate to the same Core IR as regular ADTs. The refined typing is enforced during checking and erased at runtime. Pattern matching is identical at runtime.

---

# 8. Records and Tuples

## 8.1 Record Type

`Record` is a built-in type constructor of kind `Row → Type`:

```
Record { x : Int, y : Bool }
Record { x : Int | r }
Record {}
```

## 8.2 Record Literals

```
{ x = 1, y = True }
{}
```

A record literal `{ l₁ = e₁, ..., lₙ = eₙ }` has type `Record { l₁ : T₁, ..., lₙ : Tₙ }`.

## 8.3 Projection

The `!#` operator projects a field from a record:

```
r!#x            -- project field x
r!#x!#y         -- chained projection (left-associative, atom-level precedence)
f r!#x          -- f (r!#x) — projection binds tighter than application
```

Typing rule: if `e : Record { l : T | r }`, then `e!#l : T`.

## 8.4 Update

```
{ r | x = 42 }
{ r | x = 42, y = True }
```

The field must exist in the original record.

## 8.5 Record Patterns

Record patterns are open (partial match permitted):

```
\{ x = a, y = b } -> a
\{ x = a } -> a               -- other fields ignored
case r { { x = a, y = b } -> a }
{ x = n } := r                -- block binding destructuring
```

## 8.6 Field Order

Field order is semantically irrelevant (Row property). `{ x = 1, y = 2 }` and `{ y = 2, x = 1 }` are equal.

## 8.7 Tuples

Tuples are syntactic sugar for records with positional labels `_1`, `_2`, `_3`, ...

| Surface            | Desugars to                      |
| ------------------ | -------------------------------- |
| `(1, True)`        | `{ _1 = 1, _2 = True }`          |
| `(Int, Bool)`      | `Record { _1 : Int, _2 : Bool }` |
| `t!#_1`            | record projection on `_1`        |
| `(a, b)` (pattern) | `{ _1 = a, _2 = b }` (pattern)   |

`()` is the 0-tuple, equivalent to the empty record `{}`. It replaces the former `Unit` type.

`(a, b)` replaces the former `Pair a b` type.

`(expr)` with no comma is grouping, not a 1-tuple.

## 8.8 Elaboration

Records elaborate to three Core IR formers:

| Operation  | Core Former    |
| ---------- | -------------- |
| Literal    | `RecordCon`    |
| Projection | `RecordProj`   |
| Update     | `RecordUpdate` |

Runtime representation: `RecordVal` wrapping `map[string]Value`.

---

# 9. Computation Model

## 9.1 do Notation

`do` blocks desugar to `bind` chains:

```
do {
  x <- getLine;
  y <- getLine;
  pure (append x y)
}

-- desugars to:
bind getLine (\x -> bind getLine (\y -> pure (append x y)))
```

A bare expression `e` at the end of a `do` block is the final computation. A bare expression `e` followed by more statements desugars to `bind e (\_ -> ...)`.

## 9.2 Block Expressions

Block expressions `{ x := e; body }` desugar to lambda application:

```
{ x := 1; x + 2 }
-- desugars to:
(\x -> x + 2) 1
```

## 9.3 Recursion

Recursion is opt-in. The host must call `EnableRecursion()` to permit recursive definitions. Without it, recursive bindings are rejected.

`rec` introduces recursive bindings:

```
rec fac := \n -> case eq n 0 {
  True -> 1;
  False -> n * fac (n - 1)
}
```

`fix` is the fixpoint combinator. `rec f := e` elaborates to `f := fix (\f -> e)`.

## 9.4 Capability State Transitions

Capabilities are tracked in row-typed pre/post states:

```
-- Opens a database (requires Closed, produces Opened)
dbOpen :: forall r. Computation { db : DB Closed | r }
                                { db : DB Opened | r }
                                ()

-- Queries (requires Opened, preserves Opened)
dbQuery :: forall r. String -> Computation { db : DB Opened | r }
                                           { db : DB Opened | r }
                                           (List String)

-- Compose:
program := do {
  dbOpen;
  results <- dbQuery "SELECT ...";
  pure results
}
```

The post-state of each step must match the pre-state of the next — ensured by row unification during type checking.

## 9.5 Effect Encoding

Effects are encoded as capability row patterns, not monad transformers:

```
type Effect r a = Computation r r a     -- state-preserving computation

-- Maybe as effect: fromMaybe uses the fail capability
fromMaybe :: forall a r. Maybe a -> Computation { fail : () | r } { fail : () | r } a

-- State as effect: get/put use the state capability
get :: forall s r. Computation { state : s | r } { state : s | r } s
put :: forall s r. s -> Computation { state : s | r } { state : s | r } ()
```

---

# 10. Core IR

The Core intermediate representation has **17 formers**:

| Former         | Category     | Description                    |
| -------------- | ------------ | ------------------------------ |
| `Var`          | Variable     | Variable reference             |
| `Lam`          | Function     | Lambda abstraction             |
| `App`          | Function     | Function application           |
| `TyLam`        | Polymorphism | Type abstraction               |
| `TyApp`        | Polymorphism | Type application               |
| `Con`          | Data         | Constructor application        |
| `Case`         | Data         | Pattern matching               |
| `LetRec`       | Binding      | Recursive let binding          |
| `Pure`         | Computation  | Lift value to computation (F)  |
| `Bind`         | Computation  | Sequence computations          |
| `Thunk`        | Computation  | Suspend computation (U)        |
| `Force`        | Computation  | Resume thunked computation     |
| `PrimOp`       | Primitive    | Host-provided operation        |
| `Lit`          | Literal      | Integer, String, Rune literals |
| `RecordCon`    | Record       | Record construction            |
| `RecordProj`   | Record       | Field projection               |
| `RecordUpdate` | Record       | Field update                   |

Type classes, GADTs, DataKinds, modules, and constraints all elaborate to these formers — no additional Core nodes are needed.

---

# 11. Evaluation

## 11.1 Strategy

Strict call-by-value (CBV) evaluation with an environment-based evaluator.

## 11.2 Runtime Values

| Value       | Description                                        |
| ----------- | -------------------------------------------------- |
| `HostVal`   | Opaque Go values (`int64`, `string`, `rune`, etc.) |
| `Closure`   | Functions with captured environment                |
| `ConVal`    | Constructor applications                           |
| `ThunkVal`  | Suspended computations                             |
| `RecordVal` | Record values (`map[string]Value`)                 |

## 11.3 Defense-in-Depth Limits

The evaluator enforces multiple layers of protection:

- **Step limit**: maximum number of evaluation steps (default: 100,000)
- **Depth limit**: maximum call stack depth (default: 1,000)
- **Context cancellation**: Go `context.Context` integration for timeout
- **Recursion guard**: recursive definitions rejected unless `EnableRecursion()` is called

These limits are orthogonal and cumulative — the strictest applicable limit applies.

## 11.4 Capability Environment

The `CapEnv` is a mapping from capability labels to runtime values, threaded through computation evaluation. Each `bind` step passes the updated `CapEnv` from the completed computation to the continuation.

---

# 12. Module System

## 12.1 Import

```
import ModuleName
```

`import` must appear at the top of a source file, before any other declarations. All top-level definitions from the module are brought into scope (no qualified names, no selective imports).

## 12.2 Host API

```go
eng.RegisterModule("MyLib", source)        // Register module from source string
eng.RegisterModuleFile("path/to/mod.gicel")  // Register module from file
```

Modules are parsed and type-checked at registration time. Circular imports are forbidden (topological sort at registration). Name collisions between imported modules are an error at the import site.

## 12.3 Prelude

The Prelude is split into two parts:

- **Core** (not replaceable): language-essential definitions — `IxMonad` class, `Computation` instance, `Effect` alias, `then` combinator, `Lift` type alias
- **Prelude** (replaceable): standard library types, classes, instances — `Bool`, `Maybe`, `List`, `Ordering`, all 12 type classes, instances

```go
eng.SetPrelude(customSource)  // Replace default Prelude with custom source
```

The prelude is implicitly imported. `NoPrelude()` suppresses it.

## 12.4 No Export Control

All top-level definitions in a module are exported. Selective exports are a future direction.

---

# 13. Host Boundary

## 13.1 Three-Tier Lifecycle

| Tier          | Mutability                | Purpose                                                                       |
| ------------- | ------------------------- | ----------------------------------------------------------------------------- |
| **Engine**    | Mutable                   | Configuration: register types, assumptions, primitives, modules, stdlib packs |
| **Runtime**   | Immutable, goroutine-safe | Compiled program: type-checked Core IR ready for execution                    |
| **Evaluator** | Per-execution             | Fresh evaluation state with limits, hooks, capability environment             |

## 13.2 Engine API

```go
eng := gicel.NewEngine()

// Type registration (DB : Type -> Type)
eng.RegisterType("DB", gicel.KindArrow(gicel.KindType(), gicel.KindType()))

// Assumption (effect declaration) — type must be constructed with helpers
dbOpenTy := gicel.ForallRow("r",
    gicel.ArrowType(
        gicel.ConType("()"),
        gicel.CompType(
            gicel.NewRow().And("db", gicel.AppType(gicel.ConType("DB"), gicel.ConType("Closed"))).Open("r"),
            gicel.NewRow().And("db", gicel.AppType(gicel.ConType("DB"), gicel.ConType("Opened"))).Open("r"),
            gicel.ConType("()"))))
eng.DeclareAssumption("dbOpen", dbOpenTy)

// Primitive implementation — PrimImpl is a function type
eng.RegisterPrim("dbOpen", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, apply gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
    return gicel.ToValue(nil), capEnv.Set("db", "opened"), nil
})

// Stdlib packs
eng.Use(gicel.Num)    // Num class, arithmetic operators
eng.Use(gicel.Str)    // String/Rune operations
eng.Use(gicel.List)   // List operations
eng.Use(gicel.Fail)   // fail capability
eng.Use(gicel.State)  // get/put capabilities
eng.Use(gicel.IO)     // print/debug via CapEnv buffer
```

A stdlib pack is `func(Registrar) error` — it bundles `RegisterType` + `RegisterModule` + `RegisterPrim`. A pack is not a module; it is a Go-side configuration action.

## 13.3 Runtime Compilation

```go
rt, err := eng.NewRuntime(source)
```

## 13.4 Execution

```go
result, err := rt.RunWith(ctx, &gicel.RunOptions{
    Caps:     caps,
    Bindings: bindings,
})
// result.Value, result.CapEnv, result.Stats
```

## 13.5 Sandbox API

```go
result, err := gicel.RunSandbox(source, &gicel.SandboxConfig{
    Packs:    []gicel.Pack{gicel.Num, gicel.Str},
    Timeout:  3 * time.Second,
    MaxSteps: 50_000,
    MaxAlloc: 10 * 1024 * 1024,
})
```

Single-call compile+execute for AI agents.

## 13.6 Primitive Implementation

```go
type PrimImpl func(ctx context.Context, capEnv CapEnv, args []Value, apply Applier) (Value, CapEnv, error)

type Applier func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error)
```

The `Applier` callback enables higher-order primitives (e.g., `foldl`). It applies a GICEL function value to an argument within the current capability environment.

---

# 14. Evidence System

## 14.1 Overview

The type checker uses a unified evidence architecture. Capability rows and constraint rows share the same underlying representation: `TyEvidenceRow` parameterized by an extensible `EvidenceFiber` interface.

## 14.2 Structure

```
TyEvidenceRow {
    Fiber   EvidenceEntries (interface)
    Fields  []EvidenceField
    Tail    Type
}
```

Two fibers are implemented:

- **CapabilityEntries** — capability rows (pre/post states of `Computation`)
- **ConstraintEntries** — constraint rows (type class predicates)

New fibers can be added by implementing the `EvidenceEntries` interface.

## 14.3 Unified Unification

`unifyRows` and `unifyConstraintRows` are unified into a single `unifyEvidenceRows` algorithm. The fiber interface provides label matching and entry unification; the shared algorithm handles tail resolution, occurs check, and substitution.

## 14.4 Invariant

This is an internal refactoring with no user-visible changes. All programs type-check identically to the pre-unification state.

---

# 15. Standard Library

## 15.1 Prelude Types

```
data Bool = True | False
data Maybe a = Just a | Nothing
data List a = Cons a (List a) | Nil
data Ordering = LT | EQ | GT
data Result e a = Ok a | Err e

type Effect r a = Computation r r a

fst :: forall a b. (a, b) -> a
snd :: forall a b. (a, b) -> b
```

`()` (unit) is the empty record. `(a, b)` (tuple) is record sugar.

## 15.2 Stdlib Packs

| Pack     | Provides                                                                       |
| -------- | ------------------------------------------------------------------------------ |
| `Num`    | `Num` class, `Eq`/`Ord` Int, arithmetic operators (`+`, `-`, `*`, `/`, `%`)    |
| `Str`    | `Eq`/`Ord`/`Semigroup`/`Monoid` String, `Eq`/`Ord` Rune                        |
| `List`   | `fromSlice`, `toSlice`, `length`, `concat`, `foldl`                            |
| `Fail`   | `fail` capability, `fromMaybe`, `fromResult`                                   |
| `State`  | `get`/`put` capabilities                                                       |
| `IO`     | `print`/`debug` via CapEnv buffer                                              |
| `Stream` | Lazy list: `LCons`/`LNil`, `headS`, `tailS`, `takeS`, `dropS`                  |
| `Slice`  | Contiguous array: O(1) `sliceLength`/`sliceIndex`, `Functor`/`Foldable`        |
| `Map`    | Ordered immutable map (AVL): `insert`, `mapLookup`, `delete`, `fromList`       |
| `Set`    | Ordered immutable set (backed by Map): `setInsert`, `setMember`, `setFromList` |

Types (`Int`, `String`, `Rune`) are checker built-ins; operations come from stdlib packs. Runtime representation: `HostVal` wrapping Go values (`int64`, `string`, `rune`).

---

# 16. Open Design Fork Points

| Fork Point                                         | Current State              | Decision Trigger                                                                   |
| -------------------------------------------------- | -------------------------- | ---------------------------------------------------------------------------------- |
| Branching with divergent post-states               | Equal post-states required | User demand for `if`-like branching where branches modify capabilities differently |
| `Row` as built-in kind vs general structured-index | Built-in kind              | Need for non-capability indexing (e.g., session types)                             |
| Usage judgment (linear/affine capabilities)        | Not implemented            | Graded Evidence (Level 10) design                                                  |
| Algebraic effects/handlers vs indexed monad        | Indexed monad (Atkey)      | Evidence that handler-based approach better serves the AI agent use case           |

---

# 17. Extension Assessment

| Extension                | Classification   | Prerequisite                         |
| ------------------------ | ---------------- | ------------------------------------ |
| Type Families            | Phase transition | Substantial checker changes          |
| Refinement Types         | Phase transition | Separate analysis                    |
| Dependent Types          | Full restructure | Far future                           |
| Graded Evidence          | Addition         | Unified evidence architecture (done) |
| Selective module exports | Refinement       | Module system maturity               |
