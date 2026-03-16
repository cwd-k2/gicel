# GICEL Performance Review

Date: 2026-03-16

Scope: repository-level static performance review with emphasis on runtime evaluation paths and stdlib primitives. This report is based on source inspection and existing tests. It does not include new benchmarks or profiling runs.

## Executive Summary

The evaluator core is generally well-shaped for performance. The trampoline in [internal/eval/eval.go](/Users/cwd-k2/Projects/gicel/internal/eval/eval.go) keeps deep evaluation off the Go stack, the runtime caches its builtin environment, and several rewrite rules already eliminate avoidable conversions.

The main performance risks are concentrated in stdlib primitives, especially:

- list operations that linearize whole lists into Go slices and then rebuild lists
- string operations that materialize `[]rune` eagerly
- map/set operations that flatten trees into intermediate slices before building results
- allocation accounting that is correct in spirit but sometimes charges for whole-input temporary structures, causing earlier-than-expected allocation failures on otherwise modest outputs

These issues are not abstract. Several common operations have avoidable `O(n)` temporary allocation even when the logical result is small or could share structure.

## Method

Reviewed files:

- [internal/eval/eval.go](/Users/cwd-k2/Projects/gicel/internal/eval/eval.go)
- [internal/eval/limit.go](/Users/cwd-k2/Projects/gicel/internal/eval/limit.go)
- [internal/stdlib/list.go](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go)
- [internal/stdlib/str.go](/Users/cwd-k2/Projects/gicel/internal/stdlib/str.go)
- [internal/stdlib/slice.go](/Users/cwd-k2/Projects/gicel/internal/stdlib/slice.go)
- [internal/stdlib/map.go](/Users/cwd-k2/Projects/gicel/internal/stdlib/map.go)
- [internal/stdlib/set.go](/Users/cwd-k2/Projects/gicel/internal/stdlib/set.go)
- [internal/stdlib/stream.go](/Users/cwd-k2/Projects/gicel/internal/stdlib/stream.go)

Signals used:

- asymptotic behavior
- intermediate allocation shape
- reuse or loss of structural sharing
- interaction with `ChargeAlloc`
- existing optimization rules already present in the codebase

## Healthy Areas

### Evaluator structure

The evaluator has a strong baseline:

