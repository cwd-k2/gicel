# Performance Review

Date: 2026-03-19

Scope:

- Current repository state
- Compile-time and runtime performance characteristics
- Existing benchmark quality and missing benchmark coverage
- Includes both confirmed hotspots and structurally likely hotspots

This review is stricter than a style or architecture review.
It focuses on:

- asymptotic cost
- avoidable allocation
- repeated whole-structure copying
- hot-path drift between intended and actual behavior
- the quality of the current benchmark suite

## Executive Judgment

The repository has some performance-aware design decisions already:

- evaluator environments use parent chaining rather than map-copy on every extend
- unifier zonking uses path compression
- type family reduction now has a work budget
- compile and runtime are cleanly separated enough to benchmark independently

However, there are still several important issues.

The most important confirmed problems are:

- `Env.Lookup` does not actually trigger the flattening optimization described by its own comment
- `Check` still pays the full `CheckModule` export-construction cost even when exports are unused
- `types.UnwindApp` is quadratic in the depth of the application spine
- the optimizer always does four full tree rewrites, even when nothing changes

Separately, the benchmark situation is materially incomplete.
The codebase has only a handful of microbenchmarks, no engine-level benchmarks,
and one of the main stress benchmark entry points currently fails before it can
measure anything meaningful.

## Observed Benchmark State

Existing benchmark coverage is very small.

Current benchmark definitions:

