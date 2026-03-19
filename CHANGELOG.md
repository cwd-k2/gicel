# Changelog

## v0.12.0 — 2026-03-19

### Refactoring

Checker restructuring: establish subpackage boundaries for each concern.

- **`internal/budget` extraction** — unified resource limiter (`Budget`) tracks steps, depth, and allocation bytes across all pipeline phases. Replaces the previous `eval.Limit` type
- **`check/exhaust` subpackage** — Maranget exhaustiveness checking extracted with `DataTypeInfo`/`ConInfo` types. Callback-based `Env` struct decouples from Checker state
- **`check/family` subpackage** — type family reduction engine, stuck index, injectivity verification. `StuckIndex` exported for Checker snapshot/restore
- **`check/env` subpackage** — shared environment type definitions (`AliasInfo`, `ClassInfo`, `InstanceInfo`, `ConstraintInfo`) extracted as canonical home
- **`internal/engine` extraction** — Engine/Runtime/RunSandbox moved from root package. Root `gicel` package becomes a pure facade of type aliases and re-exports; external API unchanged

### Documentation

- **Trust boundary clarification** — README and agent guide now explicitly document that host-registered primitives (`RegisterPrim`) are trusted computing base code, and that `Timeout` bounds evaluation time only

---

## v0.11.0 — 2026-03-18

### Language Features

- **Multiplicity enforcement** — `@Linear` and `@Affine` capabilities are now enforced at bind sites. Same-type preservations (re-use without state change) are counted; exceeding the limit produces error `E0290`. Type-changing preservations (protocol transitions) are unrestricted
- **LUB branch joins** — case branches with different capability multiplicities are joined via the `LUB` type family, resolving `Linear ⊔ Affine → Affine` instead of failing with a unification error
- **Session type library** — protocol states as type constructors (`Send`/`Recv`/`End`), `Dual` type family, `send`/`recv`/`close` operations with `@Linear` capability tracking

### Type System

- **Session fidelity theorem** — formal proof in `docs/spec/language.md` Chapter 18: protocol compliance, communication safety, session completion. 12 probe tests in `tests/probe/`
- **TyMeta levels** — metavariables carry implication nesting depth (`Level` field), preparing for OutsideIn(X) touchability

### Refactoring

Structural refactoring for next-version extensibility:

- **`TyCBPV` unification** — `TyComp` and `TyThunk` (structurally identical) merged into a single `TyCBPV` type with `CBPVTag`. Halves switch-case count across 23 files (-93 lines net)
- **Generic type traversals** — `types.MapType`, `types.AnyType`, `types.CollectTypes` replace 3 hand-written switch traversals. New type nodes only need updating in `MapType` and `Children()`
- **`check/unify` subpackage** — Unifier extracted into `internal/check/unify/` with compiler-enforced dependency direction. `OnSolve` callback replaces direct type family coupling
- **Stuck family decoupling** — `stuckFamilyEntry` and re-activation index moved from Unifier to `type_family.go` as `stuckFamilyIndex`. Unifier is now a pure unification engine
- **Row helpers to `types`** — `ClassifyRowFields`, `DecomposeConstraintType`, and 6 other pure helpers moved from `check` to `types/evidence.go`
- **Type family memoization** — `reduceFamilyApps` uses structural cache keys, replacing the node-budget heuristic

---

## v0.10.1 — 2026-03-18

### Language Features

- **Qualified constructor patterns** — `case x { Q.Con a -> ... }` with adjacency-based dot disambiguation, mirroring expression-level qualified names

### Type Inference

- **Type family re-activation** — stuck type family applications (blocked on unsolved metas) are now re-reduced when their blocking metas are solved, addressing the type family / row unification scheduling problem. ~190 lines, additive (OutsideIn(X) L1+L2)

### Fixes

- **`NormalizeRow` panic → error return** — duplicate labels during type checking no longer crash the host process; `RunSandbox` now wraps the entire compile+execute path in a top-level recover
- **`flatten()` data race** — `Env.Flatten()` pre-materializes the builtin environment at Runtime construction, eliminating a benign data race when sharing a Runtime across goroutines
- **`ResetFreshCounter`** — test determinism for type variable naming

### Refactoring

- **Prelude source consolidation** — `num.gicel`, `list.gicel`, `str.gicel` merged into `prelude.gicel`; Go-side string concatenation and `stripImportPrelude` removed. Core remains a separate auto-injected module
- **API parameter reduction** — `buildMethodSelector` (7→6 via `dictLayout` struct), `evalBindingsCore` (6→5 via derived `userVisible`)
- **Dead code removal** — unused `showIntImpl` in str.go
- **Selective exports excluded** from spec — `_` prefix is sufficient for GICEL's embedding-focused design

