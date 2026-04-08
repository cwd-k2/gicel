# Performance Overview

A structured map of where GICEL spends time, what we measure, and how
to take comparable snapshots.

## How to read this document

GICEL has three execution phases that need to be measured separately:

1. **Compile** — `lex → parse → check → optimize → annotate → emit
bytecode`. One-shot per source. Heavy on allocations from the IR
   graph, type metadata, and bytecode emit pools.
2. **Runtime startup** — `precompileVM → buildGlobalArray → seed
capEnv`. Done once per `RunWith` (or once-cached per process,
   depending on hot-path). Allocates the VM struct, the globals
   table, and module-binding closure values.
3. **Runtime exec** — the bytecode dispatch loop. Hot, called many
   times per `RunWith` invocation. Cost is dominated by `apply` /
   `applyN` / closure entry, and the per-call cost of effectful
   primitives.

Conflating them obscures the real cost source. Every benchmark in
this repo belongs to one of these phases, plus a "micro" tier for
isolated subsystems.

## The benchmark map

The four bench tiers, sorted from "closest to user wall-clock" to
"closest to a single instruction":

### Tier 1 — End-to-end (`tier=cold`)

Simulate `gicel run program.gicel`: every iteration creates a fresh
engine, compiles, runs, and discards. Closest to a real CLI invocation.

| Benchmark                             | Workload                          |
| ------------------------------------- | --------------------------------- |
| `BenchmarkEngineEndToEndSmall`        | Trivial `main := 1 + 2`           |
| `BenchmarkEngineEndToEndSmallCold`    | Same, with module cache reset     |
| `BenchmarkEngineEndToEndArray`        | `Effect.Array` write/read loop    |
| `BenchmarkEngineEndToEndMap`          | Three `Map.insert` + `Map.lookup` |
| `BenchmarkEndToEndMapInsert50`        | 50 sequential `Map.insert`        |
| `BenchmarkEndToEndMutableMapInsert50` | 50 `Effect.Map.insert`            |
| `BenchmarkEndToEndSetAlgebra`         | Set union/intersection/difference |

These dominate compile time (97-99% of wall time for the Map/Set
workloads after the 2026-04-07 dispatch refactor). When you optimize
runtime exec, these will show small movements; when you optimize
compile, they will show large movements.

File: `internal/app/engine/engine_bench_test.go`

### Tier 2 — Pre-compiled exec (`tier=warm`)

Compile once outside the loop, then time only `RunWith`. This isolates
steady-state runtime cost from compile cost. The right tool for
investigating VM dispatch, allocation churn, primitive call overhead.

| Benchmark                         | Workload                       |
| --------------------------------- | ------------------------------ |
| `BenchmarkExecSmall`              | Trivial `main`                 |
| `BenchmarkExecArray`              | `Effect.Array` write/read loop |
| `BenchmarkExecMapInsert50`        | 50 `Map.insert` (warm)         |
| `BenchmarkExecMutableMapInsert50` | 50 `Effect.Map.insert` (warm)  |
| `BenchmarkExecSetAlgebra`         | Set algebra (warm)             |
| `BenchmarkExecArithmeticLoop`     | `fix`-driven `sumTo 1000`      |

File: `internal/app/engine/engine_vm_bench_test.go`

Comparison against the cold counterpart reveals the compile share:

```
EndToEndMapInsert50:        6369 µs   (cold)
ExecMapInsert50:             113 µs   (warm, same source)
                            ─────────
compile share:                98%
```

### Tier 3 — Pure compile / runtime construction

Time `NewRuntime` only (no `RunWith`). Where the type checker,
optimizer, and bytecode emitter spend their time.

| Benchmark                              | What it stresses             |
| -------------------------------------- | ---------------------------- |
| `BenchmarkEngineCompileSmall`          | Minimal compile              |
| `BenchmarkEngineCompileLarge`          | 100-decl source              |
| `BenchmarkEngineCompileLarge500`       | 500-decl source              |
| `BenchmarkEngineCompileDoBlock10/30`   | Do-notation desugar at scale |
| `BenchmarkEngineNewRuntimeNoModules`   | Pure NewRuntime, no Prelude  |
| `BenchmarkEngineNewRuntimePrelude`     | NewRuntime + Prelude         |
| `BenchmarkEngineNewRuntimeWithModules` | NewRuntime + 3 user modules  |
| `BenchmarkEngineNewRuntimeAllPacks`    | All stdlib packs             |

File: `internal/app/engine/engine_bench_test.go`

### Tier 4 — Subsystem micro

Isolated subsystem benchmarks. Use these to validate that an
optimization in one component doesn't regress an unrelated one.

- `internal/compiler/parse/` — lexer, parser, expression depth
- `internal/compiler/check/` — instance resolve, type-family reduce,
  unify, super-class depth, do-block check, large-decl check
- `internal/compiler/check/unify/` — meta-solve, snapshot/restore,
  identical unify
- `internal/compiler/check/solve/` — worklist push/pop, inert set
  insert/kick-out
- `internal/compiler/optimize/` — beta-heavy, rewrite rules
- `internal/lang/types/` — equality, key, freevars, subst (capture
  and many)
