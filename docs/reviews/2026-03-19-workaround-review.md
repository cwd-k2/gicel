# Workaround And Leftover Review

Date: 2026-03-19

Scope:

- Current repository state
- Intentional workarounds still present in code or tests
- Latent bugs tolerated by current invariants
- Known theoretical limits that are currently handled operationally
- Leftover inventory debt in tests, probes, benchmarks, and docs
- Test strategy for keeping these areas visible while fixes are in flight

This review is broader than a bug review.
The question here is not only "what is broken now" but also:

- what behavior depends on a workaround
- what is intentionally best-effort rather than complete
- what remains safe only because another invariant currently holds
- what is already fixed in implementation but still recorded as if unresolved

## Executive Judgment

The repository is in a better state than a typical "workaround-heavy" codebase.
Most of the important compromises are documented, and several formerly known
gaps have already been fixed.

However, there is still a real backlog of residual risk.

The most important current leftovers are:

- tuple `Eq` and `Ord` runtime support is still absent, and tests actively route around it
- fundep improvement is intentionally advisory, which is acceptable but must stay explicitly tested
- bare record syntax in type position still does not mean the same thing as record syntax in expression position
- `SubstMany` remains non-deterministic for dependent substitutions, with safety relying on a separate invariant
- the roadmap still records three design boundaries that are only safe because of current operational workarounds

Separately, some of the "known issue" inventory is now stale.
That is lower severity than a semantic bug, but it directly affects review quality,
because stale workaround comments make it harder to tell which limitations are still real.

## Findings

## 1. Medium: tuple `Eq` and `Ord` are still unsupported at runtime

Severity: Medium

Status:

- unresolved
- explicitly acknowledged in tests
- current tests use substitutes as a workaround

Relevant code:

