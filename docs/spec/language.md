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

**Values** are pure, inert data: integers, strings, functions, form constructors, records. They are classified by types and evaluated without side effects.

**Computations** are typed transitions over capability environments. They may read, modify, or consume capabilities provided by the host. They are classified by the `Computation` type, which tracks the required and resulting capability state. A computation, once constructed, is an action awaiting execution — it is not inert data.

This split follows Call-By-Push-Value (Levy 1999). The two directions of the adjunction are:

- **F** (`pure`): lifts a value into a computation — introduction of computations from values
- **U** (`thunk`): suspends a computation into a value — introduction of suspended computations
- **force**: resumes a suspended computation — elimination of thunks

`bind` provides computation sequencing.

**Top-level binding rule.** Because a computation is not a value, a non-entry top-level binding cannot have bare `Computation` type (error E0291). The designated entry point (default `main`) is exempt — it is the single site where the host triggers execution. All other top-level computations must be either suspended with `thunk` or deferred behind a lambda:

```
-- OK: thunk suspends the computation as a value
helper := thunk do { ... }

-- OK: lambda is a value; the body runs when called
step := \x. do { ... }

-- ERROR (E0291): bare Computation at top level
helper := do { ... }
```

This rule does not apply to value-typed monads. `List`, `Maybe`, and other types that happen to support `do`-notation via `GIMonad` are values, not computations — they can be bound freely at the top level:

```
-- OK: List is a value type
triples := do { x <- range 1 10; ... }
```

### 2.1.2 pure / bind

The two primitive operations that define the algebra of computation.

```
pure : \a (r: Row) g. a -> Computation @g r r a
bind : \a b g1 g2 g3 (r1: Row) (r2: Row) (r3: Row).
         Computation @g1 r1 r2 a -> (a -> Computation @g2 r2 r3 b) -> Computation @g3 r1 r3 b
```

`pure` lifts a value into a computation that preserves capability state (identity transition). The grade parameter `g` is inferred.

`bind` sequences two computations, composing their state transitions and grades. The post-state of the first must match the pre-state of the second. Grade `g3` is resolved to `GradeCompose g1 g2` when used through `GIMonad`.

Together, these operations form an **Atkey parameterized monad** (Atkey, JFP 2009) and must satisfy three laws:

```
bind (pure a) f       =  f a                                  -- left identity
bind m pure           =  m                                     -- right identity
bind (bind m f) g     =  bind m (\a. bind (f a) g)             -- associativity
```

These laws are verified by construction in the evaluator, not by the type checker.

`pure` and `bind` are not reserved keywords. They are first-class built-in functions that can be partially applied and passed to higher-order functions (e.g. `map pure xs`). When fully applied, the checker optimizes them to direct Core nodes (`Pure`, `Bind`) for correct capability environment threading. The `GIMonad` type class provides a generalized interface; the `Computation` instance delegates to these built-in functions.

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
thunk: Computation @g pre post a -> Thunk @g pre post a
force: Thunk @g pre post a -> Computation @g pre post a
```

`thunk` suspends a computation without executing it, producing a first-class value. This is the CBPV `U` (thunk) operator.

`force` resumes a suspended computation, executing it in the current capability environment. This is the CBPV elimination of `U`.

Together with `pure` (= F), `thunk`/`force` (= U) complete the CBPV adjunction F ⊣ U:

```
pure  : Value → Computation        -- F: value to computation
thunk: Computation → Value         -- U: computation to value
force: U(Computation) → Computation  -- elimination of U
```

Laws:

```
force (thunk c) = c                 -- thunk/force cancellation
```

Semantics: `thunk` does not evaluate its argument — it captures the computation as a value. `force` triggers evaluation. Thunks are not memoized: forcing the same thunk multiple times executes the computation each time.

**When to use `thunk`.** A `Computation` is an action, not a piece of form — it cannot sit at the top level as a bare binding (see §2.1.1). To define a named computation that runs later, suspend it with `thunk` and `force` it at the call site:

```
helper :: Suspended { state: Int } Int
helper := thunk do {
  n <- get;
  put (n + 1);
  get
}

main := do {
  result <- force helper;
  pure result
}
```

`thunk do { ... }` is the idiomatic form — the parser accepts `do { ... }` as a direct argument to `thunk` without parentheses.

`thunk` and `force` are **term formers** (like `\` or `case`), not functions. They cannot be partially applied. They elaborate to `Core.Thunk` and `Core.Force` respectively.

`thunk`/`force` are part of the evaluation model (the CBPV adjunction), not the computation algebra. They remain term formers regardless of type class design.

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
       | 'Kind'               -- sort of kinds (for kind-polymorphic \)
       | Kind '->' Kind       -- kind arrow
       | UserKind             -- promoted DataKinds (e.g., DBState)
       | KindVar              -- kind variable (explicit, in \ binders)
```

Kind variables are introduced with explicit annotation: `\(k: Kind). ...`. Kind inference uses kind metavariables and unification (occurs check, substitution). Kind variables are never inferred — they must be explicitly bound.

### 2.2.4 Computation Type

```
Computation: g -> Row -> Row -> Type -> Type
```

The sole computation classifier. First argument: grade (from a `GradeAlgebra` instance). Second argument: pre-state (required capability environment). Third argument: post-state (resulting capability environment). Fourth argument: result type.

The type alias `Effect r a := Computation Zero r r a` denotes computations that preserve their capability state with zero grade.

### 2.2.5 Thunk Type

```
Thunk: g -> Row -> Row -> Type -> Type
```

The type of suspended computations. `Thunk @g pre post a` is a value that, when forced, behaves as `Computation @g pre post a`.

The type alias `Suspended r a := Thunk Zero r r a` denotes suspended computations that preserve their capability state with zero grade, mirroring `Effect` for `Computation`.

### 2.2.6 Record Type

```
Record: Row -> Type
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
\a. T              -- type polymorphism
\(r: Row). T      -- row polymorphism
\(k: Kind). T     -- kind polymorphism
```

`\` serves dual purpose: lambda in expression context (`\x. e`) and universal quantification in type context (`\a. T`). Both use `.` as the body separator. There is no ambiguity because the parser knows whether it is in type or expression context. Multi-parameter lambdas are supported: `\x y z. e` desugars to `\x. \y. \z. e`.

Higher-rank polymorphism: `\` quantifiers may appear under arrows, enabling rank-N types. Higher-rank types require explicit annotations. The checker uses subsumption (DK bidirectional approach): skolemization for checking, instantiation for inference.

### 2.3.3 Host Assumption

The sole source of effects. Host-provided operations are declared with `assumption`:

```
dbOpen :: \r. Computation { db: DB Closed | r }
                          { db: DB Opened | r }
                          ()
dbOpen := assumption
```

No ambient authority exists. Every effect requires an explicit capability in the pre-state.

### 2.3.4 Constraint

Type class predicates of kind `Constraint`. Values of kind `Constraint` are type class predicates (e.g., `Eq Bool: Constraint`). Constraints enable qualified polymorphism via dictionary passing — they elaborate to implicit function arguments.

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
case  do  form  lazy  type  impl  infixl  infixr  infixn  import  if  then  else  as  assumption
```

Keywords are listed above (15 total). Note that `pure`, `bind`, `thunk`, `force`, `rec`, and `fix` are **not** keywords — they are ordinary identifiers with built-in meaning. `lazy` introduces lazy co-data declarations where constructor arguments are implicitly wrapped in `Thunk`. `pure`, `bind`, `rec`, and `fix` are first-class functions (can be partially applied and passed to higher-order functions); `thunk` and `force` are term formers (must be fully applied). `as` and `assumption` **are** keywords (reserved by the lexer).

`\` is used for both lambda (`\x. e`) and universal quantification (`\a. T`). Both use `.` as the body separator. The parser disambiguates by context (expression vs. type). Multi-parameter lambdas are supported: `\x y. e` desugars to `\x. \y. e`.

`;` and newline are interchangeable as declaration/statement separators at all levels — both top-level declarations and inside braces (`do`, `case`, GADT bodies).

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

| Token | Meaning                                                                             |
| ----- | ----------------------------------------------------------------------------------- |
| `::`  | Type annotation                                                                     |
| `:=`  | Definition binding                                                                  |
| `->`  | Function type arrow (universe-polymorphic)                                          |
| `-\|` | Type-level application (right-associative, desugars to `TyApp`)                     |
| `=>`  | Constraint arrow / case alternative / evidence injection                            |
| `~`   | Type equality constraint                                                            |
| `\`   | Lambda                                                                              |
| `\|`  | Row extension / record update / case separator                                      |
| `.`   | lambda body separator / quantifier body separator / composition operator (infixr 9) |
| `.#`  | Record projection                                                                   |
| `@`   | Explicit type application                                                           |