- `internal/infra/budget/` — step, alloc, enter/leave (these are in
  the inner loop of every reduction)

## Allocation atlas (post Step 5, 2026-04-07)

Profile: `BenchmarkExecArray` and `BenchmarkExecMapInsert50` after
the 6-commit refactor `35254ee..9c68906`.

### Where the allocations live

| Source                              | Share | Phase           | Notes                                                                                                                                |
| ----------------------------------- | ----- | --------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `vm.applyPrim`                      | ~20%  | runtime exec    | Down from 31% pre-refactor. Remainder is `applyForPrim` (host-callback) and curried-via-local paths the unified dispatch can't reach |
| `Compiler.leaveFrame`               | ~13%  | compile         | Builds the per-Proto state struct from the stack frame. Likely unavoidable without arena alloc                                       |
| `Compiler.addSpan`                  | ~9%   | compile         | Span pool growth — one entry per IR node visited                                                                                     |
| `optimize.substMany`                | ~9%   | compile         | IR substitution copies. Step toward in-place mutation already partial (TransformMut)                                                 |
| `runtime.NewVM`                     | ~10%  | runtime startup | Per-`RunWith` VM struct + locals + frames + stack                                                                                    |
| `applyN`                            | ~8.5% | runtime exec    | Step 4-introduced batched dispatch; some retain copies still required by partial PAP/PrimVal/ConVal                                  |
| `Compiler.addMatchDesc`             | ~6%   | compile         | Pattern match descriptors                                                                                                            |
| `runtime.buildGlobalArray`          | ~6%   | runtime startup | Template clone for cached globals                                                                                                    |
| `pipelineCtx.computeModuleCacheKey` | ~6%   | compile         | String key construction                                                                                                              |
| `Compiler.allocLocalWithArity`      | ~3%   | compile         | Local var slot tracking                                                                                                              |
| `eval.IntVal (inline)`              | ~1.5% | runtime exec    | Boxing of large ints (small ints interned)                                                                                           |

### Cost categories rolled up

| Category                                   | Share |
| ------------------------------------------ | ----- |
| Compile (parse + check + optimize + emit)  | ~50%  |
| Runtime startup (NewVM + buildGlobalArray) | ~16%  |
| Runtime exec (apply paths + value boxing)  | ~30%  |
| Other (host stdlib + budget tracking)      | ~4%   |

The 2026-04-07 dispatch refactor moved the needle on runtime exec.
The next big block is **compile-side allocations**, which dominate
total runtime in cold workloads.

## Operating principles

### Always have a baseline

Before changing anything, run:

```sh
./scripts/perf-snapshot.sh main
```

After your changes, compare:

```sh
./scripts/perf-snapshot.sh HEAD
./scripts/perf-compare.sh main HEAD
```

If the change is meant to improve a specific benchmark, also profile
that benchmark directly:

```sh
./scripts/perf-profile.sh BenchmarkExecMapInsert50
```

### Use the right tier for the question

| Question                                           | Tier to use                                                            |
| -------------------------------------------------- | ---------------------------------------------------------------------- |
| "Did my runtime change regress something?"         | Tier 2 (warm exec) — compile noise drowns runtime signal in Tier 1     |
| "Did my compile change regress something?"         | Tier 3 (compile only)                                                  |
| "Does this matter for end users running the CLI?"  | Tier 1 (cold end-to-end)                                               |
| "Why is type checking slow on this program?"       | Tier 4 (check micro) + `perf-profile.sh` on the relevant compile bench |
| "Where do allocations come from in this hot loop?" | `perf-profile.sh` with `mem.prof`; pprof's `-list` is your friend      |

### GC noise mitigation

CPU profiles for short-running benchmarks are dominated by GC and
scheduler runtime samples. Mitigations:

- Use `PERF_BENCHTIME=10s` (or longer) for short benchmarks. The
  longer the run, the more user-code samples vs runtime overhead.
- For allocation profiles, use `-memprofilerate=1` to record every
  allocation (default samples 1 in 512 KiB, which misses small allocs
  entirely on micro-benches). `perf-profile.sh` does this by default.
- Re-run benchmarks at least 3 times (`PERF_COUNT=5` is the default
  for `perf-snapshot.sh`) and use `benchstat` for statistical
  significance — `~ (p=0.690)` means "no detectable change".
- For CPU profiles, also be skeptical of any single-sample claim
  under 5%. The GC kevent floor is around that magnitude.

### When CPU profiles are dominated by GC

Look at the **memory profile** instead. GC time is proportional to
allocation rate × heap size. If you reduce allocations and the CPU
profile still shows GC dominance, the GC pressure is coming from
something _other_ than the workload you're profiling — usually a
warmup phase or a cached structure that the bench isn't isolating.

`perf-profile.sh` runs with `-memprofilerate=1` so the memory
profile is precise even on micro-benches.

### Pre-compiled exec is the runtime ground truth

For runtime work, always look at the Tier 2 numbers first. Cold
end-to-end (Tier 1) is dominated by compile cost for the Map/Set
workloads (97%+ compile share), which means a 30% runtime
improvement might only show as 1% in Tier 1. The cold numbers are
the right metric for end-user experience but the wrong metric for
investigating runtime regressions.