- [typeclass_test.go](/Users/cwd-k2/Projects/gicel/internal/engine/typeclass_test.go#L225)
- [typeclass_test.go](/Users/cwd-k2/Projects/gicel/internal/engine/typeclass_test.go#L516)

Problem:

The runtime still does not support tuple evidence for `Eq (a, b)` and
`Ord (a, b)`.
The tests say this directly and avoid tuple cases by using `List` or `Maybe`
instead.

Why it matters:

- the typeclass surface is not uniform across algebraic containers and tuples
- tests currently prove only the workaround path, not the missing direct path
- user expectations are likely to be shaped by the rest of Prelude support, so this is a visible language-level hole

Current workaround:

- `Eq` coverage uses `List`
- `Ord` coverage uses `Maybe`

Assessment:

This is a real leftover, not just an internal implementation detail.
It should remain visible until runtime evidence/record interaction is completed.

## 2. Medium: fundep improvement is intentionally best-effort rather than mandatory

Severity: Medium

Status:

- intentional
- documented in code
- regression-tested

Relevant code:

- [resolve.go](/Users/cwd-k2/Projects/gicel/internal/check/resolve.go#L277)
- [regression_test.go](/Users/cwd-k2/Projects/gicel/internal/check/regression_test.go#L225)

Problem:

Fundep improvement only contributes information when the inferred `to` side can
be unified cheaply and safely.
If that unification fails, the checker silently skips the improvement.

Why it matters:

- this is not full logical fundep exploitation
- behavior depends on another path being sufficient to resolve the program
- future refactors could accidentally harden this into an error path and reject valid programs

Assessment:

This is an acceptable design choice for now, but it is still a deliberate
capability limit.
It belongs in the leftover inventory because correctness currently depends on
the distinction between advisory improvement and mandatory solving staying intact.

## 3. Medium: bare record syntax in type position still means "row", not `Record`

Severity: Medium

Status:

- unresolved
- user-visible
- currently documented only through probe tests

Relevant code:

- [unify_probe_test.go](/Users/cwd-k2/Projects/gicel/internal/check/unify_probe_test.go#L194)

Problem:

`{}` and `{ l: T }` in expression position create records.
In type position, the same syntax produces a raw row, not `Record <row>`.

Why it matters:

- the syntax is surprising and asymmetric
- users must know to write `Record { ... }` or use `()` for the empty record case
- this is the kind of inconsistency that tends to survive because there is a workaround, even though the surface language is harder to learn

Current workaround:

- use `Record { ... }` in annotations
- use `()` for the empty record type

Assessment:

This is not catastrophic, but it is a persistent user-facing inconsistency and
should stay in the review inventory until the language surface is intentionally
standardized one way or the other.

## 4. Medium: `SubstMany` is still non-deterministic for dependent substitutions

Severity: Medium

Status:

- unresolved
- explicitly documented as a latent bug
- currently masked by a separate invariant

Relevant code:

- [type_family_pathological_test.go](/Users/cwd-k2/Projects/gicel/internal/check/type_family_pathological_test.go#L536)

Problem:

`types.SubstMany` iterates over a Go map.
For dependent substitutions such as `{a -> b, b -> Int}`, the result depends on
iteration order and therefore is not a true simultaneous substitution.

Why it matters:

- behavior is non-deterministic in principle
- the current safety story depends on type-family pattern matching only generating independent substitutions
- if another caller starts using `SubstMany` with dependent substitutions, the bug becomes user-visible immediately

Assessment:

This is the clearest example of a latent bug that is "safe only because another
part of the implementation currently behaves nicely".
It should remain explicitly tracked until the substitution semantics are made deterministic.

## 5. Medium: several design boundaries are still handled by operational workarounds

Severity: Medium

Status:

- intentional
- documented in the roadmap
- not currently failing in known programs

Relevant code:

- [roadmap.md](/Users/cwd-k2/Projects/gicel/docs/roadmap.md#L44)

Problem:

The roadmap records three known theoretical boundaries where the current
implementation remains practical because of a workaround or restricted execution model:

- double grading
- type family / row unification scheduling
- evidence fiber crossing

Why it matters:

- these are not ordinary TODOs
- they define the edge of the current checker architecture
- future feature work can cross these boundaries without a local code diff looking dangerous

Assessment:

These should remain part of the active engineering inventory, not just design notes.
They are exactly the kind of issue that can be forgotten because current programs
do not trigger them.

## 6. Low: integer overflow still silently wraps at runtime

Severity: Low

Status:

- unresolved
- probe-documented
- semantics currently inherited from Go

Relevant code:

- [stdlib_test.go](/Users/cwd-k2/Projects/gicel/tests/probe/stdlib_test.go#L395)

Problem:

`Int` arithmetic currently wraps on overflow.
The probe tests describe this as a bug candidate rather than a settled design decision.

Why it matters:

- wrapping is unsurprising for Go implementers but may be surprising for users treating GICEL as a safe embedded language
- until the semantics are explicitly specified, tests are documenting implementation behavior rather than language intent

Assessment:

This is lower priority than the checker/runtime issues above, but it should be
resolved at the specification level.
Either "wraps intentionally" or "overflow is trapped" should become explicit.

## 7. Medium: some coverage still depends on workaround paths instead of direct paths

Severity: Medium

Status:

- mixed
- some items are active limitations
- some items are already fixed but the tests still preserve older workaround narratives

Relevant code:

- [stress_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/stress_test.go#L752)
- [type_family_probe_test.go](/Users/cwd-k2/Projects/gicel/internal/check/type_family_probe_test.go#L346)
- [deep_pattern_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/deep_pattern_test.go#L552)
- [deep_pattern_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/deep_pattern_test.go#L758)
- [deep_pattern_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/deep_pattern_test.go#L995)

Problem:

There are at least three different leftover patterns in test inventory:

1. A real missing capability still covered only indirectly.
2. A known bug test that is still skipped.
3. A formerly missing capability that is now fixed, but the test narrative still centers the old workaround.

Concrete examples:

- `BenchmarkStressCompile` still does not form a fully trustworthy performance sentinel because the stress harness does not reliably preload what each program expects.
- the exponential type-family blowup probe still carries an explicit `KNOWN BUG` skip
- the deep-pattern tests keep "workaround" framing even where the direct path now succeeds

Assessment:

This is inventory debt rather than a single semantic bug.
It matters because stale or workaround-centered tests make it harder to know what
the repository actually guarantees today.

## 8. Low: stale comments and release notes are now part of the leftover problem

Severity: Low

Status:

- mixed
- some comments are current
- some appear to lag behind implementation

Relevant code:

- [CHANGELOG.md](/Users/cwd-k2/Projects/gicel/CHANGELOG.md#L85)
- [deep_pattern_test.go](/Users/cwd-k2/Projects/gicel/tests/stress/deep_pattern_test.go#L552)

Problem:

Not every leftover is executable.
Some are documentation artifacts that still describe old constraints or old
workarounds after behavior has changed.

The clearest examples during this review were:

- changelog text that still reads as if multiplicity enforcement is not active
- tests whose commentary still foregrounds workaround paths even though the direct behavior has been fixed

Assessment:

This is low severity but high leverage cleanup.
If the repository wants review documents like this one to stay useful, the stale
inventory needs periodic pruning.

## 9. Low: negative allocation limits currently disable the limit entirely

Severity: Low

Status:

- unresolved
- probe-documented
- likely an edge-case semantics hole rather than an intended API guarantee

Relevant code:

- [eval_invariants_probe_test.go](/Users/cwd-k2/Projects/gicel/internal/eval/eval_invariants_probe_test.go#L77)

Problem:

`allocLimit > 0` is used as the enable guard for allocation enforcement.
As a result, `SetAllocLimit(-1)` disables the limit rather than acting as
"already exceeded" or being rejected as invalid input.

Why it matters:

- negative values are accepted as a control channel even though that does not appear to be a deliberate public contract
- edge-case budget semantics are exactly the sort of thing that later gets depended on accidentally
- probes currently document the behavior, but there is no clear statement that it is intentional

Assessment:

This is lower priority than the language and checker leftovers, but it is a real
omission from the previous draft.
It should either be specified as "negative disables" or rejected at the API boundary.

## 10. Low: some skipped tests are migration leftovers, not semantic decisions

Severity: Low

Status:

- unresolved
- explicitly skipped
- caused by package reorganization rather than product semantics

Relevant code:

- [type_family_probe_test.go](/Users/cwd-k2/Projects/gicel/internal/check/type_family_probe_test.go#L298)

Problem:

At least one probe test is skipped because the underlying implementation moved to
another package and the old test was never migrated.
This is different from a pending semantic feature: the behavior may still work,
but the repository no longer has the intended coverage.

Why it matters:

- skipped migration leftovers create false confidence about coverage
- they blend together with "known bug" skips even though the remediation is different
- they should usually be fixed by moving the test, not by changing the implementation

Assessment:

This is also a real omission from the previous draft.
It belongs in the leftover inventory because missing coverage from test drift is
still a repository health problem.

## Test Strategy

The test plan should distinguish four categories.

### A. Sentinel tests for accepted current limitations

These tests should pass today and continue passing until the design decision changes.

Targets:

- fundep best-effort behavior
- roadmap-boundary behavior that is intentionally supported by current operational strategy
- integer overflow semantics, once those semantics are explicitly chosen

Recommended shape:

- narrow regression tests with comments stating "intentional current behavior"
- avoid wording that makes the limitation sound accidental if it is not

### B. Pending tests for capabilities that are still missing

These tests should make the missing behavior explicit without pretending it works today.

Targets:

- tuple `Eq`
- tuple `Ord`
- exponential type-family reduction protection, once the reduction strategy is fixed

Recommended shape:

- either `t.Skip` with a precise blocking reason
- or probe tests that assert the current failure mode while linking to the intended future behavior

Important rule:

Do not hide these by testing only substitutes such as `List` or `Maybe`.
Keep the substitute tests, but add direct pending coverage so the true gap remains visible.

### C. Invariant tests for latent bugs that are only safe under current usage

These tests should defend the assumptions that currently keep a latent bug from surfacing.

Targets:

- `SubstMany` only receiving independent substitutions from type-family pattern matching
- single-pass family reduction assumptions that keep evidence fiber crossing out of unsupported territory

Recommended shape:

- one test documenting the latent bug directly
- one or more tests asserting the invariant that currently keeps production behavior safe

This category is important because it tells future maintainers:
"you may not see a failing program today, but this invariant is load-bearing."

### D. Inventory-cleanup tests and triage

These are not semantic tests.
They are repository hygiene work that prevents the leftover inventory from drifting.

Targets:

- stale `KNOWN BUG` skips
- stale migration-driven `t.Skip` cases
- stale workaround comments in tests
- stale changelog or guide text that no longer matches implementation
- broken benchmark entry points that still appear to provide coverage

Recommended process:

1. whenever a workaround is removed, update the associated test comment in the same change
2. whenever a `t.Skip` remains for more than one release, re-triage it explicitly
3. whenever a benchmark fails before measuring anything real, treat that as a benchmark bug rather than "missing data"

## Recommended Near-Term Actions

1. Add direct pending runtime tests for tuple `Eq` and `Ord`.
2. Keep the existing fundep best-effort regressions and label them as intentional capability bounds.
3. Add a deterministic unit test for `SubstMany` once its semantics are fixed; until then, keep the pathological probe and add an invariant test on independent substitutions.
4. Convert the bare-row syntax inconsistency into an explicit parser/checker contract test rather than leaving it probe-only.
5. Re-triage stale workaround narratives in `tests/stress/deep_pattern_test.go` and stale wording in `CHANGELOG.md`.
6. Treat `BenchmarkStressCompile` as broken coverage until the harness reliably sets up each stress program.
7. Decide whether negative allocation limits are valid API input and test the chosen behavior directly.
8. Migrate skipped reorg leftovers such as the stuck-family reactivation probe into the package that can actually exercise them.

## Bottom Line

The repository does not appear to be hiding a large number of completely unknown
workarounds.
Most of the important leftovers are already visible somewhere.

The real problem is different:

- some limitations are only visible indirectly
- some latent bugs are protected only by invariants
- some already-fixed areas still look unresolved because comments and tests were not cleaned up

That means the right next step is not only more fixing, but also sharper inventory discipline.
The codebase needs direct tests for real gaps, invariant tests for tolerated risks,
and periodic pruning of stale workaround narratives.
