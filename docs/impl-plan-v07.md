# v0.7 Implementation Plan

## Approach

- **TDD**: tests first, then implementation
- **Commit per group**: each logical group gets its own commit
- **Examples**: `examples/` programs demonstrating each feature
- **Stress tests**: parser, checker, evaluator stress tests for edge cases

## Implementation Order

```
Phase 1: Evidence Sort (Level 9)     — internal refactoring, no user-visible changes
Phase 2: HKT                        — kind variables, kind unification, poly-kinded classes
Phase 3: Records + Tuples           — parser, checker, evaluator, prelude
Phase 4: Module / Host Boundary     — Prelude split, SetPrelude, RegisterModuleFile
```

**Rationale**: Evidence Sort first simplifies the checker for subsequent work. HKT before Records ensures poly-kinded classes are available for Record instances. Module changes last because Prelude content changes with Records.

---

## Phase 1: Evidence Sort

### Group 1A: EvidenceFiber interface + TyEvidenceRow type

**Tests first:**
- `types/evidence_row_test.go` — construction, field access, equality, Children(), Span()
- Verify `CapabilityFiber` and `ConstraintFiber` implement the interface

**Implement:**
- Define `EvidenceFiber` interface in `types/`
- Define `TyEvidenceRow` replacing `TyRow` and `TyConstraintRow`
- Define `CapabilityFiber` and `ConstraintFiber`
- Migrate `RowField` / `ConstraintEntry` to `EvidenceField` (fiber-specific)

**Commit**: `refactor(types): introduce TyEvidenceRow with EvidenceFiber interface`

### Group 1B: Row operations migration

**Tests first:**
- `types/evidence_row_test.go` — Normalize, Labels, HasLabel, ExtendRow, RemoveLabel for both fibers

**Implement:**
- Migrate `row.go` + `constraint_row.go` → unified `evidence_row.go`
- Parameterize operations by fiber

**Commit**: `refactor(types): unify row/constraint operations into evidence_row`

### Group 1C: Substitution, equality, free variables, pretty-printing

**Tests first:**
- Extend existing tests to use `TyEvidenceRow`

**Implement:**
- `subst.go`: merge TyRow/TyConstraintRow cases → single TyEvidenceRow case
- `equal.go`: merge cases
- `free.go`: merge cases
- `pretty.go`: merge prettyRow/prettyConstraintRow

**Commit**: `refactor(types): migrate subst/equal/free/pretty to TyEvidenceRow`

### Group 1D: Unifier migration

**Tests first:**
- Existing `unify_constraint_test.go` + row unification tests must pass unchanged

**Implement:**
- `unify.go`: merge `unifyRows` + `unifyConstraintRows` → `unifyEvidenceRows`
- Merge `solveRowTail` + `solveConstraintTail` → `solveEvidenceTail`
- Merge `classifyFields` + `classifyConstraints` → `classifyEntries`
- Merge Zonk cases

**Commit**: `refactor(check): unify row/constraint unification into evidence unification`

### Group 1E: Checker + builder migration

**Tests first:**
- Full test suite must pass (`go test ./...`)

**Implement:**
- `bidir.go`: all `TyRow`/`TyConstraintRow` constructions → `TyEvidenceRow`
- `decl.go`, `class.go`, `alias.go`: migrate pattern matches
- `builtin.go`: migrate factories (EmptyRow, ClosedRow, etc.)
- Public API type helpers (`typehelpers.go`)

**Commit**: `refactor(check): complete Evidence Sort migration (Level 9)`

### Group 1F: Stress test + cleanup

**Tests:**
- `evidence_stress_test.go` — unification of large evidence rows (100+ fields), mixed fibers, deeply nested tails
- Verify line count reduction

**Commit**: `test: Evidence Sort stress tests and cleanup`

---

## Phase 2: HKT

### Group 2A: Kind representation

**Tests first:**
- `types/kind_test.go` — KindMeta construction, equality, substitution

