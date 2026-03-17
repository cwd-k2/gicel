# Type Families — Design Document

Status: **Implemented** (2026-03-17, branch `feature/type-system-extensions`)

This document specifies the syntax, semantics, evolution plan, and future directions for closed type families in GICEL.

---

## 1. Motivation

### 1.1 Element Type Disambiguation

The current `Elem` MPTC relies on instance uniqueness by convention:

```gicel
class Elem c e {
  cfold :: forall b. (e -> b -> b) -> b -> c -> b
}
instance Elem (List a) a     { cfold := foldr }
instance Elem String Rune    { cfold := strFoldr }
```

There is no formal mechanism preventing `instance Elem (List a) Int`, and inference fails when `c` is polymorphic. An associated type family makes the relationship structural:

```gicel
class Container c {
  type Elem c :: Type;
  cfold :: forall b. (Elem c -> b -> b) -> b -> c -> b
}
```

Now `Elem (List a)` reduces to `a` by definition, not by instance search heuristic.

### 1.2 Graded Evidence Foundation

Level 10 (Graded Evidence) requires computing the least upper bound of multiplicities at branch join points:

```gicel
data Mult = Unrestricted | Affine | Linear

type LUB (m1 : Mult) (m2 : Mult) :: Mult = {
  LUB Linear _ = Linear;
  LUB _ Linear = Linear;
  LUB Affine _ = Affine;
  LUB _ Affine = Affine;
  LUB Unrestricted Unrestricted = Unrestricted
}
```

Without type families, this join must be hardcoded in the checker.

### 1.3 Session Type Duality

If GICEL adopts session types (fork point: Row as general structured-index), duality requires recursive type-level computation:

```gicel
data Session = Send Type Session | Recv Type Session | End

type Dual (s : Session) :: Session = {
  Dual (Send a s) = Recv a (Dual s);
  Dual (Recv a s) = Send a (Dual s);
  Dual End = End
}
```

### 1.4 Constraint Families

Since `Constraint` is already a first-class kind, type families with result kind `Constraint` arise naturally — no special syntax needed:

```gicel
type Required (cap : Capability) :: Constraint = {
  Required FileHandle = (Ord FilePath);
  Required Database   = (Eq ConnStr, Show ConnStr)
}
```

---

## 2. Syntax

### 2.1 Standalone Closed Type Family

Grammar:

```
TypeFamilyDecl
  = 'type' UpperName TyBinder* '::' ResultKind '=' '{' Equation (';' Equation)* '}'

ResultKind
  = KindExpr                                         -- non-injective
  | '(' LowerName ':' KindExpr ')' '|' DepList      -- injective (named result)

DepList
  = LowerName '->' LowerName+                        -- functional dependency

Equation
  = UpperName TypePattern* '=' TypeExpr
```

Examples:

```gicel
-- Non-injective
type LUB (m1 : Mult) (m2 : Mult) :: Mult = {
  LUB Linear _ = Linear;
  LUB _ Linear = Linear;
  LUB Affine _ = Affine;
  LUB _ Affine = Affine;
  LUB Unrestricted Unrestricted = Unrestricted
}

-- Injective (named result variable with dependency)
type Elem (c : Type) :: (r : Type) | r -> c = {
  Elem (List a) = a;
  Elem (Slice a) = a;
  Elem String = Rune
}
```

### 2.2 Associated Type Family

In class declarations — kind signature only:

```
AssocTypeDecl = 'type' UpperName TyBinder* '::' ResultKind
```

In instance declarations — single equation:

```
AssocTypeDef = 'type' UpperName TypePattern* '=' TypeExpr
```

Example:

```gicel
class Container c {
  type Elem c :: Type;
  cfold :: forall b. (Elem c -> b -> b) -> b -> c -> b
}

instance Container (List a) {
  type Elem (List a) = a;
  cfold := foldr
}

instance Container String {
  type Elem String = Rune;
  cfold := strFoldr
}
```

### 2.3 Parser Disambiguation

The parser sees `type Name params` and branches:

```
type Name params =  TypeExpr      →  type alias  (existing)
type Name params :: ...           →  type family or associated type decl
```

Within `= { ... }`, equations start with an uppercase name (the type family name), unambiguous against row types (`{ label : Type }`) whose labels are lowercase.

Within class/instance `{ ... }`, `type` at statement position signals an associated type declaration or definition.