- [bench_test.go](/Users/cwd-k2/Projects/gicel/internal/check/bench_test.go)
- [eval_test.go](/Users/cwd-k2/Projects/gicel/internal/eval/eval_test.go#L252)
- [stress_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/stress_test.go#L752)

Observed benchmark runs during this review:

- `BenchmarkInstanceResolve100`: `30764 ns/op`, `28187 B/op`, `344 allocs/op`
- `BenchmarkZonkDeepChain`: `720.1 ns/op`, `0 B/op`, `0 allocs/op`
- `BenchmarkEnvExtend100`: `13682 ns/op`, `8424 B/op`, `302 allocs/op`

There are currently no dedicated benchmarks in:

- `internal/engine`
- `internal/opt`
- `internal/check/family`
- `internal/syntax/parse`

The stress benchmark harness exists, but `BenchmarkStressCompile` currently fails
for many programs because the harness does not preload the packs/modules those
programs expect.

Relevant code:

- [stress_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/stress_test.go#L752)
- [stress_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/stress_test.go#L756)

## Findings

## 1. High: `Env.Lookup` never triggers the flattening optimization it advertises

Severity: High

Status:

- unresolved
- confirmed in current code
- very likely to matter in deep lexical environments

Relevant code:

- [env.go](/Users/cwd-k2/Projects/gicel/internal/eval/env.go#L20)
- [env.go](/Users/cwd-k2/Projects/gicel/internal/eval/env.go#L51)
- [env.go](/Users/cwd-k2/Projects/gicel/internal/eval/env.go#L101)

Problem:

The `Env` design comment says lookup will trigger flattening when the parent chain
exceeds `flatThreshold`.
The actual `Lookup` implementation does not call `flatten()` at all.

Instead it:

- checks `e.flat`
- otherwise linearly walks `parent`
- returns without caching

Why it matters:

- lookup cost remains O(depth) even for deep environments
- repeated lookups in nested closures do not amortize
- the implementation no longer matches the intended performance model

Evidence:

The benchmark:

- [eval_test.go](/Users/cwd-k2/Projects/gicel/internal/eval/eval_test.go#L252)

measures repeated `Extend` plus one lookup over 100 bindings and reports:

- `13682 ns/op`
- `8424 B/op`
- `302 allocs/op`

That benchmark is not a pure lookup benchmark, but it is consistent with the fact
that deep environments are still fully traversed.

Recommended next step:

Either:

1. make `Lookup` actually trigger `flatten()` once the threshold is exceeded

or

2. change the design comment and choose a different lookup strategy explicitly

At the moment the design intent and implementation are not aligned.

## 2. High: main-source compilation still pays full module-export construction cost

Severity: High

Status:

- unresolved
- confirmed in current code

Relevant code:

- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L190)
- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L196)
- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L237)
- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L243)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L361)

Problem:

`Check` is just a wrapper around `CheckModule`.
That means even when the caller only needs a `*core.Program`, the checker still builds
full `ModuleExports` data and clones/maps several registries.

The most obvious costs are:

- `maps.Clone(ch.config.RegisteredTypes)`
- `maps.Clone(ch.reg.conTypes)`
- `maps.Clone(ch.reg.promotedKinds)`
- `maps.Clone(ch.reg.promotedCons)`
- `cloneFamilies(ch.reg.families)`

Why it matters:

- `NewRuntime` uses `Check`, not `CheckModule`, but still pays module-export cost
- compile-time fixed overhead rises even for programs that never need exports
- repeated short-lived compilations are penalized disproportionately

Recommended next step:

Split the checker exit paths:

- one path for `Check` that returns only `*core.Program`
- one path for `CheckModule` that additionally materializes exports

This is straightforward, high-signal performance work because the extra export object
is pure overhead for `NewRuntime`.

## 3. High: `types.UnwindApp` is quadratic in application depth

Severity: High

Status:

- unresolved
- confirmed in current code
- compile hot-path risk

Relevant code:

- [type.go](/Users/cwd-k2/Projects/gicel/internal/types/type.go#L195)
- [type.go](/Users/cwd-k2/Projects/gicel/internal/types/type.go#L204)

Problem:

`UnwindApp` prepends each discovered argument with:

- `append([]Type{app.Arg}, args...)`

That reallocates and copies the partial slice on each iteration.

Therefore:

- depth `n` application spine
- O(n^2) slice work

Why it matters:

`UnwindApp` is not a rare helper.
It is used in several semantic hot paths, including:

- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L176)
- [elaborate_do_monadic.go](/Users/cwd-k2/Projects/gicel/internal/check/elaborate_do_monadic.go#L51)
- [resolve.go](/Users/cwd-k2/Projects/gicel/internal/check/resolve.go#L127)
- [unify.go](/Users/cwd-k2/Projects/gicel/internal/check/unify/unify.go#L414)

So deep type application structures can repeatedly pay quadratic spine decomposition cost.

Recommended next step:

Rewrite `UnwindApp` to:

1. collect args in reverse order with plain append
2. reverse the final slice once

That reduces the helper to O(n).

## 4. Medium: optimizer always performs four full-tree passes even when nothing changes

Severity: Medium

Status:

- unresolved
- confirmed in current code

Relevant code:

- [optimize.go](/Users/cwd-k2/Projects/gicel/internal/opt/optimize.go#L11)
- [optimize.go](/Users/cwd-k2/Projects/gicel/internal/opt/optimize.go#L17)
- [optimize.go](/Users/cwd-k2/Projects/gicel/internal/opt/optimize.go#L19)
- [walk.go](/Users/cwd-k2/Projects/gicel/internal/core/walk.go#L65)

Problem:

`Optimize` always runs exactly four `core.Transform` passes over the whole tree.
There is no fixed-point detection and no short-circuit when the previous pass made
no changes.

This is expensive because `Transform` reconstructs nodes bottom-up.

Why it matters:

- compile time includes four full Core rewrites even for trivial programs
- allocation cost scales with tree size, not only with actual optimization opportunities
- custom rewrite rules make this even more expensive because all rules are applied at every node on every pass

Recommended next step:

Track whether a pass changed anything.
Stop early when a pass is a no-op.

If a stronger guarantee is needed, keep the cap of four passes but only continue
while changes are still occurring.

## 5. Medium: compile path clones multiple engine maps on every compilation

Severity: Medium

Status:

- unresolved
- currently safe but not cheap

Relevant code:

- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L247)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L255)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L258)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L389)
- [engine.go](/Users/cwd-k2/Projects/gicel/internal/engine/engine.go#L392)
- [prim.go](/Users/cwd-k2/Projects/gicel/internal/eval/prim.go#L35)

Problem:

Every compilation or runtime build clones several maps and registries:

- registered types
- assumptions
- bindings
- gated builtins
- primitive registry

Why it matters:

- compile throughput for many short-lived compilations is worse than necessary
- fixed overhead grows with engine configuration size, not program size
- larger host embeddings with many registered modules/primitives will feel this first

Important nuance:

This may still be the right default for safety and immutability.
The problem is not “cloning exists”.
The problem is that the cost is currently unmeasured and unconditional.

Recommended next step:

Benchmark these costs directly and then choose deliberately:

- keep the clones
- share immutable structures
- or use copy-on-write containers for engine configuration state

## 6. Medium: evaluator application path allocates on every constructor and primitive application

Severity: Medium

Status:

- unresolved
- confirmed in current code

Relevant code:

- [eval_apply.go](/Users/cwd-k2/Projects/gicel/internal/eval/eval_apply.go#L103)
- [eval_apply.go](/Users/cwd-k2/Projects/gicel/internal/eval/eval_apply.go#L111)

Problem:

Applying a `ConVal` or `PrimVal` always allocates a fresh argument slice and copies
the existing prefix.

This is simple and correct, but it means curried application incurs repeated small
allocations in a core runtime path.

Why it matters:

- DSL-style code tends to use many small curried applications
- constructor-heavy and primitive-heavy programs will allocate proportionally
- runtime throughput becomes sensitive to partial application style

Recommended next step:

Benchmark representative workloads first.
If this is material, consider:

- small-arity specialized storage
- cons-style arg accumulation
- immutable shared prefix structures

This should be benchmark-led; the current approach is clean and may still be acceptable.

## 7. Medium: trial unification snapshots copy whole unifier state per candidate

Severity: Medium

Status:

- unresolved
- structurally likely hotspot

Relevant code:

- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L359)
- [checker.go](/Users/cwd-k2/Projects/gicel/internal/check/checker.go#L375)
- [unify.go](/Users/cwd-k2/Projects/gicel/internal/check/unify/unify.go#L112)
- [resolve.go](/Users/cwd-k2/Projects/gicel/internal/check/resolve.go#L104)
- [resolve.go](/Users/cwd-k2/Projects/gicel/internal/check/resolve.go#L140)
- [resolve.go](/Users/cwd-k2/Projects/gicel/internal/check/resolve.go#L209)

Problem:

`withTrial` snapshots the unifier and stuck-family state before candidate checks.
The unifier snapshot deep-copies:

- solutions
- label contexts
- kind solutions

This happens inside instance resolution and superclass search.

Why it matters:

- candidate-heavy instance search can amplify snapshot cost
- the benchmark coverage for this path is too small to show scaling behavior clearly
- the current `BenchmarkInstanceResolve100` is helpful but narrow

Recommended next step:

Add scaling benchmarks for:

- instance count
- context size
- superclass depth
- fundep-heavy classes

This may or may not require redesign.
But the current cost model is too opaque.

## 8. Medium: import ambiguity checks are more expensive than they need to be at scale

Severity: Medium

Status:

- unresolved
- likely not critical for small module graphs
- could matter in large imported module sets

Relevant code:

- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L74)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L104)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L117)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L273)
- [import.go](/Users/cwd-k2/Projects/gicel/internal/check/import.go#L289)

Problem:

Ambiguity handling repeatedly does ownership checks that scan:

- `DataDecls`
- constructor lists
- direct dependency value maps

Selective import submember checks also use `slices.Contains` on sublists.

Why it matters:

- import processing cost scales with export graph size
- larger module ecosystems will pay repeated linear scans during compilation
- this is exactly the kind of overhead that stays invisible until a codebase grows

Recommended next step:

Precompute:

- per-module owned exported names
- constructor ownership map
- class method membership sets for selective import checks

This is a secondary optimization, but it is a clean one.

## 9. Low: type family reduction now has a work budget, but the cost model is still under-benchmarked

Severity: Low

Status:

- partly mitigated
- still under-measured

Relevant code:

- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L25)
- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L37)
- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L149)
- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L206)

Problem:

The worst exponential path is now bounded by `MaxReductionWork`, which is good.
But the reduction engine still combines:

- recursive traversal
- repeated zonking
- family-application caching
- `MapType` rebuilding

and there are no dedicated benchmarks that characterize its normal-case and near-limit behavior.

Why it matters:

- the new safety guard prevents hangs, but not necessarily unacceptable compile latency
- “safe” and “fast enough” are different questions

Recommended next step:

Add benchmarks for:

- deep linear reduction chains
- broad nested family trees
- stuck-family rework
- near-budget reductions that do not exceed the limit

## 10. Low: benchmark coverage is too sparse to support regression detection

Severity: Low

Status:

- unresolved
- benchmark infrastructure gap

Problem:

The current benchmark suite does not cover several core subsystems at all.

Missing or weakly covered areas:

- parse-only throughput
- compile-only throughput through `Engine`
- `NewRuntime` throughput with and without modules
- optimizer cost on large Core programs
- runtime apply-heavy workloads
- import-heavy module graphs
- type family reduction scaling
- stress benchmarks that actually run successfully in CI

Why it matters:

- several current performance claims are not backed by measurement
- regressions can land unnoticed in hot paths that have zero benchmark coverage
- optimization work has no stable scoreboard

## Benchmark Review

## 1. Current benchmark suite is too micro-heavy and too narrow

Existing microbenchmarks are useful, but they do not compose into a realistic picture.

For example:

- `BenchmarkEnvExtend100` mixes extend allocation and one lookup
- `BenchmarkInstanceResolve100` covers one source shape
- `BenchmarkZonkDeepChain` measures a favorable zero-allocation path

None of these tell you:

- end-to-end compile throughput
- runtime throughput under realistic closures and effects
- module import overhead
- optimizer cost

## 2. Stress benchmark harness is currently not trustworthy

The intended end-to-end benchmark harness in:

- [stress_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/stress_test.go#L752)

currently fails for many stress programs because it creates a fresh engine and only
applies `sp.setup`, without consistently loading required packs such as Prelude.

That means the codebase currently has a benchmark file that looks comprehensive
but is not actually serving as a reliable regression suite.

This is worth fixing early, because a working stress benchmark suite would provide
much more value than many new microbenchmarks by itself.

## 3. Recommended benchmark additions

The highest-value additions are:

### Engine benchmarks

- `BenchmarkEngineParseSmall`, `BenchmarkEngineParseLarge`
- `BenchmarkEngineCompileSmall`, `BenchmarkEngineCompileLarge`
- `BenchmarkEngineNewRuntimeSmall`, `BenchmarkEngineNewRuntimeLarge`
- `BenchmarkEngineNewRuntimeWithModulesN`

These should report:

- `ns/op`
- `B/op`
- `allocs/op`

and ideally compare:

- no modules
- Prelude only
- Prelude plus several user modules

### Checker benchmarks

- `BenchmarkResolveInstancesScale_10_100_1000`
- `BenchmarkTypeFamilyReduceLinear`
- `BenchmarkTypeFamilyReduceBroad`
- `BenchmarkImportHeavyModuleGraph`
- `BenchmarkCheckDoDeepChain`

### Evaluator benchmarks

- `BenchmarkEvalClosureChain`
- `BenchmarkEvalPrimCurriedChain`
- `BenchmarkEvalConstructorHeavy`
- `BenchmarkEvalDeepEnvLookup`
- `BenchmarkEvalEffectsStateLoop`

### Optimizer benchmarks

- `BenchmarkOptimizeNoopLarge`
- `BenchmarkOptimizeBetaHeavy`
- `BenchmarkOptimizeRewriteRulesN`

The noop benchmark is especially important because it reveals the baseline tax
of the current four-pass architecture.

## 4. Benchmark design guidance

To make the benchmark suite actually useful:

- separate parse, check, optimize, runtime assembly, and evaluation costs
- keep one canonical small program and one canonical large program per subsystem
- add scaling series rather than only one arbitrary size
- ensure all end-to-end benchmarks run without missing-module setup failures
- avoid benchmarks that accidentally measure setup unrelated to the subsystem under test
- keep benchmark source programs deterministic and checked into the repository

## 5. Suggested benchmark priorities

If benchmark work must be staged, the best order is:

1. Fix `BenchmarkStressCompile` / `BenchmarkStressEval` so they actually run.
2. Add engine-level compile/runtime benchmarks.
3. Add evaluator deep-env and curried-application benchmarks.
4. Add checker benchmarks for type family reduction and instance search scaling.
5. Add optimizer noop/large-tree benchmarks.

## Synthesis

The performance story is currently mixed:

- there are some good low-level choices
- there are also several clear hotspots and avoidable fixed costs
- and the benchmark suite is not yet strong enough to keep the codebase honest

The most important concrete work items are:

1. make `Env.Lookup` match its intended flattening strategy
2. stop building exports on the `Check` path when they are unused
3. make `UnwindApp` linear
4. add change detection to the optimizer
5. repair and expand the benchmark suite so future work is measurable