**Implement:**
- Add `KindMeta` to kind types
- Add `KindVar` (for explicit kind variables in forall)
- Add `Kind` sort (the kind of kinds, for `forall (k : Kind)`)

**Commit**: `feat(types): kind metavariables and kind variables`

### Group 2B: Kind unification

**Tests first:**
- `check/kind_unify_test.go` — unify KindMeta with concrete kinds, occurs check, kind substitution

**Implement:**
- `unify.go`: add `unifyKinds` with metavariable solving
- Kind occurs check
- Kind substitution in ordered context
- Extend `Zonk` for kind metavariables

**Commit**: `feat(check): kind unification with metavariables`

### Group 2C: Parser + checker integration

**Tests first:**
- Parse `forall (k : Kind). forall (f : k -> Type). f a -> f a`
- Check kind-polymorphic function types

**Implement:**
- Parser: recognize `Kind` as sort in kind annotations
- Checker: handle `KindVar` in kind positions, instantiate kind variables

**Commit**: `feat: kind variables in forall binders`

### Group 2D: Poly-kinded type classes

**Tests first:**
- Define and use poly-kinded `Functor`
- Instance resolution with kind matching

**Implement:**
- Class declaration: kind-polymorphic parameters
- Instance resolution: kind argument matching
- Existing `Functor`, `Foldable` etc. continue to work

**Commit**: `feat: poly-kinded type class parameters`

### Group 2E: Stress tests + examples

**Tests:**
- Stress: deeply nested kind applications, kind polymorphism in instance chains
- Example: `examples/hkt/` — user-defined HKT abstractions

**Commit**: `test: HKT stress tests and examples`

---

## Phase 3: Records + Tuples

### Group 3A: Lexer + token

**Tests first:**
- `syntax_test.go` — lex `!#` as TokBangHash, `!` alone still TokOp, `!!` still TokOp

**Implement:**
- Add `TokBangHash` to token types
- Lexer: `!` followed by `#` (not followed by operator char) → `TokBangHash`

**Commit**: `feat(syntax): add !# token for record projection`

### Group 3B: Parser — record literals + patterns

**Tests first:**
- Parse `{ x = 1, y = True }` as record literal
- Parse `{ x = a, y = b }` as record pattern
- Parse `{}` as empty record
- Disambiguation: `{ x := e; body }` vs `{ x = 1 }` vs `{ r | x = 1 }`

**Implement:**
- AST nodes: `ExprRecord`, `ExprRecordUpdate`, `ExprProject`, `PatRecord`
- Parser: record literal, record update, record pattern

**Commit**: `feat(syntax): record literal, update, and pattern parsing`

### Group 3C: Parser — tuples + unit

**Tests first:**
- Parse `(1, 2)` as tuple literal
- Parse `(a, b)` as tuple pattern
- Parse `()` as unit literal/pattern
- Parse `(Int, Bool)` as tuple type
- Parse `(Int)` as grouping (NOT tuple)

**Implement:**
- AST: extend existing paren handling with comma detection
- Desugar to record AST nodes during parsing

**Commit**: `feat(syntax): tuple and unit syntax (desugars to records)`

### Group 3D: Parser — projection + chaining

**Tests first:**
- Parse `r!#x` as projection
- Parse `r!#x!#y` as chained projection (left-associative)
- Parse `f r!#x` as `f (r!#x)` (atom-level precedence)

**Implement:**
- After parsing an atom, check for `TokBangHash` and chain

**Commit**: `feat(syntax): !# projection operator`

### Group 3E: Checker — Record type + projection + update

**Tests first:**
- `check_test.go` — record literal type inference, projection typing, update typing
- Row polymorphic records: `forall r. Record { x : Int | r } -> Int`
- Open pattern matching

**Implement:**
- `Record` as built-in type constructor (`Record : Row → Type`)
- Projection: extract field type from row
- Update: field existence check + type replacement
- Pattern matching: open patterns with row tail

**Commit**: `feat(check): record type checking, projection, update`

### Group 3F: Checker — tuples + unit