## 3.4 Symbol Design

The language uses 9 relational symbols, each corresponding to a distinct judgment form:

| Symbol | Name           | Judgment          | Usage                                                                                     |
| ------ | -------------- | ----------------- | ----------------------------------------------------------------------------------------- |
| `::`   | classification | `Γ ⊢ e : A`       | type annotation, GADT constructor types                                                   |
| `:=`   | definition     | `Γ ⊢ x ≡ e`       | value/type/data/impl definitions                                                          |
| `:`    | attribution    | `l : T`           | record fields, kind annotations, methods                                                  |
| `->`   | implication    | `A ⊃ B`           | function types                                                                            |
| `=>`   | dispatch       | `C ⊢ T` / `P ⇒ E` | constraint qualification, case alternatives, type family alternatives, evidence injection |
| `~`    | equality       | `A ~ B`           | type equality constraints                                                                 |
| `.`    | body           | `λx. e` / `∀a. T` | lambda/forall body separator, composition                                                 |
| `<-`   | bind           | `x ← M`           | monadic bind in do-notation                                                               |
| `\|`   | alternative    | `A ∨ B`           | constructors, row tail, record update                                                     |

`=` is intentionally absent from the language. In programming, `=` is notoriously overloaded across assignment, comparison, definition, and equation. GICEL uses `:=` for definitions exclusively.

`=>` unifies four roles: constraint qualification (`Eq a => ...`), case alternatives (`True => ...`), type-level case alternatives (`List a => a`), and evidence injection (`_inst => expr`). All four share the structure "from this premise/evidence, dispatch to this result." The arrow direction and the fat shape distinguish it from `->` (function type / implication).

## 3.5 Type Syntax

```
Type      ::= '\' TyBinder+ '.' Type
            | Constraint '=>' Type
            | Type '->' Type
            | TypeDashPipe
            | '(' Type ',' Type (',' Type)* ')'     -- tuple type

TypeDashPipe ::= TypeApp '-|' TypeDashPipe         -- right-associative type application
               | TypeApp

TypeApp   ::= TypeApp TypeAtom
            | TypeAtom

TypeAtom  ::= TyVar | TyCon
            | '(' Type ')'
            | RowExpr
            | '#' Label                                    -- label literal
            | 'case' TypeAtom '{' TyAlt (';' TyAlt)* '}'  -- type-level case

TyAlt     ::= TypeApp '=>' TypeCaseBody

TypeCaseBody ::= Type                             -- allows '->' but stops at '=>'

TyBinder  ::= TyVar                          -- kind inferred
            | '(' TyVar ':' Kind ')'          -- kinded

Constraint ::= TypeApp                        -- e.g., Eq a, Ord b

Kind      ::= 'Type' | 'Row' | 'Constraint' | 'Label' | 'Kind'
            | Kind '->' Kind
            | ConName                          -- promoted DataKinds
            | KindVar
```

Precedence of type operators (loosest to tightest):

1. `\ ... .`
2. `~` (type equality constraint, non-associative)
3. `=>` (constraint qualification, right-associative)
4. `->` (function arrow, right-associative, universe-polymorphic: `Type l₁ -> Type l₂ : Type (max l₁ l₂)`)
5. `-|` (type-level application, right-associative, desugars to `TyApp`)
6. Type application by juxtaposition (left-associative)

## 3.6 Expression Syntax

```
Expr      ::= 'do' '{' Stmt+ '}'                           -- do block
            | '\' Pattern+ '.' Expr                         -- lambda (multi-parameter)
            | 'case' Expr '{' Branch (';' Branch)* '}'     -- case analysis
            | Expr '=>' Expr                                -- evidence injection (right-assoc)
            | ExprInfix

ExprInfix ::= ExprInfix Op ExprApp                          -- operator application
            | ExprApp

ExprApp   ::= ExprApp ExprProj                              -- function application
            | ExprProj

ExprProj  ::= ExprProj '.#' LowerName                      -- record projection
            | ExprAtom

ExprApp   ::= ...
            | ExprApp '@' TypeAtom                            -- type application

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
            | '[' Expr (',' Expr)* ']'                        -- list literal
            | '[' ']'                                         -- empty list

FieldBind ::= LowerName ':' Expr

Stmt      ::= Var '<-' Expr                                  -- bind
            | Var ':=' Expr                                   -- pure let-bind
            | Expr                                            -- execute

Branch    ::= Pattern '=>' Expr

Lit       ::= IntLit | DoubleLit | StringLit | RuneLit
```

`.#` binds at atom level (tighter than function application).

`[e1, e2, ...]` desugars to `Cons e1 (Cons e2 (... Nil))` via the `FromList` class. `[]` is `Nil`.

**Special-form identifiers.** `thunk`, `pure`, and `force` are _not_ reserved keywords. They are ordinary identifiers that the type checker recognizes and elaborates to their corresponding Core IR formers (`Thunk`, `Pure`, `Force`) when they appear as the function in an application. Outside of application position, they produce an informative error.

Three operator section forms exist:

- `(op)` wraps an operator in parentheses to use it as a first-class value (e.g. `foldr (+) 0 xs`). This mirrors the declaration-level `(op) := ...` syntax.
- `(op expr)` is a right section: `(+ 1)` desugars to `\x. x + 1`. The operator binds the right argument.
- `(expr op)` is a left section: `(1 +)` desugars to `\x. 1 + x`. The operator binds the left argument.

All three forms produce ordinary values of function type and can be passed to higher-order functions.

`Expr '=>' Expr` is **scoped evidence injection**. It introduces a dictionary value into the local evidence scope for constraint resolution within the body. The left operand must be a value of some instance dictionary type; the right operand is the body where that evidence is available for resolution. Right-associative: `d1 => d2 => expr` means `d1 => (d2 => expr)`. This elaborates to `(\$ev. body) dict` — the evidence becomes a lambda parameter. Deferred constraints are resolved within the evidence scope (before the evidence is popped from the context). See §6.7 for the interaction with private instances.

Disambiguation of `{`: `ident :=` → block expression, `ident :` → record literal, `expr |` → record update.

`Expr '@' TypeAtom` is explicit type application. It passes a type argument to a polymorphic binding or constructor:

```
id @Bool True                     -- instantiate id at Bool
Just @Bool True                   -- instantiate Just at Bool
eq @(Maybe Int) (Just 1) Nothing  -- instantiate eq at Maybe Int
```

Works with any user-defined polymorphic binding, not just built-in forms. The `@` token must immediately follow the expression being applied (no intervening operator).

## 3.7 Pattern Syntax

```
Pattern   ::= Con PatArg*                                    -- constructor
            | Var                                            -- variable binding
            | '_'                                            -- wildcard
            | IntLit                                         -- integer literal
            | DoubleLit                                      -- double literal
            | StringLit                                      -- string literal
            | RuneLit                                        -- rune literal
            | '[' Pattern (',' Pattern)* ']'                 -- list pattern
            | '(' Pattern ')'                                -- parenthesized
            | '(' Pattern ',' Pattern (',' Pattern)* ')'     -- tuple pattern
            | '(' ')'                                        -- unit pattern
            | '{' FieldPat (',' FieldPat)* '}'               -- record pattern (open)

PatArg    ::= Var | '_' | Con | IntLit | DoubleLit | StringLit | RuneLit
            | '(' Pattern ')'                                -- nested pattern (parenthesized)

FieldPat  ::= LowerName ':' Pattern
```

Constructor patterns can be nested. A nullary constructor appearing as an argument to another constructor needs no parentheses; a multi-argument constructor argument must be parenthesized (Haskell convention):

```
case m { Just True => "yes"; Just False => "no"; Nothing => "empty" }
case xs { Cons Nothing rest => rest; Cons (Just x) rest => rest; Nil => Nil }
case m { Just (Just (Just True)) => "deep"; _ => "other" }
```