### 2.4 Injectivity Annotation

The result kind position supports an optional named binder with functional dependency:

```
:: Type                            -- unnamed, non-injective
:: (r : Type) | r -> c            -- named result r, r determines c
:: (r : Type) | r -> a b          -- r determines both a and b
```

Injectivity is checked at declaration time by pairwise comparison: for every pair of equations, if the right-hand sides can unify, the left-hand sides must also unify. Injectivity is a strong property — many natural type families (e.g., `Elem` where `List Rune` and `String` both map to `Rune`) do not satisfy it.

The `| a -> b` notation uses the same syntax as functional dependencies on MPTCs (`class C a b | a -> b`), providing a unified concept of determination relations across the type system.

### 2.5 Name Repetition in Equations

Each equation repeats the type family name:

```gicel
type F (a : Type) (b : Type) :: Type = {
  F Int Bool = String;
  F a (List b) = b
}
```

Rationale:
- **Multi-argument disambiguation**: Without the name, `Int Bool` parses as type application.
- **Consistency**: Standalone and associated forms use the same equation syntax.
- **Recursive calls**: `Dual (Send a s) = Recv a (Dual s)` — the name on both sides is natural.

### 2.6 Complete Syntax Summary

```
Declaration level:
  type Name params = TypeExpr                            -- type alias (existing)
  type Name params :: Kind = { equations }               -- standalone type family
  type Name params :: (r : Kind) | deps = { equations } -- standalone, injective

Class body:
  type Name params :: Kind                               -- associated type decl
  type Name params :: (r : Kind) | deps                 -- associated type decl, injective

Instance body:
  type Name params = TypeExpr                            -- associated type def
```

No new keywords. `type` is reused; `::` after params is the disambiguator.

---

## 3. Semantics

### 3.1 Closed Type Family Reduction

A closed type family defines a sequence of equations checked top-to-bottom. To reduce `F T₁ ... Tₙ`:

1. For each equation `F P₁ ... Pₙ = R` in order:
   a. Attempt to match `(T₁,...,Tₙ)` against `(P₁,...,Pₙ)` by type-level pattern matching.
   b. If match succeeds with substitution `σ`, the result is `σ(R)`.
   c. If match fails, try the next equation.
   d. If match is **indeterminate** (involves unsolved metavariables), reduction is **stuck** — do not try further equations.
2. If all equations fail, the application is stuck (irreducible).

The indeterminate case (1d) is critical:

```gicel
type F (a : Type) :: Type = {
  F Int = Bool;
  F a   = String
}
```

Reducing `F α` where `α` is a metavariable: the first equation might match (if `α` is later solved to `Int`) or might not. Skipping to the second equation would commit to `String` prematurely. Reduction must be stuck until `α` is resolved.

### 3.2 Type-Level Pattern Matching

| Pattern form | Matches |
|-------------|---------|
| Type variable `a` | Any type (binding) |
| Type constructor `Int`, `Bool` | Exact match |
| Promoted constructor `Opened`, `Closed` | Exact match (kind-directed) |
| Application `List a`, `Maybe Int` | Head match + recursive |
| Wildcard `_` | Any type (non-binding) |

Nested patterns are supported: `Elem (List (Maybe a)) = Maybe a`.

### 3.3 Interaction with Unification

Type family reduction integrates into the DK bidirectional checker at the normalization step:

```
apply substitution  →  reduce type families  →  unify / check
```

Reduction is attempted during unification, subsumption, and evidence resolution. If stuck, the application remains as-is and may reduce later when metavariables are solved.

### 3.4 Confluence and Termination

**Confluence**: Closed type families are confluent by construction — equations are ordered, first match wins.

**Termination (non-recursive, Phase 1)**: Reduction always terminates in one step per application.

**Termination (recursive, Phase 3)**: Fuel limit (default: 100 reductions per type expression), consistent with GICEL's existing resource-bounding philosophy.

### 3.5 Associated Type Elaboration

Associated type families desugar to standalone type families:

```gicel
class Container c {
  type Elem c :: Type;
  cfold :: forall b. (Elem c -> b -> b) -> b -> c -> b
}
```

Elaborates to:

```
-- Standalone type family (equations collected from instances)
type Elem (c : Type) :: Type = { ... }

-- Dictionary has no field for Elem (type-level only)
data Container$Dict c = Container$MkDict (forall b. (Elem c -> b -> b) -> b -> c -> b)
```

