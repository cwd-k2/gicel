# Potential Bug Review

Date: 2026-03-19

Scope:

- Current repository state
- Focus on bugs that are still reproducible in the current codebase
- Prioritize behavioral issues over structural cleanup notes

This review is intentionally evidence-driven.
It includes only issues that were either confirmed by direct code inspection,
reproduced with targeted tests, or both.

## Executive Judgment

The mainline test suite is healthy.

`go test ./...` passes across the repository.

The remaining bug surface is concentrated in two areas:

- parser separator and recovery behavior
- type family reduction worst-case behavior

The parser issues are mostly quality and correctness problems in error handling:
it often builds a usable AST while still emitting spurious syntax errors, and its
top-level recovery can swallow valid declarations after malformed ones.

The type family issue is more serious.
There is still a confirmed path to exponential reduction blowup in the checker.

## Findings

## 1. High: type family reduction still has an exponential blowup path

Severity: High

Status:

- unresolved
- confirmed in current code by inspection
- still documented in probe tests as a known skipped bug

Relevant code:

- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L147)
- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L161)
- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L163)
- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L184)
- [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L186)
- [type_family_probe_test.go](/Users/cwd-k2/Projects/gicel/internal/check/type_family_probe_test.go#L346)

Problem:

`reduceFamilyAppsN` recursively reduces both arguments of expanded type results.
For a family such as `Grow a = Pair (Grow a) (Grow a)`, each successful reduction
duplicates future work.

The current `maxReductionTypeSize` guard only checks the immediate RHS produced by
`ReduceTyFamily`.
It does not bound the total recursive expansion work performed afterward.

Why it matters:

- type checking can become exponentially expensive
- the checker can effectively hang on adversarial or accidental family definitions
- the current depth counter is not enough to prevent branching explosion

Evidence:

- [type_family_probe_test.go](/Users/cwd-k2/Projects/gicel/internal/check/type_family_probe_test.go#L356) is still explicitly skipped as a known bug
- the recursive structure in [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L163) and [reduce.go](/Users/cwd-k2/Projects/gicel/internal/check/family/reduce.go#L186) still re-enters the expanded result

Recommended next step:

Choose a hard bound on total reduction work, not only per-step RHS size.
Examples:

- maintain a global node/work budget for a full normalization pass
- memoize post-expansion subtrees more aggressively
- stop recursively reducing freshly-expanded branches once the work budget is exhausted

## 2. Medium: newline-separated `do` statements still emit spurious syntax errors

Severity: Medium

Status:

- unresolved
- reproduced in current code

Relevant code:

- [parse_expr.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_expr.go#L325)
- [parse_expr.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_expr.go#L334)

Problem:

`parseDo` only recognizes `;` as a statement separator inside braces.
When statements are separated by newlines, the parser often still recovers enough
to build multiple statements, but it emits syntax errors during the process.

Why it matters:

- valid-looking `do` blocks produce incorrect diagnostics
- editor and CLI feedback become noisy and misleading
- this weakens confidence in parser error reporting

Evidence:

Running:

```sh
go test -tags probe ./internal/syntax/parse -run TestProbeD_NewlineInDoBlock -v
```

produces errors at the second statement:

- `expected expression`
- `unexpected token in do-block`

while the same test logs that the do-block still parsed `3` statements.

Recommended next step:

Make separator handling inside `do` blocks explicitly choose between:

- semicolons only, with parser tests and docs aligned to that rule
- semicolons or newline boundaries, implemented consistently

Right now the implementation and the apparent user expectation are not aligned.

## 3. Medium: class, instance, and GADT bodies still mishandle newline separators

Severity: Medium

Status:

- unresolved
- reproduced in current code

Relevant code:

- [parse_class.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_class.go#L190)
- [parse_class.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_class.go#L329)
- [parse_class.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_class.go#L349)
- [parse_decl.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_decl.go#L106)
- [parse_decl.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_decl.go#L122)

Problem:

The same separator issue appears in several brace-delimited declaration bodies.
Newline-separated entries are partially parsed, but spurious errors are emitted
because the loops only consume semicolons as separators.

This affects:

- class method lists
- instance method lists
- GADT constructor lists

Why it matters:

- the parser reports false errors on structurally simple declarations
- the behavior is inconsistent across top-level declarations versus nested bodies
- the AST may be usable while diagnostics claim the input is broken

Evidence:

All of the following still reproduce:

```sh
go test -tags probe ./internal/syntax/parse -run TestProbeD_NewlineInClassBody -v
go test -tags probe ./internal/syntax/parse -run TestProbeD_NewlineInInstanceBody -v
go test -tags probe ./internal/syntax/parse -run TestProbeD_GADTMultipleNewline -v
```

Observed outcomes:

- class body: `expected identifier`
- instance body: `expected method name or 'type' in instance declaration`, then follow-on errors
- GADT body: `expected uppercase identifier`

In each case, the parser still recovers enough structure to log the expected number
of members/constructors.

Recommended next step:

Factor brace-body separator handling into one shared helper and use it across:

- `parseDo`
- class bodies
- instance bodies
- GADT bodies

That avoids fixing the same separator bug in multiple local loops.

## 4. Medium: top-level error recovery still swallows valid declarations after malformed ones

Severity: Medium

Status:

- unresolved
- reproduced in current code

Relevant code:

- [parser.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parser.go#L69)
- [parse_decl.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_decl.go#L28)
- [parse_decl.go](/Users/cwd-k2/Projects/gicel/internal/syntax/parse/parse_decl.go#L363)

Problem:

When a declaration such as `bad :=` has no expression body, the parser enters
expression parsing instead of synchronizing to the next declaration boundary.
That makes subsequent valid declarations vulnerable to being consumed as part of
error recovery.

Why it matters:

- one malformed declaration can hide later valid code
- diagnostics point at later declarations that are not actually broken
- parser recovery quality is materially worse for multi-error files

Evidence:

Running:

```sh
go test -tags probe ./internal/syntax/parse -run TestProbeD_MultipleSyntaxErrors -v
```

still reports:

- `recovered 1 good declarations out of 5 total`

and emits bogus `expected declaration` errors on `good2 := 99`.

Recommended next step:

Add explicit synchronization after a failed value definition body parse.
At minimum, recovery should stop at the next top-level declaration boundary rather
than continuing to consume tokens as if they belonged to the broken expression.

## Verification Notes

The following commands were used during this review:

- `go test ./...`
- `go test -tags probe ./internal/check`
- `go test -tags probe ./internal/syntax/parse`
- targeted probe test runs for the specific findings above

`internal/eval` probe tests were not used as evidence for current bugs because
they do not currently build against the post-refactor evaluator API.
