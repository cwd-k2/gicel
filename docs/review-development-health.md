# Development Health Review

This document summarizes a development-health review of the GICEL codebase,
with emphasis on architectural quality, correctness preservation, and on two project-specific risk
profiles:

1. `check` is prone to semantic false positives and other subtle bugs.
2. `parser` is prone to severe bugs such as non-termination or pathological
   parse behavior.

The review excludes test files as the primary subject, though test-adjacent
signals were used to identify risk concentration.

_Last updated: v0.15.4 (2026-03-23)._

## Executive Summary

The codebase has a strong foundation:

- package layering is clean (verified: no circular dependencies, correct
  direction at every boundary)
- product intent is coherent
- important safety concerns such as budgets, parser guards, and phased
  declaration checking are visible in code

The main quality risk is not lack of structure, but concentration of semantic
authority in a few central places:

- `internal/compiler/check`
- `internal/compiler/parse`
- `internal/app/engine`

This concentration affects quality in two ways:

- correctness becomes harder to preserve because changes cross many responsibilities
- severe regressions are more likely to be interaction bugs than local bugs

## Overall Quality Assessment

### Current strengths

- The top-level package map is understandable: `infra`, `lang`, `compiler`,
  `runtime`, `host`, `app`.
- `registry.Pack` is a good seam between host-facing configuration and stdlib
  registration.
- The declaration pipeline in `check` is explicit rather than implicit.
- Build and baseline static analysis are healthy.

### Current liabilities

- public API is tightly coupled to internal representation choices
- `check` centralizes too much semantic responsibility
- `parse` relies on multiple interacting parser modes and recovery mechanisms
- `engine` is becoming an orchestration catch-all

The codebase already has enough internal complexity that quality should be
judged primarily by how well the structure preserves correctness and prevents
bug reintroduction, not by implementation speed or schedule.

## Architecture Review

### High-level module structure

The coarse-grained structure is good:

- `internal/infra`: low-level shared services
- `internal/lang`: AST, IR, types
- `internal/compiler`: parse, check, optimize
- `internal/runtime`: evaluator
- `internal/host`: stdlib and registration
- `internal/app`: engine and API orchestration

This is a sound shape. Dependency direction is verified clean: `lang` has no
imports from `compiler` or `runtime`; `parse` depends only on `syntax` and
`infra`; `check` depends only on `lang/*` and `infra/*`; `eval` depends only on
`ir` and `infra`. No circular dependencies exist.

The issue is not layer violations but intra-package cohesion.

### Root package coupling

The root `gicel` package re-exports many internal types and functions directly
via aliases and pass-through bindings: 38 type aliases, 22+ function bindings,
and 13 constants from 7 internal packages (`engine`, `eval`, `types`, `check`,
`budget`, `registry`, `stdlib`). This is convenient for users, but it creates a
strong compatibility lock between internal implementation and public API.

Structural effect:

- internal refactors become API events
- representation debt escapes the facade instead of being absorbed by it
- long-term cleanup becomes politically harder because compatibility pressure
  accumulates outside the implementation packages

### `engine` cohesion

`internal/app/engine` currently owns (10 files, ~1,280 lines):

- host registration state (`hostenv.go`)
- compilation orchestration (`pipeline.go`, 5-stage pipeline)
- module registration/store (`modstore.go`)
- runtime assembly (`runtime.go`)
- sandbox entrypoints (`sandbox.go`)
- helper-facing type/value utility API (`typehelpers.go`, `convert.go`: 24 type
  helpers + 8 conversion helpers)

This makes `engine` useful, but weakens cohesion.

Structural effect:

- compile policy changes, runtime changes, and sandbox contract changes land in
  the same package
- `engine` becomes the default landing zone for unrelated feature work

### Export object overloading

`env.ModuleExports` (12 fields, moved from `checker.go` to `check/env/`) is
carrying multiple conceptual payloads at once:

- semantic exports for other modules (9 fields: `Types`, `ConTypes`,
  `ConstructorInfo`, `Aliases`, `Classes`, `TypeFamilies`, `Values`,
  `PromotedKinds`, `PromotedCons`)
- module ownership/re-export signals (`OwnedTypeNames`, `OwnedNames`,
  `ConstructorsByType` as precomputed index)
- coherence data (`Instances`, never filtered — by design)

Runtime no longer receives compiler IR for module ownership decisions
(`OwnedTypeNames`/`OwnedNames` maps). The structural mixing of semantic
exports and ownership signals in one struct remains.