The checker verifies: every instance provides a definition, definitions are kind-consistent, and injectivity (if declared) holds across all definitions.

---

## 4. Interaction with Existing Features

### 4.1 Row Types

Type families operate **above** rows, not within them. Row unification remains built-in (flatten-then-diff, O(n log n)). Type families can return row types but cannot pattern-match on row structure:

```gicel
-- Valid: returns a row
type DefaultCaps (mode : AppMode) :: Row = {
  DefaultCaps ReadOnly  = { get : () -> String };
  DefaultCaps ReadWrite = { get : () -> String, put : String -> () }
}

-- Invalid: pattern matching on row structure
type HasLabel (r : Row) (l : Symbol) :: Bool = { ... }  -- NOT SUPPORTED
```

### 4.2 Constraint System

Type families with result kind `Constraint` compose with quantified constraints:

```gicel
f :: forall cap. Required cap => cap -> IO ()
```

The checker reduces `Required cap`, producing a concrete constraint, then resolves evidence normally.

### 4.3 GADTs

Type family reduction occurs during GADT refinement: the checker normalizes types before computing local equalities.

### 4.4 HKT and Partial Application

**Type families cannot be partially applied.** `F Int` where `F` has arity 2 is not a valid `Type -> Type`. Workaround: wrap in a data type.

### 4.5 Evidence Architecture

The evidence system (TyEvidence, TyConstraintRow) is structurally unaffected. Type families appearing inside evidence entries are reduced before instance search:

```
Eq (Elem (List Int))  →  reduces to  →  Eq Int  →  standard resolution
```

---

## 5. Extension Lanes

The six extension lanes characterize GICEL's design space. Type families advance Lane 3 (type-equality) and enable progress on Lanes 1, 4, 5, and 6.

### 5.1 Current Position and Type Families

```
Lane 1 — Index
  parametric → phantom → row-indexed → promoted → GADT-refined
                                                       ▲ current
                                          TF: computation over promoted kinds

Lane 2 — Abstraction
  rank-1 → row poly → higher-rank → HKT → kind poly
                                              ▲ current
                                   (TF does not advance this lane)

Lane 3 — Type-equality
  syntactic → normalization → row norm → GADT refinement → TYPE-LEVEL REDUCTION
                                              ▲ current          ▲ type families

Lane 4 — Logic
  plain membership → finite state → constructor-indexed → TYPE-LEVEL PREDICATES
                                          ▲ current            ▲ TF + DataKinds

Lane 5 — Resource & Effect
  plain → indexed → row-polymorphic → COMPUTED EFFECT TYPES
                          ▲ current          ▲ TF

Lane 6 — Usage Discipline
  unrestricted → COMPUTABLE GRADES
       ▲ current       ▲ TF (LUB)
```

### 5.2 Cross-Lane Effects

| Lane | What type families enable | Example |
|------|--------------------------|---------|
| 1 (Index) | Computation over promoted kinds | `Dual (Send a s) = Recv a (Dual s)` |
| 3 (Type-equality) | Type-level reduction in unifier | `Elem (List Int) ~ Int` |
| 4 (Logic) | Type-level predicates via promoted Bool | `IsLinear FileHandle ~ True` |
| 5 (Effect) | Computed effect type aliases | `type Effects (mode : AppMode) :: Row` |
| 6 (Usage) | Multiplicity lattice operations | `LUB Linear Affine ~ Linear` |

---

## 6. Evolution Phases

### Phase 1: Closed Non-Recursive Type Families

**Scope**: Standalone closed type families. Non-recursive equations only.

**Delivers**: `Elem`, `LUB`, constraint families, effect type families.

**Checker changes**:
- New AST nodes: `DeclTypeFamily`, `TFEquation`
- New type form: `TyFamilyApp Name [Type]` (saturated application)
- Type-level pattern matcher (subset of term-level, operating on types)
- Reduction function integrated into `normalize`
- Kind checking for declarations and equations
- Injectivity verification (pairwise equation check)

**Parser changes**:
- `parseTypeAlias` → `parseTypeDecl` (branches on `::` after params)
- New `parseTypeFamilyEquations` and `parseResultKind`

**No changes to**: Core IR, evaluator, runtime. Type families are fully erased at compile time.

### Phase 2: Associated Types

**Scope**: Type family declarations in `class`, definitions in `instance`.