**Tests first:**
- Tuple type checking (desugared to records)
- Unit `()` type checking
- Tuple patterns

**Implement:**
- Ensure desugared forms type-check correctly
- `()` : `Record {}` (built-in)

**Commit**: `feat(check): tuple and unit type checking`

### Group 3G: Evaluator — record runtime

**Tests first:**
- `eval/eval_test.go` — record construction, projection, update at runtime
- Equality of records with different field orders

**Implement:**
- `RecordVal` runtime value (or `HostVal` wrapping `map[string]Value`)
- PrimOp implementations: `RecordCon`, `RecordProj`, `RecordUpdate`
- Record pattern matching in evaluator

**Commit**: `feat(eval): record runtime with PrimOp`

### Group 3H: Prelude migration

**Tests first:**
- Programs using `()` instead of `Unit`
- Programs using `(a, b)` instead of `Pair a b`
- `fst`, `snd` work on tuples

**Implement:**
- Remove `data Unit = Unit`, `data Pair a b = Pair a b`
- Add `()` as built-in
- Migrate `fst`, `snd`
- Migrate instances: Eq, Ord, Functor, Foldable, Traversable for tuples
- Update all existing tests and examples

**Commit**: `feat(prelude): replace Unit/Pair with tuples, migrate instances`

### Group 3I: Stress tests + examples

**Tests:**
- Stress: large records (50+ fields), deeply nested tuples, row polymorphic record functions
- Stress: record + tuple pattern matching exhaustiveness
- Stress: mixed records and capabilities in same program

**Examples:**
- `examples/records/` — CRUD-like structured data manipulation
- `examples/tuples/` — tuple processing, fst/snd, destructuring

**Commit**: `test: records and tuples stress tests and examples`

---

## Phase 4: Module / Host Boundary

### Group 4A: Prelude split

**Tests first:**
- `NoPrelude()` still works
- Core definitions available even with custom Prelude
- Custom Prelude replaces default

**Implement:**
- Split `stdlib.PreludeSource` into `CoreSource` + `PreludeSource`
- Core: Computation-essential types only
- Prelude: Bool, Maybe, List, Ordering, type classes, instances

**Commit**: `refactor: split Prelude into Core + Prelude`

### Group 4B: SetPrelude API

**Tests first:**
- `eng.SetPrelude(customSource)` replaces default Prelude
- Programs compile with custom Prelude

**Implement:**
- Add `SetPrelude(source string)` to Engine
- Modify `ensurePrelude()` to use custom source if set

**Commit**: `feat: Engine.SetPrelude for custom prelude replacement`

### Group 4C: RegisterModuleFile

**Tests first:**
- Load module from temp file
- Error on missing file

**Implement:**
- `RegisterModuleFile(path string) error` — read file, call `RegisterModule`

**Commit**: `feat: Engine.RegisterModuleFile for file-based module loading`

### Group 4D: Package promotion

**Implement:**
- Identify packages to promote (based on actual public API needs)
- Move or alias as needed

**Commit**: `refactor: promote selected internal packages to public API`

---

## Commit Summary

| Phase | Commits | Description |
|-------|---------|-------------|
| 1 | 6 | Evidence Sort (Level 9) |
| 2 | 5 | HKT |
| 3 | 9 | Records + Tuples |
| 4 | 4 | Module / Host Boundary |
| **Total** | **24** | |

---

## Test Strategy

### Unit Tests (per group)
- Written BEFORE implementation (TDD red→green)
- Focused on the specific feature being added

### Integration Tests (per phase)
- End-to-end: source → compile → execute → verify result
- Added at the end of each phase

### Stress Tests (per phase)
- Large inputs, edge cases, performance bounds
- Combinatorial: features interacting with each other

### Examples (per major feature)
- `examples/records/` — practical record usage
- `examples/tuples/` — tuple patterns and projections
- `examples/hkt/` — kind-polymorphic abstractions

### Regression
- `go test ./...` must pass after every commit
