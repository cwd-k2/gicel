# Changelog

## v0.15.0 — 2026-03-22

### Architecture

- **Budget layer split** — `Budget` (runtime: steps, depth, nesting, alloc) and `CheckBudget` (compiler: tfSteps, solverSteps, resolveDepth) are now separate types. The compiler/runtime boundary is enforced at the type level
- **Registry extraction** — `Registry` struct and its 15 methods moved from `checker.go` to `registry.go`. Dict-to-class reverse map (`dictToClass`) replaces `isDictName`/`classFromDict` string heuristic
- **env → syntax forward reference eliminated** — `InstanceInfo.Methods` (unevaluated `syntax.Expr`) moved to a pipeline-local map, removing the `syntax` import from `check/env/types.go`
- **parse → types layer violation fixed** — `TupleLabel` canonical definition placed in `syntax`; parser no longer imports `lang/types`. All callers migrated to `syntax.TupleLabel`, removing `types.TupleLabel` delegation wrapper
- **Structural provenance flags** — `ir.Lam.Generated`, `ir.Bind.Generated`, `ir.Alt.Generated` replace the `isCompilerGenerated` string heuristic. Compiler sets flags at elaboration; evaluator reads them directly
- **tryResolveInstance** — centralizes the error-save/truncate probe pattern for instance resolution without emitting errors

### Examples

- **5 GICEL examples fixed** — continuation, nondeterminism, maybet, free-monad (renamed from ixmonad), session: corrected Monad/IxMonad usage and bare Computation wrapping
- **All 45 GICEL examples pass** — 44 run + 1 check-only (session types)

---

## v0.14.0 — 2026-03-21

### Architecture

- **Layered directory structure** — `internal/` restructured into `lang/`, `infra/`, `compiler/`, `runtime/`, `host/`, `app/` layers with explicit dependency direction
- **Checker service extraction** — `Session`, `Registry`, `Scope`, `Solver` as named types with method-based mutation contracts. All Registry writes go through named methods (`RegisterTypeKind`, `RegisterAlias`, `ImportInstance`, etc.)
- **Parser guard extraction** — safety harness (step/depth limits, halt flag) separated into `parserGuard` struct
- **Engine compile path unification** — shared `postCheck` helper; `compileModule` now accepts `context.Context` for cancellable module compilation

### Type Checker

- **TypeFamilies export boundary** — modules only export locally defined or locally enriched type families (not purely inherited ones)

### Stdlib

- **Data.Map expansion** — `keys`, `values`, `mapValues`, `filterWithKey`
- **Data.Set expansion** — `union`, `intersection`, `difference`

### CLI

- **`--use` → `--packs`** — flag renamed to convey "restrict to these packs"; `--use` kept as silent alias
- **Runtime error source locations** — text output shows `line:col:` prefix; JSON includes `line`, `col`, `message` fields
- **Preflight JSON errors** — bad flags and setup errors respect `--json` with `"phase": "preflight"`
- **Compile-error JSON hints** — `Diagnostic.Hints` field with secondary annotations
- **JSON allocation stats** — `"allocated"` field in success stats
- **Explain failure path** — `--explain` trace flushed on runtime errors (text and JSON)
- **Explain module names** — `--verbose` shows `[SourceName]` on module transitions
- **Explain internal distinction** — `--explain-all` dims stdlib steps in color mode

### Engine

- **RunSandbox enhancements** — `SandboxConfig.Context` for parent context propagation; `Explain` and `ExplainDepth` fields for trace hooks. Timeout now covers pack application + compilation + evaluation
- **`SetCompileContext`** — public method to bound module compilation with a context

### Documentation

- **README streamlined** — 346 → 212 lines; restructured around sandbox/PoC/embedding selling points
- **Agent guide** — minimal example fixed (`main := ()`), `--max-nesting` added to flag tables, `-e` semicolon note, `--explain-all` behavior note, Effect.IO behavior clarification, Map/Set qualified import tips, host API migration path, trust boundary section, observability hooks table
- **Operator docs fix** — `¦¦` → `||`, `<¦>` → `<|>` in functions.md
- **Stale `Std.*` references** — 6 example files updated to current module names
- **CLAUDE.md** — Rules moved to top, `--max-nesting` in flag table, probe test execution policy
- **Roadmap restructured** — split into `direction.md` (project principles), `language.md` (type system roadmap), `library.md` (stdlib/tooling roadmap). Version numbers removed; items ordered by dependency

### Testing

- **Parser/budget benchmarks** — `parse_bench_test.go` (5 benchmarks), `budget_bench_test.go` (5 benchmarks)
- **Stress harness split** — monolithic `stress_test.go` split into 6 domain files (types, typeclass, effect, stdlib, grammar, helpers)
- **Boundary test structured assertions** — `strings.Contains` replaced with `Diagnostic.Code` checks
- **Smoke test expansion** — JSON contract tests for runtime errors, preflight errors, allocation stats

---

## v0.13.0 — 2026-03-20

### Core IR