**Delivers**: `Container` class with associated `Elem`.

**Additional changes**:
- Class elaboration collects associated type declarations
- Instance checking validates associated type definitions
- Desugar to standalone type families (equation collection)

### Phase 3: Recursive Type Families

**Scope**: Right-hand sides may reference the type family being defined.

**Delivers**: `Dual` for session types. **Triggered by**: session types adoption.

**Additional changes**: Fuel counter, cycle detection. Optionally structural recursion check.

### Phase Summary

```
Phase 1                          Phase 2                     Phase 3
Closed non-recursive TF          Associated types            Recursive TF
─────────────────────────        ──────────────────          ─────────────
DeclTypeFamily AST               class/instance 'type'       Fuel counter
TyFamilyApp type form            Equation collection         Cycle detection
Type-level pattern matcher       Desugar to standalone       Structural check (opt)
reduceTyFamily in normalize      Scope injection
Injectivity check
Kind check for equations
                │                         │                        │
                │    Katsumata: PT        │    Katsumata: ref      │    Katsumata: PT in PT
                ▼                         ▼                        ▼
         Elem, LUB, Effects       Container class           Dual, Append
```

---

## 7. Interaction with Open Fork Points

### 7.1 Branching with Divergent Post-States

Type families provide the **mechanism** (`LUB`) for computing the join of divergent post-states, but do not decide the **policy** (whether to allow divergence at all). The fork point remains a policy decision; type families remove the implementation obstacle.

### 7.2 Row as Built-in Kind vs General Structured-Index

Session types can use promoted data kinds (`data Session = Send | Recv | End`) without generalizing `Row`. Type families compute over the session kind independently. This **reduces the pressure** to generalize `Row`.

### 7.3 Algebraic Effects vs Indexed Monad

Type families let the indexed monad express **computed capability sets**:

```gicel
type Effects (mode : AppMode) :: Row = {
  Effects ReadOnly  = { get : () -> String };
  Effects ReadWrite = { get : () -> String, put : String -> () }
}

runApp :: forall mode a. Computation (Effects mode) {} a -> a
```

This further reduces the motivation to switch to algebraic effects.

### 7.4 Usage Judgment (Linear/Affine)

Type families enable `LUB` computation for Graded Evidence (Level 10):

```
Type families (LUB defined)
  → EvidenceFiber gains multiplicity field
  → bind checks multiplicity compatibility
  → linear capability consumption pattern
```

---

## 8. Beyond Type Families

GICEL occupies a unique coordinate: **Atkey indexed monad × row polymorphism × CBPV × Go embedding**. Type families introduce type-level computation into this setting, opening research directions that are specific to this intersection.

### 8.1 Double Grading

`Computation pre post a` is a category-graded monad (grading category: Row objects). Adding multiplicity grades creates a doubly-graded structure:

```
Computation pre post mult a
            ───  ────  ────
             │     │     └── usage grade (Linear/Affine/Unrestricted)
             │     └── capability post-state (Row)
             └── capability pre-state (Row)
```

The two grades are not always independent — a linear capability is consumed (state transition) exactly once (usage constraint). The interaction between state transition and usage discipline, mediated by row unification, is novel territory.

### 8.2 Evidence Fiber Interaction

Type families cross fiber boundaries:

```gicel
type Required (cap : Type) :: Constraint = { ... }   -- Type → Constraint
type Effects (mode : AppMode) :: Row = { ... }        -- promoted kind → Row
```

The EvidenceFiber architecture (Level 9 design) provides the structural foundation. The question of where fiber independence ends and fiber interaction begins — mediated by type family reduction — is specific to GICEL's evidence architecture.

### 8.3 Reduction and Unification Scheduling

When a type family returns a Row (`type Effects mode :: Row`), and that Row appears in a unification target (`{ Effects mode | r }`), type family reduction and row unification become interdependent. If `mode` is unsolved, reduction is stuck; if the row context provides information, it might constrain `mode`. The scheduling of reduction vs unification is a non-trivial design problem.

### 8.4 Map