### Tests

- Regression tests for qualified constraints (`P.Num a =>`, tuple constraints, user-defined classes)
- Unit tests for `eval/env.go` (Extend, Lookup, ExtendMany, Flatten, TrimTo, shadowing) and `eval/capenv.go` (Get, Set, Delete, Labels, COW semantics)

---

## v0.10.0 — 2026-03-18

### Language Features

- **Type families** — closed type families with pattern matching and reduction, associated types in class/instance declarations, recursive type families (fuel 100), data families with constructor mangling and exhaustiveness support
- **Functional dependencies** on multi-parameter type classes (`| a =: b`)
- **Divergent post-states** — case branches may consume different capabilities; post-states are joined by intersection
- **Data families** — associated data type instances with automatic constructor mangling
- **Multiplicity annotations** — `@Mult` syntax on row types (structural foundation; enforcement added in v0.11.0)
- **Constraint tuple syntax** — `(Eq a, Ord a) => T` in class/instance declarations and type annotations
- **Literal patterns** — Int, String, Rune, Double in case expressions
- **Explicit type application** — `f @Int` syntax
- **Higher-rank record fields** — records may contain polymorphic values
- **Double type** — IEEE 754 with scientific notation (`3.14`, `1.0e-3`)
- **Numeric underscore separators** — `1_000_000`
- **`<+` / `+>` operators** and `FromList` / `ToList` type classes

### Module System

- **Selective imports** — `import M (x, T(..), Class(method))`
- **Qualified imports** — `import M as N`; `N.value`, `N.Constructor`, `N.Type` in expressions and type annotations
- **Private names** — `_` prefix excludes bindings from module exports
- **Module-qualified Core IR** — `Var.Module` and `Con.Module` carry canonical module names, eliminating name shadowing between stdlib modules
- **GHC-style ambiguity detection** — open imports of modules with overlapping names produce clear errors with disambiguation guidance
- **CLI `--module` flag** — `gicel run --module Lib=lib.gicel main.gicel` for multi-file projects
- **CLI `-e` flag** — `gicel run -e 'import Prelude; main := 1 + 2'` for inline evaluation

### Optimizer

- **Core IR optimizer** — Phase 1 algebraic simplifications (beta reduction, case-of-known-constructor, bind-pure elimination) and Phase 4 registered fusion rules

### Runtime

- **Allocation tracking** — `ChargeAlloc` infrastructure; all stdlib Go-level allocations (closures, constructors, records, AVL nodes, list cells) count against `MaxAlloc`
- **Structured explain detail** — explain events carry structured `ExplainDetail` with typed fields (bindings, scrutinee, pattern, capability diff) instead of format strings
- **`--explain-all`** — trace stdlib/module internals alongside user code

### API

- **`Div` class** — integer and double division (`div`, `mod`, `divDouble`)
- **`Computation` type alias** — `Effect` as shorthand for `Computation r r a`
- **`RunSandbox`** — single-call sandbox API with conservative defaults

### Fixes

- Fix `failImpl` silently dropping error message — `failWith` now includes the argument in the error
- Fix import-order re-registration causing unimported module names to shadow
- Fix compiler-generated `$` names leaking into module exports
- Fix class method shadowing from conflicting stdlib renames
- 7 bugs from adversarial testing (118 probe tests): unknown type name acceptance, builtin type leak through qualified imports, malformed import error recovery
- Security hardening from penetration probe findings

### Refactoring

Systematic three-round structural reorganization (net -3,200 lines):

- **8 new cohesive files** — `import.go`, `bidir_cbpv.go`, `bidir_case.go`, `exhaust_matrix.go`, `decl_generalize.go`, `eval_apply.go`, `eval_letrec.go`, `avl.go`
- **Dead code removal** — unused `group` field, `evidenceSource` type, 3 temporary probe test files
- **Content coupling fix** — `Unifier.InstallTempSolution`/`RemoveTempSolution` API replaces direct `soln` map access
- **O(1) data type lookup** — `dataTypeByName` reverse index for exhaustiveness checking
- **Naming cleanup** — `hasMeta`/`containsMeta` → `sliceHasMeta`/`typeHasMeta`; raw `"\x00"` → `core.QualifiedKey`