Structural effect:

- compiler changes can still have import-semantic consequences
- module import semantics and runtime assembly are harder to separate later

## Cohesion and Coupling Review

### `internal/compiler/check`

This is the highest-risk package for structural quality (47 implementation files,
~9,750 lines; largest: `unify/unify.go` 642, `modscope/import.go` 562,
`checker.go` 528, `bidir.go` 400).

It coordinates:

- declaration processing
- type inference / checking
- solver lifecycle
- type family reduction
- instance resolution
- evidence resolution
- export semantics
- diagnostics
- mutable contextual state

Import processing is in the `modscope` subpackage (callback-based `ImportEnv`).
Other decompositions include `env`, `family`, `unify`, and `exhaust`. The
package boundary still centralizes significant authority, but import/checking
coupling has been severed.

Structural effect:

- changes are rarely local
- review requires broad semantic context
- regressions are likely to be cross-feature

### `internal/compiler/parse`

The parser (7 implementation files, ~2,400 lines + lexer 560 lines) has explicit
safety mechanisms, which is good, but it also has several interacting contextual
controls:

- `depth`
- `noBraceAtom`
- `stmtBoundaryDepth`
- `methodBodyMode`
- `guard`

This is a warning sign in a hand-written parser. Parsing correctness is not
driven only by grammar; it is driven by grammar plus mode interaction.

Structural effect:

- local reasoning about correctness is weak
- edge-case fixes can perturb unrelated paths
- termination reasoning must include recovery and mode logic, not just parsing

## Structural Causes of `check` Bugs

The project correctly identifies `check` as a place where false positives and
other semantic bugs are likely to appear. The code structure explains why.

### 1. Shared mutable semantic state

The checker is built around shared mutable state:

- `Session`
- `Registry`
- `Scope`
- `Solver`
- typing `Context`

Most semantic operations are not purely local derivations. They mutate or
inspect shared evolving state.

Structural consequence:

- bug causes are often non-local
- a feature can fail because of prior semantic state, not its own rule
- false positives are likely to manifest as state interaction bugs

### 2. Semantically significant phase ordering

The declaration pipeline is explicit, which is good, but correctness depends on
the precise ordering of phases.

Examples of order-sensitive work:

- type registration
- class registration
- instance header processing
- family reducer installation
- annotation collection
- assumption processing
- preregistration of bindings
- instance body checking
- value checking

Structural consequence:

- inserting a new feature into the wrong phase can produce legitimate-looking
  but incorrect diagnostics
- phase mistakes often surface as user-facing type errors rather than obvious
  structural failures

### 3. Trial unification is only partially isolated

The checker has `withTrial`, `withProbe`, `saveState`, and `restoreState`, but
the snapshot rolls back only unifier state. `checkerSnapshot` contains a single
`unify.Snapshot` field; metavariable solutions, row bindings, kind solutions, and
skolem solutions are restored.

The following state is **not** rolled back:

- **Inert set** (`classMap`, `funEqs`, `metaIndex`, `resolutionKeys`): constraints
  inserted during a trial are not removed on rollback. This is currently safe
  because `solveWanteds()` resets the inert set at entry.
- **Context stack**: push/pop operations are not undone.
- **Constraint worklist**: not saved or restored by `withTrial`/`withProbe`.
  `tryResolveInstance` handles worklist isolation separately.
- **Error accumulation**: not rolled back by `withTrial`/`withProbe`.
  `tryResolveInstance` handles error truncation separately.

That is a useful tool, but it also means trial reasoning is only as isolated as
the state model being snapshotted.

`withProbe` (always rollback) and `withTrial` (commit on success) express
intent explicitly. Both carry MUST NOT contracts and save/restore
`SolverLevel` for touchability isolation. `tryResolveInstance` saves/restores
the worklist independently.

Structural consequence:

- snapshot scope is limited to unifier state — intentional design
- MUST NOT contracts are documented but not mechanically enforced

### 4. Constraint solving and elaboration are tightly interwoven

Deferred class constraints are pushed into the solver, then later discharged,
deferred, generalized, or substituted into IR.

Important transitions include:

- eager resolution in check mode
- ambiguity-based deferral in infer mode
- let-generalization of unresolved constraints
- instance ambiguity checks using trial unification

Structural consequence:

- inference behavior and emitted diagnostics depend on a multi-stage pipeline,
  not a single local judgment
- a small change in deferral, ambiguity, or generalization can look like a
  change in type-checking truth rather than in elaboration strategy