```
                        GICEL
                  Atkey × Row × CBPV × Go
                          │
                   Type Families (this document)
                          │
            ┌─────────────┼─────────────┐
            │             │             │
      Graded Evidence   Session Types  Constraint Families
      (double grading)  (recursive TF) (fiber crossing)
            │             │             │
            │        ┌────┴────┐        │
            │   structural   protocol   │
            │   recursion    composition │
            │        │         │        │
            └────────┴─────────┴────────┘
                          │
               ┌──────────┴──────────┐
               │                     │
      Practical:                Theoretical:
      capability security       graded fiber interaction
      protocol verification     reduction/unification
      bounded agent execution   scheduling
               │                     │
               │  ── term/type ──    │
               │     boundary        │
               (identity change beyond here)
```

---

## 9. Concrete Examples

### 9.1 Elem — Collection Element Type

```gicel
type Elem (c : Type) :: Type = {
  Elem (List a) = a;
  Elem (Slice a) = a;
  Elem String = Rune
}

class Container c {
  type Elem c :: Type;
  cfold :: forall b. (Elem c -> b -> b) -> b -> c -> b;
  clength :: c -> Int
}

instance Container (List a) {
  type Elem (List a) = a;
  cfold := foldr;
  clength := length
}

instance Container String {
  type Elem String = Rune;
  cfold := strFoldr;
  clength := strLength
}

-- Elem reduces during type checking
cany :: forall c. Container c => (Elem c -> Bool) -> c -> Bool
cany := \p -> cfold (\x acc -> case p x { True -> True; False -> acc }) False
```

Note: `Elem` is not injective — `Elem (List Rune) = Rune` and `Elem String = Rune` produce the same result from different inputs.

### 9.2 LUB — Multiplicity Join

```gicel
data Mult = Unrestricted | Affine | Linear

type LUB (m1 : Mult) (m2 : Mult) :: Mult = {
  LUB Linear _ = Linear;
  LUB _ Linear = Linear;
  LUB Affine _ = Affine;
  LUB _ Affine = Affine;
  LUB Unrestricted Unrestricted = Unrestricted
}

type GLB (m1 : Mult) (m2 : Mult) :: Mult = {
  GLB Unrestricted _ = Unrestricted;
  GLB _ Unrestricted = Unrestricted;
  GLB Affine _ = Affine;
  GLB _ Affine = Affine;
  GLB Linear Linear = Linear
}
```

### 9.3 Effect Computation

```gicel
data AppMode = ReadOnly | ReadWrite | Admin

type Effects (mode : AppMode) :: Row = {
  Effects ReadOnly  = { get : () -> String };
  Effects ReadWrite = { get : () -> String, put : String -> () };
  Effects Admin     = { get : () -> String, put : String -> (), delete : String -> () }
}

-- Injective: each mode maps to a distinct row
type Effects' (mode : AppMode) :: (r : Row) | r -> mode = {
  Effects' ReadOnly  = { get : () -> String };
  Effects' ReadWrite = { get : () -> String, put : String -> () };
  Effects' Admin     = { get : () -> String, put : String -> (), delete : String -> () }
}
```

### 9.4 Session Type Duality (Phase 3)

```gicel
data Session = Send Type Session | Recv Type Session | Choose Session Session
             | Offer Session Session | End

type Dual (s : Session) :: Session = {
  Dual (Send a s) = Recv a (Dual s);
  Dual (Recv a s) = Send a (Dual s);
  Dual (Choose a b) = Offer (Dual a) (Dual b);
  Dual (Offer a b) = Choose (Dual a) (Dual b);
  Dual End = End
}
```

### 9.5 Constraint Family

```gicel
data Serialization = JSON | Binary | CSV

type Serializable (fmt : Serialization) :: Constraint = {
  Serializable JSON   = Show;
  Serializable Binary = Packed;
  Serializable CSV    = (Show, Packed)
}

serialize :: forall fmt a. Serializable fmt a => a -> String
```

---

## 10. Implementation Checklist (Phase 1)

### 10.1 AST

```go
type DeclTypeFamily struct {
    Name       string
    Params     []TyBinder
    ResultKind KindExpr        // nil if associated decl in class
    ResultName string          // "" if non-injective
    Deps       []FunDep        // nil if non-injective
    Equations  []TFEquation    // nil if associated decl in class
    S          span.Span
}

type TFEquation struct {
    Name     string       // repeated family name (for validation)
    Patterns []TypeExpr   // left-hand side type patterns
    RHS      TypeExpr     // right-hand side
    S        span.Span
}

type FunDep struct {
    From string    // result variable name
    To   []string  // determined parameter names
}
```

### 10.2 Type Representation

```go
type TyFamilyApp struct {
    Name string
    Args []Type
    Kind Kind   // result kind (known from declaration)
}
```