- [internal/eval/eval.go:69](/Users/cwd-k2/Projects/gicel/internal/eval/eval.go#L69) uses a trampoline loop to avoid stack growth across tail positions.
- [internal/eval/eval.go:51](/Users/cwd-k2/Projects/gicel/internal/eval/eval.go#L51) caches the applier closure per execution.
- [internal/eval/eval.go:141](/Users/cwd-k2/Projects/gicel/internal/eval/eval.go#L141) trims closure environments when free-variable sets are available.
- [internal/eval/eval.go:212](/Users/cwd-k2/Projects/gicel/internal/eval/eval.go#L212) uses knot-tying for `letrec` and includes a fixpoint-specific optimization.

This means the current critical path is not the evaluator dispatch itself. The heavier costs sit in primitives that leave the Core world and construct host-side containers.

### Existing fusion direction is good

The project already has useful rewrite rules:

- [internal/stdlib/slice.go:29](/Users/cwd-k2/Projects/gicel/internal/stdlib/slice.go#L29) fuses `map . map`
- [internal/stdlib/slice.go:51](/Users/cwd-k2/Projects/gicel/internal/stdlib/slice.go#L51) fuses `foldr` after `map`
- [internal/stdlib/slice.go:75](/Users/cwd-k2/Projects/gicel/internal/stdlib/slice.go#L75) removes `toList/fromList` roundtrips
- [internal/stdlib/str.go:38](/Users/cwd-k2/Projects/gicel/internal/stdlib/str.go#L38) removes `fromRunes/toRunes` roundtrips

This is the right direction. The list and map/set layers have not yet been pushed as far.

## Findings

### 1. `List.take` and `List.drop` do full-list materialization

Relevant code:

- [internal/stdlib/list.go:188](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L188)
- [internal/stdlib/list.go:209](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L209)
- [internal/stdlib/list.go:133](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L133)
- [internal/stdlib/list.go:339](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L339)

Current behavior:

- `take` calls `listToSlice`, which traverses the entire list into `[]eval.Value`
- `take` then slices that temporary array and rebuilds a fresh list with `buildList`
- `drop` does the same full traversal and rebuild

Performance impact:

- `take 1 xs` is `O(len(xs))` traversal with `O(len(xs))` temporary slice, even though the logical result needs only one cons cell
- `drop n xs` loses the original tail structure and pays for complete materialization even though it could return the tail pointer directly after walking `n` steps
- allocation charging is based on the temporary slice length, so large inputs may trip allocation limits even when the output is small

Expected better shape:

- `take`: traverse until `min(n, prefix length)`, collect only the needed prefix, then build only that prefix
- `drop`: walk `n` cons cells and return the remaining tail directly, preserving sharing

Severity: high

This is a common API path and the current implementation is materially more expensive than necessary.

### 2. Several list functions rely on whole-list roundtrips

Relevant code:

- [internal/stdlib/list.go:101](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L101)
- [internal/stdlib/list.go:269](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L269)
- [internal/stdlib/list.go:313](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L313)
- [internal/stdlib/list.go:419](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L419)
- [internal/stdlib/list.go:520](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L520)
- [internal/stdlib/list.go:560](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L560)

Patterns observed:

- `concat` materializes the entire left list first
- `reverse` materializes then reverses then rebuilds
- `unzip` materializes the whole input before splitting
- `sortBy` materializes before merge sort
- `unfoldr` accumulates into a slice and rebuilds at the end
- `iterateN` accumulates into a slice and rebuilds at the end

Assessment:

- `sortBy` likely needs an indexed scratch representation, so slice conversion is defensible there
- `reverse` can be implemented as a direct cons-building loop with no intermediate `[]eval.Value`
- `concat` only needs to copy the left spine, but the current implementation first builds a slice of its elements
- `unfoldr` and `iterateN` could construct a reversed list incrementally and reverse once, or use a mutable tail pointer internally if the project accepts localized imperative construction
- `unzip` currently allocates three aggregate structures: temporary slice, two result slices, then two rebuilt lists

Severity: medium to high

These are not all equally urgent, but together they show that `Std.List` is currently optimized for implementation simplicity over data-path efficiency.

### 3. String primitives eagerly allocate `[]rune`

Relevant code:

- [internal/stdlib/str.go:132](/Users/cwd-k2/Projects/gicel/internal/stdlib/str.go#L132)
- [internal/stdlib/str.go:179](/Users/cwd-k2/Projects/gicel/internal/stdlib/str.go#L179)
- [internal/stdlib/str.go:198](/Users/cwd-k2/Projects/gicel/internal/stdlib/str.go#L198)
- [internal/stdlib/str.go:296](/Users/cwd-k2/Projects/gicel/internal/stdlib/str.go#L296)

Current behavior:

- `lengthStrImpl` computes `len([]rune(s))`
- `charAtImpl` computes `runes := []rune(s)` even for a single index lookup
- `substringImpl` computes `[]rune(s)` before extracting a range
- `fromRunesImpl` converts a list to slice, then creates `[]rune`, then creates a string

Performance impact:

- every call allocates proportional to input byte length or rune count
- long ASCII strings pay a full UTF-8 decode and full secondary array allocation
- allocation charging again tracks full conversion cost, not only the output

Better options:

- `length`: use `utf8.RuneCountInString(s)`
- `charAt`: iterate with `for _, r := range s` until the requested index
- `substring`: scan rune boundaries once and slice the original string by byte offsets
- `fromRunes`: the current shape is acceptable if the input is already a list of runes, but it still inherits the `listToSlice` cost

Severity: high

These are standard library string operations that users will naturally reach for. The current implementation will scale poorly on large text inputs.

### 4. `split` and `join` allocate more than necessary

Relevant code:

- [internal/stdlib/str.go:252](/Users/cwd-k2/Projects/gicel/internal/stdlib/str.go#L252)
- [internal/stdlib/str.go:272](/Users/cwd-k2/Projects/gicel/internal/stdlib/str.go#L272)

Current behavior:

- `split` creates `[]string` via `strings.Split`, then rebuilds a list
- `join` traverses the input list into `[]string`, then calls `strings.Join`

Assessment:

- these operations inherently need aggregate work, so some allocation is unavoidable
- however, both are currently strict two-stage conversions
- for `join`, charging only `len(strs) * costSlotSize` underestimates the output string allocation
- for `split`, charging `len(s)` bytes plus list overhead is directionally reasonable but still coupled to intermediate strategy

Severity: medium

The bigger issue is not asymptotic complexity but high constant factors and somewhat inconsistent allocation accounting.

### 5. `Map.toList`, `Set.toList`, and `Map.unionWith` flatten trees into slices

Relevant code:

- [internal/stdlib/map.go:358](/Users/cwd-k2/Projects/gicel/internal/stdlib/map.go#L358)
- [internal/stdlib/map.go:441](/Users/cwd-k2/Projects/gicel/internal/stdlib/map.go#L441)
- [internal/stdlib/set.go:82](/Users/cwd-k2/Projects/gicel/internal/stdlib/set.go#L82)

Current behavior:

- `mapToList` uses `avlToList` to build a full `[]eval.Value` of tuple records, then calls `buildList`
- `setToList` uses `collectKeys` to build `[]eval.Value`, then `buildList`
- `mapUnionWith` first flattens the second map to `pairs`, then for each pair does `lookup` and `insert` into the result tree

Performance impact:

- extra full-tree traversal into temporary slices
- extra temporary memory proportional to collection size
- `unionWith` adds a large constant factor on top of its `O(m log(n+m))` tree work

Better options:

- build result lists directly during in-order traversal
- for `mapToList`, use a reverse in-order traversal that conses directly, avoiding the temporary slice
- for `setToList`, same approach
- for `unionWith`, a more advanced tree merge would be ideal, but even replacing `avlToList` with direct traversal into insertions would remove one full temporary structure

Severity: medium

This matters most once `Map` and `Set` grow beyond small utility sizes.

### 6. Allocation charging in stdlib is useful but not yet calibrated to operation shape

Relevant code:

- [internal/eval/limit.go:105](/Users/cwd-k2/Projects/gicel/internal/eval/limit.go#L105)
- [internal/stdlib/stdlib.go:18](/Users/cwd-k2/Projects/gicel/internal/stdlib/stdlib.go#L18)

Assessment:

Adding `ChargeAlloc` to primitives is the right mechanism because evaluator-visible allocation does not cover Go-side host slices and strings. The problem is not the existence of charging. The problem is that some primitives are now paying for implementation artifacts:

- `take` and `drop` charge for full temporary `[]eval.Value`
- `length`, `charAt`, and `substring` charge for full `[]rune`
- `mapToList` and `setToList` charge for list output but also implicitly rely on temporary slices

Consequences:

- programs can hit allocation limits because of hidden temporary representation choices
- user-visible limits become sensitive to implementation strategy rather than semantic output size

Severity: high

This is now part of external behavior, not just internal efficiency.

### 7. Some hot paths are already better-shaped and should be used as the standard

Relevant code:

- [internal/stdlib/list.go:230](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L230)
- [internal/stdlib/list.go:348](/Users/cwd-k2/Projects/gicel/internal/stdlib/list.go#L348)
- [internal/stdlib/stream.go:61](/Users/cwd-k2/Projects/gicel/internal/stdlib/stream.go#L61)

Positive examples:

- `indexImpl` walks the list directly with no temporary aggregate
- `dropWhileImpl` returns the surviving tail directly and preserves structure
- `dropSImpl` for streams advances and returns the suffix directly

These functions are useful design references for refactoring `drop`, `take`, and several map/list conversions.

## Priority Recommendations

### Priority 0: fix semantics-distorting costs

1. Rewrite `List.drop` to walk `n` steps and return the remaining tail without `listToSlice`.
2. Rewrite `List.take` to stop after `n` elements instead of traversing the entire list.
3. Replace `[]rune` in `lengthStrImpl` with `utf8.RuneCountInString`.
4. Rewrite `charAtImpl` and `substringImpl` to scan rune boundaries without whole-string rune slices.

Expected result:

- lower latency on large inputs
- much lower temporary allocation
- allocation limits reflect result size and consumed prefix more closely

### Priority 1: reduce avoidable aggregate conversions

1. Rewrite `reverseImpl` to build the reversed list directly.
2. Rewrite `mapToListImpl` and `setToListImpl` to cons during traversal.
3. Remove intermediate `pairs` in `mapUnionWithImpl` if a direct traversal path is acceptable.
4. Revisit `unzipImpl`, `unfoldrImpl`, and `iterateNImpl` to avoid slice-first construction where practical.

### Priority 2: extend optimizer strategy

1. Add list roundtrip elimination where safe, analogous to slice and string rules.
2. Consider rewrites around `fromSlice`/`toSlice` and `fromRunes`/list conversions if those patterns appear in optimized Core.
3. Add targeted microbenchmarks before and after each rewrite so future regressions are measurable.

## Suggested Benchmark Matrix

No new benchmarks were added in this pass, but these are the ones worth adding first:

- `BenchmarkListTakeSmallPrefixLargeList`
- `BenchmarkListDropLargePrefixLargeList`
- `BenchmarkListReverseLarge`
- `BenchmarkStringLengthASCII`
- `BenchmarkStringCharAtLateIndex`
- `BenchmarkSubstringSmallWindowLargeString`
- `BenchmarkMapToListLarge`
- `BenchmarkMapUnionWithOverlap`

Each benchmark should capture:

- `ns/op`
- `B/op`
- `allocs/op`

## Risk Notes

- Some refactors will change structural sharing. That is good for `drop`, but any code assuming deep copies in the current implementation should be reviewed.
- For strings, byte-slicing by rune boundary must be implemented carefully to preserve Unicode correctness.
- If allocation accounting is meant to approximate actual Go heap use rather than semantic output size, document that explicitly. Right now the code suggests user-facing resource control, so implementation artifacts matter.

## Conclusion

The codebase already has a solid evaluation core. The main performance work should now target stdlib primitives that currently choose convenience conversions over direct structural operations.

The fastest wins are:

1. remove full-list materialization from `take` and `drop`
2. remove eager `[]rune` allocation from string primitives
3. eliminate tree-to-slice-to-list pipelines in map/set enumeration

Those changes should improve latency, reduce memory churn, and make allocation limits behave more predictably for users.
