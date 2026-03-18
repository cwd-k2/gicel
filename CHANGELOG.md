# Changelog

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
- **Multiplicity annotations** — `@Mult` syntax on row types (structural foundation; enforcement not yet active)
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