Literal patterns match by equality on `Int`, `Double`, `String`, and `Rune`. List patterns `[p1, p2, p3]` desugar to `Cons p1 (Cons p2 (Cons p3 Nil))`:

```
case n { 0 => "zero"; 1 => "one"; _ => "other" }
case name { "Alice" => "hello"; _ => "hi" }
case ch { 'a' => True; _ => False }
```

Literal types are open — the exhaustiveness checker cannot enumerate all values of `Int`, `String`, or `Rune`, so a wildcard or variable catch-all is required.

Record patterns are **open** — partial match is permitted. Unmentioned fields are ignored.

## 3.8 Declaration Syntax

```
Program   ::= Import* Decl*

Import    ::= 'import' ModuleName                        -- open
            | 'import' ModuleName '(' ImportList ')'     -- selective
            | 'import' ModuleName 'as' Upper             -- qualified

Decl      ::= DeclBind | DeclData | DeclType | DeclFixity | DeclImpl

DeclBind  ::= Var '::' Type ';' Var ':=' Expr               -- annotated binding
            | Var ':=' Expr                                   -- unannotated binding

DeclData  ::= 'form' ConName ':=' '\' TyBinder+ '.' DataBody          -- parametric
            | 'form' ConName ':=' DataBody                              -- non-parametric
            | 'lazy' ConName ':=' '\' TyBinder+ '.' DataBody           -- lazy co-data (parametric)
            | 'lazy' ConName ':=' DataBody                              -- lazy co-data (non-parametric)

DataBody  ::= '{' ConField (';' ConField)* '}'                         -- GADT or class-like body
            | ConDecl ('|' ConDecl)*                                    -- ADT shorthand

ConField  ::= ConName ':' Type                            -- constructor (uppercase: full type sig)
            | VarName ':' Type                             -- method (lowercase: field declaration)
            | 'type' ConName TyBinder* '::' Kind          -- associated type declaration
            | 'form' ConName TyBinder* '::' Kind          -- associated form declaration

ConDecl   ::= ConName TypeAtom*

DeclType  ::= 'type' ConName '::' Kind ':=' Type           -- type alias or type family

DeclFixity ::= ('infixl' | 'infixr' | 'infixn') Int Var

DeclImpl  ::= 'impl' InstanceName? Constraint* ConName Type+ ':=' '{' ImplMember (';' ImplMember)* '}'

InstanceName ::= '_' Var '::' Constraint            -- private named instance

ImplMember ::= VarName ':=' Expr                            -- method definition
             | 'type' ConName ':=' Type                     -- associated type definition
             | 'form' ConName ':=' ConDecl ('|' ConDecl)*   -- associated form definition
```

### 3.8.1 Unified `form` Declaration

The `form` keyword serves three roles, distinguished by body structure:

**ADT shorthand** — constructors separated by `|`, no braces:

```
form Bool := True | False
form Maybe := \a. Just a | Nothing
form List := \a. Cons a (List a) | Nil
```

**GADT-style** — braces with uppercase-named fields declaring full constructor types:

```
form Expr := \a. {
  LitBool: Bool -> Expr Bool;
  LitInt:  Int -> Expr Int;
  If:      Expr Bool -> Expr a -> Expr a -> Expr a
}
```

**Class-like** — braces with lowercase-named fields declaring method signatures. Uses `:=` with lambda parameters and optional superclass constraints before the brace body:

```
form Eq := \a. {
  eq: a -> a -> Bool
}

form Functor := \(f: Type -> Type). {
  fmap: \a b. (a -> b) -> f a -> f b
}
```

Superclass constraints follow the lambda parameters:

```
form Ord := \a. Eq a => {
  compare: a -> a -> Ordering
}

form Applicative := \(f: Type -> Type). Functor f => {
  wrap: \a. a -> f a;
  ap:   \a b. f (a -> b) -> f a -> f b
}
```

Associated type and form declarations may appear inside the class-like body:

```
form Container c {
  type Elem c :: Type;
  cfold: \b. (Elem c -> b -> b) -> b -> c -> b
}
```

### 3.8.2 `impl` Declaration

`impl` provides definitions for a class-like `form` declaration. The `:=` before the body is required:

```
impl Eq Bool := {
  eq := \x y. case x {
    True  => case y { True => True; False => False };
    False => case y { True => False; False => True }
  }
}

impl Eq a => Eq (Maybe a) := {
  eq := \x y. case (x, y) {
    (Nothing, Nothing) => True;
    (Just a, Just b)   => eq a b;
    _                  => False
  }
}
```

Associated type definitions use `:=`:

```
impl Container (List a) := {
  type Elem := a;
  cfold := foldr
}
```

Associated form definitions use `:=`:

```
impl Wrappable Int := {
  form Wrapped := IntBox Int;
  wrap := \n. IntBox n;
  unwrap := \w. case w { IntBox n => n }
}
```

## 3.9 Row Syntax

```
RowExpr  ::= '{' '}'                                          -- empty row
           | '{' RowField (',' RowField)* ('|' TyVar)? '}'   -- row

RowField ::= Label ':' Type ('@' TypeAtom)?                   -- optional multiplicity
```

The optional `@Mult` suffix annotates a field with a multiplicity grade (`@Zero`, `@Linear`, `@Affine`, or `@Unrestricted`). Without annotation, fields default to `@Unrestricted`.

## 3.10 Operator Fixity

Built-in operators:

| Operator | Fixity | Precedence | Meaning                |
| -------- | ------ | ---------- | ---------------------- |
| `.`      | infixr | 9          | Function composition   |
| `*`      | infixl | 7          | Multiplication         |
| `/`      | infixl | 7          | Division               |
| `+`      | infixl | 6          | Addition               |
| `-`      | infixl | 6          | Subtraction            |
| `<>`     | infixr | 6          | Append (Semigroup)     |
| `<$>`    | infixl | 4          | Functor map            |
| `<*>`    | infixl | 4          | Applicative apply      |
| `*>`     | infixl | 4          | Applicative sequence   |
| `<*`     | infixl | 4          | Applicative discard    |
| `==`     | infixn | 4          | Equality               |
| `/=`     | infixn | 4          | Inequality             |
| `<`      | infixn | 4          | Less than              |
| `>`      | infixn | 4          | Greater than           |
| `<=`     | infixn | 4          | Less or equal          |
| `>=`     | infixn | 4          | Greater or equal       |
| `&&`     | infixr | 3          | Boolean AND            |
| `<\|>`   | infixl | 3          | Alternative choice     |
| `\|\|`   | infixr | 2          | Boolean OR             |
| `<+`     | infixr | 5          | Cons (prepend to list) |
| `>>=`    | infixl | 1          | Monad bind             |
| `>>`     | infixl | 1          | Monad sequence         |
| `<&>`    | infixl | 1          | Flipped Functor map    |
| `&`      | infixl | 1          | Reverse application    |
| `=<<`    | infixr | 1          | Flipped Monad bind     |
| `>=>`    | infixr | 1          | Kleisli left-to-right  |
| `<=<`    | infixr | 1          | Kleisli right-to-left  |
| `$`      | infixr | 0          | Low-precedence apply   |

---

# 4. Kind System

## 4.1 Base Kinds