- **Fix node** — dedicated `core.Fix` replaces `LetRec` desugaring for recursive bindings. Self-referential closure creation is now a single node (`evalFix`), eliminating the two-pass `IndirectVal` patching overhead. Polymorphic recursion is naturally supported via TyLam peeling
- **List literal patterns** — `[x, y, z]` surface syntax desugared to `Cons`/`Nil` patterns during parsing. Pattern matching, exhaustiveness checking, and explain trace all support the new form

### Evaluator

- **Multi-module source attribution** — `Closure`, `ThunkVal`, and `bounceVal` now capture their originating `*span.Source`. The evaluator tracks source context through the trampoline (save/restore in `Eval`, propagation via `bounceVal.source`), ensuring `RuntimeError` and `ExplainStep` carry the correct source for line/column resolution across module boundaries
- **Structural nesting depth guard** — `budget.Budget` enforces a nesting limit (default 256) on structurally recursive value construction, preventing Go stack overflow from deeply nested Core IR trees

### Engine

- **Caps/Bindings defensive copy** — `RunWith` shallow-copies `Caps` and `Bindings` maps on entry, fulfilling the goroutine-safety contract without relying on caller discipline
- **Spanless diagnostic fix** — errors without source location (e.g. context cancellation) report `Line=0, Col=0` instead of the misleading `1:1`. The human-readable formatter omits the location line entirely
- **Sandbox panic stack trace** — `InternalPanicError` captures the goroutine stack via `runtime.Stack`, preserving diagnostic information while maintaining the same `Error()` message

### Module System

- **Owned-only exports** — `ExportModule` restricts `Types`, constructors, aliases, classes, and promoted kinds/cons to declarations defined by the module itself. Inherited names from imported modules are no longer transitively re-exported, eliminating ghost dependencies. `TypeFamilies` and `Instances` remain fully exported (they accumulate instances across modules)

### CLI

- **JSON output improvements** — `List` values serialized as JSON arrays; `Record` and `Tuple` as objects/arrays. `--json` output is now structurally faithful to GICEL values
- **Explain trace improvements** — `PLit` and list patterns rendered in source-level syntax. `ExplainStep` includes `SourceName` field for multi-module traces

### Lexer

- **Operator boundary guards** — `->`, `<-`, and `:=` reserved symbols inside operators now produce a diagnostic instead of silently splitting tokens. Fixes `=:=`, `->>`, `<->` handling

### Documentation

- **README timeout correction** — sandbox timeout description updated to reflect the actual behavior (timeout covers the entire pipeline including compilation)

### Testing

- **CLI smoke test suite** — 57-case `scripts/smoke-test.sh` covering normal operation, error handling, resource limits, adversarial inputs, list patterns, and malformed inputs
- **Malformed input stress tests** — `tests/stress/stress_malformed_test.go` with 294+ lines of adversarial parser inputs

---

## v0.12.1 — 2026-03-20

### Core

- **`Suspended r a` type alias** — `Suspended r a := Thunk r r a` added to Core module, mirroring `Effect r a := Computation r r a` for state-preserving suspended computations

### Examples

- **`thunk (do {` → `thunk do {`** — all examples now use the parenthesis-free `thunk do { ... }` form instead of `thunk (do { ... })`. Applies to `do-notation.gicel`, `fail-effect.gicel`, `state-effect.gicel`, `state-machine.gicel`, `thunk-force.gicel`
- **Inline single-use computation in `full-grammar.gicel`** — the `computation` binding (thunk + force) replaced with a direct `main := do { ... }`

### Documentation

- **Computation top-level restriction** — spec §2.1.1 and §2.1.3 now explain that bare `Computation` cannot appear at the top level (E0291), when `thunk` is needed, and that value-typed monads are unaffected
- **Agent guide fix** — incorrect bare `do`-block example in effects.md replaced with `thunk do` pattern

---

## v0.12.2 — 2026-03-20

### CLI

- **`docs` topic listing** — `gicel docs` now shows a categorized topic listing (mirroring `gicel example`) instead of dumping the full README. Full overview available via `gicel docs about`

---

## v0.12.0 — 2026-03-19

### Type System

- **OutsideIn(X) L3** — deferred constraint batch replaced with worklist + inert set architecture. Kicked-out constraints get priority re-processing via `OnSolve` callback. Eliminates quadratic re-scanning of the constraint queue
- **CBPV discipline (E0291)** — non-entry top-level bindings with bare `Computation` type are rejected. Entry point (default `main`) is exempt. Enforces the CBPV invariant that top-level bindings are values; computations must be wrapped with `thunk`
- **Quantified constraint resolution fix** — context-evidence search now performs full structural matching (arity + head-arg unification + context compatibility), matching the same precision as global instance search

### Performance