This is a structural reason why false positives are likely.

### 5. Instance resolution mixes search, improvement, and evidence construction

`resolveInstance` performs several responsibilities in one flow:

- context search
- superclass extraction
- quantified evidence application
- functional dependency improvement
- global instance search
- recursive dictionary construction

Structural consequence:

- "no instance" errors can arise from subtle control-flow interactions, not just
  absent instances
- search strategy changes can alter type-check results in surprising ways
- recursive and improving behaviors are close enough together that regressions
  can be difficult to localize

### 6. Import/module semantics are structurally separated from semantic checking

Import logic lives in `modscope.Importer` (callback-based `ImportEnv`).
`Scope` is a pure scoping data container (4 fields).

Structural consequence:

- qualified name resolution still bridges `modscope.QualifiedScope` and checker
  state via `Scope.LookupQualified`

## Structural Causes of `parser` Non-Termination and Severe Parser Bugs

The project also correctly identifies `parser` as a source of potentially
catastrophic bugs. The current parser structure supports that diagnosis.

### 1. The parser is highly modeful

The parser does not only consume tokens. It also manages mutable contextual
parsing modes.

Examples:

- brace-as-atom suppression
- statement boundary interpretation
- method-body newline behavior
- recursion depth and token-step safety budget

Structural consequence:

- parser behavior is controlled by hidden state as much as by grammar
- proving progress or correctness locally is hard
- a narrowly targeted edge-case fix can break distant cases

### 2. Safety is layered but not unified

The parser has six layers of defense:

1. `guard.halted` — once triggered, all further parsing stops
2. step counting — global token budget (`len(tokens)*4`, min 100)
3. recursion depth limit — per-recursion call stack bound (default 256)
4. `parseBody` iteration cap — local loop bound (`len(tokens)*2`, min 100)
5. stagnation detection — `before/after pos` progress check per body iteration
6. context cancellation — external timeout via `<-ctx.Done()`

This is defense in depth, not a single mechanism. The layers cover different
failure modes (infinite recursion, stalled loops, runaway recovery, external
timeout). However, each layer is independently maintained.

Structural consequence:

- parser safety depends on remembering to add local protections in complex loops
- termination is not guaranteed by one central invariant alone
- new control-flow paths can bypass individual layers
- the layered model is stronger than it appears, but its correctness is a
  distributed property rather than a single verifiable contract

This is a structural reason severe parser bugs can recur, even though the current
layers are comprehensive.

### 3. Progress is enforced by conventions, not by a single contract

The parser repeatedly uses a pattern like:

- record `before := p.pos`
- call a parsing/recovery callback
- if no progress was made, synchronize
- if still no progress, force `advance()`

This is pragmatic and valuable, but it is a distributed convention.

Structural consequence:

- safety depends on each loop remembering to defend against stagnation
- the abstraction boundary does not inherently guarantee progress
- regressions are likely when a new loop or parser mode forgets the convention

### 4. Speculative parsing is not fully transactional

`speculate` (5 call sites) rolls back:

- `pos`
- `depth`
- `guard.steps` (restored on rollback)
- error count (`errors.Truncate`)

It does not roll back:

- `guard.halted` — if step/recursion limit fires during speculation, it is
  permanent
- mode flags (`noBraceAtom`, `stmtBoundaryDepth`, `methodBodyMode`) — not
  saved/restored by `speculate` itself, though they are generally protected by
  `defer` in callers

`speculate` saves and restores `guard.steps` on rollback.

Structural consequence:

- future speculative call sites may accidentally leak contextual state
- `guard.halted` during speculation is permanent (low risk — bounded by budget)

### 5. Error recovery and grammar disambiguation are deeply intertwined

Important parser helpers simultaneously affect correctness and safety:

- statement boundary detection
- declaration synchronization
- body parsing
- speculative disambiguation
- contextual newline handling

Structural consequence:

- recovery is not a separate concern; it participates in core parse control flow
- ambiguity fixes can accidentally change termination behavior
- catastrophic parser bugs are likely to emerge from interactions, not isolated
  functions

### 6. Context-sensitive parsing is spread across many files

The parser logic is split into declaration, class/instance, expression, and type
files, which is good organizationally. But some crucial mode interactions span
those file boundaries.

Structural consequence:

- a maintainer must hold multiple files in working memory to reason about one
  grammar corner case
- context-sensitive behavior is easy to accidentally duplicate or drift

## Practical Risk Model

### Most likely recurring `check` bug class