### 10.3 Checker Pipeline

```
Source → Parse → [DeclTypeFamily in AST]
  → Kind-check TF declarations (params, result kind, equation RHSes)
  → Validate equations (patterns well-kinded, RHS has correct kind)
  → Verify injectivity (if declared)
  → Register TF in checker environment
  → Normal type checking proceeds; normalize() calls reduceTyFamily()
```

### 10.4 Reduction Function

```go
func (c *Checker) reduceTyFamily(name string, args []Type) (Type, bool) {
    fam := c.lookupTypeFamily(name)
    if fam == nil {
        return nil, false
    }
    for _, eq := range fam.Equations {
        subst, match := c.matchTyPatterns(eq.Patterns, args)
        switch match {
        case matchSuccess:
            return applyTySubst(subst, eq.RHS), true
        case matchFail:
            continue
        case matchIndeterminate:
            return nil, false  // stuck — do not try further equations
        }
    }
    return nil, false  // no equation matched — stuck
}
```

---

## 11. Invariants and Risks

### What Type Families Do Not Change

- **Core IR**: Fully reduced at compile time. No `TyFamilyApp` survives into Core.
- **Evaluator / Runtime**: No runtime representation.
- **Row unification**: Remains built-in, O(n log n). Not expressible as type families.
- **Evidence resolution**: Instance search is unchanged; type families are reduced before search.
- **Keyword count**: Remains 11.

### Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| Checker performance (reduction loops) | Medium | Phase 1 non-recursive; Phase 3 fuel limit |
| Error message quality (stuck reductions) | High | Report "could not reduce F T" with stuck application |
| Metavariable interaction (premature reduction) | High | Indeterminate match halts reduction; retried after solving |
| Scope creep (open type families) | Low | Closed-only by design; open families require module system |

---

## 12. Forward Compatibility of `|` Notation

The `|` token currently serves as constructor separator (`data T = A | B`) and row tail (`{ l : T | r }`). The proposed injectivity annotation adds a third role in declaration headers. All three occupy distinct syntactic positions.

The `| a -> b` notation is forward-compatible with:

| Future extension | Compatible? | Notes |
|------------------|-------------|-------|
| Functional dependencies | Identical syntax | `class C a b \| a -> b` |
| Refinement types | No conflict | Injectivity is in declaration headers; refinement `\|` is in type expressions (and would need to avoid row tail ambiguity independently) |
| Dependent types | Natural generalization | `\| r -> c` extends from type families to function declarations |
| Data families | Orthogonal | Data families are generative; injectivity is structural |

---

## Implementation Status (2026-03-17)

All 9 extensions implemented across 13 commits on `feature/type-system-extensions`.

| Phase | Extension | Status |
|-------|-----------|--------|
| 0 | Foundation scaffolding | Complete |
| 1 | Closed type families + constraint families | Complete |
| 2 | Associated types | Complete |
| 3 | Functional dependencies | Complete |
| 4 | Recursive type families (fuel limit 100) | Complete |
| 5 | Graded Evidence (`RowField.Mult`) | Complete |
| 5d | Multiplicity syntax (`@Mult`) | Complete |
| 6 | Divergent post-states (intersection join) | Complete |
| 7 | Session types (library feature) | Complete |
| 8 | Data families (constructor mangling) | Complete |

Additional: do block pre/post threading fix (`elaborateStmtsChecked`).

Key implementation files:
- `internal/syntax/decl_tf.go` — AST nodes
- `internal/syntax/decl_df.go` — Data family AST
- `internal/types/type_family.go` — `TyFamilyApp` type
- `internal/check/type_family.go` — Checker: reduction, matching, injectivity, LUB
- `internal/check/elaborate_do.go` — Do block pre/post threading
- `internal/check/bidir.go` — Divergent post-states in case

---

## References

- Eisenberg, Vytiniotis, Peyton Jones, Weirich. "Closed Type Families with Overlapping Equations." POPL 2014.
- Stolarek, Peyton Jones, Eisenberg. "Injective Type Families for Haskell." Haskell Symposium 2015.
- Chakravarty, Keller, Peyton Jones. "Associated Type Synonyms." ICFP 2005.
- Atkey. "Parameterised Notions of Computation." JFP 2009.
- Katsumata. "Parametric Effect Monads and Semantics of Effect Systems." POPL 2014.