- **Undo-log trail** — Unifier snapshot/restore replaced map-copy with an append-only trail. `Snapshot()` returns a trail position; `Restore()` replays undo entries in reverse. Eliminates O(n) map cloning per trial unification
- **Deque worklist** — two-buffer deque (front stack + back queue with read cursor) replaces slice-based FIFO. Kicked-out constraints go to front for priority processing. All operations amortized O(1)
- **Ambiguity cache** — per-`solveWanteds` cache prevents redundant `isAmbiguousInstance` checks on the same constraint key
- **Precomputed SortBindings** — module bindings are topologically sorted once at `RegisterModule` time and stored in `compiledModule.sortedBindings`, eliminating per-execution re-sorting
- **Precomputed import maps** — import scope insertion consolidated into shared helpers, reducing repeated map construction

### Parser

- **Class head assertion guard** — unchecked `*TyExprVar` type assertions in `parseClassDecl` replaced with defensive checks. Malformed class heads like `class Foo (Maybe a)` now produce a parser error instead of panicking

### Refactoring

Checker restructuring: establish subpackage boundaries, then consolidate constraint solver architecture.

- **`internal/budget` extraction** — unified resource limiter (`Budget`) tracks steps, depth, and allocation bytes across all pipeline phases. Replaces the previous `eval.Limit` type
- **`check/exhaust` subpackage** — Maranget exhaustiveness checking extracted with `DataTypeInfo`/`ConInfo` types. Callback-based `Env` struct decouples from Checker state
- **`check/family` subpackage** — type family reduction engine and injectivity verification extracted. `ReduceEnv` uses callback injection for solver integration
- **`check/env` subpackage** — shared environment type definitions (`AliasInfo`, `ClassInfo`, `InstanceInfo`, `ConstraintInfo`) extracted as canonical home
- **`internal/engine` extraction** — Engine/Runtime/RunSandbox moved from root package. Root `gicel` package becomes a pure facade of type aliases and re-exports; external API unchanged
- **Legacy StuckIndex removal** — `StuckIndex`, `ProcessRework`, and `maxReworkIterations` removed. Inert set with `CtFunEq` constraints is the single mechanism for stuck type family re-activation
- **Injective type key serialization** — `typeNameForMangling` (lossy, head-only) replaced with `WriteTypeKey` (structural, collision-free) in data family mangling
- **`DefaultEntryPoint` constant** — scattered `"main"` literals consolidated into `engine.DefaultEntryPoint` (re-exported as `gicel.DefaultEntryPoint`)
- **Tuple label unification** — all tuple label sites consolidated to `types.TupleLabel`
- **Type key totality** — `WriteTypeKey` panics on unhandled variant instead of falling back to `Pretty`
- **Budget clamping** — negative allocation limits clamped to zero
- **Module boundary hardening** — `SortBindings` precomputation, strict module export filtering
- **File reorganization** — test files renamed to feature convention across `check/`, `engine/`, `eval/`, `parse/`

### Fixes

- **Import ambiguity provenance** — re-export check now verifies dependency chain instead of assuming any re-export suppresses ambiguity. `import B` (re-exporting A.x) + `import C` (native x) is correctly flagged
- **RunWith entry point default** — `RunWith(ctx, nil)` now uses the compile-time entry point (`Runtime.entryName`) instead of hardcoded "main"
- **Compile bare Computation check** — `Compile()` now enforces E0291 consistently with `NewRuntime()`, preventing "check passes but run fails" asymmetry
- **Class head parse guard** — unchecked type assertions in `parseClassDecl` now emit errors instead of panicking on malformed class heads
- **Quantified constraint context search** — full structural matching (arity + head-arg unification) instead of class-name-only match
- **Literal parse error sentinel** — invalid integer/double literals now produce `TyError` sentinel instead of zero-valued Core nodes
- **Unifier probe safety** — `bidir_case.go` tail comparison uses `tryUnify` (trial scope) instead of committed unification

### Safety & Sustainability

- **Panic defaults on sealed switches** — `core.Walk`, `core.Transform`, `core.annotateFV` panic on unhandled Core variants. `check_pattern`, `resolve_type`, `resolve_kind` emit errors for unknown AST forms. Prevents silent degradation when new variants are added
- **File splits** — `types/evidence.go` (569→327+248), `stdlib/list.go` (605→272+341)
- **Dead code removal** — `ErrUnterminatedStr`, `collectContextEvidence`/`classifyEvidence`, dead exports unexported
- **Named constants** — `TyConComputation`/`TyConThunk`, `DefaultEntryPoint`, `sandboxDefaultTimeout`, `prefixSec`/`prefixField`

### Documentation

- **Trust boundary clarification** — README and agent guide now explicitly document that host-registered primitives (`RegisterPrim`) are trusted computing base code, and that `Timeout` bounds evaluation time only
- **CLAUDE.md** — unified to English; stdlib pack name → module name mapping table; package-name-as-feature test naming rule
- **Agent guide restructured** — hierarchical directory structure with dot-separated topic names (`features.records`, `stdlib.prelude`); 6 new feature docs added
- **Examples restructured** — `basics/`, `types/`, `effects/` subdirectories; directory-based CLI grouping
- **Roadmap** — documented fundep improvement as intentional bound; design conventions for tuple labels and compiler-generated names
- **Integer overflow** — specified as Go `int64` wrapping semantics

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