| Kind         | Purpose                                               |
| ------------ | ----------------------------------------------------- |
| `Type`       | Kind of value types                                   |
| `Row`        | Kind of row descriptors (capabilities, record fields) |
| `Constraint` | Kind of type class predicates                         |
| `Label`      | Kind of type-level label literals (#name)             |

## 4.2 Kind Arrows

`Kind -> Kind` classifies type constructors. Examples:

```
Maybe      : Type -> Type
Computation: g -> Row -> Row -> Type -> Type   (grade-parametric)
Record     : Row -> Type
Eq         : Type -> Constraint
```

## 4.3 DataKinds

Every `form` declaration automatically promotes its nullary constructors to the type level:

```
form DBState := Opened | Closed
```

Produces:

| Level | Name               | Classification              |
| ----- | ------------------ | --------------------------- |
| Term  | `Opened`, `Closed` | Constructor, type `DBState` |
| Type  | `Opened`, `Closed` | Type, kind `DBState`        |
| Kind  | `DBState`          | Kind                        |

This enables precise capability tracking:

```
dbOpen :: \r. Computation { db: DB Closed | r }
                          { db: DB Opened | r }
                          ()
```

All constructors — nullary and parameterized — are promoted. A parameterized constructor like `Pi: U -> U -> U` becomes available at the type level with kind `U -> U -> U`, enabling universe decoding patterns:

```
form U :: Kind := { Set: U; Pi: U -> U -> U; }
type Decode :: Type := \(u: U). case u {
  Set => Int;
  (Pi a b) => Decode a -> Decode b
}
```

In type position, names are resolved by: (1) check type constructors, (2) check promoted form constructors, (3) ambiguity error if both match.

## 4.4 Kind Polymorphism (HKT)

Kind variables are introduced with explicit annotation in `\` binders:

```
\(k: Kind). \(f: k -> Type). f a -> f a
```

`Kind` is a distinguished sort — the kind of kinds. Kind variables range over all kinds.

### 4.4.1 Kind Cumulativity

Ground kinds (`Type`, `Row`, `Constraint`, and promoted kinds like `DBState`) are sub-kinds of `Kind`. This enables kind-polymorphic binders to accept any ground kind:

```
type Id := \(k: Kind) (a: k). a
type T1 := Id Type Int         -- k inferred as Type
type T2 := Id Row { x: Int }   -- k inferred as Row
type T3 := Id Bool True         -- k inferred as Bool (promoted)
```

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

Given rows `{ l₁: T₁, ... | r₁ }` and `{ m₁: S₁, ... | r₂ }`:

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
r ~ { l: T | r }    -- rejected: r occurs in its own definition
```

## 5.3 Qualified Types

The `=>` token introduces constraints in type expressions:

```
f :: \a. Eq a => a -> a -> Bool
```

Multiple constraints use tuple form:

```
g :: \a b. (Eq a, Ord b) => a -> b -> Bool
```

Constraints elaborate to implicit dictionary arguments via dictionary passing.

## 5.4 Implicit Quantification

Top-level type annotations with free type variables get an implicit outer `\`:

```
id :: a -> a
-- equivalent to: id :: \a. a -> a
```

## 5.5 Pattern Matching Exhaustiveness

The exhaustiveness checker uses the Maranget algorithm. For GADTs, a constructor is **relevant** for a scrutinee type `T` if its return type can unify with `T`. Irrelevant constructors need not be covered:

```
eval :: Expr Bool -> Bool
eval := \e. case e {
  BoolLit b => b;
  If c t f  => ...
}
-- IntLit is irrelevant: Expr Int does not unify with Expr Bool
```

For literal patterns on `Int`, `Double`, `String`, and `Rune`, the type's value space is open (cannot be enumerated), so a wildcard or variable catch-all is always required. Omitting the catch-all produces a non-exhaustive match error.

**Conservative fallbacks.** The exhaustiveness checker assumes "covered" (no warning) in two cases where precise analysis is impractical: (1) pattern depth exceeding the internal limit (32 levels of nesting), and (2) opaque types whose constructors are not visible to the checker. In both cases, a non-exhaustive match that escapes static checking will produce a runtime pattern-match error. This is a defense-in-depth design: the static checker is best-effort, and the runtime provides a safety net.

---

# 6. Type Classes

## 6.1 Declaration

Type classes are declared using `form` with all-lowercase method fields:

```
form <Name> <tyvar>+ <Constraint>* {
  <method>: <Type> ;
  ...
}
```

Superclass constraints follow the parameters:

```
form Ord a Eq a => {
  compare: a -> a -> Ordering
}
```

Type class parameters may be kind-polymorphic:

```
form Functor (f: Type -> Type) {
  fmap: \a b. (a -> b) -> f a -> f b
}
```

A type class `C` with `n` parameters has kind `T₁ -> ... -> Tₙ -> Constraint`.

## 6.2 Instance Declaration

Instances are declared with `impl`:

```
impl <Constraint>* <Name> <Type>+ := {
  <method> := <Expr> ;
  ...
}
```

No default methods — every method must be defined in every instance. Orphan instances are allowed (controlled namespace). No overlapping instances.

### 6.2.1 Private Instances

An instance whose name starts with `_` is **private**:

```
impl _alwaysEq :: Eq Int := {
  eq := \x y. 42
}
```

Private instances are:

- **Solver-invisible globally** — the constraint solver never selects them during automatic resolution. They cannot overlap with public instances (separate namespace).
- **Not exported** across module boundaries — importing modules cannot see them.
- **Accessible by name** for value-level reference (e.g., `_alwaysEq` is a binding of type `Eq$Dict Int`).
- **Usable via evidence injection** — `_alwaysEq => eq 1 2` makes the private instance available for resolution within the body expression (see §6.7).

Private instances enable local instance overriding without breaking global coherence.

## 6.3 Elaboration (Dictionary Passing)

Type classes elaborate entirely to existing Core IR constructs. No new Core nodes are needed.

**Class → Data Type + Selectors:**

```
form Eq a { eq: a -> a -> Bool }
-- elaborates to:
form Eq$Dict a := Eq$MkDict (a -> a -> Bool)
eq :: \a. Eq$Dict a -> a -> a -> Bool
```

A class with `n` methods and `m` superclasses produces a form type with one constructor of arity `m + n`. The first `m` fields are superclass dictionaries.

**Instance → Dictionary Value:**

```
impl Eq Bool := { eq := ... }
-- elaborates to:
eq$Bool :: Eq$Dict Bool
eq$Bool := Eq$MkDict (...)
```

**Constrained Instance → Dictionary Function:**

```
impl Eq a => Eq (Maybe a) := { ... }
-- elaborates to:
eq$Maybe :: \a. Eq$Dict a -> Eq$Dict (Maybe a)
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
2. For each `impl Ctx => C S₁ ... Sₙ`, attempt to unify `Tᵢ ~ Sᵢ`.
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

GradeAlgebra   (independent — grade lattice, Core)
UsageSemiring  (independent — value-level grade arithmetic, Core)
GIMonad        (independent — graded indexed monad, requires GradeAlgebra, Core)
Packed         (independent — collection packing)
Read           (independent)
Bifunctor      (independent)

Eq ──→ Num ──→ Div   (in Prelude)
```

Type classes (3 Core + 18 Prelude = 21 total):

| Class           | Parameters                                       | Key Methods                                                       |
| --------------- | ------------------------------------------------ | ----------------------------------------------------------------- |
| `GradeAlgebra`  | `(g: Kind)`                                      | assoc types: `GradeJoin`, `GradeCompose`, `GradeDrop` (Core)      |
| `UsageSemiring` | `(s: Type)`                                      | `zero`, `one`, `plus`, `mult` (Core)                              |
| `GIMonad`       | `(g: Kind) (m: g -> Row -> Row -> Type -> Type)` | `gipure`, `gibind` (Core)                                         |
| `Eq`            | `a`                                              | `eq: a -> a -> Bool`                                              |
| `Ord`           | `a` (requires Eq)                                | `compare: a -> a -> Ordering`                                     |
| `Num`           | `a` (requires Eq)                                | `add`, `sub`, `mul`, `negate`                                     |
| `Div`           | `a` (requires Num)                               | `div: a -> a -> a`                                                |
| `Show`          | `a`                                              | `show: a -> String`                                               |
| `Semigroup`     | `a`                                              | `append: a -> a -> a`                                             |
| `Monoid`        | `a` (requires Semigroup)                         | `empty: a`                                                        |
| `Functor`       | `f : Type -> Type`                               | `fmap: \a b. (a -> b) -> f a -> f b`                              |
| `Foldable`      | `t`                                              | `foldr: \a b. (a -> b -> b) -> b -> t a -> b`                     |
| `Applicative`   | `f` (requires Functor)                           | `wrap: \a. a -> f a`, `ap: \a b. f (a -> b) -> f a -> f b`        |
| `Alternative`   | `f` (requires Applicative)                       | `none: \a. f a`, `alt: \a. f a -> f a -> f a`                     |
| `Monad`         | `m: Type -> Type`                                | `mpure: \a. a -> m a`, `mbind: \a b. m a -> (a -> m b) -> m b`    |
| `Traversable`   | `t` (requires Functor, Foldable)                 | `traverse: \f a b. Applicative f => (a -> f b) -> t a -> f (t b)` |
| `Packed`        | `c`, `e`                                         | `pack: Slice e -> c`, `unpack: c -> Slice e`                      |
| `FromList`      | `l` (assoc type: `Elem l`)                       | `fromList: List (Elem l) -> l`                                    |
| `ToList`        | `l` (requires FromList)                          | `toList: l -> List (Elem l)`                                      |

`Num` and `Div` are type classes with instances for both `Int` and `Double`. Arithmetic operators `+`, `-`, `*` dispatch through `Num`; `/` dispatches through `Div`. Integer overflow wraps silently using Go's `int64` two's-complement semantics. Division by zero is a runtime error.

`Applicative.wrap` corresponds to Haskell's `pure` but uses a different name to avoid collision with the language built-in `pure`. `Monad.mpure` and `Monad.mbind` similarly avoid collision with the built-in `pure` and `bind`.

## 6.6 Interaction with Computation Types

Constraints are value-level functions (dictionary arguments). They compose freely with `Computation`:

```
f :: \a. Eq a => a -> Effect {} Bool
-- elaborates to:
f :: \a. Eq$Dict a -> a -> Effect {} Bool
```

Constraints do not affect the `pre`/`post` row structure.

## 6.7 Scoped Evidence Injection

The `value => expr` form introduces a dictionary value into the local evidence context for the duration of `expr`. The solver can use this evidence when resolving constraints emitted within the body.

```
impl _alwaysEq :: Eq Int := {
  eq := \x y. 42
}

result := _alwaysEq => eq 1 2    -- resolves Eq Int via _alwaysEq, returns 42
```

Multiple injections compose right-associatively:

```
d1 => d2 => expr                 -- d1 => (d2 => expr)
```

Elaboration: `dict => body` elaborates to `(\$ev. body) dict`, where `$ev` is a fresh evidence variable available to the solver during body checking. Deferred constraints are resolved within the evidence scope — the solver sees the injected evidence before it is popped from the context.

This mechanism is the primary use site for private instances (§6.2.1). Public instances are resolved automatically by the solver; private instances require explicit injection to take effect.

---

# 7. Algebraic Data Types

## 7.1 ADT Shorthand

ADT shorthand uses `|`-separated constructors:

```
form Maybe := \a. Just a | Nothing
form List := \a. Cons a (List a) | Nil
form Ordering := LT | EQ | GT
```

Non-parametric ADTs omit the lambda:

```
form Bool := True | False
form Ordering := LT | EQ | GT
```

## 7.2 GADT Syntax

GADTs use `:= {` with full constructor type signatures. Each constructor declares its complete type including the return type:

```
form Expr := \a. {
  LitBool: Bool -> Expr Bool;
  LitInt:  Int -> Expr Int;
  If:      Expr Bool -> Expr a -> Expr a -> Expr a
}
```

The arrow chain convention (Lean/Agda style) specifies constructor fields. The checker decomposes the full type signature to extract field types and return type.

Nullary constructors declare their return type directly without arrows:

```
form List := \a. {
  Nil:  List a;
  Cons: a -> List a -> List a
}
```

### 7.2.1 Type Equality Refinement

When pattern matching on a GADT constructor, the checker introduces local type equalities derived from unifying the scrutinee type with the constructor's return type.

### 7.2.2 Existential Types

GADT constructors may introduce type variables not appearing in the return type — these are existentially quantified:

```
form SomeEq := {
  MkSomeEq: \a. Eq a => a -> SomeEq
}
```

When pattern matching on an existential constructor, the hidden type variable is introduced as a fresh skolem. Packed constraints become available in the branch body. The existential must not escape the branch scope.

Existential variables must be explicitly quantified with `\`. No first-class existential types outside of constructors.

## 7.3 Elaboration

GADTs elaborate to the same Core IR as regular ADTs. The refined typing is enforced during checking and erased at runtime. Pattern matching is identical at runtime.

---

# 8. Records and Tuples

## 8.1 Record Type

`Record` is a built-in type constructor of kind `Row → Type`:

```
Record { x: Int, y: Bool }
Record { x: Int | r }
Record {}
```

Record fields may have higher-rank types:

```
r :: Record { apply: \a. a -> a }
r := { apply: \x. x }
```

The expected type propagates into the record literal, so the lambda receives the `\a. a -> a` annotation and type-checks at rank 2.

**Bare row unification:** A bare `{ ... }` in type position unifies transparently with `Record { ... }` from expression position. Writing `Record { ... }` explicitly in type annotations is still recommended for clarity, but `{ x: Int }` in a type annotation unifies with `Record { x: Int }` inferred from an expression:

```
f :: { x: Int } -> Int          -- unifies with Record { x: Int } -> Int
f :: Record { x: Int } -> Int   -- explicit form (recommended)
g :: Computation { db: DB } {} ()  -- capability rows (not Record)
```

Duplicate field labels in a record type are rejected at compile time (error E0210):

```
Record { x: Int, x: Bool }     -- compile error: duplicate field "x"
```

## 8.2 Record Literals

```
{ x: 1, y: True }
{}
```

A record literal `{ l₁: e₁, ..., lₙ: eₙ }` has type `Record { l₁: T₁, ..., lₙ: Tₙ }`.

## 8.3 Projection

The `.#` operator projects a field from a record:

```
r.#x            -- project field x
r.#x.#y         -- chained projection (left-associative, atom-level precedence)
f r.#x          -- f (r.#x) — projection binds tighter than application
```

Typing rule: if `e: Record { l: T | r }`, then `e.#l: T`.

## 8.4 Update

```
{ r | x: 42 }
{ r | x: 42, y: True }
```

The field must exist in the original record.

## 8.5 Record Patterns

Record patterns are open (partial match permitted):

```
\{ x: a, y: b }. a
\{ x: a }. a                 -- other fields ignored
case r { { x: a, y: b } => a }
{ x: n } := r                -- block binding destructuring
```

## 8.6 Field Order

Field order is semantically irrelevant (Row property). `{ x: 1, y: 2 }` and `{ y: 2, x: 1 }` are equal.

## 8.7 Tuples

Tuples are syntactic sugar for records with positional labels `_1`, `_2`, `_3`, ...

| Surface            | Desugars to                    |
| ------------------ | ------------------------------ |
| `(1, True)`        | `{ _1: 1, _2: True }`          |
| `(Int, Bool)`      | `Record { _1: Int, _2: Bool }` |
| `t.#_1`            | record projection on `_1`      |
| `(a, b)` (pattern) | `{ _1: a, _2: b }` (pattern)   |

`()` is the 0-tuple, equivalent to the empty record `{}`. It replaces the former `Unit` type.

`(a, b)` replaces the former `Pair a b` type.

`(expr)` with no comma is grouping, not a 1-tuple.

## 8.8 Elaboration

Records elaborate to three Core IR formers:

| Operation  | Core Former    |
| ---------- | -------------- |
| Literal    | `RecordLit`    |
| Projection | `RecordProj`   |
| Update     | `RecordUpdate` |

Runtime representation: `RecordVal` wrapping sorted `[]RecordField` slice.

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
bind getLine (\x. bind getLine (\y. pure (append x y)))
```

A bare expression `e` at the end of a `do` block is the final computation. A bare expression `e` followed by more statements desugars to `bind e (\_. ...)`.

## 9.2 Block Expressions

Block expressions `{ x := e; body }` desugar to lambda application:

```
{ x := 1; x + 2 }
-- desugars to:
(\x. x + 2) 1
```

## 9.3 Recursion

Recursion is opt-in. The host must call `EnableRecursion()` to permit recursive definitions. Without it, recursive bindings are rejected.

`rec` introduces recursive bindings:

```
rec fac := \n. case eq n 0 {
  True => 1;
  False => n * fac (n - 1)
}
```

`fix` is the fixpoint combinator. `rec f := e` elaborates to `f := fix (\f. e)`.

## 9.4 Capability State Transitions

Capabilities are tracked in row-typed pre/post states:

```
-- Opens a database (requires Closed, produces Opened)
dbOpen :: \r. Computation { db: DB Closed | r }
                          { db: DB Opened | r }
                          ()

-- Queries (requires Opened, preserves Opened)
dbQuery :: \r. String -> Computation { db: DB Opened | r }
                                     { db: DB Opened | r }
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
type Effect :: Type := \r a. Computation Zero r r a     -- state-preserving, zero-grade computation
type Suspended :: Type := \r a. Thunk Zero r r a        -- state-preserving, zero-grade suspension

-- Maybe as effect: fromMaybe uses the fail capability
fromMaybe :: \a r. Maybe a -> Effect { fail: () | r } a

-- State as effect: get/put use the state capability
get :: \s r. Effect { state: s | r } s
put :: \s r. s -> Effect { state: s | r } ()
```

---

# 10. Core IR

The Core intermediate representation has **19 formers**:

| Former         | Category     | Description                                     |
| -------------- | ------------ | ----------------------------------------------- |
| `Var`          | Variable     | Variable reference                              |
| `Lam`          | Function     | Lambda abstraction                              |
| `App`          | Function     | Function application                            |
| `TyLam`        | Polymorphism | Type abstraction                                |
| `TyApp`        | Polymorphism | Type application                                |
| `Con`          | Data         | Constructor application                         |
| `Case`         | Data         | Pattern matching                                |
| `Fix`          | Binding      | Fixed-point combinator                          |
| `Pure`         | Computation  | Lift value to computation (F)                   |
| `Bind`         | Computation  | Sequence computations                           |
| `Thunk`        | Computation  | Suspend computation (U)                         |
| `Force`        | Computation  | Resume thunked computation                      |
| `Merge`        | Computation  | SMC parallel composition of computations        |
| `PrimOp`       | Primitive    | Host-provided operation                         |
| `Lit`          | Literal      | Integer, Double, String, Rune literals          |
| `RecordLit`    | Record       | Record construction                             |
| `RecordProj`   | Record       | Field projection                                |
| `RecordUpdate` | Record       | Field update                                    |
| `Error`        | Recovery     | Error placeholder (never reaches the evaluator) |

Type classes, GADTs, DataKinds, modules, and constraints all elaborate to these formers — no additional Core nodes are needed.

---

# 11. Evaluation

## 11.1 Strategy

Strict call-by-value (CBV) evaluation with a bytecode VM (flat fetch-decode-execute cycle, replacing the earlier tree-walking evaluator).

## 11.2 Runtime Values

| Value         | Description                                                                      |
| ------------- | -------------------------------------------------------------------------------- |
| `HostVal`     | Opaque Go values (`int64`, `string`, `rune`, etc.)                               |
| `VMClosure`   | Bytecode functions with captured locals array                                    |
| `ConVal`      | Constructor applications                                                         |
| `VMThunkVal`  | Suspended bytecode computations                                                  |
| `PrimVal`     | Partially or fully applied primitive operations (curried until arity is reached) |
| `RecordVal`   | Record values (sorted `[]RecordField` slice)                                     |
| `IndirectVal` | Forward-reference cell for mutually-recursive top-level bindings                 |

## 11.3 Defense-in-Depth Limits

The VM enforces multiple layers of protection:

- **Step limit**: maximum number of evaluation steps (Engine default: 1,000,000; CLI default: 100,000; Sandbox default: 100,000)
- **Depth limit**: maximum call stack depth (Engine default: 1,000; CLI default: 10,000; Sandbox default: 100)
- **Nesting limit**: maximum structural nesting depth (CLI default: 512; Sandbox default: 256)
- **Allocation limit**: maximum cumulative allocation bytes (CLI default: 100 MiB; Sandbox default: 10 MiB)
- **Context cancellation**: Go `context.Context` integration for timeout
- **Recursion guard**: recursive definitions rejected unless `EnableRecursion()` is called

These limits are orthogonal and cumulative — the strictest applicable limit applies.

## 11.4 Capability Environment

The `CapEnv` is a mapping from capability labels to runtime values, threaded through computation evaluation. Each `bind` step passes the updated `CapEnv` from the completed computation to the continuation.

---

# 12. Module System

## 12.1 Import Forms

Three import forms are supported. For a given module, exactly one form is used; the same module cannot be imported twice.

### Open Import

```
import ModuleName
```

All top-level definitions from the module are brought into scope unqualified. This is the standard form.

### Selective Import

```
import ModuleName (name, T(..), C(A,B), (+))
```

Only the listed names are brought into scope. Import list entries:

| Entry       | Meaning                                          |
| ----------- | ------------------------------------------------ |
| `name`      | Value binding                                    |
| `(op)`      | Operator                                         |
| `Name`      | Type or class (bare — no constructors/methods)   |
| `Name(..)`  | Type or class with all constructors/methods      |
| `Name(A,B)` | Type or class with specific constructors/methods |

Instances are always imported regardless of the selective list (coherence requirement).

### Qualified Import

```
import ModuleName as Alias
```

All names are accessible only through the alias prefix: `Alias.value`, `Alias.Constructor`, `Alias.Type`. Operators cannot be qualified (`Alias.+` is not valid syntax). Instances are always imported (coherence).

Qualified name disambiguation: `N.x` (no whitespace) is a qualified reference; `N . x` (whitespace around `.`) is function composition. The parser uses token span adjacency to disambiguate.

### Constraints

- `import` declarations must appear at the top of a source file, before any other declarations.
- Duplicate imports of the same module are an error.
- Two modules cannot share the same alias (`import A as N; import B as N` is an error).
- `import Core` with selective or qualified form is rejected; Core is implicit and user-invisible.
- `as` is a keyword: the lexer always produces `TokAs`, so it cannot be used as a variable name.

## 12.2 Host API

```go
eng.RegisterModule("MyLib", source)          // Register module from source string
eng.RegisterModuleFile("path/to/mod.gicel")  // Register module from file
```

Modules are parsed and type-checked at registration time. Circular imports are forbidden (topological sort at registration).

CLI multi-file support:

```sh
gicel run --module Util=lib/Util.gicel main.gicel
```

## 12.3 Prelude

The Prelude is split into two parts:

- **Core** (not replaceable): language-essential definitions — `GradeAlgebra` class, `UsageSemiring` class, `GIMonad` class, `Trivial` grade, `Computation` instance, `Effect` alias, `Suspended` alias, `Lift` type alias, `seq` combinator, `merge`/`(***)` parallel composition, `dag`/`Gate` dagger
- **Prelude** (replaceable): standard library types, classes, instances — `Bool`, `Maybe`, `List`, `Ordering`, type classes (Eq through ToList, including Num and Div), instances

Core is auto-registered and auto-imported; the user cannot control it. Prelude requires explicit `Use(Prelude)` on the engine and `import Prelude` in source.

## 12.4 Private Names

Value bindings whose name starts with `_` are module-private and excluded from exports. Importing modules cannot access them through any import form (open, selective, or qualified).

```gicel
_helper :: Int -> Int        -- private: not exported
_helper := \x. x + 1

publicFn :: Int -> Int       -- exported
publicFn := \x. _helper x
```

Instance declarations with a `_` prefix name are also private — they are excluded from exports and from global instance resolution (see §6.2.1):

```gicel
impl _localEq :: Eq Int := { eq := \x y. 42 }   -- private instance: solver-invisible
```

Type names (uppercase) cannot start with `_`, so type-level privacy requires selective exports (a future direction).

## 12.5 Ambiguity Detection

When two open or selective imports bring the same name into scope from different modules, the compiler reports an ambiguity error. The user must use qualified or selective import to disambiguate.

```
import Data.Map    -- exports: empty, insert, delete, size, member, ...
import Data.Set    -- also exports: insert, delete, size, member, ...
-- ERROR: ambiguous name "insert": imported from both Data.Map and Data.Set
```

Re-exported names are not ambiguous. If module A imports module B, A's exports include B's names. When a user imports both A and another module that also imports B, the shared names from B do not conflict — they originate from the same source.

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

// Primitive implementation — PrimImpl receives an Applier struct
eng.RegisterPrim("dbOpen", func(ctx context.Context, capEnv gicel.CapEnv, args []gicel.Value, apply gicel.Applier) (gicel.Value, gicel.CapEnv, error) {
    return gicel.ToValue(nil), capEnv.Set("db", "opened"), nil
})

// Stdlib packs
eng.Use(gicel.Prelude)      // Num/Str/List (import Prelude)
eng.Use(gicel.EffectFail)   // fail capability (import Effect.Fail)
eng.Use(gicel.EffectState)  // get/put capabilities (import Effect.State)
eng.Use(gicel.EffectIO)     // log/dbg via CapEnv buffer (import Effect.IO)
```

A stdlib pack is `func(Registrar) error` — it bundles `RegisterType` + `RegisterModule` + `RegisterPrim`. A pack is not a module; it is a Go-side configuration action.

## 13.3 Runtime Compilation

```go
rt, err := eng.NewRuntime(ctx, source)
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
    Packs:    []gicel.Pack{gicel.Prelude},
    Timeout:  3 * time.Second,
    MaxSteps: 50_000,
    MaxAlloc: 10 * 1024 * 1024,
})
```

Single-call compile+execute for AI agents.

`RunSandbox` calls `DenyAssumptions` automatically — user code cannot declare `assumption` bindings in sandbox context. Hosts using the Engine API directly should call `eng.DenyAssumptions()` if untrusted code is involved.

**Limitation: timeout scope.** The timeout covers compilation and evaluation, but pack application runs before the timeout context is set. Packs are `func(Registrar) error` and do not receive a context — a misbehaving pack can block indefinitely. In practice, all stdlib packs are pure registration and complete instantly. Custom packs that perform I/O should be applied outside `RunSandbox`.

## 13.6 Primitive Implementation

```go
type PrimImpl func(ctx context.Context, capEnv CapEnv, args []Value, apply Applier) (Value, CapEnv, error)

type Applier struct {
    Apply  func(fn Value, arg Value, capEnv CapEnv) (Value, CapEnv, error)
    ApplyN func(fn Value, args []Value, capEnv CapEnv) (Value, CapEnv, error)
}
```

The `Applier` struct enables higher-order primitives (e.g., `foldl`). `Apply` handles single-argument application; `ApplyN` handles multi-argument application, eliminating intermediate partial application values and reducing VM boundary crossings.

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
form Bool := True | False
form Maybe := \a. Just a | Nothing
form List := \a. Cons a (List a) | Nil
form Ordering := LT | EQ | GT
form Result := \e a. Ok a | Err e
form Mult := Zero | Linear | Affine | Unrestricted

type Effect :: Type := \r a. Computation Zero r r a
type Suspended :: Type := \r a. Thunk Zero r r a
type Lift := \(m: Type -> Type) (g: Kind) (r1: Row) (r2: Row) a. m a

fst :: \a b. (a, b) -> a
snd :: \a b. (a, b) -> b
```

`()` (unit) is the empty record. `(a, b)` (tuple) is record sugar.

## 15.2 Stdlib Packs

Each Go-side pack bundles type registration, module source, and primitive implementations. The pack is loaded with `eng.Use(pack)` and imported in source by its module name.

| Go Pack       | Module         | Provides                                                                           |
| ------------- | -------------- | ---------------------------------------------------------------------------------- |
| `Prelude`     | `Prelude`      | Num/Str/List: arithmetic, string ops, list ops, type classes, ADTs                 |
| `EffectFail`  | `Effect.Fail`  | `fail` capability, `fromMaybe`, `fromResult`, `failWithAt`                         |
| `EffectState` | `Effect.State` | `get`/`put`/`modify`/`runState`/`evalState`/`execState` + `*At` named cap variants |
| `EffectIO`    | `Effect.IO`    | `log`/`dbg` via CapEnv buffer                                                      |
| `EffectArray` | `Effect.Array` | Mutable arrays: `new`, `read`, `write` + `*At` named cap variants                  |
| `EffectRef`   | `Effect.Ref`   | Mutable refs: `new`, `read`, `write`, `modify` + `*At` named caps                  |
| `EffectMap`   | `Effect.Map`   | Mutable ordered map (AVL) + `*At` named cap variants                               |
| `EffectSet`   | `Effect.Set`   | Mutable ordered set (AVL) + `*At` named cap variants                               |
| `DataStream`  | `Data.Stream`  | Lazy list: `LCons`/`LNil`, `head`, `tail`, `take`, `drop`                          |
| `DataSlice`   | `Data.Slice`   | Contiguous array: O(1) `length`/`index`, `Functor`/`Foldable`                      |
| `DataMap`     | `Data.Map`     | Ordered immutable map (AVL): `insert`, `lookup`, `delete`                          |
| `DataSet`     | `Data.Set`     | Ordered immutable set (backed by Map): `insert`, `member`                          |
| `DataJSON`    | `Data.JSON`    | JSON serialization: `ToJSON`/`FromJSON` type classes                               |
| `Console`     | `Console`      | CLI-only stdio: `putLine`, `getLine`                                               |

Types (`Int`, `Double`, `Byte`, `String`, `Rune`, `Slice`, `Array`, `Ref`, `Map`, `Set`, `MMap`, `MSet`) are checker built-ins registered in `NewEngine()`. Runtime representation: `HostVal` wrapping Go values.

---

# 16. Open Design Fork Points

| Fork Point                                         | Current State                                          | Decision Trigger                                                         |
| -------------------------------------------------- | ------------------------------------------------------ | ------------------------------------------------------------------------ |
| `Row` as built-in kind vs general structured-index | Built-in kind; reduced pressure via DataKinds          | Need for non-capability indexing (e.g., session types)                   |
| Algebraic effects/handlers vs indexed monad        | Indexed monad (Atkey); type families reduce motivation | Evidence that handler-based approach better serves the AI agent use case |

---

# 17. Type Families

## 17.1 Type Families

Type families introduce type-level computation: functions from types to types, evaluated during type checking and fully erased before Core IR generation. No runtime representation exists.

### 17.1.1 Standalone Closed Type Family

A closed type family is a `type` declaration whose body is a lambda containing a type-level `case`. Reduction proceeds top-to-bottom; the first matching alternative wins.

```
type Name :: ResultKind := \TyBinder+. case scrutinee { Alt (; Alt)* }

Alt ::= TypePattern '=>' TypeCaseBody
```

The `::` after the name provides the result kind. The body is a lambda over the parameters, with a `case` expression that dispatches on the scrutinee. Alternative bodies allow `->` (function arrow) but stop at `=>` (the alternative separator). This permits result types like `Decode a -> Decode b` without ambiguity.

```
type ElemOf :: Type := \(c: Type). case c {
  List a  => a;
  Slice a => a;
  String  => Rune
}
```

**Reduction semantics**: For each alternative, the checker attempts to match the scrutinee against the left-hand side pattern. On success, the result is the substituted right-hand side. On failure, the next alternative is tried. On **indeterminate** match (unsolved metavariables), reduction is **stuck** — further alternatives are not tried. This prevents premature commitment when a metavariable may later unify with an earlier alternative's pattern.

**Confluence**: guaranteed by ordered, first-match semantics. **Termination**: Non-recursive type families reduce in one step per application. Recursive type families use a shared step budget (default: 50,000 steps per compile session) and a type size bound (10,000 nodes per expression) to prevent exponential growth.

### 17.1.2 Builtin Row Type Families

`Merge :: Row -> Row -> Row` is a builtin type family that performs disjoint merge of two capability rows. Overlapping labels produce a compile error.

```
type Combined :: Row := Merge { a: Int } { b: Bool }
-- reduces to { a: Int, b: Bool }
```

Open rows (containing unsolved metavariables in tails) remain stuck until the variables are resolved.

### 17.1.3 Associated Types

A class-like `form` body may declare associated type families (kind signature only). `impl` bodies provide definitions using `:=`.

```
form Container c {
  type Elem c :: Type;
  cfold: \b. (Elem c -> b -> b) -> b -> c -> b
}

impl Container (List a) := {
  type Elem := a;
  cfold := foldr
}

impl Container String := {
  type Elem := Rune;
  cfold := strFoldr
}
```

Associated types elaborate to standalone type families whose equations are collected from all instances. The checker verifies that every instance provides a definition and that definitions are kind-consistent.

### 17.1.3 Data Families

A class-like `form` body may declare associated form families (kind signature only). `impl` bodies provide form type definitions, enabling per-instance representation.

```
form Collection c {
  form Key c :: Type;
  insert: Key c -> c -> c
}

impl Collection IntSet := {
  form Key := MkKey Int;
  insert := intSetInsert
}
```

Data families are generative: `Key IntSet` and `Key CharSet` are distinct types even if both wrap the same representation.

### 17.1.4 Injectivity Annotation

A type family may declare its result injective via a named result binder with a type equality constraint:

```
type Effects :: Row := \(mode: AppMode). case mode {
  ReadOnly  => { get: () -> String };
  ReadWrite => { get: () -> String, put: String -> () }
}
```

Injectivity is verified at declaration time by pairwise comparison: if two alternatives' right-hand sides unify, their left-hand sides must also unify. Many natural type families (e.g., `ElemOf` where both `List Rune` and `String` map to `Rune`) are not injective.

### 17.1.5 Type-Level Pattern Matching

| Pattern form           | Matches                     |
| ---------------------- | --------------------------- |
| Type variable `a`      | Any type (binding)          |
| Type constructor `Int` | Exact match                 |
| Promoted constructor   | Exact match (kind-directed) |
| Application `List a`   | Head match + recursive      |
| Wildcard `_`           | Any type (non-binding)      |

Nested patterns are supported: `List (Maybe a)` matches `List (Maybe Int)`, binding `a` to `Int`.

### 17.1.6 Interaction with Existing Features

**Row types**: Type families operate above rows. Row unification remains built-in. Type families can return row types but cannot pattern-match on row structure.

**Evidence system**: Type families appearing inside evidence entries are reduced before instance search: `Eq (ElemOf (List Int))` reduces to `Eq Int`, then resolves normally.

**GADTs**: Type family reduction occurs during GADT refinement; the checker normalizes types before computing local equalities.

**Partial application**: Type families cannot be partially applied. `F Int` where `F` has arity 2 is not a valid `Type -> Type`.

**Core IR**: Fully reduced at compile time. No `TyFamilyApp` survives into Core. No runtime representation.

**Keyword count**: 15. `type` is reused; `::` after the name is the disambiguator.

## 17.2 Multiplicity Annotations

Row fields accept an optional multiplicity annotation using `@`:

```
RowField ::= Label ':' Type ('@' TypeAtom)?
```

The annotation tracks usage discipline for capabilities:

```
open  :: Computation {} { h: Handle @Linear } ()
close :: Computation { h: Handle @Linear } {} ()
```

Without annotation, fields are `@Unrestricted`. The multiplicity kind forms a four-element lattice:

```
form Mult := Zero | Linear | Affine | Unrestricted
```

The lattice order is: `Zero` and `Linear` are incomparable, both below `Affine`, which is below `Unrestricted`. `Zero` represents a capability that has been consumed and cannot be used.

Multiplicity annotations are checked by the type system but do not affect runtime evaluation. They constrain how capabilities may be used: `@Linear` requires exactly-once consumption, `@Affine` allows at-most-once.

### 17.2.1 User-Defined Grade Algebras

The `GradeAlgebra` class (defined in Core) enables user-defined grade lattices:

```
form GradeAlgebra := \(g: Kind). {
  type GradeJoin :: g -> g -> g;
  type GradeCompose :: g -> g -> g;
  type GradeDrop :: g
}
```

`GradeJoin` computes the join (least upper bound) of two grades. `GradeCompose` computes the sequential composition of two grades (used by `bind` to compose grade parameters). `GradeDrop` is the identity element. Grade boundary enforcement checks `GradeJoin(Drop, grade) ~ grade` — a field is preservable iff joining the bottom element leaves the grade unchanged.

The standard `Mult` algebra is provided by Prelude. `GradeAlgebra` is defined in Core (auto-imported). Users can define custom algebras (e.g., security levels):

```
form Level := { Public: Level; Secret: Level }
impl GradeAlgebra Level := {
  type GradeJoin := LevelJoin;
  type GradeDrop := Public
}
```

### 17.2.2 Grade Boundary Enforcement

Grade boundary checking (whether a capability can be preserved across a bind step) resolves the `GradeAlgebra` instance for the grade's kind and checks `Join(Drop, grade) ~ grade`:

- **Concrete grades**: immediate reduction via the resolved Join family.
- **Grades with metavariables**: a `CtFunEq` constraint is emitted. The constraint blocks until the metavariable is solved, then the type family reduces and unification enforces the grade boundary. An `OnFailure` callback produces `ErrMultiplicity` on violation.

**Fallback behavior.** If no `GradeAlgebra` instance is found for the grade's kind, grade enforcement is skipped — the field is treated as unrestricted. This means `@Linear` on a field whose type's kind has no `GradeAlgebra` instance silently degrades to unrestricted. This is a known limitation; the fallback avoids false positives for types that predate the grade system.

This dual-path design enables future multiplicity polymorphism: grade-polymorphic functions emit deferred constraints that are resolved once the grade metavariable is instantiated.

The `LUB` (least upper bound) of multiplicities at branch join points can be computed via a type family:

```
type LUB :: Mult := \(m1: Mult) (m2: Mult). case (m1, m2) {
  (Linear, _)                   => Linear;
  (_, Linear)                   => Linear;
  (Affine, _)                   => Affine;
  (_, Affine)                   => Affine;
  (Unrestricted, Unrestricted)  => Unrestricted
}
```

## 17.3 Divergent Post-States in Case Expressions

The current specification requires all branches of a `case` expression to produce the same post-state. With multiplicity annotations and `LUB`, branches may produce divergent post-states that are joined:

```
case cond {
  True  => consume handle;    -- post: { }
  False => pure ()            -- post: { h: Handle @Linear }
}
-- joined post-state: LUB applied field-wise
```

The joined post-state is computed by applying `LUB` field-wise across the post-states of all branches. A field present in one branch but absent in another is treated as consumed (`Linear`), and the join reflects the most restrictive usage.

The intersection of label sets determines the joined post-state. Labels present in one branch but absent in another are treated as consumed in the join. This design permits branches to consume different capabilities while maintaining a sound post-state for the continuation.

---

# 18. Session Fidelity

## 18.1 Setup

Session types are encoded via the Atkey indexed monad without special syntax. Protocol states are regular type constructors; `@Linear` annotations on channel labels enforce usage discipline.

```
form Send := \s. MkSend          -- send a value, continue as s
form Recv := \s. MkRecv          -- receive a value, continue as s
form End := MkEnd                -- session complete

type Dual :: Type := \(s: Type). case s {
  Send s => Recv (Dual s);
  Recv s => Send (Dual s);
  End    => End
}

send  :: \s. Computation { ch: Send s @Linear } { ch: s @Linear } T
recv  :: \s. Computation { ch: Recv s @Linear } { ch: s @Linear } T
close :: Computation { ch: End @Linear } {} ()
```

## 18.2 Session Fidelity Theorem

**Theorem (GICEL Session Fidelity).** Let `e :: Computation pre post a` be well-typed, with `pre` containing `ch : S @Linear` for some protocol state `S`. Then:

**(a) Protocol compliance.** Each operation on `ch` advances the protocol state according to the type family rules: `send` transforms `Send s` to `s`, `recv` transforms `Recv s` to `s`, and `close` requires `End`.

**(b) Communication safety.** The type parameter of each send/recv operation matches the protocol specification. No send/recv is applied to an incompatible protocol state.

**(c) Session completion.** If `ch` does not appear in `post`, then the session has reached `End` and been closed. No other terminal state is reachable under the protocol encoding.

## 18.3 Proof Structure

**(a)** follows from the soundness of row unification with type family reduction. Each bind step in the do-chain unifies the current step's post-state with the next step's pre-state. The type family rules for `Send`, `Recv`, and `End` deterministically govern the transitions.

**(b)** follows from the parametric typing of `send` and `recv`. Type unification ensures that the type parameter of each operation matches the corresponding position in the protocol structure.

**(c)** follows from the multiplicity enforcement (Phase 1-2). The `@Linear` annotation ensures that each same-type preservation occurs at most once; type-changing preservations (protocol transitions) are unrestricted. The `close` operation is the only primitive that consumes `ch` at `End`. Since `@Linear` capabilities cannot be silently dropped (row unification rejects post-states that lose annotated labels without consumption), a well-typed program that removes `ch` from its post-state must have executed `close`.

## 18.4 Duality Involution

**Property.** `Dual (Dual S)` reduces to `S` for all closed protocol states `S`.

This follows from the type family alternatives by structural induction on `S`. Each constructor pair (`Send`/`Recv`) is symmetric under double application of `Dual`, and `Dual End` reduces to `End`.

# 19. Potential Extensions

| Extension                 | Classification   | Prerequisite                     |
| ------------------------- | ---------------- | -------------------------------- |
| Multiplicity polymorphism | Addition         | Grade metavariable instantiation |
| Selective module exports  | Refinement       | Module system evolution          |
| Qualified patterns        | Addition         | Module system evolution          |
| Refinement Types          | Phase transition | Separate analysis                |
| Dependent Types           | Full restructure | Far future                       |