## File and directory layout

```
scripts/
  perf-snapshot.sh   take a full snapshot, store as tmp/perf/<label>/
  perf-compare.sh    benchstat-diff two snapshots
  perf-profile.sh    deep CPU+alloc profile for one benchmark

tmp/perf/<label>/
  meta.txt           git rev, date, host, settings
  exec.txt           Tier 2 results
  end_to_end.txt     Tier 1 results
  compile.txt        Tier 3 results
  check.txt          Tier 4 (semantic) results
  parse.txt          Tier 4 (parse) results
  optimize.txt       Tier 4 (optimize) results
  runtime_micro.txt  Tier 4 (budget + types) results

tmp/perf/profile/<bench-slug>/
  bench.txt          bench output
  cpu.prof           CPU profile
  mem.prof           memory profile (alloc_space, rate=1)
  block.prof         blocking profile

docs/perf-overview.md   this document
```

## Reference snapshots

### Current state (snapshot `tmp/perf/current/`, commit `9c68906`)

**Tier 2 — Warm exec** (RunWith only, n=5, 2s benchtime):

| Benchmark                | sec/op  | B/op     | allocs/op |
| ------------------------ | ------- | -------- | --------- |
| `ExecSmall`              | 11.3 µs | 22.1 KiB | 11        |
| `ExecArray`              | 196 µs  | 71.8 KiB | 1096      |
| `ExecMapInsert50`        | 110 µs  | 74.5 KiB | 1085      |
| `ExecMutableMapInsert50` | 10.8 µs | 22.1 KiB | **11**    |
| `ExecSetAlgebra`         | 75 µs   | 60.3 KiB | 759       |
| `ExecArithmeticLoop`     | 1.65 ms | 393 KiB  | 10631     |

`ExecMutableMapInsert50` having only 11 allocs/iter is the
benchmark to beat for runtime efficiency: state mutation avoids
the per-step rebuild cost of immutable Map.

**Tier 3 — Compile only** (NewRuntime, n=5):

| Benchmark                   | sec/op      | allocs/op |
| --------------------------- | ----------- | --------- |
| `EngineNewRuntimeNoModules` | 79 µs       | 551       |
| `EngineNewRuntimePrelude`   | **1.28 ms** | **9229**  |
| `EngineCompileSmall`        | 1.27 ms     | 9229      |
| `EngineCompileLarge`        | 2.13 ms     | 13154     |

Prelude compile alone costs **1.2 ms / 9229 allocs** — that is the
floor for any program that imports Prelude. Comparing
`NewRuntimeNoModules` (79 µs) against `NewRuntimePrelude` (1.28 ms)
shows the entire ~16× gap is loading Prelude. The module cache
amortizes this within a single process, but each `NewRuntime` call
still pays the cost of evaluating Prelude's bindings.

**Tier 4 — Type checker scaling**:

| Benchmark             | sec/op  | allocs/op |
| --------------------- | ------- | --------- |
| `ResolveInstances10`  | 132 µs  | 1028      |
| `ResolveInstances50`  | 740 µs  | 6943      |
| `ResolveInstances100` | 2.00 ms | 21049     |
| `SuperclassDepth5`    | 106 µs  | 941       |
| `SuperclassDepth10`   | 180 µs  | 1596      |

Linear scaling in instance count and superclass depth — confirms
the inert-set + supersedes invariants. Watch these for any
unexpected jump.

### Cold vs warm decomposition

Comparing the same workload across tiers reveals the compile share:

| Workload              | Tier 1 (cold) | Tier 2 (warm) | compile share |
| --------------------- | ------------- | ------------- | ------------- |
| `*Small`              | 1.50 ms       | 11.3 µs       | **99.2%**     |
| `*Array`              | 1.92 ms       | 196 µs        | 89.8%         |
| `*Map`                | 2.20 ms       | (no warm yet) | —             |
| `*MapInsert50`        | 6.37 ms       | 110 µs        | **98.3%**     |
| `*MutableMapInsert50` | 3.68 ms       | 10.8 µs       | **99.7%**     |
| `*SetAlgebra`         | 2.72 ms       | 75 µs         | 97.2%         |

**This is the most important table in this document.** It says:
end-user wall time on these workloads is dominated by compile, not
runtime. After the 2026-04-07 dispatch refactor, runtime exec
optimization has reached the point of diminishing returns; the
biggest remaining lever is **compile-time work**.

### History (2026-04-07 arity-batched dispatch refactor)

Commits `35254ee..9c68906`. Cumulative geomean improvement vs the
pre-refactor baseline (`6ef77e0`):

| Tier                     | sec/op geomean | allocs/op geomean |
| ------------------------ | -------------- | ----------------- |
| Tier 1 (cold end-to-end) | -29%           | -3.5%             |
| Tier 2 (warm exec)       | -31.6%         | -8.97%            |

Single biggest mover: `ExecArray` -43.57% sec, -45.34% allocs.

See `memory/project_perf_baseline.md` for the commit-by-commit
breakdown.