- false "no instance" diagnostics
- false ambiguity/import ownership diagnostics
- incorrect deferral/generalization behavior
- feature interactions involving fundeps, families, or quantified evidence

### Most likely recurring `parser` bug class

- stagnation or quasi-stagnation on malformed input
- pathological step blowups from speculation plus recovery
- newline/boundary regressions inside brace-delimited constructs
- edge-case grammar fixes reintroducing older catastrophic behaviors

## Recommendations

### High priority

1. Narrow the authority of `internal/compiler/check`.
   Import logic extracted to `modscope`; Registry encapsulated; `Scope` reduced
   to 4 fields. **Remaining**: constraint solving / elaboration decoupling;
   instance resolution decomposition.

2. Strengthen parser progress as a first-class invariant.
   `progressGuard` applied to all unbounded loops. **Complete.**

3. Make parser speculation explicitly transactional or explicitly constrained.
   `speculate` saves/restores all mutable state; bounded loops have explicit
   terminators. **Complete.**

4. Separate semantic exports from runtime module metadata.
   `DataDecls` replaced with `OwnedTypeNames`/`OwnedNames`. **Remaining**:
   `ModuleExports` still carries 12 fields mixing semantic exports, ownership
   signals, and coherence data.

5. Reduce direct root-package alias exposure over time.
   **Not started.**

### Medium priority

1. Split `engine` into orchestration and sandbox/policy surfaces.
2. Document allowed dependency directions between major packages.
   Verified clean but not explicitly documented.
3. Add a maintainer-facing note documenting `check` phase ownership and parser
   rollback/progress invariants. Phase annotations and contracts exist in code;
   comprehensive maintainer guide not yet written.

## Quantitative Profile

### Package scale

| Package             | Impl files | Impl lines    | Test lines | Test ratio |
| ------------------- | ---------- | ------------- | ---------- | ---------- |
| `compiler/check`    | 49         | ~10,000       | ~18,400    | 1.84       |
| `compiler/parse`    | 7 (+lexer) | ~2,400 (+560) | ~7,800     | 2.51       |
| `compiler/optimize` | 3          | ~270          | ~690       | 2.55       |
| `runtime/eval`      | 10         | ~1,700        | ~2,900     | 1.70       |
| `app/engine`        | 10         | ~1,280        | ~9,000     | 7.03       |
| `lang/types`        | 8          | ~2,380        | ~1,840     | 0.77       |
| `lang/ir`           | 6          | ~1,160        | ~310       | 0.27       |
| `lang/syntax`       | 4          | ~880          | 0          | **0.00**   |

### Test coverage gap

`internal/lang/syntax` has zero test lines. While it is primarily AST struct
definitions, it also contains node constructors and helper methods that are
implicitly tested through parser and checker tests but never directly exercised.

`internal/lang/ir` has a low ratio (0.27). Core IR transformations (`Walk`,
`Transform`, `Free`) contain exhaustiveness-check panics (12 sites across
`walk.go`, `free.go`, and `optimize.go`) that would fire on unhandled node types
rather than being caught by tests.

### Root package exposure

The root `gicel` package re-exports 73+ bindings from 7 internal packages:

| Category          | Count |
| ----------------- | ----- |
| Type aliases      | 38    |
| Function bindings | 22+   |
| Constants         | 13    |

### Concurrency model

The codebase is strictly single-threaded. No goroutines, no channels, no
mutexes. `Budget`, `Engine`, and `Parser` all document non-goroutine-safety
explicitly. External timeout is handled via `context.Context` cancellation
checked at parser and checker entry points.

## Final Assessment

This is a strong and serious codebase. The review is not saying the project is
architecturally weak. It is saying the current structure now matters directly to
semantic quality and safety, especially in `check` and `parser`.

The key structural issue is concentration:

- too much semantic authority in `check`
- too much contextual state in `parse`
- too much coordination responsibility in `engine`

Given the project's own observed bug profile, the structure matches the risk:

- `check` is structurally vulnerable to false positives because correctness is
  distributed across shared mutable state, phase ordering, speculative
  unification, deferred constraints, and interacting semantic engines.
- `parser` is structurally vulnerable to severe bugs because parsing, recovery,
  contextual mode handling, and termination safeguards are deeply interwoven and
  not fully governed by one central transactional/progress model.

The next quality win is not primarily more features. It is reducing the
structural blast radius of the two highest-risk subsystems: `check` and
`parser`, so that correctness is easier to preserve and severe regressions are
harder to reintroduce.

